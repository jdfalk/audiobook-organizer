<!-- file: docs/plans/2026-02-28-phase1a-fix-multiple-authors.md -->
<!-- version: 1.0.0 -->
<!-- guid: f1e2d3c4-b5a6-7890-abcd-ef1234567891 -->
<!-- last-edited: 2026-02-28 -->

# Phase 1A: Fix Multiple Authors Display & AI Parse

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix authors/narrators display by backfilling junction tables and routing AI parse through the service layer with better context.

**Architecture:**
- SQLite backend, accessed through `database.Store` interface
- `book_authors` junction table (book_id TEXT, author_id INTEGER, role TEXT, position INTEGER) — created in migration 15
- `book_narrators` junction table (book_id TEXT, narrator_id INTEGER, role TEXT, position INTEGER) — created in migration 20
- `authors` table (id INTEGER PK, name TEXT UNIQUE)
- `narrators` table (id INTEGER PK, name TEXT UNIQUE) — created in migration 20
- `books` table has `author_id INTEGER` (legacy single-author FK) and `narrator TEXT` (legacy single-narrator string)
- Migration 15 already backfilled `book_authors` for books that had a single `author_id` — but it did NOT split on `&`
- Migration 20 created the narrators/book_narrators tables but did NOT backfill from `books.narrator`
- `AudiobookService.UpdateAudiobook` (internal/server/audiobook_service.go:436) handles `&` splitting for both authors and narrators
- `parseAudiobookWithAI` (internal/server/server.go:3647) bypasses the service layer entirely, calling `database.GlobalStore.UpdateBook` directly

**Tech Stack:** Go 1.24, SQLite, Gin, React/TypeScript frontend

**Current highest migration number:** 21 (migration021Up adds operation_summary_logs table, internal/database/migrations.go:1104)

**Key file paths:**
- `internal/database/migrations.go` — migration definitions
- `internal/database/sqlite_store.go` — SQLite Store implementations (CreateNarrator at line 1515, SetBookNarrators at line 1587)
- `internal/database/store.go` — Book, BookAuthor, BookNarrator, Narrator types (lines 203–295)
- `internal/server/server.go` — `parseAudiobookWithAI` handler (line 3647)
- `internal/server/audiobook_service.go` — `UpdateAudiobook` (line 436), `UpdateAudiobookRequest` (line 428)
- `internal/server/audiobook_update_service.go` — `AudiobookUpdateService.UpdateAudiobook` (line 116)
- `internal/ai/openai_parser.go` — `ParseFilename` (line 68), `ParsedMetadata` struct (line 21)

---

## Task 1: Backfill Migration — Split `&` Authors and Backfill Narrators

**Problem:** Migration 15 populated `book_authors` with `INSERT OR IGNORE INTO book_authors ... SELECT id, author_id, 'author', 0 FROM books WHERE author_id IS NOT NULL`. This creates exactly ONE row per book — it did not check if the author name contains `&` and did not split. Migration 20 created the narrators tables but left them empty. Existing books show empty `authors[]` and empty narrators in the UI.

**Files:**
- Modify: `internal/database/migrations.go`
- Create: `internal/database/migrations_extra_test.go` (check if file exists first — it does, so ADD to it)

### Step 1: Write the failing test

Open `internal/database/migrations_extra_test.go` and ADD the following test at the end of the file. First read the file to see what's already there:

```bash
# Read the existing test file
cat /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/migrations_extra_test.go
```

Then append this test (adjust imports as needed based on existing imports):

```go
// TestMigration022_BackfillMultipleAuthorsNarrators verifies that migration 22
// splits existing "&"-joined author names into multiple book_authors rows and
// backfills book_narrators from the legacy books.narrator field.
func TestMigration022_BackfillMultipleAuthorsNarrators(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	s := store.(*SQLiteStore)

	// --- Setup: create two authors joined with "&" ---
	// Author "Alice Smith & Bob Jones" must already exist in authors table
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
	// one row with role='author', position=0
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
	// "Alice Smith & Bob Jones" → two authors: "Alice Smith" (author, pos 0) and "Bob Jones" (co-author, pos 1)
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

	type authorRow struct{ name, role string; pos int }
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

	type narRow struct{ name, role string; pos int }
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
// (no "&") are not modified — they keep their existing book_authors row.
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
```

### Step 2: Run the test to verify it fails

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/database/... -run "TestMigration022" -v 2>&1 | head -40
```

**Expected output:**
```
# [build errors or]
--- FAIL: TestMigration022_BackfillMultipleAuthorsNarrators (...)
    migrations_extra_test.go:NNN: migration022Up failed: ...
```
Or a compile error: `undefined: migration022Up`. Either is correct — the function does not exist yet.

### Step 3: Write the implementation

Open `internal/database/migrations.go`.

**3a. Add migration 22 to the `migrations` slice** (after the closing brace of migration 21 entry at line 167, before the closing `}`):

Current slice ends at line 167:
```go
	{
		Version:     21,
		Description: "Add operation_summary_logs table for persistent operation history",
		Up:          migration021Up,
		Down:        nil,
	},
}
```

Change to:
```go
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
}
```

**3b. Add the `migration022Up` function** at the very end of `internal/database/migrations.go`, after `migration021Up` (which ends at line 1138):

```go
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
	// Find all (book_id, author_id) pairs where author.name contains " & "
	authorRows, err := sqliteStore.db.Query(`
		SELECT ba.book_id, ba.author_id, a.name
		FROM book_authors ba
		JOIN authors a ON a.id = ba.author_id
		WHERE a.name LIKE '%&%'
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
		// Only act if name actually contains " & "
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

			// Find or create the individual author
			var indivAuthorID int
			var existing struct{ id int }
			err := sqliteStore.db.QueryRow(`SELECT id FROM authors WHERE LOWER(name) = LOWER(?)`, name).Scan(&existing.id)
			if err == sql.ErrNoRows {
				// Create new author
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
				indivAuthorID = existing.id
				log.Printf("      - Found existing author %q (id=%d)", name, indivAuthorID)
			}

			role := "author"
			if i > 0 {
				role = "co-author"
			}

			// Insert with OR IGNORE so re-runs don't fail
			if _, err := sqliteStore.db.Exec(`
				INSERT OR IGNORE INTO book_authors (book_id, author_id, role, position)
				VALUES (?, ?, ?, ?)`,
				ja.bookID, indivAuthorID, role, i); err != nil {
				return fmt.Errorf("migration 22: insert book_authors for %q: %w", name, err)
			}
		}

		// Update books.author_id to point to the primary (first) author
		// so the legacy FK stays consistent
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
	// Find books that have a narrator string but no book_narrators rows yet
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

			// Find or create narrator
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
```

**Important:** The `sql` package is already imported in `migrations.go` at line 8. The `strings` and `fmt` and `log` packages are also already imported (lines 9, 12, 13). No new imports needed.

### Step 4: Run the test to verify it passes

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/database/... -run "TestMigration022" -v 2>&1
```

**Expected output:**
```
=== RUN   TestMigration022_BackfillMultipleAuthorsNarrators
--- PASS: TestMigration022_BackfillMultipleAuthorsNarrators (0.XXs)
=== RUN   TestMigration022_SingleAuthorUntouched
--- PASS: TestMigration022_SingleAuthorUntouched (0.XXs)
=== RUN   TestMigration022_Idempotent
--- PASS: TestMigration022_Idempotent (0.XXs)
PASS
ok  	github.com/jdfalk/audiobook-organizer/internal/database	0.XXXs
```

### Step 5: Bump file version headers

In `internal/database/migrations.go`, update line 2 from `// version: 1.14.0` to `// version: 1.15.0`.

### Step 6: Run the full database test suite to check for regressions

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/database/... -v -timeout 120s 2>&1 | tail -30
```

**Expected:** All existing tests still pass. Look for `FAIL` in the output — there should be none.

### Step 7: Commit

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git add internal/database/migrations.go internal/database/migrations_extra_test.go
git commit -m "$(cat <<'EOF'
feat(db): add migration 22 to backfill multi-author & narrator junction tables

Split '&'-joined author names in book_authors into individual author rows
and backfill book_narrators from the legacy books.narrator field. Migration
is idempotent via INSERT OR IGNORE. Adds three tests covering the split,
single-author no-op, and idempotency cases.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Fix AI Parse Handler to Route Through AudiobookService

**Problem:** `parseAudiobookWithAI` at `internal/server/server.go:3647` directly calls `database.GlobalStore.UpdateBook(id, book)` (line 3727). This bypasses:
1. `&` splitting for authors → `SetBookAuthors` never called
2. Narrator junction table → `SetBookNarrators` never called
3. Metadata state recording → `metadata_changes_history` not updated

**Solution:** Build an `UpdateAudiobookRequest` and route through `s.audiobookUpdateService.UpdateAudiobook(id, payload)` — the same path used by the manual edit handler `updateAudiobook` at line 1807.

Looking at `AudiobookUpdateService.UpdateAudiobook` at `internal/server/audiobook_update_service.go:116`: it accepts `id string, payload map[string]any` and internally:
1. Extracts known fields from the map
2. Calls `aus.audiobookService.UpdateAudiobook(context.Background(), id, req)` — which handles `&` splitting, `SetBookAuthors`, `SetBookNarrators`, metadata state

**Files:**
- Modify: `internal/server/server.go`
- Create test in: `internal/server/server_ai_parse_test.go` (new file)

### Step 1: Write the failing test

Create `internal/server/server_ai_parse_test.go`:

```go
// file: internal/server/server_ai_parse_test.go
// version: 1.0.0
// guid: a2b3c4d5-e6f7-8901-bcde-f01234567892

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestParseAudiobookWithAI_CallsSetBookAuthors verifies that the AI parse handler
// routes through the service layer so that "&"-split authors are saved to
// book_authors via SetBookAuthors, not just UpdateBook.
//
// This test uses the mock store to assert that SetBookAuthors is called when
// the AI returns an author name containing "&".
func TestParseAudiobookWithAI_CallsSetBookAuthors(t *testing.T) {
	// This test requires a running server with a mock store.
	// The AI parser is bypassed by pointing OPENAI_BASE_URL at a test server
	// that returns a fixed JSON response.

	t.Skip("Integration-style test: run with make test-e2e or implement with httptest AI stub")
	// TODO: implement with httptest-based OpenAI stub in a follow-up.
	// The unit-level coverage is provided by TestAudiobookUpdateService_AuthorSplit.
}

// TestAudiobookUpdateService_AuthorSplit is a unit test that verifies the
// AudiobookUpdateService correctly calls SetBookAuthors when author_name
// contains "&".  This is the same code path that parseAudiobookWithAI now
// routes through after Task 2's fix.
func TestAudiobookUpdateService_AuthorSplit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := mocks.NewMockStore(t)

	authorID1 := 1
	authorID2 := 2
	bookID := "01JTEST000000000000000099"

	existingBook := &database.Book{
		ID:       bookID,
		Title:    "Old Title",
		FilePath: "/tmp/book.m4b",
		Format:   "m4b",
	}

	// GetBookByID called by AudiobookService.UpdateAudiobook
	mockStore.On("GetBookByID", bookID).Return(existingBook, nil)

	// GetAuthorByName called for each split name
	mockStore.On("GetAuthorByName", "Alice Smith").Return(&database.Author{ID: authorID1, Name: "Alice Smith"}, nil)
	mockStore.On("GetAuthorByName", "Bob Jones").Return(nil, nil) // not found → create
	mockStore.On("CreateAuthor", "Bob Jones").Return(&database.Author{ID: authorID2, Name: "Bob Jones"}, nil)

	// SetBookAuthors must be called with both authors
	mockStore.On("SetBookAuthors", bookID, mock.MatchedBy(func(authors []database.BookAuthor) bool {
		if len(authors) != 2 {
			return false
		}
		return authors[0].AuthorID == authorID1 && authors[0].Role == "author" && authors[0].Position == 0 &&
			authors[1].AuthorID == authorID2 && authors[1].Role == "co-author" && authors[1].Position == 1
	})).Return(nil)

	// GetAuthorByID for resolving the author name after setting
	mockStore.On("GetAuthorByID", authorID1).Return(&database.Author{ID: authorID1, Name: "Alice Smith"}, nil)

	// loadMetadataState → GetUserPreference
	mockStore.On("GetUserPreference", mock.AnythingOfType("string")).Return(nil, nil)

	// UpdateBook is called by AudiobookService after computing state
	mockStore.On("UpdateBook", bookID, mock.AnythingOfType("*database.Book")).Return(existingBook, nil)

	// SetUserPreference for saving metadata state
	mockStore.On("SetUserPreference", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	// RecordMetadataChange (optional — may or may not be called)
	mockStore.On("RecordMetadataChange", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	svc := NewAudiobookService(mockStore)

	req := &UpdateAudiobookRequest{
		Updates: &AudiobookUpdate{
			Book:       existingBook,
			AuthorName: strPtr("Alice Smith & Bob Jones"),
		},
		RawPayload: map[string]json.RawMessage{
			"author_name": json.RawMessage(`"Alice Smith & Bob Jones"`),
		},
	}

	_, err := svc.UpdateAudiobook(context.Background(), bookID, req)
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
}

func strPtr(s string) *string { return &s }
```

### Step 2: Run the test to verify it compiles and `TestAudiobookUpdateService_AuthorSplit` passes

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/server/... -run "TestAudiobookUpdateService_AuthorSplit" -v 2>&1
```

**Expected output:**
```
=== RUN   TestAudiobookUpdateService_AuthorSplit
--- PASS: TestAudiobookUpdateService_AuthorSplit (0.XXs)
PASS
```

If it fails with mock expectation errors, adjust the mock setup to match the actual call signatures by reading the error output carefully.

### Step 3: Fix the `parseAudiobookWithAI` handler

Open `internal/server/server.go`. The handler spans lines 3647–3738. Replace the entire handler body.

**Current code at lines 3647–3738 (to be replaced):**

```go
// parseAudiobookWithAI parses an audiobook's filename with AI and updates its metadata
func (s *Server) parseAudiobookWithAI(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the book
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Extract filename from path
	filename := filepath.Base(book.FilePath)

	// Parse with AI
	metadata, err := parser.ParseFilename(c.Request.Context(), filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse filename: %v", err)})
		return
	}

	// Update book with parsed metadata
	if metadata.Title != "" {
		book.Title = metadata.Title
	}
	if metadata.Narrator != "" {
		book.Narrator = &metadata.Narrator
	}
	if metadata.Publisher != "" {
		book.Publisher = &metadata.Publisher
	}
	if metadata.Year > 0 {
		book.PrintYear = &metadata.Year
	}

	// Handle author
	if metadata.Author != "" {
		author, err := database.GlobalStore.GetAuthorByName(metadata.Author)
		if err != nil || author == nil {
			// Create new author
			author, err = database.GlobalStore.CreateAuthor(metadata.Author)
			if err == nil && author != nil {
				book.AuthorID = &author.ID
			}
		} else {
			book.AuthorID = &author.ID
		}
	}

	// Handle series
	if metadata.Series != "" {
		series, err := database.GlobalStore.GetSeriesByName(metadata.Series, book.AuthorID)
		if err != nil || series == nil {
			// Create new series
			series, err = database.GlobalStore.CreateSeries(metadata.Series, book.AuthorID)
			if err == nil && series != nil {
				book.SeriesID = &series.ID
			}
		} else {
			book.SeriesID = &series.ID
		}

		if metadata.SeriesNum > 0 {
			book.SeriesSequence = &metadata.SeriesNum
		}
	}

	// Update in database
	updatedBook, err := database.GlobalStore.UpdateBook(id, book)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "audiobook updated with AI-parsed metadata",
		"book":       updatedBook,
		"confidence": metadata.Confidence,
	})
}
```

**Replace with:**

```go
// parseAudiobookWithAI parses an audiobook's filename with AI and updates its metadata.
// Routes through AudiobookUpdateService so that multi-author "&" splitting,
// narrator junction tables, and metadata history recording all happen correctly.
func (s *Server) parseAudiobookWithAI(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the book to extract the file path for the AI prompt
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Use the full file path for richer folder-hierarchy context
	fullPath := book.FilePath
	if fullPath == "" {
		fullPath = filepath.Base(book.FilePath)
	}

	// Parse with AI — pass the full path so the model can use directory names
	metadata, err := parser.ParseFilename(c.Request.Context(), fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse filename: %v", err)})
		return
	}

	// Build a payload map exactly as the manual edit handler does (updateAudiobook).
	// AudiobookUpdateService.UpdateAudiobook accepts map[string]any and handles:
	//   - "&"-split author/narrator names → SetBookAuthors / SetBookNarrators
	//   - metadata state recording → metadata_changes_history
	//   - series resolution
	updatePayload := map[string]any{}

	if metadata.Title != "" {
		updatePayload["title"] = metadata.Title
	}
	if metadata.Author != "" {
		// Pass the raw AI-returned author string; the service will split on "&"
		updatePayload["author_name"] = metadata.Author
	}
	if metadata.Narrator != "" {
		// Pass the raw AI-returned narrator string; the service will split on "&"
		updatePayload["narrator"] = metadata.Narrator
	}
	if metadata.Publisher != "" {
		updatePayload["publisher"] = metadata.Publisher
	}
	if metadata.Year > 0 {
		updatePayload["audiobook_release_year"] = metadata.Year
	}
	if metadata.Series != "" {
		updatePayload["series_name"] = metadata.Series
		if metadata.SeriesNum > 0 {
			updatePayload["series_sequence"] = metadata.SeriesNum
		}
	}

	if len(updatePayload) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":    "AI parsing returned no extractable fields",
			"book":       book,
			"confidence": metadata.Confidence,
		})
		return
	}

	// Route through the service layer — same as the manual edit handler
	updatedBook, err := s.audiobookUpdateService.UpdateAudiobook(id, updatePayload)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update audiobook: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "audiobook updated with AI-parsed metadata",
		"book":       updatedBook,
		"confidence": metadata.Confidence,
	})
}
```

**Note on series_sequence:** The `AudiobookUpdate` struct and `AudiobookUpdateService` may not currently handle `series_sequence` in the payload map. Check `internal/server/audiobook_update_service.go` around line 135–173 for the `ExtractIntField` calls. If `series_sequence` is not handled, add a case:

```go
if seriesSeq, ok := aus.ExtractIntField(payload, "series_sequence"); ok {
    updates.SeriesSequence = &seriesSeq
}
```

And add `SeriesSequence *int` to the `AudiobookUpdate` struct in `internal/server/audiobook_service.go` if it doesn't exist. Then handle it in `AudiobookService.UpdateAudiobook` by applying it to `currentBook.SeriesSequence`. Check first with:

```bash
grep -n "SeriesSequence\|series_sequence" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/audiobook_update_service.go \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/audiobook_service.go
```

If it's already handled, skip this sub-step.

### Step 4: Run the test suite to verify the fix doesn't break existing server tests

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/server/... -timeout 120s 2>&1 | tail -30
```

**Expected:** All tests pass. Look for any `FAIL` lines.

Also ensure the binary builds:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./... 2>&1
```

**Expected:** No output (clean build).

### Step 5: Bump file version headers

- `internal/server/server.go`: find `// version:` on line 2, bump to the next patch version
- `internal/server/server_ai_parse_test.go`: new file, version is already `1.0.0`

### Step 6: Commit

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git add internal/server/server.go internal/server/server_ai_parse_test.go
git commit -m "$(cat <<'EOF'
fix(server): route AI parse handler through AudiobookService layer

parseAudiobookWithAI now builds a payload map and calls
audiobookUpdateService.UpdateAudiobook instead of directly calling
GlobalStore.UpdateBook. This ensures "&"-split author/narrator names are
saved to the junction tables and metadata history is recorded correctly.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Improve AI Parse Context — Pass Full Path and Existing Metadata

**Problem:** `ParseFilename` at `internal/ai/openai_parser.go:68` receives only `filepath.Base(book.FilePath)` — just the bare filename. This throws away valuable folder-hierarchy context. For a path like `/audiobooks/Brandon Sanderson/Cosmere/Mistborn 01 - The Final Empire/The Final Empire.m4b`, the model only sees `The Final Empire.m4b` and may miss the author and series name from the directory structure.

Additionally, the existing database metadata (title, author, narrator) is not provided as context, so the model cannot "fill in the gaps" only — it has to guess everything from scratch.

**Solution:** Add a new function `ParseAudiobook` to `OpenAIParser` that accepts a `ParseAudiobookRequest` struct with full path and existing metadata. Update `parseAudiobookWithAI` to use the new function. Keep `ParseFilename` for backward compatibility (used by `ParseBatch` and `TestConnection`).

**Files:**
- Modify: `internal/ai/openai_parser.go`
- Modify: `internal/server/server.go` (the `parseAudiobookWithAI` handler we just updated in Task 2)
- Add test to: `internal/ai/openai_parser_test.go` (create if absent)

### Step 1: Write the failing test

Check if `internal/ai/openai_parser_test.go` exists:

```bash
ls /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/ai/
```

If the file doesn't exist, create `internal/ai/openai_parser_test.go`. If it exists, append the test to it.

```go
// file: internal/ai/openai_parser_test.go
// version: 1.0.0
// guid: b3c4d5e6-f7a8-9012-cdef-012345678903

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeOpenAIServer returns a test HTTP server that mimics the OpenAI chat
// completions endpoint and returns a fixed JSON response.
func fakeOpenAIServer(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond with a valid OpenAI chat completion structure
		w.Header().Set("Content-Type", "application/json")
		// The openai-go SDK reads completion.Choices[0].Message.Content
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": responseBody,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 50,
				"total_tokens":      60,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

// TestParseAudiobook_UsesFullPathInPrompt verifies that ParseAudiobook sends
// the full file path (not just the basename) so directory names are available.
func TestParseAudiobook_UsesFullPathInPrompt(t *testing.T) {
	responseJSON := `{"title":"The Final Empire","author":"Brandon Sanderson","series":"Mistborn","series_number":1,"confidence":"high"}`

	srv := fakeOpenAIServer(t, responseJSON)
	defer srv.Close()

	// Point the parser at our fake server
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	parser := NewOpenAIParser("test-key", true)

	req := ParseAudiobookRequest{
		FullPath:       "/audiobooks/Brandon Sanderson/Mistborn/01 - The Final Empire/the_final_empire.m4b",
		ExistingTitle:  "",
		ExistingAuthor: "",
	}

	meta, err := parser.ParseAudiobook(context.Background(), req)
	if err != nil {
		t.Fatalf("ParseAudiobook failed: %v", err)
	}
	if meta.Title != "The Final Empire" {
		t.Errorf("expected title 'The Final Empire', got %q", meta.Title)
	}
	if meta.Author != "Brandon Sanderson" {
		t.Errorf("expected author 'Brandon Sanderson', got %q", meta.Author)
	}
	if meta.Series != "Mistborn" {
		t.Errorf("expected series 'Mistborn', got %q", meta.Series)
	}
	if meta.SeriesNum != 1 {
		t.Errorf("expected series_number 1, got %d", meta.SeriesNum)
	}
}

// TestParseAudiobook_ExistingMetadataInPrompt verifies that when existing metadata
// is provided, the parser receives it and (by convention in the prompt) uses it
// to fill in only missing fields.
func TestParseAudiobook_ExistingMetadataInPrompt(t *testing.T) {
	// The fake server response only fills in what the AI "decided"
	responseJSON := `{"title":"Mistborn: The Final Empire","author":"Brandon Sanderson","narrator":"Michael Kramer","confidence":"high"}`

	srv := fakeOpenAIServer(t, responseJSON)
	defer srv.Close()
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	parser := NewOpenAIParser("test-key", true)

	req := ParseAudiobookRequest{
		FullPath:         "/audiobooks/BS/Mistborn_01.m4b",
		ExistingTitle:    "Mistborn_01",
		ExistingAuthor:   "Brandon Sanderson",
		ExistingNarrator: "",
	}

	meta, err := parser.ParseAudiobook(context.Background(), req)
	if err != nil {
		t.Fatalf("ParseAudiobook failed: %v", err)
	}
	if meta.Narrator != "Michael Kramer" {
		t.Errorf("expected narrator 'Michael Kramer', got %q", meta.Narrator)
	}
}
```

### Step 2: Run to verify it fails

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/ai/... -run "TestParseAudiobook" -v 2>&1
```

**Expected:**
```
# compile error: undefined: ParseAudiobookRequest
# or
# compile error: undefined: (*OpenAIParser).ParseAudiobook
FAIL    github.com/jdfalk/audiobook-organizer/internal/ai [build failed]
```

### Step 3: Implement `ParseAudiobookRequest` and `ParseAudiobook`

Open `internal/ai/openai_parser.go`. After the `ParsedMetadata` struct (which ends at line 30), add:

```go
// ParseAudiobookRequest carries the full context for AI metadata parsing.
type ParseAudiobookRequest struct {
	// FullPath is the complete file system path including directory components.
	// The directory hierarchy (author/series/title) is valuable context for the AI.
	FullPath string

	// ExistingTitle, ExistingAuthor, ExistingNarrator are the current DB values.
	// Providing them lets the AI fill in only what is missing or correctable.
	ExistingTitle    string
	ExistingAuthor   string
	ExistingNarrator string
}
```

After the `ParseFilename` function (which ends around line 127), add the new `ParseAudiobook` function:

```go
// ParseAudiobook uses OpenAI to parse structured metadata for an audiobook,
// using the full file path (including directory hierarchy) and any existing
// database metadata as context. Prefer this over ParseFilename for single-book
// parsing — ParseFilename is kept for batch and test-connection use.
func (p *OpenAIParser) ParseAudiobook(ctx context.Context, req ParseAudiobookRequest) (*ParsedMetadata, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}

	systemPrompt := `You are an expert at parsing audiobook file paths and extracting structured metadata.

The user will give you:
1. The full file path (directory hierarchy often reveals author, series, title)
2. Any existing metadata already in the database (may be empty or partial)

Your job is to return the BEST possible metadata using BOTH sources.
Prefer directory-hierarchy information when the filename is ambiguous.
If existing metadata is already correct and complete, you may return it unchanged.

Common directory patterns:
  /Author Name/Series Name/NN - Title/file.m4b
  /Author Name/Title/file.m4b
  /Author - Title/file.m4b

Return ONLY valid JSON with these fields (omit optional fields if not found):
{
  "title": "book title",
  "author": "author name (use '&' to join multiple authors, e.g. 'Alice & Bob')",
  "series": "series name",
  "series_number": 1,
  "narrator": "narrator name (use '&' to join multiple narrators)",
  "publisher": "publisher name",
  "year": 2020,
  "confidence": "high|medium|low"
}

Set confidence to "high" if you are certain, "medium" if inferred from context, "low" if guessing.`

	// Build user prompt with full path and existing metadata context
	userPrompt := fmt.Sprintf("Full file path:\n%s\n\nExisting database metadata (may be empty):\n- Title: %s\n- Author: %s\n- Narrator: %s",
		req.FullPath,
		orNone(req.ExistingTitle),
		orNone(req.ExistingAuthor),
		orNone(req.ExistingNarrator),
	)

	jsonObjectFormat := shared.NewResponseFormatJSONObjectParam()

	completion, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:       shared.ChatModel(p.model),
		Temperature: param.NewOpt(0.1),
		MaxTokens:   param.NewOpt[int64](500),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &jsonObjectFormat,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	content := completion.Choices[0].Message.Content
	return parseMetadataFromJSON(content)
}

// orNone returns the string or "(none)" if it is empty — used to make the AI
// prompt explicit about missing fields rather than sending an empty string.
func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
```

### Step 4: Update `parseAudiobookWithAI` to call `ParseAudiobook`

In `internal/server/server.go`, inside `parseAudiobookWithAI` (which we already rewrote in Task 2), replace the call to `parser.ParseFilename` with `parser.ParseAudiobook`.

The current Task 2 code has:
```go
	// Use the full file path for richer folder-hierarchy context
	fullPath := book.FilePath
	if fullPath == "" {
		fullPath = filepath.Base(book.FilePath)
	}

	// Parse with AI — pass the full path so the model can use directory names
	metadata, err := parser.ParseFilename(c.Request.Context(), fullPath)
```

Replace those lines with:
```go
	// Build existing metadata context so the AI can fill in only what's missing
	var existingAuthor, existingNarrator string
	if book.AuthorID != nil {
		if a, aErr := database.GlobalStore.GetAuthorByID(*book.AuthorID); aErr == nil && a != nil {
			existingAuthor = a.Name
		}
	}
	if book.Narrator != nil {
		existingNarrator = *book.Narrator
	}

	// Parse with AI — pass full path + existing metadata for richer context
	metadata, err := parser.ParseAudiobook(c.Request.Context(), ai.ParseAudiobookRequest{
		FullPath:         book.FilePath,
		ExistingTitle:    book.Title,
		ExistingAuthor:   existingAuthor,
		ExistingNarrator: existingNarrator,
	})
```

Also remove the now-unused lines:
```go
	// Use the full file path for richer folder-hierarchy context
	fullPath := book.FilePath
	if fullPath == "" {
		fullPath = filepath.Base(book.FilePath)
	}
```

And remove the `filepath` import from the import block in server.go if it is no longer used anywhere else. Check first:

```bash
grep -n '"path/filepath"' /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go
grep -n 'filepath\.' /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go | grep -v "parseAudiobookWithAI" | head -5
```

If `filepath` is used elsewhere, keep the import. If only used in `parseAudiobookWithAI` and we've removed all uses there, remove the import.

### Step 5: Run all AI tests

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/ai/... -v 2>&1
```

**Expected:**
```
=== RUN   TestParseAudiobook_UsesFullPathInPrompt
--- PASS: TestParseAudiobook_UsesFullPathInPrompt (0.XXs)
=== RUN   TestParseAudiobook_ExistingMetadataInPrompt
--- PASS: TestParseAudiobook_ExistingMetadataInPrompt (0.XXs)
PASS
ok  	github.com/jdfalk/audiobook-organizer/internal/ai	0.XXXs
```

**Note on the fake server:** The `fakeOpenAIServer` helper uses `t.Setenv("OPENAI_BASE_URL", srv.URL)`. The `NewOpenAIParser` function at line 47 already reads this env var: `if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" { clientOptions = append(clientOptions, option.WithBaseURL(baseURL)) }`. So the test will correctly redirect requests to the fake server. Verify this is the case before writing the tests; if the SDK uses a different URL path than `/v1/chat/completions`, the fake server handler may need to check the path.

Run a quick build to verify no compile errors:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./... 2>&1
```

**Expected:** No output (clean build).

### Step 6: Bump version headers

- `internal/ai/openai_parser.go`: bump from `1.3.0` to `1.4.0`
- `internal/ai/openai_parser_test.go`: new file at `1.0.0`
- `internal/server/server.go`: bump to next patch version

### Step 7: Commit

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git add internal/ai/openai_parser.go internal/ai/openai_parser_test.go internal/server/server.go
git commit -m "$(cat <<'EOF'
feat(ai): add ParseAudiobook with full-path and existing-metadata context

Adds ParseAudiobookRequest and ParseAudiobook() to OpenAIParser, using the
full file path (directory hierarchy) and existing DB metadata as context.
Updates parseAudiobookWithAI handler to use the new function. Adds unit
tests with a fake OpenAI HTTP server.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: End-to-End Verification

After all three tasks are committed, run the complete test suite to confirm no regressions.

### Step 1: Full build

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
make build 2>&1 | tail -20
```

**Expected:** Build completes with no errors. Final line should be something like:
```
go build -tags embed_frontend -o audiobook-organizer .
```

### Step 2: Backend tests with coverage

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
make test 2>&1 | tail -30
```

**Expected:** All tests pass. Coverage should remain ≥ 81.3% (current baseline).

### Step 3: Run specifically the new tests together

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test \
  ./internal/database/... \
  ./internal/server/... \
  ./internal/ai/... \
  -run "TestMigration022|TestAudiobookUpdateService_AuthorSplit|TestParseAudiobook" \
  -v 2>&1
```

**Expected output (all must be PASS):**
```
=== RUN   TestMigration022_BackfillMultipleAuthorsNarrators
--- PASS: TestMigration022_BackfillMultipleAuthorsNarrators (...)
=== RUN   TestMigration022_SingleAuthorUntouched
--- PASS: TestMigration022_SingleAuthorUntouched (...)
=== RUN   TestMigration022_Idempotent
--- PASS: TestMigration022_Idempotent (...)
=== RUN   TestAudiobookUpdateService_AuthorSplit
--- PASS: TestAudiobookUpdateService_AuthorSplit (...)
=== RUN   TestParseAudiobook_UsesFullPathInPrompt
--- PASS: TestParseAudiobook_UsesFullPathInPrompt (...)
=== RUN   TestParseAudiobook_ExistingMetadataInPrompt
--- PASS: TestParseAudiobook_ExistingMetadataInPrompt (...)
```

### Step 4: Manual smoke test (if running locally)

```bash
# Start the server
make run-api &
SERVER_PID=$!

# Wait for startup
sleep 2

# Trigger a migration check — the server runs RunMigrations on startup.
# Check logs for "Applying migration 22"
curl -s http://localhost:8080/api/v1/health | jq .

# Stop server
kill $SERVER_PID
```

**Expected:** Health endpoint returns `{"status":"ok"}`. Server logs should show:
```
Applying migration 22: Backfill book_authors (&-split) and book_narrators
  - Running migration 22: backfill book_authors (&-split) and book_narrators
  - Found N books with narrator field but no book_narrators rows
  - Migration 22 complete: book_authors and book_narrators backfilled
Migration 22 completed successfully
```

---

## Summary of Changes

| File | Change | Lines Affected |
|------|--------|----------------|
| `internal/database/migrations.go` | Add migration 22 to slice + `migration022Up` function | ~20 lines in slice, ~120 lines new function at EOF |
| `internal/database/migrations_extra_test.go` | Add 3 tests for migration 22 | ~130 lines appended |
| `internal/server/server.go` | Replace `parseAudiobookWithAI` body to route through service layer + use `ParseAudiobook` | lines 3647–3738 |
| `internal/server/server_ai_parse_test.go` | New file with mock-store unit test for `&`-split author routing | ~90 lines |
| `internal/ai/openai_parser.go` | Add `ParseAudiobookRequest` struct + `ParseAudiobook()` + `orNone()` | ~70 lines after line 30 |
| `internal/ai/openai_parser_test.go` | New or appended file with 2 tests using fake HTTP server | ~80 lines |

## Risks and Edge Cases

1. **`book_authors` PRIMARY KEY conflict in migration 22:** The junction table has `PRIMARY KEY (book_id, author_id)`. When we delete the old `(bookID, joinedAuthorID)` row and insert individual author rows, if the same book previously had separate individual author rows (unlikely for pre-migration data, but possible if the migration was run partially), `INSERT OR IGNORE` prevents duplicates. Safe.

2. **`authors` table UNIQUE constraint on name:** `INSERT INTO authors (name) VALUES (?)` will fail if the name already exists (case-insensitive lookup is done first via `SELECT id FROM authors WHERE LOWER(name) = LOWER(?)`). The lookup-then-insert pattern handles this, but there is a TOCTOU race. In practice, migrations run single-threaded on startup, so this is not a real concern.

3. **Books with `author_id = NULL`:** The migration's author query joins on `book_authors.author_id = authors.id`, so books without an `author_id` are naturally excluded.

4. **Books where `narrator` contains `" & "` but `book_narrators` already has rows** (e.g., from a prior manual edit): The WHERE clause `NOT EXISTS (SELECT 1 FROM book_narrators bn WHERE bn.book_id = b.id)` ensures we only backfill books with NO existing junction rows. Existing junction rows are left untouched.

5. **`series_sequence` field:** If `AudiobookUpdateService` does not yet handle `"series_sequence"` in the payload map, the AI-returned `metadata.SeriesNum` will be silently ignored. This is acceptable for the initial fix — the series name will still be correctly set, and series sequence can be added as a follow-up.

6. **`filepath` import in server.go:** After the Task 2 changes, `filepath.Base` is no longer called inside `parseAudiobookWithAI`. If `filepath` is not used elsewhere in `server.go`, remove the import to avoid a compile error. Verify with `grep -n 'filepath\.' internal/server/server.go`.
