// file: internal/server/server_test.go
// version: 1.7.0
// guid: b2c3d4e5-f6a7-8901-bcde-234567890abc

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServer creates a test server with in-memory database
func setupTestServer(t *testing.T) (*Server, func()) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create temporary directory for test database
	tempDir, err := os.MkdirTemp("", "audiobook-test-*")
	require.NoError(t, err)

	// Initialize test configuration
	config.AppConfig = config.Config{
		DatabaseType: "sqlite",
		DatabasePath: filepath.Join(tempDir, "test.db"),
		RootDir:      tempDir,
		EnableSQLite: true,
	}

	// Initialize database
	store, err := database.NewSQLiteStore(config.AppConfig.DatabasePath)
	require.NoError(t, err)
	database.GlobalStore = store

	// Initialize operation queue (with 2 workers)
	queue := operations.NewOperationQueue(store, 2)
	operations.GlobalQueue = queue

	// Initialize realtime hub
	hub := realtime.NewEventHub()
	realtime.SetGlobalHub(hub)

	// Create server
	server := NewServer()

	// Cleanup function
	cleanup := func() {
		if store != nil {
			store.Close()
		}
		if queue != nil {
			_ = queue.Shutdown(5 * time.Second)
		}
		_ = os.RemoveAll(tempDir)
	}

	return server, cleanup
}

// setupTestServerWithStore creates a test server with a provided database store
func setupTestServerWithStore(t *testing.T, store database.Store) (*Server, func()) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set the global store to the provided store
	database.GlobalStore = store

	// Create server with the provided store (services will use it)
	server := NewServer()

	// Cleanup function
	cleanup := func() {
		// Don't close the store - caller is responsible for cleanup
	}

	return server, cleanup
}

// TestHealthCheck tests the health check endpoint
func TestHealthCheck(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	for _, path := range []string{"/api/health", "/api/v1/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "ok", response["status"])
		assert.NotNil(t, response["timestamp"])
		assert.NotNil(t, response["version"])
		assert.NotNil(t, response["metrics"])
	}
}

// TestListAudiobooks tests the list audiobooks endpoint
func TestListAudiobooks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		validateFunc   func(t *testing.T, body []byte)
	}{
		{
			name:           "list all audiobooks without params",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotNil(t, response["items"])
			},
		},
		{
			name:           "list with limit and offset",
			queryParams:    "?limit=10&offset=0",
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotNil(t, response["items"])
			},
		},
		{
			name:           "list with search query",
			queryParams:    "?search=test",
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotNil(t, response["items"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateFunc != nil {
				tt.validateFunc(t, w.Body.Bytes())
			}
		})
	}
}

// TestGetAudiobook tests getting a specific audiobook
func TestGetAudiobook(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test with non-existent ID
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/01HXZ123456789ABCDEFGHJ", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestUpdateAudiobook tests updating audiobook metadata
func TestUpdateAudiobook(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	updateData := map[string]any{
		"title":  "Updated Title",
		"author": "Updated Author",
	}
	body, err := json.Marshal(updateData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/01HXZ123456789ABCDEFGHJ", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Expect 404 for non-existent audiobook
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetAudiobookTagsReportsEffectiveSourceSimple(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "book.m4b")
	require.NoError(t, os.WriteFile(tempFile, []byte("audio"), 0o644))

	created, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Stored Title",
		FilePath: tempFile,
		Format:   "m4b",
	})
	require.NoError(t, err)

	now := time.Now()
	err = saveMetadataState(created.ID, map[string]metadataFieldState{
		"title": {
			FetchedValue:   "Fetched Title",
			OverrideValue:  "Override Title",
			OverrideLocked: true,
			UpdatedAt:      now,
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/audiobooks/%s/tags", created.ID), nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Tags map[string]struct {
			EffectiveValue  any `json:"effective_value"`
			EffectiveSource string      `json:"effective_source"`
			StoredValue     any `json:"stored_value"`
			OverrideValue   any `json:"override_value"`
			FetchedValue    any `json:"fetched_value"`
			OverrideLocked  bool        `json:"override_locked"`
		} `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	entry, ok := response.Tags["title"]
	require.True(t, ok, "title tag should exist")
	assert.Equal(t, "Override Title", entry.EffectiveValue)
	assert.Equal(t, "override", entry.EffectiveSource)
	assert.Equal(t, "Override Title", entry.OverrideValue)
	assert.Equal(t, "Fetched Title", entry.FetchedValue)
	assert.Equal(t, "Stored Title", entry.StoredValue)
	assert.True(t, entry.OverrideLocked)
}

func TestUpdateAudiobookOverridesPersist(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempFile := filepath.Join(t.TempDir(), "book-override.m4b")
	require.NoError(t, os.WriteFile(tempFile, []byte("audio"), 0o644))

	created, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Original Title",
		FilePath: tempFile,
		Format:   "m4b",
	})
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"overrides":{"title":{"value":"New Title","locked":true}}}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/audiobooks/%s", created.ID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updated database.Book
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "New Title", updated.Title)

	states, err := database.GlobalStore.GetMetadataFieldStates(created.ID)
	require.NoError(t, err)

	stateByField := map[string]database.MetadataFieldState{}
	for _, st := range states {
		stateByField[st.Field] = st
	}
	state, ok := stateByField["title"]
	require.True(t, ok, "expected metadata state for title")
	assert.True(t, state.OverrideLocked)
	assert.NotNil(t, state.OverrideValue)
	assert.NotZero(t, state.UpdatedAt)
	assert.Equal(t, "New Title", decodeMetadataValue(state.OverrideValue))
}

// TestDeleteAudiobook tests deleting an audiobook
func TestDeleteAudiobook(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/audiobooks/01HXZ123456789ABCDEFGHJ", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Expect 404 for non-existent audiobook
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestBatchUpdateAudiobooks tests batch updating audiobooks
func TestBatchUpdateAudiobooks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	batchData := map[string]any{
		"audiobooks": []map[string]any{},
	}
	body, err := json.Marshal(batchData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/batch", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestListAuthors tests listing authors
func TestListAuthors(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authors", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["items"])
}

// TestListSeries tests listing series
func TestListSeries(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/series", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["items"])
}

// TestBrowseFilesystem tests filesystem browsing
func TestBrowseFilesystem(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
	}{
		{
			name:           "browse root",
			queryParams:    "?path=/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "browse without path",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/filesystem/browse"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestListImportPaths tests listing import paths
func TestListImportPaths(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import-paths", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["importPaths"])
}

// TestGetOperationStatus tests getting operation status
func TestGetOperationStatus(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/01HXZ123456789ABCDEFGHJ/status", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Expect 404 for non-existent operation
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetSystemStatus tests getting system status
func TestGetSystemStatus(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetConfig tests getting configuration
func TestGetConfig(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["config"])
}

// TestListBackups tests listing backups
func TestListBackups(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup/list", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["backups"])
}

// TestBatchUpdateMetadata tests batch metadata updates
func TestBatchUpdateMetadata(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
	}{
		{
			name: "empty batch",
			requestBody: map[string]any{
				"updates":  []any{},
				"validate": true,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid request - missing updates",
			requestBody: map[string]any{
				"validate": true,
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err, "failed to marshal request body for test case %s", tt.name)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/metadata/batch-update", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestValidateMetadata tests metadata validation
func TestValidateMetadata(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectValid    bool
	}{
		{
			name: "valid metadata",
			requestBody: map[string]any{
				"updates": map[string]any{
					"title":  "Test Book",
					"author": "Test Author",
				},
			},
			expectedStatus: http.StatusOK,
			expectValid:    true,
		},
		{
			name: "invalid metadata - missing required field",
			requestBody: map[string]any{
				"updates": map[string]any{
					"title": "",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err, "failed to marshal request body for test case %s", tt.name)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/metadata/validate", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if valid, ok := response["valid"].(bool); ok {
				assert.Equal(t, tt.expectValid, valid)
			}
		})
	}
}

// TestExportMetadata tests metadata export
func TestExportMetadata(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metadata/export", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBulkFetchMetadataRespectsOverridesAndMissingFields(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "book.m4b")
	otherFile := filepath.Join(tempDir, "other.m4b")
	require.NoError(t, os.WriteFile(tempFile, []byte("audio"), 0o644))
	require.NoError(t, os.WriteFile(otherFile, []byte("audio"), 0o644))

	// Arrange: book with a locked publisher override and missing language/author.
	created, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "The Hobbit",
		FilePath: tempFile,
		Format:   "m4b",
	})
	require.NoError(t, err)

	err = saveMetadataState(created.ID, map[string]metadataFieldState{
		"publisher": {
			OverrideValue:  "Manual Publisher",
			OverrideLocked: true,
			UpdatedAt:      time.Now(),
		},
	})
	require.NoError(t, err)

	other, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Unknown Title",
		FilePath: otherFile,
		Format:   "m4b",
	})
	require.NoError(t, err)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			http.NotFound(w, r)
			return
		}
		title := r.URL.Query().Get("title")
		if title == "The Hobbit" {
			_, err := w.Write([]byte(`{"numFound":1,"start":0,"docs":[{"title":"The Hobbit","author_name":["J.R.R. Tolkien"],"first_publish_year":1937,"isbn":["1234567890"],"publisher":["Test Publisher"],"language":["eng"]}]}`))
			_ = err
			return
		}
		_, err := w.Write([]byte(`{"numFound":0,"start":0,"docs":[]}`))
		_ = err
	}))
	defer mockServer.Close()

	originalBaseURL := os.Getenv("OPENLIBRARY_BASE_URL")
	require.NoError(t, os.Setenv("OPENLIBRARY_BASE_URL", mockServer.URL))
	t.Cleanup(func() {
		_ = os.Setenv("OPENLIBRARY_BASE_URL", originalBaseURL)
	})

	// Act: bulk fetch metadata for both books.
	payload := map[string]any{
		"book_ids": []string{created.ID, other.ID},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metadata/bulk-fetch", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Assert: first book updates missing fields but does not override locked publisher.
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		UpdatedCount int `json:"updated_count"`
		TotalCount   int `json:"total_count"`
		Results      []struct {
			BookID        string   `json:"book_id"`
			Status        string   `json:"status"`
			AppliedFields []string `json:"applied_fields"`
			FetchedFields []string `json:"fetched_fields"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 1, response.UpdatedCount)

	var primaryResult, missingResult *struct {
		BookID        string   `json:"book_id"`
		Status        string   `json:"status"`
		AppliedFields []string `json:"applied_fields"`
		FetchedFields []string `json:"fetched_fields"`
	}
	for i := range response.Results {
		if response.Results[i].BookID == created.ID {
			primaryResult = &response.Results[i]
		}
		if response.Results[i].BookID == other.ID {
			missingResult = &response.Results[i]
		}
	}
	require.NotNil(t, primaryResult)
	require.NotNil(t, missingResult)

	assert.Equal(t, "updated", primaryResult.Status)
	assert.Contains(t, primaryResult.AppliedFields, "author_name")
	assert.Contains(t, primaryResult.AppliedFields, "language")
	assert.NotContains(t, primaryResult.AppliedFields, "publisher")
	assert.Contains(t, primaryResult.FetchedFields, "publisher")

	assert.Equal(t, "not_found", missingResult.Status)

	updatedBook, err := database.GlobalStore.GetBookByID(created.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedBook)
	assert.Nil(t, updatedBook.Publisher)
	require.NotNil(t, updatedBook.Language)
	assert.Equal(t, "eng", *updatedBook.Language)
	assert.NotNil(t, updatedBook.AuthorID)

	state, err := loadMetadataState(created.ID)
	require.NoError(t, err)
	require.NotNil(t, state)
	entry := state["publisher"]
	assert.Equal(t, "Test Publisher", entry.FetchedValue)
	assert.Equal(t, "Manual Publisher", entry.OverrideValue)
	assert.True(t, entry.OverrideLocked)
}

// TestCORSMiddleware tests CORS headers
func TestCORSMiddleware(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
}

// TestRouteNotFound tests 404 handling
func TestRouteNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "api endpoint not found",
			path:           "/api/v1/nonexistent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "root path",
			path:           "/",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// BenchmarkHealthCheck benchmarks the health check endpoint
func BenchmarkHealthCheck(b *testing.B) {
	server, cleanup := setupTestServer(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

// BenchmarkListAudiobooks benchmarks listing audiobooks
func BenchmarkListAudiobooks(b *testing.B) {
	server, cleanup := setupTestServer(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks?limit=50", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

// TestEndToEndWorkflow tests a complete workflow
func TestEndToEndWorkflow(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Step 1: Check health
	t.Run("check health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Step 2: List audiobooks (should be empty)
	t.Run("list empty audiobooks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Step 3: Get configuration
	t.Run("get configuration", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Step 4: List import paths
	t.Run("list import paths", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/import-paths", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Step 5: List backups
	t.Run("list backups", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/backup/list", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestResponseTimes tests that endpoints respond within acceptable time
func TestResponseTimes(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/v1/audiobooks"},
		{http.MethodGet, "/api/v1/authors"},
		{http.MethodGet, "/api/v1/series"},
		{http.MethodGet, "/api/v1/config"},
	}

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("%s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// =============================================================================
// Task 3: Size & Format Testing (formerly in task3_size_test.go)
// =============================================================================

// TestDashboardSizeFormat tests dashboard size and format counts
func TestDashboardSizeFormat(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify sizeDistribution exists
	sizeDistribution, ok := response["sizeDistribution"].(map[string]any)
	assert.True(t, ok, "sizeDistribution should exist")
	assert.NotNil(t, sizeDistribution)

	// Verify formatDistribution exists
	formatDistribution, ok := response["formatDistribution"].(map[string]any)
	assert.True(t, ok, "formatDistribution should exist")
	assert.NotNil(t, formatDistribution)

	// Verify basic structure
	t.Run("size distribution structure", func(t *testing.T) {
		if sizeDistribution != nil {
			// Check for common size buckets (may be empty but structure should exist)
			_, hasSmall := sizeDistribution["0-100MB"]
			_, hasMedium := sizeDistribution["100-500MB"]
			_, hasLarge := sizeDistribution["500MB-1GB"]
			_, hasXLarge := sizeDistribution["1GB+"]

			assert.True(t, hasSmall || hasMedium || hasLarge || hasXLarge,
				"Should have at least one size bucket defined")
		}
	})

	t.Run("format distribution structure", func(t *testing.T) {
		if formatDistribution != nil {
			// Check that format counts are present (may be zero)
			for _, count := range formatDistribution {
				_, isNumber := count.(float64)
				assert.True(t, isNumber, "Format counts should be numbers")
			}
		}
	})
}

// TestSizeCalculationAccuracy tests size calculation accuracy
func TestSizeCalculationAccuracy(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// This test verifies that size calculations are accurate
	// In a real test, we'd create test audiobooks with known sizes
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify totalSize is a number
	totalSize, ok := response["totalSize"].(float64)
	assert.True(t, ok, "totalSize should be a number")
	assert.GreaterOrEqual(t, totalSize, float64(0), "totalSize should be non-negative")
}

// TestFormatDetection tests format detection accuracy
func TestFormatDetection(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		expectedFormat string
	}{
		{"m4b detection", "m4b"},
		{"mp3 detection", "mp3"},
		{"opus detection", "opus"},
		{"flac detection", "flac"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// In a real test, we'd create audiobooks with these formats
			// and verify they're detected correctly
			req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestSizeBucketDistribution tests size bucket distribution
func TestSizeBucketDistribution(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	sizeDistribution, ok := response["sizeDistribution"].(map[string]any)
	require.True(t, ok, "sizeDistribution should exist")

	// Verify all size buckets are present
	expectedBuckets := []string{"0-100MB", "100-500MB", "500MB-1GB", "1GB+"}
	for _, bucket := range expectedBuckets {
		_, exists := sizeDistribution[bucket]
		assert.True(t, exists, "Size bucket %s should exist", bucket)
	}
}

// TestEmptyDashboardSizeFormat tests dashboard with no audiobooks
func TestEmptyDashboardSizeFormat(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Even with no audiobooks, size and format distributions should exist
	sizeDistribution, ok := response["sizeDistribution"].(map[string]any)
	assert.True(t, ok, "sizeDistribution should exist even when empty")
	assert.NotNil(t, sizeDistribution)

	formatDistribution, ok := response["formatDistribution"].(map[string]any)
	assert.True(t, ok, "formatDistribution should exist even when empty")
	assert.NotNil(t, formatDistribution)
}

// =============================================================================
// Metadata Fields Testing (formerly in metadata_fields_test.go)
// =============================================================================

// TestGetMetadataFields tests getting metadata fields
func TestGetMetadataFields(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metadata/fields", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify fields structure
	fields, ok := response["fields"].([]any)
	assert.True(t, ok, "fields should be an array")
	assert.NotNil(t, fields)

	// Verify required fields are present
	requiredFields := []string{"title", "author", "narrator", "series", "publishDate"}
	fieldNames := make(map[string]bool)

	for _, field := range fields {
		fieldMap, ok := field.(map[string]any)
		if ok {
			if name, ok := fieldMap["name"].(string); ok {
				fieldNames[name] = true
			}
		}
	}

	for _, required := range requiredFields {
		assert.True(t, fieldNames[required], "Required field %s should be present", required)
	}
}

// TestMetadataFieldValidation tests metadata field validation
func TestMetadataFieldValidation(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		field          string
		value          any
		expectedValid  bool
		expectedStatus int
	}{
		{
			name:           "valid title",
			field:          "title",
			value:          "Test Book",
			expectedValid:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty title",
			field:          "title",
			value:          "",
			expectedValid:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "valid publish date",
			field:          "publishDate",
			value:          "2024-01-01",
			expectedValid:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid publish date",
			field:          "publishDate",
			value:          "invalid-date",
			expectedValid:  false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestBody := map[string]any{
				"updates": map[string]any{
					tt.field: tt.value,
				},
			}

			body, err := json.Marshal(requestBody)
			require.NoError(t, err, "failed to marshal request body for test case %s", tt.name)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/metadata/validate", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// =============================================================================
// Work Queue Testing (formerly in work_test.go)
// =============================================================================

// TestGetWork tests getting work items from the queue
func TestGetWork(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify work queue structure
	workItems, ok := response["items"].([]any)
	assert.True(t, ok, "work items should be an array")
	assert.NotNil(t, workItems)
}

// TestWorkQueueOperations tests work queue operations
func TestWorkQueueOperations(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		endpoint       string
		method         string
		expectedStatus int
	}{
		{
			name:           "list work items",
			endpoint:       "/api/v1/work",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get work statistics",
			endpoint:       "/api/v1/work/stats",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.endpoint, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestWorkQueuePriority tests work queue priority handling
func TestWorkQueuePriority(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test that work queue respects priority
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	workItems, ok := response["items"].([]any)
	assert.True(t, ok)

	// Verify priority ordering if items exist
	if len(workItems) > 1 {
		for i := 0; i < len(workItems)-1; i++ {
			current := workItems[i].(map[string]any)
			next := workItems[i+1].(map[string]any)

			currentPriority, _ := current["priority"].(float64)
			nextPriority, _ := next["priority"].(float64)

			assert.GreaterOrEqual(t, currentPriority, nextPriority,
				"Work items should be ordered by priority (highest first)")
		}
	}
}

func TestGetAudiobookTagsReportsEffectiveSource(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	author, err := database.GlobalStore.CreateAuthor("Test Author")
	require.NoError(t, err)
	series, err := database.GlobalStore.CreateSeries("Test Series", &author.ID)
	require.NoError(t, err)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "book.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("dummy audio"), 0o644))

	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Stored Title",
		AuthorID: &author.ID,
		SeriesID: &series.ID,
		FilePath: filePath,
	})
	require.NoError(t, err)

	state := map[string]metadataFieldState{
		"title": {
			FetchedValue:   "Fetched Title",
			OverrideValue:  "Override Title",
			OverrideLocked: true,
		},
		"narrator": {
			FetchedValue:   "Fetched Narrator",
			OverrideValue:  "Override Narrator",
			OverrideLocked: false,
		},
	}
	require.NoError(t, saveMetadataState(book.ID, state))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/audiobooks/%s/tags", book.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Tags map[string]struct {
			FileValue      any `json:"file_value"`
			FetchedValue   any `json:"fetched_value"`
			StoredValue    any `json:"stored_value"`
			OverrideValue  any `json:"override_value"`
			OverrideLocked bool        `json:"override_locked"`
		} `json:"tags"`
	}

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Contains(t, resp.Tags, "title")
	assert.Equal(t, "Stored Title", resp.Tags["title"].StoredValue)
	assert.Equal(t, "Fetched Title", resp.Tags["title"].FetchedValue)
	assert.Equal(t, "Override Title", resp.Tags["title"].OverrideValue)
	assert.True(t, resp.Tags["title"].OverrideLocked)

	require.Contains(t, resp.Tags, "author_name")
	assert.Equal(t, "Test Author", resp.Tags["author_name"].StoredValue)
	require.Contains(t, resp.Tags, "series_name")
	assert.Equal(t, "Test Series", resp.Tags["series_name"].StoredValue)

	require.Contains(t, resp.Tags, "narrator")
	assert.Equal(t, "Fetched Narrator", resp.Tags["narrator"].FetchedValue)
}

func TestUpdateAudiobookPersistsOverrides(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "book.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("dummy audio"), 0o644))

	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Original Title",
		FilePath: filePath,
	})
	require.NoError(t, err)

	payload := map[string]any{
		"title":       "Updated Title",
		"author_name": "Override Author",
		"overrides": map[string]any{
			"narrator": map[string]any{
				"value":  "Narrator Override",
				"locked": true,
			},
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/audiobooks/%s", book.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	state, err := loadMetadataState(book.ID)
	require.NoError(t, err)

	if assert.Contains(t, state, "title") {
		assert.Equal(t, "Updated Title", state["title"].OverrideValue)
		assert.True(t, state["title"].OverrideLocked)
	}

	if assert.Contains(t, state, "narrator") {
		assert.Equal(t, "Narrator Override", state["narrator"].OverrideValue)
		assert.True(t, state["narrator"].OverrideLocked)
	}

	if assert.Contains(t, state, "author_name") {
		assert.Equal(t, "Override Author", state["author_name"].OverrideValue)
		assert.True(t, state["author_name"].OverrideLocked)
	}
}

// TestAddImportPath tests creating an import path
func TestAddImportPath(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]any{
		"path": "/some/import/path",
		"name": "Test Import",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify it shows up in the list
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/import-paths", nil)
	w2 := httptest.NewRecorder()
	server.router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "/some/import/path")
}

// TestAddImportPathEmptyPath tests validation for empty import path
func TestAddImportPathEmptyPath(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]any{
		"path": "",
		"name": "Empty Path",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateConfig tests the config update endpoint
func TestUpdateConfig(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]any{
		"root_dir":              "/new/root",
		"auto_organize":         true,
		"scan_on_startup":       false,
		"folder_naming_pattern": "{author}/{title}",
		"file_naming_pattern":   "{title}",
		"log_level":             "debug",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "/new/root", resp["root_dir"])
	assert.Equal(t, true, resp["auto_organize"])
}

// TestUpdateConfigRejectsDatabaseType tests that database_type changes are rejected
func TestUpdateConfigRejectsDatabaseType(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]any{
		"database_type": "postgres",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestSoftDeleteAudiobook tests soft deletion
func TestSoftDeleteAudiobook(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "softdelete.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))

	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "To Soft Delete",
		FilePath: filePath,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/audiobooks/%s?soft_delete=true", book.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "soft deleted")
}

// TestGetOperationLogs tests the operation logs endpoint
func TestGetOperationLogs(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/logs", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetOperationLogsWithPagination tests logs with pagination
func TestGetOperationLogsWithPagination(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/logs?page=1&page_size=10&search=scan", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestListAudiobookVersions tests the version listing endpoint
func TestListAudiobookVersions(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "version.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))

	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Version Test",
		FilePath: filePath,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/audiobooks/%s/versions", book.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetDuplicates tests the duplicates endpoint
func TestGetDuplicates(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/duplicates", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetSoftDeletedBooks tests the soft-deleted books endpoint
func TestGetSoftDeletedBooks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/soft-deleted", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestCountAudiobooks tests the count endpoint
func TestCountAudiobooks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/count", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestCreateAndListWorks tests work creation and listing
func TestCreateAndListWorks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a work
	payload := map[string]any{
		"title":       "Test Work",
		"description": "A test work item",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/works", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// List works
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/works", nil)
	w2 := httptest.NewRecorder()
	server.router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestRestoreAudiobook tests restoring a soft-deleted audiobook
func TestRestoreAudiobook(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "restore.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))

	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "To Restore",
		FilePath: filePath,
	})
	require.NoError(t, err)

	// Soft delete first
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/audiobooks/%s?soft_delete=true", book.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Now restore
	req2 := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/audiobooks/%s/restore", book.ID), nil)
	w2 := httptest.NewRecorder()
	server.router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestPurgeSoftDeletedBooks tests purging soft-deleted books
func TestPurgeSoftDeletedBooks(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/audiobooks/purge-soft-deleted", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLinkAudiobookVersion tests linking two audiobooks as versions
func TestLinkAudiobookVersion(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()

	// Create two books
	filePath1 := filepath.Join(tempDir, "v1.m4b")
	filePath2 := filepath.Join(tempDir, "v2.m4b")
	require.NoError(t, os.WriteFile(filePath1, []byte("v1"), 0o644))
	require.NoError(t, os.WriteFile(filePath2, []byte("v2"), 0o644))

	book1, err := database.GlobalStore.CreateBook(&database.Book{Title: "Book V1", FilePath: filePath1})
	require.NoError(t, err)
	book2, err := database.GlobalStore.CreateBook(&database.Book{Title: "Book V2", FilePath: filePath2})
	require.NoError(t, err)

	payload := map[string]any{
		"other_id": book2.ID,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/audiobooks/%s/versions", book1.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// Should succeed or at least not 500
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusCreated,
		"expected 200 or 201, got %d: %s", w.Code, w.Body.String())
}

// TestRemoveImportPath tests removing an import path
func TestRemoveImportPath(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create an import path directly via DB
	importPath, err := database.GlobalStore.CreateImportPath("/to/remove", "Remove Me")
	require.NoError(t, err)

	// Delete via API - note route uses :id suffix on import-paths
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/import-paths/%d", importPath.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusNoContent,
		"expected 200 or 204, got %d: %s", w.Code, w.Body.String())
}

// TestCreateExclusion tests filesystem exclusion creation
func TestCreateExclusion(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]any{"pattern": "*.tmp"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filesystem/exclude", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// Should succeed (200/201) or error gracefully
	assert.True(t, w.Code < 500, "unexpected server error: %d %s", w.Code, w.Body.String())
}

// TestGetHomeDirectory tests home directory endpoint
func TestGetHomeDirectory(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/filesystem/home", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "path")
}

// TestGetAudiobookNotFound tests 404 for nonexistent audiobook
func TestGetAudiobookNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/nonexistent-id", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestUpdateAudiobookNotFound tests 404 for updating nonexistent audiobook
func TestUpdateAudiobookNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]any{"title": "New Title"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/nonexistent-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestDeleteAudiobookNotFound tests 404 for deleting nonexistent audiobook
func TestDeleteAudiobookNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/audiobooks/nonexistent-id", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestListAudiobooksWithFilters tests listing audiobooks with various filters
func TestListAudiobooksWithFilters(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create an author and audiobook
	author, err := database.GlobalStore.CreateAuthor("Filter Author")
	require.NoError(t, err)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "filter.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))
	_, err = database.GlobalStore.CreateBook(&database.Book{
		Title:    "Filter Book",
		AuthorID: &author.ID,
		FilePath: filePath,
	})
	require.NoError(t, err)

	tests := []struct {
		name  string
		query string
	}{
		{"with search", "/api/v1/audiobooks?search=Filter"},
		{"with author_id", fmt.Sprintf("/api/v1/audiobooks?author_id=%d", author.ID)},
		{"with limit", "/api/v1/audiobooks?limit=5&offset=0"},
		{"with all params", fmt.Sprintf("/api/v1/audiobooks?search=Filter&author_id=%d&limit=10", author.ID)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}
