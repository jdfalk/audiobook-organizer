// file: internal/database/pebble_store.go
// version: 1.12.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

package database

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	ulid "github.com/oklog/ulid/v2"
)

// PebbleStore implements the Store interface using PebbleDB (LSM key-value store)
//
// Key Schema:
// - author:<id>                -> Author JSON
// - author:name:<name>         -> author_id (for lookups)
// - series:<id>                -> Series JSON
// - series:name:<name>:<author_id> -> series_id (for lookups)
// - book:<id>                  -> Book JSON
// - book:path:<path>           -> book_id (for lookups)
// - book:series:<series_id>:<id> -> book_id (for series queries)
// - book:author:<author_id>:<id> -> book_id (for author queries)
// - import_path:<id>           -> ImportPath JSON
// - import_path:path:<path>    -> import_path_id (for lookups)
// - operation:<id>             -> Operation JSON
// - operationlog:<operation_id>:<timestamp>:<seq> -> OperationLog JSON
// - preference:<key>           -> UserPreference JSON
// - playlist:<id>              -> Playlist JSON
// - playlist:series:<series_id> -> playlist_id
// - playlistitem:<playlist_id>:<position> -> PlaylistItem JSON
// - counter:author             -> next author ID
// - counter:series             -> next series ID
// - counter:book               -> next book ID
// - counter:import_path        -> next import path ID
// - counter:operationlog       -> next operation log ID
// - counter:playlist           -> next playlist ID
// - counter:playlistitem       -> next playlist item ID
// - metadata_state:<book_id>:<field> -> MetadataFieldState JSON

type PebbleStore struct {
	db *pebble.DB
}

// NewPebbleStore creates a new PebbleDB store
func NewPebbleStore(path string) (*PebbleStore, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to open PebbleDB: %w", err)
	}

	store := &PebbleStore{db: db}

	if err := store.migrateImportPathKeys(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate import path keys: %w", err)
	}

	// Initialize counters if they don't exist
	counters := []string{"author", "series", "book", "import_path", "operationlog", "playlist", "playlistitem", "preference"}
	for _, counter := range counters {
		key := fmt.Sprintf("counter:%s", counter)
		if _, closer, err := db.Get([]byte(key)); err == pebble.ErrNotFound {
			if err := db.Set([]byte(key), []byte("1"), pebble.Sync); err != nil {
				db.Close()
				return nil, fmt.Errorf("failed to initialize counter %s: %w", counter, err)
			}
		} else if err == nil {
			closer.Close()
		} else {
			db.Close()
			return nil, fmt.Errorf("failed to check counter %s: %w", counter, err)
		}
	}

	return store, nil
}

// Close closes the database
func (p *PebbleStore) Close() error {
	return p.db.Close()
}

// Helper functions

func (p *PebbleStore) nextID(counter string) (int, error) {
	key := []byte(fmt.Sprintf("counter:%s", counter))

	value, closer, err := p.db.Get(key)
	if err != nil {
		return 0, err
	}
	defer closer.Close()

	id, err := strconv.Atoi(string(value))
	if err != nil {
		return 0, err
	}

	nextID := id + 1
	if err := p.db.Set(key, []byte(strconv.Itoa(nextID)), pebble.Sync); err != nil {
		return 0, err
	}

	return id, nil
}

func newULID() (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// migrateImportPathKeys renames legacy library* keys and counters to import_path* equivalents.
// Safe to run multiple times and before counter initialization.
func (p *PebbleStore) migrateImportPathKeys() error {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("library:"),
		UpperBound: []byte("library;"),
	})
	if err != nil {
		return fmt.Errorf("failed to create iterator for legacy keys: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		oldKey := string(iter.Key())
		newKey := strings.Replace(oldKey, "library:path:", "import_path:path:", 1)
		if newKey == oldKey {
			newKey = strings.Replace(oldKey, "library:", "import_path:", 1)
		}
		if newKey == oldKey {
			continue
		}

		value := append([]byte(nil), iter.Value()...)
		if err := p.db.Set([]byte(newKey), value, pebble.Sync); err != nil {
			return fmt.Errorf("failed to write migrated key %s: %w", newKey, err)
		}
		if err := p.db.Delete([]byte(oldKey), pebble.Sync); err != nil {
			return fmt.Errorf("failed to delete legacy key %s: %w", oldKey, err)
		}
	}

	if value, closer, err := p.db.Get([]byte("counter:library")); err == nil {
		defer closer.Close()

		if counterValue, counterCloser, counterErr := p.db.Get([]byte("counter:import_path")); counterErr == nil {
			counterCloser.Close()
			value = counterValue // already migrated; keep existing value
		} else if counterErr != pebble.ErrNotFound {
			return fmt.Errorf("failed to read import path counter: %w", counterErr)
		} else if err := p.db.Set([]byte("counter:import_path"), value, pebble.Sync); err != nil {
			return fmt.Errorf("failed to migrate import path counter: %w", err)
		}

		if err := p.db.Delete([]byte("counter:library"), pebble.Sync); err != nil {
			return fmt.Errorf("failed to remove legacy library counter: %w", err)
		}
	} else if err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("failed to read legacy library counter: %w", err)
	}

	return nil
}

// Author operations

func (p *PebbleStore) GetAllAuthors() ([]Author, error) {
	var authors []Author
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("author:0"),
		UpperBound: []byte("author:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		if strings.Contains(string(iter.Key()), ":name:") {
			continue
		}

		var author Author
		if err := json.Unmarshal(iter.Value(), &author); err != nil {
			return nil, err
		}
		authors = append(authors, author)
	}

	return authors, nil
}

func (p *PebbleStore) GetAuthorByID(id int) (*Author, error) {
	key := []byte(fmt.Sprintf("author:%d", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var author Author
	if err := json.Unmarshal(value, &author); err != nil {
		return nil, err
	}
	return &author, nil
}

func (p *PebbleStore) GetAuthorByName(name string) (*Author, error) {
	// Use lowercase for case-insensitive lookup
	indexKey := []byte(fmt.Sprintf("author:name:%s", strings.ToLower(name)))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id, err := strconv.Atoi(string(value))
	if err != nil {
		return nil, err
	}

	return p.GetAuthorByID(id)
}

func (p *PebbleStore) CreateAuthor(name string) (*Author, error) {
	// Check if author already exists
	existing, err := p.GetAuthorByName(name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	id, err := p.nextID("author")
	if err != nil {
		return nil, err
	}

	author := &Author{ID: id, Name: name}
	data, err := json.Marshal(author)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("author:%d", id))
	// Use lowercase for case-insensitive lookup
	indexKey := []byte(fmt.Sprintf("author:name:%s", strings.ToLower(name)))

	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}
	if err := batch.Set(indexKey, []byte(strconv.Itoa(id)), nil); err != nil {
		batch.Close()
		return nil, err
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}

	return author, nil
}

// Series operations

func (p *PebbleStore) GetAllSeries() ([]Series, error) {
	var series []Series
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("series:0"),
		UpperBound: []byte("series:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		if strings.Contains(string(iter.Key()), ":name:") {
			continue
		}

		var s Series
		if err := json.Unmarshal(iter.Value(), &s); err != nil {
			return nil, err
		}
		series = append(series, s)
	}

	return series, nil
}

func (p *PebbleStore) GetSeriesByID(id int) (*Series, error) {
	key := []byte(fmt.Sprintf("series:%d", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var series Series
	if err := json.Unmarshal(value, &series); err != nil {
		return nil, err
	}
	return &series, nil
}

func (p *PebbleStore) GetSeriesByName(name string, authorID *int) (*Series, error) {
	authorIDStr := "nil"
	if authorID != nil {
		authorIDStr = strconv.Itoa(*authorID)
	}

	// Use lowercase for case-insensitive lookup
	indexKey := []byte(fmt.Sprintf("series:name:%s:%s", strings.ToLower(name), authorIDStr))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id, err := strconv.Atoi(string(value))
	if err != nil {
		return nil, err
	}

	return p.GetSeriesByID(id)
}

func (p *PebbleStore) CreateSeries(name string, authorID *int) (*Series, error) {
	// Check if series already exists
	existing, err := p.GetSeriesByName(name, authorID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	id, err := p.nextID("series")
	if err != nil {
		return nil, err
	}

	series := &Series{ID: id, Name: name, AuthorID: authorID}
	data, err := json.Marshal(series)
	if err != nil {
		return nil, err
	}

	authorIDStr := "nil"
	if authorID != nil {
		authorIDStr = strconv.Itoa(*authorID)
	}

	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("series:%d", id))
	// Use lowercase for case-insensitive lookup
	indexKey := []byte(fmt.Sprintf("series:name:%s:%s", strings.ToLower(name), authorIDStr))

	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}
	if err := batch.Set(indexKey, []byte(strconv.Itoa(id)), nil); err != nil {
		batch.Close()
		return nil, err
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}

	return series, nil
}

// ---- Work operations (logical title-level grouping) ----

func (p *PebbleStore) GetAllWorks() ([]Work, error) {
	var works []Work
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: []byte("work:0"), UpperBound: []byte("work:;")})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		if strings.Contains(string(iter.Key()), ":title:") {
			continue
		}
		var w Work
		if err := json.Unmarshal(iter.Value(), &w); err != nil {
			return nil, err
		}
		works = append(works, w)
	}
	return works, nil
}

func (p *PebbleStore) GetWorkByID(id string) (*Work, error) {
	key := []byte(fmt.Sprintf("work:%s", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var w Work
	if err := json.Unmarshal(value, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (p *PebbleStore) CreateWork(work *Work) (*Work, error) {
	if work.ID == "" {
		id, err := newULID()
		if err != nil {
			return nil, err
		}
		work.ID = id
	}
	data, err := json.Marshal(work)
	if err != nil {
		return nil, err
	}
	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("work:%s", work.ID))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}
	// Basic title index (case-insensitive normalized) for future lookup
	normTitle := strings.ToLower(strings.TrimSpace(work.Title))
	if normTitle != "" {
		idxKey := []byte(fmt.Sprintf("work:title:%s:%s", normTitle, work.ID))
		if err := batch.Set(idxKey, []byte(work.ID), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}
	return work, nil
}

func (p *PebbleStore) UpdateWork(id string, work *Work) (*Work, error) {
	old, err := p.GetWorkByID(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, fmt.Errorf("work not found")
	}
	work.ID = id
	data, err := json.Marshal(work)
	if err != nil {
		return nil, err
	}
	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("work:%s", id))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}
	oldNorm := strings.ToLower(strings.TrimSpace(old.Title))
	newNorm := strings.ToLower(strings.TrimSpace(work.Title))
	if oldNorm != newNorm {
		if oldNorm != "" {
			_ = batch.Delete([]byte(fmt.Sprintf("work:title:%s:%s", oldNorm, id)), nil)
		}
		if newNorm != "" {
			_ = batch.Set([]byte(fmt.Sprintf("work:title:%s:%s", newNorm, id)), []byte(id), nil)
		}
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}
	return work, nil
}

func (p *PebbleStore) DeleteWork(id string) error {
	work, err := p.GetWorkByID(id)
	if err != nil {
		return err
	}
	if work == nil {
		return nil
	}
	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("work:%s", id))
	if err := batch.Delete(key, nil); err != nil {
		batch.Close()
		return err
	}
	norm := strings.ToLower(strings.TrimSpace(work.Title))
	if norm != "" {
		_ = batch.Delete([]byte(fmt.Sprintf("work:title:%s:%s", norm, id)), nil)
	}
	return batch.Commit(pebble.Sync)
}

func (p *PebbleStore) GetBooksByWorkID(workID string) ([]Book, error) {
	// Scan all books and filter by WorkID (could add index later)
	books, err := p.GetAllBooks(1_000_000, 0)
	if err != nil {
		return nil, err
	}
	var filtered []Book
	for _, b := range books {
		if b.WorkID != nil && *b.WorkID == workID {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

// Book operations

func (p *PebbleStore) GetAllBooks(limit, offset int) ([]Book, error) {
	var books []Book
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	skipped := 0
	count := 0

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") {
			continue
		}

		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			return nil, err
		}
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		if limit > 0 && count >= limit {
			break
		}
		books = append(books, book)
		count++
	}

	return books, nil
}

func (p *PebbleStore) GetBookByID(id string) (*Book, error) {
	key := []byte(fmt.Sprintf("book:%s", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var book Book
	if err := json.Unmarshal(value, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (p *PebbleStore) GetBookByFilePath(path string) (*Book, error) {
	indexKey := []byte(fmt.Sprintf("book:path:%s", path))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id := string(value) // ULID string

	return p.GetBookByID(id)
}

func (p *PebbleStore) GetBookByFileHash(hash string) (*Book, error) {
	indexKey := []byte(fmt.Sprintf("book:hash:%s", hash))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id := string(value) // ULID string

	return p.GetBookByID(id)
}

func (p *PebbleStore) GetBookByOriginalHash(hash string) (*Book, error) {
	indexKey := []byte(fmt.Sprintf("book:originalhash:%s", hash))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id := string(value)
	return p.GetBookByID(id)
}

func (p *PebbleStore) GetBookByOrganizedHash(hash string) (*Book, error) {
	indexKey := []byte(fmt.Sprintf("book:organizedhash:%s", hash))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id := string(value)
	return p.GetBookByID(id)
}

// GetDuplicateBooks returns groups of books with identical file hashes
// Only returns groups with 2+ books (actual duplicates)
func (p *PebbleStore) GetDuplicateBooks() ([][]Book, error) {
	// Map to group books by hash (preferring organized_file_hash over file_hash)

	hashGroups := make(map[string][]Book)

	// Iterate through all books to find duplicates
	prefix := []byte("book:id:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys (they have : in specific patterns)
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") || strings.Contains(key, ":hash:") ||
			strings.Contains(key, ":organizedhash:") {
			continue
		}

		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			return nil, fmt.Errorf("failed to unmarshal book: %w", err)
		}
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}

		// Use organized_file_hash if available, otherwise file_hash
		var hash string
		if book.OrganizedFileHash != nil && *book.OrganizedFileHash != "" {
			hash = *book.OrganizedFileHash
		} else if book.FileHash != nil && *book.FileHash != "" {
			hash = *book.FileHash
		}

		// Only track books with valid hashes
		if hash != "" {
			hashGroups[hash] = append(hashGroups[hash], book)
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	// Extract groups with 2+ books (actual duplicates), sorted by file_path
	var duplicateGroups [][]Book
	for _, group := range hashGroups {
		if len(group) >= 2 {
			// Sort by file_path within each group
			sort.Slice(group, func(i, j int) bool {
				return group[i].FilePath < group[j].FilePath
			})
			duplicateGroups = append(duplicateGroups, group)
		}
	}

	return duplicateGroups, nil
}

func (p *PebbleStore) GetBooksBySeriesID(seriesID int) ([]Book, error) {
	var books []Book
	prefix := []byte(fmt.Sprintf("book:series:%d:", seriesID))

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		id := string(iter.Value()) // ULID string

		book, err := p.GetBookByID(id)
		if err != nil {
			return nil, err
		}
		if book != nil && (book.MarkedForDeletion == nil || !*book.MarkedForDeletion) {
			books = append(books, *book)
		}
	}

	return books, nil
}

func (p *PebbleStore) GetBooksByAuthorID(authorID int) ([]Book, error) {
	var books []Book
	prefix := []byte(fmt.Sprintf("book:author:%d:", authorID))

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		id := string(iter.Value()) // ULID string

		book, err := p.GetBookByID(id)
		if err != nil {
			return nil, err
		}
		if book != nil && (book.MarkedForDeletion == nil || !*book.MarkedForDeletion) {
			books = append(books, *book)
		}
	}

	return books, nil
}

func (p *PebbleStore) GetBookAuthors(bookID string) ([]BookAuthor, error) {
	key := []byte(fmt.Sprintf("book_authors:%s", bookID))
	val, closer, err := p.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	var authors []BookAuthor
	if err := json.Unmarshal(val, &authors); err != nil {
		return nil, err
	}
	return authors, nil
}

func (p *PebbleStore) SetBookAuthors(bookID string, authors []BookAuthor) error {
	key := []byte(fmt.Sprintf("book_authors:%s", bookID))
	data, err := json.Marshal(authors)
	if err != nil {
		return err
	}
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetBooksByAuthorIDWithRole(authorID int) ([]Book, error) {
	// For Pebble, fall back to the same logic as GetBooksByAuthorID
	return p.GetBooksByAuthorID(authorID)
}

func (p *PebbleStore) CreateBook(book *Book) (*Book, error) {
	// Generate ULID if not provided
	if book.ID == "" {
		id, err := newULID()
		if err != nil {
			return nil, err
		}
		book.ID = id
	}

	// Set timestamps
	now := time.Now()
	book.CreatedAt = &now
	book.UpdatedAt = &now

	data, err := json.Marshal(book)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()

	// Main key
	key := []byte(fmt.Sprintf("book:%s", book.ID))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}

	// Path index
	pathKey := []byte(fmt.Sprintf("book:path:%s", book.FilePath))
	if err := batch.Set(pathKey, []byte(book.ID), nil); err != nil {
		batch.Close()
		return nil, err
	}

	// Hash index (if hash provided)
	if book.FileHash != nil && *book.FileHash != "" {
		hashKey := []byte(fmt.Sprintf("book:hash:%s", *book.FileHash))
		if err := batch.Set(hashKey, []byte(book.ID), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	if book.OriginalFileHash != nil && *book.OriginalFileHash != "" {
		origKey := []byte(fmt.Sprintf("book:originalhash:%s", *book.OriginalFileHash))
		if err := batch.Set(origKey, []byte(book.ID), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	if book.OrganizedFileHash != nil && *book.OrganizedFileHash != "" {
		orgKey := []byte(fmt.Sprintf("book:organizedhash:%s", *book.OrganizedFileHash))
		if err := batch.Set(orgKey, []byte(book.ID), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	// Series index
	if book.SeriesID != nil {
		seriesKey := []byte(fmt.Sprintf("book:series:%d:%s", *book.SeriesID, book.ID))
		if err := batch.Set(seriesKey, []byte(book.ID), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	// Author index
	if book.AuthorID != nil {
		authorKey := []byte(fmt.Sprintf("book:author:%d:%s", *book.AuthorID, book.ID))
		if err := batch.Set(authorKey, []byte(book.ID), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}

	return book, nil
}

func (p *PebbleStore) UpdateBook(id string, book *Book) (*Book, error) {
	// Get old book to clean up old indexes
	oldBook, err := p.GetBookByID(id)
	if err != nil {
		return nil, err
	}
	if oldBook == nil {
		return nil, fmt.Errorf("book not found")
	}

	book.ID = id

	// Preserve created_at from old book, update updated_at
	if oldBook.CreatedAt != nil {
		book.CreatedAt = oldBook.CreatedAt
	}
	now := time.Now()
	book.UpdatedAt = &now

	data, err := json.Marshal(book)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()

	// Update main key
	key := []byte(fmt.Sprintf("book:%s", id))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}

	// Update path index if changed
	if oldBook.FilePath != book.FilePath {
		oldPathKey := []byte(fmt.Sprintf("book:path:%s", oldBook.FilePath))
		if err := batch.Delete(oldPathKey, nil); err != nil {
			batch.Close()
			return nil, err
		}
		newPathKey := []byte(fmt.Sprintf("book:path:%s", book.FilePath))
		if err := batch.Set(newPathKey, []byte(id), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	updateHashIndex := func(oldVal, newVal *string, prefix string) error {
		var oldStr, newStr string
		if oldVal != nil {
			oldStr = *oldVal
		}
		if newVal != nil {
			newStr = *newVal
		}
		if oldStr == newStr {
			return nil
		}
		if oldStr != "" {
			oldKey := []byte(fmt.Sprintf("book:%s:%s", prefix, oldStr))
			if err := batch.Delete(oldKey, nil); err != nil {
				return err
			}
		}
		if newStr != "" {
			newKey := []byte(fmt.Sprintf("book:%s:%s", prefix, newStr))
			if err := batch.Set(newKey, []byte(id), nil); err != nil {
				return err
			}
		}
		return nil
	}

	if err := updateHashIndex(oldBook.FileHash, book.FileHash, "hash"); err != nil {
		batch.Close()
		return nil, err
	}
	if err := updateHashIndex(oldBook.OriginalFileHash, book.OriginalFileHash, "originalhash"); err != nil {
		batch.Close()
		return nil, err
	}
	if err := updateHashIndex(oldBook.OrganizedFileHash, book.OrganizedFileHash, "organizedhash"); err != nil {
		batch.Close()
		return nil, err
	}

	// Update series index if changed
	oldSeriesID := -1
	if oldBook.SeriesID != nil {
		oldSeriesID = *oldBook.SeriesID
	}
	newSeriesID := -1
	if book.SeriesID != nil {
		newSeriesID = *book.SeriesID
	}
	if oldSeriesID != newSeriesID {
		if oldSeriesID != -1 {
			oldSeriesKey := []byte(fmt.Sprintf("book:series:%d:%s", oldSeriesID, id))
			if err := batch.Delete(oldSeriesKey, nil); err != nil {
				batch.Close()
				return nil, err
			}
		}
		if newSeriesID != -1 {
			newSeriesKey := []byte(fmt.Sprintf("book:series:%d:%s", newSeriesID, id))
			if err := batch.Set(newSeriesKey, []byte(id), nil); err != nil {
				batch.Close()
				return nil, err
			}
		}
	}

	// Update author index if changed
	oldAuthorID := -1
	if oldBook.AuthorID != nil {
		oldAuthorID = *oldBook.AuthorID
	}
	newAuthorID := -1
	if book.AuthorID != nil {
		newAuthorID = *book.AuthorID
	}
	if oldAuthorID != newAuthorID {
		if oldAuthorID != -1 {
			oldAuthorKey := []byte(fmt.Sprintf("book:author:%d:%s", oldAuthorID, id))
			if err := batch.Delete(oldAuthorKey, nil); err != nil {
				batch.Close()
				return nil, err
			}
		}
		if newAuthorID != -1 {
			newAuthorKey := []byte(fmt.Sprintf("book:author:%d:%s", newAuthorID, id))
			if err := batch.Set(newAuthorKey, []byte(id), nil); err != nil {
				batch.Close()
				return nil, err
			}
		}
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}

	return book, nil
}

func (p *PebbleStore) DeleteBook(id string) error {
	book, err := p.GetBookByID(id)
	if err != nil {
		return err
	}
	if book == nil {
		return nil
	}

	batch := p.db.NewBatch()

	// Delete main key
	key := []byte(fmt.Sprintf("book:%s", id))
	if err := batch.Delete(key, nil); err != nil {
		batch.Close()
		return err
	}

	// Delete path index
	pathKey := []byte(fmt.Sprintf("book:path:%s", book.FilePath))
	if err := batch.Delete(pathKey, nil); err != nil {
		batch.Close()
		return err
	}

	// Delete series index
	if book.SeriesID != nil {
		seriesKey := []byte(fmt.Sprintf("book:series:%d:%s", *book.SeriesID, id))
		if err := batch.Delete(seriesKey, nil); err != nil {
			batch.Close()
			return err
		}
	}

	// Delete author index
	if book.AuthorID != nil {
		authorKey := []byte(fmt.Sprintf("book:author:%d:%s", *book.AuthorID, id))
		if err := batch.Delete(authorKey, nil); err != nil {
			batch.Close()
			return err
		}
	}

	statePrefix := []byte(fmt.Sprintf("metadata_state:%s:", id))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: statePrefix,
		UpperBound: append(statePrefix, 0xFF),
	})
	if err != nil {
		batch.Close()
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		if err := batch.Delete(iter.Key(), nil); err != nil {
			batch.Close()
			return err
		}
	}

	return batch.Commit(pebble.Sync)
}

func (p *PebbleStore) SearchBooks(query string, limit, offset int) ([]Book, error) {
	// For PebbleDB, we need to scan all books and filter by title
	// In production, you'd want a proper full-text search solution
	allBooks, err := p.GetAllBooks(1000000, 0) // Get all books
	if err != nil {
		return nil, err
	}

	var filtered []Book
	lowerQuery := strings.ToLower(query)
	for _, book := range allBooks {
		if strings.Contains(strings.ToLower(book.Title), lowerQuery) {
			filtered = append(filtered, book)
		}
	}

	// Apply pagination
	start := offset
	if start >= len(filtered) {
		return []Book{}, nil
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], nil
}

func (p *PebbleStore) CountBooks() (int, error) {
	count := 0
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") {
			continue
		}
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			return 0, err
		}
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}
		count++
	}

	return count, nil
}

// GetDashboardStats iterates all books and computes aggregate stats.
// PebbleDB has no SQL, so this scans the full key range.
func (p *PebbleStore) GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{
		StateDistribution:  make(map[string]int),
		FormatDistribution: make(map[string]int),
	}
	books, err := p.GetAllBooks(1_000_000, 0)
	if err != nil {
		return nil, err
	}
	for _, b := range books {
		stats.TotalBooks++
		if b.Duration != nil {
			stats.TotalDuration += int64(*b.Duration)
		}
		if b.FileSize != nil {
			stats.TotalSize += *b.FileSize
		}
		state := "imported"
		if b.LibraryState != nil {
			state = *b.LibraryState
		}
		stats.StateDistribution[state]++
		codec := "unknown"
		if b.Codec != nil {
			codec = *b.Codec
		}
		stats.FormatDistribution[codec]++
	}
	return stats, nil
}

func (p *PebbleStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error) {
	var books []Book
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	skipped := 0
	collected := 0

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") || strings.Contains(key, ":version:") {
			continue
		}

		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			return nil, err
		}
		if book.MarkedForDeletion == nil || !*book.MarkedForDeletion {
			continue
		}
		if olderThan != nil && book.MarkedForDeletionAt != nil && book.MarkedForDeletionAt.After(*olderThan) {
			continue
		}

		if skipped < offset {
			skipped++
			continue
		}
		if limit > 0 && collected >= limit {
			break
		}
		books = append(books, book)
		collected++
	}

	return books, nil
}

// GetBooksByVersionGroup returns all books in a version group
func (p *PebbleStore) GetBooksByVersionGroup(groupID string) ([]Book, error) {
	var books []Book
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") || strings.Contains(key, ":version:") {
			continue
		}

		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}

		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}

		if book.VersionGroupID != nil && *book.VersionGroupID == groupID {
			books = append(books, book)
		}
	}

	// Sort by primary version first, then by title
	sort.Slice(books, func(i, j int) bool {
		if books[i].IsPrimaryVersion != nil && *books[i].IsPrimaryVersion {
			return true
		}
		if books[j].IsPrimaryVersion != nil && *books[j].IsPrimaryVersion {
			return false
		}
		return books[i].Title < books[j].Title
	})

	return books, nil
}

// Import path operations

func (p *PebbleStore) GetAllImportPaths() ([]ImportPath, error) {
	var importPaths []ImportPath
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("import_path:0"),
		UpperBound: []byte("import_path:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Skip index keys
		if strings.Contains(string(iter.Key()), ":path:") {
			continue
		}

		var importPath ImportPath
		if err := json.Unmarshal(iter.Value(), &importPath); err != nil {
			return nil, err
		}
		importPaths = append(importPaths, importPath)
	}

	return importPaths, nil
}

func (p *PebbleStore) GetImportPathByID(id int) (*ImportPath, error) {
	key := []byte(fmt.Sprintf("import_path:%d", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var importPath ImportPath
	if err := json.Unmarshal(value, &importPath); err != nil {
		return nil, err
	}
	return &importPath, nil
}

func (p *PebbleStore) GetImportPathByPath(path string) (*ImportPath, error) {
	indexKey := []byte(fmt.Sprintf("import_path:path:%s", path))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id, err := strconv.Atoi(string(value))
	if err != nil {
		return nil, err
	}

	return p.GetImportPathByID(id)
}

func (p *PebbleStore) CreateImportPath(path, name string) (*ImportPath, error) {
	existing, err := p.GetImportPathByPath(path)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("import path with path %s already exists", path)
	}

	id, err := p.nextID("import_path")
	if err != nil {
		return nil, err
	}

	importPath := &ImportPath{
		ID:        id,
		Path:      path,
		Name:      name,
		Enabled:   true,
		CreatedAt: time.Now(),
		BookCount: 0,
	}

	data, err := json.Marshal(importPath)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("import_path:%d", id))
	indexKey := []byte(fmt.Sprintf("import_path:path:%s", path))

	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}
	if err := batch.Set(indexKey, []byte(strconv.Itoa(id)), nil); err != nil {
		batch.Close()
		return nil, err
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}

	return importPath, nil
}

func (p *PebbleStore) UpdateImportPath(id int, importPath *ImportPath) error {
	importPath.ID = id

	// If the path changed, update the index accordingly
	current, err := p.GetImportPathByID(id)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("import path %d not found", id)
	}

	batch := p.db.NewBatch()

	if current.Path != importPath.Path {
		oldIndexKey := []byte(fmt.Sprintf("import_path:path:%s", current.Path))
		if err := batch.Delete(oldIndexKey, nil); err != nil {
			batch.Close()
			return err
		}
		newIndexKey := []byte(fmt.Sprintf("import_path:path:%s", importPath.Path))
		if err := batch.Set(newIndexKey, []byte(strconv.Itoa(id)), nil); err != nil {
			batch.Close()
			return err
		}
	}

	data, err := json.Marshal(importPath)
	if err != nil {
		batch.Close()
		return err
	}

	key := []byte(fmt.Sprintf("import_path:%d", id))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return err
	}

	return batch.Commit(pebble.Sync)
}

func (p *PebbleStore) DeleteImportPath(id int) error {
	importPath, err := p.GetImportPathByID(id)
	if err != nil {
		return err
	}
	if importPath == nil {
		return nil
	}

	batch := p.db.NewBatch()

	key := []byte(fmt.Sprintf("import_path:%d", id))
	if err := batch.Delete(key, nil); err != nil {
		batch.Close()
		return err
	}

	indexKey := []byte(fmt.Sprintf("import_path:path:%s", importPath.Path))
	if err := batch.Delete(indexKey, nil); err != nil {
		batch.Close()
		return err
	}

	return batch.Commit(pebble.Sync)
}

// Operation operations

func (p *PebbleStore) CreateOperation(id, opType string, folderPath *string) (*Operation, error) {
	op := &Operation{
		ID:         id,
		Type:       opType,
		Status:     "pending",
		Progress:   0,
		Total:      0,
		Message:    "",
		FolderPath: folderPath,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(op)
	if err != nil {
		return nil, err
	}

	key := []byte(fmt.Sprintf("operation:%s", id))
	if err := p.db.Set(key, data, pebble.Sync); err != nil {
		return nil, err
	}

	return op, nil
}

func (p *PebbleStore) GetOperationByID(id string) (*Operation, error) {
	key := []byte(fmt.Sprintf("operation:%s", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var op Operation
	if err := json.Unmarshal(value, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func (p *PebbleStore) GetRecentOperations(limit int) ([]Operation, error) {
	var operations []Operation
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("operation:"),
		UpperBound: []byte("operation:~"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var op Operation
		if err := json.Unmarshal(iter.Value(), &op); err != nil {
			continue
		}
		operations = append(operations, op)
	}

	// Sort by created_at descending (most recent first)
	// In production, you'd want a better indexing strategy
	for i := 0; i < len(operations)-1; i++ {
		for j := i + 1; j < len(operations); j++ {
			if operations[j].CreatedAt.After(operations[i].CreatedAt) {
				operations[i], operations[j] = operations[j], operations[i]
			}
		}
	}

	if len(operations) > limit {
		operations = operations[:limit]
	}

	return operations, nil
}

func (p *PebbleStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	op, err := p.GetOperationByID(id)
	if err != nil {
		return err
	}
	if op == nil {
		return fmt.Errorf("operation not found")
	}

	op.Status = status
	op.Progress = progress
	op.Total = total
	op.Message = message

	now := time.Now()
	if status == "running" && op.StartedAt == nil {
		op.StartedAt = &now
	} else if (status == "completed" || status == "failed") && op.CompletedAt == nil {
		op.CompletedAt = &now
	}

	data, err := json.Marshal(op)
	if err != nil {
		return err
	}

	key := []byte(fmt.Sprintf("operation:%s", id))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) UpdateOperationError(id, errorMessage string) error {
	op, err := p.GetOperationByID(id)
	if err != nil {
		return err
	}
	if op == nil {
		return fmt.Errorf("operation not found")
	}

	op.Status = "failed"
	op.ErrorMessage = &errorMessage
	now := time.Now()
	op.CompletedAt = &now

	data, err := json.Marshal(op)
	if err != nil {
		return err
	}

	key := []byte(fmt.Sprintf("operation:%s", id))
	return p.db.Set(key, data, pebble.Sync)
}

// Operation Log operations

func (p *PebbleStore) AddOperationLog(operationID, level, message string, details *string) error {
	id, err := p.nextID("operationlog")
	if err != nil {
		return err
	}

	log := &OperationLog{
		ID:          id,
		OperationID: operationID,
		Level:       level,
		Message:     message,
		Details:     details,
		CreatedAt:   time.Now(),
	}

	data, err := json.Marshal(log)
	if err != nil {
		return err
	}

	// Key format: operationlog:<operation_id>:<timestamp>:<seq>
	key := []byte(fmt.Sprintf("operationlog:%s:%d:%d", operationID, log.CreatedAt.UnixNano(), id))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetOperationLogs(operationID string) ([]OperationLog, error) {
	var logs []OperationLog
	prefix := []byte(fmt.Sprintf("operationlog:%s:", operationID))

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var log OperationLog
		if err := json.Unmarshal(iter.Value(), &log); err != nil {
			continue
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// Metadata provenance operations

func (p *PebbleStore) metadataStateKey(bookID, field string) []byte {
	return []byte(fmt.Sprintf("metadata_state:%s:%s", bookID, field))
}

func (p *PebbleStore) GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error) {
	var states []MetadataFieldState
	prefix := []byte(fmt.Sprintf("metadata_state:%s:", bookID))

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		if !strings.HasPrefix(string(iter.Key()), string(prefix)) {
			break
		}
		var state MetadataFieldState
		if err := json.Unmarshal(iter.Value(), &state); err != nil {
			return nil, err
		}
		states = append(states, state)
	}

	return states, nil
}

func (p *PebbleStore) UpsertMetadataFieldState(state *MetadataFieldState) error {
	if state == nil {
		return fmt.Errorf("metadata state cannot be nil")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now()
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return p.db.Set(p.metadataStateKey(state.BookID, state.Field), data, pebble.Sync)
}

func (p *PebbleStore) DeleteMetadataFieldState(bookID, field string) error {
	return p.db.Delete(p.metadataStateKey(bookID, field), pebble.Sync)
}

// User Preference operations

func (p *PebbleStore) GetUserPreference(key string) (*UserPreference, error) {
	dbKey := []byte(fmt.Sprintf("preference:%s", key))
	value, closer, err := p.db.Get(dbKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var pref UserPreference
	if err := json.Unmarshal(value, &pref); err != nil {
		return nil, err
	}
	return &pref, nil
}

func (p *PebbleStore) SetUserPreference(key, value string) error {
	existing, err := p.GetUserPreference(key)
	if err != nil {
		return err
	}

	var pref *UserPreference
	if existing != nil {
		pref = existing
		pref.Value = &value
		pref.UpdatedAt = time.Now()
	} else {
		id, err := p.nextID("preference")
		if err != nil {
			return err
		}
		pref = &UserPreference{
			ID:        id,
			Key:       key,
			Value:     &value,
			UpdatedAt: time.Now(),
		}
	}

	data, err := json.Marshal(pref)
	if err != nil {
		return err
	}

	dbKey := []byte(fmt.Sprintf("preference:%s", key))
	return p.db.Set(dbKey, data, pebble.Sync)
}

func (p *PebbleStore) GetAllUserPreferences() ([]UserPreference, error) {
	var preferences []UserPreference
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("preference:"),
		UpperBound: []byte("preference:~"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var pref UserPreference
		if err := json.Unmarshal(iter.Value(), &pref); err != nil {
			continue
		}
		preferences = append(preferences, pref)
	}

	return preferences, nil
}

// Playlist operations

func (p *PebbleStore) CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error) {
	id, err := p.nextID("playlist")
	if err != nil {
		return nil, err
	}

	playlist := &Playlist{
		ID:       id,
		Name:     name,
		SeriesID: seriesID,
		FilePath: filePath,
	}

	data, err := json.Marshal(playlist)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()
	key := []byte(fmt.Sprintf("playlist:%d", id))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}

	if seriesID != nil {
		indexKey := []byte(fmt.Sprintf("playlist:series:%d", *seriesID))
		if err := batch.Set(indexKey, []byte(strconv.Itoa(id)), nil); err != nil {
			batch.Close()
			return nil, err
		}
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, err
	}

	return playlist, nil
}

func (p *PebbleStore) GetPlaylistByID(id int) (*Playlist, error) {
	key := []byte(fmt.Sprintf("playlist:%d", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var playlist Playlist
	if err := json.Unmarshal(value, &playlist); err != nil {
		return nil, err
	}
	return &playlist, nil
}

func (p *PebbleStore) GetPlaylistBySeriesID(seriesID int) (*Playlist, error) {
	indexKey := []byte(fmt.Sprintf("playlist:series:%d", seriesID))
	value, closer, err := p.db.Get(indexKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	id, err := strconv.Atoi(string(value))
	if err != nil {
		return nil, err
	}

	return p.GetPlaylistByID(id)
}

func (p *PebbleStore) AddPlaylistItem(playlistID, bookID, position int) error {
	id, err := p.nextID("playlistitem")
	if err != nil {
		return err
	}

	item := &PlaylistItem{
		ID:         id,
		PlaylistID: playlistID,
		BookID:     bookID,
		Position:   position,
	}

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	key := []byte(fmt.Sprintf("playlistitem:%d:%d", playlistID, position))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetPlaylistItems(playlistID int) ([]PlaylistItem, error) {
	var items []PlaylistItem
	prefix := []byte(fmt.Sprintf("playlistitem:%d:", playlistID))

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var item PlaylistItem
		if err := json.Unmarshal(iter.Value(), &item); err != nil {
			continue
		}
		items = append(items, item)
	}

	return items, nil
}

// ---- Extended keyspace implementation ----

// Users & Auth
func (p *PebbleStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error) {
	lowerUser := strings.ToLower(username)
	lowerEmail := strings.ToLower(email)

	// uniqueness checks
	if _, closer, err := p.db.Get([]byte("idx:user:username:" + lowerUser)); err == nil {
		closer.Close()
		return nil, fmt.Errorf("username already exists")
	}
	if _, closer, err := p.db.Get([]byte("idx:user:email:" + lowerEmail)); err == nil {
		closer.Close()
		return nil, fmt.Errorf("email already exists")
	}

	id, err := newULID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user := &User{
		ID: id, Username: username, Email: email,
		PasswordHashAlgo: passwordHashAlgo, PasswordHash: passwordHash,
		Roles: roles, Status: status, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	data, _ := json.Marshal(user)
	b := p.db.NewBatch()
	if err := b.Set([]byte("u:"+id), data, nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Set([]byte("idx:user:username:"+lowerUser), []byte(id), nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Set([]byte("idx:user:email:"+lowerEmail), []byte(id), nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return nil, err
	}
	return user, nil
}

func (p *PebbleStore) GetUserByID(id string) (*User, error) {
	v, closer, err := p.db.Get([]byte("u:" + id))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var u User
	if err := json.Unmarshal(v, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (p *PebbleStore) getUserByIndex(idx string) (*User, error) {
	v, closer, err := p.db.Get([]byte(idx))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	id := string(v)
	return p.GetUserByID(id)
}

func (p *PebbleStore) GetUserByUsername(username string) (*User, error) {
	return p.getUserByIndex("idx:user:username:" + strings.ToLower(username))
}

func (p *PebbleStore) GetUserByEmail(email string) (*User, error) {
	return p.getUserByIndex("idx:user:email:" + strings.ToLower(email))
}

func (p *PebbleStore) UpdateUser(user *User) error {
	user.UpdatedAt = time.Now()
	data, _ := json.Marshal(user)
	return p.db.Set([]byte("u:"+user.ID), data, pebble.Sync)
}

// Sessions
func (p *PebbleStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	id, err := newULID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{ID: id, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl), IP: ip, UserAgent: userAgent, Revoked: false, Version: 1}
	data, _ := json.Marshal(sess)
	b := p.db.NewBatch()
	if err := b.Set([]byte("sess:"+id), data, nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Set([]byte("idx:sess:user:"+userID+":"+id), []byte("1"), nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return nil, err
	}
	return sess, nil
}

func (p *PebbleStore) GetSession(id string) (*Session, error) {
	v, closer, err := p.db.Get([]byte("sess:" + id))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var s Session
	if err := json.Unmarshal(v, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (p *PebbleStore) RevokeSession(id string) error {
	s, err := p.GetSession(id)
	if err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	s.Revoked = true
	data, _ := json.Marshal(s)
	return p.db.Set([]byte("sess:"+id), data, pebble.Sync)
}

func (p *PebbleStore) ListUserSessions(userID string) ([]Session, error) {
	prefix := []byte("idx:sess:user:" + userID + ":")
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: append(prefix, 0xFF)})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var res []Session
	for iter.First(); iter.Valid(); iter.Next() {
		sessID := strings.TrimPrefix(string(iter.Key()), "idx:sess:user:"+userID+":")
		s, err := p.GetSession(sessID)
		if err == nil && s != nil {
			res = append(res, *s)
		}
	}
	return res, nil
}

func (p *PebbleStore) DeleteExpiredSessions(now time.Time) (int, error) {
	prefix := []byte("sess:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	batch := p.db.NewBatch()
	defer batch.Close()

	deleted := 0
	for iter.First(); iter.Valid(); iter.Next() {
		key := append([]byte(nil), iter.Key()...)
		value := append([]byte(nil), iter.Value()...)

		var sess Session
		if err := json.Unmarshal(value, &sess); err != nil {
			continue
		}
		if !sess.Revoked && sess.ExpiresAt.After(now) {
			continue
		}

		if err := batch.Delete(key, nil); err != nil {
			return deleted, err
		}
		if err := batch.Delete([]byte("idx:sess:user:"+sess.UserID+":"+sess.ID), nil); err != nil {
			return deleted, err
		}
		deleted++
	}

	if deleted == 0 {
		return 0, nil
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func (p *PebbleStore) CountUsers() (int, error) {
	prefix := []byte("u:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	return count, nil
}

// Per-user preferences
func (p *PebbleStore) SetUserPreferenceForUser(userID, key, value string) error {
	kv := &UserPreferenceKV{UserID: userID, Key: key, Value: value, UpdatedAt: time.Now(), Version: 1}
	data, _ := json.Marshal(kv)
	return p.db.Set([]byte("pref:"+userID+":"+key), data, pebble.Sync)
}
func (p *PebbleStore) GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error) {
	v, closer, err := p.db.Get([]byte("pref:" + userID + ":" + key))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var kv UserPreferenceKV
	if err := json.Unmarshal(v, &kv); err != nil {
		return nil, err
	}
	return &kv, nil
}
func (p *PebbleStore) GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error) {
	prefix := []byte("pref:" + userID + ":")
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: append(prefix, 0xFF)})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var res []UserPreferenceKV
	for iter.First(); iter.Valid(); iter.Next() {
		var kv UserPreferenceKV
		if err := json.Unmarshal(iter.Value(), &kv); err == nil {
			res = append(res, kv)
		}
	}
	return res, nil
}

// Book segments & merge
func (p *PebbleStore) CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error) {
	segID, err := newULID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	segment.ID = segID
	segment.BookID = bookNumericID
	segment.Active = true
	segment.CreatedAt = now
	segment.UpdatedAt = now
	segment.Version = 1
	data, _ := json.Marshal(segment)
	b := p.db.NewBatch()
	if err := b.Set([]byte("bf:"+segID), data, nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Set([]byte(fmt.Sprintf("bfs:%d:%s", bookNumericID, segID)), []byte("1"), nil); err != nil {
		b.Close()
		return nil, err
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return nil, err
	}
	// recompute duration map
	_ = p.recomputeDurationMap(bookNumericID)
	return segment, nil
}

func (p *PebbleStore) ListBookSegments(bookNumericID int) ([]BookSegment, error) {
	prefix := []byte(fmt.Sprintf("bfs:%d:", bookNumericID))
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: append(prefix, 0xFF)})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var segs []BookSegment
	for iter.First(); iter.Valid(); iter.Next() {
		segID := strings.TrimPrefix(string(iter.Key()), fmt.Sprintf("bfs:%d:", bookNumericID))
		v, closer, err := p.db.Get([]byte("bf:" + segID))
		if err == nil {
			var s BookSegment
			if err := json.Unmarshal(v, &s); err == nil {
				segs = append(segs, s)
			}
			closer.Close()
		}
	}
	return segs, nil
}

func (p *PebbleStore) MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error {
	// Create new segment
	_, err := p.CreateBookSegment(bookNumericID, newSegment)
	if err != nil {
		return err
	}
	// Mark old segments
	b := p.db.NewBatch()
	for _, id := range supersedeIDs {
		v, closer, err := p.db.Get([]byte("bf:" + id))
		if err == nil {
			var s BookSegment
			if err := json.Unmarshal(v, &s); err == nil {
				closer.Close()
				s.Active = false
				sid := newSegment.ID
				s.SupersededBy = &sid
				s.UpdatedAt = time.Now()
				data, _ := json.Marshal(&s)
				if err := b.Set([]byte("bf:"+id), data, nil); err != nil {
					b.Close()
					return err
				}
			} else {
				closer.Close()
			}
		}
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return err
	}
	// recompute duration map
	return p.recomputeDurationMap(bookNumericID)
}

func (p *PebbleStore) recomputeDurationMap(bookNumericID int) error {
	segs, err := p.ListBookSegments(bookNumericID)
	if err != nil {
		return err
	}
	// simple stable ordering: by TrackNumber(if present) then FilePath
	// bubble sort (small lists expected)
	for i := 0; i < len(segs)-1; i++ {
		for j := i + 1; j < len(segs); j++ {
			less := false
			if segs[i].TrackNumber != nil && segs[j].TrackNumber != nil {
				less = *segs[i].TrackNumber > *segs[j].TrackNumber
			} else if segs[i].TrackNumber != nil {
				less = false
			} else if segs[j].TrackNumber != nil {
				less = true
			} else {
				less = segs[i].FilePath > segs[j].FilePath
			}
			if less {
				segs[i], segs[j] = segs[j], segs[i]
			}
		}
	}
	type segMap struct {
		ID          string `json:"id"`
		Duration    int    `json:"duration"`
		Active      bool   `json:"active"`
		OffsetStart int    `json:"offset_start"`
	}
	var arr []segMap
	total := 0
	for _, s := range segs {
		arr = append(arr, segMap{ID: s.ID, Duration: s.DurationSec, Active: s.Active, OffsetStart: total})
		total += s.DurationSec
	}
	m := map[string]any{"segments": arr, "total_duration": total, "version": 1}
	data, _ := json.Marshal(m)
	return p.db.Set([]byte(fmt.Sprintf("b:duration_map:%d", bookNumericID)), data, pebble.Sync)
}

// Playback events & progress
func (p *PebbleStore) AddPlaybackEvent(event *PlaybackEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	event.Version = 1
	data, _ := json.Marshal(event)
	key := fmt.Sprintf("playe:%s:%d:%d", event.UserID, event.BookID, event.CreatedAt.UnixNano())
	return p.db.Set([]byte(key), data, pebble.Sync)
}

func (p *PebbleStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error) {
	prefix := []byte(fmt.Sprintf("playe:%s:%d:", userID, bookNumericID))
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: append(prefix, 0xFF)})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var events []PlaybackEvent
	for iter.First(); iter.Valid(); iter.Next() {
		var ev PlaybackEvent
		if err := json.Unmarshal(iter.Value(), &ev); err == nil {
			events = append(events, ev)
		}
	}
	// reverse chronological and cap to limit
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func (p *PebbleStore) UpdatePlaybackProgress(progress *PlaybackProgress) error {
	if progress.UpdatedAt.IsZero() {
		progress.UpdatedAt = time.Now()
	}
	progress.Version = 1
	data, _ := json.Marshal(progress)
	key := fmt.Sprintf("playp:%s:%d", progress.UserID, progress.BookID)
	return p.db.Set([]byte(key), data, pebble.Sync)
}

func (p *PebbleStore) GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error) {
	v, closer, err := p.db.Get([]byte(fmt.Sprintf("playp:%s:%d", userID, bookNumericID)))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var pr PlaybackProgress
	if err := json.Unmarshal(v, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// Stats aggregation
func (p *PebbleStore) IncrementBookPlayStats(bookNumericID int, seconds int) error {
	// increment counters stored as decimal strings
	if err := p.incrementIntKey(fmt.Sprintf("stats:book:plays:%d", bookNumericID), 1); err != nil {
		return err
	}
	return p.incrementIntKey(fmt.Sprintf("stats:book:listen_seconds:%d", bookNumericID), seconds)
}
func (p *PebbleStore) GetBookStats(bookNumericID int) (*BookStats, error) {
	plays, _ := p.readIntKey(fmt.Sprintf("stats:book:plays:%d", bookNumericID))
	secs, _ := p.readIntKey(fmt.Sprintf("stats:book:listen_seconds:%d", bookNumericID))
	return &BookStats{BookID: bookNumericID, PlayCount: plays, ListenSeconds: secs, Version: 1}, nil
}
func (p *PebbleStore) IncrementUserListenStats(userID string, seconds int) error {
	return p.incrementIntKey("stats:user:listen_seconds:"+userID, seconds)
}
func (p *PebbleStore) GetUserStats(userID string) (*UserStats, error) {
	secs, _ := p.readIntKey("stats:user:listen_seconds:" + userID)
	return &UserStats{UserID: userID, ListenSeconds: secs, Version: 1}, nil
}

func (p *PebbleStore) readIntKey(key string) (int, error) {
	v, closer, err := p.db.Get([]byte(key))
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closer.Close()
	return strconv.Atoi(string(v))
}
func (p *PebbleStore) incrementIntKey(key string, delta int) error {
	cur, _ := p.readIntKey(key)
	cur += delta
	return p.db.Set([]byte(key), []byte(strconv.Itoa(cur)), pebble.Sync)
}

// Blocked hash (do-not-import) methods
func (p *PebbleStore) IsHashBlocked(hash string) (bool, error) {
	key := []byte(fmt.Sprintf("blocked:hash:%s", hash))
	_, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	closer.Close()
	return true, nil
}

func (p *PebbleStore) AddBlockedHash(hash, reason string) error {
	item := DoNotImport{
		Hash:      hash,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal blocked hash: %w", err)
	}

	key := []byte(fmt.Sprintf("blocked:hash:%s", hash))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) RemoveBlockedHash(hash string) error {
	key := []byte(fmt.Sprintf("blocked:hash:%s", hash))
	return p.db.Delete(key, pebble.Sync)
}

func (p *PebbleStore) GetAllBlockedHashes() ([]DoNotImport, error) {
	var items []DoNotImport
	prefix := []byte("blocked:hash:")

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var item DoNotImport
		if err := json.Unmarshal(iter.Value(), &item); err != nil {
			return nil, fmt.Errorf("failed to unmarshal blocked hash: %w", err)
		}
		items = append(items, item)
	}

	return items, iter.Error()
}

func (p *PebbleStore) GetBlockedHashByHash(hash string) (*DoNotImport, error) {
	key := []byte(fmt.Sprintf("blocked:hash:%s", hash))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var item DoNotImport
	if err := json.Unmarshal(value, &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal blocked hash: %w", err)
	}

	return &item, nil
}

// Reset clears all data from the store and resets all counters to initial state
func (p *PebbleStore) Reset() error {
	// Delete all keys by iterating through the entire keyspace
	iter, err := p.db.NewIter(nil)
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	// Collect keys to delete (can't delete during iteration)
	var keysToDelete [][]byte
	for iter.First(); iter.Valid(); iter.Next() {
		keysToDelete = append(keysToDelete, append([]byte(nil), iter.Key()...))
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("iterator error: %w", err)
	}

	// Delete all keys
	for _, key := range keysToDelete {
		if err := p.db.Delete(key, pebble.Sync); err != nil {
			return fmt.Errorf("failed to delete key: %w", err)
		}
	}

	// Reinitialize counters to their initial state
	// Note: "library" counter was removed as it's a legacy counter that was migrated
	// to use the new distributed library system and is no longer maintained
	counters := []string{"author", "series", "book", "import_path", "operationlog", "playlist", "playlistitem", "preference"}
	for _, counter := range counters {
		key := fmt.Sprintf("counter:%s", counter)
		if err := p.db.Set([]byte(key), []byte("1"), pebble.Sync); err != nil {
			return fmt.Errorf("failed to initialize counter %s: %w", counter, err)
		}
	}

	return nil
}
