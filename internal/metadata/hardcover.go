// file: internal/metadata/hardcover.go
// version: 1.0.0
// guid: e7e02554-8931-49ba-9528-d3d51279da1d

package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HardcoverClient fetches metadata from the Hardcover.app GraphQL API.
// Requires a Bearer token for authentication.
type HardcoverClient struct {
	httpClient *http.Client
	baseURL    string
	apiToken   string

	// Simple rate limiter: 60 requests per minute
	mu          sync.Mutex
	requestLog  []time.Time
	rateLimit   int
	ratePeriod  time.Duration
}

// NewHardcoverClient creates a new Hardcover API client with the given token.
func NewHardcoverClient(apiToken string) *HardcoverClient {
	return &HardcoverClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.hardcover.app/v1/graphql",
		apiToken:   apiToken,
		rateLimit:  60,
		ratePeriod: time.Minute,
	}
}

// NewHardcoverClientWithBaseURL creates a client with a custom base URL (for testing).
func NewHardcoverClientWithBaseURL(baseURL, apiToken string) *HardcoverClient {
	return &HardcoverClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiToken:   apiToken,
		rateLimit:  60,
		ratePeriod: time.Minute,
	}
}

// Name returns the display name for this metadata source.
func (c *HardcoverClient) Name() string {
	return "Hardcover"
}

// waitForRateLimit blocks until a request can be made within the rate limit.
func (c *HardcoverClient) waitForRateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-c.ratePeriod)

	// Remove expired entries
	valid := c.requestLog[:0]
	for _, t := range c.requestLog {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	c.requestLog = valid

	if len(c.requestLog) >= c.rateLimit {
		// Wait until the oldest request expires
		waitUntil := c.requestLog[0].Add(c.ratePeriod)
		sleepDuration := time.Until(waitUntil)
		if sleepDuration > 0 {
			c.mu.Unlock()
			time.Sleep(sleepDuration)
			c.mu.Lock()
			// Re-clean after sleep
			now = time.Now()
			cutoff = now.Add(-c.ratePeriod)
			valid = c.requestLog[:0]
			for _, t := range c.requestLog {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			c.requestLog = valid
		}
	}

	c.requestLog = append(c.requestLog, time.Now())
}

// GraphQL request/response types

type hardcoverGraphQLRequest struct {
	Query string `json:"query"`
}

type hardcoverGraphQLResponse struct {
	Data   *hardcoverData   `json:"data"`
	Errors []hardcoverError `json:"errors"`
}

type hardcoverError struct {
	Message string `json:"message"`
}

type hardcoverData struct {
	SearchBooks *hardcoverSearchBooks `json:"search_books"`
}

type hardcoverSearchBooks struct {
	Results *hardcoverResults `json:"results"`
}

type hardcoverResults struct {
	Hits []hardcoverHit `json:"hits"`
}

type hardcoverHit struct {
	Document hardcoverDocument `json:"document"`
}

type hardcoverDocument struct {
	Title       string          `json:"title"`
	AuthorNames []string        `json:"author_names"`
	Image       *hardcoverImage `json:"image"`
	Description string          `json:"description"`
	ReleaseYear int             `json:"release_year"`
	Slug        string          `json:"slug"`
	Publisher   string          `json:"publisher"`
}

type hardcoverImage struct {
	URL string `json:"url"`
}

// SearchByTitle searches Hardcover by title.
func (c *HardcoverClient) SearchByTitle(title string) ([]BookMetadata, error) {
	if c.apiToken == "" {
		log.Printf("[DEBUG] Hardcover: no API token configured, skipping")
		return nil, nil
	}
	return c.search(title)
}

// SearchByTitleAndAuthor searches Hardcover by title (author is used for ranking but
// the GraphQL search_books endpoint only accepts a single query string).
func (c *HardcoverClient) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	if c.apiToken == "" {
		log.Printf("[DEBUG] Hardcover: no API token configured, skipping")
		return nil, nil
	}
	query := title + " " + author
	return c.search(query)
}

func (c *HardcoverClient) search(query string) ([]BookMetadata, error) {
	c.waitForRateLimit()

	// Escape the query for embedding in the GraphQL string
	escapedQuery := strings.ReplaceAll(query, `\`, `\\`)
	escapedQuery = strings.ReplaceAll(escapedQuery, `"`, `\"`)

	graphqlQuery := fmt.Sprintf(`query { search_books(query: "%s", limit: 5) { results { hits { document { title author_names image { url } description release_year slug publisher } } } } }`, escapedQuery)

	reqBody := hardcoverGraphQLRequest{Query: graphqlQuery}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Hardcover request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create Hardcover request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Hardcover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hardcover API returned status %d", resp.StatusCode)
	}

	var gqlResp hardcoverGraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("failed to decode Hardcover response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("Hardcover GraphQL error: %s", gqlResp.Errors[0].Message)
	}

	if gqlResp.Data == nil || gqlResp.Data.SearchBooks == nil || gqlResp.Data.SearchBooks.Results == nil {
		return nil, nil
	}

	hits := gqlResp.Data.SearchBooks.Results.Hits
	results := make([]BookMetadata, 0, len(hits))
	for _, hit := range hits {
		doc := hit.Document
		meta := BookMetadata{
			Title:       doc.Title,
			Description: doc.Description,
			Publisher:   doc.Publisher,
			PublishYear: doc.ReleaseYear,
		}
		if len(doc.AuthorNames) > 0 {
			meta.Author = strings.Join(doc.AuthorNames, ", ")
		}
		if doc.Image != nil && doc.Image.URL != "" {
			meta.CoverURL = doc.Image.URL
		}
		results = append(results, meta)
	}

	return results, nil
}
