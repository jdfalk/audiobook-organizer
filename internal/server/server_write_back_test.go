// file: internal/server/server_write_back_test.go
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8901-bcde-f23456789012

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

func TestWriteBackEndpoint_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/nonexistent-id/write-back", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWriteBackEndpoint_ExistingBook_NoFiles(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Insert a book that has no real file on disk
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Write-back Test",
		FilePath: "/tmp/no-such-file.m4b",
		Format:   "m4b",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/audiobooks/"+book.ID+"/write-back",
		nil,
	)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// Endpoint returns 200 even when files fail — failures are warnings not errors
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "written_count")
}

func TestBatchWriteBackEndpoint_ReturnsSummary(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Batch Write-back Test",
		FilePath: "/tmp/no-such-file.m4b",
		Format:   "m4b",
	})
	require.NoError(t, err)

	body, err := json.Marshal(map[string]any{
		"book_ids": []string{book.ID, "missing-id"},
		"rename":   false,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/audiobooks/batch-write-back",
		bytes.NewBuffer(body),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Written int `json:"written"`
		Failed  int `json:"failed"`
		Errors  []struct {
			BookID string `json:"book_id"`
			Error  string `json:"error"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 1, resp.Written)
	assert.Equal(t, 1, resp.Failed)
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "missing-id", resp.Errors[0].BookID)
}
