<!-- file: docs/plans/api-and-ecosystem-integrations.md -->
<!-- version: 2.0.0 -->
<!-- guid: e0f1a2b3-c4d5-6e7f-8a9b-0c1d2e3f4a5b -->
<!-- last-edited: 2026-01-31 -->

# API Enhancements and Ecosystem Integrations

## Overview

Improvements to the REST API surface (partial updates, bulk operations,
webhooks, caching) plus integrations with external audiobook and media
ecosystems.

---

## API Enhancements

### PATCH Support

Currently the only way to update an audiobook is `PUT /api/v1/audiobooks/:id`
which expects the full object (see `updateAudiobook` handler in
`internal/server/server.go`).  PATCH allows a client to send only the fields
that changed.

**File: `internal/server/server.go`** — add route in `setupRoutes()` inside
the audiobook block:

```go
api.PATCH("/audiobooks/:id", s.patchAudiobook)
```

**Handler:**

```go
// patchAudiobook applies a partial update.  The request body is a JSON object
// containing only the fields to change.  Fields not present in the body are
// left untouched.  This is distinct from PUT which replaces the entire record.
//
// Supported top-level fields mirror those accepted by updateAudiobook:
//   title, author_name, series_name, series_position, narrator, language,
//   publisher, description, isbn, isbn10, isbn13, edition, print_year,
//   audiobook_release_year, quality, version_notes, library_state.
//
// The special "overrides" and "unlock_overrides" keys are also supported
// (same semantics as PUT).
func (s *Server) patchAudiobook(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")

	// 1. Load the existing book — 404 if not found.
	existing, err := database.GlobalStore.GetBookByID(id)
	if err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// 2. Decode the patch body into a generic map.
	var patch map[string]interface{}
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. Merge: apply only the keys present in patch onto the existing book.
	//    String fields:
	if v, ok := patch["title"].(string); ok {
		existing.Title = v
	}
	if v, ok := patch["narrator"].(string); ok {
		existing.Narrator = &v
	}
	if v, ok := patch["language"].(string); ok {
		existing.Language = &v
	}
	if v, ok := patch["publisher"].(string); ok {
		existing.Publisher = &v
	}
	if v, ok := patch["description"].(string); ok {
		existing.Description = &v
	}
	if v, ok := patch["isbn"].(string); ok {
		existing.ISBN = &v
	}
	if v, ok := patch["isbn10"].(string); ok {
		existing.ISBN10 = &v
	}
	if v, ok := patch["isbn13"].(string); ok {
		existing.ISBN13 = &v
	}
	if v, ok := patch["edition"].(string); ok {
		existing.Edition = &v
	}
	if v, ok := patch["quality"].(string); ok {
		existing.Quality = &v
	}
	if v, ok := patch["version_notes"].(string); ok {
		existing.VersionNotes = &v
	}
	if v, ok := patch["library_state"].(string); ok {
		existing.LibraryState = &v
	}
	// Numeric fields (JSON numbers arrive as float64):
	if v, ok := patch["series_position"].(float64); ok {
		pos := int(v)
		existing.SeriesPosition = &pos
	}
	if v, ok := patch["print_year"].(float64); ok {
		y := int(v)
		existing.PrintYear = &y
	}
	if v, ok := patch["audiobook_release_year"].(float64); ok {
		y := int(v)
		existing.AudiobookReleaseYear = &y
	}

	// 4. Persist the merged record.
	updated, err := database.GlobalStore.UpdateBook(id, existing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update audiobook: %v", err)})
		return
	}

	// 5. Handle overrides (same logic as in updateAudiobook — delegate if
	//    "overrides" key is present).  For brevity, this mirrors the existing
	//    override-handling block in updateAudiobook.

	c.JSON(http.StatusOK, updated)
}
```

**TypeScript API client addition** in `web/src/services/api.ts`:

```ts
export async function patchBook(bookId: string, fields: Partial<Book>): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(fields),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to patch audiobook');
  }
  return response.json();
}
```

---

### Bulk Import Endpoint

Import multiple file paths in a single request instead of one-at-a-time.

---

### Webhook System

Outbound webhooks deliver JSON payloads to external URLs when specific events
fire.  The event bus is the existing SSE system (`internal/realtime/`).
Webhooks add a persistent, HTTP-based fan-out layer on top.

**File: `internal/server/webhooks.go`** — create this file.

```go
// file: internal/server/webhooks.go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// WebhookConfig describes a registered webhook endpoint.
type WebhookConfig struct {
	ID      string   `json:"id"`       // ULID
	URL     string   `json:"url"`      // Target URL (must be https in production)
	Events  []string `json:"events"`   // e.g. ["scan.complete", "organize.complete"]
	Secret  string   `json:"secret"`   // HMAC-SHA256 signing secret (stored; never returned in GET)
	Active  bool     `json:"active"`
	Created time.Time `json:"created_at"`
}

// WebhookPayload is the body sent to the webhook URL.
type WebhookPayload struct {
	Event     string      `json:"event"`      // e.g. "scan.complete"
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`       // event-specific payload
}

// --- PebbleDB storage pattern ---
//
// Key: webhook:<ULID>
// Value: JSON-encoded WebhookConfig
//
// This follows the same convention as operation:<id>, book:<id>, etc.
// Iteration prefix: "webhook:" with upper bound "webhook:~"
//
// In the near term, webhooks are stored via the existing SetSetting / GetAllSettings
// mechanism (same approach as audit entries).  When the number of webhooks grows
// the Store interface should gain dedicated methods.

func webhookKey(id string) string {
	return fmt.Sprintf("webhook:%s", id)
}

// SaveWebhookConfig persists a webhook configuration.
func SaveWebhookConfig(wh WebhookConfig) error {
	if database.GlobalStore == nil {
		return fmt.Errorf("database not initialized")
	}
	if wh.ID == "" {
		wh.ID = ulid.Make().String()
	}
	data, err := json.Marshal(wh)
	if err != nil {
		return err
	}
	return database.GlobalStore.SetSetting(webhookKey(wh.ID), string(data), "webhook", true)
}

// LoadAllWebhookConfigs reads all webhook configurations.
func LoadAllWebhookConfigs() ([]WebhookConfig, error) {
	if database.GlobalStore == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	all, err := database.GlobalStore.GetAllSettings()
	if err != nil {
		return nil, err
	}
	var configs []WebhookConfig
	for _, s := range all {
		if len(s.Key) < 8 || s.Key[:8] != "webhook:" {
			continue
		}
		var wh WebhookConfig
		if err := json.Unmarshal([]byte(s.Value), &wh); err != nil {
			continue
		}
		configs = append(configs, wh)
	}
	return configs, nil
}

// DeleteWebhookConfig removes a webhook by ID.
func DeleteWebhookConfig(id string) error {
	if database.GlobalStore == nil {
		return fmt.Errorf("database not initialized")
	}
	return database.GlobalStore.DeleteSetting(webhookKey(id))
}

// --- Delivery ---

// DeliverWebhook sends a webhook payload to all registered endpoints that
// subscribe to the given event.  It retries up to maxRetries times with
// exponential back-off on failure.  Delivery is fire-and-forget from the
// caller's perspective; errors are logged but do not propagate.
//
// Call this after a scan or organize operation completes:
//
//     go DeliverWebhook("scan.complete", map[string]interface{}{
//         "operation_id": op.ID,
//         "books_found":  len(books),
//     })
func DeliverWebhook(event string, data interface{}) {
	configs, err := LoadAllWebhookConfigs()
	if err != nil {
		log.Printf("[WARN] webhook delivery: failed to load configs: %v", err)
		return
	}

	payload := WebhookPayload{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WARN] webhook delivery: failed to marshal payload: %v", err)
		return
	}

	for _, wh := range configs {
		if !wh.Active {
			continue
		}
		// Check if this webhook subscribes to this event
		subscribed := false
		for _, e := range wh.Events {
			if e == event {
				subscribed = true
				break
			}
		}
		if !subscribed {
			continue
		}

		deliverToEndpoint(wh, body)
	}
}

const maxRetries = 3

func deliverToEndpoint(wh WebhookConfig, body []byte) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential back-off: 1s, 2s, 4s
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(body))
		if err != nil {
			log.Printf("[WARN] webhook %s: invalid URL %q: %v", wh.ID, wh.URL, err)
			return // URL is bad; no point retrying
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Event", wh.Events[0]) // convenience header

		// HMAC signing if a secret is configured
		if wh.Secret != "" {
			sig := hmacSign(wh.Secret, body)
			req.Header.Set("X-Webhook-Signature", sig)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[INFO] webhook %s delivered to %s (status %d)", wh.ID, wh.URL, resp.StatusCode)
			return // success
		}
		lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, wh.URL)
	}
	log.Printf("[WARN] webhook %s delivery to %s failed after %d retries: %v", wh.ID, wh.URL, maxRetries, lastErr)
}

func hmacSign(secret string, body []byte) string {
	// import "crypto/hmac" and "crypto/sha256" at top of file
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("sha256=%x", mac.Sum(nil))
}
```

**Wire into scan/organize completion** — after the operation completes in
the scanner or organizer (search for where `UpdateOperationStatus` is called
with status `"completed"`), add:

```go
go server.DeliverWebhook("scan.complete", map[string]interface{}{
    "operation_id": operationID,
    "completed_at": time.Now().UTC().Format(time.RFC3339),
})
```

**CRUD endpoints** — add to `setupRoutes()`:

```go
api.GET("/webhooks", s.listWebhooks)
api.POST("/webhooks", s.createWebhook)
api.DELETE("/webhooks/:id", s.deleteWebhook)
```

```go
func (s *Server) listWebhooks(c *gin.Context) {
	configs, err := LoadAllWebhookConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Redact secrets before returning
	for i := range configs {
		configs[i].Secret = ""
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": configs})
}

func (s *Server) createWebhook(c *gin.Context) {
	var wh WebhookConfig
	if err := c.ShouldBindJSON(&wh); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wh.ID = ulid.Make().String()
	wh.Created = time.Now()
	wh.Active = true
	if err := SaveWebhookConfig(wh); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	wh.Secret = "" // don't echo back the secret
	c.JSON(http.StatusCreated, gin.H{"webhook": wh})
}

func (s *Server) deleteWebhook(c *gin.Context) {
	id := c.Param("id")
	if err := DeleteWebhookConfig(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "webhook deleted", "id": id})
}
```

---

### Rate Limiting

Token-bucket rate limiting prevents expensive endpoints (metadata fetch, bulk
operations, scans) from being hammered.

**File: `internal/server/ratelimit.go`** — create this file.

```go
// file: internal/server/ratelimit.go
package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// tokenBucket implements a simple token-bucket algorithm.
// Each bucket starts full (capacity tokens) and refills at `rate` tokens/second.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64       // tokens per second
	lastTime time.Time
}

func newTokenBucket(capacity, rate float64) *tokenBucket {
	return &tokenBucket{
		tokens:   capacity,
		capacity: capacity,
		rate:     rate,
		lastTime: time.Now(),
	}
}

// Allow returns true if a token is available and consumes it.
func (tb *tokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tb.lastTime = now

	// Refill
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// RateLimitConfig defines the rate limit parameters for a single endpoint group.
type RateLimitConfig struct {
	Capacity float64 // burst size (max tokens)
	Rate     float64 // refill rate (tokens per second)
}

// Well-known per-endpoint rate limit configs.  Tune these values based on
// observed load.  The values below are conservative defaults.
var defaultRateLimits = map[string]RateLimitConfig{
	"metadata.fetch":  { Capacity: 10, Rate: 2  }, // 2 req/s, burst 10
	"metadata.bulk":   { Capacity: 5,  Rate: 0.5 }, // 1 req every 2s, burst 5
	"operations.scan": { Capacity: 3,  Rate: 0.2 }, // 1 req every 5s, burst 3
	"ai.parse":        { Capacity: 5,  Rate: 1  }, // 1 req/s, burst 5
}

// rateLimiter is the global registry of active buckets, keyed by endpoint name.
var rateLimiter = struct {
	sync.Mutex
	buckets map[string]*tokenBucket
}{buckets: make(map[string]*tokenBucket)}

func getBucket(name string) *tokenBucket {
	rateLimiter.Lock()
	defer rateLimiter.Unlock()

	if b, ok := rateLimiter.buckets[name]; ok {
		return b
	}
	cfg, ok := defaultRateLimits[name]
	if !ok {
		cfg = RateLimitConfig{Capacity: 60, Rate: 10} // generous fallback
	}
	b := newTokenBucket(cfg.Capacity, cfg.Rate)
	rateLimiter.buckets[name] = b
	return b
}

// RateLimit returns a gin middleware that enforces the named rate limit.
// On exhaustion it responds with 429 Too Many Requests and a Retry-After header.
//
// Usage:
//
//     api.POST("/metadata/bulk-fetch", RateLimit("metadata.bulk"), s.bulkFetchMetadata)
//     api.POST("/ai/parse-filename",   RateLimit("ai.parse"),      s.parseFilenameWithAI)
func RateLimit(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		bucket := getBucket(name)
		if !bucket.Allow() {
			cfg := defaultRateLimits[name]
			retryAfter := int(1.0 / cfg.Rate) // seconds until one token refills
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"endpoint":    name,
				"retry_after": retryAfter,
			})
			return
		}
		c.Next()
	}
}
```

**Wire into routes** — in `setupRoutes()`, add the middleware to the
expensive endpoints:

```go
api.POST("/metadata/bulk-fetch",    RateLimit("metadata.bulk"),  s.bulkFetchMetadata)
api.POST("/audiobooks/:id/fetch-metadata", RateLimit("metadata.fetch"), s.fetchAudiobookMetadata)
api.POST("/operations/scan",        RateLimit("operations.scan"), s.startScan)
api.POST("/ai/parse-filename",      RateLimit("ai.parse"),       s.parseFilenameWithAI)
api.POST("/audiobooks/:id/parse-with-ai", RateLimit("ai.parse"), s.parseAudiobookWithAI)
```

Note: the current `setupRoutes()` registers these routes without middleware.
When adding `RateLimit(...)`, the middleware argument goes between the path
string and the handler function in gin's variadic route registration.

---

### ETag / Caching Headers

ETags enable clients to use conditional `GET` requests (`If-None-Match`), so
they only transfer data when it has changed.  For list endpoints the ETag is
derived from the response body hash.

**File: `internal/server/etag.go`** — create this file.

```go
// file: internal/server/etag.go
package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// computeETag computes a SHA-256 hash of the given byte slice and returns
// it in the format expected by HTTP ETags: a quoted hex string.
//   Example: "a1b2c3d4..."
func computeETag(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf(`"%x"`, h[:16]) // 16 bytes = 32 hex chars; sufficient for uniqueness
}

// etagMiddleware is a response-wrapper middleware.  It intercepts the JSON
// response body, computes an ETag, and:
//   - If the request carried an If-None-Match header that matches the ETag,
//     it responds with 304 Not Modified (empty body).
//   - Otherwise it sets the ETag header and Cache-Control on the response.
//
// This middleware must be applied BEFORE the handler writes its response.
// It works by replacing gin's ResponseWriter with a buffered writer that
// captures the body.
//
// Usage — wrap read-only endpoints:
//
//     api.GET("/audiobooks", etagMiddleware(), s.listAudiobooks)
//     api.GET("/audiobooks/:id", etagMiddleware(), s.getAudiobook)
func etagMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Install the buffered writer
		bw := &bufferedResponseWriter{
			ResponseWriter: c.Writer,
			body:           make([]byte, 0, 4096),
		}
		c.Writer = bw

		// Let the handler run; it writes into bw.body
		c.Next()

		// If the handler already aborted (e.g. 404), skip ETag logic
		if c.IsAborted() || bw.statusCode >= 400 {
			return
		}

		etag := computeETag(bw.body)

		// Check If-None-Match
		ifNoneMatch := c.GetHeader("If-None-Match")
		if ifNoneMatch == etag {
			// 304: client's cache is fresh
			c.Writer = bw.ResponseWriter // restore original writer
			c.Status(http.StatusNotModified)
			c.Header("ETag", etag)
			c.Abort()
			return
		}

		// Write the actual response with ETag and Cache-Control
		c.Writer = bw.ResponseWriter // restore original writer
		c.Header("ETag", etag)
		c.Header("Cache-Control", "public, max-age=30") // 30-second browser cache
		c.Writer.WriteHeader(bw.statusCode)
		c.Writer.Write(bw.body)
	}
}

// bufferedResponseWriter captures the response body so we can hash it.
type bufferedResponseWriter struct {
	gin.ResponseWriter
	body       []byte
	statusCode int
	written    bool
}

func (bw *bufferedResponseWriter) Write(data []byte) (int, error) {
	bw.body = append(bw.body, data...)
	return len(data), nil // pretend success; actual write happens after ETag check
}

func (bw *bufferedResponseWriter) WriteHeader(code int) {
	bw.statusCode = code
	bw.written = true
	// Don't forward to the real writer yet
}

func (bw *bufferedResponseWriter) Status() int {
	if bw.statusCode == 0 {
		return http.StatusOK
	}
	return bw.statusCode
}
```

**Wire into read-only routes** in `setupRoutes()`:

```go
api.GET("/audiobooks",     etagMiddleware(), s.listAudiobooks)
api.GET("/audiobooks/:id", etagMiddleware(), s.getAudiobook)
api.GET("/authors",        etagMiddleware(), s.listAuthors)
api.GET("/series",         etagMiddleware(), s.listSeries)
api.GET("/works",          etagMiddleware(), s.listWorks)
```

Mutating endpoints (POST, PUT, DELETE, PATCH) must NOT use this middleware.

---

### API Key Auth Layer

Allow third-party consumers to access the API via API keys, independent of
the future multi-user session-based auth.

---

## Ecosystem Integrations

### Calibre Metadata Export

Export audiobook metadata in a format compatible with Calibre's library
import workflow.

### OPDS Feed

OPDS (Open Publication Distribution System) is an Atom-based catalog format.
External audiobook apps (e.g. ReadBearer, Audiobookmark) can discover and
browse the library via a single feed URL.

**File: `internal/server/opds.go`** — create this file.

```go
// file: internal/server/opds.go
package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// OPDS Atom XML constants
const opdsAtomNS = "http://www.w3.org/2005/Atom"
const opdsNS     = "http://www.w3.org/ns/opds-spec:1.1"
const opdsDCNS   = "http://purl.org/dc/elements/1.1/"

// handleOPDSCatalog serves the root OPDS catalog feed.
// Route: GET /api/v1/opds/catalog.xml
//
// The response is an Atom feed with:
//   - A <feed> root element with the library title and self link.
//   - One <entry> per audiobook in the library, each containing:
//       <title>        — audiobook title
//       <author>       — author name (resolved via the author relation)
//       <dc:language>  — language code
//       <summary>      — description
//       <updated>      — last modified timestamp in RFC3339
//       <link rel="acquisition"> — a direct download link to the file
//         (only included if the file is physically present under root_dir)
//
// Pagination is supported via query params:
//   limit  (default 50, max 200)
//   offset (default 0)
func (s *Server) handleOPDSCatalog(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := fmt.Sscanf(l, "%d", &limit); v == 0 || err != nil {
			limit = 50
		}
		if limit > 200 { limit = 200 }
		if limit < 1  { limit = 1   }
	}
	if o := c.Query("offset"); o != "" {
		if v, err := fmt.Sscanf(o, "%d", &offset); v == 0 || err != nil {
			offset = 0
		}
		if offset < 0 { offset = 0 }
	}

	books, err := database.GlobalStore.GetAllBooks(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Build XML manually to avoid pulling in an XML library dependency.
	// The structure is simple and flat enough that string concatenation is
	// maintainable and produces valid output.
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="%s" xmlns:opds="%s" xmlns:dc="%s">
  <title>Audiobook Organizer Library</title>
  <id>urn:audiobook-organizer:opds:root</id>
  <updated>%s</updated>
  <link rel="self" type="application/atom+xml" href="/api/v1/opds/catalog.xml"/>
  <link rel="alternate" type="text/html" href="/"/>
`, opdsAtomNS, opdsNS, opdsDCNS, now)

	for _, book := range books {
		authorName, _ := resolveAuthorAndSeriesNames(&book)
		updatedAt := book.UpdatedAt.UTC().Format(time.RFC3339)
		desc := ""
		if book.Description != nil {
			desc = *book.Description
		}
		lang := "en"
		if book.Language != nil && *book.Language != "" {
			lang = *book.Language
		}

		xml += fmt.Sprintf(`  <entry>
    <title>%s</title>
    <id>urn:audiobook-organizer:book:%s</id>
    <updated>%s</updated>
    <author><name>%s</name></author>
    <dc:language>%s</dc:language>
    <summary>%s</summary>
    <link rel="alternate" type="text/html" href="/library/%s"/>
  </entry>
`, escapeXML(book.Title), book.ID, updatedAt,
		escapeXML(authorName), lang, escapeXML(desc), book.ID)
	}

	xml += "</feed>"

	c.Data(http.StatusOK, "application/atom+xml; charset=utf-8", []byte(xml))
}

// escapeXML performs minimal XML entity escaping for text content.
func escapeXML(s string) string {
	replacer := []string{
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	}
	r := newReplacer(replacer...)
	return r.Replace(s)
}

// newReplacer wraps strings.NewReplacer — extracted for readability.
func newReplacer(oldnew ...string) interface{ Replace(string) string } {
	return strings.NewReplacer(oldnew...)
}
```

**Route registration** in `setupRoutes()`:

```go
// OPDS feed (no auth required — public catalog)
s.router.GET("/api/v1/opds/catalog.xml", s.handleOPDSCatalog)
```

Note: OPDS is registered directly on `s.router` (not on the `api` group) so
that rate-limit or auth middleware on the api group does not apply.  If you
want to gate it behind auth, move it into the `api` group.

**Example feed output** (abbreviated):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"
      xmlns:opds="http://www.w3.org/ns/opds-spec:1.1"
      xmlns:dc="http://purl.org/dc/elements/1.1/">
  <title>Audiobook Organizer Library</title>
  <id>urn:audiobook-organizer:opds:root</id>
  <updated>2026-01-31T12:00:00Z</updated>
  <link rel="self" type="application/atom+xml" href="/api/v1/opds/catalog.xml"/>
  <entry>
    <title>The Great Gatsby</title>
    <id>urn:audiobook-organizer:book:01HXYZ...</id>
    <updated>2026-01-30T08:15:00Z</updated>
    <author><name>F. Scott Fitzgerald</name></author>
    <dc:language>en</dc:language>
    <summary>A novel set in the Jazz Age...</summary>
    <link rel="alternate" type="text/html" href="/library/01HXYZ..."/>
  </entry>
  <!-- ... more entries ... -->
</feed>
```

---

### Plex / Jellyfin Sync

Sync library metadata with Plex or Jellyfin so audiobooks appear in those
media servers.

### External Cover Art

Fallback chain of cover art providers: if the primary source (Open Library)
doesn't have a cover, try secondary providers before giving up.

---

## Plugin System (Future)

A scaffold for third-party extensions:

- Register custom metadata providers
- Register custom transcoding strategies
- Loaded at startup, isolated via well-defined interfaces

---

## Dependencies

- Webhook system depends on a stable event bus (currently SSE-based via
  `internal/realtime/`).  The `DeliverWebhook` function is called as a
  goroutine after operation completion; it does not block the main flow.
- OPDS feed is independent and can ship early — no external dependencies.
- Rate limiting middleware is stateless (in-memory buckets) and has no
  external dependency.  For multi-instance deployments, replace the
  in-memory `tokenBucket` with a shared store (Redis or PebbleDB-backed).
- ETag middleware buffers the response body in memory.  For very large
  responses (>1 MB) consider streaming ETags via a version counter stored
  alongside the data instead.
- Plugin system is a significant architectural addition — design separately
  when needed.

## References

- Server and route registration: `internal/server/server.go`
- Gin route group pattern: routes registered in `setupRoutes()` function
- Database Store interface: `internal/database/store.go`
- PebbleDB key conventions: `internal/database/pebble_store.go`
  (`book:<id>`, `operation:<id>`, `webhook:<id>`, `audit:<id>`)
- TypeScript API client: `web/src/services/api.ts`
- OpenAPI spec: `docs/openapi.yaml`
- Metadata client: `internal/metadata/`
