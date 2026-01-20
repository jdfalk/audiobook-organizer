# file: .github/scripts/USAGE_GUIDE.md
# version: 1.0.0
# guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

# Release Fix Script - Complete Usage Guide

## Table of Contents

1. [Quick Start](#quick-start)
2. [Understanding the Problem](#understanding-the-problem)
3. [How the Script Works](#how-the-script-works)
4. [Step-by-Step Execution](#step-by-step-execution)
5. [Safety Mechanisms](#safety-mechanisms)
6. [Common Scenarios](#common-scenarios)
7. [Troubleshooting](#troubleshooting)
8. [Advanced Usage](#advanced-usage)

## Quick Start

### Prerequisites Check

Before running the script, ensure you have:

```bash
# 1. GitHub CLI installed
which gh || echo "Install: https://cli.github.com/"

# 2. Git installed
which git || echo "Install git"

# 3. Authenticated with GitHub
gh auth status

# 4. Full git history (not shallow clone)
git log --oneline | wc -l  # Should show many commits
```

### Basic Usage

```bash
# 1. Review what would be done (safe)
.github/scripts/fix-releases.sh

# 2. Execute the fix (requires confirmation)
.github/scripts/fix-releases.sh --execute

# 3. Skip confirmation (use with caution)
.github/scripts/fix-releases.sh --execute --skip-confirmation
```

## Understanding the Problem

### What Was Broken?

The repository had releases with these issues:

| Release | Issue | Impact |
|---------|-------|--------|
| v0.2.0 | Duplicate commit hash | Tag points to wrong version |
| v0.3.0 | "No commits available" | Empty changelog |
| v0.4.0 | "untagged" draft | Release not properly tagged |
| v0.5.0-v0.11.0 | Wrong commits | Tags don't match actual changes |

### Root Cause

The reusable workflow from `jdfalk/ghcommon` was:
1. Using cached/stale commit references
2. Generating synthetic "No commits available" changelogs
3. Not properly creating git tags before releases
4. Reusing old commit hashes across multiple releases

### Impact on Users

- ❌ Unable to see what changed in each release
- ❌ Releases don't match actual code state
- ❌ Changelog history is lost
- ❌ Tags don't work for version pinning

## How the Script Works

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                  fix-releases.sh                     │
├─────────────────────────────────────────────────────┤
│                                                      │
│  Phase 1: Delete Broken Releases                    │
│  ├─ Query GitHub for each version                   │
│  ├─ Delete release if exists                        │
│  └─ Handle missing releases gracefully              │
│                                                      │
│  Phase 2: Delete Broken Tags                        │
│  ├─ Check local tags                                │
│  ├─ Delete local tags if exist                      │
│  ├─ Check remote tags                               │
│  └─ Delete remote tags if exist                     │
│                                                      │
│  Phase 3: Recreate Releases                         │
│  ├─ Get current commit hash                         │
│  ├─ Create new tag at correct commit                │
│  ├─ Generate changelog from git history             │
│  ├─ Create release with proper description          │
│  └─ Verify release creation                         │
│                                                      │
└─────────────────────────────────────────────────────┘
```

### Data Flow

```
Input:
  - List of versions to fix (v0.2.0 - v0.11.0)
  - Repository name (jdfalk/audiobook-organizer)
  - Execution mode (dry-run or execute)

Processing:
  1. Validate prerequisites (gh, git, auth)
  2. For each version:
     - Delete existing release
     - Delete existing tags
     - Create new tag at HEAD
     - Generate real changelog
     - Create new release

Output:
  - Fixed releases with proper tags
  - Real changelogs from git history
  - Success/failure reports
```

## Step-by-Step Execution

### Step 1: Prepare Environment

```bash
# Navigate to repository
cd /path/to/audiobook-organizer

# Ensure on main branch
git checkout main

# Pull latest changes
git pull origin main

# Unshallow if needed (for full history)
git fetch --unshallow
```

### Step 2: Dry-Run (Always Do This First!)

```bash
# Run in dry-run mode
.github/scripts/fix-releases.sh

# Review output carefully
# Look for:
# - [DRY-RUN] prefix on all operations
# - List of releases to delete
# - List of tags to delete
# - "would create release" messages
```

Expected output:
```
==========================================
  Release Fix Script
  Repository: jdfalk/audiobook-organizer
==========================================

[INFO] Running in DRY-RUN mode (no changes will be made)

[INFO] Checking prerequisites...
[SUCCESS] All prerequisites met

[INFO] Phase 1: Deleting broken releases...
[INFO] [DRY-RUN] Would delete release: v0.2.0
[INFO] [DRY-RUN] Would delete release: v0.3.0
...

[INFO] Phase 2: Deleting broken tags...
[INFO] [DRY-RUN] Would delete tag: v0.2.0
...

[INFO] [DRY-RUN] Phase 3 would recreate releases with proper changelogs

[INFO] Dry-run complete. To execute these changes, run:
[INFO]   fix-releases.sh --execute
```

### Step 3: Execute with Confirmation

```bash
# Run with execute flag
.github/scripts/fix-releases.sh --execute

# You will be prompted:
# "Are you sure you want to continue? (yes/no):"
# Type 'yes' and press Enter
```

### Step 4: Monitor Execution

Watch for:
- ✅ Green `[SUCCESS]` messages
- ⚠️ Yellow `[WARNING]` messages (usually OK)
- ❌ Red `[ERROR]` messages (investigate these)

### Step 5: Verify Results

```bash
# Check releases on GitHub
open "https://github.com/jdfalk/audiobook-organizer/releases"

# Or use gh CLI
gh release list --repo jdfalk/audiobook-organizer

# Check tags
git tag -l

# Verify a specific release
gh release view v0.5.0 --repo jdfalk/audiobook-organizer
```

## Safety Mechanisms

### 1. Dry-Run Mode (Default)

**What it does:**
- Shows all operations that would be performed
- Makes NO actual changes
- Safe to run anytime

**When to use:**
- First time running the script
- After any script modifications
- To preview changes before execution

### 2. Confirmation Prompts

**What it does:**
- Requires typing "yes" to proceed
- Shows list of affected versions
- Gives you a chance to abort

**When triggered:**
- Only in execute mode (not dry-run)
- Before any destructive operations
- After showing what will be changed

**To skip:**
```bash
# Use --skip-confirmation flag (use with caution!)
.github/scripts/fix-releases.sh --execute --skip-confirmation
```

### 3. Idempotent Operations

**What it means:**
- Safe to run multiple times
- Already-deleted releases are skipped
- Already-deleted tags are handled gracefully
- No errors if running on clean state

**Example:**
```bash
# First run - deletes and recreates everything
.github/scripts/fix-releases.sh --execute

# Second run - mostly no-ops, safe
.github/scripts/fix-releases.sh --execute
# Output: "Release v0.2.0 not found or already deleted"
```

### 4. Error Handling

**Built-in protections:**
- Checks for required tools (gh, git)
- Validates authentication
- Handles network failures gracefully
- Continues on non-critical errors
- Reports all errors clearly

### 5. Color-Coded Output

**Visual indicators:**
- 🔵 Blue `[INFO]` - Informational messages
- 🟢 Green `[SUCCESS]` - Successful operations
- 🟡 Yellow `[WARNING]` - Non-critical issues
- 🔴 Red `[ERROR]` - Critical problems

## Common Scenarios

### Scenario 1: First Time Fixing Releases

```bash
# 1. Check prerequisites
gh auth status

# 2. Dry-run to see what would happen
.github/scripts/fix-releases.sh

# 3. Review output carefully

# 4. Execute with confirmation
.github/scripts/fix-releases.sh --execute
# Type 'yes' when prompted

# 5. Verify results
gh release list --repo jdfalk/audiobook-organizer
```

### Scenario 2: Re-running After Partial Failure

```bash
# The script failed partway through
# It's safe to just re-run it

.github/scripts/fix-releases.sh --execute

# The script will:
# - Skip already-deleted releases
# - Skip already-deleted tags
# - Recreate only missing releases
# - Report success for already-done operations
```

### Scenario 3: Fixing Only Specific Versions

```bash
# Edit the script to change VERSIONS array
# This requires modifying .github/scripts/fix-releases.sh

# Find this section:
VERSIONS=(
    "v0.2.0"
    "v0.3.0"
    # ... comment out versions you don't want to fix
)

# Then run normally
.github/scripts/fix-releases.sh --execute
```

### Scenario 4: Testing in a Fork

```bash
# 1. Fork the repository
# 2. Clone your fork
git clone https://github.com/YOUR_USERNAME/audiobook-organizer

# 3. Update REPO variable in script
# Edit .github/scripts/fix-releases.sh
# Change: REPO="jdfalk/audiobook-organizer"
# To: REPO="YOUR_USERNAME/audiobook-organizer"

# 4. Run the script
.github/scripts/fix-releases.sh --execute
```

## Troubleshooting

### Issue: "gh: command not found"

**Solution:**
```bash
# macOS
brew install gh

# Linux
# See: https://github.com/cli/cli/blob/trunk/docs/install_linux.md

# Verify installation
gh --version
```

### Issue: "GitHub CLI is not authenticated"

**Solution:**
```bash
# Authenticate with GitHub
gh auth login

# Follow the prompts:
# 1. Select "GitHub.com"
# 2. Choose HTTPS or SSH
# 3. Authenticate via web browser
# 4. Authorize GitHub CLI

# Verify
gh auth status
```

### Issue: "Permission denied" when deleting tags

**Solution:**
```bash
# Check your GitHub permissions
gh api user/repos/jdfalk/audiobook-organizer --jq '.permissions'

# You need:
# - push: true
# - admin: true (for deleting tags)

# If you don't have permissions:
# - Ask repository owner for access
# - Or use a personal access token with repo scope
```

### Issue: "Shallow clone" - limited git history

**Solution:**
```bash
# Unshallow the repository
git fetch --unshallow

# Verify you have full history
git log --oneline | wc -l
# Should show many commits (not just 1-2)

# Then re-run the script
.github/scripts/fix-releases.sh --execute
```

### Issue: Script hangs during confirmation

**Solution:**
```bash
# The script is waiting for "yes" input
# If in CI/CD or automated environment, use:
.github/scripts/fix-releases.sh --execute --skip-confirmation

# Or provide input via echo:
echo "yes" | .github/scripts/fix-releases.sh --execute
```

### Issue: "No commits available" in generated changelog

**Problem:** Git history is missing or incomplete

**Solution:**
```bash
# 1. Ensure you're not in a shallow clone
git fetch --unshallow

# 2. Pull all tags
git fetch --tags

# 3. Verify git log works
git log --oneline -20

# 4. Re-run the script
.github/scripts/fix-releases.sh --execute
```

## Advanced Usage

### Running in CI/CD

```yaml
# Example GitHub Actions workflow
name: Fix Releases
on:
  workflow_dispatch:

jobs:
  fix-releases:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history
      
      - name: Run fix script
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          .github/scripts/fix-releases.sh --execute --skip-confirmation
```

### Customizing Changelog Format

Edit `.github/workflows/scripts/release_workflow.py`:

```python
def generate_changelog() -> None:
    """Generate changelog from git history."""
    commits = get_git_commits_since_last_tag()
    
    # Customize format here
    content = "## What's New\n\n"
    for commit in commits:
        # Parse commit message
        # Format as needed
        content += f"{commit}\n"
    
    write_output("changelog_content", content)
```

### Batch Processing Multiple Repositories

```bash
# Create a wrapper script
#!/bin/bash

REPOS=(
    "jdfalk/audiobook-organizer"
    "jdfalk/another-repo"
    # Add more repos
)

for repo in "${REPOS[@]}"; do
    echo "Processing $repo..."
    cd "/path/to/$repo"
    .github/scripts/fix-releases.sh --execute --skip-confirmation
done
```

### Logging Output for Audit

```bash
# Capture all output to a log file
.github/scripts/fix-releases.sh --execute 2>&1 | tee release-fix-$(date +%Y%m%d-%H%M%S).log

# Review log later
less release-fix-*.log

# Search for errors
grep '\[ERROR\]' release-fix-*.log
```

## Best Practices

1. **Always dry-run first** - Never skip the dry-run step
2. **Verify prerequisites** - Check gh and git are working
3. **Use confirmation prompts** - Don't skip them unless absolutely needed
4. **Monitor execution** - Watch for errors and warnings
5. **Verify results** - Check GitHub releases page after running
6. **Keep logs** - Save output for troubleshooting
7. **Test in fork first** - If unsure, test in a forked repository
8. **Document changes** - Note when and why you ran the script

## Next Steps After Fixing

1. **Verify all releases** - Check each release has proper changelog
2. **Test version pinning** - Try `git checkout v0.5.0` to verify tags work
3. **Update documentation** - Note the fix in CHANGELOG.md
4. **Monitor future releases** - Ensure new releases work correctly
5. **Consider automation** - Set up workflow to prevent future issues

## Support

If you encounter issues:

1. Check this guide's troubleshooting section
2. Review the main README in `.github/scripts/README.md`
3. Open an issue on GitHub with:
   - Full error output
   - Your environment details (OS, gh version, git version)
   - Steps you took before the error
   - Output of dry-run mode
