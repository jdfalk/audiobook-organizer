---
title: Remove ActivityRecorder package global (migrate-loop dogfood)
slug: remove-activity-recorder-global
target_packages:
  - internal/operations
test_runner: "go test -race -json ./internal/operations/..."
prior_examples:
  - docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md
  - docs/superpowers/specs/2026-04-15-replace-globalstore-with-di-design.md
success_criteria:
  - All tests in internal/operations/ pass
  - Package var operations.ActivityRecorder is deleted
  - operations.NewOperationQueue (or equivalent constructor) accepts an
    ActivityLogger interface parameter and uses it to record activity entries
  - When the logger is nil, activity recording is silently skipped (matches
    pre-existing nil-check behavior in tests)
---

# Remove ActivityRecorder package global

## Goal

Eliminate the `operations.ActivityRecorder` package-level function variable
(a callback hook the Server installs at startup so the operations package can
record activity entries without importing the server package). Replace with
constructor-injected `ActivityLogger` interface.

This is **Phase 1 / Target 1 of 4** from the broader eliminate-globals spec
([2026-04-17-eliminate-remaining-globals-design.md](2026-04-17-eliminate-remaining-globals-design.md)).
We are deliberately scoping this first migrate-loop dogfood run to **just the
operations package and just the ActivityRecorder global** so a partial failure
is easy to debug and revert.

## Scope

**In scope:**
- Delete `var ActivityRecorder func(ActivityEntry)` from the operations package.
- Define a new `ActivityLogger` interface in operations (typically in a new
  `operations/activity.go`).
- Add an `ActivityLogger` parameter to whatever constructor is the right place
  (probably `NewOperationQueue`). When the parameter is nil, activity recording
  is a silent no-op.
- Replace internal calls to `ActivityRecorder(entry)` with `logger.RecordActivity(entry)`.
- Update the operations package's tests to construct queues with a
  test-friendly logger (a captured-events fake is fine).

**Out of scope:**
- Updating callers in `cmd/`, `internal/server`, or wherever `NewOperationQueue`
  is currently constructed at the application level. Those callers will fail
  to compile until they pass a logger; that's expected — they're addressed in
  the broader eliminate-globals migration. For *this* migration's success
  criteria, only `internal/operations/` tests need to pass.
- Other Phase 1 callbacks (ScanActivityRecorder, DedupOnImportHook,
  itunesActivityRecorder). Each gets its own migrate-loop run.

## Behavior — what the failing tests should exercise

### Constructor surface

There must be a constructor (likely `NewOperationQueue`) that accepts an
`ActivityLogger`. A test should verify that:

- Passing a real logger causes activity entries to flow through it during
  normal queue operations (start/complete/fail of an operation).
- Passing `nil` does not panic — the queue functions normally, just doesn't
  record activity.

### Per-operation activity recording

The queue should call `logger.RecordActivity(entry)` at the same points where
the legacy code called the package var `ActivityRecorder(entry)`. Tests should
capture activity events through a fake logger and assert on the entry shape
(operation type, status transitions, timestamps) for at least one happy path
and one failure path.

### No remaining references to the global

The package should not define or read `operations.ActivityRecorder`. A grep-
based test (or a vet rule) is fine here, but the planner should also write
behavioral tests that exercise the new constructor path so the global removal
is enforced by API shape, not just absence-of-symbol.

## Edge cases to cover

- Concurrent enqueue calls with the activity logger — the logger fake should
  be safe to call from multiple goroutines. The queue likely already uses
  goroutines for workers; tests should not introduce races (`go test -race`
  is part of the runner).
- Operations that complete vs. fail vs. retry — each should produce
  recognizable entries (or the test should document which paths emit and
  which don't, matching pre-existing behavior).
- Logger that returns an error or panics — current behavior is fire-and-forget
  (the package var was a `func(ActivityEntry)` with no error return). Preserve
  that: if `RecordActivity` panics or blocks, that is the caller's bug, not
  the queue's. Don't add error handling that wasn't there before.

## Gotchas / non-obvious

- The operations package likely has *internal* helpers that also call the
  global. Migrate them in the same change — anywhere `ActivityRecorder(...)`
  appears in the package, replace with the injected logger.
- There may be a test in operations that already sets the global directly:
  `ActivityRecorder = func(...) {...}`. That test must be rewritten to use
  the constructor injection instead. The planner should flag this in test
  setup, not silently keep the old pattern by adding an exported setter
  (which would defeat the migration).
- If the package currently does `if ActivityRecorder != nil { ActivityRecorder(e) }`,
  the new pattern is `if logger != nil { logger.RecordActivity(e) }`. Same
  shape; the only difference is who supplies the value.

## Why this is a good first migrate-loop dogfood

- **Tiny scope:** one global, one package, one test directory.
- **Well-precedented:** the broader spec already documented exactly the pattern
  (`ActivityLogger` interface + constructor injection). The planner doesn't
  need to invent a design.
- **Failure is recoverable:** if migrate-loop produces nonsense, `git worktree
  remove` and the parent audiobook-organizer repo is untouched. With PR #2 of
  migrate-loop merged, sibling worktrees are also unaffected by the hook.
- **Real test surface:** operations has actual tests, so the planner can write
  tests that fail and the coder can iteratively make them pass — exercises the
  full RED→GREEN→COVER loop.
