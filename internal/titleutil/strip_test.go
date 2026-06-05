// file: internal/titleutil/strip_test.go
// version: 1.0.0
// guid: 8f3b2c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package titleutil_test

import (
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/titleutil"
)

func TestStripChapterPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Fraction form
		{"(76/85) Tarkin: Star Wars (Unabridged)", "Tarkin: Star Wars (Unabridged)"},
		{"(76 of 85) Tarkin: Star Wars", "Tarkin: Star Wars"},
		{"(1-2) Some Title", "Some Title"},
		{"(1_2) Some Title", "Some Title"},

		// Chapter prefix
		{"Chapter 03 - The Storm", "The Storm"},
		{"Chapter 03: The Storm", "The Storm"},
		{"chapter 3 The Storm", "The Storm"},
		{"CHAPTER 12 - Finale", "Finale"},

		// Track prefix
		{"Track 12 - Foo", "Foo"},
		{"track 1: Bar", "Bar"},

		// Part prefix
		{"Part 4 - Bar", "Bar"},
		{"Part 4 of 8 - Bar", "Bar"},
		{"PART 2: Intro", "Intro"},

		// Bare number with delimiter
		{"03 - Foo", "Foo"},
		{"002. Title Here", "Title Here"},
		{"1: Something", "Something"},

		// Clean titles — must be untouched
		{"The Hobbit", "The Hobbit"},
		{"Tarkin: Star Wars (Unabridged)", "Tarkin: Star Wars (Unabridged)"},
		{"A Tale of Two Cities", "A Tale of Two Cities"},

		// Edge cases
		{"", ""},
		{"   ", ""},
		{"  (1/2) Padded  ", "Padded"},
	}

	for _, tc := range cases {
		got := titleutil.StripChapterPrefix(tc.in)
		if got != tc.want {
			t.Errorf("StripChapterPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
