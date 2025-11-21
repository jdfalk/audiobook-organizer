<!-- file: scripts/README-notifications.md -->
<!-- version: 1.1.0 -->
<!-- guid: d4e5f6a7-b8c9-0123-def0-456789012345 -->

# GitHub Notifications Cleanup Scripts

This directory contains scripts to automatically mark old GitHub notifications
as done (completely dismissed).

## Scripts

### `mark_old_notifications_done.py`

Main Python script that uses the GitHub API to mark notifications older than a
specified threshold as done.

**Features:**

- Fetches unread notifications using GitHub API
- Filters notifications older than specified hours (default: 24)
- Marks old notifications as done (completely dismissed) individually
- Supports dry-run mode to preview actions
- Comprehensive error handling and logging
- Rate limiting awareness

**Requirements:**

- Python 3.7+
- GitHub Personal Access Token with `notifications` scope
- Required packages: `requests`, `python-dotenv`

### `cleanup-notifications.sh`

Bash wrapper script that automatically handles dependency installation and runs
the Python script.

**Features:**

- Automatic dependency checking and installation
- Python 3 availability verification
- Pass-through of all command-line arguments

## Setup

### 1. GitHub Token Setup

Create a GitHub Personal Access Token with the `notifications` scope:

1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Click "Generate new token (classic)"
3. Select the `notifications` scope
4. Copy the generated token

Set the token as an environment variable:

```bash
# Option 1: Export in your shell
export GITHUB_TOKEN="your_token_here"

# Option 2: Create .env file in the repository root
echo "GITHUB_TOKEN=your_token_here" > .env

# Option 3: Use alternative environment variable names
export JF_CI_GH_PAT="your_token_here"
export GH_TOKEN="your_token_here"
```

### 2. Install Dependencies

**Option A: Using the wrapper script (automatic)**

```bash
./scripts/cleanup-notifications.sh --dry-run
```

**Option B: Manual installation**

```bash
pip3 install -r scripts/requirements-notifications.txt
```

## Usage

### Basic Usage

Mark notifications older than 24 hours as done:

```bash
# Using wrapper script (recommended)
./scripts/cleanup-notifications.sh

# Using Python script directly
python3 scripts/mark_old_notifications_done.py
```

### Dry Run Mode

Preview what notifications would be marked as done:

```bash
./scripts/cleanup-notifications.sh --dry-run
python3 scripts/mark_old_notifications_done.py --dry-run
```

### Custom Time Threshold

Mark notifications older than specific number of hours:

```bash
# Mark notifications older than 48 hours
./scripts/cleanup-notifications.sh --hours 48

# Mark notifications older than 12 hours
python3 scripts/mark_old_notifications_done.py --hours 12
```

### Combined Options

```bash
# Dry run for notifications older than 6 hours
./scripts/cleanup-notifications.sh --hours 6 --dry-run
```

## Command Line Options

| Option          | Description                               | Default |
| --------------- | ----------------------------------------- | ------- |
| `--hours HOURS` | Number of hours old notifications must be | 24      |
| `--dry-run`     | Show what would be done without doing it  | False   |
| `--verbose`     | Enable verbose output                     | False   |
| `--help`        | Show help message and exit                | -       |

## Examples

### Daily Cleanup

```bash
# Add to crontab for daily cleanup at 9 AM
0 9 * * * /path/to/scripts/cleanup-notifications.sh --hours 24
```

### Weekly Cleanup

```bash
# Clean up week-old notifications every Sunday
0 9 * * 0 /path/to/scripts/cleanup-notifications.sh --hours 168
```

### Interactive Usage

```bash
# Check what would be cleaned up first
./scripts/cleanup-notifications.sh --dry-run

# If the output looks good, run it for real
./scripts/cleanup-notifications.sh
```

## Output Examples

### Dry Run Output

```
GitHub Notification Cleanup
Threshold: 24 hours
Mode: DRY RUN
--------------------------------------------------
Fetching unread notifications...
Found 15 unread notifications
Found 8 notifications older than 24 hours
[DRY RUN] Would mark as done: user/repo1 - Pull request #123 (updated: 2023-12-20T10:30:00Z)
[DRY RUN] Would mark as done: user/repo2 - Issue #456 (updated: 2023-12-19T15:45:00Z)
...
--------------------------------------------------
[DRY RUN] Would have marked 8 notifications as done
```

### Live Run Output

```
GitHub Notification Cleanup
Threshold: 24 hours
Mode: LIVE
--------------------------------------------------
Fetching unread notifications...
Found 15 unread notifications
Found 8 notifications older than 24 hours
Marking as done: user/repo1 - Pull request #123 (updated: 2023-12-20T10:30:00Z)
Marking as done: user/repo2 - Issue #456 (updated: 2023-12-19T15:45:00Z)
...
--------------------------------------------------
Marked 8 notifications as done
```

## Troubleshooting

### Common Issues

**Token not found:**

```
Error: GitHub token not found. Please set GITHUB_TOKEN environment variable.
```

- Solution: Set up GitHub token as described in Setup section

**Permission denied:**

```
Error: 401 Unauthorized
```

- Solution: Check that your token has the `notifications` scope

**Rate limiting:**

```
Error: 403 rate limit exceeded
```

- Solution: Wait for rate limit to reset (usually 1 hour) or use authenticated
  requests

**Python dependencies:**

```
ModuleNotFoundError: No module named 'requests'
```

- Solution: Install dependencies with
  `pip3 install -r scripts/requirements-notifications.txt`

### Debugging

Enable verbose output for more detailed information:

```bash
./scripts/cleanup-notifications.sh --verbose --dry-run
```

Check your GitHub token permissions:

```bash
curl -H "Authorization: token YOUR_TOKEN" https://api.github.com/user
```

## Security Notes

- Never commit your GitHub token to the repository
- Use environment variables or `.env` files for token storage
- The `.env` file should be in your `.gitignore`
- Consider using a dedicated token with minimal required scopes
- Regularly rotate your access tokens

## API Rate Limits

GitHub API has rate limits:

- Authenticated requests: 5,000 per hour
- Unauthenticated requests: 60 per hour

The script handles pagination and should work well within these limits for
normal usage.
