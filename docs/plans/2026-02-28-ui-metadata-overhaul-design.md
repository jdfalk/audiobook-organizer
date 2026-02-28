<!-- file: docs/plans/2026-02-28-ui-metadata-overhaul-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->
<!-- last-edited: 2026-02-28 -->

# UI & Metadata Overhaul Design

**Date:** 2026-02-28
**Status:** Approved

## Problem Statement

10 issues identified during real-world usage of multi-file audiobooks:

1. UI doesn't display multiple authors from the `authors[]` array
2. Fetch metadata matches box sets/collections instead of individual books
3. AI parse sends only the bare filename — useless for multi-file books
4. No explicit save-to-files button; file write-back doesn't happen on manual edits
5. No strategy for reading/writing metadata across multi-file books
6. No way to view/edit individual file metadata
7. Missing iTunes-level features for metadata and library management
8. Tab layout doesn't work for multi-file books
9. History/changelog shows no changes (AI parse doesn't record history)
10. "Updated" timestamp is misleading — fires on any DB save

## Design

### Phase 1: Fix What's Broken

#### 1.1 Multiple Authors Display
- Backfill `book_authors` junction table from existing `author_name` fields containing `&`
- Client-side fallback: if `authors[]` empty but `author_name` has `&`, split for display
- Same treatment for narrators

#### 1.2 Fetch Metadata Matching
- Penalize results containing "collection", "box set", "series X books", "complete series"
- Length penalty: result title >2x longer than search title gets penalized
- Require minimum similarity threshold (0.6 Jaccard) — reject below
- When book has series info, require matched result to have matching series position

#### 1.3 AI Parse Context
- Send full folder path, not just `filepath.Base()`
- Include first file's ID3 tags if available
- Include file count and total duration
- Restructured prompt asking AI to extract from ALL context (folder hierarchy, tags, filename pattern)

#### 1.4 History Recording
- All metadata change paths must call `recordChange()`: AI parse, fetch, manual edit, undo
- Add change recording to AI parse handler
- Ensure manual edits record complete before/after snapshots

#### 1.5 Updated Timestamp
- Only set `updated_at` when user-visible fields actually change (compare old vs new)
- Add `metadata_updated_at` for metadata-specific changes

### Phase 2: Save Button & File Write-back

#### 2.1 New Endpoint
- `POST /api/v1/audiobooks/:id/write-back` — writes current DB metadata to audio files

#### 2.2 "Save to Files" Button
- Added to book header action bar
- Shows confirmation dialog: "Write metadata to X files?"
- Summary of fields being written
- Disabled if no metadata changed since last write-back

#### 2.3 Write-back Strategy for Multi-file Books
- Book-level tags written to ALL segment files: title, author, album (= book title), narrator, year, series, genre, cover art
- Per-segment tags:
  - Track number = sequential position (1, 2, 3...)
  - Total tracks = segment count
  - Title tag per segment: `001 - Book Title`, `002 - Book Title` (zero-padded)
  - Album tag = book title (groups files in players)
- Record history entry (change_type: "write-back", source: "manual")
- Track `last_written_at` timestamp on book

### Phase 3: Multi-file Tab Layout

#### 3.1 File Selector Bar
- Positioned between book header and tab bar
- Hidden for single-file books (no layout change)
- Shows `[All Files ▼]` dropdown + horizontal scrollable chips per file
- Chips show: track number + short filename
- For >20 files, collapse to dropdown-only

#### 3.2 Scoped Tab Content
- **"All Files" selected:** tabs show book-level data (current behavior)
- **Specific file selected:**
  - Info: that file's individual tags (title, track, duration, format)
  - Tags: that file's raw ID3/M4B tags
  - Compare: file tags vs book-level metadata (discrepancy view)
  - Versions: that file's version history

### Phase 4: Multi-file Metadata Read Strategy

- **Folder path:** Extract author/series/title from directory hierarchy
- **First file tags:** Extract narrator/year/genre from ID3/M4B tags
- **Filename pattern:** Extract track ordering from filename numbering
- Combine all three sources with priority: file tags > folder path > filename

### Phase 5: iTunes Feature Parity (Backlog)

#### Metadata Fields (Missing)
- Genre/category taxonomy
- Rating (1-5 stars)
- Copyright field
- Explicit/clean flag
- Chapter marks display (M4B/MP4)
- Per-chapter artwork
- Grouping field
- Sort fields (sort-title, sort-author, sort-narrator)
- Comments/notes field

#### Library Management (Missing)
- Smart collections / saved filters
- Bulk metadata editing (multi-select)
- Duplicate detection
- Missing file detection
- Storage usage dashboard
- Column-customizable list view (iTunes-style sortable table)
- Keyboard navigation (arrow keys, spacebar)
- Import/export library metadata
- Mark as read/unread status
- Reading progress tracking

#### Improvements Needed
- Cover art display/editing
- Search with filters/facets
- Sorting with more fields and saved orders
