// file: internal/database/sqlite_store.go
// version: 1.11.0
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type rowScanner interface {
	Scan(dest ...interface{}) error
}

const bookSelectColumns = `
	id, title, author_id, series_id, series_sequence,
	file_path, original_filename, format, duration,
	work_id, narrator, edition, language, publisher,
	print_year, audiobook_release_year, isbn10, isbn13,
	file_hash, file_size, bitrate_kbps, codec, sample_rate_hz, channels,
	bit_depth, quality, is_primary_version, version_group_id, version_notes,
	original_file_hash, organized_file_hash, library_state, quantity,
	marked_for_deletion, marked_for_deletion_at
`

func scanBook(scanner rowScanner, book *Book) error {
	var (
		authorID, seriesID, seriesSequence, duration, printYear, releaseYear sql.NullInt64
		fileSize, bitrate, sampleRate, channels, bitDepth, quantity          sql.NullInt64
		title, filePath, format                                              string
		originalFilename                                                     sql.NullString
		workID, narrator, edition, language, publisher                       sql.NullString
		isbn10, isbn13, fileHash, quality, codec                             sql.NullString
		originalFileHash, organizedFileHash                                  sql.NullString
		versionGroupID, versionNotes                                         sql.NullString
		isPrimaryVersion                                                     sql.NullBool
		libraryState                                                         sql.NullString
		markedForDeletion                                                    sql.NullBool
		markedForDeletionAt                                                  sql.NullTime
	)

	if err := scanner.Scan(
		&book.ID, &title, &authorID, &seriesID, &seriesSequence,
		&filePath, &originalFilename, &format, &duration,
		&workID, &narrator, &edition, &language, &publisher,
		&printYear, &releaseYear, &isbn10, &isbn13,
		&fileHash, &fileSize, &bitrate, &codec, &sampleRate, &channels,
		&bitDepth, &quality, &isPrimaryVersion, &versionGroupID, &versionNotes,
		&originalFileHash, &organizedFileHash, &libraryState, &quantity,
		&markedForDeletion, &markedForDeletionAt,
	); err != nil {
		return err
	}

	book.Title = title
	book.FilePath = filePath
	book.OriginalFilename = nullableString(originalFilename)
	book.Format = format
	book.AuthorID = nullableInt(authorID)
	book.SeriesID = nullableInt(seriesID)
	book.SeriesSequence = nullableInt(seriesSequence)
	book.Duration = nullableInt(duration)
	book.WorkID = nullableString(workID)
	book.Narrator = nullableString(narrator)
	book.Edition = nullableString(edition)
	book.Language = nullableString(language)
	book.Publisher = nullableString(publisher)
	book.PrintYear = nullableInt(printYear)
	book.AudiobookReleaseYear = nullableInt(releaseYear)
	book.ISBN10 = nullableString(isbn10)
	book.ISBN13 = nullableString(isbn13)
	book.FileHash = nullableString(fileHash)
	if fileSize.Valid {
		size := fileSize.Int64
		book.FileSize = &size
	}
	book.Bitrate = nullableInt(bitrate)
	book.Codec = nullableString(codec)
	book.SampleRate = nullableInt(sampleRate)
	book.Channels = nullableInt(channels)
	book.BitDepth = nullableInt(bitDepth)
	book.Quality = nullableString(quality)
	if isPrimaryVersion.Valid {
		val := isPrimaryVersion.Bool
		book.IsPrimaryVersion = &val
	}
	book.VersionGroupID = nullableString(versionGroupID)
	book.VersionNotes = nullableString(versionNotes)
	book.OriginalFileHash = nullableString(originalFileHash)
	book.OrganizedFileHash = nullableString(organizedFileHash)
	book.LibraryState = nullableString(libraryState)
	book.Quantity = nullableInt(quantity)
	if markedForDeletion.Valid {
		val := markedForDeletion.Bool
		book.MarkedForDeletion = &val
	}
	if markedForDeletionAt.Valid {
		book.MarkedForDeletionAt = &markedForDeletionAt.Time
	}
	return nil
}

func nullableString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	val := ns.String
	return &val
}

func nullableInt(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	val := int(ni.Int64)
	return &val
}

// SQLiteStore implements the Store interface using SQLite3
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	store := &SQLiteStore{db: db}

	// Create tables
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return store, nil
}

// createTables creates all required tables
func (s *SQLiteStore) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS authors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	);

	CREATE INDEX IF NOT EXISTS idx_authors_name ON authors(name);

	CREATE TABLE IF NOT EXISTS series (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		author_id INTEGER,
		FOREIGN KEY (author_id) REFERENCES authors(id),
		UNIQUE(name, author_id)
	);

	CREATE INDEX IF NOT EXISTS idx_series_name ON series(name);
	CREATE INDEX IF NOT EXISTS idx_series_author ON series(author_id);

	CREATE TABLE IF NOT EXISTS works (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		author_id INTEGER,
		series_id INTEGER,
		alt_titles TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_works_title ON works(title);

	CREATE TABLE IF NOT EXISTS books (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		author_id INTEGER,
		series_id INTEGER,
		series_sequence INTEGER,
		file_path TEXT NOT NULL UNIQUE,
		original_filename TEXT,
		format TEXT,
		duration INTEGER,
		work_id TEXT,
		narrator TEXT,
		edition TEXT,
		language TEXT,
		publisher TEXT,
		print_year INTEGER,
		audiobook_release_year INTEGER,
		isbn10 TEXT,
		isbn13 TEXT,
		file_hash TEXT,
		file_size INTEGER,
		bitrate_kbps INTEGER,
		codec TEXT,
		sample_rate_hz INTEGER,
		channels INTEGER,
		bit_depth INTEGER,
		quality TEXT,
		is_primary_version BOOLEAN DEFAULT 1,
		version_group_id TEXT,
		version_notes TEXT,
		original_file_hash TEXT,
		organized_file_hash TEXT,
		library_state TEXT DEFAULT 'imported',
		quantity INTEGER DEFAULT 1,
		marked_for_deletion BOOLEAN DEFAULT 0,
		marked_for_deletion_at DATETIME,
		FOREIGN KEY (author_id) REFERENCES authors(id),
		FOREIGN KEY (series_id) REFERENCES series(id)
	);

	CREATE INDEX IF NOT EXISTS idx_books_title ON books(title);
	CREATE INDEX IF NOT EXISTS idx_books_author ON books(author_id);
	CREATE INDEX IF NOT EXISTS idx_books_series ON books(series_id);
	CREATE INDEX IF NOT EXISTS idx_books_file_path ON books(file_path);
	CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash);
	CREATE INDEX IF NOT EXISTS idx_books_original_hash ON books(original_file_hash);
	CREATE INDEX IF NOT EXISTS idx_books_organized_hash ON books(organized_file_hash);
	CREATE INDEX IF NOT EXISTS idx_books_library_state ON books(library_state);
	CREATE INDEX IF NOT EXISTS idx_books_marked_for_deletion ON books(marked_for_deletion);

	CREATE TABLE IF NOT EXISTS playlists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		series_id INTEGER,
		file_path TEXT NOT NULL,
		FOREIGN KEY (series_id) REFERENCES series(id)
	);

	CREATE TABLE IF NOT EXISTS playlist_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		playlist_id INTEGER NOT NULL,
		book_id INTEGER NOT NULL,
		position INTEGER NOT NULL,
		FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE,
		FOREIGN KEY (book_id) REFERENCES books(id)
	);

	CREATE INDEX IF NOT EXISTS idx_playlist_items_playlist ON playlist_items(playlist_id);

	CREATE TABLE IF NOT EXISTS import_paths (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_scan DATETIME,
		book_count INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_import_paths_path ON import_paths(path);

	CREATE TABLE IF NOT EXISTS operations (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		progress INTEGER NOT NULL DEFAULT 0,
		total INTEGER NOT NULL DEFAULT 0,
		message TEXT NOT NULL DEFAULT '',
		folder_path TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME,
		completed_at DATETIME,
		error_message TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_operations_status ON operations(status);
	CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations(created_at);

	CREATE TABLE IF NOT EXISTS operation_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		operation_id TEXT NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		details TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (operation_id) REFERENCES operations(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_operation_logs_operation ON operation_logs(operation_id);

	CREATE TABLE IF NOT EXISTS user_preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key TEXT NOT NULL UNIQUE,
		value TEXT,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_user_preferences_key ON user_preferences(key);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'string',
		is_secret BOOLEAN NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key);

	CREATE TABLE IF NOT EXISTS metadata_states (
		book_id TEXT NOT NULL,
		field TEXT NOT NULL,
		fetched_value TEXT,
		override_value TEXT,
		override_locked BOOLEAN NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (book_id, field)
	);

	CREATE INDEX IF NOT EXISTS idx_metadata_states_book ON metadata_states(book_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Non-destructive migration for existing databases: add missing columns
	return s.ensureExtendedBookColumns()
}

// ensureExtendedBookColumns adds newly introduced optional metadata columns to the books table
// for existing databases created before these columns were part of the CREATE TABLE statement.
// SQLite lacks IF NOT EXISTS for ADD COLUMN, so we inspect PRAGMA table_info and conditionally ALTER.
func (s *SQLiteStore) ensureExtendedBookColumns() error {
	// Map of desired columns (name -> type)
	columns := map[string]string{
		"work_id":                "TEXT",
		"narrator":               "TEXT",
		"edition":                "TEXT",
		"language":               "TEXT",
		"publisher":              "TEXT",
		"isbn10":                 "TEXT",
		"isbn13":                 "TEXT",
		"bitrate_kbps":           "INTEGER",
		"codec":                  "TEXT",
		"sample_rate_hz":         "INTEGER",
		"channels":               "INTEGER",
		"bit_depth":              "INTEGER",
		"quality":                "TEXT",
		"is_primary_version":     "BOOLEAN DEFAULT 1",
		"version_group_id":       "TEXT",
		"version_notes":          "TEXT",
		"original_file_hash":     "TEXT",
		"organized_file_hash":    "TEXT",
		"library_state":          "TEXT DEFAULT 'imported'",
		"quantity":               "INTEGER DEFAULT 1",
		"marked_for_deletion":    "BOOLEAN DEFAULT 0",
		"marked_for_deletion_at": "DATETIME",
	}

	// Fetch existing columns
	rows, err := s.db.Query("PRAGMA table_info(books)")
	if err != nil {
		return fmt.Errorf("failed to inspect books schema: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		// PRAGMA table_info returns: cid,name,type,notnull,dflt_value,pk
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table_info row: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating table_info: %w", err)
	}

	// Add any missing columns
	for name, colType := range columns {
		if _, ok := existing[name]; ok {
			continue
		}
		alter := fmt.Sprintf("ALTER TABLE books ADD COLUMN %s %s", name, colType)
		if _, err := s.db.Exec(alter); err != nil {
			return fmt.Errorf("failed adding column %s: %w", name, err)
		}
	}

	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_original_hash ON books(original_file_hash)",
		"CREATE INDEX IF NOT EXISTS idx_books_organized_hash ON books(organized_file_hash)",
		"CREATE INDEX IF NOT EXISTS idx_books_library_state ON books(library_state)",
		"CREATE INDEX IF NOT EXISTS idx_books_marked_for_deletion ON books(marked_for_deletion)",
	}
	for _, stmt := range indexStatements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed creating index with statement %s: %w", stmt, err)
		}
	}
	return nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ---- Extended interface no-op / minimal implementations for SQLite ----
// These satisfy the expanded Store interface but SQLite mode does not yet
// implement advanced features. They return informative errors or empty values.

func (s *SQLiteStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error) {
	return nil, fmt.Errorf("advanced user management not supported in SQLite mode")
}
func (s *SQLiteStore) GetUserByID(id string) (*User, error)             { return nil, nil }
func (s *SQLiteStore) GetUserByUsername(username string) (*User, error) { return nil, nil }
func (s *SQLiteStore) GetUserByEmail(email string) (*User, error)       { return nil, nil }
func (s *SQLiteStore) UpdateUser(user *User) error                      { return fmt.Errorf("not supported") }
func (s *SQLiteStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	return nil, fmt.Errorf("not supported")
}
func (s *SQLiteStore) GetSession(id string) (*Session, error)            { return nil, nil }
func (s *SQLiteStore) RevokeSession(id string) error                     { return fmt.Errorf("not supported") }
func (s *SQLiteStore) ListUserSessions(userID string) ([]Session, error) { return []Session{}, nil }
func (s *SQLiteStore) SetUserPreferenceForUser(userID, key, value string) error {
	return fmt.Errorf("not supported")
}
func (s *SQLiteStore) GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error) {
	return nil, nil
}
func (s *SQLiteStore) GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error) {
	return []UserPreferenceKV{}, nil
}
func (s *SQLiteStore) CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error) {
	return nil, fmt.Errorf("not supported")
}
func (s *SQLiteStore) ListBookSegments(bookNumericID int) ([]BookSegment, error) {
	return []BookSegment{}, nil
}
func (s *SQLiteStore) MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error {
	return fmt.Errorf("not supported")
}
func (s *SQLiteStore) AddPlaybackEvent(event *PlaybackEvent) error {
	return fmt.Errorf("not supported")
}
func (s *SQLiteStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error) {
	return []PlaybackEvent{}, nil
}
func (s *SQLiteStore) UpdatePlaybackProgress(progress *PlaybackProgress) error {
	return fmt.Errorf("not supported")
}
func (s *SQLiteStore) GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error) {
	return nil, nil
}
func (s *SQLiteStore) IncrementBookPlayStats(bookNumericID int, seconds int) error {
	return fmt.Errorf("not supported")
}
func (s *SQLiteStore) GetBookStats(bookNumericID int) (*BookStats, error) { return nil, nil }
func (s *SQLiteStore) IncrementUserListenStats(userID string, seconds int) error {
	return fmt.Errorf("not supported")
}
func (s *SQLiteStore) GetUserStats(userID string) (*UserStats, error) { return nil, nil }

// Author operations

func (s *SQLiteStore) GetAllAuthors() ([]Author, error) {
	rows, err := s.db.Query("SELECT id, name FROM authors ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []Author
	for rows.Next() {
		var author Author
		if err := rows.Scan(&author.ID, &author.Name); err != nil {
			return nil, err
		}
		authors = append(authors, author)
	}
	return authors, rows.Err()
}

func (s *SQLiteStore) GetAuthorByID(id int) (*Author, error) {
	var author Author
	err := s.db.QueryRow("SELECT id, name FROM authors WHERE id = ?", id).Scan(&author.ID, &author.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &author, nil
}

func (s *SQLiteStore) GetAuthorByName(name string) (*Author, error) {
	var author Author
	err := s.db.QueryRow("SELECT id, name FROM authors WHERE name = ?", name).Scan(&author.ID, &author.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &author, nil
}

func (s *SQLiteStore) CreateAuthor(name string) (*Author, error) {
	result, err := s.db.Exec("INSERT INTO authors (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Author{ID: int(id), Name: name}, nil
}

// Series operations

func (s *SQLiteStore) GetAllSeries() ([]Series, error) {
	rows, err := s.db.Query("SELECT id, name, author_id FROM series ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []Series
	for rows.Next() {
		var s Series
		if err := rows.Scan(&s.ID, &s.Name, &s.AuthorID); err != nil {
			return nil, err
		}
		series = append(series, s)
	}
	return series, rows.Err()
}

func (s *SQLiteStore) GetSeriesByID(id int) (*Series, error) {
	var series Series
	err := s.db.QueryRow("SELECT id, name, author_id FROM series WHERE id = ?", id).
		Scan(&series.ID, &series.Name, &series.AuthorID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &series, nil
}

func (s *SQLiteStore) GetSeriesByName(name string, authorID *int) (*Series, error) {
	var series Series
	var err error
	if authorID != nil {
		err = s.db.QueryRow("SELECT id, name, author_id FROM series WHERE name = ? AND author_id = ?", name, *authorID).
			Scan(&series.ID, &series.Name, &series.AuthorID)
	} else {
		err = s.db.QueryRow("SELECT id, name, author_id FROM series WHERE name = ? AND author_id IS NULL", name).
			Scan(&series.ID, &series.Name, &series.AuthorID)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &series, nil
}

func (s *SQLiteStore) CreateSeries(name string, authorID *int) (*Series, error) {
	result, err := s.db.Exec("INSERT INTO series (name, author_id) VALUES (?, ?)", name, authorID)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Series{ID: int(id), Name: name, AuthorID: authorID}, nil
}

// Work operations

func (s *SQLiteStore) GetAllWorks() ([]Work, error) {
	rows, err := s.db.Query("SELECT id, title, author_id, series_id, alt_titles FROM works ORDER BY title")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var works []Work
	for rows.Next() {
		var w Work
		var altTitlesStr *string
		if err := rows.Scan(&w.ID, &w.Title, &w.AuthorID, &w.SeriesID, &altTitlesStr); err != nil {
			return nil, err
		}
		if altTitlesStr != nil && *altTitlesStr != "" {
			w.AltTitles = strings.Split(*altTitlesStr, "|")
		}
		works = append(works, w)
	}
	return works, rows.Err()
}

func (s *SQLiteStore) GetWorkByID(id string) (*Work, error) {
	var w Work
	var altTitlesStr *string
	err := s.db.QueryRow("SELECT id, title, author_id, series_id, alt_titles FROM works WHERE id = ?", id).
		Scan(&w.ID, &w.Title, &w.AuthorID, &w.SeriesID, &altTitlesStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if altTitlesStr != nil && *altTitlesStr != "" {
		w.AltTitles = strings.Split(*altTitlesStr, "|")
	}
	return &w, nil
}

func (s *SQLiteStore) CreateWork(work *Work) (*Work, error) {
	if work.ID == "" {
		id, err := newULID()
		if err != nil {
			return nil, err
		}
		work.ID = id
	}
	var altTitlesStr *string
	if len(work.AltTitles) > 0 {
		joined := strings.Join(work.AltTitles, "|")
		altTitlesStr = &joined
	}
	_, err := s.db.Exec("INSERT INTO works (id, title, author_id, series_id, alt_titles, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		work.ID, work.Title, work.AuthorID, work.SeriesID, altTitlesStr, time.Now())
	if err != nil {
		return nil, err
	}
	return work, nil
}

func (s *SQLiteStore) UpdateWork(id string, work *Work) (*Work, error) {
	var altTitlesStr *string
	if len(work.AltTitles) > 0 {
		joined := strings.Join(work.AltTitles, "|")
		altTitlesStr = &joined
	}
	result, err := s.db.Exec("UPDATE works SET title = ?, author_id = ?, series_id = ?, alt_titles = ?, updated_at = ? WHERE id = ?",
		work.Title, work.AuthorID, work.SeriesID, altTitlesStr, time.Now(), id)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("work not found")
	}
	work.ID = id
	return work, nil
}

func (s *SQLiteStore) DeleteWork(id string) error {
	result, err := s.db.Exec("DELETE FROM works WHERE id = ?", id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("work not found")
	}
	return nil
}

func (s *SQLiteStore) GetBooksByWorkID(workID string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE work_id = ? ORDER BY title`, bookSelectColumns)
	rows, err := s.db.Query(query, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// Book operations

func (s *SQLiteStore) GetAllBooks(limit, offset int) ([]Book, error) {
	if limit <= 0 {
		limit = 1_000_000
	}
	if offset < 0 {
		offset = 0
	}
	query := fmt.Sprintf(`SELECT %s FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 ORDER BY title LIMIT ? OFFSET ?`, bookSelectColumns)
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) GetBookByID(id string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE id = ?`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, id), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByFilePath(path string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE file_path = ?`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, path), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByFileHash(hash string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE file_hash = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByOriginalHash(hash string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE original_file_hash = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByOrganizedHash(hash string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE organized_file_hash = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

// GetDuplicateBooks returns groups of books with identical file hashes
// Only returns groups with 2+ books (actual duplicates)
func (s *SQLiteStore) GetDuplicateBooks() ([][]Book, error) {
	// Find all hashes that have duplicates (appear more than once)
	// Use COALESCE to handle null hashes and prefer organized_file_hash
	hashQuery := `
		SELECT COALESCE(organized_file_hash, file_hash) as hash, COUNT(*) as count
		FROM books
		WHERE COALESCE(organized_file_hash, file_hash) IS NOT NULL
		  AND COALESCE(marked_for_deletion, 0) = 0
		GROUP BY COALESCE(organized_file_hash, file_hash)
		HAVING count > 1
		ORDER BY count DESC
	`

	hashRows, err := s.db.Query(hashQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query duplicate hashes: %w", err)
	}
	defer hashRows.Close()

	var duplicateGroups [][]Book
	for hashRows.Next() {
		var hash string
		var count int
		if err := hashRows.Scan(&hash, &count); err != nil {
			return nil, fmt.Errorf("failed to scan hash row: %w", err)
		}

		// Get all books with this hash
		booksQuery := fmt.Sprintf(`
				SELECT %s FROM books
				WHERE COALESCE(organized_file_hash, file_hash) = ?
				  AND COALESCE(marked_for_deletion, 0) = 0
				ORDER BY file_path
			`, bookSelectColumns)

		bookRows, err := s.db.Query(booksQuery, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to query books for hash %s: %w", hash, err)
		}

		var group []Book
		for bookRows.Next() {
			var book Book
			if err := scanBook(bookRows, &book); err != nil {
				bookRows.Close()
				return nil, fmt.Errorf("failed to scan book: %w", err)
			}
			group = append(group, book)
		}
		bookRows.Close()

		if err := bookRows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating book rows: %w", err)
		}
		// Only add groups with 2+ books
		if len(group) >= 2 {
			duplicateGroups = append(duplicateGroups, group)
		}
	}

	if err := hashRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hash rows: %w", err)
	}
	return duplicateGroups, nil
}

func (s *SQLiteStore) GetBooksBySeriesID(seriesID int) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE series_id = ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY series_sequence, title`, bookSelectColumns)
	rows, err := s.db.Query(query, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) GetBooksByAuthorID(authorID int) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE author_id = ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY title`, bookSelectColumns)
	rows, err := s.db.Query(query, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) CreateBook(book *Book) (*Book, error) {
	// Generate ULID if not provided
	if book.ID == "" {
		id, err := newULID()
		if err != nil {
			return nil, err
		}
		book.ID = id
	}

	query := `INSERT INTO books (
		id, title, author_id, series_id, series_sequence, file_path, original_filename,
		format, duration, work_id, narrator, edition, language, publisher,
		print_year, audiobook_release_year, isbn10, isbn13,
		file_hash, file_size, bitrate_kbps, codec, sample_rate_hz, channels,
		bit_depth, quality, is_primary_version, version_group_id, version_notes,
		original_file_hash, organized_file_hash, library_state, quantity, marked_for_deletion, marked_for_deletion_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query,
		book.ID, book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence, book.FilePath, book.OriginalFilename,
		book.Format, book.Duration, book.WorkID, book.Narrator, book.Edition, book.Language, book.Publisher,
		book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13,
		book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
		book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
		book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity, book.MarkedForDeletion, book.MarkedForDeletionAt,
	)
	if err != nil {
		return nil, err
	}
	return book, nil
}

func (s *SQLiteStore) UpdateBook(id string, book *Book) (*Book, error) {
	query := `UPDATE books SET
		title = ?, author_id = ?, series_id = ?, series_sequence = ?,
		file_path = ?, original_filename = ?, format = ?, duration = ?,
		work_id = ?, narrator = ?, edition = ?, language = ?, publisher = ?,
		print_year = ?, audiobook_release_year = ?, isbn10 = ?, isbn13 = ?,
		file_hash = ?, file_size = ?, bitrate_kbps = ?, codec = ?, sample_rate_hz = ?, channels = ?,
		bit_depth = ?, quality = ?, is_primary_version = ?, version_group_id = ?, version_notes = ?,
		original_file_hash = ?, organized_file_hash = ?, library_state = ?, quantity = ?,
		marked_for_deletion = ?, marked_for_deletion_at = ?
	WHERE id = ?`
	result, err := s.db.Exec(query,
		book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence,
		book.FilePath, book.OriginalFilename, book.Format, book.Duration,
		book.WorkID, book.Narrator, book.Edition, book.Language, book.Publisher,
		book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13,
		book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
		book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
		book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity,
		book.MarkedForDeletion, book.MarkedForDeletionAt, id,
	)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("book not found")
	}
	book.ID = id
	return book, nil
}

func (s *SQLiteStore) DeleteBook(id string) error {
	result, err := s.db.Exec("DELETE FROM books WHERE id = ?", id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("book not found")
	}
	if _, err := s.db.Exec("DELETE FROM metadata_states WHERE book_id = ?", id); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) SearchBooks(query string, limit, offset int) ([]Book, error) {
	searchQuery := fmt.Sprintf(`SELECT %s FROM books WHERE title LIKE ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY title LIMIT ? OFFSET ?`, bookSelectColumns)
	rows, err := s.db.Query(searchQuery, "%"+query+"%", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) CountBooks() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0").Scan(&count)
	return count, err
}

func (s *SQLiteStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error) {
	if limit <= 0 {
		limit = 1_000_000
	}
	if offset < 0 {
		offset = 0
	}
	query := fmt.Sprintf(`SELECT %s FROM books WHERE COALESCE(marked_for_deletion, 0) = 1`, bookSelectColumns)
	args := []interface{}{}
	if olderThan != nil {
		query += " AND marked_for_deletion_at IS NOT NULL AND marked_for_deletion_at <= ?"
		args = append(args, olderThan.UTC())
	}
	query += " ORDER BY (marked_for_deletion_at IS NULL), marked_for_deletion_at DESC, title LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// GetBooksByVersionGroup returns all books in a version group
func (s *SQLiteStore) GetBooksByVersionGroup(groupID string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE version_group_id = ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY is_primary_version DESC, title`, bookSelectColumns)
	rows, err := s.db.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}

	return books, rows.Err()
}

// Import path operations

func (s *SQLiteStore) GetAllImportPaths() ([]ImportPath, error) {
	query := `SELECT id, path, name, enabled, created_at, last_scan, book_count
			  FROM import_paths ORDER BY name`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []ImportPath
	for rows.Next() {
		var folder ImportPath
		if err := rows.Scan(&folder.ID, &folder.Path, &folder.Name, &folder.Enabled,
			&folder.CreatedAt, &folder.LastScan, &folder.BookCount); err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (s *SQLiteStore) GetImportPathByID(id int) (*ImportPath, error) {
	var folder ImportPath
	query := `SELECT id, path, name, enabled, created_at, last_scan, book_count
			  FROM import_paths WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(&folder.ID, &folder.Path, &folder.Name,
		&folder.Enabled, &folder.CreatedAt, &folder.LastScan, &folder.BookCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *SQLiteStore) GetImportPathByPath(path string) (*ImportPath, error) {
	var folder ImportPath
	query := `SELECT id, path, name, enabled, created_at, last_scan, book_count
			  FROM import_paths WHERE path = ?`
	err := s.db.QueryRow(query, path).Scan(&folder.ID, &folder.Path, &folder.Name,
		&folder.Enabled, &folder.CreatedAt, &folder.LastScan, &folder.BookCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *SQLiteStore) CreateImportPath(path, name string) (*ImportPath, error) {
	result, err := s.db.Exec("INSERT INTO import_paths (path, name) VALUES (?, ?)", path, name)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &ImportPath{
		ID:        int(id),
		Path:      path,
		Name:      name,
		Enabled:   true,
		CreatedAt: time.Now(),
		BookCount: 0,
	}, nil
}

func (s *SQLiteStore) UpdateImportPath(id int, folder *ImportPath) error {
	_, err := s.db.Exec(`UPDATE import_paths SET path = ?, name = ?, enabled = ?,
		last_scan = ?, book_count = ? WHERE id = ?`,
		folder.Path, folder.Name, folder.Enabled, folder.LastScan, folder.BookCount, id)
	return err
}

func (s *SQLiteStore) DeleteImportPath(id int) error {
	_, err := s.db.Exec("DELETE FROM import_paths WHERE id = ?", id)
	return err
}

// Operation operations

func (s *SQLiteStore) CreateOperation(id, opType string, folderPath *string) (*Operation, error) {
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO operations (id, type, status, folder_path, created_at)
		VALUES (?, ?, ?, ?, ?)`, id, opType, "pending", folderPath, now)
	if err != nil {
		return nil, err
	}
	return &Operation{
		ID:         id,
		Type:       opType,
		Status:     "pending",
		Progress:   0,
		Total:      0,
		Message:    "",
		FolderPath: folderPath,
		CreatedAt:  now,
	}, nil
}

func (s *SQLiteStore) GetOperationByID(id string) (*Operation, error) {
	var op Operation
	query := `SELECT id, type, status, progress, total, message, folder_path,
			  created_at, started_at, completed_at, error_message
			  FROM operations WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(&op.ID, &op.Type, &op.Status, &op.Progress,
		&op.Total, &op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
		&op.CompletedAt, &op.ErrorMessage)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *SQLiteStore) GetRecentOperations(limit int) ([]Operation, error) {
	query := `SELECT id, type, status, progress, total, message, folder_path,
			  created_at, started_at, completed_at, error_message
			  FROM operations ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operations []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total,
			&op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
			&op.CompletedAt, &op.ErrorMessage); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

func (s *SQLiteStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	now := time.Now()
	var startedAt *time.Time
	var completedAt *time.Time

	if status == "running" {
		startedAt = &now
	} else if status == "completed" || status == "failed" {
		completedAt = &now
	}

	_, err := s.db.Exec(`UPDATE operations SET status = ?, progress = ?, total = ?,
		message = ?, started_at = COALESCE(started_at, ?), completed_at = ? WHERE id = ?`,
		status, progress, total, message, startedAt, completedAt, id)
	return err
}

func (s *SQLiteStore) UpdateOperationError(id, errorMessage string) error {
	_, err := s.db.Exec("UPDATE operations SET error_message = ?, status = 'failed' WHERE id = ?",
		errorMessage, id)
	return err
}

// Operation Log operations

func (s *SQLiteStore) AddOperationLog(operationID, level, message string, details *string) error {
	_, err := s.db.Exec(`INSERT INTO operation_logs (operation_id, level, message, details)
		VALUES (?, ?, ?, ?)`, operationID, level, message, details)
	return err
}

func (s *SQLiteStore) GetOperationLogs(operationID string) ([]OperationLog, error) {
	query := `SELECT id, operation_id, level, message, details, created_at
			  FROM operation_logs WHERE operation_id = ? ORDER BY created_at`
	rows, err := s.db.Query(query, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []OperationLog
	for rows.Next() {
		var log OperationLog
		if err := rows.Scan(&log.ID, &log.OperationID, &log.Level, &log.Message,
			&log.Details, &log.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// User Preference operations

func (s *SQLiteStore) GetUserPreference(key string) (*UserPreference, error) {
	var pref UserPreference
	err := s.db.QueryRow("SELECT id, key, value, updated_at FROM user_preferences WHERE key = ?", key).
		Scan(&pref.ID, &pref.Key, &pref.Value, &pref.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

func (s *SQLiteStore) SetUserPreference(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO user_preferences (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?`,
		key, value, time.Now(), value, time.Now())
	return err
}

func (s *SQLiteStore) GetAllUserPreferences() ([]UserPreference, error) {
	rows, err := s.db.Query("SELECT id, key, value, updated_at FROM user_preferences ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preferences []UserPreference
	for rows.Next() {
		var pref UserPreference
		if err := rows.Scan(&pref.ID, &pref.Key, &pref.Value, &pref.UpdatedAt); err != nil {
			return nil, err
		}
		preferences = append(preferences, pref)
	}
	return preferences, rows.Err()
}

// Metadata field provenance operations

func (s *SQLiteStore) GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error) {
	rows, err := s.db.Query(`SELECT book_id, field, fetched_value, override_value, override_locked, updated_at
		FROM metadata_states WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []MetadataFieldState
	for rows.Next() {
		var state MetadataFieldState
		if err := rows.Scan(&state.BookID, &state.Field, &state.FetchedValue, &state.OverrideValue, &state.OverrideLocked, &state.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *SQLiteStore) UpsertMetadataFieldState(state *MetadataFieldState) error {
	if state == nil {
		return fmt.Errorf("metadata state cannot be nil")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now()
	}
	_, err := s.db.Exec(`INSERT INTO metadata_states (book_id, field, fetched_value, override_value, override_locked, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id, field) DO UPDATE SET
			fetched_value = excluded.fetched_value,
			override_value = excluded.override_value,
			override_locked = excluded.override_locked,
			updated_at = excluded.updated_at`,
		state.BookID, state.Field, state.FetchedValue, state.OverrideValue, state.OverrideLocked, state.UpdatedAt)
	return err
}

func (s *SQLiteStore) DeleteMetadataFieldState(bookID, field string) error {
	_, err := s.db.Exec("DELETE FROM metadata_states WHERE book_id = ? AND field = ?", bookID, field)
	return err
}

// Playlist operations

func (s *SQLiteStore) CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error) {
	result, err := s.db.Exec("INSERT INTO playlists (name, series_id, file_path) VALUES (?, ?, ?)",
		name, seriesID, filePath)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Playlist{
		ID:       int(id),
		Name:     name,
		SeriesID: seriesID,
		FilePath: filePath,
	}, nil
}

func (s *SQLiteStore) GetPlaylistByID(id int) (*Playlist, error) {
	var playlist Playlist
	err := s.db.QueryRow("SELECT id, name, series_id, file_path FROM playlists WHERE id = ?", id).
		Scan(&playlist.ID, &playlist.Name, &playlist.SeriesID, &playlist.FilePath)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &playlist, nil
}

func (s *SQLiteStore) GetPlaylistBySeriesID(seriesID int) (*Playlist, error) {
	var playlist Playlist
	err := s.db.QueryRow("SELECT id, name, series_id, file_path FROM playlists WHERE series_id = ?", seriesID).
		Scan(&playlist.ID, &playlist.Name, &playlist.SeriesID, &playlist.FilePath)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &playlist, nil
}

func (s *SQLiteStore) AddPlaylistItem(playlistID, bookID, position int) error {
	_, err := s.db.Exec("INSERT INTO playlist_items (playlist_id, book_id, position) VALUES (?, ?, ?)",
		playlistID, bookID, position)
	return err
}

func (s *SQLiteStore) GetPlaylistItems(playlistID int) ([]PlaylistItem, error) {
	rows, err := s.db.Query(`SELECT id, playlist_id, book_id, position
		FROM playlist_items WHERE playlist_id = ? ORDER BY position`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PlaylistItem
	for rows.Next() {
		var item PlaylistItem
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.BookID, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Hash Blocklist Methods

// IsHashBlocked checks if a hash is in the blocklist
func (s *SQLiteStore) IsHashBlocked(hash string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM do_not_import WHERE hash = ?", hash).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddBlockedHash adds a hash to the blocklist
func (s *SQLiteStore) AddBlockedHash(hash, reason string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO do_not_import (hash, reason, created_at) VALUES (?, ?, ?)",
		hash, reason, time.Now(),
	)
	return err
}

// RemoveBlockedHash removes a hash from the blocklist
func (s *SQLiteStore) RemoveBlockedHash(hash string) error {
	_, err := s.db.Exec("DELETE FROM do_not_import WHERE hash = ?", hash)
	return err
}

// GetAllBlockedHashes returns all blocked hashes
func (s *SQLiteStore) GetAllBlockedHashes() ([]DoNotImport, error) {
	rows, err := s.db.Query("SELECT hash, reason, created_at FROM do_not_import ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocked []DoNotImport
	for rows.Next() {
		var item DoNotImport
		if err := rows.Scan(&item.Hash, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		blocked = append(blocked, item)
	}
	return blocked, rows.Err()
}

// GetBlockedHashByHash retrieves a specific blocked hash entry
func (s *SQLiteStore) GetBlockedHashByHash(hash string) (*DoNotImport, error) {
	var item DoNotImport
	err := s.db.QueryRow(
		"SELECT hash, reason, created_at FROM do_not_import WHERE hash = ?",
		hash,
	).Scan(&item.Hash, &item.Reason, &item.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
