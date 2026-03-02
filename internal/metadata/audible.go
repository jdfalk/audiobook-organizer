// file: internal/metadata/audible.go
// version: 1.0.0
// guid: a9b8c7d6-e5f4-3a2b-1c0d-9e8f7a6b5c4d

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
)

// AudibleClient fetches audiobook metadata from Audible's undocumented catalog API.
// No authentication required. Rate limit: ~1 req/sec recommended.
type AudibleClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewAudibleClient creates a new Audible API client.
func NewAudibleClient() *AudibleClient {
	baseURL := os.Getenv("AUDIBLE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.audible.com/1.0"
	}
	return &AudibleClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// NewAudibleClientWithBaseURL creates a client with a custom base URL (for testing).
func NewAudibleClientWithBaseURL(baseURL string) *AudibleClient {
	return &AudibleClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// Name returns the display name for this metadata source.
func (c *AudibleClient) Name() string {
	return "Audible"
}

// Audible API response types

type audibleCatalogResponse struct {
	Products    []audibleProduct `json:"products"`
	TotalResult int              `json:"total_results"`
}

type audibleProductResponse struct {
	Product audibleProduct `json:"product"`
}

type audibleProduct struct {
	ASIN                string                    `json:"asin"`
	Title               string                    `json:"title"`
	Subtitle            string                    `json:"subtitle"`
	Authors             []audiblePerson           `json:"authors"`
	Narrators           []audiblePerson           `json:"narrators"`
	PublisherName       string                    `json:"publisher_name"`
	Language            string                    `json:"language"`
	IssueDate           string                    `json:"issue_date"`
	ReleaseDate         string                    `json:"release_date"`
	FormatType          string                    `json:"format_type"`
	MerchandisingSummary string                   `json:"merchandising_summary"`
	ProductImages       map[string]string         `json:"product_images"`
	Series              []audibleSeries           `json:"series"`
	ContentDeliveryType string                    `json:"content_delivery_type"`
	RuntimeLengthMin    int                       `json:"runtime_length_min"`
}

type audiblePerson struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
}

type audibleSeries struct {
	ASIN     string `json:"asin"`
	Title    string `json:"title"`
	Sequence string `json:"sequence"`
}

const audibleResponseGroups = "product_desc,contributors,media,product_attrs,series"

// SearchByTitle searches Audible's catalog by title.
func (c *AudibleClient) SearchByTitle(title string) ([]BookMetadata, error) {
	searchURL := fmt.Sprintf("%s/catalog/products?title=%s&num_results=10&products_sort_by=Relevance&response_groups=%s",
		c.baseURL, url.QueryEscape(title), audibleResponseGroups)
	return c.searchCatalog(searchURL)
}

// SearchByTitleAndAuthor searches Audible's catalog by title and author.
func (c *AudibleClient) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	searchURL := fmt.Sprintf("%s/catalog/products?title=%s&author=%s&num_results=10&products_sort_by=Relevance&response_groups=%s",
		c.baseURL, url.QueryEscape(title), url.QueryEscape(author), audibleResponseGroups)
	return c.searchCatalog(searchURL)
}

// LookupByASIN fetches a product directly by ASIN.
func (c *AudibleClient) LookupByASIN(asin string) (*BookMetadata, error) {
	productURL := fmt.Sprintf("%s/catalog/products/%s?response_groups=%s",
		c.baseURL, url.PathEscape(asin), audibleResponseGroups)

	req, err := http.NewRequest("GET", productURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Audible request: %w", err)
	}
	req.Header.Set("User-Agent", "Audible/3.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Audible API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Audible API returned status %d for ASIN %s", resp.StatusCode, asin)
	}

	var result audibleProductResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode Audible response: %w", err)
	}

	if result.Product.Title == "" {
		return nil, fmt.Errorf("Audible returned empty product for ASIN %s", asin)
	}

	meta := c.productToMetadata(&result.Product)
	return &meta, nil
}

func (c *AudibleClient) searchCatalog(searchURL string) ([]BookMetadata, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Audible request: %w", err)
	}
	req.Header.Set("User-Agent", "Audible/3.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Audible API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Audible API returned status %d", resp.StatusCode)
	}

	var catalog audibleCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("failed to decode Audible response: %w", err)
	}

	log.Printf("[DEBUG] Audible API returned %d products", len(catalog.Products))

	results := make([]BookMetadata, 0, len(catalog.Products))
	for _, p := range catalog.Products {
		results = append(results, c.productToMetadata(&p))
	}
	return results, nil
}

func (c *AudibleClient) productToMetadata(p *audibleProduct) BookMetadata {
	meta := BookMetadata{
		Title:     p.Title,
		Publisher: p.PublisherName,
		Language:  p.Language,
		ASIN:      p.ASIN,
	}

	// Strip HTML from merchandising summary
	if p.MerchandisingSummary != "" {
		desc := p.MerchandisingSummary
		// Simple HTML tag stripping
		for strings.Contains(desc, "<") {
			start := strings.Index(desc, "<")
			end := strings.Index(desc, ">")
			if end > start {
				desc = desc[:start] + desc[end+1:]
			} else {
				break
			}
		}
		meta.Description = strings.TrimSpace(desc)
	}

	// Authors
	authorNames := make([]string, 0, len(p.Authors))
	for _, a := range p.Authors {
		authorNames = append(authorNames, a.Name)
	}
	if len(authorNames) > 0 {
		meta.Author = strings.Join(authorNames, ", ")
	}

	// Narrators
	narratorNames := make([]string, 0, len(p.Narrators))
	for _, n := range p.Narrators {
		narratorNames = append(narratorNames, n.Name)
	}
	if len(narratorNames) > 0 {
		meta.Narrator = strings.Join(narratorNames, ", ")
	}

	// Year from issue_date or release_date
	dateStr := p.IssueDate
	if dateStr == "" {
		dateStr = p.ReleaseDate
	}
	if len(dateStr) >= 4 {
		fmt.Sscanf(dateStr[:4], "%d", &meta.PublishYear)
	}

	// Cover image — prefer largest available
	for _, size := range []string{"500", "252", "128"} {
		if imgURL, ok := p.ProductImages[size]; ok && imgURL != "" {
			meta.CoverURL = imgURL
			break
		}
	}

	// Series
	if len(p.Series) > 0 {
		meta.Series = p.Series[0].Title
		meta.SeriesPosition = p.Series[0].Sequence
	}

	return meta
}
