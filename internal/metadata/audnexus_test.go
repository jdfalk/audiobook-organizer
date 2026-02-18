// file: internal/metadata/audnexus_test.go
// version: 1.0.0
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

func TestAudnexusClient_SearchByTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{
			"asin": "B003JVHRU0",
			"title": "The Hobbit",
			"authors": ["J.R.R. Tolkien"],
			"narrators": ["Martin Freeman"],
			"publisherName": "HarperAudio",
			"releaseDate": "2012-09-18",
			"language": "English",
			"image": "http://example.com/hobbit.jpg",
			"summary": "A hobbit goes on an adventure"
		}]`))
	}))
	defer server.Close()

	client := NewAudnexusClientWithBaseURL(server.URL)
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
		t.Errorf("expected author, got %q", r.Author)
	}
	if r.Publisher != "HarperAudio" {
		t.Errorf("expected publisher 'HarperAudio', got %q", r.Publisher)
	}
	if r.PublishYear != 2012 {
		t.Errorf("expected year 2012, got %d", r.PublishYear)
	}
}

func TestAudnexusClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewAudnexusClientWithBaseURL(server.URL)
	_, err := client.SearchByTitle("test")
	if err == nil {
		t.Error("expected error on 503 response")
	}
}

func TestAudnexusClient_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewAudnexusClientWithBaseURL(server.URL)
	results, err := client.SearchByTitleAndAuthor("Unknown", "Nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// Verify interface compliance
var _ MetadataSource = (*AudnexusClient)(nil)
