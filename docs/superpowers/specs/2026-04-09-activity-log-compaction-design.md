# Activity Log Compaction Design

## Goal

Replace old individual activity log entries with daily digest rows that preserve what happened (book, action, outcome) in a compact, clickable format. Triggered automatically during maintenance windows and manually via a UI button.

## Problem

The activity log accumulates entries rapidly — 900+ pages at 50/page = 45K+ entries. Old entries are individually stored even though nobody reads them line by line. The existing `Summarize()` function creates bland summary rows grouped by operation_id+type, losing meaningful context.

## Design

### Database Changes

Add a `compacted` boolean column to the existing `activity_log` table (default `false`). A new migration adds:

```sql
ALTER TABLE activity_log ADD COLUMN compacted BOOLEAN NOT NULL DEFAULT 0;
CREATE INDEX idx_activity_compacted ON activity_log(compacted);
```

Compacted digest entries use the existing table with these conventions:
- `tier = 'digest'` — new tier value, distinct from audit/change/debug
- `type = 'daily_digest'`
- `source = 'compaction'`
- `summary` = human-readable header, e.g. "April 5, 2026 — 847 activities"
- `details` = JSON object with structured digest data (see below)
- `compacted = true`
- `timestamp` = end of that day (23:59:59 UTC) for correct sort order
- `level = 'info'`

No new tables needed.

### Digest Details JSON Structure

```json
{
  "date": "2026-04-05",
  "original_count": 847,
  "counts": {
    "metadata_applied": 12,
    "tag_written": 8,
    "organize_completed": 34,
    "organize_failed": 2,
    "book_added": 15,
    "scan_completed": 2,
    "maintenance_run": 1,
    "config_changed": 3
  },
  "items": [
    {
      "type": "organize_completed",
      "book": "Dead Sky Morning",
      "book_id": "01KNDC17MJX7FEX1FBQVP9N2CW",
      "summary": "moved to /Karina Halle/Dead Sky Morning/"
    },
    {
      "type": "metadata_applied",
      "book": "We Hunt Monsters 8",
      "book_id": "01KNDBA9SHDQZ0X13MSSHP4GBT",
      "summary": "title, narrator, series from Audible"
    },
    {
      "type": "tag_written",
      "book": "Aurora CV-01",
      "book_id": "01KNDBZEP434H5W1VHPZ247Z8P",
      "summary": "wrote 14 tags to 3 files"
    },
    {
      "type": "error",
      "book": "Brief Cases",
      "book_id": "01KNDC17MJX7FEX1FBQVP9N2CW",
      "summary": "tag write failed: taglib file not valid",
      "details": "path: /mnt/bigdata/.../Brief Cases.m4b, error: taglib: file not valid/supported"
    }
  ]
}
```

Each original entry becomes one item in the `items` array. Error/failure items include extra `details` with file paths and error messages. The `counts` object powers the summary header.

### Compaction Logic

New method `CompactByDay(olderThan time.Time)` on `ActivityStore`:

1. Query all entries where `tier IN ('change', 'debug')` AND `compacted = false` AND `timestamp < olderThan`, ordered by timestamp.
2. Group entries by date (UTC day boundary).
3. For each day with entries:
   a. Check if a digest row already exists for that date (idempotent — skip if found).
   b. Build `counts` map by aggregating entry types.
   c. Build `items` array — one item per original row:
      - `type`: from original entry's `type` field
      - `book`: from original entry's `summary` (parse book name) or `details.book_title`
      - `book_id`: from original entry's `book_id` field
      - `summary`: condensed one-liner from original summary + details
      - `details`: (errors only) include file paths, error messages from original details JSON
   d. Insert one digest row with the structured JSON.
   e. Delete all original entries for that day (same tier/date/compacted=false filter).
4. Return count of days compacted and entries deleted.

**Exclusions:**
- `audit` tier entries are NEVER compacted (high-priority, long retention)
- Entries already marked `compacted = true` are skipped
- Days with a single entry are still compacted (consistency)

### Item Summary Generation

For each entry type, extract a meaningful one-liner:

| Type | Summary Pattern |
|------|----------------|
| `metadata_applied` | "fields changed from Source" (parse from details) |
| `tag_written` | "wrote N tags to M files" (parse from details) |
| `organize_completed` | "moved to new_path" (parse from details) |
| `book_added` / `book_deleted` | "title by author" |
| `scan_completed` | "found N new, N updated" (parse from details) |
| `config_changed` | "key: old → new" (parse from details) |
| `maintenance_run` | "task_name completed" |
| Any error | full error message + file path from details |

Fallback: if details can't be parsed, use the original `summary` field verbatim.

### API

No new endpoints needed. The existing `GET /api/v1/activity` returns digest entries with `tier: 'digest'`. The frontend detects this tier and renders them as expandable cards.

Add one new endpoint for manual compaction trigger:

```
POST /api/v1/activity/compact
Body: { "older_than_days": 14 }
Response: { "days_compacted": 12, "entries_deleted": 3847 }
```

### Frontend Changes

**ActivityLog.tsx — Digest Row Rendering:**
- Detect `tier === 'digest'` entries
- Render as expandable card:
  - **Header** (always visible): date, total count, type breakdown chips (e.g. "12 metadata", "34 organized", "2 errors")
  - **Expanded body** (click to toggle): scrollable list of items, one line each
  - Each item shows: type icon, book name (clickable link to book detail), one-line summary
  - Error items highlighted in red
- Digest tier gets a distinct color (e.g. teal or orange) in the tier legend

**Compact Button:**
- Add a button in the Activity Log toolbar: "Compact" with a dropdown menu
- Options: "Older than 7 days", "14 days", "30 days", "60 days"
- On click: POST to `/api/v1/activity/compact` with the selected days
- Show loading spinner, then toast with result ("Compacted 12 days, removed 3,847 entries")
- Refresh the activity list

**Tier Filter Update:**
- Add 'digest' to the tier toggle buttons so users can filter to only see digests or hide them

### Maintenance Window Integration

Modify the existing `cleanup_activity_log` scheduled task in `scheduler.go`:

1. **New first step**: Run `CompactByDay(now - ActivityLogCompactionDays)` 
2. Then existing: Summarize change tier (this becomes a no-op since entries are already compacted)
3. Then existing: Prune debug tier (also a no-op for compacted days)

New config key: `ActivityLogCompactionDays` (default: 14). Entries older than this get auto-compacted during maintenance.

### Edge Cases

- **Empty days**: If a date has entries but all are audit tier, no digest is created (nothing to compact)
- **Partial compaction**: If compaction fails mid-day (crash), the next run picks up where it left off because it checks for existing digest rows per day
- **Very large days**: A day with 5000+ entries will produce a large `items` JSON. Cap at 500 items in the array; if exceeded, keep all errors + a representative sample + add `"truncated": true` and `"truncated_count": 4500` to the details.
- **Timezone**: Use UTC day boundaries for grouping to avoid timezone-dependent compaction

### Config

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ActivityLogCompactionDays` | int | 14 | Auto-compact entries older than this during maintenance |

### Files to Create/Modify

**Backend:**
- `internal/database/activity_store.go` — add `CompactByDay()` method, item summary extraction helpers
- `internal/server/activity_handlers.go` — add `POST /api/v1/activity/compact` handler
- `internal/server/server.go` — register new route
- `internal/server/scheduler.go` — add compaction step to `cleanup_activity_log` task
- `internal/config/config.go` — add `ActivityLogCompactionDays` config key

**Frontend:**
- `web/src/pages/ActivityLog.tsx` — digest row rendering, compact button, tier filter update
- `web/src/services/api.ts` — add `compactActivityLog()` API function
