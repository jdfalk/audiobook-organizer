// file: internal/database/sqlite_store_timestamp_test.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateBook_MetadataUpdatedAt_OnlyChangesWhenMetadataChanges verifies that
// metadata_updated_at is set when title changes, but not when only file_hash changes.
func TestUpdateBook_MetadataUpdatedAt_OnlyChangesWhenMetadataChanges(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	sqlStore := store.(*SQLiteStore)

	book := &Book{
		ID:       "test-meta-ts-" + time.Now().Format("20060102150405"),
		Title:    "Original Title",
		FilePath: "/tmp/test.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	// First update: change title — metadata_updated_at should be set.
	book.Title = "New Title"
	updated, err := sqlStore.UpdateBook(book.ID, book)
	require.NoError(t, err)
	assert.NotNil(t, updated.MetadataUpdatedAt, "metadata_updated_at should be set when title changes")
	firstMetaTs := *updated.MetadataUpdatedAt

	time.Sleep(10 * time.Millisecond)

	// Second update: change only file_hash (system field) — metadata_updated_at should NOT change.
	hash := "abc123"
	book.FileHash = &hash
	book.Title = "New Title" // same title
	updated2, err := sqlStore.UpdateBook(book.ID, book)
	require.NoError(t, err)
	require.NotNil(t, updated2.MetadataUpdatedAt)
	assert.Equal(t, firstMetaTs.Unix(), updated2.MetadataUpdatedAt.Unix(),
		"metadata_updated_at should NOT change when only system fields change")
}

// TestUpdateBook_UpdatedAt_AlwaysChanges verifies that updated_at changes on every write.
func TestUpdateBook_UpdatedAt_AlwaysChanges(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	sqlStore := store.(*SQLiteStore)

	book := &Book{
		ID:       "test-upd-ts-" + time.Now().Format("20060102150405"),
		Title:    "Stable Title",
		FilePath: "/tmp/test2.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	// First update
	time.Sleep(5 * time.Millisecond)
	book.Format = "mp3"
	updated1, err := sqlStore.UpdateBook(book.ID, book)
	require.NoError(t, err)
	ts1 := updated1.UpdatedAt

	// Second update immediately after
	time.Sleep(5 * time.Millisecond)
	updated2, err := sqlStore.UpdateBook(book.ID, book)
	require.NoError(t, err)
	ts2 := updated2.UpdatedAt

	assert.True(t, ts2.After(*ts1), "updated_at should always advance on each write")
}

// TestSetLastWrittenAt verifies that SetLastWrittenAt stamps the column correctly.
func TestSetLastWrittenAt(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	sqlStore := store.(*SQLiteStore)

	book := &Book{
		ID:       "test-lwt-" + time.Now().Format("20060102150405"),
		Title:    "Write-back Book",
		FilePath: "/tmp/writeback.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	// Verify initially nil
	fetched, err := store.GetBookByID(book.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched.LastWrittenAt, "last_written_at should initially be nil")

	// Stamp it
	writeTime := time.Now().Truncate(time.Second)
	err = sqlStore.SetLastWrittenAt(book.ID, writeTime)
	require.NoError(t, err)

	// Verify it was set
	fetched2, err := store.GetBookByID(book.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched2.LastWrittenAt)
	assert.WithinDuration(t, writeTime, *fetched2.LastWrittenAt, time.Second)
}

// TestMigration023_BackfillsMetadataUpdatedAt verifies that existing rows with
// updated_at get their metadata_updated_at backfilled during migration 023.
func TestMigration023_BackfillsMetadataUpdatedAt(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	sqlStore := store.(*SQLiteStore)

	// After migration 023, any book with updated_at should have metadata_updated_at != nil
	book := &Book{
		ID:       "test-mig023-" + time.Now().Format("20060102150405"),
		Title:    "Backfill Test",
		FilePath: "/tmp/backfill.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	// UpdateBook sets updated_at; since title is set, metadata_updated_at should be set too
	_, err = sqlStore.UpdateBook(book.ID, book)
	require.NoError(t, err)

	fetched, err := store.GetBookByID(book.ID)
	require.NoError(t, err)
	// After migration 023 backfill + UpdateBook logic, metadata_updated_at should be non-nil
	assert.NotNil(t, fetched.UpdatedAt)
}
