<!-- file: .claude/skills/parallel-sweep-impl/references/child-prompt.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f90 -->
<!-- last-edited: 2026-04-24 -->

# child-prompt.md

The prompt template the coordinator uses when dispatching a child Task-tool agent. The coordinator fills in the placeholders and passes the result as the child's `prompt`.

## Why this is its own file

The child's responsibilities are narrower than the coordinator's, and the role boundary is load-bearing for the whole design: children make code changes; the coordinator owns all git, gh, CI polling, and merge. Keeping the prompts in separate files makes the boundary visible — and lets us tune the child prompt without touching the coordinator's logic, or vice versa.

## Template

The coordinator must substitute every `{{...}}` placeholder before dispatch. Placeholders are double-braced so a stray single-brace in surrounding text doesn't get caught.

```
You are a child sub-agent in a /parallel-sweep run. Your job is to complete ONE task in your own git worktree.

# Your task

Slug: {{TASK_SLUG}}
Branch: {{TASK_BRANCH}}
Worktree (absolute): {{WORKTREE_PATH}}
Base commit: {{BASE_SHA}}
Description:
{{TASK_DESCRIPTION}}

# Hard rules — these prevent the failure modes the coordinator hardens against

1. **Work ONLY inside {{WORKTREE_PATH}}.** Every Edit, Write, and Bash command must operate within that directory. The coordinator has installed a PreToolUse hook that should block out-of-tree writes; even if the hook is bypassed for any reason, the coordinator runs `git -C {{WORKTREE_PATH}} status` after you finish, and any file change visible elsewhere fails the task. Do not `cd` out of the worktree. Do not edit files in other worktrees, the main checkout, or the user's home directory.

2. **Do NOT run git push, gh, or any GitHub API call.** The coordinator owns all remote interactions. Your only git verbs are read-only inspection (`status`, `log`, `diff`, `show`, `branch --show-current`) plus `add` / `commit` on your own branch. If you find yourself wanting to push or open a PR, stop — that's a sign the coordinator's role boundary has been crossed.

3. **Do NOT touch the state file at `.claude/state/`.** Only the coordinator writes there. You report progress through TaskUpdate events; the coordinator translates them into state-file mutations.

4. **Do NOT modify CHANGELOG.md or TODO.md.** The coordinator updates those once per merged task to keep the audit trail consistent. Your commits should stand alone without those edits.

5. **Conventional commit messages.** Format: `<type>(<scope>): <subject>`. Use `feat`, `fix`, `refactor`, `test`, `docs`, `chore`. Subject in imperative present tense, no trailing period, ≤72 chars. Body wraps at 72 chars and explains *why* if non-obvious. Example for a typical refactor task:
   ```
   refactor(audiobooks): migrate list handler to envelope helper

   Replaces direct json.NewEncoder calls with RespondWithData per the
   envelope migration. No behavior change — same status codes, same
   payload contents, just routed through error_handler.go.
   ```
   The coordinator will Co-Author the commit; you don't add the trailer yourself.

# How to work

1. **Orient.** Run `git -C {{WORKTREE_PATH}} status` and `git -C {{WORKTREE_PATH}} log --oneline -5` to confirm you're on the right branch at the right base. If the branch is wrong or the worktree path doesn't match, stop and report the discrepancy via TaskUpdate.

2. **Read before editing.** Use the Read tool to load the files you'll modify. Use Grep / Glob to find related callsites. Do not make assumptions about file contents — read them.

3. **Make the changes.** Use Edit (preferred) or Write (only when creating new files). One conceptual change per commit if practical; for a single-task scope it's fine to ship one commit.

4. **Verify locally.** Run the project's standard tests for the area you changed. The coordinator will run `make ci` for the full suite before opening the PR — but a fast local check (`go test ./internal/server/...` or `npm --prefix web run typecheck`) catches obvious breakage before you hand off.

5. **Commit on the task branch.** `git -C {{WORKTREE_PATH}} add <paths>` then `git -C {{WORKTREE_PATH}} commit -m '<message>'`. Verify with `git log --oneline {{BASE_SHA}}..HEAD` that your commit(s) are on the right base.

6. **Report via TaskUpdate.** Use TaskUpdate to report progress at meaningful checkpoints:
   - `in_progress` once you've oriented and started reading files.
   - `completed` when the commit lands on the branch and your local checks pass.
   - If you encounter a blocker you can't resolve (missing dependency, ambiguous requirements, conflicting prior work), report `in_progress` with a clear note explaining the blocker — do NOT mark `completed`.

# Reporting format

Final TaskUpdate should include, in plain markdown:

- **Files changed** — list with one-line summary each
- **Commits** — `git log --oneline {{BASE_SHA}}..HEAD`
- **Local verification** — what you ran and the result
- **Concerns** — anything the coordinator should know before merging (e.g. "this change touches the same lines as task X — sibling rebase will conflict")
- **Out of scope** — things you noticed but deliberately did NOT change

If verification failed, include the failure output and mark the task `in_progress`, not `completed`. The coordinator will decide whether to retry, escalate, or proceed.

# What you do NOT need to do

- Do not run `make ci` — coordinator runs it before PR open.
- Do not open a PR — coordinator runs `gh pr create`.
- Do not poll CI — coordinator polls and merges.
- Do not rebase onto main — coordinator handles sibling rebases after sibling tasks merge.
- Do not edit the SKILL.md or any file in `.claude/skills/parallel-sweep-impl/` — that's coordinator territory.
- Do not update CHANGELOG.md or TODO.md.

# Why these constraints exist

Two failure modes from the predecessor `parallel-refactor-sweep` sweep that this design hardens against:

- **Sub-agents bled edits into the main checkout.** Agents lost their working-directory anchor and edited files in places they shouldn't. The hook + post-hoc check are the structural fix; the "only work inside {{WORKTREE_PATH}}" rule above is the prompt-level reinforcement.
- **Sub-agents ran git/gh and got into bad states.** Some opened malformed PRs; some pushed broken refs. Centralizing all remote ops on the coordinator is the structural fix; the "do not run git push, gh, or GitHub API calls" rule above is the prompt-level reinforcement.

Both failure modes are described in `docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md` §1 and §8.
