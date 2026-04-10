# Embedding-Based Metadata Candidate Scoring Design

## Goal

Wire the existing embedding infrastructure into the metadata candidate scoring pipeline so that search results from Google Books, Audible, Audnexus, and Open Library are ranked by semantic similarity against the book's stored vector, instead of by the current `significantWords` F1 token overlap. Add an opt-in LLM rerank tier for ambiguous top candidates, behind a per-search user toggle.

## Problem

`metadata_fetch_service.go` scores candidate search results with `scoreOneResult()`, which computes F1 on significant-word sets between the search title and each result title. This has known failure modes:

- Variant word orders ("The Way of Kings" vs "Kings, The Way of") get low F1 despite being the same book.
- Subtitle and edition differences ("Dune" vs "Dune: Deluxe Edition") split tokens across result sets.
- Multi-language editions and transliterations fall off the F1 scale entirely.
- The signal is weak enough that the existing code compensates with aggressive author/narrator/series/compilation bonuses and penalties, which are carrying most of the ranking weight.

Meanwhile, every book in the library already has a `text-embedding-3-large` vector stored in `embeddings.db`, computed during the PR #203 backfill. Cosine similarity against those vectors is a dramatically stronger title-identity signal than F1, as the Layer 2 dedup system has demonstrated (1,919 book pairs flagged as likely duplicates from 24K vectors).

The `significantWords` path is also infrastructure-only: there is no abstraction boundary that would let us swap in a different scorer. Adding embeddings, or later a reranker, currently means editing the scoring function inline.

## Non-Goals

- Replacing the author / narrator / series / compilation / audiobook domain bonuses. Those are hard-won heuristics orthogonal to title similarity and stay exactly as they are.
- Changing what `metadata_fetch_service.go` searches for, how candidates are fetched from providers, or how results are presented in the UI.
- Adding a new external vendor. OpenAI-only. Cohere Rerank was considered and explicitly declined to avoid a second API key, second bill, and second vendor relationship. The scorer interface keeps the door open for adding it later.
- Scoring metadata candidates during background maintenance tasks. This is the interactive search path only.

## Architecture Overview

```
┌── metadata_fetch_service.go (search loop) ──────────────────┐
│                                                              │
│   1. Provider search returns []BookMetadata                  │
│   2. Pick base scorer (highest available, highest first):    │
│        EmbeddingScorer (default)                             │
│        F1 fallback (always available)                        │
│   3. Call scorer.Score(query, candidates) → []float64        │
│   4. If user requested LLM rerank AND top-K within ε:        │
│        call LLMScorer.Score on just the top-K                │
│   5. Apply existing domain bonuses (author, narrator, ...)   │
│   6. Apply existing penalties (compilation, length)          │
│   7. Sort, threshold, return MetadataCandidate[] to caller   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
                         ▲
                         │ interface
┌── ai.MetadataCandidateScorer ─────────────────────────────────┐
│                                                                │
│   Score(ctx, Query, []Candidate) ([]float64, error)            │
│   Name() string                                                │
│                                                                │
│   Implementations:                                             │
│     EmbeddingScorer  — existing OpenAI key, ~$0.00013/search   │
│     LLMScorer        — existing OpenAI key, ~$0.003/search     │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

## Scorer Interface

New file `internal/ai/metadata_scorer.go`:

```go
package ai

import "context"

// MetadataCandidateScorer ranks candidate search results by relevance to a
// query book. Implementations may batch internally and are responsible for
// their own API calls and caching. All implementations must:
//   - Return exactly one score per candidate, in the same order as input.
//   - Return scores clamped to [0, 1] where 1 is most relevant.
//   - Return (nil, err) on any failure so callers can fall back to the next
//     tier. Implementations should NEVER return partial results with a nil
//     error.
type MetadataCandidateScorer interface {
    Score(ctx context.Context, q Query, cands []Candidate) ([]float64, error)
    Name() string
}

// Query describes the book we're searching metadata for. BookID is an
// optional fast-path: if set and the scorer has a pre-computed vector for
// that book in the EmbeddingStore, it can skip re-embedding the query.
type Query struct {
    BookID   string
    Title    string
    Author   string
    Narrator string
}

// Candidate is one search result being scored. Fields mirror
// metadata.BookMetadata but are stripped to the identity fields that matter
// for scoring — extra fields like publisher and description don't improve
// ranking quality and just inflate token counts.
type Candidate struct {
    Title    string
    Author   string
    Narrator string
}
```

### EmbeddingScorer

`internal/ai/embedding_scorer.go`:

```go
type EmbeddingScorer struct {
    client *EmbeddingClient
    store  *database.EmbeddingStore // optional; enables BookID fast-path
}
```

Algorithm:

1. If `query.BookID != ""` and `store != nil`, try `store.Get("book", query.BookID)`. On hit, use the stored vector as `qVec` and skip query embedding.
2. Otherwise, build `queryText = BuildEmbeddingText("book", query.Title, query.Author, query.Narrator)` and embed via `client.EmbedOne(ctx, queryText)`.
3. Build candidate texts for each input: `BuildEmbeddingText("book", c.Title, c.Author, c.Narrator)`.
4. Batch-embed all candidates in one `client.EmbedBatch(ctx, texts)` call (OpenAI caps at 100 inputs per call; we batch across that boundary if necessary).
5. For each candidate vector, compute `cosine = database.CosineSimilarity(qVec, candVec)`, then `score = max(0.0, float64(cosine))`. Clamp to [0, 1]. Negative cosine means "actively opposite," which for text embeddings essentially never happens; we floor to 0 for safety.
6. Return `[]float64` in input order.

Error handling:
- Empty candidate list → return `nil, nil` (not an error, just nothing to score).
- Query embedding failure (no cache hit AND API fails) → return `nil, err`.
- Candidate batch failure → return `nil, err` (no partial results).

### LLMScorer

`internal/ai/llm_scorer.go`:

```go
type LLMScorer struct {
    parser *OpenAIParser
    // Reused model from existing parser config — currently gpt-5-mini.
}
```

Algorithm:

1. Build a structured prompt listing the query book and numbered candidates.
2. Call a new `(*OpenAIParser).ScoreMetadataCandidates(ctx, query, candidates)` that uses the same chat + JSON response format as `ReviewDedupPairs`. Response shape:
```json
{"scores": [{"index": 0, "score": 0.92, "reason": "Same book, exact title match"}, ...]}
```
3. Rehydrate into `[]float64` indexed by input order. Missing indices default to 0.0.
4. Batch size: LLMScorer is typically called on the top 5–10 candidates (after the base scorer picks them), so one batch per search is sufficient. If called on a larger set, chunk at 25 pairs per request like `ReviewDedupPairs`.

Error handling mirrors EmbeddingScorer — all-or-nothing.

### Caching

No persistent cache for candidate embeddings in this iteration. The math:

- A typical metadata search returns 10–20 candidates.
- At text-embedding-3-large: 20 × 50 tokens × $0.13 / 1M = $0.00013 per search.
- At typical usage (dozens of searches per day), monthly cost is pennies.
- Caching candidate embeddings would need keying by `TextHash(title+author+narrator)` and either a new entity_type in the embeddings table or a separate cache. Complexity isn't worth the savings yet.

We do reuse the **book's own** stored vector via the `BookID` fast-path, since that one is already cached from the PR #203 backfill.

## Scoring Pipeline Changes

### Current pipeline in `metadata_fetch_service.go`

```go
searchWords := significantWords(searchTitle)  // token set
for _, r := range allResults {
    score := scoreOneResult(r, searchWords)   // F1 + compilation + length + rich-metadata
    // ... apply author/narrator/series/audiobook bonuses ...
    candidates = append(candidates, MetadataCandidate{..., Score: score})
}
```

### New pipeline

```go
baseScores, baseScorer := mfs.scoreBaseCandidates(ctx, book, allResults)
// baseScores is aligned to allResults; baseScorer names the tier used (for logging).

for i, r := range allResults {
    score := baseScores[i]
    score = applyNonBaseAdjustments(score, r, searchWords) // compilation, length, rich-metadata
    // ... apply author/narrator/series/audiobook bonuses ... (unchanged)
    candidates = append(candidates, MetadataCandidate{..., Score: score, BaseScorer: baseScorer})
}

// Optional LLM rerank pass for ambiguous top candidates
if req.UseRerank && mfs.llmScorer != nil {
    candidates = mfs.rerankTopK(ctx, book, candidates)
}
```

### `scoreBaseCandidates` — base tier selection

Picks the highest-available base scorer, highest first. Falls through on nil scorer or scorer error:

1. `EmbeddingScorer` if `MetadataEmbeddingScoringEnabled` and `mfs.embeddingScorer != nil`
2. F1 fallback (inline, using existing `significantWords` logic)

Returns `([]float64, string)` — the scores and the name of the tier that produced them, for logging and UI display. If embedding tier fails, logs the error and falls back; search never fails because of scorer problems.

### `applyNonBaseAdjustments` — extracted from `scoreOneResult`

The existing `scoreOneResult` does three things: compute F1, apply compilation/length penalties, add rich-metadata bonus. The first one is now handled by the scorer; the other two are general-purpose post-processing that should run regardless of which base scorer produced the score.

Refactor: split `scoreOneResult` into `computeF1Base` (only used by the F1 fallback path) and `applyNonBaseAdjustments(score, r)` (used by all paths). No behavior change for the F1 path — the two halves are equivalent to the current function.

### `rerankTopK` — optional LLM rerank pass

```go
func (mfs *MetadataFetchService) rerankTopK(ctx context.Context, book *database.Book, candidates []MetadataCandidate) []MetadataCandidate
```

Algorithm:

1. Sort candidates by current score, descending.
2. Identify the "ambiguous top": `candidates[0]`, plus any subsequent candidate whose score is within `MetadataLLMRerankEpsilon` of `candidates[0].Score`.
3. If the ambiguous top has fewer than 2 candidates, skip rerank (nothing to resolve).
4. If it has more than `MetadataLLMRerankTopK` candidates, cap at K.
5. Call `llmScorer.Score(ctx, query, topK)` where `topK` is projected into `[]Candidate`.
6. On success, replace the `Score` field of each top-K candidate with `llmScore` directly, bypassing the author/narrator/series/audiobook bonus multiplication for those specific candidates. Rationale: the LLM prompt already sees title + author + narrator and is explicitly told to judge overall match quality. Multiplying its output by the same bonus signals would double-count the author/narrator evidence and distort the comparison against non-reranked candidates. The compilation penalty still applies since it's about the candidate's own title shape, not the query-candidate relationship.
7. On failure, log and return candidates unchanged (pure no-op, base-tier scores preserved).
8. Resort the full list by final `Score`, descending.

Default thresholds (all config-driven):
- `MetadataLLMRerankEpsilon = 0.01` — ultra-conservative starting point. Rerank almost never triggers. Tune up 0.01 at a time as we learn.
- `MetadataLLMRerankTopK = 5` — never send more than 5 candidates to the LLM per search, even if more are within ε.

### Thresholds and filter bands

The current F1 path has two filter bands:
- Main loop: `score <= 0` → drop
- `bestTitleMatch`: `score < 0.35` → drop

Cosine similarity produces a different distribution. Typical same-book matches land at 0.85+, unrelated matches at 0.5–0.7. The 0.35 threshold is meaningless on a cosine scale. Two new config keys:

- `MetadataEmbeddingMinScore = 0.50` — candidates below this are filtered in the main loop. Rough equivalent of "score > 0" for F1.
- `MetadataEmbeddingBestMatchMin = 0.70` — used by `bestTitleMatch` when the embedding scorer is the base tier.

These only apply when the base tier is embedding; the F1 fallback keeps its existing 0.35 threshold unchanged.

## Config Keys

Added to `internal/config/config.go`:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `MetadataEmbeddingScoringEnabled` | bool | `true` | Use embedding cosine as the primary metadata candidate score. Falls back to F1 when false or when embedding tier fails. |
| `MetadataEmbeddingMinScore` | float64 | `0.50` | Minimum embedding cosine for a candidate to be kept in the main search loop. |
| `MetadataEmbeddingBestMatchMin` | float64 | `0.70` | Minimum embedding cosine for `bestTitleMatch` to return a single best result. |
| `MetadataLLMScoringEnabled` | bool | `false` | Server-wide kill switch for the LLM rerank tier. When false, the per-search `use_rerank` param is ignored. |
| `MetadataLLMRerankEpsilon` | float64 | `0.01` | LLM rerank fires only on candidates whose score is within this distance of the best. Ultra-conservative starting point. |
| `MetadataLLMRerankTopK` | int | `5` | Maximum number of candidates the LLM is asked to rerank per search. |

No new API keys. No new vendor relationships. Everything uses the existing `OpenAIAPIKey`.

## API Surface

**Modified endpoint:** `POST /api/v1/audiobooks/:id/fetch-metadata`

Request body gains one optional field:
```json
{
  "title": "...",
  "author": "...",
  "use_rerank": false
}
```

- `use_rerank` defaults to false.
- When true AND `MetadataLLMScoringEnabled` is true on the server AND `OpenAIAPIKey` is set, the LLM rerank pass runs after the base scorer.
- When the server config makes rerank unreachable (feature disabled or key missing), the flag is silently ignored — we log it but return normal results. No 4xx error for a UI-toggle glitch.

No new endpoints. No breaking changes.

## UI Changes

**BookDetail metadata search dialog** grows one new control:

- A `FormControlLabel` + `Switch` labeled **"AI rerank (higher quality, ~$0.003/search)"**.
- Default **OFF** (opt-in for cost visibility).
- Disabled with a tooltip "Enable 'Metadata LLM scoring' in Settings" if server config has it off.
- When the user flips it on, the next search sends `use_rerank: true` in the request body.
- Result candidates show a small badge on the base scorer name (`embedding`, `llm`, or `f1`) so the user can tell what scored each one, similar to the existing layer badges in the dedup UI.

**Settings page** gains one new row in the AI section:

- **"Metadata LLM scoring (opt-in rerank)"** toggle, wired to `MetadataLLMScoringEnabled`.
- Short help text: "Allows users to request higher-quality metadata search rerank using the OpenAI API. Adds ~$0.003 per search when enabled by the user."

No Cohere API key field. No second vendor anywhere in the UI.

## PR Split

This ships as two sequential PRs on the same branch family.

### PR 1 — Scorer interface + EmbeddingScorer

**Scope:** introduce the `MetadataCandidateScorer` interface, implement `EmbeddingScorer`, refactor `scoreOneResult` into base + non-base halves, wire the embedding scorer into `metadata_fetch_service.go`. Zero UI changes, zero new vendors, zero new API params.

**Why this is low risk:** the embedding scorer is a strict improvement over F1 for the title-similarity signal, and everything else in the scoring pipeline (author/narrator/series bonuses, compilation penalty, rich-metadata bonus, thresholds via new config keys) is either unchanged or has safe-fallback behavior. Any failure in the embedding tier falls through to the existing F1 path.

**Files:**
- NEW `internal/ai/metadata_scorer.go` — interface + types
- NEW `internal/ai/embedding_scorer.go` — EmbeddingScorer implementation
- NEW `internal/ai/embedding_scorer_test.go` — unit tests with a fake embedding client
- NEW `internal/ai/metadata_scorer_test.go` — interface contract tests
- MODIFY `internal/config/config.go` — add three new config keys (`MetadataEmbeddingScoringEnabled`, `MetadataEmbeddingMinScore`, `MetadataEmbeddingBestMatchMin`)
- MODIFY `internal/config/persistence.go` — load the new keys
- MODIFY `internal/server/metadata_fetch_service.go`:
  - Add `embeddingScorer ai.MetadataCandidateScorer` field + setter
  - Split `scoreOneResult` into `computeF1Base` + `applyNonBaseAdjustments`
  - New `scoreBaseCandidates` that picks the tier and returns aligned scores
  - Update the main search loop and `bestTitleMatchWithContext` to use it
- MODIFY `internal/server/server.go` — construct `EmbeddingScorer` during startup (when embedding store and client exist) and inject it into `metadataFetchService`

### PR 2 — LLMScorer + per-search toggle + UI

**Scope:** add the optional LLM rerank tier. Builds directly on PR 1.

**Files:**
- NEW `internal/ai/llm_scorer.go` — LLMScorer implementation
- NEW `internal/ai/llm_scorer_test.go`
- NEW `internal/ai/metadata_llm_review.go` — new method on OpenAIParser: `ScoreMetadataCandidates(ctx, query, cands)` using the same JSON pattern as ReviewDedupPairs
- MODIFY `internal/config/config.go` — three more keys (`MetadataLLMScoringEnabled`, `MetadataLLMRerankEpsilon`, `MetadataLLMRerankTopK`)
- MODIFY `internal/server/metadata_fetch_service.go`:
  - Add `llmScorer ai.MetadataCandidateScorer` field + setter
  - Add `rerankTopK` method
  - Main search loop reads `use_rerank` from the request and conditionally invokes rerank
- MODIFY `internal/server/server.go` — construct `LLMScorer` during startup
- MODIFY `internal/server/metadata_fetch_service.go` request types — add `UseRerank bool` to the fetch-metadata request struct
- MODIFY the fetch-metadata handler to pass the flag through
- MODIFY `web/src/services/api.ts` — `fetchMetadataCandidates` call gains an optional `use_rerank` param
- MODIFY `web/src/pages/BookDetail.tsx` — add the AI rerank toggle to the search dialog, wire it into the API call
- MODIFY `web/src/pages/Settings.tsx` — add the `MetadataLLMScoringEnabled` switch in the AI section
- MODIFY `internal/server/metadata_fetch_service.go` response types — add `BaseScorer string` to `MetadataCandidate` so the UI can badge each result with its scoring tier

## Testing Strategy

### Unit tests (both PRs)

- **Scorer interface contract tests** in `metadata_scorer_test.go`: given any implementation, `len(scores) == len(candidates)`, all scores in [0, 1], empty candidate list returns `nil, nil`.
- **EmbeddingScorer** tests use a fake client that returns deterministic vectors (e.g., one-hot per title), verify cosine math, verify BookID fast-path skips query embedding, verify fallback to on-the-fly embedding when BookID is empty or not in the store.
- **LLMScorer** tests use a stubbed OpenAIParser chat response with known JSON, verify score extraction and ordering, verify that missing indices default to 0.0.
- **`scoreBaseCandidates`** tests cover: happy path with embedding tier, embedding tier returns error → falls back to F1, embedding tier disabled → F1 directly.
- **`rerankTopK`** tests cover: epsilon filter (only the ambiguous top gets sent), topK cap, LLM failure leaves candidates untouched.

### Integration tests (PR 1 only)

Extend `internal/server/metadata_search_test.go` with cases where the scorer is injected:
- With a real EmbeddingStore + fake embedding client, verify that a result with cosine=0.9 ranks above a result with cosine=0.7 regardless of F1.
- With the scorer disabled via config, verify the F1 fallback path is used and old behavior is preserved.
- With an empty result list, the scorer is never called.

### Manual verification (both PRs)

Deploy to prod, run a metadata search on a handful of known-hard cases where the old F1 path ranked poorly (book with subtitle-only variant, book in a series with trailing number, book with narrator/author swap). Verify the embedding tier ranks the correct candidate first. For PR 2, flip the UI toggle and verify the rerank pass fires only when top candidates are within 0.01 of each other.

## Failure Modes

| Failure | Behavior |
|---------|----------|
| No embedding client configured (no API key) | `scoreBaseCandidates` skips embedding tier, uses F1 |
| Embedding scorer returns error | Logs, falls through to F1, search still works |
| Book has no stored vector (pre-backfill) | EmbeddingScorer embeds query on the fly, still returns a score |
| Candidate embedding batch fails | Returns `nil, err`, caller falls through to F1 |
| LLM scorer returns error during rerank | Logs, keeps base-tier scores, search still returns candidates |
| `use_rerank=true` but LLM feature disabled server-side | Silently ignored, logged, base-tier results returned |
| All candidate lists are empty | Scorer is never called, existing code paths handle zero results |

No failure mode should cause the metadata search to fail — the fallback chain terminates in the existing F1 path, which has no dependencies on any scorer.

## Cost & Performance

Single metadata search with embedding tier:
- 1 query embed call (skipped if BookID cached): ~50 tokens × $0.13/1M = $0.0000065
- 1 candidate batch embed (20 candidates): ~1000 tokens × $0.13/1M = $0.00013
- Cosine math locally: ~1ms
- **Total: ~$0.00014 per search, ~300ms added latency over F1 (dominated by one HTTP round-trip to OpenAI).**

Single metadata search with LLM rerank when it triggers:
- Base tier as above: $0.00014
- 1 LLM call on top 5 candidates: ~$0.003
- **Total when triggered: ~$0.003, adds ~2–5s latency.**

With `MetadataLLMRerankEpsilon = 0.01` default, rerank triggers on maybe 1–5% of searches. Monthly cost impact at 100 searches/day: ~$0.40 from embeddings + ~$0.45 from rare rerank = **under $1/month**.

## Out-of-Scope / Future Work

- **Cohere Rerank or Voyage Rerank integration.** The interface supports adding it later. Deferred because it introduces a second vendor and was explicitly declined during design.
- **Candidate embedding cache** keyed by text hash. Cheap optimization if search volume grows; not worth building now.
- **Author and series metadata candidate scoring.** Same infrastructure applies but the UX surface is different. Separate design.
- **Precomputed "query vectors" for common searches.** Could cache book vectors keyed by searchTitle when users run unusual queries. Premature.
- **Hybrid scoring that blends embedding + F1 + reranker.** Current design picks one base tier; a weighted blend is a future experiment if we see specific failure modes the single-tier approach misses.
