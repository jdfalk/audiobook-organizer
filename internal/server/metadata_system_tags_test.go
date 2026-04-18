// file: internal/server/metadata_system_tags_test.go
// version: 1.0.0
// guid: 8e9f0a1b-2c3d-4e5f-6a7b-8c9d0e1f2a3b

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
)

// TestMetadataSourceTag locks in the slug format for the
// metadata:source:* namespace. Every metadata source's Name()
// string gets passed through this helper, and the resulting tag
// is what downstream filters (review dialog, upgrade jobs) key
// on — so drift here would silently break filters.
func TestMetadataSourceTag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"hardcover", "Hardcover", "metadata:source:hardcover"},
		{"open library", "Open Library", "metadata:source:open_library"},
		{"google books", "Google Books", "metadata:source:google_books"},
		{"audible", "Audible", "metadata:source:audible"},
		// Audnexus wraps Audible upstream — we strip the
		// "(Audible)" parenthetical so the tag cleanly names
		// the source provider rather than its upstream.
		{"audnexus", "Audnexus (Audible)", "metadata:source:audnexus"},
		{"audnexus_bare", "Audnexus", "metadata:source:audnexus"},
		{"wikipedia", "Wikipedia", "metadata:source:wikipedia"},
		{"hyphens", "Some-Source", "metadata:source:some_source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metafetch.MetadataSourceTag(tt.in)
			if got != tt.want {
				t.Errorf("metafetch.MetadataSourceTag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestMetadataLanguageTag locks in the canonicalization from
// the grab-bag of formats real sources return (ISO-639-1,
// ISO-639-2, English names) to the lowercase 2-letter code the
// review-dialog filter expects. Unknowns fall through to a
// slugified tag so we never drop data.
func TestMetadataLanguageTag(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"en", "metadata:language:en"},
		{"EN", "metadata:language:en"},
		{"English", "metadata:language:en"},
		{"eng", "metadata:language:en"},
		{"Spanish", "metadata:language:es"},
		{"spa", "metadata:language:es"},
		{"Mandarin", "metadata:language:zh"},
		{"zho", "metadata:language:zh"},
		{"Chinese", "metadata:language:zh"},
		{"de", "metadata:language:de"},
		{"German", "metadata:language:de"},
		{"Portuguese", "metadata:language:pt"},
		// Unknown: slugified fallthrough, never dropped.
		{"Klingon", "metadata:language:klingon"},
		{"Old English", "metadata:language:old_english"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := metafetch.MetadataLanguageTag(tt.in)
			if got != tt.want {
				t.Errorf("metafetch.MetadataLanguageTag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
