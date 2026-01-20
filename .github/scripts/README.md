# file: .github/scripts/README.md
# version: 1.0.0
# guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

# Release Fix Script

This directory contains scripts to fix broken releases and tags in the audiobook-organizer repository.

## Problem

The repository had broken releases with:
- Duplicate commit hashes across multiple releases
- Empty changelogs showing "No commits available"
- Untagged draft releases
- Tags pointing to wrong commits

## Solution

### fix-releases.sh

A comprehensive shell script that:
1. Deletes all broken releases (v0.2.0 through v0.11.0)
2. Deletes corresponding git tags (both local and remote)
3. Recreates releases with proper changelogs from git history
4. Includes safety features (dry-run mode, confirmation prompts)

### Usage

#### Dry-run (default, safe to test)
```bash
.github/scripts/fix-releases.sh
```

#### Execute with confirmation
```bash
.github/scripts/fix-releases.sh --execute
```

#### Execute without confirmation (use with caution!)
```bash
.github/scripts/fix-releases.sh --execute --skip-confirmation
```

#### Show help
```bash
.github/scripts/fix-releases.sh --help
```

## Prerequisites

- GitHub CLI (`gh`) installed and authenticated
- Git installed
- Write access to the repository
- Full git history (not a shallow clone)

## Safety Features

1. **Dry-run mode by default**: Shows what would be done without making changes
2. **Confirmation prompts**: Asks for confirmation before destructive operations
3. **Idempotent**: Safe to run multiple times
4. **Detailed logging**: Shows progress and results of each operation
5. **Error handling**: Gracefully handles missing releases/tags

## How It Works

### Phase 1: Delete Broken Releases
- Iterates through all versions (v0.2.0 - v0.11.0)
- Uses `gh release delete` to remove each release
- Handles cases where releases don't exist

### Phase 2: Delete Broken Tags
- Deletes local tags using `git tag -d`
- Deletes remote tags using `git push origin :refs/tags/<tag>`
- Handles cases where tags don't exist

### Phase 3: Recreate Releases
- Creates new git tags at the correct commits
- Generates changelogs from git history using `git log`
- Creates releases with proper descriptions using `gh release create`
- Links each release to the correct commit

## Expected Outcome

After running the script:
- All releases v0.2.0-v0.11.0 are deleted and recreated
- Each release points to the correct commit hash
- Changelogs show actual commits between versions
- No more "untagged" releases
- No more "No commits available" messages

## Integration with Workflows

The fix also updates the prerelease workflow to prevent future issues:

### release_workflow.py
Updated to generate real changelogs from git history instead of synthetic placeholders:
- Uses `git log` to extract commits since last tag
- Formats commits as bullet points
- Includes commit hash and description

### prerelease.yml
Minor version bump to reflect the changelog generation improvements.

## Troubleshooting

### "gh: To use GitHub CLI in a GitHub Actions workflow..."
Make sure the `GH_TOKEN` environment variable is set:
```bash
export GH_TOKEN=$GITHUB_TOKEN
```

### "GitHub CLI is not authenticated"
Run `gh auth login` to authenticate:
```bash
gh auth login
```

### "Release not found or already deleted"
This is expected if you run the script multiple times. The script is idempotent.

### "Remote tag not found"
This is expected if you run the script multiple times or if tags were manually deleted.

## Manual Verification

After running the script, verify the releases:

1. Visit: https://github.com/jdfalk/audiobook-organizer/releases
2. Check that all releases show proper changelogs
3. Verify that each release points to the correct commit
4. Confirm no "untagged" releases exist

## Rollback

If you need to rollback:

1. Delete the newly created releases:
   ```bash
   for version in v0.2.0 v0.3.0 v0.4.0 v0.5.0 v0.6.0 v0.7.0 v0.8.0 v0.9.0 v0.10.0 v0.11.0; do
       gh release delete $version --repo jdfalk/audiobook-organizer --yes
   done
   ```

2. Delete the tags:
   ```bash
   for version in v0.2.0 v0.3.0 v0.4.0 v0.5.0 v0.6.0 v0.7.0 v0.8.0 v0.9.0 v0.10.0 v0.11.0; do
       git push origin :refs/tags/$version
   done
   ```

## Future Improvements

Consider these enhancements for future versions:

1. **Commit mapping**: Map each version to its actual commit based on commit messages
2. **Automated testing**: Add tests for the script logic
3. **GitHub Actions integration**: Run as a workflow for automated fixes
4. **Better error recovery**: More sophisticated error handling
5. **Progress tracking**: Show progress percentage during execution
