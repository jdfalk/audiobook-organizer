// file: internal/util/normalize_test.go
// version: 1.0.0
// guid: b4e8f3a2-0c5d-4f9b-a7e1-3d6c8b2e4f0a
// last-edited: 2026-05-02

package util_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/util"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/foo/Bar/BAZ.mp3", "/foo/bar/baz.mp3"},
		{"./Audio/Book/../file.MP3", "audio/file.mp3"},
		{"/Audiobooks/Author/Title/", "/audiobooks/author/title"},
		{"UPPER/lower/Mixed.go", "upper/lower/mixed.go"},
	}
	for _, c := range cases {
		if got := util.NormalizePath(c.in); got != c.want {
			t.Errorf("NormalizePath(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  The Hobbit  ", "the hobbit"},
		{"DUNE", "dune"},
		{"Foundation and Empire", "foundation and empire"},
		{"\tWhitespace\n", "whitespace"},
	}
	for _, c := range cases {
		if got := util.NormalizeTitle(c.in); got != c.want {
			t.Errorf("NormalizeTitle(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeAuthor(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  J.R.R. Tolkien  ", "j.r.r. tolkien"},
		{"TOLKIEN", "tolkien"},
		{"Isaac Asimov", "isaac asimov"},
		{"\tFrank Herbert\n", "frank herbert"},
	}
	for _, c := range cases {
		if got := util.NormalizeAuthor(c.in); got != c.want {
			t.Errorf("NormalizeAuthor(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  Hello World  ", "hello world"},
		{"SCIENCE-FICTION", "science-fiction"},
		{"Mystery", "mystery"},
		{"  ", ""},
	}
	for _, c := range cases {
		if got := util.NormalizeString(c.in); got != c.want {
			t.Errorf("NormalizeString(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestCollapseSpaces(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  hello   world  ", "hello world"},
		{"no  extra\t\tspaces", "no extra spaces"},
		{"single", "single"},
		{"   ", ""},
		{"a\nb\tc", "a b c"},
	}
	for _, c := range cases {
		if got := util.CollapseSpaces(c.in); got != c.want {
			t.Errorf("CollapseSpaces(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
