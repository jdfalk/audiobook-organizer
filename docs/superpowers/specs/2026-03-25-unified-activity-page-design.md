# Unified Activity Page Design

**Date:** 2026-03-25
**Status:** Approved
**Replaces:** Operations page (`/operations`), current Activity page (`/activity`)

## Goal

Merge the Operations page and Activity page into a single unified Activity page that captures **every log line** the server produces. One page to see everything — operations progress, event history, system logs, errors — eliminating the need to SSH into the server to read logs.

## Background

The current system has two overlapping pages:

- **Operations** (`/operations`) — shows active operations with progress bars, operation history with expand/collapse, separate "Logs" and "Changes" modals, revert dialogs. 771 lines of React.
- **Activity** (`/activity`) — shows activity log entries with tier/type filters and a table. 255 lines of React. Just built but only captures specific dual-write events, not all server output.

These serve the same fundamental purpose — "what happened" — but split across two views with different data sources. The Operations page also has a redundant "Changes" button now that the activity log tracks changes.

Inspiration: Radarr/Sonarr's unified events page, but with our tagging/filtering approach which is more flexible.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Log capture scope | Everything (hybrid) | All `log.Printf` goes to activity.db. Debug hidden by default, shown on demand. 30-day debug retention keeps DB bounded. |
| Performance | Buffered channel | ~10K capacity channel with batch INSERT. Non-blocking sends. Drop debug entries if channel full. No blocking the hot path. |
| Active operations | Pinned section at top | Always visible above feed. Collapsible. Pin toggle to keep visible even when empty. |
| Operation detail | Filter-in-place | Clicking operation sets `operation_id` filter on the feed. Reuses existing filter infrastructure. URL updates for bookmarkability. |
| Operations page | Remove entirely | Redirect `/operations` to `/activity`. No legacy page. |
| Source filtering | Dropdown with localStorage | Checkbox per source subsystem. Muted sources persisted in browser. Badge shows hidden count. |
| Debug visibility | Hidden by default | Debug tier chip OFF by default. When ON, debug entries shown at 60% opacity. Source filters still apply. |

## Architecture

### Backend Changes

#### 1. Global Log Capture

Intercept Go's standard `log` package output by replacing `log.SetOutput()` with a custom `io.Writer` that:
1. Writes to stdout (preserving current behavior and journalctl capture)
2. Parses the log line to extract level, source, and message
3. Sends an `ActivityEntry` to the buffered channel

```
log.Printf("[info] scheduler: iTunes sync started")
         ↓ custom Writer
    ┌─────────────┐
    │ tee to stdout│ → journalctl (unchanged)
    │ parse line   │ → tier=debug, type=system, level=info, source=scheduler
    │ channel send │ → buffered channel (non-blocking)
    └─────────────┘
         ↓ background goroutine
    ┌─────────────┐
    │ batch INSERT │ → activity.db (50-100 rows per tx)
    └─────────────┘
```

**Log line parsing rules:**
- `[debug]` → level=debug, tier=debug
- `[info]` → level=info, tier=debug (system logs default to debug tier; only explicit `Record()` calls produce change/audit tier)
- `[warn]` → level=warn, tier=debug
- `[error]` → level=error, tier=debug
- `[GIN]` prefix → source=gin, level=info, tier=debug
- Subsystem extracted from `[level] subsystem: message` format
- Lines without level prefix → level=info, source=server

**GIN request logs** get source="gin" so they can be muted via source filtering without affecting other debug output.

**Dual-write path removal:** The existing `globalActivityRecorder` in `logger/standard.go` (which writes INFO+ logger calls directly to `activityService.Record()`) is **removed**. All log output now flows through the teeWriter → buffered channel path. This prevents duplicate entries. The explicit `Record()` calls for change/audit tier events (operation changes, metadata changes, renames, etc.) remain — those produce entries with tier=change or tier=audit, not tier=debug.

#### 2. Buffered Write Channel

```go
type activityBuffer struct {
    ch      chan database.ActivityEntry
    store   *database.ActivityStore
    done    chan struct{}
}
```

- Channel capacity: 10,000 entries
- Background goroutine drains channel, batches INSERTs (50-100 per transaction)
- Non-blocking send: if channel full, drop debug-tier entries silently, log a warning for non-debug drops
- Flush on shutdown (drain remaining entries before closing)

#### 3. New API Parameters

Add to `GET /api/v1/activity`:

| Param | Type | Description |
|-------|------|-------------|
| `search` | string | Full-text search on summary field (`LIKE %term%`) |
| `exclude_sources` | string | Comma-separated sources to hide (e.g., `gin,background`) |
| `source` | string | Show only this source (exclusive with exclude_sources) |

Add new endpoint:

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/activity/sources` | Returns list of distinct sources with entry counts. **Filter-aware**: accepts the same `tier`, `level`, `since`, `until` params so counts reflect the current view, not global totals. |

#### 4. Active Operations Endpoint

The existing `GET /api/v1/operations/active` stays as-is — the pinned operations section polls it for progress bars. No change needed; this endpoint already returns `ActiveOperationSummary[]` with progress/total/message.

### Frontend Changes

#### 1. Remove Operations Page

- Delete `web/src/pages/Operations.tsx`
- Remove "Operations" from `Sidebar.tsx` nav items
- In `App.tsx`: replace Operations route with redirect to `/activity`

**Migrated operation actions:**
- **Cancel** — cancel button stays on active operations in the pinned section (calls `api.cancelOperation()`)
- **Revert** — for completed organize/metadata_fetch operations, a revert icon appears in the activity feed row (with confirmation dialog). Only shown on entries with `type=organize` or `type=metadata_fetch` and `tier=change`.
- **Clear stale/failed** — toolbar button in the pinned operations section header: "Clear Stale" calls `api.clearStaleOperations()`. No bulk-clear for completed operations (the activity feed IS the history now, pruning handles cleanup).
- **Pre-migration operations** — operations that ran before the teeWriter was deployed have no activity.db entries. When filtering by `operation_id` returns zero results, show a message: "No activity entries for this operation (pre-migration)." The old `operation_logs` table data is not queried.

#### 2. Unified Activity Page (`ActivityLog.tsx` rewrite)

**Layout (top to bottom):**

1. **Header bar** — "Activity" title, refresh button, auto-refresh indicator
2. **Pinned operations section** — polls `GET /api/v1/operations/active`
   - Shows progress bars for running operations
   - Collapsible with pin toggle (pinned = visible even when empty)
   - Pin state persisted in localStorage
   - Each operation row: type, message, progress bar, elapsed time, cancel button
3. **Filter bar** — compound filters, all composable
   - Text search input (searches summary field)
   - Tier chips: audit, change, debug (toggle on/off, debug OFF by default)
   - Type dropdown: All Types + known event types
   - Level dropdown: All Levels / debug / info / warn / error
   - Date range: since/until inputs (date picker or RFC3339 text)
   - Sources dropdown: checkbox per source, entry counts, All/None/Reset, localStorage persistence
   - Active filter chips: show applied filters with ✕ to remove
   - Entry count display
4. **Activity feed** — paginated table of entries
   - Columns: Time, Level (chip), Type (chip), Summary, Source, Tags
   - Visual hierarchy: amber background for warnings, red for errors, green for completions, debug entries at 60% opacity
   - Clickable "view operation →" links that set operation_id filter
   - Clickable book IDs that navigate to book detail
   - Pagination: 50 per page with page controls

**Source dropdown behavior:**
- Button shows "Sources ▾" with optional badge "-N" showing hidden count
- Dropdown lists all sources from `GET /api/v1/activity/sources`
- Each row: checkbox, source name, entry count
- Unchecked sources excluded via `exclude_sources` API param
- Bottom actions: All (check all), None (uncheck all), Reset (restore defaults)
- Choices persisted in localStorage key `activity-source-prefs`
- Muted sources appear with strikethrough and red background in dropdown

**Operation detail flow:**
1. User clicks operation row or "view operation →" link
2. `operation_id` filter is set, URL updates to `/activity?operation_id=XXX`
3. Feed shows all entries for that operation (logs, changes, errors)
4. "Clear filter" chip in filter bar to return to full view

#### 3. ChangeLog Component

`web/src/components/ChangeLog.tsx` — unchanged. Still reads from activity API with `book_id` filter. Already updated in previous work.

### Data Flow

```
Server code                    Activity System                    Frontend
─────────                      ───────────────                    ────────
log.Printf() ──→ teeWriter ──→ buffered channel ──→ activity.db
                     │
                     └──→ stdout (journalctl)

logger.Info() ──→ StandardLogger.log() ──→ globalActivityRecorder ──→ buffered channel
                       │
                       └──→ stdout + activityWriter (old system)

operations ──→ ActivityRecorder hook ──→ buffered channel

                                    GET /api/v1/activity ←── Activity page (polls)
                                    GET /api/v1/operations/active ←── Pinned ops (polls)
                                    GET /api/v1/activity/sources ←── Sources dropdown
```

### Retention (unchanged from previous spec)

| Tier | Retention | Action |
|------|-----------|--------|
| audit | Forever | Never deleted |
| change | 90 days | Summarized (compressed into summary rows) |
| debug | 30 days | Hard deleted |

The `cleanup_activity_log` maintenance task handles this daily.

## Files Affected

### Create
| File | Purpose |
|------|---------|
| `internal/server/activity_writer.go` | teeWriter + buffered channel + batch inserter |
| `internal/server/activity_writer_test.go` | Tests for log capture and buffering |

### Modify (Backend)
| File | Change |
|------|--------|
| `internal/server/server.go` | Replace `log.SetOutput()`, init activity writer, add sources endpoint |
| `internal/server/activity_handlers.go` | Add `search`, `exclude_sources`, `source` params; add `listActivitySources` handler |
| `internal/database/activity_store.go` | Add `Search`, `Source`, `ExcludeSources` fields to `ActivityFilter`; add `GetDistinctSources()` method; add `CREATE INDEX idx_activity_source ON activity_log(source)` to schema |

### Modify (Frontend)
| File | Change |
|------|--------|
| `web/src/pages/ActivityLog.tsx` | Full rewrite: pinned ops, compound filters, source dropdown, operation detail |
| `web/src/services/activityApi.ts` | Add `search`, `exclude_sources`, `source` params; add `fetchActivitySources()` |
| `web/src/App.tsx` | Replace Operations route with redirect to `/activity` |
| `web/src/components/layout/Sidebar.tsx` | Remove "Operations" nav item |

### Delete
| File | Reason |
|------|--------|
| `web/src/pages/Operations.tsx` | Replaced by unified Activity page |

## Out of Scope

- Migrating old operation_logs / operation_changes / metadata_changes_history data into activity.db (future migration)
- WebSocket/SSE for real-time streaming (polling is sufficient for now)
- Log export/download feature
- Custom retention periods per source
