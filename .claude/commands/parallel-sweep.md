---
description: Run a coordinated multi-task refactor sweep — one git worktree + one child sub-agent per task, with autonomous PR + rebase + merge. Use for ≥3 mechanically-similar refactor tasks where parallelism + autonomous merge would save real time. Resumable across usage limits via --resume.
argument-hint: [--resume <runID>] | <task list as markdown body>
allowed-tools: Bash, Read, Write, Edit, Task, Glob, Grep
---

# /parallel-sweep

You have been invoked as the **coordinator** of a `/parallel-sweep` run. You are NOT executing the user's request directly — you are dispatching child sub-agents, watching them, opening PRs, polling CI, admin-merging on green, and rebasing siblings, all autonomously.

## Step 1 — read the procedural body

The full coordinator workflow lives in:

- **`.claude/skills/parallel-sweep-impl/SKILL.md`** — high-level role + roadmap of what's currently implemented vs. not
- **`.claude/skills/parallel-sweep-impl/references/coordinator-prompt.md`** — your full operating prompt (workflow phases, hard constraints, logging format)
- **`.claude/skills/parallel-sweep-impl/references/state-schema.md`** — the state file schema and task lifecycle
- **`.claude/skills/parallel-sweep-impl/references/child-prompt.md`** — what you ask child agents to do
- **`docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`** — design rationale + locked decisions (§13) + failure-mode hardening (§8)

Read SKILL.md first. It will tell you whether the feature you need is implemented yet — this command is being built incrementally per the plan.

## Step 2 — parse arguments

Arguments are in `$ARGUMENTS`. Two shapes:

- **Fresh run:** `$ARGUMENTS` is a markdown or YAML body listing tasks. Each task needs at minimum a slug and a one-line description; model defaults to `haiku`. Generate a runID via `python3 -c "import sys; sys.path.insert(0, '.claude/skills/parallel-sweep-impl/scripts'); from state import make_run_id; print(make_run_id('<short-slug-from-prompt>'))"`.
- **Resume run:** `$ARGUMENTS` starts with `--resume <runID>`. Load that run's state file from `.claude/state/parallel-sweep-<runID>.json` and continue per the resume rules in `coordinator-prompt.md` (Phase 0 — resume mode).

If `$ARGUMENTS` is empty or unparseable, stop and ask the user what they intended.

## Step 3 — confirm scope before dispatching

Before creating any worktrees, briefly tell the user:

- The runID
- The tasks parsed (slug + one-line description each)
- The model assignments
- The expected number of PRs that will result

Wait for explicit `go` / `proceed` / equivalent before starting Phase 1 (fan-out). This is the single human checkpoint in the whole sweep — once the user approves, you operate autonomously through to merge.

## Step 4 — execute

Follow the workflow in `coordinator-prompt.md` exactly. Log meaningful events as you go (`[runID][task=<slug>] <event>` format). Persist state on every event via `state.py`. On usage-limit hit, `set_status("paused")` and exit — the run is resumable.

## Implementation status

Build state per the plan's 9 steps:

- ✅ Step 1 — skeleton + state schema (state.py + tests)
- 🚧 Step 2 — coordinator + child prompts (this PR)
- ⬜ Step 3 — PreToolUse hook spike
- ⬜ Step 4 — PR + merge loop (single-task end-to-end)
- ⬜ Step 5 — sibling rebase loop (clean)
- ⬜ Step 6 — Sonnet conflict resolver
- ⬜ Step 7 — Opus file-copy fallback
- ⬜ Step 8 — resume from last completed task
- ⬜ Step 9 — polish + spec

If the user invokes `/parallel-sweep` before steps 4-8 are done, run as far as the implemented surface allows (e.g. through step 4 = create worktrees + dispatch + open PRs but no auto-merge yet) and report what's still manual.

$ARGUMENTS
