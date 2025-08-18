#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-check-changes.py
# version: 1.0.0
# guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

"""
Check if there are changes after sync operation.
Usage: sync-receiver-check-changes.py
"""

import subprocess
import sys


def main():
    """Check if there are changes after sync operation."""
    try:
        # Check git status for changes
        result = subprocess.run(
            ["git", "status", "--porcelain"], capture_output=True, text=True, check=True
        )

        if result.stdout.strip():
            print("true")
            print("Changes detected after sync operation")
        else:
            print("false")
            print("No changes detected")

    except subprocess.CalledProcessError as e:
        print(f"Error checking git status: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
