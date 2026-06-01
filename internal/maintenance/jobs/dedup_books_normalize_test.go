// file: internal/maintenance/jobs/dedup_books_normalize_test.go
// version: 1.0.0
// guid: 7f3c9a12-b4d8-4e21-9c6f-0a1b2d3e4f58
// last-edited: 2026-06-01

package jobs

import "testing"

func TestDdNormalizeDedupTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
		desc string
	}{
		// Unambiguous chapter markers — should be stripped.
		{"001 - The Hobbit", "the hobbit", "number + space-dash-space"},
		{"003: Midnight Run", "midnight run", "number + colon-space"},
		{"01. Chapter Name", "chapter name", "number + dot-space"},
		{"(76/85) Tarkin: Star Wars", "tarkin star wars", "N/M fraction prefix"},
		{"(3 of 85) Tarkin", "3 of 85 tarkin", "N of M — not matched by regex (no parens)"},

		// Bare number + space only — must NOT be stripped; could be a real title.
		{"001 Tarkin", "001 tarkin", "bare number+space is NOT a chapter marker"},
		{"2001 A Space Odyssey", "2001 a space odyssey", "year-like number"},
		{"007 James Bond", "007 james bond", "brand-identifier number"},

		// Noise removed, case folded.
		{"The Hobbit (Unabridged)", "the hobbit", "unabridged suffix"},
		{"The Hobbit", "the hobbit", "plain title unchanged"},
		{"", "", "empty"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got := ddNormalizeDedupTitle(c.in)
			if got != c.want {
				t.Errorf("ddNormalizeDedupTitle(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}
