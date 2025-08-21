#!/usr/bin/env python3
"""
Collect Operation Results

Collects operation results and file changes for workflow summary reporting.
"""

import os
import subprocess
import sys
from typing import Dict, List


def get_changed_files() -> List[str]:
    """Get list of changed files in the repository"""
    try:
        result = subprocess.run(
            ["git", "diff", "--name-only", "HEAD~1", "HEAD"],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode == 0 and result.stdout.strip():
            return [f for f in result.stdout.strip().split("\n") if f.strip()]
        return []
    except Exception as e:
        print(f"Error getting changed files: {e}")
        return []


def check_repository_changes() -> Dict[str, str]:
    """Check for repository changes and return summary"""
    try:
        # Check if there are any changes
        result = subprocess.run(
            ["git", "diff", "--quiet", "HEAD~1", "HEAD"],
            capture_output=True,
            check=False,
        )
        has_changes = result.returncode != 0

        if has_changes:
            print("Repository has changes since last commit")
            changed_files = get_changed_files()

            if changed_files:
                changed_files_list = ",".join(changed_files)
                print(f"Changed files: {changed_files_list}")
                return {"has_changes": "true", "changed_files": changed_files_list}
            else:
                return {"has_changes": "false", "changed_files": ""}
        else:
            print("No changes detected in repository")
            return {"has_changes": "false", "changed_files": ""}
    except Exception as e:
        print(f"Error checking repository changes: {e}")
        return {"has_changes": "false", "changed_files": ""}


def set_github_output(key: str, value: str) -> None:
    """Set GitHub Actions output"""
    github_output = os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a", encoding="utf-8") as f:
            f.write(f"{key}={value}\n")
    else:
        print(f"Would set output: {key}={value}")


def check_recent_prs() -> str:
    """Check for recent PRs that might have been created by this workflow"""
    # Note: This is a placeholder - in practice, you might want to use the GitHub API
    print("Checking for recent PRs...")
    # For now, return empty since we'd need additional setup for GitHub API calls
    return ""


def main():
    """Main entry point"""
    if len(sys.argv) > 1 and sys.argv[1] == "--help":
        print(__doc__)
        print("\nUsage:")
        print("  collect_results.py")
        print("\nOutputs:")
        print("  has_changes - 'true' if files were changed, 'false' otherwise")
        print("  changed_files - comma-separated list of changed file paths")
        print("  recent_prs - information about recent PRs (placeholder)")
        return

    print("Collecting operation results and file changes...")

    # Check for repository changes
    changes = check_repository_changes()
    set_github_output("has_changes", changes["has_changes"])
    set_github_output("changed_files", changes["changed_files"])

    # Check for recent PRs
    recent_prs = check_recent_prs()
    set_github_output("recent_prs", recent_prs)

    print("✅ Operation results collection completed")


if __name__ == "__main__":
    main()
