// file: internal/scanner/process_file_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir returns the absolute path to the project testdata/fixtures directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	// The test binary runs with the package directory as cwd, but the testdata
	// dir is two levels up (internal/scanner → repo root).
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "testdata", "fixtures")
}

func TestProcessFile_EmptyPath(t *testing.T) {
	_, _, _, err := ProcessFile("")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestProcessFile_NonExistentFile(t *testing.T) {
	_, _, _, err := ProcessFile("/tmp/audiobook-organizer-nonexistent-file-xyz.mp3")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestProcessFile_Directory(t *testing.T) {
	dir := t.TempDir()

	meta, mi, hash, err := ProcessFile(dir)
	if err != nil {
		t.Fatalf("expected no error for directory, got: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata for directory, got nil")
	}
	if mi != nil {
		t.Fatalf("expected nil mediainfo for directory, got: %+v", mi)
	}
	if hash != "" {
		t.Fatalf("expected empty hash for directory, got: %q", hash)
	}
}

func TestProcessFile_MP3(t *testing.T) {
	fixtures := testdataDir(t)
	mp3Path := filepath.Join(fixtures, "test_sample.mp3")
	if _, err := os.Stat(mp3Path); os.IsNotExist(err) {
		t.Skipf("test fixture not found at %s, skipping", mp3Path)
	}

	meta, mi, hash, err := ProcessFile(mp3Path)
	if err != nil {
		t.Fatalf("ProcessFile(%q) returned error: %v", mp3Path, err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if mi == nil {
		t.Fatal("expected non-nil mediainfo for MP3")
	}
	if hash == "" {
		t.Fatal("expected non-empty hash for MP3")
	}
	if len(hash) != 64 {
		t.Fatalf("expected 64-char SHA-256 hex hash, got %d chars: %q", len(hash), hash)
	}
}

func TestProcessFile_M4B(t *testing.T) {
	fixtures := testdataDir(t)
	m4bPath := filepath.Join(fixtures, "test_sample.m4b")
	if _, err := os.Stat(m4bPath); os.IsNotExist(err) {
		t.Skipf("test fixture not found at %s, skipping", m4bPath)
	}

	meta, mi, hash, err := ProcessFile(m4bPath)
	if err != nil {
		t.Fatalf("ProcessFile(%q) returned error: %v", m4bPath, err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if mi == nil {
		t.Fatal("expected non-nil mediainfo for M4B")
	}
	if hash == "" {
		t.Fatal("expected non-empty hash for M4B")
	}
}

func TestProcessFile_FLAC(t *testing.T) {
	fixtures := testdataDir(t)
	flacPath := filepath.Join(fixtures, "test_sample.flac")
	if _, err := os.Stat(flacPath); os.IsNotExist(err) {
		t.Skipf("test fixture not found at %s, skipping", flacPath)
	}

	meta, mi, hash, err := ProcessFile(flacPath)
	if err != nil {
		t.Fatalf("ProcessFile(%q) returned error: %v", flacPath, err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if mi == nil {
		t.Fatal("expected non-nil mediainfo for FLAC")
	}
	if hash == "" {
		t.Fatal("expected non-empty hash for FLAC")
	}
}

// TestProcessFile_HashConsistency verifies that ProcessFile produces the same
// hash as ComputeFileHash for the same file.
func TestProcessFile_HashConsistency(t *testing.T) {
	fixtures := testdataDir(t)
	mp3Path := filepath.Join(fixtures, "test_sample.mp3")
	if _, err := os.Stat(mp3Path); os.IsNotExist(err) {
		t.Skipf("test fixture not found at %s, skipping", mp3Path)
	}

	_, _, hashFromProcessFile, err := ProcessFile(mp3Path)
	if err != nil {
		t.Fatalf("ProcessFile error: %v", err)
	}

	hashFromComputeFileHash, err := ComputeFileHash(mp3Path)
	if err != nil {
		t.Fatalf("ComputeFileHash error: %v", err)
	}

	if hashFromProcessFile != hashFromComputeFileHash {
		t.Fatalf("hash mismatch: ProcessFile=%q, ComputeFileHash=%q", hashFromProcessFile, hashFromComputeFileHash)
	}
}
