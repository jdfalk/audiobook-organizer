// file: internal/server/scan_service_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-b8c9-d0e1-f2a3b4c5d6e7

package server

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestScanService_DetermineFoldersToScan_SpecificFolder(t *testing.T) {
	mockDB := &database.MockStore{}
	ss := NewScanService(mockDB)

	mockProgress := &mockProgressReporter{}
	folderPath := "/test/folder"
	req := &ScanRequest{FolderPath: &folderPath}

	folders, err := ss.determineFoldersToScan(req.FolderPath, false, mockProgress)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(folders) != 1 || folders[0] != "/test/folder" {
		t.Errorf("expected ['/test/folder'], got %v", folders)
	}
}

func TestScanService_DetermineFoldersToScan_AllImportPaths(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{
				{Path: "/import/path1", Enabled: true},
				{Path: "/import/path2", Enabled: false},
				{Path: "/import/path3", Enabled: true},
			}, nil
		},
	}
	ss := NewScanService(mockDB)

	mockProgress := &mockProgressReporter{}
	folders, err := ss.determineFoldersToScan(nil, false, mockProgress)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	// Should include only enabled import paths
	if len(folders) != 2 {
		t.Errorf("expected 2 folders, got %d", len(folders))
	}
}

func TestScanService_PerformScan_NoFolders(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{}, nil
		},
	}
	ss := NewScanService(mockDB)

	ctx := context.Background()
	mockProgress := &mockProgressReporter{}
	req := &ScanRequest{}

	err := ss.PerformScan(ctx, req, mockProgress)

	if err != nil {
		t.Errorf("expected no error for empty folders, got %v", err)
	}
}

type mockProgressReporter struct{}

func (m *mockProgressReporter) Log(level, message string, details *string) error {
	return nil
}

func (m *mockProgressReporter) UpdateProgress(current, total int, message string) error {
	return nil
}

func (m *mockProgressReporter) IsCanceled() bool {
	return false
}
