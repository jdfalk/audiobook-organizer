// file: internal/metadata/googlebooks_test.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-b4c5d6e7f8a9

package metadata

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleBooksClient_Name(t *testing.T) {
	c := NewGoogleBooksClient()
	if c.Name() != "Google Books" {
		t.Errorf("expected 'Google Books', got %q", c.Name())
	}
}

func TestGoogleBooksClient_SearchByTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/volumes" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
			"totalItems": 1,
			"items": [{
				"volumeInfo": {
					"title": "The Hobbit",
					"authors": ["J.R.R. Tolkien"],
					"publisher": "HarperCollins",
					"publishedDate": "1937-09-21",
					"language": "en",
					"industryIdentifiers": [
						{"type": "ISBN_13", "identifier": "9780261103344"},
						{"type": "ISBN_10", "identifier": "0261103342"}
					],
					"imageLinks": {"thumbnail": "http://example.com/cover.jpg"}
				}
			}]
		}`))
	}))
	defer server.Close()

	client := NewGoogleBooksClientWithBaseURL(server.URL)
	results, err := client.SearchByTitle("The Hobbit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Title != "The Hobbit" {
		t.Errorf("expected title 'The Hobbit', got %q", r.Title)
	}
	if r.Author != "J.R.R. Tolkien" {
		t.Errorf("expected author 'J.R.R. Tolkien', got %q", r.Author)
	}
	if r.ISBN != "9780261103344" {
		t.Errorf("expected ISBN13, got %q", r.ISBN)
	}
	if r.Publisher != "HarperCollins" {
		t.Errorf("expected publisher 'HarperCollins', got %q", r.Publisher)
	}
	if r.PublishYear != 1937 {
		t.Errorf("expected year 1937, got %d", r.PublishYear)
	}
	if r.CoverURL != "http://example.com/cover.jpg" {
		t.Errorf("expected cover URL, got %q", r.CoverURL)
	}
}

func TestGoogleBooksClient_SearchByTitleAndAuthor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"totalItems": 0, "items": []}`))
	}))
	defer server.Close()

	client := NewGoogleBooksClientWithBaseURL(server.URL)
	results, err := client.SearchByTitleAndAuthor("Unknown", "Nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestGoogleBooksClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewGoogleBooksClientWithBaseURL(server.URL)
	_, err := client.SearchByTitle("test")
	if err == nil {
		t.Error("expected error on 500 response")
	}
}

// Verify interface compliance
var _ MetadataSource = (*GoogleBooksClient)(nil)
