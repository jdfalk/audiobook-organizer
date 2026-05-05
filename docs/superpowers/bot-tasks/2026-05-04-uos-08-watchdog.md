<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-08-watchdog.md -->
<!-- version: 1.0.0 -->
<!-- guid: 68c9d0e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e -->
<!-- last-edited: 2026-05-04 -->

# UOS-08 — Watchdog + strikes + resume orchestration

**Companion human spec:** §1.3, §2.1 (`op_strikes_v2`), §15.

## Branch

```
feat/uos-08-watchdog
```

## Goal

1. Add the watchdog goroutine described in spec §1.3.
2. Implement abandoned-goroutine accounting (Q3/B) for in-process
   ops.
3. Implement subprocess SIGTERM→SIGKILL escalation (Q3/C).
4. Implement startup resume orchestration honoring `ResumePolicy`.

## Files to add

1. `internal/operations/registry/watchdog.go`:
   - `func (r *Registry) runWatchdog(ctx context.Context)` — runs
     every 30s. Walks `r.running`. Per spec §1.3:
     - Strike for `uncheckpointed`: ResumeRestart op without
       `Reporter.Checkpoint(...)` for ≥5 consecutive minutes (since
       `last_checkpoint_at` or `started_at` if never checkpointed)
       AND `min_checkpoint_interval` is 60s (default).
     - Strike for `stuck`: any op with `last_progress_at` older than
       `progress_timeout` (5m default). Kill the run.
     - Strike for `infinite-restart`: at start of each run, check
       `resume_count`. If ≥3 and `high_water_progress` did not
       advance compared to previous run's high-water, log the strike
       and force `ResumeDrop` for this run.

2. `internal/operations/registry/abandoned.go`:
   - Tracks "abandoned" goroutine count per plugin.
   - When watchdog kills an in-process op via ctx-cancel, the goroutine
     may still be running. Mark its slot as freed (worker spawns
     replacement) and increment `abandonedCount[plugin]`.
   - When the goroutine eventually returns, decrement.
   - If `abandonedCount[plugin] >= 4` (default cap), refuse new
     dispatches for that plugin until one finishes. Surface in UI as
     "plugin X has abandoned operations; investigate before retrying."

3. `internal/operations/registry/resume.go`:
   - `func (r *Registry) resumeAfterStartup(ctx context.Context)` —
     called from `New()` (or a separate Start step). Walks
     `operations_v2` where `status IN ('queued', 'running')`:
     - For each, look up `OperationDef.ResumePolicy`.
     - Apply per spec §1.1:
       - `ResumeRestart`: increment `resume_count`, dispatch to
         worker with state from `op_state_v2`.
       - `ResumeRequeue`: clear state, dispatch fresh.
       - `ResumeDrop`: set status `interrupted_dropped`. Do not
         re-dispatch.
       - `ResumeAsk`: set status `interrupted_pending_user`. Surface
         in UI.

4. `internal/operations/registry/subprocess_kill.go` — extracted from
   UOS-03's subprocess.go:
   - `func killSubprocess(p *exec.Cmd, gracefulTimeout time.Duration)` —
     SIGTERM, wait `gracefulTimeout` (default 30s), SIGKILL if
     still alive.
   - Tied into Cancel and watchdog kills.

5. Tests:
   - `internal/operations/registry/watchdog_test.go`:
     - Op with ResumeRestart that never checkpoints accumulates
       1 strike per 5-min window.
     - Op with no progress updates for 6m is killed and gets a
       `stuck` strike.
     - Op resumed 4 times without progress advancement is force-
       dropped on the 4th attempt.
   - `internal/operations/registry/resume_test.go`:
     - ResumeDrop op left in `running` at startup ends as
       `interrupted_dropped`.
     - ResumeRequeue op left in `running` at startup re-runs
       fresh.
     - ResumeRestart op left in `running` at startup with valid
       state in `op_state_v2` resumes from that state.
     - ResumeAsk op left in `running` at startup ends as
       `interrupted_pending_user`.
   - `internal/operations/registry/abandoned_test.go`:
     - In-process op ignoring ctx is abandoned; replacement worker
       spawns; abandoned count increments; op eventually returns
       and count decrements.
     - 4 abandoned ops for a plugin block new dispatches until one
       returns.

## Files to edit

1. `internal/operations/registry/registry.go`:
   - `New(...)` calls `resumeAfterStartup` after migrations apply.
   - `Shutdown(...)` honors ResumePolicy: ops left running are
     marked appropriately, NOT just `interrupted` blanket.
2. `internal/operations/registry/worker.go`:
   - Subprocess kill path uses `subprocess_kill.go`.
   - In-process kill path increments abandoned counter, signals
     dispatcher to spawn replacement.

## Hard rules

- Strike RECORDS go in `op_strikes_v2` always. Strike-based
  quarantining (Q6/C) is NOT implemented in v1; only recording is.
- `ResumePolicy: ResumeUnspecified` was already rejected at
  registration time (UOS-02); resume code does not need to handle it.
- `ResumeAsk` runs surface in UI as a card the user can click; the
  card's "Resume" / "Drop" actions are wired in this PR via
  `POST /api/v1/operations/v2/:id/resume?action=resume|drop`.

## Acceptance criteria

- [ ] All watchdog and resume tests pass.
- [ ] `make ci` passes.
- [ ] Manual: register a fake op that ignores ctx, run it, click
      Cancel. Within `progress_timeout + grace`, the worker slot is
      freed and a new op can run.
- [ ] Manual: server restart with embed-scan in the queue (canary)
      results in correct ResumeRequeue behavior — no zombie runs.

## PR title

```
feat(uos): watchdog + strikes + startup resume orchestration
```
