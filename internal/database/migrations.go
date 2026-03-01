// file: internal/database/migrations.go
// version: 1.16.0
// guid: 9a8b7c6d-5e4f-3d2c-1b0a-9f8e7d6c5b4a

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// MigrationFunc represents a migration operation
type MigrationFunc func(store Store) error

// Migration represents a single database migration
type Migration struct {
	Version     int
	Description string
	Up          MigrationFunc
	Down        MigrationFunc // Optional rollback
}

// MigrationRecord tracks applied migrations
type MigrationRecord struct {
	Version     int       `json:"version"`
	Description string    `json:"description"`
	AppliedAt   time.Time `json:"applied_at"`
}

// DatabaseVersion stores the current schema version
type DatabaseVersion struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// migrations is the ordered list of all migrations
var migrations = []Migration{
	{
		Version:     1,
		Description: "Initial schema with authors, series, books, playlists",
		Up:          migration001Up,
		Down:        nil,
	},
	{
		Version:     2,
		Description: "Add import paths and operations tables",
		Up:          migration002Up,
		Down:        nil,
	},
	{
		Version:     3,
		Description: "Add user preferences",
		Up:          migration003Up,
		Down:        nil,
	},
	{
		Version:     4,
		Description: "Add extended Pebble keyspace (users, sessions, segments, playback)",
		Up:          migration004Up,
		Down:        nil,
	},
	{
		Version:     5,
		Description: "Add media info and version management fields to books",
		Up:          migration005Up,
		Down:        nil,
	},
	{
		Version:     6,
		Description: "Add original and organized file hash tracking",
		Up:          migration006Up,
		Down:        nil,
	},
	{
		Version:     7,
		Description: "Rename import paths to import paths",
		Up:          migration007Up,
		Down:        nil,
	},
	{
		Version:     8,
		Description: "Add do_not_import table for hash blocklist",
		Up:          migration008Up,
		Down:        nil,
	},
	{
		Version:     9,
		Description: "Add state machine and lifecycle fields to books",
		Up:          migration009Up,
		Down:        nil,
	},
	{
		Version:     10,
		Description: "Add metadata_states table for metadata provenance",
		Up:          migration010Up,
		Down:        nil,
	},
	{
		Version:     11,
		Description: "Add iTunes import metadata fields to books",
		Up:          migration011Up,
		Down:        nil,
	},
	{
		Version:     12,
		Description: "Add created_at and updated_at timestamps to books table",
		Up:          migration012Up,
		Down:        nil,
	},
	{
		Version:     13,
		Description: "Add wanted state support and multi-path tracking",
		Up:          migration013Up,
		Down:        nil,
	},
	{
		Version:     14,
		Description: "Flag books with corrupted organize paths for review",
		Up:          migration014Up,
		Down:        nil,
	},
	{
		Version:     15,
		Description: "Add book_authors junction table, cover_url, and narrators_json",
		Up:          migration015Up,
		Down:        nil,
	},
	{
		Version:     16,
		Description: "Add users, sessions, book_segments, playback tables",
		Up:          migration016Up,
		Down:        nil,
	},
	{
		Version:     17,
		Description: "Add composite indexes and FTS5 full-text search",
		Up:          migration017Up,
		Down:        nil,
	},
	{
		Version:     18,
		Description: "Add itunes_library_state table for change detection",
		Up:          migration018Up,
		Down:        nil,
	},
	{
		Version:     19,
		Description: "Add metadata_changes_history table for undo support",
		Up:          migration019Up,
		Down:        nil,
	},
	{
		Version:     20,
		Description: "Add narrators and book_narrators tables",
		Up:          migration020Up,
		Down:        nil,
	},
	{
		Version:     21,
		Description: "Add operation_summary_logs table for persistent operation history",
		Up:          migration021Up,
		Down:        nil,
	},
	{
		Version:     22,
		Description: "Backfill book_authors by splitting '&'-joined author names; backfill book_narrators from legacy narrator field",
		Up:          migration022Up,
		Down:        nil,
	},
	{
		Version:     23,
		Description: "Add metadata_updated_at and last_written_at timestamp columns to books",
		Up:          migration023Up,
		Down:        nil,
	},
	{
		Version:     24,
		Description: "Add metadata_review_status column to books",
		Up:          migration024Up,
		Down:        nil,
	},
}

// RunMigrations applies all pending migrations
func RunMigrations(store Store) error {
	currentVersion, err := getCurrentVersion(store)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	log.Printf("Current database version: %d", currentVersion)

	// Find migrations to apply
	pendingMigrations := []Migration{}
	for _, m := range migrations {
		if m.Version > currentVersion {
			pendingMigrations = append(pendingMigrations, m)
		}
	}

	if len(pendingMigrations) == 0 {
		log.Printf("Database is up to date (version %d)", currentVersion)
		return nil
	}

	log.Printf("Applying %d migrations...", len(pendingMigrations))

	// Apply each migration
	for _, m := range pendingMigrations {
		log.Printf("Applying migration %d: %s", m.Version, m.Description)

		if err := m.Up(store); err != nil {
			return fmt.Errorf("migration %d failed: %w", m.Version, err)
		}

		// Record migration
		if err := recordMigration(store, m); err != nil {
			return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
		}

		// Update version
		if err := setVersion(store, m.Version); err != nil {
			return fmt.Errorf("failed to update version to %d: %w", m.Version, err)
		}

		log.Printf("Migration %d completed successfully", m.Version)
	}

	log.Printf("All migrations completed. Current version: %d", pendingMigrations[len(pendingMigrations)-1].Version)
	return nil
}

// getCurrentVersion retrieves the current schema version
func getCurrentVersion(store Store) (int, error) {
	// Try to get version from preferences
	pref, err := store.GetUserPreference("db_version")
	if err != nil || pref == nil || pref.Value == nil {
		// No version found, assume version 0 (fresh database)
		return 0, nil
	}

	var version DatabaseVersion
	if err := json.Unmarshal([]byte(*pref.Value), &version); err != nil {
		return 0, fmt.Errorf("failed to parse version: %w", err)
	}

	return version.Version, nil
}

// setVersion updates the current schema version
func setVersion(store Store, version int) error {
	dbVersion := DatabaseVersion{
		Version:   version,
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(dbVersion)
	if err != nil {
		return fmt.Errorf("failed to marshal version: %w", err)
	}

	return store.SetUserPreference("db_version", string(data))
}

// recordMigration stores a migration record
func recordMigration(store Store, m Migration) error {
	record := MigrationRecord{
		Version:     m.Version,
		Description: m.Description,
		AppliedAt:   time.Now(),
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal migration record: %w", err)
	}

	key := fmt.Sprintf("migration_%d", m.Version)
	return store.SetUserPreference(key, string(data))
}

// Migration implementations

// migration001Up initializes the basic schema
func migration001Up(store Store) error {
	// Basic schema is created automatically by store initialization
	// This migration just validates the structure exists
	log.Println("  - Validating basic schema (authors, series, books, playlists)")
	return nil
}

// migration002Up adds import paths and operations support
func migration002Up(store Store) error {
	// These structures are already supported by the current store interface
	log.Println("  - Adding import paths and operations support")
	return nil
}

// migration003Up adds user preferences support
func migration003Up(store Store) error {
	// User preferences already supported by current interface
	log.Println("  - Adding user preferences support")
	return nil
}

// migration004Up adds extended Pebble keyspace
func migration004Up(store Store) error {
	// Extended keyspace (users, sessions, segments, playback) already supported
	log.Println("  - Adding extended Pebble keyspace (users, sessions, segments, playback)")
	return nil
}

// migration005Up adds media info and version management fields to books table
func migration005Up(store Store) error {
	log.Println("  - Adding media info and version management fields to books table")

	// Check if this is a SQLite store (we need direct SQL access for ALTER TABLE)
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		// For non-SQLite stores, these fields are handled by the store implementation
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Add media info fields
	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN bitrate_kbps INTEGER",
		"ALTER TABLE books ADD COLUMN codec TEXT",
		"ALTER TABLE books ADD COLUMN sample_rate_hz INTEGER",
		"ALTER TABLE books ADD COLUMN channels INTEGER",
		"ALTER TABLE books ADD COLUMN bit_depth INTEGER",
		"ALTER TABLE books ADD COLUMN quality TEXT",
		// Add version management fields
		"ALTER TABLE books ADD COLUMN is_primary_version BOOLEAN DEFAULT 1",
		"ALTER TABLE books ADD COLUMN version_group_id TEXT",
		"ALTER TABLE books ADD COLUMN version_notes TEXT",
	}

	for _, stmt := range alterStatements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			// Check if column already exists (this is not an error)
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	// Create indices for version management
	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_version_group ON books(version_group_id)",
		"CREATE INDEX IF NOT EXISTS idx_books_is_primary ON books(is_primary_version)",
	}

	for _, stmt := range indexStatements {
		log.Printf("    - Creating index: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Println("  - Media info and version management fields added successfully")
	return nil
}

// migration006Up adds original and organized file hash tracking columns
func migration006Up(store Store) error {
	log.Println("  - Adding original/organized file hash columns to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN original_file_hash TEXT",
		"ALTER TABLE books ADD COLUMN organized_file_hash TEXT",
	}

	for _, stmt := range alterStatements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_original_hash ON books(original_file_hash)",
		"CREATE INDEX IF NOT EXISTS idx_books_organized_hash ON books(organized_file_hash)",
	}

	for _, stmt := range indexStatements {
		log.Printf("    - Creating index: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Backfill original hash so future duplicate detection works immediately
	if _, err := sqliteStore.db.Exec("UPDATE books SET original_file_hash = file_hash WHERE original_file_hash IS NULL AND file_hash IS NOT NULL"); err != nil {
		return fmt.Errorf("failed to backfill original_file_hash: %w", err)
	}

	return nil
}

// migration007Up renames library folder entities to import paths across backends.
func migration007Up(store Store) error {
	log.Println("  - Renaming import paths to import paths")

	switch s := store.(type) {
	case *PebbleStore:
		if err := s.migrateImportPathKeys(); err != nil {
			return fmt.Errorf("failed to migrate Pebble import path keys: %w", err)
		}
	case *SQLiteStore:
		var tableName string
		if err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='library_folders'`).Scan(&tableName); err != nil {
			if err == sql.ErrNoRows {
				// Already renamed or never created.
				return nil
			}
			return fmt.Errorf("failed to check legacy library_folders table: %w", err)
		}

		statements := []string{
			"ALTER TABLE library_folders RENAME TO import_paths",
			"DROP INDEX IF EXISTS idx_library_folders_path",
			"CREATE INDEX IF NOT EXISTS idx_import_paths_path ON import_paths(path)",
		}

		for _, stmt := range statements {
			log.Printf("    - Executing: %s", stmt)
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to execute '%s': %w", stmt, err)
			}
		}
	default:
		log.Println("  - Unknown store type; skipping migration")
	}

	return nil
}

func migration008Up(store Store) error {
	log.Println("  - Adding do_not_import table for hash blocklist")

	switch s := store.(type) {
	case *SQLiteStore:
		statements := []string{
			`CREATE TABLE IF NOT EXISTS do_not_import (
				hash TEXT PRIMARY KEY NOT NULL,
				reason TEXT NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			"CREATE INDEX IF NOT EXISTS idx_do_not_import_hash ON do_not_import(hash)",
		}

		for _, stmt := range statements {
			log.Printf("    - Executing: %s", stmt)
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to execute '%s': %w", stmt, err)
			}
		}
	case *PebbleStore:
		// For PebbleDB, we just need to log that the keyspace is available
		// No schema changes needed for Pebble
		log.Println("    - Pebble keyspace for do_not_import enabled")
	default:
		log.Println("  - Unknown store type; skipping migration")
	}

	return nil
}

// migration009Up adds state machine and lifecycle tracking fields to books table
func migration009Up(store Store) error {
	log.Println("  - Adding state machine and lifecycle fields to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN library_state TEXT DEFAULT 'imported'",
		"ALTER TABLE books ADD COLUMN quantity INTEGER DEFAULT 1",
		"ALTER TABLE books ADD COLUMN marked_for_deletion BOOLEAN DEFAULT 0",
		"ALTER TABLE books ADD COLUMN marked_for_deletion_at DATETIME",
	}

	for _, stmt := range alterStatements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	// Create index for state queries
	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_library_state ON books(library_state)",
		"CREATE INDEX IF NOT EXISTS idx_books_marked_for_deletion ON books(marked_for_deletion)",
	}

	for _, stmt := range indexStatements {
		log.Printf("    - Creating index: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Println("  - State machine fields added successfully")
	return nil
}

// migration010Up adds metadata_states table for persisted metadata provenance
func migration010Up(store Store) error {
	log.Println("  - Adding metadata_states table for metadata provenance")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS metadata_states (
			book_id TEXT NOT NULL,
			field TEXT NOT NULL,
			fetched_value TEXT,
			override_value TEXT,
			override_locked BOOLEAN NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (book_id, field)
		)`,
		"CREATE INDEX IF NOT EXISTS idx_metadata_states_book ON metadata_states(book_id)",
	}

	for _, stmt := range statements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute '%s': %w", stmt, err)
		}
	}

	return nil
}

// migration011Up adds iTunes import metadata fields to books table.
func migration011Up(store Store) error {
	log.Println("  - Adding iTunes import metadata fields to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN itunes_persistent_id TEXT",
		"ALTER TABLE books ADD COLUMN itunes_date_added TIMESTAMP",
		"ALTER TABLE books ADD COLUMN itunes_play_count INTEGER DEFAULT 0",
		"ALTER TABLE books ADD COLUMN itunes_last_played TIMESTAMP",
		"ALTER TABLE books ADD COLUMN itunes_rating INTEGER",
		"ALTER TABLE books ADD COLUMN itunes_bookmark INTEGER",
		"ALTER TABLE books ADD COLUMN itunes_import_source TEXT",
	}

	for _, stmt := range alterStatements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_itunes_persistent_id ON books(itunes_persistent_id)",
	}

	for _, stmt := range indexStatements {
		log.Printf("    - Creating index: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Println("  - iTunes import metadata fields added successfully")
	return nil
}

// migration012Up adds created_at and updated_at timestamp columns to books table
func migration012Up(store Store) error {
	log.Println("  - Adding created_at and updated_at timestamp columns to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration (PebbleDB handles timestamps natively)")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP",
		"ALTER TABLE books ADD COLUMN updated_at DATETIME",
	}

	for _, stmt := range alterStatements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	log.Println("  - Timestamp columns added successfully")
	return nil
}

// migration013Up adds wanted state support and multi-path tracking
func migration013Up(store Store) error {
	log.Println("  - Adding wanted state support and multi-path tracking")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Step 1: Create audiobook_source_paths table for multi-path tracking
	log.Println("    - Creating audiobook_source_paths table")
	createTableSQL := `CREATE TABLE IF NOT EXISTS audiobook_source_paths (
		id TEXT PRIMARY KEY,
		audiobook_id TEXT NOT NULL,
		source_path TEXT NOT NULL UNIQUE,
		still_exists BOOLEAN DEFAULT 1,
		added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_verified DATETIME,
		FOREIGN KEY (audiobook_id) REFERENCES books(id) ON DELETE CASCADE
	)`
	if _, err := sqliteStore.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create audiobook_source_paths table: %w", err)
	}

	// Step 2: Create indices for source_paths table
	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_source_paths_audiobook ON audiobook_source_paths(audiobook_id)",
		"CREATE INDEX IF NOT EXISTS idx_source_paths_path ON audiobook_source_paths(source_path)",
	}
	for _, idx := range indices {
		log.Printf("    - Creating index: %s", idx)
		if _, err := sqliteStore.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Step 3: Migrate existing file paths to source_paths table
	log.Println("    - Migrating existing file paths to source_paths table")
	migrateSQLQuery := `
		INSERT INTO audiobook_source_paths (id, audiobook_id, source_path, added_at)
		SELECT
			lower(hex(randomblob(16))),
			id,
			file_path,
			COALESCE(created_at, CURRENT_TIMESTAMP)
		FROM books
		WHERE file_path IS NOT NULL AND file_path != ''
		ON CONFLICT(source_path) DO NOTHING
	`
	if _, err := sqliteStore.db.Exec(migrateSQLQuery); err != nil {
		log.Printf("    - Warning: Could not migrate paths (may already exist): %v", err)
	}

	// Step 4: Add wanted boolean to authors table
	log.Println("    - Adding wanted field to authors table")
	alterAuthors := "ALTER TABLE authors ADD COLUMN wanted BOOLEAN DEFAULT 0"
	if _, err := sqliteStore.db.Exec(alterAuthors); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add wanted to authors: %w", err)
		}
		log.Printf("    - Column already exists, skipping")
	}

	// Step 5: Add wanted boolean to series table
	log.Println("    - Adding wanted field to series table")
	alterSeries := "ALTER TABLE series ADD COLUMN wanted BOOLEAN DEFAULT 0"
	if _, err := sqliteStore.db.Exec(alterSeries); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add wanted to series: %w", err)
		}
		log.Printf("    - Column already exists, skipping")
	}

	// Step 6: Note about library_state - it already exists and supports 'wanted'
	// The library_state column was added in migration 9 as TEXT with default 'imported'
	// It can already store 'wanted', 'imported', 'organized', 'deleted' values
	// No schema change needed, just update documentation
	log.Println("    - library_state already supports 'wanted' value (no change needed)")

	// Step 7: Note about file_path - we DON'T make it nullable to preserve data integrity
	// Instead, wanted books will use a special sentinel value or empty string
	// This prevents breaking existing queries and constraints
	log.Println("    - file_path remains NOT NULL; wanted books will use empty string ''")

	log.Println("  - Wanted state and multi-path tracking added successfully")
	return nil
}

// migration014Up flags books with corrupted organize paths (unresolved
// placeholders like {series} or {author}) by setting library_state to
// 'needs_review'. This is a one-time cleanup for paths written before the
// leftover-placeholder guard was added to expandPattern.
func migration014Up(store Store) error {
	log.Println("  Running migration 14: Flag books with corrupted organize paths")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		// PebbleDB: iterate all books and check FilePath
		log.Println("    - Skipping SQLite-specific path; checking PebbleDB books")
		return migration014UpPebble(store)
	}

	// SQLite path: UPDATE in bulk using LIKE '%{%}%' pattern
	query := `
		UPDATE books
		SET library_state = 'needs_review'
		WHERE file_path LIKE '%{%}%'
		  AND library_state != 'needs_review'
	`
	result, err := sqliteStore.db.Exec(query)
	if err != nil {
		return fmt.Errorf("migration 14: failed to flag corrupted paths: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	log.Printf("    - Flagged %d books with corrupted organize paths for review", rowsAffected)
	return nil
}

// migration014UpPebble handles the corrupted-path check for PebbleDB stores.
func migration014UpPebble(store Store) error {
	books, err := store.GetAllBooks(1000000, 0)
	if err != nil {
		return fmt.Errorf("migration 14: failed to list books: %w", err)
	}

	flagged := 0
	for _, book := range books {
		if !strings.Contains(book.FilePath, "{") {
			continue
		}
		// FilePath contains a literal brace — flag for review
		state := "needs_review"
		book.LibraryState = &state
		if _, updateErr := store.UpdateBook(book.ID, &book); updateErr != nil {
			log.Printf("    - Warning: could not flag book %s (%s): %v", book.ID, book.FilePath, updateErr)
			continue
		}
		flagged++
	}
	log.Printf("    - Flagged %d books with corrupted organize paths for review (PebbleDB)", flagged)
	return nil
}

// migration015Up adds book_authors junction table, cover_url, and narrators_json
func migration015Up(store Store) error {
	log.Println("  - Adding book_authors junction table, cover_url, and narrators_json")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Create book_authors junction table
	createTableSQL := `CREATE TABLE IF NOT EXISTS book_authors (
		book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		author_id INTEGER NOT NULL REFERENCES authors(id),
		role TEXT NOT NULL DEFAULT 'author',
		position INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (book_id, author_id)
	)`
	if _, err := sqliteStore.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create book_authors table: %w", err)
	}

	// Create indices
	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_book_authors_book ON book_authors(book_id)",
		"CREATE INDEX IF NOT EXISTS idx_book_authors_author ON book_authors(author_id)",
	}
	for _, idx := range indices {
		if _, err := sqliteStore.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Migrate existing author_id data into junction table
	migrateSQL := `INSERT OR IGNORE INTO book_authors (book_id, author_id, role, position)
		SELECT id, author_id, 'author', 0 FROM books WHERE author_id IS NOT NULL`
	result, err := sqliteStore.db.Exec(migrateSQL)
	if err != nil {
		return fmt.Errorf("failed to migrate existing author data: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	log.Printf("    - Migrated %d existing book-author relationships", rowsAffected)

	// Add cover_url and narrators_json columns to books
	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN cover_url TEXT",
		"ALTER TABLE books ADD COLUMN narrators_json TEXT",
	}
	for _, stmt := range alterStatements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping: %s", stmt)
				continue
			}
			return fmt.Errorf("failed to execute '%s': %w", stmt, err)
		}
	}

	log.Println("  - book_authors, cover_url, and narrators_json added successfully")
	return nil
}

// migration016Up creates users, sessions, book_segments, and playback tracking tables
func migration016Up(store Store) error {
	log.Println("  - Adding users, sessions, book_segments, and playback tables")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
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
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			revoked INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS book_segments (
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
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			version INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_book_segments_book ON book_segments(book_id)`,
		`CREATE TABLE IF NOT EXISTS playback_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			book_id INTEGER NOT NULL,
			segment_id TEXT NOT NULL DEFAULT '',
			position_seconds INTEGER NOT NULL DEFAULT 0,
			event_type TEXT NOT NULL DEFAULT 'progress',
			play_speed REAL NOT NULL DEFAULT 1.0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			version INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_playback_events_user_book ON playback_events(user_id, book_id)`,
		`CREATE TABLE IF NOT EXISTS playback_progress (
			user_id TEXT NOT NULL,
			book_id INTEGER NOT NULL,
			segment_id TEXT NOT NULL DEFAULT '',
			position_seconds INTEGER NOT NULL DEFAULT 0,
			percent_complete REAL NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			version INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (user_id, book_id)
		)`,
		`CREATE TABLE IF NOT EXISTS book_stats (
			book_id INTEGER PRIMARY KEY,
			play_count INTEGER NOT NULL DEFAULT 0,
			listen_seconds INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS user_stats (
			user_id TEXT PRIMARY KEY,
			listen_seconds INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1
		)`,
	}

	for _, sql := range tables {
		if _, err := sqliteStore.db.Exec(sql); err != nil {
			return fmt.Errorf("failed to execute migration 16: %w", err)
		}
	}

	log.Println("  - Users, sessions, book_segments, and playback tables created")
	return nil
}

// migration017Up adds composite indexes for common queries and FTS5 full-text search
func migration017Up(store Store) error {
	log.Println("  - Adding composite indexes and FTS5 full-text search")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Composite indexes for common query patterns
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_notdeleted_title ON books(COALESCE(marked_for_deletion, 0), title)",
		"CREATE INDEX IF NOT EXISTS idx_books_created_at ON books(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_books_author_title ON books(author_id, title)",
	}

	for _, stmt := range indexes {
		log.Printf("    - Creating index: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// FTS5 virtual table for full-text search on book titles
	ftsStatements := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(title, content=books, content_rowid=rowid)`,
		`CREATE TRIGGER IF NOT EXISTS books_fts_insert AFTER INSERT ON books BEGIN
			INSERT INTO books_fts(rowid, title) VALUES (new.rowid, new.title);
		END`,
		`CREATE TRIGGER IF NOT EXISTS books_fts_update AFTER UPDATE OF title ON books BEGIN
			INSERT INTO books_fts(books_fts, rowid, title) VALUES('delete', old.rowid, old.title);
			INSERT INTO books_fts(rowid, title) VALUES (new.rowid, new.title);
		END`,
		`CREATE TRIGGER IF NOT EXISTS books_fts_delete AFTER DELETE ON books BEGIN
			INSERT INTO books_fts(books_fts, rowid, title) VALUES('delete', old.rowid, old.title);
		END`,
	}

	// FTS5 may not be compiled into all SQLite builds; skip gracefully if unavailable
	ftsAvailable := true
	for _, stmt := range ftsStatements {
		log.Printf("    - Executing FTS5 setup: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "no such module") {
				log.Printf("    - FTS5 module not available, skipping full-text search setup")
				ftsAvailable = false
				break
			}
			return fmt.Errorf("failed FTS5 setup: %w", err)
		}
	}

	// Populate FTS index from existing data
	if ftsAvailable {
		log.Println("    - Populating FTS5 index from existing books")
		if _, err := sqliteStore.db.Exec(`INSERT INTO books_fts(rowid, title) SELECT rowid, title FROM books`); err != nil {
			log.Printf("    - Warning: FTS5 population failed (may already be populated): %v", err)
		}
	}

	log.Println("  - Composite indexes and FTS5 added successfully")
	return nil
}

func migration018Up(store Store) error {
	if sqlStore, ok := store.(*SQLiteStore); ok {
		_, err := sqlStore.db.Exec(`
			CREATE TABLE IF NOT EXISTS itunes_library_state (
				path       TEXT PRIMARY KEY,
				size       INTEGER NOT NULL,
				mod_time   TEXT NOT NULL,
				crc32      INTEGER NOT NULL,
				updated_at TEXT NOT NULL
			)
		`)
		return err
	}
	// PebbleDB: no schema needed, uses key-value pairs
	return nil
}

func migration019Up(store Store) error {
	log.Println("  - Adding metadata_changes_history table for undo support")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS metadata_changes_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id TEXT NOT NULL,
			field TEXT NOT NULL,
			previous_value TEXT,
			new_value TEXT,
			change_type TEXT NOT NULL,
			source TEXT,
			changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		"CREATE INDEX IF NOT EXISTS idx_metadata_changes_book ON metadata_changes_history(book_id)",
		"CREATE INDEX IF NOT EXISTS idx_metadata_changes_book_field ON metadata_changes_history(book_id, field)",
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 19 failed: %w", err)
		}
	}

	return nil
}

// GetMigrationHistory returns all applied migrations
func GetMigrationHistory(store Store) ([]MigrationRecord, error) {
	// Get all preferences that start with "migration_"
	allPrefs, err := store.GetAllUserPreferences()
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences: %w", err)
	}

	records := []MigrationRecord{}
	for _, pref := range allPrefs {
		if len(pref.Key) > 10 && pref.Key[:10] == "migration_" {
			if pref.Value == nil {
				continue
			}
			var record MigrationRecord
			if err := json.Unmarshal([]byte(*pref.Value), &record); err != nil {
				log.Printf("Warning: failed to parse migration record %s: %v", pref.Key, err)
				continue
			}
			records = append(records, record)
		}
	}

	return records, nil
}

// migration020Up adds narrators and book_narrators tables
func migration020Up(store Store) error {
	log.Println("  - Adding narrators and book_narrators tables")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS narrators (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_narrators_name ON narrators(name)`,
		`CREATE TABLE IF NOT EXISTS book_narrators (
			book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			narrator_id INTEGER NOT NULL REFERENCES narrators(id),
			role TEXT NOT NULL DEFAULT 'narrator',
			position INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (book_id, narrator_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_book_narrators_book ON book_narrators(book_id)`,
		`CREATE INDEX IF NOT EXISTS idx_book_narrators_narrator ON book_narrators(narrator_id)`,
	}

	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute '%s': %w", stmt, err)
		}
	}

	log.Println("  - narrators and book_narrators tables added successfully")
	return nil
}

// migration021Up adds the operation_summary_logs table for persistent operation history
func migration021Up(store Store) error {
	log.Println("  - Adding operation_summary_logs table for persistent operation history")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS operation_summary_logs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			progress REAL NOT NULL DEFAULT 0,
			result TEXT,
			error TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_op_summary_logs_status ON operation_summary_logs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_op_summary_logs_created ON operation_summary_logs(created_at)`,
	}

	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute '%s': %w", stmt, err)
		}
	}

	log.Println("  - operation_summary_logs table added successfully")
	return nil
}

// migration022Up backfills the book_authors and book_narrators junction tables
// for books that were imported before the multi-author "&" splitting feature.
//
// Authors: For each book_authors row whose referenced author name contains " & ",
// this migration splits the name, creates individual author records as needed,
// replaces the old junction row with one row per split name (role: "author" for
// position 0, "co-author" for subsequent positions).
//
// Narrators: For each book that has a non-empty books.narrator field but no rows
// in book_narrators, this migration splits on " & ", creates narrator records as
// needed, and inserts the junction rows.
//
// The migration is idempotent: it uses INSERT OR IGNORE and only touches rows
// where the author name actually contains " & ".
func migration022Up(store Store) error {
	log.Println("  - Running migration 22: backfill book_authors (&-split) and book_narrators")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// -------------------------------------------------------------------------
	// PART 1: Authors — split "&"-joined names
	// -------------------------------------------------------------------------
	authorRows, err := sqliteStore.db.Query(`
		SELECT ba.book_id, ba.author_id, a.name
		FROM book_authors ba
		JOIN authors a ON a.id = ba.author_id
		WHERE a.name LIKE '% & %'
	`)
	if err != nil {
		return fmt.Errorf("migration 22: query joined authors: %w", err)
	}

	type joinedAuthor struct {
		bookID   string
		authorID int
		name     string
	}
	var joinedAuthors []joinedAuthor
	for authorRows.Next() {
		var ja joinedAuthor
		if err := authorRows.Scan(&ja.bookID, &ja.authorID, &ja.name); err != nil {
			authorRows.Close()
			return fmt.Errorf("migration 22: scan author row: %w", err)
		}
		joinedAuthors = append(joinedAuthors, ja)
	}
	authorRows.Close()
	if err := authorRows.Err(); err != nil {
		return fmt.Errorf("migration 22: author rows error: %w", err)
	}

	for _, ja := range joinedAuthors {
		if !strings.Contains(ja.name, " & ") {
			continue
		}

		parts := strings.Split(ja.name, " & ")
		log.Printf("    - Splitting author %q for book %s into %d parts", ja.name, ja.bookID, len(parts))

		// Remove the old junction row for this book+author pair
		if _, err := sqliteStore.db.Exec(`DELETE FROM book_authors WHERE book_id = ? AND author_id = ?`,
			ja.bookID, ja.authorID); err != nil {
			return fmt.Errorf("migration 22: delete old book_authors row: %w", err)
		}

		// Create/find each split author and insert into junction table
		for i, rawName := range parts {
			name := strings.TrimSpace(rawName)
			if name == "" {
				continue
			}

			var indivAuthorID int
			var existingID int
			err := sqliteStore.db.QueryRow(`SELECT id FROM authors WHERE LOWER(name) = LOWER(?)`, name).Scan(&existingID)
			if err == sql.ErrNoRows {
				result, createErr := sqliteStore.db.Exec(`INSERT INTO authors (name) VALUES (?)`, name)
				if createErr != nil {
					return fmt.Errorf("migration 22: create author %q: %w", name, createErr)
				}
				insertedID, _ := result.LastInsertId()
				indivAuthorID = int(insertedID)
				log.Printf("      - Created new author %q (id=%d)", name, indivAuthorID)
			} else if err != nil {
				return fmt.Errorf("migration 22: lookup author %q: %w", name, err)
			} else {
				indivAuthorID = existingID
				log.Printf("      - Found existing author %q (id=%d)", name, indivAuthorID)
			}

			role := "author"
			if i > 0 {
				role = "co-author"
			}

			if _, err := sqliteStore.db.Exec(`
				INSERT OR IGNORE INTO book_authors (book_id, author_id, role, position)
				VALUES (?, ?, ?, ?)`,
				ja.bookID, indivAuthorID, role, i); err != nil {
				return fmt.Errorf("migration 22: insert book_authors for %q: %w", name, err)
			}
		}

		// Update books.author_id to point to the primary (first) author
		primaryName := strings.TrimSpace(parts[0])
		if primaryName != "" {
			var primaryID int
			if err := sqliteStore.db.QueryRow(`SELECT id FROM authors WHERE LOWER(name) = LOWER(?)`, primaryName).Scan(&primaryID); err == nil {
				if _, err := sqliteStore.db.Exec(`UPDATE books SET author_id = ? WHERE id = ?`, primaryID, ja.bookID); err != nil {
					log.Printf("    - Warning: could not update books.author_id for book %s: %v", ja.bookID, err)
				}
			}
		}
	}

	// -------------------------------------------------------------------------
	// PART 2: Narrators — backfill from books.narrator field
	// -------------------------------------------------------------------------
	narBookRows, err := sqliteStore.db.Query(`
		SELECT b.id, b.narrator
		FROM books b
		WHERE b.narrator IS NOT NULL
		  AND b.narrator != ''
		  AND NOT EXISTS (
			SELECT 1 FROM book_narrators bn WHERE bn.book_id = b.id
		  )
	`)
	if err != nil {
		return fmt.Errorf("migration 22: query narrator books: %w", err)
	}

	type narBook struct {
		bookID   string
		narrator string
	}
	var narBooks []narBook
	for narBookRows.Next() {
		var nb narBook
		if err := narBookRows.Scan(&nb.bookID, &nb.narrator); err != nil {
			narBookRows.Close()
			return fmt.Errorf("migration 22: scan narrator book: %w", err)
		}
		narBooks = append(narBooks, nb)
	}
	narBookRows.Close()
	if err := narBookRows.Err(); err != nil {
		return fmt.Errorf("migration 22: narrator book rows error: %w", err)
	}

	log.Printf("    - Found %d books with narrator field but no book_narrators rows", len(narBooks))

	for _, nb := range narBooks {
		parts := strings.Split(nb.narrator, " & ")
		log.Printf("    - Backfilling narrators for book %s: %q (%d part(s))", nb.bookID, nb.narrator, len(parts))

		for i, rawName := range parts {
			name := strings.TrimSpace(rawName)
			if name == "" {
				continue
			}

			var narratorID int
			var existingID int
			err := sqliteStore.db.QueryRow(`SELECT id FROM narrators WHERE LOWER(name) = LOWER(?)`, name).Scan(&existingID)
			if err == sql.ErrNoRows {
				result, createErr := sqliteStore.db.Exec(`INSERT INTO narrators (name) VALUES (?)`, name)
				if createErr != nil {
					return fmt.Errorf("migration 22: create narrator %q: %w", name, createErr)
				}
				insertedID, _ := result.LastInsertId()
				narratorID = int(insertedID)
				log.Printf("      - Created new narrator %q (id=%d)", name, narratorID)
			} else if err != nil {
				return fmt.Errorf("migration 22: lookup narrator %q: %w", name, err)
			} else {
				narratorID = existingID
				log.Printf("      - Found existing narrator %q (id=%d)", name, narratorID)
			}

			role := "narrator"
			if i > 0 {
				role = "co-narrator"
			}

			if _, err := sqliteStore.db.Exec(`
				INSERT OR IGNORE INTO book_narrators (book_id, narrator_id, role, position)
				VALUES (?, ?, ?, ?)`,
				nb.bookID, narratorID, role, i); err != nil {
				return fmt.Errorf("migration 22: insert book_narrators for %q: %w", name, err)
			}
		}
	}

	log.Println("  - Migration 22 complete: book_authors and book_narrators backfilled")
	return nil
}

// migration023Up adds metadata_updated_at and last_written_at timestamp columns to books table.
// metadata_updated_at is set only when user-visible metadata changes; last_written_at is set
// when metadata is written back to audio files on disk.
func migration023Up(store Store) error {
	log.Println("  - Adding metadata_updated_at and last_written_at to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN metadata_updated_at DATETIME",
		"ALTER TABLE books ADD COLUMN last_written_at DATETIME",
	}

	for _, stmt := range alterStatements {
		log.Printf("    - Executing: %s", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				log.Printf("    - Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	// Backfill: set metadata_updated_at = updated_at for existing books that already
	// have an updated_at. This preserves the approximate "last edited" time for
	// books that were already in the library before this migration.
	if _, err := sqliteStore.db.Exec(
		`UPDATE books SET metadata_updated_at = updated_at WHERE updated_at IS NOT NULL AND metadata_updated_at IS NULL`,
	); err != nil {
		return fmt.Errorf("failed to backfill metadata_updated_at: %w", err)
	}

	log.Println("  - metadata_updated_at and last_written_at added successfully")
	return nil
}

// migration024Up adds metadata_review_status column to books table.
func migration024Up(store Store) error {
	log.Println("  - Adding metadata_review_status to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		log.Println("  - Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Check if column already exists (schema may include it for fresh DBs).
	var count int
	err := sqliteStore.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('books') WHERE name = 'metadata_review_status'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for metadata_review_status column: %w", err)
	}
	if count > 0 {
		log.Println("  - Column already exists, skipping")
		return nil
	}

	_, err = sqliteStore.db.Exec(`ALTER TABLE books ADD COLUMN metadata_review_status TEXT`)
	if err != nil {
		return fmt.Errorf("failed to add metadata_review_status column: %w", err)
	}

	log.Println("  - metadata_review_status added successfully")
	return nil
}
