# Files & History Tab Redesign

**Date:** 2026-03-18
**Status:** Approved

## Problem

The current "Files & Versions" tab is confusing:
- Shows duplicate entries for the same file (snapshots treated as versions)
- No iTunes original visible even when PID-linked
- Can't compare two files side-by-side
- No history of changes (renames, tag writes, metadata applies)
- Multi-file MP3 books show one tray per chapter instead of grouped

## Solution

Redesign the tab as "Files & History" with two clear sections: **Formats** (different physical files) and **Change Log** (timeline of changes). Add a comparison dropdown that shows a 4th column for side-by-side tag comparison.

## Terminology

- **Format** = a distinct physical file or set of files (M4B single-file, MP3 multi-chapter, etc.)
- **Segment** = one file within a multi-file format (chapter 1.mp3, chapter 2.mp3, etc.)
- **Snapshot** = a point-in-time record of a file's state (before/after tag write, rename, etc.)
- **Version** is deprecated in the UI — replaced by "Format" for different files

## Tab Name

Rename from "FILES & VERSIONS (N)" to **"FILES & HISTORY"**

## Section 1: Formats

One expanding tray per format (not per file). Each tray shows:

### Header bar (collapsed)
- Star icon if primary
- Format name: "M4B (AAC)" / "MP3"
- File count, total size, duration
- Badges: "Primary", "iTunes"
- Expand/collapse arrow

### Expanded content
- **Path** and **codec/bitrate** info
- **Key Tags summary** — badges showing which tags are present (✓ title, ✓ author, ✗ isbn). Links to full tag comparison.
- **Full tag comparison** (toggled by "View full tag comparison →"):
  - 3-column table: Tag | File Value | DB Value
  - Dropdown "Compare against: [None | iTunes Original | Previous Snapshot]"
  - When comparison selected, 4th column appears with comparison values
  - Diff highlighting: amber background = different, green background = present here but missing in comparison, red text = comparison value differs

### Multi-file formats (MP3 chapters)
- Expanded tray shows **overall metadata summary** (from first file, with ≠ DB indicators)
- **Segment table** below: # | File | Duration | Size
- Collapsed by default, "show all N files" toggle
- NOT one tray per segment

### iTunes link banner
Below the format trays, a subtle info bar:
- "iTunes Linked — N PIDs mapped · Source: /path/to/xml"
- "View PIDs →" link (expandable list of PID mappings)

## Section 2: Change Log

Timeline of all changes to this book, newest first:

- **Tag writes** — "Tags written — author, series, publisher, language"
- **Renames** — "Renamed — old/path → new/path"
- **Metadata applies** — "Metadata applied — Audible match (Title, Author)"
- **Import** — "Imported from iTunes — PID ABC123"
- **Transcode** — "Transcoded MP3 → M4B"

Each entry shows timestamp and a "Compare snapshot →" link for tag-write events (loads the before/after comparison in the tag table).

## Data Sources

| Data | Source |
|------|--------|
| Formats | Books in the same version group + the book itself |
| Segments | `book_segments` table |
| iTunes PIDs | `external_id_map` table (source="itunes") |
| Tag values | Live ffprobe on current file |
| Comparison tags | Live ffprobe on comparison file OR stored snapshot |
| Change Log | `book_path_history` + `metadata_history` + `operation_changes` tables |
| Snapshots | Version snapshots (existing feature) |

## API Changes

### Modified endpoints

**GET /api/v1/audiobooks/:id/tags**
- Add query param `?compare_id=<book_id>` — returns a 4th `comparison_value` in each tag entry
- Add query param `?compare_snapshot=<snapshot_id>` — compares against a stored snapshot

### New endpoints

**GET /api/v1/audiobooks/:id/changelog**
- Returns merged timeline from path_history, metadata_history, and operation_changes
- Response: `{"entries": [{"timestamp": "...", "type": "tag_write|rename|metadata_apply|import|transcode", "summary": "...", "details": {...}}]}`

## Frontend Components

### Modified: `BookDetail.tsx`
- Rename tab from "FILES & VERSIONS" to "FILES & HISTORY"
- Refactor version list rendering:
  - Group by format (deduplicate same-path entries)
  - Multi-file formats show segment table inside one tray
  - Add key tag badges to collapsed/expanded view
  - Add "View full tag comparison" toggle

### New: `TagComparison.tsx`
- Dropdown selector for comparison target
- 4-column table with diff highlighting
- Amber/green/red color coding

### New: `ChangeLog.tsx`
- Timeline component for the Change Log section
- Each entry is a row with timestamp, icon, summary, and optional "Compare" link
- "Compare snapshot" opens the tag comparison with the snapshot loaded

### Modified: `api.ts`
- Add `compare_id` param to `getBookTags()`
- Add `getBookChangelog(bookId)` function

## Testing

- E2E: Tag comparison dropdown renders 4th column with correct diff highlighting
- E2E: Multi-file format shows segment table, not individual trays
- E2E: Change Log shows timeline entries
- Unit: Changelog API merges path_history + metadata_history correctly
- Unit: Tag comparison correctly identifies diffs
