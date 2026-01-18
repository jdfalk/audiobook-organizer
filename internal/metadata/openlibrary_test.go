// file: internal/metadata/openlibrary_test.go
// version: 1.1.0
// guid: 2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e

package metadata

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewOpenLibraryClient(t *testing.T) {
	t.Setenv("OPENLIBRARY_BASE_URL", "")
	client := NewOpenLibraryClient()
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.baseURL != "https://openlibrary.org" {
		t.Errorf("Expected baseURL to be https://openlibrary.org, got %s", client.baseURL)
	}
	if client.httpClient == nil {
		t.Fatal("Expected non-nil HTTP client")
	}
}

func TestNewOpenLibraryClientUsesEnvBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("OPENLIBRARY_BASE_URL", server.URL)

	client := NewOpenLibraryClient()
	if client.baseURL != server.URL {
		t.Errorf("Expected baseURL to use env %s, got %s", server.URL, client.baseURL)
	}
}

func TestSearchByTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"numFound":1,"start":0,"docs":[{"title":"The Hobbit","author_name":["J.R.R. Tolkien"],"first_publish_year":1937}]}`))
	}))
	defer server.Close()

	client := NewOpenLibraryClientWithBaseURL(server.URL)

	results, err := client.SearchByTitle("The Hobbit")
	if err != nil {
		t.Fatalf("SearchByTitle failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result for 'The Hobbit'")
	}

	if len(results) > 0 {
		firstResult := results[0]
		if firstResult.Title == "" {
			t.Error("Expected non-empty title in result")
		}
	}
}

func TestSearchByTitleAndAuthor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"numFound":1,"start":0,"docs":[{"title":"The Hobbit","author_name":["J.R.R. Tolkien"],"first_publish_year":1937,"publisher":["Allen & Unwin"]}]}`))
	}))
	defer server.Close()

	client := NewOpenLibraryClientWithBaseURL(server.URL)

	results, err := client.SearchByTitleAndAuthor("The Hobbit", "Tolkien")
	if err != nil {
		t.Fatalf("SearchByTitleAndAuthor failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result for 'The Hobbit' by Tolkien")
	}

	if len(results) > 0 {
		found := false
		for _, result := range results {
			if result.Author != "" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected at least one result with author information")
		}
	}
}

func TestSearchByTitleNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"numFound":0,"start":0,"docs":[]}`))
	}))
	defer server.Close()

	client := NewOpenLibraryClientWithBaseURL(server.URL)

	results, err := client.SearchByTitle("xyzabc123456789nonexistent")
	if err != nil {
		t.Fatalf("SearchByTitle failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected no results, got %d", len(results))
	}
}

func TestGetBookByISBN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/isbn/9780547928227.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"title":"The Hobbit","publish_date":"1937","publishers":["Allen & Unwin"],"covers":[123]}`))
	}))
	defer server.Close()

	client := NewOpenLibraryClientWithBaseURL(server.URL)

	result, err := client.GetBookByISBN("9780547928227")
	if err != nil {
		t.Fatalf("GetBookByISBN failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result for valid ISBN")
	}

	if result.ISBN != "9780547928227" {
		t.Errorf("Expected ISBN 9780547928227, got %s", result.ISBN)
	}
	if result.Title != "The Hobbit" {
		t.Errorf("Expected title The Hobbit, got %s", result.Title)
	}
	if result.Publisher != "Allen & Unwin" {
		t.Errorf("Expected publisher Allen & Unwin, got %s", result.Publisher)
	}
	if result.PublishYear != 1937 {
		t.Errorf("Expected publish year 1937, got %d", result.PublishYear)
	}
	if result.CoverURL == "" {
		t.Error("Expected cover URL to be set")
	}
}

func TestGetBookByISBNInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewOpenLibraryClientWithBaseURL(server.URL)

	_, err := client.GetBookByISBN("0000000000")
	if err == nil {
		t.Error("Expected error for invalid ISBN")
	}
}
