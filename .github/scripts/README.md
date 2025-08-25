# GitHub Scripts

This directory contains standalone scripts used by GitHub Actions workflows.

## Scripts

### super-linter-pr-comment.js

Creates or updates PR comments with Super Linter results and auto-fix information.

**Usage**: Called by the `reusable-super-linter.yml` workflow via `actions/github-script`.

**Environment Variables**:

- `HAS_AUTO_FIXES`: Boolean indicating if auto-fixes were applied
- `AUTO_FIX_ENABLED`: Boolean indicating if auto-fix is enabled
- `AUTO_COMMIT_ENABLED`: Boolean indicating if auto-commit is enabled

**Features**:

- Creates comprehensive PR comments with linting results
- Shows auto-fix status and configuration
- Handles error reporting and truncation
- Updates existing comments instead of creating duplicates
