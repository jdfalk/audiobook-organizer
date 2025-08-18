#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-commit-push.py
# version: 1.0.0
# guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

"""
Commit and push synchronized changes.
"""

import subprocess
import sys
from datetime import datetime


def run_command(cmd, description):
    """Run a shell command and return success status."""
    print(f"Running: {description}")

    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=True, text=True, timeout=60
        )

        if result.returncode == 0:
            print(f"✅ {description} successful")
            if result.stdout.strip():
                print(f"Output: {result.stdout.strip()}")
            return True
        else:
            print(f"❌ {description} failed")
            if result.stderr.strip():
                print(f"Error: {result.stderr.strip()}")
            return False
    except subprocess.TimeoutExpired:
        print(f"❌ {description} timed out")
        return False
    except Exception as e:
        print(f"❌ {description} error: {e}")
        return False


def check_for_changes():
    """Check if there are any changes to commit."""
    result = subprocess.run(
        "git status --porcelain", shell=True, capture_output=True, text=True
    )

    return bool(result.stdout.strip())


def main():
    """Main entry point."""
    print("Checking for synchronized changes to commit...")

    if not check_for_changes():
        print("ℹ️  No changes detected, nothing to commit")
        return

    print("📋 Changes detected, proceeding with commit and push...")

    # Configure git user (in case not set in CI)
    run_command('git config user.name "GitHub Actions"', "Setting git user name")
    run_command('git config user.email "actions@github.com"', "Setting git user email")

    # Add all changes
    if not run_command("git add .", "Adding all changes"):
        sys.exit(1)

    # Create commit message with timestamp
    timestamp = datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S UTC")
    commit_message = f"chore(sync): synchronize from ghcommon ({timestamp})"

    # Commit changes
    if not run_command(f'git commit -m "{commit_message}"', "Committing changes"):
        sys.exit(1)

    # Push changes
    if not run_command("git push", "Pushing changes"):
        sys.exit(1)

    print("✅ Successfully committed and pushed synchronized changes")


if __name__ == "__main__":
    main()
