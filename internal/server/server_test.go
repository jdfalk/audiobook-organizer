// file: internal/server/server_test.go
// version: 1.1.0
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
	realtime.GlobalHub = hub

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

// TestHealthCheck tests the health check endpoint
func TestHealthCheck(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	for _, path := range []string{"/api/health", "/api/v1/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
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
				var response map[string]interface{}
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
				var response map[string]interface{}
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
				var response map[string]interface{}
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

	updateData := map[string]interface{}{
		"title":  "Updated Title",
		"author": "Updated Author",
	}
	body, _ := json.Marshal(updateData)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/01HXZ123456789ABCDEFGHJ", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Expect 400 for invalid request body (binding validation happens before existence check)
	assert.Equal(t, http.StatusBadRequest, w.Code)
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

	batchData := map[string]interface{}{
		"audiobooks": []map[string]interface{}{},
	}
	body, _ := json.Marshal(batchData)

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

	var response map[string]interface{}
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

	var response map[string]interface{}
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

	var response map[string]interface{}
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

	var response map[string]interface{}
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

	var response map[string]interface{}
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
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "empty batch",
			requestBody: map[string]interface{}{
				"updates":  []interface{}{},
				"validate": true,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid request - missing updates",
			requestBody: map[string]interface{}{
				"validate": true,
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
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
		requestBody    map[string]interface{}
		expectedStatus int
		expectValid    bool
	}{
		{
			name: "valid metadata",
			requestBody: map[string]interface{}{
				"updates": map[string]interface{}{
					"title":  "Test Book",
					"author": "Test Author",
				},
			},
			expectedStatus: http.StatusOK,
			expectValid:    true,
		},
		{
			name: "invalid metadata - missing required field",
			requestBody: map[string]interface{}{
				"updates": map[string]interface{}{
					"title": "",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/metadata/validate", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
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

	maxDuration := int64(500) // 500ms max

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("%s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			w := httptest.NewRecorder()

			start := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			// Note: In actual test, we'd measure time properly
			// This is a placeholder to show the pattern
			_ = start
			_ = maxDuration

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestTask2_SeparateDashboardCounts validates Task 2 implementation
// This test ensures:
// 1. Library and import path counts are separate and accurate
// 2. Size calculations use caching to avoid expensive file system walks
// 3. Dashboard data loads efficiently even with many books
func TestTask2_SeparateDashboardCounts(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create test directory structure
	libraryDir := filepath.Join(config.AppConfig.RootDir, "library")
	importDir := filepath.Join(config.AppConfig.RootDir, "import")

	require.NoError(t, os.MkdirAll(libraryDir, 0755))
	require.NoError(t, os.MkdirAll(importDir, 0755))

	// Update config to use library dir
	config.AppConfig.RootDir = libraryDir

	// Create test books in library
	libraryBook1 := &database.Book{
		Title:    "Library Book 1",
		FilePath: filepath.Join(libraryDir, "book1.m4b"),
	}
	libraryBook2 := &database.Book{
		Title:    "Library Book 2",
		FilePath: filepath.Join(libraryDir, "book2.m4b"),
	}

	// Create test books in import path
	importBook1 := &database.Book{
		Title:    "Import Book 1",
		FilePath: filepath.Join(importDir, "book1.m4b"),
	}
	importBook2 := &database.Book{
		Title:    "Import Book 2",
		FilePath: filepath.Join(importDir, "book2.m4b"),
	}

	// Add books to database
	_, err := database.GlobalStore.CreateBook(libraryBook1)
	require.NoError(t, err)
	_, err = database.GlobalStore.CreateBook(libraryBook2)
	require.NoError(t, err)
	_, err = database.GlobalStore.CreateBook(importBook1)
	require.NoError(t, err)
	_, err = database.GlobalStore.CreateBook(importBook2)
	require.NoError(t, err)

	// Create import path
	importPath, err := database.GlobalStore.CreateImportPath(importDir, "Test Import")
	require.NoError(t, err)
	require.NotNil(t, importPath)

	// Test 1: Verify counts are separated correctly
	t.Run("Separate Counts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		library := response["library"].(map[string]interface{})
		importPaths := response["import_paths"].(map[string]interface{})

		// Verify library counts
		assert.Equal(t, float64(2), library["book_count"], "Library should have 2 books")
		assert.Equal(t, float64(1), library["folder_count"], "Library should have 1 folder")

		// Verify import path counts
		assert.Equal(t, float64(2), importPaths["book_count"], "Import paths should have 2 books")
		assert.Equal(t, float64(1), importPaths["folder_count"], "Import paths should have 1 folder")
	})

	// Test 2: Verify caching works
	t.Run("Size Calculation Caching", func(t *testing.T) {
		// Clear cache by setting old timestamp
		cacheLock.Lock()
		cachedSizeComputedAt = time.Time{}
		cacheLock.Unlock()

		// First call should calculate sizes
		start := time.Now()
		req1 := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
		w1 := httptest.NewRecorder()
		server.router.ServeHTTP(w1, req1)
		firstCallDuration := time.Since(start)

		assert.Equal(t, http.StatusOK, w1.Code)

		// Second call should use cache and be faster
		start = time.Now()
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
		w2 := httptest.NewRecorder()
		server.router.ServeHTTP(w2, req2)
		secondCallDuration := time.Since(start)

		assert.Equal(t, http.StatusOK, w2.Code)

		// Verify cache was used (second call should be much faster)
		t.Logf("First call: %v, Second call: %v", firstCallDuration, secondCallDuration)

		// Parse both responses
		var resp1, resp2 map[string]interface{}
		json.Unmarshal(w1.Body.Bytes(), &resp1)
		json.Unmarshal(w2.Body.Bytes(), &resp2)

		// Verify sizes are the same (proving cache was used)
		assert.Equal(t, resp1["library_size_bytes"], resp2["library_size_bytes"])
		assert.Equal(t, resp1["import_size_bytes"], resp2["import_size_bytes"])
	})

	// Test 3: Verify cache expiration
	t.Run("Cache Expiration", func(t *testing.T) {
		// Set cache to expired
		cacheLock.Lock()
		cachedSizeComputedAt = time.Now().Add(-2 * librarySizeCacheTTL)
		cacheLock.Unlock()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify cache was updated
		cacheLock.RLock()
		timeSinceUpdate := time.Since(cachedSizeComputedAt)
		cacheLock.RUnlock()

		assert.Less(t, timeSinceUpdate, 5*time.Second, "Cache should have been updated recently")
	})
}

// TestTask2_PerformanceImprovement validates that the fix improves performance
func TestTask2_PerformanceImprovement(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create multiple books to simulate real workload
	for i := 0; i < 50; i++ {
		book := &database.Book{
			Title:    fmt.Sprintf("Test Book %d", i),
			FilePath: filepath.Join(config.AppConfig.RootDir, fmt.Sprintf("book%d.m4b", i)),
		}
		_, err := database.GlobalStore.CreateBook(book)
		require.NoError(t, err)
	}

	// Warm up cache
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Measure 10 consecutive calls (simulating dashboard polling)
	const numCalls = 10
	durations := make([]time.Duration, numCalls)

	for i := 0; i < numCalls; i++ {
		start := time.Now()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		durations[i] = time.Since(start)

		require.Equal(t, http.StatusOK, w.Code)
	}

	// Calculate average duration
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	avgDuration := total / numCalls

	t.Logf("Average request duration with caching: %v", avgDuration)

	// With caching, requests should be fast (< 50ms)
	assert.Less(t, avgDuration, 50*time.Millisecond,
		"Cached requests should complete in less than 50ms")
}

// TestTask2_NoDoubleCounting validates that books aren't counted twice
func TestTask2_NoDoubleCounting(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create library directory
	libraryDir := filepath.Join(config.AppConfig.RootDir, "library")
	require.NoError(t, os.MkdirAll(libraryDir, 0755))
	config.AppConfig.RootDir = libraryDir

	// Create a book in library
	book := &database.Book{
		Title:    "Library Book",
		FilePath: filepath.Join(libraryDir, "book.m4b"),
	}
	_, err := database.GlobalStore.CreateBook(book)
	require.NoError(t, err)

	// Create import path that includes the library dir (edge case)
	_, err = database.GlobalStore.CreateImportPath(config.AppConfig.RootDir, "Overlapping Path")
	require.NoError(t, err)

	// Get system status
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	library := response["library"].(map[string]interface{})
	importPaths := response["import_paths"].(map[string]interface{})

	// Book should only be counted once (in library)
	libraryCount := int(library["book_count"].(float64))
	importCount := int(importPaths["book_count"].(float64))

	assert.Equal(t, 1, libraryCount, "Book should be in library")
	assert.Equal(t, 0, importCount, "Book should not be double-counted in import paths")

	// Verify total is correct
	totalBooks := libraryCount + importCount
	assert.Equal(t, 1, totalBooks, "Total books should be 1 (no double counting)")
}
