# Failed Book Quarantine — Design Spec

**Date:** 2026-04-22
**Status:** Approved

---

## Problem

~29 M4B files in the library are permanently unreadable by taglib. They
currently sit in the main library and are silently skipped by write-back,
organize, and iTunes sync — with no visibility. There is no way to mark a
file as permanently failed, remove it from normal operations, notify
external systems (webhooks), or purge it from iTunes.

---

## Goal

A `.failed/` quarantine folder inside the library root. Files moved there
are excluded from all write-back, organize, scan mutation, and iTunes sync.
Failed books remain visible in the UI (filtered by default). Quarantine can
be triggered manually by an admin or automatically by the system. An
EventBus event fires on quarantine so webhooks can react.

---

## Approach: Path-as-State

The file's location under `.failed/` is the canonical quarantine state. No
status enum to keep in sync. Every existing guard that checks
`isProtectedPath` automatically hard-blocks `.failed/` paths.

---

## Data Model

### Migration (new migration number)

```sql
ALTER TABLE books ADD COLUMN quarantine_reason TEXT;
ALTER TABLE books ADD COLUMN quarantined_at    TIMESTAMP;
```

- `quarantined_at IS NOT NULL` is the single source of truth for "is
  quarantined."
- `book_path_history.change_type` gains two new values: `"quarantine"` and
  `"unquarantine"`.

---

## Path Guard

`isProtectedPath` (defined in `internal/server/server.go` and duplicated in
`internal/metafetch/helpers.go`) gains one additional branch:

```go
if strings.Contains(filepath.ToSlash(path), "/.failed/") {
    return true
}
```

This covers all existing callers automatically:

| Caller | Effect |
|--------|--------|
| `metafetch/service.go` write-back | Skipped |
| `metafetch/service.go` runApplyPipeline | Skipped |
| `revert_service.go` revert | Skipped |
| `audiobook_service.go` delete | Skipped |
| `metadata_handlers.go` batch ops | Skipped |

The scanner skips `.failed/` by hardcoding a directory-name check alongside
the existing `.bak-*` / `.tmp.` skip logic in `internal/scanner/scanner.go`.

---

## Quarantine Action

`QuarantineBook(bookID, reason string) error` on the server service:

1. Load book from DB.
2. Compute destination: `{rootDir}/.failed/{author}/{title}/{filename}`.
3. `os.MkdirAll` destination directory.
4. `os.Rename` original path → destination (atomic, same filesystem).
5. Update book in DB: `FilePath = newPath`, `quarantine_reason = reason`,
   `quarantined_at = now`.
6. Insert into `book_path_history`: `change_type = "quarantine"`,
   `old_path = original`, `new_path = destination`.
7. If book has iTunes PID → set `itunes_sync_status = "purge_pending"` so
   the iTunes scheduler deletes it on next run.
8. Publish `book.quarantined` event via EventBus.

### EventBus Event

**Type:** `book.quarantined`

```json
{
  "book_id":       "...",
  "title":         "...",
  "author":        "...",
  "file_path":     "...",
  "original_path": "...",
  "reason":        "...",
  "quarantined_at":"..."
}
```

### Un-quarantine

`UnquarantineBook(bookID string) error`:

1. Load book.
2. `os.Rename` `.failed/` path → original path (computed from path history).
3. Update book: clear `quarantine_reason`, `quarantined_at`.
4. Insert `book_path_history` with `change_type = "unquarantine"`.
5. No automatic rescan — admin triggers manually.

---

## iTunes Purge

On quarantine, if the book has an iTunes PID:

- Set `itunes_sync_status = "purge_pending"`.
- The existing iTunes write-back scheduler picks this up on its next run and
  sends a delete command to iTunes.
- `"purge_pending"` is a new value alongside `"synced"`, `"dirty"`,
  `"unlinked"`, `"pending"`.

---

## Triggering

### Manual

| Endpoint | Permission |
|----------|------------|
| `POST /api/v1/audiobooks/:id/quarantine` body `{"reason":"..."}` | `PermSettingsManage` |
| `DELETE /api/v1/audiobooks/:id/quarantine` | `PermSettingsManage` |

### Automatic — Transcode migration

`transcodeMalformedM4BFiles` currently sets a `transcode_skip_*` PebbleDB
flag when a file is permanently unreadable after full AAC transcode. It will
also call `QuarantineBook` at that point with reason:

> `"taglib cannot parse file after full AAC transcode"`

The existing skip flag stays as the "don't retry" guard. On next startup with
the new code, the 29 known-bad files are quarantined automatically.

### Automatic — Scanner

When taglib `ReadTags` fails on a file during a normal scan:

1. Increment a `scan_fail_{hash8}` counter in PebbleDB (keyed on
   `sha256(path)[:8]`, same scheme as transcode skip keys).
2. After **3 consecutive failures**, call `QuarantineBook` with reason:

   > `"taglib failed to read file after 3 consecutive scan attempts"`

3. Counter resets to zero on any successful read.

---

## UI

- Failed books are **hidden by default**. A "Show Failed" toggle in the
  library filter bar reveals them.
- Failed books display a red **"Failed"** badge and the `quarantine_reason`
  string.
- Book detail page shows "Quarantine" / "Un-quarantine" buttons for admins
  (`PermSettingsManage`).
- The Files & History tab records the quarantine move in the path history
  timeline.

---

## What Is Not Changing

- The 29 permanently-malformed files that taglib cannot read even after full
  AAC transcode will be quarantined automatically on next startup once this
  feature ships. Their `transcode_skip_*` PebbleDB flags remain in place.
- The `malformed_m4b_transcode.go` startup call was already removed. A new
  one-time startup migration (`quarantine_known_bad_v1`) walks the
  `transcode_skip_*` PebbleDB keys, looks up each file by path hash, and
  calls `QuarantineBook` for any that are not already quarantined. This
  handles the 29 existing files without re-running the full transcode walk.
- No changes to any existing scanner exclusion config — `.failed/` is
  hardcoded, not a user-configurable pattern.

---

## Files to Create / Modify

| File | Change |
|------|--------|
| `internal/database/migrations.go` | New migration: `quarantine_reason`, `quarantined_at` on books; `"quarantine"` / `"unquarantine"` change_type |
| `internal/database/store.go` | Add fields to `Book` struct; add `"purge_pending"` iTunes status constant |
| `internal/database/pebble_store.go` | `QuarantineBook`, `UnquarantineBook`, `GetQuarantinedBooks` |
| `internal/server/server.go` | Extend `isProtectedPath` with `/.failed/` branch |
| `internal/metafetch/helpers.go` | Same `isProtectedPath` extension |
| `internal/scanner/scanner.go` | Skip `.failed/` directory; scan-fail counter logic |
| `internal/server/audiobook_service.go` | Wire `QuarantineBook` / `UnquarantineBook` |
| `internal/server/audiobooks_handlers.go` | New quarantine/unquarantine HTTP handlers |
| `internal/server/server.go` (routes) | Register `POST/DELETE /audiobooks/:id/quarantine` |
| `internal/server/quarantine_known_bad.go` | New one-time startup migration for the 29 known-bad files |
| `internal/plugin/events.go` (or equivalent) | Define `book.quarantined` event type |
| `web/src/` | "Show Failed" toggle; Failed badge; Quarantine button |
