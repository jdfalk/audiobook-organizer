// file: internal/metadata/audible_test.go
// version: 1.0.0
// guid: b8c7d6e5-f4a3-2b1c-0d9e-8f7a6b5c4d3e

package metadata

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAudibleClient_Name(t *testing.T) {
	c := NewAudibleClient()
	if c.Name() != "Audible" {
		t.Errorf("expected 'Audible', got %q", c.Name())
	}
}

func TestAudibleClient_SearchByTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog/products" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("title") == "" {
			t.Error("expected title param")
		}
		resp := audibleCatalogResponse{
			Products: []audibleProduct{
				{
					ASIN:  "B0CPKNRSL5",
					Title: "Rogue Ascension",
					Authors: []audiblePerson{
						{ASIN: "B0BHR8YPLS", Name: "Hunter Mythos"},
					},
					Narrators: []audiblePerson{
						{Name: "André Santana"},
					},
					IssueDate:     "2024-02-13",
					Language:      "english",
					PublisherName: "Podium Audio",
					ProductImages: map[string]string{"500": "https://example.com/cover.jpg"},
					Series: []audibleSeries{
						{ASIN: "B0CPLN9WDR", Title: "Rogue Ascension", Sequence: "1"},
					},
				},
			},
			TotalResult: 1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAudibleClientWithBaseURL(server.URL)
	results, err := client.SearchByTitle("Rogue Ascension")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Title != "Rogue Ascension" {
		t.Errorf("title: got %q", r.Title)
	}
	if r.Author != "Hunter Mythos" {
		t.Errorf("author: got %q", r.Author)
	}
	if r.Narrator != "André Santana" {
		t.Errorf("narrator: got %q", r.Narrator)
	}
	if r.ASIN != "B0CPKNRSL5" {
		t.Errorf("ASIN: got %q", r.ASIN)
	}
	if r.Series != "Rogue Ascension" {
		t.Errorf("series: got %q", r.Series)
	}
	if r.SeriesPosition != "1" {
		t.Errorf("series position: got %q", r.SeriesPosition)
	}
	if r.PublishYear != 2024 {
		t.Errorf("year: got %d", r.PublishYear)
	}
	if r.CoverURL != "https://example.com/cover.jpg" {
		t.Errorf("cover: got %q", r.CoverURL)
	}
}

func TestAudibleClient_LookupByASIN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog/products/B0CPKNRSL5" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := audibleProductResponse{
			Product: audibleProduct{
				ASIN:  "B0CPKNRSL5",
				Title: "Rogue Ascension",
				Authors: []audiblePerson{
					{Name: "Hunter Mythos"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAudibleClientWithBaseURL(server.URL)
	result, err := client.LookupByASIN("B0CPKNRSL5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "Rogue Ascension" {
		t.Errorf("title: got %q", result.Title)
	}
}

func TestAudibleClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewAudibleClientWithBaseURL(server.URL)
	_, err := client.SearchByTitle("test")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestAudibleClient_HTMLStripping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := audibleCatalogResponse{
			Products: []audibleProduct{
				{
					ASIN:                 "B123",
					Title:                "Test",
					MerchandisingSummary: "<p>Hello <b>world</b></p>",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAudibleClientWithBaseURL(server.URL)
	results, err := client.SearchByTitle("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Description != "Hello world" {
		t.Errorf("description: got %q, want 'Hello world'", results[0].Description)
	}
}

var _ MetadataSource = (*AudibleClient)(nil)
