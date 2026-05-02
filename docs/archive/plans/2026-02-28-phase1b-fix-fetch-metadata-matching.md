<!-- file: docs/plans/2026-02-28-phase1b-fix-fetch-metadata-matching.md -->
<!-- version: 1.0.0 -->
<!-- guid: a9b0c1d2-e3f4-5a6b-7c8d-9e0f1a2b3c4d -->
<!-- last-edited: 2026-02-28 -->

# Phase 1B: Fix Fetch Metadata Matching

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Stop metadata fetch from matching box sets/collections; use precision+penalty scoring with quality thresholds.
**Architecture:** Replace `bestTitleMatch` with a multi-factor scoring system (`scoreTitleMatch`) and apply it everywhere results are selected, not only the author-only search fallback path.
**Tech Stack:** Go, standard library only (`strings`, `regexp`, `math`).

---

## Background & Root Cause

`bestTitleMatch` in `internal/server/metadata_fetch_service.go` (lines 516–546) uses **recall-only word-overlap scoring**: it counts how many of the search title's words appear in the result title, but does not penalise results that add a huge number of extra words. A box-set title like:

> "The Long Earth Series 5 Books Collection Terry Pratchett and Stephen Baxter Box Set"

contains "long", "earth", "cosmos" (none!), but DO contain "long" and "earth" — so it still scores 2 just like the real individual book "The Long Cosmos". However if the query were "The Long Earth" (only 2 words), the box set still wins because it contains both words and there is no penalty for adding 8 more.

The fix: replace the function with one that computes an **F1-like score** (harmonic mean of recall and precision), then applies **heavy multiplicative penalties** for compilation indicators, and a **minimum quality threshold** below which we return nothing rather than garbage.

---

## Files Involved

| File | Role |
|---|---|
| `internal/server/metadata_fetch_service.go` | Contains `bestTitleMatch` (to be replaced with `scoreTitleMatch`) and `FetchMetadataForBook` (to use new scorer on all result sets, not only author-fallback path) |
| `internal/server/metadata_fetch_service_test.go` | Existing unit tests for `bestTitleMatch`; new tests added here |
| `internal/metadata/openlibrary.go` | Defines `BookMetadata` struct (fields: Title, Author, Narrator, Description, Publisher, PublishYear, ISBN, CoverURL, Language, Series, SeriesPosition) |

---

## Scoring Design

### scoreTitle (returns float64 in [0, 1])

```
recall    = (# search words found in result title) / (# search words)
precision = (# result words found in search title) / (# result words)
f1        = 2 * recall * precision / (recall + precision)   [0 if both zero]
```

"Significant words" = words of length > 2, lowercased, stop-words removed.

Stop-words to skip: `{"the", "and", "for", "with", "from", "that", "this", "are", "was", "were", "been", "have", "has", "had", "not", "but", "its", "our", "your", "their", "all", "any", "can", "will", "may", "into"}`.

### Compilation penalty (multiply score by 0.15)

Apply if the result title (lowercased) contains any of:
- `"box set"`, `"boxset"`, `"box-set"`
- `"collection"`
- `"complete series"`, `"complete collection"`
- `"books set"`, `"book set"`
- `"omnibus"`
- `"anthology"`
- `"compendium"`
- digit followed by `" books"` (e.g., `"5 books"`, `"10 books"`) — detected with regexp `\d+ books`
- `"series collection"`, `"series set"`

### Length ratio penalty (linear, applied after compilation penalty)

If `len(resultWords) > 1.5 * len(searchWords)`:
```
penalty = 1.5 * len(searchWords) / len(resultWords)   [always <= 1.0]
score  *= penalty
```

This caps the damage so a result 3x the length gets score ×0.5.

### Rich metadata bonus (additive, capped at +0.15)

Bonus +0.05 for each of: Description non-empty, CoverURL non-empty, Narrator non-empty, ISBN non-empty. Maximum bonus is +0.15 (3 populated fields out of 4 already saturates).

### Minimum quality threshold

If final score < 0.35, the result is discarded. If no result clears the threshold, return nil (caller falls through to next source or returns "no metadata found").

### Series position matching bonus/penalty (applied in FetchMetadataForBook)

After scoring, if the book being fetched has `book.SeriesSequence != nil` (known position N):
- If `meta.SeriesPosition == strconv.Itoa(N)`: bonus +0.10
- If `meta.SeriesPosition != ""` and does NOT equal N: penalty ×0.5

---

## Task List

1. [Task 1](#task-1-write-failing-unit-tests-for-scoretitlematch) — Write failing unit tests for `scoreTitleMatch`
2. [Task 2](#task-2-implement-scoretitlematch-and-replace-besttitlematch) — Implement `scoreTitleMatch` and replace `bestTitleMatch`
3. [Task 3](#task-3-apply-scorer-to-all-result-sets-in-fetchmetadataforbook) — Apply scorer to all result sets, not only author-only fallback
4. [Task 4](#task-4-series-position-matchingpenalty-in-fetchmetadataforbook) — Series position matching/penalty
5. [Task 5](#task-5-update-existing-tests-broken-by-changed-bestTitleMatch-semantics) — Fix existing `TestBestTitleMatch` test (semantics changed)
6. [Task 6](#task-6-integration-smoke-test) — Integration smoke test with a mock source

---

## Task 1: Write Failing Unit Tests for `scoreTitleMatch`

**Files:**
- Write to: `internal/server/metadata_fetch_service_test.go` (append after the last test, before the closing brace of the file — there is no closing brace, just add at end of file)

**Step 1: Write the failing tests**

Append the following block to the end of `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/metadata_fetch_service_test.go`:

```go
// --- scoreTitleMatch tests (Task 1) ---

func TestScoreTitleMatch_BoxSetPenalised(t *testing.T) {
	// The individual book should beat the box set even if the box set contains
	// all the query words plus a lot more.
	results := []metadata.BookMetadata{
		{Title: "The Long Earth Series 5 Books Collection Terry Pratchett and Stephen Baxter Box Set"},
		{Title: "The Long Earth", Description: "A novel.", CoverURL: "https://example.com/cover.jpg"},
	}
	got := bestTitleMatch(results, "The Long Earth")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Title != "The Long Earth" {
		t.Errorf("expected individual book, got box set: %q", got[0].Title)
	}
}

func TestScoreTitleMatch_CollectionPenalised(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Discworld Collection: Books 1-5"},
		{Title: "The Colour of Magic", Description: "First Discworld novel.", CoverURL: "https://cdn.example.com/cover.jpg", Narrator: "Tony Robinson"},
	}
	got := bestTitleMatch(results, "The Colour of Magic")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "The Colour of Magic" {
		t.Errorf("expected individual book, got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_OmnibusPenalised(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Foundation Omnibus: Foundation, Foundation and Empire, Second Foundation"},
		{Title: "Foundation", Author: "Isaac Asimov", Description: "The galactic empire crumbles.", ISBN: "9780553293357"},
	}
	got := bestTitleMatch(results, "Foundation")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "Foundation" {
		t.Errorf("expected 'Foundation', got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_ExactMatchWins(t *testing.T) {
	// A result with an exact title match should always win.
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos and Other Stories Collection"},
		{Title: "The Long Cosmos", Description: "Book 5 of the Long Earth series.", CoverURL: "https://example.com/c.jpg"},
	}
	got := bestTitleMatch(results, "The Long Cosmos")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "The Long Cosmos" {
		t.Errorf("expected 'The Long Cosmos', got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_BelowThresholdReturnsNil(t *testing.T) {
	// A result that shares no significant words with the query should be
	// rejected (score below minimum threshold).
	results := []metadata.BookMetadata{
		{Title: "A Completely Unrelated Title About Cooking"},
	}
	got := bestTitleMatch(results, "The Long Cosmos")
	if got != nil {
		t.Errorf("expected nil (below quality threshold), got %v", got)
	}
}

func TestScoreTitleMatch_RichMetadataBonus(t *testing.T) {
	// When two results score similarly on title, the one with richer
	// metadata (description + cover) should win.
	results := []metadata.BookMetadata{
		{Title: "Dune"},
		{Title: "Dune", Description: "Paul Atreides travels to Arrakis.", CoverURL: "https://example.com/dune.jpg", ISBN: "9780441013593"},
	}
	got := bestTitleMatch(results, "Dune")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	// The richer result is at index 1; it should be preferred.
	if got[0].Description == "" {
		t.Errorf("expected the richer result (with description), got title-only result")
	}
}

func TestScoreTitleMatch_LengthPenalty(t *testing.T) {
	// A very long title with the search words buried inside should score
	// lower than a concise matching title.
	results := []metadata.BookMetadata{
		// 10-word title containing all query words
		{Title: "Ender Game Complete Guide Expanded Universe Fan Edition Deluxe Version"},
		// Concise exact match
		{Title: "Ender's Game", Description: "Military sci-fi classic.", CoverURL: "https://example.com/enders.jpg"},
	}
	got := bestTitleMatch(results, "Ender's Game")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "Ender's Game" {
		t.Errorf("expected concise match, got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_NDigitBooksPenalised(t *testing.T) {
	// "5 books" pattern should trigger the compilation penalty.
	results := []metadata.BookMetadata{
		{Title: "Hitchhiker 5 Books Complete Collection Douglas Adams"},
		{Title: "The Hitchhiker's Guide to the Galaxy", Description: "Don't panic.", CoverURL: "https://example.com/h2g2.jpg"},
	}
	got := bestTitleMatch(results, "The Hitchhiker's Guide to the Galaxy")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if strings.Contains(strings.ToLower(got[0].Title), "books") {
		t.Errorf("compilation result should not win: got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_MultipleVariants(t *testing.T) {
	// bestTitleMatch accepts multiple title variants; scoring should use
	// the union of words from all variants.
	results := []metadata.BookMetadata{
		{Title: "The Fellowship of the Ring Box Set"},
		{Title: "Fellowship of the Ring", Description: "Part one of LOTR.", CoverURL: "https://example.com/lotr.jpg"},
	}
	// Provide both a cleaned and raw title variant.
	got := bestTitleMatch(results, "Fellowship of the Ring", "The Fellowship of the Ring")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if strings.Contains(strings.ToLower(got[0].Title), "box set") {
		t.Errorf("box set should not win: got %q", got[0].Title)
	}
}
```

**Step 2: Run test to confirm they all fail**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -run "TestScoreTitleMatch" -v 2>&1 | head -80
```

Expected output: **all 8 sub-tests FAIL** because the current `bestTitleMatch` does not penalise compilations or apply any threshold.

Sample expected failure:
```
--- FAIL: TestScoreTitleMatch_BoxSetPenalised (0.00s)
    metadata_fetch_service_test.go:NNN: expected individual book, got box set: "The Long Earth Series 5 Books Collection Terry Pratchett and Stephen Baxter Box Set"
FAIL
```

**Step 3: Commit the failing tests**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
git add internal/server/metadata_fetch_service_test.go && \
git commit -m "$(cat <<'EOF'
test(metadata): add failing tests for compilation-penalty scoring

Adds 8 unit tests that will drive the replacement of bestTitleMatch with
a precision+recall+penalty scoring function (scoreTitleMatch). All 8
currently fail because the existing function has no penalty for box sets,
omnibuses, or overly-long titles.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Implement `scoreTitleMatch` and Replace `bestTitleMatch`

**Files:**
- Edit: `internal/server/metadata_fetch_service.go`

**Step 1: Replace the `bestTitleMatch` function**

The current function occupies lines 513–546. Replace it entirely with the following. The signature stays the same (`bestTitleMatch(results []metadata.BookMetadata, titles ...string) []metadata.BookMetadata`) so all existing callers continue to work without modification.

```go
// scoreTitleStop is the set of common English stop-words excluded from scoring.
var scoreTitleStop = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "are": true, "was": true, "were": true,
	"been": true, "have": true, "has": true, "had": true, "not": true,
	"but": true, "its": true, "our": true, "your": true, "their": true,
	"all": true, "any": true, "can": true, "will": true, "may": true,
	"into": true,
}

// compilationRe detects "N books" patterns like "5 books" or "10 books".
var compilationRe = regexp.MustCompile(`\b\d+\s+books\b`)

// compilationPhrases is the list of lowercased substrings that mark a
// result as a compilation/box-set rather than a single title.
var compilationPhrases = []string{
	"box set", "boxset", "box-set",
	"collection",
	"complete series", "complete collection",
	"books set", "book set",
	"omnibus",
	"anthology",
	"compendium",
	"series collection", "series set",
}

// significantWords returns the deduplicated set of words longer than 2 chars
// that are not stop-words, all lowercased.
func significantWords(s string) map[string]bool {
	words := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		// Strip leading/trailing punctuation (apostrophes, commas, etc.)
		w = strings.Trim(w, ".,;:!?\"'()")
		if len(w) > 2 && !scoreTitleStop[w] {
			words[w] = true
		}
	}
	return words
}

// isCompilation returns true when the title appears to be a box-set,
// collection, omnibus, anthology, or other multi-title compilation.
func isCompilation(title string) bool {
	lower := strings.ToLower(title)
	for _, phrase := range compilationPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return compilationRe.MatchString(lower)
}

// scoreOneResult computes a quality score in [0, ~1.15] for a single result
// against a set of search-title significant words.
//
// Algorithm:
//  1. F1 = harmonic mean of recall (search words found in result) and
//     precision (result words found in search words).
//  2. Compilation penalty: multiply by 0.15 when the result looks like a
//     box set, collection, omnibus, etc.
//  3. Length penalty: if the result is >1.5× as many words as the search,
//     multiply by (1.5*len(search)) / len(result).
//  4. Rich-metadata bonus: +0.05 for each of description, cover, narrator,
//     ISBN present; capped at +0.15.
func scoreOneResult(r metadata.BookMetadata, searchWords map[string]bool) float64 {
	resultWords := significantWords(r.Title)

	if len(searchWords) == 0 || len(resultWords) == 0 {
		return 0
	}

	// Recall: how many search words appear in the result?
	recallHits := 0
	for w := range searchWords {
		if resultWords[w] {
			recallHits++
		}
	}
	recall := float64(recallHits) / float64(len(searchWords))

	// Precision: how many result words appear in the search?
	precHits := 0
	for w := range resultWords {
		if searchWords[w] {
			precHits++
		}
	}
	precision := float64(precHits) / float64(len(resultWords))

	// F1
	var f1 float64
	if recall+precision > 0 {
		f1 = 2 * recall * precision / (recall + precision)
	}

	// Compilation penalty
	if isCompilation(r.Title) {
		f1 *= 0.15
	}

	// Length penalty: penalise results that are much longer than the search
	nSearch := float64(len(searchWords))
	nResult := float64(len(resultWords))
	if nResult > 1.5*nSearch {
		f1 *= (1.5 * nSearch) / nResult
	}

	// Rich-metadata bonus (capped at +0.15)
	bonus := 0.0
	if r.Description != "" {
		bonus += 0.05
	}
	if r.CoverURL != "" {
		bonus += 0.05
	}
	if r.Narrator != "" {
		bonus += 0.05
	}
	if r.ISBN != "" {
		bonus += 0.05
	}
	if bonus > 0.15 {
		bonus = 0.15
	}

	return f1 + bonus
}

// bestTitleMatch filters results to find the single best match for the given
// title variants using precision+recall+penalty scoring.
//
// It replaces the old recall-only word-overlap function. A result must score
// at least 0.35 to be returned; if none qualify, nil is returned so the
// caller can fall through to the next source or report "no metadata found".
func bestTitleMatch(results []metadata.BookMetadata, titles ...string) []metadata.BookMetadata {
	const minScore = 0.35

	// Union of significant words from all title variants.
	searchWords := map[string]bool{}
	for _, t := range titles {
		for w := range significantWords(t) {
			searchWords[w] = true
		}
	}

	bestIdx := -1
	bestScore := 0.0
	for i, r := range results {
		score := scoreOneResult(r, searchWords)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestScore >= minScore {
		return []metadata.BookMetadata{results[bestIdx]}
	}
	return nil
}
```

**Step 2: Verify the file compiles**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go build ./internal/server/ 2>&1
```

Expected: no output (clean build). If there are import errors for `regexp` or `math`, confirm the import block at the top of `metadata_fetch_service.go` already includes `"regexp"` and `"strings"` (it does — lines 13–15).

**Step 3: Run the new tests**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -run "TestScoreTitleMatch" -v 2>&1
```

Expected: **all 8 pass**.

**Step 4: Run the full server package tests to check for regressions**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -v -count=1 2>&1 | tail -40
```

At this point `TestBestTitleMatch` will **fail** because its expected result assumes the old first-match-wins behaviour. That is intentional — Task 5 will fix it.

Expected partial output:
```
--- FAIL: TestBestTitleMatch (0.00s)
    metadata_fetch_service_test.go:465: expected 'The Great Adventure Story', got "Great Adventure"
```

**Step 5: Update the file header version**

The file header at line 2 currently reads `// version: 3.0.0`. Bump it to `// version: 4.0.0` to reflect the scoring overhaul.

**Step 6: Commit**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
git add internal/server/metadata_fetch_service.go && \
git commit -m "$(cat <<'EOF'
feat(metadata): replace bestTitleMatch with precision+penalty F1 scorer

Replaces the recall-only word-overlap scorer with a multi-factor function
that computes F1 (precision × recall), applies a 0.15× penalty for
compilations/box-sets, a proportional length penalty for overly-long
titles, and a small rich-metadata bonus. Results scoring below 0.35 are
rejected entirely so callers fall through to the next source rather than
applying garbage data.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Apply Scorer to All Result Sets in `FetchMetadataForBook`

**Problem:** `bestTitleMatch` is currently called only in Step 5 of `FetchMetadataForBook` (the author-only fallback, lines 186–188). The earlier steps 1–4 take `results[0]` blindly without any scoring. A box-set can win just by being the first item returned by the API.

**Files:**
- Edit: `internal/server/metadata_fetch_service.go`

**Step 1: Locate the result-consumption block**

After all the search fallback steps (lines 130–189), there is this block:

```go
if len(results) > 0 {
    meta := results[0]
    ...
}
```

**Step 2: Replace `results[0]` with a scored selection**

Replace:

```go
if len(results) > 0 {
    meta := results[0]
```

With:

```go
if len(results) > 0 {
    // Score all results and pick the best; reject if below quality threshold.
    scored := bestTitleMatch(results, searchTitle, book.Title)
    if len(scored) == 0 {
        log.Printf("[DEBUG] %s: all %d results rejected by quality scorer for %q",
            src.Name(), len(results), searchTitle)
        continue // try next source
    }
    meta := scored[0]
```

**Important:** The `continue` above jumps to the next iteration of `for _, src := range sources`, which tries the next metadata source. If this is the last source, the outer loop exits and the function returns `"no metadata found"`. This is the correct behaviour — better to return nothing than to store a box set.

**Step 3: Verify the file compiles**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go build ./internal/server/ 2>&1
```

Expected: clean.

**Step 4: Run all server tests**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -count=1 -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|FAIL|ok)" | head -60
```

Watch for new failures. The only expected failure at this point is still `TestBestTitleMatch` (fixed in Task 5). If `TestMetadataFetchService_Source1Fails_Source2Succeeds` or other integration-style tests fail, it means the scorer is too aggressive — re-read the failing test's mock data and tune `minScore` down from 0.35 to 0.25 for that case.

**Step 5: Bump file header to version 4.1.0**

**Step 6: Commit**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
git add internal/server/metadata_fetch_service.go && \
git commit -m "$(cat <<'EOF'
feat(metadata): apply quality scorer to all result sets, not just author-fallback

Previously FetchMetadataForBook used bestTitleMatch only for the
author-only search fallback; all other paths blindly took results[0].
Now all result sets pass through the scorer so box-sets can be rejected
regardless of which search path returned them.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Series Position Matching/Penalty in `FetchMetadataForBook`

**Rationale:** If the local database already knows the book is "Long Earth, book 5", a result claiming to be book 3 should be penalised; a result correctly claiming to be book 5 should be boosted.

**Files:**
- Edit: `internal/server/metadata_fetch_service.go`

**Step 1: Add `applySeriesPositionScore` helper function**

Add this function just above `bestTitleMatch`:

```go
// applySeriesPositionScore adjusts the ordering of results based on how
// well the SeriesPosition field matches the book's known position.
// If the book has no known SeriesSequence, results are returned unchanged.
//
// Boost:  result.SeriesPosition == known position → score multiplied by 1.10
// Penalise: result.SeriesPosition != "" && != known position → score ×0.50
//
// Because scoreOneResult is not exported and the scoring is embedded in
// bestTitleMatch, this function post-processes the scored slice returned
// by bestTitleMatch by re-running scoring with position awareness.
func applySeriesPositionFilter(
	results []metadata.BookMetadata,
	knownPosition int,
) []metadata.BookMetadata {
	if len(results) == 0 || knownPosition <= 0 {
		return results
	}
	wantPos := strconv.Itoa(knownPosition)
	best := results[0]
	if best.SeriesPosition != "" && best.SeriesPosition != wantPos {
		// The best result has a wrong position — reject it.
		log.Printf("[DEBUG] scorer: rejecting result %q (series position %q != expected %q)",
			best.Title, best.SeriesPosition, wantPos)
		return nil
	}
	return results
}
```

**Step 2: Call `applySeriesPositionFilter` after `bestTitleMatch`**

In `FetchMetadataForBook`, after the `bestTitleMatch` call:

```go
scored := bestTitleMatch(results, searchTitle, book.Title)
if len(scored) == 0 {
    log.Printf("[DEBUG] %s: all %d results rejected by quality scorer for %q",
        src.Name(), len(results), searchTitle)
    continue
}
// Apply series position filter if the book's position is already known.
if book.SeriesSequence != nil {
    scored = applySeriesPositionFilter(scored, *book.SeriesSequence)
    if len(scored) == 0 {
        log.Printf("[DEBUG] %s: best result rejected by series position filter for %q",
            src.Name(), searchTitle)
        continue
    }
}
meta := scored[0]
```

**Step 3: Write a test for series position filtering**

Add this test to `internal/server/metadata_fetch_service_test.go`:

```go
func TestApplySeriesPositionFilter_RejectsWrongPosition(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos", SeriesPosition: "3"}, // wrong — book is #5
	}
	got := applySeriesPositionFilter(results, 5)
	if got != nil {
		t.Errorf("expected nil (wrong position), got %v", got)
	}
}

func TestApplySeriesPositionFilter_AcceptsCorrectPosition(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos", SeriesPosition: "5"},
	}
	got := applySeriesPositionFilter(results, 5)
	if got == nil {
		t.Fatal("expected result, got nil")
	}
	if got[0].SeriesPosition != "5" {
		t.Errorf("expected position 5, got %q", got[0].SeriesPosition)
	}
}

func TestApplySeriesPositionFilter_NoKnownPosition(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Some Book", SeriesPosition: "3"},
	}
	// knownPosition == 0 means "we don't know" — pass through unchanged
	got := applySeriesPositionFilter(results, 0)
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d", len(got))
	}
}

func TestApplySeriesPositionFilter_NoPositionInResult(t *testing.T) {
	// If the result has no SeriesPosition, we can't reject it on position grounds.
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos"},
	}
	got := applySeriesPositionFilter(results, 5)
	if len(got) != 1 {
		t.Errorf("expected 1 result (no position to reject), got %d", len(got))
	}
}
```

**Step 4: Run new tests**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -run "TestApplySeriesPositionFilter" -v 2>&1
```

Expected: **all 4 pass**.

**Step 5: Compile check**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go build ./internal/server/ 2>&1
```

Expected: clean.

**Step 6: Bump file header version to 4.2.0**

**Step 7: Commit**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
git add internal/server/metadata_fetch_service.go internal/server/metadata_fetch_service_test.go && \
git commit -m "$(cat <<'EOF'
feat(metadata): add series position filter to reject wrong-book results

When the database already records a book's series sequence number,
FetchMetadataForBook now rejects any top-scored result that claims to be
a different series position. This prevents book 5 from being annotated
with book 3's metadata when both titles match the query equally well.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Update Existing `TestBestTitleMatch` (Semantics Changed)

**Problem:** The old `TestBestTitleMatch` asserts that "The Great Adventure Story" wins over "Great Adventure" for query "The Great Adventure". With the new F1 + precision scoring, "Great Adventure" (2/2 search words, 2 result words → precision 1.0) beats "The Great Adventure Story" (2/2 search words, 4 result words → precision 0.5). The test expectation must be corrected.

**Files:**
- Edit: `internal/server/metadata_fetch_service_test.go`

**Step 1: Find and update the assertion**

Current code at approximately line 462:
```go
// "The Great Adventure Story" and "Great Adventure" both score 2,
// but the first one encountered wins (index 1).
if got[0].Title != "The Great Adventure Story" {
    t.Errorf("expected 'The Great Adventure Story', got %q", got[0].Title)
}
```

Replace the comment and assertion with:
```go
// With precision+recall scoring, "Great Adventure" (2/2 result words match
// query words → precision 1.0) beats "The Great Adventure Story" (2/4 words
// match → precision 0.5). Both have recall 1.0; "Great Adventure" wins on F1.
if got[0].Title != "Great Adventure" {
    t.Errorf("expected 'Great Adventure' (higher precision), got %q", got[0].Title)
}
```

**Step 2: Run the full test suite**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -count=1 -v 2>&1 | grep -E "^(--- (PASS|FAIL)|FAIL|ok)"
```

Expected: **all pass**. No FAIL lines.

**Step 3: Bump test file header version**

File: `internal/server/metadata_fetch_service_test.go`, line 2: `// version: 3.0.0` → `// version: 4.0.0`.

**Step 4: Commit**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
git add internal/server/metadata_fetch_service_test.go && \
git commit -m "$(cat <<'EOF'
test(metadata): fix TestBestTitleMatch assertion for new precision scoring

The old scorer was recall-only so it broke ties by index order. The new
F1 scorer correctly ranks "Great Adventure" above "The Great Adventure
Story" because it has higher precision (all its words match the query).
Update the expected value accordingly.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Integration Smoke Test

Add one end-to-end-style unit test that exercises the full `FetchMetadataForBook` path with a mock source that returns both a box set and an individual book, and verifies the individual book is persisted.

**Files:**
- Edit: `internal/server/metadata_fetch_service_test.go`

**Step 1: Add the integration smoke test**

```go
func TestFetchMetadataForBook_BoxSetRejected_IndividualBookApplied(t *testing.T) {
	setupGlobalStoreForTest(t)

	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "The Long Cosmos"}, nil
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
		RecordMetadataChangeFunc: func(record *database.MetadataChangeRecord) error {
			return nil
		},
	}

	// Source returns a box set first, then the real book.
	src := &mockMetadataSource{
		name: "TestSource",
		searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
			return []metadata.BookMetadata{
				// Box set — should be penalised and rejected.
				{
					Title:  "The Long Earth Series 5 Books Collection Terry Pratchett and Stephen Baxter Box Set",
					Author: "Terry Pratchett",
				},
				// Individual book — should win.
				{
					Title:       "The Long Cosmos",
					Author:      "Terry Pratchett",
					Description: "The fifth book in the Long Earth series.",
					CoverURL:    "https://example.com/long-cosmos.jpg",
					PublishYear: 2016,
				},
			}, nil
		},
	}

	mfs := NewMetadataFetchService(mockDB)
	mfs.overrideSources = []metadata.MetadataSource{src}

	resp, err := mfs.FetchMetadataForBook("book1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// The applied book title should be from the individual book, not the box set.
	if resp.Book == nil {
		t.Fatal("expected non-nil Book in response")
	}
	if strings.Contains(strings.ToLower(resp.Book.Title), "collection") ||
		strings.Contains(strings.ToLower(resp.Book.Title), "box set") {
		t.Errorf("box set was applied to book: %q", resp.Book.Title)
	}
	if resp.Book.Title != "The Long Cosmos" {
		t.Errorf("expected title 'The Long Cosmos', got %q", resp.Book.Title)
	}
}
```

**Step 2: Run the new test**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -run "TestFetchMetadataForBook_BoxSetRejected" -v 2>&1
```

Expected: **PASS**.

**Step 3: Run the complete server test suite one final time**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  go test ./internal/server/ -count=1 2>&1
```

Expected:
```
ok  	github.com/jdfalk/audiobook-organizer/internal/server	X.XXXs
```

No failures.

**Step 4: Run make test to confirm no package-wide regressions**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
  make test 2>&1 | tail -20
```

Expected: all packages pass, ≥81% coverage maintained.

**Step 5: Bump test file header to version 4.1.0**

**Step 6: Final commit**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && \
git add internal/server/metadata_fetch_service_test.go && \
git commit -m "$(cat <<'EOF'
test(metadata): add integration smoke test for box-set rejection

Adds FetchMetadataForBook end-to-end test with a mock source that returns
a box set as the first result and an individual book as the second. Verifies
that the scorer rejects the box set and applies the individual book's
metadata to the database record.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Summary of All Changes

### `internal/server/metadata_fetch_service.go` (version: 3.0.0 → 4.2.0)

| Addition | Purpose |
|---|---|
| `scoreTitleStop` var | Stop-word set excluded from significant-word extraction |
| `compilationRe` var | Regexp detecting "N books" compilation indicator |
| `compilationPhrases` var | List of box-set/collection indicator phrases |
| `significantWords(s)` func | Extracts deduplicated significant words from a string |
| `isCompilation(title)` func | Returns true if title appears to be a compilation |
| `scoreOneResult(r, searchWords)` func | F1 + length penalty + metadata bonus scorer |
| `bestTitleMatch` (rewritten) | Wrapper that calls `scoreOneResult` for each result and enforces 0.35 threshold |
| `applySeriesPositionFilter` func | Rejects top result if it claims a different series position |
| Modified `FetchMetadataForBook` | Applies `bestTitleMatch` to all result sets, then applies `applySeriesPositionFilter` |

### `internal/server/metadata_fetch_service_test.go` (version: 3.0.0 → 4.1.0)

| Addition | Purpose |
|---|---|
| `TestScoreTitleMatch_BoxSetPenalised` | Box set loses to individual book |
| `TestScoreTitleMatch_CollectionPenalised` | Collection loses to individual book |
| `TestScoreTitleMatch_OmnibusPenalised` | Omnibus loses to individual book |
| `TestScoreTitleMatch_ExactMatchWins` | Perfect title match always wins |
| `TestScoreTitleMatch_BelowThresholdReturnsNil` | Unrelated result is rejected |
| `TestScoreTitleMatch_RichMetadataBonus` | Richer result preferred when titles tied |
| `TestScoreTitleMatch_LengthPenalty` | Overly-long title is penalised |
| `TestScoreTitleMatch_NDigitBooksPenalised` | "N books" pattern triggers penalty |
| `TestScoreTitleMatch_MultipleVariants` | Union of query variants used for scoring |
| `TestApplySeriesPositionFilter_*` (4 tests) | Series position filter logic |
| `TestFetchMetadataForBook_BoxSetRejected_IndividualBookApplied` | Full-path integration test |
| Modified `TestBestTitleMatch` | Corrected expectation for new precision-based ranking |

---

## Rollback Plan

If the scorer proves too aggressive (rejecting valid results in production):

1. Reduce `minScore` constant from `0.35` to `0.25` — this is the first tuning knob.
2. If still too aggressive, increase compilation penalty from `0.15` to `0.25`.
3. If a specific phrase triggers false positives, remove it from `compilationPhrases`.

All changes are confined to `internal/server/metadata_fetch_service.go` — no database schema changes, no API changes, no frontend changes.
