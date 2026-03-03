# Copy-on-Write Versioning, Smart Apply Pipeline, and Tag Editor Redesign

**Date:** 2026-03-03
**Status:** Approved

## Problem

Applying metadata (auto-fetch or manual search) can destroy book data with no recovery path. The Leviathan Falls incident: applying a correct metadata result wiped the title, left no history, and provided no way to undo. Root causes:

1. `UpdateBook()` does destructive in-place replacement of the JSON blob at `book:{id}`
2. History recording had call-order bugs (recorded AFTER mutation, so old==new)
3. No snapshot/versioning mechanism at the DB layer
4. Metadata apply is incomplete — updates DB fields but doesn't rename files, write tags, or verify
5. Tag editor is a passive comparison table, not an interactive editor

## Design

Four interconnected changes:

### 1. Copy-on-Write Book Versioning (PebbleDB Layer)

**Key scheme:**
```
book:{id}                              → current state (hot reads, unchanged)
book_ver:{id}:{nanoTimestamp}           → immutable snapshot (append-only)
```

**Behavior:**
- `UpdateBook()` atomically writes BOTH keys in one batch — current state + versioned snapshot
- `GetBookByID()` unchanged — reads `book:{id}`, zero read overhead
- New methods:
  - `GetBookVersions(id string, limit int) ([]BookVersion, error)` — list versions with timestamps
  - `GetBookAtVersion(id string, ts time.Time) (*Book, error)` — read specific version
  - `RevertBookToVersion(id string, ts time.Time) (*Book, error)` — copy version back to current (creates new version too)
  - `PruneBookVersions(id string, keepCount int) (int, error)` — manual cleanup, delete all but last N versions
- `BookVersion` struct: `{ Timestamp time.Time, Book *Book }`

**Retention:** Keep everything forever by default. Manual cleanup via settings UI or CLI flag (`--prune-history`).

**Migration:** Existing books have no versions. First update after migration creates the first version. No backfill needed.

**Removes:** The `saveBookSnapshot` hack from metadata_fetch_service.go. The `__snapshot__` metadata change records. The `RevertBookToSnapshot` method. All replaced by native DB-layer versioning.

### 2. Unified Path Format Template

**Single setting** used by organizer, tag editor, file renaming, and write-back:

```
Settings key: "path_format"
Default:      "{author}/{series_prefix}{title}/{track_title}.{ext}"

Settings key: "segment_title_format"
Default:      "{title} - {track}/{total_tracks}"
```

**Template variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `{author}` | Resolved author name | `James S.A. Corey` |
| `{title}` | Book title | `Leviathan Falls` |
| `{series}` | Series name (empty if none) | `The Expanse` |
| `{series_position}` | Series number | `9` |
| `{series_prefix}` | Expands to `{series} {series_position} - ` if series exists, empty otherwise | `The Expanse 9 - ` |
| `{year}` | Release year | `2021` |
| `{narrator}` | Narrator name | `Jefferson Mays` |
| `{lang}` | ISO 639-1 language code (en, de, fr) | `en` |
| `{track}` | Track number, zero-padded to total digit count | `01` |
| `{total_tracks}` | Total track count | `51` |
| `{track_title}` | Per-segment title (computed from segment_title_format) | `Leviathan Falls - 01/51` |
| `{ext}` | File extension | `mp3` |

**Empty variable handling:** If a variable resolves to empty, its surrounding separators collapse. `{title}.{lang}.{ext}` with no language → `Title.mp3` not `Title..mp3`. Path segments with all-empty variables are removed (no empty folders).

**Segment title format examples:**
- `"{title} - {track}/{total_tracks}"` → `Leviathan Falls - 15/51`
- `"{title} - {track} of {total_tracks}"` → `Leviathan Falls - 15 of 51`
- `"{title} - Part {track}"` → `Leviathan Falls - Part 15`
- `"{track:02d} - {title}"` → `15 - Leviathan Falls`

**Single source of truth:** All code paths that compute file paths or segment titles read these two settings. No per-feature format strings.

### 3. Smart Metadata Apply Pipeline

When metadata is applied (auto-fetch, manual search+apply, or bulk update), it triggers a complete pipeline:

```
Step 1: SNAPSHOT   — CoW version saved automatically (DB layer, via UpdateBook)
Step 2: APPLY      — Book-level fields updated (title, author, series, narrator, year, etc.)
                     Author resolved/created. Series resolved/created.
                     Fetched provenance values persisted.
                     Field-level change history recorded BEFORE mutation.
Step 3: GENERATE   — Segment titles computed from segment_title_format
                     Track numbers auto-assigned if missing (by filename sort order)
Step 4: RENAME     — Physical files moved/renamed to match path_format
                     Atomic: rename to temp, verify, rename to final
Step 5: WRITE      — Tags written to audio files:
                     title (segment title), album (book title), artist (author),
                     album_artist (author), track (number), genre (Audiobook),
                     year, narrator (comment or custom field), series (grouping)
Step 6: VERIFY     — Read tags back from file, compare to expected values
                     If mismatch → log warning, mark segment as "write_failed"
Step 7: RECORD     — Update segment records with new file paths
                     Update book record with new file path (if folder changed)
```

**Today we only do steps 1-2.** Steps 3-7 are new.

**Config flags (settings):**
- `auto_rename_on_apply: true` — rename files when metadata changes
- `auto_write_tags_on_apply: true` — write tags to audio files when metadata changes
- `verify_after_write: true` — read-after-write verification

All default to true. Users who want manual control can disable any step.

**Failure handling:**
- Any step fails → CoW version is there for one-click revert
- Step 6 (verify) fails → book flagged with `write_verification_failed` status, visible in UI
- Step 4 (rename) fails → partial state recorded, operation can be resumed
- Entire pipeline is idempotent — safe to re-run

### 4. Tag Editor Redesign

The editor is the escape hatch for when automation fails. Priority: fast, capable, keyboard-driven.

#### Single file selected:
- Comparison table with **inline-editable cells** — click any value cell to edit
- Tab between fields, Enter to save cell, Escape to cancel
- Match/mismatch chips are actionable: click → dropdown with Use File / Use Book / Edit

#### Multiple files selected (iTunes-style Get Info):
- **Top section: book-level fields** — author, title/album, series, genre, year, narrator, language, publisher
  - Shared values shown normally, editable
  - Mixed values show `< mixed >` in grey italic
  - Typing over `< mixed >` applies new value to all files
  - Leaving `< mixed >` untouched preserves per-file values
- **Bottom section: per-file table** — track#, segment title (from template), filename
  - Track numbers editable inline
  - Auto-fill button: sorts by filename, assigns sequential numbers
  - Segment title preview: shows what the title WILL be based on template + track number

#### No files selected:
- Provenance view (file/fetched/stored/override) — unchanged from today

#### Key interactions:
- **Save button** — batches all changes into one operation (one CoW version, triggers pipeline steps 3-7)
- **Preview toggle** — shows computed filenames/paths BEFORE committing
- **Keyboard-driven:** Tab/Shift-Tab, Ctrl+S to save, Escape to cancel edits
- No separate Edit Metadata dialog needed — the table IS the editor

## Implementation Order

1. **CoW versioning in PebbleDB** — foundation, fixes data loss risk immediately
2. **Path format template engine** — needed by pipeline and editor
3. **Smart apply pipeline** — steps 3-7 (generate, rename, write, verify, record)
4. **Tag editor redesign** — UI changes, inline editing, multi-select

## File Impact Summary

| Area | Files |
|------|-------|
| DB layer | `internal/database/store.go`, `pebble_store.go`, `sqlite_store.go` |
| DB mocks | `internal/database/mocks/mock_store.go` |
| Format engine | `internal/server/path_format.go` (new) |
| Apply pipeline | `internal/server/metadata_fetch_service.go`, `server.go` |
| Tagger | `internal/tagger/tagger.go` (needs real implementation) |
| Settings | `internal/config/config.go` |
| Frontend API | `web/src/services/api.ts` |
| Tag editor | `web/src/pages/BookDetail.tsx` |
| Components | `web/src/components/audiobooks/FileSelector.tsx`, `MetadataHistory.tsx` |
| Settings UI | `web/src/pages/Settings.tsx` (path format config) |
