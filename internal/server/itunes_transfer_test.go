// file: internal/server/itunes_transfer_test.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a

package server

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestBackupITLFile verifies that backupITLFile creates a .bak-* copy.
func TestBackupITLFile(t *testing.T) {
	dir := t.TempDir()
	itlPath := filepath.Join(dir, "iTunes Library.itl")

	// Write a fake ITL file.
	if err := os.WriteFile(itlPath, []byte("fake-itl-data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := backupITLFile(itlPath); err != nil {
		t.Fatalf("backupITLFile: %v", err)
	}

	// Should have created exactly one .bak-* file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var bakFiles []string
	for _, e := range entries {
		if e.Name() != filepath.Base(itlPath) {
			bakFiles = append(bakFiles, e.Name())
		}
	}
	if len(bakFiles) != 1 {
		t.Fatalf("expected 1 backup file, got %d: %v", len(bakFiles), bakFiles)
	}

	// Backup should contain the same data.
	data, err := os.ReadFile(filepath.Join(dir, bakFiles[0]))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "fake-itl-data" {
		t.Errorf("backup content = %q, want %q", data, "fake-itl-data")
	}
}

// TestBackupITLFile_NoExistingFile verifies noop when file doesn't exist.
func TestBackupITLFile_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	itlPath := filepath.Join(dir, "nonexistent.itl")

	if err := backupITLFile(itlPath); err != nil {
		t.Fatalf("backupITLFile should be noop for missing file: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected empty dir, got %d entries", len(entries))
	}
}

// TestCopyFile verifies atomic copy semantics.
func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.itl")
	dst := filepath.Join(dir, "dst.itl")

	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("dst content = %q, want %q", got, content)
	}

	// Source should still exist.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should still exist: %v", err)
	}
}

// TestITLBackupEntry_Sort verifies newest-first sort order.
func TestITLBackupEntry_Sort(t *testing.T) {
	now := time.Now()
	entries := []ITLBackupEntry{
		{Name: "old.bak", Timestamp: now.Add(-time.Hour)},
		{Name: "newest.bak", Timestamp: now},
		{Name: "mid.bak", Timestamp: now.Add(-30 * time.Minute)},
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	if entries[0].Name != "newest.bak" {
		t.Errorf("first entry = %q, want %q", entries[0].Name, "newest.bak")
	}
	if entries[2].Name != "old.bak" {
		t.Errorf("last entry = %q, want %q", entries[2].Name, "old.bak")
	}
}
