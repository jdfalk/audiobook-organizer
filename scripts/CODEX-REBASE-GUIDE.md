# Codex Rebase Quick Reference

## When to Use

Use these scripts when Codex encounters rebase conflicts or gets stuck in rebase loops.

## Simple Auto-Rebase (Recommended for Codex)

```bash
./scripts/rebase --mode automated --force-push main
```

This unified script:

- ✅ Automatically detects best implementation (Python preferred, shell fallback)
- ✅ Resolves conflicts intelligently based on file types
- ✅ Creates backup branch automatically
- ✅ Force pushes the result
- ✅ Generates comprehensive summary report
- ✅ Saves both versions for code files that need manual review

## Advanced Rebase with Options

```bash
# Dry run to see what would happen
./scripts/rebase --dry-run --verbose main

# Automated mode with force push (best for Codex)
./scripts/rebase --mode automated --force-push main

# Interactive mode for manual conflict resolution
./scripts/rebase --mode interactive main

# Force specific implementation
./scripts/rebase --implementation python --force-push main
./scripts/rebase --implementation shell --force-push main
```

## VS Code Tasks

Use these task IDs in VS Code:

- `"Codex Auto Rebase"` - Legacy automated rebase (still available)
- `"Smart Rebase Dry Run"` - Preview changes (still available)
- `"Smart Rebase onto Main"` - Legacy full featured rebase (still available)

**New recommended approach:** Use the terminal or create custom tasks with the unified `rebase`
script:

```bash
# Add to tasks.json for new unified approach
{
    "label": "Rebase: Auto onto Main",
    "type": "shell",
    "command": "./scripts/rebase",
    "args": ["--mode", "automated", "--force-push", "main"],
    "group": "build"
},
{
    "label": "Rebase: Dry Run",
    "type": "shell",
    "command": "./scripts/rebase",
    "args": ["--dry-run", "--verbose", "main"],
    "group": "build"
}
```

## Suggested VS Code Tasks

Add these tasks to your `.vscode/tasks.json` for easy access:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Rebase: Auto onto Main (Recommended for Codex)",
      "type": "shell",
      "command": "./scripts/rebase",
      "args": ["--mode", "automated", "--force-push", "main"],
      "group": "build",
      "detail": "Automated rebase with intelligent conflict resolution"
    },
    {
      "label": "Rebase: Dry Run Preview",
      "type": "shell",
      "command": "./scripts/rebase",
      "args": ["--dry-run", "--verbose", "main"],
      "group": "build",
      "detail": "Preview what rebase would do without executing"
    },
    {
      "label": "Rebase: Interactive Mode",
      "type": "shell",
      "command": "./scripts/rebase",
      "args": ["--mode", "interactive", "main"],
      "group": "build",
      "detail": "Manual conflict resolution with prompts"
    },
    {
      "label": "Rebase: Force Shell Implementation",
      "type": "shell",
      "command": "./scripts/rebase",
      "args": ["--implementation", "shell", "--mode", "automated", "--force-push", "main"],
      "group": "build",
      "detail": "Use shell fallback implementation"
    }
  ]
}
```

## Conflict Resolution Strategy

The unified rebase script uses intelligent strategies based on file types:

| File Type                                | Python Implementation                                        | Shell Implementation                   |
| ---------------------------------------- | ------------------------------------------------------------ | -------------------------------------- |
| Documentation (`.md`, `docs/*`)          | Accept incoming from main                                    | Accept incoming from main              |
| Build files (`.github/`, `Dockerfile`)   | Accept incoming from main                                    | Accept incoming from main              |
| Package files (`go.mod`, `package.json`) | Accept incoming from main                                    | Accept incoming from main              |
| Config files (`.json`, `.yml`)           | Smart merge or accept incoming                               | Accept incoming                        |
| Source code (`.go`, `.js`, `.py`)        | Save both versions (`.current` + `.incoming`) + use incoming | Accept incoming (safer for automation) |

**Python Implementation Features:**

- More sophisticated conflict resolution
- Saves both versions for manual review
- Better error handling and recovery
- Comprehensive logging

**Shell Implementation Features:**

- Simpler, more predictable behavior
- Works in minimal environments
- Safer defaults (always prefers incoming)

## Recovery if Something Goes Wrong

1. Find backup branch: `git branch | grep backup`
2. Restore: `git checkout backup-TIMESTAMP-BRANCH`
3. Recreate branch: `git checkout -b original-branch-name`
4. Force push: `git push --force-with-lease origin original-branch-name`

## Files Created

After running the new unified script, you may see:

- `*.current` and `*.incoming` - Both versions of conflicted files (Python implementation)
- `rebase-summary-TIMESTAMP.md` - Comprehensive summary of what happened
- `backup/BRANCH/TIMESTAMP` - Backup branch (safe to delete after verification)

## Best Practices for Codex

1. **Use the unified script**: `./scripts/rebase --mode automated --force-push main`
2. **Always use automated mode** for AI agents: `--mode automated`
3. **Include force-push flag** for continuous integration: `--force-push`
4. **Use dry-run for testing**: `--dry-run --verbose` to preview changes
5. **Check the summary file** for important information about what happened
6. **Review conflict resolution files** when available (`.current` and `.incoming`)
7. **Test the rebased code** before continuing with other tasks
8. **Clean up backup branches** when done: `git branch -D backup/*`

## Environment Variables for Automation

The new script doesn't require environment variables, but you can set these for consistency:

```bash
# For backward compatibility with legacy scripts
export FORCE_PUSH=true
export DRY_RUN=false
export VERBOSE=true
```

## Quick Commands for Codex

**Most Common (Recommended):**

```bash
./scripts/rebase --mode automated --force-push main
```

**Safe Testing:**

```bash
./scripts/rebase --dry-run --verbose main
```

**Emergency Fallback (if Python fails):**

```bash
./scripts/rebase --implementation shell --mode automated --force-push main
```

## Common Error Solutions

- **"Not in git repository"**: Make sure you're in project root
- **"Target branch does not exist"**: Run `git fetch origin` first
- **"Force push failed"**: Someone else pushed changes, try `git pull --rebase` first
- **Script hangs**: Kill with Ctrl+C, then `git rebase --abort`
