// file: internal/metadata/audnexus_test.go
// version: 2.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-c5d6e7f8a9b0

package metadata

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAudnexusClient_Name(t *testing.T) {
	c := NewAudnexusClient()
	if c.Name() != "Audnexus (Audible)" {
		t.Errorf("expected 'Audnexus (Audible)', got %q", c.Name())
	}
}

func TestAudnexusClient_SearchByTitle_ReturnsEmpty(t *testing.T) {
	// SearchByTitle should return nil (no endpoint exists)
	client := NewAudnexusClient()
	results, err := client.SearchByTitle("The Hobbit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (no title search endpoint), got %d", len(results))
	}
}

func TestAudnexusClient_LookupByASIN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books/B003JVHRU0" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
			"asin": "B003JVHRU0",
			"title": "The Hobbit",
			"authors": [{"asin": "B000AP6TLO", "name": "J.R.R. Tolkien"}],
			"narrators": [{"name": "Martin Freeman"}],
			"publisherName": "HarperAudio",
			"releaseDate": "2012-09-18T00:00:00.000Z",
			"language": "English",
			"image": "http://example.com/hobbit.jpg",
			"summary": "A hobbit goes on an adventure",
			"isbn": "9780007489943",
			"seriesPrimary": {"asin": "B00SVDQ2DO", "name": "The Lord of the Rings", "position": "0.5"}
		}`))
	}))
	defer server.Close()

	client := NewAudnexusClientWithBaseURL(server.URL)
	meta, err := client.LookupByASIN("B003JVHRU0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Title != "The Hobbit" {
		t.Errorf("expected title 'The Hobbit', got %q", meta.Title)
	}
	if meta.Author != "J.R.R. Tolkien" {
		t.Errorf("expected author, got %q", meta.Author)
	}
	if meta.Narrator != "Martin Freeman" {
		t.Errorf("expected narrator 'Martin Freeman', got %q", meta.Narrator)
	}
	if meta.Publisher != "HarperAudio" {
		t.Errorf("expected publisher 'HarperAudio', got %q", meta.Publisher)
	}
	if meta.PublishYear != 2012 {
		t.Errorf("expected year 2012, got %d", meta.PublishYear)
	}
	if meta.ISBN != "9780007489943" {
		t.Errorf("expected ISBN, got %q", meta.ISBN)
	}
	if meta.Series != "The Lord of the Rings" {
		t.Errorf("expected series, got %q", meta.Series)
	}
	if meta.SeriesPosition != "0.5" {
		t.Errorf("expected series position '0.5', got %q", meta.SeriesPosition)
	}
}

func TestAudnexusClient_SearchByTitleAndAuthor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authors" && r.URL.Query().Get("name") != "" {
			_, _ = w.Write([]byte(`[{
				"asin": "B000AP6TLO",
				"name": "J.R.R. Tolkien",
				"description": "English author"
			}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewAudnexusClientWithBaseURL(server.URL)
	// Returns nil because we can't enumerate an author's books
	results, err := client.SearchByTitleAndAuthor("The Hobbit", "Tolkien")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (can't enumerate books), got %d", len(results))
	}
}

func TestAudnexusClient_LookupByASIN_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewAudnexusClientWithBaseURL(server.URL)
	_, err := client.LookupByASIN("BADASIN")
	if err == nil {
		t.Error("expected error on 404 response")
	}
}

// Verify interface compliance
var _ MetadataSource = (*AudnexusClient)(nil)
