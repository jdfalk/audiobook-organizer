# Smart Rebase Automation Documentation

This directory contains intelligent rebase automation scripts designed to handle Git conflicts
automatically, particularly useful for AI agents like Codex.

## Scripts Overview

### 1. `smart-rebase.sh` - Advanced Rebase Script

A comprehensive rebase script with intelligent conflict resolution strategies.

**Features:**

- Automatic conflict resolution based on file types
- Backup creation before rebase
- Multiple resolution strategies (incoming, current, smart merge, save both)
- Dry-run mode for testing
- Verbose logging
- Smart merge for JSON/YAML files (requires `jq`/`yq`)

**Usage:**

```bash
# Basic rebase
./scripts/smart-rebase.sh main

# Rebase with force push
./scripts/smart-rebase.sh -f main

# Dry run to see what would happen
./scripts/smart-rebase.sh --dry-run main

# Verbose output
./scripts/smart-rebase.sh -v -f origin/main
```

**Options:**

- `-f, --force-push`: Force push after successful rebase
- `-d, --dry-run`: Show what would be done without executing
- `-v, --verbose`: Enable verbose output
- `-h, --help`: Show help message

### 2. `codex-rebase.sh` - Codex-Optimized Script

A simpler, more automated script specifically designed for AI agents.

**Features:**

- Automatic conflict resolution with "keep current, save incoming" strategy
- Force push enabled by default
- Automatic backup creation
- Summary generation
- Minimal user interaction required

**Usage:**

```bash
# Rebase current branch onto main
./scripts/codex-rebase.sh

# Rebase onto specific branch
./scripts/codex-rebase.sh origin/develop
```

**What it does:**

1. Creates backup branch automatically
2. Stashes any uncommitted changes
3. Fetches latest changes
4. Performs rebase with auto-conflict resolution
5. Saves incoming versions as `.main.incoming` files
6. Keeps current version as resolved
7. Force pushes rebased branch
8. Generates summary report

## Conflict Resolution Strategies

### File Type Based Resolution

| File Type                                | Strategy    | Reasoning                            |
| ---------------------------------------- | ----------- | ------------------------------------ |
| Documentation (`.md`, `docs/`)           | Incoming    | Main branch docs are usually current |
| Build files (`.github/`, `Dockerfile`)   | Incoming    | Build config should match main       |
| Package files (`go.mod`, `package.json`) | Incoming    | Dependencies should match main       |
| Configuration (`.json`, `.yml`)          | Smart Merge | Configs need careful merging         |
| Source code (`.go`, `.js`, `.py`)        | Save Both   | Code needs manual review             |
| Test files (`*_test.go`, `test/`)        | Save Both   | Tests need careful review            |
| Data files (`.sql`, `data/`)             | Current     | Data is often environment-specific   |

### Smart Merge Features

For JSON and YAML files, the script attempts intelligent merging:

- **JSON**: Uses `jq` to merge objects at root level
- **YAML**: Uses `yq` to merge maps at root level
- Falls back to "save both" if smart merge fails

## Configuration

The `rebase-config.yml` file contains detailed configuration for:

- File pattern matching
- Resolution strategies per file type
- Special file handling
- Codex-specific settings
- Commit message templates

## Recovery and Cleanup

### If Something Goes Wrong

Each rebase creates a backup branch. To recover:

```bash
# Find your backup branch
git branch | grep backup

# Restore from backup
git checkout backup-20241222-143022-feature-branch
git branch -D your-feature-branch
git checkout -b your-feature-branch
git push --force-with-lease origin your-feature-branch
```

### Cleanup After Successful Rebase

```bash
# Remove backup branch
git branch -D backup-20241222-143022-feature-branch

# Remove .main.incoming files if no longer needed
rm *.main.incoming

# Remove rebase summary if no longer needed
rm rebase-summary-*.md
```

## Integration with VS Code Tasks

Add these tasks to your `.vscode/tasks.json`:

```json
{
    "label": "Smart Rebase onto Main",
    "type": "shell",
    "command": "./scripts/smart-rebase.sh",
    "args": ["-f", "main"],
    "group": "build",
    "detail": "Rebase current branch onto main with force push"
},
{
    "label": "Codex Auto Rebase",
    "type": "shell",
    "command": "./scripts/codex-rebase.sh",
    "args": ["main"],
    "group": "build",
    "detail": "Automated rebase for Codex"
}
```

## Environment Variables

Both scripts support environment variables:

```bash
# Enable force push by default
export FORCE_PUSH=true

# Enable dry run mode
export DRY_RUN=true

# Enable verbose output
export VERBOSE=true
```

## Dependencies

### Required

- Git (obviously)
- Bash 4.0+

### Optional (for enhanced features)

- `jq` - for JSON smart merging
- `yq` - for YAML smart merging

Install on macOS:

```bash
brew install jq yq
```

## Troubleshooting

### Common Issues

1. **"Not in a git repository"**
   - Ensure you're in the root of your git repository

2. **"Target branch does not exist"**
   - Check branch name spelling
   - Ensure you've fetched latest changes: `git fetch origin`

3. **"Force push failed"**
   - Someone else may have pushed changes
   - Use `git pull --rebase` first, then retry

4. **"Smart merge failed"**
   - Install `jq` and `yq` for better config file merging
   - Script will fall back to "save both" strategy

### Debug Mode

Run with verbose flag to see detailed output:

```bash
./scripts/smart-rebase.sh -v main
```

### Manual Conflict Resolution

If automatic resolution isn't sufficient:

1. Stop the script (Ctrl+C)
2. Resolve conflicts manually
3. Continue with: `git rebase --continue`
4. Push manually: `git push --force-with-lease origin branch-name`

## Best Practices

1. **Always test with dry-run first** on important branches
2. **Review .main.incoming files** before deleting them
3. **Keep backup branches** until you're sure everything works
4. **Use descriptive commit messages** when manual intervention is needed
5. **Test the rebased code** before considering the rebase complete

## Codex Usage Tips

For AI agents using these scripts:

1. Use `codex-rebase.sh` for simplicity
2. Always check the generated summary file
3. Review any `.main.incoming` files for important changes
4. Test rebased code before proceeding with further changes
5. Clean up backup branches and temporary files when done

## Security Considerations

- Scripts never commit sensitive information
- Backup branches are local only unless explicitly pushed
- Force push uses `--force-with-lease` for safety
- Temporary files are cleaned up automatically
