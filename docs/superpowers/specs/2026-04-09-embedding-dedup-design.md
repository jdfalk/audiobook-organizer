# Embedding-Based Deduplication Design

## Goal

Replace the expensive dual-LLM dedup pipeline with a 3-layer system: exact matching (free), embedding cosine similarity (cheap), and LLM review only for ambiguous cases. Handles both book-level and author-level duplicates. Embeddings computed on ingest and metadata change, stored in a SQLite sidecar, with idempotent backfill for existing data.

## Problem

The current dedup system sends the entire author list to OpenAI chat completions (gpt-5-mini / o4-mini) in two parallel modes (groups + full), then cross-validates. This is:
- Expensive: $5-20 per full scan
- Slow: minutes of LLM inference for string comparison
- Not incremental: can't cheaply check a single new book against the library
- Author-only: no book-level duplicate detection

Meanwhile, 90%+ of duplicates are obvious from exact signals (same file, same ISBN) or high string/semantic similarity that doesn't need an LLM.

## Architecture Overview

```
Book Created/Updated
        │
        ▼
┌─ Layer 1: Exact Match (free, instant) ──────────────┐
│  file hash + author + title identical → AUTO-MERGE   │
│  same ISBN/ASIN → flag candidate                     │
│  normalized Levenshtein < 3 → flag candidate         │
└──────────────────────────────────────────────────────┘
        │ (no auto-merge)
        ▼
┌─ Layer 2: Embedding Similarity (cheap, ~250ms) ─────┐
│  Embed "title by author narrated by narrator"        │
│  Cosine similarity vs all stored vectors             │
│  > 0.95 → high confidence candidate                  │
│  0.85-0.95 → likely candidate                        │
│  < 0.85 → skip                                       │
└──────────────────────────────────────────────────────┘
        │ (ambiguous zone: 0.80-0.92)
        ▼
┌─ Layer 3: LLM Review (expensive, batch only) ───────┐
│  Maintenance window / on-demand                      │
│  gpt-5-mini structured JSON                          │
│  {is_duplicate, confidence, reason}                  │
│  Only for candidates embeddings couldn't resolve     │
└──────────────────────────────────────────────────────┘
        │
        ▼
   dedup_candidates table → UI review → merge/dismiss
```

## Embedding Infrastructure

### Storage: embeddings.db (SQLite sidecar)

```sql
CREATE TABLE embeddings (
    id          TEXT PRIMARY KEY,   -- "{type}:{entity_id}" e.g. "book:01KN..." or "author:39079"
    entity_type TEXT NOT NULL,      -- "book" or "author"
    entity_id   TEXT NOT NULL,
    text_hash   TEXT NOT NULL,      -- SHA-256 of input text (staleness detection)
    vector      BLOB NOT NULL,      -- float32 array, 3072 dimensions
    model       TEXT NOT NULL,      -- "text-embedding-3-large"
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
CREATE INDEX idx_embeddings_type ON embeddings(entity_type);
CREATE INDEX idx_embeddings_entity ON embeddings(entity_id);
```

Stored as a new `EmbeddingStore` following the same pattern as `ActivityStore` — separate SQLite DB opened by the server on startup.

### Embedding Text Construction

- **Books**: `"{title} by {author} narrated by {narrator}"` — the three key identity fields. If narrator is empty, omit that clause.
- **Authors**: `"{author_name}"` — just the name.

The `text_hash` is SHA-256 of the constructed text. On metadata change, recompute the hash and compare — if unchanged, skip the API call.

### Embedding Client

Thin wrapper around OpenAI `/v1/embeddings` endpoint:
- Model: `text-embedding-3-large` (3072 dimensions)
- Batch up to 100 texts per API call (OpenAI max)
- Rate limiting: 3000 RPM / 1M TPM (OpenAI tier limits)
- Retry with exponential backoff (same pattern as existing `openai_parser.go`)
- Returns `[][]float32`

### Cosine Similarity

Computed in Go — brute-force scan of all vectors of the same entity type:

```go
func cosineSimilarity(a, b []float32) float32 {
    var dot, normA, normB float32
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    return dot / (sqrt(normA) * sqrt(normB))
}
```

At 15K vectors × 3072 dimensions, a single query takes ~50ms. Acceptable for on-ingest checks. Full all-vs-all scan (~12 minutes) runs as a background operation.

## Three-Layer Dedup Engine

### Layer 1: Exact Match

Runs synchronously on book/author create and update. No API calls.

**Auto-merge (no human needed):**
- Two book records with identical file hash AND identical normalized author AND identical normalized title → merge automatically, keep the older record as primary

**Flag as candidate (human review):**
- Same ISBN or ASIN on different book records
- Same author + normalized title with Levenshtein distance < 3
- For authors: normalized name equality after case folding, punctuation removal, initial expansion ("J.R.R. Tolkien" = "J. R. R. Tolkien" = "JRR Tolkien")

Normalization reuses existing functions from `author_dedup.go` (`NormalizeAuthorName`, `SplitCompositeAuthorName`).

### Layer 2: Embedding Similarity

Runs after Layer 1 if no auto-merge occurred. Makes one embedding API call for the new/changed entity, then scans stored vectors.

**Book similarity thresholds:**
- \> 0.95: high confidence duplicate → flag as candidate with `layer = "embedding"`, high priority
- 0.85 - 0.95: likely duplicate → flag as candidate, medium priority
- < 0.85: not a duplicate, skip

**Author similarity thresholds:**
- \> 0.92: high confidence → flag
- 0.80 - 0.92: likely → flag
- < 0.80: skip

Each candidate pair is stored in `dedup_candidates` with the cosine similarity score.

### Layer 3: LLM Review

Runs during maintenance window or on-demand via API. Only processes candidates in the ambiguous zone:
- Books: similarity 0.80 - 0.92
- Authors: similarity 0.75 - 0.85
- Also: Layer 1 flags that aren't auto-merge (same ISBN but different title, etc.)

Uses existing OpenAI chat completion (`gpt-5-mini`) with structured JSON:

```json
{
  "is_duplicate": true,
  "confidence": "high",
  "reason": "Same book, different subtitle format. 'The Way of Kings' and 'Stormlight Archive 1 - The Way of Kings' are the same novel."
}
```

Results stored in `dedup_candidates.llm_verdict` and `llm_reason`.

## Candidate Storage

```sql
CREATE TABLE dedup_candidates (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type   TEXT NOT NULL,                      -- "book" or "author"
    entity_a_id   TEXT NOT NULL,
    entity_b_id   TEXT NOT NULL,
    layer         TEXT NOT NULL,                      -- "exact", "embedding", "llm"
    similarity    REAL,                               -- cosine similarity or null
    llm_verdict   TEXT,                               -- "duplicate" / "not_duplicate" / null
    llm_reason    TEXT,
    status        TEXT NOT NULL DEFAULT 'pending',    -- "pending" / "merged" / "dismissed"
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    UNIQUE(entity_type, entity_a_id, entity_b_id)
);
CREATE INDEX idx_dedup_status ON dedup_candidates(status);
CREATE INDEX idx_dedup_type_status ON dedup_candidates(entity_type, status);
CREATE INDEX idx_dedup_entity_a ON dedup_candidates(entity_a_id);
CREATE INDEX idx_dedup_entity_b ON dedup_candidates(entity_b_id);
```

This table lives in `embeddings.db` alongside the vectors — keeps all dedup data in one sidecar.

## Pipeline Triggers

### On Ingest (book created)

1. Construct embedding text from title + author + narrator
2. Hash it → check if embedding exists with same hash (skip if so)
3. Call OpenAI embeddings API → store vector in `embeddings` table
4. Run Layer 1 exact checks against all books
5. Run Layer 2 cosine similarity scan against all book vectors
6. If auto-merge criteria met (hash + author + title identical) → merge automatically
7. Otherwise store candidates in `dedup_candidates`

### On Metadata Change (metadata apply, manual edit)

1. Recompute embedding text hash
2. If hash unchanged → skip (embedding is still valid)
3. If changed → re-embed via API, update vector
4. Remove stale candidates for this entity (old embedding produced them)
5. Re-run Layer 1 + Layer 2 with updated vector
6. Store new candidates

### User-Triggered Refresh

- API: `POST /api/v1/dedup/scan`
- Re-embeds all entities whose `text_hash` is stale (metadata changed since last embed)
- Runs full Layer 2 scan to find new candidates
- Returns operation ID for progress tracking

### Maintenance Window

- Run Layer 3 (LLM) on pending ambiguous candidates
- Clean up dismissed candidates older than 30 days
- Re-embed any stale vectors (belt and suspenders for missed change events)

### One-Time Backfill

On first startup after deployment:
- Check setting `embedding_backfill_done` — skip if already complete
- Embed all books + authors in batches of 100
- Rate-limited, runs as background goroutine
- Progress tracked: `embedding_backfill_progress` setting stores last processed offset
- After embedding, run full Layer 2 scan to populate initial candidates
- Set `embedding_backfill_done = true`
- Survives restarts: checks offset, skips already-embedded entities via `text_hash`

**Cost estimate:** ~15.5K entities × ~50 tokens = ~775K tokens × $0.13/1M = ~$0.10

## API Endpoints

```
POST   /api/v1/dedup/scan                    — trigger re-embed stale + full Layer 2 scan
POST   /api/v1/dedup/scan-llm                — trigger Layer 3 LLM on pending ambiguous candidates
GET    /api/v1/dedup/candidates               — list candidates (query: entity_type, layer, status, min_similarity)
POST   /api/v1/dedup/candidates/:id/merge     — merge the pair (calls existing book/author merge)
POST   /api/v1/dedup/candidates/:id/dismiss   — mark dismissed (won't resurface unless embedding changes)
GET    /api/v1/dedup/stats                    — counts by layer, status, entity_type
POST   /api/v1/dedup/refresh                  — user-triggered: re-embed all stale + re-scan
```

## UI Integration

### Existing Pages

- **Book Dedup page** (`BookDedup`): reads from `dedup_candidates WHERE entity_type = 'book'` instead of current scan results. Shows similarity percentage, layer badge, LLM reason if available.
- **Author Dedup page**: reads from `dedup_candidates WHERE entity_type = 'author'`. Same treatment.
- Both pages keep existing merge/dismiss UX — just backed by new data source.

### Display

- Candidates sorted by similarity descending (most likely dupes first)
- Layer badge: "exact" (red), "embedding" (blue), "llm" (purple)
- Similarity shown as percentage (e.g. "94.2%")
- LLM reason shown as tooltip or expandable text when available
- Status filter: pending / merged / dismissed

### Controls

- "Re-scan" button → `POST /api/v1/dedup/scan`
- "Run AI Review" button → `POST /api/v1/dedup/scan-llm` (only for ambiguous candidates)
- "Refresh Embeddings" button → `POST /api/v1/dedup/refresh`

## Relationship to Existing Code

### Keep

- `author_dedup.go` heuristic functions (`NormalizeAuthorName`, `SplitCompositeAuthorName`, `jaroWinklerSimilarity`) — used by Layer 1
- `merge_service.go` — called by the merge endpoint, unchanged
- Book merge logic in version management — unchanged

### Replace

- `ai_scan_pipeline.go` dual-scan approach (groups + full) → Layer 3 only, on the ambiguous subset
- `cross_validation.go` → removed (no longer running two parallel LLM scans)
- Current dedup scan results storage → `dedup_candidates` table

### Future Integration Point

The embedding infrastructure can power metadata candidate matching: embed a search result from Google Books/Audible, compare against the book's vector, get an instant similarity score. This replaces the current `significantWords` F1 scoring in `metadata_fetch_service.go`. Out of scope for this implementation but the infrastructure supports it.

## Files to Create/Modify

### New Files
- `internal/database/embedding_store.go` — EmbeddingStore (SQLite sidecar, CRUD, cosine similarity)
- `internal/database/embedding_store_test.go` — tests
- `internal/server/embedding_service.go` — EmbeddingService (orchestrates embed + dedup pipeline)
- `internal/server/embedding_client.go` — OpenAI embeddings API client
- `internal/server/dedup_engine.go` — 3-layer dedup logic (exact, embedding, LLM)
- `internal/server/dedup_handlers.go` — API endpoint handlers
- `internal/server/dedup_engine_test.go` — tests

### Modify
- `internal/server/server.go` — initialize EmbeddingStore/Service, register routes, wire triggers
- `internal/server/scheduler.go` — add dedup maintenance tasks (Layer 3, stale cleanup)
- `internal/config/config.go` — add config keys (thresholds, model name, enable/disable)
- `internal/server/metadata_fetch_service.go` — trigger re-embed on metadata apply
- `web/src/pages/BookDedup.tsx` — read from new candidates API
- `web/src/services/api.ts` — add dedup API functions

### Config Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `EmbeddingModel` | string | `text-embedding-3-large` | OpenAI embedding model |
| `EmbeddingEnabled` | bool | `true` | Enable/disable embedding pipeline |
| `DedupBookHighThreshold` | float | `0.95` | Book similarity: high confidence |
| `DedupBookLowThreshold` | float | `0.85` | Book similarity: minimum to flag |
| `DedupAuthorHighThreshold` | float | `0.92` | Author similarity: high confidence |
| `DedupAuthorLowThreshold` | float | `0.80` | Author similarity: minimum to flag |
| `DedupLLMBookRange` | [2]float | `[0.80, 0.92]` | Book range that triggers LLM review |
| `DedupLLMAuthorRange` | [2]float | `[0.75, 0.85]` | Author range that triggers LLM review |
| `DedupAutoMergeEnabled` | bool | `true` | Auto-merge on exact hash+author+title match |

## Cost & Performance

| Operation | Cost | Time |
|-----------|------|------|
| One-time backfill (15.5K entities) | ~$0.10 | ~30 seconds |
| Single book ingest check | ~$0.000007 | ~250ms |
| Full Layer 2 scan (all-vs-all) | $0 (local compute) | ~12 minutes |
| Layer 3 LLM (50-200 ambiguous) | ~$0.50-2.00 | ~2-5 minutes |
| Current full AI dedup scan | ~$5-20 | ~10-30 minutes |

**Storage:** ~186MB for 15.5K vectors at 3072 dimensions.
