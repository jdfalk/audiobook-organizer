<!-- file: docs/superpowers/specs/2026-05-04-dedup-100pct-false-positives.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4784084c-1302-4e1b-921f-d2429354a6d8 -->
<!-- last-edited: 2026-05-04 -->

# Dedup 100% false-positives — series-aware embeddings + chromem sync

**Status:** Shipped (PR #695, merged 2026-05-04)
**Related code:** `internal/dedup/engine.go`, `internal/ai/embedding_client.go`, `internal/server/server.go`
**Related specs:**

- [2026-04-09 Embedding dedup design](2026-04-09-embedding-dedup-design.md)
- [2026-04-11 chromem-go embedding store](2026-04-11-chromem-go-embedding-store.md)

## Problem

Production ran with **33,640 pending embeddings**. Once dedup processed
them, the engine reported **100% similarity matches across different
volumes of the same series**: e.g. all six books of one series all
clustered as a single exact match, two different volumes flagged as
identical, etc.

## Root causes

Three independent bugs all contributed:

1. **Embedding text omitted series info.** `BuildEmbeddingText("book", title, author, narrator)` produced
   `"<title> by <author> narrated by <narrator>"`. Two volumes of the
   same series with the same author/narrator and minor title variation
   (e.g. "Foo: Volume 3" vs "Foo: Volume 4") embedded to nearly
   identical vectors → cosine similarity ≈ 1.0.

2. **chromem-go ANN index was never written from the engine.** `EmbedBook`
   and `EmbedAuthor` only called `embedStore.Upsert` (SQLite). Yet
   `findSimilarBooks` queries `chromemStore.FindSimilar` whenever
   `chromemStore != nil`. So the ANN backend was either empty or
   contained stale data from a previous code path; either way, results
   were wrong.

3. **Existing 33k SQLite embeddings stay invisible to ANN search.** Even
   after fixing #1 and #2, books that were already embedded would not
   appear in chromem until each one was re-embedded by hand. Production
   had no path to recover.

## Fix

### 1. Series-aware embedding text — `BuildBookEmbeddingText`

New helper in `internal/ai/embedding_client.go`:

```go
func BuildBookEmbeddingText(title, author, narrator, seriesName, seriesSequence string) string
```

Appends a `(SeriesName #N)` suffix to the base text:

| series | seq | output |
|--------|-----|--------|
| both | both | `"Words of Radiance by Brandon Sanderson narrated by Michael Kramer (Stormlight Archive #2)"` |
| series only | — | `"... (Stormlight Archive)"` |
| seq only | — | `"... (#2)"` |
| neither | — | base text (unchanged) |

`BuildEmbeddingText("book", ...)` is left as-is (back-compat) but its
docstring now points new callers at `BuildBookEmbeddingText`.

### 2. Mirror engine writes/deletes to chromem

`Engine.EmbedBook` (in `internal/dedup/engine.go`) now:

- Loads the series via `bookStore.GetSeriesByID(*book.SeriesID)`
- Builds text with `BuildBookEmbeddingText`
- On both **cache-hit and fresh-embed paths**, calls
  `mirrorBookToChromem(ctx, book, vec)`
- Skip-paths (non-primary version, empty title) call
  `deleteBookFromChromem(ctx, bookID)` so chromem never carries stale
  entries

`Engine.EmbedAuthor` symmetrically calls `mirrorAuthorToChromem` on
both paths.

New helpers (engine.go):

- `mirrorBookToChromem(ctx, book, vec)` — Upserts with metadata
  `is_primary_version`, `series_id`, `series_sequence` (all string→string
  per chromem's metadata constraint). Best-effort: log + drop on error.
- `mirrorAuthorToChromem(ctx, authorID, vec)` — same shape, no metadata.
- `deleteBookFromChromem(ctx, bookID)` — chromem's `Delete` is a no-op
  for missing IDs, so this is always safe to call.

SQLite `embedStore.Upsert` remains the source of truth; chromem is
treated as a derived ANN index.

### 3. Startup hydration — `Engine.HydrateChromem`

```go
func (de *Engine) HydrateChromem(ctx context.Context) (booksHydrated, authorsHydrated int, err error)
```

Walks `embedStore.ListByType("book")` and `("author")`, copies any
non-empty vectors into chromem with full metadata (skipping books that
no longer exist or are non-primary version-group members).

Wired in `internal/server/server.go` immediately after
`SetChromemStore`, in a background goroutine with a 30-minute timeout.
Logs:

```
[INFO] chromem hydrate complete: books=N authors=M
```

Errors are logged with partial counts; the engine works (slowly) before
hydration completes because `mirrorBookToChromem` populates entries on
demand whenever `EmbedBook` runs.

### Bonus: `EmbedOne` cache-passthrough docstring

Multiple reviewers had flagged `EmbedOne` as "missing cache". It calls
`EmbedBatch([]string{text})` which already does
`GetCachedEmbedding`/`PutCachedEmbedding` via the `EmbeddingCache`
interface (wired in `server.go` via `WithCache(embeddingStore)`). Added
an explicit doc comment so the question stops coming up.

## Tests

- `TestBuildBookEmbeddingText` — 4 sub-tests covering all
  series/sequence combinations.
- Existing tests (`TestBuildEmbeddingText_*`) unchanged — `BuildEmbeddingText`
  signature is back-compat.
- `go test ./internal/dedup/ ./internal/ai/ ./internal/database/` — all
  green.

## Followups (not in PR #695)

- **Wire embedding refresh into metadata-apply path** — when title /
  author / narrator / series changes via `UpdateBook` or
  `ApplyMetadataCandidate` (`metadata_handlers.go:286`), enqueue an
  `EmbedBook` call so the vector reflects current metadata. Currently
  the embedding only refreshes on a full re-scan or explicit re-embed.
- **Update `embedding_scorer.go` callers** to use `BuildBookEmbeddingText`
  if the metadata `Candidate` carries series fields (need to check struct
  shape).
- **Backfill existing matches** — once hydration runs in prod, the
  pending-merge queue may need a re-evaluation pass to clear out 100%
  matches that are actually different volumes. An ops script that walks
  open dedup batches and re-runs `findSimilarBooks` would do it.

## References

- PR #695: https://github.com/jdfalk/audiobook-organizer/pull/695
- Earlier dedup engine work: `docs/superpowers/specs/2026-04-09-embedding-dedup-design.md`
- chromem-go store impl: `docs/superpowers/specs/2026-04-11-chromem-go-embedding-store.md`
