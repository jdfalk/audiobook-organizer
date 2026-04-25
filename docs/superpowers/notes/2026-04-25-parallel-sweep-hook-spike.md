<!-- file: docs/superpowers/notes/2026-04-25-parallel-sweep-hook-spike.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2d3e4f5a-6b7c-8d9e-0f1a-2b3c4d5e6f7a -->
<!-- last-edited: 2026-04-25 -->

# Spike — does the per-worktree PreToolUse hook fire for sub-agents?

**Date:** 2026-04-25
**Step:** /parallel-sweep build, step 3
**Plan reference:** [`docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`](../plans/2026-04-24-parallel-sweep-slash-command.md) §5, §13 Q1

## Question

The plan's primary worktree-isolation defense is a `.claude/settings.local.json` file dropped into each child worktree, containing a PreToolUse hook that blocks any Edit/Write whose `file_path` doesn't begin with that worktree's absolute root. Q1 of the locked design decisions said: ship belt-and-suspenders (this hook AND a post-hoc `git status` cross-check), but spike-test in step 3 to learn whether the hook actually fires for sub-agent tool calls. If it doesn't, drop the hook and rely solely on the post-hoc check.

## Method

1. `git worktree add /tmp/parallel-sweep-spike -b spike/hook-test origin/main`
2. `python3 dispatch.py write /tmp/parallel-sweep-spike` — wrote the per-worktree settings.local.json with the hook templated to `/private/tmp/parallel-sweep-spike` (macOS resolves `/tmp` → `/private/tmp`).
3. Dispatched a `general-purpose` sub-agent via the Agent tool with an explicit two-step prompt:
   - Edit 1: `/private/tmp/parallel-sweep-spike/SPIKE_INSIDE.md` — should succeed (path inside worktree).
   - Edit 2: `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/SPIKE_OUTSIDE.md` — should be blocked by the hook (path outside worktree).
4. Recorded the agent's report in the format `EDIT_1: ... / EDIT_2: ... / HOOK_OUTPUT_OBSERVED: ...`.
5. Ran `dispatch.py check --expected /private/tmp/parallel-sweep-spike --sibling /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer` to verify the post-hoc detection.
6. Cleaned up: removed the spike worktree, deleted the spike branch, removed `SPIKE_OUTSIDE.md` from main.

## Result

**The PreToolUse hook does NOT fire for sub-agent tool calls.** Both writes succeeded silently with the standard "File created successfully" response. No `BLOCKED:` message appeared. The agent's report:

```
EDIT_1_INSIDE: succeeded
EDIT_2_OUTSIDE: succeeded
HOOK_OUTPUT_OBSERVED: no
NOTES: Both Write calls returned the standard "File created successfully" message
       with no PreToolUse hook output, indicating the worktree's project-scope
       .claude/settings.local.json hook did not fire for this sub-agent — the
       outside-the-worktree write went through unblocked.
```

The post-hoc cross-check **did** catch the violation (exit code 1, `SPIKE_OUTSIDE.md` flagged as one of the unexpected file changes in main).

## Interpretation

Sub-agents dispatched via the Agent tool inherit the *parent* session's hook configuration, not the working-directory-scoped hooks of wherever their tool calls happen to land. This makes sense from a security model standpoint — if a sub-agent could pick up arbitrary hooks just by virtue of writing to a path that has a `.claude/settings.local.json` somewhere, that would be its own privilege-escalation vector. But it means **the per-worktree hook is structurally not load-bearing** for the worktree-isolation defense.

## Decision

The post-hoc `git status` cross-check (`dispatch.cross_check_isolation`) is the **authoritative** worktree-isolation barrier. The per-worktree settings.local.json is **kept** anyway, because:

- It costs ~200 bytes per worktree to write, near-zero risk.
- If a future Claude Code release changes the sub-agent hook inheritance model (e.g. opt-in propagation), the file is already in place.
- It serves as an explicit, machine-readable declaration of the worktree's isolation intent — useful for future debugging or audit.

This matches the locked Q1 decision (belt-and-suspenders): the post-hoc check is the belt; the hook is the suspenders, currently decorative.

## Implications for the rest of the build

- Coordinator's hard constraint #5 ("post-hoc isolation check is mandatory") is upgraded from defense-in-depth to load-bearing. If the cross-check ever fails to run, the sweep loses its only worktree-isolation guarantee.
- Step 4's PR+merge loop must invoke `cross_check_isolation` immediately when the child reports `completed`, before opening the PR. A violation should mark the task `failed` with the specific files and skip merge.
- The child-prompt.md "do not edit files outside the worktree" instruction is the only *prevention* (the hook isn't preventing anything); the cross-check is *detection*. Agents must take that prompt seriously — there's no enforcement until after they've already done the damage.

## Cost

- 1 sub-agent dispatch, ~29k tokens, ~5s wall.
- Worth it once. Future spikes of this kind should be similarly cheap.
