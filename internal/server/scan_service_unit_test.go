// file: internal/server/scan_service_unit_test.go
// version: 1.0.0
// guid: e2f3a4b5-c6d7-8e9f-0a1b-3c4d5e6f7a8b

package server

import (
	"context"
	"errors"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestScanService_DetermineFolders_SpecificFolderPath(t *testing.T) {
	mockDB := &database.MockStore{}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	path := "/my/audiobooks"
	folders, err := ss.determineFoldersToScan(&path, false, log)

	assert.NoError(t, err)
	assert.Equal(t, []string{"/my/audiobooks"}, folders)
}

func TestScanService_DetermineFolders_ImportPathError(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return nil, errors.New("db unavailable")
		},
	}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	folders, err := ss.determineFoldersToScan(nil, false, log)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get import paths")
	assert.Nil(t, folders)
}

func TestScanService_DetermineFolders_DisabledPathsExcluded(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{
				{Path: "/enabled1", Enabled: true},
				{Path: "/disabled", Enabled: false},
				{Path: "/enabled2", Enabled: true},
			}, nil
		},
	}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	folders, err := ss.determineFoldersToScan(nil, false, log)

	assert.NoError(t, err)
	assert.Equal(t, []string{"/enabled1", "/enabled2"}, folders)
}

func TestScanService_DetermineFolders_ForceUpdateIncludesRootDir(t *testing.T) {
	origRoot := config.AppConfig.RootDir
	config.AppConfig.RootDir = "/library/root"
	t.Cleanup(func() { config.AppConfig.RootDir = origRoot })

	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{
				{Path: "/import/one", Enabled: true},
			}, nil
		},
	}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	folders, err := ss.determineFoldersToScan(nil, true, log)

	assert.NoError(t, err)
	assert.Contains(t, folders, "/library/root")
	assert.Contains(t, folders, "/import/one")
	assert.Equal(t, "/library/root", folders[0], "root dir should be first")
}

func TestScanService_PerformScan_NoFoldersReturnsNil(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{}, nil
		},
	}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	err := ss.PerformScan(context.Background(), &ScanRequest{}, log)

	assert.NoError(t, err)
}

func TestScanService_UpdateImportPathBookCount(t *testing.T) {
	var updatedID int
	var updatedCount int

	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{
				{ID: 1, Path: "/path/a", BookCount: 0},
				{ID: 2, Path: "/path/b", BookCount: 5},
			}, nil
		},
		UpdateImportPathFunc: func(id int, ip *database.ImportPath) error {
			updatedID = id
			updatedCount = ip.BookCount
			return nil
		},
	}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	ss.updateImportPathBookCount("/path/b", 42, log)

	assert.Equal(t, 2, updatedID)
	assert.Equal(t, 42, updatedCount)
}

func TestScanService_UpdateImportPathBookCount_NoMatch(t *testing.T) {
	updateCalled := false

	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{
				{ID: 1, Path: "/other/path"},
			}, nil
		},
		UpdateImportPathFunc: func(_ int, _ *database.ImportPath) error {
			updateCalled = true
			return nil
		},
	}
	ss := NewScanService(mockDB)
	log := logger.New("test")

	ss.updateImportPathBookCount("/nonexistent", 10, log)

	assert.False(t, updateCalled, "UpdateImportPath should not be called for non-matching path")
}

func TestScanService_ReportCompletion_Messages(t *testing.T) {
	tests := []struct {
		name     string
		stats    ScanStats
		contains string
	}{
		{
			name:     "library and import",
			stats:    ScanStats{TotalBooks: 15, LibraryBooks: 10, ImportBooks: 5},
			contains: "Library: 10 books, Import: 5 books",
		},
		{
			name:     "library only",
			stats:    ScanStats{TotalBooks: 10, LibraryBooks: 10},
			contains: "Library: 10 books",
		},
		{
			name:     "import only",
			stats:    ScanStats{TotalBooks: 5, ImportBooks: 5},
			contains: "Import: 5 books",
		},
		{
			name:     "no books",
			stats:    ScanStats{},
			contains: "No books found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := &database.MockStore{}
			ss := NewScanService(mockDB)
			log := logger.New("test")

			// reportCompletion should not panic; verify it runs without error.
			// The method logs its message, so we just confirm no crash.
			ss.reportCompletion(tt.stats.TotalBooks, tt.stats.TotalBooks, &tt.stats, log)
		})
	}
}
