<!-- file: .claude/skills/parallel-sweep-impl/references/state-schema.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3f4e5d6c-7b8a-9c0d-1e2f-3a4b5c6d7e8f -->
<!-- last-edited: 2026-04-24 -->

# parallel-sweep state file schema

Every `/parallel-sweep` run owns exactly one state file at:

```
.claude/state/parallel-sweep-<runID>.json
```

The file is the single source of truth for resume, post-merge sanity, and the rebase loop. It is written atomically (`tmp + os.replace`) on every checkpoint so a SIGKILL leaves either the previous version or the new version — never a partial.

## Top-level shape

| Field | Type | Notes |
|---|---|---|
| `runID` | string | `YYYY-MM-DD-HHMM-<slug>`. Generated at create. |
| `createdAt` | ISO-8601 UTC | Set once. |
| `lastCheckpointAt` | ISO-8601 UTC | Updated on every checkpoint. |
| `status` | enum | `running` / `paused` / `complete` / `failed` |
| `userPrompt` | string | Original `/parallel-sweep` input verbatim — used for resume + audit. |
| `tasks` | array | One entry per task; ordered by user-supplied order. |
| `siblingRebaseQueue` | array of slugs | Tasks waiting to be rebased after the most recent merge. |
| `conflictResolverInvocations` | array | Audit log of every Sonnet/Opus resolver dispatch. |

## Task entry

| Field | Type | Notes |
|---|---|---|
| `slug` | string | Unique within run. Used as worktree dir name and branch suffix. |
| `description` | string | One-line task body, fed to the child agent. |
| `model` | enum | `haiku` / `sonnet` / `opus` — coordinator picks per task complexity. |
| `worktreePath` | string | Absolute path; `null` until coordinator creates it. |
| `branch` | string | `refactor/<slug>` by convention. |
| `status` | enum | See lifecycle below. |
| `agentID` | string \| null | Set when child Task agent dispatched; cleared on resume. |
| `prNumber` | int \| null | Set when PR opened. |
| `lastUpdate` | ISO-8601 UTC | Updated on any task field change. |
| `errors` | array of strings | Append-only; each entry is a one-line error summary. |

## Task lifecycle

```
pending
  └─ dispatched          (child agent spawned; agentID set)
       └─ in_progress    (first TaskUpdate received from child)
            └─ committed (child's branch has commits; child reported done)
                 └─ pr_opened       (PR created; prNumber set)
                      ├─ merged              (CI green + local make ci passed; admin-merge done)
                      ├─ rebase_blocked      (sibling rebase failed; needs user)
                      └─ failed              (any unrecoverable error)
```

States are forward-only with one exception: `--resume` may move an `in_progress` or `dispatched` task back to `pending` after `git -C <worktree> reset --hard <base>`.

## Conflict resolver invocation entry

| Field | Type | Notes |
|---|---|---|
| `task` | string | Slug of the task being rebased. |
| `triggeredBy` | string | Slug of the task that just merged (the rebase target). |
| `outcome` | enum | `resolved` / `escalated_to_fallback` / `escalated_to_user` |
| `subagentID` | string | The Task tool agent ID. |
| `model` | enum | `sonnet` (trivial path) or `opus` (file-copy fallback). |
| `markersBefore` | int | Total `<<<<<<<` count in conflicted files at dispatch. |
| `filesAffected` | int | Number of files with conflict markers. |
| `at` | ISO-8601 UTC | |

## Atomicity guarantees

- Every mutation goes through `state.checkpoint()`, which:
  1. Serializes to `<path>.tmp`.
  2. `os.fsync()` the tmp file.
  3. `os.replace(tmp, path)` — atomic on POSIX.
- The state file is therefore safe against SIGKILL at any point. After a kill, `State.load()` reads either the pre-checkpoint or post-checkpoint version, never a half-written file.
- Concurrency model: exactly one coordinator process per `runID`. No file locking needed. If a second `/parallel-sweep --resume <runID>` is invoked while another is alive, the second will see the already-`running` status and refuse — coordinator must check this on resume.

## Why JSON over SQLite

Considered SQLite for ACID and richer queries. Rejected:

- The data is small (≤100 tasks) and only one writer.
- JSON is `cat`-able and `jq`-able from shell hooks during debugging.
- Plain text diffs cleanly in `git status` if a stale state file leaks into a commit.
