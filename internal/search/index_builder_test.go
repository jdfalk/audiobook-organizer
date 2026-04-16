// file: internal/search/index_builder_test.go
// version: 1.0.0
// guid: 9d8e2c1a-5b4f-4f70-a7c6-2d8e0f1b9a47

package search

import (
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestBookToDoc_ResolvesRelations(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	author, _ := store.CreateAuthor("Brandon Sanderson")
	series, _ := store.CreateSeries("Stormlight Archive", &author.ID)
	seq := 1
	year := 2010
	lang := "en"
	book := &database.Book{
		ID: "b1", Title: "The Way of Kings",
		AuthorID: &author.ID, SeriesID: &series.ID,
		SeriesSequence:       &seq,
		Format:               "m4b",
		Language:             &lang,
		AudiobookReleaseYear: &year,
	}
	created, err := store.CreateBook(book)
	if err != nil {
		t.Fatalf("create book: %v", err)
	}
	// Tag wiring is verified separately; PebbleStore tag methods have
	// their own semantics. Here we only verify the resolver doesn't
	// choke when tags are absent.

	doc := BookToDoc(store, created)

	if doc.BookID != "b1" {
		t.Errorf("BookID = %q", doc.BookID)
	}
	if doc.Title != "The Way of Kings" {
		t.Errorf("Title = %q", doc.Title)
	}
	if doc.Author != "Brandon Sanderson" {
		t.Errorf("Author = %q, want resolved from AuthorID", doc.Author)
	}
	if doc.Series != "Stormlight Archive" {
		t.Errorf("Series = %q, want resolved from SeriesID", doc.Series)
	}
	if doc.SeriesNumber != 1 {
		t.Errorf("SeriesNumber = %d, want 1", doc.SeriesNumber)
	}
	if doc.Year != 2010 {
		t.Errorf("Year = %d, want 2010", doc.Year)
	}
	if doc.Format != "m4b" {
		t.Errorf("Format = %q", doc.Format)
	}
	if doc.Language != "en" {
		t.Errorf("Language = %q", doc.Language)
	}
	if doc.Type != BookDocType {
		t.Errorf("Type = %q", doc.Type)
	}
}

func TestBookToDoc_MissingRelationsSafelySkipped(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	// Book with nil AuthorID/SeriesID.
	book, _ := store.CreateBook(&database.Book{
		ID: "b2", Title: "Orphan Book", Format: "mp3",
	})
	doc := BookToDoc(store, book)
	if doc.Author != "" || doc.Series != "" {
		t.Errorf("expected empty Author/Series, got %q / %q", doc.Author, doc.Series)
	}
	if doc.Title != "Orphan Book" {
		t.Errorf("Title = %q", doc.Title)
	}
}
