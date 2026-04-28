<!-- file: docs/superpowers/specs/2026-04-28-opus-review-brief.md -->
<!-- version: 1.0.0 -->
<!-- guid: 04a5b6c7-d8e9-0123-0abc-567890123456 -->

# Opus Review Brief: Unified Maintenance System + PR Label Dependencies

**Written for:** Claude Opus fresh review session  
**Ask:** (1) Devil's advocate — alternative architectures with pros/cons. (2) Full code review of all spec and bot-task files for mistakes, ambiguities, or implementation traps.

---

## Context: What We're Building

**Repo:** `jdfalk/audiobook-organizer` — Go backend + React/TypeScript frontend, ~12K audiobooks, production on Linux.

**The problem being solved:** Every new maintenance one-off fix (fix author names, clean up series, repair file paths, etc.) requires:
- A new `func (s *Server) handlerName(c *gin.Context)` method (~100–200 lines)
- Manual route registration in `server.go`
- Manual async queue wrapping
- Manual resume wiring in `resumeInterruptedOperations()`
- A hardcoded button in the React maintenance UI

There are currently 13 of these synchronous handlers. Adding a new one takes 30+ minutes of boilerplate. None show progress, none are cancellable, none resume on restart.

**What we designed:**
1. A `MaintenanceJob` interface in `internal/maintenance/` that each fix implements
2. A global registry with `Register(job)` called from `init()` — self-registering
3. A single dispatcher `POST /api/v1/maintenance/jobs/:job_id` replaces 13 routes
4. A discovery endpoint `GET /api/v1/maintenance/jobs` drives a dynamic React UI
5. All jobs: async (202 + operation_id), progress-reporting, cancellable, resumable
6. PR label dependency system for multi-wave burndown bot execution

**The three spec files:**
- `docs/superpowers/specs/2026-04-28-unified-maintenance-system.md` — full architecture
- `docs/superpowers/specs/2026-04-28-pr-label-dependencies.md` — GitHub label dependency tracking
- `docs/superpowers/specs/2026-04-28-async-operations-design.md` — earlier draft (now superseded by unified-maintenance-system.md)

**The 19 bot-task files** in `docs/superpowers/bot-tasks/2026-04-28-async-*.md`:
- ASYNC-CORE-1: interface + registry package
- ASYNC-CORE-2: dispatcher handler + resume catch-all
- ASYNC-CORE-3: frontend API client
- ASYNC-CORE-4: dynamic React maintenance tab section
- ASYNC-W1-1..4: Wave 1 — 4 library-level fixes
- ASYNC-W2-1..4: Wave 2 — 4 file/folder fixes
- ASYNC-W3-1..5: Wave 3 — 5 complex fixes (iTunes, dedup, tag reads)
- ASYNC-CLEAN-1: remove the 13 old routes after all waves done

**Dependency graph:**
```
ASYNC-CORE-1
    ↓
ASYNC-CORE-2
    ↓ (all can run in parallel)
ASYNC-CORE-3   ASYNC-W1-1  ASYNC-W1-2  ASYNC-W1-3  ASYNC-W1-4
               ASYNC-W2-1  ASYNC-W2-2  ASYNC-W2-3  ASYNC-W2-4
               ASYNC-W3-1  ASYNC-W3-2  ASYNC-W3-3  ASYNC-W3-4  ASYNC-W3-5
    ↓
ASYNC-CORE-4
    ↓ (only after ALL above merged)
ASYNC-CLEAN-1
```

**PR label dependency system:**
- Each bot-task PR gets a GitHub label `task:ASYNC-CORE-1` etc.
- Dependent tasks check `gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length > 0'`
- No status files (they break across concurrent bots on different machines)
- GitHub API is the shared authoritative state

---

## Key Design Decisions (and the reasoning)

### 1. `init()` self-registration vs explicit wiring

Each job calls `maintenance.Register(&MyJob{})` in an `init()` function. The server
adds one blank import `_ "...internal/maintenance/jobs"` which triggers all inits.

*Why:* No per-job wiring in server.go. Adding a new job = one new file.

*Risk:* `init()` ordering between packages is not guaranteed in Go, but since all
jobs just call `maintenance.Register()` which is protected by a mutex, ordering
doesn't matter.

### 2. Store injection via `InjectStore()` interface

Jobs embedded in the registry don't have a store at init-time (server hasn't
started yet). The server calls `maintenance.InjectStore(s.store)` once after
initialization, which iterates all registered jobs and calls `InjectStore` on
those that implement `StoreInjectable`.

*Why:* Avoids global `database.GlobalStore` (which was already removed in the
DI migration, PR #280-291). Keeps jobs testable with mock stores.

*Risk:* If a job's `Run()` is called before `InjectStore()`, it panics on nil
store. The queue only starts after server init, so this shouldn't happen in
production, but tests need to inject a mock store explicitly.

### 3. `startFrom int` for resume in `Run()`

Every job's `Run(ctx, reporter, params, startFrom int)` takes a start index.
`startFrom=0` = fresh run. `startFrom=N` = resume from that index.

*Why:* Simple, no extra interface methods. Jobs checkpoint every 100 items by
writing `PhaseIndex=i` to the operation state. On resume, the dispatcher loads
the checkpoint and passes `startFrom`.

*Risk:* Resume only works if the "affected" set is deterministically ordered on
each run. All jobs sort by ID before iterating. If records were added/deleted
between crash and resume, the set changes — `startFrom=50` might skip different
records than the first run processed. This is acceptable: the job runs idempotently
so re-processing an already-fixed record is a no-op.

### 4. `json.RawMessage` params flowing through the dispatcher

The dispatcher receives raw JSON, validates it (returns 400 on failure), enqueues
the function closure that captures the raw bytes, and the job re-unmarshals inside
`Run()`. The raw params are also saved to the DB for resume.

*Why:* Type-safe per-job params without generics or reflection in the dispatcher.

*Risk:* Double unmarshal (once in ValidateParams, once in Run). Minor cost, no
correctness issue.

### 5. Keeping old routes until ASYNC-CLEAN-1

The old synchronous routes stay live throughout all wave conversions. ASYNC-CLEAN-1
removes them at the end.

*Why:* If any wave task's PR is reverted, the old route is still available as
a fallback. Zero downtime migration.

*Risk:* Two code paths for the same operation during the transition period. Both
point to the same underlying logic (moved to the job struct), so they can't
diverge. Old handlers can be refactored to call the job's Run() directly to
avoid duplication, or left in place until ASYNC-CLEAN-1.

---

## What We Need From Opus

### Task 1: Devil's Advocate

For each of the 5 design decisions above, propose at least one alternative
architecture. For each alternative, give:
- What it would look like concretely
- Pros vs. our chosen approach
- Cons vs. our chosen approach
- Your recommendation: stick with ours or switch?

Also consider: is the `MaintenanceJob` interface the right abstraction level?
Should this be a plugin/handler map? A code-generated approach? A simpler
"just add a case to a switch" approach?

### Task 2: Code Review of All Spec + Bot-Task Files

Review all files matching `docs/superpowers/bot-tasks/2026-04-28-async-*.md`
and `docs/superpowers/specs/2026-04-28-unified-maintenance-system.md`.

For each file, flag:
- **Implementation traps:** things the bot will do wrong because the spec is
  ambiguous or missing a constraint
- **Missing steps:** things the spec assumes but doesn't say explicitly
- **Go correctness issues:** wrong import paths, wrong interface method signatures,
  incorrect use of `json.RawMessage`, context handling mistakes
- **Test gaps:** important behaviors that have no test coverage specified
- **Dependency mistakes:** are the prerequisite chains correct? Is ASYNC-CLEAN-1
  correctly gated on all 13 wave tasks?

Be blunt. The goal is to find mistakes before bots execute against these specs.

### Task 3: Verdict

After your review, give a one-paragraph verdict: is this design ready for
burndown bot execution, needs minor fixes, or has a fundamental flaw that
requires a redesign?

---

## Files to Read (in order)

1. `docs/superpowers/specs/2026-04-28-unified-maintenance-system.md`
2. `docs/superpowers/specs/2026-04-28-pr-label-dependencies.md`
3. `docs/superpowers/bot-tasks/2026-04-28-async-core-1-interface.md`
4. `docs/superpowers/bot-tasks/2026-04-28-async-core-2-dispatcher.md`
5. `docs/superpowers/bot-tasks/2026-04-28-async-core-3-discovery.md`
6. `docs/superpowers/bot-tasks/2026-04-28-async-core-4-frontend.md`
7. `docs/superpowers/bot-tasks/2026-04-28-async-w1-1-fix-read-by-narrator.md`
8. Any 2-3 additional wave files to spot-check
9. `docs/superpowers/bot-tasks/2026-04-28-async-clean-1-remove-old-routes.md`

Existing codebase context you'll need:
- `internal/operations/queue.go` — the `ProgressReporter` interface and `OperationQueue`
- `internal/operations/state.go` — `OperationState`, `SaveCheckpoint`, `LoadCheckpoint`
- `internal/server/server.go` — how routes are registered, `resumeInterruptedOperations()`
- `internal/server/maintenance_fixups.go` — the 13 handlers being replaced
- `web/src/components/system/MaintenanceTab.tsx` — existing maintenance UI
