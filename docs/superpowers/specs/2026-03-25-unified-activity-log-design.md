# Unified Activity Log

**Date:** 2026-03-25
**Status:** Approved

## Problem

The app has 5 disconnected logging/tracking systems that don't talk to each other:
- `operations` — job lifecycle (works)
- `operation_logs` — per-operation log lines (partially works)
- `operation_changes` — structural changes (mostly empty, was a no-op)
- `metadata_changes_history` — field-level metadata diffs (works but isolated)
- `system_activity_log` — background events (invisible to users)

The "Changes" button on the Operations page shows empty for most operations. There's no unified way to see what happened, when, or why.

## Solution

One table. One API. One UI component. Everything writes to `activity_log` in a dedicated SQLite database (`activity.db`). Freeform tags for discovery, JSON details for payload, three tiers for retention.

## Schema

**Database:** Separate SQLite file `activity.db` alongside the PebbleDB main store.

```sql
CREATE TABLE activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    tier TEXT NOT NULL,              -- 'audit', 'change', 'debug'
    type TEXT NOT NULL,              -- event type (see table below)
    level TEXT NOT NULL DEFAULT 'info',  -- 'debug', 'info', 'warn', 'error'
    source TEXT NOT NULL,            -- 'scheduler', 'api', 'manual', 'background'
    operation_id TEXT,               -- links to operations table (nullable)
    book_id TEXT,                    -- links to book (nullable)
    summary TEXT NOT NULL,           -- human-readable one-liner
    details JSON,                    -- structured payload
    tags TEXT,                       -- comma-separated freeform tags
    pruned_at DATETIME               -- set when detail→summary compression happens
);

CREATE INDEX idx_activity_timestamp ON activity_log(timestamp);
CREATE INDEX idx_activity_type_ts ON activity_log(type, timestamp);
CREATE INDEX idx_activity_operation ON activity_log(operation_id) WHERE operation_id IS NOT NULL;
CREATE INDEX idx_activity_book_ts ON activity_log(book_id, timestamp) WHERE book_id IS NOT NULL;
CREATE INDEX idx_activity_tier ON activity_log(tier);
CREATE INDEX idx_activity_tags ON activity_log(tags);
```

## Tiers

| Tier | Purpose | Retention | Pruning |
|------|---------|-----------|---------|
| `audit` | Who did what (future multi-user) | Forever | Never |
| `change` | What changed and why | Configurable (default 90 days) | Details compressed to summary. Summary row stays forever with `pruned_at` set. Original detail rows deleted. |
| `debug` | Progress updates, file checks, system events | Configurable (default 30 days) | Deleted entirely |

## Event Types

| Type | Tier | Example summary |
|------|------|----------------|
| `itunes_sync` | change | "Sync: 312 updated, 39 new (play counts: 280, ratings: 32)" |
| `metadata_apply` | change | "Applied Audible metadata: title, author, narrator, series" |
| `metadata_fetch` | change | "Fetched metadata from Audible for 'Return of the Archon'" |
| `tag_write` | change | "Wrote 23 tags to Return of the Archon.m4b (8 custom)" |
| `rename` | change | "Moved 80 books to /audiobook-organizer/Author/" |
| `scan` | change | "Scan found 39 new books in /import/" |
| `organize` | change | "Organized 150 books into library structure" |
| `transcode` | change | "Transcoded Return of the Archon (MP3 → M4B)" |
| `merge` | change | "Merged 3 duplicate books into version group" |
| `delete` | change | "Soft-deleted 'Untitled' (orphan track)" |
| `import` | change | "Imported from iTunes: 'The Way of Kings' by Brandon Sanderson" |
| `isbn_enrichment` | change | "Found ISBN 9781234567890 for 'Return of the Archon'" |
| `system` | debug | "Maintenance window started" |
| `progress` | debug | "Processing book 45 of 312..." |
| `error` | debug | "Failed to stat /path/to/file.m4b: no such file" |

## Summarization

The maintenance task (`cleanup_activity_log`) runs during the maintenance window:

1. Find `debug` entries older than `debug_retention_days` → delete
2. Find `change` entries older than `change_retention_days` where `pruned_at IS NULL`:
   a. Group by `(operation_id, type)`
   b. Build summary from the group:
      - Count entries
      - Collect unique book IDs into `details.books[]`
      - Collapse repetitive messages (e.g., "moved 1 of 80" × 80 → "Moved 80 books")
      - Preserve from/to paths, field names, old/new values in summary
   c. Insert ONE summary row: `tier='change'`, `pruned_at=NOW()`
   d. Delete the original detail rows

### Summarization examples

**Before (80 rows):**
```
moved file 1 of 80: /old/book1.m4b → /new/book1.m4b
moved file 2 of 80: /old/book2.m4b → /new/book2.m4b
...
moved file 80 of 80: /old/book80.m4b → /new/book80.m4b
```

**After (1 row):**
```
summary: "Moved 80 books from /old/ to /new/"
details: {
  "count": 80,
  "books": ["id1", "id2", ...],
  "from_dir": "/old/",
  "to_dir": "/new/",
  "original_entry_count": 80
}
pruned_at: "2026-06-25T01:00:00Z"
```

**Before (312 rows from iTunes sync):**
```
Updated play count for 'Book A': 5 → 6
Updated play count for 'Book B': 0 → 1
Updated rating for 'Book C': 0 → 80
...
```

**After (1 row):**
```
summary: "iTunes sync: updated 312 books (play counts: 280, ratings: 32)"
details: {
  "updated_count": 312,
  "play_count_changes": 280,
  "rating_changes": 32,
  "books": ["id1", "id2", ...],
  "operation_id": "01KMHK8NFH..."
}
pruned_at: "2026-06-25T01:00:00Z"
```

## Service Interface

```go
type ActivityLogService struct {
    db *sql.DB  // activity.db
}

func (s *ActivityLogService) Record(entry ActivityEntry) error
func (s *ActivityLogService) Query(filter ActivityFilter) ([]ActivityEntry, int, error)
func (s *ActivityLogService) Summarize(olderThan time.Time, tier string) (int, error)
func (s *ActivityLogService) Prune(olderThan time.Time, tier string) (int, error)

type ActivityEntry struct {
    ID          int64
    Timestamp   time.Time
    Tier        string   // "audit", "change", "debug"
    Type        string   // event type
    Level       string   // "debug", "info", "warn", "error"
    Source      string   // "scheduler", "api", "manual", "background"
    OperationID string   // optional
    BookID      string   // optional
    Summary     string
    Details     map[string]any  // JSON
    Tags        []string        // freeform
    PrunedAt    *time.Time
}

type ActivityFilter struct {
    Limit       int
    Offset      int
    Type        string
    Tier        string
    Level       string
    OperationID string
    BookID      string
    Since       *time.Time
    Until       *time.Time
    Tags        []string   // all must match
}
```

## API

### `GET /api/v1/activity`

Query params: `limit`, `offset`, `type`, `tier`, `level`, `operation_id`, `book_id`, `since`, `until`, `tags` (comma-separated)

Response:
```json
{
  "entries": [
    {
      "id": 12345,
      "timestamp": "2026-03-25T23:20:16Z",
      "tier": "change",
      "type": "itunes_sync",
      "level": "info",
      "source": "scheduler",
      "operation_id": "01KMHJAAQJ...",
      "book_id": null,
      "summary": "Sync: 312 updated, 39 new",
      "details": {"updated": 312, "new": 39},
      "tags": ["scheduled", "itunes"],
      "pruned_at": null
    }
  ],
  "total": 5678
}
```

## Write Path Replacements

Each existing write path calls `activityLog.Record(...)` instead of (or in addition to) the old method:

| Current call | New call | Tier |
|-------------|----------|------|
| `store.CreateOperationChange(change)` | `activityLog.Record({Tier:"change", Type:change.ChangeType, BookID:change.BookID, ...})` | change |
| `store.RecordMetadataChange(record)` | `activityLog.Record({Tier:"change", Type:"metadata_apply", BookID:record.BookID, ...})` | change |
| `store.RecordPathChange(change)` | `activityLog.Record({Tier:"change", Type:"rename", BookID:change.BookID, ...})` | change |
| `store.AddSystemActivityLog(src, lvl, msg)` | `activityLog.Record({Tier:"debug", Type:"system", Source:src, ...})` | debug |
| `store.AddOperationLog(opID, lvl, msg, details)` | `activityLog.Record({Tier:"debug", Type:opType, OperationID:opID, ...})` | debug |
| iTunes sync per-book update | `activityLog.Record({Tier:"change", Type:"itunes_sync", BookID:id, ...})` | change |
| Scan new book | `activityLog.Record({Tier:"change", Type:"scan", BookID:id, ...})` | change |
| Tag write per-file | `activityLog.Record({Tier:"change", Type:"tag_write", BookID:id, ...})` | change |

## Frontend

### Operations page "Changes" button
Queries: `GET /api/v1/activity?operation_id=X&tier=change`
Shows the unified activity entries for that operation.

### Book detail ChangeLog component
Queries: `GET /api/v1/activity?book_id=X&tier=change&limit=50`
Replaces the current `changelog_service.go` merger.

### New "Activity" nav item (optional)
Full activity feed with filters by type, tier, date range, tags.

## Config

```go
ActivityLogRetentionChangeDays int  `json:"activity_log_retention_change_days"`  // default 90
ActivityLogRetentionDebugDays  int  `json:"activity_log_retention_debug_days"`   // default 30
```

## Scheduler task

Register `cleanup_activity_log` maintenance task:
1. Call `Summarize(now - changeDays, "change")` — compress old change entries
2. Call `Prune(now - debugDays, "debug")` — delete old debug entries
3. Log: "Activity log cleanup: summarized N change entries, pruned M debug entries"

## Migration Path

1. Create `activity.db` alongside PebbleDB on first startup
2. New `ActivityLogService` initialized in `NewServer()`
3. Add `Record()` calls alongside existing write calls (dual-write)
4. Frontend reads from new API
5. Stop writing to old tables (leave them read-only)
6. Future migration: convert old table data to summary entries, drop old tables

## Testing

- Unit: `Record` + `Query` round-trip
- Unit: `Summarize` compresses correctly (80 rows → 1 summary)
- Unit: `Prune` deletes debug entries, preserves change/audit
- Unit: Tag filtering works
- Integration: iTunes sync produces activity entries
- Integration: Metadata apply produces activity entries
- E2E: Operations "Changes" button shows real data
- E2E: Book ChangeLog shows activity entries
