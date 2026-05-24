---
description: Create a worktree + branch for new work before any edits
---

1. Ask user for branch name and short description (if not already provided)
2. Run: `git worktree add ../$(basename $PWD)-<feature> -b <branch>`
3. Confirm you are now operating in the new worktree path (not main)
4. Run `git status` to confirm a clean working tree before making any changes
5. Remind the user: all commits go through PRs — never directly to main
