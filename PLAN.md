# Memdb filter pushdown — make filtered queries O(matches), not O(corpus)

## Goal

Stop materializing all 392K books for any query with a "heavy" filter
(`library_state`, `review`, `has_cover`, `has_written`, `has_organized`,
`needs_writeback`, `format`, `language`, etc.). Push each predicate into a
memdb index iterator so a filtered page query touches at most
`O(predicate-positives + limit)` rows. Counts use the same indexes without
materializing structs. After this lands, the warmer can be re-enabled in a
slim form without OOM risk, and the cache value can shrink from full
enriched gin.H to ID lists.

Memory ceiling target: process RSS stays under **3GB** through full
startup + memdb warm + 50 concurrent UI filter queries (vs the 67GB OOM
we just hit). And a slow background "trickle warmer" backfills the
remaining ~170 common filter combos over 20–30 min — one query per
tick, GC + RSS-guard between each, so total cache coverage is reached
without ever spiking memory.

## Affected files

- `internal/database/memdb_schema.go` — add 7-8 new derived bool/string
  indexes on `books` table (library_state, review_status, has_cover,
  has_written, has_organized, needs_writeback, format, language).
- `internal/database/memdb_indexers.go` — new derived indexers (bool
  computed from nullable timestamp / string presence, with appropriate
  byte encoding for memdb radix tree).
- `internal/database/memdb_reads.go` — new methods:
  - `ListBookIDsByPredicate(filters []PredicateFilter) ([]string, error)`
  - `CountBooksByPredicate(filters []PredicateFilter) (int, error)`
  - `ListBookIDsTitleSorted(asc bool, filters []PredicateFilter, limit, offset int) ([]string, error)`
- `internal/database/pebble_store.go` — delegate `CountBooksByPredicate`
  and the new top-K title path to memdb when published.
- `internal/audiobooks/service.go` — rewrite the `hasHeavyPostFilters`
  branch (line ~749). Translate `FieldFilter` slice to `PredicateFilter`,
  call the new top-K method, then enrich only the returned IDs. Same for
  `CountAudiobooksFiltered`.
- `internal/server/library_list_warmer.go` — re-enable in slim mode:
  title-asc-primary first 2 pages only, sequential, `runtime.GC()` between
  calls, hard RSS cap check (skip remaining warm-ups if RSS > 2GB).

## Steps (each step lands as its own commit, reviewable independently)

1. **Indexers**: derived bool/string indexers for the 8 fields, with
   unit tests for byte encoding (nil timestamps, empty strings, computed
   `needs_writeback`).
2. **Schema**: wire indexers into `memdb_schema.go` books table. Verify
   memdb warmup still publishes (no schema panic).
3. **Predicate type + index walker**: introduce `PredicateFilter` (field
   + value + negated) in `database` package. Implement
   `ListBookIDsByPredicate` — picks the first indexed predicate, walks
   it, in-memory tests the rest against pointers (not copies). Returns
   ID slice only.
4. **Top-K title-sorted iterator**: `ListBookIDsTitleSorted` — walks
   title index in order, tests each predicate against pointer, skips
   `offset` count + collects `limit` IDs. Returns IDs only.
5. **CountBooksByPredicate**: same iteration, just increments counter.
6. **Service rewrite**: replace the heavy-filter branch with a call to
   the new top-K method; batch-load only the returned 20 IDs via
   `GetBookByID`. Same for `CountAudiobooksFiltered`.
7. **Eager slim warmer (the small one that runs immediately)**:
   title-asc-primary first 2 pages only, sequential, `runtime.GC()` +
   `debug.FreeOSMemory()` between calls, `readMemStatsMB() > 2048`
   guard that skips remaining. Total: ~2 queries, <30s. Covers the
   "user clicks All Books right after restart" case.
8. **Trickle warmer (the slow one that fills in the rest over 30 min)**:
   after the eager warmer drains, a background ticker pops one query
   per tick from a prioritized backlog (~150-170 entries — same shape
   as today's 177 minus the eager 2), runs it via the new top-K path,
   sets cache, then `runtime.GC()` + `debug.FreeOSMemory()`. Design:
   - Interval: 10–15s per query (operator-tunable env var
     `LIST_WARMER_TRICKLE_INTERVAL_MS`, default 10s → ~30 min total).
   - One in flight at a time. Never overlapping with the eager warmer
     or with itself.
   - Pre-tick RSS guard: if `readMemStatsMB() > 2048`, skip this tick
     and back off (double interval up to 60s, reset on success).
   - Pre-tick cancel: if the entry is already cached (e.g. a user
     visited that filter before the trickle reached it), skip.
   - Backlog priority order: highest-value first —
     `-review:matched` p1-3, `library_state:imported` p1, then the
     compound triage queries, then the rest. So even if the trickle
     is interrupted by a deploy after 5 min, the most-useful entries
     are already warm.
   - Logs each pop: `"trickle warmer tick name=... cached=N/M rss_mb=X"`
     so we can observe progress.
   - On 24h listCache TTL: trickle restarts hourly to keep the cache
     fresh (re-runs only entries whose cache expired or were evicted).
9. **(Optional, P2) Cache value shrink**: change `listCache` value from
   `gin.H` to `[]string` (book IDs). On hit, batch-enrich 20 books and
   build response. Cuts per-entry memory ~50× but adds small cost per
   hit. Defer if Phase 1 alone is enough.

## Test strategy

- `go test ./internal/database/... -run TestMemdbPredicate` — unit
  tests for each indexer (byte encoding) and predicate combo, top-K
  with offset.
- `go test ./internal/audiobooks/... -run TestGetAudiobooksFiltered` —
  filtered queries return identical results to the old full-scan path
  (table-driven against a 1000-book fixture).
- Manual prod smoke after each phase:
  - `time curl ...?library_state=imported&sort_by=title` → <100ms
  - `time curl ...?filters=[{"field":"review","value":"matched","negated":true}]` → <100ms
  - `ps -o rss` during a 10-query burst: stays <1.5GB.
- Re-enable warmer (step 7): watch `journalctl` for
  `library list warm-up complete` with RSS still under 2GB.

## Rollback

- Each step is its own commit. If any step regresses prod, revert that
  commit (schema/indexer additions are additive — no data migration).
- If the service rewrite (step 6) misbehaves, revert it and the heavy
  filter branch returns to the existing full-scan path (slow but
  correct). The disabled warmer (PR #1147) stays disabled.
- Slim warmer (step 7) has its own kill switch (`return` early); flipping
  takes us back to today's state.

## Out of scope

- Frontend filter URL format does not change.
- `listCache` HTTP-level cache key contract does not change.
- `is_primary_version` pushdown is already correct; not touched.
- Search (`?search=...`) still goes through Bleve — unchanged.
- Pebble-only fallback (when memdb hasn't published yet) stays on the
  old full-scan code — slow during 2-3min cold start, not steady-state.
