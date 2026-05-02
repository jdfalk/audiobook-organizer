<!-- file: docs/plans/2026-02-28-phase4b-manual-metadata-matching-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5a6b7c8d-9e0f-1a2b-3c4d-5e6f7a8b9c0d -->
<!-- last-edited: 2026-02-28 -->

# Phase 4B: Manual Metadata Matching UI — Design

## Goal

Let users manually search, review, and select metadata matches for their audiobooks. The current "Fetch Metadata" button auto-selects the best match (or fails silently when nothing scores above 0.35). Users need to see candidates, pick the right one, and optionally cherry-pick which fields to apply.

## Architecture

Two buttons in the BookDetail action bar:

1. **Fetch Metadata** (existing) — auto-applies best match. Used for bulk operations. Unchanged except it now skips books marked "no match".
2. **Search** (new) — opens a dialog showing scored results from all sources. User picks a result or searches again.

### New API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/audiobooks/:id/search-metadata` | Returns scored candidates from all sources. Accepts optional `query` body param (defaults to book title). |
| POST | `/api/v1/audiobooks/:id/apply-metadata` | Applies a selected candidate to the book. Accepts `candidate` object + optional `fields[]` array. |
| POST | `/api/v1/audiobooks/:id/mark-no-match` | Sets `metadata_review_status = 'no_match'`. Bulk fetch skips these books. |

### New DB Column

Migration 24 adds `metadata_review_status TEXT` to the books table. Values: `NULL` (default), `'no_match'`, `'matched'`.

## Backend Changes

### `MetadataCandidate` struct

```go
type MetadataCandidate struct {
    Title          string  `json:"title"`
    Author         string  `json:"author"`
    Narrator       string  `json:"narrator,omitempty"`
    Series         string  `json:"series,omitempty"`
    SeriesPosition int     `json:"series_position,omitempty"`
    Year           int     `json:"year,omitempty"`
    Publisher      string  `json:"publisher,omitempty"`
    ISBN           string  `json:"isbn,omitempty"`
    CoverURL       string  `json:"cover_url,omitempty"`
    Description    string  `json:"description,omitempty"`
    Source         string  `json:"source"`
    Score          float64 `json:"score"`
}
```

### `SearchMetadataForBook(id, query string) ([]MetadataCandidate, error)`

Extracted from the existing multi-step search logic in `FetchMetadataForBook`. Returns ALL scored results (score > 0) from all enabled sources, sorted by score descending. No minimum threshold — the user decides.

### `ApplyMetadataCandidate(bookID string, candidate MetadataCandidate, fields []string) error`

- If `fields` is empty: applies all non-empty fields from the candidate (Quick Apply).
- If `fields` is provided: applies only those fields.
- Uses existing `applyMetadataToBook()` downgrade protection.
- Records change history.
- Sets `metadata_review_status = 'matched'`.

### Bulk fetch change

`FetchMetadataForBook` and `bulkFetchMetadata` skip books where `metadata_review_status = 'no_match'`.

## Frontend Components

### `MetadataSearchDialog` (new component)

MUI Dialog (`fullWidth`, `maxWidth="md"`).

**Layout:**
- **Top:** Search bar pre-filled with book title + author. "Search" button to re-query.
- **Middle:** Result cards (up to 10 from all sources), sorted by score descending. Each card shows:
  - Title, Author, Year
  - Source badge (e.g. "OpenLibrary", "Hardcover")
  - Series name + position if present
  - Cover thumbnail if available
  - Match score as subtle percentage
  - "Apply" button (Quick Apply)
  - "Select fields..." link that expands to field checkboxes (all pre-checked). User unchecks unwanted fields, then clicks "Apply Selected".
- **Bottom:** "No Match Found" button (left), "Cancel" button (right).

**After applying:** Dialog closes, book refreshes, toast confirms source + fields applied.

### BookDetail.tsx changes

- Add Search IconButton (MUI SearchIcon) next to existing "Fetch Metadata" button.
- New state: `metadataSearchOpen: boolean`.
- Opens `MetadataSearchDialog` with current book as prop.

### api.ts additions

- `searchMetadata(bookId: string, query?: string): Promise<{ results: MetadataCandidate[], query: string }>`
- `applyMetadataCandidate(bookId: string, candidate: MetadataCandidate, fields?: string[]): Promise<FetchMetadataResponse>`
- `markNoMatch(bookId: string): Promise<void>`

## Data Flow

```
User clicks "Search" button
  → MetadataSearchDialog opens
  → POST /search-metadata (query = book title + author)
  → Backend queries all enabled sources
  → Returns scored candidates (no threshold)
  → Dialog shows ranked results

User picks a result:
  Quick Apply → POST /apply-metadata { candidate, fields: [] }
  Advanced   → POST /apply-metadata { candidate, fields: ["title", "author", ...] }
  → Backend applies with downgrade protection + history
  → Dialog closes, book refreshes

User clicks "No Match Found":
  → POST /mark-no-match
  → Dialog closes, toast confirms
  → Future bulk fetches skip this book
```

## Testing

- Backend: unit tests for `SearchMetadataForBook` with mock sources, `ApplyMetadataCandidate` field filtering, `mark-no-match` flag behavior.
- Frontend: component test for `MetadataSearchDialog` rendering, search interaction, apply flow.
- E2E: Playwright test for search → select → verify book updated.
