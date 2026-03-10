# Series Management Page Implementation Plan

**Goal:** Create a Series management page where users can browse, search, sort, merge, split, and delete series with their books displayed inline.

**Architecture:** New React page component with MUI data table, backend enrichment endpoint returning series with book counts, merge/split operations using existing endpoints.

**Tech Stack:** React, TypeScript, MUI, existing Go backend endpoints

---

## Backend Changes

### Task 1: Add series list with book counts endpoint

**Files:**
- Modify: `internal/server/author_series_service.go`
- Modify: `internal/server/server.go` (route registration + handler)
- Modify: `internal/database/store.go` (interface)
- Modify: `internal/database/pebble_store.go` (implementation)
- Modify: `internal/database/sqlite_store.go` (implementation)

**What to build:**

Add `GetAllSeriesBookCounts() (map[int]int, error)` to the Store interface (similar to existing `GetAllAuthorBookCounts()`).

In `author_series_service.go`, create a new response type and method:

```go
type SeriesWithCount struct {
    database.Series
    BookCount  int    `json:"book_count"`
    AuthorName string `json:"author_name,omitempty"`
}

type SeriesListWithCountsResponse struct {
    Items []SeriesWithCount `json:"items"`
    Count int               `json:"count"`
}

func (as *AuthorSeriesService) ListSeriesWithCounts() (*SeriesListWithCountsResponse, error)
```

This method should:
1. Call `as.db.GetAllSeries()`
2. Call `as.db.GetAllSeriesBookCounts()` (new method)
3. For each series with an AuthorID, look up the author name
4. Return combined data

Register a new handler at `GET /api/v1/series/with-counts` in `setupRoutes()` around line 1206. Or modify the existing `GET /api/v1/series` handler to include counts. Prefer modifying existing since the frontend will replace its usage anyway.

For PebbleStore: iterate all books, count by SeriesID.
For SQLiteStore: `SELECT series_id, COUNT(*) FROM audiobooks WHERE series_id IS NOT NULL GROUP BY series_id`

### Task 2: Add series rename endpoint

**Files:**
- Modify: `internal/server/server.go`

Check if `PUT /api/v1/series/:id/name` already exists. If not, add it following the pattern of `renameAuthor()`:

```go
func (s *Server) renameSeries(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    var body struct { Name string `json:"name"` }
    c.ShouldBindJSON(&body)
    store.UpdateSeriesName(id, body.Name)
}
```

### Task 3: Add series split endpoint

**Files:**
- Modify: `internal/server/server.go`

Add `POST /api/v1/series/:id/split` that takes `{ book_ids: string[] }` and moves those books to a new series.

```go
func (s *Server) splitSeries(c *gin.Context) {
    // Get series by ID
    // Create new series with same name + " (split)"
    // Move specified book_ids to new series
    // Return new series
}
```

---

## Frontend Changes

### Task 4: Add API functions

**Files:**
- Modify: `web/src/services/api.ts`

Add these functions:

```typescript
export interface SeriesWithCount {
  id: number;
  name: string;
  author_id?: number;
  author_name?: string;
  book_count: number;
  created_at: string;
}

export async function getSeriesWithCounts(): Promise<SeriesWithCount[]> {
  const response = await fetch(`${API_BASE}/series`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch series');
  const data = await response.json();
  return data.items || [];
}

export async function renameSeries(id: number, name: string): Promise<void> {
  const response = await fetch(`${API_BASE}/series/${id}/name`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to rename series');
}

export async function splitSeries(id: number, bookIds: string[]): Promise<void> {
  const response = await fetch(`${API_BASE}/series/${id}/split`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to split series');
}

export async function deleteSeries(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/series/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to delete series');
}

export async function getSeriesBooks(seriesId: number): Promise<Audiobook[]> {
  // Use existing getBooks with series filter, or call GetBooksBySeriesID
  const response = await fetch(`${API_BASE}/series/${seriesId}/books`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch series books');
  const data = await response.json();
  return data.books || [];
}
```

### Task 5: Create Series.tsx page

**Files:**
- Create: `web/src/pages/Series.tsx`

**Structure:**
- Header with title "Series" and search bar
- Sort controls: name (A-Z, Z-A), book count (high-low, low-high)
- Filter: show empty series toggle, min book count
- Data table with columns: Name, Author, Book Count, Actions
- Each row expandable to show books in that series
- Multi-select with checkbox for bulk merge/delete
- Action buttons per row: Rename, Split, Delete
- Bulk action bar: Merge Selected, Delete Selected

**UI Layout:**

```
┌─────────────────────────────────────────────────┐
│ Series                          [Search...] [Sort▼]│
├─────────────────────────────────────────────────┤
│ □ Show empty series  Min books: [___]           │
├─────────────────────────────────────────────────┤
│ [Merge Selected (3)] [Delete Selected (3)]      │
├─────────────────────────────────────────────────┤
│ ☐ ▶ The Expanse          James S.A. Corey   9  │
│ ☐ ▼ Vampire Earth        E.E. Knight        6  │
│     ├ Valentine's Rising                        │
│     ├ Way of the Wolf                           │
│     └ ...                                       │
│ ☐ ▶ Dragon Born          Dante King         3  │
└─────────────────────────────────────────────────┘
```

Use MUI components:
- `Table` / `TableBody` / `TableRow` for the list
- `Collapse` for expandable book lists
- `Checkbox` for selection
- `IconButton` with `KeyboardArrowDown`/`KeyboardArrowRight` for expand
- `TextField` for search
- `Select` for sort
- `Dialog` for merge confirmation and rename
- `Chip` for book count badges
- `Pagination` from MUI for paginating (client-side is fine since series count is manageable)

**Key interactions:**
1. Click row → expand to show books
2. Click "Rename" → inline edit or dialog
3. Select multiple → "Merge" button appears → dialog asks which to keep
4. Click "Split" on expanded series → checkboxes appear on books → select books to split out
5. Click "Delete" on empty series → confirmation dialog
6. Search filters in real-time (client-side filter)

### Task 6: Add route and sidebar entry

**Files:**
- Modify: `web/src/App.tsx` — add `import { Series } from './pages/Series'` and `<Route path="/series" element={<Series />} />`
- Modify: `web/src/components/layout/Sidebar.tsx` — add `{ text: 'Series', icon: <MenuBookIcon />, path: '/series' }` after the Library entry

### Task 7: Wire up merge dialog

The merge flow:
1. User selects 2+ series via checkboxes
2. Clicks "Merge Selected"
3. Dialog shows selected series with radio buttons to pick the "keep" series
4. Confirm → calls `api.mergeSeriesGroup(keepId, mergeIds)`
5. Refresh list on success

Use existing `POST /api/v1/series/merge` endpoint which already accepts `{ keep_id, merge_ids }`.
