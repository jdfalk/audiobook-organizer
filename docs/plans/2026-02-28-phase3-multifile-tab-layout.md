<!-- file: docs/plans/2026-02-28-phase3-multifile-tab-layout.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9a1b2c3d-4e5f-6789-abcd-ef0123456789 -->
<!-- last-edited: 2026-02-28 -->

# Phase 3: Multi-file Tab Layout with File Selector

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add file selector bar for multi-file books that scopes tab content to individual files.
**Architecture:** File selector component + per-file tag API + scoped tab rendering.
**Tech Stack:** React/TypeScript (MUI), Go

---

## Overview

Currently `BookDetail.tsx` loads segments lazily only when the Files tab is
active. The new feature lifts segment loading to component mount, adds a file
selector bar that appears only for multi-file books, and scopes the Info, Tags,
and Compare tabs to show either book-level or file-level data depending on the
selection. The Files tab always shows the full list regardless.

### Layout change (visual)

```
BEFORE (all books):
┌──────────────────────────────────────┐
│ Book Header (title, author, dates)   │
│ Action buttons bar                   │
├──────────────────────────────────────┤
│ INFO | FILES | VERSIONS | TAGS | CMP │
├──────────────────────────────────────┤
│ Tab content (book-level only)        │
└──────────────────────────────────────┘

AFTER (multi-file books only):
┌──────────────────────────────────────┐
│ Book Header (title, author, dates)   │
│ Action buttons bar                   │
├──────────────────────────────────────┤
│ [All Files ▼] | Part 1 | Part 2 ... │   ← NEW: file selector bar
├──────────────────────────────────────┤
│ INFO | FILES | VERSIONS | TAGS | CMP │
├──────────────────────────────────────┤
│ Tab content scoped to selection      │
└──────────────────────────────────────┘

Single-file books: file selector bar hidden, behavior unchanged.
```

---

## Task List

- [ ] Task 1 — Go: Add `GET /api/v1/audiobooks/:id/segments/:segmentId/tags` endpoint
- [ ] Task 2 — TypeScript: Add `SegmentTags` type and `getSegmentTags()` to api.ts
- [ ] Task 3 — React: Lift segment loading to BookDetail mount, add `selectedSegmentId` state
- [ ] Task 4 — React: Build `FileSelector` component
- [ ] Task 5 — React: Render `FileSelector` between action bar and tabs (multi-file only)
- [ ] Task 6 — React: Scope Info tab to show per-file metadata when a segment is selected
- [ ] Task 7 — React: Scope Tags tab to show per-file raw tags when a segment is selected
- [ ] Task 8 — React: Scope Compare tab to show per-file vs book-level diff when a segment is selected
- [ ] Task 9 — Go tests: Unit test for the new segment-tags handler
- [ ] Task 10 — React tests: Unit tests for FileSelector and scoped tab content
- [ ] Task 11 — Verify & integrate: Run full test suite, fix any failures

---

## Prerequisite knowledge

### How segments work today

`BookDetail.tsx` line 155–170: segments are fetched via
`fetch('/api/v1/audiobooks/${id}/segments')` only when `activeTab === 'files'`
and `!segmentsLoaded`. The response is an array of `database.BookSegment`
objects (JSON field names: `id`, `file_path`, `format`, `size_bytes`,
`duration_seconds`, `track_number`, `total_tracks`, `active`).

The handler is `listAudiobookSegments` in
`internal/server/server.go` line 1639. It calls
`database.GlobalStore.ListBookSegments(bookNumericID)` where the numeric ID is
derived from `crc32.ChecksumIEEE([]byte(book.ID))`.

### How `getAudiobookTags` works

`internal/server/audiobook_service.go` line 232: `GetAudiobookTags` calls
`metadata.ExtractMetadata(book.FilePath)` then wraps the raw tags, stored DB
state, and override state into a `map[string]database.MetadataProvenanceEntry`
response. For the new per-segment endpoint we want a simpler response: just the
raw tags extracted from that specific file — no DB provenance overlay.

### BookDetail.tsx current state shape (lines 83–100)

```typescript
const [segments, setSegments] = useState<Array<{
  id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_seconds: number;
  track_number?: number;
  total_tracks?: number;
  active: boolean;
}>>([]);
const [segmentsLoaded, setSegmentsLoaded] = useState(false);
```

The `useEffect` that loads segments currently guards on
`activeTab !== 'files'`.

---

## Task 1 — Go: New segment-tags handler

### File: `internal/server/server.go`

**Current version:** `1.72.0`
**New version:** `1.73.0`

#### 1a. Register the route

Find the route block (around line 1049) and add one line after the segments route:

```go
protected.GET("/audiobooks/:id/segments", s.listAudiobookSegments)
// ADD THIS LINE:
protected.GET("/audiobooks/:id/segments/:segmentId/tags", s.getAudiobookSegmentTags)
```

#### 1b. Add the handler function

Add the following function anywhere after `listAudiobookSegments` (e.g., at
line ~1668, after the closing brace of `listAudiobookSegments`):

```go
// getAudiobookSegmentTags extracts raw metadata tags from a specific segment file.
// It does NOT apply DB overrides or provenance — it returns the raw embedded tags
// exactly as read from disk by metadata.ExtractMetadata.
func (s *Server) getAudiobookSegmentTags(c *gin.Context) {
	bookID := c.Param("id")
	segmentID := c.Param("segmentId")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Verify the book exists
	book, err := database.GlobalStore.GetBookByID(bookID)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Look up the segment from the book's segment list
	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, err := database.GlobalStore.ListBookSegments(bookNumericID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var segment *database.BookSegment
	for i := range segments {
		if segments[i].ID == segmentID {
			segment = &segments[i]
			break
		}
	}

	if segment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "segment not found"})
		return
	}

	if segment.FilePath == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "segment has no file path"})
		return
	}

	// Extract raw tags from the file on disk
	meta, err := metadata.ExtractMetadata(segment.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to extract metadata: %v", err)})
		return
	}

	// Build a flat tag map — no provenance wrapping, just raw values.
	// Zero-value strings are omitted to keep the response clean.
	tagMap := map[string]any{}
	if meta.Title != "" {
		tagMap["title"] = meta.Title
	}
	if meta.Artist != "" {
		tagMap["artist"] = meta.Artist
	}
	if meta.Album != "" {
		tagMap["album"] = meta.Album
	}
	if meta.Genre != "" {
		tagMap["genre"] = meta.Genre
	}
	if meta.Series != "" {
		tagMap["series"] = meta.Series
	}
	if meta.SeriesIndex != 0 {
		tagMap["series_index"] = meta.SeriesIndex
	}
	if meta.Comments != "" {
		tagMap["comments"] = meta.Comments
	}
	if meta.Year != 0 {
		tagMap["year"] = meta.Year
	}
	if meta.Narrator != "" {
		tagMap["narrator"] = meta.Narrator
	}
	if meta.Language != "" {
		tagMap["language"] = meta.Language
	}
	if meta.Publisher != "" {
		tagMap["publisher"] = meta.Publisher
	}
	if meta.ISBN10 != "" {
		tagMap["isbn10"] = meta.ISBN10
	}
	if meta.ISBN13 != "" {
		tagMap["isbn13"] = meta.ISBN13
	}

	c.JSON(http.StatusOK, gin.H{
		"segment_id":     segment.ID,
		"file_path":      segment.FilePath,
		"format":         segment.Format,
		"size_bytes":     segment.SizeBytes,
		"duration_sec":   segment.DurationSec,
		"track_number":   segment.TrackNumber,
		"total_tracks":   segment.TotalTracks,
		"tags":           tagMap,
		"used_filename_fallback": meta.UsedFilenameFallback,
	})
}
```

**Update the file header** at the top of `server.go`:

```go
// file: internal/server/server.go
// version: 1.73.0
```

---

## Task 2 — TypeScript: Add `SegmentTags` type and API function

### File: `web/src/services/api.ts`

**Current version:** `1.24.0`
**New version:** `1.25.0`

#### 2a. Add the interface after `BookTags` (currently at line ~154)

```typescript
export interface SegmentTags {
  segment_id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_sec: number;
  track_number?: number;
  total_tracks?: number;
  used_filename_fallback: boolean;
  tags: Record<string, string | number | boolean>;
}
```

#### 2b. Add the API function after `getBookTags` (currently at line ~597)

```typescript
export async function getSegmentTags(
  bookId: string,
  segmentId: string
): Promise<SegmentTags> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/segments/${segmentId}/tags`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch segment tags');
  }
  return response.json();
}
```

**Update the file header:**

```typescript
// file: web/src/services/api.ts
// version: 1.25.0
```

---

## Task 3 — React: Lift segment loading and add `selectedSegmentId` state

### File: `web/src/pages/BookDetail.tsx`

**Current version:** `1.15.0`
**New version:** `1.16.0`

#### 3a. Add `selectedSegmentId` state near the existing `segments` state (around line 83)

The current segments state block is:

```typescript
const [segments, setSegments] = useState<Array<{
  id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_seconds: number;
  track_number?: number;
  total_tracks?: number;
  active: boolean;
}>>([]);
const [segmentsLoaded, setSegmentsLoaded] = useState(false);
```

Replace it with:

```typescript
const [segments, setSegments] = useState<Array<{
  id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_seconds: number;
  track_number?: number;
  total_tracks?: number;
  active: boolean;
}>>([]);
const [segmentsLoaded, setSegmentsLoaded] = useState(false);
// null = "All Files" (book-level view); string = specific segment ID
const [selectedSegmentId, setSelectedSegmentId] = useState<string | null>(null);
```

#### 3b. Lift segment loading to the primary `useEffect`

Find the current segments `useEffect` (around line 155):

```typescript
// Load segments when files tab is active
useEffect(() => {
  if (activeTab !== 'files' || !id || segmentsLoaded) return;
  const loadSegments = async () => {
    try {
      const resp = await fetch(`/api/v1/audiobooks/${id}/segments`);
      if (resp.ok) {
        const data = await resp.json();
        setSegments(data);
      }
    } catch {
      // Segments not available, that's fine
    } finally {
      setSegmentsLoaded(true);
    }
  };
  loadSegments();
}, [activeTab, id, segmentsLoaded]);
```

Replace it with (always loads on mount, not gated on Files tab):

```typescript
// Load segments on mount so the file selector bar can render immediately
useEffect(() => {
  if (!id || segmentsLoaded) return;
  const loadSegments = async () => {
    try {
      const resp = await fetch(`/api/v1/audiobooks/${id}/segments`);
      if (resp.ok) {
        const data = await resp.json();
        setSegments(data);
      }
    } catch {
      // Segments not available for single-file books — that is fine
    } finally {
      setSegmentsLoaded(true);
    }
  };
  loadSegments();
}, [id, segmentsLoaded]);
```

#### 3c. Add `SegmentTags` import

At the top of the file, update the api import line (currently line 44):

```typescript
import type { Book, BookTags, OverridePayload, SegmentTags } from '../services/api';
```

#### 3d. Add per-segment tag state

After the `[tagsError, setTagsError]` state declarations (around line 87), add:

```typescript
const [segmentTags, setSegmentTags] = useState<SegmentTags | null>(null);
const [segmentTagsLoading, setSegmentTagsLoading] = useState(false);
const [segmentTagsError, setSegmentTagsError] = useState<string | null>(null);
```

#### 3e. Add `loadSegmentTags` callback after `loadTags`

```typescript
const loadSegmentTags = useCallback(async (segId: string) => {
  if (!id) return;
  setSegmentTagsLoading(true);
  setSegmentTagsError(null);
  try {
    const data = await api.getSegmentTags(id, segId);
    setSegmentTags(data);
  } catch (error) {
    console.error('Failed to load segment tags', error);
    setSegmentTagsError('Unable to load file tags for this segment.');
    setSegmentTags(null);
  } finally {
    setSegmentTagsLoading(false);
  }
}, [id]);
```

#### 3f. Add `useEffect` to load segment tags when selection changes

After the `useEffect` that calls `loadBook/loadVersions/loadTags`, add:

```typescript
useEffect(() => {
  if (selectedSegmentId === null) {
    setSegmentTags(null);
    setSegmentTagsError(null);
    return;
  }
  loadSegmentTags(selectedSegmentId);
}, [selectedSegmentId, loadSegmentTags]);
```

#### 3g. Derive `selectedSegment` from state

Add this derived value after the `versionSummary` memo (around line 340):

```typescript
const selectedSegment = useMemo(
  () => segments.find((s) => s.id === selectedSegmentId) ?? null,
  [segments, selectedSegmentId]
);

const isMultiFile = segments.length > 1;
```

---

## Task 4 — React: Build `FileSelector` component

Create a **new file** at:

### File: `web/src/components/audiobooks/FileSelector.tsx`

```typescript
// file: web/src/components/audiobooks/FileSelector.tsx
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890

import {
  Box,
  Chip,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Typography,
} from '@mui/material';
import FolderOpenIcon from '@mui/icons-material/FolderOpen.js';

export interface FileSelectorSegment {
  id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_seconds: number;
  track_number?: number;
  total_tracks?: number;
  active: boolean;
}

interface FileSelectorProps {
  segments: FileSelectorSegment[];
  selectedSegmentId: string | null;
  onSelect: (segmentId: string | null) => void;
}

/**
 * Returns a short display label for a segment chip.
 * Uses track number (zero-padded) + the basename of the file path,
 * truncated to 24 characters.
 */
function segmentLabel(seg: FileSelectorSegment): string {
  const basename = seg.file_path.split('/').pop() ?? seg.file_path;
  const nameWithoutExt = basename.replace(/\.[^.]+$/, '');
  const prefix =
    seg.track_number != null
      ? `${String(seg.track_number).padStart(2, '0')} `
      : '';
  const full = `${prefix}${nameWithoutExt}`;
  return full.length > 26 ? `${full.slice(0, 24)}…` : full;
}

/**
 * FileSelector renders a horizontal bar with an "All Files" anchor plus
 * scrollable chips for each segment. For books with >20 segments a Select
 * dropdown replaces the chip list to avoid overflow.
 *
 * Renders nothing when segments.length <= 1 (single-file books).
 */
export function FileSelector({
  segments,
  selectedSegmentId,
  onSelect,
}: FileSelectorProps) {
  // Do not render for single-file books
  if (segments.length <= 1) return null;

  const useDropdownOnly = segments.length > 20;

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        px: 2,
        py: 1,
        bgcolor: 'background.paper',
        borderBottom: '1px solid',
        borderColor: 'divider',
        overflowX: 'auto',
        flexShrink: 0,
      }}
      role="navigation"
      aria-label="File selector"
    >
      <FolderOpenIcon fontSize="small" color="action" sx={{ flexShrink: 0 }} />

      {/* "All Files" — always visible as a dropdown anchor */}
      {useDropdownOnly ? (
        <FormControl size="small" sx={{ minWidth: 200 }}>
          <InputLabel id="file-selector-label">File</InputLabel>
          <Select
            labelId="file-selector-label"
            label="File"
            value={selectedSegmentId ?? ''}
            onChange={(e) => {
              const val = e.target.value;
              onSelect(val === '' ? null : val);
            }}
          >
            <MenuItem value="">
              <Typography variant="body2">All Files</Typography>
            </MenuItem>
            {segments.map((seg) => (
              <MenuItem key={seg.id} value={seg.id}>
                <Typography variant="body2">{segmentLabel(seg)}</Typography>
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      ) : (
        <Stack direction="row" spacing={1} sx={{ flexWrap: 'nowrap' }}>
          {/* "All Files" chip */}
          <Chip
            label="All Files"
            size="small"
            variant={selectedSegmentId === null ? 'filled' : 'outlined'}
            color={selectedSegmentId === null ? 'primary' : 'default'}
            onClick={() => onSelect(null)}
            aria-pressed={selectedSegmentId === null}
          />
          {/* Individual file chips */}
          {segments.map((seg) => (
            <Chip
              key={seg.id}
              label={segmentLabel(seg)}
              size="small"
              variant={selectedSegmentId === seg.id ? 'filled' : 'outlined'}
              color={selectedSegmentId === seg.id ? 'primary' : 'default'}
              onClick={() =>
                onSelect(selectedSegmentId === seg.id ? null : seg.id)
              }
              aria-pressed={selectedSegmentId === seg.id}
              title={seg.file_path.split('/').pop()}
            />
          ))}
        </Stack>
      )}
    </Box>
  );
}
```

---

## Task 5 — React: Render FileSelector in BookDetail

### File: `web/src/pages/BookDetail.tsx`

#### 5a. Add import at the top

After the existing component imports (around line 46), add:

```typescript
import { FileSelector } from '../components/audiobooks/FileSelector';
```

#### 5b. Insert the FileSelector between the action bar Paper and the Tabs Paper

Find the JSX block that renders the Tabs Paper (around line 855 — the Paper
containing `<Tabs value={activeTab} ...>`):

```tsx
<Paper sx={{ p: 2, mb: 3 }}>
  <Tabs
    value={activeTab}
    onChange={(_, value) => setActiveTab(value)}
    ...
  >
```

Insert the `FileSelector` immediately **before** this Paper:

```tsx
{/* File selector bar — only shown for multi-file books */}
{isMultiFile && (
  <Paper sx={{ mb: 1, overflow: 'hidden' }}>
    <FileSelector
      segments={segments}
      selectedSegmentId={selectedSegmentId}
      onSelect={setSelectedSegmentId}
    />
  </Paper>
)}

<Paper sx={{ p: 2, mb: 3 }}>
  <Tabs
    value={activeTab}
    onChange={(_, value) => setActiveTab(value)}
    ...
```

---

## Task 6 — React: Scope Info tab to per-file metadata

### File: `web/src/pages/BookDetail.tsx`

The Info tab currently renders book-level fields. When `selectedSegment` is
non-null, replace the body with file-level fields from that segment.

Find the `{activeTab === 'info' && (` block. Wrap the entire inner content
with a conditional:

```tsx
{activeTab === 'info' && (
  <Paper sx={{ p: 3, mb: 3 }}>
    {selectedSegment ? (
      /* Per-file info view */
      <Stack spacing={2}>
        <Typography variant="h6">
          File Info:{' '}
          <Typography component="span" variant="body1" color="text.secondary">
            {selectedSegment.file_path.split('/').pop()}
          </Typography>
        </Typography>
        <Grid container spacing={2}>
          {[
            { label: 'File Path', value: selectedSegment.file_path },
            { label: 'Format', value: selectedSegment.format?.toUpperCase() },
            {
              label: 'Duration',
              value: formatDuration(selectedSegment.duration_seconds),
            },
            {
              label: 'Size',
              value:
                selectedSegment.size_bytes > 0
                  ? `${(selectedSegment.size_bytes / 1048576).toFixed(1)} MB`
                  : undefined,
            },
            {
              label: 'Track Number',
              value:
                selectedSegment.track_number != null
                  ? `${selectedSegment.track_number}${selectedSegment.total_tracks ? ` of ${selectedSegment.total_tracks}` : ''}`
                  : undefined,
            },
          ]
            .filter((item) => item.value !== undefined && item.value !== '')
            .map((item) => (
              <Grid item xs={12} sm={6} md={4} key={item.label}>
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 1,
                    bgcolor: 'background.default',
                    border: '1px solid',
                    borderColor: 'divider',
                    height: '100%',
                  }}
                >
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: 'uppercase' }}
                  >
                    {item.label}
                  </Typography>
                  <Typography variant="body1" sx={{ wordBreak: 'break-all' }}>
                    {item.value as string}
                  </Typography>
                </Box>
              </Grid>
            ))}
        </Grid>
      </Stack>
    ) : (
      /* Book-level info view — existing JSX unchanged */
      <>
        <Grid container spacing={2}>
          {[
            { label: 'Title', value: book.title || 'Untitled' },
            {
              label: 'Author',
              value:
                book.authors && book.authors.length > 0
                  ? book.authors.map((a) => a.name).join(' & ')
                  : book.author_name || 'Unknown',
            },
            { label: 'Series', value: book.series_name },
            {
              label: 'Narrator',
              value:
                book.narrators && book.narrators.length > 0
                  ? book.narrators.map((n) => n.name).join(' & ')
                  : book.narrator,
            },
            { label: 'Language', value: book.language },
            { label: 'ISBN 13', value: book.isbn13 },
            { label: 'Work ID', value: book.work_id },
          ]
            .filter(
              (item) =>
                item.value !== undefined &&
                item.value !== '' &&
                item.value !== null
            )
            .map((item) => (
              <Grid item xs={12} sm={6} md={4} key={item.label}>
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 1,
                    bgcolor: 'background.default',
                    border: '1px solid',
                    borderColor: 'divider',
                    height: '100%',
                  }}
                >
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: 'uppercase' }}
                  >
                    {item.label}
                  </Typography>
                  <Typography variant="body1">
                    {item.value as string}
                  </Typography>
                </Box>
              </Grid>
            ))}
        </Grid>
        {book.description && (
          <Box mt={3}>
            <Typography variant="h6" gutterBottom>
              Description
            </Typography>
            <Typography variant="body1" color="text.secondary">
              {book.description}
            </Typography>
          </Box>
        )}
      </>
    )}
  </Paper>
)}
```

---

## Task 7 — React: Scope Tags tab to per-file raw tags

### File: `web/src/pages/BookDetail.tsx`

Find the `{activeTab === 'tags' && (` block.

Add the per-file section at the **top** of the Tags tab Paper, before the
existing media info grid:

```tsx
{activeTab === 'tags' && (
  <Paper sx={{ p: 3, mb: 3 }}>
    <Stack direction="row" alignItems="center" spacing={1} mb={2}>
      <InfoIcon />
      <Typography variant="h6">
        {selectedSegment
          ? `File Tags: ${selectedSegment.file_path.split('/').pop()}`
          : 'Tags & Media'}
      </Typography>
    </Stack>

    {/* Per-file raw tags view */}
    {selectedSegment ? (
      <>
        {segmentTagsLoading && (
          <Stack direction="row" spacing={1} alignItems="center" mb={2}>
            <CircularProgress size={18} />
            <Typography variant="body2">Loading file tags...</Typography>
          </Stack>
        )}
        {segmentTagsError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {segmentTagsError}
          </Alert>
        )}
        {segmentTags && !segmentTagsLoading && (
          <>
            <Grid container spacing={1}>
              {Object.entries(segmentTags.tags).map(([key, value]) => (
                <Grid item xs={12} sm={6} md={4} key={key}>
                  <Box
                    sx={{
                      p: 1.5,
                      borderRadius: 1,
                      border: '1px dashed',
                      borderColor: 'divider',
                      bgcolor: 'background.default',
                    }}
                  >
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ textTransform: 'uppercase' }}
                    >
                      {key.replace(/_/g, ' ')}
                    </Typography>
                    <Typography variant="body2">{String(value)}</Typography>
                  </Box>
                </Grid>
              ))}
              {Object.keys(segmentTags.tags).length === 0 && (
                <Grid item xs={12}>
                  <Alert severity="info">
                    No embedded tags found in this file.
                  </Alert>
                </Grid>
              )}
            </Grid>
            {segmentTags.used_filename_fallback && (
              <Alert severity="warning" sx={{ mt: 2 }}>
                Some fields were inferred from the filename because embedded
                tags were missing or unreadable.
              </Alert>
            )}
          </>
        )}
        {!segmentTags && !segmentTagsLoading && !segmentTagsError && (
          <Alert severity="info">Select a file above to view its tags.</Alert>
        )}
      </>
    ) : (
      /* Book-level Tags view — existing JSX unchanged */
      <>
        {tagsError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {tagsError}
          </Alert>
        )}
        {tagsLoading && (
          <Stack direction="row" spacing={1} alignItems="center" mb={2}>
            <CircularProgress size={18} />
            <Typography variant="body2">Loading tags...</Typography>
          </Stack>
        )}
        {/* ... existing Tags tab content goes here unchanged ... */}
      </>
    )}
  </Paper>
)}
```

> **Note to implementor:** The "existing Tags tab content" comment represents
> the full existing JSX inside the Tags tab Paper (the two-column media info
> grid and the File Tags expandable section). Copy it verbatim into the else
> branch above.

---

## Task 8 — React: Scope Compare tab to per-file vs book-level diff

### File: `web/src/pages/BookDetail.tsx`

When a segment is selected, the Compare tab shows a two-column table:
left column = raw tag value from that file, right column = book-level
effective value (from `tags?.tags`), highlighting mismatches.

Find the `{activeTab === 'compare' && (` block. Add the per-file branch at
the top of the Paper:

```tsx
{activeTab === 'compare' && (
  <Paper sx={{ p: 3, mb: 3 }}>
    <Stack direction="row" alignItems="center" spacing={1} mb={2}>
      <CompareIcon />
      <Typography variant="h6">
        {selectedSegment
          ? `Compare: ${selectedSegment.file_path.split('/').pop()} vs Book`
          : 'Compare & Resolve'}
      </Typography>
    </Stack>

    {/* Per-file compare view */}
    {selectedSegment ? (
      <>
        {segmentTagsLoading && (
          <Stack direction="row" spacing={1} alignItems="center" mb={2}>
            <CircularProgress size={18} />
            <Typography variant="body2">Loading segment tags...</Typography>
          </Stack>
        )}
        {segmentTagsError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {segmentTagsError}
          </Alert>
        )}
        {segmentTags && !segmentTagsLoading && (
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Field</TableCell>
                <TableCell>
                  File Tag ({selectedSegment.file_path.split('/').pop()})
                </TableCell>
                <TableCell>Book-level Effective Value</TableCell>
                <TableCell>Match?</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {/* Union of keys from file tags and book tags */}
              {Array.from(
                new Set([
                  ...Object.keys(segmentTags.tags),
                  ...Object.keys(tags?.tags ?? {}),
                ])
              ).map((field) => {
                const fileVal =
                  segmentTags.tags[field] != null
                    ? String(segmentTags.tags[field])
                    : '—';
                const bookEntry = tags?.tags?.[field];
                const bookVal = bookEntry
                  ? String(
                      bookEntry.effective_value ??
                        bookEntry.override_value ??
                        bookEntry.stored_value ??
                        bookEntry.fetched_value ??
                        bookEntry.file_value ??
                        '—'
                    )
                  : '—';
                const matches =
                  fileVal !== '—' && bookVal !== '—' && fileVal === bookVal;
                return (
                  <TableRow
                    key={field}
                    sx={
                      !matches && fileVal !== '—' && bookVal !== '—'
                        ? { bgcolor: 'warning.light' }
                        : undefined
                    }
                  >
                    <TableCell>
                      <Typography
                        variant="body2"
                        sx={{ textTransform: 'uppercase', fontWeight: 500 }}
                      >
                        {field.replace(/_/g, ' ')}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">{fileVal}</Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">{bookVal}</Typography>
                    </TableCell>
                    <TableCell>
                      {fileVal === '—' || bookVal === '—' ? (
                        <Chip label="n/a" size="small" variant="outlined" />
                      ) : matches ? (
                        <Chip label="match" size="small" color="success" />
                      ) : (
                        <Chip label="mismatch" size="small" color="warning" />
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
        {!segmentTags && !segmentTagsLoading && !segmentTagsError && (
          <Alert severity="info">Select a file above to compare its tags.</Alert>
        )}
      </>
    ) : (
      /* Existing book-level Compare tab JSX — copy verbatim here */
      <>
        {/* ... existing Compare tab content unchanged ... */}
      </>
    )}
  </Paper>
)}
```

> **Note to implementor:** Replace the `{/* ... existing Compare tab content unchanged ... */}`
> comment with the full existing Compare tab JSX (the Table with Field / File
> Tag / Fetched / Stored / Override / Actions columns).

---

## Task 9 — Go tests: Unit test for the new segment-tags handler

### File: `internal/server/server_test.go`

**Current version:** check header, bump patch.

Add the following test function. It follows the exact same pattern as
`TestGetAudiobookTagsReportsEffectiveSourceSimple` (line 220).

```go
func TestGetAudiobookSegmentTags_ReturnsRawTags(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	// Create a minimal real audio fixture — use an existing fixture from testdata
	// or create a stub file (metadata extraction will return empty tags on a stub,
	// but the handler should still respond 200 with an empty tags map)
	tempFile := filepath.Join(tempDir, "part01.m4b")
	require.NoError(t, os.WriteFile(tempFile, []byte("stub"), 0o644))

	created, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Multi-file Book",
		FilePath: tempDir, // book-level path is the directory
		Format:   "m4b",
	})
	require.NoError(t, err)

	bookNumericID := int(crc32.ChecksumIEEE([]byte(created.ID)))
	trackNum := 1
	seg, err := database.GlobalStore.CreateBookSegment(bookNumericID, &database.BookSegment{
		ID:          "seg-test-01",
		BookID:      bookNumericID,
		FilePath:    tempFile,
		Format:      "m4b",
		SizeBytes:   4,
		DurationSec: 60,
		TrackNumber: &trackNum,
		Active:      true,
	})
	require.NoError(t, err)
	require.NotNil(t, seg)

	url := fmt.Sprintf("/api/v1/audiobooks/%s/segments/%s/tags", created.ID, seg.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		SegmentID string         `json:"segment_id"`
		FilePath  string         `json:"file_path"`
		Format    string         `json:"format"`
		Tags      map[string]any `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, seg.ID, response.SegmentID)
	assert.Equal(t, tempFile, response.FilePath)
	assert.Equal(t, "m4b", response.Format)
	assert.NotNil(t, response.Tags) // may be empty, but must be present
}

func TestGetAudiobookSegmentTags_NotFoundBook(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/audiobooks/nonexistent-book-id/segments/seg-1/tags",
		nil,
	)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetAudiobookSegmentTags_NotFoundSegment(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	created, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Book without segment",
		FilePath: tempDir,
		Format:   "m4b",
	})
	require.NoError(t, err)

	url := fmt.Sprintf("/api/v1/audiobooks/%s/segments/no-such-seg/tags", created.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

**Required imports** (add to import block if not already present):

```go
"hash/crc32"
```

Note: `crc32` is already imported in `server.go` — in the test file check
whether it is already imported; if not add it.

---

## Task 10 — React tests

### 10a. FileSelector unit test

### File: `web/src/components/audiobooks/FileSelector.test.tsx` (new file)

```typescript
// file: web/src/components/audiobooks/FileSelector.test.tsx
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-012345678901

import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { FileSelector } from './FileSelector';
import type { FileSelectorSegment } from './FileSelector';

const makeSegments = (count: number): FileSelectorSegment[] =>
  Array.from({ length: count }, (_, i) => ({
    id: `seg-${i + 1}`,
    file_path: `/books/book/Part ${i + 1}.m4b`,
    format: 'm4b',
    size_bytes: 1024 * 1024 * (i + 1),
    duration_seconds: 3600,
    track_number: i + 1,
    total_tracks: count,
    active: true,
  }));

describe('FileSelector', () => {
  it('renders nothing for single-file books', () => {
    const { container } = render(
      <FileSelector
        segments={makeSegments(1)}
        selectedSegmentId={null}
        onSelect={vi.fn()}
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders nothing for empty segment list', () => {
    const { container } = render(
      <FileSelector
        segments={[]}
        selectedSegmentId={null}
        onSelect={vi.fn()}
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it('shows All Files chip and segment chips for 2-20 segment books', () => {
    render(
      <FileSelector
        segments={makeSegments(3)}
        selectedSegmentId={null}
        onSelect={vi.fn()}
      />
    );
    expect(screen.getByText('All Files')).toBeInTheDocument();
    expect(screen.getByText(/01 Part 1/)).toBeInTheDocument();
    expect(screen.getByText(/02 Part 2/)).toBeInTheDocument();
    expect(screen.getByText(/03 Part 3/)).toBeInTheDocument();
  });

  it('calls onSelect(null) when All Files chip is clicked', () => {
    const onSelect = vi.fn();
    render(
      <FileSelector
        segments={makeSegments(3)}
        selectedSegmentId="seg-1"
        onSelect={onSelect}
      />
    );
    fireEvent.click(screen.getByText('All Files'));
    expect(onSelect).toHaveBeenCalledWith(null);
  });

  it('calls onSelect(segmentId) when a segment chip is clicked', () => {
    const onSelect = vi.fn();
    render(
      <FileSelector
        segments={makeSegments(3)}
        selectedSegmentId={null}
        onSelect={onSelect}
      />
    );
    fireEvent.click(screen.getByText(/01 Part 1/));
    expect(onSelect).toHaveBeenCalledWith('seg-1');
  });

  it('deselects (calls onSelect(null)) when an active segment chip is clicked again', () => {
    const onSelect = vi.fn();
    render(
      <FileSelector
        segments={makeSegments(3)}
        selectedSegmentId="seg-1"
        onSelect={onSelect}
      />
    );
    fireEvent.click(screen.getByText(/01 Part 1/));
    expect(onSelect).toHaveBeenCalledWith(null);
  });

  it('renders a dropdown for books with >20 segments', () => {
    render(
      <FileSelector
        segments={makeSegments(25)}
        selectedSegmentId={null}
        onSelect={vi.fn()}
      />
    );
    expect(screen.getByRole('combobox')).toBeInTheDocument();
    // No individual chips
    expect(screen.queryByRole('button', { name: /Part 1/ })).toBeNull();
  });
});
```

### 10b. BookDetail integration: segment selection test

### File: `web/tests/unit/BookDetail.test.tsx`

**Current version:** check header, bump patch.

Add the following test block after existing tests. The existing mock setup at
the top of the file already has `getBookTags: vi.fn()` — add `getSegmentTags`
to the mock object:

```typescript
// Inside the vi.mock('../../../src/services/api', ...) factory, add:
getSegmentTags: vi.fn(),
```

Then add the test:

```typescript
describe('FileSelector integration', () => {
  const twoSegmentMockBook = {
    ...mockBook,
    id: 'multi-book-1',
  };

  const mockSegments = [
    {
      id: 'seg-a',
      file_path: '/library/book/Part 01.m4b',
      format: 'm4b',
      size_bytes: 10485760,
      duration_seconds: 3600,
      track_number: 1,
      total_tracks: 2,
      active: true,
    },
    {
      id: 'seg-b',
      file_path: '/library/book/Part 02.m4b',
      format: 'm4b',
      size_bytes: 10485760,
      duration_seconds: 3600,
      track_number: 2,
      total_tracks: 2,
      active: true,
    },
  ];

  const mockSegmentTags = {
    segment_id: 'seg-a',
    file_path: '/library/book/Part 01.m4b',
    format: 'm4b',
    size_bytes: 10485760,
    duration_sec: 3600,
    track_number: 1,
    total_tracks: 2,
    used_filename_fallback: false,
    tags: { title: 'Part One', artist: 'Some Author', year: 2020 },
  };

  beforeEach(() => {
    vi.mocked(api.getBook).mockResolvedValue(twoSegmentMockBook);
    vi.mocked(api.getBookVersions).mockResolvedValue([]);
    vi.mocked(api.getBookTags).mockResolvedValue(mockTags);
    vi.mocked(api.getSegmentTags).mockResolvedValue(mockSegmentTags);

    // Override global fetch for segments
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockSegments,
    });
  });

  it('shows FileSelector bar for a multi-file book', async () => {
    render(<BookDetail />, { wrapper: BrowserRouter });
    expect(await screen.findByText('All Files')).toBeInTheDocument();
    expect(screen.getByText(/01 Part 01/)).toBeInTheDocument();
  });

  it('loads segment tags when a file chip is clicked', async () => {
    render(<BookDetail />, { wrapper: BrowserRouter });
    await screen.findByText('All Files');
    fireEvent.click(screen.getByText(/01 Part 01/));
    expect(api.getSegmentTags).toHaveBeenCalledWith('multi-book-1', 'seg-a');
  });
});
```

---

## Task 11 — Verify & integrate

### Commands to run in order

```bash
# 1. Backend tests (includes the new segment-tags handler tests)
make test

# 2. Frontend unit tests
make test-all

# 3. Build to ensure TypeScript compiles with the new types
make build

# 4. E2E smoke test (optional but recommended)
make test-e2e
```

### Expected results

- `make test` — all existing tests pass + 3 new Go tests pass
- `make test-all` — all existing tests pass + 7 new FileSelector tests pass + 2 new BookDetail integration tests pass
- `make build` — exits 0, no TypeScript errors
- Coverage should stay at or above 81.3% (new handler tests add coverage)

---

## File header summary (all files that change)

| File | Old version | New version |
|------|-------------|-------------|
| `internal/server/server.go` | 1.72.0 | 1.73.0 |
| `web/src/services/api.ts` | 1.24.0 | 1.25.0 |
| `web/src/pages/BookDetail.tsx` | 1.15.0 | 1.16.0 |
| `web/src/components/audiobooks/FileSelector.tsx` | (new) | 1.0.0 |
| `web/src/components/audiobooks/FileSelector.test.tsx` | (new) | 1.0.0 |
| `internal/server/server_test.go` | (bump patch) | (bump patch) |
| `web/tests/unit/BookDetail.test.tsx` | (bump patch) | (bump patch) |

---

## Edge cases and gotchas

1. **Single-file books** — `segments.length <= 1` makes `FileSelector` return
   `null` and `isMultiFile` is `false`. The whole feature is transparent for
   single-file books.

2. **Segment with empty `file_path`** — the Go handler returns 422 with an
   error message. The frontend will show the `segmentTagsError` Alert.

3. **Stub / non-audio files in tests** — `metadata.ExtractMetadata` on a stub
   file (e.g., `[]byte("stub")`) returns an empty `Metadata{}` and a non-nil
   error in real scenarios, but the test uses `GlobalMetadataExtractor` which
   may or may not be set. The handler logs a warning and returns an empty tag
   map (not an error) only if `ExtractMetadata` returns an error — actually
   per the implementation above the handler **does** return a 500 on extract
   error. For tests, use the existing `testdata/` fixtures if available, or
   accept that the test will hit 500 and adjust the assertion accordingly (see
   the test for `TestGetAudiobookSegmentTags_ReturnsRawTags` — the assertion
   uses `assert.Equal(t, http.StatusOK, w.Code)` which will fail on a stub
   file if `ExtractMetadata` errors). **Fix:** in the test, set
   `metadata.GlobalMetadataExtractor` to a mock that returns empty `Metadata{}`:

   ```go
   // At the start of TestGetAudiobookSegmentTags_ReturnsRawTags:
   origExtractor := metadata.GlobalMetadataExtractor
   metadata.GlobalMetadataExtractor = &metadata.MockExtractor{} // returns empty Metadata, nil
   t.Cleanup(func() { metadata.GlobalMetadataExtractor = origExtractor })
   ```

   Check `internal/metadata/metadata.go` for the `Extractor` interface and any
   existing mock implementation. If none exists, inline:

   ```go
   type stubExtractor struct{}
   func (s *stubExtractor) ExtractMetadata(path string) (metadata.Metadata, error) {
       return metadata.Metadata{Title: "Stub"}, nil
   }
   metadata.GlobalMetadataExtractor = &stubExtractor{}
   ```

4. **Segment ID format** — segments use ULIDs (from `github.com/oklog/ulid/v2`)
   as their `ID` field (a string). The URL param `:segmentId` is therefore a
   raw string — no numeric conversion needed.

5. **crc32 import in test** — `crc32` is already imported in `server.go`. In
   `server_test.go` you may need to add it. Check the existing imports at the
   top of `server_test.go`.

6. **React state reset on book navigation** — `selectedSegmentId` is local
   component state, so navigating away and back resets it to `null`. This is
   correct behavior.

7. **Tab-triggered re-fetch** — segment tags are fetched whenever
   `selectedSegmentId` changes (via the `useEffect` in Task 3f). Switching tabs
   does NOT re-fetch if the selection hasn't changed; the cached `segmentTags`
   state is used. This is intentional — the data does not change between tabs.

8. **Versions tab** — The spec mentions scoping Versions tab to a specific
   segment, but the existing `VersionManagement` component operates at the book
   level (versions are book-level records, not segment-level). Leave Versions
   tab behavior unchanged regardless of segment selection. Add an informational
   note if a segment is selected:

   ```tsx
   {activeTab === 'versions' && selectedSegment && (
     <Alert severity="info" sx={{ mb: 2 }}>
       Version management applies to the whole book, not individual files.
     </Alert>
   )}
   ```

   Insert this Alert inside the Versions tab Paper, before the existing content.

---

## Complete new code artifacts (consolidated for copy-paste)

### Go handler (add to `internal/server/server.go` after line ~1667)

```go
// getAudiobookSegmentTags extracts raw metadata tags from a specific segment file.
func (s *Server) getAudiobookSegmentTags(c *gin.Context) {
	bookID := c.Param("id")
	segmentID := c.Param("segmentId")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(bookID)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, err := database.GlobalStore.ListBookSegments(bookNumericID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var segment *database.BookSegment
	for i := range segments {
		if segments[i].ID == segmentID {
			segment = &segments[i]
			break
		}
	}

	if segment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "segment not found"})
		return
	}

	if segment.FilePath == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "segment has no file path"})
		return
	}

	meta, err := metadata.ExtractMetadata(segment.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to extract metadata: %v", err)})
		return
	}

	tagMap := map[string]any{}
	if meta.Title != "" { tagMap["title"] = meta.Title }
	if meta.Artist != "" { tagMap["artist"] = meta.Artist }
	if meta.Album != "" { tagMap["album"] = meta.Album }
	if meta.Genre != "" { tagMap["genre"] = meta.Genre }
	if meta.Series != "" { tagMap["series"] = meta.Series }
	if meta.SeriesIndex != 0 { tagMap["series_index"] = meta.SeriesIndex }
	if meta.Comments != "" { tagMap["comments"] = meta.Comments }
	if meta.Year != 0 { tagMap["year"] = meta.Year }
	if meta.Narrator != "" { tagMap["narrator"] = meta.Narrator }
	if meta.Language != "" { tagMap["language"] = meta.Language }
	if meta.Publisher != "" { tagMap["publisher"] = meta.Publisher }
	if meta.ISBN10 != "" { tagMap["isbn10"] = meta.ISBN10 }
	if meta.ISBN13 != "" { tagMap["isbn13"] = meta.ISBN13 }

	c.JSON(http.StatusOK, gin.H{
		"segment_id":             segment.ID,
		"file_path":              segment.FilePath,
		"format":                 segment.Format,
		"size_bytes":             segment.SizeBytes,
		"duration_sec":           segment.DurationSec,
		"track_number":           segment.TrackNumber,
		"total_tracks":           segment.TotalTracks,
		"tags":                   tagMap,
		"used_filename_fallback": meta.UsedFilenameFallback,
	})
}
```
