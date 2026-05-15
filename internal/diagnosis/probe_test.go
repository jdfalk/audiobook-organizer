// file: internal/diagnosis/probe_test.go
// version: 1.0.0
// guid: e2f3a4b5-c6d7-8e9f-0a1b-2c3d4e5f6a7b
// last-edited: 2026-05-15

package diagnosis

import (
	"context"
	"testing"
)

// writeScript removed — helper unused; removed to satisfy staticcheck U1000

func TestClassify_EmptyFile(t *testing.T) {
	d := FileDiagnostic{IsEmpty: true}
	reason, detail := Classify(d, "")
	if reason != ReasonEmptyFile {
		t.Errorf("got reason %q, want %q", reason, ReasonEmptyFile)
	}
	if detail == "" {
		t.Error("expected non-empty detail")
	}
}

func TestClassify_Truncated(t *testing.T) {
	d := FileDiagnostic{IsTruncated: true, FFProbeErrorStr: "moov atom not found"}
	reason, _ := Classify(d, "")
	if reason != ReasonIncompleteDownload {
		t.Errorf("got %q, want %q", reason, ReasonIncompleteDownload)
	}
}

func TestClassify_ActiveDRM(t *testing.T) {
	d := FileDiagnostic{HasActiveDRM: true, Encryption: "Encrypted"}
	reason, _ := Classify(d, "")
	if reason != ReasonActiveDRM {
		t.Errorf("got %q, want %q", reason, ReasonActiveDRM)
	}
}

func TestClassify_OriginallyDRM(t *testing.T) {
	d := FileDiagnostic{WasOriginallyDRM: true, EncodedApplication: "inAudible 1.94"}
	reason, detail := Classify(d, "")
	if reason != ReasonOriginallyDRM {
		t.Errorf("got %q, want %q", reason, ReasonOriginallyDRM)
	}
	if !contains(detail, "inAudible") {
		t.Errorf("detail %q should mention encoding app", detail)
	}
}

func TestClassify_WrongFormat(t *testing.T) {
	d := FileDiagnostic{FileMagic: "HTML document, ASCII text"}
	reason, _ := Classify(d, "")
	if reason != ReasonWrongFormat {
		t.Errorf("got %q, want %q", reason, ReasonWrongFormat)
	}
}

func TestClassify_TooShort(t *testing.T) {
	d := FileDiagnostic{DurationSec: 0.3}
	reason, _ := Classify(d, "")
	if reason != ReasonTooShort {
		t.Errorf("got %q, want %q", reason, ReasonTooShort)
	}
}

func TestClassify_UnsupportedCodec(t *testing.T) {
	d := FileDiagnostic{FFProbeErrorStr: "Decoder opus not found"}
	reason, _ := Classify(d, "")
	if reason != ReasonUnsupportedCodec {
		t.Errorf("got %q, want %q", reason, ReasonUnsupportedCodec)
	}
}

func TestClassify_CorruptAudio(t *testing.T) {
	d := FileDiagnostic{FFProbeErrorStr: "Invalid data found when processing input"}
	reason, _ := Classify(d, "")
	if reason != ReasonCorruptAudio {
		t.Errorf("got %q, want %q", reason, ReasonCorruptAudio)
	}
}

func TestClassify_FpcalcError_Fallback(t *testing.T) {
	d := FileDiagnostic{}
	reason, detail := Classify(d, "fpcalc: some unknown error")
	if reason != ReasonFpcalcError {
		t.Errorf("got %q, want %q", reason, ReasonFpcalcError)
	}
	if !contains(detail, "fpcalc") {
		t.Errorf("detail should reference fpcalc stderr, got: %q", detail)
	}
}

func TestDeriveFlags_Truncated(t *testing.T) {
	d := &FileDiagnostic{FFProbeErrorStr: "moov atom not found"}
	deriveFlags(d)
	if !d.IsTruncated {
		t.Error("expected IsTruncated=true for moov-not-found error")
	}
}

func TestDeriveFlags_ActiveDRM_ChannelError(t *testing.T) {
	d := &FileDiagnostic{FFProbeErrorStr: "channel element 2.1 is not allocated"}
	deriveFlags(d)
	if !d.HasActiveDRM {
		t.Error("expected HasActiveDRM=true for channel element error")
	}
}

func TestDeriveFlags_OriginallyDRM_InAudible(t *testing.T) {
	d := &FileDiagnostic{EncodedApplication: "inAudible 1.94"}
	deriveFlags(d)
	if !d.WasOriginallyDRM {
		t.Error("expected WasOriginallyDRM=true for inAudible encoder")
	}
}

func TestLooksLikeAudio(t *testing.T) {
	cases := []struct {
		magic string
		want  bool
	}{
		{"Audio file with ID3 version 2.3.0, contains: MPEG ADTS, layer III", true},
		{"ISO Media, Apple iTunes ALAC/AAC-LC (.M4A) Audio", true},
		{"RIFF (little-endian) data, WAVE audio", true},
		{"HTML document, ASCII text", false},
		{"Zip archive data", false},
		{"empty", false},
	}
	for _, c := range cases {
		if got := looksLikeAudio(c.magic); got != c.want {
			t.Errorf("looksLikeAudio(%q) = %v, want %v", c.magic, got, c.want)
		}
	}
}

func TestToJSON_RoundTrip(t *testing.T) {
	d := FileDiagnostic{
		Codec:       "aac",
		DurationSec: 3661.5,
		IsTruncated: true,
		ToolsUsed:   []string{"ffprobe"},
	}
	s := ToJSON(d)
	if s == "" {
		t.Fatal("expected non-empty JSON")
	}
	if !contains(s, "aac") || !contains(s, "3661.5") {
		t.Errorf("JSON missing expected fields: %s", s)
	}
}

func TestTruncate(t *testing.T) {
	s := truncate("abcdefgh", 4)
	if s != "abcd" {
		t.Errorf("got %q, want %q", s, "abcd")
	}
	if truncate("ab", 10) != "ab" {
		t.Error("should not truncate short string")
	}
}

// TestProbeFile_NonexistentPath verifies ProbeFile doesn't panic on a missing path.
func TestProbeFile_NonexistentPath(t *testing.T) {
	ctx := context.Background()
	d := ProbeFile(ctx, "/tmp/does-not-exist-audiobook-test-12345.m4b")
	// We only care that it doesn't panic; results depend on installed tools.
	_ = d
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
