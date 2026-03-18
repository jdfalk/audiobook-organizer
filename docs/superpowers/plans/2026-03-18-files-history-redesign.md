# Files & History Tab Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the book detail "Files & Versions" tab into "Files & History" with format-grouped trays, tag comparison dropdown, change log timeline, and multi-file segment tables.

**Architecture:** Backend adds a changelog API endpoint and comparison param to the tags endpoint. Frontend refactors BookDetail.tsx's file tab into new components: FormatTray, TagComparison, SegmentTable, and ChangeLog.

**Tech Stack:** Go (gin), React/TypeScript/MUI, existing tag extraction via ffprobe

**Spec:** `docs/superpowers/specs/2026-03-18-files-history-redesign.md`

---

## Chunk 1: Backend — Changelog API + Tag Comparison

### Task 1: Add changelog API endpoint

**Files:**
- Create: `internal/server/changelog_service.go`
- Modify: `internal/server/server.go` (route registration ~line 1200, handler)
- Test: `internal/server/changelog_service_test.go`

- [ ] **Step 1: Create ChangelogService**

`internal/server/changelog_service.go`:
- `ChangeLogEntry` struct: `Timestamp time.Time`, `Type string` (tag_write|rename|metadata_apply|import|transcode), `Summary string`, `Details map[string]any`
- `GetBookChangelog(bookID string) ([]ChangeLogEntry, error)` — merges data from:
  - `book_path_history` (rename entries)
  - `metadata_history` (metadata_apply and tag_write entries)
  - `operation_changes` (import/transcode entries)
- Sort by timestamp descending
- Limit to 50 most recent entries

- [ ] **Step 2: Register route and handler**

In `server.go` route block (~line 1200):
```go
protected.GET("/audiobooks/:id/changelog", s.getBookChangelog)
```

Handler:
```go
func (s *Server) getBookChangelog(c *gin.Context) {
    id := c.Param("id")
    entries, err := s.changelogService.GetBookChangelog(id)
    // return {"entries": entries}
}
```

- [ ] **Step 3: Test and commit**

```bash
go test ./internal/server/... -run TestChangelog -v
git commit -m "feat(api): add book changelog endpoint merging path/metadata/operation history"
```

---

### Task 2: Add tag comparison param to tags endpoint

**Files:**
- Modify: `internal/server/audiobook_service.go:332` (GetAudiobookTags)
- Modify: `internal/server/server.go:2767` (getAudiobookTags handler)
- Modify: `internal/server/server.go:384` (buildMetadataProvenance)

- [ ] **Step 1: Add compare_id query param to handler**

In `getAudiobookTags` handler, read `compare_id` from query params:
```go
compareID := c.Query("compare_id")
```

Pass to `GetAudiobookTags(ctx, id, compareID)`.

- [ ] **Step 2: Load comparison metadata**

In `GetAudiobookTags`, if `compareID != ""`:
- Load comparison book by ID
- Extract metadata from comparison book's file via `metadata.ExtractMetadata`
- Add `comparison_value` to each `MetadataProvenanceEntry`

- [ ] **Step 3: Update buildMetadataProvenance**

Add optional `comparisonMeta *metadata.Metadata` parameter. When non-nil, add `ComparisonValue` field to each entry.

- [ ] **Step 4: Test and commit**

```bash
go test ./internal/server/... -run TestAudiobookTags -v
git commit -m "feat(api): add compare_id param to tags endpoint for side-by-side comparison"
```

---

## Chunk 2: Frontend — Core Components

### Task 3: Create TagComparison component

**Files:**
- Create: `web/src/components/TagComparison.tsx`
- Modify: `web/src/services/api.ts:693` (add compare_id param to getBookTags)

- [ ] **Step 1: Update API function**

In `api.ts`, update `getBookTags`:
```typescript
export async function getBookTags(bookId: string, compareId?: string): Promise<...> {
  const params = compareId ? `?compare_id=${compareId}` : '';
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/tags${params}`);
  ...
}

export async function getBookChangelog(bookId: string): Promise<{entries: ChangeLogEntry[]}> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/changelog`);
  ...
}
```

- [ ] **Step 2: Create TagComparison.tsx**

Props: `bookId: string`, `versions: Book[]`, `snapshots: any[]`

Component:
- Dropdown selector: "Compare against: [None | version names | snapshot dates]"
- When selection changes, calls `getBookTags(bookId, selectedCompareId)`
- Renders table with 3 or 4 columns depending on comparison selection
- Diff highlighting: amber row background for different values, green for present-but-missing, red text for comparison values that differ
- Key tag badges at top (✓ title, ✓ author, ✗ isbn)

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(ui): add TagComparison component with dropdown comparison"
```

---

### Task 4: Create ChangeLog component

**Files:**
- Create: `web/src/components/ChangeLog.tsx`

- [ ] **Step 1: Create ChangeLog.tsx**

Props: `bookId: string`

Component:
- Calls `api.getBookChangelog(bookId)` on mount
- Renders timeline: each entry is a row with:
  - Timestamp (formatted)
  - Icon by type (🏷️ tag_write, 📁 rename, 📥 metadata_apply, 📦 import, 🔄 transcode)
  - Summary text
  - "Compare snapshot →" link for tag_write entries
- Newest first, max 50 entries

- [ ] **Step 2: Commit**

```bash
git commit -m "feat(ui): add ChangeLog timeline component"
```

---

## Chunk 3: Frontend — BookDetail Refactor

### Task 5: Refactor BookDetail Files tab

**Files:**
- Modify: `web/src/pages/BookDetail.tsx:1117-1584` (entire files tab section)
- Modify: `web/src/services/api.ts` (add types)

- [ ] **Step 1: Rename tab**

Change tab label from "FILES & VERSIONS (N)" to "FILES & HISTORY".

- [ ] **Step 2: Group versions by format**

Instead of rendering one tray per version, group by format:
```typescript
const formatGroups = useMemo(() => {
  const groups = new Map<string, Book[]>();
  for (const v of versions) {
    const fmt = v.format || 'unknown';
    if (!groups.has(fmt)) groups.set(fmt, []);
    groups.get(fmt)!.push(v);
  }
  return groups;
}, [versions]);
```

Render one expanding tray per format group.

- [ ] **Step 3: Multi-file segment table**

For format groups with segments, show a compact table inside the tray:
- Columns: #, File, Duration, Size
- Collapsed by default with "show all N files" toggle
- Overall metadata summary from first file with ≠ DB indicators

- [ ] **Step 4: Integrate TagComparison**

Replace the inline embedded tags table with `<TagComparison>` component.
Add "View full tag comparison →" link that toggles the component visibility.

- [ ] **Step 5: Integrate ChangeLog**

Add `<ChangeLog bookId={book.id} />` below the Formats section, with a section header "📝 Change Log".

- [ ] **Step 6: iTunes link banner**

Below format trays, show iTunes PID info banner:
```tsx
{itunesLinked && (
  <Alert severity="info" variant="outlined" sx={{mt:1}}>
    iTunes Linked — {itunesPidCount} PIDs mapped
  </Alert>
)}
```

- [ ] **Step 7: Test and commit**

```bash
cd web && npx tsc --noEmit
git commit -m "feat(ui): refactor Files tab to Files & History with format grouping and changelog"
```

---

## Chunk 4: E2E Tests + Deploy

### Task 6: E2E tests

**Files:**
- Create: `web/tests/e2e/files-history.spec.ts`

- [ ] **Step 1: Write E2E tests**

Tests using Phase 2 (mocked routes):
1. Tab shows "FILES & HISTORY" label
2. Format trays group by format (mock 2 versions with different formats)
3. Tag comparison dropdown appears and adds 4th column
4. Change Log section renders timeline entries
5. Multi-file format shows segment table

- [ ] **Step 2: Commit**

```bash
git commit -m "test(e2e): add Files & History tab tests"
```

---

### Task 7: Final build + deploy

- [ ] **Step 1: Run full test suite**

```bash
go test ./... && cd web && npx tsc --noEmit
```

- [ ] **Step 2: Deploy**

```bash
make deploy
```
