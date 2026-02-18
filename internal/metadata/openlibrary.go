// file: internal/metadata/openlibrary.go
// version: 1.3.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package metadata

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
)

// OpenLibraryClient handles metadata fetching from Open Library API.
// When olStore is set, local dump data is checked before hitting the API.
type OpenLibraryClient struct {
	httpClient *http.Client
	baseURL    string
	olStore    *openlibrary.OLStore
}

// NewOpenLibraryClient creates a new Open Library API client
func NewOpenLibraryClient() *OpenLibraryClient {
	baseURL := os.Getenv("OPENLIBRARY_BASE_URL")
	if baseURL == "" {
		baseURL = "https://openlibrary.org"
	}
	return NewOpenLibraryClientWithBaseURL(baseURL)
}

// NewOpenLibraryClientWithBaseURL creates a client with a custom base URL.
func NewOpenLibraryClientWithBaseURL(baseURL string) *OpenLibraryClient {
	return &OpenLibraryClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// Name returns the display name for this metadata source.
func (c *OpenLibraryClient) Name() string {
	return "Open Library"
}

// SetOLStore attaches a local Open Library dump store for local-first lookups.
func (c *OpenLibraryClient) SetOLStore(store *openlibrary.OLStore) {
	c.olStore = store
}

// editionToMetadata converts an OLEdition to BookMetadata.
func editionToMetadata(ed *openlibrary.OLEdition, store *openlibrary.OLStore) BookMetadata {
	meta := BookMetadata{
		Title: ed.Title,
	}
	if len(ed.ISBN13) > 0 {
		meta.ISBN = ed.ISBN13[0]
	} else if len(ed.ISBN10) > 0 {
		meta.ISBN = ed.ISBN10[0]
	}
	if len(ed.Publishers) > 0 {
		meta.Publisher = ed.Publishers[0]
	}
	if len(ed.Covers) > 0 {
		meta.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", ed.Covers[0])
	}
	if store != nil && len(ed.Authors) > 0 {
		author, err := store.LookupAuthor(ed.Authors[0].Key)
		if err == nil && author != nil {
			meta.Author = author.Name
		}
	}
	if ed.PublishDate != "" && len(ed.PublishDate) >= 4 {
		fmt.Sscanf(ed.PublishDate, "%d", &meta.PublishYear)
	}
	return meta
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

// SearchByTitle searches for books by title. Checks local dump store first if available.
func (c *OpenLibraryClient) SearchByTitle(title string) ([]BookMetadata, error) {
	if c.olStore != nil {
		editions, err := c.olStore.SearchByTitle(title)
		if err == nil && len(editions) > 0 {
			results := make([]BookMetadata, 0, len(editions))
			for i := range editions {
				results = append(results, editionToMetadata(&editions[i], c.olStore))
			}
			log.Printf("[DEBUG] SearchByTitle: found %d results from local dump for %q", len(results), title)
			return results, nil
		}
	}

	// Fall back to API
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

// GetBookByISBN fetches book details by ISBN. Checks local dump store first if available.
func (c *OpenLibraryClient) GetBookByISBN(isbn string) (*BookMetadata, error) {
	if c.olStore != nil {
		ed, err := c.olStore.LookupByISBN(isbn)
		if err == nil && ed != nil {
			meta := editionToMetadata(ed, c.olStore)
			log.Printf("[DEBUG] GetBookByISBN: found ISBN %s in local dump", isbn)
			return &meta, nil
		}
	}

	// Fall back to API
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
