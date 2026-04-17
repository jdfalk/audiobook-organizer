# Property-Based Tests with rapid — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Backlog item:** 4.5 — Property-based tests for dedup engine (expanded to full codebase)
**Library:** [pgregory.net/rapid](https://github.com/flyingmutant/rapid)
**Spec:** None — this plan is self-contained.

## Overview

Add property-based tests using `rapid` across the codebase. Property-based tests generate random inputs and verify invariants always hold, catching edge cases that hand-written unit tests miss. Each task is one PR.

## Prerequisites

- Add `pgregory.net/rapid` to go.mod: `go get pgregory.net/rapid@latest`
- All property tests go in `*_prop_test.go` files alongside the code they test

---

### Task 1: rapid generators for core types (1 PR)

**Goal:** Create reusable rapid generators for Book, Author, Series, BookFile, BookVersion, User, UserPlaylist so all subsequent tasks can use them.

**Files:**
- Create: `internal/database/rapid_generators_test.go` — generators for all core types

Generators to write:
- [ ] `genBook(t *rapid.T) *Book` — generates a Book with random title, format, year, optional fields. IDs left empty (CreateBook assigns ULID).
- [ ] `genAuthor(t *rapid.T) Author` — random name, optional aliases
- [ ] `genSeries(t *rapid.T) Series` — random name
- [ ] `genBookFile(t *rapid.T, bookID string) BookFile` — random file path, format, duration, track number
- [ ] `genBookVersion(t *rapid.T, bookID string) BookVersion` — random status from valid set, random format, source
- [ ] `genUser(t *rapid.T) (username, email, password string)` — random strings with constraints (username alphanum, email valid-ish, password 8+ chars)
- [ ] `genUserPlaylist(t *rapid.T) UserPlaylist` — random name, type (static|smart), book_ids or query
- [ ] `genTag(t *rapid.T) string` — random lowercase string 2-20 chars
- [ ] `genOperationChange(t *rapid.T) OperationChange` — random change_type from valid set, random old/new values

Each generator should produce values that pass validation — no empty titles, no invalid statuses, etc.

**Test:** Verify each generator produces valid values by running `rapid.Check(t, func(t *rapid.T) { b := genBook(t); assert b.Title != "" })` etc.

---

### Task 2: PebbleStore CRUD invariants (1 PR)

**Goal:** Verify that Create → Get → Update → Delete round-trips preserve data for all entity types.

**Files:**
- Create: `internal/database/pebble_store_prop_test.go`

Properties to test:
- [ ] **Book round-trip:** `CreateBook(genBook) → GetBookByID → fields match`
- [ ] **Book update preserves ID:** `CreateBook → UpdateBook(modified) → GetBookByID → ID unchanged, modified fields match`
- [ ] **Delete then Get returns nil:** `CreateBook → DeleteBook → GetBookByID returns nil`
- [ ] **BookVersion single-active invariant:** create N versions for same book, at most one has status=active
- [ ] **UserPlaylist name uniqueness:** create two playlists with same name → second fails
- [ ] **User username uniqueness:** create two users with same username → second fails
- [ ] **Tag add/remove roundtrip:** `AddBookTag(bookID, tag) → GetBookTags(bookID) contains tag → RemoveBookTag(bookID, tag) → GetBookTags(bookID) doesn't contain tag`
- [ ] **Session create/revoke:** `CreateSession → GetSession works → RevokeSession → GetSession shows revoked`
- [ ] **OperationChange persistence:** `CreateOperationChange → GetOperationChanges returns it`
- [ ] **ListUsers contains created user:** `CreateUser → ListUsers includes that user`

Each property uses a fresh PebbleStore per `rapid.Check` invocation (via `t.TempDir()`).

---

### Task 3: Search parser round-trip + invariants (1 PR)

**Goal:** Verify the DSL query parser handles arbitrary inputs without panicking and preserves semantics on round-trip.

**Files:**
- Create: `internal/search/query_parser_prop_test.go`

Properties to test:
- [ ] **No panics on arbitrary input:** `ParseQuery(rapid.String())` never panics (may return error)
- [ ] **Parsed AST re-stringifies:** `ParseQuery(input) → ast.String() → ParseQuery(re-stringified) → same AST shape`
- [ ] **Field nodes preserve field names:** any `FieldNode` in the AST has a non-empty `Field`
- [ ] **AND/OR children have arity ≥ 2:** parser never produces singleton AND/OR nodes
- [ ] **Negation wraps exactly one child:** every `NotNode` has a non-nil `Child`
- [ ] **Valid DSL round-trips through translator:** `ParseQuery → Translate → no error` for well-formed queries
- [ ] **Generated valid queries:** build a rapid generator for valid DSL strings (field:value, field:>N, field:(a|b), quoted values) and verify `ParseQuery` succeeds on all of them

---

### Task 4: Dedup engine similarity invariants (1 PR)

**Goal:** Verify dedup similarity properties that must hold regardless of input.

**Files:**
- Create: `internal/server/dedup_engine_prop_test.go`

Properties to test:
- [ ] **Cosine similarity symmetry:** `CosineSimilarity(a, b) == CosineSimilarity(b, a)` for random float32 vectors
- [ ] **Cosine self-similarity is 1.0:** `CosineSimilarity(v, v) ≈ 1.0` for non-zero vectors
- [ ] **Cosine range:** `CosineSimilarity(a, b) ∈ [-1, 1]` for all inputs
- [ ] **Zero vector:** `CosineSimilarity(zero, v) == 0`
- [ ] **FindSimilar result ordering:** results are sorted by similarity descending
- [ ] **FindSimilar excludes below threshold:** no result has similarity < minSimilarity
- [ ] **FindSimilar maxResults cap:** `len(results) <= maxResults`
- [ ] **Chromem FindSimilar matches SQLite FindSimilar:** for the same inputs, both backends return the same entity IDs (order may differ due to ANN approximation — check set equality above threshold)

For the dual-backend comparison test, create a small test collection (10-20 random vectors), upsert into both stores, query from both, verify the top-N sets overlap significantly.

---

### Task 5: Sort stability + filter partitioning (1 PR)

**Goal:** Verify that the library list sorting and filtering operations are well-behaved.

**Files:**
- Create: `internal/server/audiobook_service_prop_test.go`

Properties to test:
- [ ] **Sort stability:** sorting the same list twice produces identical order
- [ ] **Sort is a permutation:** sorted output contains exactly the same elements as input
- [ ] **Filter partitioning:** for any predicate P, `filter(books, P) ∪ filter(books, ¬P) == books`
- [ ] **Pagination consistency:** `GetAllBooks(limit=N, offset=0) ++ GetAllBooks(limit=N, offset=N)` contains no duplicates and covers the same set as `GetAllBooks(limit=2N, offset=0)`

---

### Task 6: Version lifecycle state machine (1 PR)

**Goal:** Verify the version lifecycle transitions are valid and reversible where expected.

**Files:**
- Create: `internal/server/version_lifecycle_prop_test.go`

Properties to test:
- [ ] **Trash is reversible:** `active → trash → restore → status is alt`
- [ ] **Purge is irreversible:** `trash → purge → status is inactive_purged`, restore fails
- [ ] **Auto-promote picks most recent:** create N alt versions with random ingest dates, trash the active one, verify the promoted version has the latest ingest date
- [ ] **Single-active invariant maintained through transitions:** after any sequence of trash/restore/swap operations, at most one version per book has status=active

---

### Task 7: Auth + permissions invariants (1 PR)

**Goal:** Verify permission system properties.

**Files:**
- Create: `internal/auth/permissions_prop_test.go`

Properties to test:
- [ ] **All() returns known permissions:** every element of `All()` passes `IsKnown()`
- [ ] **Admin role has all permissions:** admin's permission set is a superset of every other role
- [ ] **Viewer is subset of editor:** every viewer permission is also in editor
- [ ] **Editor is subset of admin:** every editor permission is also in admin
- [ ] **Context round-trip:** `WithPermissions(ctx, perms) → PermissionsFromContext(ctx) == perms`
- [ ] **Can checks membership:** `WithPermissions(ctx, [P]) → Can(ctx, P) == true`, `Can(ctx, Q) == false` for Q ∉ perms

---

### Task 8: Undo engine idempotency (1 PR)

**Goal:** Verify undo operations are idempotent and produce consistent results.

**Files:**
- Create: `internal/server/undo_engine_prop_test.go`

Properties to test:
- [ ] **Double-undo is idempotent:** `RunUndoOperation(opID) twice → second run has 0 reverted, N skipped`
- [ ] **Undo + redo preserves state:** for file_move changes, undo moves file back, re-running the original operation moves it forward again
- [ ] **Conflict detection is conservative:** if a file was modified after the operation, the undo reports it as a conflict (never silently clobbers)

---

### Task 9: Playlist evaluator invariants (1 PR)

**Goal:** Verify smart playlist evaluation properties.

**Files:**
- Create: `internal/server/playlist_evaluator_prop_test.go`

Properties to test:
- [ ] **Limit is respected:** `EvaluateSmartPlaylist(limit=N) → len(result) <= N`
- [ ] **Empty query errors:** empty/whitespace queries return an error
- [ ] **Deterministic evaluation:** same query + same index → same results
- [ ] **Sort is stable:** evaluating with sortJSON twice produces identical order
- [ ] **Per-user filter isolation:** user A's finished books don't appear in user B's `read_status:finished` results

---

### Estimated effort

| Task | Size | Tests |
|------|------|-------|
| 1. Generators | S | ~10 |
| 2. PebbleStore CRUD | M | ~10 |
| 3. Search parser | M | ~7 |
| 4. Dedup similarity | M | ~8 |
| 5. Sort/filter | S | ~4 |
| 6. Version lifecycle | M | ~4 |
| 7. Auth/permissions | S | ~6 |
| 8. Undo idempotency | M | ~3 |
| 9. Playlist evaluator | S | ~5 |
| **Total** | | **~57 property tests** |

### Critical path

Tasks 1 (generators) must be done first. All others are independent and can run in parallel.
