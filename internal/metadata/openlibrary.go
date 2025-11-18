// file: internal/metadata/openlibrary.go
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package metadata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenLibraryClient handles metadata fetching from Open Library API
type OpenLibraryClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewOpenLibraryClient creates a new Open Library API client
func NewOpenLibraryClient() *OpenLibraryClient {
	return &OpenLibraryClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://openlibrary.org",
	}
}

// SearchResult represents a book search result from Open Library
type SearchResult struct {
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	FirstPublishYear int      `json:"first_publish_year"`
	ISBN             []string `json:"isbn"`
	Publisher        []string `json:"publisher"`
	Language         []string `json:"language"`
	CoverI           int      `json:"cover_i"`
	EditionCount     int      `json:"edition_count"`
}

// SearchResponse represents the API response from Open Library search
type SearchResponse struct {
	NumFound int            `json:"numFound"`
	Start    int            `json:"start"`
	Docs     []SearchResult `json:"docs"`
}

// BookMetadata represents enriched book metadata
type BookMetadata struct {
	Title       string
	Author      string
	Description string
	Publisher   string
	PublishYear int
	ISBN        string
	CoverURL    string
	Language    string
}

// SearchByTitle searches for books by title
func (c *OpenLibraryClient) SearchByTitle(title string) ([]BookMetadata, error) {
	// Build search query
	query := url.QueryEscape(title)
	searchURL := fmt.Sprintf("%s/search.json?title=%s&limit=5", c.baseURL, query)

	// Make HTTP request
	resp, err := c.httpClient.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search Open Library: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open Library API returned status %d", resp.StatusCode)
	}

	// Parse response
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Convert to BookMetadata
	results := make([]BookMetadata, 0, len(searchResp.Docs))
	for _, doc := range searchResp.Docs {
		metadata := BookMetadata{
			Title:       doc.Title,
			PublishYear: doc.FirstPublishYear,
		}

		if len(doc.AuthorName) > 0 {
			metadata.Author = doc.AuthorName[0]
		}

		if len(doc.Publisher) > 0 {
			metadata.Publisher = doc.Publisher[0]
		}

		if len(doc.ISBN) > 0 {
			metadata.ISBN = doc.ISBN[0]
		}

		if len(doc.Language) > 0 {
			metadata.Language = doc.Language[0]
		}

		if doc.CoverI > 0 {
			metadata.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
		}

		results = append(results, metadata)
	}

	return results, nil
}

// SearchByTitleAndAuthor searches for books by title and author
func (c *OpenLibraryClient) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	// Build search query
	titleQuery := url.QueryEscape(title)
	authorQuery := url.QueryEscape(author)
	searchURL := fmt.Sprintf("%s/search.json?title=%s&author=%s&limit=5", c.baseURL, titleQuery, authorQuery)

	// Make HTTP request
	resp, err := c.httpClient.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search Open Library: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open Library API returned status %d", resp.StatusCode)
	}

	// Parse response
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Convert to BookMetadata
	results := make([]BookMetadata, 0, len(searchResp.Docs))
	for _, doc := range searchResp.Docs {
		metadata := BookMetadata{
			Title:       doc.Title,
			PublishYear: doc.FirstPublishYear,
		}

		if len(doc.AuthorName) > 0 {
			metadata.Author = strings.Join(doc.AuthorName, ", ")
		}

		if len(doc.Publisher) > 0 {
			metadata.Publisher = doc.Publisher[0]
		}

		if len(doc.ISBN) > 0 {
			metadata.ISBN = doc.ISBN[0]
		}

		if len(doc.Language) > 0 {
			metadata.Language = doc.Language[0]
		}

		if doc.CoverI > 0 {
			metadata.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
		}

		results = append(results, metadata)
	}

	return results, nil
}

// GetBookByISBN fetches book details by ISBN
func (c *OpenLibraryClient) GetBookByISBN(isbn string) (*BookMetadata, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/isbn/%s.json", c.baseURL, isbn)

	// Make HTTP request
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch book by ISBN: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("book not found with ISBN: %s", isbn)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open Library API returned status %d", resp.StatusCode)
	}

	// Parse response
	var book map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&book); err != nil {
		return nil, fmt.Errorf("failed to decode book response: %w", err)
	}

	// Extract metadata
	metadata := &BookMetadata{
		ISBN: isbn,
	}

	if title, ok := book["title"].(string); ok {
		metadata.Title = title
	}

	if publishDate, ok := book["publish_date"].(string); ok {
		// Try to extract year from publish date
		if len(publishDate) >= 4 {
			var year int
			fmt.Sscanf(publishDate, "%d", &year)
			metadata.PublishYear = year
		}
	}

	if publishers, ok := book["publishers"].([]interface{}); ok && len(publishers) > 0 {
		if pub, ok := publishers[0].(string); ok {
			metadata.Publisher = pub
		}
	}

	// Get cover URL if available
	if covers, ok := book["covers"].([]interface{}); ok && len(covers) > 0 {
		if coverID, ok := covers[0].(float64); ok {
			metadata.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", int(coverID))
		}
	}

	return metadata, nil
}
