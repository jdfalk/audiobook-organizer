package database

// version: 1.0.0
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
			normalized_name TEXT NOT NULL,
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
			normalized_name TEXT NOT NULL,
			author_id INTEGER,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			marked_for_deletion BOOLEAN DEFAULT false
		)
	`
	_, err := cs.db.ExecContext(ctx, query)
	return err
}

// createBooksTable creates the books table
func (cs *ChaiSchema) createBooksTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS books (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			normalized_title TEXT,
			series_id INTEGER,
			series_sequence INTEGER,
			is_primary_version BOOLEAN DEFAULT true,
			created_at TIMESTAMP,
			updated_at TIMESTAMP,
			marked_for_deletion BOOLEAN DEFAULT false
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
	indexes := []string{
		// Authors indexes
		`CREATE INDEX IF NOT EXISTS idx_authors_normalized_name ON authors(normalized_name)`,

		// Series indexes
		`CREATE INDEX IF NOT EXISTS idx_series_author_id ON series(author_id)`,
		`CREATE INDEX IF NOT EXISTS idx_series_normalized_name ON series(normalized_name)`,

		// Books indexes
		`CREATE INDEX IF NOT EXISTS idx_books_series_id ON books(series_id)`,
		`CREATE INDEX IF NOT EXISTS idx_books_is_primary_version ON books(is_primary_version)`,
		`CREATE INDEX IF NOT EXISTS idx_books_marked_for_deletion ON books(marked_for_deletion)`,

		// Book files indexes
		`CREATE INDEX IF NOT EXISTS idx_book_files_book_id ON book_files(book_id)`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_format ON book_files(format)`,

		// Book authors indexes
		`CREATE INDEX IF NOT EXISTS idx_book_authors_book_id ON book_authors(book_id)`,
		`CREATE INDEX IF NOT EXISTS idx_book_authors_author_id ON book_authors(author_id)`,
	}

	for _, indexSQL := range indexes {
		if _, err := cs.db.ExecContext(ctx, indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// DropAllTables drops all tables (for testing/cleanup)
// WARNING: This is destructive and should only be used in test scenarios
func (cs *ChaiSchema) DropAllTables(ctx context.Context) error {
	tables := []string{
		"book_authors",
		"book_files",
		"books",
		"series",
		"authors",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s", table)
		if _, err := cs.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	return nil
}
