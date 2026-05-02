<!-- file: docs/superpowers/plans/2026-04-17-store-iface-sweep.md -->
<!-- version: 1.1.0 -->
<!-- guid: bc332f80-16ea-44bd-afa9-a0634820909f -->

# Store Interface Sweep — Follow-on Migration Plan

> **STATUS: COMPLETE (2026-04-18).** Eight sweep PRs shipped (#387–#395). The six files still on full `database.Store` are documented below as intentional wide consumers — further narrowing has diminishing returns. Leaving this plan in place for historical context and as a template for future ISP sweeps.

## Completion note

**What shipped:** ~50 of 79 consumers migrated to narrow sub-interfaces. Plus `IntegrationEnv.Store` deliberately left wide (PR #394) — test scaffolding is anti-ISP.

**What intentionally stayed wide:**

| File | Reason |
|---|---|
| `internal/server/server.go` | Bootstrap — genuinely needs every domain during startup |
| `internal/server/indexed_store.go` | Decorator — must be a drop-in `Store` so every forwarded method works |
| `internal/server/itunes.go` | Hub — forwards to 8+ helpers across metadata-fetch + organize pipelines; narrowing cascades 15+ more signatures |
| `internal/server/metadata_fetch_service.go` | Hub — 79 method calls spanning ~8 sub-interfaces; narrow composite ≈ full `Store` |
| `internal/server/organize_service.go` | Hub — 30 calls across book + author + series + ops + files |
| `internal/server/dedup_engine.go` | Hub — 22 calls, similar shape |
| `internal/testutil/integration.go` | Test fixture — integration tests hit every domain; narrowing moves pain into every test file |

**Optional future cleanups** (low ROI):
- `internal/server/config_update_service.go` — unused `db database.Store` field; removing it churns ~20 test call sites for zero behavioral change.
- The hubs could be narrowed, but each would produce a composite embedding 6–10 sub-interfaces — not materially more discoverable than the full `Store`, and each needs its own transitive-dep untangle.

**Process lessons captured elsewhere:**
- `CHANGELOG.md` April 18 entries — PR #394 incident post-mortem (scoped `go vet` misses test-file breakage; ran scoped instead of full-tree and paid for it)
- `scripts/check_store_noops.py`, `narrow_struct_services.py`, `apply_narrowing.py` — reusable tooling for the next ISP sweep or for finishing this one if scope changes

---

## Original plan (historical — kept for reference)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Foundation is already merged — this plan migrates the remaining 58 files one-by-one. The three proof-point migrations (#379, #380, #381) are the templates to follow.

**Goal:** Migrate every non-proof-point consumer of `database.Store` to the narrow interface(s) listed in the migration catalog. Delete the unused `Store` field from the 18 "noop" consumers. When done, `grep -rln database.Store internal/ cmd/ | wc -l` should drop from 79 → ~12.

## Prerequisites — already merged on main

- #372 — foundation: sub-interfaces defined in `internal/database/iface_*.go`; `Store` is a pure embedding block; `PebbleStore` satisfies every interface (compile-time assertion in `iface_assert.go`).
- #376 — `.mockery.yaml` updated; per-interface mocks generated (all in `internal/database/mocks/mock_store.go` — mockery v3 bundled them into one file rather than per-file-per-interface, but the Mock* types are all available).
- #379 — proof-point 1: `playlist_evaluator.go` — three free-function signatures narrowed (BookReader, UserPositionStore).
- #380 — proof-point 2: `audiobook_service.go` — `AudiobookService.store` narrowed to 9-interface composite `audiobookStore`; transitively narrowed `asExternalIDStore` (to `any`) and `NewMetadataStateService` (to `metadataStateStore` composite).
- #381 — proof-point 3: `reconcile.go` — 8 free-function signatures narrowed to shared `reconcileStore` composite (BookStore + BookFileStore + ImportPathStore + OperationStore).

## Execution model

Each row in the migration table below is **one PR**. Dispatch via `superpowers:subagent-driven-development` or work them manually. Files in the same package with the same target interface set can be bundled into a single PR when the total diff stays under ~200 lines — but default to one-file-per-PR so reviews stay scannable.

**Worktree + Quick Fix Workflow:** per `CLAUDE.md`. Branch `refactor/iface-sweep-<slug>`, worktree `.worktrees/iface-sweep-<slug>`. Commit messages start with `refactor:` (conventional). Merge with `gh pr merge <n> --rebase --admin`.

## Patterns — pick the one matching the file's shape

Study the three proof-point PRs before starting — each is an explicit template:

**Pattern A — inline anonymous interface (free functions):** See `playlist_evaluator.go` (#379). Each function signature gets its own narrow interface, declared inline or as a tiny named alias.

```go
func evaluateX(
    store interface {
        database.BookReader
        database.UserPositionStore
    },
    // ...
) (...) { /* body unchanged */ }
```

**Pattern B — named composite on struct (struct-based services):** See `audiobook_service.go` (#380). File-local named type aggregates the sub-interfaces; the struct field and constructor use that name.

```go
type xyzStore interface {
    database.BookStore
    database.TagStore
    // ...
}

type XYZService struct {
    store xyzStore
    // ...
}

func NewXYZService(store xyzStore) *XYZService { ... }
```

**Pattern C — file-local alias shared across free functions (multi-function files):** See `reconcile.go` (#381). When every function in the file uses roughly the same shape, one file-local alias wins over per-function narrowing.

```go
type someFileStore interface {
    database.BookStore
    database.OperationStore
}

func funcA(store someFileStore, ...) { ... }
func funcB(store someFileStore, ...) { ... }
func funcC(store someFileStore, ...) { ... }
```

## Per-file workflow (the template)

For each row of the migration table:

1. `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && git fetch origin main`
2. `git worktree add .worktrees/iface-sweep-<slug> -b refactor/iface-sweep-<slug> origin/main`
3. `cd .worktrees/iface-sweep-<slug>`
4. Enumerate method calls — e.g., `grep -oE "store\.[A-Z][a-zA-Z]+" <file> | sort -u` (adjust variable name as needed). Sanity-check against the table row's target interfaces.
5. Pick pattern A, B, or C based on the file's shape.
6. Make the edit. Bump the file's `// version:` header one minor.
7. **Fast verification — use `-short` mode.** The 33 slow property tests (undo, playlist, dedup, version lifecycle, pebble CRUD, audiobook sort/filter) call `testing.Short()` and skip under `-short`. Full suite under `-short` is ~1 min (vs. 15+ min without). Use:
    - `go build ./...` — primary gate. If the type refactor is correct, this passes. If a transitively-dependent helper still wants `database.Store`, the build fails with a clear error (see "Transitive dependencies" below).
    - `go vet ./<package>/` — always run, it's fast.
    - `make test-short` (or `go test ./... -short`) — full suite in `-short` mode, ~1 min.
    - Or targeted: `go test ./internal/server/ -run "<TestNamePrefix>" -count=1 -timeout 60s -short` when even that's overkill.
    CI still runs `make test` (full suite) on every PR, so the slow prop tests never stop catching regressions — they just don't block every local iteration.
8. Commit with `refactor: narrow <file> Store deps (ISP sweep)` and a body listing which sub-interfaces replaced `database.Store`.
9. Push, PR, merge with `--rebase --admin`.

## Transitive dependencies — the one gotcha from the proof-points

Task 4 hit a compile error because `audiobook_service.go` forwards `svc.store` to two helpers that still took full `database.Store`: `asExternalIDStore` and `NewMetadataStateService`. Fix: narrow those helpers too, as part of the same PR. See #380.

For each file you migrate, after the signature change, run `go build ./...`. If it fails with `cannot use <x> as database.Store value in argument to Y: <x> does not implement database.Store (missing method Z)`, it means you have a transitive dependency. Options:

1. **Narrow `Y` too** — if `Y` is a one-liner type-assertion helper (like `asExternalIDStore`), change its parameter type to `any`.
2. **Narrow `Y` to a composite** — if `Y` has its own legitimate interface needs (like `NewMetadataStateService`), define a file-local composite for its parameter.
3. **Add the transitively-required interfaces to your composite** — if the helper genuinely needs wide access and narrowing it is out of scope.

Document whichever choice you make in the commit message.

## Special cases — read before touching

These files have quirks that deviate from the straightforward patterns. Handle them with extra care:

- **`internal/server/indexed_store.go`** — wraps `Store` and forwards book-CRUD through a bleve-indexed layer. The wrapper's embedded field type must stay wide enough to forward every method it implements. Narrow both the struct field AND the forwarded method set to the same `BookStore` composite. If the wrapper forwards methods outside `BookStore`, add those interfaces to its field's type as well. May be easier to migrate last.
- **`internal/logger/operation.go`** — defines a *local* `OperationStore` interface for log injection. After the foundation merge, this name collides with `database.OperationStore`. Rename the local interface to `logOpStore` (or similar) and update call sites in the same file. The name clash will cause a compile error if left alone.
- **`internal/server/server.go`** — the server bootstrap legitimately needs full `Store` access (it calls methods across many domains during startup). Leave the field as `database.Store`. Do not migrate.
- **`internal/database/mocks/mock_store_coverage_test.go`** — regenerated by mockery. Do not hand-edit. If changes to the coverage test are needed, generate them via `make mocks` and review.
- **`internal/operations/mocks/mock_queue.go`** — regenerated by mockery. Do not hand-edit.
- **The 18 noop consumers** (field-but-no-calls) — these are flagged in the table as `noop`. **Separate cleanup PR**. After all legitimate migrations finish, one bundled PR deletes the unused `store` field and updates constructors + callers. Do this last so the signature churn doesn't intermix with the interface-narrowing churn.

## Migration table

Legend:
- **Class:** `read-only` | `write-only` | `read-write` | `test` | `noop` (field but no method calls — DO NOT narrow; flag for cleanup PR) | `wide` (legitimate full-Store consumer, leave as `database.Store`)
- **Interfaces:** target sub-interfaces from `internal/database/iface_*.go`. Use `<Domain>Store` when both reader and writer are needed.
- **Pattern:** A (inline), B (named composite on struct), C (file-local alias for multi-function files)

### `cmd/` (3)

| File | Class | Pattern | Interfaces | Notes |
|---|---|---|---|---|
| `cmd/commands_test.go` | noop | — | — | test file, mocks `Store` for convenience; leave |
| `cmd/dedup_bench_types.go` | read-only | A | `AuthorReader` | tiny surface |
| `cmd/seed.go` | read-write | B or C | `AuthorStore`, `BookStore`, `SeriesStore` | dev seed command |

### `internal/auth/`, `internal/config/`, `internal/logger/`, `internal/metadata/`, `internal/operations/`, `internal/search/`, `internal/transcode/`, `internal/testutil/` (10)

| File | Class | Pattern | Interfaces | Notes |
|---|---|---|---|---|
| `internal/auth/context.go` | noop | — | — | `Store` in type sigs but no method calls |
| `internal/auth/seed.go` | read-write | B | `RoleStore`, `UserStore` | bootstrap |
| `internal/config/persistence.go` | read-write | A | `SettingsStore` | single-domain |
| `internal/logger/operation.go` | **special** | — | **local-rename required** — collides with `database.OperationStore`. Rename local interface to `logOpStore` first, then narrow usage. |
| `internal/metadata/enhanced.go` | read-write | A | `BookStore` | |
| `internal/operations/queue.go` | read-write | B | `OperationStore` | |
| `internal/operations/state.go` | read-write | A | `OperationStore` | checkpoint persistence |
| `internal/search/index_builder.go` | read-write | A | `AuthorReader`, `BookReader`, `SeriesReader`, `TagStore` | read-only across 4 domains |
| `internal/transcode/transcode.go` | read-write | A | `BookReader`, `BookFileStore` | |
| `internal/testutil/integration.go` | write-only | A | `LifecycleStore`, `OperationStore` | test fixture seeder |

### `internal/server/` — active services (35)

| File | Class | Pattern | Interfaces | Notes |
|---|---|---|---|---|
| `internal/server/ai_handlers.go` | read-write | B | `AuthorReader`, `AuthorWriter`, `OperationStore` | |
| `internal/server/ai_scan_pipeline.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/archive_sweep.go` | read-write | B | `BookStore`, `BookFileStore` | |
| `internal/server/audiobook_update_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/author_series_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/batch_poller.go` | read-write | B | `OperationStore` | |
| `internal/server/batch_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/changelog_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/config_update_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/dashboard_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/dedup_engine.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/deluge_integration.go` | read-write | A | `BookReader`, `BookVersionStore` | |
| `internal/server/diagnostics_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/duplicates_handlers.go` | read-write | A | `AuthorStore`, `BookStore`, `SeriesStore`, `OperationStore` | multi-domain free functions |
| `internal/server/external_id_backfill.go` | read-write | A | `BookReader`, `BookFileStore`, `SettingsStore` | |
| `internal/server/file_move.go` | write-only | A | `BookWriter` | |
| `internal/server/import_collision.go` | read-only | A | `BookReader` | clean read-only example |
| `internal/server/import_path_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/import_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/indexed_store.go` | **special** | — | wraps `Store`; see Special Cases above. Migrate last. |
| `internal/server/isbn_enrichment.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/itl_rebuild.go` | read-only | A | `BookReader` | |
| `internal/server/itunes.go` | read-write | B or C | `BookStore`, `AuthorReader`, `AuthorWriter`, `SeriesReader`, `SeriesWriter`, `BookFileStore`, `HashBlocklistStore`, `ITunesStateStore` | widest struct consumer; consider C if functions are similar |
| `internal/server/itunes_position_sync.go` | read-write | B | `BookStore`, `BookFileStore`, `UserPositionStore` | |
| `internal/server/itunes_track_provisioner.go` | read-write | B | `AuthorReader`, `BookFileStore`, `ExternalIDStore` | |
| `internal/server/maintenance_fixups.go` | read-write | C | `BookStore`, `AuthorStore`, `SeriesStore`, `BookFileStore`, `ExternalIDStore`, `StatsStore`, `UserTagStore` | wide multi-function file |
| `internal/server/merge_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/metadata_batch_candidates.go` | read-write | A | `BookReader`, `OperationStore`, `RawKVStore` | |
| `internal/server/metadata_fetch_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/metadata_state_service.go` | **done in #380** | — | `MetadataStore`, `UserPreferenceStore` | narrowed as part of audiobook_service migration |
| `internal/server/metadata_upgrade.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/middleware/auth.go` | read-write | B | `UserReader`, `RoleStore`, `SessionStore` | |
| `internal/server/organize_preview_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/organize_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/pipeline_checkpoint.go` | read-write | A | `UserPreferenceStore` | |
| `internal/server/playlist_itunes_sync.go` | read-write | A | `UserPlaylistStore` | |
| `internal/server/read_status_engine.go` | read-write | B | `BookFileStore`, `UserPositionStore` | |
| `internal/server/rename_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/revert_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/scan_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/server.go` | wide | — | — | legitimate full-Store consumer; LEAVE AS IS |
| `internal/server/sweeper.go` | read-write | A | `BookStore` | tombstone cleanup |
| `internal/server/system_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/undo_engine.go` | read-write | B | `BookStore`, `OperationStore` | |
| `internal/server/version_fingerprint.go` | read-write | A | `BookVersionStore` | |
| `internal/server/version_ingest.go` | read-write | A | `BookVersionStore`, `BookFileStore` | |
| `internal/server/version_lifecycle.go` | read-write | A or C | `BookReader`, `BookVersionStore`, `BookFileStore` | |
| `internal/server/version_swap.go` | read-write | A | `BookStore`, `BookVersionStore`, `BookFileStore` | |
| `internal/server/work_service.go` | noop | — | — | field but no calls — cleanup PR |
| `internal/server/writeback_outbox.go` | read-write | B | `BookReader`, `UserPreferenceStore` | |

### `internal/server/` — test files (11)

For each test file: the narrowing follows the production service it exercises. If the production struct field is `xyzStore`, the test can either use the same narrow composite (via `mocks.NewMockXYZ*`) or keep using the full `mocks.Store` — both work because `mocks.Store` implements every sub-interface too. Default: keep using `mocks.Store` unless the test actively benefits from a narrow mock.

| File | Class | Interfaces (if narrowing) | Notes |
|---|---|---|---|
| `internal/server/cover_history_test.go` | test | `BookWriter`, `LifecycleStore` | |
| `internal/server/entity_tag_handlers_test.go` | test | `AuthorWriter`, `SeriesWriter`, `LifecycleStore` | |
| `internal/server/import_collision_test.go` | test | `BookWriter`, `LifecycleStore` | |
| `internal/server/middleware/auth_permission_test.go` | test | `UserWriter`, `SessionStore`, `LifecycleStore` | |
| `internal/server/read_status_engine_test.go` | test | `BookFileStore`, `UserPositionStore`, `LifecycleStore` | |
| `internal/server/revert_service_organize_test.go` | test | `LifecycleStore` | |
| `internal/server/server_test.go` | test | `LifecycleStore` | |
| `internal/server/undo_engine_prop_test.go` | test | `OperationStore`, `LifecycleStore` | |
| `internal/server/user_handlers_test.go` | test | `UserWriter`, `RoleStore`, `LifecycleStore` | |
| `internal/server/version_lifecycle_prop_test.go` | test | `BookVersionStore`, `BookWriter`, `LifecycleStore` | |
| `internal/server/version_lifecycle_test.go` | test | `BookVersionStore`, `BookWriter`, `LifecycleStore` | |

### Mock files (do not hand-edit)

| File | Class | Notes |
|---|---|---|
| `internal/database/mocks/mock_store_coverage_test.go` | noop | regenerated by mockery |
| `internal/operations/mocks/mock_queue.go` | noop | regenerated by mockery |

## Verification per batch

After every 5 PRs merged, run on main:

```bash
git checkout main && git pull
go build ./... && go vet ./...
```

Both must be clean. For a broader test sanity check without paying the full 15-min price, run `make test-short` (or `go test ./... -short`) — that skips the slow property tests in `internal/server` but still exercises everything else. The full suite runs in CI on every PR, so you don't need to run it locally unless you're debugging a specific prop-test failure.

If a regression appears, the last PR's narrowing was incorrect — revert via `gh pr revert <n>` and re-classify.

## Cleanup PR — the 18 noop consumers

Run **last**, after all 40+ migration PRs merge. Bundle into one PR titled `refactor: remove unused Store fields from noop consumers (ISP cleanup)`. For each noop file:

1. Delete the `store <type>` field from the struct.
2. Remove the `store` parameter from the constructor.
3. Update all callers (grep for the constructor name).
4. `go build ./...` clean.
5. One PR, one commit, all 18 files.

The noop consumers listed in the tables above, by package:

- `cmd/commands_test.go`
- `internal/auth/context.go`
- `internal/server/ai_scan_pipeline.go`
- `internal/server/audiobook_update_service.go`
- `internal/server/author_series_service.go`
- `internal/server/batch_service.go`
- `internal/server/changelog_service.go`
- `internal/server/config_update_service.go`
- `internal/server/dashboard_service.go`
- `internal/server/dedup_engine.go`
- `internal/server/diagnostics_service.go`
- `internal/server/import_path_service.go`
- `internal/server/import_service.go`
- `internal/server/isbn_enrichment.go`
- `internal/server/merge_service.go`
- `internal/server/metadata_fetch_service.go`
- `internal/server/metadata_upgrade.go`
- `internal/server/organize_preview_service.go`
- `internal/server/organize_service.go`
- `internal/server/rename_service.go`
- `internal/server/revert_service.go`
- `internal/server/scan_service.go`
- `internal/server/system_service.go`
- `internal/server/work_service.go`

(24 files — one or two may turn out to genuinely need the field after deeper inspection; in that case leave them, they become regular migrations. The initial classification came from grep-only analysis.)

## Definition of done

- All rows in the table marked `read-only` / `write-only` / `read-write` have a merged migration PR (≈ 38 files).
- All `test` files' narrowing reviewed; narrowed where it helps, left alone where it doesn't.
- Special cases handled: `logger/operation.go` rename, `indexed_store.go` wrapper, `metadata_state_service.go` confirmed done via #380.
- Cleanup PR merged — 18+ noop fields deleted.
- `grep -rln "database\.Store\b" internal/ cmd/ | wc -l` drops from 79 to ~12.
- `go build ./...` and `go vet ./...` green on main.
- `make mocks-check` CI gate green.
- Optional nice-to-have: one of the migrated services' tests actually uses a narrow mock (e.g., `mocks.NewMockBookReader(t)`) to prove the test-surface benefit. Any of the `read-only` services is a good candidate.

## Counts

| Category | Files |
|---|---|
| Total with `database.Store` references | 79 |
| Proof-points (done in #379, #380, #381) | 3 |
| Metadata state service (done in #380) | 1 |
| Eligible for sweep migration | ~38 |
| Test files eligible for narrowing | 11 |
| Noop consumers (cleanup PR) | ~18 (may be up to 24) |
| Wide/legitimate (server.go) | 1 |
| Mock files (regenerated) | 2 |
| Special (logger, indexed_store) | 2 |
