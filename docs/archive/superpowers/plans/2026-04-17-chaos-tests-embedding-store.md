<!-- file: docs/superpowers/plans/2026-04-17-chaos-tests-embedding-store.md -->
<!-- version: 1.0.0 -->
<!-- guid: 844cc2fc-bbb9-4361-829e-3d828649ab9b -->
<!-- last-edited: 2026-04-16 -->

# 4.6: Chaos Tests for Embedding Store Under Shutdown — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Backlog item:** 4.6 — Chaos tests for embedding store under shutdown
**Spec:** None — this plan is self-contained.
**Depends on:** `pgregory.net/rapid` already in go.mod (from 4.5 property-based tests)

## Overview

The embedding store has two backends: `EmbeddingStore` (SQLite, `internal/database/embedding_store.go`) for candidate CRUD and linear-scan similarity, and `ChromemEmbeddingStore` (chromem-go, `internal/database/chromem_embedding_store.go`) for HNSW-based ANN search. Both can be accessed by background goroutines (dedup engine, ISBN enrichment, metadata scorer) while the server is shutting down. This plan adds chaos and concurrency tests to verify graceful degradation: no panics, no data corruption, no deadlocks.

## Prerequisites

- `pgregory.net/rapid` in go.mod
- Familiarity with `EmbeddingStore.Close()` (embedding_store.go:93) and `ChromemEmbeddingStore.Close()` (chromem_embedding_store.go:179)
- Both stores use `sync.RWMutex` for internal synchronization

---

### Task 1: ChaosEmbeddingStore wrapper (1 PR)

**Goal:** Create a wrapper around `EmbeddingStore` that injects random failures at configurable rates, similar to the existing ChaosStore pattern from the fault injection tests in 4.5.

**Files:**
- Create: `internal/database/chaos_embedding_store_test.go`

- [ ] `ChaosEmbeddingStore` struct wrapping `*EmbeddingStore` with a `failRate float64` field and an `rng *rand.Rand`
- [ ] Wrap all public methods: `Upsert`, `FindSimilar`, `Close`, `UpsertCandidate`, `ListCandidates`, `UpdateCandidateStatus`
- [ ] Each wrapped method: with probability `failRate`, return a synthetic error instead of calling the real method
- [ ] `SetFailRate(rate float64)` — allows dynamic adjustment during tests
- [ ] `SetLatency(min, max time.Duration)` — adds random sleep before each call to simulate slow I/O
- [ ] Verify: 100 Upsert calls with 30% fail rate produces ~30 errors (within statistical bounds)

**Acceptance criteria:**
- [ ] ChaosEmbeddingStore passes the same interface as EmbeddingStore for all embedding operations
- [ ] Fail rate is statistically correct over 1000 trials (within 5% of target)
- [ ] Latency injection works without deadlocking

---

### Task 2: Concurrent read/write stress tests (1 PR)

**Goal:** Verify that concurrent reads and writes to both embedding stores do not cause data races, panics, or deadlocks.

**Files:**
- Create: `internal/database/embedding_store_concurrent_test.go`

Tests to write:
- [ ] **SQLite concurrent upserts:** 10 goroutines each upserting 100 embeddings with random entity IDs; verify all succeed or return serialization errors (no panics)
- [ ] **SQLite concurrent read+write:** 5 writers upserting, 5 readers calling FindSimilar simultaneously; verify no data races (`go test -race`)
- [ ] **Chromem concurrent upserts:** 10 goroutines each upserting 50 embeddings to the in-memory chromem store; verify all succeed
- [ ] **Chromem concurrent FindSimilar:** 5 goroutines querying while 5 goroutines upserting; verify results are valid (similarity in [-1, 1], correct entity IDs)
- [ ] **Mixed backend concurrent access:** SQLite candidate CRUD + chromem vector upsert running simultaneously; no cross-contamination or deadlocks
- [ ] **Property test (rapid):** generate random sequences of Upsert/FindSimilar/Delete operations, run them concurrently, verify the final state is consistent (all upserted-and-not-deleted entities are findable)

**Acceptance criteria:**
- [ ] All tests pass with `-race` flag
- [ ] No test takes longer than 10 seconds
- [ ] Property test runs at least 100 iterations

---

### Task 3: Shutdown race tests — writes after Close (1 PR)

**Goal:** Verify that calling store methods after `Close()` returns errors gracefully instead of panicking.

**Files:**
- Create: `internal/database/embedding_store_shutdown_test.go`

Tests to write:
- [ ] **SQLite Upsert after Close:** close the store, then call Upsert — verify it returns a non-nil error (not a panic)
- [ ] **SQLite FindSimilar after Close:** close the store, then call FindSimilar — verify error return
- [ ] **SQLite UpsertCandidate after Close:** same pattern
- [ ] **Chromem Upsert after Close:** close the chromem store, then Upsert — verify error or graceful no-op
- [ ] **Chromem FindSimilar after Close:** close, then query — verify error
- [ ] **Chromem Delete after Close:** close, then delete — verify error
- [ ] **Double Close:** call Close() twice on each store — verify no panic
- [ ] **Property test (rapid):** generate a random sequence of operations, insert a Close() at a random position, run remaining operations — verify no panics, all post-Close operations return errors

If any of these tests reveal that the current code panics instead of returning an error, note the fix needed as a sub-task.

**Acceptance criteria:**
- [ ] All post-Close operations return errors, never panic
- [ ] Double-Close is idempotent (no error on second call, or same error)
- [ ] Property test covers at least 50 random Close positions

---

### Task 4: Shutdown race tests — concurrent Close + operations (1 PR)

**Goal:** Verify that calling Close() while other goroutines are mid-operation does not cause panics, deadlocks, or data corruption.

**Files:**
- Modify: `internal/database/embedding_store_shutdown_test.go` (or create a separate file if cleaner)

Tests to write:
- [ ] **SQLite Close during concurrent writes:** start 10 goroutines upserting in a loop; after 50ms, call Close() from the main goroutine; verify all goroutines finish (no hang), some may get errors
- [ ] **SQLite Close during concurrent reads:** start 5 goroutines calling FindSimilar in a loop; Close() mid-flight; verify no panic
- [ ] **Chromem Close during concurrent writes:** same pattern with chromem store
- [ ] **Chromem Close during concurrent reads:** same pattern
- [ ] **Deadline test:** wrap Close-during-writes test with a 5-second timeout; if it hangs, the test fails instead of blocking forever
- [ ] **Property test (rapid):** generate N goroutines with random operation sequences, inject Close() at a random time, verify: (1) no panics, (2) no goroutines leaked (use runtime.NumGoroutine before/after), (3) no deadlocks (timeout)

**Acceptance criteria:**
- [ ] No deadlocks (all tests complete within 5 seconds)
- [ ] No panics under `-race`
- [ ] Goroutine count returns to baseline after all operations complete

---

### Task 5: Data integrity verification after chaos (1 PR)

**Goal:** After running a chaos sequence (random failures, concurrent access, mid-operation close), verify that the data in the store is internally consistent.

**Files:**
- Create: `internal/database/embedding_store_integrity_test.go`

Tests to write:
- [ ] **SQLite integrity after chaos:** run 1000 random operations through ChaosEmbeddingStore (30% fail rate), then: (a) every entity that was successfully upserted and not deleted should be findable via FindSimilar with self-similarity ~1.0, (b) every entity that was successfully deleted should not appear in FindSimilar results
- [ ] **Chromem integrity after chaos:** same pattern with chromem store + chaos wrapper
- [ ] **Cross-backend consistency:** upsert the same set of entities into both stores through chaos, then compare: for each entity present in SQLite's FindSimilar, it should also be present in chromem's FindSimilar (and vice versa), allowing for ANN approximation tolerance
- [ ] **Candidate table integrity:** after chaos CRUD on dedup candidates, verify: (a) no duplicate (entity_a_id, entity_b_id, layer) triples, (b) all status values are valid enum members, (c) updated_at >= created_at for all rows
- [ ] **Property test (rapid):** model the expected state as a simple map, replay the successful operations against the model and the real store, assert they agree

**Acceptance criteria:**
- [ ] All integrity checks pass after 1000-operation chaos sequences
- [ ] Cross-backend agreement is >95% (allowing for ANN approximation in chromem)
- [ ] Property test runs at least 50 iterations with different random seeds

---

### Estimated effort

| Task | Size | Depends on |
|------|------|------------|
| 1 (chaos wrapper) | S | -- |
| 2 (concurrent stress) | M | -- |
| 3 (writes after close) | M | -- |
| 4 (close during ops) | M | 3 |
| 5 (integrity after chaos) | M | 1 |
| **Total** | ~5 PRs, M overall | |

### Critical path

Tasks 1, 2, and 3 are independent and can run in parallel. Task 4 builds on task 3's patterns. Task 5 depends on the chaos wrapper from task 1. All tasks should run with `go test -race`.
