// file: internal/metadata/audnexus.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-a3b4c5d6e7f8

package metadata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// AudnexusClient fetches audiobook metadata from the Audnexus community API,
// which provides Audible-sourced data including narrator information.
// No authentication required.
type AudnexusClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewAudnexusClient creates a new Audnexus API client.
func NewAudnexusClient() *AudnexusClient {
	baseURL := os.Getenv("AUDNEXUS_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.audnex.us"
	}
	return &AudnexusClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// NewAudnexusClientWithBaseURL creates a client with a custom base URL (for testing).
func NewAudnexusClientWithBaseURL(baseURL string) *AudnexusClient {
	return &AudnexusClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// Name returns the display name for this metadata source.
func (c *AudnexusClient) Name() string {
	return "Audnexus (Audible)"
}

type audnexusSearchResult struct {
	ASIN        string   `json:"asin"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Narrators   []string `json:"narrators"`
	Publisher   string   `json:"publisherName"`
	ReleaseDate string   `json:"releaseDate"`
	Language    string   `json:"language"`
	Image       string   `json:"image"`
	Description string   `json:"summary"`
}

// SearchByTitle searches Audnexus by title.
func (c *AudnexusClient) SearchByTitle(title string) ([]BookMetadata, error) {
	searchURL := fmt.Sprintf("%s/books?title=%s", c.baseURL, url.QueryEscape(title))
	return c.doSearch(searchURL)
}

// SearchByTitleAndAuthor searches Audnexus by title and author.
func (c *AudnexusClient) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	searchURL := fmt.Sprintf("%s/books?title=%s&author=%s", c.baseURL, url.QueryEscape(title), url.QueryEscape(author))
	return c.doSearch(searchURL)
}

func (c *AudnexusClient) doSearch(searchURL string) ([]BookMetadata, error) {
	resp, err := c.httpClient.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search Audnexus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Audnexus API returned status %d", resp.StatusCode)
	}

	var items []audnexusSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode Audnexus response: %w", err)
	}

	results := make([]BookMetadata, 0, len(items))
	for _, item := range items {
		meta := BookMetadata{
			Title:       item.Title,
			Publisher:   item.Publisher,
			Description: item.Description,
			Language:    item.Language,
			CoverURL:    item.Image,
		}
		if len(item.Authors) > 0 {
			meta.Author = strings.Join(item.Authors, ", ")
		}
		if len(item.ReleaseDate) >= 4 {
			fmt.Sscanf(item.ReleaseDate, "%d", &meta.PublishYear)
		}
		results = append(results, meta)
	}
	return results, nil
}
