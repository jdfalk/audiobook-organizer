// file: internal/fileops/service_test.go
// version: 1.1.0
// guid: c9d0e1f2-a3b4-5c6d-7e8f-9a0b1c2d3e4f
// last-edited: 2026-05-15

package fileops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/database/mocks"
)

func TestFilesystemService_BrowseDirectory_Empty(t *testing.T) {
	mockStore := new(mocks.MockImportPathStore)
	mockStore.On("GetAllImportPaths").Return([]database.ImportPath{}, nil)
	fs := NewFilesystemService(mockStore)

	_, err := fs.BrowseDirectory(context.Background(), "")

	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestFilesystemService_BrowseDirectory_InvalidPath(t *testing.T) {
	mockStore := new(mocks.MockImportPathStore)
	mockStore.On("GetAllImportPaths").Return([]database.ImportPath{}, nil)
	fs := NewFilesystemService(mockStore)

	_, err := fs.BrowseDirectory(context.Background(), "/nonexistent/path/that/does/not/exist")

	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestFilesystemService_CreateExclusion_Success(t *testing.T) {
	mockStore := new(mocks.MockImportPathStore)
	fs := NewFilesystemService(mockStore)

	tmpDir, err := os.MkdirTemp("", "test-exclusion")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = fs.CreateExclusion(context.Background(), tmpDir)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	excludeFile := filepath.Join(tmpDir, ".jabexclude")
	if _, err := os.Stat(excludeFile); err != nil {
		t.Errorf("expected .jabexclude file to exist, got %v", err)
	}
}

func TestFilesystemService_RemoveExclusion_NotFound(t *testing.T) {
	mockStore := new(mocks.MockImportPathStore)
	fs := NewFilesystemService(mockStore)

	tmpDir, err := os.MkdirTemp("", "test-remove-exclusion")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = fs.RemoveExclusion(context.Background(), tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent exclusion")
	}
}
