# Diagnostics Export & AI Analysis

**Date:** 2026-03-14
**Status:** Approved (revised after spec review)

## Problem

Troubleshooting library issues (duplicates, bad metadata, broken imports, errors) requires manually querying the API, cross-referencing iTunes data, and hand-building analysis scripts. Users reporting bugs can't easily share their library state. There's no way to leverage AI for bulk data quality analysis.

## Solution

A dedicated Diagnostics page that generates categorized data exports (ZIP) for manual troubleshooting or direct submission to the OpenAI batch API for automated analysis. Results are shown as an actionable review list with approve/reject per suggestion.

## UI Flow

### Step 1: Category Selection

User picks a diagnostic category:

| Category | Data Included | JSONL Prompt Focus |
|----------|--------------|-------------------|
| **Error Analysis** | Last 24h logs, system info, config (redacted), recent operations, failed operations | Identify root causes, suggest fixes, flag patterns |
| **Deduplication** | All books (slim), authors, series, version groups, file hashes, iTunes albums | Find duplicate books, orphan tracks, missing merges, bad groupings |
| **Metadata Quality** | Books with missing fields, broken paths, narrator-as-author | Flag incorrect metadata, suggest corrections, identify patterns |
| **General / Custom** | Everything above combined | User describes the issue in free text |

Optional text box: "Describe what you're seeing" — included as context in the JSONL system prompt.

### Step 2: Export Generation (Async)

Backend endpoint `POST /api/v1/diagnostics/export` creates a background Operation (type="diagnostics_export") and returns an operation ID immediately. The ZIP is generated asynchronously and stored on disk. When ready, `GET /api/v1/diagnostics/export/:operationId/download` streams the ZIP.

ZIP contains:

**Always included:**
- `system_info.json` — OS, app version, DB type, book/author/series counts, config (secrets redacted with `***REDACTED***`). Note: uptime requires adding `StartTime` to the Server struct.
- `books.json` — all books with: id, title, author, narrator, series, format, duration, file_path, file_size, version_group_id, is_primary_version, work_id, itunes_persistent_id, year, publisher, library_state, marked_for_deletion
- `authors.json` — id, name, book_count, file_count, aliases
- `series.json` — id, name, book_count, file_count

**Category-specific:**
- **Error Analysis:** `logs.json` (last 24h system_activity_log — fetched via `GetSystemActivityLogs("", 10000)` then filtered by `CreatedAt >= now-24h` in application code), `operations.json` (last 100 operations with status/errors)
- **Deduplication:** `itunes_albums.json` (parsed from iTunes XML via `itunes.ParseLibrary` if `config.ITunesLibraryReadPath` is configured; **omitted with empty array if not configured**), `version_groups.json` (built by fetching all books via `GetAllBooks` and grouping by `VersionGroupID` in application memory)
- **Metadata Quality:** `missing_fields.json` (books missing title/author/series — computed from books.json data, no ffprobe needed)
- **General:** all of the above

**Always included:**
- `batch.jsonl` — pre-built OpenAI batch API file with category-appropriate prompts and chunked data

### Step 3: Two Actions

**Download ZIP** — fetches the completed ZIP from `GET /api/v1/diagnostics/export/:operationId/download`. User can attach to a GitHub issue for developer troubleshooting.

**Submit to AI** — uploads `batch.jsonl` to OpenAI batch API using the configured `openai_api_key`. Creates an Operation record (type="diagnostics_ai") to track progress. Polls for completion.

### Step 4: AI Results View

When the batch completes, results are parsed and stored in the operation's `result_data` as JSON with schema version:

```json
{
  "schema_version": 1,
  "suggestions": [...],
  "raw_responses": [...]
}
```

**Formatted review list** (default view):
- Suggestions grouped by action type: Merge Versions, Delete Orphans, Fix Metadata, Reassign Series
- Each suggestion shows: affected books, reason, proposed fix
- Approve/reject checkboxes per suggestion
- "Apply Selected" button to execute approved suggestions
- Applied suggestions create real operations with undo support via `operation_changes`

**"View Raw" toggle:**
- Shows the raw JSON response from each batch chunk
- Useful for debugging the AI's reasoning or extracting data manually

## API Endpoints

### `POST /api/v1/diagnostics/export`

Request body:
```json
{
  "category": "deduplication",
  "description": "I think I have duplicate Expanse books"
}
```

Response (immediate):
```json
{
  "operation_id": "01KKN...",
  "status": "generating"
}
```

### `GET /api/v1/diagnostics/export/:operationId/download`

Response: ZIP file download (Content-Type: application/zip) if ready, 202 if still generating, 404 if not found.

### `POST /api/v1/diagnostics/submit-ai`

Request body:
```json
{
  "category": "deduplication",
  "description": "Find and merge all duplicate books"
}
```

Response:
```json
{
  "operation_id": "01KKN...",
  "batch_id": "batch_abc123",
  "status": "submitted",
  "request_count": 30
}
```

### `GET /api/v1/diagnostics/ai-results/:operationId`

Response:
```json
{
  "status": "completed",
  "schema_version": 1,
  "suggestions": [
    {
      "id": "sug-001",
      "action": "merge_versions",
      "book_ids": ["01KJB...", "01KJX..."],
      "primary_id": "01KJB...",
      "reason": "Same audiobook in m4b and mp3 format",
      "applied": false
    }
  ],
  "raw_responses": [...]
}
```

### `POST /api/v1/diagnostics/apply-suggestions`

Request body:
```json
{
  "operation_id": "01KKN...",
  "approved_suggestion_ids": ["sug-001", "sug-003"]
}
```

Response:
```json
{
  "applied": 2,
  "failed": 0,
  "errors": []
}
```

## Suggestion Actions

| Action | Behavior |
|--------|----------|
| `merge_versions` | Calls service-layer merge function (extracted from `mergeBookDuplicatesAsVersions` handler) |
| `delete_orphan` | Soft-deletes the book (`marked_for_deletion = true`) — user can purge later |
| `fix_metadata` | Calls `UpdateBook` with the corrected fields from `fix` object |
| `reassign_series` | Updates `series_id` on the book |

## JSONL Prompt Design

Each batch request includes:
- **System prompt:** Category-specific instructions for what to analyze and the exact JSON output format expected
- **User message:** A chunk of the relevant data (500 books per chunk for dedup, 100 operations for errors, etc.)

The system prompt instructs the model to output a JSON array of suggestion objects with standardized fields (`action`, `book_ids`, `primary_id`, `reason`, `fix`). This makes parsing deterministic.

### Chunking Strategy

| Data Type | Chunk Size | Reason |
|-----------|-----------|--------|
| Books (dedup) | 500 per request | Fits in context, allows cross-referencing within chunk |
| iTunes albums (cross-ref) | 500 per request | Separate requests comparing iTunes ↔ our library |
| Logs (errors) | 200 log entries per request | Recent logs grouped by time window |
| Metadata (quality) | 500 books per request | Similar to dedup chunking |

Cross-chunk duplicates (book in chunk 1 is duplicate of book in chunk 37) are handled by a final "reconciliation" request that receives only the titles/authors/IDs flagged by previous chunks.

## Implementation Notes

- Export is async: `POST /export` returns operation ID, ZIP generated in background, downloaded via separate GET
- Need to add `StartTime time.Time` field to Server struct for uptime reporting
- Need to extract merge logic from `mergeBookDuplicatesAsVersions` HTTP handler into a `DiagnosticsService.MergeBooks(bookIDs []string, primaryID string)` service function
- Need a generic `DownloadBatchRaw(ctx, outputFileID)` function in `openai_batch.go` that returns raw JSON responses (existing download functions are typed to author-dedup domain)
- Need a multi-request JSONL builder (existing batch helpers create single-request JSONLs)
- iTunes XML parsing via `itunes.ParseLibrary(config.ITunesLibraryReadPath)` — **skip with empty array** if path not configured
- `version_groups.json` built by loading all books and grouping by `VersionGroupID` in memory
- `logs.json` time filtering: fetch via `GetSystemActivityLogs("", 10000)`, filter `CreatedAt >= now-24h` in Go
- Config secrets redacted with `***REDACTED***`
- ZIP uses Go stdlib `archive/zip` (no existing ZIP code in the codebase to reuse)
- Result data stored in `operation.result_data` with `schema_version: 1` for forward compatibility

## Frontend Components

- `web/src/pages/Diagnostics.tsx` — new page
  - Category selector (radio buttons or card grid)
  - Description text area
  - Download ZIP / Submit to AI buttons
  - Progress indicator while generating/polling
  - Results panel (shows after AI completion)
    - Suggestion list with approve/reject
    - View Raw toggle
    - Apply Selected button with confirmation dialog

## Testing

- **Unit:** ZIP generation includes expected files per category
- **Unit:** JSONL prompt structure is valid for OpenAI batch API
- **Unit:** Service-layer merge function works independently of HTTP handler
- **Integration:** Export endpoint returns operation ID, download returns valid ZIP
- **E2E:** Category selection → Download ZIP flow, Submit to AI → mock results → approve/apply flow
