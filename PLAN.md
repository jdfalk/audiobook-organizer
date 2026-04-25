# Cache Metrics — Plan

Branch: `feat/cache-metrics` · Worktree: `audiobook-organizer-cache-metrics`

## Goal
Add per-cache observability (hits, misses, sets, invalidations, evictions, size, latency) for every cache in the system. Expose via existing Prometheus `/metrics`, a JSON endpoint for the Diagnostics UI, a per-key debug endpoint, and a persistent history table for restart-surviving trends.

OTel deferred to a separate future PR.

## Files to change

### Wave 0 — foundation (coordinator, sequential)
- `internal/metrics/metrics.go` — add cache counter/gauge/histogram primitives + helper funcs
- `internal/cache/cache.go` — add `name` field, `New(name, ttl)`, instrument Get/Set/Invalidate(All)
- `internal/cache/cache_test.go` — update tests for new signature, add metric-emission test
- All 6 callsites of `cache.New[...]` (mechanical name addition):
  - `internal/server/server.go:838-840` (dashboardCache, dedupCache, listCache)
  - `internal/server/audiobook_service.go:80-81` (bookCache, listCache)
  - `internal/ai/openai_parser.go:66` (responseCache)

### Wave 1 — parallel children
- **T1 (Sonnet)**: `internal/database/metadata_fetch_cache.go` — wire counters at read/write boundaries
- **T2 (Sonnet)**: `internal/database/embedding_store.go` — wire counters at cache lookup paths
- **T3 (Haiku)**: `web/src/pages/Diagnostics.tsx` (or equivalent) — add Cache Stats panel
- **T4 (Haiku)**: `internal/server/system_handlers.go` — add `GET /api/v1/cache/stats` JSON handler + `GET /api/v1/cache/stats/keys?cache=X` (admin-gated, key names only)

### Wave 2 — persistence (coordinator, sequential)
- New migration N+1: `cache_stats_history` table (cache_name, ts, hits, misses, sets, invalidations, evictions, size)
- `internal/server/server.go` — background snapshotter goroutine (every 5 min)
- `internal/server/system_handlers.go` — `GET /api/v1/cache/stats/history?cache=X&since=Y`
- Tests + CHANGELOG.md + TODO.md

### Wave 3 — LRU eviction (coordinator, sequential)
- `internal/cache/cache.go` — add optional `maxEntries int` (0 = unbounded, current behavior). When set, maintain an access-ordered list (`container/list` doubly-linked list + map index) and evict the LRU entry on `Set` once `len(items) > maxEntries`. Each eviction calls `metrics.RecordCacheEviction(name)`.
- New constructor: `NewWithLimit[T](name string, ttl time.Duration, maxEntries int) *Cache[T]` — keeps the existing `New` signature stable for callers that want unbounded.
- Also evict expired entries lazily on `Get` (currently they linger): on a miss-due-to-expiry path, delete the stale entry and record an eviction with `reason="expired"`. This gives `cache_evictions_total{cache,reason}` real signal without forcing every cache to opt into LRU.
- Pick sensible default limits per cache once we have a wave or two of production stats showing actual sizes — don't guess up front.
- Tests: cover capacity-eviction ordering, expired-on-Get eviction, and that LRU access updates recency.

## Ordered steps
1. **Wave 0a**: extend `internal/metrics/metrics.go` (new counter/gauge/histogram + helpers + register).
2. **Wave 0b**: add `name` to `Cache[T]`, instrument operations, update 6 callsites in one commit. `make test` green.
3. **Wave 1**: dispatch T1–T4 in parallel as subagents (worktree-isolated). Each opens its own PR or returns a patch the coordinator commits to this branch.
4. **Wave 2**: write migration, snapshotter, history endpoint. Add tests.
5. Run full `make ci`, update CHANGELOG/TODO, open PR.

## Test strategy
- Unit: `cache_test.go` asserts that Get hit/miss/expired paths emit the right counter.
- Integration: hit `/api/v1/cache/stats` after seeding a cache, verify shape.
- Metrics: scrape `/metrics`, assert `cache_hits_total{cache="dashboard"}` exists.
- DB: history table populated after a forced snapshot tick.

## Rollback
Single branch, single PR (or PR-per-wave). Revert merge commit. No schema destructive changes — the new table is additive; cache name parameter is required but defaults are easy to add if needed.

## Cardinality safety
Labels are bounded: `{cache}` ∈ {dashboard, dedup, list, book, ai_response, metadata_fetch, embedding, ...} — single digits. `{reason}` ∈ {not_found, expired}. No per-key labels — that's what the per-key debug endpoint is for, and it returns *names only*.

## Out of scope
- OTel migration (separate future PR)
- Per-key value inspection (returns names only)
- Tuning per-cache `maxEntries` defaults (Wave 3 ships the mechanism unbounded; tuning is a follow-up once we have history data)
