# Authors Management Page Implementation Plan

**Goal:** Create an Authors management page where users can browse, search, sort, merge, split, and manage author aliases, with books displayed inline per author.

**Architecture:** New React page component with MUI data table, backend enrichment using existing `GetAllAuthorBookCounts()`, merge/split/alias operations using existing endpoints.

**Tech Stack:** React, TypeScript, MUI, existing Go backend endpoints

---

## Backend Changes

### Task 1: Add authors list with book counts endpoint

**Files:**
- Modify: `internal/server/author_series_service.go`
- Modify: `internal/server/server.go` (handler)

**What to build:**

In `author_series_service.go`, create enriched response:

```go
type AuthorWithCount struct {
    database.Author
    BookCount int              `json:"book_count"`
    Aliases   []database.AuthorAlias `json:"aliases,omitempty"`
}

type AuthorListWithCountsResponse struct {
    Items []AuthorWithCount `json:"items"`
    Count int               `json:"count"`
}

func (as *AuthorSeriesService) ListAuthorsWithCounts() (*AuthorListWithCountsResponse, error)
```

This method should:
1. Call `as.db.GetAllAuthors()`
2. Call `as.db.GetAllAuthorBookCounts()` (already exists)
3. Combine into AuthorWithCount items
4. Return enriched list

Modify the existing `GET /api/v1/authors` handler (`listAuthors` around line 2885 in server.go) to call `ListAuthorsWithCounts()` instead of `ListAuthors()`. This is backward-compatible since we're adding fields.

### Task 2: Add GET /api/v1/authors/:id/books endpoint

**Files:**
- Modify: `internal/server/server.go`

Add handler that calls `store.GetBooksByAuthorID(id)` and returns `{ books: []Book }`. This may already exist as `getBooksByAuthor` — verify and add route if missing.

### Task 3: Add DELETE /api/v1/series/:id and DELETE /api/v1/authors/:id endpoints if missing

**Files:**
- Modify: `internal/server/server.go`

Check if delete endpoints exist. If not, add simple handlers:
```go
func (s *Server) deleteAuthor(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    // Check no books reference this author
    books, _ := store.GetBooksByAuthorID(id)
    if len(books) > 0 {
        c.JSON(400, gin.H{"error": "cannot delete author with books"})
        return
    }
    store.DeleteAuthor(id)
}
```

---

## Frontend Changes

### Task 4: Add API functions

**Files:**
- Modify: `web/src/services/api.ts`

Add/update these functions:

```typescript
export interface AuthorWithCount {
  id: number;
  name: string;
  book_count: number;
  aliases?: AuthorAlias[];
  created_at: string;
}

export async function getAuthorsWithCounts(): Promise<AuthorWithCount[]> {
  const response = await fetch(`${API_BASE}/authors`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch authors');
  const data = await response.json();
  return data.items || [];
}

export async function renameAuthor(id: number, name: string): Promise<void> {
  const response = await fetch(`${API_BASE}/authors/${id}/name`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to rename author');
}

export async function deleteAuthor(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/authors/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to delete author');
}

export async function getAuthorBooks(authorId: number): Promise<Audiobook[]> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/books`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch author books');
  const data = await response.json();
  return data.books || [];
}
```

### Task 5: Create Authors.tsx page

**Files:**
- Create: `web/src/pages/Authors.tsx`

**Structure:**
- Header with title "Authors" and search bar
- Sort controls: name (A-Z, Z-A), book count (high-low, low-high)
- Filter: show zero-book authors toggle, min book count
- Data table with columns: Name, Book Count, Aliases, Actions
- Each row expandable to show books by that author
- Multi-select with checkbox for bulk merge/delete
- Action buttons per row: Rename, Split, Aliases, Delete
- Bulk action bar: Merge Selected, Delete Selected

**UI Layout:**

```
┌──────────────────────────────────────────────────────┐
│ Authors                           [Search...] [Sort▼] │
├──────────────────────────────────────────────────────┤
│ □ Show zero-book authors   Min books: [___]           │
├──────────────────────────────────────────────────────┤
│ [Merge Selected (2)] [Delete Selected (2)]            │
├──────────────────────────────────────────────────────┤
│ ☐ ▶ Brandon Sanderson                    42 books    │
│      Aliases: Brando Sando                            │
│ ☐ ▼ James S.A. Corey                     9 books     │
│     ├ Leviathan Wakes                                 │
│     ├ Caliban's War                                   │
│     └ ...                                             │
│ ☐ ▶ Unknown Author                       234 books   │
└──────────────────────────────────────────────────────┘
```

Use MUI components:
- `Table` / `TableBody` / `TableRow` for the list
- `Collapse` for expandable book lists
- `Checkbox` for selection
- `IconButton` with expand/collapse icons
- `TextField` for search
- `Select` for sort
- `Dialog` for merge confirmation, rename, alias management
- `Chip` for aliases and book count
- `Pagination` for paging

**Key interactions:**
1. Click row → expand to show books
2. Click "Rename" → dialog with text field
3. Select multiple → "Merge" button → dialog with radio to pick canonical author
4. Click "Split" → calls existing `POST /api/v1/authors/:id/split` for composite names
5. Click "Aliases" → dialog showing current aliases, add/remove
6. Click "Delete" on zero-book author → confirmation
7. Search filters in real-time (client-side)

### Task 6: Add route and sidebar entry

**Files:**
- Modify: `web/src/App.tsx` — add `import { Authors } from './pages/Authors'` and `<Route path="/authors" element={<Authors />} />`
- Modify: `web/src/components/layout/Sidebar.tsx` — add `{ text: 'Authors', icon: <PersonIcon />, path: '/authors' }` after Series entry

### Task 7: Wire up merge dialog

The merge flow (same pattern as series):
1. User selects 2+ authors via checkboxes
2. Clicks "Merge Selected"
3. Dialog shows selected authors with radio buttons to pick the canonical author
4. Confirm → calls existing `api.mergeAuthors(keepId, mergeIds)`
5. Refresh list on success

### Task 8: Wire up aliases dialog

1. Click "Aliases" button on an author row
2. Dialog opens showing existing aliases as Chips (with delete X)
3. Text field + "Add" button to create new alias
4. Uses existing `api.getAuthorAliases()`, `api.createAuthorAlias()`, `api.deleteAuthorAlias()`
