<!-- file: docs/plans/2026-02-26-itunes-incremental-sync-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->

# iTunes Incremental Sync Design

## Problem

Re-importing an iTunes library skips existing books entirely. Play counts, ratings, bookmarks, and last-played timestamps that change in iTunes never propagate to the organizer.

## Solution

A background job that periodically re-reads iTunes Library.xml, matches existing books by `itunes_persistent_id`, and updates changed iTunes-authoritative fields. Also imports new audiobooks added since the last import.

## Synced Fields (iTunes → Organizer)

- `itunes_play_count`
- `itunes_rating`
- `itunes_bookmark`
- `itunes_last_played`
- `itunes_date_added`

## Not Synced

Title, author, narrator, series, file_path — these may have been edited by the user or metadata enrichment.

## Architecture

### Backend

- `executeITunesSync()` in `itunes.go` — reuses `groupTracksByAlbum` and `buildBookFromAlbumGroup`
- For each album group: lookup by `itunes_persistent_id` → update if changed, import if new
- Runs through operation queue (visible in Operations UI, cancelable)
- `POST /api/v1/itunes/sync` endpoint for manual trigger

### Scheduling

- Goroutine started in `server.go` after startup
- Checks fingerprint first — skips if Library.xml unchanged
- Configurable interval via `ITunesSyncInterval` (default 30 min)
- Only runs if a previous import source path exists in DB

### Config

- `ITunesSyncEnabled` (bool, default true)
- `ITunesSyncInterval` (int, minutes, default 30)

### Frontend

- "Sync Now" button in iTunes Import settings
- Sync operations appear in Operations bell indicator

## Out of Scope

- Bidirectional sync (use existing write-back)
- Conflict resolution (iTunes fields are authoritative)
- File operations (sync is metadata-only)
