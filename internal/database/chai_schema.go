package database

// version: 1.2.0
// guid: a1b2c3d4-e5f6-47g8-h9i0-j1k2l3m4n5o6
// last-edited: 2026-05-24

import (
	"context"
	"database/sql"
	"fmt"
)

// ChaiSchema contains the SQL schema for Chai database initialization
type ChaiSchema struct {
	db *sql.DB
}

// NewChaiSchema creates a new schema helper for the given database
func NewChaiSchema(db *sql.DB) *ChaiSchema {
	return &ChaiSchema{db: db}
}

// InitializeSchema creates all necessary tables and indexes
// This is idempotent - can be called multiple times without error
// Uses simplified schema without CURRENT_TIMESTAMP defaults (not supported in Chai)
func (cs *ChaiSchema) InitializeSchema(ctx context.Context) error {
	// Create authors table
	if err := cs.createAuthorsTable(ctx); err != nil {
		return fmt.Errorf("failed to create authors table: %w", err)
	}

	// Create series table
	if err := cs.createSeriesTable(ctx); err != nil {
		return fmt.Errorf("failed to create series table: %w", err)
	}

	// Create books table
	if err := cs.createBooksTable(ctx); err != nil {
		return fmt.Errorf("failed to create books table: %w", err)
	}

	// Create book_files table (for file segments)
	if err := cs.createBookFilesTable(ctx); err != nil {
		return fmt.Errorf("failed to create book_files table: %w", err)
	}

	// Create book_authors junction table
	if err := cs.createBookAuthorsTable(ctx); err != nil {
		return fmt.Errorf("failed to create book_authors table: %w", err)
	}

	// Create indexes for performance
	if err := cs.createIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

// createAuthorsTable creates the authors table
func (cs *ChaiSchema) createAuthorsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS authors (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			marked_for_deletion BOOLEAN DEFAULT false
		)
	`
	_, err := cs.db.ExecContext(ctx, query)
	return err
}

// createSeriesTable creates the series table
func (cs *ChaiSchema) createSeriesTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS series (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			author_id INTEGER,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			marked_for_deletion BOOLEAN DEFAULT false
		)
	`
	_, err := cs.db.ExecContext(ctx, query)
	return err
}

// createBooksTable creates the books table with all fields needed for GetAllBooks_Chai
func (cs *ChaiSchema) createBooksTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS books (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			author_id INTEGER,
			series_id INTEGER,
			series_sequence INTEGER,
			file_path TEXT,
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
			itunes_play_count INTEGER,
			itunes_last_played TIMESTAMP,
			itunes_rating INTEGER,
			itunes_bookmark INTEGER,
			itunes_import_source TEXT,
			itunes_path TEXT,
			original_filename TEXT,
			bitrate_kbps INTEGER,
			codec TEXT,
			sample_rate_hz INTEGER,
			channels INTEGER,
			bit_depth INTEGER,
			quality TEXT,
			is_primary_version BOOLEAN DEFAULT true,
			version_group_id TEXT,
			version_notes TEXT,
			file_hash TEXT,
			file_size INTEGER,
			original_file_hash TEXT,
			organized_file_hash TEXT,
			library_state TEXT,
			quantity INTEGER,
			marked_for_deletion BOOLEAN DEFAULT false,
			marked_for_deletion_at TIMESTAMP,
			quarantine_reason TEXT,
			quarantined_at TIMESTAMP,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			metadata_updated_at TIMESTAMP,
			last_written_at TIMESTAMP,
			last_organize_operation_id TEXT,
			last_organized_at TIMESTAMP,
			metadata_review_status TEXT,
			metadata_source TEXT,
			book_sig_v1 TEXT,
			book_sig_segments TEXT,
			book_sig_built_at TIMESTAMP,
			book_sig_v1_mask TEXT,
			book_sig_coverage_pct INTEGER,
			itunes_sync_status TEXT,
			audible_runtime_min INTEGER,
			metadata_source_hash TEXT,
			merged_into_book_id TEXT,
			audible_rating_overall REAL,
			audible_rating_performance REAL,
			audible_rating_story REAL,
			audible_rating_count INTEGER,
			audible_num_reviews INTEGER,
			google_rating_average REAL,
			google_rating_count INTEGER,
			user_rating_overall REAL,
			user_rating_story REAL,
			user_rating_performance REAL,
			user_rating_notes TEXT,
			cover_url TEXT,
			narrators_json TEXT,
			source_import_path TEXT,
			last_scan_mtime INTEGER,
			last_scan_size INTEGER,
			needs_rescan BOOLEAN
		)
	`
	_, err := cs.db.ExecContext(ctx, query)
	return err
}

// createBookFilesTable creates the book_files table for file segments
func (cs *ChaiSchema) createBookFilesTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS book_files (
			id TEXT PRIMARY KEY,
			book_id TEXT NOT NULL,
			file_path TEXT NOT NULL,
			format TEXT,
			duration_ms INTEGER,
			file_size_bytes INTEGER,
			file_hash TEXT,
			missing BOOLEAN DEFAULT false,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			marked_for_deletion BOOLEAN DEFAULT false
		)
	`
	_, err := cs.db.ExecContext(ctx, query)
	return err
}

// createBookAuthorsTable creates the book_authors junction table
func (cs *ChaiSchema) createBookAuthorsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS book_authors (
			id TEXT PRIMARY KEY,
			book_id TEXT NOT NULL,
			author_id INTEGER NOT NULL,
			role TEXT DEFAULT 'author',
			position INTEGER DEFAULT 0,
			created_at TIMESTAMP,
			marked_for_deletion BOOLEAN DEFAULT false
		)
	`
	_, err := cs.db.ExecContext(ctx, query)
	return err
}

// createIndexes creates all necessary indexes for query performance
func (cs *ChaiSchema) createIndexes(ctx context.Context) error {
	indexes := map[string]string{
		"idx_books_series_id": "CREATE INDEX IF NOT EXISTS idx_books_series_id ON books(series_id)",
		"idx_books_author_id": "CREATE INDEX IF NOT EXISTS idx_books_author_id ON books(author_id)",
		"idx_books_is_primary": "CREATE INDEX IF NOT EXISTS idx_books_is_primary ON books(is_primary_version)",
		"idx_books_marked_del": "CREATE INDEX IF NOT EXISTS idx_books_marked_del ON books(marked_for_deletion)",
		"idx_book_files_book_id": "CREATE INDEX IF NOT EXISTS idx_book_files_book_id ON book_files(book_id)",
		"idx_book_authors_author_id": "CREATE INDEX IF NOT EXISTS idx_book_authors_author_id ON book_authors(author_id)",
		"idx_book_authors_book_id": "CREATE INDEX IF NOT EXISTS idx_book_authors_book_id ON book_authors(book_id)",
	}

	for _, stmt := range indexes {
		if _, err := cs.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// DropAllTables drops all tables (for testing)
func (cs *ChaiSchema) DropAllTables(ctx context.Context) error {
	tables := []string{
		"book_authors",
		"book_files",
		"books",
		"series",
		"authors",
	}
	
	for _, table := range tables {
		_, err := cs.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", table))
		if err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}
	
	return nil
}
