// file: internal/itunes/fingerprint_integration_test.go
// version: 2.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package itunes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBackSafety_IntegrationFlow(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")

	// Step 1: Create initial library
	content := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Major Version</key><integer>1</integer>
<key>Minor Version</key><integer>1</integer>
<key>Tracks</key><dict></dict>
<key>Playlists</key><array></array>
</dict></plist>`)
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

	// Step 3: File unchanged — fingerprint should match
	currentFP, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatalf("ComputeFingerprint (unchanged) failed: %v", err)
	}
	if !importFP.Matches(currentFP) {
		t.Error("fingerprints should match for unchanged file")
	}

	// Step 4: Simulate external modification (iTunes syncs)
	modifiedContent := append(content, []byte("\n<!-- iTunes sync -->")...)
	if err := os.WriteFile(libPath, modifiedContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Step 5: Fingerprint should now differ
	modFP, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatalf("ComputeFingerprint (modified) failed: %v", err)
	}
	if importFP.Matches(modFP) {
		t.Error("fingerprints should differ after external modification")
	}
	if modFP.Size == importFP.Size && modFP.CRC32 == importFP.CRC32 {
		t.Error("stored and current fingerprints should differ in size or CRC32")
	}

	// Step 6: After re-reading, fingerprint should be stable
	modFP2, err := ComputeFingerprint(libPath)
	if err != nil {
		t.Fatalf("ComputeFingerprint (second read) failed: %v", err)
	}
	if !modFP.Matches(modFP2) {
		t.Error("fingerprints of same file should be stable across reads")
	}
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
