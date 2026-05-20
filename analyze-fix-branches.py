#!/usr/bin/env python3
"""
Analyze fix/* branches to understand their status and content.
Shows commit history, conflicts, and whether they're still needed.
"""
import subprocess
import sys


def run(cmd, capture=False):
    """Run a shell command."""
    try:
        if capture:
            result = subprocess.run(cmd, shell=True, capture_output=True, text=True, check=True)
            return result.stdout.strip()
        else:
            subprocess.run(cmd, shell=True, check=True)
            return None
    except subprocess.CalledProcessError:
        return None


def get_fix_branches():
    """Get all fix/* branches."""
    output = run("git branch --list 'fix/*' --format='%(refname:short)'", capture=True)
    if not output:
        return []
    return sorted([b.strip() for b in output.split('\n') if b.strip()])


def get_branch_info(branch):
    """Get info about a branch."""
    # Get commits ahead of main
    commits_ahead = run(f"git rev-list --count origin/main..{branch}", capture=True)

    # Get last commit message
    last_msg = run(f"git log -1 --format=%s {branch}", capture=True)

    # Get last commit date
    last_date = run(f"git log -1 --format=%ai {branch}", capture=True)

    # Check if branch has been merged
    merged = run(f"git branch --merged origin/main | grep -q '{branch}'", capture=False)

    return {
        'commits_ahead': commits_ahead or '0',
        'last_msg': last_msg or '(unknown)',
        'last_date': last_date or '(unknown)',
        'merged': merged is None
    }


def main():
    # Fetch latest
    print("Fetching origin...")
    run("git fetch origin")

    branches = get_fix_branches()
    print(f"\nFound {len(branches)} fix/* branches:\n")
    print(f"{'Branch':<50} {'Commits':<10} {'Last Update':<20} {'Status'}")
    print("=" * 100)

    for branch in branches:
        info = get_branch_info(branch)
        status = "MERGED" if info['merged'] else "ACTIVE"

        # Truncate branch name if too long
        branch_display = branch if len(branch) <= 48 else branch[:45] + "..."

        print(f"{branch_display:<50} {info['commits_ahead']:<10} {info['last_date'][:10]:<20} {status}")
        print(f"  └─ {info['last_msg'][:80]}")

    return 0


if __name__ == '__main__':
    sys.exit(main())
