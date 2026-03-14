// file: internal/server/merge_service_test.go
// version: 1.0.0
// guid: 8e847d3e-f1a0-41be-a05c-1b18cd3fb7af

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeService_MergeBooks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()
	_ = server

	store := database.GlobalStore

	// Create two test books
	book1 := &database.Book{
		ID:       ulid.Make().String(),
		Title:    "Test Book MP3",
		Format:   "mp3",
		FilePath: "/tmp/test1.mp3",
	}
	book2 := &database.Book{
		ID:       ulid.Make().String(),
		Title:    "Test Book M4B",
		Format:   "m4b",
		FilePath: "/tmp/test2.m4b",
	}

	_, err := store.CreateBook(book1)
	require.NoError(t, err)
	_, err = store.CreateBook(book2)
	require.NoError(t, err)

	ms := NewMergeService(store)
	result, err := ms.MergeBooks([]string{book1.ID, book2.ID}, "")
	require.NoError(t, err)

	assert.Equal(t, 2, result.MergedCount)
	assert.NotEmpty(t, result.VersionGroupID)
	// M4B should be selected as primary since it's the preferred format
	assert.Equal(t, book2.ID, result.PrimaryID)

	// Verify books in database
	b1, err := store.GetBookByID(book1.ID)
	require.NoError(t, err)
	require.NotNil(t, b1.VersionGroupID)
	assert.Equal(t, result.VersionGroupID, *b1.VersionGroupID)
	require.NotNil(t, b1.IsPrimaryVersion)
	assert.False(t, *b1.IsPrimaryVersion)

	b2, err := store.GetBookByID(book2.ID)
	require.NoError(t, err)
	require.NotNil(t, b2.VersionGroupID)
	assert.Equal(t, result.VersionGroupID, *b2.VersionGroupID)
	require.NotNil(t, b2.IsPrimaryVersion)
	assert.True(t, *b2.IsPrimaryVersion)
}

func TestMergeService_MergeBooks_ExplicitPrimary(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()
	_ = server

	store := database.GlobalStore

	book1 := &database.Book{
		ID:       ulid.Make().String(),
		Title:    "Book A",
		Format:   "mp3",
		FilePath: "/tmp/a.mp3",
	}
	book2 := &database.Book{
		ID:       ulid.Make().String(),
		Title:    "Book B",
		Format:   "m4b",
		FilePath: "/tmp/b.m4b",
	}

	_, err := store.CreateBook(book1)
	require.NoError(t, err)
	_, err = store.CreateBook(book2)
	require.NoError(t, err)

	ms := NewMergeService(store)
	// Force MP3 as primary even though M4B would normally win
	result, err := ms.MergeBooks([]string{book1.ID, book2.ID}, book1.ID)
	require.NoError(t, err)

	assert.Equal(t, book1.ID, result.PrimaryID)
}

func TestMergeService_MergeBooks_TooFew(t *testing.T) {
	ms := NewMergeService(nil)
	_, err := ms.MergeBooks([]string{"one"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2")
}

func TestMergeService_MergeBooks_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()
	_ = server

	ms := NewMergeService(database.GlobalStore)
	_, err := ms.MergeBooks([]string{"nonexistent1", "nonexistent2"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
