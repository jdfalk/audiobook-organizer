#!/usr/bin/env python3
# file: .github/scripts/sync-release-run-semantic-release.py
# version: 1.0.0
# guid: b3c4d5e6-f7a8-b9c0-d1e2-f3a4b5c6d7e8

"""Run semantic-release with proper environment and configuration."""

import subprocess
import sys
import os


def main():
    """Run semantic-release."""
    # Ensure npm dependencies are installed
    print("Installing npm dependencies...")
    result = subprocess.run(["npm", "install"], capture_output=True, text=True)
    if result.returncode != 0:
        print(f"Failed to install npm dependencies: {result.stderr}")
        sys.exit(1)

    # Run semantic-release
    print("Running semantic-release...")
    env = os.environ.copy()
    result = subprocess.run(["npx", "semantic-release"], env=env)
    sys.exit(result.returncode)


if __name__ == "__main__":
    main()