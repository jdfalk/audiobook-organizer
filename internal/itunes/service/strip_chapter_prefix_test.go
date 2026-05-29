// file: internal/itunes/service/strip_chapter_prefix_test.go
// version: 1.0.0
// guid: 5e8a3b9c-2d4f-4a1b-8c6d-9e0f1a2b3c4d

package itunesservice

import "testing"

func TestStripChapterPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"paren_slash", "(76/85) Tarkin: Star Wars (Unabridged)", "Tarkin: Star Wars (Unabridged)"},
		{"paren_of", "(76 of 85) Tarkin: Star Wars", "Tarkin: Star Wars"},
		{"paren_dash", "(1-12) Foundation", "Foundation"},
		{"paren_underscore", "(03_85) Tarkin", "Tarkin"},
		{"paren_leading_one", "(1/85) Tarkin: Star Wars", "Tarkin: Star Wars"},
		{"chapter_dash", "Chapter 03 - The Storm", "The Storm"},
		{"chapter_colon", "Chapter 03: The Storm", "The Storm"},
		{"chapter_space", "Chapter 12 The Storm", "The Storm"},
		{"chapter_underscore", "Chapter_05 - Foo", "Foo"},
		{"track_dash", "Track 12 - Foo", "Foo"},
		{"part_dash", "Part 4 - Bar", "Bar"},
		{"part_of", "Part 1 of 8 - Bar", "Bar"},
		{"bare_dash", "03 - Foo", "Foo"},
		{"bare_dot", "002. Foo", "Foo"},
		{"bare_colon", "1: Foo", "Foo"},
		{"no_prefix", "The Hobbit", "The Hobbit"},
		{"no_prefix_paren", "Tarkin (Unabridged)", "Tarkin (Unabridged)"},
		{"empty", "", ""},
		{"whitespace_only", "   ", ""},
		{"idempotent", "Tarkin: Star Wars (Unabridged)", "Tarkin: Star Wars (Unabridged)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stripChapterPrefix(c.in)
			if got != c.want {
				t.Errorf("stripChapterPrefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestStripChapterPrefix_Idempotent(t *testing.T) {
	// Applying twice should yield the same result as applying once.
	inputs := []string{
		"(76/85) Tarkin: Star Wars (Unabridged)",
		"Chapter 03 - The Storm",
		"03 - Foo",
		"The Hobbit",
	}
	for _, in := range inputs {
		once := stripChapterPrefix(in)
		twice := stripChapterPrefix(once)
		if once != twice {
			t.Errorf("not idempotent: %q → %q → %q", in, once, twice)
		}
	}
}
