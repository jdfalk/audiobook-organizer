#!/usr/bin/env python3
"""
Workflow Summary Generator

Generates comprehensive summaries for GitHub Actions workflows,
specifically for the unified issue management workflow.
"""

import json
import os
import subprocess
import sys
from datetime import datetime
from typing import List


def write_summary(content: str) -> None:
    """Write content to GitHub Step Summary"""
    summary_file = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary_file:
        with open(summary_file, "a", encoding="utf-8") as f:
            f.write(content + "\n")
    else:
        print(content)


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
            return result.stdout.strip().split("\n")
        return []
    except Exception:
        return []


def safe_json_parse(json_str: str, fallback=None) -> any:
    """Safely parse JSON string with fallback"""
    if not json_str or json_str.strip() == "":
        return fallback if fallback is not None else []
    try:
        return json.loads(json_str)
    except json.JSONDecodeError as e:
        print(f"Warning: Could not parse JSON '{json_str}': {e}")
        return fallback if fallback is not None else []


def get_env_var(name: str, default: str = "") -> str:
    """Get environment variable safely"""
    return os.environ.get(name, default)


def generate_workflow_summary() -> None:
    """Generate comprehensive workflow summary"""
    # Get workflow inputs and context from environment variables
    operations_json = get_env_var("OPERATIONS_JSON")
    run_id = get_env_var("GITHUB_RUN_ID")
    repository = get_env_var("GITHUB_REPOSITORY")
    event_name = get_env_var("EVENT_NAME")
    triggered_by = get_env_var("TRIGGERED_BY")
    issue_management_result = get_env_var("ISSUE_MANAGEMENT_RESULT", "success")

    # Configuration
    operations_mode = get_env_var("OPERATIONS_MODE")
    dry_run = get_env_var("DRY_RUN", "false")
    force_update = get_env_var("FORCE_UPDATE", "false")
    issue_updates_file = get_env_var("ISSUE_UPDATES_FILE")
    issue_updates_directory = get_env_var("ISSUE_UPDATES_DIRECTORY")

    # Parse operations with safer handling
    operations = safe_json_parse(operations_json, [])

    # Debug information
    print(f"Debug: operations_json = '{operations_json}'")
    print(f"Debug: parsed operations = {operations}")

    # Get changed files
    changed_files = get_changed_files()

    # Generate summary
    write_summary("# 🚀 Unified Issue Management Workflow")
    write_summary("")
    write_summary(
        f"**Run ID:** [`{run_id}`](https://github.com/{repository}/actions/runs/{run_id})"
    )
    write_summary(f"**Repository:** [`{repository}`](https://github.com/{repository})")
    write_summary(f"**Triggered by:** {event_name}")
    write_summary(f"**Actor:** {triggered_by}")
    write_summary(
        f"**Timestamp:** {datetime.utcnow().strftime('%Y-%m-%d %H:%M:%S UTC')}"
    )
    write_summary("")

    # Show overall workflow status
    if not operations:
        write_summary("## 📝 Result")
        write_summary("ℹ️  No operations were required for this event.")
    else:
        write_summary("## 📊 Workflow Status")
        write_summary("")

        if issue_management_result == "success":
            write_summary("✅ **Status:** Completed successfully")
            write_summary("")
            write_summary(f"📋 **Operations executed:** `{', '.join(operations)}`")
            write_summary("")

            # Show results table
            write_summary("## 📈 Summary of Results")
            write_summary("")
            write_summary("| Operation | Status | Details |")
            write_summary("|-----------|--------|---------|")

            operation_details = {
                "update-issues": "Processed issue update files and created archive PRs",
                "copilot-tickets": "Managed review comment tickets",
                "close-duplicates": "Scanned for duplicate issues",
                "codeql-alerts": "Generated tickets for security alerts",
                "update-permalinks": "Updated permalinks in processed files",
            }

            operation_icons = {
                "update-issues": "📝",
                "copilot-tickets": "🤖",
                "close-duplicates": "🔄",
                "codeql-alerts": "🔒",
                "update-permalinks": "🔗",
            }

            for operation in operations:
                icon = operation_icons.get(operation, "❓")
                detail = operation_details.get(operation, "Custom operation executed")
                operation_title = operation.replace("-", " ").title()
                write_summary(f"| {icon} {operation_title} | ✅ Completed | {detail} |")

            write_summary("")
            write_summary(f"**Total operations:** {len(operations)}")

            # Show file changes
            if changed_files:
                write_summary("**Repository changes:** ✅ Files were modified")
                write_summary("")
                write_summary("### 📁 Files Modified")
                for file in changed_files:
                    if file.strip():
                        write_summary(f"- `{file.strip()}`")
            else:
                write_summary("**Repository changes:** ℹ️  No files were modified")

            write_summary("")
            write_summary(
                "ℹ️  Detailed operation results are shown above in individual job summaries."
            )

        elif issue_management_result == "failure":
            write_summary("❌ **Status:** Failed")
            write_summary("")
            write_summary(f"📋 **Operations attempted:** `{', '.join(operations)}`")
            write_summary("")
            write_summary("🔍 Check the job logs above for detailed error information.")
        else:
            write_summary("⏭️ **Status:** Skipped")
            write_summary("")
            write_summary(f"📋 **Operations requested:** `{', '.join(operations)}`")

    # Show configuration
    write_summary("")
    write_summary("## ⚙️ Configuration")
    write_summary(f"- **Operations mode:** `{operations_mode}`")
    write_summary(f"- **Dry run:** {dry_run}")
    write_summary(f"- **Force update:** {force_update}")
    write_summary(f"- **Issue updates file:** `{issue_updates_file}`")
    write_summary(f"- **Issue updates directory:** `{issue_updates_directory}`")

    # Show quick links
    write_summary("")
    write_summary("## 🔗 Quick Links")
    write_summary(f"- [🔄 Workflow runs](https://github.com/{repository}/actions)")
    write_summary(f"- [🐛 Issues](https://github.com/{repository}/issues)")
    write_summary(f"- [🔒 Security alerts](https://github.com/{repository}/security)")
    write_summary(f"- [📋 Pull requests](https://github.com/{repository}/pulls)")
    write_summary("- [🏠 ghcommon repository](https://github.com/jdfalk/ghcommon)")

    # Show recent activity links
    write_summary("")
    write_summary("## 🕒 Recent Activity")
    write_summary(
        f"- [🆕 Recent issues](https://github.com/{repository}/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc)"
    )
    write_summary(
        f"- [🔄 Recent PRs](https://github.com/{repository}/pulls?q=is%3Apr+is%3Aopen+sort%3Aupdated-desc)"
    )
    write_summary(
        f"- [🏃 Recent workflow runs](https://github.com/{repository}/actions?query=workflow%3A%22Issue+Management%22)"
    )

    write_summary("")
    write_summary("---")
    write_summary(
        "*This workflow uses the [Unified Issue Management Workflow](https://github.com/jdfalk/ghcommon/.github/workflows/reusable-unified-issue-management.yml) from ghcommon.*"
    )


def main():
    """Main entry point"""
    if len(sys.argv) > 1 and sys.argv[1] == "--help":
        print(__doc__)
        print("\nUsage:")
        print("  workflow_summary.py")
        print("\nEnvironment variables:")
        print("  OPERATIONS_JSON - JSON array of operations")
        print("  GITHUB_RUN_ID - GitHub Actions run ID")
        print("  GITHUB_REPOSITORY - Repository name")
        print("  EVENT_NAME - Event that triggered the workflow")
        print("  TRIGGERED_BY - Actor who triggered the workflow")
        print("  ISSUE_MANAGEMENT_RESULT - Result of issue management job")
        print("  And other configuration variables...")
        return

    generate_workflow_summary()


if __name__ == "__main__":
    main()
