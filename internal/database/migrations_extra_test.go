// file: internal/database/migrations_extra_test.go
// version: 1.2.0
// guid: 67d3f1c5-8c24-4a3c-9a79-35fb6d68fdd9

package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGetCurrentVersionWithPreference(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	version := DatabaseVersion{Version: 3, UpdatedAt: time.Now()}
	data, err := json.Marshal(version)
	if err != nil {
		t.Fatalf("marshal version failed: %v", err)
	}
	if err := store.SetUserPreference("db_version", string(data)); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}
	got, err := getCurrentVersion(store)
	if err != nil {
		t.Fatalf("getCurrentVersion failed: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected version 3, got %d", got)
	}
}

func TestGetCurrentVersionInvalidPreference(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	if err := store.SetUserPreference("db_version", "not-json"); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}
	if _, err := getCurrentVersion(store); err == nil {
		t.Fatal("expected getCurrentVersion to fail on invalid JSON")
	}
}

func TestMigration007UpWithLegacyTable(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	sqliteStore := store.(*SQLiteStore)

	if _, err := sqliteStore.db.Exec("DROP TABLE IF EXISTS import_paths"); err != nil {
		t.Fatalf("failed to drop import_paths: %v", err)
	}
	if _, err := sqliteStore.db.Exec("DROP INDEX IF EXISTS idx_import_paths_path"); err != nil {
		t.Fatalf("failed to drop idx_import_paths_path: %v", err)
	}
	if _, err := sqliteStore.db.Exec(`
		CREATE TABLE library_folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_scan DATETIME,
			book_count INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		t.Fatalf("failed to create library_folders: %v", err)
	}
	if _, err := sqliteStore.db.Exec("CREATE INDEX idx_library_folders_path ON library_folders(path)"); err != nil {
		t.Fatalf("failed to create legacy index: %v", err)
	}

	if err := migration007Up(store); err != nil {
		t.Fatalf("migration007Up failed: %v", err)
	}
}

// TestGetMigrationHistory tests the GetMigrationHistory function.
func TestGetMigrationHistory(t *testing.T) {
	t.Run("returns empty list when no migrations", func(t *testing.T) {
		store, cleanup := setupTestDB(t)
		defer cleanup()

		records, err := GetMigrationHistory(store)
		if err != nil {
			t.Fatalf("GetMigrationHistory failed: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("expected 0 migration records, got %d", len(records))
		}
	})

	t.Run("returns migration records", func(t *testing.T) {
		store, cleanup := setupTestDB(t)
		defer cleanup()

		// Add some migration records as preferences
		migrations := []MigrationRecord{
			{
				Version:     1,
				Description: "initial_schema",
				AppliedAt:   time.Now().Add(-2 * time.Hour),
			},
			{
				Version:     2,
				Description: "add_tags_table",
				AppliedAt:   time.Now().Add(-1 * time.Hour),
			},
			{
				Version:     3,
				Description: "add_indexes",
				AppliedAt:   time.Now(),
			},
		}

		for _, migration := range migrations {
			data, err := json.Marshal(migration)
			if err != nil {
				t.Fatalf("failed to marshal migration: %v", err)
			}
			key := "migration_" + migration.Description
			if err := store.SetUserPreference(key, string(data)); err != nil {
				t.Fatalf("failed to set migration preference: %v", err)
			}
		}

		// Get migration history
		records, err := GetMigrationHistory(store)
		if err != nil {
			t.Fatalf("GetMigrationHistory failed: %v", err)
		}

		if len(records) != len(migrations) {
			t.Errorf("expected %d migration records, got %d", len(migrations), len(records))
		}

		// Verify records contain expected data
		foundVersions := make(map[int]bool)
		for _, record := range records {
			foundVersions[record.Version] = true
			if record.Description == "" {
				t.Error("migration record has empty description")
			}
			if record.AppliedAt.IsZero() {
				t.Error("migration record has zero AppliedAt time")
			}
		}

		// Check that all expected versions were found
		for _, migration := range migrations {
			if !foundVersions[migration.Version] {
				t.Errorf("migration version %d not found in history", migration.Version)
			}
		}
	})

	t.Run("skips invalid preference values", func(t *testing.T) {
		store, cleanup := setupTestDB(t)
		defer cleanup()

		// Add a migration with invalid JSON
		if err := store.SetUserPreference("migration_invalid", "not-json"); err != nil {
			t.Fatalf("failed to set invalid preference: %v", err)
		}

		// Add a migration with nil value
		if err := store.SetUserPreference("migration_nil", ""); err != nil {
			t.Fatalf("failed to set nil preference: %v", err)
		}

		// Should not error, just skip invalid entries
		records, err := GetMigrationHistory(store)
		if err != nil {
			t.Fatalf("GetMigrationHistory failed: %v", err)
		}

		// Should be 0 since we only added invalid entries
		if len(records) != 0 {
			t.Errorf("expected 0 records with only invalid migrations, got %d", len(records))
		}
	})

	t.Run("ignores non-migration preferences", func(t *testing.T) {
		store, cleanup := setupTestDB(t)
		defer cleanup()

		// Add some non-migration preferences
		if err := store.SetUserPreference("some_other_pref", "value"); err != nil {
			t.Fatalf("failed to set non-migration preference: %v", err)
		}
		if err := store.SetUserPreference("another_pref", "value2"); err != nil {
			t.Fatalf("failed to set another preference: %v", err)
		}

		// Add one valid migration
		migration := MigrationRecord{
			Version:     99,
			Description: "test_migration",
			AppliedAt:   time.Now(),
		}
		data, err := json.Marshal(migration)
		if err != nil {
			t.Fatalf("failed to marshal migration: %v", err)
		}
		if err := store.SetUserPreference("migration_test", string(data)); err != nil {
			t.Fatalf("failed to set migration preference: %v", err)
		}

		// Should only return migration preferences
		records, err := GetMigrationHistory(store)
		if err != nil {
			t.Fatalf("GetMigrationHistory failed: %v", err)
		}

		if len(records) != 1 {
			t.Errorf("expected 1 migration record, got %d", len(records))
		}

		if len(records) > 0 && records[0].Version != 99 {
			t.Errorf("expected migration version 99, got %d", records[0].Version)
		}
	})
}

// TestMigration022_BackfillMultipleAuthorsNarrators verifies that migration 22
// splits existing "&"-joined author names into multiple book_authors rows and
// backfills book_narrators from the legacy books.narrator field.
func TestMigration022_BackfillMultipleAuthorsNarrators(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	s := store.(*SQLiteStore)

	// --- Setup: create two authors joined with "&" ---
	result, err := s.db.Exec(`INSERT INTO authors (name) VALUES (?)`, "Alice Smith & Bob Jones")
	if err != nil {
		t.Fatalf("insert joined author: %v", err)
	}
	joinedAuthorID, _ := result.LastInsertId()

	// Insert a book that references the joined author and has a narrator with "&"
	bookID := "01JTEST000000000000000001"
	narratorStr := "Carol Davis & Dave Evans"
	_, err = s.db.Exec(`
		INSERT INTO books (id, title, author_id, narrator, file_path, format)
		VALUES (?, ?, ?, ?, ?, ?)`,
		bookID, "Test Book", int(joinedAuthorID), narratorStr, "/tmp/test.m4b", "m4b")
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}

	// Seed the existing book_authors row (as migration 15 would have done):
	_, err = s.db.Exec(`INSERT OR IGNORE INTO book_authors (book_id, author_id, role, position) VALUES (?, ?, 'author', 0)`,
		bookID, int(joinedAuthorID))
	if err != nil {
		t.Fatalf("seed book_authors: %v", err)
	}

	// Confirm pre-condition: only 1 row in book_authors, 0 rows in book_narrators
	var baCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM book_authors WHERE book_id = ?`, bookID).Scan(&baCount)
	if baCount != 1 {
		t.Fatalf("pre-condition: expected 1 book_authors row, got %d", baCount)
	}
	var bnCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM book_narrators WHERE book_id = ?`, bookID).Scan(&bnCount)
	if bnCount != 0 {
		t.Fatalf("pre-condition: expected 0 book_narrators rows, got %d", bnCount)
	}

	// --- Run migration 22 ---
	if err := migration022Up(store); err != nil {
		t.Fatalf("migration022Up failed: %v", err)
	}

	// --- Verify authors were split ---
	rows, err := s.db.Query(`
		SELECT a.name, ba.role, ba.position
		FROM book_authors ba
		JOIN authors a ON a.id = ba.author_id
		WHERE ba.book_id = ?
		ORDER BY ba.position`, bookID)
	if err != nil {
		t.Fatalf("query book_authors: %v", err)
	}
	defer rows.Close()

	type authorRow struct {
		name, role string
		pos        int
	}
	var authors []authorRow
	for rows.Next() {
		var ar authorRow
		rows.Scan(&ar.name, &ar.role, &ar.pos)
		authors = append(authors, ar)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("book_authors rows.Err: %v", err)
	}

	if len(authors) != 2 {
		t.Fatalf("expected 2 book_authors rows after migration, got %d: %+v", len(authors), authors)
	}
	if authors[0].name != "Alice Smith" || authors[0].role != "author" || authors[0].pos != 0 {
		t.Errorf("first author wrong: %+v", authors[0])
	}
	if authors[1].name != "Bob Jones" || authors[1].role != "co-author" || authors[1].pos != 1 {
		t.Errorf("second author wrong: %+v", authors[1])
	}

	// --- Verify narrators were backfilled ---
	narRows, err := s.db.Query(`
		SELECT n.name, bn.role, bn.position
		FROM book_narrators bn
		JOIN narrators n ON n.id = bn.narrator_id
		WHERE bn.book_id = ?
		ORDER BY bn.position`, bookID)
	if err != nil {
		t.Fatalf("query book_narrators: %v", err)
	}
	defer narRows.Close()

	type narRow struct {
		name, role string
		pos        int
	}
	var narrators []narRow
	for narRows.Next() {
		var nr narRow
		narRows.Scan(&nr.name, &nr.role, &nr.pos)
		narrators = append(narrators, nr)
	}
	if err := narRows.Err(); err != nil {
		t.Fatalf("book_narrators rows.Err: %v", err)
	}

	if len(narrators) != 2 {
		t.Fatalf("expected 2 book_narrators rows after migration, got %d: %+v", len(narrators), narrators)
	}
	if narrators[0].name != "Carol Davis" || narrators[0].role != "narrator" || narrators[0].pos != 0 {
		t.Errorf("first narrator wrong: %+v", narrators[0])
	}
	if narrators[1].name != "Dave Evans" || narrators[1].role != "co-narrator" || narrators[1].pos != 1 {
		t.Errorf("second narrator wrong: %+v", narrators[1])
	}
}

// TestMigration022_SingleAuthorUntouched verifies that books with a single author
// (no "&") are not modified.
func TestMigration022_SingleAuthorUntouched(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	s := store.(*SQLiteStore)

	result, err := s.db.Exec(`INSERT INTO authors (name) VALUES (?)`, "Solo Author")
	if err != nil {
		t.Fatalf("insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	bookID := "01JTEST000000000000000002"
	_, err = s.db.Exec(`
		INSERT INTO books (id, title, author_id, file_path, format)
		VALUES (?, ?, ?, ?, ?)`,
		bookID, "Solo Book", int(authorID), "/tmp/solo.m4b", "m4b")
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}

	_, err = s.db.Exec(`INSERT OR IGNORE INTO book_authors (book_id, author_id, role, position) VALUES (?, ?, 'author', 0)`,
		bookID, int(authorID))
	if err != nil {
		t.Fatalf("seed book_authors: %v", err)
	}

	if err := migration022Up(store); err != nil {
		t.Fatalf("migration022Up failed: %v", err)
	}

	var baCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM book_authors WHERE book_id = ?`, bookID).Scan(&baCount)
	if baCount != 1 {
		t.Errorf("expected 1 book_authors row for solo author, got %d", baCount)
	}
}

// TestMigration022_Idempotent verifies that running migration 22 twice does not
// produce duplicate rows.
func TestMigration022_Idempotent(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	s := store.(*SQLiteStore)

	result, err := s.db.Exec(`INSERT INTO authors (name) VALUES (?)`, "Foo & Bar")
	if err != nil {
		t.Fatalf("insert author: %v", err)
	}
	joinedAuthorID, _ := result.LastInsertId()

	bookID := "01JTEST000000000000000003"
	_, err = s.db.Exec(`
		INSERT INTO books (id, title, author_id, file_path, format)
		VALUES (?, ?, ?, ?, ?)`,
		bookID, "Idempotent Book", int(joinedAuthorID), "/tmp/idempotent.m4b", "m4b")
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO book_authors (book_id, author_id, role, position) VALUES (?, ?, 'author', 0)`,
		bookID, int(joinedAuthorID))
	if err != nil {
		t.Fatalf("seed book_authors: %v", err)
	}

	// Run twice
	if err := migration022Up(store); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := migration022Up(store); err != nil {
		t.Fatalf("second run: %v", err)
	}

	var baCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM book_authors WHERE book_id = ?`, bookID).Scan(&baCount)
	if baCount != 2 {
		t.Errorf("after two runs, expected exactly 2 book_authors rows (Foo + Bar), got %d", baCount)
	}
}
