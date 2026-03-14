# Diagnostics Export & AI Analysis

**Date:** 2026-03-14
**Status:** Approved

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
| **Metadata Quality** | Books with missing fields, file vs DB tag mismatches, broken paths, narrator-as-author | Flag incorrect metadata, suggest corrections, identify patterns |
| **General / Custom** | Everything above combined | User describes the issue in free text |

Optional text box: "Describe what you're seeing" — included as context in the JSONL system prompt.

### Step 2: Export Generation

Backend endpoint `POST /api/v1/diagnostics/export` generates a ZIP containing:

**Always included:**
- `system_info.json` — OS, app version, uptime, DB type, book/author/series counts, config (secrets redacted)
- `books.json` — all books with: id, title, author, narrator, series, format, duration, file_path, file_size, version_group_id, is_primary_version, work_id, itunes_persistent_id, year, publisher, library_state, marked_for_deletion
- `authors.json` — id, name, book_count, file_count, aliases
- `series.json` — id, name, book_count, file_count

**Category-specific:**
- **Error Analysis:** `logs.json` (last 24h system_activity_log), `operations.json` (last 100 operations with status/errors)
- **Deduplication:** `itunes_albums.json` (parsed from iTunes XML — album, artist, track count, total duration, PIDs), `version_groups.json` (all version group IDs with their member book IDs)
- **Metadata Quality:** `tag_mismatches.json` (books where file tags differ from DB values — requires sampling, not full library ffprobe), `missing_fields.json` (books missing title/author/series)
- **General:** all of the above

**Always included:**
- `batch.jsonl` — pre-built OpenAI batch API file with category-appropriate prompts and chunked data

### Step 3: Two Actions

**Download ZIP** — standard browser download. User can attach to a GitHub issue for developer troubleshooting.

**Submit to AI** — uploads `batch.jsonl` to OpenAI batch API using the configured `openai_api_key`. Creates an Operation record (type="diagnostics_ai") to track progress. Polls for completion via the existing operation queue's checkpoint system.

### Step 4: AI Results View

When the batch completes, results are parsed and stored in the operation's `result_data`.

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

Response: ZIP file download (Content-Type: application/zip)

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

- The ZIP generation runs as a background operation (could take 30+ seconds for large libraries)
- iTunes XML parsing reuses `internal/itunes/parser.go` (already has `ParseLibrary`)
- Batch polling reuses `internal/ai/openai_batch.go` (already has batch upload/poll/download)
- Suggestion application reuses existing merge (`mergeBookDuplicatesAsVersions`), delete, and update endpoints
- Config secrets (API keys, passwords) are redacted in the export with `***REDACTED***`
- The `tag_mismatches.json` for metadata quality samples up to 100 books via ffprobe (not the full library — too slow)

## Frontend Components

- `web/src/pages/Diagnostics.tsx` — new page
  - Category selector (radio buttons or card grid)
  - Description text area
  - Download ZIP / Submit to AI buttons
  - Results panel (shows after AI completion)
    - Suggestion list with approve/reject
    - View Raw toggle
    - Apply Selected button with confirmation dialog

## Testing

- **Unit:** ZIP generation includes expected files per category
- **Unit:** JSONL prompt structure is valid for OpenAI batch API
- **Integration:** Export endpoint returns valid ZIP with parseable JSON files
- **E2E:** Category selection → Download ZIP flow, Submit to AI → mock results → approve/apply flow
