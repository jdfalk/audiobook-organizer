<!-- file: docs/plans/2026-03-01-bookdetail-ux-overhaul-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d -->
<!-- last-edited: 2026-03-01 -->

# BookDetail UX Overhaul — Design

## Goal

Fix multiple UX issues on the BookDetail page: button bar ordering/styling, merge Tags+Compare tabs, improve mismatch readability, add resolve actions with pending-write tracking, add track/disk fields to edit dialog, and fix several bugs (author display, cover art reload, metadata history recording).

## Sections

### 1. Button Bar Redesign

Reorder and restyle the action buttons (left to right):

1. **Manage Versions** — `variant="outlined"`, `color="info"` (neutral blue/indigo, not pink/red)
2. **History** — `variant="outlined"`, default
3. **Parse with AI** — `variant="outlined"`, default
4. **Edit Metadata** — `variant="outlined"`, default
5. **Fetch Metadata** — `variant="outlined"`, default
6. **Search Metadata** — `variant="outlined"`, `SearchIcon`, proper labeled button (not bare IconButton)
7. **Save to Files** — `variant="outlined"`, default
8. **Delete** — `variant="contained"`, `color="error"` (red) — only red/prominent button

### 2. Merge Tags + Compare into Unified "Tags" Tab

- Remove separate "Tags" and "Compare" tabs → single **"Tags"** tab
- Tab order: Info, Files, Versions, Tags (4 tabs)
- Unified table columns: **Field | File Tag Value | Book Effective Value | Match | Actions**
- Always show all fields (title, author, narrator, album, genre, year, publisher, series, language, track, disk)
- Mismatch rows: subtle red/coral tint (replacing unreadable yellow/orange). White text readable in dark mode.
- Match column chips: "mismatch" uses red-tinted chip for legibility
- Actions column per row:
  - "Use File" button — copies file tag value into DB, marks as pending write
  - "Use Book" button — queues writing book value to file tag
  - Visual "pending write" indicator (dot/icon) on resolved-but-unsaved rows
- Per-file "Save to File" button at top when a segment is selected

### 3. Edit Metadata Dialog — Track/Disk Fields

Add four fields after Series Number, before Genre:
- **Track Number** (text, 50% width) — e.g. "3" or "3/14"
- **Total Tracks** (number, 50% width)
- **Disk Number** (text, 50% width) — e.g. "1" or "1/2"
- **Total Disks** (number, 50% width)

### 4. Bug Fixes

**4a. Author shows "Unknown" after metadata fetch**
After `setBook(result.book)`, call `loadBook()` to re-fetch the fully enriched book (with populated `authors` array).

**4b. Author ID showing in header**
Remove `book.author_id` from the fallback chain. Show "Unknown Author" instead.

**4c. Cover art not loading after metadata fetch**
Reset `setCoverError(false)` whenever `book.cover_url` changes via useEffect.

**4d. Metadata change history not recording for manual edits**
Add history recording in the PUT /audiobooks/:id update flow. Compare old vs new field values and record changes with `source: "manual"`, `change_type: "manual"`.

### 5. Audiobook Edition Indicator in Search Results

In MetadataSearchDialog, show a small badge on results that have narrator data, indicating they're audiobook editions vs print editions. Helps users pick the right match.

### 6. File Renaming — Deferred

File renaming on save (e.g. `disk 01 track 03 - Title.mp3`) is deferred to a follow-up plan due to cascading DB path updates. Current tag writing (track numbers, album, artist in file tags) is sufficient for player ordering.
