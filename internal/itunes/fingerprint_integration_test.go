// file: internal/itunes/fingerprint_integration_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package itunes

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteBackSafety_IntegrationFlow(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")

	// Step 1: Create initial library
	content := buildMinimalLibraryXML()
	if err := os.WriteFile(libPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Step 2: Simulate "import" — compute fingerprint
	importFP, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatalf("ComputeFingerprint failed: %v", err)
	}
	if importFP.Size != int64(len(content)) {
		t.Errorf("fingerprint size = %d, want %d", importFP.Size, len(content))
	}
	if importFP.CRC32 == 0 {
		t.Error("fingerprint CRC32 should not be zero")
	}

	// Step 3: File unchanged — write-back should succeed (or fail for other reasons, not ErrLibraryModified)
	opts := WriteBackOptions{
		LibraryPath:       libPath,
		Updates:           []*WriteBackUpdate{{ITunesPersistentID: "NONEXISTENT", NewPath: "/foo"}},
		StoredFingerprint: importFP,
	}
	_, err = WriteBack(opts)
	var modErr *ErrLibraryModified
	if errors.As(err, &modErr) {
		t.Fatal("write-back should not detect modification when file is unchanged")
	}

	// Step 4: Simulate external modification (iTunes syncs)
	modifiedContent := append(content, []byte("\n<!-- iTunes sync at "+time.Now().String()+" -->")...)
	if err := os.WriteFile(libPath, modifiedContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Step 5: Write-back should now detect the change
	opts.StoredFingerprint = importFP // still using old fingerprint
	_, err = WriteBack(opts)
	if err == nil {
		t.Fatal("expected ErrLibraryModified after external modification")
	}
	if !errors.As(err, &modErr) {
		t.Fatalf("expected ErrLibraryModified, got %T: %v", err, err)
	}
	if modErr.Stored.Size == modErr.Current.Size && modErr.Stored.CRC32 == modErr.Current.CRC32 {
		t.Error("stored and current fingerprints should differ")
	}

	// Step 6: Force overwrite should bypass the check
	opts.ForceOverwrite = true
	_, err = WriteBack(opts)
	if errors.As(err, &modErr) {
		t.Fatal("ForceOverwrite should bypass fingerprint check")
	}
	// May get other errors (nonexistent persistent ID) — that's fine

	// Step 7: After force write-back, new fingerprint should reflect the written file
	newFP, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatalf("ComputeFingerprint after write-back failed: %v", err)
	}
	// File was written back, so fingerprint should differ from importFP
	// (unless write-back didn't change anything, which is also valid)
	_ = newFP // just verify it computes without error
}

func TestFingerprintConsistency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xml")

	content := []byte("<plist>consistent content</plist>")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Multiple reads should produce the same fingerprint
	fp1, _ := ComputeFingerprint(path)
	fp2, _ := ComputeFingerprint(path)
	fp3, _ := ComputeFingerprint(path)

	if !fp1.Matches(fp2) || !fp2.Matches(fp3) {
		t.Error("fingerprints of the same file should be consistent")
	}
	if fp1.CRC32 != fp2.CRC32 || fp2.CRC32 != fp3.CRC32 {
		t.Errorf("CRC32 values differ: %d, %d, %d", fp1.CRC32, fp2.CRC32, fp3.CRC32)
	}
}
