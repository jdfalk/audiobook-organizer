// file: internal/plugins/acoustid/backfill_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-4f3a-4b5c-6d7e8f9a0123

package acoustid

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// makeBookFile builds a BookFile with sensible defaults plus the overrides
// in mods. Defaults reflect a "freshly scanned, not yet fingerprinted" row.
func makeBookFile(mods func(*database.BookFile)) database.BookFile {
	f := database.BookFile{
		ID:       "bf-test",
		BookID:   "book-test",
		FilePath: "/tmp/does-not-exist.m4b",
	}
	if mods != nil {
		mods(&f)
	}
	return f
}

func TestFingerprintEligibility_SkipsWhenWholeFilePresent(t *testing.T) {
	f := makeBookFile(func(bf *database.BookFile) {
		bf.AcoustIDFingerprint = []byte("not really a fingerprint but non-empty")
	})
	got, _, stop := fingerprintEligibility(f, false)
	if !stop {
		t.Fatal("expected stop=true when whole-file fp already present")
	}
	if got != fingerprintOutcomeSkipped {
		t.Errorf("expected skipped, got %v", got)
	}
}

func TestFingerprintEligibility_SkipsWhenSeg0Present(t *testing.T) {
	f := makeBookFile(func(bf *database.BookFile) {
		bf.AcoustIDSeg0 = "AQADtAcSRY"
	})
	got, _, stop := fingerprintEligibility(f, false)
	if !stop {
		t.Fatal("expected stop=true when seg0 already present")
	}
	if got != fingerprintOutcomeSkipped {
		t.Errorf("expected skipped, got %v", got)
	}
}

func TestFingerprintEligibility_ForceOverridesPresentSeg0(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.m4b")
	if err := os.WriteFile(path, []byte("not real audio but the path resolves"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := makeBookFile(func(bf *database.BookFile) {
		bf.FilePath = path
		bf.AcoustIDSeg0 = "AQADtAcSRY"
	})
	got, _, stop := fingerprintEligibility(f, true) // force
	if stop {
		t.Fatalf("expected stop=false with force=true, got stop=true outcome=%v", got)
	}
}

func TestFingerprintEligibility_ForceOverridesWholeFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.flac")
	if err := os.WriteFile(path, []byte("not real audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := makeBookFile(func(bf *database.BookFile) {
		bf.FilePath = path
		bf.AcoustIDFingerprint = []byte("existing fp bytes")
	})
	got, _, stop := fingerprintEligibility(f, true)
	if stop {
		t.Fatalf("expected stop=false with force=true, got stop=true outcome=%v", got)
	}
}

func TestFingerprintEligibility_IneligibleWhenMissing(t *testing.T) {
	f := makeBookFile(func(bf *database.BookFile) { bf.Missing = true })
	got, _, stop := fingerprintEligibility(f, false)
	if !stop {
		t.Fatal("expected stop=true when Missing=true")
	}
	if got != fingerprintOutcomeIneligible {
		t.Errorf("expected ineligible, got %v", got)
	}
}

func TestFingerprintEligibility_IneligibleWhenEmptyPath(t *testing.T) {
	f := makeBookFile(func(bf *database.BookFile) { bf.FilePath = "" })
	got, _, stop := fingerprintEligibility(f, false)
	if !stop {
		t.Fatal("expected stop=true with empty file path")
	}
	if got != fingerprintOutcomeIneligible {
		t.Errorf("expected ineligible, got %v", got)
	}
}

func TestFingerprintEligibility_IneligibleWhenBadExtension(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pdf")
	if err := os.WriteFile(path, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := makeBookFile(func(bf *database.BookFile) { bf.FilePath = path })
	got, _, stop := fingerprintEligibility(f, false)
	if !stop {
		t.Fatal("expected stop=true for non-audio extension")
	}
	if got != fingerprintOutcomeIneligible {
		t.Errorf("expected ineligible, got %v", got)
	}
}

func TestFingerprintEligibility_IneligibleWhenFileDoesNotExist(t *testing.T) {
	f := makeBookFile(func(bf *database.BookFile) {
		bf.FilePath = "/this/path/definitely/does/not/exist.m4b"
	})
	got, _, stop := fingerprintEligibility(f, false)
	if !stop {
		t.Fatal("expected stop=true when file missing on disk")
	}
	if got != fingerprintOutcomeIneligible {
		t.Errorf("expected ineligible, got %v", got)
	}
}

func TestFingerprintEligibility_ProceedsWhenAllChecksPass(t *testing.T) {
	tmp := t.TempDir()
	for _, ext := range []string{".m4b", ".mp3", ".flac", ".m4a", ".ogg", ".opus", ".aac", ".wav"} {
		ext := ext
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(tmp, "ok"+ext)
			if err := os.WriteFile(path, []byte("placeholder"), 0o644); err != nil {
				t.Fatal(err)
			}
			f := makeBookFile(func(bf *database.BookFile) { bf.FilePath = path })
			outcome, _, stop := fingerprintEligibility(f, false)
			if stop {
				t.Fatalf("expected stop=false for %s, got outcome=%v", ext, outcome)
			}
		})
	}
}

func TestAudioExtensions_KnownFormats(t *testing.T) {
	want := []string{".aac", ".aiff", ".alac", ".ape", ".flac", ".m4a", ".m4b", ".mp3", ".ogg", ".opus", ".wav", ".wma", ".wv"}
	for _, ext := range want {
		if !audioExtensions[ext] {
			t.Errorf("expected %q in audioExtensions", ext)
		}
	}
}

func TestAudioExtensions_RejectsNonAudio(t *testing.T) {
	for _, ext := range []string{".txt", ".pdf", ".jpg", ".mkv", ".mp4", ".avi", ".epub"} {
		if audioExtensions[ext] {
			t.Errorf("did not expect %q in audioExtensions", ext)
		}
	}
}
