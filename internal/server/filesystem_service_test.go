// file: internal/server/filesystem_service_test.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-5c6d-7e8f-9a0b1c2d3e4f

package server

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemService_BrowseDirectory_Empty(t *testing.T) {
	fs := NewFilesystemService()

	_, err := fs.BrowseDirectory("")

	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestFilesystemService_BrowseDirectory_InvalidPath(t *testing.T) {
	fs := NewFilesystemService()

	_, err := fs.BrowseDirectory("/nonexistent/path/that/does/not/exist")

	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestFilesystemService_CreateExclusion_Success(t *testing.T) {
	fs := NewFilesystemService()

	tmpDir, err := ioutil.TempDir("", "test-exclusion")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = fs.CreateExclusion(tmpDir)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	excludeFile := filepath.Join(tmpDir, ".jabexclude")
	if _, err := os.Stat(excludeFile); err != nil {
		t.Errorf("expected .jabexclude file to exist, got %v", err)
	}
}

func TestFilesystemService_RemoveExclusion_NotFound(t *testing.T) {
	fs := NewFilesystemService()

	tmpDir, err := ioutil.TempDir("", "test-remove-exclusion")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = fs.RemoveExclusion(tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent exclusion")
	}
}
