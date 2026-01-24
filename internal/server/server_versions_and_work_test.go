// file: internal/server/server_versions_and_work_test.go
// version: 1.0.0
// guid: 3a4b5c6d-7e8f-9012-a345-678901234567
// last-edited: 2026-01-24

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionEndpoints_HappyPaths(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	book1, err := database.GlobalStore.CreateBook(&database.Book{Title: "One", FilePath: "/tmp/one.m4b", Format: "m4b"})
	require.NoError(t, err)
	book2, err := database.GlobalStore.CreateBook(&database.Book{Title: "Two", FilePath: "/tmp/two.m4b", Format: "m4b"})
	require.NoError(t, err)

	// list versions when no group
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+book1.ID+"/versions", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// link the two books (creates group)
	body := bytes.NewBufferString(`{"other_id":"` + book2.ID + `"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+book1.ID+"/versions", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var linkResp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &linkResp))
	groupID := linkResp["version_group_id"]
	require.NotEmpty(t, groupID)

	// getVersionGroup
	req = httptest.NewRequest(http.MethodGet, "/api/v1/version-groups/"+groupID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// set primary (book1)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/"+book1.ID+"/set-primary", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	updated1, err := database.GlobalStore.GetBookByID(book1.ID)
	require.NoError(t, err)
	updated2, err := database.GlobalStore.GetBookByID(book2.ID)
	require.NoError(t, err)
	require.NotNil(t, updated1.VersionGroupID)
	require.NotNil(t, updated2.VersionGroupID)
	require.NotNil(t, updated1.IsPrimaryVersion)
	require.NotNil(t, updated2.IsPrimaryVersion)
	assert.True(t, *updated1.IsPrimaryVersion)
	assert.False(t, *updated2.IsPrimaryVersion)
}

func TestWorkEndpoints_WithMockStore(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	store := dbmocks.NewMockStore(t)
	origStore := database.GlobalStore
	database.GlobalStore = store
	t.Cleanup(func() { database.GlobalStore = origStore })

	author1 := 1
	author2 := 2
	works := []database.Work{{ID: "w1", Title: "Work One", AuthorID: &author1}, {ID: "w2", Title: "Work Two", AuthorID: &author2}}
	store.EXPECT().GetAllWorks().Return(works, nil)
	store.EXPECT().GetBooksByWorkID("w1").Return([]database.Book{{ID: "b1"}, {ID: "b2"}}, nil)
	store.EXPECT().GetBooksByWorkID("w2").Return(nil, assert.AnError)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	store2 := dbmocks.NewMockStore(t)
	database.GlobalStore = store2
	store2.EXPECT().GetAllWorks().Return(works, nil)
	store2.EXPECT().GetBooksByWorkID("w1").Return([]database.Book{{ID: "b1"}, {ID: "b2"}}, nil)
	store2.EXPECT().GetBooksByWorkID("w2").Return(nil, assert.AnError)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/work/stats", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var statsResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &statsResp))
	assert.Equal(t, float64(2), statsResp["total_books"])
	assert.Equal(t, float64(2), statsResp["total_works"])
	assert.Equal(t, float64(1), statsResp["works_with_multiple_editions"])
}
