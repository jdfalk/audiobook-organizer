// file: internal/database/migrations.go
// version: 1.3.0
// guid: 9a8b7c6d-5e4f-3d2c-1b0a-9f8e7d6c5b4a

package database

import (
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
		Description: "Add library folders and operations tables",
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

// migration002Up adds library folders and operations support
func migration002Up(store Store) error {
	// These structures are already supported by the current store interface
	log.Println("  - Adding library folders and operations support")
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
