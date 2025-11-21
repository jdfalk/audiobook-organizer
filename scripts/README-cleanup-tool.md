# Repository Cleanup Tool

Automatically detects and cleans up archived or outdated GitHub repositories
from your local filesystem.

## Features

- 🔍 **Smart Detection**: Scans directories for Git repositories and checks
  their GitHub status
- 🗃️ **Archive Detection**: Identifies archived, disabled, or outdated
  repositories
- 🧹 **Safe Cleanup**: Removes local directories for repositories that are no
  longer active
- 🛡️ **Safety First**: Dry-run mode by default, interactive confirmations
- 📊 **Detailed Logging**: Comprehensive logs with timestamps and statistics
- ⚡ **GitHub Integration**: Uses GitHub CLI for accurate repository status

## Prerequisites

1. **GitHub CLI**: Install from <https://cli.github.com/>
2. **Authentication**: Run `gh auth login` to authenticate
3. **Python 3.7+**: Required for the script execution

## Quick Start

```bash
# Safe dry run (shows what would be done)
./scripts/cleanup-repos.sh

# Interactive cleanup with confirmations
./scripts/cleanup-repos.sh --no-dry-run

# Automated cleanup (no prompts)
./scripts/cleanup-repos.sh --no-dry-run --no-interactive
```

## Usage Examples

### Basic Usage

```bash
# Default dry run - safe to run anywhere
python scripts/cleanup-archived-repos.py

# Real cleanup with confirmations
python scripts/cleanup-archived-repos.py --no-dry-run

# Fully automated cleanup
python scripts/cleanup-archived-repos.py --no-dry-run --no-interactive
```

### Advanced Options

```bash
# Custom repository path
python scripts/cleanup-archived-repos.py --path ~/my-projects

# Verbose logging
python scripts/cleanup-archived-repos.py --verbose

# Different base directory
python scripts/cleanup-archived-repos.py --path /Users/username/code
```

## What Gets Removed

The tool identifies repositories for removal based on:

1. **Archived repositories** - Marked as archived on GitHub
2. **Disabled repositories** - Disabled due to policy violations
3. **Stale repositories** - Not updated in over 2 years

## Safety Features

- ✅ **Dry Run Default**: Shows what would be done without making changes
- ✅ **Interactive Mode**: Prompts for confirmation before each removal
- ✅ **Detailed Logging**: All actions logged with timestamps
- ✅ **GitHub Verification**: Checks actual GitHub status, not just local state
- ✅ **Error Handling**: Graceful handling of network issues and API limits

## Sample Output

```text
2025-08-10 15:30:45,123 - INFO - Starting repository cleanup
2025-08-10 15:30:45,124 - INFO - Base path: /Users/jdfalk/repos/github.com/jdfalk
2025-08-10 15:30:45,124 - INFO - Dry run: True
2025-08-10 15:30:45,124 - INFO - Interactive: True
2025-08-10 15:30:45,234 - INFO - GitHub CLI version: gh

============================================================
Processing: old-project
============================================================
2025-08-10 15:30:46,123 - INFO - GitHub repository: jdfalk/old-project
2025-08-10 15:30:46,456 - INFO - Archived: True
2025-08-10 15:30:46,456 - INFO - Disabled: False
2025-08-10 15:30:46,456 - INFO - Visibility: public
2025-08-10 15:30:46,456 - INFO - Last updated: 2022-03-15T10:30:45Z
2025-08-10 15:30:46,456 - WARNING - CLEANUP CANDIDATE: Repository is archived
2025-08-10 15:30:46,456 - INFO - [DRY RUN] Would remove: /Users/jdfalk/repos/github.com/jdfalk/old-project

============================================================
CLEANUP SUMMARY
============================================================
Total repositories scanned: 18
Archived/outdated repositories found: 3
Repositories removed: 0
Repositories skipped: 15
Errors encountered: 0
Log file: /Users/jdfalk/logs/repo-cleanup-20250810_153045.log

⚠️  This was a DRY RUN - no changes were made
```

## Command Line Options

| Option             | Description                    | Default                     |
| ------------------ | ------------------------------ | --------------------------- |
| `--path PATH`      | Base directory to scan         | `~/repos/github.com/jdfalk` |
| `--dry-run`        | Show actions without executing | `True`                      |
| `--no-dry-run`     | Actually perform cleanup       | -                           |
| `--interactive`    | Prompt for confirmations       | `True`                      |
| `--no-interactive` | Run without prompts            | -                           |
| `--verbose`        | Enable debug logging           | `False`                     |

## Log Files

All operations are logged to timestamped files in `~/logs/`:

- `repo-cleanup-YYYYMMDD_HHMMSS.log`

Logs include:

- Repository scan results
- GitHub API responses
- Actions taken or simulated
- Error details and statistics
- Final summary report

## Error Handling

The tool handles various error conditions gracefully:

- **Network issues**: Retries with exponential backoff
- **API rate limits**: Respects GitHub API limits
- **Authentication failures**: Clear error messages
- **File system errors**: Detailed error logging
- **Interrupted operations**: Clean exit with partial results

## Contributing

1. Test thoroughly with `--dry-run` first
2. Add new repository detection criteria carefully
3. Maintain backward compatibility
4. Update documentation for new features
5. Follow existing code style and patterns

## Safety Notice

⚠️ **Important**: Always run with `--dry-run` first to see what would be
removed!

This tool permanently deletes local directories. While it has safety features,
you should:

1. Backup important work before running
2. Review the dry-run output carefully
3. Understand what each repository contains
4. Use version control for important projects

The tool only removes local directories - it never touches your GitHub
repositories online.
