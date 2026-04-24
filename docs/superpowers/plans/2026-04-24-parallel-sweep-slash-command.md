<!-- file: docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md -->
<!-- version: 1.1.0 -->
<!-- guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d -->
<!-- last-edited: 2026-04-24 -->

# /parallel-sweep — repo-scoped slash command

Author: Claude Opus 4.7 (planning)
Status: **APPROVED — open questions resolved 2026-04-24; implementation begins at step 1**
Successor of: `parallel-refactor-sweep` skill (lessons baked in)

## 1. Goal

A repo-scoped slash command `/parallel-sweep` that takes a list of independent refactor tasks, spawns isolated worktrees per task, dispatches a sub-agent per worktree, and drives the whole sweep through PR + merge + sibling-rebase without human intervention beyond approval gates. Resumable across usage-limit interruptions.

It hardens the `parallel-refactor-sweep` pattern (proven on TODO 4.15) against three failure modes seen in recent sessions:

- agents bleeding edits into the main checkout (worktree isolation bypass via absolute paths)
- silent gaps in test coverage breaking the suite post-merge (the SQLite schema + `.data` unwrap class)
- coordinator overhead blowing up when work is split into many small PRs

## 2. End-state design

User invokes:
```
/parallel-sweep
```
…with a YAML or markdown body listing tasks. The slash command:

1. Parses the task list.
2. Loads (or creates) a state file under `.claude/state/parallel-sweep-<runID>.json` keyed by run ID.
3. Spawns a **coordinator agent** (Opus) via the Task tool. The coordinator:
   - Creates one worktree per task under `../worktrees/<task-slug>`.
   - Drops a `.claude/settings.local.json` in each worktree with a PreToolUse hook that blocks any Edit/Write whose `file_path` doesn't begin with that worktree's absolute path. (Hard guardrail against defect 1.)
   - Dispatches one child `Task` agent (Haiku/Sonnet, model chosen per task) per worktree, in parallel via a single Task-tool call with multiple invocations.
   - Watches `TaskUpdate` events; persists progress to the state file after each event.
   - When a child reports done: runs full-suite verification (the project's `make ci` or equivalent), opens a PR, watches CI, admin-merges on green.
   - After each merge: rebases sibling worktree branches onto main. On trivial conflicts, dispatches a **conflict-resolver subagent** (Sonnet) with a tight scope ("resolve only the textual conflict markers; preserve both intents"). On non-trivial divergence, falls back to a **file-copy cherry-pick** path (cherry-pick aborted → identify changed files → copy them across with `git checkout <sha> -- <path>` and re-test).
   - On usage-limit / context exhaustion: writes a final state-file checkpoint with `status: paused` and exits cleanly. Resumable on next `/parallel-sweep --resume <runID>`.

## 3. Architecture

```
.claude/
├── commands/
│   └── parallel-sweep.md         ← the slash command (frontmatter + prompt)
├── skills/
│   └── parallel-sweep-impl/      ← the procedural detail the slash command points at
│       ├── SKILL.md
│       └── references/
│           ├── coordinator-prompt.md
│           ├── child-prompt.md
│           ├── conflict-resolver-prompt.md
│           └── state-schema.md
├── state/
│   └── parallel-sweep-<runID>.json    ← gitignored; per-run state
└── settings.local.json           ← gitignored; injected per-worktree by coordinator
```

The slash command itself is thin (~30 lines). The procedural meat lives in the skill so it can be edited and reviewed independently of the trigger.

## 4. State file schema

```json
{
  "runID": "2026-04-24-1530-batch-api-migration",
  "createdAt": "...",
  "lastCheckpointAt": "...",
  "status": "running | paused | complete | failed",
  "userPrompt": "<original /parallel-sweep input>",
  "tasks": [
    {
      "slug": "audiobooks-handlers",
      "description": "...",
      "model": "haiku",
      "worktreePath": "../worktrees/audiobooks-handlers",
      "branch": "refactor/audiobooks-handlers",
      "status": "pending | dispatched | in_progress | committed | pr_opened | merged | failed | rebase_blocked",
      "agentID": "...",
      "prNumber": 442,
      "lastUpdate": "...",
      "errors": []
    }
  ],
  "siblingRebaseQueue": ["audiobooks-handlers", "..."],
  "conflictResolverInvocations": [
    {"task": "...", "outcome": "resolved | escalated", "subagentID": "..."}
  ]
}
```

## 5. The PreToolUse worktree-isolation hook

This is the single most important hardening. Every child worktree gets a `.claude/settings.local.json` containing:

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Edit|Write",
      "hooks": [{
        "type": "command",
        "command": "file=$(echo \"$TOOL_INPUT\" | jq -r '.file_path // empty'); root=\"<ABSOLUTE_WORKTREE_PATH>\"; [[ \"$file\" == \"$root\"/* || -z \"$file\" ]] && exit 0; echo \"BLOCKED: $file is outside this worktree ($root)\" >&2; exit 1"
      }]
    }]
  }
}
```

The coordinator templates `<ABSOLUTE_WORKTREE_PATH>` to the worktree's `pwd` before writing the file. If a child agent tries to Edit a file in main (or another sibling worktree), the hook rejects the call.

**Caveat to verify:** sub-agents may not inherit the host's hook config. Spike test in implementation: dispatch a Haiku agent in a worktree with this `.claude/settings.local.json` and try to Edit a file outside the tree. If sub-agents bypass project-scope hooks, fall back to wrapping the agent's prompt with the absolute path constraint and post-hoc verification (compare the agent's reported edits against `git status` in each worktree before consolidating).

## 6. CI-aware merge

Per project convention (rebase/FF only, no squash) and learned during TODO 4.15:

- `gh pr merge <n> --rebase` only after CI is green (`gh pr checks <n> --watch` or polling `--json statusCheckRollup`).
- If CI is failing because of pre-existing unrelated breakage on main: `--admin` is acceptable for refactor PRs. Otherwise the merge must wait or escalate.
- Polling loop has a wall-clock cap (default 30 min); on timeout, mark task as `pr_opened` in state file and continue with siblings.

## 7. Sibling rebase + conflict resolution

After merge of task A:
1. For each sibling task B still unmerged: `cd <B's worktree> && git fetch origin main && git rebase origin/main`.
2. If clean: log success, continue to next sibling.
3. If conflict markers in 1–3 small files (heuristic: total `<<<<<<<` count < 30): dispatch **conflict-resolver subagent** (Sonnet, tight prompt: "Resolve conflicts in these files; preserve both intents; if unsure, write a comment and exit 1"). Then `git add -u && git rebase --continue`. If the subagent exited 1 or another conflict surfaces, escalate.
4. If broader conflict or rebase aborted: **file-copy cherry-pick** path:
   - `git rebase --abort`
   - Identify the original feature commits on B's branch via `git log main..HEAD --oneline`.
   - For each commit, identify changed files via `git show --stat`.
   - `git checkout main -- <those files>`, then re-apply B's changes by reading from a stashed copy of the pre-rebase tree, with the conflict-resolver subagent reconciling each file individually.
   - Mark task as `rebase_blocked` in state if the fallback also fails — escalate to user.

## 8. Failure-mode hardening (per session learnings)

| Failure | Mitigation |
|---|---|
| Agent edits files in main (defect 1 from `parallel-refactor-sweep`) | Per-worktree PreToolUse hook (see §5) + post-hoc `git status` cross-check before consolidating |
| Sub-agent can't run git/gh (defect 2) | Coordinator owns ALL git/gh; child agents report-only |
| Tests grep by handler name miss URL-decoded callers (defect 3) | Coordinator runs `make ci` in each worktree before opening PR — surfaces missing test updates BEFORE merge |
| `.data` unwraps missed in service layer (Wave 5 fallout) | Each child's verification step includes a `tsc --noEmit` (frontend) and a project-level test script. State file records which verifications passed. |
| SQLite schema gap breaking post-merge tests | A coordinator-level "post-merge sanity" job runs the full project test suite on main after each PR merges; if it fails, the next sibling is held until the user resolves. |
| Coordinator hits usage limit mid-flight | Checkpoint state file after every TaskUpdate; resume on `/parallel-sweep --resume <runID>` |

## 9. File-by-file deliverables

### New files

| Path | Purpose | Approx LOC |
|---|---|---|
| `.claude/commands/parallel-sweep.md` | The slash command frontmatter + thin prompt that invokes the skill | ~40 |
| `.claude/skills/parallel-sweep-impl/SKILL.md` | Procedural body: argument parsing, dispatch loop, merge/rebase loop, resume logic | ~300 |
| `.claude/skills/parallel-sweep-impl/references/coordinator-prompt.md` | The full system-style prompt for the coordinator agent | ~200 |
| `.claude/skills/parallel-sweep-impl/references/child-prompt.md` | The full prompt for child task agents (templated per task) | ~150 |
| `.claude/skills/parallel-sweep-impl/references/conflict-resolver-prompt.md` | Tight prompt for the conflict-resolver subagent | ~80 |
| `.claude/skills/parallel-sweep-impl/references/state-schema.md` | JSON schema doc + lifecycle diagram for the state file | ~100 |
| `.claude/skills/parallel-sweep-impl/scripts/dispatch.py` | Optional: helper that emits the worktree settings.local.json + computes paths | ~120 |
| `.claude/skills/parallel-sweep-impl/scripts/state.py` | State file CRUD helpers | ~80 |
| `docs/superpowers/specs/parallel-sweep.md` | Public-facing spec / user docs (when to use, args, examples) | ~150 |

### Modified files

| Path | Change |
|---|---|
| `.gitignore` | Add `.claude/state/` and ensure `.claude/settings.local.json` is ignored repo-wide |
| `CLAUDE.md` | One-line pointer in Workflow Discipline: "For multi-task parallel sweeps, use `/parallel-sweep`." |
| `TODO.md` | Add `4.16: /parallel-sweep slash command` as `[~]`-in-progress, mark `[x]` after merge |

## 10. Implementation order

Each step is its own commit. Each step's deliverables compile/run on their own.

1. **Skeleton + state schema.** `parallel-sweep-impl/SKILL.md` stub, `state-schema.md`, `state.py` with create/read/update/checkpoint, unit tests for state.py. Worktree blocks on agent dispatch — no actual sweep yet, just plumbing.

2. **Coordinator prompt + dispatch.** Write `coordinator-prompt.md` and `child-prompt.md`. Wire the slash command to invoke the coordinator with a synthetic 1-task input. Verify the coordinator creates a worktree, drops `.claude/settings.local.json`, dispatches a child Haiku, and the child reports back.

3. **PreToolUse hook spike.** Verify that the per-worktree hook actually blocks out-of-tree edits when a sub-agent runs there. If sub-agents bypass project hooks, switch to the prompt-constraint + post-hoc `git status` cross-check fallback before continuing.

4. **PR + merge loop.** Coordinator opens a PR after child completes; polls CI; admin-merges on green. Single-task end-to-end smoke.

5. **Sibling rebase loop (clean case only).** Two tasks; merge first; rebase second cleanly; merge second.

6. **Conflict resolver subagent.** Synthetic conflict between two tasks; verify the resolver subagent fixes simple textual conflicts and the rebase continues.

7. **File-copy cherry-pick fallback.** Force a non-trivial conflict; verify fallback works.

8. **Resume.** Kill the coordinator mid-sweep (SIGTERM); verify `/parallel-sweep --resume <runID>` reads state and continues from `lastCheckpointAt`.

9. **Polish.** Spec doc, CLAUDE.md pointer, TODO.md entry, gitignore changes. Final verification: re-run a known-good sweep (e.g., a small mechanical refactor) end-to-end on this repo.

## 11. Test strategy

- Unit tests for `state.py` (create / load / update / corruption recovery / concurrent-checkpoint safety): pytest, run via `python3 -m pytest`.
- Unit tests for `dispatch.py` path-templating logic.
- **Integration test 1 (clean sweep)**: 2 synthetic refactor tasks on a throwaway test repo. Expected: 2 PRs, both merged in order, state file shows `complete`.
- **Integration test 2 (conflict)**: 2 tasks that touch the same file. Expected: first merges; second hits conflict; resolver subagent invoked; merges.
- **Integration test 3 (resume)**: kill coordinator after first merge; resume; expected: continues to second task without re-running first.
- **Integration test 4 (out-of-tree write)**: child agent prompt instructs it to also edit a file in main. Expected: hook blocks the Edit; child reports failure.
- Smoke: real end-to-end sweep on this repo with a 2-file mechanical refactor that's pre-known-clean.

## 12. Rollback

The slash command is purely additive. Rollback = `git revert` the merging PR.

In-flight rollback during a live sweep: coordinator can be killed at any checkpoint; state file persists; orphan worktrees can be cleaned with `git worktree prune` + `git branch -D`. Document this explicitly in `parallel-sweep-impl/SKILL.md`.

## 13. Resolved design decisions (2026-04-24)

1. **Hook scoping → belt-and-suspenders.** Drop the per-worktree `.claude/settings.local.json` PreToolUse hook AND run a post-hoc `git -C <worktree> status` cross-check after each child completes. The post-hoc check is the authoritative barrier; the hook is defense in depth. Step 3 spike confirms whether the hook actually fires for sub-agents — if not, delete the hook and rely on the post-hoc check alone.

2. **Auto-merge policy → green PR + local `make ci` both required.** Coordinator admin-merges only when (a) GitHub CI is green AND (b) `make ci` passed in the worktree before PR open. On disagreement: GitHub CI is authoritative — if remote red, don't merge. If local red but remote green, also don't merge (local is authoritative on coverage/tests not run by the workflow).

3. **Resume granularity → last completed task.** Re-dispatching an in-flight task starts from a clean known state: `git -C <worktree> reset --hard <base>` before re-spawn. One code path; no special-case logic for partial state. Worst-case cost: one task's worth of agent work re-done per kill.

4. **Conflict resolver model → Sonnet trivial, Opus fallback.**
   - Trivial path (≤30 markers, ≤3 files): Sonnet conflict-resolver subagent.
   - Non-trivial path (file-copy cherry-pick): Opus directly. No speculative Sonnet pass — the heuristic that triggers the fallback already says Sonnet would struggle.

5. **Slash command scope → project first, user-global later.** Ship in `.claude/commands/parallel-sweep.md` for this repo. After ~3 real sweeps, extract the universal parts (coordinator logic, state schema, rebase loop) to `~/.claude/commands/` and leave repo-specific bits (rebase/FF merge, `make ci` 80% gate, taglib deps) as configurable. Tracked as future work — see §15.

## 14. Definition of done

- `/parallel-sweep <task-list>` end-to-end ships 2-task synthetic sweeps on this repo with all integration tests passing.
- Documented in `docs/superpowers/specs/parallel-sweep.md`.
- TODO 4.16 marked `[x]`.
- A morning after first real-world use produces no "agent worked on main" or "missed test fixture" surprises.

## 15. Future work — universal extraction

Once the project-scoped command has run cleanly through ~3 real sweeps:

- Extract coordinator/child/conflict-resolver prompts and the state schema into a generic `~/.claude/commands/parallel-sweep.md` + `~/.claude/skills/parallel-sweep-impl/`.
- Move repo-specific knobs (merge style, CI gate command, post-merge hygiene targets) into a per-repo `.claude/parallel-sweep.local.md` config file (YAML frontmatter), discoverable from `${CLAUDE_PROJECT_DIR}`.
- Default config falls back to safe-but-conservative behavior (no auto-merge, prompt before each merge, single-task fallback) when no local config is present.
- Publish as a standalone skill in the `superpowers` plugin so other repos pick it up via plugin install.
