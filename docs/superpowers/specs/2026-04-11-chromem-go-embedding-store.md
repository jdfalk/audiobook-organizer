<!-- file: docs/superpowers/specs/2026-04-11-chromem-go-embedding-store.md -->
<!-- version: 1.0.0 -->
<!-- guid: c1d2e3f4-a5b6-7890-1234-567890abcdef -->
<!-- last-edited: 2026-04-11 -->

# chromem-go Embedding Store Design

**Status:** Design spec — not yet implemented.
**Owner:** TBD.
**Parent task:** Backlog item 4.7 (per-workload DB evaluation).
**Depends on:** Nothing. Standalone replacement for the current SQLite-backed
embedding store.

## Problem statement

Today, every dedup similarity query loads the entire embedding set into
memory and computes cosine in Go.

### Current implementation

Storage: SQLite sidecar `embeddings.db` (`internal/database/embedding_store.go`).
Vectors are stored as length-prefixed little-endian float32 blobs in the
`vector` column of an `embeddings` table keyed by `(entity_type, entity_id)`.

Query path, `EmbeddingStore.FindSimilar`:

1. `ListByType(entityType)` — a full-table scan that allocates and decodes
   every vector in the store for that type into `[]Embedding`.
2. Linear iteration, computing `CosineSimilarity(query, e.Vector)` per row.
3. Filter by `minSimilarity`, sort by similarity desc, truncate to `maxResults`.

At current library size:
- ~12K primary books × 3072 dims × 4 bytes = **~150 MB allocated and copied
  out of SQLite on every single similarity query**.
- Linear cosine over 12K vectors is ~37M float multiplies — fast on a modern
  CPU (<100 ms) but not free.
- Every `findSimilarBooks` call during `FullScan` triggers this. A scan of
  24K primary books that calls `findSimilarBooks` per book is
  12K × 150 MB = **~1.8 TB of cumulative allocation** over the course of the
  scan. Go's GC can handle it but it's pressure we don't need.

The dedup throughput floor is set by this pattern, not by the cosine math.

### What we'd gain from chromem-go

chromem-go is a pure-Go, embedded vector database designed for exactly this
workload. Key properties relevant to us:

- **HNSW-based approximate nearest neighbor (ANN) index.** Query cost is
  sub-linear in collection size — roughly O(log n) for small dimensions,
  with recall tunable via `ef` and `M` parameters. For our 12K×3072 set this
  would mean querying the top-20 in a handful of vector comparisons instead
  of all 12K.
- **Documents with metadata + optional content.** Each stored item has
  arbitrary metadata (string-keyed maps) that can be used as a filter
  predicate at query time. We can shove book ID, text hash, model name,
  entity type, and primary-version status into the metadata and filter
  them server-side.
- **Persistence to a directory of files**, with optional in-memory mode for
  tests. No daemon, no network round-trip, pure Go (no CGO).
- **Chroma-compatible query semantics.** Familiar shape if anyone on the
  team has used Chroma.

## Non-goals

- Replacing the `dedup_candidates` table. chromem-go is a vector store, not
  a relational store. The dedup candidates are a relational / filter-heavy
  workload and stay in SQLite.
- Multi-user collaborative indexing. Single-writer, single-process, same as
  today.
- Re-embedding existing books. The text-hash cache stays authoritative; we
  just move where the vectors live, not how they're computed.
- Replacing OpenAI as the embedding provider. Still `text-embedding-3-large`.

## Architecture

### New package

`internal/database/chromem_embedding_store.go` — wraps a chromem-go
`Collection` and implements a subset of the current `EmbeddingStore`
interface plus the new native similarity API. The old
`internal/database/embedding_store.go` stays for the `DedupCandidate`
CRUD methods.

```
┌──────────────────────────────────────────────┐
│ dedup_engine.go (findSimilarBooks)           │
│                                              │
│  ┌─────────────────┐   ┌─────────────────┐   │
│  │ EmbeddingStore  │   │ CandidateStore  │   │
│  │ (chromem-go)    │   │ (SQLite)        │   │
│  │ ─ vectors       │   │ ─ dedup_cands   │   │
│  │ ─ metadata      │   │ ─ status        │   │
│  │ ─ FindSimilar   │   │ ─ LLM verdicts  │   │
│  └─────────────────┘   └─────────────────┘   │
└──────────────────────────────────────────────┘
```

### Collection layout

One chromem collection per entity type:

- `books` collection — one document per primary book embedding
- `authors` collection — one document per author embedding

Each document:

| Field      | Type               | Purpose                                  |
|------------|--------------------|------------------------------------------|
| `ID`       | string             | `entityID` (book ULID, author numeric-ID) |
| `Embedding`| `[]float32` (3072) | The vector itself                        |
| `Content`  | string             | The source text we embedded (for debug)  |
| `Metadata` | `map[string]string`| See below                                |

Metadata keys stored per document:

| Key                  | Example                      | Used for                             |
|----------------------|------------------------------|--------------------------------------|
| `text_hash`          | `7f3ab...`                   | Cache-invalidation — skip re-embed when hash matches |
| `model`              | `text-embedding-3-large`     | Catch model-version drift            |
| `is_primary_version` | `true`/`false`               | Filter to primary books only at query time |
| `version_group_id`   | `vg-01KN...`                 | Exclude candidates in same version group during scan |
| `author_id`          | `42`                         | Enables cross-reference to book's author |
| `series_id`          | `17`                         | Series-aware filtering               |
| `series_number`      | `4`                          | Skip different volumes of same series |
| `created_at`         | ISO8601                      | Age-based debugging                  |
| `updated_at`         | ISO8601                      | Last-write timestamp                 |

Note: chromem-go metadata values are string-typed. Bools become `"true"`/
`"false"`, ints become their decimal string form. We wrap access in typed
helpers so callers never see raw strings.

### Persistence

chromem-go persists collections to a directory (`chromem.PersistentDB`).
Path: `<embedding_store_dir>/chromem/` parallel to the existing
`embeddings.db`. Both stores coexist during migration so a rollback is
just deleting the chromem directory and flipping a config flag.

On startup, chromem-go loads the collection files into memory (the HNSW
graph is reconstructed at load time). For 12K × 3072-dim vectors with
`float32` quantization this is ~150 MB resident — the same as the peak
working-set of the old store but amortized across the process lifetime
instead of re-allocated per query.

### Interface

New interface `EmbeddingVectorStore` scoped to the vector-only operations:

```go
// EmbeddingVectorStore is the vector-side subset of the dedup store.
// It replaces FindSimilar / Upsert / Get / Delete / ListByType from
// the old EmbeddingStore. The DedupCandidate CRUD remains on the
// SQLite-backed CandidateStore.
type EmbeddingVectorStore interface {
    // Upsert stores or replaces an embedding identified by
    // (entityType, entityID). If text_hash metadata matches an
    // existing document, the upsert is a no-op and the function
    // returns EmbedStatusCached.
    Upsert(ctx context.Context, entityType, entityID string, vec []float32, meta map[string]string) (EmbedStatus, error)

    // Get returns a single embedding by (entityType, entityID).
    // Returns (nil, nil) when not found.
    Get(ctx context.Context, entityType, entityID string) (*EmbeddingDoc, error)

    // Delete removes an embedding. No-op if absent.
    Delete(ctx context.Context, entityType, entityID string) error

    // FindSimilar runs an ANN query against the collection for
    // entityType, returning the top-maxResults documents with
    // similarity >= minSimilarity. Filter is an optional metadata
    // predicate applied server-side.
    FindSimilar(ctx context.Context, entityType string, query []float32, minSimilarity float32, maxResults int, filter map[string]string) ([]SimilarityResult, error)

    // CountByType returns the total document count in a collection.
    CountByType(ctx context.Context, entityType string) (int, error)

    // Close releases resources. Must be called from the shutdown path
    // under bgWG so in-flight queries complete before disk flush.
    Close() error
}
```

`EmbeddingDoc` is a minimal wrapper over chromem's `Document`:

```go
type EmbeddingDoc struct {
    EntityID string
    Vector   []float32
    TextHash string
    Model    string
    Metadata map[string]string
}
```

### How callers change

Current `FindSimilar` call site in `dedup_engine.findSimilarBooks`:

```go
// OLD
results, err := de.embedStore.FindSimilar("book", queryVec, 0.80, 50)
for _, r := range results {
    // ... filter by primary-version, same-series, etc. in Go
}
```

With native metadata filters, the Go-side post-filtering moves into the
query itself:

```go
// NEW
filter := map[string]string{
    "is_primary_version": "true",
}
results, err := de.embedStore.FindSimilar(ctx, "book", queryVec, 0.80, 50, filter)
// Optionally post-filter version_group_id == queryBook.VersionGroupID to
// exclude same-group candidates. Can't express "!= X" in the metadata
// filter DSL cleanly so this stays in Go for now.
```

Post-query Go filters stay in place for anything chromem's metadata
predicate language can't express (series-number inequality, digit-diff
title guard, etc.).

## Migration plan

### Phase 1: side-by-side (weeks 1-2)

- Add `chromem-go` dependency.
- Implement `EmbeddingVectorStore` wrapping a chromem collection.
- Wire into `DedupEngine` behind a config flag
  `DedupVectorStoreBackend` (default `"sqlite"`, new value `"chromem"`).
- When set to `chromem`, both stores are opened. Writes dual-write to both.
  Reads go to chromem; a sanity check compares `FindSimilar` top-5 results
  against the SQLite path on a sample rate (1%) and logs any divergence
  above a tolerance.
- The dual-write period is the safety net. Any correctness issue surfaces
  as divergence logs before we commit to chromem-only.

### Phase 2: migration tool (week 3)

- Add a maintenance endpoint
  `POST /api/v1/maintenance/migrate-embeddings-to-chromem` that:
  - Walks the existing `embeddings.db` in batches.
  - Upserts each into chromem with full metadata.
  - Reports progress + verifies counts match at the end.
- Idempotent: can be re-run after interruptions. Uses a marker setting
  `chromem_migration_v1_done` similar to `embedding_backfill_v5_done`.

### Phase 3: cut over (week 4)

- Change default of `DedupVectorStoreBackend` to `"chromem"`.
- Keep SQLite path compiled in and wired up for one more release as
  rollback insurance. Rollback is: flip the config flag back to
  `"sqlite"`, restart. No data loss — the old store never stopped being
  written to.

### Phase 4: cleanup (month 2)

- Remove the SQLite vector path, drop the `embeddings` table from the
  embedding DB (leaving `dedup_candidates` alone), remove the dual-write
  and divergence-check code.

## Benchmarks to run before committing

All against a snapshot of current production data (~12K primary books,
3072 dims). Three workloads:

1. **Cold query latency**: time a single `FindSimilar` call against each
   store after a process restart. Old store: load + cosine scan. New:
   load HNSW + query. Expect chromem to win by 2-10×.
2. **Hot query latency**: 1000 `FindSimilar` calls in sequence,
   measure p50/p95/p99. Old store should be limited by allocation +
   cosine. New should be near-constant.
3. **FullScan throughput**: wall-clock time to run a complete
   `DedupEngine.FullScan` end-to-end against each store. This is the
   number that matters — it's the user-visible dedup scan time.

Acceptance: chromem must beat SQLite on all three by at least 2× on the
FullScan wall-clock, or the migration is not worth the disruption.

## Risks

### chromem-go's HNSW parameters

Defaults may over-approximate for our similarity thresholds. If recall
drops below the SQLite baseline (exact cosine) by more than a few percent
at the thresholds we use (`0.85` for embedding layer, `0.80`-`0.92` for
LLM-bucket), we'll see false-negative candidates — real duplicates that
slip past the scan.

**Mitigation:** The Phase 1 divergence check is explicitly designed to
catch this. Tune `ef_construction` and `ef_search` until divergence is
below 1% before cutover.

### Index rebuild cost on startup

HNSW graph loads from disk on `chromem.PersistentDB` open. At 12K docs
this is fast (sub-second); at 100K+ it could become a startup-time
concern. For our projected library growth this is probably fine.

**Mitigation:** benchmark open time at 100K docs before committing. If
it's a problem, we can lazy-load per-collection on first query.

### Format stability

chromem-go is a young project. Its on-disk format may break between
versions.

**Mitigation:** Pin to a specific version tag. Treat chromem's directory
as ephemeral — on upgrade, run the migration tool from SQLite to rebuild
chromem. The `embeddings.db` vector table remains the source of truth for
the first release cycle so we can rebuild chromem from it at any time.

## Out of scope (explicitly)

- Replacing OpenAI embedding calls or switching models.
- Storing dedup candidates in chromem.
- Storing book metadata (title, author, etc.) in chromem as vector-searchable
  documents. That's a different project (see the bleve spec in this same
  docs folder).
- Cross-process access. chromem-go is in-process only. If we ever need
  multi-process vector search we'd move to a standalone vector DB like
  Qdrant and accept the operational cost.

## Open questions

1. **HNSW vs. brute-force at our scale?** At 12K docs, brute-force with
   SIMD is competitive with HNSW and has exact-cosine recall by definition.
   chromem-go uses HNSW unconditionally — if we want brute-force as a
   fallback we'd need to run benchmarks that prove HNSW is actually
   winning, not just assume it.
2. **Single collection vs. per-entity-type collections?** Currently
   proposing `books` and `authors` as separate collections. Alternative:
   one `dedup` collection with `entity_type` in metadata. The separate-
   collection approach is cleaner but means two HNSW graphs in memory.
   Measure.
3. **Metadata filter expressiveness.** chromem's filter language is
   simple equality — can't express `version_group_id != X`. We'd keep
   some post-filtering in Go. Is that acceptable or should we push to
   upstream for predicate support?
4. **Backup/restore.** chromem's directory isn't a single file like
   SQLite. How do we handle backups? Probably tar the directory. Document
   as part of the maintenance page.

## Next step

1. Review this spec.
2. If accepted, create a Plan doc at
   `docs/superpowers/plans/2026-04-XX-chromem-embedding-store.md` with
   bite-sized tasks.
3. Run the benchmarks first. If chromem doesn't beat SQLite 2× on the
   FullScan workload, abandon and document why.
