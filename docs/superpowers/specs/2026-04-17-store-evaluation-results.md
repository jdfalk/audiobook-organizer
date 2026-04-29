<!-- file: docs/superpowers/specs/2026-04-17-store-evaluation-results.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3b6f7f2f-61f7-4a7d-8a44-9b77739f5e11 -->
<!-- last-edited: 2026-04-29 -->

# 4.1 / 4.7 PostgreSQL Research Track — Store Evaluation Results

## Executive summary

PostgreSQL should be adopted as the long-term primary metadata store for the project (books/authors/series/files/users/playlists/tags/activity/external IDs), while keeping specialized engines where they already fit well (Bleve for full-text search in the short term, and existing embedding path until pgvector migration is explicitly approved). The key reason is not only performance headroom but operational maturity and query expressiveness: PostgreSQL gives durable transactions, relational constraints, mature indexing, strong tooling, and a large Go ecosystem, reducing the risk of continued ad-hoc key/value evolution in PebbleDB as feature complexity grows.

## Scope and framing

This document closes the research track by providing an architecture-level recommendation and migration analysis. It is intentionally a decision artifact, not an implementation PR for store replacement.

Workloads considered:
1. Metadata CRUD
2. Vector search
3. Full-text search
4. Activity log / time-series style append+query
5. External ID map
6. Configuration / state

Candidate technologies considered:
- PebbleDB (status quo)
- PostgreSQL
- SQLite (expanded/unified)
- CockroachDB (PostgreSQL-compatible distributed option)
- Meilisearch/Typesense (dedicated full-text search)
- pgvector (vector extension for PostgreSQL)

## Current architecture snapshot

- **Primary transactional-ish application state:** PebbleDB
- **Embeddings:** SQLite-backed chromem-go integration
- **Full-text search:** Bleve index path
- **Server architecture state:** DI + partial interface segregation already underway (4.4 complete, 4.8 in progress), which lowers migration risk versus earlier codebase state.

## Method and evidence notes

A full benchmark harness and production-grade comparative load testing was outlined in `docs/superpowers/plans/2026-04-17-store-evaluation.md` but has not been completed in this branch. Therefore this decision document provides:
- a scoring matrix grounded in known system behavior and ecosystem constraints,
- explicit confidence levels,
- a concrete migration plan and rollback strategy,
- and a clear list of follow-up empirical benchmarks required before cutover.

Confidence labels used below:
- **High:** operationally validated patterns and broadly established DB behavior
- **Medium:** likely in this system, pending harness confirmation
- **Low:** speculative without targeted benchmark data

## Evaluation criteria and weights

- **Performance** (30%)
- **Operational complexity** (20%)
- **Migration cost** (20%)
- **Scalability** (10%)
- **Feature richness** (10%)
- **Community / ecosystem** (10%)

## Scoring matrix (1–5, weighted)

| Backend | Perf (30) | Ops (20) | Mig cost (20) | Scale (10) | Features (10) | Ecosystem (10) | Weighted total |
|---|---:|---:|---:|---:|---:|---:|---:|
| PebbleDB (status quo) | 3 | 3 | 5 | 3 | 2 | 3 | **3.3** |
| PostgreSQL | 4 | 4 | 2 | 4 | 5 | 5 | **3.8** |
| SQLite (expanded) | 3 | 5 | 3 | 2 | 3 | 5 | **3.5** |
| CockroachDB | 3 | 2 | 2 | 5 | 4 | 4 | **3.0** |

Notes:
- PebbleDB scores very high on migration cost only because it is already in place.
- PostgreSQL wins on aggregate despite migration cost because long-term maintainability and feature velocity are stronger.
- SQLite remains attractive for low-ops single-node deployments but concurrency/multi-writer behavior is a practical ceiling for this project’s growth direction.
- CockroachDB is not justified at current scale/complexity.

## Per-workload recommendation

### 1) Metadata CRUD
**Recommendation:** Migrate to PostgreSQL.  
**Confidence:** Medium-high.

Rationale: normalized relationships, constraints, and transactional semantics align better with current feature breadth (users/permissions/playlists/history/tagging/maintenance operations).

### 2) Vector search
**Recommendation:** Hold current embedding store now; evaluate pgvector as a phase-2 migration after metadata stabilization on PostgreSQL.  
**Confidence:** Medium.

Rationale: avoid coupling two major storage migrations at once; vector recall/perf tuning should be isolated.

### 3) Full-text search
**Recommendation:** Keep Bleve short-term; optionally evaluate PostgreSQL FTS or external search only after metadata migration settles.  
**Confidence:** Medium.

Rationale: Bleve already integrated; replacing search at same time as primary store adds unnecessary risk.

### 4) Activity log / time-series
**Recommendation:** Move to PostgreSQL alongside metadata.  
**Confidence:** High.

Rationale: append-heavy writes plus filtered/range queries are straightforward in PostgreSQL with proper indexing/partition strategy if needed later.

### 5) External ID map
**Recommendation:** Move to PostgreSQL.  
**Confidence:** High.

Rationale: point lookups and reverse lookups map cleanly to indexed relational tables.

### 6) Configuration / state
**Recommendation:** Move durable config/state to PostgreSQL; keep process-local caches in memory as today.  
**Confidence:** High.

Rationale: consolidates backup/restore and consistency model.

## PostgreSQL migration path (phased)

1. **Schema foundation + adapter layer (no cutover):** add PostgreSQL store implementation behind existing interfaces.
2. **Dual-read validation for low-risk tables:** external IDs, activity log reads compared for correctness.
3. **Dual-write (idempotent) for selected workloads:** external IDs and activity append path first.
4. **Metadata cutover by bounded domains:** authors/series/books/book_files, then users/playlists/tags.
5. **Stop writes to PebbleDB for migrated domains; keep read-only fallback window.**
6. **Finalize:** disable fallback, freeze export snapshots, remove dead Pebble-only code paths in follow-up cleanup.

## Risk assessment

Top risks:
- **Schema drift / semantic mismatch** (Medium): Pebble key patterns encode assumptions not yet explicit in SQL schema.
- **Migration correctness on edge metadata states** (Medium-high): historical records may violate stricter relational constraints.
- **Operational unfamiliarity for contributors** (Low-medium): mitigated by broad PostgreSQL familiarity and tooling.
- **Performance regressions on unindexed queries** (Medium): mitigated via query plan reviews and pre-cutover perf checks.

Mitigations:
- invariant tests between old/new store behavior,
- staged domain cutovers,
- dual-write + read-compare canary period,
- rollback switch per workload domain.

## Store interface migration complexity

Based on `internal/database/store.go`, complexity should be tracked per method during implementation. For planning purposes:

- **S (simple CRUD / lookup):** ~40%
- **M (query/filter/list with pagination or joins):** ~45%
- **L (batch/stateful/multi-entity semantics):** ~15%

Estimated total implementation effort for full production-grade PostgreSQL parity: **30–45 developer-days** (including tests, migration tooling, and soak time), excluding optional vector/full-text migrations.

## Pebble-specific code path impact

Expected migration hotspots:
- key encoding/decoding helpers,
- prefix iteration logic,
- manual secondary-index patterns,
- atomic batch assumptions tied to Pebble write batches.

These will need either SQL-native equivalents (indexes/transactions) or compatibility shims during transition.

## Rollback plan

- Maintain feature flag controlling write target per workload domain.
- During cutover, keep Pebble snapshot exports at each phase gate.
- If production issues occur, switch affected domain writes back to Pebble and replay queued events if dual-write lag exists.
- Keep read-compare diagnostics enabled for at least one release cycle after each domain migration.

## Explicit 4.1 decision

**Decision:** Proceed with a phased PostgreSQL adoption for primary application data, starting with low-risk domains and preserving existing specialized search/vector components until separate benchmark-backed decisions are made.

This resolves the PostgreSQL research track at the recommendation level and sets a reviewable migration direction with bounded risk.

## Follow-up required before implementation PRs

1. Build/land benchmark harness from plan 4.7 Task 1.
2. Produce baseline Pebble numbers for W1–W6.
3. Run PostgreSQL adapter benchmark pass and attach results.
4. Re-score the performance row using measured values before first production migration PR.
