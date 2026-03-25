# Unified Change Tracking System

**Date:** 2026-03-25
**Status:** Draft

## Problem

The "Changes" button on the Operations page is useless — it shows empty for most operations because:
1. Most operations (scan, iTunes import, metadata fetch) write zero `operation_changes` records
2. `queueStoreAdapter.CreateOperationChange` is a no-op (`queue.go:521`)
3. Metadata changes go to a separate `metadata_changes_history` table not visible from Operations
4. `system_activity_log` has data but no API endpoint exposes it
5. There are 5 separate logging systems that don't talk to each other

## Current State (5 disconnected systems)

| Table | What writes it | What reads it | UI |
|-------|---------------|---------------|-----|
| `operations` | All async jobs | Operations page list | Works |
| `operation_logs` | `progress.Log()` calls | Operations Logs dialog | Partially works |
| `operation_changes` | organize, rename, author-merge only | Operations Changes button | **Mostly empty** |
| `metadata_changes_history` | metadata fetch/apply, manual edits | Book detail ChangeLog | Works but isolated |
| `system_activity_log` | Background goroutines | **Nothing** | Invisible |
| `book_path_history` | Renames via apply pipeline | Book detail ChangeLog | Works |

## Solution

### Phase 1: Make the Changes button work (quick fix)

**Fix the no-op:** Remove the no-op at `queue.go:521` so `CreateOperationChange` actually persists.

**Add change tracking to all major operations:**
- iTunes sync: record `itunes_sync` changes (books updated, play counts changed)
- Metadata fetch/apply: also write to `operation_changes` (not just `metadata_changes_history`)
- Scan: record `book_import` changes for new books discovered
- Tag write-back: record `tag_write` changes with before/after values

**Result:** The Changes button shows actual data for every operation type.

### Phase 2: Unified activity feed (Operations page upgrade)

Replace the current Operations page with a unified activity feed that merges ALL sources:

1. **Recent Activity** — merged timeline from all 5 tables, sorted by timestamp
2. **Operation Details** — click any operation to see its logs, changes, and metadata diffs
3. **Filters** — by type (scan, sync, metadata, organize), by status, by date range
4. **Per-book link** — from any change, click to go to the book detail

**API:** `GET /api/v1/activity?limit=50&offset=0&type=itunes_sync&since=2026-03-25T00:00:00Z`

Returns merged entries from all tables with a unified schema:
```json
{
  "entries": [
    {
      "id": "...",
      "timestamp": "2026-03-25T23:20:16Z",
      "source": "itunes_sync",
      "operation_id": "01KMHJAAQJ...",
      "book_id": "01KKRY0QPG...",  // optional
      "level": "info",
      "summary": "Sync completed: 312 updated, 39 new",
      "details": {"updated": 312, "new": 39, "unchanged": 11649},
      "change_type": "sync_completed"
    }
  ],
  "total": 1234
}
```

### Phase 3: iTunes sync change detail

When an iTunes sync runs, record EXACTLY what changed for each book:
- Play count changes: `old_play_count → new_play_count`
- Rating changes: `old_rating → new_rating`
- New books imported: title, author, PID
- Books with path errors: path, error

Store in `operation_changes` linked to the sync operation ID. The Changes button then shows a detailed breakdown of what the sync did.

### Phase 4: Expose system_activity_log

Add `GET /api/v1/system/activity-log?limit=50&source=itunes-scheduler` endpoint. Show in a new "System Log" section on the Operations page or in Settings.

## Implementation Priority

1. **Fix the no-op** (`queue.go:521`) — 1 line change
2. **Add iTunes sync change tracking** — record what each sync updates
3. **Add scan change tracking** — record new books found
4. **Unified activity feed API** — merge all sources
5. **Frontend upgrade** — replace Operations page Changes button with useful data
