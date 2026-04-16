<!-- file: docs/superpowers/specs/2026-04-15-replace-globalstore-with-di-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5c8d3a2f-9b1e-4f70-a4d8-2c7e0f1b9a58 -->

# Replace `database.GlobalStore` with DI — Design

**Status:** Design complete (Apr 15, 2026). Ready for implementation plan.
**Scope item:** TODO.md §4.4.
**Brainstorm:** Apr 15, 2026 session.

## Goal

Remove `database.GlobalStore` as a package-level variable and inject the `database.Store` dependency explicitly through the `Server` struct and service constructors. Unlocks clean per-test mocking, removes the "package global mutation in tests" pattern, and sets up a clean foundation for multi-user request-scoped state (3.7) and any future per-tenant store sharding.

## Current state

- `database.GlobalStore Store` — 455 references across 30 non-test files
- `Server` struct already exists as the natural DI root; holds services (scanService, organizeService, metadataFetchService, ...) but currently they all reach out to `database.GlobalStore`
- Tests universally use `database.GlobalStore = mock; defer database.GlobalStore = nil` — fragile under parallel tests
- 5 other package-globals in the same shape: `GlobalQueue`, `GlobalScanner`, `GlobalMetadataExtractor`, `GlobalWriteBackBatcher`, `GlobalFileIOPool`

## Scope

**This design covers only `GlobalStore`.** The other five globals get the same treatment in follow-up PRs using this work as the template. Reasoning:

- Migration of one global is already ~6 PRs; bundling all six would be 30+
- The design choices (accessor shape, test-rewrite pattern, how non-handler packages accept a Store) are identical across all globals — nail them once for GlobalStore, then replay mechanically
- Each global has its own test-setup idiom; touching them all at once means every test file gets touched multiple times

## DI mechanism

**Manual constructor injection. No framework.**

- `Server` struct gains a `store database.Store` field
- New `Server` constructor takes a `Store` as an argument
- Handlers remain `func (s *Server) handlerX(c *gin.Context)` — replace `database.GlobalStore` with `s.store`
- Non-handler packages (`scanner`, `organizer`, `backup`) take a `Store` as a constructor arg: `scanner.New(store, ...)`
- Tests construct a `Server` directly with a mock store — no more package-var swap

Explicitly **not** using:

- **Wire** (google/wire): codegen + review overhead with no payoff at this scale
- **Uber fx / do**: runtime-reflective DI, not idiomatic Go
- **Context-based** (store in `ctx.Value`): stringly-typed, opaque, wrong for long-lived deps (fine for request-scoped state — see below)

## Request-scoped state (multi-user foresight)

Two kinds of dependency, handled differently:

| Kind | Where it lives | Example |
|---|---|---|
| Long-lived | `Server` struct field | `store`, `queue`, `scanner` |
| Request-scoped | `context.Context` via typed helper | current user, request ID, per-request logger |

For request-scoped, standard Go pattern:

```go
type ctxKey int
const userKey ctxKey = iota

func WithUser(ctx context.Context, u *User) context.Context { return context.WithValue(ctx, userKey, u) }
func UserFromContext(ctx context.Context) (*User, bool)     { u, ok := ctx.Value(userKey).(*User); return u, ok }
```

Middleware sets them on `c.Request.Context()`. Handlers read from there. This spec does **not** implement request-scoped state — but `Server` stays stateless per-request so 3.7 (multi-user) can plug in without a restructure.

## Migration sequencing

### Phase 1 — Bootstrap (1 PR)

- Add `store database.Store` field to `Server` struct
- Add `NewServer(store Store, ...)` constructor
- In `cmd/root.go`'s startup code: initialize the store, then pass it into `NewServer`
- **Dual-live**: `database.GlobalStore` is still assigned at startup so un-migrated code keeps working
- Add `(s *Server) Store() Store` accessor — handlers will migrate to this

**Acceptance**: build passes, all existing tests pass, `s.store` is reachable from handlers even though no handler uses it yet.

### Phase 2 — Migrate handlers (~6 PRs)

One PR per ~5 handler files. In each PR:

- Replace `database.GlobalStore` with `s.Store()` (or `s.store` if the method-call overhead matters; prefer the method for future flexibility)
- Convert the matching test file: replace `database.GlobalStore = mock; defer ...` with `srv := &Server{store: mock}; srv.setupRoutes()`
- File groupings roughly by topic to keep diffs reviewable:
  1. audiobooks_handlers + audiobook_service + versions_handlers
  2. metadata_handlers + metadata_batch_candidates + metadata_fetch_service
  3. organize_handlers + reconcile + maintenance_fixups
  4. auth_handlers + user_tags + entities_handlers
  5. ai_handlers + dedup_handlers + diagnostics_handlers
  6. operations_handlers + system_handlers + file_ops_handlers + filesystem_handlers

**Acceptance per PR**: target files have zero `database.GlobalStore` references, their tests pass without the package-var dance, no behavior change.

### Phase 3 — Migrate non-handler packages (3 PRs)

- `internal/scanner`: `scanner.Scanner` interface already exists; add a `store` field, update `ScanDirectory` / `ProcessBooks` signatures or move them to methods. Uses are in `cmd/root.go` and via `s.scanService`.
- `internal/organizer`: similar.
- `internal/backup`: smaller, straightforward.

Each PR converts one package, updates callers, updates tests.

### Phase 4 — Delete the global (1 PR)

- Remove `var GlobalStore Store` from `internal/database/store.go`
- Remove `InitializeStore` / `CloseStore` side-effects on GlobalStore (may need reshaping to return a `Store` instead of assigning to the package var)
- Verify compile across the repo — any surviving reference is a compile error
- Remove `database.GlobalStore = ...` boilerplate from test files (should be zero by this point)

No special linter needed. If anyone later re-adds `var GlobalStore Store`, the declaration is an obvious code-review red flag. Compile-error > lint-warning.

### Phase 5 (follow-up, out of scope here)

Apply the same five-phase template to:

1. `GlobalQueue` (`internal/operations`)
2. `GlobalScanner` (`internal/scanner`) — may be subsumed by Phase 3 of this spec
3. `GlobalMetadataExtractor` (`internal/metadata`)
4. `GlobalWriteBackBatcher` (`internal/server`)
5. `GlobalFileIOPool` (`internal/server`) + `globalServer` reference

## Testing strategy

Current pattern:

```go
database.GlobalStore = mockStore
t.Cleanup(func() { database.GlobalStore = nil })
srv := &Server{router: gin.New()}
```

New pattern:

```go
srv := &Server{store: mockStore, router: gin.New()}
```

- No more global mutation
- Parallel-safe (`t.Parallel()` works)
- Each test explicitly names its deps

For handlers that also touch `GlobalQueue`, `GlobalMetadataExtractor`, etc., those globals stay in place until their Phase-5 migration — existing test setup for those unchanged.

## Risks

- **Merge conflicts with ongoing feature work.** Phase 2 PRs touch handler files that are hot in other branches. Mitigation: land phases quickly and in small chunks; don't hold a Phase 2 PR open for days.
- **Test file churn.** Every test that mocks the store changes. Reviewers need to look at test conversions as a pattern, not line-by-line.
- **Hidden shared state.** Some helpers may assume `database.GlobalStore` is set globally (e.g., a func outside `Server`). Phase 4 compile errors surface these — each becomes a small follow-up to refactor the helper.

## Non-goals

- Changing the `Store` interface
- Changing `PebbleStore` / `SQLiteStore` implementations
- Introducing a DI framework
- Refactoring services into constructor chains (services still hang off `Server` as fields)
- Multi-user support (3.7) — this spec only makes it possible cleanly, doesn't implement it

## Open implementation questions (deferred, not blockers)

- Should `Server.Store()` be an interface method accessor, or do we expose `s.store` directly? Probably method, to allow future decorator wrapping (audit log, per-tenant routing).
- Does `cmd/root.go` need a meaningful refactor or just a few line changes? Likely the latter — existing `initializeStore` / `closeStore` vars already decouple the init call.
