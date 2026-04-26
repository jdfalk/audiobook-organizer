---
title: Remove GlobalQueue package global (migrate-loop dogfood #2)
slug: remove-global-queue
target_packages:
  - internal/operations
test_runner: "go test -race -json ./internal/operations/..."
prior_examples:
  - docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md
  - docs/superpowers/specs/2026-04-25-remove-activity-recorder-global.md
success_criteria:
  - All tests in internal/operations/ pass
  - Package var operations.GlobalQueue is deleted
  - Package func InitializeQueue is deleted
  - Package func ShutdownQueue is deleted
  - Package func SetGlobalOperationTimeout is deleted
  - The functions SetActivityLogger, SetStore, SetOperationTimeout on
    *OperationQueue remain — they are instance methods, not global helpers
  - Existing tests that set GlobalQueue directly (e.g. TestGlobalQueueFunctions)
    are rewritten to call NewOperationQueue directly instead
---

# Remove GlobalQueue package global

## Goal

Delete `var GlobalQueue Queue` and its three global helper functions
(`InitializeQueue`, `ShutdownQueue`, `SetGlobalOperationTimeout`) from
`internal/operations/queue.go`.

These globals exist only because `main.go` and `cmd/root.go` initialize the
queue before the `Server` struct is created. The server then assigns its own
`server.queue` back to `operations.GlobalQueue` (server.go:1078). This
two-phase init pattern is the source of the global.

This is the **last remaining global** from Phase 1+2 of the eliminate-globals
spec
([2026-04-17-eliminate-remaining-globals-design.md](2026-04-17-eliminate-remaining-globals-design.md)).
All four callback-hook globals (`ActivityRecorder`, `ScanActivityRecorder`,
`DedupOnImportHook`, `itunesActivityRecorder`) were already removed in
earlier sessions.

## Scope

**In scope (operations package only):**
- Delete `var GlobalQueue Queue` from queue.go.
- Delete `func InitializeQueue(...)`.
- Delete `func ShutdownQueue(...)`.
- Delete `func SetGlobalOperationTimeout(...)`.
- Rewrite the test `TestGlobalQueueFunctions` in queue_test.go — it currently
  saves/restores `GlobalQueue` directly. It should instead call
  `NewOperationQueue` and exercise the same behaviors (double-init warning,
  shutdown, timeout) without touching any package var.

**Out of scope (intentionally left for a follow-up):**
- Updating `main.go` — it references `operations.GlobalQueue` and
  `operations.InitializeQueue`. Those will fail to compile after this change.
  That is expected and correct: the caller migration is a separate concern.
- Updating `cmd/root.go` — same rationale.
- Updating `internal/server/server.go` — it assigns
  `operations.GlobalQueue = server.queue`. That line becomes dead code after
  this migration; remove it in the follow-up.
- Updating `internal/testutil/integration.go` — it sets
  `operations.GlobalQueue = queue`. Follow-up.

For *this* migration's success criteria, only `internal/operations/` tests
need to pass. The build will be broken outside that package until the
follow-up runs.

## Behavior — what the failing tests must exercise

### No package-level queue state

A test should import `internal/operations` and use `go/ast` or `go/parser`
(or a simple string search via `os.ReadFile`) to assert that the source file
`queue.go` does NOT contain the string `var GlobalQueue`. This test fails
today and passes after the deletion.

Alternatively (and preferably): write tests that previously relied on the
`GlobalQueue` var being settable, and rewrite them to use `NewOperationQueue`
directly. The existing `TestGlobalQueueFunctions` does `GlobalQueue = nil`
and `GlobalQueue = NewOperationQueue(...)` — these assignments stop compiling
once the var is gone. Rewriting them to just call `NewOperationQueue` and
operate on the returned value is the correct fix, and the resulting tests are
strictly better.

### InitializeQueue behavior tested via NewOperationQueue

The current `TestGlobalQueueFunctions` covers:
1. Double-init logs a warning (currently via `InitializeQueue` on a non-nil
   `GlobalQueue`). After migration: test that calling `NewOperationQueue`
   twice returns two independent queues — no shared state, no warning needed,
   because callers are responsible for not leaking queues. The double-init
   warning was only needed because the global was mutable.
2. `ShutdownQueue` on a nil global is a no-op. After migration: test that
   calling `.Shutdown(timeout)` on a queue returned by `NewOperationQueue`
   works correctly without panicking.
3. `SetGlobalOperationTimeout` sets timeout on the global. After migration:
   test that `queue.SetOperationTimeout(d)` sets the timeout on the returned
   `*OperationQueue` instance directly.

### The test must fail before the deletion

The critical requirement: the tests written in the PLAN phase must actually
fail against the **current** code (which still has `GlobalQueue`), and pass
after the global is deleted. The easiest way to guarantee this:

Write a test that calls `operations.GlobalQueue` by name. Since `GlobalQueue`
is exported, a test that does:

```go
func TestGlobalQueueIsGone(t *testing.T) {
    // This should not compile once GlobalQueue is removed.
    // As a runtime proxy: assert that no exported package-level Queue var exists.
    // Use reflect or ast parsing.
}
```

But the simplest guaranteed-failing approach: the existing
`TestGlobalQueueFunctions` directly references `GlobalQueue` by name. If the
planner **renames** or **rewrites** that test to not reference `GlobalQueue`,
the test will fail to compile today (because the var exists and the test no
longer references it — wait, that's backwards). 

The correct sequence:
1. Planner writes `TestGlobalQueueIsGone` that calls `operations.GlobalQueue`
   and asserts it is nil (it is not nil in current code after `InitializeQueue`
   runs) — this test fails today.
2. Coder deletes `GlobalQueue`, which breaks the existing
   `TestGlobalQueueFunctions` (compile error).
3. Coder also rewrites `TestGlobalQueueFunctions` to not reference the global.
4. All tests pass.

The planner MUST write at least one test that fails today. The simplest
option: a test that asserts the package has no exported `Queue`-typed var
(checked via reflection on the package's exported symbols). OR a test that
directly tries to set `GlobalQueue = nil` — which will fail to compile once
the var is removed, but currently compiles. The harness runs `go test`, so a
compile error counts as a test failure.

Actually the cleanest approach: rewrite `TestGlobalQueueFunctions` to remove
all `GlobalQueue` references AND add `TestQueueIsNotGlobal` (an ast/source
scan). The rewritten test fails today because `GlobalQueue` still compiles
(removing references means the test no longer tests the global behavior, so
the behaviors are untested — but the harness measures test *failures*, not
coverage gaps). 

**Recommended planner strategy:** Add `TestQueueIsNotGlobal` that does:
```go
src, _ := os.ReadFile("queue.go")
if bytes.Contains(src, []byte("var GlobalQueue")) {
    t.Fatal("GlobalQueue package var still exists; delete it")
}
```
This test fails today (the string is present) and passes after deletion. It
is a legitimate behavioral assertion: "the package must not export a global
queue." The coder's job is then to delete the var and fix the compile errors
it causes within the package.

## Edge cases to cover

- `ShutdownQueue(timeout)` when GlobalQueue is nil is currently a no-op.
  The replacement: callers simply don't call `Shutdown` if they never created
  a queue. The test should verify that a queue returned by `NewOperationQueue`
  shuts down cleanly with `q.Shutdown(timeout)`.
- `SetGlobalOperationTimeout` does a type-assertion to `*OperationQueue`.
  After migration, callers hold the concrete type directly, so no
  type-assertion needed. Test that `SetOperationTimeout` works on a freshly
  constructed `*OperationQueue`.
- `go test -race`: the existing tests use `t.Parallel()` in some places.
  The rewritten tests must not share any package-level state.

## Gotchas

- `TestGlobalQueueFunctions` saves and restores `GlobalQueue` with
  `defer func() { GlobalQueue = oldQueue }()`. This is the global-dependency
  smell. The rewrite must eliminate this pattern entirely, not just wrap it
  more carefully.
- `internal/testutil/integration.go` sets `operations.GlobalQueue = queue`.
  After this migration that line is a compile error. The test runner is
  `./internal/operations/...` only, so integration.go is outside scope —
  the harness won't see that compile error. But the planner should note it
  in the plan so a human can run the follow-up.
- `main.go:33` checks `operations.GlobalQueue == nil`. After deletion this
  is also a compile error, outside scope of this run.
