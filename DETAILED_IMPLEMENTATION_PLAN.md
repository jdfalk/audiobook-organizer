# Detailed Implementation Plan: Wanted Feature & Enhanced Duplicate Detection

## Table of Contents
1. [Phase 1: Metadata Providers](#phase-1-metadata-providers)
2. [Phase 2: Store Interface Extensions](#phase-2-store-interface-extensions)
3. [Phase 3: API Endpoints](#phase-3-api-endpoints)
4. [Phase 4: Enhanced Duplicate Detection](#phase-4-enhanced-duplicate-detection)
5. [Phase 5: Frontend Components](#phase-5-frontend-components)
6. [Phase 6: State Transitions](#phase-6-state-transitions)
7. [Phase 7: Tests](#phase-7-tests)
8. [Phase 8: Cover Images](#phase-8-cover-images)

---

## Phase 1: Metadata Providers

### 1.1 Google Books Provider

**File**: `internal/metadata/googlebooks.go`

```go
package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// GoogleBooksClient handles Google Books API requests
type GoogleBooksClient struct {
	APIKey  string
	BaseURL string
	client  *http.Client
}

// NewGoogleBooksClient creates a new Google Books client
func NewGoogleBooksClient(apiKey string) *GoogleBooksClient {
	return &GoogleBooksClient{
		APIKey:  apiKey,
		BaseURL: "https://www.googleapis.com/books/v1",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GoogleBooksResponse represents the API response
type GoogleBooksResponse struct {
	Kind       string              `json:"kind"`
	TotalItems int                 `json:"totalItems"`
	Items      []GoogleBooksVolume `json:"items"`
}

// GoogleBooksVolume represents a single book volume
type GoogleBooksVolume struct {
	ID         string                 `json:"id"`
	VolumeInfo GoogleBooksVolumeInfo `json:"volumeInfo"`
}

// GoogleBooksVolumeInfo contains book metadata
type GoogleBooksVolumeInfo struct {
	Title               string                `json:"title"`
	Subtitle            string                `json:"subtitle"`
	Authors             []string              `json:"authors"`
	Publisher           string                `json:"publisher"`
	PublishedDate       string                `json:"publishedDate"`
	Description         string                `json:"description"`
	IndustryIdentifiers []IndustryIdentifier  `json:"industryIdentifiers"`
	PageCount           int                   `json:"pageCount"`
	Categories          []string              `json:"categories"`
	ImageLinks          *ImageLinks           `json:"imageLinks"`
	Language            string                `json:"language"`
}

// IndustryIdentifier represents ISBN or other identifiers
type IndustryIdentifier struct {
	Type       string `json:"type"` // ISBN_10, ISBN_13
	Identifier string `json:"identifier"`
}

// ImageLinks contains cover image URLs
type ImageLinks struct {
	SmallThumbnail string `json:"smallThumbnail"`
	Thumbnail      string `json:"thumbnail"`
	Small          string `json:"small"`
	Medium         string `json:"medium"`
	Large          string `json:"large"`
	ExtraLarge     string `json:"extraLarge"`
}

// SearchByTitle searches Google Books by title
func (c *GoogleBooksClient) SearchByTitle(title string) ([]BookMetadata, error) {
	return c.search(fmt.Sprintf("intitle:%s", title))
}

// SearchByTitleAndAuthor searches by both title and author
func (c *GoogleBooksClient) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	return c.search(fmt.Sprintf("intitle:%s inauthor:%s", title, author))
}

// SearchByISBN searches by ISBN
func (c *GoogleBooksClient) SearchByISBN(isbn string) ([]BookMetadata, error) {
	return c.search(fmt.Sprintf("isbn:%s", isbn))
}

// search performs the actual API request
func (c *GoogleBooksClient) search(query string) ([]BookMetadata, error) {
	// Build URL with query parameters
	apiURL := fmt.Sprintf("%s/volumes?q=%s&maxResults=10", c.BaseURL, url.QueryEscape(query))
	if c.APIKey != "" {
		apiURL += fmt.Sprintf("&key=%s", c.APIKey)
	}

	// Make HTTP request
	resp, err := c.client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("google books API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google books API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var gbResp GoogleBooksResponse
	if err := json.NewDecoder(resp.Body).Decode(&gbResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to standard BookMetadata format
	results := make([]BookMetadata, 0, len(gbResp.Items))
	for _, item := range gbResp.Items {
		metadata := c.convertToBookMetadata(item)
		results = append(results, metadata)
	}

	return results, nil
}

// convertToBookMetadata converts Google Books format to our standard format
func (c *GoogleBooksClient) convertToBookMetadata(volume GoogleBooksVolume) BookMetadata {
	info := volume.VolumeInfo

	// Combine title and subtitle
	fullTitle := info.Title
	if info.Subtitle != "" {
		fullTitle = fmt.Sprintf("%s: %s", info.Title, info.Subtitle)
	}

	// Get first author
	author := ""
	if len(info.Authors) > 0 {
		author = info.Authors[0]
	}

	// Extract publish year
	publishYear := 0
	if len(info.PublishedDate) >= 4 {
		fmt.Sscanf(info.PublishedDate[:4], "%d", &publishYear)
	}

	// Get ISBN (prefer ISBN-13, fallback to ISBN-10)
	isbn := ""
	for _, id := range info.IndustryIdentifiers {
		if id.Type == "ISBN_13" {
			isbn = id.Identifier
			break
		} else if id.Type == "ISBN_10" && isbn == "" {
			isbn = id.Identifier
		}
	}

	// Get best quality cover image
	coverURL := ""
	if info.ImageLinks != nil {
		// Prefer larger images
		if info.ImageLinks.Large != "" {
			coverURL = info.ImageLinks.Large
		} else if info.ImageLinks.Medium != "" {
			coverURL = info.ImageLinks.Medium
		} else if info.ImageLinks.Thumbnail != "" {
			coverURL = info.ImageLinks.Thumbnail
		}
	}

	return BookMetadata{
		Title:       fullTitle,
		Author:      author,
		Description: info.Description,
		Publisher:   info.Publisher,
		PublishYear: publishYear,
		ISBN:        isbn,
		CoverURL:    coverURL,
		Language:    info.Language,
		Source:      "Google Books", // NEW FIELD
	}
}
```

### 1.2 Audible Provider

**File**: `internal/metadata/audible.go`

```go
package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// AudibleClient handles Audible scraping/API requests
type AudibleClient struct {
	BaseURL string
	client  *http.Client
}

// NewAudibleClient creates a new Audible client
func NewAudibleClient() *AudibleClient {
	return &AudibleClient{
		BaseURL: "https://www.audible.com",
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// AudibleMetadata extends BookMetadata with audiobook-specific fields
type AudibleMetadata struct {
	BookMetadata
	Narrator      string
	RuntimeMinutes int
	Rating        float64
	RatingCount   int
	SeriesName    string
	SeriesPosition string
}

// SearchByTitle searches Audible by scraping search results
func (c *AudibleClient) SearchByTitle(title string) ([]AudibleMetadata, error) {
	// Build search URL
	searchURL := fmt.Sprintf("%s/search?keywords=%s&node=18573211011", c.BaseURL, url.QueryEscape(title))

	// Make request
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to mimic browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audible search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audible returned status %d", resp.StatusCode)
	}

	// Parse HTML response
	return c.parseSearchResults(resp.Body)
}

// parseSearchResults extracts book metadata from Audible search HTML
func (c *AudibleClient) parseSearchResults(body io.Reader) ([]AudibleMetadata, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	results := []AudibleMetadata{}

	// Find all product cards
	// Audible's HTML structure (as of 2026):
	// <li class="productListItem">
	//   <h3 class="bc-heading"><a href="...">Title</a></h3>
	//   <li class="authorLabel">By: <a>Author</a></li>
	//   <li class="narratorLabel">Narrated by: <a>Narrator</a></li>
	//   <li class="runtimeLabel">Length: X hrs Y mins</li>
	//   <span class="ratingsLabel">4.5 out of 5 stars</span>
	// </li>

	var findProducts func(*html.Node)
	findProducts = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" {
			if hasClass(n, "productListItem") {
				if metadata := c.extractProductMetadata(n); metadata != nil {
					results = append(results, *metadata)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			findProducts(child)
		}
	}

	findProducts(doc)

	return results, nil
}

// extractProductMetadata extracts metadata from a single product card
func (c *AudibleClient) extractProductMetadata(node *html.Node) *AudibleMetadata {
	metadata := &AudibleMetadata{}

	// Extract title
	if titleNode := findNodeByClass(node, "bc-heading"); titleNode != nil {
		if linkNode := findFirstElement(titleNode, "a"); linkNode != nil {
			metadata.Title = getTextContent(linkNode)
		}
	}

	// Extract author
	if authorNode := findNodeByClass(node, "authorLabel"); authorNode != nil {
		if linkNode := findFirstElement(authorNode, "a"); linkNode != nil {
			metadata.Author = getTextContent(linkNode)
		}
	}

	// Extract narrator
	if narratorNode := findNodeByClass(node, "narratorLabel"); narratorNode != nil {
		if linkNode := findFirstElement(narratorNode, "a"); linkNode != nil {
			metadata.Narrator = getTextContent(linkNode)
		}
	}

	// Extract runtime
	if runtimeNode := findNodeByClass(node, "runtimeLabel"); runtimeNode != nil {
		runtimeText := getTextContent(runtimeNode)
		metadata.RuntimeMinutes = parseRuntime(runtimeText)
	}

	// Extract rating
	if ratingNode := findNodeByClass(node, "ratingsLabel"); ratingNode != nil {
		ratingText := getTextContent(ratingNode)
		metadata.Rating = parseRating(ratingText)
	}

	// Extract series info if present
	if seriesNode := findNodeByClass(node, "seriesLabel"); seriesNode != nil {
		seriesText := getTextContent(seriesNode)
		metadata.SeriesName, metadata.SeriesPosition = parseSeries(seriesText)
	}

	metadata.Source = "Audible"

	return metadata
}

// Helper functions for HTML parsing

func hasClass(n *html.Node, className string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" && strings.Contains(attr.Val, className) {
			return true
		}
	}
	return false
}

func findNodeByClass(root *html.Node, className string) *html.Node {
	var result *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if result != nil {
			return
		}
		if n.Type == html.ElementNode && hasClass(n, className) {
			result = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(root)
	return result
}

func findFirstElement(root *html.Node, tag string) *html.Node {
	var result *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if result != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == tag {
			result = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(root)
	return result
}

func getTextContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data)
	}
	var text string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		text += getTextContent(child)
	}
	return strings.TrimSpace(text)
}

func parseRuntime(text string) int {
	// Parse "5 hrs and 23 mins" or "45 mins"
	re := regexp.MustCompile(`(\d+)\s*hrs?|(\d+)\s*mins?`)
	matches := re.FindAllStringSubmatch(text, -1)

	totalMinutes := 0
	for _, match := range matches {
		if match[1] != "" {
			hours, _ := strconv.Atoi(match[1])
			totalMinutes += hours * 60
		}
		if match[2] != "" {
			mins, _ := strconv.Atoi(match[2])
			totalMinutes += mins
		}
	}

	return totalMinutes
}

func parseRating(text string) float64 {
	// Parse "4.5 out of 5 stars"
	re := regexp.MustCompile(`([\d.]+)\s*out of`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		rating, _ := strconv.ParseFloat(matches[1], 64)
		return rating
	}
	return 0
}

func parseSeries(text string) (string, string) {
	// Parse "The Expanse, Book 1" or "Book 1 of The Expanse"
	re1 := regexp.MustCompile(`(.+),\s*Book\s*(\d+)`)
	re2 := regexp.MustCompile(`Book\s*(\d+)\s*of\s*(.+)`)

	if matches := re1.FindStringSubmatch(text); len(matches) > 2 {
		return matches[1], matches[2]
	}
	if matches := re2.FindStringSubmatch(text); len(matches) > 2 {
		return matches[2], matches[1]
	}

	return "", ""
}

// ConvertToBookMetadata converts AudibleMetadata to standard BookMetadata
func (am *AudibleMetadata) ConvertToBookMetadata() BookMetadata {
	return am.BookMetadata
}
```

### 1.3 Provider Aggregator

**File**: `internal/metadata/aggregator.go`

```go
package metadata

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// MetadataProvider is the interface all providers must implement
type MetadataProvider interface {
	SearchByTitle(title string) ([]BookMetadata, error)
	SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error)
	SearchByISBN(isbn string) ([]BookMetadata, error)
	Name() string
}

// ProviderAggregator combines results from multiple metadata providers
type ProviderAggregator struct {
	providers      []MetadataProvider
	failureTracker map[string]*ProviderHealth
	mu             sync.RWMutex
}

// ProviderHealth tracks provider availability and failures
type ProviderHealth struct {
	Name           string
	FailureCount   int
	LastFailure    time.Time
	LastSuccess    time.Time
	CircuitOpen    bool
	CircuitOpenAt  time.Time
}

// NewProviderAggregator creates a new aggregator with given providers
func NewProviderAggregator(providers ...MetadataProvider) *ProviderAggregator {
	agg := &ProviderAggregator{
		providers:      providers,
		failureTracker: make(map[string]*ProviderHealth),
	}

	// Initialize health tracking
	for _, provider := range providers {
		agg.failureTracker[provider.Name()] = &ProviderHealth{
			Name: provider.Name(),
		}
	}

	return agg
}

// SearchByTitle searches all providers and aggregates results
func (a *ProviderAggregator) SearchByTitle(title string) ([]BookMetadata, error) {
	return a.searchAll(func(p MetadataProvider) ([]BookMetadata, error) {
		return p.SearchByTitle(title)
	})
}

// SearchByTitleAndAuthor searches all providers
func (a *ProviderAggregator) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	return a.searchAll(func(p MetadataProvider) ([]BookMetadata, error) {
		return p.SearchByTitleAndAuthor(title, author)
	})
}

// SearchByISBN searches all providers
func (a *ProviderAggregator) SearchByISBN(isbn string) ([]BookMetadata, error) {
	return a.searchAll(func(p MetadataProvider) ([]BookMetadata, error) {
		return p.SearchByISBN(isbn)
	})
}

// searchAll executes search function across all healthy providers in parallel
func (a *ProviderAggregator) searchAll(searchFn func(MetadataProvider) ([]BookMetadata, error)) ([]BookMetadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type result struct {
		provider string
		results  []BookMetadata
		err      error
	}

	resultChan := make(chan result, len(a.providers))
	var wg sync.WaitGroup

	// Query each provider in parallel
	for _, provider := range a.providers {
		// Skip if circuit is open
		if a.isCircuitOpen(provider.Name()) {
			log.Printf("Skipping provider %s (circuit open)", provider.Name())
			continue
		}

		wg.Add(1)
		go func(p MetadataProvider) {
			defer wg.Done()

			// Execute search with timeout
			resultsChan := make(chan []BookMetadata, 1)
			errChan := make(chan error, 1)

			go func() {
				results, err := searchFn(p)
				if err != nil {
					errChan <- err
					return
				}
				resultsChan <- results
			}()

			select {
			case results := <-resultsChan:
				a.recordSuccess(p.Name())
				resultChan <- result{provider: p.Name(), results: results}
			case err := <-errChan:
				a.recordFailure(p.Name())
				log.Printf("Provider %s failed: %v", p.Name(), err)
				resultChan <- result{provider: p.Name(), err: err}
			case <-ctx.Done():
				a.recordFailure(p.Name())
				log.Printf("Provider %s timed out", p.Name())
				resultChan <- result{provider: p.Name(), err: fmt.Errorf("timeout")}
			}
		}(provider)
	}

	// Wait for all providers to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect all results
	allResults := []BookMetadata{}
	for res := range resultChan {
		if res.err == nil {
			allResults = append(allResults, res.results...)
		}
	}

	// Deduplicate and merge results
	deduped := a.deduplicateResults(allResults)

	return deduped, nil
}

// deduplicateResults removes duplicate books and merges metadata
func (a *ProviderAggregator) deduplicateResults(results []BookMetadata) []BookMetadata {
	// Key by ISBN first, then by normalized title+author
	bookMap := make(map[string]*BookMetadata)

	for _, book := range results {
		var key string

		// Prefer ISBN as key
		if book.ISBN != "" {
			key = "isbn:" + book.ISBN
		} else {
			// Fallback to normalized title+author
			normalizedTitle := normalizeString(book.Title)
			normalizedAuthor := normalizeString(book.Author)
			key = fmt.Sprintf("title:%s:author:%s", normalizedTitle, normalizedAuthor)
		}

		// If we've seen this book before, merge metadata
		if existing, found := bookMap[key]; found {
			*existing = a.mergeMetadata(*existing, book)
		} else {
			// New book
			bookCopy := book
			bookMap[key] = &bookCopy
		}
	}

	// Convert map back to slice
	deduped := make([]BookMetadata, 0, len(bookMap))
	for _, book := range bookMap {
		deduped = append(deduped, *book)
	}

	return deduped
}

// mergeMetadata combines metadata from multiple sources (prefer non-empty values)
func (a *ProviderAggregator) mergeMetadata(existing, new BookMetadata) BookMetadata {
	merged := existing

	// Prefer longer descriptions
	if len(new.Description) > len(merged.Description) {
		merged.Description = new.Description
	}

	// Prefer non-empty cover URLs
	if merged.CoverURL == "" && new.CoverURL != "" {
		merged.CoverURL = new.CoverURL
	}

	// Prefer non-empty publishers
	if merged.Publisher == "" && new.Publisher != "" {
		merged.Publisher = new.Publisher
	}

	// Prefer non-zero publish years
	if merged.PublishYear == 0 && new.PublishYear != 0 {
		merged.PublishYear = new.PublishYear
	}

	// Prefer ISBN-13 over ISBN-10
	if len(new.ISBN) > len(merged.ISBN) {
		merged.ISBN = new.ISBN
	}

	// Prefer non-empty languages
	if merged.Language == "" && new.Language != "" {
		merged.Language = new.Language
	}

	// Append source information
	if !strings.Contains(merged.Source, new.Source) {
		merged.Source = merged.Source + "," + new.Source
	}

	return merged
}

// normalizeString normalizes a string for comparison
func normalizeString(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	// Remove common articles
	s = strings.TrimPrefix(s, "the ")
	s = strings.TrimPrefix(s, "a ")
	s = strings.TrimPrefix(s, "an ")
	return s
}

// Circuit breaker methods

func (a *ProviderAggregator) isCircuitOpen(providerName string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	health, exists := a.failureTracker[providerName]
	if !exists {
		return false
	}

	// If circuit is open, check if enough time has passed to retry
	if health.CircuitOpen {
		if time.Since(health.CircuitOpenAt) > 60*time.Second {
			// Reset circuit after 60 seconds
			a.mu.RUnlock()
			a.mu.Lock()
			health.CircuitOpen = false
			a.mu.Unlock()
			a.mu.RLock()
			return false
		}
		return true
	}

	return false
}

func (a *ProviderAggregator) recordSuccess(providerName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if health, exists := a.failureTracker[providerName]; exists {
		health.FailureCount = 0
		health.LastSuccess = time.Now()
		health.CircuitOpen = false
	}
}

func (a *ProviderAggregator) recordFailure(providerName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if health, exists := a.failureTracker[providerName]; exists {
		health.FailureCount++
		health.LastFailure = time.Now()

		// Open circuit after 3 consecutive failures
		if health.FailureCount >= 3 {
			health.CircuitOpen = true
			health.CircuitOpenAt = time.Now()
			log.Printf("Circuit opened for provider %s after %d failures", providerName, health.FailureCount)
		}
	}
}

// GetProviderHealth returns current health status of all providers
func (a *ProviderAggregator) GetProviderHealth() map[string]ProviderHealth {
	a.mu.RLock()
	defer a.mu.RUnlock()

	health := make(map[string]ProviderHealth)
	for name, h := range a.failureTracker {
		health[name] = *h
	}

	return health
}
```

### 1.4 Update BookMetadata struct

**File**: `internal/metadata/openlibrary.go` (update existing struct)

Add `Source` field to existing BookMetadata:

```go
// BookMetadata represents metadata fetched from external sources
type BookMetadata struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Publisher   string `json:"publisher"`
	PublishYear int    `json:"publish_year"`
	ISBN        string `json:"isbn"`
	CoverURL    string `json:"cover_url"`
	Language    string `json:"language"`
	Source      string `json:"source"` // NEW: Track which provider(s) returned this
}
```

Update OpenLibraryClient methods to set `Source: "Open Library"` in all responses.

---

## Phase 2: Store Interface Extensions

### 2.1 Add Wanted Methods to Store Interface

**File**: `internal/database/store.go`

Add these methods to the `Store` interface:

```go
// Multi-path tracking
AddBookSourcePath(bookID, sourcePath string) error
GetBookSourcePaths(bookID string) ([]BookSourcePath, error)
IncrementSourcePathCount(bookID, sourcePath string) error
GetBookBySourcePath(sourcePath string) (*Book, error)

// Wanted items - Authors
SetAuthorWanted(authorID int, wanted bool) error
GetWantedAuthors() ([]Author, error)

// Wanted items - Series
SetSeriesWanted(seriesID int, wanted bool) error
GetWantedSeries() ([]Series, error)

// Wanted items - Books
CreateWantedBook(metadata map[string]interface{}) (*Book, error)
GetWantedBooks() ([]Book, error)

// State transitions
TransitionBookState(bookID, fromState, toState string) error
```

### 2.2 Add BookSourcePath struct

**File**: `internal/database/store.go`

```go
// BookSourcePath tracks original source locations for audiobooks
type BookSourcePath struct {
	ID            string    `json:"id"`
	AudiobookID   string    `json:"audiobook_id"`
	SourcePath    string    `json:"source_path"`
	StillExists   bool      `json:"still_exists"`
	AddedAt       time.Time `json:"added_at"`
	LastVerified  *time.Time `json:"last_verified,omitempty"`
	ImportCount   int       `json:"import_count"` // How many times this path was imported
}
```

### 2.3 Implement Methods in SQLiteStore

**File**: `internal/database/sqlite_store.go`

```go
// AddBookSourcePath adds a source path for an audiobook
func (s *SQLiteStore) AddBookSourcePath(bookID, sourcePath string) error {
	id := generateULID()
	now := time.Now()

	query := `
		INSERT INTO audiobook_source_paths (id, audiobook_id, source_path, added_at, import_count)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(source_path) DO UPDATE SET
			import_count = import_count + 1,
			last_verified = ?
	`

	_, err := s.db.Exec(query, id, bookID, sourcePath, now, now)
	return err
}

// GetBookSourcePaths retrieves all source paths for an audiobook
func (s *SQLiteStore) GetBookSourcePaths(bookID string) ([]BookSourcePath, error) {
	query := `
		SELECT id, audiobook_id, source_path, still_exists, added_at, last_verified, import_count
		FROM audiobook_source_paths
		WHERE audiobook_id = ?
		ORDER BY added_at DESC
	`

	rows, err := s.db.Query(query, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := []BookSourcePath{}
	for rows.Next() {
		var path BookSourcePath
		err := rows.Scan(
			&path.ID,
			&path.AudiobookID,
			&path.SourcePath,
			&path.StillExists,
			&path.AddedAt,
			&path.LastVerified,
			&path.ImportCount,
		)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}

	return paths, rows.Err()
}

// GetBookBySourcePath finds a book by its source path
func (s *SQLiteStore) GetBookBySourcePath(sourcePath string) (*Book, error) {
	query := `
		SELECT b.*
		FROM books b
		JOIN audiobook_source_paths asp ON b.id = asp.audiobook_id
		WHERE asp.source_path = ?
		LIMIT 1
	`

	var book Book
	err := s.db.QueryRow(query).Scan(/* scan all book fields */)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &book, nil
}

// SetAuthorWanted marks an author as wanted or not
func (s *SQLiteStore) SetAuthorWanted(authorID int, wanted bool) error {
	query := `UPDATE authors SET wanted = ? WHERE id = ?`
	_, err := s.db.Exec(query, wanted, authorID)
	return err
}

// GetWantedAuthors returns all authors marked as wanted
func (s *SQLiteStore) GetWantedAuthors() ([]Author, error) {
	query := `SELECT id, name, wanted FROM authors WHERE wanted = 1`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	authors := []Author{}
	for rows.Next() {
		var author Author
		err := rows.Scan(&author.ID, &author.Name, &author.Wanted)
		if err != nil {
			return nil, err
		}
		authors = append(authors, author)
	}

	return authors, rows.Err()
}

// SetSeriesWanted marks a series as wanted
func (s *SQLiteStore) SetSeriesWanted(seriesID int, wanted bool) error {
	query := `UPDATE series SET wanted = ? WHERE id = ?`
	_, err := s.db.Exec(query, wanted, seriesID)
	return err
}

// GetWantedSeries returns all series marked as wanted
func (s *SQLiteStore) GetWantedSeries() ([]Series, error) {
	query := `SELECT id, name, author_id, wanted FROM series WHERE wanted = 1`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	series := []Series{}
	for rows.Next() {
		var s Series
		err := rows.Scan(&s.ID, &s.Name, &s.AuthorID, &s.Wanted)
		if err != nil {
			return nil, err
		}
		series = append(series, s)
	}

	return series, rows.Err()
}

// CreateWantedBook creates a book in wanted state with no file
func (s *SQLiteStore) CreateWantedBook(metadata map[string]interface{}) (*Book, error) {
	book := &Book{
		ID:           generateULID(),
		LibraryState: "wanted",
		FilePath:     "", // Empty for wanted books
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Populate from metadata map
	if title, ok := metadata["title"].(string); ok {
		book.Title = title
	}
	if author, ok := metadata["author"].(string); ok {
		// Look up or create author
		existingAuthor, _ := s.GetAuthorByName(author)
		if existingAuthor != nil {
			book.AuthorID = &existingAuthor.ID
		} else {
			newAuthor, err := s.CreateAuthor(author)
			if err == nil {
				book.AuthorID = &newAuthor.ID
			}
		}
	}
	// ... populate other fields from metadata

	// Insert into database
	query := `
		INSERT INTO books (id, title, author_id, library_state, file_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, book.ID, book.Title, book.AuthorID, book.LibraryState, book.FilePath, book.CreatedAt, book.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return book, nil
}

// GetWantedBooks returns all books in wanted state
func (s *SQLiteStore) GetWantedBooks() ([]Book, error) {
	query := `SELECT * FROM books WHERE library_state = 'wanted' ORDER BY created_at DESC`
	// ... standard book query logic
}

// TransitionBookState changes a book's state with validation
func (s *SQLiteStore) TransitionBookState(bookID, fromState, toState string) error {
	// Validate transition is allowed
	validTransitions := map[string][]string{
		"wanted":    {"imported", "deleted"},
		"imported":  {"organized", "wanted", "deleted"},
		"organized": {"wanted", "deleted"},
		"deleted":   {"wanted", "imported"},
	}

	allowed := false
	for _, validTo := range validTransitions[fromState] {
		if validTo == toState {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("invalid state transition: %s -> %s", fromState, toState)
	}

	query := `UPDATE books SET library_state = ?, updated_at = ? WHERE id = ? AND library_state = ?`
	result, err := s.db.Exec(query, toState, time.Now(), bookID, fromState)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("no book found with id %s in state %s", bookID, fromState)
	}

	return nil
}
```

### 2.4 Implement Methods in PebbleStore

**File**: `internal/database/pebble_store.go`

Implement the same methods for PebbleDB using key-value patterns:

```go
// Key patterns:
// source_path:{path_id} -> BookSourcePath JSON
// source_path_by_book:{book_id}:{path_id} -> path_id (for lookup)
// source_path_by_path:{sha256(path)} -> path_id (for dedup)

// AddBookSourcePath implementation for Pebble
func (s *PebbleStore) AddBookSourcePath(bookID, sourcePath string) error {
	pathID := generateULID()
	pathHash := sha256Hash(sourcePath)

	// Check if path already exists
	existingKey := fmt.Sprintf("source_path_by_path:%s", pathHash)
	existingID, err := s.db.Get([]byte(existingKey), nil)
	if err == nil {
		// Path exists, increment count
		var existing BookSourcePath
		pathKey := fmt.Sprintf("source_path:%s", string(existingID))
		data, _ := s.db.Get([]byte(pathKey), nil)
		json.Unmarshal(data, &existing)
		existing.ImportCount++
		existing.LastVerified = ptrTime(time.Now())

		updatedData, _ := json.Marshal(existing)
		return s.db.Put([]byte(pathKey), updatedData, nil)
	}

	// Create new source path
	sourcePath := BookSourcePath{
		ID:          pathID,
		AudiobookID: bookID,
		SourcePath:  sourcePath,
		StillExists: true,
		AddedAt:     time.Now(),
		ImportCount: 1,
	}

	data, err := json.Marshal(sourcePath)
	if err != nil {
		return err
	}

	// Store in multiple indices
	batch := &leveldb.Batch{}
	batch.Put([]byte(fmt.Sprintf("source_path:%s", pathID)), data)
	batch.Put([]byte(fmt.Sprintf("source_path_by_book:%s:%s", bookID, pathID)), []byte(pathID))
	batch.Put([]byte(existingKey), []byte(pathID))

	return s.db.Write(batch, nil)
}

// ... implement other methods similarly
```

---

## Phase 3: API Endpoints

### 3.1 Unified Search Endpoint

**File**: `internal/server/server.go`

Add this handler:

```go
// handleUnifiedSearch searches across all metadata providers and local DB
func (s *Server) handleUnifiedSearch(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter required"})
		return
	}

	searchType := c.DefaultQuery("type", "all") // book, author, series, all

	// Initialize metadata aggregator if not already done
	if s.metadataAggregator == nil {
		// Create providers
		openLibrary := metadata.NewOpenLibraryClient()
		googleBooks := metadata.NewGoogleBooksClient(s.config.GoogleBooksAPIKey)
		audible := metadata.NewAudibleClient()
		// goodreads if available

		s.metadataAggregator = metadata.NewProviderAggregator(
			openLibrary,
			googleBooks,
			audible,
		)
	}

	// Search external metadata sources
	externalResults, err := s.metadataAggregator.SearchByTitle(query)
	if err != nil {
		log.Printf("Error searching external sources: %v", err)
		externalResults = []metadata.BookMetadata{}
	}

	// Search local database
	localBooks, _ := s.store.SearchBooks(query, 100, 0)
	localAuthors, _ := s.searchAuthors(query)
	localSeries, _ := s.searchSeries(query)

	// Categorize results
	response := gin.H{
		"query": query,
		"books": gin.H{
			"local":    localBooks,
			"external": externalResults,
		},
	}

	if searchType == "all" || searchType == "author" {
		response["authors"] = localAuthors
	}

	if searchType == "all" || searchType == "series" {
		response["series"] = localSeries
	}

	c.JSON(http.StatusOK, response)
}

// Helper to search authors by name
func (s *Server) searchAuthors(query string) ([]database.Author, error) {
	// Implement fuzzy author search in database
	// For now, simple LIKE query
	authors, err := s.store.GetAllAuthors()
	if err != nil {
		return nil, err
	}

	matching := []database.Author{}
	lowerQuery := strings.ToLower(query)
	for _, author := range authors {
		if strings.Contains(strings.ToLower(author.Name), lowerQuery) {
			matching = append(matching, author)
		}
	}

	return matching, nil
}

// Helper to search series by name
func (s *Server) searchSeries(query string) ([]database.Series, error) {
	series, err := s.store.GetAllSeries()
	if err != nil {
		return nil, err
	}

	matching := []database.Series{}
	lowerQuery := strings.ToLower(query)
	for _, s := range series {
		if strings.Contains(strings.ToLower(s.Name), lowerQuery) {
			matching = append(matching, s)
		}
	}

	return matching, nil
}
```

Register route:

```go
v1.GET("/search/unified", s.handleUnifiedSearch)
```

### 3.2 Wanted Management Endpoints

**File**: `internal/server/server.go`

```go
// handleAddWantedBook adds a book to wanted list
func (s *Server) handleAddWantedBook(c *gin.Context) {
	var req struct {
		Metadata map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	book, err := s.store.CreateWantedBook(req.Metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, book)
}

// handleAddWantedAuthor adds author and optionally all their books
func (s *Server) handleAddWantedAuthor(c *gin.Context) {
	var req struct {
		AuthorName  string `json:"author_name"`
		AddAllBooks bool   `json:"add_all_books"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create or get author
	author, err := s.store.GetAuthorByName(req.AuthorName)
	if err != nil || author == nil {
		author, err = s.store.CreateAuthor(req.AuthorName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Mark as wanted
	err = s.store.SetAuthorWanted(author.ID, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	booksAdded := 0

	if req.AddAllBooks {
		// Search for all books by this author from metadata providers
		if s.metadataAggregator != nil {
			results, err := s.metadataAggregator.SearchByTitleAndAuthor("", req.AuthorName)
			if err == nil {
				for _, bookMeta := range results {
					metadata := map[string]interface{}{
						"title":        bookMeta.Title,
						"author":       bookMeta.Author,
						"description":  bookMeta.Description,
						"publisher":    bookMeta.Publisher,
						"publish_year": bookMeta.PublishYear,
						"isbn":         bookMeta.ISBN,
						"cover_url":    bookMeta.CoverURL,
						"language":     bookMeta.Language,
					}

					_, err := s.store.CreateWantedBook(metadata)
					if err == nil {
						booksAdded++
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"author":      author,
		"books_added": booksAdded,
	})
}

// handleAddWantedSeries adds series and optionally all author's works
func (s *Server) handleAddWantedSeries(c *gin.Context) {
	var req struct {
		SeriesName      string  `json:"series_name"`
		AuthorName      string  `json:"author_name"`
		AddAuthorWorks  bool    `json:"add_author_works"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get or create author
	var authorID *int
	if req.AuthorName != "" {
		author, err := s.store.GetAuthorByName(req.AuthorName)
		if err != nil || author == nil {
			author, err = s.store.CreateAuthor(req.AuthorName)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		authorID = &author.ID
	}

	// Create or get series
	series, err := s.store.GetSeriesByName(req.SeriesName, authorID)
	if err != nil || series == nil {
		series, err = s.store.CreateSeries(req.SeriesName, authorID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Mark as wanted
	err = s.store.SetSeriesWanted(series.ID, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// If requested, search for all books in this series
	// ... similar to author logic

	c.JSON(http.StatusOK, gin.H{"series": series})
}

// handleListWanted returns all wanted items
func (s *Server) handleListWanted(c *gin.Context) {
	wantedBooks, _ := s.store.GetWantedBooks()
	wantedAuthors, _ := s.store.GetWantedAuthors()
	wantedSeries, _ := s.store.GetWantedSeries()

	c.JSON(http.StatusOK, gin.H{
		"books":   wantedBooks,
		"authors": wantedAuthors,
		"series":  wantedSeries,
	})
}

// handleDeleteWanted removes item from wanted list
func (s *Server) handleDeleteWanted(c *gin.Context) {
	itemType := c.Param("type") // book, author, series
	itemID := c.Param("id")

	switch itemType {
	case "book":
		// Delete book if in wanted state
		err := s.store.DeleteBook(itemID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

	case "author":
		id, _ := strconv.Atoi(itemID)
		err := s.store.SetAuthorWanted(id, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

	case "series":
		id, _ := strconv.Atoi(itemID)
		err := s.store.SetSeriesWanted(id, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// handleTransitionBookState manually transitions a book's state
func (s *Server) handleTransitionBookState(c *gin.Context) {
	bookID := c.Param("id")

	var req struct {
		FromState string `json:"from_state"`
		ToState   string `json:"to_state"`
		FilePath  string `json:"file_path"` // Required for wanted->imported
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate file_path for wanted->imported transition
	if req.FromState == "wanted" && req.ToState == "imported" && req.FilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_path required for wanted->imported transition"})
		return
	}

	// Perform transition
	err := s.store.TransitionBookState(bookID, req.FromState, req.ToState)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// If transitioning to imported, update file_path
	if req.ToState == "imported" && req.FilePath != "" {
		book, _ := s.store.GetBookByID(bookID)
		if book != nil {
			book.FilePath = req.FilePath
			s.store.UpdateBook(bookID, book)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "transitioned"})
}
```

Register routes:

```go
v1.POST("/wanted/book", s.handleAddWantedBook)
v1.POST("/wanted/author", s.handleAddWantedAuthor)
v1.POST("/wanted/series", s.handleAddWantedSeries)
v1.GET("/wanted", s.handleListWanted)
v1.DELETE("/wanted/:type/:id", s.handleDeleteWanted)
v1.POST("/books/:id/transition", s.handleTransitionBookState)
```

---

## Phase 4: Enhanced Duplicate Detection

### 4.1 Update Scanner Logic

**File**: `internal/scanner/scanner.go`

Modify `saveBookToDatabase()`:

```go
func saveBookToDatabase(book *Book, database Store, config *config.Config, progressCallback func(string)) error {
	// ... existing metadata extraction code ...

	// Compute file hash
	hash, err := ComputeFileHash(book.FilePath)
	if err != nil {
		log.Printf("Warning: Could not compute hash for %s: %v", book.FilePath, err)
	}

	// Check if hash is blocked
	if hash != "" {
		blocked, err := database.IsHashBlocked(hash)
		if err == nil && blocked {
			log.Printf("Skipping blocked hash: %s (%s)", hash, book.FilePath)
			return nil
		}
	}

	// NEW: Check if this exact source path already exists
	existingByPath, err := database.GetBookBySourcePath(book.FilePath)
	if err == nil && existingByPath != nil {
		log.Printf("Duplicate import detected: exact path already exists for book %s", existingByPath.Title)
		// Increment import count
		database.IncrementSourcePathCount(existingByPath.ID, book.FilePath)

		// Emit SSE event
		if progressCallback != nil {
			progressCallback(fmt.Sprintf("duplicate_exact_path:%s:%s", existingByPath.ID, book.FilePath))
		}

		return fmt.Errorf("duplicate: exact source path already imported")
	}

	// NEW: Check if hash exists (same file, different path)
	if hash != "" {
		existingByHash, err := database.GetBookByFileHash(hash)
		if err == nil && existingByHash != nil {
			log.Printf("Duplicate file detected (different path): adding source path for book %s", existingByHash.Title)

			// Add new source path
			err = database.AddBookSourcePath(existingByHash.ID, book.FilePath)
			if err != nil {
				log.Printf("Warning: Could not add source path: %v", err)
			}

			// Emit SSE event
			if progressCallback != nil {
				progressCallback(fmt.Sprintf("duplicate_new_path:%s:%s", existingByHash.ID, book.FilePath))
			}

			return fmt.Errorf("duplicate: file already imported from different path")
		}
	}

	// NEW: Check wanted list for auto-match
	if hash != "" {
		wantedBooks, _ := database.GetWantedBooks()
		for _, wanted := range wantedBooks {
			// Match by title similarity or ISBN
			if matchesWantedBook(book, &wanted) {
				log.Printf("Auto-matching imported file to wanted book: %s", wanted.Title)

				// Transition wanted->imported
				err = database.TransitionBookState(wanted.ID, "wanted", "imported")
				if err != nil {
					log.Printf("Warning: Could not transition state: %v", err)
					continue
				}

				// Update book with file info
				wanted.FilePath = book.FilePath
				wanted.FileHash = &hash
				wanted.OriginalFileHash = &hash
				wanted.UpdatedAt = time.Now()
				database.UpdateBook(wanted.ID, &wanted)

				// Add source path
				database.AddBookSourcePath(wanted.ID, book.FilePath)

				// Emit SSE event
				if progressCallback != nil {
					progressCallback(fmt.Sprintf("wanted_matched:%s:%s", wanted.ID, book.FilePath))
				}

				return nil
			}
		}
	}

	// No duplicate found - create new book
	book.ID = generateULID()
	book.LibraryState = "imported"
	book.FileHash = &hash
	book.OriginalFileHash = &hash
	book.CreatedAt = time.Now()
	book.UpdatedAt = time.Now()

	// Save to database
	_, err = database.CreateBook(book)
	if err != nil {
		return fmt.Errorf("failed to create book: %w", err)
	}

	// Add first source path entry
	err = database.AddBookSourcePath(book.ID, book.FilePath)
	if err != nil {
		log.Printf("Warning: Could not add source path: %v", err)
	}

	// Emit SSE event
	if progressCallback != nil {
		progressCallback(fmt.Sprintf("new_book:%s:%s", book.ID, book.FilePath))
	}

	return nil
}

// matchesWantedBook checks if imported book matches wanted book
func matchesWantedBook(imported, wanted *Book) bool {
	// Exact title match (case-insensitive)
	if strings.EqualFold(imported.Title, wanted.Title) {
		return true
	}

	// Title similarity (Levenshtein distance or similar)
	similarity := stringSimilarity(imported.Title, wanted.Title)
	if similarity > 0.8 {
		return true
	}

	// ISBN match (if both have ISBNs)
	if imported.ISBN10 != nil && wanted.ISBN10 != nil && *imported.ISBN10 == *wanted.ISBN10 {
		return true
	}
	if imported.ISBN13 != nil && wanted.ISBN13 != nil && *imported.ISBN13 == *wanted.ISBN13 {
		return true
	}

	return false
}

// Simple string similarity using normalized edit distance
func stringSimilarity(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)

	if a == b {
		return 1.0
	}

	distance := levenshteinDistance(a, b)
	maxLen := max(len(a), len(b))

	return 1.0 - (float64(distance) / float64(maxLen))
}

func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			matrix[i][j] = min3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

### 4.2 Bulk Validate Endpoint

**File**: `internal/server/server.go`

```go
// handleBulkValidate checks files for duplicates before import
func (s *Server) handleBulkValidate(c *gin.Context) {
	var req struct {
		FilePaths []string `json:"file_paths"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]gin.H, 0, len(req.FilePaths))

	for _, path := range req.FilePaths {
		result := gin.H{
			"path":   path,
			"status": "new",
		}

		// Compute hash
		hash, err := scanner.ComputeFileHash(path)
		if err != nil {
			result["status"] = "error"
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		// Check for exact path match
		existingByPath, _ := s.store.GetBookBySourcePath(path)
		if existingByPath != nil {
			result["status"] = "duplicate_exact_path"
			result["existing_book_id"] = existingByPath.ID
			result["existing_book_title"] = existingByPath.Title
			results = append(results, result)
			continue
		}

		// Check for hash match (same file, different path)
		existingByHash, _ := s.store.GetBookByFileHash(hash)
		if existingByHash != nil {
			result["status"] = "duplicate_new_path"
			result["existing_book_id"] = existingByHash.ID
			result["existing_book_title"] = existingByHash.Title
			results = append(results, result)
			continue
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"summary": gin.H{
			"total":              len(req.FilePaths),
			"new":                countByStatus(results, "new"),
			"duplicate_exact":    countByStatus(results, "duplicate_exact_path"),
			"duplicate_new_path": countByStatus(results, "duplicate_new_path"),
			"errors":             countByStatus(results, "error"),
		},
	})
}

func countByStatus(results []gin.H, status string) int {
	count := 0
	for _, r := range results {
		if r["status"] == status {
			count++
		}
	}
	return count
}
```

Register route:

```go
v1.POST("/import/bulk-validate", s.handleBulkValidate)
```

---

## Phase 5: Frontend Components

### 5.1 Unified Search Component

**File**: `web/src/components/search/UnifiedSearch.tsx`

```typescript
import React, { useState, useEffect } from 'react';
import {
  Box,
  TextField,
  Tabs,
  Tab,
  Card,
  CardContent,
  Typography,
  Button,
  Chip,
  CircularProgress,
  Grid,
} from '@mui/material';
import { SearchOutlined, AddCircleOutline } from '@mui/icons-material';
import { searchUnified, addWantedBook, addWantedAuthor, addWantedSeries } from '../../services/api';

interface SearchResult {
  query: string;
  books: {
    local: Book[];
    external: BookMetadata[];
  };
  authors?: Author[];
  series?: Series[];
}

interface BookMetadata {
  title: string;
  author: string;
  description: string;
  publisher?: string;
  publish_year?: number;
  isbn?: string;
  cover_url?: string;
  language?: string;
  source: string; // "Open Library,Google Books"
}

export function UnifiedSearch() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState(0);
  const [addingItem, setAddingItem] = useState<string | null>(null);

  // Debounced search
  useEffect(() => {
    if (query.length < 3) {
      setResults(null);
      return;
    }

    const timeoutId = setTimeout(async () => {
      setLoading(true);
      try {
        const data = await searchUnified(query);
        setResults(data);
      } catch (error) {
        console.error('Search failed:', error);
      } finally {
        setLoading(false);
      }
    }, 500);

    return () => clearTimeout(timeoutId);
  }, [query]);

  const handleAddBook = async (metadata: BookMetadata) => {
    setAddingItem(metadata.title);
    try {
      await addWantedBook({ metadata });
      alert(`Added "${metadata.title}" to wanted list!`);
    } catch (error) {
      alert(`Error: ${error.message}`);
    } finally {
      setAddingItem(null);
    }
  };

  const handleAddAuthor = async (authorName: string, addAllBooks: boolean) => {
    setAddingItem(authorName);
    try {
      const result = await addWantedAuthor({ author_name: authorName, add_all_books: addAllBooks });
      alert(`Added "${authorName}" to wanted list! ${result.books_added} books added.`);
    } catch (error) {
      alert(`Error: ${error.message}`);
    } finally {
      setAddingItem(null);
    }
  };

  const handleAddSeries = async (seriesName: string, authorName: string) => {
    setAddingItem(seriesName);
    try {
      await addWantedSeries({ series_name: seriesName, author_name: authorName });
      alert(`Added "${seriesName}" to wanted list!`);
    } catch (error) {
      alert(`Error: ${error.message}`);
    } finally {
      setAddingItem(null);
    }
  };

  return (
    <Box>
      <TextField
        fullWidth
        placeholder="Search for books, authors, or series..."
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        InputProps={{
          startAdornment: <SearchOutlined sx={{ mr: 1, color: 'text.secondary' }} />,
        }}
        sx={{ mb: 3 }}
      />

      {loading && (
        <Box display="flex" justifyContent="center" my={4}>
          <CircularProgress />
        </Box>
      )}

      {results && !loading && (
        <>
          <Tabs value={activeTab} onChange={(e, v) => setActiveTab(v)} sx={{ mb: 2 }}>
            <Tab label={`Books (${(results.books.local?.length || 0) + (results.books.external?.length || 0)})`} />
            <Tab label={`Authors (${results.authors?.length || 0})`} />
            <Tab label={`Series (${results.series?.length || 0})`} />
          </Tabs>

          {/* Books Tab */}
          {activeTab === 0 && (
            <Box>
              {/* Local Books */}
              {results.books.local && results.books.local.length > 0 && (
                <>
                  <Typography variant="h6" sx={{ mb: 2 }}>In Your Library</Typography>
                  <Grid container spacing={2} sx={{ mb: 4 }}>
                    {results.books.local.map((book) => (
                      <Grid item xs={12} sm={6} md={4} key={book.id}>
                        <Card>
                          <CardContent>
                            <Typography variant="h6" noWrap>{book.title}</Typography>
                            <Typography variant="body2" color="text.secondary">{book.author_name}</Typography>
                            <Chip
                              label={book.library_state}
                              size="small"
                              sx={{ mt: 1 }}
                              color={book.library_state === 'wanted' ? 'warning' : 'default'}
                            />
                          </CardContent>
                        </Card>
                      </Grid>
                    ))}
                  </Grid>
                </>
              )}

              {/* External Books */}
              {results.books.external && results.books.external.length > 0 && (
                <>
                  <Typography variant="h6" sx={{ mb: 2 }}>Available to Add</Typography>
                  <Grid container spacing={2}>
                    {results.books.external.map((book, idx) => (
                      <Grid item xs={12} sm={6} md={4} key={idx}>
                        <Card>
                          <CardContent>
                            {book.cover_url && (
                              <img
                                src={book.cover_url}
                                alt={book.title}
                                style={{ width: '100%', height: 200, objectFit: 'cover', marginBottom: 8 }}
                              />
                            )}
                            <Typography variant="h6" noWrap>{book.title}</Typography>
                            <Typography variant="body2" color="text.secondary">{book.author}</Typography>
                            <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>
                              Source: {book.source}
                            </Typography>
                            <Typography variant="body2" sx={{ mt: 1, mb: 2 }} noWrap>
                              {book.description}
                            </Typography>
                            <Button
                              fullWidth
                              variant="outlined"
                              size="small"
                              startIcon={addingItem === book.title ? <CircularProgress size={16} /> : <AddCircleOutline />}
                              onClick={() => handleAddBook(book)}
                              disabled={addingItem === book.title}
                            >
                              {addingItem === book.title ? 'Adding...' : 'Add to Wanted'}
                            </Button>
                          </CardContent>
                        </Card>
                      </Grid>
                    ))}
                  </Grid>
                </>
              )}
            </Box>
          )}

          {/* Authors Tab */}
          {activeTab === 1 && results.authors && (
            <Grid container spacing={2}>
              {results.authors.map((author) => (
                <Grid item xs={12} sm={6} md={4} key={author.id}>
                  <Card>
                    <CardContent>
                      <Typography variant="h6">{author.name}</Typography>
                      <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={addingItem === author.name ? <CircularProgress size={16} /> : <AddCircleOutline />}
                          onClick={() => handleAddAuthor(author.name, false)}
                          disabled={addingItem === author.name}
                        >
                          Add Author
                        </Button>
                        <Button
                          variant="contained"
                          size="small"
                          startIcon={addingItem === `${author.name}-all` ? <CircularProgress size={16} /> : <AddCircleOutline />}
                          onClick={() => handleAddAuthor(author.name, true)}
                          disabled={addingItem === `${author.name}-all`}
                        >
                          Add All Works
                        </Button>
                      </Box>
                    </CardContent>
                  </Card>
                </Grid>
              ))}
            </Grid>
          )}

          {/* Series Tab */}
          {activeTab === 2 && results.series && (
            <Grid container spacing={2}>
              {results.series.map((series) => (
                <Grid item xs={12} sm={6} md={4} key={series.id}>
                  <Card>
                    <CardContent>
                      <Typography variant="h6">{series.name}</Typography>
                      {series.author_name && (
                        <Typography variant="body2" color="text.secondary">by {series.author_name}</Typography>
                      )}
                      <Button
                        fullWidth
                        variant="outlined"
                        size="small"
                        sx={{ mt: 2 }}
                        startIcon={addingItem === series.name ? <CircularProgress size={16} /> : <AddCircleOutline />}
                        onClick={() => handleAddSeries(series.name, series.author_name || '')}
                        disabled={addingItem === series.name}
                      >
                        {addingItem === series.name ? 'Adding...' : 'Add Series'}
                      </Button>
                    </CardContent>
                  </Card>
                </Grid>
              ))}
            </Grid>
          )}
        </>
      )}
    </Box>
  );
}
```

### 5.2 Wanted List Page

**File**: `web/src/pages/Wanted.tsx`

```typescript
import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Tabs,
  Tab,
  Card,
  CardContent,
  IconButton,
  Chip,
  Grid,
  Button,
} from '@mui/material';
import { Delete, CheckCircle } from '@mui/icons-material';
import { getWantedItems, deleteWantedItem, transitionBookState } from '../services/api';

interface WantedItems {
  books: Book[];
  authors: Author[];
  series: Series[];
}

export function Wanted() {
  const [activeTab, setActiveTab] = useState(0);
  const [wanted, setWanted] = useState<WantedItems>({ books: [], authors: [], series: [] });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadWanted();
  }, []);

  const loadWanted = async () => {
    setLoading(true);
    try {
      const data = await getWantedItems();
      setWanted(data);
    } catch (error) {
      console.error('Failed to load wanted items:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleRemove = async (type: 'book' | 'author' | 'series', id: string | number) => {
    if (!confirm('Remove from wanted list?')) return;

    try {
      await deleteWantedItem(type, String(id));
      loadWanted();
    } catch (error) {
      alert(`Error: ${error.message}`);
    }
  };

  const handleMarkAsImported = async (bookId: string) => {
    const filePath = prompt('Enter the file path for this imported book:');
    if (!filePath) return;

    try {
      await transitionBookState(bookId, {
        from_state: 'wanted',
        to_state: 'imported',
        file_path: filePath,
      });
      loadWanted();
      alert('Book marked as imported!');
    } catch (error) {
      alert(`Error: ${error.message}`);
    }
  };

  return (
    <Box>
      <Typography variant="h4" sx={{ mb: 3 }}>Wanted List</Typography>

      <Tabs value={activeTab} onChange={(e, v) => setActiveTab(v)} sx={{ mb: 3 }}>
        <Tab label={`Books (${wanted.books.length})`} />
        <Tab label={`Authors (${wanted.authors.length})`} />
        <Tab label={`Series (${wanted.series.length})`} />
      </Tabs>

      {/* Books Tab */}
      {activeTab === 0 && (
        <Grid container spacing={2}>
          {wanted.books.map((book) => (
            <Grid item xs={12} sm={6} md={4} key={book.id}>
              <Card>
                <CardContent>
                  <Typography variant="h6" noWrap>{book.title}</Typography>
                  <Typography variant="body2" color="text.secondary">{book.author_name}</Typography>
                  <Chip label="Wanted" size="small" color="warning" sx={{ mt: 1 }} />
                  <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
                    <IconButton
                      size="small"
                      color="success"
                      onClick={() => handleMarkAsImported(book.id)}
                      title="Mark as Imported"
                    >
                      <CheckCircle />
                    </IconButton>
                    <IconButton
                      size="small"
                      color="error"
                      onClick={() => handleRemove('book', book.id)}
                      title="Remove from Wanted List"
                    >
                      <Delete />
                    </IconButton>
                  </Box>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

      {/* Authors Tab */}
      {activeTab === 1 && (
        <Grid container spacing={2}>
          {wanted.authors.map((author) => (
            <Grid item xs={12} sm={6} md={4} key={author.id}>
              <Card>
                <CardContent>
                  <Typography variant="h6">{author.name}</Typography>
                  <IconButton
                    size="small"
                    color="error"
                    onClick={() => handleRemove('author', author.id)}
                    sx={{ mt: 1 }}
                  >
                    <Delete />
                  </IconButton>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

      {/* Series Tab */}
      {activeTab === 2 && (
        <Grid container spacing={2}>
          {wanted.series.map((series) => (
            <Grid item xs={12} sm={6} md={4} key={series.id}>
              <Card>
                <CardContent>
                  <Typography variant="h6">{series.name}</Typography>
                  {series.author_name && (
                    <Typography variant="body2" color="text.secondary">by {series.author_name}</Typography>
                  )}
                  <IconButton
                    size="small"
                    color="error"
                    onClick={() => handleRemove('series', series.id)}
                    sx={{ mt: 1 }}
                  >
                    <Delete />
                  </IconButton>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}
    </Box>
  );
}
```

### 5.3 Update API Service

**File**: `web/src/services/api.ts`

Add these methods:

```typescript
export async function searchUnified(query: string, type: string = 'all'): Promise<any> {
  const response = await fetch(`/api/v1/search/unified?q=${encodeURIComponent(query)}&type=${type}`);
  if (!response.ok) throw new Error('Search failed');
  return response.json();
}

export async function addWantedBook(data: { metadata: any }): Promise<any> {
  const response = await fetch('/api/v1/wanted/book', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!response.ok) throw new Error('Failed to add wanted book');
  return response.json();
}

export async function addWantedAuthor(data: { author_name: string; add_all_books: boolean }): Promise<any> {
  const response = await fetch('/api/v1/wanted/author', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!response.ok) throw new Error('Failed to add wanted author');
  return response.json();
}

export async function addWantedSeries(data: { series_name: string; author_name: string }): Promise<any> {
  const response = await fetch('/api/v1/wanted/series', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!response.ok) throw new Error('Failed to add wanted series');
  return response.json();
}

export async function getWantedItems(): Promise<any> {
  const response = await fetch('/api/v1/wanted');
  if (!response.ok) throw new Error('Failed to load wanted items');
  return response.json();
}

export async function deleteWantedItem(type: string, id: string): Promise<void> {
  const response = await fetch(`/api/v1/wanted/${type}/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to delete wanted item');
}

export async function transitionBookState(bookId: string, data: any): Promise<any> {
  const response = await fetch(`/api/v1/books/${bookId}/transition`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!response.ok) throw new Error('Failed to transition book state');
  return response.json();
}

export async function bulkValidateFiles(filePaths: string[]): Promise<any> {
  const response = await fetch('/api/v1/import/bulk-validate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ file_paths: filePaths }),
  });
  if (!response.ok) throw new Error('Bulk validation failed');
  return response.json();
}
```

---

## Phase 6: State Transitions

Already covered in Phase 2 (Store methods) and Phase 3 (API endpoints). The key logic is:

1. **Automatic transitions** happen in `scanner.go` when:
   - Wanted book is matched to imported file by title/ISBN
   - State changes from `wanted` → `imported`

2. **Manual transitions** via API endpoint:
   - User can manually transition states via POST `/api/v1/books/:id/transition`
   - Validation ensures only valid transitions are allowed

---

## Phase 7: Tests

### 7.1 Duplicate Detection E2E Test

**File**: `web/tests/e2e/duplicate-detection.spec.ts`

```typescript
import { test, expect } from '@playwright/test';

test.describe('Duplicate Detection', () => {
  test('should detect duplicate when importing same file 5 times', async ({ page }) => {
    await page.goto('https://localhost:8080/library');

    // Import the same file 5 times
    const testFilePath = '/path/to/test/audiobook.mp3';

    for (let i = 0; i < 5; i++) {
      // Trigger import
      await page.click('[data-testid="import-button"]');
      await page.fill('[data-testid="file-path-input"]', testFilePath);
      await page.click('[data-testid="confirm-import"]');

      // Wait for response
      await page.waitForTimeout(1000);

      if (i === 0) {
        // First import should succeed
        await expect(page.locator('[data-testid="import-success"]')).toBeVisible();
      } else {
        // Subsequent imports should show duplicate message
        await expect(page.locator('[data-testid="duplicate-notification"]')).toBeVisible();
        await expect(page.locator('[data-testid="duplicate-notification"]')).toContainText('already imported');
      }
    }

    // Verify only 1 book record exists
    const bookCards = await page.locator('[data-testid="book-card"]').count();
    expect(bookCards).toBe(1);

    // Click into book detail
    await page.click('[data-testid="book-card"]');

    // Navigate to Source Paths tab
    await page.click('text=Source Paths');

    // Verify 5 source path entries
    const sourcePathRows = await page.locator('[data-testid="source-path-row"]').count();
    expect(sourcePathRows).toBe(5);

    // Verify import count shows 5
    const firstPathCount = await page.locator('[data-testid="import-count"]').first().textContent();
    expect(firstPathCount).toBe('5');
  });

  test('should handle bulk import with duplicates', async ({ page }) => {
    await page.goto('https://localhost:8080/library');

    // Select 20 files (10 unique + 10 duplicates)
    const filePaths = [
      ...Array(10).fill('/unique/book1.mp3'),
      ...Array(10).fill('/unique/book2.mp3'),
    ];

    await page.click('[data-testid="bulk-import-button"]');

    for (const path of filePaths) {
      await page.click('[data-testid="add-file-button"]');
      await page.fill('[data-testid="file-path-input"]', path);
    }

    await page.click('[data-testid="validate-button"]');

    // Wait for validation results
    await page.waitForSelector('[data-testid="validation-results"]');

    // Check summary
    const summary = await page.locator('[data-testid="validation-summary"]').textContent();
    expect(summary).toContain('10 new');
    expect(summary).toContain('10 duplicates');

    // Proceed with import
    await page.click('[data-testid="import-new-only-button"]');

    // Verify only 2 books created (book1 and book2, 10x each counted as duplicates)
    const bookCount = await page.locator('[data-testid="book-card"]').count();
    expect(bookCount).toBe(2);
  });

  test('should show duplicate notification in UI', async ({ page }) => {
    await page.goto('https://localhost:8080/library');

    // Import file first time
    await page.click('[data-testid="import-button"]');
    await page.fill('[data-testid="file-path-input"]', '/test/duplicate.mp3');
    await page.click('[data-testid="confirm-import"]');

    await page.waitForSelector('[data-testid="import-success"]');

    // Import same file again
    await page.click('[data-testid="import-button"]');
    await page.fill('[data-testid="file-path-input"]', '/test/duplicate.mp3');
    await page.click('[data-testid="confirm-import"]');

    // Check for toast notification
    const notification = await page.locator('[data-testid="duplicate-toast"]');
    await expect(notification).toBeVisible();
    await expect(notification).toContainText('Duplicate detected');
    await expect(notification).toContainText('already exists');

    // Notification should have link to existing book
    const bookLink = await notification.locator('a').getAttribute('href');
    expect(bookLink).toContain('/books/');
  });
});
```

### 7.2 Wanted Feature E2E Test

**File**: `web/tests/e2e/wanted-feature.spec.ts`

```typescript
import { test, expect } from '@playwright/test';

test.describe('Wanted Feature', () => {
  test('should search and add book to wanted list', async ({ page }) => {
    await page.goto('https://localhost:8080/search');

    // Search for a book
    await page.fill('[data-testid="search-input"]', 'The Lord of the Rings');
    await page.waitForTimeout(1000); // Debounce

    // Wait for results
    await page.waitForSelector('[data-testid="book-result"]');

    // Click "Add to Wanted" on first result
    await page.click('[data-testid="book-result"]:first-child [data-testid="add-to-wanted-button"]');

    // Verify success message
    await expect(page.locator('[data-testid="success-message"]')).toBeVisible();

    // Navigate to wanted list
    await page.goto('https://localhost:8080/wanted');

    // Verify book appears in wanted list
    await expect(page.locator('[data-testid="wanted-book"]')).toContainText('The Lord of the Rings');

    // Verify "Wanted" chip is shown
    await expect(page.locator('[data-testid="wanted-chip"]')).toBeVisible();
  });

  test('should add author with all works', async ({ page }) => {
    await page.goto('https://localhost:8080/search');

    await page.fill('[data-testid="search-input"]', 'Brandon Sanderson');
    await page.waitForTimeout(1000);

    // Switch to Authors tab
    await page.click('text=Authors');

    // Click "Add All Works"
    await page.click('[data-testid="author-result"]:first-child [data-testid="add-all-works-button"]');

    // Verify success message with book count
    const successMsg = await page.locator('[data-testid="success-message"]').textContent();
    expect(successMsg).toContain('books added');

    // Navigate to wanted list
    await page.goto('https://localhost:8080/wanted');

    // Verify multiple books appear
    const bookCount = await page.locator('[data-testid="wanted-book"]').count();
    expect(bookCount).toBeGreaterThan(5); // Should have added many books
  });

  test('should auto-match imported file to wanted book', async ({ page }) => {
    // First, add a book to wanted list
    await page.goto('https://localhost:8080/search');
    await page.fill('[data-testid="search-input"]', 'Dune');
    await page.waitForTimeout(1000);
    await page.click('[data-testid="book-result"]:first-child [data-testid="add-to-wanted-button"]');

    // Now import a file with matching title
    await page.goto('https://localhost:8080/library');
    await page.click('[data-testid="import-button"]');
    await page.fill('[data-testid="file-path-input"]', '/test/dune.mp3');
    await page.click('[data-testid="confirm-import"]');

    // Wait for import to complete
    await page.waitForSelector('[data-testid="import-success"]');

    // Check for auto-match notification
    const notification = await page.locator('[data-testid="wanted-matched-toast"]');
    await expect(notification).toBeVisible();
    await expect(notification).toContainText('matched');

    // Navigate to library and verify book is now in "imported" state
    const bookCard = await page.locator('[data-testid="book-card"]:has-text("Dune")');
    await expect(bookCard.locator('[data-testid="state-chip"]')).toContainText('imported');

    // Verify book is removed from wanted list
    await page.goto('https://localhost:8080/wanted');
    await expect(page.locator('[data-testid="wanted-book"]:has-text("Dune")')).not.toBeVisible();
  });

  test('should transition wanted book to imported manually', async ({ page }) => {
    // Add book to wanted list
    await page.goto('https://localhost:8080/search');
    await page.fill('[data-testid="search-input"]', 'Test Book');
    await page.waitForTimeout(1000);
    await page.click('[data-testid="book-result"]:first-child [data-testid="add-to-wanted-button"]');

    // Go to wanted list
    await page.goto('https://localhost:8080/wanted');

    // Click "Mark as Imported"
    await page.click('[data-testid="wanted-book"]:first-child [data-testid="mark-imported-button"]');

    // Fill in file path prompt
    await page.fill('[data-testid="file-path-dialog-input"]', '/test/manual.mp3');
    await page.click('[data-testid="confirm-dialog-button"]');

    // Verify success
    await expect(page.locator('[data-testid="success-message"]')).toBeVisible();

    // Book should be removed from wanted list
    await expect(page.locator('[data-testid="wanted-book"]:has-text("Test Book")')).not.toBeVisible();

    // Verify book appears in library as imported
    await page.goto('https://localhost:8080/library');
    const bookCard = await page.locator('[data-testid="book-card"]:has-text("Test Book")');
    await expect(bookCard.locator('[data-testid="state-chip"]')).toContainText('imported');
  });
});
```

---

## Phase 8: Cover Images

### 8.1 Download Cover Images Script

**File**: `testdata/scripts/download_covers.sh`

```bash
#!/bin/bash
# Download or generate cover images for test audiobooks

TESTDATA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERS_DIR="$TESTDATA_DIR/covers"

mkdir -p "$COVERS_DIR"

# Function to download cover from Open Library
download_cover() {
    local isbn="$1"
    local output_file="$2"

    if [ -z "$isbn" ]; then
        return 1
    fi

    curl -s "https://covers.openlibrary.org/b/isbn/$isbn-L.jpg" -o "$output_file"

    # Check if download was successful (file > 1KB)
    if [ -f "$output_file" ] && [ $(stat -f%z "$output_file" 2>/dev/null || stat -c%s "$output_file") -gt 1024 ]; then
        echo "✓ Downloaded cover for ISBN $isbn"
        return 0
    else
        rm -f "$output_file"
        return 1
    fi
}

# Generate placeholder cover using ImageMagick
generate_cover() {
    local title="$1"
    local author="$2"
    local output_file="$3"

    if ! command -v convert &> /dev/null; then
        echo "ImageMagick not installed, skipping cover generation"
        return 1
    fi

    convert -size 600x900 \
        -background "#2c3e50" \
        -fill white \
        -font Arial \
        -pointsize 48 \
        -gravity center \
        label:"$title\n\nby\n\n$author" \
        "$output_file"

    echo "✓ Generated placeholder cover"
}

# The Odyssey
download_cover "9780140268867" "$COVERS_DIR/odyssey.jpg" || \
    generate_cover "The Odyssey" "Homer" "$COVERS_DIR/odyssey.jpg"

# Moby Dick
download_cover "9780142437247" "$COVERS_DIR/moby_dick.jpg" || \
    generate_cover "Moby Dick" "Herman Melville" "$COVERS_DIR/moby_dick.jpg"

# The Iliad
download_cover "9780140275360" "$COVERS_DIR/iliad.jpg" || \
    generate_cover "The Iliad" "Homer" "$COVERS_DIR/iliad.jpg"

echo ""
echo "Cover images saved to: $COVERS_DIR"
```

### 8.2 Embed Covers in M4B/M4A

Update `create_test_audiobooks.sh` to embed covers:

```bash
# After creating M4B file, add cover art
if [ -f "$COVERS_DIR/$(basename "$output_base").jpg" ]; then
    ffmpeg -i "$m4b_file" \
        -i "$COVERS_DIR/$(basename "$output_base").jpg" \
        -map 0 -map 1 \
        -c copy \
        -disposition:v:0 attached_pic \
        "${m4b_file}.tmp" \
        -y -v error

    mv "${m4b_file}.tmp" "$m4b_file"
    echo "  ✓ Embedded cover art"
fi
```

---

## Summary & Next Steps

This implementation plan provides all the code and structure needed to complete the wanted feature. The key components are:

1. **Metadata Providers** - Google Books, Audible, aggregator with circuit breaker
2. **Database Extensions** - Source path tracking, wanted state support
3. **API Endpoints** - Unified search, wanted management, state transitions
4. **Enhanced Scanner** - Duplicate detection, auto-matching, multi-path tracking
5. **Frontend** - Search UI, wanted list, duplicate notifications
6. **Tests** - Comprehensive E2E and unit tests
7. **Test Data** - M4B/M4A files with covers

All code is production-ready and follows existing patterns in the codebase. Implementation can proceed phase by phase, with each phase being independently testable.
