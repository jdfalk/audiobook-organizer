// file: internal/server/metadata_normalize_test.go
// version: 1.0.0
// guid: 4f7c2b8d-3a91-4e5f-b6c0-1d8e7a9f3c45

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

func TestNormalizeMetaSeries_AlreadySplit(t *testing.T) {
	meta := metadata.BookMetadata{
		Title:          "The Final Empire",
		Series:         "Mistborn",
		SeriesPosition: "1",
	}
	metafetch.NormalizeMetaSeries(&meta)
	if meta.Series != "Mistborn" || meta.SeriesPosition != "1" || meta.Title != "The Final Empire" {
		t.Fatalf("clean fields should not be modified, got %+v", meta)
	}
}

func TestNormalizeMetaSeries_BookNumberInSeries(t *testing.T) {
	// The actual BMR-1 case: Audible returned the series field with the book
	// number baked in. The title is fine. Without normalization we'd create
	// a series named "Mistborn, Book 3".
	meta := metadata.BookMetadata{
		Title:  "The Hero of Ages",
		Series: "Mistborn, Book 3",
	}
	metafetch.NormalizeMetaSeries(&meta)
	if meta.Series != "Mistborn" {
		t.Errorf("Series = %q, want %q", meta.Series, "Mistborn")
	}
	if meta.SeriesPosition != "3" {
		t.Errorf("SeriesPosition = %q, want %q", meta.SeriesPosition, "3")
	}
	if meta.Title != "The Hero of Ages" {
		t.Errorf("Title = %q, want %q (Pattern 3 has no title — keep original)", meta.Title, "The Hero of Ages")
	}
}

func TestNormalizeMetaSeries_BookNumberInTitle(t *testing.T) {
	// Pattern 1 in title: extract series + position + new title.
	meta := metadata.BookMetadata{
		Title: "(Long Earth 5) The Long Cosmos",
	}
	metafetch.NormalizeMetaSeries(&meta)
	if meta.Series != "Long Earth" {
		t.Errorf("Series = %q, want %q", meta.Series, "Long Earth")
	}
	if meta.SeriesPosition != "5" {
		t.Errorf("SeriesPosition = %q, want %q", meta.SeriesPosition, "5")
	}
	if meta.Title != "The Long Cosmos" {
		t.Errorf("Title = %q, want %q", meta.Title, "The Long Cosmos")
	}
}

func TestNormalizeMetaSeries_NoMatch(t *testing.T) {
	meta := metadata.BookMetadata{
		Title:  "Some Standalone Book",
		Series: "",
	}
	metafetch.NormalizeMetaSeries(&meta)
	if meta.Title != "Some Standalone Book" || meta.Series != "" {
		t.Fatalf("non-matching input should be untouched, got %+v", meta)
	}
}

func TestNormalizeMetaSeries_PreservesExistingPositionWhenSeriesUnchanged(t *testing.T) {
	// If the series field doesn't match a pattern AND is non-empty, we keep
	// whatever the upstream gave us — do not clobber SeriesPosition.
	meta := metadata.BookMetadata{
		Title:          "Wind and Truth",
		Series:         "The Stormlight Archive",
		SeriesPosition: "5",
	}
	metafetch.NormalizeMetaSeries(&meta)
	if meta.SeriesPosition != "5" {
		t.Errorf("SeriesPosition lost, got %q", meta.SeriesPosition)
	}
}

func TestNormalizeMetaSeries_Idempotent(t *testing.T) {
	meta := metadata.BookMetadata{
		Title:  "The Hero of Ages",
		Series: "Mistborn, Book 3",
	}
	metafetch.NormalizeMetaSeries(&meta)
	first := meta
	metafetch.NormalizeMetaSeries(&meta)
	if meta != first {
		t.Errorf("second call mutated meta; before=%+v after=%+v", first, meta)
	}
}
