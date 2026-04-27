<!-- file: docs/superpowers/specs/2026-04-27-recent-ships-backfill.md -->
<!-- version: 1.0.0 -->
<!-- guid: 791bc33e-6 82d-b7db-5849-bcde9ab34234 -->

# Recent Ships Backfill — April 26–27, 2026

**Audience:** human reviewer
**Why this exists:** Several PRs shipped in the April 26–27 window without an accompanying design spec — the work was responsive (bug fixes, performance hot-fixes) and the rationale lived in commit messages and CHANGELOG entries. This doc consolidates that rationale so future readers (and future me) have the design context in one place rather than spread across 15 commit footers.

This is **not** a forward-looking spec — there's nothing to implement. Use it as a reference when touching any of these areas.

## Table of contents

1. [iTunes path repair operation](#itunes-path-repair) (PRs #467–#471)
2. [Metadata review pagination + refresh cycle fix](#metadata-review) (PRs #466, #473, #481)
3. [Config persistence JSON round-trip](#config-persistence) (PR #472)
4. [Activity batcher (15s / 200-item)](#activity-batcher) (PRs #477–#481)
5. [Cache tuning sweep](#cache-tuning) (PRs #461–#465)
6. [System Maintenance tab merged into System tab](#maintenance-tab) (PRs #474, #475, #476)
7. [Browser RAM optimization](#browser-ram) (commit `54146c96`)

---

## iTunes path repair operation {#itunes-path-repair}

**Shipped:** April 26–27, 2026.
**Folds into:** TODO **7.9** (Full iTunes regenerate) as Phase 1 — "diff-and-repair" mode.

### Problem
iTunes still references stale on-disk paths after organize/rename — common when many files have been moved out from under iTunes and the existing path reconciler can't help because `Book.FilePath` itself is also stale.

### Solution: three-tier resolver
Per missing track, escalate through tiers:
- **Tier A** — PID → DB lookup via `external_id_map`, then prefer matching `BookFile.FilePath` before falling back to `Book.FilePath`. Only auto-applies when the DB-known path also exists on disk.
- **Tier B** — embedded `AUDIOBOOK_ORGANIZER_ID` tag scan. Lazy: only fires after tier A leaves residue. Walks the audiobook root once, builds book-ID → on-disk-path index, resolves single-match cases.
- **Tier C** — fuzzy ranking via `matcher.ScoreMatch` against iTunes track title + original basename. Threshold 85 (Jaro-Winkler 0.85). Top-3 candidates emit to `needs_review_items`. **Never auto-applied.**

### Apply behavior
`?apply=true` updates `BookFile.FilePath` / `ITunesPath` (or `Book.*` fallback), records `book_path_history` row with `change_type="itunes_path_repair"`, enqueues through `WriteBackBatcher`. Dry-run by default. Resume after interruption also defaults to dry-run.

### Reports
Every run drops a JSON to `<RootDir>/reports/itunes-repair-<opID>.json` plus inline payload via `UpdateOperationResultData`. Partial reports are persisted on every progress tick (PR #471) so usage-limit interruptions don't lose data.

### What ships
- `internal/itunes/service/path_repair.go` — `PathRepairer` operation
- `internal/itunes/service/path_repair_resolver.go` — pure-function tier A/B/C resolvers + `fsTagScanner`
- `POST /operations/itunes-path-repair` (PermScanTrigger gated)
- 18 new tests covering all three tiers, fsTagScanner, lookupBookID, apply mode, end-to-end across all four track outcomes

### Production validation
Dry-run `01KQ64DYPNC6BABHGV2575VKXQ` (April 27) resolved 7,637 of 8,066 stale paths (94.7%). 429 candidates remain in tier-C `needs_review_items` for human triage.

### Why this is part of 7.9
Phase 1 of the "full iTunes regenerate" work. Phase 2 (full rebuild from scratch when ITL is corrupt) is the remaining open work — see TODO 7.9 for that scope.

---

## Metadata review pagination + refresh cycle fix {#metadata-review}

**Shipped:** April 26 (#466), April 27 (#473, #481).

### Problem
The metadata review dialog "spun forever showing 0 books" when opening for large fetches. Root cause: `handleGetOperationResults` returned all N results in one response, then the frontend made N sequential `getBook()` API calls to check `metadata_review_status`. For a 5,000-book fetch that's 5,000+ HTTP round-trips before the first render.

### Solution: server-side pagination
- `GetOperationResultsPage(id, limit, offset)` added to `OperationStore` interface — SQL `LIMIT/OFFSET` in SQLite, load+slice in PebbleDB.
- `handleGetOperationResults` accepts `?limit=&offset=` (default 100/0), returns `total_count`.
- `MetadataReviewDialog` uses server-side pagination; per-book `getBook()` waterfall removed entirely; polling uses `limit=1` to cheaply check total count.
- Mocks regenerated via `make mocks`.

### Follow-up (PRs #473, #481): apply-refresh cycle
The dialog still re-fetched the entire library each time the user applied a candidate. Fix:
- `MetadataReviewDialog.tsx`: removed `triggerRefetch()` mid-review; added `hasChangesRef` to defer library refresh until dialog close.
- `listCache` TTL tightened (24h/unbounded → 10m/200), `bookCache` capped (unbounded → 2000).
- Stale-fetch guard added (PR #473) so a fetch initiated before a state change can't overwrite the newer state.

---

## Config persistence JSON round-trip {#config-persistence}

**Shipped:** PR #472, April 26.

### Problem
Every new `config.Config` field required manual registration in 3 separate places. Any miss caused silent data loss on restart. Google Books API key, AI options, and several other fields were silently dropping.

### Solution
- `SaveConfigToDatabase` stores the full non-secret `Config` as a single `config_blob` JSON entry; secrets still encrypted individually.
- `UpdateConfig` applies all non-secret fields via `json.Unmarshal` partial merge — any new field with a `json` tag is handled automatically with zero additional code.
- `LoadConfigFromDatabase` reads blob-first (new installs), falls back to legacy key-value for existing installs, writes blob on first save transparently.

### Knock-on benefit
The new burndown-queue items that add config fields (e.g. AI-MODEL-1's per-feature model knobs, CACHE-FOLLOWUP-1's TTL field) don't need any registration glue — just add the JSON-tagged field and it persists.

---

## Activity batcher (15s / 200-item) {#activity-batcher}

**Shipped:** PRs #477–#481, April 27.
**Implementation plan:** [`docs/plans/activity-batcher-plan.md`](../../plans/activity-batcher-plan.md) — kept under `docs/plans/` rather than `docs/superpowers/plans/` because it predates the superpowers convention adoption for this kind of work. Future activity-batcher follow-ups go under `docs/superpowers/`.

### Problem
A moderate library scan emitted 2,000–5,000 activity rows per run. The activity log became noise; the frontend struggled to render thousands of rows; `CompactByDay` only helped retroactively.

### Solution: semantic batching window
- 15-second window per `BatchKey = (type, source, operation_id)`.
- 200-item cap per batch.
- Non-batchable entries (one-off events, errors, audit entries) unchanged.
- Frontend renderer expands/collapses batched entries.

### Architecture
```
log.Printf / LogBatch()
        │
        ▼
  activity.Writer.sendEntry() ──► Is batchable? ─Yes─► ActivityBatcher
                                         │                │  15s timer (per BatchKey)
                                         No               │  or 500-item cap
                                         │                ▼
                                  channel (10k deep)   Flush → channel
                                         │
                                         ▼
                                  drain() goroutine
                                  (100 entries / 500ms)
                                         │
                                         ▼
                                  ActivityStore.Record()
```

### What ships per PR
- **#477** `ActivityBatcher` core (semantic 15s grouping)
- **#478** Pagination + last-updated indicator + large-log warning
- **#479** Expandable batch entry rendering in ActivityLog
- **#480** Wire ActivityBatcher into Writer
- **#481** `LogBatch` / `FlushOperation` structured batch API (opt-in)

### Open follow-ups
- **ACT-BATCH-FU-1** — Test that pending batches flush on context cancel. ([spec](2026-04-27-activity-batcher-followups-design.md))
- **ACT-BATCH-FU-2** — Convert scanner per-file logs to LogBatch (first real LogBatch consumer).

---

## Cache tuning sweep {#cache-tuning}

**Shipped:** PRs #461–#465, April 26.

### Problem
After cache observability landed (PR #444, April 25), telemetry exposed several issues:
- 461: `indexedStore` decorator ate the `AIJobsStore` interface assertion (cache stats Total column was wrong).
- 462: All cache TTLs at 5 min — too aggressive for slow-changing data.
- 463: List-cache invalidated on every book update — wrecked hit rate during scans.
- 464: Metadata-fetch cache invalidated on apply, defeating its purpose.
- 465: Bulk-fetch cache write was dead code (executed inside the source loop, never reached the store).

### Solution
- All cache TTLs raised to 24h.
- List-cache invalidation made opt-in (`skipListCacheInvalidation` config).
- Metadata-fetch cache preserved across apply (configurable TTL); see [`metadata-fetch-ttl-design.md`](2026-04-27-metadata-fetch-ttl-design.md) for the **next** step (TTL enforcement on read).
- Bulk-fetch cache write moved outside the source loop.
- Indexed-store unwrap added for `AIJobsStore` assertion.

### Open follow-ups
- **CACHE-FOLLOWUP-1** — metadata-fetch TTL enforcement on read. ([spec](2026-04-27-metadata-fetch-ttl-design.md))
- Tune per-cache `maxEntries` from 30 days of history (deferred — needs data).
- OTel migration (deferred — Prometheus already covers metrics; OTel is a separate, larger PR).

---

## System Maintenance tab {#maintenance-tab}

**Shipped:** PRs #474–#476, April 27.
**Original spec:** [`docs/superpowers/specs/2026-04-27-system-maintenance-tab-design.md`](2026-04-27-system-maintenance-tab-design.md)
**Implementation plan:** [`docs/superpowers/plans/2026-04-27-system-maintenance-tab-plan.md`](../plans/2026-04-27-system-maintenance-tab-plan.md)

### Deviation from spec
The spec proposed a top-level "Maintenance" tab. PR #475 merged the feature into the existing "System" tab instead — surfaced as a section within System rather than a peer tab. Reason: navigational clutter; users already think of these knobs as "system stuff."

### What ships
- Backend: `is_running` field on `TaskInfo`; new endpoints for maintenance window status + config (PR #476).
- Frontend: maintenance window scheduling UI; null-tasks-array crash fix (#474).

### Status
Largely complete. Any remaining UI polish would be incremental and is not currently tracked.

---

## Browser RAM optimization {#browser-ram}

**Shipped:** commit `54146c96`, April 27 (alongside metadata-review apply-refresh fix in PR #481).

### Problem
Library views (grid + list + cover gallery) accumulated DOM nodes and blob URLs without bound. Long-running tabs reached 2 GB+ resident set.

### Solution
- `AudiobookCard.tsx` — `loading="lazy"` on cover images.
- `AudiobookGrid.tsx` / `AudiobookList.tsx` — `content-visibility: auto` CSS so off-screen rows skip layout/paint.
- `listCache` capped (24h/unbounded → 10m/200).
- `bookCache` capped (unbounded → 2000).

### Result
Steady-state RAM for a 10K-book library tab dropped from ~1.8 GB to ~250 MB after a few minutes of scrolling.
