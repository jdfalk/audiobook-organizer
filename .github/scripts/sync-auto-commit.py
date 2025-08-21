#!/usr/bin/env python3
# file: .github/scripts/sync-auto-commit.py
# version: 1.0.0
# guid: f7a8b9c0-d1e2-3f4a-5b6c-7d8e9f0a1b2c

"""
Auto-commit script for workflow modernization.
Creates conventional commits for workflow changes.
"""

import subprocess


def run_command(cmd, capture_output=True):
    """Run a shell command."""
    result = subprocess.run(cmd, shell=True, capture_output=capture_output, text=True)
    if result.returncode != 0 and capture_output:
        print(f"Error running command: {cmd}")
        print(f"Error: {result.stderr}")
        return None
    return result.stdout.strip() if capture_output else result.returncode == 0


def main():
    """Auto-commit workflow modernization changes."""
    # Check if there are changes to commit
    status = run_command("git status --porcelain")
    if not status:
        print("No changes to commit")
        return

    # Create commit message based on changed files
    commit_msg = """feat(workflows): continue release workflow modernization

Convert more workflow scripts to Python for better reliability and maintainability.
Extract inline scripts to dedicated Python modules in .github/scripts/.

Scripts converted:
- sync-release-build-artifacts.py - Build and package artifacts for releases
- Additional workflow modernization improvements

Part of comprehensive workflow system overhaul for automated repository management."""

    # Commit the changes
    print("Committing workflow modernization changes...")
    if run_command(f'git commit -m "{commit_msg}"', capture_output=False):
        print("Successfully committed changes")

        # Push the changes
        print("Pushing changes to remote...")
        if run_command("git push", capture_output=False):
            print("Successfully pushed changes")
        else:
            print("Failed to push changes")
    else:
        print("Failed to commit changes")


if __name__ == "__main__":
    main()
