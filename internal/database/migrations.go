// file: internal/database/migrations.go
// version: 1.39.0
// guid: 9a8b7c6d-5e4f-3d2c-1b0a-9f8e7d6c5b4a
// last-edited: 2026-05-05

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log/slog"
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
	{
		Version:     25,
		Description: "Add asin column to books",
		Up:          migration025Up,
		Down:        nil,
	},
	{
		Version:     26,
		Description: "Create book_tombstones table",
		Up:          migration026Up,
		Down:        nil,
	},
	{
		Version:     27,
		Description: "Add result_data column to operations",
		Up:          migration027Up,
		Down:        nil,
	},
	{
		Version:     28,
		Description: "Add external provider ID columns (open_library_id, hardcover_id, google_books_id)",
		Up:          migration028Up,
		Down:        nil,
	},
	{
		Version:     29,
		Description: "Add operation_changes table for undo/rollback tracking",
		Up:          migration029Up,
		Down:        nil,
	},
	{
		Version:     30,
		Description: "Add file_hash column to book_segments for auto-relinking",
		Up:          migration030Up,
		Down:        nil,
	},
	{
		Version:     31,
		Description: "Add system_activity_log table and logs_pruned flag",
		Up:          migration031Up,
		Down:        nil,
	},
	{
		Version:     32,
		Description: "Add scan cache columns for incremental scanning",
		Up:          migration032Up,
		Down:        nil,
	},
	{
		Version:     33,
		Description: "Add deferred_itunes_updates table for transcode path changes",
		Up:          migration033Up,
		Down:        nil,
	},
	{
		Version:     34,
		Description: "Add external_id_map table for PID/ASIN mapping",
		Up:          migration034Up,
		Down:        nil,
	},
	{
		Version:     35,
		Description: "Add book_path_history table for file rename/move tracking",
		Up:          migration035Up,
		Down:        nil,
	},
	{
		Version:     36,
		Description: "Add genre column to books table",
		Up:          migration036Up,
		Down:        nil,
	},
	{
		Version:     37,
		Description: "Add book_tags table for user-defined tags",
		Up:          migration037Up,
		Down:        nil,
	},
	{
		Version:     38,
		Description: "Add itunes_path column to books table",
		Up:          migration038Up,
		Down:        nil,
	},
	{
		Version:     39,
		Description: "Create book_files table and migrate book_segments data",
		Up:          migration039Up,
		Down:        nil,
	},
	{
		Version:     40,
		Description: "Add last_organize_operation_id and last_organized_at columns to books",
		Up:          migration040Up,
		Down:        nil,
	},
	{
		Version:     41,
		Description: "Add itunes_sync_status column to books for dirty tracking",
		Up:          migration041Up,
		Down:        nil,
	},
	{
		Version:     42,
		Description: "Drop dead tables and add missing indexes",
		Up:          migration042Up,
		Down:        nil,
	},
	{
		Version:     43,
		Description: "Drop deprecated book_segments table",
		Up:          migration043Up,
		Down:        nil,
	},
	{
		Version:     44,
		Description: "Add PID provenance and removed_at to external_id_map",
		Up:          migration044Up,
		Down:        nil,
	},
	{
		Version:     45,
		Description: "Add operation_results table for structured operation output",
		Up:          migration045Up,
		Down:        nil,
	},
	{
		Version:     46,
		Description: "Add book_alternative_titles table for search + dedup variant matching",
		Up:          migration046Up,
		Down:        nil,
	},
	{
		Version:     47,
		Description: "Add source column to book_tags so system-applied tags can be distinguished from user tags",
		Up:          migration047Up,
		Down:        nil,
	},
	{
		Version:     48,
		Description: "Add author_tags and series_tags tables for author/series-level tagging",
		Up:          migration048Up,
		Down:        nil,
	},
	{
		Version:     49,
		Description: "Add acoustid_fingerprint and acoustid_duration columns to book_files for content-based matching",
		Up:          migration049Up,
		Down:        nil,
	},
	{
		Version:     50,
		Description: "Replace single acoustid_fingerprint with 7 segment columns",
		Up:          migration050Up,
		Down:        nil,
	},
	{
		Version:     51,
		Description: "Add quarantine_reason and quarantined_at columns to books",
		Up:          migration051Up,
		Down:        nil,
	},
	{
		Version:     52,
		Description: "Add ai_jobs and ai_job_payloads tables for unified batch tracking",
		Up:          migration052Up,
		Down:        nil,
	},
	{
		Version:     53,
		Description: "Add post_metadata_hash column to book_files for post-write SHA tracking",
		Up:          migration053Up,
		Down:        nil,
	},
	{
		Version:     54,
		Description: "Add metadata_rejections table for auditing rejected metadata candidates",
		Up:          migration054Up,
		Down:        nil,
	},
	{
		Version:     55,
		Description: "Add metadata_source_hash column to books for metadata-based deduplication (MATCH-1)",
		Up:          migration055Up,
		Down:        nil,
	},
	{
		Version:     56,
		Description: "Add merged_into_book_id column to books for chapter consolidation (MATCH-2)",
		Up:          migration056Up,
		Down:        nil,
	},
	{
		Version:     57,
		Description: "Add unique index on (file_hash, source_import_path) to prevent duplicate audiobook records",
		Up:          migration057Up,
		Down:        nil,
	},
	{
		Version:     58,
		Description: "Add book signature columns for unified per-book audio fingerprint",
		Up:          migration058Up,
		Down:        nil,
	},
	{
		Version:     59,
		Description: "Add Unified Operations System v2 core schema",
		Up:          migration059Up,
		Down:        migration059Down,
	},
	{
		Version:     60,
		Description: "Add partial book signature mask/coverage and file fingerprint diagnosis columns",
		Up:          migration060Up,
		Down:        nil,
	},
}

// RunMigrations applies all pending migrations
func RunMigrations(store Store) error {
	currentVersion, err := getCurrentVersion(store)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	slog.Info("current database version", "version", currentVersion)

	// Find migrations to apply
	pendingMigrations := []Migration{}
	for _, m := range migrations {
		if m.Version > currentVersion {
			pendingMigrations = append(pendingMigrations, m)
		}
	}

	if len(pendingMigrations) == 0 {
		slog.Info("database is up to date", "version", currentVersion)
		return nil
	}

	slog.Info("applying migrations", "count", len(pendingMigrations))

	// Apply each migration
	for _, m := range pendingMigrations {
		slog.Info("applying migration", "version", m.Version, "description", m.Description)

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

		slog.Info("migration completed", "version", m.Version)
	}

	slog.Info("all migrations completed", "version", pendingMigrations[len(pendingMigrations)-1].Version)

	// After all migrations run, ensure optional extended columns exist on book_files.
	// This is a no-op for fresh databases (columns already in CREATE TABLE) and adds
	// missing columns to older databases via PRAGMA table_info + ALTER TABLE.
	if sqliteStore, ok := store.(*SQLiteStore); ok {
		if err := sqliteStore.ensureExtendedBookFileColumns(); err != nil {
			return fmt.Errorf("ensureExtendedBookFileColumns: %w", err)
		}
	}
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
	slog.Info("- Validating basic schema (authors, series, books, playlists)")
	return nil
}

// migration002Up adds import paths and operations support
func migration002Up(store Store) error {
	// These structures are already supported by the current store interface
	slog.Info("- Adding import paths and operations support")
	return nil
}

// migration003Up adds user preferences support
func migration003Up(store Store) error {
	// User preferences already supported by current interface
	slog.Info("- Adding user preferences support")
	return nil
}

// migration004Up adds extended Pebble keyspace
func migration004Up(store Store) error {
	// Extended keyspace (users, sessions, segments, playback) already supported
	slog.Info("- Adding extended Pebble keyspace (users, sessions, segments, playback)")
	return nil
}

// migration005Up adds media info and version management fields to books table
func migration005Up(store Store) error {
	slog.Info("- Adding media info and version management fields to books table")

	// Check if this is a SQLite store (we need direct SQL access for ALTER TABLE)
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		// For non-SQLite stores, these fields are handled by the store implementation
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			// Check if column already exists (this is not an error)
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping")
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
		slog.Info("creating index", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	slog.Info("- Media info and version management fields added successfully")
	return nil
}

// migration006Up adds original and organized file hash tracking columns
func migration006Up(store Store) error {
	slog.Info("- Adding original/organized file hash columns to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN original_file_hash TEXT",
		"ALTER TABLE books ADD COLUMN organized_file_hash TEXT",
	}

	for _, stmt := range alterStatements {
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping")
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
		slog.Info("creating index", "stmt", stmt)
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
	slog.Info("- Renaming import paths to import paths")

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
			slog.Info("executing statement", "stmt", stmt)
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to execute '%s': %w", stmt, err)
			}
		}
	default:
		slog.Info("- Unknown store type; skipping migration")
	}

	return nil
}

func migration008Up(store Store) error {
	slog.Info("- Adding do_not_import table for hash blocklist")

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
			slog.Info("executing statement", "stmt", stmt)
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to execute '%s': %w", stmt, err)
			}
		}
	case *PebbleStore:
		// For PebbleDB, we just need to log that the keyspace is available
		// No schema changes needed for Pebble
		slog.Info("- Pebble keyspace for do_not_import enabled")
	default:
		slog.Info("- Unknown store type; skipping migration")
	}

	return nil
}

// migration009Up adds state machine and lifecycle tracking fields to books table
func migration009Up(store Store) error {
	slog.Info("- Adding state machine and lifecycle fields to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN library_state TEXT DEFAULT 'imported'",
		"ALTER TABLE books ADD COLUMN quantity INTEGER DEFAULT 1",
		"ALTER TABLE books ADD COLUMN marked_for_deletion BOOLEAN DEFAULT 0",
		"ALTER TABLE books ADD COLUMN marked_for_deletion_at DATETIME",
	}

	for _, stmt := range alterStatements {
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping")
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
		slog.Info("creating index", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	slog.Info("- State machine fields added successfully")
	return nil
}

// migration010Up adds metadata_states table for persisted metadata provenance
func migration010Up(store Store) error {
	slog.Info("- Adding metadata_states table for metadata provenance")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute '%s': %w", stmt, err)
		}
	}

	return nil
}

// migration011Up adds iTunes import metadata fields to books table.
func migration011Up(store Store) error {
	slog.Info("- Adding iTunes import metadata fields to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	indexStatements := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_itunes_persistent_id ON books(itunes_persistent_id)",
	}

	for _, stmt := range indexStatements {
		slog.Info("creating index", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	slog.Info("- iTunes import metadata fields added successfully")
	return nil
}

// migration012Up adds created_at and updated_at timestamp columns to books table
func migration012Up(store Store) error {
	slog.Info("- Adding created_at and updated_at timestamp columns to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB handles timestamps natively)")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP",
		"ALTER TABLE books ADD COLUMN updated_at DATETIME",
	}

	for _, stmt := range alterStatements {
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping")
				continue
			}
			return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
		}
	}

	slog.Info("- Timestamp columns added successfully")
	return nil
}

// migration013Up adds wanted state support and multi-path tracking
func migration013Up(store Store) error {
	slog.Info("- Adding wanted state support and multi-path tracking")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Step 1: Create audiobook_source_paths table for multi-path tracking
	slog.Info("- Creating audiobook_source_paths table")
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
		slog.Info("- Creating index", "value", idx)
		if _, err := sqliteStore.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Step 3: Migrate existing file paths to source_paths table
	slog.Info("- Migrating existing file paths to source_paths table")
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
		slog.Info("- Warning Could not migrate paths (may already exist)", "error", err)
	}

	// Step 4: Add wanted boolean to authors table
	slog.Info("- Adding wanted field to authors table")
	alterAuthors := "ALTER TABLE authors ADD COLUMN wanted BOOLEAN DEFAULT 0"
	if _, err := sqliteStore.db.Exec(alterAuthors); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add wanted to authors: %w", err)
		}
		slog.Info("- Column already exists, skipping")
	}

	// Step 5: Add wanted boolean to series table
	slog.Info("- Adding wanted field to series table")
	alterSeries := "ALTER TABLE series ADD COLUMN wanted BOOLEAN DEFAULT 0"
	if _, err := sqliteStore.db.Exec(alterSeries); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add wanted to series: %w", err)
		}
		slog.Info("- Column already exists, skipping")
	}

	// Step 6: Note about library_state - it already exists and supports 'wanted'
	// The library_state column was added in migration 9 as TEXT with default 'imported'
	// It can already store 'wanted', 'imported', 'organized', 'deleted' values
	// No schema change needed, just update documentation
	slog.Info("- library_state already supports 'wanted' value (no change needed)")

	// Step 7: Note about file_path - we DON'T make it nullable to preserve data integrity
	// Instead, wanted books will use a special sentinel value or empty string
	// This prevents breaking existing queries and constraints
	slog.Info("- file_path remains NOT NULL; wanted books will use empty string ''")

	slog.Info("- Wanted state and multi-path tracking added successfully")
	return nil
}

// migration014Up flags books with corrupted organize paths (unresolved
// placeholders like {series} or {author}) by setting library_state to
// 'needs_review'. This is a one-time cleanup for paths written before the
// leftover-placeholder guard was added to expandPattern.
func migration014Up(store Store) error {
	slog.Info("Running migration 14 Flag books with corrupted organize paths")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		// PebbleDB: iterate all books and check FilePath
		slog.Info("- Skipping SQLite-specific path; checking PebbleDB books")
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
	slog.Info("- Flagged", "count", rowsAffected)
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
			slog.Info("- Warning could not flag book", "value", book.ID, "path", book.FilePath, "error", updateErr)
			continue
		}
		flagged++
	}
	slog.Info("- Flagged", "count", flagged)
	return nil
}

// migration015Up adds book_authors junction table, cover_url, and narrators_json
func migration015Up(store Store) error {
	slog.Info("- Adding book_authors junction table, cover_url, and narrators_json")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
	slog.Info("- Migrated", "count", rowsAffected)

	// Add cover_url and narrators_json columns to books
	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN cover_url TEXT",
		"ALTER TABLE books ADD COLUMN narrators_json TEXT",
	}
	for _, stmt := range alterStatements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping", "value", stmt)
				continue
			}
			return fmt.Errorf("failed to execute '%s': %w", stmt, err)
		}
	}

	slog.Info("- book_authors, cover_url, and narrators_json added successfully")
	return nil
}

// migration016Up creates users, sessions, book_segments, and playback tracking tables
func migration016Up(store Store) error {
	slog.Info("- Adding users, sessions, book_segments, and playback tables")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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

	slog.Info("- Users, sessions, book_segments, and playback tables created")
	return nil
}

// migration017Up adds composite indexes for common queries and FTS5 full-text search
func migration017Up(store Store) error {
	slog.Info("- Adding composite indexes and FTS5 full-text search")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	// Composite indexes for common query patterns
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_books_notdeleted_title ON books(COALESCE(marked_for_deletion, 0), title)",
		"CREATE INDEX IF NOT EXISTS idx_books_created_at ON books(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_books_author_title ON books(author_id, title)",
	}

	for _, stmt := range indexes {
		slog.Info("creating index", "stmt", stmt)
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
		slog.Info("executing FTS5 setup", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "no such module") {
				slog.Info("- FTS5 module not available, skipping full-text search setup")
				ftsAvailable = false
				break
			}
			return fmt.Errorf("failed FTS5 setup: %w", err)
		}
	}

	// Populate FTS index from existing data
	if ftsAvailable {
		slog.Info("- Populating FTS5 index from existing books")
		if _, err := sqliteStore.db.Exec(`INSERT INTO books_fts(rowid, title) SELECT rowid, title FROM books`); err != nil {
			slog.Info("- Warning FTS5 population failed (may already be populated)", "error", err)
		}
	}

	slog.Info("- Composite indexes and FTS5 added successfully")
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
	slog.Info("- Adding metadata_changes_history table for undo support")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
				slog.Warn("failed to parse migration record", "key", pref.Key, "error", err)
				continue
			}
			records = append(records, record)
		}
	}

	return records, nil
}

// migration020Up adds narrators and book_narrators tables
func migration020Up(store Store) error {
	slog.Info("- Adding narrators and book_narrators tables")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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

	slog.Info("- narrators and book_narrators tables added successfully")
	return nil
}

// migration021Up adds the operation_summary_logs table for persistent operation history
func migration021Up(store Store) error {
	slog.Info("- Adding operation_summary_logs table for persistent operation history")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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

	slog.Info("- operation_summary_logs table added successfully")
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
	slog.Info("- Running migration 22 backfill book_authors (&-split) and book_narrators")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
		slog.Info("- Splitting author", "value", ja.name, "value", ja.bookID, "value", len(parts))

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
				slog.Info("- Created new author", "value", name, "value", indivAuthorID)
			} else if err != nil {
				return fmt.Errorf("migration 22: lookup author %q: %w", name, err)
			} else {
				indivAuthorID = existingID
				slog.Info("- Found existing author", "value", name, "value", indivAuthorID)
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
					slog.Info("- Warning could not update books.author_id for book", "value", ja.bookID, "error", err)
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

	slog.Info("- Found", "value", len(narBooks))

	for _, nb := range narBooks {
		parts := strings.Split(nb.narrator, " & ")
		slog.Info("- Backfilling narrators for book", "value", nb.bookID, "value", nb.narrator, "value", len(parts))

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
				slog.Info("- Created new narrator", "value", name, "value", narratorID)
			} else if err != nil {
				return fmt.Errorf("migration 22: lookup narrator %q: %w", name, err)
			} else {
				narratorID = existingID
				slog.Info("- Found existing narrator", "value", name, "value", narratorID)
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

	slog.Info("- Migration 22 complete book_authors and book_narrators backfilled")
	return nil
}

// migration023Up adds metadata_updated_at and last_written_at timestamp columns to books table.
// metadata_updated_at is set only when user-visible metadata changes; last_written_at is set
// when metadata is written back to audio files on disk.
func migration023Up(store Store) error {
	slog.Info("- Adding metadata_updated_at and last_written_at to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
		return nil
	}

	alterStatements := []string{
		"ALTER TABLE books ADD COLUMN metadata_updated_at DATETIME",
		"ALTER TABLE books ADD COLUMN last_written_at DATETIME",
	}

	for _, stmt := range alterStatements {
		slog.Info("executing statement", "stmt", stmt)
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping")
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

	slog.Info("- metadata_updated_at and last_written_at added successfully")
	return nil
}

// migration024Up adds metadata_review_status column to books table.
func migration024Up(store Store) error {
	slog.Info("- Adding metadata_review_status to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration")
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
		slog.Info("- Column already exists, skipping")
		return nil
	}

	_, err = sqliteStore.db.Exec(`ALTER TABLE books ADD COLUMN metadata_review_status TEXT`)
	if err != nil {
		return fmt.Errorf("failed to add metadata_review_status column: %w", err)
	}

	slog.Info("- metadata_review_status added successfully")
	return nil
}

// migration025Up adds asin column to books table.
func migration025Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Skipping migration 25 for non-SQLite store")
		return nil
	}

	var count int
	err := sqliteStore.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('books') WHERE name='asin'`).Scan(&count)
	if err == nil && count > 0 {
		slog.Info("- Column already exists, skipping")
		return nil
	}

	_, err = sqliteStore.db.Exec(`ALTER TABLE books ADD COLUMN asin TEXT`)
	if err != nil {
		return fmt.Errorf("failed to add asin column: %w", err)
	}

	slog.Info("- asin added successfully")
	return nil
}

// migration026Up creates book_tombstones table for safe deletion pattern.
func migration026Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Skipping migration 26 for non-SQLite store (PebbleDB uses key prefix)")
		return nil
	}

	_, err := sqliteStore.db.Exec(`CREATE TABLE IF NOT EXISTS book_tombstones (
		id TEXT PRIMARY KEY,
		data TEXT NOT NULL,
		created_at DATETIME DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("failed to create book_tombstones table: %w", err)
	}

	slog.Info("- book_tombstones table created successfully")
	return nil
}

// migration027Up adds result_data column to operations table.
func migration027Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Skipping migration 27 for non-SQLite store (PebbleDB uses JSON)")
		return nil
	}

	_, err := sqliteStore.db.Exec(`ALTER TABLE operations ADD COLUMN result_data TEXT`)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate column") {
			slog.Info("- result_data column already exists")
			return nil
		}
		return fmt.Errorf("failed to add result_data column: %w", err)
	}

	slog.Info("- result_data column added to operations")
	return nil
}

// migration028Up adds external provider ID columns to books table.
func migration028Up(store Store) error {
	slog.Info("- Adding external provider ID columns (open_library_id, hardcover_id, google_books_id)")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	columns := []string{
		"ALTER TABLE books ADD COLUMN open_library_id TEXT",
		"ALTER TABLE books ADD COLUMN hardcover_id TEXT",
		"ALTER TABLE books ADD COLUMN google_books_id TEXT",
	}

	for _, stmt := range columns {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column") {
				continue
			}
			return fmt.Errorf("migration 28 failed: %w", err)
		}
	}

	slog.Info("- External provider ID columns added successfully")
	return nil
}

func migration029Up(store Store) error {
	slog.Info("- Creating operation_changes table for undo/rollback tracking")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS operation_changes (
			id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL,
			book_id TEXT NOT NULL,
			change_type TEXT NOT NULL,
			field_name TEXT,
			old_value TEXT,
			new_value TEXT,
			reverted_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (operation_id) REFERENCES operations(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_changes_op ON operation_changes(operation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_changes_book ON operation_changes(book_id)`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 29 failed: %w", err)
		}
	}

	slog.Info("- operation_changes table created successfully")
	return nil
}

func migration030Up(store Store) error {
	slog.Info("- Adding file_hash column to book_segments for auto-relinking")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`ALTER TABLE book_segments ADD COLUMN file_hash TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_book_segments_file_hash ON book_segments(file_hash)`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping", "value", stmt)
				continue
			}
			return fmt.Errorf("migration 30 failed: %w", err)
		}
	}

	slog.Info("- file_hash column added to book_segments successfully")
	return nil
}

func migration031Up(store Store) error {
	slog.Info("- Adding system_activity_log table and logs_pruned flag")

	sqlStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB handles this via prefix keys
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS system_activity_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_activity_source ON system_activity_log(source)`,
		`CREATE INDEX IF NOT EXISTS idx_system_activity_created ON system_activity_log(created_at)`,
		`ALTER TABLE operations ADD COLUMN logs_pruned BOOLEAN DEFAULT 0`,
	}
	for _, stmt := range statements {
		if _, err := sqlStore.db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migration 31: %w", err)
			}
		}
	}

	slog.Info("- system_activity_log table created successfully")
	return nil
}

// migration032Up adds scan cache columns for incremental scanning
func migration032Up(store Store) error {
	slog.Info("- Adding scan cache columns for incremental scanning")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`ALTER TABLE books ADD COLUMN last_scan_mtime INTEGER DEFAULT NULL`,
		`ALTER TABLE books ADD COLUMN last_scan_size INTEGER DEFAULT NULL`,
		`ALTER TABLE books ADD COLUMN needs_rescan BOOLEAN DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_books_scan_cache ON books(file_path, last_scan_mtime, last_scan_size)`,
		`CREATE INDEX IF NOT EXISTS idx_books_needs_rescan ON books(needs_rescan) WHERE needs_rescan = 1`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				slog.Info("- Column already exists, skipping", "value", stmt)
				continue
			}
			return fmt.Errorf("migration 32 failed: %w", err)
		}
	}

	slog.Info("- Scan cache columns added to books successfully")
	return nil
}

// migration033Up creates the deferred_itunes_updates table for storing
// transcode path changes that should be applied on the next iTunes sync.
func migration033Up(store Store) error {
	slog.Info("- Creating deferred_itunes_updates table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS deferred_itunes_updates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id TEXT NOT NULL,
			persistent_id TEXT NOT NULL,
			old_path TEXT NOT NULL,
			new_path TEXT NOT NULL,
			update_type TEXT NOT NULL DEFAULT 'transcode',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			applied_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_deferred_itunes_pending ON deferred_itunes_updates(applied_at) WHERE applied_at IS NULL`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 33 failed: %w", err)
		}
	}

	slog.Info("- deferred_itunes_updates table created successfully")
	return nil
}

// migration034Up creates the external_id_map table for mapping external
// identifiers (iTunes PIDs, Audible ASINs, etc.) to book IDs.
func migration034Up(store Store) error {
	slog.Info("- Creating external_id_map table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS external_id_map (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			external_id TEXT NOT NULL,
			book_id TEXT NOT NULL,
			track_number INTEGER,
			file_path TEXT,
			tombstoned INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ext_id_source_eid ON external_id_map(source, external_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ext_id_book ON external_id_map(book_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ext_id_tombstone ON external_id_map(source, tombstoned) WHERE tombstoned = 0`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 34 failed: %w", err)
		}
	}

	slog.Info("- external_id_map table created successfully")
	return nil
}

// migration035Up creates the book_path_history table for tracking file
// rename/move operations over time.
func migration035Up(store Store) error {
	slog.Info("- Creating book_path_history table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS book_path_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id TEXT NOT NULL,
			old_path TEXT NOT NULL,
			new_path TEXT NOT NULL,
			change_type TEXT NOT NULL DEFAULT 'rename',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_path_history_book ON book_path_history(book_id)`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 35 failed: %w", err)
		}
	}

	slog.Info("- book_path_history table created successfully")
	return nil
}

// migration036Up adds the genre column to the books table.
func migration036Up(store Store) error {
	slog.Info("- Adding genre column to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	// Check if column already exists (idempotent)
	rows, err := sqliteStore.db.Query("PRAGMA table_info(books)")
	if err != nil {
		return fmt.Errorf("migration 36: failed to read table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("migration 36: failed to scan column info: %w", err)
		}
		if name == "genre" {
			slog.Info("- genre column already exists, skipping")
			return nil
		}
	}

	if _, err := sqliteStore.db.Exec(`ALTER TABLE books ADD COLUMN genre TEXT`); err != nil {
		return fmt.Errorf("migration 36 failed: %w", err)
	}

	slog.Info("- genre column added successfully")
	return nil
}

// migration038Up adds itunes_path column to books table.
func migration038Up(store Store) error {
	slog.Info("- Adding itunes_path column to books table")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	_, err := sqliteStore.db.Exec("ALTER TABLE books ADD COLUMN itunes_path TEXT")
	if err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			slog.Info("- Column itunes_path already exists, skipping")
			return nil
		}
		return fmt.Errorf("migration 38 failed: %w", err)
	}

	slog.Info("- itunes_path column added successfully")
	return nil
}

// migration037Up creates the book_tags table for user-defined tags.
func migration037Up(store Store) error {
	slog.Info("- Creating book_tags table for user-defined tags")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS book_tags (
			book_id TEXT NOT NULL,
			tag TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (book_id, tag)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_book_tags_tag ON book_tags(tag)`,
	}

	for _, stmt := range statements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 37 failed: %w", err)
		}
	}

	slog.Info("- book_tags table created successfully")
	return nil
}

// migration039Up creates the book_files table and migrates data from book_segments.
// book_files uses ULID string book_id (matching books.id directly), whereas
// book_segments used a legacy CRC32 numeric hash of the ULID as book_id.
func migration039Up(store Store) error {
	slog.Info("- Creating book_files table and migrating book_segments data")

	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		slog.Info("- Non-SQLite store detected, skipping SQL migration (PebbleDB uses JSON)")
		return nil
	}

	// Create table and indexes
	ddlStatements := []string{
		`CREATE TABLE IF NOT EXISTS book_files (
			id TEXT PRIMARY KEY,
			book_id TEXT NOT NULL,
			file_path TEXT NOT NULL,
			original_filename TEXT,
			itunes_path TEXT,
			itunes_persistent_id TEXT,
			track_number INTEGER,
			track_count INTEGER,
			disc_number INTEGER,
			disc_count INTEGER,
			title TEXT,
			format TEXT,
			codec TEXT,
			duration INTEGER,
			file_size INTEGER,
			bitrate_kbps INTEGER,
			sample_rate_hz INTEGER,
			channels INTEGER,
			bit_depth INTEGER,
			file_hash TEXT,
			original_file_hash TEXT,
			missing INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_book_id ON book_files(book_id)`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_itunes_pid ON book_files(itunes_persistent_id) WHERE itunes_persistent_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_file_hash ON book_files(file_hash) WHERE file_hash IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_file_path ON book_files(file_path)`,
	}

	for _, stmt := range ddlStatements {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 39: failed to execute DDL: %w", err)
		}
	}

	// Migrate data from book_segments to book_files.
	// book_segments.book_id is a CRC32 of the ULID string (numeric legacy field).
	// We need to build a CRC32(book.id) → book.id map to resolve the relationship.
	rows, err := sqliteStore.db.Query("SELECT id FROM books")
	if err != nil {
		return fmt.Errorf("migration 39: failed to query books: %w", err)
	}
	crcToULID := make(map[uint32]string)
	for rows.Next() {
		var bookID string
		if err := rows.Scan(&bookID); err != nil {
			rows.Close()
			return fmt.Errorf("migration 39: failed to scan book id: %w", err)
		}
		crc := crc32.ChecksumIEEE([]byte(bookID))
		crcToULID[crc] = bookID
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migration 39: books query error: %w", err)
	}

	// Fetch all book_segments
	segRows, err := sqliteStore.db.Query(`
		SELECT id, book_id, file_path, format, size_bytes, duration_seconds,
		       track_number, total_tracks, file_hash, active, created_at, updated_at
		FROM book_segments
	`)
	if err != nil {
		// Table may not exist on a fresh DB — that is fine.
		if strings.Contains(err.Error(), "no such table") {
			slog.Info("- No book_segments table found, skipping data migration")
			slog.Info("- book_files table created successfully")
			return nil
		}
		return fmt.Errorf("migration 39: failed to query book_segments: %w", err)
	}
	defer segRows.Close()

	type segRow struct {
		id              string
		bookIDNumeric   int64
		filePath        string
		format          string
		sizeBytes       int64
		durationSeconds int
		trackNumber     sql.NullInt64
		totalTracks     sql.NullInt64
		fileHash        sql.NullString
		active          int
		createdAt       time.Time
		updatedAt       time.Time
	}

	var segments []segRow
	for segRows.Next() {
		var s segRow
		if err := segRows.Scan(
			&s.id, &s.bookIDNumeric, &s.filePath, &s.format,
			&s.sizeBytes, &s.durationSeconds,
			&s.trackNumber, &s.totalTracks,
			&s.fileHash, &s.active, &s.createdAt, &s.updatedAt,
		); err != nil {
			return fmt.Errorf("migration 39: failed to scan segment row: %w", err)
		}
		segments = append(segments, s)
	}
	if err := segRows.Err(); err != nil {
		return fmt.Errorf("migration 39: segment rows error: %w", err)
	}

	skipped := 0
	inserted := 0
	for _, s := range segments {
		ulidBookID, ok := crcToULID[uint32(s.bookIDNumeric)]
		if !ok {
			slog.Info("- migration 39 no ULID found for CRC32", "value", s.bookIDNumeric, "value", s.id)
			skipped++
			continue
		}

		missing := 0
		if s.active == 0 {
			missing = 1
		}

		// Duration stored in book_files as milliseconds; book_segments stores seconds.
		durationMs := s.durationSeconds * 1000

		var trackNumber, totalTracks *int64
		if s.trackNumber.Valid {
			trackNumber = &s.trackNumber.Int64
		}
		if s.totalTracks.Valid {
			totalTracks = &s.totalTracks.Int64
		}

		var fileHash *string
		if s.fileHash.Valid {
			fileHash = &s.fileHash.String
		}

		_, err := sqliteStore.db.Exec(`
			INSERT OR IGNORE INTO book_files
				(id, book_id, file_path, format, file_size, duration,
				 track_number, track_count, file_hash, missing, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			s.id, ulidBookID, s.filePath, s.format, s.sizeBytes, durationMs,
			trackNumber, totalTracks, fileHash, missing, s.createdAt, s.updatedAt,
		)
		if err != nil {
			return fmt.Errorf("migration 39: failed to insert book_file %s: %w", s.id, err)
		}
		inserted++
	}

	slog.Info("- book_files table created successfully (inserted", "count", inserted, "count", skipped)
	return nil
}

// migration040Up adds last_organize_operation_id and last_organized_at columns to books.
func migration040Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // no-op for non-SQLite stores
	}

	slog.Info("- Adding last_organize_operation_id and last_organized_at to books table")
	stmts := []string{
		`ALTER TABLE books ADD COLUMN last_organize_operation_id TEXT`,
		`ALTER TABLE books ADD COLUMN last_organized_at DATETIME`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue // already applied
			}
			return fmt.Errorf("migration 40: %w", err)
		}
	}
	slog.Info("- last_organize_operation_id and last_organized_at added successfully")
	return nil
}

// migration041Up adds itunes_sync_status column to books for tracking whether
// each book's metadata has been written back to the iTunes library.
// Values: "synced" (up-to-date), "dirty" (changed), "unlinked" (no iTunes), nil (unknown).
func migration041Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB handles schema differently
	}
	slog.Info("- Adding itunes_sync_status column to books table")

	if _, err := sqliteStore.db.Exec(
		`ALTER TABLE books ADD COLUMN itunes_sync_status TEXT`,
	); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			slog.Info("- Column itunes_sync_status already exists, skipping")
			return nil
		}
		return fmt.Errorf("migration 41: %w", err)
	}

	// Backfill: books with an iTunes PID start as "synced" (they came from iTunes).
	// Books without a PID are left as NULL (unlinked).
	result, err := sqliteStore.db.Exec(
		`UPDATE books SET itunes_sync_status = 'synced' WHERE itunes_persistent_id IS NOT NULL AND itunes_persistent_id != ''`,
	)
	if err != nil {
		return fmt.Errorf("migration 41 backfill: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		slog.Info("- Backfilled", "count", rows)
	}

	slog.Info("- itunes_sync_status column added successfully")
	return nil
}

// migration042Up drops dead tables and adds missing indexes for common query patterns.
func migration042Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	slog.Info("- Dropping dead tables and adding missing indexes")

	stmts := []string{
		// Drop dead tables
		`DROP TABLE IF EXISTS audiobook_source_paths`,
		// Don't drop book_segments yet — deprecate interface first, drop in a future migration

		// Missing indexes for common query patterns
		`CREATE INDEX IF NOT EXISTS idx_books_itunes_sync_status ON books(itunes_sync_status) WHERE itunes_sync_status IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_books_dirty_primary ON books(itunes_sync_status, is_primary_version) WHERE itunes_sync_status = 'dirty'`,
		`CREATE INDEX IF NOT EXISTS idx_metadata_changes_book_time ON metadata_changes_history(book_id, changed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_path_history_book_time ON book_path_history(book_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_book_active ON book_files(book_id, missing)`,
		`CREATE INDEX IF NOT EXISTS idx_ext_id_book_source ON external_id_map(book_id, source)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			// Non-fatal: some indexes may already exist or tables may not exist
			slog.Warn("migration warning migration 42", "error", err)
		}
	}

	slog.Info("- Dead tables dropped and missing indexes added")
	return nil
}

// migration043Up drops the deprecated book_segments table.
// Data was migrated to book_files in migration 39. No production code reads this table.
func migration043Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	slog.Info("- Dropping deprecated book_segments table and adding remaining indexes")
	if _, err := sqliteStore.db.Exec(`DROP TABLE IF EXISTS book_segments`); err != nil {
		slog.Warn("migration warning migration 43", "error", err)
	}
	if _, err := sqliteStore.db.Exec(`DROP INDEX IF EXISTS idx_book_segments_book`); err != nil {
		// non-fatal
	}
	if _, err := sqliteStore.db.Exec(`DROP INDEX IF EXISTS idx_book_segments_hash`); err != nil {
		// non-fatal
	}
	// Additional indexes
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_operations_type_status ON operations(type, status)`,
		`CREATE INDEX IF NOT EXISTS idx_metadata_changes_time ON metadata_changes_history(changed_at DESC)`,
	} {
		if _, err := sqliteStore.db.Exec(idx); err != nil {
			slog.Warn("migration warning migration 43 index", "error", err)
		}
	}

	slog.Info("- book_segments dropped and additional indexes added")
	return nil
}

// migration044Up adds PID lifecycle tracking to external_id_map.
// provenance: "itunes" (imported), "generated" (we created), "recycled" (reused)
// removed_at: timestamp when we sent a remove to the ITL; null while live
func migration044Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	stmts := []string{
		`ALTER TABLE external_id_map ADD COLUMN provenance TEXT`,
		`ALTER TABLE external_id_map ADD COLUMN removed_at DATETIME`,
		`CREATE INDEX IF NOT EXISTS idx_ext_id_removed ON external_id_map(source, removed_at) WHERE removed_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_ext_id_provenance ON external_id_map(source, provenance) WHERE provenance IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 44", "error", err)
		}
	}
	// Backfill existing rows as itunes-imported
	if _, err := sqliteStore.db.Exec(`UPDATE external_id_map SET provenance = 'itunes' WHERE provenance IS NULL AND source = 'itunes'`); err != nil {
		slog.Warn("migration warning migration 44 backfill", "error", err)
	}
	slog.Info("- Added provenance and removed_at to external_id_map")
	return nil
}

// migration045Up creates the operation_results table for structured per-book operation output.
func migration045Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS operation_results (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            operation_id TEXT NOT NULL,
            book_id TEXT NOT NULL,
            result_json TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'matched',
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_op_results_op ON operation_results(operation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_op_results_book ON operation_results(operation_id, book_id)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 45", "error", err)
		}
	}
	slog.Info("- Created operation_results table")
	return nil
}

// migration046Up creates the book_alternative_titles table. This powers
// two things:
//
//  1. Dedup Layer 1 exact-title matching can iterate every alt title on
//     both sides of a comparison, so manga romaji vs English, subtitle
//     reordering, or rebrands all collapse to the same normalized form.
//  2. Library search can index alt titles alongside the primary title,
//     so searching for either variant finds the book.
//
// Schema:
//
//	id         — autoincrement primary key
//	book_id    — books.id (cascade on delete)
//	title      — the alternate title string
//	source     — where the alt came from: 'user', 'metadata_fetch',
//	             'auto_ampersand', 'itunes_import', 'manga_romaji', etc.
//	language   — optional ISO-639 code for the language variant
//	created_at — audit trail
//
// UNIQUE(book_id, title) prevents dup rows for the same variant.
func migration046Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS book_alternative_titles (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            book_id TEXT NOT NULL,
            title TEXT NOT NULL,
            source TEXT NOT NULL DEFAULT 'user',
            language TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(book_id, title)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_book_alt_titles_book ON book_alternative_titles(book_id)`,
		`CREATE INDEX IF NOT EXISTS idx_book_alt_titles_title ON book_alternative_titles(title)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 46", "error", err)
		}
	}
	slog.Info("- Created book_alternative_titles table")
	return nil
}

// migration047Up adds a `source` column to book_tags so system-
// applied tags (dedup:merge-survivor:llm-auto, metadata:source:*,
// ...) can be distinguished from user-applied tags. PebbleStore
// stores the same field in the serialized value; this migration
// keeps SQLite as a first-class store option in sync.
//
// Example sources and the tags they emit:
//
//	source='user'   — tag was added via the UI or API by a human
//	source='system' — tag was added automatically by the server:
//	    dedup:merge-survivor[:auto-hash|auto-isbn|llm-auto]
//	    dedup:duration-match
//	    dedup:duration-abridged
//	    metadata:source:{audible,hardcover,google_books,openlibrary,audnexus}
//	    metadata:language:{en,es,fr,...} (from applied metadata)
//	    import:scan (future), organize:applied (future), ...
//
// The column defaults to 'user' so every existing row stays valid
// without a data migration, and existing AddBookTag callers don't
// need to be touched — they just keep writing user-sourced tags.
//
// An index on source lets "tag:metadata:source:google_books AND
// source=system" filter cheaply for the metadata-upgrade workflow.
func migration047Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	stmts := []string{
		`ALTER TABLE book_tags ADD COLUMN source TEXT NOT NULL DEFAULT 'user'`,
		`CREATE INDEX IF NOT EXISTS idx_book_tags_source ON book_tags(source)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			// ALTER TABLE ADD COLUMN fails if the column already
			// exists — SQLite has no IF NOT EXISTS for ALTER. Log
			// and continue so a re-run is idempotent.
			slog.Warn("migration warning migration 47", "error", err)
		}
	}
	slog.Info("- Added source column to book_tags")
	return nil
}

// migration048Up adds parallel `author_tags` and `series_tags`
// tables so tagging works at every entity level, not just books.
// Motivating use cases:
//
//   - Mark an author as language-locked: tag with `language:en`,
//     then the metadata fetcher and dedup engine skip candidates
//     in other languages without inspecting every book.
//   - Mark a series as `completed` / `on-hold` / `dropped` for
//     read-list management.
//   - Future "policy" tags: `policy:english-only` on an author or
//     series binds a named settings bundle to every book that
//     inherits through the author/series hierarchy.
//   - System-applied provenance tags on authors/series after a
//     merge or metadata apply — same namespace as book_tags.
//
// Schema mirrors book_tags after migration 47: primary key on
// (entity_id, tag), `source` column defaulting to 'user', plus
// indexes on tag and source for reverse lookup and filtering.
func migration048Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS author_tags (
            author_id INTEGER NOT NULL,
            tag TEXT NOT NULL,
            source TEXT NOT NULL DEFAULT 'user',
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (author_id, tag)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_author_tags_tag ON author_tags(tag)`,
		`CREATE INDEX IF NOT EXISTS idx_author_tags_source ON author_tags(source)`,
		`CREATE TABLE IF NOT EXISTS series_tags (
            series_id INTEGER NOT NULL,
            tag TEXT NOT NULL,
            source TEXT NOT NULL DEFAULT 'user',
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (series_id, tag)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_series_tags_tag ON series_tags(tag)`,
		`CREATE INDEX IF NOT EXISTS idx_series_tags_source ON series_tags(source)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 48", "error", err)
		}
	}
	slog.Info("- Created author_tags and series_tags tables")
	return nil
}

// migration049Up adds acoustid_fingerprint and acoustid_duration columns to
// book_files. These store AcoustID fingerprints (from fpcalc) for
// content-based matching that survives metadata rewrites and file moves.
func migration049Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB is schema-free; columns live on the struct
	}
	stmts := []string{
		`ALTER TABLE book_files ADD COLUMN acoustid_fingerprint TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_duration INTEGER`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_acoustid ON book_files(acoustid_fingerprint) WHERE acoustid_fingerprint IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 49", "error", err)
		}
	}
	slog.Info("- Added acoustid_fingerprint, acoustid_duration to book_files")
	return nil
}

// migration051Up adds quarantine_reason and quarantined_at columns to books.
func migration051Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free, fields live on the struct
	}
	stmts := []string{
		`ALTER TABLE books ADD COLUMN quarantine_reason TEXT`,
		`ALTER TABLE books ADD COLUMN quarantined_at TIMESTAMP`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 051", "error", err)
		}
	}
	slog.Info("- Added quarantine_reason, quarantined_at to books")
	return nil
}

// migration052Up creates the ai_jobs tracking table and ai_job_payloads
// blob table used by the internal/ai/aijobs package to route bulk LLM work
// through the OpenAI Batch API.
func migration052Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ai_jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			batch_id TEXT,
			custom_id_prefix TEXT NOT NULL,
			status TEXT NOT NULL,
			item_count INTEGER NOT NULL,
			success_count INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			row_errors TEXT,
			error_msg TEXT,
			submitted_at TIMESTAMP,
			completed_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_jobs_status_created ON ai_jobs(status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_jobs_type_created ON ai_jobs(type, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_jobs_batch_id ON ai_jobs(batch_id) WHERE batch_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS ai_job_payloads (
			job_id TEXT PRIMARY KEY,
			items_json BLOB NOT NULL,
			FOREIGN KEY (job_id) REFERENCES ai_jobs(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 52", "error", err)
		}
	}
	slog.Info("- Created ai_jobs, ai_job_payloads")
	return nil
}

// migration050Up replaces the single acoustid_fingerprint/acoustid_duration
// columns with 7 per-segment columns: acoustid_seg0 through acoustid_seg6.
// The old acoustid_fingerprint column is retained (SQLite cannot drop columns
// in older versions) but is no longer read or written by the application.
// PebbleDB is schema-free; the new segment fields live on the struct.
func migration050Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: no schema change needed
	}
	stmts := []string{
		`ALTER TABLE book_files ADD COLUMN acoustid_seg0 TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_seg1 TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_seg2 TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_seg3 TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_seg4 TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_seg5 TEXT`,
		`ALTER TABLE book_files ADD COLUMN acoustid_seg6 TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_acoustid_seg0 ON book_files(acoustid_seg0) WHERE acoustid_seg0 IS NOT NULL`,
		// Keep old acoustid_fingerprint column — SQLite can't drop columns easily.
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 50", "error", err)
		}
	}
	slog.Info("- Added acoustid_seg0–acoustid_seg6 to book_files")
	return nil
}

// migration053Up adds post_metadata_hash to book_files for recording the SHA-256
// of the file after a metadata tag write. This allows the pre-write identity
// (original_file_hash) to always be recoverable even after tags are modified.
func migration053Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free, struct carries the new field
	}
	stmts := []string{
		`ALTER TABLE book_files ADD COLUMN post_metadata_hash TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_post_metadata_hash ON book_files(post_metadata_hash) WHERE post_metadata_hash IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 53", "error", err)
		}
	}
	slog.Info("- Added post_metadata_hash to book_files")
	return nil
}

// migration054Up creates the metadata_rejections table for auditing every
// candidate that was rejected for a book (user action, below-threshold score,
// duration mismatch, wrong language, or skipped in the UI).
func migration054Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: no schema change needed
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS metadata_rejections (
			id TEXT PRIMARY KEY,
			book_id TEXT NOT NULL,
			source TEXT NOT NULL,
			candidate_asin TEXT,
			candidate_isbn TEXT,
			candidate_title TEXT,
			candidate_author TEXT,
			rejection_reason TEXT NOT NULL,
			score REAL,
			rejected_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metadata_rejections_book_id ON metadata_rejections(book_id)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 54", "error", err)
		}
	}
	slog.Info("- Created metadata_rejections table")
	return nil
}

// migration055Up adds metadata_source_hash to books for metadata-based dedup (MATCH-1).
// The value is sha256("{source}:{canonical_id}"), e.g. sha256("audible:B0XXXXXXXX").
// Identical hashes mean two book records were applied from the exact same external record
// and are almost certainly duplicates.
func migration055Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free, field lives on struct
	}
	stmts := []string{
		`ALTER TABLE books ADD COLUMN metadata_source_hash TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_books_metadata_source_hash ON books(metadata_source_hash) WHERE metadata_source_hash IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 055", "error", err)
		}
	}
	slog.Info("- Added metadata_source_hash to books, created index idx_books_metadata_source_hash")
	return nil
}

// migration056Up adds merged_into_book_id to books for chapter consolidation (MATCH-2).
// When MergeChapterBooks() absorbs a chapter file into a consolidated book, the
// source book row has is_primary_version set to 0 and merged_into_book_id set to
// the primary book's ID so the merge is auditable.
func migration056Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free, field lives on struct
	}
	stmts := []string{
		`ALTER TABLE books ADD COLUMN merged_into_book_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_books_merged_into ON books(merged_into_book_id) WHERE merged_into_book_id IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 056", "error", err)
		}
	}
	slog.Info("- Added merged_into_book_id to books, created index idx_books_merged_into")
	return nil
}

// migration057Up adds a unique index on (file_hash, source_import_path) to prevent
// duplicate audiobook records for the same physical file within each library context.
// The index is partial (WHERE file_hash IS NOT NULL) to avoid affecting existing rows
// with NULL or empty hashes during migration.
func migration057Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free, field lives on struct
	}
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_audiobooks_file_hash_library ON books (file_hash, source_import_path) WHERE file_hash IS NOT NULL AND file_hash != ''`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 057", "error", err)
		}
	}
	slog.Info("- Added unique index on (file_hash, source_import_path)")
	return nil
}

// migration058Up adds book signature columns for unified per-book audio fingerprint
// (spec: 2026-05-03-unified-book-fingerprint.md). These columns synthesize a
// deterministic book-level fingerprint from the per-file 7-segment chromaprints,
// enabling dedup matching across different file splits (e.g., 1 .m4b vs 30 .mp3s).
func migration058Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free, fields live on struct
	}
	stmts := []string{
		`ALTER TABLE books ADD COLUMN book_sig_v1 TEXT`,
		`ALTER TABLE books ADD COLUMN book_sig_segments INTEGER`,
		`ALTER TABLE books ADD COLUMN book_sig_built_at DATETIME`,
		`CREATE INDEX IF NOT EXISTS idx_books_book_sig_v1 ON books(book_sig_v1) WHERE book_sig_v1 IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 058", "error", err)
		}
	}
	slog.Info("- Added book_sig_v1, book_sig_segments, book_sig_built_at to books")
	return nil
}

// migration059Up adds the UOS v2 core schema described in
// docs/superpowers/specs/2026-05-04-unified-operations-system.md §2.1.
func migration059Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}

	stmts := []string{
		`CREATE TABLE op_definitions_v2 (
    id              TEXT PRIMARY KEY,
    plugin          TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    description     TEXT NOT NULL,
    capabilities    TEXT NOT NULL,
    permissions     TEXT NOT NULL,
    cancellable     BOOLEAN NOT NULL,
    isolate         BOOLEAN NOT NULL,
    resume_policy   TEXT NOT NULL,
    schedule_cron   TEXT,
    triggers        TEXT NOT NULL,
    depends_on      TEXT NOT NULL,
    phases          TEXT NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    registered_at   TIMESTAMP NOT NULL
)`,
		`CREATE TABLE operations_v2 (
    id                  TEXT PRIMARY KEY,
    def_id              TEXT NOT NULL,
    plugin              TEXT NOT NULL,
    parent_id           TEXT,
    actor_user_id       TEXT,
    trace_id            TEXT NOT NULL,
    span_id             TEXT NOT NULL,
    parent_span_id      TEXT,
    status              TEXT NOT NULL,
    priority            INTEGER NOT NULL,
    progress_current    INTEGER NOT NULL DEFAULT 0,
    progress_total      INTEGER NOT NULL DEFAULT 0,
    progress_message    TEXT NOT NULL DEFAULT '',
    current_phase       TEXT,
    params              TEXT NOT NULL DEFAULT '{}',
    error_message       TEXT,
    result_data         TEXT,
    queued_at           TIMESTAMP NOT NULL,
    started_at          TIMESTAMP,
    completed_at        TIMESTAMP,
    last_progress_at    TIMESTAMP,
    last_checkpoint_at  TIMESTAMP,
    high_water_progress INTEGER NOT NULL DEFAULT 0,
    resume_count        INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE INDEX idx_operations_v2_status ON operations_v2(status, queued_at)`,
		`CREATE INDEX idx_operations_v2_parent ON operations_v2(parent_id)`,
		`CREATE INDEX idx_operations_v2_def    ON operations_v2(def_id, completed_at DESC)`,
		`CREATE TABLE op_logs_v2 (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL,
    level        TEXT NOT NULL,
    message      TEXT NOT NULL,
    attrs        TEXT NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP NOT NULL
)`,
		`CREATE INDEX idx_op_logs_v2_op_time ON op_logs_v2(operation_id, created_at)`,
		`CREATE TABLE op_errors_v2 (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL,
    plugin       TEXT NOT NULL,
    def_id       TEXT NOT NULL,
    message      TEXT NOT NULL,
    attrs        TEXT NOT NULL DEFAULT '{}',
    occurred_at  TIMESTAMP NOT NULL
)`,
		`CREATE INDEX idx_op_errors_v2_def     ON op_errors_v2(def_id, occurred_at DESC)`,
		`CREATE INDEX idx_op_errors_v2_plugin  ON op_errors_v2(plugin, occurred_at DESC)`,
		`CREATE TABLE op_state_v2 (
    operation_id TEXT PRIMARY KEY,
    phase        TEXT,
    state_blob   BLOB NOT NULL,
    schema_version INTEGER NOT NULL,
    written_at   TIMESTAMP NOT NULL
)`,
		`CREATE TABLE op_strikes_v2 (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    def_id      TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    kind        TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMP NOT NULL
)`,
		`CREATE INDEX idx_op_strikes_v2_def_time ON op_strikes_v2(def_id, occurred_at DESC)`,
		`CREATE TABLE plugin_schema_v2 (
    plugin               TEXT NOT NULL,
    migration_version    INTEGER NOT NULL,
    applied_at           TIMESTAMP NOT NULL,
    PRIMARY KEY (plugin, migration_version)
)`,
		`CREATE TABLE core_schema_meta_v2 (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    core_schema_version INTEGER NOT NULL
)`,
		`INSERT INTO core_schema_meta_v2 (id, core_schema_version) VALUES (1, 1)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 059 failed running %q: %w", stmt, err)
		}
	}
	slog.Info("- Added UOS v2 core schema")
	return nil
}

// migration059Down removes the UOS v2 core schema added by migration059Up.
func migration059Down(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil
	}

	stmts := []string{
		`DROP TABLE IF EXISTS core_schema_meta_v2`,
		`DROP TABLE IF EXISTS plugin_schema_v2`,
		`DROP INDEX IF EXISTS idx_op_strikes_v2_def_time`,
		`DROP TABLE IF EXISTS op_strikes_v2`,
		`DROP TABLE IF EXISTS op_state_v2`,
		`DROP INDEX IF EXISTS idx_op_errors_v2_plugin`,
		`DROP INDEX IF EXISTS idx_op_errors_v2_def`,
		`DROP TABLE IF EXISTS op_errors_v2`,
		`DROP INDEX IF EXISTS idx_op_logs_v2_op_time`,
		`DROP TABLE IF EXISTS op_logs_v2`,
		`DROP INDEX IF EXISTS idx_operations_v2_def`,
		`DROP INDEX IF EXISTS idx_operations_v2_parent`,
		`DROP INDEX IF EXISTS idx_operations_v2_status`,
		`DROP TABLE IF EXISTS operations_v2`,
		`DROP TABLE IF EXISTS op_definitions_v2`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration 059 down failed running %q: %w", stmt, err)
		}
	}
	slog.Info("- Removed UOS v2 core schema")
	return nil
}

// migration060Up adds partial book-signature coverage columns to books and
// structured fingerprint-failure diagnosis columns to book_files.
func migration060Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // Pebble needs no schema migrations
	}
	stmts := []string{
		`ALTER TABLE books ADD COLUMN book_sig_v1_mask           TEXT`,
		`ALTER TABLE books ADD COLUMN book_sig_coverage_pct      INTEGER`,
		// fingerprint_failed_at and organize_method were in the struct but never added to SQLite schema
		`ALTER TABLE book_files ADD COLUMN fingerprint_failed_at     DATETIME`,
		`ALTER TABLE book_files ADD COLUMN organize_method            TEXT`,
		// New diagnosis columns
		`ALTER TABLE book_files ADD COLUMN fingerprint_failure_reason TEXT`,
		`ALTER TABLE book_files ADD COLUMN fingerprint_failure_detail TEXT`,
		`ALTER TABLE book_files ADD COLUMN fingerprint_diagnostic_json TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_book_files_fingerprint_failed ON book_files(fingerprint_failed_at) WHERE fingerprint_failed_at IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			slog.Warn("migration warning migration 060", "error", err)
		}
	}
	slog.Info("+ Added partial sig mask/coverage to books, diagnosis columns to book_files")
	return nil
}
