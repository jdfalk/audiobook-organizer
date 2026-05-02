# Author Dedup Redesign — Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix author/series dedup matching quality, canonical selection, and add selective merge UX.

**Architecture:** Backend changes to `author_dedup.go` for dirty data detection, multi-author awareness, and smart canonical picking. Frontend changes to `BookDedup.tsx` for checkbox multi-select and per-group merge on all tabs.

**Tech Stack:** Go (backend matching logic), React/TypeScript/MUI (frontend dedup UI)

---

### Task 1: Add dirty data and multi-author detection to author_dedup.go

**Files:**
- Modify: `internal/server/author_dedup.go`
- Test: `internal/server/author_dedup_test.go`

**Step 1: Write failing tests for dirty data detection**

Add to `internal/server/author_dedup_test.go`:

```go
func TestIsDirtyAuthorName(t *testing.T) {
	dirty := []string{
		"Neal Stephenson - Snow Crash",       // book title in author
		"Big Finish Production",               // publisher pattern
		"BBC Studios",                         // publisher pattern
		"Penguin Random House",                // publisher pattern
	}
	for _, name := range dirty {
		if !isDirtyAuthorName(name) {
			t.Errorf("expected %q to be flagged as dirty", name)
		}
	}

	clean := []string{
		"Neal Stephenson",
		"James S. A. Corey",
		"Brandon Sanderson",
		"Natalie Maher (aka Thundamoo)",
	}
	for _, name := range clean {
		if isDirtyAuthorName(name) {
			t.Errorf("expected %q to NOT be flagged as dirty", name)
		}
	}
}

func TestIsCompositeAuthorName(t *testing.T) {
	composite := []string{
		"Orson Scott Card/A Johnston",          // two authors with slash
		"Mark Tufo, Sean Runnette",             // two full names comma-separated
	}
	for _, name := range composite {
		if !isCompositeAuthorName(name) {
			t.Errorf("expected %q to be composite", name)
		}
	}

	single := []string{
		"David Kushner",
		"Smith, John",                          // Last, First format
		"J. K. Rowling",
		"Natalie Maher (aka Thundamoo)",        // alias, not composite
	}
	for _, name := range single {
		if isCompositeAuthorName(name) {
			t.Errorf("expected %q to NOT be composite", name)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -run "TestIsDirty|TestIsComposite" -v`
Expected: FAIL — functions not defined

**Step 3: Implement isDirtyAuthorName and isCompositeAuthorName**

Add to `internal/server/author_dedup.go`:

```go
// isDirtyAuthorName returns true if the name is obviously not a real author
// (contains book titles, is a publisher, etc.)
func isDirtyAuthorName(name string) bool {
	// Contains " - " → likely "Author - Book Title"
	if strings.Contains(name, " - ") {
		return true
	}

	// Publisher/production company patterns
	lower := strings.ToLower(name)
	publisherSuffixes := []string{"production", "productions", "publishing", "publishers",
		"press", "studios", "studio", "media", "entertainment", "books", "audio",
		"house", "group", "company", "records", "recordings"}
	for _, suffix := range publisherSuffixes {
		if strings.HasSuffix(lower, " "+suffix) {
			return true
		}
	}

	// Known publisher prefixes
	publisherPrefixes := []string{"bbc ", "penguin ", "harpercollins", "hachette", "simon & schuster"}
	for _, prefix := range publisherPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

// isCompositeAuthorName returns true if the name contains multiple real authors
// (slash-separated full names or comma-separated full names that aren't Last, First).
func isCompositeAuthorName(name string) bool {
	// "(aka ...)" pattern is an alias, NOT composite
	if regexp.MustCompile(`(?i)\(aka\s`).MatchString(name) {
		return false
	}

	// Slash with names on both sides: "Author One/Author Two"
	if idx := strings.Index(name, "/"); idx > 0 {
		left := strings.TrimSpace(name[:idx])
		right := strings.TrimSpace(name[idx+1:])
		// Both sides must have at least 2 chars (not just an initial)
		if len(left) > 2 && len(right) > 2 {
			return true
		}
	}

	// Comma-separated: distinguish "Last, First" from "Author One, Author Two"
	parts := strings.SplitN(name, ",", 2)
	if len(parts) == 2 {
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		// If right side contains a space, it's likely a full name → two authors
		// "Smith, John" → right="John" (no space) → Last, First
		// "Mark Tufo, Sean Runnette" → right="Sean Runnette" (has space) → two authors
		if strings.Contains(right, " ") && strings.Contains(left, " ") {
			return true
		}
		// If right side itself has a space and left doesn't, still could be two authors
		// "Thundamoo, Natalie Maher" — but this is ambiguous. Require both to have spaces.
	}

	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -run "TestIsDirty|TestIsComposite" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/author_dedup.go internal/server/author_dedup_test.go
git commit -m "feat(dedup): add dirty data and composite author detection"
```

---

### Task 2: Fix areAuthorsDuplicate to reject false positives and skip dirty/composite names

**Files:**
- Modify: `internal/server/author_dedup.go`
- Modify: `internal/server/author_dedup_test.go`

**Step 1: Write failing tests for the false positive cases from screenshots**

Add to `internal/server/author_dedup_test.go`:

```go
func TestAreAuthorsDuplicate(t *testing.T) {
	shouldMatch := []struct{ a, b string }{
		{"James S. A. Corey", "James S.A. Corey"},
		{"Brandon Sanderson", "Brandon  Sanderson"},
		{"David Kushner", "David Kushner/Wil Wheaton"},  // base extraction
		{"Stephen King", "Steven King"},                  // close first names
		{"J. K. Rowling", "J.K. Rowling"},
	}
	for _, tt := range shouldMatch {
		if !areAuthorsDuplicate(tt.a, tt.b) {
			t.Errorf("expected %q and %q to match", tt.a, tt.b)
		}
	}

	shouldNotMatch := []struct{ a, b string }{
		{"Michael Grant", "Michael Angel"},
		{"Michael Grant", "Michael Troughton"},
		{"Michael Grant", "Michael Langan"},
		{"Michael Grant", "Michael Braun"},
		{"Michael Grant", "Michael Dalton"},
		{"Alex Karne", "Alex Irvine"},
		{"Mark Tufo", "Mark Twain"},
		{"Neal Stephenson", "Neal Stephenson - Snow Crash"},  // dirty name
	}
	for _, tt := range shouldNotMatch {
		if areAuthorsDuplicate(tt.a, tt.b) {
			t.Errorf("expected %q and %q to NOT match", tt.a, tt.b)
		}
	}
}
```

**Step 2: Run tests to verify failures**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -run "TestAreAuthorsDuplicate$" -v`
Expected: Some FAIL (especially "Neal Stephenson" vs dirty name)

**Step 3: Update areAuthorsDuplicate to reject dirty names**

In `internal/server/author_dedup.go`, add at the top of `areAuthorsDuplicate`:

```go
func areAuthorsDuplicate(name1, name2 string) bool {
	// Skip dirty names (book titles, publishers)
	if isDirtyAuthorName(name1) || isDirtyAuthorName(name2) {
		return false
	}

	// ... rest of existing function unchanged
```

**Step 4: Update FindDuplicateAuthors to also skip composite names**

In `FindDuplicateAuthors`, change the skip condition:

```go
// Old:
if used[authors[i].ID] || isMultiAuthorString(authors[i].Name) {

// New:
if used[authors[i].ID] || isMultiAuthorString(authors[i].Name) || isCompositeAuthorName(authors[i].Name) || isDirtyAuthorName(authors[i].Name) {
```

Same for the inner loop check on `authors[j]`.

**Step 5: Run all author dedup tests**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -run "TestAreAuthors|TestFindDuplicate|TestIsDirty|TestIsComposite|TestNormalize|TestJaro|TestIsMulti" -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/server/author_dedup.go internal/server/author_dedup_test.go
git commit -m "fix(dedup): reject false positives, skip dirty/composite author names"
```

---

### Task 3: Smart canonical selection — pick the cleanest name

**Files:**
- Modify: `internal/server/author_dedup.go`
- Modify: `internal/server/author_dedup_test.go`

**Step 1: Write failing test**

```go
func TestPickCanonicalAuthor(t *testing.T) {
	tests := []struct {
		names    []database.Author
		counts   map[int]int
		expectID int
	}{
		{
			names:    []database.Author{{ID: 1, Name: "David Kushner/Wil Wheaton"}, {ID: 2, Name: "David Kushner"}},
			counts:   map[int]int{1: 3, 2: 3},
			expectID: 2, // shorter, no slash
		},
		{
			names:    []database.Author{{ID: 1, Name: "Natalie Maher (aka Thundamoo)"}, {ID: 2, Name: "Natalie Maher"}},
			counts:   map[int]int{1: 1, 2: 1},
			expectID: 2, // no parenthetical
		},
		{
			names:    []database.Author{{ID: 1, Name: "Mark Tufo"}, {ID: 2, Name: "Mark Tufo (Sean Runnette)"}},
			counts:   map[int]int{1: 5, 2: 1},
			expectID: 1, // cleaner + more books
		},
	}

	for _, tt := range tests {
		countFn := func(id int) int { return tt.counts[id] }
		canonical := pickCanonicalAuthor(tt.names, countFn)
		if canonical.ID != tt.expectID {
			t.Errorf("for names %v, expected canonical ID %d, got %d (%s)",
				tt.names, tt.expectID, canonical.ID, canonical.Name)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -run "TestPickCanonical" -v`
Expected: FAIL — function not defined

**Step 3: Implement pickCanonicalAuthor**

Add to `internal/server/author_dedup.go`:

```go
// authorNameScore returns a penalty score for a name. Lower = cleaner/better.
func authorNameScore(name string) int {
	score := 0
	if strings.Contains(name, "/") {
		score += 10
	}
	if strings.Contains(name, "(") {
		score += 10
	}
	if strings.Contains(name, " - ") {
		score += 20
	}
	// Prefer shorter names (fewer extra characters)
	score += len(name)
	return score
}

// pickCanonicalAuthor selects the cleanest author name from a group.
// Prefers names without slashes, parentheticals, or embedded titles.
// Uses book count as tiebreaker.
func pickCanonicalAuthor(authors []database.Author, bookCountFn func(int) int) database.Author {
	if len(authors) == 0 {
		return database.Author{}
	}
	best := 0
	bestScore := authorNameScore(authors[0].Name)
	bestBooks := bookCountFn(authors[0].ID)

	for i := 1; i < len(authors); i++ {
		score := authorNameScore(authors[i].Name)
		books := bookCountFn(authors[i].ID)
		if score < bestScore || (score == bestScore && books > bestBooks) {
			best = i
			bestScore = score
			bestBooks = books
		}
	}
	return authors[best]
}
```

**Step 4: Wire pickCanonicalAuthor into FindDuplicateAuthors**

In `FindDuplicateAuthors`, replace the current canonical assignment:

```go
// Old:
group := AuthorDedupGroup{
	Canonical: authors[i],
}

// After finding all variants, before appending to groups:
if len(group.Variants) > 0 {
	// Pick the cleanest name as canonical
	allInGroup := append([]database.Author{authors[i]}, group.Variants...)
	canonical := pickCanonicalAuthor(allInGroup, bookCountFn)
	group.Canonical = canonical
	// Variants = everyone except canonical
	var variants []database.Author
	for _, a := range allInGroup {
		if a.ID != canonical.ID {
			variants = append(variants, a)
		}
	}
	group.Variants = variants
	// ... rest of book count logic
}
```

**Step 5: Run all tests**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -run "TestPickCanonical|TestFindDuplicate" -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/server/author_dedup.go internal/server/author_dedup_test.go
git commit -m "feat(dedup): smart canonical author selection — prefer cleanest name"
```

---

### Task 4: Frontend — Add selective merge with checkboxes to all dedup tabs

**Files:**
- Modify: `web/src/pages/BookDedup.tsx`

**Step 1: Update BookDedup.tsx with checkbox multi-select on all three tabs**

The key changes for each tab:
- Add `selectedGroups` state: `useState<Set<string>>(new Set())`
- Each card gets a `<Checkbox>` in the header
- A "Select All" / "Deselect All" toggle
- Floating action bar when any selected: "Merge Selected (N)" button
- "Merge All" gets a confirmation dialog: "This will merge N groups. This action cannot be undone. Are you sure?"
- Individual "Merge" button stays on each card

For the **Series tab** specifically:
- Add per-group "Merge" button (currently missing — only has "Merge All")
- The merge button should call a new `mergeSeriesGroup` API function

**Step 2: Add series individual merge API function**

In `web/src/services/api.ts`, add:

```typescript
export async function mergeSeriesGroup(keepId: number, mergeIds: number[]): Promise<Operation> {
  const response = await fetch(`${API_BASE}/series/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ keep_id: keepId, merge_ids: mergeIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge series');
  }
  return response.json();
}
```

**Step 3: Verify TypeScript compiles**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add web/src/pages/BookDedup.tsx web/src/services/api.ts
git commit -m "feat(dedup): selective merge with checkboxes, per-group series merge"
```

---

### Task 5: Backend — Add series individual merge endpoint

**Files:**
- Modify: `internal/server/server.go`

**Step 1: Add `POST /series/merge` route and handler**

Register the route near the existing series routes (around line 1168):

```go
protected.POST("/series/merge", s.mergeSeriesGroup)
```

Add handler (similar pattern to `mergeAuthors`):

```go
func (s *Server) mergeSeriesGroup(c *gin.Context) {
	var req struct {
		KeepID   int   `json:"keep_id" binding:"required"`
		MergeIDs []int `json:"merge_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := database.GlobalStore
	keepSeries, err := store.GetSeriesByID(req.KeepID)
	if err != nil || keepSeries == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep series not found"})
		return
	}

	opID := ulid.Make().String()
	detail := gin.H{"keep_id": req.KeepID, "merge_ids": req.MergeIDs, "keep_name": keepSeries.Name}
	op, _ := store.CreateOperation(opID, "series-merge", &detail)

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		for i, mergeID := range req.MergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			progress.UpdateProgress(i, len(req.MergeIDs), fmt.Sprintf("Merging series %d into %d", mergeID, req.KeepID))

			// Get all books linked to the merge series and relink to keep series
			books, err := store.GetBooksBySeriesID(mergeID)
			if err != nil {
				progress.Log("error", fmt.Sprintf("Failed to get books for series %d: %v", mergeID, err), "")
				continue
			}
			for _, book := range books {
				if book.SeriesID != nil && *book.SeriesID == mergeID {
					book.SeriesID = &req.KeepID
					if err := store.UpdateBook(&book); err != nil {
						progress.Log("error", fmt.Sprintf("Failed to update book %s: %v", book.ID, err), "")
					}
				}
			}
			if err := store.DeleteSeries(mergeID); err != nil {
				progress.Log("error", fmt.Sprintf("Failed to delete series %d: %v", mergeID, err), "")
			}
		}
		progress.UpdateProgress(len(req.MergeIDs), len(req.MergeIDs), "Complete")
		return nil
	}

	operations.GlobalQueue.Enqueue(op.ID, "series-merge", operations.PriorityNormal, operationFunc)
	c.JSON(http.StatusAccepted, op)
}
```

**Step 2: Check if GetBooksBySeriesID exists**

Search the store interface. If it doesn't exist, you'll need to check existing methods. The deduplicateSeriesHandler already does this — look at how it gets books by series and follow the same pattern.

**Step 3: Run Go build**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./...`
Expected: No errors

**Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(dedup): add POST /series/merge endpoint for individual series merge"
```

---

### Task 6: Full verification

**Step 1: Run all Go tests**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/server/ -v -count=1 2>&1 | tail -30`

**Step 2: Run TypeScript check**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web && npx tsc --noEmit`

**Step 3: Full build**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && make build`

**Step 4: Bump version headers on all modified files**

Update version headers in:
- `internal/server/author_dedup.go` → 1.2.0
- `internal/server/author_dedup_test.go` → 1.1.0
- `web/src/pages/BookDedup.tsx` → 2.2.0
- `web/src/services/api.ts` → 1.37.0

**Step 5: Final commit**

```bash
git add -A
git commit -m "chore: version bumps for dedup redesign phase 1"
```
