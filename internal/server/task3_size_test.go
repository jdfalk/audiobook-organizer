// file: internal/server/task3_size_test.go
// version: 1.0.1
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearSizeCache clears the library size cache for testing
func clearSizeCache() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cachedLibrarySize = 0
	cachedImportSize = 0
	cachedSizeComputedAt = time.Time{}
}

// TestCalculateLibrarySizesNoNegative verifies that importSize is never negative
func TestCalculateLibrarySizesNoNegative(t *testing.T) {
	clearSizeCache()

	// Create temporary directories
	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	importDir, err := os.MkdirTemp("", "import-*")
	require.NoError(t, err)
	defer os.RemoveAll(importDir)

	// Create files in library (larger size)
	for i := 0; i < 10; i++ {
		data := make([]byte, 1024*100) // 100KB each
		err := os.WriteFile(filepath.Join(rootDir, "book"+string(rune(i+'0'))+".m4b"), data, 0644)
		require.NoError(t, err)
	}

	// Create files in import (smaller size)
	for i := 0; i < 5; i++ {
		data := make([]byte, 1024*50) // 50KB each
		err := os.WriteFile(filepath.Join(importDir, "import"+string(rune(i+'0'))+".m4b"), data, 0644)
		require.NoError(t, err)
	}

	importFolders := []database.ImportPath{
		{Path: importDir, Enabled: true},
	}

	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)

	// Verify no negative values
	assert.GreaterOrEqual(t, librarySize, int64(0), "Library size should never be negative")
	assert.GreaterOrEqual(t, importSize, int64(0), "Import size should never be negative")

	// Verify sizes are calculated independently (not by subtraction)
	assert.Greater(t, librarySize, int64(0), "Library should have content")
	assert.Greater(t, importSize, int64(0), "Import should have content")

	// Library size should be ~1MB (10 * 100KB)
	assert.InDelta(t, 1024*1024, librarySize, 1024*50, "Library size should be ~1MB")

	// Import size should be ~250KB (5 * 50KB)
	assert.InDelta(t, 1024*250, importSize, 1024*50, "Import size should be ~250KB")
}

// TestCalculateLibrarySizesIndependentCalculation ensures import and library sizes are independent
func TestCalculateLibrarySizesIndependentCalculation(t *testing.T) {
	clearSizeCache()

	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	importDir, err := os.MkdirTemp("", "import-*")
	require.NoError(t, err)
	defer os.RemoveAll(importDir)

	// Create library file
	libraryData := make([]byte, 1024*200) // 200KB
	err = os.WriteFile(filepath.Join(rootDir, "library.m4b"), libraryData, 0644)
	require.NoError(t, err)

	// Create import file
	importData := make([]byte, 1024*300) // 300KB
	err = os.WriteFile(filepath.Join(importDir, "import.m4b"), importData, 0644)
	require.NoError(t, err)

	importFolders := []database.ImportPath{
		{Path: importDir, Enabled: true},
	}

	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)

	// Each should reflect only its own content
	assert.InDelta(t, 1024*200, librarySize, 1024*10, "Library size should match library file")
	assert.InDelta(t, 1024*300, importSize, 1024*10, "Import size should match import file")

	// Total should be sum
	expectedTotal := librarySize + importSize
	assert.InDelta(t, 1024*500, expectedTotal, 1024*20, "Total should be sum of both")
}

// TestCalculateLibrarySizesNoDoubleCounting ensures files in overlapping paths aren't double-counted
func TestCalculateLibrarySizesNoDoubleCounting(t *testing.T) {
	clearSizeCache()
	
	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	// Create subdirectory in root
	subDir := filepath.Join(rootDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// Create files in root
	rootData := make([]byte, 1024*100)
	err = os.WriteFile(filepath.Join(rootDir, "root.m4b"), rootData, 0644)
	require.NoError(t, err)

	// Create files in subdirectory
	subData := make([]byte, 1024*50)
	err = os.WriteFile(filepath.Join(subDir, "sub.m4b"), subData, 0644)
	require.NoError(t, err)

	// Add subdirectory as import path
	importFolders := []database.ImportPath{
		{Path: subDir, Enabled: true},
	}

	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)

	// Library should include both files
	assert.InDelta(t, 1024*150, librarySize, 1024*20, "Library should include root and subdir")

	// Import should be 0 (subdir is under rootDir, should be skipped)
	assert.Equal(t, int64(0), importSize, "Import should be 0 (files already in library)")
}

// TestCalculateLibrarySizesEnabledCheck verifies only enabled import paths are counted
func TestCalculateLibrarySizesEnabledCheck(t *testing.T) {
	clearSizeCache()
	
	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	importDir1, err := os.MkdirTemp("", "import1-*")
	require.NoError(t, err)
	defer os.RemoveAll(importDir1)

	importDir2, err := os.MkdirTemp("", "import2-*")
	require.NoError(t, err)
	defer os.RemoveAll(importDir2)

	// Create files
	data1 := make([]byte, 1024*100)
	err = os.WriteFile(filepath.Join(importDir1, "file1.m4b"), data1, 0644)
	require.NoError(t, err)

	data2 := make([]byte, 1024*200)
	err = os.WriteFile(filepath.Join(importDir2, "file2.m4b"), data2, 0644)
	require.NoError(t, err)

	importFolders := []database.ImportPath{
		{Path: importDir1, Enabled: true},
		{Path: importDir2, Enabled: false}, // Disabled
	}

	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)

	// Library should be empty
	assert.Equal(t, int64(0), librarySize, "Library should be empty")

	// Import should only include enabled path
	assert.InDelta(t, 1024*100, importSize, 1024*10, "Import should only count enabled path")
}

// TestCalculateLibrarySizesCaching verifies that caching works correctly
func TestCalculateLibrarySizesCaching(t *testing.T) {
	clearSizeCache()
	
	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	data := make([]byte, 1024*100)
	err = os.WriteFile(filepath.Join(rootDir, "book.m4b"), data, 0644)
	require.NoError(t, err)

	importFolders := []database.ImportPath{}

	// First call should calculate
	lib1, imp1 := calculateLibrarySizes(rootDir, importFolders)

	// Second call should use cache
	lib2, imp2 := calculateLibrarySizes(rootDir, importFolders)

	assert.Equal(t, lib1, lib2, "Cached library size should match")
	assert.Equal(t, imp1, imp2, "Cached import size should match")
	assert.Greater(t, lib1, int64(0), "Library size should be positive")
}

// TestCalculateLibrarySizesEmptyDirectories verifies handling of empty directories
func TestCalculateLibrarySizesEmptyDirectories(t *testing.T) {
	clearSizeCache()
	
	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	importDir, err := os.MkdirTemp("", "import-*")
	require.NoError(t, err)
	defer os.RemoveAll(importDir)

	// Don't create any files - directories are empty

	importFolders := []database.ImportPath{
		{Path: importDir, Enabled: true},
	}

	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)

	assert.Equal(t, int64(0), librarySize, "Empty library should have 0 size")
	assert.Equal(t, int64(0), importSize, "Empty import should have 0 size")
}

// TestCalculateLibrarySizesLargeFiles verifies handling of large files (overflow prevention)
func TestCalculateLibrarySizesLargeFiles(t *testing.T) {
	clearSizeCache()
	
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	rootDir, err := os.MkdirTemp("", "library-*")
	require.NoError(t, err)
	defer os.RemoveAll(rootDir)

	// Create multiple large files to test int64 handling
	for i := 0; i < 5; i++ {
		data := make([]byte, 1024*1024*100) // 100MB each
		err := os.WriteFile(filepath.Join(rootDir, "large"+string(rune(i+'0'))+".m4b"), data, 0644)
		require.NoError(t, err)
	}

	librarySize, importSize := calculateLibrarySizes(rootDir, []database.ImportPath{})

	// Should handle 500MB without overflow
	assert.Greater(t, librarySize, int64(0), "Should handle large files")
	assert.Less(t, librarySize, int64(1)<<62, "Should not overflow int64")
	assert.Equal(t, int64(0), importSize, "No import paths")
}

// TestCalculateLibrarySizesNonExistentPaths verifies handling of non-existent paths
func TestCalculateLibrarySizesNonExistentPaths(t *testing.T) {
	clearSizeCache()
	
	rootDir := "/nonexistent/library/path"
	importDir := "/nonexistent/import/path"

	importFolders := []database.ImportPath{
		{Path: importDir, Enabled: true},
	}

	// Should not crash on non-existent paths
	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)

	assert.Equal(t, int64(0), librarySize, "Non-existent library should have 0 size")
	assert.Equal(t, int64(0), importSize, "Non-existent import should have 0 size")
}
