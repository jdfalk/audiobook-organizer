# Series Name Normalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strip title/position contamination from series names at every ingest point and provide a one-shot remediation endpoint that renames bad rows, merges duplicates, then runs write-back + organize for affected books.

**Architecture:** A pure `StripSeriesContamination` function in `internal/metadata/series_normalize.go` is called at the three key ingest sites (metafetch, scanner, iTunes importer). A new `POST /api/v1/series/normalize` endpoint (modeled on the existing `series-prune` pattern) runs a full async remediation pass. The operation is also registered as a manual scheduler task so it can be triggered from the Maintenance tab.

**Tech Stack:** Go stdlib (`strings`, `regexp`), PebbleDB via existing store interface, `organizer.Service.ReOrganizeInPlace`, `WriteBackBatcher.Enqueue`, gin HTTP framework.

**Worktree:** `.worktrees/series-normalize` on branch `feat/series-name-normalization`

---

## File Map

| File | Action | What changes |
|------|--------|--------------|
| `internal/metadata/series_normalize.go` | Create | `StripSeriesContamination` function |
| `internal/metadata/series_normalize_test.go` | Create | Unit tests for all rules and edge cases |
| `internal/metafetch/service.go` | Modify | Extend `NormalizeMetaSeries` to call `StripSeriesContamination` |
| `internal/scanner/scanner.go` | Modify | Normalize series name in `resolveSeriesID` before store calls |
| `internal/itunes/service/importer.go` | Modify | Normalize series name in `ensureSeriesID` before store calls |
| `internal/server/duplicates_handlers.go` | Modify | Add `seriesNormalizePreview`, `seriesNormalize`, `executeSeriesNormalizeCore` |
| `internal/server/server.go` | Modify | Register routes + add `series-normalize` to interrupt-recovery list |
| `internal/server/scheduler.go` | Modify | Register `series_normalize` task definition |

---

## Task 1: `StripSeriesContamination` pure function

**Files:**
- Create: `internal/metadata/series_normalize.go`
- Create: `internal/metadata/series_normalize_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// file: internal/metadata/series_normalize_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package metadata

import (
	"testing"
)

func TestStripSeriesContamination(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		title          string
		wantSeries     string
		wantPosition   string
		wantFlagReview bool
	}{
		// Rule 1: dash-embedded position+title
		{
			name:         "dash embedded position and title",
			input:        "The Long Earth - 1 - The Long Earth",
			wantSeries:   "The Long Earth",
			wantPosition: "1",
		},
		{
			name:         "dash embedded with different title",
			input:        "My Long Series - 3 - The Third Book",
			wantSeries:   "My Long Series",
			wantPosition: "3",
		},
		// Rule 2: trailing digit
		{
			name:         "trailing digit with space",
			input:        "The Long Earth 2",
			wantSeries:   "The Long Earth",
			wantPosition: "2",
		},
		{
			name:         "trailing digit with dash-space",
			input:        "The Long Earth - 2",
			wantSeries:   "The Long Earth",
			wantPosition: "2",
		},
		// Rule 3: trailing ordinal word
		{
			name:         "trailing ordinal One",
			input:        "The Long Earth One",
			wantSeries:   "The Long Earth",
			wantPosition: "1",
		},
		{
			name:         "trailing ordinal Two lowercase",
			input:        "the long earth two",
			wantSeries:   "the long earth",
			wantPosition: "2",
		},
		{
			name:         "trailing Twenty",
			input:        "My Series Twenty",
			wantSeries:   "My Series",
			wantPosition: "20",
		},
		// Rule 4: series equals title (no other pattern matched)
		{
			name:           "exact series==title with no other match",
			input:          "Just A Title",
			title:          "Just A Title",
			wantSeries:     "Just A Title",
			wantPosition:   "",
			wantFlagReview: true,
		},
		// No-op cases
		{
			name:       "clean series name unchanged",
			input:      "The Expanse",
			wantSeries: "The Expanse",
		},
		{
			name:       "Discworld unchanged",
			input:      "Discworld",
			wantSeries: "Discworld",
		},
		// Edge cases
		{
			name:       "ordinal Twenty-One not matched (out of range)",
			input:      "My Series Twenty-One",
			wantSeries: "My Series Twenty-One",
		},
		{
			name:       "word Someone not matched as ordinal",
			input:      "Someone",
			wantSeries: "Someone",
		},
		{
			name:       "empty name unchanged",
			input:      "",
			wantSeries: "",
		},
		{
			name:         "trailing digit 99 matched",
			input:        "Big Series 99",
			wantSeries:   "Big Series",
			wantPosition: "99",
		},
		{
			name:       "trailing 3-digit number not matched",
			input:      "Fahrenheit 451",
			wantSeries: "Fahrenheit 451",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSeries, gotPos, gotFlag := StripSeriesContamination(tt.input, tt.title)
			if gotSeries != tt.wantSeries {
				t.Errorf("series: got %q, want %q", gotSeries, tt.wantSeries)
			}
			if gotPos != tt.wantPosition {
				t.Errorf("position: got %q, want %q", gotPos, tt.wantPosition)
			}
			if gotFlag != tt.wantFlagReview {
				t.Errorf("flagForReview: got %v, want %v", gotFlag, tt.wantFlagReview)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/.worktrees/series-normalize
go test ./internal/metadata/... -run TestStripSeriesContamination -v 2>&1 | head -20
```

Expected: `FAIL` — `StripSeriesContamination` undefined.

- [ ] **Step 3: Write the implementation**

```go
// file: internal/metadata/series_normalize.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package metadata

import (
	"regexp"
	"strings"
)

var (
	// "Series Name - 1 - Title" — position and title embedded in the series field
	reDashPositionTitle = regexp.MustCompile(`^(.+?)\s+-\s+(\d+)\s+-\s+.+$`)
	// "Series Name 2" or "Series Name - 2" — trailing 1-2 digit number
	reTrailingDigit = regexp.MustCompile(`^(.+?)(?:\s+-|\s)+(\d{1,2})$`)
	// "Series Name One" — trailing ordinal word, one through twenty only
	reTrailingOrdinal = regexp.MustCompile(`(?i)^(.+?)\s+(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)$`)
)

var ordinalToDigit = map[string]string{
	"one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
	"six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10",
	"eleven": "11", "twelve": "12", "thirteen": "13", "fourteen": "14",
	"fifteen": "15", "sixteen": "16", "seventeen": "17", "eighteen": "18",
	"nineteen": "19", "twenty": "20",
}

// StripSeriesContamination removes title and position info that has been incorrectly
// embedded in a series name. Returns the cleaned series name, the extracted position
// (empty if none), and flagForReview=true when the series name equals the book title
// and no structural pattern was matched (needs human review).
//
// Rules applied in order, stopping at first match:
//
//  1. Dash-embedded: "Series - 1 - Title" → series="Series", pos="1"
//  2. Trailing 1-2 digit number: "Series 2" or "Series - 2" → series="Series", pos="2"
//  3. Trailing ordinal word (one–twenty): "Series One" → series="Series", pos="1"
//  4. Series equals title → flagForReview=true, series unchanged
func StripSeriesContamination(name, title string) (series, position string, flagForReview bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}

	if m := reDashPositionTitle.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1]), m[2], false
	}

	if m := reTrailingDigit.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1]), m[2], false
	}

	if m := reTrailingOrdinal.FindStringSubmatch(name); m != nil {
		pos := ordinalToDigit[strings.ToLower(m[2])]
		return strings.TrimSpace(m[1]), pos, false
	}

	if title != "" && strings.EqualFold(name, strings.TrimSpace(title)) {
		return name, "", true
	}

	return name, "", false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/metadata/... -run TestStripSeriesContamination -v
```

Expected: All cases PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metadata/series_normalize.go internal/metadata/series_normalize_test.go
git commit -m "feat(metadata): add StripSeriesContamination pure function with tests"
```

---

## Task 2: Wire into `NormalizeMetaSeries`

**Files:**
- Modify: `internal/metafetch/service.go` (function `NormalizeMetaSeries`, around line 824)

- [ ] **Step 1: Write the failing test**

In `internal/metafetch/service_test.go`, find the existing `TestNormalizeMetaSeries` table and add:

```go
{
    name:         "dash-embedded series cleaned",
    inputSeries:  "The Long Earth - 1 - The Long Earth",
    inputTitle:   "The Long Earth",
    wantSeries:   "The Long Earth",
    wantPosition: "1",
    wantTitle:    "The Long Earth",
},
{
    name:         "trailing ordinal word cleaned",
    inputSeries:  "The Long Earth One",
    wantSeries:   "The Long Earth",
    wantPosition: "1",
},
```

(Check the existing test struct field names in service_test.go and match them.)

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/metafetch/... -run TestNormalizeMetaSeries -v 2>&1 | tail -20
```

Expected: New cases FAIL.

- [ ] **Step 3: Extend `NormalizeMetaSeries`**

In `internal/metafetch/service.go`, replace the `NormalizeMetaSeries` function body
(around line 824) with:

```go
func NormalizeMetaSeries(meta *metadata.BookMetadata) {
	// Strip contamination (embedded title/position) from the series field first.
	if meta.Series != "" {
		cleaned, pos, flagged := metadata.StripSeriesContamination(meta.Series, meta.Title)
		if !flagged && cleaned != meta.Series {
			meta.Series = cleaned
			if pos != "" && meta.SeriesPosition == "" {
				meta.SeriesPosition = pos
			}
		}
	}

	// Existing logic: parse series info embedded in the title field.
	parsedSeries, parsedPosition, parsedTitle := ParseSeriesFromTitle(meta.Title)
	if parsedSeries == "" && meta.Series != "" {
		parsedSeries, parsedPosition, parsedTitle = ParseSeriesFromTitle(meta.Series)
		if parsedTitle == "" {
			parsedTitle = meta.Title
		}
	}
	if parsedSeries == "" {
		return
	}
	meta.Series = parsedSeries
	if parsedPosition != "" {
		meta.SeriesPosition = parsedPosition
	}
	if parsedTitle != "" {
		meta.Title = parsedTitle
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/metafetch/... -run TestNormalizeMetaSeries -v
```

Expected: All cases PASS including the new ones.

- [ ] **Step 5: Commit**

```bash
git add internal/metafetch/service.go internal/metafetch/service_test.go
git commit -m "feat(metafetch): wire StripSeriesContamination into NormalizeMetaSeries"
```

---

## Task 3: Wire into scanner's `resolveSeriesID`

**Files:**
- Modify: `internal/scanner/scanner.go` (function `resolveSeriesID`, around line 1782)

- [ ] **Step 1: Apply the wire-up**

In `internal/scanner/scanner.go`, modify `resolveSeriesID` to normalize before
any store calls:

```go
func resolveSeriesID(seriesName string, authorID *int) (*int, error) {
	trimmed := strings.TrimSpace(seriesName)
	if trimmed == "" {
		return nil, nil
	}

	// Strip any embedded title/position contamination from the series name.
	// Position info is discarded here; the scanner does not set SeriesSequence.
	if cleaned, _, flagged := metadata.StripSeriesContamination(trimmed, ""); !flagged && cleaned != "" {
		trimmed = cleaned
	}

	series, err := database.GetGlobalStore().GetSeriesByName(trimmed, authorID)
	// ... rest of existing code unchanged ...
```

The `internal/scanner` package already imports `internal/metadata` — no new import needed.

- [ ] **Step 2: Verify compilation and existing tests**

```bash
go build ./internal/scanner/... && go test ./internal/scanner/... 2>&1 | tail -10
```

Expected: Compiles, existing tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat(scanner): strip series contamination in resolveSeriesID"
```

---

## Task 4: Wire into iTunes importer's `ensureSeriesID`

**Files:**
- Modify: `internal/itunes/service/importer.go` (function `ensureSeriesID`, around line 1262)

- [ ] **Step 1: Apply the wire-up**

In `internal/itunes/service/importer.go`, modify `ensureSeriesID`:

```go
func (imp *Importer) ensureSeriesID(name string, authorID *int) (*int, error) {
	// Strip any embedded title/position contamination from the series name.
	if cleaned, _, flagged := metadata.StripSeriesContamination(strings.TrimSpace(name), ""); !flagged && cleaned != "" {
		name = cleaned
	}

	series, err := imp.store.GetSeriesByName(name, authorID)
	if err != nil {
		return nil, err
	}
	if series != nil {
		return &series.ID, nil
	}
	series, err = imp.store.CreateSeries(name, authorID)
	if err != nil {
		return nil, err
	}
	return &series.ID, nil
}
```

`internal/itunes/service/importer.go` already imports `internal/metadata` — no new import needed.

- [ ] **Step 2: Verify compilation and existing tests**

```bash
go build ./internal/itunes/... && go test ./internal/itunes/... 2>&1 | tail -10
```

Expected: Compiles, existing tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/itunes/service/importer.go
git commit -m "feat(itunes): strip series contamination in ensureSeriesID"
```

---

## Task 5: Dry-run preview logic

**Files:**
- Modify: `internal/server/duplicates_handlers.go`

Implements `computeSeriesNormalizeActions` and `seriesNormalizePreview`. No writes.

- [ ] **Step 1: Write the failing test**

Add to a test file in the `server` package (check for existing
`duplicates_handlers_test.go` or equivalent):

```go
func TestComputeSeriesNormalizeActions_Basic(t *testing.T) {
	authorID := 1
	store := &database.MockStore{}
	store.GetAllSeriesFunc = func() ([]database.Series, error) {
		return []database.Series{
			{ID: 1, Name: "The Long Earth One", AuthorID: &authorID},
			{ID: 2, Name: "The Long Earth Two", AuthorID: &authorID},
			{ID: 3, Name: "Discworld", AuthorID: &authorID},
		}, nil
	}
	store.GetBooksBySeriesIDFunc = func(id int) ([]database.Book, error) {
		return []database.Book{{ID: fmt.Sprintf("book-%d", id)}}, nil
	}

	actions := computeSeriesNormalizeActions(store)

	for _, a := range actions {
		if a.OldName == "Discworld" {
			t.Errorf("clean series Discworld should not appear in actions")
		}
	}
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/server/... -run TestComputeSeriesNormalizeActions -v 2>&1 | head -20
```

Expected: FAIL — `computeSeriesNormalizeActions` undefined.

- [ ] **Step 3: Implement types and `computeSeriesNormalizeActions`**

Add to `internal/server/duplicates_handlers.go`:

```go
type seriesNormalizeAction struct {
	SeriesID      int    `json:"series_id"`
	OldName       string `json:"old_name"`
	NewName       string `json:"new_name"`
	NewPosition   string `json:"new_position,omitempty"`
	Action        string `json:"action"` // "rename", "merge_into", "flag"
	MergeTargetID *int   `json:"merge_target_id,omitempty"`
	BookCount     int    `json:"book_count"`
}

type seriesNormalizePreviewResult struct {
	Actions             []seriesNormalizeAction `json:"actions"`
	TotalSeriesAffected int                     `json:"total_series_affected"`
	TotalBooksAffected  int                     `json:"total_books_affected"`
	FlaggedForReview    []seriesNormalizeAction `json:"flagged_for_review"`
}

func computeSeriesNormalizeActions(store interface {
	database.SeriesStore
	database.BookStore
}) []seriesNormalizeAction {
	allSeries, err := store.GetAllSeries()
	if err != nil {
		return nil
	}

	type groupKey struct {
		name     string
		authorID int
	}
	canonical := make(map[groupKey]int)
	var actions []seriesNormalizeAction

	for _, s := range allSeries {
		cleaned, pos, flagged := metadata.StripSeriesContamination(s.Name, "")

		if flagged {
			books, _ := store.GetBooksBySeriesID(s.ID)
			actions = append(actions, seriesNormalizeAction{
				SeriesID:  s.ID,
				OldName:   s.Name,
				NewName:   s.Name,
				Action:    "flag",
				BookCount: len(books),
			})
			continue
		}

		if cleaned == s.Name && pos == "" {
			continue
		}

		aid := 0
		if s.AuthorID != nil {
			aid = *s.AuthorID
		}
		key := groupKey{name: strings.ToLower(cleaned), authorID: aid}
		books, _ := store.GetBooksBySeriesID(s.ID)

		if existingID, ok := canonical[key]; ok {
			actions = append(actions, seriesNormalizeAction{
				SeriesID:      s.ID,
				OldName:       s.Name,
				NewName:       cleaned,
				NewPosition:   pos,
				Action:        "merge_into",
				MergeTargetID: &existingID,
				BookCount:     len(books),
			})
		} else {
			canonical[key] = s.ID
			actions = append(actions, seriesNormalizeAction{
				SeriesID:    s.ID,
				OldName:     s.Name,
				NewName:     cleaned,
				NewPosition: pos,
				Action:      "rename",
				BookCount:   len(books),
			})
		}
	}
	return actions
}
```

- [ ] **Step 4: Add `seriesNormalizePreview` handler**

```go
func (s *Server) seriesNormalizePreview(c *gin.Context) {
	store := s.Store()
	if store == nil {
		RespondWithInternalError(c, "database not initialized")
		return
	}

	actions := computeSeriesNormalizeActions(store)

	var flagged, normal []seriesNormalizeAction
	totalBooks := 0
	for _, a := range actions {
		if a.Action == "flag" {
			flagged = append(flagged, a)
		} else {
			normal = append(normal, a)
			totalBooks += a.BookCount
		}
	}

	RespondWithOK(c, seriesNormalizePreviewResult{
		Actions:             normal,
		TotalSeriesAffected: len(normal),
		TotalBooksAffected:  totalBooks,
		FlaggedForReview:    flagged,
	})
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/server/... -run TestComputeSeriesNormalizeActions -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/duplicates_handlers.go
git commit -m "feat(series): add computeSeriesNormalizeActions and dry-run preview handler"
```

---

## Task 6: Full normalize operation (rename + merge + write-back + organize)

**Files:**
- Modify: `internal/server/duplicates_handlers.go`

- [ ] **Step 1: Write the failing test**

```go
func TestExecuteSeriesNormalizeCore_RenamesAndEnqueues(t *testing.T) {
	authorID := 1
	store := &database.MockStore{}
	store.GetAllSeriesFunc = func() ([]database.Series, error) {
		return []database.Series{
			{ID: 1, Name: "The Long Earth One", AuthorID: &authorID},
			{ID: 2, Name: "The Long Earth Two", AuthorID: &authorID},
		}, nil
	}
	store.GetBooksBySeriesIDFunc = func(id int) ([]database.Book, error) {
		switch id {
		case 1:
			return []database.Book{{ID: "book-1"}}, nil
		case 2:
			return []database.Book{{ID: "book-2"}}, nil
		}
		return nil, nil
	}
	renamed := map[int]string{}
	store.UpdateSeriesNameFunc = func(id int, name string) error {
		renamed[id] = name
		return nil
	}
	store.GetBookByIDFunc = func(id string) (*database.Book, error) {
		sid := 1
		return &database.Book{ID: id, SeriesID: &sid}, nil
	}
	store.UpdateBookFunc = func(id string, b *database.Book) (*database.Book, error) { return b, nil }
	store.DeleteSeriesFunc = func(id int) error { return nil }

	var enqueuedBooks []string
	enqueueWB := func(id string) { enqueuedBooks = append(enqueuedBooks, id) }

	affected, err := executeSeriesNormalizeCore(store, enqueueWB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if renamed[1] != "The Long Earth" {
		t.Errorf("expected series 1 renamed to 'The Long Earth', got %q", renamed[1])
	}
	if len(enqueuedBooks) == 0 {
		t.Errorf("expected write-back enqueues for affected books")
	}
	if len(affected) == 0 {
		t.Errorf("expected affected book IDs returned")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/server/... -run TestExecuteSeriesNormalizeCore -v 2>&1 | head -20
```

Expected: FAIL — `executeSeriesNormalizeCore` undefined.

- [ ] **Step 3: Implement `executeSeriesNormalizeCore`**

```go
// executeSeriesNormalizeCore renames and merges contaminated series, enqueues
// write-back for affected books, and returns the affected book IDs for the
// caller to run organize on.
func executeSeriesNormalizeCore(
	store interface {
		database.SeriesStore
		database.BookStore
	},
	enqueueWriteBack func(bookID string),
) (affectedBookIDs []string, err error) {
	actions := computeSeriesNormalizeActions(store)

	// Collect affected book IDs BEFORE renaming/merging.
	seen := make(map[string]bool)
	for _, a := range actions {
		if a.Action == "flag" {
			continue
		}
		books, bErr := store.GetBooksBySeriesID(a.SeriesID)
		if bErr != nil {
			continue
		}
		for _, b := range books {
			if !seen[b.ID] {
				seen[b.ID] = true
				affectedBookIDs = append(affectedBookIDs, b.ID)
			}
		}
	}

	// First pass: rename.
	for _, a := range actions {
		if a.Action != "rename" {
			continue
		}
		if rErr := store.UpdateSeriesName(a.SeriesID, a.NewName); rErr != nil {
			return nil, fmt.Errorf("UpdateSeriesName(%d, %q): %w", a.SeriesID, a.NewName, rErr)
		}
	}

	// Second pass: merge.
	for _, a := range actions {
		if a.Action != "merge_into" || a.MergeTargetID == nil {
			continue
		}
		if mErr := mergeSeriesGroup(store, *a.MergeTargetID, []int{a.SeriesID}); mErr != nil {
			return nil, fmt.Errorf("mergeSeriesGroup(keep=%d, merge=%d): %w", *a.MergeTargetID, a.SeriesID, mErr)
		}
	}

	for _, id := range affectedBookIDs {
		enqueueWriteBack(id)
	}

	return affectedBookIDs, nil
}
```

- [ ] **Step 4: Add `seriesNormalize` HTTP handler**

```go
func (s *Server) seriesNormalize(c *gin.Context) {
	store := s.Store()
	if store == nil {
		RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.queue == nil {
		RespondWithInternalError(c, "operation queue not initialized")
		return
	}

	opID := ulid.Make().String()
	detail := "series-normalize"
	op, err := store.CreateOperation(opID, "series-normalize", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", "Starting series name normalization...", nil)
		log2 := logger.NewWithActivityLog("series-normalize", store)

		enqueueWB := func(bookID string) {
			if s.writeBackBatcher != nil {
				s.writeBackBatcher.Enqueue(bookID)
			}
		}

		affectedBookIDs, err := executeSeriesNormalizeCore(store, enqueueWB)
		if err != nil {
			return err
		}

		_ = progress.Log("info", fmt.Sprintf("Renamed/merged series; organizing %d affected books...", len(affectedBookIDs)), nil)

		for _, bookID := range affectedBookIDs {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			book, bErr := store.GetBookByID(bookID)
			if bErr != nil || book == nil {
				continue
			}
			if _, oErr := s.organizeService.ReOrganizeInPlace(book, log2); oErr != nil {
				_ = progress.Log("warn", fmt.Sprintf("organize failed for book %s: %v", bookID, oErr), nil)
			}
		}

		_ = progress.Log("info", "Series normalization complete.", nil)
		return nil
	}

	if err := s.queue.Enqueue(op.ID, "series-normalize", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	RespondWithSuccess(c, 202, op)
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/server/... -run TestExecuteSeriesNormalizeCore -v
go build ./internal/server/...
```

Expected: Tests PASS, no compile errors.

- [ ] **Step 6: Commit**

```bash
git add internal/server/duplicates_handlers.go
git commit -m "feat(series): implement executeSeriesNormalizeCore and seriesNormalize handler"
```

---

## Task 7: Register route and interrupt-recovery entry

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add routes**

In `internal/server/server.go`, find the series routes block (around line 2310,
after `protected.POST("/series/prune", ...)`). Add:

```go
protected.GET("/series/normalize/preview", s.perm(auth.PermLibraryView), s.seriesNormalizePreview)
protected.POST("/series/normalize", s.perm(auth.PermLibraryEditMetadata), s.seriesNormalize)
```

- [ ] **Step 2: Add to interrupt-recovery list**

Find the `case` statement around line 1509 listing non-resumable operation types.
Add `"series-normalize"` to the list:

```go
case "transcode", "diagnostics_export", "diagnostics_ai",
    "cleanup_activity_log", "purge_old_logs",
    "purge-deleted", "tombstone-cleanup",
    "author-dedup-scan", "author-split-scan", "series-prune",
    "db-optimize", "cleanup-old-backups", "batch_poller",
    "itunes_sync", "series-normalize":
```

- [ ] **Step 3: Verify**

```bash
go build ./... && go test ./internal/server/... 2>&1 | tail -10
```

Expected: Compiles, no regressions.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(series): register /series/normalize routes and interrupt-recovery entry"
```

---

## Task 8: Register scheduler task

**Files:**
- Modify: `internal/server/scheduler.go`

- [ ] **Step 1: Add task registration**

Find where `series_prune` is registered in `internal/server/scheduler.go` (around
line 261) and add immediately after it:

```go
ts.registerTask(TaskDefinition{
    Name:        "series_normalize",
    Description: "Strip title/position contamination from series names and run write-back + organize for affected books",
    Category:    "maintenance",
    TriggerFn: func() (*database.Operation, error) {
        return ts.triggerOperation("series-normalize", func(ctx context.Context, progress operations.ProgressReporter) error {
            store := ts.server.Store()
            if store == nil {
                return fmt.Errorf("database not initialized")
            }
            enqueueWB := func(bookID string) {
                if ts.server.writeBackBatcher != nil {
                    ts.server.writeBackBatcher.Enqueue(bookID)
                }
            }
            _, err := executeSeriesNormalizeCore(store, enqueueWB)
            return err
        })
    },
    IsEnabled:              func() bool { return true },
    GetInterval:            func() time.Duration { return 0 },
    RunOnStart:             func() bool { return false },
    RunInMaintenanceWindow: func() bool { return false },
})
```

- [ ] **Step 2: Verify compilation and full test suite**

```bash
go build ./... && make test
```

Expected: Builds clean, all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "feat(series): register series_normalize maintenance task in scheduler"
```

---

## Task 9: Integration smoke test

- [ ] **Step 1: Run full test suite**

```bash
make test
```

Expected: All tests pass, coverage ≥ 80%.

- [ ] **Step 2: Deploy**

```bash
make deploy
```

- [ ] **Step 3: Dry-run against production**

```bash
curl -s -H "Authorization: Bearer $(grep ADMIN_API_KEY .env | cut -d= -f2)" \
  "http://localhost:8080/api/v1/series/normalize/preview" | jq '{total: .total_series_affected, first_three: .actions[:3]}'
```

Review output, confirm renamed/merged names look correct.

- [ ] **Step 4: Execute normalization**

```bash
curl -s -X POST -H "Authorization: Bearer $(grep ADMIN_API_KEY .env | cut -d= -f2)" \
  "http://localhost:8080/api/v1/series/normalize" | jq .
```

Monitor operation in UI until complete. Verify affected books have correct series
names, corrected file tags, and paths moved to shorter directories.
