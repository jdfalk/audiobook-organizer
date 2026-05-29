// file: internal/organizer/path_format_test.go
// version: 1.0.0
// guid: a7b3c1d2-e4f5-6789-abcd-ef0123456f01

package organizer

import (
	"strings"
	"testing"
)

// TestFormatPath_SlashInVariableDoesNotCreateDirectory exercises the prod
// bug from 2026-05-28: book 01KQGDQTJ44FCAPW5Z9D2KNQDE had its 85-chapter
// audiobook split into 85 single-file books because the Title metadata
// contained a "/" which the path formatter passed through unescaped,
// turning into a real directory boundary on disk.
func TestFormatPath_SlashInVariableDoesNotCreateDirectory(t *testing.T) {
	cases := []struct {
		name string
		vars FormatVars
	}{
		{
			name: "title contains slash",
			vars: FormatVars{
				Author:      "James Luceno",
				Title:       "Tarkin - Star Wars - 3/85", // <-- the killer
				Ext:         "mp3",
				Track:       3,
				TotalTracks: 85,
			},
		},
		{
			name: "series contains slash",
			vars: FormatVars{
				Author: "Test Author",
				Series: "Foo/Bar Series",
				Title:  "Book One",
				Ext:    "mp3",
			},
		},
		{
			name: "author contains slash",
			vars: FormatVars{
				Author: "First / Last",
				Title:  "Title",
				Ext:    "mp3",
			},
		},
		{
			name: "narrator contains slash",
			vars: FormatVars{
				Author:   "Test Author",
				Title:    "Title",
				Narrator: "Reader A / Reader B",
				Ext:      "mp3",
			},
		},
		{
			name: "title begins with dot (hidden file)",
			vars: FormatVars{
				Author: "Test Author",
				Title:  ".hidden",
				Ext:    "mp3",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatPath(DefaultPathFormat, tc.vars)
			// Count "/" — should equal the count of "/" in the TEMPLATE itself,
			// not anything more. DefaultPathFormat has 2 path separators:
			// {author}/{series_prefix}{title}/{track_title}.{ext}
			//        ^                       ^
			// Any extra "/" means a variable leaked one.
			templateSlashes := strings.Count(DefaultPathFormat, "/")
			gotSlashes := strings.Count(got, "/")
			if gotSlashes != templateSlashes {
				t.Errorf("FormatPath(%+v) = %q\n  has %d '/' separators; template has %d.\n  A variable value leaked a path separator into the result.",
					tc.vars, got, gotSlashes, templateSlashes)
			}
		})
	}
}

// TestFormatPath_TarkinReproduces the exact prod path that would have been
// written by the buggy code, and confirms the scrubbed result lands in
// ONE directory, not 85.
func TestFormatPath_TarkinReproducesAsOneFile(t *testing.T) {
	vars := FormatVars{
		Author:      "James Luceno",
		Series:      "Star Wars",
		SeriesPos:   "24",
		Title:       "Tarkin",
		Track:       3,
		TotalTracks: 85,
		Ext:         "mp3",
	}
	got := FormatPath(DefaultPathFormat, vars)

	// Expected: James Luceno/Star Wars 24 - Tarkin/Tarkin - 3_85.mp3
	// Each segment is one path component; the "3_85" uses underscore (the
	// segment-title default), so no rogue directory.
	if strings.Contains(got, "/85.mp3") {
		t.Fatalf("FormatPath emitted %q — '/85.mp3' suffix means the per-track total leaked as a directory separator", got)
	}
	if strings.Count(got, "/") != 2 {
		t.Fatalf("FormatPath emitted %q — expected exactly 2 path separators (author/, title-folder/), got %d", got, strings.Count(got, "/"))
	}
}

func TestScrubVar(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"normal":            "normal",
		"with/slash":        "with slash",
		"back\\slash":       "back slash",
		"both/and\\both":    "both and both",
		".hidden":           "hidden",
		"..parent":          "parent",
		"....many":          "many",
		"trailing.":         "trailing.",
		"middle.dot.ok":     "middle.dot.ok",
	}
	for in, want := range cases {
		if got := scrubVar(in); got != want {
			t.Errorf("scrubVar(%q) = %q; want %q", in, got, want)
		}
	}
}
