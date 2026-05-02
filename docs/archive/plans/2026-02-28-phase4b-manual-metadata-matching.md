<!-- file: docs/plans/2026-02-28-phase4b-manual-metadata-matching.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6b7c8d9e-0f1a-2b3c-4d5e-6f7a8b9c0d1e -->
<!-- last-edited: 2026-02-28 -->

# Phase 4B: Manual Metadata Matching UI — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a "Search" button that opens a dialog showing scored metadata results from all sources, letting users pick the right match or search manually.

**Architecture:** New search-metadata/apply-metadata/mark-no-match API endpoints backed by refactored search logic in MetadataFetchService. New MetadataSearchDialog React component. Migration 24 adds metadata_review_status column.

**Tech Stack:** Go (Gin), React/TypeScript (MUI), SQLite

---

## Task 1: Migration 24 — Add `metadata_review_status` column

**Files:**
- Modify: `internal/database/migrations.go` (after line 179)
- Modify: `internal/database/sqlite_store.go` (bookSelectColumns, scanBook)
- Modify: `internal/database/store.go` (Book struct)

**Step 1: Add field to Book struct**

In `internal/database/store.go`, find the Book struct (around line 235). Add after the `LastWrittenAt` field (around line 290):

```go
MetadataReviewStatus *string    `json:"metadata_review_status,omitempty"` // null, "no_match", "matched"
```

**Step 2: Add migration 24**

In `internal/database/migrations.go`, add after the migration 23 entry (line 179):

```go
{
    Version:     24,
    Description: "Add metadata_review_status column to books",
    Up:          migration024Up,
    Down:        nil,
},
```

Add the migration function at the bottom of the file:

```go
func migration024Up(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE books ADD COLUMN metadata_review_status TEXT`)
	return err
}
```

**Step 3: Update sqlite_store.go**

In `internal/database/sqlite_store.go`, find `bookSelectColumns` (a string constant listing all SELECT columns). Add `metadata_review_status` to the end of the column list.

Find `scanBook` function. Add `&book.MetadataReviewStatus` to the scan targets, in the same position as the column was added.

Find the `UPDATE books SET` query in `UpdateBook`. Add `metadata_review_status = ?` to the SET clause and add `book.MetadataReviewStatus` to the args slice.

**Step 4: Verify**

Run: `go build ./...`
Expected: Clean build.

Run: `go test ./internal/database/... -count=1 -timeout 60s`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/database/migrations.go internal/database/sqlite_store.go internal/database/store.go
git commit -m "feat: add metadata_review_status column (migration 24)"
```

---

## Task 2: `MetadataCandidate` struct and `SearchMetadataForBook` method

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

**Step 1: Add MetadataCandidate struct**

Add near the top of `metadata_fetch_service.go`, after the existing `FetchMetadataResponse` struct:

```go
// MetadataCandidate represents a single metadata match from any source.
// Returned by SearchMetadataForBook for user selection.
type MetadataCandidate struct {
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	Narrator       string  `json:"narrator,omitempty"`
	Series         string  `json:"series,omitempty"`
	SeriesPosition string  `json:"series_position,omitempty"`
	Year           int     `json:"year,omitempty"`
	Publisher      string  `json:"publisher,omitempty"`
	ISBN           string  `json:"isbn,omitempty"`
	CoverURL       string  `json:"cover_url,omitempty"`
	Description    string  `json:"description,omitempty"`
	Language       string  `json:"language,omitempty"`
	Source         string  `json:"source"`
	Score          float64 `json:"score"`
}

// SearchMetadataResponse is the response from the search-metadata endpoint.
type SearchMetadataResponse struct {
	Results []MetadataCandidate `json:"results"`
	Query   string              `json:"query"`
}
```

**Step 2: Add SearchMetadataForBook method**

This method reuses the existing multi-step search logic but collects ALL results from ALL sources instead of stopping at the first match. It scores them but does not apply a minimum threshold.

```go
// SearchMetadataForBook searches all enabled metadata sources and returns
// scored candidates for user selection. Does not apply any changes.
func (mfs *MetadataFetchService) SearchMetadataForBook(id string, query string) (*SearchMetadataResponse, error) {
	book, err := mfs.store.GetBook(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("book not found: %s", id)
	}

	// Use provided query or fall back to book title.
	searchTitle := query
	if searchTitle == "" {
		searchTitle = stripChapterFromTitle(book.Title)
	}

	// Resolve author name for search.
	authorName := ""
	if book.AuthorID != nil {
		if author, err := mfs.store.GetAuthor(*book.AuthorID); err == nil && author != nil {
			authorName = author.Name
		}
	}
	if authorName == "" {
		authorName = book.Author
	}

	sources := mfs.buildSourceChain()
	var allCandidates []MetadataCandidate

	for _, src := range sources {
		sourceName := src.Name()
		var results []metadata.BookMetadata

		// Try multiple search strategies and collect all results.
		seen := make(map[string]bool) // dedupe by title+author

		tryAdd := func(r []metadata.BookMetadata) {
			for _, bm := range r {
				key := strings.ToLower(bm.Title + "|" + bm.Author)
				if seen[key] {
					continue
				}
				seen[key] = true
				results = append(results, bm)
			}
		}

		if r, err := src.SearchByTitle(searchTitle); err == nil {
			tryAdd(r)
		}
		if searchTitle != book.Title {
			if r, err := src.SearchByTitle(book.Title); err == nil {
				tryAdd(r)
			}
		}
		if authorName != "" {
			if r, err := src.SearchByTitleAndAuthor(searchTitle, authorName); err == nil {
				tryAdd(r)
			}
		}

		// Score each result.
		searchWords := significantWords(searchTitle)
		for _, r := range results {
			score := scoreOneResult(r, searchWords)
			candidate := MetadataCandidate{
				Title:          r.Title,
				Author:         r.Author,
				Narrator:       r.Narrator,
				Series:         r.Series,
				SeriesPosition: r.SeriesPosition,
				Year:           r.PublishYear,
				Publisher:      r.Publisher,
				ISBN:           r.ISBN,
				CoverURL:       r.CoverURL,
				Description:    r.Description,
				Language:       r.Language,
				Source:         sourceName,
				Score:          score,
			}
			if score > 0 {
				allCandidates = append(allCandidates, candidate)
			}
		}
	}

	// Sort by score descending.
	sort.Slice(allCandidates, func(i, j int) bool {
		return allCandidates[i].Score > allCandidates[j].Score
	})

	// Cap at 10 results.
	if len(allCandidates) > 10 {
		allCandidates = allCandidates[:10]
	}

	return &SearchMetadataResponse{
		Results: allCandidates,
		Query:   searchTitle,
	}, nil
}
```

Make sure `"sort"` and `"strings"` are in the import block (they likely already are).

**Step 3: Verify**

Run: `go build ./...`
Expected: Clean build.

**Step 4: Commit**

```bash
git add internal/server/metadata_fetch_service.go
git commit -m "feat: add MetadataCandidate struct and SearchMetadataForBook"
```

---

## Task 3: `ApplyMetadataCandidate` and `MarkNoMatch` methods

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

**Step 1: Add ApplyMetadataCandidate method**

```go
// ApplyMetadataCandidate applies a user-selected metadata candidate to a book.
// If fields is empty, all non-empty fields are applied.
// If fields is non-empty, only those fields are applied.
func (mfs *MetadataFetchService) ApplyMetadataCandidate(id string, candidate MetadataCandidate, fields []string) (*FetchMetadataResponse, error) {
	book, err := mfs.store.GetBook(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("book not found: %s", id)
	}

	// Convert candidate to BookMetadata for applyMetadataToBook.
	bm := metadata.BookMetadata{
		Title:          candidate.Title,
		Author:         candidate.Author,
		Narrator:       candidate.Narrator,
		Series:         candidate.Series,
		SeriesPosition: candidate.SeriesPosition,
		Publisher:       candidate.Publisher,
		PublishYear:    candidate.Year,
		ISBN:           candidate.ISBN,
		CoverURL:       candidate.CoverURL,
		Description:    candidate.Description,
		Language:       candidate.Language,
	}

	// If fields filter is specified, zero out fields not in the list.
	if len(fields) > 0 {
		fieldSet := make(map[string]bool, len(fields))
		for _, f := range fields {
			fieldSet[f] = true
		}
		if !fieldSet["title"] {
			bm.Title = ""
		}
		if !fieldSet["author"] {
			bm.Author = ""
		}
		if !fieldSet["narrator"] {
			bm.Narrator = ""
		}
		if !fieldSet["series"] {
			bm.Series = ""
			bm.SeriesPosition = ""
		}
		if !fieldSet["year"] {
			bm.PublishYear = 0
		}
		if !fieldSet["publisher"] {
			bm.Publisher = ""
		}
		if !fieldSet["isbn"] {
			bm.ISBN = ""
		}
		if !fieldSet["cover"] {
			bm.CoverURL = ""
		}
		if !fieldSet["description"] {
			bm.Description = ""
		}
		if !fieldSet["language"] {
			bm.Language = ""
		}
	}

	mfs.applyMetadataToBook(book, bm)

	// Set review status to matched.
	matched := "matched"
	book.MetadataReviewStatus = &matched

	updated, err := mfs.store.UpdateBook(book.ID, book)
	if err != nil {
		return nil, fmt.Errorf("failed to update book: %w", err)
	}

	// Record change history.
	mfs.recordChangeHistory(book, bm, candidate.Source)

	// Download cover if available.
	if bm.CoverURL != "" {
		mfs.downloadAndSetCover(updated, bm.CoverURL)
	}

	return &FetchMetadataResponse{
		Message: fmt.Sprintf("Metadata applied from %s", candidate.Source),
		Book:    updated,
		Source:  candidate.Source,
	}, nil
}
```

**Step 2: Add MarkNoMatch method**

```go
// MarkNoMatch sets the book's metadata_review_status to "no_match".
// Bulk fetch operations will skip books with this status.
func (mfs *MetadataFetchService) MarkNoMatch(id string) error {
	book, err := mfs.store.GetBook(id)
	if err != nil || book == nil {
		return fmt.Errorf("book not found: %s", id)
	}

	status := "no_match"
	book.MetadataReviewStatus = &status
	_, err = mfs.store.UpdateBook(book.ID, book)
	return err
}
```

**Step 3: Update FetchMetadataForBook to skip no_match books**

In `FetchMetadataForBook` (around line 101), after the book is loaded from the database, add:

```go
	if book.MetadataReviewStatus != nil && *book.MetadataReviewStatus == "no_match" {
		return nil, fmt.Errorf("book %q is marked as no-match; use search-metadata to re-evaluate", book.Title)
	}
```

**Step 4: Verify**

Run: `go build ./...`
Expected: Clean build.

**Step 5: Commit**

```bash
git add internal/server/metadata_fetch_service.go
git commit -m "feat: add ApplyMetadataCandidate and MarkNoMatch methods"
```

---

## Task 4: API route handlers

**Files:**
- Modify: `internal/server/server.go`

**Step 1: Add handler functions**

Add these three handler functions near the existing `fetchAudiobookMetadata` handler (around line 3289):

```go
// searchAudiobookMetadata returns scored metadata candidates for user selection.
func (s *Server) searchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var body struct {
		Query string `json:"query"`
	}
	// Bind body if present; query is optional.
	_ = c.ShouldBindJSON(&body)

	resp, err := s.metadataFetchService.SearchMetadataForBook(id, body.Query)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// applyAudiobookMetadata applies a user-selected metadata candidate.
func (s *Server) applyAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var body struct {
		Candidate MetadataCandidate `json:"candidate"`
		Fields    []string          `json:"fields"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	resp, err := s.metadataFetchService.ApplyMetadataCandidate(id, body.Candidate, body.Fields)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if GlobalWriteBackBatcher != nil {
		GlobalWriteBackBatcher.Enqueue(id)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": resp.Message,
		"book":    resp.Book,
		"source":  resp.Source,
	})
}

// markAudiobookNoMatch marks a book as having no metadata match.
func (s *Server) markAudiobookNoMatch(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	if err := s.metadataFetchService.MarkNoMatch(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Book marked as no match"})
}
```

**Step 2: Register routes**

Find the route registration block (around line 1137 where `fetch-metadata` is registered). Add after it:

```go
		protected.POST("/audiobooks/:id/search-metadata", s.searchAudiobookMetadata)
		protected.POST("/audiobooks/:id/apply-metadata", s.applyAudiobookMetadata)
		protected.POST("/audiobooks/:id/mark-no-match", s.markAudiobookNoMatch)
```

**Step 3: Bump version header**

Change the version in `internal/server/server.go` line 2.

**Step 4: Verify**

Run: `go build ./...`
Expected: Clean build.

Run: `go test ./internal/server/... -count=1 -timeout 120s`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/server/server.go
git commit -m "feat: add search-metadata, apply-metadata, mark-no-match API routes"
```

---

## Task 5: Backend tests

**Files:**
- Create: `internal/server/metadata_search_test.go`

**Step 1: Write tests**

```go
// file: internal/server/metadata_search_test.go
// version: 1.0.0
// guid: 7c8d9e0f-1a2b-3c4d-5e6f-7a8b9c0d1e2f

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchMetadata_ReturnsResults(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore

	bookID := ulid.Make().String()
	book := &database.Book{
		ID:       bookID,
		Title:    "The Long Cosmos",
		Author:   "Terry Pratchett",
		FilePath: "/tmp/search_test.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	// Search endpoint should return 200 (even if no external sources return results)
	body := `{"query":"The Long Cosmos"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+bookID+"/search-metadata", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp SearchMetadataResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "The Long Cosmos", resp.Query)
	// Results may be empty in test (no real sources), but structure is valid.
	assert.NotNil(t, resp.Results)
}

func TestSearchMetadata_BookNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/nonexistent/search-metadata", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMarkNoMatch_SetsStatus(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore

	bookID := ulid.Make().String()
	book := &database.Book{
		ID:       bookID,
		Title:    "Unknown Book",
		FilePath: "/tmp/nomatch_test.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+bookID+"/mark-no-match", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify status was set in DB.
	updated, err := store.GetBook(bookID)
	require.NoError(t, err)
	require.NotNil(t, updated.MetadataReviewStatus)
	assert.Equal(t, "no_match", *updated.MetadataReviewStatus)
}

func TestApplyMetadata_AppliesCandidate(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore

	bookID := ulid.Make().String()
	book := &database.Book{
		ID:       bookID,
		Title:    "Old Title",
		FilePath: "/tmp/apply_test.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	body := `{
		"candidate": {
			"title": "New Title",
			"author": "New Author",
			"source": "test",
			"score": 0.95
		},
		"fields": []
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+bookID+"/apply-metadata", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify book was updated.
	updated, err := store.GetBook(bookID)
	require.NoError(t, err)
	assert.Equal(t, "New Title", updated.Title)
}

func TestApplyMetadata_FieldFiltering(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	store := database.GlobalStore

	bookID := ulid.Make().String()
	book := &database.Book{
		ID:       bookID,
		Title:    "Keep This Title",
		Author:   "Keep This Author",
		FilePath: "/tmp/field_filter_test.m4b",
		Format:   "m4b",
	}
	_, err := store.CreateBook(book)
	require.NoError(t, err)

	// Only apply narrator field.
	body := `{
		"candidate": {
			"title": "Different Title",
			"author": "Different Author",
			"narrator": "New Narrator",
			"source": "test",
			"score": 0.9
		},
		"fields": ["narrator"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+bookID+"/apply-metadata", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	updated, err := store.GetBook(bookID)
	require.NoError(t, err)
	// Title and author should be unchanged.
	assert.Equal(t, "Keep This Title", updated.Title)
	// Narrator should be updated (stored as pointer field).
}
```

**Step 2: Run tests**

Run: `go test ./internal/server/... -v -run "TestSearchMetadata|TestMarkNoMatch|TestApplyMetadata" -count=1 -timeout 120s`
Expected: All pass.

**Step 3: Commit**

```bash
git add internal/server/metadata_search_test.go
git commit -m "test: add backend tests for search/apply/mark-no-match metadata"
```

---

## Task 6: Frontend API client methods

**Files:**
- Modify: `web/src/services/api.ts`

**Step 1: Add TypeScript interfaces**

Find the existing type definitions section in `api.ts` (near the top where `Book`, `BookSegment`, etc. are defined). Add `metadata_review_status` to the `Book` type if not already present.

Add these new interfaces:

```typescript
export interface MetadataCandidate {
  title: string;
  author: string;
  narrator?: string;
  series?: string;
  series_position?: string;
  year?: number;
  publisher?: string;
  isbn?: string;
  cover_url?: string;
  description?: string;
  language?: string;
  source: string;
  score: number;
}

export interface SearchMetadataResponse {
  results: MetadataCandidate[];
  query: string;
}
```

**Step 2: Add API functions**

Add near the existing `fetchBookMetadata` function (around line 1295):

```typescript
export async function searchMetadata(
  bookId: string,
  query?: string
): Promise<SearchMetadataResponse> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/search-metadata`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: query || '' }),
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to search metadata');
  }
  return response.json();
}

export async function applyMetadataCandidate(
  bookId: string,
  candidate: MetadataCandidate,
  fields?: string[]
): Promise<{ message: string; book: Book; source: string }> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/apply-metadata`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ candidate, fields: fields || [] }),
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to apply metadata');
  }
  return response.json();
}

export async function markNoMatch(bookId: string): Promise<void> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/mark-no-match`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to mark as no match');
  }
}
```

**Step 3: Verify**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors.

**Step 4: Commit**

```bash
git add web/src/services/api.ts
git commit -m "feat: add searchMetadata, applyMetadataCandidate, markNoMatch API client"
```

---

## Task 7: MetadataSearchDialog component

**Files:**
- Create: `web/src/components/audiobooks/MetadataSearchDialog.tsx`

**Step 1: Create the component**

```tsx
// file: web/src/components/audiobooks/MetadataSearchDialog.tsx
// version: 1.0.0
// guid: 8d9e0f1a-2b3c-4d5e-6f7a-8b9c0d1e2f3a

import React, { useState, useEffect, useCallback } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Card,
  CardContent,
  CardActions,
  Typography,
  Chip,
  Collapse,
  FormControlLabel,
  Checkbox,
  CircularProgress,
  InputAdornment,
  IconButton,
  Avatar,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import * as api from '../../services/api';
import type { Book, MetadataCandidate } from '../../services/api';

interface MetadataSearchDialogProps {
  open: boolean;
  book: Book;
  onClose: () => void;
  onApplied: (updatedBook: Book) => void;
  toast: (message: string, severity: 'success' | 'error' | 'info') => void;
}

const ALL_FIELDS = [
  'title',
  'author',
  'narrator',
  'series',
  'year',
  'publisher',
  'isbn',
  'cover',
  'description',
  'language',
];

const sourceColors: Record<string, string> = {
  'Open Library': '#3d85c6',
  'Google Books': '#4285f4',
  Audnexus: '#ff9900',
  Hardcover: '#6b4c9a',
};

export default function MetadataSearchDialog({
  open,
  book,
  onClose,
  onApplied,
  toast,
}: MetadataSearchDialogProps) {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<MetadataCandidate[]>([]);
  const [loading, setLoading] = useState(false);
  const [expandedCard, setExpandedCard] = useState<number | null>(null);
  const [selectedFields, setSelectedFields] = useState<Record<number, Set<string>>>({});
  const [applying, setApplying] = useState<number | null>(null);

  // Pre-populate query when dialog opens.
  useEffect(() => {
    if (open && book) {
      const q = [book.title, book.author].filter(Boolean).join(' - ');
      setQuery(q);
      setResults([]);
      setExpandedCard(null);
      setSelectedFields({});
      // Auto-search on open.
      doSearch(q);
    }
  }, [open, book?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const doSearch = useCallback(
    async (searchQuery?: string) => {
      if (!book) return;
      setLoading(true);
      try {
        const resp = await api.searchMetadata(book.id, searchQuery || query);
        setResults(resp.results || []);
        if ((resp.results || []).length === 0) {
          toast('No results found. Try a different search.', 'info');
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Search failed';
        toast(msg, 'error');
      } finally {
        setLoading(false);
      }
    },
    [book, query, toast]
  );

  const handleApply = async (index: number, fieldsOverride?: string[]) => {
    const candidate = results[index];
    if (!candidate || !book) return;
    setApplying(index);
    try {
      const fields = fieldsOverride || [];
      const resp = await api.applyMetadataCandidate(book.id, candidate, fields);
      toast(resp.message || 'Metadata applied.', 'success');
      onApplied(resp.book);
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Apply failed';
      toast(msg, 'error');
    } finally {
      setApplying(null);
    }
  };

  const handleApplySelected = (index: number) => {
    const fields = selectedFields[index];
    if (!fields || fields.size === 0) return;
    handleApply(index, Array.from(fields));
  };

  const handleMarkNoMatch = async () => {
    if (!book) return;
    try {
      await api.markNoMatch(book.id);
      toast('Book marked as no match. Bulk fetch will skip it.', 'info');
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to mark';
      toast(msg, 'error');
    }
  };

  const toggleExpanded = (index: number) => {
    if (expandedCard === index) {
      setExpandedCard(null);
    } else {
      setExpandedCard(index);
      // Initialize all fields as selected.
      if (!selectedFields[index]) {
        setSelectedFields((prev) => ({
          ...prev,
          [index]: new Set(ALL_FIELDS),
        }));
      }
    }
  };

  const toggleField = (index: number, field: string) => {
    setSelectedFields((prev) => {
      const current = new Set(prev[index] || ALL_FIELDS);
      if (current.has(field)) {
        current.delete(field);
      } else {
        current.add(field);
      }
      return { ...prev, [index]: current };
    });
  };

  const getCandidateFieldValue = (c: MetadataCandidate, field: string): string => {
    switch (field) {
      case 'title': return c.title || '';
      case 'author': return c.author || '';
      case 'narrator': return c.narrator || '';
      case 'series': return [c.series, c.series_position].filter(Boolean).join(' #');
      case 'year': return c.year ? String(c.year) : '';
      case 'publisher': return c.publisher || '';
      case 'isbn': return c.isbn || '';
      case 'cover': return c.cover_url ? 'Yes' : '';
      case 'description': return c.description ? c.description.slice(0, 80) + '...' : '';
      case 'language': return c.language || '';
      default: return '';
    }
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>Search Metadata</DialogTitle>
      <DialogContent>
        {/* Search bar */}
        <Box sx={{ display: 'flex', gap: 1, mb: 2, mt: 1 }}>
          <TextField
            fullWidth
            size="small"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && doSearch()}
            placeholder="Search by title, author, ISBN..."
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={() => doSearch()} size="small">
                    <SearchIcon />
                  </IconButton>
                </InputAdornment>
              ),
            }}
          />
        </Box>

        {/* Loading indicator */}
        {loading && (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress />
          </Box>
        )}

        {/* Results */}
        {!loading &&
          results.map((candidate, idx) => (
            <Card key={idx} variant="outlined" sx={{ mb: 1.5 }}>
              <CardContent sx={{ pb: 1, display: 'flex', gap: 2 }}>
                {/* Cover thumbnail */}
                {candidate.cover_url ? (
                  <Avatar
                    variant="rounded"
                    src={candidate.cover_url}
                    sx={{ width: 56, height: 80 }}
                  />
                ) : (
                  <Avatar variant="rounded" sx={{ width: 56, height: 80, bgcolor: 'grey.300' }}>
                    ?
                  </Avatar>
                )}

                {/* Info */}
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Typography variant="subtitle1" noWrap fontWeight="bold">
                    {candidate.title}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {candidate.author}
                    {candidate.year ? ` (${candidate.year})` : ''}
                  </Typography>
                  {candidate.series && (
                    <Typography variant="body2" color="text.secondary">
                      Series: {candidate.series}
                      {candidate.series_position ? ` #${candidate.series_position}` : ''}
                    </Typography>
                  )}
                  {candidate.narrator && (
                    <Typography variant="caption" color="text.secondary">
                      Narrated by {candidate.narrator}
                    </Typography>
                  )}
                </Box>

                {/* Score + source */}
                <Box sx={{ textAlign: 'right', flexShrink: 0 }}>
                  <Chip
                    label={candidate.source}
                    size="small"
                    sx={{
                      bgcolor: sourceColors[candidate.source] || 'grey.500',
                      color: 'white',
                      mb: 0.5,
                    }}
                  />
                  <Typography variant="caption" display="block" color="text.secondary">
                    {Math.round(candidate.score * 100)}% match
                  </Typography>
                </Box>
              </CardContent>

              <CardActions sx={{ justifyContent: 'space-between', px: 2, pt: 0 }}>
                <Button
                  size="small"
                  onClick={() => toggleExpanded(idx)}
                  endIcon={expandedCard === idx ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                >
                  Select fields...
                </Button>
                <Button
                  variant="contained"
                  size="small"
                  onClick={() => handleApply(idx)}
                  disabled={applying !== null}
                >
                  {applying === idx ? <CircularProgress size={16} /> : 'Apply'}
                </Button>
              </CardActions>

              {/* Advanced field selection */}
              <Collapse in={expandedCard === idx}>
                <Box sx={{ px: 2, pb: 2 }}>
                  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0 }}>
                    {ALL_FIELDS.map((field) => {
                      const value = getCandidateFieldValue(candidate, field);
                      if (!value) return null;
                      return (
                        <FormControlLabel
                          key={field}
                          control={
                            <Checkbox
                              size="small"
                              checked={selectedFields[idx]?.has(field) ?? true}
                              onChange={() => toggleField(idx, field)}
                            />
                          }
                          label={
                            <Typography variant="body2">
                              {field}: {value}
                            </Typography>
                          }
                          sx={{ mr: 2, mb: 0 }}
                        />
                      );
                    })}
                  </Box>
                  <Button
                    variant="outlined"
                    size="small"
                    sx={{ mt: 1 }}
                    onClick={() => handleApplySelected(idx)}
                    disabled={
                      applying !== null || !selectedFields[idx] || selectedFields[idx].size === 0
                    }
                  >
                    Apply Selected
                  </Button>
                </Box>
              </Collapse>
            </Card>
          ))}

        {/* No results message */}
        {!loading && results.length === 0 && query && (
          <Typography color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
            No results yet. Click the search icon or press Enter.
          </Typography>
        )}
      </DialogContent>

      <DialogActions sx={{ justifyContent: 'space-between', px: 3, pb: 2 }}>
        <Button color="warning" onClick={handleMarkNoMatch}>
          No Match Found
        </Button>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  );
}
```

**Step 2: Verify**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors.

**Step 3: Commit**

```bash
git add web/src/components/audiobooks/MetadataSearchDialog.tsx
git commit -m "feat: add MetadataSearchDialog component for manual metadata matching"
```

---

## Task 8: Integrate MetadataSearchDialog into BookDetail

**Files:**
- Modify: `web/src/pages/BookDetail.tsx`

**Step 1: Add import**

At the top of `BookDetail.tsx`, add:

```tsx
import MetadataSearchDialog from '../components/audiobooks/MetadataSearchDialog';
import SearchIcon from '@mui/icons-material/Search';
```

Note: `SearchIcon` may conflict if already imported. Check existing imports first.

**Step 2: Add state**

Inside the `BookDetail` component, near the existing `fetchingMetadata` state (around line 50), add:

```tsx
const [metadataSearchOpen, setMetadataSearchOpen] = useState(false);
```

**Step 3: Add the Search button**

Find the "Fetch Metadata" `<Button>` (around line 874). Add immediately after it:

```tsx
<IconButton
  color="primary"
  onClick={() => setMetadataSearchOpen(true)}
  title="Search metadata manually"
  disabled={actionLoading}
>
  <SearchIcon />
</IconButton>
```

If `IconButton` is not imported from MUI, add it to the existing MUI import.

**Step 4: Add the dialog**

Find the end of the JSX return, just before the final closing tags. Add:

```tsx
{book && (
  <MetadataSearchDialog
    open={metadataSearchOpen}
    book={book}
    onClose={() => setMetadataSearchOpen(false)}
    onApplied={(updatedBook) => {
      setBook(updatedBook);
      loadTags();
    }}
    toast={toast}
  />
)}
```

**Step 5: Verify**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors.

Run: `cd web && npm run build`
Expected: Builds successfully.

**Step 6: Commit**

```bash
git add web/src/pages/BookDetail.tsx
git commit -m "feat: add Search button and MetadataSearchDialog to BookDetail"
```

---

## Task 9: Update TODO.md and bump version

**Files:**
- Modify: `TODO.md`

**Step 1: Update Phase 4B status**

Change the Phase 4B items from `🔴` to `🟢` and add plan link:

```markdown
### Phase 4B: Manual Metadata Matching UI

| Item | Status | Plan |
| --- | --- | --- |
| Show top 10 scored results to user, let them pick or search | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
| Search from UI (title/author/ISBN) | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
| "No match" option that marks book as manually reviewed | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
| Field-level apply (Advanced mode with checkboxes) | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
```

**Step 2: Commit**

```bash
git add TODO.md
git commit -m "docs: update Phase 4B status in TODO"
```
