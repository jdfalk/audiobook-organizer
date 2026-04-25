<!-- file: docs/superpowers/specs/parallel-sweep.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5f6a7b8c-9d0e-1f2a-3b4c-5d6e7f8a9b0c -->
<!-- last-edited: 2026-04-25 -->

# /parallel-sweep — user spec

A coordinated multi-task refactor sweep. One git worktree per task, one child sub-agent per worktree, autonomous PR + rebase + merge. Resumable across usage limits.

This is the user-facing spec. For design rationale, see [`docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`](../plans/2026-04-24-parallel-sweep-slash-command.md). For the coordinator's procedural body, see [`.claude/skills/parallel-sweep-impl/SKILL.md`](/.claude/skills/parallel-sweep-impl/SKILL.md).

## When to use

Use `/parallel-sweep` when:

- You have **≥3 mechanically-similar refactor tasks** that can ship as independent PRs.
- The tasks touch *different* files (or non-overlapping regions of shared files). Conflicts are fine — there's a Sonnet/Opus resolver — but the lower the overlap the faster the sweep.
- The work is the kind a Haiku/Sonnet child agent can do unsupervised: well-scoped textual transforms, callsite migrations, doc-comment additions, schema bumps. Not: multi-step debugging, design judgment, anything where the right answer requires understanding the broader system.
- You want **autonomous merge** — the coordinator opens PRs, waits for CI, admin-merges on green, and rebases siblings without human input. If you want manual gates between merges, use the predecessor `parallel-refactor-sweep` user-global skill instead.

Do NOT use it for:

- 1-2 tasks (just do them yourself).
- Tasks requiring deep cross-file reasoning (the per-task isolation defeats this).
- Anything where the wrong answer is expensive to roll back (the sweep ships PRs autonomously).

## How to invoke

### Fresh run

```
/parallel-sweep
```

…with the task list as the body of the message. YAML or markdown both work; the coordinator parses both. Each task needs at minimum a slug and a one-line description; model defaults to `haiku`.

YAML example:

```yaml
- slug: audiobooks-list
  description: Migrate /api/v1/audiobooks GET handler to RespondWithData envelope.
- slug: audiobooks-detail
  description: Migrate /api/v1/audiobooks/{id} GET handler to RespondWithData.
  model: sonnet
- slug: entities-authors
  description: Migrate authors handlers (8 endpoints) to envelope.
```

Markdown example:

```markdown
- **audiobooks-list**: migrate the list endpoint to RespondWithData envelope
- **audiobooks-detail** (sonnet): migrate the detail endpoint
- **entities-authors**: migrate the 8 author endpoints
```

The coordinator parses, generates a runID like `2026-04-25-1530-envelope-wave-6`, and asks for explicit `proceed` before fanning out. **That's the only human checkpoint.** After approval, the coordinator runs autonomously through merge.

### Resume after a kill

```
/parallel-sweep --resume 2026-04-25-1530-envelope-wave-6
```

Picks up from the state file. Any task that was in flight when the previous coordinator died gets `git reset --hard origin/main` and re-dispatched from scratch. PRs from the previous run are abandoned (orphan PRs are visible on GitHub; the coordinator's Phase 6 cleanup handles them at the end of the sweep).

If the previous run's status is still `running` (not `paused`), the coordinator refuses — that's the most likely sign another coordinator is alive. Verify (`ps`, the state file's `lastCheckpointAt` timestamp), then re-invoke with `--force` if certain it's dead.

## What happens during a sweep

```
                ┌──────────────────────────────────────────────────────┐
                │ Phase 0: init or resume                              │
                │ (parse tasks, write state file, generate runID)      │
                └─────────────────────┬────────────────────────────────┘
                                      │
                                      ▼
                ┌──────────────────────────────────────────────────────┐
                │ Phase 1: fan out                                     │
                │ (one worktree per task; settings.local.json drop;    │
                │  dispatch all children in one parallel Agent call)   │
                └─────────────────────┬────────────────────────────────┘
                                      │
                                      ▼
                ┌──────────────────────────────────────────────────────┐
                │ Phase 2: watch                                       │
                │ (mirror TaskUpdate events into state.py)             │
                └─────────────────────┬────────────────────────────────┘
                                      │ child reports completed
                                      ▼
                ┌──────────────────────────────────────────────────────┐
                │ Phase 3: per-task verification + PR                  │
                │ • cross_check_isolation (load-bearing barrier)       │
                │ • run_local_ci  (= make ci in the worktree)          │
                │ • git push + gh pr create                            │
                └─────────────────────┬────────────────────────────────┘
                                      │
                                      ▼
                ┌──────────────────────────────────────────────────────┐
                │ Phase 4: merge gate                                  │
                │ • poll_ci until all checks complete                  │
                │ • admin merge ONLY when GitHub green AND local CI    │
                │   passed                                             │
                └─────────────────────┬────────────────────────────────┘
                                      │ task merged
                                      ▼
                ┌──────────────────────────────────────────────────────┐
                │ Phase 5: sibling rebase loop                         │
                │ • for each unmerged sibling: rebase_onto_main        │
                │ • CONFLICT trivial (≤30 markers, ≤3 files)           │
                │       → Sonnet resolver subagent                     │
                │ • CONFLICT non-trivial OR Sonnet uncertain           │
                │       → Opus per-commit cherry-pick fallback         │
                │ • CONFLICT unresolvable                              │
                │       → mark task rebase_blocked, escalate           │
                └─────────────────────┬────────────────────────────────┘
                                      │ all tasks terminal
                                      ▼
                ┌──────────────────────────────────────────────────────┐
                │ Phase 6: completion                                  │
                │ (consolidated CHANGELOG/TODO summary, worktree       │
                │  cleanup, terminal report)                           │
                └──────────────────────────────────────────────────────┘
```

## Hard guarantees

- **One PR per task** (rebase/FF merges only — no squash anywhere).
- **Worktree isolation enforced post-hoc.** A `git -C <worktree> status` cross-check after each child runs catches any out-of-tree write before merge. (The PreToolUse hook drop is decorative — sub-agents don't pick up project-scope hooks; see [`docs/superpowers/notes/2026-04-25-parallel-sweep-hook-spike.md`](../notes/2026-04-25-parallel-sweep-hook-spike.md).)
- **Two-gate merge.** GitHub CI green AND local `make ci` exit-zero. Either red → no merge.
- **Resumable.** Every state mutation is checkpointed atomically (`tmp + fsync + os.replace`). A SIGKILL leaves the state file in either pre- or post-checkpoint shape, never partial.
- **Children never run git push, gh, or any GitHub API call.** Coordinator owns all remote interactions. (Defect-mitigation from the predecessor sweep.)

## State file location

`.claude/state/parallel-sweep-<runID>.json` — gitignored, persists across coordinator kills, schema documented in [`.claude/skills/parallel-sweep-impl/references/state-schema.md`](/.claude/skills/parallel-sweep-impl/references/state-schema.md).

You can `cat` it during a live sweep to inspect what's happening:

```bash
jq '.tasks[] | {slug, status, prNumber}' .claude/state/parallel-sweep-2026-04-25-1530-envelope-wave-6.json
```

## Logging

The coordinator logs structured per-task and run-level events:

```
[2026-04-25-1530-envelope-wave-6] phase=fan-out — 3 task(s) dispatched
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] dispatched (haiku, agt-abc123)
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] in_progress
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] completed
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] isolation check: clean
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] make ci: pass
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] pr_opened (#451)
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] github CI: green
[2026-04-25-1530-envelope-wave-6][task=audiobooks-list] merged (1/3)
[2026-04-25-1530-envelope-wave-6] phase=sibling-rebase — 2 sibling(s) to rebase
[2026-04-25-1530-envelope-wave-6][task=audiobooks-detail] rebase: clean
[2026-04-25-1530-envelope-wave-6][task=entities-authors] rebase: conflict (1 file, 1 marker — trivial)
[2026-04-25-1530-envelope-wave-6][task=entities-authors] resolver: sonnet → success
[2026-04-25-1530-envelope-wave-6][task=entities-authors] rebase --continue: clean
```

If the format is wrong, that's a bug in the coordinator's prompt — file an issue. The structured format is what makes a long sweep diagnosable.

## Cost / time guidance

Per task on this codebase (rough order-of-magnitude from the envelope-migration sweep + step 6 spike):

| Phase | Time | Tokens |
|---|---|---|
| Child agent (Haiku, typical refactor) | 2–10 min | 30k–80k |
| Local `make ci` | 1–4 min | 0 |
| GitHub CI (with rebase + Python lint, no Go changes) | 30s–2 min | 0 |
| Admin merge | <5s | 0 |
| Sibling rebase (clean) | <1s | 0 |
| Sibling rebase + Sonnet resolver (trivial) | 5–15s | ~30k |
| Sibling rebase + Opus fallback (non-trivial) | 30s–2min per file | ~50k–150k per file |

A 10-task sweep with no conflicts: ~30 min wall, ~500k tokens, 10 PRs merged.
A 10-task sweep where every merge produces one trivial conflict on one sibling: add ~5 min, ~150k tokens.

## End-to-end smoke procedure (manual verification)

The 9-step build was unit-tested (87/87 green) and per-step empirical spikes confirmed the load-bearing pieces (PreToolUse hook scoping, Sonnet resolver behavior). The full coordinator-driven smoke — slash command → real refactor → real merges — is reserved for the first real use. To run it deliberately:

1. Pick 2–3 trivially-mergeable tasks. The classic safe choice: add a one-line doc comment to a different rarely-edited file per task. Example task list:
   ```yaml
   - slug: doc-error-handler
     description: Add a one-sentence package-level doc comment to internal/server/error_handler.go explaining the envelope helpers.
   - slug: doc-health-handlers
     description: Add a one-sentence package-level doc comment to internal/server/health_handlers.go.
   ```
2. Invoke `/parallel-sweep` with that body.
3. Observe the coordinator's structured log output. The end state should be 2 PRs merged, state file `complete`, worktrees cleaned.
4. Verify on GitHub: 2 commits added to main, each with one Co-Author trailer for the child's model.
5. Note the wall time + tokens; update this doc if they diverge significantly from the table above.

If anything goes sideways, the state file at `.claude/state/parallel-sweep-<runID>.json` has the full audit trail. `--resume <runID>` is always safe.

## Future work

Tracked in [`docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`](../plans/2026-04-24-parallel-sweep-slash-command.md) §15:

- After ~3 real sweeps, extract the universal parts of the coordinator/child/resolver prompts to `~/.claude/commands/` so other repos can use `/parallel-sweep` too. Repo-specific config (merge style, CI gate, post-merge hygiene) moves into a per-repo `.claude/parallel-sweep.local.md`.
- CHANGELOG-conflict avoidance: every step PR currently conflicts on CHANGELOG with main even when the actual code is non-overlapping. A coordinator-level "CHANGELOG batched at the end" mode would eliminate the constant rebase friction.

## Reference

- **Plan:** [`docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`](../plans/2026-04-24-parallel-sweep-slash-command.md)
- **Skill:** [`.claude/skills/parallel-sweep-impl/SKILL.md`](/.claude/skills/parallel-sweep-impl/SKILL.md)
- **Slash command:** [`.claude/commands/parallel-sweep.md`](/.claude/commands/parallel-sweep.md)
- **State schema:** [`.claude/skills/parallel-sweep-impl/references/state-schema.md`](/.claude/skills/parallel-sweep-impl/references/state-schema.md)
- **Spike notes:**
  - [`docs/superpowers/notes/2026-04-25-parallel-sweep-hook-spike.md`](../notes/2026-04-25-parallel-sweep-hook-spike.md)
  - [`docs/superpowers/notes/2026-04-25-parallel-sweep-conflict-resolver-spike.md`](../notes/2026-04-25-parallel-sweep-conflict-resolver-spike.md)
