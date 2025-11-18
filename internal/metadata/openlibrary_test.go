// file: internal/metadata/openlibrary_test.go
// version: 1.0.0
// guid: 2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e

package metadata

import (
	"testing"
)

func TestNewOpenLibraryClient(t *testing.T) {
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

func TestSearchByTitle(t *testing.T) {
	client := NewOpenLibraryClient()
	
	// Test with a well-known book
	results, err := client.SearchByTitle("The Hobbit")
	if err != nil {
		t.Fatalf("SearchByTitle failed: %v", err)
	}
	
	if len(results) == 0 {
		t.Error("Expected at least one result for 'The Hobbit'")
	}
	
	// Verify first result has expected fields
	if len(results) > 0 {
		firstResult := results[0]
		if firstResult.Title == "" {
			t.Error("Expected non-empty title in result")
		}
	}
}

func TestSearchByTitleAndAuthor(t *testing.T) {
	client := NewOpenLibraryClient()
	
	// Test with specific book and author
	results, err := client.SearchByTitleAndAuthor("The Hobbit", "Tolkien")
	if err != nil {
		t.Fatalf("SearchByTitleAndAuthor failed: %v", err)
	}
	
	if len(results) == 0 {
		t.Error("Expected at least one result for 'The Hobbit' by Tolkien")
	}
	
	// Verify results contain author information
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
	client := NewOpenLibraryClient()
	
	// Test with unlikely title that should return no results
	results, err := client.SearchByTitle("xyzabc123456789nonexistent")
	if err != nil {
		t.Fatalf("SearchByTitle failed: %v", err)
	}
	
	// Empty results is valid for non-existent books
	if len(results) > 0 {
		t.Logf("Unexpectedly found %d results for nonsense title", len(results))
	}
}

func TestGetBookByISBN(t *testing.T) {
	client := NewOpenLibraryClient()
	
	// Test with a valid ISBN (The Hobbit)
	result, err := client.GetBookByISBN("9780547928227")
	if err != nil {
		t.Skipf("ISBN lookup failed (may be rate limited or API issue): %v", err)
		return
	}
	
	if result == nil {
		t.Fatal("Expected non-nil result for valid ISBN")
	}
	
	if result.ISBN != "9780547928227" {
		t.Errorf("Expected ISBN 9780547928227, got %s", result.ISBN)
	}
}

func TestGetBookByISBNInvalid(t *testing.T) {
	client := NewOpenLibraryClient()
	
	// Test with invalid ISBN
	_, err := client.GetBookByISBN("0000000000")
	if err == nil {
		t.Error("Expected error for invalid ISBN")
	}
}
