---
name: quick-fix
description: Create a quick fix branch, commit, PR, merge, and rebase worktrees
disable-model-invocation: true
---

# Quick Fix Workflow

Use this for small fixes while another Claude process is working on main. Run through the entire sequence without asking for confirmation at each step.

## Arguments

- First argument: short description of the fix (used for branch name and commit)

## Steps

Execute all steps in sequence without pausing:

1. **Create branch from main:**
   ```bash
   git checkout main && git pull
   git checkout -b fix/<description>
   ```

2. **Make the fix** — edit the necessary files

3. **Commit with conventional commit message:**
   ```bash
   git add <changed-files>
   git commit -m "fix(<scope>): <description>"
   ```

4. **Push and create PR:**
   ```bash
   git push -u origin fix/<description>
   gh pr create --title "fix(<scope>): <description>" --body "<summary>"
   ```

5. **Merge with rebase** (this repo uses rebase/FF only, no squash):
   ```bash
   gh pr merge <number> --rebase
   ```

6. **Return to main:**
   ```bash
   git checkout main && git pull
   ```

7. **Update any active worktrees:**
   ```bash
   # For each worktree that exists:
   cd <worktree> && git fetch origin main && git rebase origin/main
   ```

## Rules

- Use conventional commit format: `fix(<scope>): <description>`
- This repo uses rebase/FF merges only — never squash
- Do NOT pause for confirmation between steps
- If the PR has CI checks, wait for them to pass before merging
