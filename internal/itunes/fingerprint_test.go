// file: internal/itunes/fingerprint_test.go
// version: 1.0.0
// guid: c7d8e9f0-a1b2-3c4d-5e6f-7a8b9c0d1e2f

package itunes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFingerprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xml")
	content := []byte("<plist>test content</plist>")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	fp, err := ComputeFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}

	if fp.Path != path {
		t.Errorf("Path = %q, want %q", fp.Path, path)
	}
	if fp.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", fp.Size, int64(len(content)))
	}
	if fp.CRC32 == 0 {
		t.Error("CRC32 should not be zero")
	}
	if fp.ModTime.IsZero() {
		t.Error("ModTime should not be zero")
	}
}

func TestComputeFingerprint_FileNotFound(t *testing.T) {
	_, err := ComputeFingerprint("/nonexistent/file.xml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLibraryFingerprint_Matches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xml")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	fp1, _ := ComputeFingerprint(path)
	fp2, _ := ComputeFingerprint(path)

	if !fp1.Matches(fp2) {
		t.Error("identical fingerprints should match")
	}

	// Modify file
	if err := os.WriteFile(path, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	fp3, _ := ComputeFingerprint(path)
	if fp1.Matches(fp3) {
		t.Error("different fingerprints should not match")
	}
}

func TestLibraryFingerprint_Matches_NilHandling(t *testing.T) {
	var fp *LibraryFingerprint
	other := &LibraryFingerprint{Size: 1, CRC32: 123}
	if fp.Matches(other) {
		t.Error("nil fingerprint should not match")
	}
	if other.Matches(nil) {
		t.Error("should not match nil")
	}
}
