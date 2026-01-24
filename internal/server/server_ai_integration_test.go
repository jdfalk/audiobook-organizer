// file: internal/server/server_ai_integration_test.go
// version: 1.0.0
// guid: 6d5c4b3a-2918-1706-f5e4-d3c2b1a09f8e
// last-edited: 2026-01-24

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIEndpoints_WithStubbedOpenAI(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Stub OpenAI Chat Completions.
	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept any path that ends with /chat/completions.
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"title":"The Hobbit","author":"J.R.R. Tolkien","series":"Middle Earth","series_number":1,"narrator":"Rob Inglis","publisher":"Random House","year":1937,"confidence":"high"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer openAI.Close()

	// Configure AI parsing to be enabled and point at stub server.
	origCfg := config.AppConfig
	t.Cleanup(func() { config.AppConfig = origCfg })
	config.AppConfig.EnableAIParsing = true
	config.AppConfig.OpenAIAPIKey = "test-key"
	t.Setenv("OPENAI_BASE_URL", openAI.URL+"/v1")

	// 1) POST /ai/parse-filename should succeed.
	parseBody := bytes.NewBufferString(`{"filename":"The Hobbit - J.R.R. Tolkien"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/parse-filename", parseBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// 2) POST /ai/test-connection should succeed using provided api_key.
	connBody := bytes.NewBufferString(`{"api_key":"test-key"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/ai/test-connection", connBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// 3) POST /audiobooks/:id/parse-with-ai should update a book.
	filePath := filepath.Join(t.TempDir(), "The Hobbit - J.R.R. Tolkien.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))
	book, err := database.GlobalStore.CreateBook(&database.Book{Title: "Old", FilePath: filePath, Format: "m4b"})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+book.ID+"/parse-with-ai", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	updated, err := database.GlobalStore.GetBookByID(book.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "The Hobbit", updated.Title)
	require.NotNil(t, updated.AuthorID)
}
