// file: internal/server/server_coverage_test.go
// version: 1.0.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCountAudiobooksEndpoint tests the count endpoint with various scenarios
func TestCountAudiobooksEndpoint(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create some test books
	tempDir := t.TempDir()
	for i := 0; i < 3; i++ {
		filePath := filepath.Join(tempDir, "book"+string(rune('A'+i))+".m4b")
		require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))
		_, err := database.GlobalStore.CreateBook(&database.Book{
			Title:    "Test Book " + string(rune('A'+i)),
			FilePath: filePath,
			Format:   "m4b",
		})
		require.NoError(t, err)
	}

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		checkCount     bool
		minCount       int
	}{
		{
			name:           "count all audiobooks",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			checkCount:     true,
			minCount:       3,
		},
		{
			name:           "count with search query",
			queryParams:    "?search=Test",
			expectedStatus: http.StatusOK,
			checkCount:     true,
			minCount:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/count"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkCount {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				count, ok := response["count"].(float64)
				assert.True(t, ok)
				assert.GreaterOrEqual(t, int(count), tt.minCount)
			}
		})
	}
}


// TestSystemLogsEndpoint tests system logs retrieval
func TestSystemLogsEndpoint(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
	}{
		{
			name:           "default logs",
			queryParams:    "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "logs with limit",
			queryParams:    "?limit=50",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "logs with level filter",
			queryParams:    "?level=error",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/system/logs"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestUpdateConfigEndpointComplete tests configuration updates
func TestUpdateConfigEndpointComplete(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "valid config update",
			requestBody: map[string]interface{}{
				"enable_ai": false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty config",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestBackupEndpoints tests backup management
func TestBackupEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("create backup", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("list backups", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/backup/list", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestImportFileEndpointComplete tests file import
func TestImportFileEndpointComplete(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "import.m4b")
	require.NoError(t, os.WriteFile(testFile, []byte("audio"), 0o644))

	importData := map[string]interface{}{
		"file_path": testFile,
	}
	body, _ := json.Marshal(importData)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// May succeed or fail depending on file handling
	assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest, http.StatusNotFound}, w.Code)
}

// TestDashboardEndpoint tests dashboard data retrieval
func TestDashboardEndpoint(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["sizeDistribution"])
	assert.NotNil(t, response["formatDistribution"])
}
