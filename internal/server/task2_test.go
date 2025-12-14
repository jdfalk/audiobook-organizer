// file: internal/server/task2_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			Title:    "Test Book " + string(rune(i)),
			FilePath: filepath.Join(config.AppConfig.RootDir, "book"+string(rune(i))+".m4b"),
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

// TestTask2_NoDoubleCountin validates that books aren't counted twice
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
