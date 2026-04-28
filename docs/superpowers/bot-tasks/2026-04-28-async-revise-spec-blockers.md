<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-revise-spec-blockers.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1c2e3d4-5678-4901-23ab-cdef45678901 -->

# BOT TASK: Address Opus review BLOCKERs in unified-maintenance specs

**TODO ID:** ASYNC-REVISE
**Audience:** burndown bot
**Companion review:** [`docs/superpowers/specs/2026-04-28-opus-review-brief.md`](../specs/2026-04-28-opus-review-brief.md) (the brief Opus answered)

## Why this exists

The 18 ASYNC-CORE-* / ASYNC-W*-* / ASYNC-CLEAN-1 bot-tasks are tagged
`[hold]` in TODO.md so the burndown bot won't pick them up. The hold lifts
when **this** task is merged. Opus's review (rendered into the conversation
log of session that wrote PR #502) flagged 6 BLOCKERs and ~20 MAJORs that
will produce non-compiling Go or silently no-op destructive jobs if a bot
runs the specs as-is. This task patches the specs so the held bot-tasks
become safe to dispatch.

## Prerequisites

None.

## Branch

```
fix/async-revise-spec-blockers
```

## Label

```bash
gh label create "task:ASYNC-REVISE" --color "d73a4a" --description "Bot task: address Opus BLOCKERs in unified-maintenance specs" 2>/dev/null || true
```

## Files to modify

All under `docs/superpowers/`:

- `specs/2026-04-28-unified-maintenance-system.md`
- `bot-tasks/2026-04-28-async-core-1-interface.md`
- `bot-tasks/2026-04-28-async-core-2-dispatcher.md`
- `bot-tasks/2026-04-28-async-core-3-discovery.md`
- `bot-tasks/2026-04-28-async-core-4-frontend.md`
- `bot-tasks/2026-04-28-async-w1-3-fix-author-narrator-swap.md`
- `bot-tasks/2026-04-28-async-w1-4-fix-version-groups.md`
- `bot-tasks/2026-04-28-async-w2-2-cleanup-empty-folders.md`
- `bot-tasks/2026-04-28-async-w2-3-cleanup-organize-mess.md`
- `bot-tasks/2026-04-28-async-w2-4-fix-library-states.md`
- `bot-tasks/2026-04-28-async-w3-2-dedup-books.md`
- `bot-tasks/2026-04-28-async-w3-3-fix-book-file-paths.md`
- `bot-tasks/2026-04-28-async-w3-5-recompute-itunes-paths.md`
- `bot-tasks/2026-04-28-async-clean-1-remove-old-routes.md`

And, after the spec edits land:

- `TODO.md` — remove the `[hold]` marker from every `**ASYNC-*` line in the
  "Async Operations — Unified Maintenance System" section, and replace the
  🛑 banner with a 🟢 "ready for bot pickup, dependencies enforced via PR
  labels" line.

## What you must change

### 1. Pin every codebase-touching signature in CORE-1 + CORE-2

Before any edit to other files, **read these and paste the exact
signatures into `unified-maintenance-system.md`** under a new
"Codebase contract" section:

- `internal/operations/queue.go` — copy the `ProgressReporter` interface
  declaration and the `OperationQueue.Enqueue` method signature verbatim.
  If `EnqueueResume` exists, paste it too; if it doesn't, the spec must
  remove every reference to it.
- `internal/operations/state.go` — copy the `OperationState` struct, plus
  the signatures of `SaveCheckpoint`, `LoadCheckpoint`, `LoadRawParams`,
  `SaveRawParams`, `UpdateOperationStatus`. If a function the spec
  references doesn't exist, EITHER add it in CORE-1 (preferred) OR remove
  the reference from the spec.
- `internal/server/server.go` — confirm whether `s.Store()` is a method or
  whether code uses `s.store` directly. Pin whichever the spec uses.
- `internal/auth/` — confirm `auth.PermLibraryAdmin` exists. If the
  permission constant is named differently, fix the spec.
- `internal/server/server.go::resumeInterruptedOperations` — paste the
  function body so the bot writing CORE-2 knows whether to insert a
  `default:` case (existing `switch`) or refactor an `if/else if` chain
  to a switch first.

This step is the single largest source of compile-fail risk. If the
signatures aren't in the spec, the bots will invent them.

### 2. CORE-1 / CORE-2 BLOCKERs

In `bot-tasks/2026-04-28-async-core-2-dispatcher.md`:

- **Body parsing.** Replace `c.ShouldBindJSON(&raw)` (where `raw` is a
  `json.RawMessage`) with:

  ```go
  body, err := io.ReadAll(c.Request.Body)
  if err != nil { c.JSON(400, gin.H{"error": err.Error()}); return }
  if !json.Valid(body) { c.JSON(400, gin.H{"error": "invalid JSON"}); return }
  raw := json.RawMessage(body)
  ```

  Add `io` to the imports.

- **`SaveRawParams` ordering.** Change the dispatcher so `SaveRawParams`
  runs *before* `s.queue.Enqueue(...)`, not after. A worker that picks up
  the operation, crashes, and resumes before the params row is written
  will hand `Run` an empty params blob. If `SaveRawParams` can be folded
  into `Enqueue`, do that instead and document it.

- **Resume re-validation.** Resume catch-all in
  `resumeInterruptedOperations` must call `job.ValidateParams(raw)` after
  `LoadRawParams` and before `Run`. A corrupted DB row should fail with a
  clean error, not panic inside `Run`.

- **Concurrent-run dedup.** Add a per-`job_id` mutex check (or queue-level
  dedup) at the top of the dispatcher. Two POSTs of the same `job_id`
  must NOT enqueue two operations writing the same DB rows. Spec the
  rejection: HTTP 409 + `{operation_id: "<existing>"}`.

- **Cancellation channel.** The interface takes `ctx context.Context`.
  Decide once: either (a) drop `ctx` from the interface and document
  `IsCanceled()` as canonical, OR (b) keep `ctx`, drop `IsCanceled()`,
  and require every job's main loop to select on `ctx.Done()`. Pick (b)
  — taglib reads, `os.Stat` calls, and `filepath.Walk` ignore
  `IsCanceled()` polling. Update CORE-1 + every wave spec to match.

- **Cancel sentinel.** Jobs returning `nil` on cancel makes "cancelled
  clean" indistinguishable from "completed". Require `return ctx.Err()`
  (= `context.Canceled` or `context.DeadlineExceeded`) on cancel
  paths. Update CORE-1 + every wave spec.

- **`default:` insertion.** Read `resumeInterruptedOperations` first.
  If the dispatch is a `switch opType`, add the `default:` case as
  written. If it's an `if/else if` chain, refactor it to a `switch`
  first (as a mechanical pass with no behavior change), then add the
  `default:`. Document which path was taken.

In `bot-tasks/2026-04-28-async-core-1-interface.md`:

- Add a `Reset()` (test-only, via `export_test.go`) so registry-mutating
  tests can clean up after themselves. Without this, `t.Parallel` will
  race and table tests leak across iterations.
- Add a `Register` policy test: `Register(nil)` panics; duplicate ID
  panics with a specific message. Pin it.
- Add an `ID()` validator: regex `^[a-z0-9-]+$`, panics on
  `Register` if invalid. Otherwise `ID() = "../etc"` becomes a path
  segment.
- Add an `InjectStore` test: a job that implements `StoreInjectable`
  receives the store; one that doesn't is left alone (no panic). Right
  now the entire DI mechanism has zero coverage.

In `bot-tasks/2026-04-28-async-core-3-discovery.md`:

- Frontend `fetch` calls must include `credentials: 'include'` (or the
  project's existing auth-header helper — read the file first to find
  it). Bare `fetch` will 401 in production.
- `default_params` may be `nil` from Go. Frontend must treat null as `{}`,
  not `unknown`. Spec the coercion.

In `bot-tasks/2026-04-28-async-core-4-frontend.md`:

- POST body building: only include `dry_run` when the job declares it.
  Build the body from `job.default_params` overlaid with user toggles —
  do not always send `{ dry_run }`.
- Verify `useOperationsStore.startPolling` actually accepts
  `(opId, jobName)` — read the file. If the signature differs, fix the
  call site.
- Add error / loading states to the dynamic section. Right now
  `.catch(() => {})` swallows auth errors silently.

### 3. Resume-correctness BLOCKERs

In `unified-maintenance-system.md` and every wave bot-task:

- **`startFrom int` is unsafe for jobs that mutate their iteration set.**
  Jobs that delete (cleanup-empty-folders, cleanup-organize-mess,
  dedup-books) or merge (cleanup-series, fix-version-groups) shrink
  `affected` between runs, so `startFrom=N` after restart points at a
  different record.

  Fix one of two ways:
  - **Preferred**: switch to opaque cursor — add
    `Resume(ctx, reporter, params, cursor []byte) error` and a
    `reporter.Checkpoint(cursor []byte)` that persists opaque state.
    Each job decides what the cursor means (last-processed ID, page
    token).
  - **Acceptable interim**: every wave spec MUST add
    `sort.Slice(targets, byID)` before iterating, and the resume path
    MUST re-derive `affected` and skip already-processed items by ID,
    not by index.

- **Phase resume off-by-one in W1-2 + W1-4.** Cumulative `PhaseIndex`
  underflows on resume because phase-1 length shrinks. Add a `Phase int`
  field to `OperationState` (or a separate checkpoint key per phase)
  and update both wave specs to checkpoint phase explicitly.

### 4. Wave specs with placeholder business logic

These ship `// placeholder` / `return false` / `_ = g` stubs in the
core code path. The bot will compile + green-test a no-op PR, and on
destructive paths (W2-4 library states, W3-2 dedup) silently destroy
data. For each, replace the placeholder with the actual heuristic
ported from `internal/server/maintenance_fixups.go`:

- `bot-tasks/2026-04-28-async-w1-3-fix-author-narrator-swap.md`:
  port `detectAuthorNarratorSwap` body. Cite the source line range.
- `bot-tasks/2026-04-28-async-w1-4-fix-version-groups.md`:
  populate phase-1 + phase-2 target slices. Cite source line range.
- `bot-tasks/2026-04-28-async-w2-3-cleanup-organize-mess.md`:
  replace the macOS-only pattern list with the real organizer pattern
  list (`.bak-*`, `partial.*`, `*.tmp`, etc.). Cite source.
- `bot-tasks/2026-04-28-async-w2-4-fix-library-states.md`:
  replace the hardcoded `"present"|"missing"` enum with the actual
  state set. Add a respect-`isProtectedPath` check (MEMORY.md gotcha).
- `bot-tasks/2026-04-28-async-w3-2-dedup-books.md`:
  the most dangerous spec. Replace `isJunkBook=return false` and the
  phase 2/3/4 placeholders with real logic. ALSO add: (a) audit-trail
  rows via `book_path_history` + activity-log, (b) explicit Phase 3
  keep-rule (file count? bitrate? iTunes-linked?), (c) skip-iTunes-PID
  guard (purge gotcha — `external_id_map` has 97K mappings that depend
  on the books staying alive). DO NOT MERGE this without (a)+(b)+(c).

### 5. Other wave-specific bugs

- `bot-tasks/2026-04-28-async-w3-3-fix-book-file-paths.md`:
  `filepath.Glob` does NOT support `**`. Replace with
  `filepath.WalkDir` recursive search. Add `isProtectedPath` check.
  Reference + reconcile with the merged path-repair work in commit
  `a9bdb56d`.
- `bot-tasks/2026-04-28-async-w3-5-recompute-itunes-paths.md`:
  consult `itunes_path_trim_enabled` setting (recently added) before
  assigning `b.ItunesPath`. Resolve via `external_id_map` first; only
  fall back to the track scan for unmapped books. Normalize paths
  (case-fold, trailing slash, `\ ` ↔ `/`) before keying the lookup map.
- `bot-tasks/2026-04-28-async-w3-1-enrich-book-files.md`:
  duration enrichment must actually call the duration probe (taglib
  `Length` or ffprobe). Filter currently includes `Duration == 0` but
  loop never sets it.
- `bot-tasks/2026-04-28-async-w3-4-refetch-missing-authors.md`:
  taglib import path is `go.senan.xyz/taglib`, not `internal/taglib`
  (per MEMORY.md). Verify before bot ships.

### 6. CLEAN-1 gating

In `bot-tasks/2026-04-28-async-clean-1-remove-old-routes.md`:

- Gating must verify (a) all 13 wave PRs `--state merged --base main`,
  (b) `CORE-3` and `CORE-4` are merged (FE depends on dynamic tab),
  AND (c) every expected job ID exists in `internal/maintenance/jobs/`
  on `main`. Add a shell check:

  ```bash
  for id in fix-read-by-narrator cleanup-series fix-author-narrator-swap \
            fix-version-groups backfill-book-files cleanup-empty-folders \
            cleanup-organize-mess fix-library-states enrich-book-files \
            dedup-books fix-book-file-paths refetch-missing-authors \
            recompute-itunes-paths; do
    grep -q "\"$id\"" internal/maintenance/jobs/*.go || {
      echo "missing job $id in registry"; exit 1
    }
  done
  ```

- Verify dispatcher returns exactly 13 jobs and **fail** the run on
  `< 13`, not just print.
- Remove `scan-composer-tags` from the keep-list — it was already
  converted to async ops in commit `2f588b23`.

### 7. Architectural simplifications

These are not BLOCKERs but they remove whole classes of bug. Apply if
the diff stays small; otherwise document as a follow-up:

- Replace `init()` self-registration + `InjectStore` with explicit
  constructor DI (`registerMaintenanceJobs(store)` in
  `cmd/server/main.go`). Eliminates the InjectStore race and the
  missing-blank-import footgun.
- Replace `c.ShouldBindJSON(&json.RawMessage{})` with `io.ReadAll`
  (BLOCKER — already covered above).
- Wrap `Job` in a generic decoder factory (`Job[P any]` + erased
  `runErased(raw json.RawMessage)`) so params get one decode at the
  edge instead of two (in `ValidateParams` + in `Run`). Optional.
- Cutover-per-wave instead of "keep old routes until CLEAN-1": each
  wave PR removes the route it replaces. Optional.

### 8. Lift the [hold] marker

Final commit on this branch:

```
sed -i.bak 's/- \[ \] \[hold\] \*\*ASYNC-/- [ ] **ASYNC-/g' TODO.md
rm TODO.md.bak
```

Replace the 🛑 banner block (lines ~187–201 of TODO.md, see git log
of `chore(todo): tag ASYNC bot-tasks [hold]`) with:

```
> 🟢 **READY FOR BOT PICKUP — dependencies enforced via PR labels.**
> All Opus review BLOCKERs addressed in PR #<this-PR-number>. Bot tasks
> may be dispatched in dependency order: CORE-1 → CORE-2 → (CORE-3 +
> all W*) in parallel → CORE-4 → CLEAN-1.
```

## Definition of Done

- [ ] All 14 spec / bot-task files modified to address the items above.
- [ ] Spec includes a "Codebase contract" section with verbatim
  signatures from `internal/operations/`, `internal/database/`,
  `internal/auth/`, and `internal/server/server.go`.
- [ ] No remaining `// placeholder` or `return false  // placeholder`
  in any wave bot-task's code blocks.
- [ ] `grep -nR '\*\*-glob' docs/superpowers/bot-tasks/` returns empty
  (W3-3 fixed).
- [ ] `grep -nR '\[hold\] \*\*ASYNC-' TODO.md` returns empty.
- [ ] PR description summarizes which BLOCKERs were addressed and
  which alternatives (item 7) were taken vs deferred.

## Out of scope

- Implementing the maintenance jobs themselves (that's CORE-1..4 +
  W1..W3 + CLEAN-1; this task only fixes the specs).
- Any code changes to `internal/server/maintenance_fixups.go`.
- Architectural alternative #4 (generics) and #5 (cutover-per-wave)
  are optional; mark deferred if the bot judges the diff too large.
