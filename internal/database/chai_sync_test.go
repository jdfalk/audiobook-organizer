// file: internal/database/chai_sync_test.go
// version: 1.0.0
// guid: a9b8c7d6-e5f4-4321-9876-fedcba012345
// last-edited: 2026-05-24

package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// newTestChaiDB opens an in-memory-like (temp dir) ChaiDB for use in sync tests.
func newTestChaiDB(t *testing.T) *ChaiDB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chai_sync_test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestPebbleStoreWithChai creates a PebbleStore with a Chai database attached
// and UseChaiDB = true.
func newTestPebbleStoreWithChai(t *testing.T) *PebbleStore {
	t.Helper()
	tmpDir := t.TempDir()

	store, err := NewPebbleStore(filepath.Join(tmpDir, "pebble"))
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	chaiDB := newTestChaiDB(t)
	store.chai = chaiDB
	store.UseChaiDB = true
	return store
}

// makeTestBook returns a minimal valid Book for use in tests.
func makeTestBook(id, title string) *Book {
	now := time.Now()
	isPrimary := true
	markedDel := false
	return &Book{
		ID:               id,
		Title:            title,
		FilePath:         "/test/" + id + ".m4b",
		Format:           "m4b",
		IsPrimaryVersion: &isPrimary,
		MarkedForDeletion: &markedDel,
		CreatedAt:        &now,
		UpdatedAt:        &now,
	}
}

// TestUpsertBookToChaiDB_InsertAndQuery verifies that a book inserted via
// UpsertBookToChaiDB is queryable through the Chai SQL layer.
func TestUpsertBookToChaiDB_InsertAndQuery(t *testing.T) {
	store := newTestPebbleStoreWithChai(t)
	ctx := context.Background()

	book := makeTestBook("01TESTBOOKID00000000000000", "Test Book One")
	authorID := 42
	book.AuthorID = &authorID

	if err := store.UpsertBookToChaiDB(ctx, book); err != nil {
		t.Fatalf("UpsertBookToChaiDB failed: %v", err)
	}

	// Verify the book is queryable via SQL.
	var count int
	err := store.chai.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books WHERE id = '01TESTBOOKID00000000000000'").Scan(&count)
	if err != nil {
		t.Fatalf("query after upsert failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 book in chai, got %d", count)
	}
}

// TestUpsertBookToChaiDB_Idempotent verifies that upserting the same book twice
// does not create duplicate rows.
func TestUpsertBookToChaiDB_Idempotent(t *testing.T) {
	store := newTestPebbleStoreWithChai(t)
	ctx := context.Background()

	book := makeTestBook("01TESTBOOKID00000000000001", "Idempotent Book")

	if err := store.UpsertBookToChaiDB(ctx, book); err != nil {
		t.Fatalf("first UpsertBookToChaiDB failed: %v", err)
	}
	if err := store.UpsertBookToChaiDB(ctx, book); err != nil {
		t.Fatalf("second UpsertBookToChaiDB failed: %v", err)
	}

	var count int
	err := store.chai.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books WHERE id = '01TESTBOOKID00000000000001'").Scan(&count)
	if err != nil {
		t.Fatalf("query after double upsert failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 book after idempotent upsert, got %d", count)
	}
}

// TestUpsertBookToChaiDB_BookAuthors verifies that book_authors rows are
// populated when the book has author associations.
func TestUpsertBookToChaiDB_BookAuthors(t *testing.T) {
	store := newTestPebbleStoreWithChai(t)
	ctx := context.Background()

	book := makeTestBook("01TESTBOOKID00000000000002", "Multi-Author Book")
	authorID := 7
	book.AuthorID = &authorID

	// Store authors via SetBookAuthors (mirrors what CreateBook callers do).
	authors := []BookAuthor{
		{BookID: book.ID, AuthorID: 7, Role: "author", Position: 0},
		{BookID: book.ID, AuthorID: 8, Role: "co-author", Position: 1},
	}
	if err := store.SetBookAuthors(book.ID, authors); err != nil {
		t.Fatalf("SetBookAuthors failed: %v", err)
	}

	if err := store.UpsertBookToChaiDB(ctx, book); err != nil {
		t.Fatalf("UpsertBookToChaiDB failed: %v", err)
	}

	var count int
	err := store.chai.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM book_authors WHERE book_id = '01TESTBOOKID00000000000002'").Scan(&count)
	if err != nil {
		t.Fatalf("query book_authors failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 book_authors rows, got %d", count)
	}
}

// TestDeleteBookFromChaiDB verifies that a book is removed from Chai after delete.
func TestDeleteBookFromChaiDB_RemovesBook(t *testing.T) {
	store := newTestPebbleStoreWithChai(t)
	ctx := context.Background()

	book := makeTestBook("01TESTBOOKID00000000000003", "Book To Delete")

	if err := store.UpsertBookToChaiDB(ctx, book); err != nil {
		t.Fatalf("UpsertBookToChaiDB failed: %v", err)
	}

	// Confirm it's there.
	var countBefore int
	if err := store.chai.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books WHERE id = '01TESTBOOKID00000000000003'").Scan(&countBefore); err != nil {
		t.Fatalf("pre-delete query failed: %v", err)
	}
	if countBefore != 1 {
		t.Fatalf("expected 1 book before delete, got %d", countBefore)
	}

	if err := store.DeleteBookFromChaiDB(ctx, book.ID); err != nil {
		t.Fatalf("DeleteBookFromChaiDB failed: %v", err)
	}

	var countAfter int
	if err := store.chai.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books WHERE id = '01TESTBOOKID00000000000003'").Scan(&countAfter); err != nil {
		t.Fatalf("post-delete query failed: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("expected 0 books after delete, got %d", countAfter)
	}
}

// TestBackfillChaiFromPebble verifies that BackfillChaiFromPebble syncs all
// existing books from Pebble into Chai.
func TestBackfillChaiFromPebble_SyncsAllBooks(t *testing.T) {
	store := newTestPebbleStoreWithChai(t)
	ctx := context.Background()

	// Temporarily disable write-through so we can test backfill independently.
	store.UseChaiDB = false

	// Create books via Pebble only (no Chai sync).
	bookIDs := []string{
		"01BACKFILLBOOK000000000001",
		"01BACKFILLBOOK000000000002",
		"01BACKFILLBOOK000000000003",
	}
	for i, id := range bookIDs {
		b := makeTestBook(id, "Backfill Book")
		b.ID = id
		b.FilePath = "/test/backfill/" + id + ".m4b"
		b.Title = "Backfill Book " + string(rune('A'+i))
		if _, err := store.CreateBook(b); err != nil {
			t.Fatalf("CreateBook %s failed: %v", id, err)
		}
	}

	// Re-enable chai and run backfill.
	store.UseChaiDB = true
	synced, err := store.BackfillChaiFromPebble(ctx)
	if err != nil {
		t.Fatalf("BackfillChaiFromPebble failed: %v", err)
	}
	if synced < len(bookIDs) {
		t.Errorf("expected at least %d synced, got %d", len(bookIDs), synced)
	}

	// Verify all 3 books appear in Chai.
	var count int
	if err := store.chai.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&count); err != nil {
		t.Fatalf("query after backfill failed: %v", err)
	}
	if count < len(bookIDs) {
		t.Errorf("expected at least %d books in chai after backfill, got %d", len(bookIDs), count)
	}
}

// TestChaiSyncHelper_NullableHelpers verifies the SQL null-formatting helpers.
func TestChaiSyncHelper_NullableHelpers(t *testing.T) {
	t.Run("chaiNullableString_nil", func(t *testing.T) {
		if got := chaiNullableString(nil); got != "NULL" {
			t.Errorf("expected NULL, got %q", got)
		}
	})
	t.Run("chaiNullableString_value", func(t *testing.T) {
		s := "it's a test"
		got := chaiNullableString(&s)
		want := "'it''s a test'"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})
	t.Run("chaiNullableInt_nil", func(t *testing.T) {
		if got := chaiNullableInt(nil); got != "NULL" {
			t.Errorf("expected NULL, got %q", got)
		}
	})
	t.Run("chaiNullableInt_value", func(t *testing.T) {
		v := 42
		if got := chaiNullableInt(&v); got != "42" {
			t.Errorf("expected 42, got %q", got)
		}
	})
	t.Run("chaiNullableBool_true", func(t *testing.T) {
		v := true
		if got := chaiNullableBool(&v); got != "true" {
			t.Errorf("expected true, got %q", got)
		}
	})
	t.Run("chaiNullableBool_nil", func(t *testing.T) {
		if got := chaiNullableBool(nil); got != "NULL" {
			t.Errorf("expected NULL, got %q", got)
		}
	})
	t.Run("chaiNullableTime_nil", func(t *testing.T) {
		if got := chaiNullableTime(nil); got != "NULL" {
			t.Errorf("expected NULL, got %q", got)
		}
	})
	t.Run("chaiNullableTime_value", func(t *testing.T) {
		ts := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
		got := chaiNullableTime(&ts)
		want := "'2026-05-24T12:00:00'"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})
}
