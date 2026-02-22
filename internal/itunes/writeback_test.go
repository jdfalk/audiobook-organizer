// file: internal/itunes/writeback_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package itunes

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func buildMinimalLibraryXML() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Major Version</key><integer>1</integer>
<key>Minor Version</key><integer>1</integer>
<key>Tracks</key><dict></dict>
<key>Playlists</key><array></array>
</dict></plist>`)
}

func TestWriteBack_DetectsModifiedLibrary(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")

	initialContent := buildMinimalLibraryXML()
	if err := os.WriteFile(libPath, initialContent, 0644); err != nil {
		t.Fatal(err)
	}

	fp, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate external modification
	if err := os.WriteFile(libPath, append(initialContent, []byte("<!-- modified -->")...), 0644); err != nil {
		t.Fatal(err)
	}

	opts := WriteBackOptions{
		LibraryPath:       libPath,
		Updates:           []*WriteBackUpdate{{ITunesPersistentID: "ABC", NewPath: "/new/path"}},
		ForceOverwrite:    false,
		StoredFingerprint: fp,
	}

	_, err = WriteBack(opts)
	if err == nil {
		t.Fatal("expected ErrLibraryModified, got nil")
	}

	var modErr *ErrLibraryModified
	if !errors.As(err, &modErr) {
		t.Fatalf("expected ErrLibraryModified, got %T: %v", err, err)
	}
}

func TestWriteBack_ForceOverwriteSkipsCheck(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")
	if err := os.WriteFile(libPath, buildMinimalLibraryXML(), 0644); err != nil {
		t.Fatal(err)
	}

	fp, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatal(err)
	}

	// Modify file
	if err := os.WriteFile(libPath, append(buildMinimalLibraryXML(), []byte("<!-- changed -->")...), 0644); err != nil {
		t.Fatal(err)
	}

	opts := WriteBackOptions{
		LibraryPath:       libPath,
		Updates:           []*WriteBackUpdate{}, // empty updates = "no updates" error, not ErrLibraryModified
		ForceOverwrite:    true,
		StoredFingerprint: fp,
	}

	_, err = WriteBack(opts)
	var modErr *ErrLibraryModified
	if errors.As(err, &modErr) {
		t.Fatal("ForceOverwrite should skip fingerprint check")
	}
	// It's OK if we get "no updates provided" error - that proves we bypassed the fingerprint check
}

func TestWriteBack_NilFingerprintSkipsCheck(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")
	if err := os.WriteFile(libPath, buildMinimalLibraryXML(), 0644); err != nil {
		t.Fatal(err)
	}

	opts := WriteBackOptions{
		LibraryPath:       libPath,
		Updates:           []*WriteBackUpdate{}, // will fail with "no updates" but NOT ErrLibraryModified
		StoredFingerprint: nil,                  // nil = skip check
	}

	_, err := WriteBack(opts)
	var modErr *ErrLibraryModified
	if errors.As(err, &modErr) {
		t.Fatal("nil StoredFingerprint should skip fingerprint check")
	}
}
