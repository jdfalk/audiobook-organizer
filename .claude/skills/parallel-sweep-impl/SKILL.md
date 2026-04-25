---
name: parallel-sweep-impl
description: Procedural body for the /parallel-sweep slash command. Implements the coordinator that parses a task list, creates one git worktree per task, dispatches child sub-agents in parallel, watches their TaskUpdate events, opens PRs, polls CI, admin-merges on green, runs the sibling-rebase loop (with Sonnet/Opus conflict-resolver subagents), and persists per-run state to .claude/state/ for resume across usage-limit interruptions. Invoked by /parallel-sweep — not by direct user phrasing. Read this skill when /parallel-sweep is invoked, when troubleshooting an in-flight sweep, or when extending the sweep workflow.
---

# parallel-sweep-impl

Procedural body of the `/parallel-sweep` slash command. The slash command itself is a thin trigger; everything load-bearing lives here so it can be edited and reviewed independently.

The full design is in `docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`. Read that before changing this skill — it captures the why behind every decision (failure modes from prior sweeps, locked Q&A from 2026-04-24, future-work plan for universal extraction).

## What this skill does

Given a YAML/markdown task list, run a coordinated multi-agent refactor sweep:

1. Create one git worktree per task under `../worktrees/<slug>`.
2. Drop a per-worktree `.claude/settings.local.json` containing a PreToolUse hook that blocks Edit/Write outside that worktree.
3. Dispatch one child Task-tool agent per worktree, in parallel via a single tool call with multiple invocations.
4. Watch each child's TaskUpdate stream. Persist progress to `.claude/state/parallel-sweep-<runID>.json` after every event.
5. When a child finishes: run `make ci` *in the worktree* (catches test gaps GitHub Actions misses). Open PR. Poll GitHub CI. Admin-merge only when both gates are green.
6. After each merge: rebase every still-unmerged sibling onto new `main`. Trivial conflicts (≤30 markers, ≤3 files) → Sonnet conflict-resolver subagent. Non-trivial → Opus file-copy cherry-pick fallback. Unresolvable → mark `rebase_blocked` and continue with other siblings.
7. On usage-limit / SIGTERM: write a final state-file checkpoint with `status: paused` and exit. Resumable via `/parallel-sweep --resume <runID>` — the in-flight task gets `git reset --hard <base>` and re-dispatches from `pending`.

## Why this exists

`parallel-refactor-sweep` (the predecessor skill) ran the envelope-migration sweep (TODO 4.15, PRs #425-#438) successfully but exposed three failure modes:

- Agents bled edits into the main checkout (worktree isolation bypassed via absolute paths).
- Test fixture gaps caused green-CI PRs to break the suite post-merge.
- Coordinator overhead grew linearly with PR count when work was split into many small PRs.

This skill hardens against all three:

- **Worktree isolation:** PreToolUse hook + post-hoc `git -C <worktree> status` cross-check (belt-and-suspenders — the post-hoc check is the authoritative barrier).
- **Test gaps:** local `make ci` runs in each worktree before PR open; a green PR alone does not authorize merge.
- **Coordinator overhead:** state file design lets the coordinator hand off mid-flight without losing context. Conflict resolution is delegated to subagents so the coordinator stays decisive.

## State file

The single source of truth for everything: which tasks are pending, dispatched, in-flight, merged; which PRs are open; what the sibling rebase queue looks like; an audit log of every conflict-resolver invocation. Schema and lifecycle in `references/state-schema.md`. CRUD via `scripts/state.py`.

Atomicity matters because the coordinator can be SIGKILLed at any point — every mutation goes through `State.checkpoint()` which writes to a tmp file, fsyncs, and `os.replace`s atomically into place.

## Implementation status — all 9 steps complete (2026-04-25)

| Step | Status | Adds |
|---|---|---|
| 1. Skeleton + state schema | ✅ done (`b16cb0ec`) | `state.py`, `state-schema.md`, SKILL.md stub, 19 unit tests |
| 2. Coordinator + child prompts | ✅ done (`a04feb47`) | `references/coordinator-prompt.md`, `references/child-prompt.md`, `.claude/commands/parallel-sweep.md` |
| 3. PreToolUse hook spike | ✅ done (`34028e71`) | `scripts/dispatch.py`, spike report. Result: hook does NOT fire for sub-agents → post-hoc check is load-bearing. |
| 4. PR + merge loop | ✅ done (`b42196db`) | `scripts/pr_merge.py` (run_local_ci, push, open_pr, poll_ci, admin_merge, merge_task). 14 unit tests. |
| 5. Sibling rebase (clean) | ✅ done (`faa7b829`) | `scripts/rebase.py` with RebaseOutcome enum (CLEAN / UP_TO_DATE / DIRTY_TREE / FETCH_FAILED / CONFLICT). 9 unit tests. |
| 6. Conflict-resolver subagent (Sonnet) | ✅ done (`2e8a3e32`) | `references/conflict-resolver-prompt.md`, `scripts/conflict_resolver.py`, 14 unit tests, live spike. |
| 7. File-copy cherry-pick fallback (Opus) | ✅ done | `scripts/fallback.py` — per-commit cherry-pick replay (no squash). 11 unit tests. |
| 8. Resume from last completed task | ✅ done (`688b19a3`) | `scripts/resume.py` — load_for_resume / reset_in_flight / mark_resumed. 8 unit tests. |
| 9. Polish | **in progress** | Spec doc at `docs/superpowers/specs/parallel-sweep.md`, CLAUDE.md pointer, TODO 4.16 marked complete. |

**Test status:** 87/87 green. Lint clean. Coordinator-driven smoke (slash command → real refactor → real merges) deferred to first real use; the procedure is documented in the spec doc.

## Files

```
.claude/
├── commands/
│   └── parallel-sweep.md                    ← slash command (thin trigger; step 2)
└── skills/parallel-sweep-impl/
    ├── SKILL.md                             ← this file (procedural overview + roadmap)
    ├── references/
    │   ├── state-schema.md                  ← state file schema + lifecycle (step 1)
    │   ├── coordinator-prompt.md            ← coordinator role + workflow phases (step 2)
    │   ├── child-prompt.md                  ← child role + hard rules (step 2)
    │   └── conflict-resolver-prompt.md      ← Sonnet resolver role + report format (step 6)
    └── scripts/
        ├── state.py                         ← state CRUD (step 1)
        ├── test_state.py                    ← state unit tests (step 1)
        ├── dispatch.py                      ← settings render + post-hoc isolation check (step 3)
        ├── test_dispatch.py                 ← dispatch unit tests (step 3)
        ├── pr_merge.py                      ← per-task PR + 2-gate merge pipeline (step 4)
        ├── test_pr_merge.py                 ← pr_merge unit tests (step 4)
        ├── rebase.py                        ← sibling rebase loop, clean case (step 5)
        ├── test_rebase.py                   ← rebase unit tests (step 5)
        ├── conflict_resolver.py             ← Sonnet trivial-conflict path (step 6)
        ├── test_conflict_resolver.py        ← conflict_resolver unit tests (step 6)
        ├── fallback.py                      ← Opus per-commit cherry-pick fallback (step 7)
        ├── test_fallback.py                 ← fallback unit tests (step 7)
        ├── resume.py                        ← --resume support (load + reset + mark) (step 8)
        └── test_resume.py                   ← resume unit tests (step 8)
```
