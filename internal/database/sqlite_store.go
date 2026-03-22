// file: internal/database/sqlite_store.go
// version: 1.45.0
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"path/filepath"
	"strings"
	"time"

	matcher "github.com/jdfalk/audiobook-organizer/internal/matcher"
	_ "github.com/mattn/go-sqlite3"
	ulid "github.com/oklog/ulid/v2"
)

type rowScanner interface {
	Scan(dest ...interface{}) error
}

const bookSelectColumns = `
	id, title, author_id, series_id, series_sequence,
	file_path, original_filename, format, duration,
	work_id, narrator, edition, description, language, publisher, genre,
	print_year, audiobook_release_year, isbn10, isbn13, asin,
	open_library_id, hardcover_id, google_books_id,
	itunes_persistent_id, itunes_date_added, itunes_play_count,
	itunes_last_played, itunes_rating, itunes_bookmark, itunes_import_source,
	file_hash, file_size, bitrate_kbps, codec, sample_rate_hz, channels,
	bit_depth, quality, is_primary_version, version_group_id, version_notes,
	original_file_hash, organized_file_hash, library_state, quantity,
	marked_for_deletion, marked_for_deletion_at, created_at, updated_at,
	metadata_updated_at, last_written_at, metadata_review_status, cover_url, narrators_json
`

// bookSelectColumnsQualified prefixes all columns with "books." for use in JOINs.
const bookSelectColumnsQualified = `
	books.id, books.title, books.author_id, books.series_id, books.series_sequence,
	books.file_path, books.original_filename, books.format, books.duration,
	books.work_id, books.narrator, books.edition, books.description, books.language, books.publisher, books.genre,
	books.print_year, books.audiobook_release_year, books.isbn10, books.isbn13, books.asin,
	books.open_library_id, books.hardcover_id, books.google_books_id,
	books.itunes_persistent_id, books.itunes_date_added, books.itunes_play_count,
	books.itunes_last_played, books.itunes_rating, books.itunes_bookmark, books.itunes_import_source,
	books.file_hash, books.file_size, books.bitrate_kbps, books.codec, books.sample_rate_hz, books.channels,
	books.bit_depth, books.quality, books.is_primary_version, books.version_group_id, books.version_notes,
	books.original_file_hash, books.organized_file_hash, books.library_state, books.quantity,
	books.marked_for_deletion, books.marked_for_deletion_at, books.created_at, books.updated_at,
	books.metadata_updated_at, books.last_written_at, books.metadata_review_status, books.cover_url, books.narrators_json
`

func scanBook(scanner rowScanner, book *Book) error {
	var (
		authorID, seriesID, seriesSequence, duration, printYear, releaseYear sql.NullInt64
		itunesPlayCount, itunesRating, itunesBookmark                        sql.NullInt64
		fileSize, bitrate, sampleRate, channels, bitDepth, quantity          sql.NullInt64
		title, filePath, format                                              string
		originalFilename                                                     sql.NullString
		workID, narrator, edition, description, language, publisher, genre    sql.NullString
		itunesPersistentID, itunesImportSource                               sql.NullString
		itunesDateAdded, itunesLastPlayed                                    sql.NullTime
		isbn10, isbn13, asin                                                  sql.NullString
		openLibraryID, hardcoverID, googleBooksID                              sql.NullString
		fileHash, quality, codec                                               sql.NullString
		originalFileHash, organizedFileHash                                  sql.NullString
		versionGroupID, versionNotes                                         sql.NullString
		coverURL, narratorsJSON, metadataReviewStatus                         sql.NullString
		isPrimaryVersion                                                     sql.NullBool
		libraryState                                                         sql.NullString
		markedForDeletion                                                    sql.NullBool
		markedForDeletionAt, createdAt, updatedAt                            sql.NullTime
		metadataUpdatedAt, lastWrittenAt                                      sql.NullTime
	)

	if err := scanner.Scan(
		&book.ID, &title, &authorID, &seriesID, &seriesSequence,
		&filePath, &originalFilename, &format, &duration,
		&workID, &narrator, &edition, &description, &language, &publisher, &genre,
		&printYear, &releaseYear, &isbn10, &isbn13, &asin,
		&openLibraryID, &hardcoverID, &googleBooksID,
		&itunesPersistentID, &itunesDateAdded, &itunesPlayCount,
		&itunesLastPlayed, &itunesRating, &itunesBookmark, &itunesImportSource,
		&fileHash, &fileSize, &bitrate, &codec, &sampleRate, &channels,
		&bitDepth, &quality, &isPrimaryVersion, &versionGroupID, &versionNotes,
		&originalFileHash, &organizedFileHash, &libraryState, &quantity,
		&markedForDeletion, &markedForDeletionAt, &createdAt, &updatedAt,
		&metadataUpdatedAt, &lastWrittenAt, &metadataReviewStatus, &coverURL, &narratorsJSON,
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
	book.Description = nullableString(description)
	book.Language = nullableString(language)
	book.Publisher = nullableString(publisher)
	book.Genre = nullableString(genre)
	book.PrintYear = nullableInt(printYear)
	book.AudiobookReleaseYear = nullableInt(releaseYear)
	book.ISBN10 = nullableString(isbn10)
	book.ISBN13 = nullableString(isbn13)
	book.ASIN = nullableString(asin)
	book.OpenLibraryID = nullableString(openLibraryID)
	book.HardcoverID = nullableString(hardcoverID)
	book.GoogleBooksID = nullableString(googleBooksID)
	book.ITunesPersistentID = nullableString(itunesPersistentID)
	if itunesDateAdded.Valid {
		book.ITunesDateAdded = &itunesDateAdded.Time
	}
	book.ITunesPlayCount = nullableInt(itunesPlayCount)
	if itunesLastPlayed.Valid {
		book.ITunesLastPlayed = &itunesLastPlayed.Time
	}
	book.ITunesRating = nullableInt(itunesRating)
	if itunesBookmark.Valid {
		bookmark := itunesBookmark.Int64
		book.ITunesBookmark = &bookmark
	}
	book.ITunesImportSource = nullableString(itunesImportSource)
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
	if createdAt.Valid {
		book.CreatedAt = &createdAt.Time
	}
	if updatedAt.Valid {
		book.UpdatedAt = &updatedAt.Time
	}
	if metadataUpdatedAt.Valid {
		book.MetadataUpdatedAt = &metadataUpdatedAt.Time
	}
	if lastWrittenAt.Valid {
		book.LastWrittenAt = &lastWrittenAt.Time
	}
	book.MetadataReviewStatus = nullableString(metadataReviewStatus)
	book.CoverURL = nullableString(coverURL)
	book.NarratorsJSON = nullableString(narratorsJSON)
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

	// Improve concurrency and durability defaults for SQLite.
	pragmaStatements := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, stmt := range pragmaStatements {
		if _, err := db.Exec(stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to apply sqlite pragma %q: %w", stmt, err)
		}
	}

	if strings.Contains(path, ":memory:") {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(10)
	}
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	store := &SQLiteStore{db: db}

	// Create tables
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	// Deduplicate series records (NULL author_id bypasses UNIQUE constraint)
	if err := store.deduplicateSeries(); err != nil {
		// Non-fatal — log and continue
		fmt.Printf("Warning: series deduplication failed: %v\n", err)
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

	CREATE TABLE IF NOT EXISTS author_aliases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		author_id INTEGER NOT NULL REFERENCES authors(id) ON DELETE CASCADE,
		alias_name TEXT NOT NULL,
		alias_type TEXT NOT NULL DEFAULT 'alias',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_author_aliases_author ON author_aliases(author_id);
	CREATE INDEX IF NOT EXISTS idx_author_aliases_name ON author_aliases(alias_name);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_author_aliases_unique ON author_aliases(author_id, alias_name);

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
		description TEXT,
		language TEXT,
		publisher TEXT,
		genre TEXT,
		print_year INTEGER,
		audiobook_release_year INTEGER,
		isbn10 TEXT,
		isbn13 TEXT,
		asin TEXT,
		open_library_id TEXT,
		hardcover_id TEXT,
		google_books_id TEXT,
		itunes_persistent_id TEXT,
		itunes_date_added TIMESTAMP,
		itunes_play_count INTEGER DEFAULT 0,
		itunes_last_played TIMESTAMP,
		itunes_rating INTEGER,
		itunes_bookmark INTEGER,
		itunes_import_source TEXT,
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
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME,
		metadata_updated_at DATETIME,
		last_written_at DATETIME,
		metadata_review_status TEXT,
		cover_url TEXT,
		narrators_json TEXT,
		FOREIGN KEY (author_id) REFERENCES authors(id),
		FOREIGN KEY (series_id) REFERENCES series(id)
	);

	CREATE TABLE IF NOT EXISTS book_authors (
		book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		author_id INTEGER NOT NULL REFERENCES authors(id),
		role TEXT NOT NULL DEFAULT 'author',
		position INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (book_id, author_id)
	);

	CREATE INDEX IF NOT EXISTS idx_book_authors_book ON book_authors(book_id);
	CREATE INDEX IF NOT EXISTS idx_book_authors_author ON book_authors(author_id);

	CREATE TABLE IF NOT EXISTS narrators (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_narrators_name ON narrators(name);

	CREATE TABLE IF NOT EXISTS book_narrators (
		book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		narrator_id INTEGER NOT NULL REFERENCES narrators(id),
		role TEXT NOT NULL DEFAULT 'narrator',
		position INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (book_id, narrator_id)
	);

	CREATE INDEX IF NOT EXISTS idx_book_narrators_book ON book_narrators(book_id);
	CREATE INDEX IF NOT EXISTS idx_book_narrators_narrator ON book_narrators(narrator_id);

	CREATE INDEX IF NOT EXISTS idx_books_title ON books(title);
	CREATE INDEX IF NOT EXISTS idx_books_author ON books(author_id);
	CREATE INDEX IF NOT EXISTS idx_books_series ON books(series_id);
	CREATE INDEX IF NOT EXISTS idx_books_file_path ON books(file_path);
	CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash);
	CREATE INDEX IF NOT EXISTS idx_books_itunes_persistent_id ON books(itunes_persistent_id);
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
		error_message TEXT,
		result_data TEXT
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

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash_algo TEXT NOT NULL DEFAULT 'bcrypt',
		password_hash TEXT NOT NULL,
		roles TEXT NOT NULL DEFAULT '["user"]',
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		version INTEGER NOT NULL DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		revoked INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

	CREATE TABLE IF NOT EXISTS book_segments (
		id TEXT PRIMARY KEY,
		book_id INTEGER NOT NULL,
		file_path TEXT NOT NULL,
		format TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER NOT NULL DEFAULT 0,
		duration_seconds INTEGER NOT NULL DEFAULT 0,
		track_number INTEGER,
		total_tracks INTEGER,
		active INTEGER NOT NULL DEFAULT 1,
		superseded_by TEXT,
		file_hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		version INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_book_segments_book ON book_segments(book_id);
	CREATE INDEX IF NOT EXISTS idx_book_segments_hash ON book_segments(file_hash);

	CREATE TABLE IF NOT EXISTS playback_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL,
		book_id INTEGER NOT NULL,
		segment_id TEXT NOT NULL DEFAULT '',
		position_seconds INTEGER NOT NULL DEFAULT 0,
		event_type TEXT NOT NULL DEFAULT 'progress',
		play_speed REAL NOT NULL DEFAULT 1.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		version INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_playback_events_user_book ON playback_events(user_id, book_id);

	CREATE TABLE IF NOT EXISTS playback_progress (
		user_id TEXT NOT NULL,
		book_id INTEGER NOT NULL,
		segment_id TEXT NOT NULL DEFAULT '',
		position_seconds INTEGER NOT NULL DEFAULT 0,
		percent_complete REAL NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		version INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (user_id, book_id)
	);

	CREATE TABLE IF NOT EXISTS book_stats (
		book_id INTEGER PRIMARY KEY,
		play_count INTEGER NOT NULL DEFAULT 0,
		listen_seconds INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS user_stats (
		user_id TEXT PRIMARY KEY,
		listen_seconds INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1
	);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Non-destructive migration for existing databases: add missing columns
	return s.ensureExtendedBookColumns()
}

// deduplicateSeries merges duplicate series records that share the same (name, author_id).
// SQL UNIQUE constraints don't catch NULL=NULL, so duplicates accumulate for series with no author.
func (s *SQLiteStore) deduplicateSeries() error {
	// Find duplicate groups: same LOWER(name) and same author_id (including NULL)
	rows, err := s.db.Query(`
		SELECT LOWER(name) as lname, COALESCE(author_id, -1) as aid, MIN(id) as keep_id, COUNT(*) as cnt
		FROM series
		GROUP BY LOWER(name), COALESCE(author_id, -1)
		HAVING COUNT(*) > 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var totalMerged int
	for rows.Next() {
		var lname string
		var aid, keepID, cnt int
		if err := rows.Scan(&lname, &aid, &keepID, &cnt); err != nil {
			return err
		}

		// Update books and works pointing to duplicate series IDs → keep the lowest ID
		var updateQuery string
		var deleteQuery string
		if aid == -1 {
			updateQuery = "UPDATE books SET series_id = ? WHERE series_id IN (SELECT id FROM series WHERE LOWER(name) = LOWER(?) AND author_id IS NULL AND id != ?)"
			deleteQuery = "DELETE FROM series WHERE LOWER(name) = LOWER(?) AND author_id IS NULL AND id != ?"
		} else {
			updateQuery = "UPDATE books SET series_id = ? WHERE series_id IN (SELECT id FROM series WHERE LOWER(name) = LOWER(?) AND author_id = ? AND id != ?)"
			deleteQuery = "DELETE FROM series WHERE LOWER(name) = LOWER(?) AND author_id = ? AND id != ?"
		}

		if aid == -1 {
			if _, err := s.db.Exec(updateQuery, keepID, lname, keepID); err != nil {
				return fmt.Errorf("failed to reassign books for series %q: %w", lname, err)
			}
			// Also update works table
			s.db.Exec("UPDATE works SET series_id = ? WHERE series_id IN (SELECT id FROM series WHERE LOWER(name) = LOWER(?) AND author_id IS NULL AND id != ?)", keepID, lname, keepID)
			if _, err := s.db.Exec(deleteQuery, lname, keepID); err != nil {
				return fmt.Errorf("failed to delete duplicate series %q: %w", lname, err)
			}
		} else {
			if _, err := s.db.Exec(updateQuery, keepID, lname, aid, keepID); err != nil {
				return fmt.Errorf("failed to reassign books for series %q: %w", lname, err)
			}
			s.db.Exec("UPDATE works SET series_id = ? WHERE series_id IN (SELECT id FROM series WHERE LOWER(name) = LOWER(?) AND author_id = ? AND id != ?)", keepID, lname, aid, keepID)
			if _, err := s.db.Exec(deleteQuery, lname, aid, keepID); err != nil {
				return fmt.Errorf("failed to delete duplicate series %q: %w", lname, err)
			}
		}
		totalMerged += cnt - 1
	}

	if totalMerged > 0 {
		fmt.Printf("Deduplicated %d series records\n", totalMerged)
	}
	return rows.Err()
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
		"description":            "TEXT",
		"language":               "TEXT",
		"publisher":              "TEXT",
		"isbn10":                 "TEXT",
		"isbn13":                 "TEXT",
		"asin":                   "TEXT",
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
		"created_at":             "DATETIME DEFAULT CURRENT_TIMESTAMP",
		"updated_at":             "DATETIME",
		"cover_url":              "TEXT",
		"narrators_json":         "TEXT",
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

// ---- User Management ----

func (s *SQLiteStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error) {
	id := ulid.Make().String()
	now := time.Now()
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal roles: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO users (id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		id, username, email, passwordHashAlgo, passwordHash, string(rolesJSON), status, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &User{
		ID: id, Username: username, Email: email,
		PasswordHashAlgo: passwordHashAlgo, PasswordHash: passwordHash,
		Roles: roles, Status: status, CreatedAt: now, UpdatedAt: now, Version: 1,
	}, nil
}

func (s *SQLiteStore) scanUser(row rowScanner) (*User, error) {
	var u User
	var rolesJSON string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHashAlgo, &u.PasswordHash,
		&rolesJSON, &u.Status, &u.CreatedAt, &u.UpdatedAt, &u.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(rolesJSON), &u.Roles)
	return &u, nil
}

func (s *SQLiteStore) GetUserByID(id string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version FROM users WHERE id = ?`, id))
}

func (s *SQLiteStore) GetUserByUsername(username string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version FROM users WHERE username = ?`, username))
}

func (s *SQLiteStore) GetUserByEmail(email string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version FROM users WHERE email = ?`, email))
}

func (s *SQLiteStore) UpdateUser(user *User) error {
	rolesJSON, _ := json.Marshal(user.Roles)
	user.UpdatedAt = time.Now()
	user.Version++
	_, err := s.db.Exec(
		`UPDATE users SET username=?, email=?, password_hash_algo=?, password_hash=?, roles=?, status=?, updated_at=?, version=? WHERE id=?`,
		user.Username, user.Email, user.PasswordHashAlgo, user.PasswordHash,
		string(rolesJSON), user.Status, user.UpdatedAt, user.Version, user.ID,
	)
	return err
}

// ---- Sessions ----

func (s *SQLiteStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	id := ulid.Make().String()
	now := time.Now()
	expiresAt := now.Add(ttl)
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at, ip, user_agent, revoked, version) VALUES (?, ?, ?, ?, ?, ?, 0, 1)`,
		id, userID, now, expiresAt, ip, userAgent,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return &Session{
		ID: id, UserID: userID, CreatedAt: now, ExpiresAt: expiresAt,
		IP: ip, UserAgent: userAgent, Revoked: false, Version: 1,
	}, nil
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var sess Session
	var revoked int
	err := s.db.QueryRow(
		`SELECT id, user_id, created_at, expires_at, ip, user_agent, revoked, version FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UserAgent, &revoked, &sess.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.Revoked = revoked != 0
	return &sess, nil
}

func (s *SQLiteStore) RevokeSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET revoked = 1 WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListUserSessions(userID string) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, created_at, expires_at, ip, user_agent, revoked, version FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var sess Session
		var revoked int
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UserAgent, &revoked, &sess.Version); err != nil {
			return nil, err
		}
		sess.Revoked = revoked != 0
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *SQLiteStore) DeleteExpiredSessions(now time.Time) (int, error) {
	result, err := s.db.Exec(`DELETE FROM sessions WHERE revoked = 1 OR expires_at <= ?`, now)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(rows), nil
}

func (s *SQLiteStore) CountUsers() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ---- Per-User Preferences ----

func (s *SQLiteStore) SetUserPreferenceForUser(userID, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_preferences (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		fmt.Sprintf("user:%s:%s", userID, key), value, time.Now(),
	)
	return err
}

func (s *SQLiteStore) GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error) {
	var pref UserPreferenceKV
	var rawValue sql.NullString
	err := s.db.QueryRow(
		`SELECT value, updated_at FROM user_preferences WHERE key = ?`,
		fmt.Sprintf("user:%s:%s", userID, key),
	).Scan(&rawValue, &pref.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pref.UserID = userID
	pref.Key = key
	if rawValue.Valid {
		pref.Value = rawValue.String
	}
	return &pref, nil
}

func (s *SQLiteStore) GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error) {
	prefix := fmt.Sprintf("user:%s:", userID)
	rows, err := s.db.Query(`SELECT key, value, updated_at FROM user_preferences WHERE key LIKE ?`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prefs []UserPreferenceKV
	for rows.Next() {
		var fullKey string
		var rawValue sql.NullString
		var pref UserPreferenceKV
		if err := rows.Scan(&fullKey, &rawValue, &pref.UpdatedAt); err != nil {
			return nil, err
		}
		pref.UserID = userID
		pref.Key = strings.TrimPrefix(fullKey, prefix)
		if rawValue.Valid {
			pref.Value = rawValue.String
		}
		prefs = append(prefs, pref)
	}
	return prefs, rows.Err()
}

// ---- Book Segments ----

func (s *SQLiteStore) CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error) {
	if segment.ID == "" {
		segment.ID = ulid.Make().String()
	}
	now := time.Now()
	segment.BookID = bookNumericID
	segment.CreatedAt = now
	segment.UpdatedAt = now
	segment.Version = 1
	_, err := s.db.Exec(
		`INSERT INTO book_segments (id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, superseded_by, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		segment.ID, bookNumericID, segment.FilePath, segment.Format, segment.SizeBytes, segment.DurationSec,
		segment.TrackNumber, segment.TotalTracks, segment.FileHash, func() int {
			if segment.Active {
				return 1
			}
			return 0
		}(), segment.SupersededBy,
		segment.CreatedAt, segment.UpdatedAt, segment.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create book segment: %w", err)
	}
	return segment, nil
}

func (s *SQLiteStore) UpdateBookSegment(segment *BookSegment) error {
	segment.UpdatedAt = time.Now()
	segment.Version++
	_, err := s.db.Exec(
		`UPDATE book_segments SET track_number=?, total_tracks=?, updated_at=?, version=? WHERE id=?`,
		segment.TrackNumber, segment.TotalTracks, segment.UpdatedAt, segment.Version, segment.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update book segment: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListBookSegments(bookNumericID int) ([]BookSegment, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, superseded_by, created_at, updated_at, version
		 FROM book_segments WHERE book_id = ? ORDER BY track_number ASC, created_at ASC`, bookNumericID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var segments []BookSegment
	for rows.Next() {
		var seg BookSegment
		var active int
		if err := rows.Scan(&seg.ID, &seg.BookID, &seg.FilePath, &seg.Format, &seg.SizeBytes, &seg.DurationSec,
			&seg.TrackNumber, &seg.TotalTracks, &seg.FileHash, &active, &seg.SupersededBy, &seg.CreatedAt, &seg.UpdatedAt, &seg.Version); err != nil {
			return nil, err
		}
		seg.Active = active != 0
		segments = append(segments, seg)
	}
	return segments, rows.Err()
}

// GetBookSegmentByID retrieves a single segment by its ULID.
func (s *SQLiteStore) GetBookSegmentByID(segmentID string) (*BookSegment, error) {
	row := s.db.QueryRow(
		`SELECT id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, superseded_by, created_at, updated_at, version
		 FROM book_segments WHERE id = ?`, segmentID,
	)
	var seg BookSegment
	var active int
	if err := row.Scan(&seg.ID, &seg.BookID, &seg.FilePath, &seg.Format, &seg.SizeBytes, &seg.DurationSec,
		&seg.TrackNumber, &seg.TotalTracks, &seg.FileHash, &active, &seg.SupersededBy, &seg.CreatedAt, &seg.UpdatedAt, &seg.Version); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("segment not found: %s", segmentID)
		}
		return nil, err
	}
	seg.Active = active != 0
	return &seg, nil
}

// MoveSegmentsToBook reassigns segments to a different book (by numeric ID).
func (s *SQLiteStore) MoveSegmentsToBook(segmentIDs []string, targetBookNumericID int) error {
	if len(segmentIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	for _, segID := range segmentIDs {
		result, err := tx.Exec(
			`UPDATE book_segments SET book_id = ?, updated_at = ?, version = version + 1 WHERE id = ?`,
			targetBookNumericID, now, segID,
		)
		if err != nil {
			return fmt.Errorf("failed to move segment %s: %w", segID, err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("segment not found: %s", segID)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Mark old segments as superseded
	for _, oldID := range supersedeIDs {
		if _, err := tx.Exec(
			`UPDATE book_segments SET active = 0, superseded_by = ?, updated_at = ? WHERE id = ? AND book_id = ?`,
			newSegment.ID, time.Now(), oldID, bookNumericID,
		); err != nil {
			return fmt.Errorf("failed to supersede segment %s: %w", oldID, err)
		}
	}

	// Insert the new merged segment
	if newSegment.ID == "" {
		newSegment.ID = ulid.Make().String()
	}
	now := time.Now()
	if _, err := tx.Exec(
		`INSERT INTO book_segments (id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, 1)`,
		newSegment.ID, bookNumericID, newSegment.FilePath, newSegment.Format, newSegment.SizeBytes, newSegment.DurationSec,
		newSegment.TrackNumber, newSegment.TotalTracks, newSegment.FileHash, now, now,
	); err != nil {
		return fmt.Errorf("failed to insert merged segment: %w", err)
	}

	return tx.Commit()
}

// ---- Playback Tracking ----

func (s *SQLiteStore) AddPlaybackEvent(event *PlaybackEvent) error {
	event.CreatedAt = time.Now()
	event.Version = 1
	_, err := s.db.Exec(
		`INSERT INTO playback_events (user_id, book_id, segment_id, position_seconds, event_type, play_speed, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.UserID, event.BookID, event.SegmentID, event.PositionSec, event.EventType, event.PlaySpeed, event.CreatedAt, event.Version,
	)
	return err
}

func (s *SQLiteStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error) {
	rows, err := s.db.Query(
		`SELECT user_id, book_id, segment_id, position_seconds, event_type, play_speed, created_at, version
		 FROM playback_events WHERE user_id = ? AND book_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, bookNumericID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []PlaybackEvent
	for rows.Next() {
		var e PlaybackEvent
		if err := rows.Scan(&e.UserID, &e.BookID, &e.SegmentID, &e.PositionSec, &e.EventType, &e.PlaySpeed, &e.CreatedAt, &e.Version); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) UpdatePlaybackProgress(progress *PlaybackProgress) error {
	progress.UpdatedAt = time.Now()
	progress.Version++
	_, err := s.db.Exec(
		`INSERT INTO playback_progress (user_id, book_id, segment_id, position_seconds, percent_complete, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, book_id) DO UPDATE SET segment_id=excluded.segment_id, position_seconds=excluded.position_seconds,
		 percent_complete=excluded.percent_complete, updated_at=excluded.updated_at, version=excluded.version`,
		progress.UserID, progress.BookID, progress.SegmentID, progress.PositionSec, progress.Percent, progress.UpdatedAt, progress.Version,
	)
	return err
}

func (s *SQLiteStore) GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error) {
	var p PlaybackProgress
	err := s.db.QueryRow(
		`SELECT user_id, book_id, segment_id, position_seconds, percent_complete, updated_at, version
		 FROM playback_progress WHERE user_id = ? AND book_id = ?`, userID, bookNumericID,
	).Scan(&p.UserID, &p.BookID, &p.SegmentID, &p.PositionSec, &p.Percent, &p.UpdatedAt, &p.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- Stats ----

func (s *SQLiteStore) IncrementBookPlayStats(bookNumericID int, seconds int) error {
	_, err := s.db.Exec(
		`INSERT INTO book_stats (book_id, play_count, listen_seconds, version) VALUES (?, 1, ?, 1)
		 ON CONFLICT(book_id) DO UPDATE SET play_count = play_count + 1, listen_seconds = listen_seconds + ?, version = version + 1`,
		bookNumericID, seconds, seconds,
	)
	return err
}

func (s *SQLiteStore) GetBookStats(bookNumericID int) (*BookStats, error) {
	var bs BookStats
	err := s.db.QueryRow(
		`SELECT book_id, play_count, listen_seconds, version FROM book_stats WHERE book_id = ?`, bookNumericID,
	).Scan(&bs.BookID, &bs.PlayCount, &bs.ListenSeconds, &bs.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &bs, nil
}

func (s *SQLiteStore) IncrementUserListenStats(userID string, seconds int) error {
	_, err := s.db.Exec(
		`INSERT INTO user_stats (user_id, listen_seconds, version) VALUES (?, ?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET listen_seconds = listen_seconds + ?, version = version + 1`,
		userID, seconds, seconds,
	)
	return err
}

func (s *SQLiteStore) GetUserStats(userID string) (*UserStats, error) {
	var us UserStats
	err := s.db.QueryRow(
		`SELECT user_id, listen_seconds, version FROM user_stats WHERE user_id = ?`, userID,
	).Scan(&us.UserID, &us.ListenSeconds, &us.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &us, nil
}

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
	// Use LOWER() for case-insensitive lookup
	err := s.db.QueryRow("SELECT id, name FROM authors WHERE LOWER(name) = LOWER(?)", name).Scan(&author.ID, &author.Name)
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

func (s *SQLiteStore) DeleteAuthor(id int) error {
	_, err := s.db.Exec("DELETE FROM book_authors WHERE author_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM authors WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateAuthorName(id int, name string) error {
	_, err := s.db.Exec("UPDATE authors SET name = ? WHERE id = ?", name, id)
	return err
}

// Author Alias operations

func (s *SQLiteStore) GetAuthorAliases(authorID int) ([]AuthorAlias, error) {
	rows, err := s.db.Query("SELECT id, author_id, alias_name, alias_type, created_at FROM author_aliases WHERE author_id = ? ORDER BY alias_name", authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []AuthorAlias
	for rows.Next() {
		var a AuthorAlias
		if err := rows.Scan(&a.ID, &a.AuthorID, &a.AliasName, &a.AliasType, &a.CreatedAt); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

func (s *SQLiteStore) GetAllAuthorAliases() ([]AuthorAlias, error) {
	rows, err := s.db.Query("SELECT id, author_id, alias_name, alias_type, created_at FROM author_aliases ORDER BY alias_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []AuthorAlias
	for rows.Next() {
		var a AuthorAlias
		if err := rows.Scan(&a.ID, &a.AuthorID, &a.AliasName, &a.AliasType, &a.CreatedAt); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

func (s *SQLiteStore) CreateAuthorAlias(authorID int, aliasName string, aliasType string) (*AuthorAlias, error) {
	result, err := s.db.Exec("INSERT INTO author_aliases (author_id, alias_name, alias_type) VALUES (?, ?, ?)", authorID, aliasName, aliasType)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &AuthorAlias{
		ID:        int(id),
		AuthorID:  authorID,
		AliasName: aliasName,
		AliasType: aliasType,
	}, nil
}

func (s *SQLiteStore) DeleteAuthorAlias(id int) error {
	_, err := s.db.Exec("DELETE FROM author_aliases WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) FindAuthorByAlias(aliasName string) (*Author, error) {
	var a Author
	err := s.db.QueryRow("SELECT a.id, a.name FROM authors a JOIN author_aliases aa ON a.id = aa.author_id WHERE LOWER(aa.alias_name) = LOWER(?)", aliasName).Scan(&a.ID, &a.Name)
	if err != nil {
		return nil, err
	}
	return &a, nil
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

func (s *SQLiteStore) DeleteSeries(id int) error {
	_, err := s.db.Exec("DELETE FROM series WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateSeriesName(id int, name string) error {
	_, err := s.db.Exec("UPDATE series SET name = ? WHERE id = ?", name, id)
	return err
}

func (s *SQLiteStore) GetAllSeriesBookCounts() (map[int]int, error) {
	rows, err := s.db.Query(`SELECT series_id, COUNT(*)
		FROM books
		WHERE series_id IS NOT NULL AND COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1
		GROUP BY series_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int]int)
	for rows.Next() {
		var seriesID, count int
		if err := rows.Scan(&seriesID, &count); err != nil {
			return nil, err
		}
		counts[seriesID] = count
	}
	return counts, rows.Err()
}

// GetAllSeriesFileCounts returns the number of audio files per series.
// Books with active segments count their segments; books without count as 1.
func (s *SQLiteStore) GetAllSeriesFileCounts() (map[int]int, error) {
	rows, err := s.db.Query(`
		SELECT series_id, id
		FROM books
		WHERE series_id IS NOT NULL AND COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect book IDs per series
	seriesBooks := make(map[int][]string)
	for rows.Next() {
		var seriesID int
		var bookID string
		if err := rows.Scan(&seriesID, &bookID); err != nil {
			return nil, err
		}
		seriesBooks[seriesID] = append(seriesBooks[seriesID], bookID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get segment counts per book_id (numeric)
	bookSegCounts := make(map[string]int)
	segRows, err := s.db.Query("SELECT book_id, COUNT(*) FROM book_segments WHERE active = 1 GROUP BY book_id")
	if err == nil {
		defer segRows.Close()
		for segRows.Next() {
			var numericID, count int
			if err := segRows.Scan(&numericID, &count); err != nil {
				break
			}
			bookSegCounts[fmt.Sprintf("%d", numericID)] = count
		}
	}

	counts := make(map[int]int)
	for seriesID, ids := range seriesBooks {
		total := 0
		for _, id := range ids {
			crc := fmt.Sprintf("%d", int(crc32.ChecksumIEEE([]byte(id))))
			if segCount, ok := bookSegCounts[crc]; ok && segCount > 0 {
				total += segCount
			} else {
				total++ // No segments, counts as 1 file
			}
		}
		counts[seriesID] = total
	}

	return counts, nil
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
	// Use LOWER() for case-insensitive lookup
	if authorID != nil {
		err = s.db.QueryRow("SELECT id, name, author_id FROM series WHERE LOWER(name) = LOWER(?) AND author_id = ?", name, *authorID).
			Scan(&series.ID, &series.Name, &series.AuthorID)
	} else {
		err = s.db.QueryRow("SELECT id, name, author_id FROM series WHERE LOWER(name) = LOWER(?) AND author_id IS NULL", name).
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
	// Check for existing series first (handles NULL author_id which bypasses UNIQUE constraint)
	existing, err := s.GetSeriesByName(name, authorID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

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

func (s *SQLiteStore) GetBookByITunesPersistentID(persistentID string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE itunes_persistent_id = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, persistentID), &book)
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

// GetFolderDuplicates detects potential duplicates by grouping books
// that share the same parent directory and title (case-insensitive).
// This catches M4B + MP3 versions of the same book in the same folder.
// It prefers single-file M4B over multi-file formats.
// GetBooksByTitleInDir finds books with the given normalized (lowercased) title
// in the given directory path. Results are ordered so M4B files come first.
func (s *SQLiteStore) GetBooksByTitleInDir(normalizedTitle, dirPath string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE LOWER(title) = ? AND file_path LIKE ? AND COALESCE(marked_for_deletion, 0) = 0
		ORDER BY CASE WHEN format = 'm4b' THEN 0 ELSE 1 END`, bookSelectColumns)
	rows, err := s.db.Query(query, normalizedTitle, dirPath+"/%")
	if err != nil {
		return nil, fmt.Errorf("failed to query books by title in dir: %w", err)
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

func (s *SQLiteStore) GetFolderDuplicates() ([][]Book, error) {
	// Group by parent directory + lower(title) where there are 2+ books
	query := fmt.Sprintf(`
		WITH book_dirs AS (
			SELECT id, LOWER(title) as ltitle,
			       SUBSTR(file_path, 1, LENGTH(file_path) - LENGTH(REPLACE(file_path, '/', '')) - LENGTH(SUBSTR(file_path, LENGTH(file_path) - LENGTH(REPLACE(file_path, '/', '')) + 1))) as dir
			FROM books
			WHERE COALESCE(marked_for_deletion, 0) = 0
			  AND (version_group_id IS NULL OR version_group_id = '')
		)
		SELECT dir, ltitle, COUNT(*) as cnt
		FROM book_dirs
		WHERE dir != '' AND ltitle != ''
		GROUP BY dir, ltitle
		HAVING cnt > 1
	`)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query folder duplicates: %w", err)
	}
	defer rows.Close()

	var groups [][]Book
	for rows.Next() {
		var dir, ltitle string
		var cnt int
		if err := rows.Scan(&dir, &ltitle, &cnt); err != nil {
			return nil, err
		}
		// Fetch the actual books
		booksQuery := fmt.Sprintf(`
			SELECT %s FROM books
			WHERE LOWER(title) = ? AND file_path LIKE ?
			  AND COALESCE(marked_for_deletion, 0) = 0
			  AND (version_group_id IS NULL OR version_group_id = '')
			ORDER BY CASE WHEN format = 'm4b' THEN 0 ELSE 1 END, file_path
		`, bookSelectColumns)
		bookRows, err := s.db.Query(booksQuery, ltitle, dir+"/%")
		if err != nil {
			return nil, err
		}
		var group []Book
		for bookRows.Next() {
			var book Book
			if err := scanBook(bookRows, &book); err != nil {
				bookRows.Close()
				return nil, err
			}
			group = append(group, book)
		}
		bookRows.Close()
		if len(group) >= 2 {
			groups = append(groups, group)
		}
	}
	return groups, nil
}

// normalizeTitle normalizes a book title for comparison: lowercase, strip articles,
// remove parenthesized suffixes like "(Unabridged)", collapse whitespace.
func normalizeTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	// Remove common parenthesized suffixes
	for _, suffix := range []string{"(unabridged)", "(abridged)", "(audiobook)", "(audio)"} {
		s = strings.ReplaceAll(s, suffix, "")
	}
	// Remove leading articles
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(s, article) {
			s = s[len(article):]
			break
		}
	}
	// Collapse multiple spaces
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

// jaroWinkler computes the Jaro-Winkler similarity between two strings (0.0–1.0).
func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// Jaro distance
	matchDist := max(len(s1), len(s2))/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	s1Matches := make([]bool, len(s1))
	s2Matches := make([]bool, len(s2))
	matches := 0
	transpositions := 0

	for i := 0; i < len(s1); i++ {
		start := i - matchDist
		if start < 0 {
			start = 0
		}
		end := i + matchDist + 1
		if end > len(s2) {
			end = len(s2)
		}
		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := 0; i < len(s1); i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	jaro := (float64(matches)/float64(len(s1)) +
		float64(matches)/float64(len(s2)) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0

	// Winkler modification: boost for common prefix (up to 4 chars)
	prefix := 0
	for i := 0; i < min(4, min(len(s1), len(s2))); i++ {
		if s1[i] == s2[i] {
			prefix++
		} else {
			break
		}
	}

	return jaro + float64(prefix)*0.1*(1.0-jaro)
}

// GetDuplicateBooksByMetadata finds books that appear to be duplicates based on
// title + author matching. Books with the same author_id and similar titles
// (after normalization) are grouped together. The threshold parameter controls
// how similar titles must be (0.0–1.0, where 1.0 = exact match). Duration is
// used as an additional signal: if both books have duration, they must be within
// 5% to be grouped.
func (s *SQLiteStore) GetDuplicateBooksByMetadata(threshold float64) ([][]Book, error) {
	// Fetch all non-deleted books that aren't already in a version group
	query := fmt.Sprintf(`
		SELECT %s FROM books
		WHERE COALESCE(marked_for_deletion, 0) = 0
		  AND title != ''
		  AND author_id IS NOT NULL
		ORDER BY author_id, LOWER(title)
	`, bookSelectColumns)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query books for metadata dedup: %w", err)
	}
	defer rows.Close()

	var allBooks []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		allBooks = append(allBooks, book)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Group books by author_id first, then find title matches within each author group
	authorGroups := map[int][]Book{}
	for _, b := range allBooks {
		if b.AuthorID == nil {
			continue
		}
		authorGroups[*b.AuthorID] = append(authorGroups[*b.AuthorID], b)
	}

	var duplicateGroups [][]Book

	for _, books := range authorGroups {
		if len(books) < 2 {
			continue
		}
		// Track which books have been assigned to a group
		assigned := make([]bool, len(books))

		for i := 0; i < len(books); i++ {
			if assigned[i] {
				continue
			}
			group := []Book{books[i]}
			assigned[i] = true

			normI := normalizeTitle(books[i].Title)

			for j := i + 1; j < len(books); j++ {
				if assigned[j] {
					continue
				}
				normJ := normalizeTitle(books[j].Title)
				sim := jaroWinkler(normI, normJ)
				if sim < threshold {
					continue
				}
				// If both have duration, check within 5%
				if books[i].Duration != nil && books[j].Duration != nil {
					di := float64(*books[i].Duration)
					dj := float64(*books[j].Duration)
					if di > 0 && dj > 0 {
						ratio := di / dj
						if ratio < 0.95 || ratio > 1.05 {
							continue
						}
					}
				}
				group = append(group, books[j])
				assigned[j] = true
			}

			if len(group) >= 2 {
				duplicateGroups = append(duplicateGroups, group)
			}
		}
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

func (s *SQLiteStore) GetBookAuthors(bookID string) ([]BookAuthor, error) {
	rows, err := s.db.Query(`SELECT book_id, author_id, role, position FROM book_authors WHERE book_id = ? ORDER BY position`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []BookAuthor
	for rows.Next() {
		var ba BookAuthor
		if err := rows.Scan(&ba.BookID, &ba.AuthorID, &ba.Role, &ba.Position); err != nil {
			return nil, err
		}
		authors = append(authors, ba)
	}
	return authors, rows.Err()
}

func (s *SQLiteStore) SetBookAuthors(bookID string, authors []BookAuthor) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM book_authors WHERE book_id = ?`, bookID); err != nil {
		return err
	}

	for _, ba := range authors {
		if _, err := tx.Exec(
			`INSERT INTO book_authors (book_id, author_id, role, position) VALUES (?, ?, ?, ?)`,
			bookID, ba.AuthorID, ba.Role, ba.Position,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetBooksByAuthorIDWithRole(authorID int) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE id IN (
		SELECT book_id FROM book_authors WHERE author_id = ?
	) AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY title`, bookSelectColumns)
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

func (s *SQLiteStore) GetAllAuthorBookCounts() (map[int]int, error) {
	rows, err := s.db.Query(`SELECT ba.author_id, COUNT(DISTINCT ba.book_id)
		FROM book_authors ba
		JOIN books b ON b.id = ba.book_id
		WHERE COALESCE(b.marked_for_deletion, 0) = 0 AND COALESCE(b.is_primary_version, 1) = 1
		GROUP BY ba.author_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int]int)
	for rows.Next() {
		var authorID, count int
		if err := rows.Scan(&authorID, &count); err != nil {
			return nil, err
		}
		counts[authorID] = count
	}
	return counts, rows.Err()
}

// GetAllAuthorFileCounts returns the number of audio files per author.
// Books with active segments count their segments; books without count as 1.
func (s *SQLiteStore) GetAllAuthorFileCounts() (map[int]int, error) {
	// For each author's books, calculate file count:
	// - Books with active segments: count segments
	// - Books without segments: count as 1 file
	rows, err := s.db.Query(`
		SELECT ba.author_id, b.id
		FROM book_authors ba
		JOIN books b ON b.id = ba.book_id
		WHERE COALESCE(b.marked_for_deletion, 0) = 0 AND COALESCE(b.is_primary_version, 1) = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect book IDs per author
	authorBooks := make(map[int][]string)
	for rows.Next() {
		var authorID int
		var bookID string
		if err := rows.Scan(&authorID, &bookID); err != nil {
			return nil, err
		}
		authorBooks[authorID] = append(authorBooks[authorID], bookID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Build a set of all unique book IDs to look up segments
	allBookIDs := make(map[string]bool)
	for _, ids := range authorBooks {
		for _, id := range ids {
			allBookIDs[id] = true
		}
	}

	// Get segment counts per book (using book_segments table)
	bookSegCounts := make(map[string]int)
	segRows, err := s.db.Query("SELECT book_id, COUNT(*) FROM book_segments WHERE active = 1 GROUP BY book_id")
	if err == nil {
		defer segRows.Close()
		for segRows.Next() {
			var numericID, count int
			if err := segRows.Scan(&numericID, &count); err != nil {
				break
			}
			// We need to map numeric IDs back; store by numeric ID for now
			bookSegCounts[fmt.Sprintf("%d", numericID)] = count
		}
	}

	// Build CRC32 lookup for our book IDs
	bookCRC := make(map[string]string) // CRC32 string -> book ID
	for id := range allBookIDs {
		crc := fmt.Sprintf("%d", int(crc32.ChecksumIEEE([]byte(id))))
		bookCRC[crc] = id
	}

	// Calculate file counts per author
	counts := make(map[int]int)
	for authorID, ids := range authorBooks {
		total := 0
		for _, id := range ids {
			crc := fmt.Sprintf("%d", int(crc32.ChecksumIEEE([]byte(id))))
			if segCount, ok := bookSegCounts[crc]; ok && segCount > 0 {
				total += segCount
			} else {
				total++ // No segments, counts as 1 file
			}
		}
		counts[authorID] = total
	}

	return counts, nil
}

// --- Narrator methods ---

func (s *SQLiteStore) CreateNarrator(name string) (*Narrator, error) {
	result, err := s.db.Exec("INSERT INTO narrators (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetNarratorByID(int(id))
}

func (s *SQLiteStore) GetNarratorByID(id int) (*Narrator, error) {
	var n Narrator
	err := s.db.QueryRow("SELECT id, name, created_at FROM narrators WHERE id = ?", id).Scan(&n.ID, &n.Name, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *SQLiteStore) GetNarratorByName(name string) (*Narrator, error) {
	var n Narrator
	err := s.db.QueryRow("SELECT id, name, created_at FROM narrators WHERE LOWER(name) = LOWER(?)", name).Scan(&n.ID, &n.Name, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *SQLiteStore) ListNarrators() ([]Narrator, error) {
	rows, err := s.db.Query("SELECT id, name, created_at FROM narrators ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var narrators []Narrator
	for rows.Next() {
		var n Narrator
		if err := rows.Scan(&n.ID, &n.Name, &n.CreatedAt); err != nil {
			return nil, err
		}
		narrators = append(narrators, n)
	}
	return narrators, rows.Err()
}

func (s *SQLiteStore) GetBookNarrators(bookID string) ([]BookNarrator, error) {
	rows, err := s.db.Query(`SELECT book_id, narrator_id, role, position FROM book_narrators WHERE book_id = ? ORDER BY position`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var narrators []BookNarrator
	for rows.Next() {
		var bn BookNarrator
		if err := rows.Scan(&bn.BookID, &bn.NarratorID, &bn.Role, &bn.Position); err != nil {
			return nil, err
		}
		narrators = append(narrators, bn)
	}
	return narrators, rows.Err()
}

func (s *SQLiteStore) SetBookNarrators(bookID string, narrators []BookNarrator) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM book_narrators WHERE book_id = ?`, bookID); err != nil {
		return err
	}

	for _, bn := range narrators {
		if _, err := tx.Exec(
			`INSERT INTO book_narrators (book_id, narrator_id, role, position) VALUES (?, ?, ?, ?)`,
			bookID, bn.NarratorID, bn.Role, bn.Position,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
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

	// Set timestamps
	now := time.Now()
	book.CreatedAt = &now
	book.UpdatedAt = &now

	query := `INSERT INTO books (
		id, title, author_id, series_id, series_sequence, file_path, original_filename,
		format, duration, work_id, narrator, edition, description, language, publisher, genre,
		print_year, audiobook_release_year, isbn10, isbn13, asin,
		open_library_id, hardcover_id, google_books_id,
		itunes_persistent_id, itunes_date_added, itunes_play_count, itunes_last_played,
		itunes_rating, itunes_bookmark, itunes_import_source,
		file_hash, file_size, bitrate_kbps, codec, sample_rate_hz, channels,
		bit_depth, quality, is_primary_version, version_group_id, version_notes,
		original_file_hash, organized_file_hash, library_state, quantity, marked_for_deletion, marked_for_deletion_at,
		created_at, updated_at, cover_url, narrators_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query,
		book.ID, book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence, book.FilePath, book.OriginalFilename,
		book.Format, book.Duration, book.WorkID, book.Narrator, book.Edition, book.Description, book.Language, book.Publisher, book.Genre,
		book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13, book.ASIN,
		book.OpenLibraryID, book.HardcoverID, book.GoogleBooksID,
		book.ITunesPersistentID, book.ITunesDateAdded, book.ITunesPlayCount, book.ITunesLastPlayed,
		book.ITunesRating, book.ITunesBookmark, book.ITunesImportSource,
		book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
		book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
		book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity, book.MarkedForDeletion, book.MarkedForDeletionAt,
		book.CreatedAt, book.UpdatedAt, book.CoverURL, book.NarratorsJSON,
	)
	if err != nil {
		return nil, err
	}
	return book, nil
}

// metadataChanged returns true if any user-visible metadata field differs between
// old and new. Internal-only fields (FileHash, LibraryState, ITunes*, etc.) are
// intentionally excluded so that system updates do not bump metadata_updated_at.
func metadataChanged(old, new *Book) bool {
	if old.Title != new.Title {
		return true
	}
	if !equalIntPtr(old.AuthorID, new.AuthorID) {
		return true
	}
	if !equalIntPtr(old.SeriesID, new.SeriesID) {
		return true
	}
	if !equalIntPtr(old.SeriesSequence, new.SeriesSequence) {
		return true
	}
	if !equalStringPtr(old.Narrator, new.Narrator) {
		return true
	}
	if !equalStringPtr(old.Publisher, new.Publisher) {
		return true
	}
	if !equalStringPtr(old.Language, new.Language) {
		return true
	}
	if !equalIntPtr(old.AudiobookReleaseYear, new.AudiobookReleaseYear) {
		return true
	}
	if !equalIntPtr(old.PrintYear, new.PrintYear) {
		return true
	}
	if !equalStringPtr(old.ISBN10, new.ISBN10) {
		return true
	}
	if !equalStringPtr(old.ISBN13, new.ISBN13) {
		return true
	}
	if !equalStringPtr(old.CoverURL, new.CoverURL) {
		return true
	}
	if !equalStringPtr(old.NarratorsJSON, new.NarratorsJSON) {
		return true
	}
	return false
}

// equalStringPtr returns true if both pointers are nil, or both point to equal strings.
func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// equalIntPtr returns true if both pointers are nil, or both point to equal ints.
func equalIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (s *SQLiteStore) UpdateBook(id string, book *Book) (*Book, error) {
	// Always stamp updated_at — this tracks every DB write for debugging.
	now := time.Now()
	book.UpdatedAt = &now

	// Fetch the current book to detect whether metadata actually changed.
	current, fetchErr := s.GetBookByID(id)

	if fetchErr == nil && current != nil && metadataChanged(current, book) {
		book.MetadataUpdatedAt = &now
	} else if fetchErr == nil && current != nil {
		// Preserve the existing metadata_updated_at value — nothing changed.
		book.MetadataUpdatedAt = current.MetadataUpdatedAt
	}

	// Never touch last_written_at in UpdateBook. It is set by SetLastWrittenAt only.
	if current != nil {
		book.LastWrittenAt = current.LastWrittenAt
	}

	query := `UPDATE books SET
		title = ?, author_id = ?, series_id = ?, series_sequence = ?,
		file_path = ?, original_filename = ?, format = ?, duration = ?,
		work_id = ?, narrator = ?, edition = ?, description = ?, language = ?, publisher = ?, genre = ?,
		print_year = ?, audiobook_release_year = ?, isbn10 = ?, isbn13 = ?, asin = ?,
		open_library_id = ?, hardcover_id = ?, google_books_id = ?,
		itunes_persistent_id = ?, itunes_date_added = ?, itunes_play_count = ?, itunes_last_played = ?,
		itunes_rating = ?, itunes_bookmark = ?, itunes_import_source = ?,
		file_hash = ?, file_size = ?, bitrate_kbps = ?, codec = ?, sample_rate_hz = ?, channels = ?,
		bit_depth = ?, quality = ?, is_primary_version = ?, version_group_id = ?, version_notes = ?,
		original_file_hash = ?, organized_file_hash = ?, library_state = ?, quantity = ?,
		marked_for_deletion = ?, marked_for_deletion_at = ?,
		updated_at = ?, metadata_updated_at = ?, last_written_at = ?,
		metadata_review_status = ?, cover_url = ?, narrators_json = ?
	WHERE id = ?`
	result, err := s.db.Exec(query,
		book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence,
		book.FilePath, book.OriginalFilename, book.Format, book.Duration,
		book.WorkID, book.Narrator, book.Edition, book.Description, book.Language, book.Publisher, book.Genre,
		book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13, book.ASIN,
		book.OpenLibraryID, book.HardcoverID, book.GoogleBooksID,
		book.ITunesPersistentID, book.ITunesDateAdded, book.ITunesPlayCount, book.ITunesLastPlayed,
		book.ITunesRating, book.ITunesBookmark, book.ITunesImportSource,
		book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
		book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
		book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity,
		book.MarkedForDeletion, book.MarkedForDeletionAt,
		book.UpdatedAt, book.MetadataUpdatedAt, book.LastWrittenAt,
		book.MetadataReviewStatus, book.CoverURL, book.NarratorsJSON, id,
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

// SetLastWrittenAt stamps the last_written_at timestamp for book id.
func (s *SQLiteStore) SetLastWrittenAt(id string, t time.Time) error {
	_, err := s.db.Exec(
		`UPDATE books SET last_written_at = ? WHERE id = ?`,
		t, id,
	)
	return err
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
	// Fetch a larger pool for fuzzy re-ranking (we re-rank in Go, then paginate)
	fetchLimit := limit * 3
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	// Search by title (FTS5) and author name (LIKE on authors table)
	ftsQuery := sanitizeFTS5Query(query)
	likeParam := "%" + query + "%"

	// Try FTS5 for title + LIKE for author via UNION
	bq := bookSelectColumnsQualified
	searchSQL := fmt.Sprintf(
		`SELECT %s FROM books
		 JOIN books_fts ON books.rowid = books_fts.rowid
		 WHERE books_fts MATCH ? AND COALESCE(books.marked_for_deletion, 0) = 0
		 UNION
		 SELECT %s FROM books
		 JOIN authors ON books.author_id = authors.id
		 WHERE authors.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
		 UNION
		 SELECT %s FROM books
		 JOIN book_authors ba ON ba.book_id = books.id
		 JOIN authors a2 ON ba.author_id = a2.id
		 WHERE a2.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
		 LIMIT ?`, bq, bq, bq)

	rows, err := s.db.Query(searchSQL, ftsQuery, likeParam, likeParam, fetchLimit)
	if err != nil {
		// Fall back to pure LIKE if FTS5 not available
		likeSQL := fmt.Sprintf(
			`SELECT %s FROM books
			 WHERE books.title LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
			 UNION
			 SELECT %s FROM books
			 JOIN authors ON books.author_id = authors.id
			 WHERE authors.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
			 UNION
			 SELECT %s FROM books
			 JOIN book_authors ba ON ba.book_id = books.id
			 JOIN authors a2 ON ba.author_id = a2.id
			 WHERE a2.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
			 LIMIT ?`, bq, bq, bq)
		rows, err = s.db.Query(likeSQL, likeParam, likeParam, likeParam, fetchLimit)
		if err != nil {
			return nil, err
		}
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Re-rank using fuzzy scoring
	books = fuzzyRankBooks(query, books)

	// Apply pagination after ranking
	if offset >= len(books) {
		return []Book{}, nil
	}
	end := offset + limit
	if end > len(books) {
		end = len(books)
	}
	return books[offset:end], nil
}

// fuzzyRankBooks re-ranks books by fuzzy match score against the query.
func fuzzyRankBooks(query string, books []Book) []Book {
	if len(books) <= 1 {
		return books
	}
	type scored struct {
		book  Book
		score int
	}
	items := make([]scored, len(books))
	for i, b := range books {
		items[i] = scored{book: b, score: matcher.ScoreMatch(query, b.Title)}
	}
	// Insertion sort (stable, fine for small N)
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].score > items[j-1].score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	result := make([]Book, len(items))
	for i, it := range items {
		result[i] = it.book
	}
	return result
}

// sanitizeFTS5Query escapes FTS5 special characters and wraps terms for prefix matching.
func sanitizeFTS5Query(q string) string {
	// Remove FTS5 operators that could cause syntax errors
	replacer := strings.NewReplacer(
		`"`, ``,
		`*`, ``,
		`(`, ``,
		`)`, ``,
	)
	cleaned := replacer.Replace(q)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return `""`
	}
	// Quote the whole thing and add prefix matching
	return `"` + cleaned + `"` + "*"
}

func (s *SQLiteStore) CountBooks() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1").Scan(&count)
	return count, err
}

// CountFiles returns the total number of audio files across all books.
// Books with active segments count their segments; books without segments count as 1 file each.
func (s *SQLiteStore) CountFiles() (int, error) {
	// Count active segments
	var segCount int
	err := s.db.QueryRow("SELECT COUNT(*) FROM book_segments WHERE active = 1").Scan(&segCount)
	if err != nil {
		// Table may not exist yet; treat as 0
		segCount = 0
	}

	// Count all primary, non-deleted books
	var bookCount int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1`).Scan(&bookCount)
	if err != nil {
		return 0, err
	}

	// Count distinct books that have active segments
	var booksWithSegs int
	err = s.db.QueryRow("SELECT COUNT(DISTINCT book_id) FROM book_segments WHERE active = 1").Scan(&booksWithSegs)
	if err != nil {
		// Table may not exist yet; treat as 0
		booksWithSegs = 0
	}

	// Total files = active segments + books without any segments (each counts as 1 file)
	return segCount + (bookCount - booksWithSegs), nil
}

func (s *SQLiteStore) CountAuthors() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM authors").Scan(&count)
	return count, err
}

func (s *SQLiteStore) CountSeries() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM series").Scan(&count)
	return count, err
}

func (s *SQLiteStore) GetBookCountsByLocation(rootDir string) (library, import_ int, err error) {
	const primaryFilter = " AND COALESCE(is_primary_version, 1) = 1"
	if rootDir == "" {
		// No root dir configured, all books are imports
		err = s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0" + primaryFilter).Scan(&import_)
		return 0, import_, err
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND file_path LIKE ?" + primaryFilter, rootDir+"%").Scan(&library)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND file_path NOT LIKE ?" + primaryFilter, rootDir+"%").Scan(&import_)
	return
}

func (s *SQLiteStore) GetBookSizesByLocation(rootDir string) (librarySize, importSize int64, err error) {
	if rootDir == "" {
		err = s.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0").Scan(&importSize)
		return 0, importSize, err
	}
	err = s.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND file_path LIKE ?", rootDir+"%").Scan(&librarySize)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND file_path NOT LIKE ?", rootDir+"%").Scan(&importSize)
	return
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

// GetDashboardStats returns aggregated dashboard statistics using SQL aggregation
// instead of loading all books into memory.
func (s *SQLiteStore) GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{
		StateDistribution:  make(map[string]int),
		FormatDistribution: make(map[string]int),
	}

	// Aggregate counts and totals
	err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(duration), 0), COALESCE(SUM(file_size), 0)
		FROM books WHERE COALESCE(marked_for_deletion, 0) = 0`).Scan(
		&stats.TotalBooks, &stats.TotalDuration, &stats.TotalSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get book aggregates: %w", err)
	}

	// File count
	if fc, err := s.CountFiles(); err == nil {
		stats.TotalFiles = fc
	}

	// Author and series counts
	if ac, err := s.CountAuthors(); err == nil {
		stats.TotalAuthors = ac
	}
	if sc, err := s.CountSeries(); err == nil {
		stats.TotalSeries = sc
	}

	// State distribution
	rows, err := s.db.Query(`SELECT COALESCE(library_state, 'imported'), COUNT(*)
		FROM books WHERE COALESCE(marked_for_deletion, 0) = 0
		GROUP BY COALESCE(library_state, 'imported')`)
	if err != nil {
		return nil, fmt.Errorf("failed to get state distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		stats.StateDistribution[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Format distribution
	rows2, err := s.db.Query(`SELECT COALESCE(codec, 'unknown'), COUNT(*)
		FROM books WHERE COALESCE(marked_for_deletion, 0) = 0
		GROUP BY COALESCE(codec, 'unknown')`)
	if err != nil {
		return nil, fmt.Errorf("failed to get format distribution: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var codec string
		var count int
		if err := rows2.Scan(&codec, &count); err != nil {
			return nil, err
		}
		stats.FormatDistribution[codec] = count
	}
	return stats, rows2.Err()
}

// Book Tombstones (safe deletion pattern)
// SQLite uses a dedicated tombstones table. For simplicity, we serialize the book as JSON.

func (s *SQLiteStore) CreateBookTombstone(book *Book) error {
	data, err := json.Marshal(book)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO book_tombstones (id, data, created_at) VALUES (?, ?, datetime('now'))`, book.ID, string(data))
	return err
}

func (s *SQLiteStore) GetBookTombstone(id string) (*Book, error) {
	row := s.db.QueryRow(`SELECT data FROM book_tombstones WHERE id = ?`, id)
	var data string
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var book Book
	if err := json.Unmarshal([]byte(data), &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) DeleteBookTombstone(id string) error {
	_, err := s.db.Exec(`DELETE FROM book_tombstones WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListBookTombstones(limit int) ([]Book, error) {
	rows, err := s.db.Query(`SELECT data FROM book_tombstones ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var book Book
		if err := json.Unmarshal([]byte(data), &book); err != nil {
			continue
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
			  created_at, started_at, completed_at, error_message, result_data
			  FROM operations WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(&op.ID, &op.Type, &op.Status, &op.Progress,
		&op.Total, &op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
		&op.CompletedAt, &op.ErrorMessage, &op.ResultData)
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
			  created_at, started_at, completed_at, error_message, result_data
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
			&op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

func (s *SQLiteStore) ListOperations(limit, offset int) ([]Operation, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM operations").Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT id, type, status, progress, total, message, folder_path,
			  created_at, started_at, completed_at, error_message, result_data
			  FROM operations ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var operations []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total,
			&op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
			&op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, 0, err
		}
		operations = append(operations, op)
	}
	return operations, total, rows.Err()
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

// ---- Operation Summary Logs (persistent across restarts) ----

func (s *SQLiteStore) SaveOperationSummaryLog(op *OperationSummaryLog) error {
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO operation_summary_logs (id, type, status, progress, result, error, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, progress=excluded.progress,
		result=excluded.result, error=excluded.error, updated_at=excluded.updated_at,
		completed_at=excluded.completed_at`,
		op.ID, op.Type, op.Status, op.Progress, op.Result, op.Error, op.CreatedAt, now, op.CompletedAt)
	return err
}

func (s *SQLiteStore) GetOperationSummaryLog(id string) (*OperationSummaryLog, error) {
	var op OperationSummaryLog
	err := s.db.QueryRow(`SELECT id, type, status, progress, result, error, created_at, updated_at, completed_at
		FROM operation_summary_logs WHERE id = ?`, id).Scan(
		&op.ID, &op.Type, &op.Status, &op.Progress, &op.Result, &op.Error,
		&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *SQLiteStore) ListOperationSummaryLogs(limit, offset int) ([]OperationSummaryLog, error) {
	rows, err := s.db.Query(`SELECT id, type, status, progress, result, error, created_at, updated_at, completed_at
		FROM operation_summary_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []OperationSummaryLog
	for rows.Next() {
		var op OperationSummaryLog
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Result, &op.Error,
			&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt); err != nil {
			return nil, err
		}
		logs = append(logs, op)
	}
	return logs, rows.Err()
}

// ---- Operation State Persistence (resumable operations) ----

func (s *SQLiteStore) ensureOpStateTable() {
	s.db.Exec(`CREATE TABLE IF NOT EXISTS operation_state (
		op_id TEXT NOT NULL,
		key_suffix TEXT NOT NULL DEFAULT '',
		data BLOB NOT NULL,
		PRIMARY KEY (op_id, key_suffix)
	)`)
}

func (s *SQLiteStore) SaveOperationState(opID string, state []byte) error {
	s.ensureOpStateTable()
	_, err := s.db.Exec(`INSERT INTO operation_state (op_id, key_suffix, data) VALUES (?, '', ?)
		ON CONFLICT(op_id, key_suffix) DO UPDATE SET data = ?`, opID, state, state)
	return err
}

func (s *SQLiteStore) GetOperationState(opID string) ([]byte, error) {
	s.ensureOpStateTable()
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM operation_state WHERE op_id = ? AND key_suffix = ''`, opID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

func (s *SQLiteStore) SaveOperationParams(opID string, params []byte) error {
	s.ensureOpStateTable()
	_, err := s.db.Exec(`INSERT INTO operation_state (op_id, key_suffix, data) VALUES (?, 'params', ?)
		ON CONFLICT(op_id, key_suffix) DO UPDATE SET data = ?`, opID, params, params)
	return err
}

func (s *SQLiteStore) GetOperationParams(opID string) ([]byte, error) {
	s.ensureOpStateTable()
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM operation_state WHERE op_id = ? AND key_suffix = 'params'`, opID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

func (s *SQLiteStore) DeleteOperationState(opID string) error {
	s.ensureOpStateTable()
	_, err := s.db.Exec(`DELETE FROM operation_state WHERE op_id = ?`, opID)
	return err
}

func (s *SQLiteStore) DeleteOperationsByStatus(statuses []string) (int, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	placeholders := ""
	args := make([]interface{}, len(statuses))
	for i, s := range statuses {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = s
	}
	result, err := s.db.Exec(`DELETE FROM operations WHERE status IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	// Also clean up summary logs
	_, _ = s.db.Exec(`DELETE FROM operation_summary_logs WHERE status IN (`+placeholders+`)`, args...)
	return int(n), nil
}

func (s *SQLiteStore) UpdateOperationResultData(id string, resultData string) error {
	_, err := s.db.Exec("UPDATE operations SET result_data = ? WHERE id = ?", resultData, id)
	return err
}

func (s *SQLiteStore) GetInterruptedOperations() ([]Operation, error) {
	query := `SELECT id, type, status, progress, total, message, folder_path,
		created_at, started_at, completed_at, error_message, result_data
		FROM operations WHERE status IN ('running', 'queued', 'interrupted')`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total,
			&op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
			&op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
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
		FROM metadata_states WHERE book_id = ? ORDER BY field`, bookID)
	if err != nil {
		return nil, fmt.Errorf("failed to query metadata_states: %w", err)
	}
	defer rows.Close()

	var states []MetadataFieldState
	for rows.Next() {
		var state MetadataFieldState
		var fetchedVal, overrideVal sql.NullString

		if err := rows.Scan(&state.BookID, &state.Field, &fetchedVal, &overrideVal, &state.OverrideLocked, &state.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan metadata_state: %w", err)
		}

		if fetchedVal.Valid {
			state.FetchedValue = &fetchedVal.String
		}
		if overrideVal.Valid {
			state.OverrideValue = &overrideVal.String
		}

		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *SQLiteStore) UpsertMetadataFieldState(state *MetadataFieldState) error {
	if state == nil {
		return fmt.Errorf("metadata state cannot be nil")
	}
	if state.BookID == "" || state.Field == "" {
		return fmt.Errorf("book_id and field are required")
	}

	_, err := s.db.Exec(`INSERT INTO metadata_states (book_id, field, fetched_value, override_value, override_locked, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(book_id, field) DO UPDATE SET
			fetched_value = excluded.fetched_value,
			override_value = excluded.override_value,
			override_locked = excluded.override_locked,
			updated_at = CURRENT_TIMESTAMP`,
		state.BookID, state.Field, state.FetchedValue, state.OverrideValue, state.OverrideLocked)
	return err
}

func (s *SQLiteStore) DeleteMetadataFieldState(bookID, field string) error {
	if bookID == "" || field == "" {
		return fmt.Errorf("book_id and field are required")
	}
	_, err := s.db.Exec("DELETE FROM metadata_states WHERE book_id = ? AND field = ?", bookID, field)
	return err
}

// Metadata change history operations

func (s *SQLiteStore) RecordMetadataChange(record *MetadataChangeRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO metadata_changes_history (book_id, field, previous_value, new_value, change_type, source, changed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.BookID, record.Field, record.PreviousValue, record.NewValue,
		record.ChangeType, record.Source, record.ChangedAt,
	)
	return err
}

func (s *SQLiteStore) GetMetadataChangeHistory(bookID string, field string, limit int) ([]MetadataChangeRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, book_id, field, previous_value, new_value, change_type, source, changed_at
		 FROM metadata_changes_history WHERE book_id = ? AND field = ? ORDER BY changed_at DESC LIMIT ?`,
		bookID, field, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MetadataChangeRecord
	for rows.Next() {
		var r MetadataChangeRecord
		var prevVal, newVal, source sql.NullString
		if err := rows.Scan(&r.ID, &r.BookID, &r.Field, &prevVal, &newVal, &r.ChangeType, &source, &r.ChangedAt); err != nil {
			return nil, err
		}
		if prevVal.Valid {
			r.PreviousValue = &prevVal.String
		}
		if newVal.Valid {
			r.NewValue = &newVal.String
		}
		if source.Valid {
			r.Source = source.String
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) GetBookChangeHistory(bookID string, limit int) ([]MetadataChangeRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, book_id, field, previous_value, new_value, change_type, source, changed_at
		 FROM metadata_changes_history WHERE book_id = ? ORDER BY changed_at DESC LIMIT ?`,
		bookID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MetadataChangeRecord
	for rows.Next() {
		var r MetadataChangeRecord
		var prevVal, newVal, source sql.NullString
		if err := rows.Scan(&r.ID, &r.BookID, &r.Field, &prevVal, &newVal, &r.ChangeType, &source, &r.ChangedAt); err != nil {
			return nil, err
		}
		if prevVal.Valid {
			r.PreviousValue = &prevVal.String
		}
		if newVal.Valid {
			r.NewValue = &newVal.String
		}
		if source.Valid {
			r.Source = source.String
		}
		records = append(records, r)
	}
	return records, rows.Err()
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

// SaveLibraryFingerprint stores or updates the fingerprint for an iTunes library file.
func (s *SQLiteStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32val uint32) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO itunes_library_state (path, size, mod_time, crc32, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		path, size, modTime.Format(time.RFC3339), crc32val, time.Now().Format(time.RFC3339),
	)
	return err
}

// GetLibraryFingerprint retrieves the stored fingerprint for an iTunes library file.
func (s *SQLiteStore) GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error) {
	row := s.db.QueryRow(
		"SELECT path, size, mod_time, crc32, updated_at FROM itunes_library_state WHERE path = ?",
		path,
	)
	var rec LibraryFingerprintRecord
	var modTimeStr, updatedAtStr string
	err := row.Scan(&rec.Path, &rec.Size, &modTimeStr, &rec.CRC32, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.ModTime, _ = time.Parse(time.RFC3339, modTimeStr)
	rec.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	return &rec, nil
}

// CreateDeferredITunesUpdate stores a deferred iTunes path update.
func (s *SQLiteStore) CreateDeferredITunesUpdate(bookID, persistentID, oldPath, newPath, updateType string) error {
	_, err := s.db.Exec(
		`INSERT INTO deferred_itunes_updates (book_id, persistent_id, old_path, new_path, update_type)
		 VALUES (?, ?, ?, ?, ?)`,
		bookID, persistentID, oldPath, newPath, updateType,
	)
	return err
}

// GetPendingDeferredITunesUpdates returns all deferred updates that haven't been applied yet.
func (s *SQLiteStore) GetPendingDeferredITunesUpdates() ([]DeferredITunesUpdate, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, persistent_id, old_path, new_path, update_type, created_at
		 FROM deferred_itunes_updates WHERE applied_at IS NULL ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DeferredITunesUpdate
	for rows.Next() {
		var d DeferredITunesUpdate
		var createdAtStr string
		if err := rows.Scan(&d.ID, &d.BookID, &d.PersistentID, &d.OldPath, &d.NewPath, &d.UpdateType, &createdAtStr); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		results = append(results, d)
	}
	return results, rows.Err()
}

// MarkDeferredITunesUpdateApplied sets the applied_at timestamp on a deferred update.
func (s *SQLiteStore) MarkDeferredITunesUpdateApplied(id int) error {
	_, err := s.db.Exec(
		`UPDATE deferred_itunes_updates SET applied_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id,
	)
	return err
}

// GetDeferredITunesUpdatesByBookID returns all deferred updates for a specific book.
func (s *SQLiteStore) GetDeferredITunesUpdatesByBookID(bookID string) ([]DeferredITunesUpdate, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, persistent_id, old_path, new_path, update_type, created_at, applied_at
		 FROM deferred_itunes_updates WHERE book_id = ? ORDER BY created_at ASC`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DeferredITunesUpdate
	for rows.Next() {
		var d DeferredITunesUpdate
		var createdAtStr string
		var appliedAtStr *string
		if err := rows.Scan(&d.ID, &d.BookID, &d.PersistentID, &d.OldPath, &d.NewPath, &d.UpdateType, &createdAtStr, &appliedAtStr); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if appliedAtStr != nil {
			t, _ := time.Parse(time.RFC3339, *appliedAtStr)
			d.AppliedAt = &t
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// CreateExternalIDMapping creates or replaces an external ID mapping.
func (s *SQLiteStore) CreateExternalIDMapping(mapping *ExternalIDMapping) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO external_id_map (source, external_id, book_id, track_number, file_path, tombstoned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM external_id_map WHERE source = ? AND external_id = ?), ?), ?)`,
		mapping.Source, mapping.ExternalID, mapping.BookID, mapping.TrackNumber, mapping.FilePath,
		boolToInt(mapping.Tombstoned),
		mapping.Source, mapping.ExternalID, now, now,
	)
	return err
}

// GetBookByExternalID returns the book_id for a non-tombstoned external ID.
func (s *SQLiteStore) GetBookByExternalID(source, externalID string) (string, error) {
	var bookID string
	err := s.db.QueryRow(
		`SELECT book_id FROM external_id_map WHERE source = ? AND external_id = ? AND tombstoned = 0`,
		source, externalID,
	).Scan(&bookID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return bookID, err
}

// GetExternalIDsForBook returns all external ID mappings for a book.
func (s *SQLiteStore) GetExternalIDsForBook(bookID string) ([]ExternalIDMapping, error) {
	rows, err := s.db.Query(
		`SELECT id, source, external_id, book_id, track_number, file_path, tombstoned, created_at, updated_at
		 FROM external_id_map WHERE book_id = ? ORDER BY source, external_id`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ExternalIDMapping
	for rows.Next() {
		var m ExternalIDMapping
		var trackNumber sql.NullInt64
		var filePath sql.NullString
		var tombstoned int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&m.ID, &m.Source, &m.ExternalID, &m.BookID, &trackNumber, &filePath, &tombstoned, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		if trackNumber.Valid {
			tn := int(trackNumber.Int64)
			m.TrackNumber = &tn
		}
		if filePath.Valid {
			m.FilePath = filePath.String
		}
		m.Tombstoned = tombstoned != 0
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		m.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		results = append(results, m)
	}
	return results, rows.Err()
}

// IsExternalIDTombstoned checks whether an external ID is tombstoned.
func (s *SQLiteStore) IsExternalIDTombstoned(source, externalID string) (bool, error) {
	var tombstoned int
	err := s.db.QueryRow(
		`SELECT tombstoned FROM external_id_map WHERE source = ? AND external_id = ?`,
		source, externalID,
	).Scan(&tombstoned)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tombstoned != 0, nil
}

// TombstoneExternalID marks an external ID as tombstoned to prevent reimport.
func (s *SQLiteStore) TombstoneExternalID(source, externalID string) error {
	_, err := s.db.Exec(
		`UPDATE external_id_map SET tombstoned = 1, updated_at = ? WHERE source = ? AND external_id = ?`,
		time.Now().Format(time.RFC3339), source, externalID,
	)
	return err
}

// ReassignExternalIDs moves all external ID mappings from one book to another (for merges).
func (s *SQLiteStore) ReassignExternalIDs(oldBookID, newBookID string) error {
	_, err := s.db.Exec(
		`UPDATE external_id_map SET book_id = ?, updated_at = ? WHERE book_id = ?`,
		newBookID, time.Now().Format(time.RFC3339), oldBookID,
	)
	return err
}

// BulkCreateExternalIDMappings inserts multiple external ID mappings in a transaction.
// Uses INSERT OR IGNORE so existing mappings are not overwritten.
func (s *SQLiteStore) BulkCreateExternalIDMappings(mappings []ExternalIDMapping) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR IGNORE INTO external_id_map (source, external_id, book_id, track_number, file_path, tombstoned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Format(time.RFC3339)
	for _, m := range mappings {
		if _, err := stmt.Exec(m.Source, m.ExternalID, m.BookID, m.TrackNumber, m.FilePath, now, now); err != nil {
			return fmt.Errorf("failed to insert external ID mapping (%s/%s): %w", m.Source, m.ExternalID, err)
		}
	}

	return tx.Commit()
}

// --- User Tags (free-form labels on books) ---

// GetBookUserTags returns all user-defined tags for a book.
func (s *SQLiteStore) GetBookUserTags(bookID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM book_user_tags WHERE book_id = ? ORDER BY tag`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

// SetBookUserTags replaces all user-defined tags for a book.
func (s *SQLiteStore) SetBookUserTags(bookID string, tags []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM book_user_tags WHERE book_id = ?`, bookID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.Exec(`INSERT INTO book_user_tags (book_id, tag) VALUES (?, ?)`, bookID, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddBookUserTag adds a single user-defined tag to a book (idempotent).
func (s *SQLiteStore) AddBookUserTag(bookID string, tag string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO book_user_tags (book_id, tag) VALUES (?, ?)`, bookID, tag)
	return err
}

// RemoveBookUserTag removes a single user-defined tag from a book.
func (s *SQLiteStore) RemoveBookUserTag(bookID string, tag string) error {
	_, err := s.db.Exec(`DELETE FROM book_user_tags WHERE book_id = ? AND tag = ?`, bookID, tag)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Reset clears all data from all tables
func (s *SQLiteStore) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Dynamically discover all user tables from sqlite_master
	// This ensures we don't miss any tables if the schema evolves
	rows, err := tx.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table'
		AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return fmt.Errorf("failed to query table list: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate table list: %w", err)
	}

	// Delete all rows from each discovered table
	for _, table := range tables {
		// Use parameterized table name verification by checking it's in our discovered list
		// Table names from sqlite_master are safe, but we double-check format anyway
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			// Log but continue - some tables might have constraints or other issues
			// This is safe because table names come directly from sqlite_master metadata
			continue
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetBookVersions is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) GetBookVersions(id string, limit int) ([]BookVersion, error) {
	return nil, nil
}

// GetBookAtVersion is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) GetBookAtVersion(id string, ts time.Time) (*Book, error) {
	return nil, fmt.Errorf("book versioning not supported in SQLite store")
}

// RevertBookToVersion is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) RevertBookToVersion(id string, ts time.Time) (*Book, error) {
	return nil, fmt.Errorf("book versioning not supported in SQLite store")
}

// PruneBookVersions is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) PruneBookVersions(id string, keepCount int) (int, error) {
	return 0, nil
}

// Optimize runs PRAGMA optimize and VACUUM to compact and optimize the SQLite database.
func (s *SQLiteStore) Optimize() error {
	if _, err := s.db.Exec("PRAGMA analysis_limit=1000; PRAGMA optimize"); err != nil {
		return fmt.Errorf("PRAGMA optimize failed: %w", err)
	}
	if _, err := s.db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("VACUUM failed: %w", err)
	}
	return nil
}

// CreateOperationChange inserts a new operation change record.
func (s *SQLiteStore) CreateOperationChange(change *OperationChange) error {
	if change.ID == "" {
		change.ID = ulid.Make().String()
	}
	_, err := s.db.Exec(
		`INSERT INTO operation_changes (id, operation_id, book_id, change_type, field_name, old_value, new_value, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		change.ID, change.OperationID, change.BookID, change.ChangeType, change.FieldName, change.OldValue, change.NewValue,
	)
	return err
}

// GetOperationChanges returns all changes for a given operation.
func (s *SQLiteStore) GetOperationChanges(operationID string) ([]*OperationChange, error) {
	rows, err := s.db.Query(
		`SELECT id, operation_id, book_id, change_type, field_name, old_value, new_value, reverted_at, created_at
		 FROM operation_changes WHERE operation_id = ? ORDER BY created_at ASC`, operationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOperationChanges(rows)
}

// GetBookChanges returns all changes for a given book.
func (s *SQLiteStore) GetBookChanges(bookID string) ([]*OperationChange, error) {
	rows, err := s.db.Query(
		`SELECT id, operation_id, book_id, change_type, field_name, old_value, new_value, reverted_at, created_at
		 FROM operation_changes WHERE book_id = ? ORDER BY created_at DESC`, bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOperationChanges(rows)
}

// RevertOperationChanges marks all changes for an operation as reverted.
func (s *SQLiteStore) RevertOperationChanges(operationID string) error {
	_, err := s.db.Exec(
		`UPDATE operation_changes SET reverted_at = CURRENT_TIMESTAMP WHERE operation_id = ? AND reverted_at IS NULL`,
		operationID,
	)
	return err
}

// CreateAuthorTombstone is a no-op for SQLite (uses SQL foreign keys instead).
func (s *SQLiteStore) CreateAuthorTombstone(oldID, canonicalID int) error {
	return nil
}

// GetAuthorTombstone is a no-op for SQLite (uses SQL foreign keys instead).
func (s *SQLiteStore) GetAuthorTombstone(oldID int) (int, error) {
	return 0, nil
}

// ResolveTombstoneChains is a no-op for SQLite (uses SQL foreign keys instead).
func (s *SQLiteStore) ResolveTombstoneChains() (int, error) {
	return 0, nil
}

// AddSystemActivityLog inserts a log entry from a housekeeping goroutine.
func (s *SQLiteStore) AddSystemActivityLog(source, level, message string) error {
	_, err := s.db.Exec(
		"INSERT INTO system_activity_log (source, level, message) VALUES (?, ?, ?)",
		source, level, message,
	)
	return err
}

// GetSystemActivityLogs retrieves recent system activity log entries.
func (s *SQLiteStore) GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error) {
	query := "SELECT id, source, level, message, created_at FROM system_activity_log"
	args := []interface{}{}
	if source != "" {
		query += " WHERE source = ?"
		args = append(args, source)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []SystemActivityLog
	for rows.Next() {
		var l SystemActivityLog
		if err := rows.Scan(&l.ID, &l.Source, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// PruneOperationLogs deletes operation log entries older than the given time.
func (s *SQLiteStore) PruneOperationLogs(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM operation_logs WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// PruneOperationChanges deletes operation change entries older than the given time.
func (s *SQLiteStore) PruneOperationChanges(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM operation_changes WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// PruneSystemActivityLogs deletes system activity log entries older than the given time.
func (s *SQLiteStore) PruneSystemActivityLogs(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM system_activity_log WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// GetScanCacheMap returns a map of file_path -> ScanCacheEntry for all books
// that have a non-empty file_path and a non-NULL last_scan_mtime.
func (s *SQLiteStore) GetScanCacheMap() (map[string]ScanCacheEntry, error) {
	rows, err := s.db.Query(
		`SELECT file_path, last_scan_mtime, last_scan_size, needs_rescan
		 FROM books WHERE file_path != '' AND last_scan_mtime IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]ScanCacheEntry)
	for rows.Next() {
		var path string
		var mtime, size sql.NullInt64
		var needsRescan sql.NullBool
		if err := rows.Scan(&path, &mtime, &size, &needsRescan); err != nil {
			return nil, err
		}
		entry := ScanCacheEntry{}
		if mtime.Valid {
			entry.Mtime = mtime.Int64
		}
		if size.Valid {
			entry.Size = size.Int64
		}
		if needsRescan.Valid {
			entry.NeedsRescan = needsRescan.Bool
		}
		result[path] = entry
	}
	return result, rows.Err()
}

// UpdateScanCache sets last_scan_mtime, last_scan_size, and clears needs_rescan for a book.
func (s *SQLiteStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	_, err := s.db.Exec(
		`UPDATE books SET last_scan_mtime = ?, last_scan_size = ?, needs_rescan = 0 WHERE id = ?`,
		mtime, size, bookID,
	)
	return err
}

// MarkNeedsRescan sets needs_rescan = 1 for the given book.
func (s *SQLiteStore) MarkNeedsRescan(bookID string) error {
	_, err := s.db.Exec(
		`UPDATE books SET needs_rescan = 1 WHERE id = ?`,
		bookID,
	)
	return err
}

// GetDirtyBookFolders returns a deduplicated list of parent directories for all
// books that have needs_rescan = 1.
func (s *SQLiteStore) GetDirtyBookFolders() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT file_path FROM books WHERE needs_rescan = 1 AND file_path != ''`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var dirs []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		dir := filepath.Dir(path)
		if _, ok := seen[dir]; !ok {
			seen[dir] = struct{}{}
			dirs = append(dirs, dir)
		}
	}
	return dirs, rows.Err()
}

// RecordPathChange inserts a path change record into book_path_history.
func (s *SQLiteStore) RecordPathChange(change *BookPathChange) error {
	_, err := s.db.Exec(
		`INSERT INTO book_path_history (book_id, old_path, new_path, change_type) VALUES (?, ?, ?, ?)`,
		change.BookID, change.OldPath, change.NewPath, change.ChangeType,
	)
	return err
}

// GetBookPathHistory returns all path changes for a book, newest first.
func (s *SQLiteStore) GetBookPathHistory(bookID string) ([]BookPathChange, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, old_path, new_path, change_type, created_at
		 FROM book_path_history WHERE book_id = ? ORDER BY created_at DESC`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BookPathChange
	for rows.Next() {
		var c BookPathChange
		var createdAtStr string
		if err := rows.Scan(&c.ID, &c.BookID, &c.OldPath, &c.NewPath, &c.ChangeType, &createdAtStr); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		results = append(results, c)
	}
	return results, rows.Err()
}

// AddBookTag adds a tag to a book (idempotent — no error if already exists).
func (s *SQLiteStore) AddBookTag(bookID, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO book_tags (book_id, tag) VALUES (?, ?)`,
		bookID, tag,
	)
	return err
}

// RemoveBookTag removes a tag from a book.
func (s *SQLiteStore) RemoveBookTag(bookID, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	_, err := s.db.Exec(
		`DELETE FROM book_tags WHERE book_id = ? AND tag = ?`,
		bookID, tag,
	)
	return err
}

// GetBookTags returns all tags for a book, sorted alphabetically.
func (s *SQLiteStore) GetBookTags(bookID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT tag FROM book_tags WHERE book_id = ? ORDER BY tag`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// SetBookTags replaces all tags on a book with the given set.
func (s *SQLiteStore) SetBookTags(bookID string, tags []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Remove all existing tags
	if _, err := tx.Exec(`DELETE FROM book_tags WHERE book_id = ?`, bookID); err != nil {
		return err
	}

	// Insert new tags
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO book_tags (book_id, tag) VALUES (?, ?)`,
			bookID, tag,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListAllTags returns all unique tags with their usage counts.
func (s *SQLiteStore) ListAllTags() ([]TagWithCount, error) {
	rows, err := s.db.Query(
		`SELECT tag, COUNT(*) as count FROM book_tags GROUP BY tag ORDER BY tag`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TagWithCount
	for rows.Next() {
		var tc TagWithCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// GetBooksByTag returns all book IDs that have the given tag.
func (s *SQLiteStore) GetBooksByTag(tag string) ([]string, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}

	rows, err := s.db.Query(
		`SELECT book_id FROM book_tags WHERE tag = ?`,
		tag,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		bookIDs = append(bookIDs, id)
	}
	return bookIDs, rows.Err()
}

func scanOperationChanges(rows *sql.Rows) ([]*OperationChange, error) {
	var changes []*OperationChange
	for rows.Next() {
		c := &OperationChange{}
		if err := rows.Scan(&c.ID, &c.OperationID, &c.BookID, &c.ChangeType, &c.FieldName, &c.OldValue, &c.NewValue, &c.RevertedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}
