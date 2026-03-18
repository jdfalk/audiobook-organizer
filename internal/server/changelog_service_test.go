// file: internal/server/changelog_service_test.go
// version: 1.0.0
// guid: 3dcbb4a3-7fa5-425f-8247-5f96f81f2f22

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangelogService_GetBookChangelog_Empty(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	svc := server.changelogService
	require.NotNil(t, svc)

	entries, err := svc.GetBookChangelog("nonexistent-book-id")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestChangelogService_GetBookChangelog_NilDB(t *testing.T) {
	svc := &ChangelogService{db: nil}
	_, err := svc.GetBookChangelog("some-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestChangelogService_WithPathHistory(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore
	book, err := store.CreateBook(&database.Book{
		Title:    "Changelog Test",
		FilePath: "/tmp/changelog-test.m4b",
	})
	require.NoError(t, err)
	require.NotNil(t, book)

	// Record a path change
	err = store.RecordPathChange(&database.BookPathChange{
		BookID:     book.ID,
		OldPath:    "/old/path.m4b",
		NewPath:    "/new/path.m4b",
		ChangeType: "rename",
	})
	require.NoError(t, err)

	entries, err := server.changelogService.GetBookChangelog(book.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "rename", entries[0].Type)
	assert.Contains(t, entries[0].Summary, "/old/path.m4b")
	assert.Contains(t, entries[0].Summary, "/new/path.m4b")
}

func TestChangelogEndpoint_Returns200(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore
	book, err := store.CreateBook(&database.Book{
		Title:    "Changelog Endpoint Test",
		FilePath: "/tmp/changelog-endpoint-test.m4b",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/audiobooks/"+book.ID+"/changelog", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "entries")
}

func TestChangelogEndpoint_WithEntries(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore
	book, err := store.CreateBook(&database.Book{
		Title:    "Changelog With Entries",
		FilePath: "/tmp/changelog-entries.m4b",
	})
	require.NoError(t, err)

	// Add a path change
	err = store.RecordPathChange(&database.BookPathChange{
		BookID:     book.ID,
		OldPath:    "/a.m4b",
		NewPath:    "/b.m4b",
		ChangeType: "rename",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/audiobooks/"+book.ID+"/changelog", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Entries []ChangeLogEntry `json:"entries"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "rename", resp.Entries[0].Type)
}

func TestDerefStr(t *testing.T) {
	s := "hello"
	assert.Equal(t, "hello", derefStr(&s))
	assert.Equal(t, "<nil>", derefStr(nil))
}
