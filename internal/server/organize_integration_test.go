// file: internal/server/organize_integration_test.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4567-bcde-890123456f01

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizeService_ViaHTTP(t *testing.T) {
	env, cleanup := testutil.SetupIntegration(t)
	defer cleanup()

	// Create a book in the DB with a file outside RootDir
	srcPath := env.CopyFixture("test_sample.m4b", env.ImportDir, "Book.m4b")
	author, err := env.Store.CreateAuthor("Test Author")
	require.NoError(t, err)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcPath,
		Format:   "m4b",
		AuthorID: &author.ID,
	}
	_, err = env.Store.CreateBook(book)
	require.NoError(t, err)

	// Trigger organize via HTTP
	server := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)

	// Wait for operation
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	opID, ok := resp["id"].(string)
	if !ok {
		// Fallback: try operation_id key
		opID, ok = resp["operation_id"].(string)
	}
	require.True(t, ok, "response should contain id or operation_id, got: %v", resp)
	testutil.WaitForOp(t, env.Store, opID, 15*time.Second)

	// Verify book was organized
	books, err := env.Store.GetAllBooks(100, 0)
	require.NoError(t, err)
	require.Len(t, books, 1)

	updated := books[0]
	assert.Contains(t, updated.FilePath, env.RootDir, "book should be in library dir")

	// Verify file exists at organized location
	_, err = os.Stat(updated.FilePath)
	assert.NoError(t, err, "organized file should exist")
}
