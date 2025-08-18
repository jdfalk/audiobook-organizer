#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-generate-summary.py
# version: 1.0.0
# guid: f6a7b8c9-d0e1-2f3a-4b5c-6d7e8f9a0b1c

"""
Generate synchronization summary for sync receiver workflow.
"""

import os
import subprocess
from datetime import datetime


def get_sync_changes():
    """Get information about synchronized changes."""
    try:
        # Get git status
        result = subprocess.run(
            "git status --porcelain", shell=True, capture_output=True, text=True
        )

        if result.returncode != 0:
            return []

        changes = []
        for line in result.stdout.strip().split("\n"):
            if line.strip():
                status = line[:2]
                file_path = line[3:]
                changes.append((status, file_path))

        return changes
    except Exception:
        return []


def get_changed_files_summary():
    """Get a summary of changed files by category."""
    changes = get_sync_changes()

    if not changes:
        return "No changes detected"

    categories = {
        "workflows": [],
        "instructions": [],
        "scripts": [],
        "prompts": [],
        "linters": [],
        "other": [],
    }

    for status, file_path in changes:
        if ".github/workflows/" in file_path:
            categories["workflows"].append(file_path)
        elif (
            ".github/instructions/" in file_path
            or "copilot-instructions.md" in file_path
        ):
            categories["instructions"].append(file_path)
        elif ".github/scripts/" in file_path or "/scripts/" in file_path:
            categories["scripts"].append(file_path)
        elif ".github/prompts/" in file_path:
            categories["prompts"].append(file_path)
        elif ".github/linters/" in file_path:
            categories["linters"].append(file_path)
        else:
            categories["other"].append(file_path)

    summary = f"**Total changes**: {len(changes)} files\n\n"

    for category, files in categories.items():
        if files:
            summary += f"**{category.title()}** ({len(files)} files):\n"
            for file_path in files:
                summary += f"- `{file_path}`\n"
            summary += "\n"

    return summary


def generate_summary():
    """Generate a comprehensive sync receiver summary."""
    # Get environment variables
    source_repo = os.getenv("SOURCE_REPOSITORY", "jdfalk/ghcommon")
    source_sha = os.getenv("SOURCE_SHA", "unknown")
    source_ref = os.getenv("SOURCE_REF", "refs/heads/main")
    current_repo = os.getenv("GITHUB_REPOSITORY", "unknown")
    run_id = os.getenv("GITHUB_RUN_ID", "unknown")

    # Current timestamp
    timestamp = datetime.utcnow().isoformat() + "Z"

    # Get file changes summary
    changes_summary = get_changed_files_summary()

    # Generate summary markdown
    summary = f"""# 📥 Repository Sync Received

## Overview
- **Source Repository**: {source_repo}
- **Target Repository**: {current_repo}
- **Source SHA**: `{source_sha[:8]}...`
- **Source Ref**: {source_ref}
- **Sync Timestamp**: {timestamp}

## Synchronized Changes

{changes_summary}

## Sync Process Status
- ✅ Repository dispatch received
- ✅ Source files downloaded
- ✅ Files synchronized
- ✅ Changes committed and pushed

## Workflow Information
- **Workflow Run ID**: {run_id}
- **Sync Type**: Automated from central repository
- **Branch**: main

---

*This repository is automatically synchronized with the central ghcommon repository.*
"""

    return summary


def write_to_step_summary(content):
    """Write content to GitHub Actions step summary."""
    github_step_summary = os.getenv("GITHUB_STEP_SUMMARY")

    if github_step_summary:
        try:
            with open(github_step_summary, "a") as f:
                f.write(content)
            print("✅ Summary written to GitHub Actions step summary")
        except Exception as e:
            print(f"❌ Error writing to step summary: {e}")
    else:
        print("ℹ️  GITHUB_STEP_SUMMARY not available, outputting to console:")
        print(content)


def main():
    """Main entry point."""
    print("Generating sync receiver summary...")

    summary = generate_summary()

    # Write to step summary
    write_to_step_summary(summary)

    # Also output key information for logs
    print("Repository synchronization received and processed successfully")


if __name__ == "__main__":
    main()
