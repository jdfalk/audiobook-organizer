// file: internal/database/pebble_store.go
// version: 1.42.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

package database

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble/v2"
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
// - author_alias:<id>           -> AuthorAlias JSON
// - author_alias:author:<author_id>:<alias_id> -> alias_id (for author queries)
// - author_alias:name:<name>    -> alias_id (for name lookups)
// - counter:author              -> next author ID
// - counter:author_alias        -> next author alias ID
// - counter:series             -> next series ID
// - counter:book               -> next book ID
// - counter:import_path        -> next import path ID
// - counter:operationlog       -> next operation log ID
// - counter:playlist           -> next playlist ID
// - counter:playlistitem       -> next playlist item ID
// - metadata_state:<book_id>:<field> -> MetadataFieldState JSON
// - author_tombstone:<old_id>        -> canonical_id (merged author redirect)

type PebbleStore struct {
	db      *pebble.DB
	counterMu sync.Mutex // protects nextID read-modify-write
}

// NewPebbleStore creates a new PebbleDB store
func NewPebbleStore(path string) (*PebbleStore, error) {
	db, err := pebble.Open(path, &pebble.Options{
		FormatMajorVersion: pebble.FormatNewest,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open PebbleDB: %w", err)
	}

	store := &PebbleStore{db: db}

	log.Printf("[INFO] PebbleDB opened at %s (format version: %s)", path, db.FormatMajorVersion())

	if err := store.migrateImportPathKeys(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate import path keys: %w", err)
	}

	// Initialize counters if they don't exist
	counters := []string{"author", "author_alias", "series", "book", "import_path", "operationlog", "playlist", "playlistitem", "preference"}
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
	p.counterMu.Lock()
	defer p.counterMu.Unlock()

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
		// Check for tombstone redirect
		canonicalID, tErr := p.GetAuthorTombstone(id)
		if tErr != nil || canonicalID == 0 {
			return nil, nil
		}
		return p.GetAuthorByID(canonicalID)
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

func (p *PebbleStore) DeleteAuthor(id int) error {
	// Get the author to find name for index cleanup
	author, err := p.GetAuthorByID(id)
	if err != nil {
		return err
	}
	if author == nil {
		return nil
	}

	batch := p.db.NewBatch()
	batch.Delete([]byte(fmt.Sprintf("author:%d", id)), nil)
	batch.Delete([]byte(fmt.Sprintf("author:name:%s", strings.ToLower(author.Name))), nil)

	// Delete aliases for this author (cascade)
	if err := p.deleteAuthorAliases(batch, id); err != nil {
		batch.Close()
		return fmt.Errorf("delete author aliases: %w", err)
	}

	// Delete book_author entries for this author
	iter, iterErr := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book_author:"),
		UpperBound: []byte("book_author;"),
	})
	if iterErr == nil {
		defer iter.Close()
		for iter.First(); iter.Valid(); iter.Next() {
			val, valErr := iter.ValueAndErr()
			if valErr != nil {
				continue
			}
			var ba BookAuthor
			if json.Unmarshal(val, &ba) == nil && ba.AuthorID == id {
				batch.Delete(iter.Key(), nil)
			}
		}
	}

	return batch.Commit(pebble.Sync)
}

func (p *PebbleStore) UpdateAuthorName(id int, name string) error {
	author, err := p.GetAuthorByID(id)
	if err != nil {
		return err
	}
	if author == nil {
		return fmt.Errorf("author %d not found", id)
	}

	batch := p.db.NewBatch()
	// Remove old name index
	batch.Delete([]byte(fmt.Sprintf("author:name:%s", strings.ToLower(author.Name))), nil)

	// Update author record
	author.Name = name
	data, err := json.Marshal(author)
	if err != nil {
		batch.Close()
		return err
	}
	if err := batch.Set([]byte(fmt.Sprintf("author:%d", id)), data, nil); err != nil {
		batch.Close()
		return err
	}
	// Add new name index
	if err := batch.Set([]byte(fmt.Sprintf("author:name:%s", strings.ToLower(name))), []byte(strconv.Itoa(id)), nil); err != nil {
		batch.Close()
		return err
	}

	return batch.Commit(pebble.Sync)
}

// Author Alias operations
//
// Key schema:
//   author_alias:<id>                              → AuthorAlias JSON
//   author_alias:author:<author_id>:<alias_id>     → alias_id (iterate by author)
//   author_alias:name:<lowercase_alias_name>       → alias_id (lookup by name)

func (p *PebbleStore) GetAuthorAliases(authorID int) ([]AuthorAlias, error) {
	prefix := []byte(fmt.Sprintf("author_alias:author:%d:", authorID))
	upper := []byte(fmt.Sprintf("author_alias:author:%d;", authorID))
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var aliases []AuthorAlias
	for iter.First(); iter.Valid(); iter.Next() {
		aliasID, _ := strconv.Atoi(string(iter.Value()))
		alias, err := p.getAuthorAliasByID(aliasID)
		if err != nil {
			return nil, err
		}
		if alias != nil {
			aliases = append(aliases, *alias)
		}
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].AliasName < aliases[j].AliasName })
	return aliases, nil
}

func (p *PebbleStore) GetAllAuthorAliases() ([]AuthorAlias, error) {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("author_alias:0"),
		UpperBound: []byte("author_alias:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var aliases []AuthorAlias
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Only match primary records (author_alias:<digits>), skip index keys
		if strings.Contains(key, ":author:") || strings.Contains(key, ":name:") {
			continue
		}
		var a AuthorAlias
		if err := json.Unmarshal(iter.Value(), &a); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, nil
}

func (p *PebbleStore) CreateAuthorAlias(authorID int, aliasName string, aliasType string) (*AuthorAlias, error) {
	if aliasType == "" {
		aliasType = "alias"
	}

	// Check for duplicate
	nameKey := fmt.Sprintf("author_alias:name:%s", strings.ToLower(aliasName))
	if _, closer, err := p.db.Get([]byte(nameKey)); err == nil {
		closer.Close()
		return nil, fmt.Errorf("alias %q already exists", aliasName)
	}

	id, err := p.nextID("author_alias")
	if err != nil {
		return nil, err
	}

	alias := AuthorAlias{
		ID:        id,
		AuthorID:  authorID,
		AliasName: aliasName,
		AliasType: aliasType,
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(alias)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()
	batch.Set([]byte(fmt.Sprintf("author_alias:%d", id)), data, nil)
	batch.Set([]byte(fmt.Sprintf("author_alias:author:%d:%d", authorID, id)), []byte(strconv.Itoa(id)), nil)
	batch.Set([]byte(nameKey), []byte(strconv.Itoa(id)), nil)

	if err := batch.Commit(pebble.Sync); err != nil {
		batch.Close()
		return nil, err
	}
	return &alias, nil
}

func (p *PebbleStore) DeleteAuthorAlias(id int) error {
	alias, err := p.getAuthorAliasByID(id)
	if err != nil {
		return err
	}
	if alias == nil {
		return nil
	}

	batch := p.db.NewBatch()
	batch.Delete([]byte(fmt.Sprintf("author_alias:%d", id)), nil)
	batch.Delete([]byte(fmt.Sprintf("author_alias:author:%d:%d", alias.AuthorID, id)), nil)
	batch.Delete([]byte(fmt.Sprintf("author_alias:name:%s", strings.ToLower(alias.AliasName))), nil)
	return batch.Commit(pebble.Sync)
}

func (p *PebbleStore) FindAuthorByAlias(aliasName string) (*Author, error) {
	nameKey := []byte(fmt.Sprintf("author_alias:name:%s", strings.ToLower(aliasName)))
	value, closer, err := p.db.Get(nameKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	aliasID, _ := strconv.Atoi(string(value))
	closer.Close()

	alias, err := p.getAuthorAliasByID(aliasID)
	if err != nil || alias == nil {
		return nil, err
	}
	return p.GetAuthorByID(alias.AuthorID)
}

func (p *PebbleStore) getAuthorAliasByID(id int) (*AuthorAlias, error) {
	key := []byte(fmt.Sprintf("author_alias:%d", id))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var alias AuthorAlias
	if err := json.Unmarshal(value, &alias); err != nil {
		return nil, err
	}
	return &alias, nil
}

// deleteAuthorAliases removes all aliases for an author (cascade on delete).
func (p *PebbleStore) deleteAuthorAliases(batch *pebble.Batch, authorID int) error {
	prefix := []byte(fmt.Sprintf("author_alias:author:%d:", authorID))
	upper := []byte(fmt.Sprintf("author_alias:author:%d;", authorID))
	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		aliasID, _ := strconv.Atoi(string(iter.Value()))
		alias, err := p.getAuthorAliasByID(aliasID)
		if err != nil {
			return err
		}
		if alias != nil {
			batch.Delete([]byte(fmt.Sprintf("author_alias:%d", aliasID)), nil)
			batch.Delete([]byte(fmt.Sprintf("author_alias:name:%s", strings.ToLower(alias.AliasName))), nil)
		}
		batch.Delete(iter.Key(), nil)
	}
	return nil
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

func (p *PebbleStore) DeleteSeries(id int) error {
	key := []byte(fmt.Sprintf("series:%d", id))

	// Read the series first to clean up the name index
	val, closer, err := p.db.Get(key)
	if err == nil {
		var series Series
		if json.Unmarshal(val, &series) == nil {
			authorIDStr := "nil"
			if series.AuthorID != nil {
				authorIDStr = strconv.Itoa(*series.AuthorID)
			}
			indexKey := []byte(fmt.Sprintf("series:name:%s:%s", strings.ToLower(series.Name), authorIDStr))
			_ = p.db.Delete(indexKey, pebble.Sync)
		}
		closer.Close()
	}

	return p.db.Delete(key, pebble.Sync)
}

func (p *PebbleStore) UpdateSeriesName(id int, name string) error {
	key := []byte(fmt.Sprintf("series:%d", id))
	val, closer, err := p.db.Get(key)
	if err != nil {
		return fmt.Errorf("series %d not found: %w", id, err)
	}
	var series Series
	if err := json.Unmarshal(val, &series); err != nil {
		closer.Close()
		return err
	}
	closer.Close()

	// Delete old name index
	oldAuthorIDStr := "nil"
	if series.AuthorID != nil {
		oldAuthorIDStr = strconv.Itoa(*series.AuthorID)
	}
	oldIndexKey := []byte(fmt.Sprintf("series:name:%s:%s", strings.ToLower(series.Name), oldAuthorIDStr))
	_ = p.db.Delete(oldIndexKey, pebble.Sync)

	// Update name
	series.Name = name
	data, err := json.Marshal(series)
	if err != nil {
		return err
	}
	if err := p.db.Set(key, data, pebble.Sync); err != nil {
		return err
	}

	// Create new name index
	newIndexKey := []byte(fmt.Sprintf("series:name:%s:%s", strings.ToLower(name), oldAuthorIDStr))
	idBytes := []byte(fmt.Sprintf("%d", id))
	return p.db.Set(newIndexKey, idBytes, pebble.Sync)
}

func (p *PebbleStore) GetAllSeriesBookCounts() (map[int]int, error) {
	series, err := p.GetAllSeries()
	if err != nil {
		return nil, err
	}
	counts := make(map[int]int, len(series))
	for _, s := range series {
		books, _ := p.GetBooksBySeriesID(s.ID)
		count := 0
		for _, b := range books {
			if b.IsPrimaryVersion == nil || *b.IsPrimaryVersion {
				count++
			}
		}
		counts[s.ID] = count
	}
	return counts, nil
}

// GetAllSeriesFileCounts returns the number of audio files per series.
func (p *PebbleStore) GetAllSeriesFileCounts() (map[int]int, error) {
	series, err := p.GetAllSeries()
	if err != nil {
		return nil, err
	}
	counts := make(map[int]int, len(series))
	for _, s := range series {
		books, _ := p.GetBooksBySeriesID(s.ID)
		total := 0
		for _, b := range books {
			if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
				continue
			}
			files, err := p.GetBookFiles(b.ID)
			if err != nil || len(files) == 0 {
				total++
				continue
			}
			activeCount := 0
			for _, f := range files {
				if !f.Missing {
					activeCount++
				}
			}
			if activeCount > 0 {
				total += activeCount
			} else {
				total++
			}
		}
		counts[s.ID] = total
	}
	return counts, nil
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

// GetBookByITunesPersistentID scans all books to find one matching the given
// iTunes persistent ID. This is O(n) but syncs are infrequent.
func (p *PebbleStore) GetBookByITunesPersistentID(persistentID string) (*Book, error) {
	if persistentID == "" {
		return nil, nil
	}
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") || strings.Contains(key, ":hash:") {
			continue
		}
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.ITunesPersistentID != nil && *book.ITunesPersistentID == persistentID {
			return &book, nil
		}
	}
	return nil, nil
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

func (p *PebbleStore) GetBooksByTitleInDir(normalizedTitle, dirPath string) ([]Book, error) {
	return nil, nil
}

func (p *PebbleStore) GetFolderDuplicates() ([][]Book, error) {
	// PebbleStore doesn't support folder-based duplicate detection efficiently.
	return nil, nil
}

// GetDuplicateBooksByMetadata is not efficiently supported in PebbleStore.
func (p *PebbleStore) GetDuplicateBooksByMetadata(threshold float64) ([][]Book, error) {
	return nil, nil
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

func (p *PebbleStore) GetAllAuthorBookCounts() (map[int]int, error) {
	authors, err := p.GetAllAuthors()
	if err != nil {
		return nil, err
	}
	counts := make(map[int]int, len(authors))
	for _, a := range authors {
		books, _ := p.GetBooksByAuthorID(a.ID)
		count := 0
		for _, b := range books {
			if b.IsPrimaryVersion == nil || *b.IsPrimaryVersion {
				count++
			}
		}
		counts[a.ID] = count
	}
	return counts, nil
}

// GetAllAuthorFileCounts returns the number of audio files per author.
func (p *PebbleStore) GetAllAuthorFileCounts() (map[int]int, error) {
	authors, err := p.GetAllAuthors()
	if err != nil {
		return nil, err
	}
	counts := make(map[int]int, len(authors))
	for _, a := range authors {
		books, _ := p.GetBooksByAuthorID(a.ID)
		total := 0
		for _, b := range books {
			if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
				continue
			}
			files, err := p.GetBookFiles(b.ID)
			if err != nil || len(files) == 0 {
				total++
				continue
			}
			activeCount := 0
			for _, f := range files {
				if !f.Missing {
					activeCount++
				}
			}
			if activeCount > 0 {
				total += activeCount
			} else {
				total++
			}
		}
		counts[a.ID] = total
	}
	return counts, nil
}

func (p *PebbleStore) CreateNarrator(name string) (*Narrator, error) {
	// Check if narrator already exists
	existing, err := p.GetNarratorByName(name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// Generate a new ID by incrementing a counter
	counterKey := []byte("narrator_counter")
	var nextID int
	if val, closer, err := p.db.Get(counterKey); err == nil {
		json.Unmarshal(val, &nextID)
		closer.Close()
	}
	nextID++

	narrator := &Narrator{ID: nextID, Name: name, CreatedAt: time.Now()}
	data, err := json.Marshal(narrator)
	if err != nil {
		return nil, err
	}

	key := []byte(fmt.Sprintf("narrator:%d", nextID))
	if err := p.db.Set(key, data, pebble.Sync); err != nil {
		return nil, err
	}

	// Save name index
	nameKey := []byte(fmt.Sprintf("narrator_name:%s", strings.ToLower(name)))
	idData, _ := json.Marshal(nextID)
	p.db.Set(nameKey, idData, pebble.Sync)

	// Update counter
	counterData, _ := json.Marshal(nextID)
	p.db.Set(counterKey, counterData, pebble.Sync)

	return narrator, nil
}

func (p *PebbleStore) GetNarratorByID(id int) (*Narrator, error) {
	key := []byte(fmt.Sprintf("narrator:%d", id))
	val, closer, err := p.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	var narrator Narrator
	if err := json.Unmarshal(val, &narrator); err != nil {
		return nil, err
	}
	return &narrator, nil
}

func (p *PebbleStore) GetNarratorByName(name string) (*Narrator, error) {
	nameKey := []byte(fmt.Sprintf("narrator_name:%s", strings.ToLower(name)))
	val, closer, err := p.db.Get(nameKey)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	var id int
	if err := json.Unmarshal(val, &id); err != nil {
		return nil, err
	}
	return p.GetNarratorByID(id)
}

func (p *PebbleStore) ListNarrators() ([]Narrator, error) {
	var narrators []Narrator
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("narrator:"),
		UpperBound: []byte("narrator;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var n Narrator
		if err := json.Unmarshal(iter.Value(), &n); err == nil {
			narrators = append(narrators, n)
		}
	}
	return narrators, nil
}

func (p *PebbleStore) GetBookNarrators(bookID string) ([]BookNarrator, error) {
	key := []byte(fmt.Sprintf("book_narrators:%s", bookID))
	val, closer, err := p.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	var narrators []BookNarrator
	if err := json.Unmarshal(val, &narrators); err != nil {
		return nil, err
	}
	return narrators, nil
}

func (p *PebbleStore) SetBookNarrators(bookID string, narrators []BookNarrator) error {
	key := []byte(fmt.Sprintf("book_narrators:%s", bookID))
	data, err := json.Marshal(narrators)
	if err != nil {
		return err
	}
	return p.db.Set(key, data, pebble.Sync)
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

	// CoW: snapshot old state before overwriting
	oldData, marshalErr := json.Marshal(oldBook)
	if marshalErr != nil {
		return nil, fmt.Errorf("failed to marshal old book for version: %w", marshalErr)
	}
	versionKey := []byte(fmt.Sprintf("book_ver:%s:%d", id, time.Now().UnixNano()))

	batch := p.db.NewBatch()

	// Write version snapshot before main key
	if err := batch.Set(versionKey, oldData, nil); err != nil {
		batch.Close()
		return nil, err
	}

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

// SetLastWrittenAt stamps the last_written_at timestamp for book id.
func (p *PebbleStore) SetLastWrittenAt(id string, t time.Time) error {
	book, err := p.GetBookByID(id)
	if err != nil {
		return err
	}
	if book == nil {
		return nil // non-fatal: book not found
	}
	book.LastWrittenAt = &t
	_, err = p.UpdateBook(id, book)
	return err
}

// MarkITunesSynced sets itunes_sync_status to "synced" for the given book IDs.
func (p *PebbleStore) MarkITunesSynced(bookIDs []string) (int64, error) {
	var count int64
	synced := "synced"
	for _, id := range bookIDs {
		book, err := p.GetBookByID(id)
		if err != nil || book == nil {
			continue
		}
		book.ITunesSyncStatus = &synced
		if _, err := p.UpdateBook(id, book); err == nil {
			count++
		}
	}
	return count, nil
}

// GetITunesDirtyBooks returns all primary books with itunes_sync_status = "dirty".
func (p *PebbleStore) GetITunesDirtyBooks() ([]Book, error) {
	allBooks, err := p.GetAllBooks(100000, 0)
	if err != nil {
		return nil, err
	}
	var dirty []Book
	for _, b := range allBooks {
		if b.ITunesSyncStatus != nil && *b.ITunesSyncStatus == "dirty" {
			if b.IsPrimaryVersion == nil || *b.IsPrimaryVersion {
				dirty = append(dirty, b)
			}
		}
	}
	return dirty, nil
}

// GetBookVersions returns CoW version snapshots for a book, newest-first.
func (p *PebbleStore) GetBookVersions(id string, limit int) ([]BookVersion, error) {
	prefix := fmt.Sprintf("book_ver:%s:", id)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(prefix + "\xff"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var versions []BookVersion
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 {
			continue
		}
		nsec, parseErr := strconv.ParseInt(parts[2], 10, 64)
		if parseErr != nil {
			continue
		}
		dataCopy := make([]byte, len(iter.Value()))
		copy(dataCopy, iter.Value())
		versions = append(versions, BookVersion{
			BookID:    id,
			Timestamp: time.Unix(0, nsec),
			Data:      dataCopy,
		})
	}
	// Reverse for newest-first
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}
	if limit > 0 && len(versions) > limit {
		versions = versions[:limit]
	}
	return versions, nil
}

// GetBookAtVersion retrieves a book snapshot at a specific version timestamp.
func (p *PebbleStore) GetBookAtVersion(id string, ts time.Time) (*Book, error) {
	key := []byte(fmt.Sprintf("book_ver:%s:%d", id, ts.UnixNano()))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, fmt.Errorf("version not found")
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

// RevertBookToVersion restores a book to a previous version snapshot.
func (p *PebbleStore) RevertBookToVersion(id string, ts time.Time) (*Book, error) {
	oldBook, err := p.GetBookAtVersion(id, ts)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	oldBook.ID = id
	return p.UpdateBook(id, oldBook)
}

// PruneBookVersions keeps the newest keepCount versions and deletes the rest.
func (p *PebbleStore) PruneBookVersions(id string, keepCount int) (int, error) {
	if keepCount < 0 {
		keepCount = 0
	}
	versions, err := p.GetBookVersions(id, 0)
	if err != nil {
		return 0, err
	}
	if len(versions) <= keepCount {
		return 0, nil
	}
	toDelete := versions[keepCount:]
	batch := p.db.NewBatch()
	for _, v := range toDelete {
		key := []byte(fmt.Sprintf("book_ver:%s:%d", id, v.Timestamp.UnixNano()))
		if err := batch.Delete(key, nil); err != nil {
			batch.Close()
			return 0, err
		}
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return 0, err
	}
	return len(toDelete), nil
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

	// Build author name index for search matching
	authorNames := make(map[int]string)
	authIter, authErr := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("author:0"),
		UpperBound: []byte("author:;"),
	})
	if authErr == nil {
		defer authIter.Close()
		for authIter.First(); authIter.Valid(); authIter.Next() {
			key := string(authIter.Key())
			if strings.Contains(key, ":name:") || strings.Contains(key, ":book:") {
				continue
			}
			var a Author
			if err := json.Unmarshal(authIter.Value(), &a); err == nil {
				authorNames[a.ID] = strings.ToLower(a.Name)
			}
		}
	}

	var filtered []Book
	lowerQuery := strings.ToLower(query)
	for _, book := range allBooks {
		titleMatch := strings.Contains(strings.ToLower(book.Title), lowerQuery)

		// Check author name
		authorMatch := false
		if book.AuthorID != nil {
			if name, ok := authorNames[*book.AuthorID]; ok {
				authorMatch = strings.Contains(name, lowerQuery)
			}
		}

		// Check narrator
		narratorMatch := book.Narrator != nil && strings.Contains(strings.ToLower(*book.Narrator), lowerQuery)

		if titleMatch || authorMatch || narratorMatch {
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
		// Skip non-primary versions so duplicate editions don't inflate counts
		if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
			continue
		}
		count++
	}

	return count, nil
}

// CountFiles returns the total number of audio files across all books.
// Books with active segments count their segments; books without segments count as 1 file each.
func (p *PebbleStore) CountFiles() (int, error) {
	// Collect IDs of all primary, non-deleted books
	var bookIDs []string

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
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
		if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
			continue
		}
		bookIDs = append(bookIDs, book.ID)
	}

	totalFiles := 0
	for _, id := range bookIDs {
		files, err := p.GetBookFiles(id)
		if err != nil || len(files) == 0 {
			totalFiles++ // No files means single file
			continue
		}
		activeCount := 0
		for _, f := range files {
			if !f.Missing {
				activeCount++
			}
		}
		if activeCount > 0 {
			totalFiles += activeCount
		} else {
			totalFiles++ // No active files, treat as single file
		}
	}

	return totalFiles, nil
}

func (p *PebbleStore) CountAuthors() (int, error) {
	count := 0
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("author:0"),
		UpperBound: []byte("author:;"),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		if strings.Contains(string(iter.Key()), ":name:") {
			continue
		}
		count++
	}
	return count, nil
}

func (p *PebbleStore) CountSeries() (int, error) {
	count := 0
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("series:0"),
		UpperBound: []byte("series:;"),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		if strings.Contains(string(iter.Key()), ":name:") {
			continue
		}
		count++
	}
	return count, nil
}

func (p *PebbleStore) GetBookCountsByLocation(rootDir string) (library, import_ int, err error) {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return 0, 0, err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") {
			continue
		}
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}
		// Skip non-primary versions so organized originals don't inflate import count
		if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
			continue
		}
		if rootDir != "" && strings.HasPrefix(book.FilePath, rootDir) {
			library++
		} else {
			import_++
		}
	}
	return
}

func (p *PebbleStore) GetBookSizesByLocation(rootDir string) (librarySize, importSize int64, err error) {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return 0, 0, err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") {
			continue
		}
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}
		// Skip non-primary versions (consistent with count logic)
		if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
			continue
		}
		size := int64(0)
		if book.FileSize != nil {
			size = *book.FileSize
		}
		if rootDir != "" && strings.HasPrefix(book.FilePath, rootDir) {
			librarySize += size
		} else {
			importSize += size
		}
	}
	return
}

// GetDashboardStats iterates all books and computes aggregate stats.
// PebbleDB has no SQL, so this scans the full key range.
func (p *PebbleStore) GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{
		StateDistribution:  make(map[string]int),
		FormatDistribution: make(map[string]int),
	}
	if fc, err := p.CountFiles(); err == nil {
		stats.TotalFiles = fc
	}
	if ac, err := p.CountAuthors(); err == nil {
		stats.TotalAuthors = ac
	}
	if sc, err := p.CountSeries(); err == nil {
		stats.TotalSeries = sc
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

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})

	if len(operations) > limit {
		operations = operations[:limit]
	}

	return operations, nil
}

func (p *PebbleStore) ListOperations(limit, offset int) ([]Operation, int, error) {
	var operations []Operation
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("operation:"),
		UpperBound: []byte("operation:~"),
	})
	if err != nil {
		return nil, 0, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var op Operation
		if err := json.Unmarshal(iter.Value(), &op); err != nil {
			continue
		}
		operations = append(operations, op)
	}

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})

	total := len(operations)
	if offset >= len(operations) {
		return []Operation{}, total, nil
	}
	end := offset + limit
	if end > len(operations) {
		end = len(operations)
	}
	return operations[offset:end], total, nil
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

func (p *PebbleStore) UpdateOperationResultData(id string, resultData string) error {
	op, err := p.GetOperationByID(id)
	if err != nil {
		return err
	}
	if op == nil {
		return fmt.Errorf("operation not found: %s", id)
	}
	op.ResultData = &resultData
	data, err := json.Marshal(op)
	if err != nil {
		return err
	}
	return p.db.Set([]byte(fmt.Sprintf("operation:%s", id)), data, pebble.Sync)
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

// Book Tombstones (safe deletion pattern)

func (p *PebbleStore) CreateBookTombstone(book *Book) error {
	data, err := json.Marshal(book)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("tombstone:%s", book.ID))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetBookTombstone(id string) (*Book, error) {
	key := []byte(fmt.Sprintf("tombstone:%s", id))
	val, closer, err := p.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()
	var book Book
	if err := json.Unmarshal(val, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (p *PebbleStore) DeleteBookTombstone(id string) error {
	key := []byte(fmt.Sprintf("tombstone:%s", id))
	return p.db.Delete(key, pebble.Sync)
}

func (p *PebbleStore) ListBookTombstones(limit int) ([]Book, error) {
	var books []Book
	prefix := []byte("tombstone:")

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		books = append(books, book)
		if limit > 0 && len(books) >= limit {
			break
		}
	}
	return books, nil
}

// Operation Summary Logs (persistent across restarts)

func (p *PebbleStore) SaveOperationSummaryLog(op *OperationSummaryLog) error {
	data, err := json.Marshal(op)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("opsummary:%s", op.ID))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetOperationSummaryLog(id string) (*OperationSummaryLog, error) {
	key := []byte(fmt.Sprintf("opsummary:%s", id))
	val, closer, err := p.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()
	var op OperationSummaryLog
	if err := json.Unmarshal(val, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func (p *PebbleStore) ListOperationSummaryLogs(limit, offset int) ([]OperationSummaryLog, error) {
	var logs []OperationSummaryLog
	prefix := []byte("opsummary:")

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var op OperationSummaryLog
		if err := json.Unmarshal(iter.Value(), &op); err != nil {
			continue
		}
		logs = append(logs, op)
	}

	// Sort by created_at descending
	for i := 0; i < len(logs)-1; i++ {
		for j := i + 1; j < len(logs); j++ {
			if logs[j].CreatedAt.After(logs[i].CreatedAt) {
				logs[i], logs[j] = logs[j], logs[i]
			}
		}
	}

	// Apply offset and limit
	if offset >= len(logs) {
		return nil, nil
	}
	logs = logs[offset:]
	if limit > 0 && len(logs) > limit {
		logs = logs[:limit]
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

// Metadata change history operations

func (p *PebbleStore) RecordMetadataChange(record *MetadataChangeRecord) error {
	key := fmt.Sprintf("metadata_change:%s:%s:%d", record.BookID, record.Field, record.ChangedAt.UnixNano())
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return p.db.Set([]byte(key), data, pebble.Sync)
}

func (p *PebbleStore) GetMetadataChangeHistory(bookID string, field string, limit int) ([]MetadataChangeRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	prefix := fmt.Sprintf("metadata_change:%s:%s:", bookID, field)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(prefix + "\xff"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var all []MetadataChangeRecord
	for iter.First(); iter.Valid(); iter.Next() {
		var r MetadataChangeRecord
		if err := json.Unmarshal(iter.Value(), &r); err != nil {
			continue
		}
		all = append(all, r)
	}
	// Reverse for newest-first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (p *PebbleStore) GetBookChangeHistory(bookID string, limit int) ([]MetadataChangeRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	prefix := fmt.Sprintf("metadata_change:%s:", bookID)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(prefix + "\xff"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var all []MetadataChangeRecord
	for iter.First(); iter.Valid(); iter.Next() {
		var r MetadataChangeRecord
		if err := json.Unmarshal(iter.Value(), &r); err != nil {
			continue
		}
		all = append(all, r)
	}
	// Reverse for newest-first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
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

func (p *PebbleStore) UpdateBookSegment(segment *BookSegment) error {
	segment.UpdatedAt = time.Now()
	segment.Version++
	key := []byte(fmt.Sprintf("bfs:%d:%s", segment.BookID, segment.ID))
	data, err := json.Marshal(segment)
	if err != nil {
		return err
	}
	return p.db.Set(key, data, pebble.Sync)
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

// GetBookSegmentByID retrieves a single segment by its ULID.
func (p *PebbleStore) GetBookSegmentByID(segmentID string) (*BookSegment, error) {
	v, closer, err := p.db.Get([]byte("bf:" + segmentID))
	if err != nil {
		return nil, fmt.Errorf("segment not found: %s", segmentID)
	}
	defer closer.Close()
	var seg BookSegment
	if err := json.Unmarshal(v, &seg); err != nil {
		return nil, err
	}
	return &seg, nil
}

// MoveSegmentsToBook reassigns segments to a different book (by numeric ID).
func (p *PebbleStore) MoveSegmentsToBook(segmentIDs []string, targetBookNumericID int) error {
	b := p.db.NewBatch()
	for _, segID := range segmentIDs {
		v, closer, err := p.db.Get([]byte("bf:" + segID))
		if err != nil {
			b.Close()
			return fmt.Errorf("segment not found: %s", segID)
		}
		var seg BookSegment
		if err := json.Unmarshal(v, &seg); err != nil {
			closer.Close()
			b.Close()
			return err
		}
		closer.Close()

		// Delete old index key
		oldKey := []byte(fmt.Sprintf("bfs:%d:%s", seg.BookID, seg.ID))
		if err := b.Delete(oldKey, nil); err != nil {
			b.Close()
			return err
		}

		// Update segment
		seg.BookID = targetBookNumericID
		seg.UpdatedAt = time.Now()
		seg.Version++

		data, _ := json.Marshal(&seg)
		if err := b.Set([]byte("bf:"+segID), data, nil); err != nil {
			b.Close()
			return err
		}
		// Create new index key
		newKey := []byte(fmt.Sprintf("bfs:%d:%s", targetBookNumericID, seg.ID))
		if err := b.Set(newKey, []byte("1"), nil); err != nil {
			b.Close()
			return err
		}
	}
	return b.Commit(pebble.Sync)
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

// ---- Operation State Persistence (resumable operations) ----

func (p *PebbleStore) SaveOperationState(opID string, state []byte) error {
	key := []byte(fmt.Sprintf("opstate:%s", opID))
	return p.db.Set(key, state, pebble.Sync)
}

func (p *PebbleStore) GetOperationState(opID string) ([]byte, error) {
	key := []byte(fmt.Sprintf("opstate:%s", opID))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), nil
}

func (p *PebbleStore) SaveOperationParams(opID string, params []byte) error {
	key := []byte(fmt.Sprintf("opstate:%s:params", opID))
	return p.db.Set(key, params, pebble.Sync)
}

func (p *PebbleStore) GetOperationParams(opID string) ([]byte, error) {
	key := []byte(fmt.Sprintf("opstate:%s:params", opID))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), nil
}

func (p *PebbleStore) DeleteOperationState(opID string) error {
	batch := p.db.NewBatch()
	if err := batch.Delete([]byte(fmt.Sprintf("opstate:%s", opID)), nil); err != nil {
		batch.Close()
		return err
	}
	if err := batch.Delete([]byte(fmt.Sprintf("opstate:%s:params", opID)), nil); err != nil {
		batch.Close()
		return err
	}
	return batch.Commit(pebble.Sync)
}

func (p *PebbleStore) DeleteOperationsByStatus(statuses []string) (int, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	statusSet := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		statusSet[s] = true
	}
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("operation:"),
		UpperBound: []byte("operation:~"),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	deleted := 0
	batch := p.db.NewBatch()
	for iter.First(); iter.Valid(); iter.Next() {
		var op Operation
		if err := json.Unmarshal(iter.Value(), &op); err != nil {
			continue
		}
		if statusSet[op.Status] {
			_ = batch.Delete(iter.Key(), nil)
			deleted++
		}
	}
	if deleted > 0 {
		if err := batch.Commit(pebble.Sync); err != nil {
			return 0, err
		}
	} else {
		batch.Close()
	}
	return deleted, nil
}

func (p *PebbleStore) GetInterruptedOperations() ([]Operation, error) {
	var ops []Operation
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
		if op.Status == "running" || op.Status == "queued" || op.Status == "interrupted" {
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// SaveLibraryFingerprint stores or updates the fingerprint for an iTunes library file.
func (p *PebbleStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32val uint32) error {
	rec := LibraryFingerprintRecord{
		Path:      path,
		Size:      size,
		ModTime:   modTime,
		CRC32:     crc32val,
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("itunes:fingerprint:%s", path))
	return p.db.Set(key, data, pebble.Sync)
}

// GetLibraryFingerprint retrieves the stored fingerprint for an iTunes library file.
func (p *PebbleStore) GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error) {
	key := []byte(fmt.Sprintf("itunes:fingerprint:%s", path))
	data, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var rec LibraryFingerprintRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// CreateDeferredITunesUpdate stores a deferred iTunes path update.
func (p *PebbleStore) CreateDeferredITunesUpdate(bookID, persistentID, oldPath, newPath, updateType string) error {
	id := time.Now().UnixNano()
	rec := DeferredITunesUpdate{
		ID:           int(id),
		BookID:       bookID,
		PersistentID: persistentID,
		OldPath:      oldPath,
		NewPath:      newPath,
		UpdateType:   updateType,
		CreatedAt:    time.Now(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("deferred_itunes:%019d", id))
	return p.db.Set(key, data, pebble.Sync)
}

// GetPendingDeferredITunesUpdates returns all deferred updates that haven't been applied yet.
func (p *PebbleStore) GetPendingDeferredITunesUpdates() ([]DeferredITunesUpdate, error) {
	prefix := []byte("deferred_itunes:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xff),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []DeferredITunesUpdate
	for iter.First(); iter.Valid(); iter.Next() {
		var rec DeferredITunesUpdate
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if rec.AppliedAt == nil {
			results = append(results, rec)
		}
	}
	return results, nil
}

// MarkDeferredITunesUpdateApplied sets the applied_at timestamp on a deferred update.
func (p *PebbleStore) MarkDeferredITunesUpdateApplied(id int) error {
	key := []byte(fmt.Sprintf("deferred_itunes:%019d", id))
	data, closer, err := p.db.Get(key)
	if err != nil {
		return err
	}
	var rec DeferredITunesUpdate
	if err := json.Unmarshal(data, &rec); err != nil {
		closer.Close()
		return err
	}
	closer.Close()

	now := time.Now()
	rec.AppliedAt = &now
	updated, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return p.db.Set(key, updated, pebble.Sync)
}

// GetDeferredITunesUpdatesByBookID returns all deferred updates for a specific book.
func (p *PebbleStore) GetDeferredITunesUpdatesByBookID(bookID string) ([]DeferredITunesUpdate, error) {
	all, err := p.getPendingAndAppliedDeferredUpdates()
	if err != nil {
		return nil, err
	}
	var results []DeferredITunesUpdate
	for _, rec := range all {
		if rec.BookID == bookID {
			results = append(results, rec)
		}
	}
	return results, nil
}

func (p *PebbleStore) getPendingAndAppliedDeferredUpdates() ([]DeferredITunesUpdate, error) {
	prefix := []byte("deferred_itunes:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xff),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []DeferredITunesUpdate
	for iter.First(); iter.Valid(); iter.Next() {
		var rec DeferredITunesUpdate
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		results = append(results, rec)
	}
	return results, nil
}

// CreateExternalIDMapping creates or replaces an external ID mapping.
func (p *PebbleStore) CreateExternalIDMapping(mapping *ExternalIDMapping) error {
	now := time.Now()
	mapping.CreatedAt = now
	mapping.UpdatedAt = now

	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	primaryKey := []byte(fmt.Sprintf("ext_id:%s:%s", mapping.Source, mapping.ExternalID))
	reverseKey := []byte(fmt.Sprintf("ext_id:book:%s:%s:%s", mapping.BookID, mapping.Source, mapping.ExternalID))

	batch := p.db.NewBatch()
	defer batch.Close()

	batch.Set(primaryKey, data, pebble.Sync)
	batch.Set(reverseKey, []byte(mapping.ExternalID), pebble.Sync)

	return batch.Commit(pebble.Sync)
}

// GetBookByExternalID returns the book_id for a non-tombstoned external ID.
func (p *PebbleStore) GetBookByExternalID(source, externalID string) (string, error) {
	key := []byte(fmt.Sprintf("ext_id:%s:%s", source, externalID))
	data, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer closer.Close()

	var mapping ExternalIDMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		return "", err
	}
	if mapping.Tombstoned {
		return "", nil
	}
	return mapping.BookID, nil
}

// GetExternalIDsForBook returns all external ID mappings for a book.
func (p *PebbleStore) GetExternalIDsForBook(bookID string) ([]ExternalIDMapping, error) {
	prefix := []byte(fmt.Sprintf("ext_id:book:%s:", bookID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xff),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []ExternalIDMapping
	for iter.First(); iter.Valid(); iter.Next() {
		// Parse source and externalID from key: ext_id:book:<bookID>:<source>:<externalID>
		parts := strings.SplitN(string(iter.Key()), ":", 5)
		if len(parts) < 5 {
			continue
		}
		source := parts[3]
		extID := parts[4]

		primaryKey := []byte(fmt.Sprintf("ext_id:%s:%s", source, extID))
		data, closer, err := p.db.Get(primaryKey)
		if err != nil {
			continue
		}
		var mapping ExternalIDMapping
		if err := json.Unmarshal(data, &mapping); err != nil {
			closer.Close()
			continue
		}
		closer.Close()
		results = append(results, mapping)
	}
	return results, nil
}

// IsExternalIDTombstoned checks whether an external ID is tombstoned.
func (p *PebbleStore) IsExternalIDTombstoned(source, externalID string) (bool, error) {
	key := []byte(fmt.Sprintf("ext_id:%s:%s", source, externalID))
	data, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer closer.Close()

	var mapping ExternalIDMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		return false, err
	}
	return mapping.Tombstoned, nil
}

// TombstoneExternalID marks an external ID as tombstoned to prevent reimport.
func (p *PebbleStore) TombstoneExternalID(source, externalID string) error {
	key := []byte(fmt.Sprintf("ext_id:%s:%s", source, externalID))
	data, closer, err := p.db.Get(key)
	if err != nil {
		return err
	}
	var mapping ExternalIDMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		closer.Close()
		return err
	}
	closer.Close()

	mapping.Tombstoned = true
	mapping.UpdatedAt = time.Now()

	updated, err := json.Marshal(mapping)
	if err != nil {
		return err
	}
	return p.db.Set(key, updated, pebble.Sync)
}

// ReassignExternalIDs moves all external ID mappings from one book to another (for merges).
func (p *PebbleStore) ReassignExternalIDs(oldBookID, newBookID string) error {
	mappings, err := p.GetExternalIDsForBook(oldBookID)
	if err != nil {
		return err
	}

	batch := p.db.NewBatch()
	defer batch.Close()

	now := time.Now()
	for _, m := range mappings {
		// Delete old reverse key
		oldReverseKey := []byte(fmt.Sprintf("ext_id:book:%s:%s:%s", oldBookID, m.Source, m.ExternalID))
		batch.Delete(oldReverseKey, pebble.Sync)

		// Update mapping
		m.BookID = newBookID
		m.UpdatedAt = now
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		primaryKey := []byte(fmt.Sprintf("ext_id:%s:%s", m.Source, m.ExternalID))
		batch.Set(primaryKey, data, pebble.Sync)

		// Add new reverse key
		newReverseKey := []byte(fmt.Sprintf("ext_id:book:%s:%s:%s", newBookID, m.Source, m.ExternalID))
		batch.Set(newReverseKey, []byte(m.ExternalID), pebble.Sync)
	}

	return batch.Commit(pebble.Sync)
}

// BulkCreateExternalIDMappings inserts multiple external ID mappings.
// Existing mappings are not overwritten (ignore semantics).
func (p *PebbleStore) BulkCreateExternalIDMappings(mappings []ExternalIDMapping) error {
	batch := p.db.NewBatch()
	defer batch.Close()

	now := time.Now()
	for _, m := range mappings {
		primaryKey := []byte(fmt.Sprintf("ext_id:%s:%s", m.Source, m.ExternalID))
		// Check if already exists
		if _, closer, err := p.db.Get(primaryKey); err == nil {
			closer.Close()
			continue // skip existing
		}

		m.CreatedAt = now
		m.UpdatedAt = now
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		batch.Set(primaryKey, data, pebble.Sync)

		reverseKey := []byte(fmt.Sprintf("ext_id:book:%s:%s:%s", m.BookID, m.Source, m.ExternalID))
		batch.Set(reverseKey, []byte(m.ExternalID), pebble.Sync)
	}

	return batch.Commit(pebble.Sync)
}

// MarkExternalIDRemoved is a stub — PID lifecycle tracking is SQLite-only.
func (p *PebbleStore) MarkExternalIDRemoved(source, externalID string) error { return nil }

// SetExternalIDProvenance is a stub — PID lifecycle tracking is SQLite-only.
func (p *PebbleStore) SetExternalIDProvenance(source, externalID, provenance string) error {
	return nil
}

// GetRemovedExternalIDs is a stub — PID lifecycle tracking is SQLite-only.
func (p *PebbleStore) GetRemovedExternalIDs(source string) ([]ExternalIDMapping, error) {
	return nil, nil
}

func (p *PebbleStore) SetRaw(key string, value []byte) error {
	return p.db.Set([]byte(key), value, pebble.Sync)
}

// GetRaw reads a single key. Returns (nil, nil) on miss so callers
// can handle cache-style lookups with a two-valued result instead
// of a sentinel error.
func (p *PebbleStore) GetRaw(key string) ([]byte, error) {
	val, closer, err := p.db.Get([]byte(key))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	// Copy because the closer frees the underlying bytes.
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (p *PebbleStore) DeleteRaw(key string) error {
	return p.db.Delete([]byte(key), pebble.Sync)
}

func (p *PebbleStore) ScanPrefix(prefix string) ([]KVPair, error) {
	prefixBytes := []byte(prefix)
	upperBound := make([]byte, len(prefixBytes))
	copy(upperBound, prefixBytes)
	upperBound[len(upperBound)-1]++
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefixBytes,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var pairs []KVPair
	for iter.First(); iter.Valid(); iter.Next() {
		val := make([]byte, len(iter.Value()))
		copy(val, iter.Value())
		pairs = append(pairs, KVPair{Key: string(iter.Key()), Value: val})
	}
	return pairs, nil
}

func (p *PebbleStore) CreateOperationResult(result *OperationResult) error {
	result.CreatedAt = time.Now()
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("op_result:%s:%s", result.OperationID, result.BookID))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetOperationResults(operationID string) ([]OperationResult, error) {
	prefix := []byte(fmt.Sprintf("op_result:%s:", operationID))
	upperBound := make([]byte, len(prefix))
	copy(upperBound, prefix)
	upperBound[len(upperBound)-1]++
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []OperationResult
	for iter.First(); iter.Valid(); iter.Next() {
		var r OperationResult
		if err := json.Unmarshal(iter.Value(), &r); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func (p *PebbleStore) GetRecentCompletedOperations(limit int) ([]Operation, error) {
	// Scan all operations, collect completed/failed, sort by time, take limit
	prefix := []byte("operation:")
	upperBound := make([]byte, len(prefix))
	copy(upperBound, prefix)
	upperBound[len(upperBound)-1]++
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var ops []Operation
	for iter.First(); iter.Valid(); iter.Next() {
		var op Operation
		if err := json.Unmarshal(iter.Value(), &op); err != nil {
			continue
		}
		if op.Status == "completed" || op.Status == "failed" {
			ops = append(ops, op)
		}
	}

	// Sort by CreatedAt descending
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].CreatedAt.After(ops[j].CreatedAt)
	})

	if len(ops) > limit {
		ops = ops[:limit]
	}
	return ops, nil
}

// --- User Tags (free-form labels on books) ---

// GetBookUserTags returns all user-defined tags for a book.
func (p *PebbleStore) GetBookUserTags(bookID string) ([]string, error) {
	dbKey := []byte(fmt.Sprintf("user_tag:book:%s", bookID))
	value, closer, err := p.db.Get(dbKey)
	if err == pebble.ErrNotFound {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var tags []string
	if err := json.Unmarshal(value, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// SetBookUserTags replaces all user-defined tags for a book.
func (p *PebbleStore) SetBookUserTags(bookID string, tags []string) error {
	dbKey := []byte(fmt.Sprintf("user_tag:book:%s", bookID))
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	return p.db.Set(dbKey, data, pebble.Sync)
}

// AddBookUserTag adds a single user-defined tag to a book (idempotent).
func (p *PebbleStore) AddBookUserTag(bookID string, tag string) error {
	existing, err := p.GetBookUserTags(bookID)
	if err != nil {
		return err
	}
	for _, t := range existing {
		if t == tag {
			return nil // already present
		}
	}
	existing = append(existing, tag)
	return p.SetBookUserTags(bookID, existing)
}

// RemoveBookUserTag removes a single user-defined tag from a book.
func (p *PebbleStore) RemoveBookUserTag(bookID string, tag string) error {
	existing, err := p.GetBookUserTags(bookID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(existing))
	for _, t := range existing {
		if t != tag {
			filtered = append(filtered, t)
		}
	}
	return p.SetBookUserTags(bookID, filtered)
}

// --- Book Alternative Titles ---
//
// Stored as one JSON blob per book under key `alt_titles:book:<id>`.
// The Pebble store doesn't persist to any SQL table so the schema
// from migration 046 is irrelevant here — this is the Pebble-native
// representation used by production. The SQLite implementation is
// only for the test-backed sidecar path.

// GetBookAlternativeTitles returns every alt title for a book.
func (p *PebbleStore) GetBookAlternativeTitles(bookID string) ([]BookAlternativeTitle, error) {
	dbKey := []byte(fmt.Sprintf("alt_titles:book:%s", bookID))
	value, closer, err := p.db.Get(dbKey)
	if err == pebble.ErrNotFound {
		return []BookAlternativeTitle{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var alts []BookAlternativeTitle
	if err := json.Unmarshal(value, &alts); err != nil {
		return nil, err
	}
	return alts, nil
}

// SetBookAlternativeTitles replaces every alt title for a book.
func (p *PebbleStore) SetBookAlternativeTitles(bookID string, titles []BookAlternativeTitle) error {
	dbKey := []byte(fmt.Sprintf("alt_titles:book:%s", bookID))
	// Normalize: make sure every row has book_id populated + a
	// created_at, and default source="user" when omitted.
	now := time.Now().UTC()
	normalized := make([]BookAlternativeTitle, 0, len(titles))
	for _, alt := range titles {
		if alt.Title == "" {
			continue
		}
		if alt.BookID == "" {
			alt.BookID = bookID
		}
		if alt.Source == "" {
			alt.Source = "user"
		}
		if alt.CreatedAt.IsZero() {
			alt.CreatedAt = now
		}
		normalized = append(normalized, alt)
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return p.db.Set(dbKey, data, pebble.Sync)
}

// AddBookAlternativeTitle appends one alt title. Idempotent on (book_id,
// title) — if the same title already exists, the call is a no-op and
// the existing source/language/created_at are preserved.
func (p *PebbleStore) AddBookAlternativeTitle(bookID, title, source, language string) error {
	if title == "" {
		return fmt.Errorf("alternative title cannot be empty")
	}
	existing, err := p.GetBookAlternativeTitles(bookID)
	if err != nil {
		return err
	}
	for _, alt := range existing {
		if alt.Title == title {
			return nil // already present
		}
	}
	existing = append(existing, BookAlternativeTitle{
		BookID:    bookID,
		Title:     title,
		Source:    source,
		Language:  language,
		CreatedAt: time.Now().UTC(),
	})
	return p.SetBookAlternativeTitles(bookID, existing)
}

// RemoveBookAlternativeTitle deletes one variant. No-op if absent.
func (p *PebbleStore) RemoveBookAlternativeTitle(bookID, title string) error {
	existing, err := p.GetBookAlternativeTitles(bookID)
	if err != nil {
		return err
	}
	filtered := make([]BookAlternativeTitle, 0, len(existing))
	for _, alt := range existing {
		if alt.Title != title {
			filtered = append(filtered, alt)
		}
	}
	return p.SetBookAlternativeTitles(bookID, filtered)
}

// Reset clears all data from the store and resets all counters to initial state
func (p *PebbleStore) Reset() error {
	// Use DeleteRange to wipe the entire keyspace in one operation.
	// The range ["\x00", "\xff\xff") covers all possible keys.
	batch := p.db.NewBatch()
	if err := batch.DeleteRange([]byte{0x00}, []byte{0xff, 0xff}, pebble.NoSync); err != nil {
		batch.Close()
		return fmt.Errorf("failed to delete all keys: %w", err)
	}

	// Reinitialize counters to their initial state
	counters := []string{"author", "author_alias", "series", "book", "import_path", "operationlog", "playlist", "playlistitem", "preference"}
	for _, counter := range counters {
		key := fmt.Sprintf("counter:%s", counter)
		if err := batch.Set([]byte(key), []byte("1"), pebble.NoSync); err != nil {
			batch.Close()
			return fmt.Errorf("failed to initialize counter %s: %w", counter, err)
		}
	}

	// Commit with sync for durability
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("failed to commit reset batch: %w", err)
	}

	// Force flush to ensure deletes are persisted to disk
	if err := p.db.Flush(); err != nil {
		return fmt.Errorf("failed to flush after reset: %w", err)
	}

	return nil
}

// CountByPrefix counts keys that start with the given prefix.
func (p *PebbleStore) CountByPrefix(prefix string) (int, error) {
	lb := []byte(prefix)
	ub := make([]byte, len(lb))
	copy(ub, lb)
	ub[len(ub)-1]++

	iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: lb, UpperBound: ub})
	if err != nil {
		return 0, fmt.Errorf("CountByPrefix %q: %w", prefix, err)
	}
	defer iter.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	return count, iter.Error()
}

// WipeByPrefixes deletes all keys that start with any of the given prefix strings.
// Returns the total number of keys deleted.
func (p *PebbleStore) WipeByPrefixes(prefixes []string) (int, error) {
	total := 0
	for _, prefix := range prefixes {
		lb := []byte(prefix)
		// Upper bound: increment the last byte to cover all keys with this prefix.
		ub := make([]byte, len(lb))
		copy(ub, lb)
		ub[len(ub)-1]++

		iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: lb, UpperBound: ub})
		if err != nil {
			return total, fmt.Errorf("wipe prefix %q: iter: %w", prefix, err)
		}

		var keys [][]byte
		for iter.First(); iter.Valid(); iter.Next() {
			k := make([]byte, len(iter.Key()))
			copy(k, iter.Key())
			keys = append(keys, k)
		}
		if err := iter.Close(); err != nil {
			return total, fmt.Errorf("wipe prefix %q: iter close: %w", prefix, err)
		}

		if len(keys) == 0 {
			continue
		}

		batch := p.db.NewBatch()
		for _, k := range keys {
			if err := batch.Delete(k, nil); err != nil {
				batch.Close()
				return total, fmt.Errorf("wipe prefix %q: delete: %w", prefix, err)
			}
		}
		if err := batch.Commit(pebble.Sync); err != nil {
			return total, fmt.Errorf("wipe prefix %q: commit: %w", prefix, err)
		}
		total += len(keys)
	}
	return total, nil
}

// Optimize compacts the PebbleDB database to reclaim space.
func (p *PebbleStore) Optimize() error {
	return p.db.Compact(context.Background(), nil, []byte{0xff}, false)
}

// CreateOperationChange stores an operation change in PebbleDB.
func (p *PebbleStore) CreateOperationChange(change *OperationChange) error {
	if change.ID == "" {
		change.ID = ulid.Make().String()
	}
	change.CreatedAt = time.Now()
	data, err := json.Marshal(change)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("opchange:%s:%s", change.OperationID, change.ID)
	return p.db.Set([]byte(key), data, pebble.Sync)
}

// GetOperationChanges returns all changes for a given operation.
func (p *PebbleStore) GetOperationChanges(operationID string) ([]*OperationChange, error) {
	prefix := []byte(fmt.Sprintf("opchange:%s:", operationID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var changes []*OperationChange
	for iter.First(); iter.Valid(); iter.Next() {
		var c OperationChange
		if err := json.Unmarshal(iter.Value(), &c); err != nil {
			return nil, err
		}
		changes = append(changes, &c)
	}
	return changes, iter.Error()
}

// GetBookChanges returns all changes for a given book.
func (p *PebbleStore) GetBookChanges(bookID string) ([]*OperationChange, error) {
	prefix := []byte("opchange:")
	upperBound := []byte("opchange;") // ':' + 1 = ';'
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var changes []*OperationChange
	for iter.First(); iter.Valid(); iter.Next() {
		var c OperationChange
		if err := json.Unmarshal(iter.Value(), &c); err != nil {
			return nil, err
		}
		if c.BookID == bookID {
			changes = append(changes, &c)
		}
	}
	return changes, iter.Error()
}

// RevertOperationChanges marks all changes for an operation as reverted.
func (p *PebbleStore) RevertOperationChanges(operationID string) error {
	changes, err := p.GetOperationChanges(operationID)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, c := range changes {
		if c.RevertedAt == nil {
			c.RevertedAt = &now
			data, err := json.Marshal(c)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("opchange:%s:%s", c.OperationID, c.ID)
			if err := p.db.Set([]byte(key), data, pebble.Sync); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateAuthorTombstone writes a tombstone that redirects oldID to canonicalID.
func (p *PebbleStore) CreateAuthorTombstone(oldID, canonicalID int) error {
	key := []byte(fmt.Sprintf("author_tombstone:%d", oldID))
	value := []byte(strconv.Itoa(canonicalID))
	return p.db.Set(key, value, pebble.Sync)
}

// GetAuthorTombstone returns the canonical author ID for a tombstoned author.
// Returns 0 if no tombstone exists.
func (p *PebbleStore) GetAuthorTombstone(oldID int) (int, error) {
	key := []byte(fmt.Sprintf("author_tombstone:%d", oldID))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closer.Close()

	canonicalID, err := strconv.Atoi(string(value))
	if err != nil {
		return 0, fmt.Errorf("invalid tombstone value for author %d: %w", oldID, err)
	}
	return canonicalID, nil
}

// ResolveTombstoneChains finds chains like A→B→C and collapses them so A→C, B→C.
// Returns the number of tombstones updated.
func (p *PebbleStore) ResolveTombstoneChains() (int, error) {
	// Collect all tombstones
	tombstones := make(map[int]int) // oldID → canonicalID
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("author_tombstone:"),
		UpperBound: []byte("author_tombstone;"),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create tombstone iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		keyStr := string(iter.Key())
		parts := strings.SplitN(keyStr, ":", 2)
		if len(parts) != 2 {
			continue
		}
		oldID, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		val, valErr := iter.ValueAndErr()
		if valErr != nil {
			continue
		}
		canonicalID, err := strconv.Atoi(string(val))
		if err != nil {
			continue
		}
		tombstones[oldID] = canonicalID
	}

	// Resolve chains: follow each tombstone to its final destination
	updated := 0
	for oldID, canonicalID := range tombstones {
		finalID := canonicalID
		visited := map[int]bool{oldID: true}
		for {
			nextID, exists := tombstones[finalID]
			if !exists {
				break
			}
			if visited[finalID] {
				break // cycle detection
			}
			visited[finalID] = true
			finalID = nextID
		}
		if finalID != canonicalID {
			// Update the tombstone to point directly to the final destination
			key := []byte(fmt.Sprintf("author_tombstone:%d", oldID))
			if err := p.db.Set(key, []byte(strconv.Itoa(finalID)), pebble.Sync); err != nil {
				return updated, fmt.Errorf("failed to update tombstone %d: %w", oldID, err)
			}
			updated++
		}
	}

	return updated, nil
}

// AddSystemActivityLog stores a log entry from a housekeeping goroutine.
func (p *PebbleStore) AddSystemActivityLog(source, level, message string) error {
	key := fmt.Sprintf("syslog:%s:%s", time.Now().Format(time.RFC3339Nano), source)
	val := SystemActivityLog{
		Source:    source,
		Level:     level,
		Message:   message,
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return p.db.Set([]byte(key), data, pebble.Sync)
}

// GetSystemActivityLogs retrieves recent system activity log entries.
func (p *PebbleStore) GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error) {
	prefix := []byte("syslog:")
	upperBound := append(append([]byte{}, prefix...), 0xFF)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var logs []SystemActivityLog
	for iter.Last(); iter.Valid(); iter.Prev() {
		var l SystemActivityLog
		if err := json.Unmarshal(iter.Value(), &l); err != nil {
			continue
		}
		if source != "" && l.Source != source {
			continue
		}
		logs = append(logs, l)
		if len(logs) >= limit {
			break
		}
	}
	return logs, nil
}

// PruneOperationLogs deletes operation log entries older than the given time.
func (p *PebbleStore) PruneOperationLogs(olderThan time.Time) (int, error) {
	return p.pruneByTimestampPrefix("oplog:", olderThan)
}

// PruneOperationChanges deletes operation change entries older than the given time.
func (p *PebbleStore) PruneOperationChanges(olderThan time.Time) (int, error) {
	return p.pruneByTimestampPrefix("opchange:", olderThan)
}

// PruneSystemActivityLogs deletes system activity log entries older than the given time.
func (p *PebbleStore) PruneSystemActivityLogs(olderThan time.Time) (int, error) {
	return p.pruneByTimestampPrefix("syslog:", olderThan)
}

// pruneByTimestampPrefix deletes all keys with the given prefix whose
// embedded RFC3339 timestamp is before olderThan.
func (p *PebbleStore) pruneByTimestampPrefix(prefix string, olderThan time.Time) (int, error) {
	prefixBytes := []byte(prefix)
	upperBound := append(append([]byte{}, prefixBytes...), 0xFF)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefixBytes,
		UpperBound: upperBound,
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	deleted := 0
	batch := p.db.NewBatch()
	defer batch.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		parts := strings.SplitN(strings.TrimPrefix(key, prefix), ":", 2)
		if len(parts) == 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			continue
		}
		if ts.Before(olderThan) {
			_ = batch.Delete(iter.Key(), nil)
			deleted++
		}
	}
	if deleted > 0 {
		return deleted, batch.Commit(pebble.Sync)
	}
	return 0, nil
}

// derefInt64 safely dereferences a *int64, returning 0 for nil.
func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// derefBool safely dereferences a *bool, returning false for nil.
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// GetScanCacheMap returns a map of file_path -> ScanCacheEntry for all books
// that have a non-empty FilePath and a non-nil LastScanMtime.
func (p *PebbleStore) GetScanCacheMap() (map[string]ScanCacheEntry, error) {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	result := make(map[string]ScanCacheEntry)
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") {
			continue
		}
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.FilePath == "" || book.LastScanMtime == nil {
			continue
		}
		result[book.FilePath] = ScanCacheEntry{
			Mtime:       derefInt64(book.LastScanMtime),
			Size:        derefInt64(book.LastScanSize),
			NeedsRescan: derefBool(book.NeedsRescan),
		}
	}
	return result, nil
}

// UpdateScanCache sets LastScanMtime, LastScanSize, and clears NeedsRescan for a book.
func (p *PebbleStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	book, err := p.GetBookByID(bookID)
	if err != nil {
		return err
	}
	if book == nil {
		return nil // non-fatal: book not found
	}
	book.LastScanMtime = &mtime
	book.LastScanSize = &size
	f := false
	book.NeedsRescan = &f
	_, err = p.UpdateBook(bookID, book)
	return err
}

// MarkNeedsRescan sets NeedsRescan = true for the given book.
func (p *PebbleStore) MarkNeedsRescan(bookID string) error {
	book, err := p.GetBookByID(bookID)
	if err != nil {
		return err
	}
	if book == nil {
		return nil // non-fatal: book not found
	}
	t := true
	book.NeedsRescan = &t
	_, err = p.UpdateBook(bookID, book)
	return err
}

// GetDirtyBookFolders returns a deduplicated list of parent directories for all
// books that have NeedsRescan = true.
func (p *PebbleStore) GetDirtyBookFolders() ([]string, error) {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	seen := make(map[string]struct{})
	var dirs []string
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") {
			continue
		}
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.FilePath == "" || !derefBool(book.NeedsRescan) {
			continue
		}
		dir := filepath.Dir(book.FilePath)
		if _, ok := seen[dir]; !ok {
			seen[dir] = struct{}{}
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// RecordPathChange stores a path change record in PebbleDB.
// Key format: path_history:<book_id>:<timestamp>
func (p *PebbleStore) RecordPathChange(change *BookPathChange) error {
	ts := time.Now().UnixNano()
	change.CreatedAt = time.Now()
	change.ID = int(ts)
	data, err := json.Marshal(change)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("path_history:%s:%019d", change.BookID, ts))
	return p.db.Set(key, data, pebble.Sync)
}

// GetBookPathHistory returns all path changes for a book, newest first.
func (p *PebbleStore) GetBookPathHistory(bookID string) ([]BookPathChange, error) {
	prefix := []byte(fmt.Sprintf("path_history:%s:", bookID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []BookPathChange
	for iter.First(); iter.Valid(); iter.Next() {
		var c BookPathChange
		if err := json.Unmarshal(iter.Value(), &c); err != nil {
			continue
		}
		results = append(results, c)
	}
	// Reverse for newest-first
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results, nil
}

// AddBookTag adds a user-sourced tag to a book. Server code that
// auto-applies tags should use AddBookTagWithSource so provenance
// is preserved.
func (p *PebbleStore) AddBookTag(bookID, tag string) error {
	return p.AddBookTagWithSource(bookID, tag, "user")
}

// AddBookTagWithSource adds a tag with an explicit source. Typical
// sources: "user" (default), "system" (auto-applied by the server).
// Upserts when the row already exists — later writes overwrite the
// source field so a user-claimed tag can promote to system or vice
// versa without needing a delete-first step.
func (p *PebbleStore) AddBookTagWithSource(bookID, tag, source string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	if source == "" {
		source = "user"
	}

	bt := BookTag{
		BookID:    bookID,
		Tag:       tag,
		Source:    source,
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(bt)
	if err != nil {
		return err
	}

	// Primary key: book_tag:<bookID>:<tag>
	bookTagKey := []byte(fmt.Sprintf("book_tag:%s:%s", bookID, tag))
	if err := p.db.Set(bookTagKey, data, pebble.Sync); err != nil {
		return err
	}

	// Reverse index: tag_idx:<tag>:<bookID>
	tagIdxKey := []byte(fmt.Sprintf("tag_idx:%s:%s", tag, bookID))
	return p.db.Set(tagIdxKey, []byte{}, pebble.Sync)
}

// RemoveBookTag removes a tag from a book regardless of source.
func (p *PebbleStore) RemoveBookTag(bookID, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}

	bookTagKey := []byte(fmt.Sprintf("book_tag:%s:%s", bookID, tag))
	if err := p.db.Delete(bookTagKey, pebble.Sync); err != nil && err != pebble.ErrNotFound {
		return err
	}

	tagIdxKey := []byte(fmt.Sprintf("tag_idx:%s:%s", tag, bookID))
	if err := p.db.Delete(tagIdxKey, pebble.Sync); err != nil && err != pebble.ErrNotFound {
		return err
	}

	return nil
}

// RemoveBookTagsByPrefix removes every tag on a book whose name
// begins with `prefix`, optionally scoped to a specific source.
// Used to clear a namespace before writing a fresh system tag —
// e.g., re-applying metadata from a new source removes any
// existing `metadata:source:*` system tags first so each book has
// exactly one source tag at a time.
//
// If `source` is empty, all sources match.
func (p *PebbleStore) RemoveBookTagsByPrefix(bookID, prefix, source string) error {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return fmt.Errorf("prefix cannot be empty")
	}

	detailed, err := p.GetBookTagsDetailed(bookID)
	if err != nil {
		return err
	}
	for _, bt := range detailed {
		if !strings.HasPrefix(bt.Tag, prefix) {
			continue
		}
		if source != "" && bt.Source != source {
			continue
		}
		if err := p.RemoveBookTag(bookID, bt.Tag); err != nil {
			return err
		}
	}
	return nil
}

// GetBookTags returns all tag strings for a book, sorted alphabetically.
func (p *PebbleStore) GetBookTags(bookID string) ([]string, error) {
	prefix := []byte(fmt.Sprintf("book_tag:%s:", bookID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var tags []string
	for iter.First(); iter.Valid(); iter.Next() {
		var bt BookTag
		if err := json.Unmarshal(iter.Value(), &bt); err != nil {
			continue
		}
		tags = append(tags, bt.Tag)
	}
	sort.Strings(tags)
	return tags, nil
}

// GetBookTagsDetailed returns tags with their source attribution.
// Rows written before migration 47 deserialize with source="" which
// we promote to "user" so downstream filters treat them as user
// tags (the sensible default for legacy data).
func (p *PebbleStore) GetBookTagsDetailed(bookID string) ([]BookTag, error) {
	prefix := []byte(fmt.Sprintf("book_tag:%s:", bookID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var out []BookTag
	for iter.First(); iter.Valid(); iter.Next() {
		var bt BookTag
		if err := json.Unmarshal(iter.Value(), &bt); err != nil {
			continue
		}
		if bt.Source == "" {
			bt.Source = "user"
		}
		out = append(out, bt)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

// SetBookTags replaces all USER tags on a book with the given set.
// System tags (dedup:*, metadata:source:*, ...) are preserved so the
// user-facing bulk-replace doesn't clobber server-applied provenance.
func (p *PebbleStore) SetBookTags(bookID string, tags []string) error {
	detailed, err := p.GetBookTagsDetailed(bookID)
	if err != nil {
		return err
	}

	// Normalize incoming tags.
	normalized := make(map[string]bool)
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			normalized[t] = true
		}
	}

	// Existing user tags we may need to drop.
	existingUser := make(map[string]bool)
	for _, bt := range detailed {
		if bt.Source == "user" || bt.Source == "" {
			existingUser[bt.Tag] = true
		}
	}

	// Remove user tags not in new set.
	for t := range existingUser {
		if !normalized[t] {
			if err := p.RemoveBookTag(bookID, t); err != nil {
				return err
			}
		}
	}

	// Add user tags not already present.
	for t := range normalized {
		if !existingUser[t] {
			if err := p.AddBookTagWithSource(bookID, t, "user"); err != nil {
				return err
			}
		}
	}

	return nil
}

// ListAllTags returns all unique tags with their usage counts.
func (p *PebbleStore) ListAllTags() ([]TagWithCount, error) {
	prefix := []byte("tag_idx:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	counts := make(map[string]int)
	for iter.First(); iter.Valid(); iter.Next() {
		// Key format: tag_idx:<tag>:<bookID>
		key := string(iter.Key())
		parts := strings.SplitN(key, ":", 3)
		if len(parts) >= 2 {
			counts[parts[1]]++
		}
	}

	result := make([]TagWithCount, 0, len(counts))
	for tag, count := range counts {
		result = append(result, TagWithCount{Tag: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Tag < result[j].Tag
	})
	return result, nil
}

// GetBooksByTag returns all book IDs that have the given tag.
func (p *PebbleStore) GetBooksByTag(tag string) ([]string, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}

	prefix := []byte(fmt.Sprintf("tag_idx:%s:", tag))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var bookIDs []string
	for iter.First(); iter.Valid(); iter.Next() {
		// Key format: tag_idx:<tag>:<bookID>
		key := string(iter.Key())
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 {
			bookIDs = append(bookIDs, parts[2])
		}
	}
	return bookIDs, nil
}

// ---------- Author / Series tag storage ----------
//
// Authors and series follow the same tag shape as books. Pebble
// keys are parameterized by a keyspace prefix so the same helper
// functions serve all three entity types:
//
//	Books:   book_tag:<bookID>:<tag>       tag_idx:<tag>:<bookID>
//	Authors: author_tag:<authorID>:<tag>   author_tag_idx:<tag>:<authorID>
//	Series:  series_tag:<seriesID>:<tag>   series_tag_idx:<tag>:<seriesID>
//
// Entity IDs are string-formatted for author/series (integer → string)
// because Pebble keys are flat bytes — the caller provides the ID
// formatting and the helper never has to care about the type.

// pebbleTagKeyspace bundles the prefixes for one entity type.
type pebbleTagKeyspace struct {
	tagPrefix    string // e.g. "author_tag:"
	indexPrefix  string // e.g. "author_tag_idx:"
	entityLabel  string // for error messages / logging
}

var (
	bookTagKeyspace = pebbleTagKeyspace{
		tagPrefix:   "book_tag:",
		indexPrefix: "tag_idx:",
		entityLabel: "book",
	}
	authorTagKeyspace = pebbleTagKeyspace{
		tagPrefix:   "author_tag:",
		indexPrefix: "author_tag_idx:",
		entityLabel: "author",
	}
	seriesTagKeyspace = pebbleTagKeyspace{
		tagPrefix:   "series_tag:",
		indexPrefix: "series_tag_idx:",
		entityLabel: "series",
	}
)

// pebbleAddTag upserts a tag for any entity type. Serializes a
// BookTag with the source field so it survives round-trips.
func (p *PebbleStore) pebbleAddTag(ks pebbleTagKeyspace, entityID, tag, source string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	if source == "" {
		source = "user"
	}
	bt := BookTag{
		BookID:    entityID, // reused as the generic entity ID
		Tag:       tag,
		Source:    source,
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(bt)
	if err != nil {
		return err
	}
	primary := []byte(fmt.Sprintf("%s%s:%s", ks.tagPrefix, entityID, tag))
	if err := p.db.Set(primary, data, pebble.Sync); err != nil {
		return err
	}
	idx := []byte(fmt.Sprintf("%s%s:%s", ks.indexPrefix, tag, entityID))
	return p.db.Set(idx, []byte{}, pebble.Sync)
}

func (p *PebbleStore) pebbleRemoveTag(ks pebbleTagKeyspace, entityID, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	primary := []byte(fmt.Sprintf("%s%s:%s", ks.tagPrefix, entityID, tag))
	if err := p.db.Delete(primary, pebble.Sync); err != nil && err != pebble.ErrNotFound {
		return err
	}
	idx := []byte(fmt.Sprintf("%s%s:%s", ks.indexPrefix, tag, entityID))
	if err := p.db.Delete(idx, pebble.Sync); err != nil && err != pebble.ErrNotFound {
		return err
	}
	return nil
}

func (p *PebbleStore) pebbleGetTags(ks pebbleTagKeyspace, entityID string) ([]string, error) {
	prefix := []byte(fmt.Sprintf("%s%s:", ks.tagPrefix, entityID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var tags []string
	for iter.First(); iter.Valid(); iter.Next() {
		var bt BookTag
		if err := json.Unmarshal(iter.Value(), &bt); err != nil {
			continue
		}
		tags = append(tags, bt.Tag)
	}
	sort.Strings(tags)
	return tags, nil
}

func (p *PebbleStore) pebbleGetTagsDetailed(ks pebbleTagKeyspace, entityID string) ([]BookTag, error) {
	prefix := []byte(fmt.Sprintf("%s%s:", ks.tagPrefix, entityID))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var out []BookTag
	for iter.First(); iter.Valid(); iter.Next() {
		var bt BookTag
		if err := json.Unmarshal(iter.Value(), &bt); err != nil {
			continue
		}
		if bt.Source == "" {
			bt.Source = "user"
		}
		out = append(out, bt)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

func (p *PebbleStore) pebbleRemoveTagsByPrefix(ks pebbleTagKeyspace, entityID, prefix, source string) error {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return fmt.Errorf("prefix cannot be empty")
	}
	detailed, err := p.pebbleGetTagsDetailed(ks, entityID)
	if err != nil {
		return err
	}
	for _, bt := range detailed {
		if !strings.HasPrefix(bt.Tag, prefix) {
			continue
		}
		if source != "" && bt.Source != source {
			continue
		}
		if err := p.pebbleRemoveTag(ks, entityID, bt.Tag); err != nil {
			return err
		}
	}
	return nil
}

func (p *PebbleStore) pebbleSetTags(ks pebbleTagKeyspace, entityID string, tags []string) error {
	detailed, err := p.pebbleGetTagsDetailed(ks, entityID)
	if err != nil {
		return err
	}
	normalized := make(map[string]bool)
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			normalized[t] = true
		}
	}
	existingUser := make(map[string]bool)
	for _, bt := range detailed {
		if bt.Source == "user" || bt.Source == "" {
			existingUser[bt.Tag] = true
		}
	}
	for t := range existingUser {
		if !normalized[t] {
			if err := p.pebbleRemoveTag(ks, entityID, t); err != nil {
				return err
			}
		}
	}
	for t := range normalized {
		if !existingUser[t] {
			if err := p.pebbleAddTag(ks, entityID, t, "user"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *PebbleStore) pebbleListAllTags(ks pebbleTagKeyspace) ([]TagWithCount, error) {
	prefix := []byte(ks.indexPrefix)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	counts := make(map[string]int)
	for iter.First(); iter.Valid(); iter.Next() {
		// Key format: <indexPrefix><tag>:<entityID>
		key := string(iter.Key())
		rest := strings.TrimPrefix(key, ks.indexPrefix)
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) >= 1 {
			counts[parts[0]]++
		}
	}

	result := make([]TagWithCount, 0, len(counts))
	for tag, count := range counts {
		result = append(result, TagWithCount{Tag: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Tag < result[j].Tag
	})
	return result, nil
}

func (p *PebbleStore) pebbleEntitiesByTag(ks pebbleTagKeyspace, tag string) ([]string, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}
	prefix := []byte(fmt.Sprintf("%s%s:", ks.indexPrefix, tag))
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix[:len(prefix)-1], prefix[len(prefix)-1]+1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var ids []string
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		rest := strings.TrimPrefix(key, fmt.Sprintf("%s%s:", ks.indexPrefix, tag))
		if rest != "" {
			ids = append(ids, rest)
		}
	}
	return ids, nil
}

// ---------- Author tag wrappers (PebbleStore) ----------

func (p *PebbleStore) AddAuthorTag(authorID int, tag string) error {
	return p.pebbleAddTag(authorTagKeyspace, strconv.Itoa(authorID), tag, "user")
}
func (p *PebbleStore) AddAuthorTagWithSource(authorID int, tag, source string) error {
	return p.pebbleAddTag(authorTagKeyspace, strconv.Itoa(authorID), tag, source)
}
func (p *PebbleStore) RemoveAuthorTag(authorID int, tag string) error {
	return p.pebbleRemoveTag(authorTagKeyspace, strconv.Itoa(authorID), tag)
}
func (p *PebbleStore) RemoveAuthorTagsByPrefix(authorID int, prefix, source string) error {
	return p.pebbleRemoveTagsByPrefix(authorTagKeyspace, strconv.Itoa(authorID), prefix, source)
}
func (p *PebbleStore) GetAuthorTags(authorID int) ([]string, error) {
	return p.pebbleGetTags(authorTagKeyspace, strconv.Itoa(authorID))
}
func (p *PebbleStore) GetAuthorTagsDetailed(authorID int) ([]BookTag, error) {
	return p.pebbleGetTagsDetailed(authorTagKeyspace, strconv.Itoa(authorID))
}
func (p *PebbleStore) SetAuthorTags(authorID int, tags []string) error {
	return p.pebbleSetTags(authorTagKeyspace, strconv.Itoa(authorID), tags)
}
func (p *PebbleStore) ListAllAuthorTags() ([]TagWithCount, error) {
	return p.pebbleListAllTags(authorTagKeyspace)
}
func (p *PebbleStore) GetAuthorsByTag(tag string) ([]int, error) {
	raw, err := p.pebbleEntitiesByTag(authorTagKeyspace, tag)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(raw))
	for _, s := range raw {
		n, err := strconv.Atoi(s)
		if err != nil {
			continue // skip malformed entries
		}
		ids = append(ids, n)
	}
	return ids, nil
}

// ---------- Series tag wrappers (PebbleStore) ----------

func (p *PebbleStore) AddSeriesTag(seriesID int, tag string) error {
	return p.pebbleAddTag(seriesTagKeyspace, strconv.Itoa(seriesID), tag, "user")
}
func (p *PebbleStore) AddSeriesTagWithSource(seriesID int, tag, source string) error {
	return p.pebbleAddTag(seriesTagKeyspace, strconv.Itoa(seriesID), tag, source)
}
func (p *PebbleStore) RemoveSeriesTag(seriesID int, tag string) error {
	return p.pebbleRemoveTag(seriesTagKeyspace, strconv.Itoa(seriesID), tag)
}
func (p *PebbleStore) RemoveSeriesTagsByPrefix(seriesID int, prefix, source string) error {
	return p.pebbleRemoveTagsByPrefix(seriesTagKeyspace, strconv.Itoa(seriesID), prefix, source)
}
func (p *PebbleStore) GetSeriesTags(seriesID int) ([]string, error) {
	return p.pebbleGetTags(seriesTagKeyspace, strconv.Itoa(seriesID))
}
func (p *PebbleStore) GetSeriesTagsDetailed(seriesID int) ([]BookTag, error) {
	return p.pebbleGetTagsDetailed(seriesTagKeyspace, strconv.Itoa(seriesID))
}
func (p *PebbleStore) SetSeriesTags(seriesID int, tags []string) error {
	return p.pebbleSetTags(seriesTagKeyspace, strconv.Itoa(seriesID), tags)
}
func (p *PebbleStore) ListAllSeriesTags() ([]TagWithCount, error) {
	return p.pebbleListAllTags(seriesTagKeyspace)
}
func (p *PebbleStore) GetSeriesByTag(tag string) ([]int, error) {
	raw, err := p.pebbleEntitiesByTag(seriesTagKeyspace, tag)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(raw))
	for _, s := range raw {
		n, err := strconv.Atoi(s)
		if err != nil {
			continue
		}
		ids = append(ids, n)
	}
	return ids, nil
}

// ---- BookFile CRUD ----

// bookFilePathCRC returns the lowercase hex CRC32 of the file path, used as
// the secondary index key suffix for book_file_path lookups.
func bookFilePathCRC(filePath string) string {
	return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(filePath)))
}

// getBookFileByID fetches a BookFile by its primary key (book_file:<bookID>:<fileID>).
func (s *PebbleStore) getBookFileByID(bookID, fileID string) (*BookFile, error) {
	key := []byte(fmt.Sprintf("book_file:%s:%s", bookID, fileID))
	value, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var f BookFile
	if err := json.Unmarshal(value, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// writeBookFileSecondaryIndexes adds the PID and path secondary index entries
// to the batch. Either index is only written when the relevant field is non-empty.
func writeBookFileSecondaryIndexes(batch *pebble.Batch, f *BookFile) error {
	ref := []byte(fmt.Sprintf("%s:%s", f.BookID, f.ID))

	if f.ITunesPersistentID != "" {
		pidKey := []byte(fmt.Sprintf("book_file_pid:%s", f.ITunesPersistentID))
		if err := batch.Set(pidKey, ref, nil); err != nil {
			return err
		}
	}

	if f.FilePath != "" {
		pathKey := []byte(fmt.Sprintf("book_file_path:%s", bookFilePathCRC(f.FilePath)))
		if err := batch.Set(pathKey, ref, nil); err != nil {
			return err
		}
	}
	return nil
}

// deleteBookFileSecondaryIndexes removes PID and path secondary index entries
// from the batch for the given BookFile.
func deleteBookFileSecondaryIndexes(batch *pebble.Batch, f *BookFile) error {
	if f.ITunesPersistentID != "" {
		pidKey := []byte(fmt.Sprintf("book_file_pid:%s", f.ITunesPersistentID))
		if err := batch.Delete(pidKey, nil); err != nil {
			return err
		}
	}

	if f.FilePath != "" {
		pathKey := []byte(fmt.Sprintf("book_file_path:%s", bookFilePathCRC(f.FilePath)))
		if err := batch.Delete(pathKey, nil); err != nil {
			return err
		}
	}
	return nil
}

// CreateBookFile stores a new BookFile, generating a ULID if the ID is empty.
// It writes the primary key book_file:<bookID>:<fileID> and secondary indexes
// for iTunes PID and file path (when non-empty) atomically in a single batch.
func (s *PebbleStore) CreateBookFile(file *BookFile) error {
	if file.ID == "" {
		id, err := newULID()
		if err != nil {
			return err
		}
		file.ID = id
	}

	now := time.Now()
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	file.UpdatedAt = now

	data, err := json.Marshal(file)
	if err != nil {
		return err
	}

	batch := s.db.NewBatch()

	key := []byte(fmt.Sprintf("book_file:%s:%s", file.BookID, file.ID))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return err
	}

	if err := writeBookFileSecondaryIndexes(batch, file); err != nil {
		batch.Close()
		return err
	}

	return batch.Commit(pebble.Sync)
}

// UpdateBookFile replaces an existing BookFile, cleaning up stale secondary
// indexes when the PID or path changes.
func (s *PebbleStore) UpdateBookFile(id string, file *BookFile) error {
	// We need the bookID to build the primary key; it must be set on file.
	old, err := s.getBookFileByID(file.BookID, id)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("book file not found: %s", id)
	}

	file.ID = id
	file.CreatedAt = old.CreatedAt
	file.UpdatedAt = time.Now()

	data, err := json.Marshal(file)
	if err != nil {
		return err
	}

	batch := s.db.NewBatch()

	// Remove stale secondary indexes before writing new ones.
	if err := deleteBookFileSecondaryIndexes(batch, old); err != nil {
		batch.Close()
		return err
	}

	key := []byte(fmt.Sprintf("book_file:%s:%s", file.BookID, file.ID))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return err
	}

	if err := writeBookFileSecondaryIndexes(batch, file); err != nil {
		batch.Close()
		return err
	}

	return batch.Commit(pebble.Sync)
}

// GetBookFiles returns all BookFile records for the given bookID by iterating
// the prefix book_file:<bookID>:.
func (s *PebbleStore) GetBookFiles(bookID string) ([]BookFile, error) {
	prefix := []byte(fmt.Sprintf("book_file:%s:", bookID))
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(append([]byte(nil), prefix...), 0xFF),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var files []BookFile
	for iter.First(); iter.Valid(); iter.Next() {
		var f BookFile
		if err := json.Unmarshal(iter.Value(), &f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// GetBookFileByPID looks up a BookFile by iTunes persistent ID using the
// book_file_pid:<pid> secondary index.
func (s *PebbleStore) GetBookFileByPID(itunesPID string) (*BookFile, error) {
	if itunesPID == "" {
		return nil, nil
	}
	pidKey := []byte(fmt.Sprintf("book_file_pid:%s", itunesPID))
	value, closer, err := s.db.Get(pidKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ref := string(value)
	closer.Close()

	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("corrupt book_file_pid index value: %q", ref)
	}
	return s.getBookFileByID(parts[0], parts[1])
}

// GetBookFileByPath looks up a BookFile by file path using the
// book_file_path:<crc32hex> secondary index.
func (s *PebbleStore) GetBookFileByPath(filePath string) (*BookFile, error) {
	if filePath == "" {
		return nil, nil
	}
	pathKey := []byte(fmt.Sprintf("book_file_path:%s", bookFilePathCRC(filePath)))
	value, closer, err := s.db.Get(pathKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ref := string(value)
	closer.Close()

	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("corrupt book_file_path index value: %q", ref)
	}
	f, err := s.getBookFileByID(parts[0], parts[1])
	if err != nil {
		return nil, err
	}
	// Verify the stored path matches (CRC collision guard).
	if f != nil && f.FilePath != filePath {
		return nil, nil
	}
	return f, nil
}

// DeleteBookFile removes the BookFile with the given ID (and its secondary
// indexes) from the store. It requires the bookID to be available on the
// struct; the caller must have obtained the record first, so we scan the
// secondary path index or retrieve by ID. Since we only have fileID here we
// perform a prefix scan to locate the record.
func (s *PebbleStore) DeleteBookFile(id string) error {
	// Scan all book_file: keys to find the one with this file ID.
	prefix := []byte("book_file:")
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: []byte("book_file;"),
	})
	if err != nil {
		return err
	}

	var found *BookFile
	for iter.First(); iter.Valid(); iter.Next() {
		// Key format: book_file:<bookID>:<fileID>
		key := string(iter.Key())
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 && parts[2] == id {
			var f BookFile
			if jsonErr := json.Unmarshal(iter.Value(), &f); jsonErr == nil {
				found = &f
			}
			break
		}
	}
	iter.Close()

	if found == nil {
		return nil // already gone
	}

	batch := s.db.NewBatch()

	// Delete primary key.
	primaryKey := []byte(fmt.Sprintf("book_file:%s:%s", found.BookID, found.ID))
	if err := batch.Delete(primaryKey, nil); err != nil {
		batch.Close()
		return err
	}

	// Delete secondary indexes.
	if err := deleteBookFileSecondaryIndexes(batch, found); err != nil {
		batch.Close()
		return err
	}

	return batch.Commit(pebble.Sync)
}

// DeleteBookFilesForBook removes all BookFile records for a given bookID,
// including their secondary indexes.
func (s *PebbleStore) DeleteBookFilesForBook(bookID string) error {
	files, err := s.GetBookFiles(bookID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	batch := s.db.NewBatch()

	for i := range files {
		f := &files[i]
		primaryKey := []byte(fmt.Sprintf("book_file:%s:%s", f.BookID, f.ID))
		if err := batch.Delete(primaryKey, nil); err != nil {
			batch.Close()
			return err
		}
		if err := deleteBookFileSecondaryIndexes(batch, f); err != nil {
			batch.Close()
			return err
		}
	}

	return batch.Commit(pebble.Sync)
}

// UpsertBookFile creates or updates a BookFile. Lookup order:
//  1. If ITunesPersistentID is set, look up by PID.
//  2. Otherwise look up by FilePath.
//  3. If still not found, create a new record.
func (s *PebbleStore) UpsertBookFile(file *BookFile) error {
	var existing *BookFile
	var err error

	if file.ITunesPersistentID != "" {
		existing, err = s.GetBookFileByPID(file.ITunesPersistentID)
		if err != nil {
			return err
		}
	}

	if existing == nil && file.FilePath != "" {
		existing, err = s.GetBookFileByPath(file.FilePath)
		if err != nil {
			return err
		}
	}

	if existing == nil {
		return s.CreateBookFile(file)
	}

	// Preserve the existing ID and bookID; update in place.
	file.ID = existing.ID
	file.BookID = existing.BookID
	return s.UpdateBookFile(existing.ID, file)
}

// BatchUpsertBookFiles upserts a slice of BookFile records using a single
// PebbleDB batch for all writes. Each file is matched by iTunes persistent ID
// (if set) or by file path. This amortises the per-Commit overhead across
// all records in the slice.
func (s *PebbleStore) BatchUpsertBookFiles(files []*BookFile) error {
	if len(files) == 0 {
		return nil
	}

	batch := s.db.NewBatch()

	now := time.Now()
	for _, file := range files {
		if file == nil {
			continue
		}

		var existing *BookFile
		var lookupErr error

		if file.ITunesPersistentID != "" {
			existing, lookupErr = s.GetBookFileByPID(file.ITunesPersistentID)
		}
		if lookupErr != nil {
			batch.Close()
			return lookupErr
		}
		if existing == nil && file.FilePath != "" {
			existing, lookupErr = s.GetBookFileByPath(file.FilePath)
			if lookupErr != nil {
				batch.Close()
				return lookupErr
			}
		}

		if existing != nil {
			// Preserve identity fields; remove stale secondary indexes.
			file.ID = existing.ID
			file.BookID = existing.BookID
			file.CreatedAt = existing.CreatedAt
			file.UpdatedAt = now

			if err := deleteBookFileSecondaryIndexes(batch, existing); err != nil {
				batch.Close()
				return err
			}
		} else {
			if file.ID == "" {
				id, err := newULID()
				if err != nil {
					batch.Close()
					return err
				}
				file.ID = id
			}
			if file.CreatedAt.IsZero() {
				file.CreatedAt = now
			}
			file.UpdatedAt = now
		}

		data, err := json.Marshal(file)
		if err != nil {
			batch.Close()
			return err
		}

		key := []byte(fmt.Sprintf("book_file:%s:%s", file.BookID, file.ID))
		if err := batch.Set(key, data, nil); err != nil {
			batch.Close()
			return err
		}

		if err := writeBookFileSecondaryIndexes(batch, file); err != nil {
			batch.Close()
			return err
		}
	}

	return batch.Commit(pebble.Sync)
}

// GetBookFileByID returns a single BookFile by bookID and fileID.
func (s *PebbleStore) GetBookFileByID(bookID, fileID string) (*BookFile, error) {
	return s.getBookFileByID(bookID, fileID)
}

// MoveBookFilesToBook reassigns BookFile records from sourceBookID to targetBookID.
func (s *PebbleStore) MoveBookFilesToBook(fileIDs []string, sourceBookID, targetBookID string) error {
	batch := s.db.NewBatch()

	for _, fid := range fileIDs {
		f, err := s.getBookFileByID(sourceBookID, fid)
		if err != nil {
			batch.Close()
			return fmt.Errorf("file not found: %s: %w", fid, err)
		}
		if f == nil {
			batch.Close()
			return fmt.Errorf("file not found: %s", fid)
		}

		// Delete old primary key
		oldKey := []byte(fmt.Sprintf("book_file:%s:%s", sourceBookID, fid))
		if err := batch.Delete(oldKey, nil); err != nil {
			batch.Close()
			return err
		}

		// Delete old secondary indexes
		if err := deleteBookFileSecondaryIndexes(batch, f); err != nil {
			batch.Close()
			return err
		}

		// Update book ID and write under new primary key
		f.BookID = targetBookID
		f.UpdatedAt = time.Now()

		data, err := json.Marshal(f)
		if err != nil {
			batch.Close()
			return err
		}
		newKey := []byte(fmt.Sprintf("book_file:%s:%s", targetBookID, fid))
		if err := batch.Set(newKey, data, nil); err != nil {
			batch.Close()
			return err
		}

		// Re-create secondary indexes with updated bookID
		if err := writeBookFileSecondaryIndexes(batch, f); err != nil {
			batch.Close()
			return err
		}
	}

	return batch.Commit(pebble.Sync)
}
