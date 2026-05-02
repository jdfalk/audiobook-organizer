<!-- file: docs/plans/2026-01-31-anthology-handling-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-01-31 -->

# Anthology Handling Design

## Overview

Anthologies are single audiobook files that contain multiple books from a series
(e.g., "The Complete Mistborn Series"). They present a unique challenge: one file
can replace several existing books in the library, and they may or may not have
their own ISBN.

This design covers detection, queueing, and resolution of anthologies.

**Current scope**: Deferred review queue — automatic detection feeds a queue that
the user processes on their own schedule. See [Future Work](#5-future-work) for
planned configurability (very low priority).

---

## 1. Data Model

Three new concepts layered onto the existing `Audiobook` model:

### AnthologyReview — the queue entry

| Field            | Type   | Description                                                                 |
| ---------------- | ------ | --------------------------------------------------------------------------- |
| id               | int    | Primary key                                                                 |
| audiobook_id     | int    | FK → the anthology file                                                     |
| status           | enum   | `pending_high_confidence`, `pending_needs_review`, `timed_out`, `resolved`  |
| resolution       | enum   | `contains`, `replaces`, `dismissed`, or `null` (unresolved)                 |
| detected_signals | JSON   | Which signals fired: `title_pattern`, `duration_ratio`, `isbn_match`        |
| created_at       | time   | When the review entry was created                                           |
| times_out_at     | time   | 60 days from creation — only set for `pending_needs_review` items           |
| resolved_at      | time   | When the user resolved it                                                   |

### AnthologyBookMapping — anthology ↔ individual book relationship

| Field        | Type   | Description                                                   |
| ------------ | ------ | ------------------------------------------------------------- |
| review_id    | int    | FK → AnthologyReview                                          |
| audiobook_id | int    | FK → the individual book this anthology contains              |
| match_source | enum   | `isbn`, `title_pattern`, `series_context`, `manual`           |
| confidence   | float  | 0.0–1.0 per-book match confidence                             |

### Audiobook model additions

| Field          | Type   | Description                                                              |
| -------------- | ------ | ------------------------------------------------------------------------ |
| superseded_by  | \*int  | Points to the anthology that replaced this book (`null` if not superseded) |
| is_anthology   | \*bool | Marks confirmed anthologies after resolution                            |

**Rationale for separate tables**: The existing `VersionGroupID` field is close
but conceptually wrong. Anthologies are not versions of the same book — they are
containers that hold multiple distinct books. This warrants its own schema.

### Go Structs

Add to `internal/database/store.go`:

```go
// AnthologyReview represents a pending or resolved anthology detection event.
type AnthologyReview struct {
    ID              int       `json:"id"`
    AudiobookID     string    `json:"audiobook_id"`      // ULID — FK → Book.ID
    Status          string    `json:"status"`            // pending_high_confidence | pending_needs_review | timed_out | resolved
    Resolution      *string   `json:"resolution"`        // contains | replaces | dismissed | nil
    DetectedSignals []string  `json:"detected_signals"` // ["title_pattern", "duration_ratio", "isbn_match"]
    CreatedAt       time.Time `json:"created_at"`
    TimesOutAt      *time.Time `json:"times_out_at"`     // set only for pending_needs_review
    ResolvedAt      *time.Time `json:"resolved_at"`
}

// AnthologyBookMapping links an anthology to an individual book it contains.
type AnthologyBookMapping struct {
    ReviewID    int     `json:"review_id"`     // FK → AnthologyReview.ID
    AudiobookID string  `json:"audiobook_id"`  // ULID — FK → Book.ID (the contained book)
    MatchSource string  `json:"match_source"`  // isbn | title_pattern | series_context | manual
    Confidence  float64 `json:"confidence"`    // 0.0–1.0
}
```

Add fields to the existing `Book` struct in `store.go`:

```go
// In the Book struct, after the existing lifecycle fields:
SupersededBy *string `json:"superseded_by,omitempty"` // ULID of anthology that replaced this book
IsAnthology   *bool   `json:"is_anthology,omitempty"`  // true if confirmed anthology
```

---

## 2. Detection Logic

Detection runs as a **post-processing step** in the existing scan/import
pipeline, after metadata extraction is complete. No separate scan pass is added.

### Signal evaluation

All matching signals are recorded in `detected_signals`. The first qualifying
signal determines which queue tier the item lands in:

1. **ISBN match** → `pending_high_confidence`
   - If the scanned file has an ISBN, check whether any known series contains
     books sharing the same publisher prefix.
   - Strongest signal. Goes straight to high-confidence queue.

2. **Title pattern match** → tier depends on series match
   - Regex against keywords: `Complete`, `Collection`, `Omnibus`, `Box Set`,
     `Books \d+-\d+`
   - Pattern matches a known series name → `pending_high_confidence`
   - Pattern match but no series match → `pending_needs_review`

3. **Duration threshold** → `pending_needs_review` (never high-confidence alone)
   - Only evaluated if the file is already associated with a known series
     (via title or author).
   - Triggers if the file's duration ≥ 2× the average duration of books in
     that series.
   - This signal alone never produces high confidence — it either reinforces
     another signal or lands in needs-review.

**No signal matches** = not flagged. Processed as a normal audiobook.

### Detection Go Code

Add this function to a new file `internal/anthology/detect.go`:

```go
package anthology

import (
    "regexp"
    "strings"

    "github.com/jdfalk/audiobook-organizer/internal/database"
)

var (
    // titlePatternRe matches anthology keywords in a title string.
    // Case-insensitive matching is done by lowercasing the input first.
    titlePatternRe = regexp.MustCompile(
        `(?i)(complete|collection|omnibus|box\s*set|books?\s*\d+\s*-\s*\d+|trilogy|quadrilogy|saga)`,
    )

    // isbnPublisherPrefixLen is the number of leading digits of an ISBN-13
    // used for publisher-prefix matching (the "group + publisher" segment).
    // ISBN-13 structure: 978-<group>-<publisher>-<title>-<check>
    // We use the first 7 digits (978 + 4-digit group+publisher) as the prefix.
    isbnPublisherPrefixLen = 7
)

// DetectionResult holds the outcome of anthology detection for one book.
type DetectionResult struct {
    IsAnthology      bool     // true if any signal fired
    Tier            string   // "pending_high_confidence" | "pending_needs_review" | ""
    DetectedSignals []string // which signals matched
    MatchedSeriesID *int     // series that matched (if any)
}

// DetectAnthology evaluates a single book against anthology signals.
// store is used to look up series, books, and ISBN data.
func DetectAnthology(book *database.Book, store database.Store) DetectionResult {
    result := DetectionResult{}
    var signals []string

    // --- Signal 1: ISBN publisher-prefix match ---
    if book.ISBN13 != nil && *book.ISBN13 != "" {
        isbn := strings.ReplaceAll(*book.ISBN13, "-", "")
        if len(isbn) >= isbnPublisherPrefixLen {
            prefix := isbn[:isbnPublisherPrefixLen]
            if matchedSeriesID := findSeriesByISBNPrefix(prefix, store); matchedSeriesID != nil {
                signals = append(signals, "isbn_match")
                result.MatchedSeriesID = matchedSeriesID
                result.Tier = "pending_high_confidence"
            }
        }
    }

    // --- Signal 2: Title pattern match ---
    if titlePatternRe.MatchString(book.Title) {
        signals = append(signals, "title_pattern")

        // Check if title also references a known series name
        if result.Tier == "" { // don't downgrade if ISBN already set high confidence
            seriesID := findSeriesInTitle(book.Title, store)
            if seriesID != nil {
                result.MatchedSeriesID = seriesID
                result.Tier = "pending_high_confidence"
            } else {
                result.Tier = "pending_needs_review"
            }
        }
    }

    // --- Signal 3: Duration threshold ---
    // Only evaluate if book has a known series context AND a duration.
    if book.Duration != nil && *book.Duration > 0 {
        seriesID := book.SeriesID
        if seriesID == nil && result.MatchedSeriesID != nil {
            seriesID = result.MatchedSeriesID
        }
        if seriesID != nil {
            avgDuration := computeSeriesAverageDuration(*seriesID, store)
            if avgDuration > 0 && *book.Duration >= 2*avgDuration {
                signals = append(signals, "duration_ratio")
                if result.Tier == "" {
                    result.Tier = "pending_needs_review"
                }
            }
        }
    }

    result.DetectedSignals = signals
    result.IsAnthology = len(signals) > 0
    return result
}

// findSeriesByISBNPrefix checks whether any book in any series shares the
// same ISBN publisher prefix as the given prefix string.
func findSeriesByISBNPrefix(prefix string, store database.Store) *int {
    allBooks, err := store.GetAllBooks(1_000_000, 0)
    if err != nil {
        return nil
    }
    for _, b := range allBooks {
        if b.ISBN13 == nil || *b.ISBN13 == "" || b.SeriesID == nil {
            continue
        }
        existingISBN := strings.ReplaceAll(*b.ISBN13, "-", "")
        if len(existingISBN) >= isbnPublisherPrefixLen && existingISBN[:isbnPublisherPrefixLen] == prefix {
            return b.SeriesID
        }
    }
    return nil
}

// findSeriesInTitle checks whether the book title contains any known series name.
// Returns the series ID if found.
func findSeriesInTitle(title string, store database.Store) *int {
    series, err := store.GetAllSeries()
    if err != nil {
        return nil
    }
    lowerTitle := strings.ToLower(title)
    for _, s := range series {
        if len(s.Name) < 3 {
            continue // skip very short series names to avoid false positives
        }
        if strings.Contains(lowerTitle, strings.ToLower(s.Name)) {
            id := s.ID
            return &id
        }
    }
    return nil
}

// computeSeriesAverageDuration computes the average duration (in seconds)
// of all books in a series. Returns 0 if no books have duration data.
func computeSeriesAverageDuration(seriesID int, store database.Store) int {
    books, err := store.GetBooksBySeriesID(seriesID)
    if err != nil {
        return 0
    }
    total := 0
    count := 0
    for _, b := range books {
        if b.Duration != nil && *b.Duration > 0 {
            total += *b.Duration
            count++
        }
    }
    if count == 0 {
        return 0
    }
    return total / count
}
```

### Hook into Post-Scan Pipeline

In `internal/scanner/scanner.go`, at the end of each book's processing
(after `saveBook` succeeds), add:

```go
// After saveBook(&books[idx]) succeeds:
if detection := anthology.DetectAnthology(&dbBook, database.GlobalStore); detection.IsAnthology {
    log.Printf("[INFO] scanner: anthology signal(s) detected for %s: %v", books[idx].Title, detection.DetectedSignals)
    // Create the review entry — implemented via store method (see migration below)
    review := &database.AnthologyReview{
        AudiobookID:     dbBook.ID,
        Status:          detection.Tier,
        DetectedSignals: detection.DetectedSignals,
        CreatedAt:       time.Now(),
    }
    if detection.Tier == "pending_needs_review" {
        timeout := time.Now().Add(60 * 24 * time.Hour) // 60 days
        review.TimesOutAt = &timeout
    }
    // store.CreateAnthologyReview(review) — new Store method
}
```

### Confidence Scoring Algorithm

Per-book match confidence is computed independently from the anthology-level
tier. Each mapping source contributes a base score, adjusted by corroborating
evidence:

```
Algorithm: ComputeBookMatchConfidence(anthology, candidateBook, matchSource)

base_scores = {
    "isbn":            0.95,   // ISBN prefix match is very strong
    "title_pattern":   0.60,   // title keyword is moderate
    "series_context":  0.50,   // same series but no direct match
    "manual":          1.00,   // user explicitly selected
}

score = base_scores[matchSource]

// Corroborating signals raise confidence:
if anthology.SeriesID == candidateBook.SeriesID && anthology.SeriesID != nil:
    score += 0.15   // same series

if anthology.AuthorID == candidateBook.AuthorID && anthology.AuthorID != nil:
    score += 0.10   // same author

if candidateBook.ISBN13 != nil && anthology.ISBN13 != nil:
    // shared ISBN publisher prefix
    if samePublisherPrefix(anthology.ISBN13, candidateBook.ISBN13):
        score += 0.10

// Clamp to [0.0, 1.0]
score = min(score, 1.0)
score = max(score, 0.0)

return score
```

Go implementation:

```go
// ComputeBookMatchConfidence returns a 0.0–1.0 confidence score for a
// candidate book being contained in an anthology.
func ComputeBookMatchConfidence(anthology, candidate *database.Book, matchSource string) float64 {
    baseScores := map[string]float64{
        "isbn":           0.95,
        "title_pattern":  0.60,
        "series_context": 0.50,
        "manual":         1.00,
    }

    score, ok := baseScores[matchSource]
    if !ok {
        score = 0.40 // unknown source — conservative default
    }

    // Same series bonus
    if anthology.SeriesID != nil && candidate.SeriesID != nil && *anthology.SeriesID == *candidate.SeriesID {
        score += 0.15
    }
    // Same author bonus
    if anthology.AuthorID != nil && candidate.AuthorID != nil && *anthology.AuthorID == *candidate.AuthorID {
        score += 0.10
    }
    // Shared ISBN publisher prefix bonus
    if anthology.ISBN13 != nil && candidate.ISBN13 != nil {
        anthISBN := strings.ReplaceAll(*anthology.ISBN13, "-", "")
        candISBN := strings.ReplaceAll(*candidate.ISBN13, "-", "")
        if len(anthISBN) >= 7 && len(candISBN) >= 7 && anthISBN[:7] == candISBN[:7] {
            score += 0.10
        }
    }

    // Clamp
    if score > 1.0 { score = 1.0 }
    if score < 0.0 { score = 0.0 }
    return score
}
```

---

## 3. Review Queue

Surfaced in the UI as a dedicated section with a badge count indicating pending
items. Three views:

### View 1 — "Matches Found" (`pending_high_confidence`)

- Shows the anthology alongside the individual books it likely contains.
- Each matched book displays its confidence score and match source.
- **Actions**: Contains · Replaces · Dismiss · Re-match
- **No timeout** — these stay until the user reviews them.

### View 2 — "Needs Review" (`pending_needs_review`)

- Low-confidence detections or cases where auto-matching failed.
- Shows which signals triggered the flag so the user understands why it's here.
- **Actions**: Contains · Replaces · Dismiss · Manual Match
- **60-day timeout** — a background job (or check-on-read) transitions these to
  `timed_out` when the deadline passes.

### View 3 — "Timed Out / Failed" (`timed_out`)

- Everything that aged out of View 2.
- Sortable by timeout date so items can be batch-processed efficiently.
- **Actions**: Restart · Dismiss
  - **Restart** moves the item back to `pending_needs_review` with a fresh
    60-day window.
  - **Dismiss** permanently closes it.

### Resolution action effects

| Action                  | Effect                                                                                                                                      |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| **Contains**            | Links anthology → individual books via AnthologyBookMapping. Both the anthology and individual books remain visible. Sets `is_anthology = true` on the anthology. |
| **Replaces**            | Same as Contains, plus sets `superseded_by` on each linked individual book. Superseded books are hidden by default in the main library view but preserved in the database. |
| **Dismiss**             | Resolves the review with no relationship created. The file is treated as a normal audiobook going forward.                                 |
| **Re-match / Manual Match** | Opens a manual matcher to search and select which books are actually contained in this anthology. Overwrites any auto-detected mappings. |
| **Restart** (View 3 only) | Resets the item to `pending_needs_review` with a fresh 60-day `times_out_at`.                                                           |

### Review Queue API Endpoints

Register in `internal/server/server.go` under the existing `api` group:

```go
// In setupRoutes(), inside the api group:
reviewGroup := api.Group("/anthology-reviews")
{
    reviewGroup.GET("",          s.listAnthologyReviews)   // GET /api/v1/anthology-reviews?status=pending_needs_review
    reviewGroup.GET("/:id",      s.getAnthologyReview)     // GET /api/v1/anthology-reviews/:id
    reviewGroup.PUT("/:id",      s.resolveAnthologyReview) // PUT /api/v1/anthology-reviews/:id  { resolution, mappings }
    reviewGroup.POST("/:id/restart", s.restartAnthologyReview) // POST /api/v1/anthology-reviews/:id/restart
}
```

Handler signatures and implementation skeleton:

```go
// listAnthologyReviews returns reviews filtered by status.
// Query params: status (default: all pending), limit, offset
func (s *Server) listAnthologyReviews(c *gin.Context) {
    status := c.DefaultQuery("status", "")
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
    offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

    reviews, err := database.GlobalStore.ListAnthologyReviews(status, limit, offset)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"reviews": reviews, "count": len(reviews)})
}

// getAnthologyReview returns a single review with its mappings and the
// anthology book record populated.
func (s *Server) getAnthologyReview(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
        return
    }

    review, err := database.GlobalStore.GetAnthologyReview(id)
    if err != nil || review == nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
        return
    }

    mappings, _ := database.GlobalStore.GetAnthologyMappings(id)
    anthology, _ := database.GlobalStore.GetBookByID(review.AudiobookID)

    c.JSON(http.StatusOK, gin.H{
        "review":    review,
        "mappings":  mappings,
        "anthology": anthology,
    })
}

// resolveAnthologyReview applies a user resolution to a review.
// Body: { "resolution": "contains"|"replaces"|"dismissed",
//          "mappings": [{ "audiobook_id": "...", "match_source": "manual", "confidence": 1.0 }] }
func (s *Server) resolveAnthologyReview(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
        return
    }

    var body struct {
        Resolution string `json:"resolution" binding:"required,oneof=contains replaces dismissed"`
        Mappings   []struct {
            AudiobookID string  `json:"audiobook_id"`
            MatchSource string  `json:"match_source"`
            Confidence  float64 `json:"confidence"`
        } `json:"mappings"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    now := time.Now()

    // Update review status
    review, _ := database.GlobalStore.GetAnthologyReview(id)
    if review == nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
        return
    }
    review.Status = "resolved"
    review.Resolution = &body.Resolution
    review.ResolvedAt = &now
    _ = database.GlobalStore.UpdateAnthologyReview(review)

    // Write mappings
    for _, m := range body.Mappings {
        mapping := &database.AnthologyBookMapping{
            ReviewID:    id,
            AudiobookID: m.AudiobookID,
            MatchSource: m.MatchSource,
            Confidence:  m.Confidence,
        }
        _ = database.GlobalStore.CreateAnthologyMapping(mapping)
    }

    // Side effects based on resolution
    anthology, _ := database.GlobalStore.GetBookByID(review.AudiobookID)
    if anthology != nil {
        if body.Resolution == "contains" || body.Resolution == "replaces" {
            t := true
            anthology.IsAnthology = &t
            _ = database.GlobalStore.UpdateBook(anthology.ID, anthology)
        }
        if body.Resolution == "replaces" {
            // Mark each mapped book as superseded
            for _, m := range body.Mappings {
                contained, _ := database.GlobalStore.GetBookByID(m.AudiobookID)
                if contained != nil {
                    contained.SupersededBy = &anthology.ID
                    _ = database.GlobalStore.UpdateBook(contained.ID, contained)
                }
            }
        }
    }

    c.JSON(http.StatusOK, gin.H{"status": "resolved", "resolution": body.Resolution})
}

// restartAnthologyReview resets a timed_out review back to pending_needs_review.
func (s *Server) restartAnthologyReview(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
        return
    }

    review, _ := database.GlobalStore.GetAnthologyReview(id)
    if review == nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
        return
    }
    if review.Status != "timed_out" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "can only restart timed_out reviews"})
        return
    }

    review.Status = "pending_needs_review"
    timeout := time.Now().Add(60 * 24 * time.Hour)
    review.TimesOutAt = &timeout
    _ = database.GlobalStore.UpdateAnthologyReview(review)

    c.JSON(http.StatusOK, gin.H{"status": "pending_needs_review", "times_out_at": review.TimesOutAt})
}
```

### The 60-Day Timeout Job

Two implementation strategies. The project already uses a ticker-based
background loop in `server.go` (the soft-delete purge loop, around line 633).
Use the same pattern:

#### Option A: Background Ticker (preferred — matches existing pattern)

In `internal/server/server.go`, inside the `Start()` method where the
soft-delete purge ticker already runs, add a second ticker:

```go
// Alongside the existing purgeTicker:
anthologyTimeoutTicker := time.NewTicker(6 * time.Hour) // check every 6 hours
defer anthologyTimeoutTicker.Stop()

go func() {
    for {
        select {
        case <-ctx.Done():
            return
        case <-anthologyTimeoutTicker.C:
            if database.GlobalStore == nil {
                continue
            }
            // Find all pending_needs_review entries past their times_out_at
            reviews, err := database.GlobalStore.ListAnthologyReviews("pending_needs_review", 10000, 0)
            if err != nil {
                log.Printf("[WARN] anthology timeout check failed: %v", err)
                continue
            }
            now := time.Now()
            timedOut := 0
            for _, r := range reviews {
                if r.TimesOutAt != nil && now.After(*r.TimesOutAt) {
                    r.Status = "timed_out"
                    if err := database.GlobalStore.UpdateAnthologyReview(&r); err != nil {
                        log.Printf("[WARN] failed to time out review %d: %v", r.ID, err)
                        continue
                    }
                    timedOut++
                }
            }
            if timedOut > 0 {
                log.Printf("[INFO] anthology timeout: %d reviews timed out", timedOut)
            }
        }
    }
}()
```

#### Option B: Check-on-Read (simpler, no background goroutine)

In the `listAnthologyReviews` and `getAnthologyReview` handlers, before
returning data, check and transition any expired reviews inline:

```go
func (s *Server) checkAndTransitionExpiredReviews() {
    if database.GlobalStore == nil { return }
    now := time.Now()
    reviews, err := database.GlobalStore.ListAnthologyReviews("pending_needs_review", 10000, 0)
    if err != nil { return }
    for _, r := range reviews {
        if r.TimesOutAt != nil && now.After(*r.TimesOutAt) {
            r.Status = "timed_out"
            _ = database.GlobalStore.UpdateAnthologyReview(&r)
        }
    }
}

// Call at the top of listAnthologyReviews:
func (s *Server) listAnthologyReviews(c *gin.Context) {
    s.checkAndTransitionExpiredReviews()
    // ... rest of handler
}
```

Option B is simpler but means timeouts are only applied when someone visits the
review queue. Option A ensures the badge count stays accurate even when no one
is looking. **Recommendation: use Option A** (ticker), matching the project's
existing background-job pattern.

---

## 4. Manual Matcher & Superseded Visibility

### Manual matcher scope

When the user opens the manual matcher (via Re-match or Manual Match), it
defaults to searching within the same author or series as the anthology. A
control is available to expand the search to the full library — this handles the
case where the anthology genuinely crosses series or authors (e.g., a "Best of"
collection across an author's unrelated series).

### Superseded book visibility

A single "Show superseded" toggle controls visibility of superseded books
everywhere simultaneously — both in the main library list and in search results.
When off (default), superseded books are completely hidden. When on, they appear
inline alongside non-superseded books, visually distinguished (e.g., a badge or
muted style indicating they have been superseded and which anthology replaced
them).

---

## 5. Future Work

> **Very low priority — document only, do not implement now.**

### Configurable queue behavior

The current system hardcodes the default behavior to **deferred review queue**
(detect → queue → user reviews on their own schedule). In the future, this
default should be user-configurable in Settings:

| Option              | Behavior                                                                  |
| ------------------- | ------------------------------------------------------------------------- |
| **Auto-flag**       | Mark as potential anthology with a badge. No other action.                |
| **Prompt on import**| Show a dialog during import asking what to do with the detected anthology.|
| **Review queue**    | Current behavior — deferred batch review. Remains the default option.     |

Per-anthology overrides should be available regardless of the global setting.

See `TODO.md` for tracking.

---

## 6. PebbleDB Key Schema (proposed additions)

```
anthology_review:<id>                          → AnthologyReview JSON
anthology_review:audiobook:<audiobook_id>      → review_id (index)
anthology_review:status:<status>:<id>          → review_id (secondary index)
anthology_mapping:<review_id>:<audiobook_id>   → AnthologyBookMapping JSON
counter:anthology_review                       → next review ID
```

### Migration Code — PebbleDB Key Writes

Migration 015 (or whichever version follows the junction-table migration).
For PebbleDB, no structural schema change is needed — the keys above are
simply available for use once the Store methods are implemented. The migration
registers the counter and verifies the keyspace is ready:

```go
// migration015Up initializes anthology review keyspace.
func migration015Up(store Store) error {
    log.Println("  - Initializing anthology review keyspace")

    pebbleStore, ok := store.(*PebbleStore)
    if !ok {
        // SQLite: create the tables
        sqliteStore, ok2 := store.(*SQLiteStore)
        if !ok2 {
            log.Println("  - Unknown store type; skipping migration015")
            return nil
        }
        statements := []string{
            `CREATE TABLE IF NOT EXISTS anthology_reviews (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                audiobook_id TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'pending_needs_review',
                resolution TEXT,
                detected_signals TEXT,
                created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                times_out_at DATETIME,
                resolved_at DATETIME,
                FOREIGN KEY (audiobook_id) REFERENCES books(id)
            )`,
            `CREATE TABLE IF NOT EXISTS anthology_mappings (
                review_id INTEGER NOT NULL,
                audiobook_id TEXT NOT NULL,
                match_source TEXT NOT NULL,
                confidence REAL NOT NULL DEFAULT 0.0,
                PRIMARY KEY (review_id, audiobook_id),
                FOREIGN KEY (review_id) REFERENCES anthology_reviews(id) ON DELETE CASCADE,
                FOREIGN KEY (audiobook_id) REFERENCES books(id)
            )`,
            "CREATE INDEX IF NOT EXISTS idx_anthology_reviews_audiobook ON anthology_reviews(audiobook_id)",
            "CREATE INDEX IF NOT EXISTS idx_anthology_reviews_status ON anthology_reviews(status)",
            "CREATE INDEX IF NOT EXISTS idx_anthology_mappings_review ON anthology_mappings(review_id)",
        }
        for _, stmt := range statements {
            if _, err := sqliteStore.db.Exec(stmt); err != nil {
                return fmt.Errorf("migration015 SQLite: %s: %w", stmt, err)
            }
        }
        // Add superseded_by and is_anthology to books
        for _, stmt := range []string{
            "ALTER TABLE books ADD COLUMN superseded_by TEXT",
            "ALTER TABLE books ADD COLUMN is_anthology BOOLEAN",
        } {
            if _, err := sqliteStore.db.Exec(stmt); err != nil {
                if !strings.Contains(err.Error(), "duplicate column name") {
                    return fmt.Errorf("migration015 SQLite: %s: %w", stmt, err)
                }
            }
        }
        return nil
    }

    // PebbleDB: initialize the counter
    counterKey := []byte("counter:anthology_review")
    if _, closer, err := pebbleStore.db.Get(counterKey); err == pebble.ErrNotFound {
        if err := pebbleStore.db.Set(counterKey, []byte("1"), pebble.Sync); err != nil {
            return fmt.Errorf("failed to initialize anthology_review counter: %w", err)
        }
    } else if err == nil {
        closer.Close() // already exists
    } else {
        return fmt.Errorf("failed to check anthology_review counter: %w", err)
    }

    log.Println("  - Anthology review keyspace initialized")
    return nil
}
```

### Store Interface Additions

Add to the `Store` interface in `internal/database/store.go`:

```go
// Anthology reviews
ListAnthologyReviews(status string, limit, offset int) ([]AnthologyReview, error)
GetAnthologyReview(id int) (*AnthologyReview, error)
CreateAnthologyReview(review *AnthologyReview) (*AnthologyReview, error)
UpdateAnthologyReview(review *AnthologyReview) error

// Anthology mappings
GetAnthologyMappings(reviewID int) ([]AnthologyBookMapping, error)
CreateAnthologyMapping(mapping *AnthologyBookMapping) error
```

---

## References

- Audiobook model: `internal/database/store.go` (Book struct)
- PebbleDB key schema: `docs/database-pebble-schema.md`
- Database migration summary: `docs/database-migration-summary.md`
- Scanner pipeline: `internal/scanner/scanner.go` → `ProcessBooksParallel`
- Operation queue pattern: `internal/operations/queue.go`
- Background ticker pattern: `internal/server/server.go` (soft-delete purge loop)
- TODO tracking: `TODO.md` → vNEXT → Anthology Handling
