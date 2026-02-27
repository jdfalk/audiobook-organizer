// file: internal/server/server_narrators_fieldstates_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-narrator0test

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListNarrators(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Empty list first
	req := httptest.NewRequest(http.MethodGet, "/api/v1/narrators", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var narrators []database.Narrator
	err := json.Unmarshal(w.Body.Bytes(), &narrators)
	require.NoError(t, err)
	assert.Empty(t, narrators)

	// Add a narrator and re-check
	store := database.GlobalStore.(*database.SQLiteStore)
	_, err = store.CreateNarrator("Morgan Freeman")
	require.NoError(t, err)
	_, err = store.CreateNarrator("Stephen Fry")
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/narrators", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &narrators)
	require.NoError(t, err)
	assert.Len(t, narrators, 2)
}

func TestCountNarrators(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Zero count
	req := httptest.NewRequest(http.MethodGet, "/api/v1/narrators/count", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["count"])

	// Add narrators
	store := database.GlobalStore.(*database.SQLiteStore)
	_, err = store.CreateNarrator("Narrator A")
	require.NoError(t, err)
	_, err = store.CreateNarrator("Narrator B")
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/narrators/count", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(2), resp["count"])
}

func TestListAudiobookNarrators(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a book
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Test Book",
		FilePath: "/tmp/test.m4b",
	})
	require.NoError(t, err)

	// No narrators yet — should return empty array
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+book.ID+"/narrators", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var narrators []database.BookNarrator
	err = json.Unmarshal(w.Body.Bytes(), &narrators)
	require.NoError(t, err)
	assert.Empty(t, narrators)

	// Create narrator and assign to book
	store := database.GlobalStore.(*database.SQLiteStore)
	narrator, err := store.CreateNarrator("Test Narrator")
	require.NoError(t, err)

	err = database.GlobalStore.SetBookNarrators(book.ID, []database.BookNarrator{
		{BookID: book.ID, NarratorID: narrator.ID, Role: "narrator", Position: 0},
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+book.ID+"/narrators", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &narrators)
	require.NoError(t, err)
	assert.Len(t, narrators, 1)
	assert.Equal(t, narrator.ID, narrators[0].NarratorID)
}

func TestSetAudiobookNarrators(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a book and narrator
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Narrator Set Test",
		FilePath: "/tmp/narrator-set.m4b",
	})
	require.NoError(t, err)

	store := database.GlobalStore.(*database.SQLiteStore)
	narrator, err := store.CreateNarrator("PUT Narrator")
	require.NoError(t, err)

	// PUT narrators
	body := []database.BookNarrator{
		{BookID: book.ID, NarratorID: narrator.ID, Role: "narrator", Position: 0},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/"+book.ID+"/narrators", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])

	// Verify by reading back
	narrators, err := database.GlobalStore.GetBookNarrators(book.ID)
	require.NoError(t, err)
	assert.Len(t, narrators, 1)
	assert.Equal(t, narrator.ID, narrators[0].NarratorID)

	// Test bad JSON body
	req = httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/"+book.ID+"/narrators", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAudiobookFieldStates(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a book
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Field States Test",
		FilePath: "/tmp/field-states.m4b",
	})
	require.NoError(t, err)

	// Get field states — should return empty map for new book
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+book.ID+"/field-states", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "field_states")

	// field_states should be a map (possibly empty)
	fieldStates, ok := resp["field_states"].(map[string]any)
	require.True(t, ok, "field_states should be a map")
	assert.NotNil(t, fieldStates)
}
