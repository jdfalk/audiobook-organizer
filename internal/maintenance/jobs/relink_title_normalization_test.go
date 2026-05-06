// file: internal/maintenance/jobs/relink_title_normalization_test.go
// version: 1.0.0
// guid: 7f3a1b2c-4d5e-6f7a-8b9c-0d1e2f3a4b5c
// last-edited: 2026-05-05

package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeForFilename(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Mistborn: The Final Empire", "mistborn_ the final empire"},
		{"Mistborn:The Final Empire", "mistborn_the final empire"},
		{"The Name of the Wind", "the name of the wind"},
		{"SHOGUN", "shogun"},
		{"  Leading Space  ", "leading space"},
		{"A: B: C", "a_ b_ c"},
	}
	for _, tc := range cases {
		got := normalizeForFilename(tc.input)
		if got != tc.want {
			t.Errorf("normalizeForFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFindInITunes_ColonTitleMatchesUnderscoreFilename(t *testing.T) {
	// Build fake iTunes root:
	//   <root>/Brandon Sanderson/Mistborn_ The Final Empire.m4b
	iTunesRoot := t.TempDir()
	authorDir := filepath.Join(iTunesRoot, "Brandon Sanderson")
	if err := os.MkdirAll(authorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fname := "Mistborn_ The Final Empire.m4b"
	if err := os.WriteFile(filepath.Join(authorDir, fname), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}

	// The book's title has a colon; the file has an underscore.
	results := rmt_findInITunes(iTunesRoot, "Brandon Sanderson", "Mistborn: The Final Empire", audioExts)

	if len(results) == 0 {
		t.Fatalf("expected a match for colon-title vs underscore-filename, got none")
	}
	if !strings.Contains(results[0], "Mistborn_") && !strings.Contains(results[0], "Brandon Sanderson") {
		t.Errorf("unexpected match path: %v", results)
	}
}
