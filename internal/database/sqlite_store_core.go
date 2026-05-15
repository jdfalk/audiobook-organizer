// file: internal/database/sqlite_store_core.go
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8901-bcde-f23456789012
// last-edited: 2026-05-01

package database

import (
"database/sql"
"fmt"
"log/slog"
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
work_id, narrator, edition, description, language, publisher, genre,
print_year, audiobook_release_year, isbn10, isbn13, asin,
open_library_id, hardcover_id, google_books_id,
itunes_persistent_id, itunes_date_added, itunes_play_count,
itunes_last_played, itunes_rating, itunes_bookmark, itunes_import_source, itunes_path,
file_hash, file_size, bitrate_kbps, codec, sample_rate_hz, channels,
bit_depth, quality, is_primary_version, version_group_id, version_notes,
original_file_hash, organized_file_hash, library_state, quantity,
marked_for_deletion, marked_for_deletion_at, created_at, updated_at,
metadata_updated_at, last_written_at, metadata_review_status, cover_url, narrators_json,
last_organize_operation_id, last_organized_at, itunes_sync_status,
quarantine_reason, quarantined_at,
audible_rating_overall, audible_rating_performance, audible_rating_story,
audible_rating_count, audible_num_reviews,
google_rating_average, google_rating_count,
user_rating_overall, user_rating_story, user_rating_performance, user_rating_notes,
metadata_source_hash,
merged_into_book_id
`

// bookSelectColumnsQualified prefixes all columns with "books." for use in JOINs.
const bookSelectColumnsQualified = `
books.id, books.title, books.author_id, books.series_id, books.series_sequence,
books.file_path, books.original_filename, books.format, books.duration,
books.work_id, books.narrator, books.edition, books.description, books.language, books.publisher, books.genre,
books.print_year, books.audiobook_release_year, books.isbn10, books.isbn13, books.asin,
books.open_library_id, books.hardcover_id, books.google_books_id,
books.itunes_persistent_id, books.itunes_date_added, books.itunes_play_count,
books.itunes_last_played, books.itunes_rating, books.itunes_bookmark, books.itunes_import_source, books.itunes_path,
books.file_hash, books.file_size, books.bitrate_kbps, books.codec, books.sample_rate_hz, books.channels,
books.bit_depth, books.quality, books.is_primary_version, books.version_group_id, books.version_notes,
books.original_file_hash, books.organized_file_hash, books.library_state, books.quantity,
books.marked_for_deletion, books.marked_for_deletion_at, books.created_at, books.updated_at,
books.metadata_updated_at, books.last_written_at, books.metadata_review_status, books.cover_url, books.narrators_json,
books.last_organize_operation_id, books.last_organized_at, books.itunes_sync_status,
books.quarantine_reason, books.quarantined_at,
books.audible_rating_overall, books.audible_rating_performance, books.audible_rating_story,
books.audible_rating_count, books.audible_num_reviews,
books.google_rating_average, books.google_rating_count,
books.user_rating_overall, books.user_rating_story, books.user_rating_performance, books.user_rating_notes,
books.metadata_source_hash,
books.merged_into_book_id
`

// bookSummarySelectColumns defines the minimal set of columns needed for BookSummary
// (excludes heavy fields like description, cover image data, and embeddings)
const bookSummarySelectColumns = `
id, title, author_id, series_id, series_sequence,
file_path, format, duration, original_filename,
file_hash, file_size, original_file_hash, organized_file_hash,
library_state, quarantined_at, quarantine_reason, cover_url, narrator,
created_at, updated_at, metadata_updated_at,
is_primary_version, version_group_id, metadata_review_status
`

const bookFileCols = `id, book_id, file_path, original_filename, itunes_path, itunes_persistent_id,
track_number, track_count, disc_number, disc_count, title, format, codec, duration,
file_size, bitrate_kbps, sample_rate_hz, channels, bit_depth, file_hash, original_file_hash,
post_metadata_hash,
acoustid_seg0, acoustid_seg1, acoustid_seg2, acoustid_seg3, acoustid_seg4, acoustid_seg5, acoustid_seg6,
missing, created_at, updated_at,
deluge_hash, deluge_original_path, imported_from_deluge_at,
fingerprint_failed_at, organize_method,
fingerprint_failure_reason, fingerprint_failure_detail, fingerprint_diagnostic_json`

type SQLiteStore struct {
	db      *sql.DB
	rootDir string
}

func (s *SQLiteStore) SetRootDir(rootDir string) { s.rootDir = rootDir }
func (s *SQLiteStore) InvalidateLibraryStats()   {} // SQLite uses SQL aggregation; no persistent cache to clear

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
		slog.Warn("series deduplication failed", "error", err)
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
		itunes_path TEXT,
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
		last_organize_operation_id TEXT,
		last_organized_at DATETIME,
		itunes_sync_status TEXT,
		quarantine_reason TEXT,
		quarantined_at TIMESTAMP,
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
	if err := s.ensureExtendedBookColumns(); err != nil {
		return err
	}
	return s.ensureExtendedBookFileColumns()
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
			if _, err := s.db.Exec("UPDATE works SET series_id = ? WHERE series_id IN (SELECT id FROM series WHERE LOWER(name) = LOWER(?) AND author_id IS NULL AND id != ?)", keepID, lname, keepID); err != nil {
				return fmt.Errorf("failed to reassign works for series %q: %w", lname, err)
			}
			if _, err := s.db.Exec(deleteQuery, lname, keepID); err != nil {
				return fmt.Errorf("failed to delete duplicate series %q: %w", lname, err)
			}
		} else {
			if _, err := s.db.Exec(updateQuery, keepID, lname, aid, keepID); err != nil {
				return fmt.Errorf("failed to reassign books for series %q: %w", lname, err)
			}
			if _, err := s.db.Exec("UPDATE works SET series_id = ? WHERE series_id IN (SELECT id FROM series WHERE LOWER(name) = LOWER(?) AND author_id = ? AND id != ?)", keepID, lname, aid, keepID); err != nil {
				return fmt.Errorf("failed to reassign works for series %q: %w", lname, err)
			}
			if _, err := s.db.Exec(deleteQuery, lname, aid, keepID); err != nil {
				return fmt.Errorf("failed to delete duplicate series %q: %w", lname, err)
			}
		}
		totalMerged += cnt - 1
	}

	if totalMerged > 0 {
		slog.Info("series deduplication complete", "merged", totalMerged)
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
		"cover_url":                    "TEXT",
		"narrators_json":               "TEXT",
		"audible_rating_overall":       "REAL",
		"audible_rating_performance":   "REAL",
		"audible_rating_story":         "REAL",
		"audible_rating_count":         "INTEGER",
		"audible_num_reviews":          "INTEGER",
		"google_rating_average":        "REAL",
		"google_rating_count":          "INTEGER",
		"user_rating_overall":          "REAL",
		"user_rating_story":            "REAL",
		"user_rating_performance":      "REAL",
		"user_rating_notes":            "TEXT",
		"metadata_source_hash":         "TEXT",
		"merged_into_book_id":          "TEXT",
		"source_import_path":           "TEXT",
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
		"CREATE INDEX IF NOT EXISTS idx_books_source_import_path ON books(source_import_path)",
	}
	for _, stmt := range indexStatements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed creating index with statement %s: %w", stmt, err)
		}
	}
	return nil
}

// ensureExtendedBookFileColumns adds newly introduced optional columns to the
// book_files table for existing databases created before these columns existed.
// SQLite lacks IF NOT EXISTS for ADD COLUMN, so we inspect PRAGMA table_info
// and conditionally ALTER TABLE.
func (s *SQLiteStore) ensureExtendedBookFileColumns() error {
	columns := map[string]string{
		"deluge_hash":             "TEXT",
		"deluge_original_path":    "TEXT",
		"imported_from_deluge_at": "TIMESTAMP",
	}

	rows, err := s.db.Query("PRAGMA table_info(book_files)")
	if err != nil {
		return fmt.Errorf("failed to inspect book_files schema: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan book_files table_info: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating book_files table_info: %w", err)
	}

	// If the table does not exist yet (created by a later migration), skip.
	if len(existing) == 0 {
		return nil
	}

	for name, colType := range columns {
		if _, ok := existing[name]; ok {
			continue
		}
		alter := fmt.Sprintf("ALTER TABLE book_files ADD COLUMN %s %s", name, colType)
		if _, err := s.db.Exec(alter); err != nil {
			return fmt.Errorf("failed adding column %s to book_files: %w", name, err)
		}
	}
	return nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

