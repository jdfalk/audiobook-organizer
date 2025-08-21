#!/usr/bin/env python3
"""
Determine Operations

Determines which operations to run based on workflow inputs and context.
"""

import json
import os
import sys
from typing import List


def check_file_exists(file_path: str) -> bool:
    """Check if a file exists"""
    return os.path.isfile(file_path)


def check_directory_has_json_files(directory: str) -> int:
    """Check if directory has JSON files (excluding README.json)"""
    if not os.path.isdir(directory):
        return 0

    count = 0
    for filename in os.listdir(directory):
        if filename.endswith(".json") and filename != "README.json":
            count += 1

    return count


def check_for_files(issue_updates_file: str, issue_updates_directory: str) -> dict:
    """Check for issue update files"""
    has_legacy_file = check_file_exists(issue_updates_file)
    has_update_files = False

    if has_legacy_file:
        print(f"📄 Found legacy issue updates file: {issue_updates_file}")

    json_files_count = check_directory_has_json_files(issue_updates_directory)
    if json_files_count > 0:
        has_update_files = True
        print(
            f"📁 Found {json_files_count} issue update files in {issue_updates_directory}"
        )

    has_issue_updates = has_legacy_file or has_update_files

    if has_issue_updates:
        print("✅ Issue updates found")
    else:
        print("❌ No issue updates found")

    return {
        "has_issue_updates": has_issue_updates,
        "has_legacy_file": has_legacy_file,
        "has_update_files": has_update_files,
    }


def validate_operation(operation: str) -> str:
    """Validate and auto-correct operation names"""
    valid_operations = [
        "update-issues",
        "copilot-tickets",
        "close-duplicates",
        "codeql-alerts",
        "update-permalinks",
    ]

    # Special handling for common typos
    corrections = {
        "comma-separated": None,  # Error case
        "comma,separated": None,  # Error case
        "comma separated": None,  # Error case
        "update-issue": "update-issues",
        "update_issues": "update-issues",
        "copilot-ticket": "copilot-tickets",
        "copilot_tickets": "copilot-tickets",
        "close-duplicate": "close-duplicates",
        "close_duplicates": "close-duplicates",
        "codeql-alert": "codeql-alerts",
        "codeql_alerts": "codeql-alerts",
        "update-permalink": "update-permalinks",
        "update_permalinks": "update-permalinks",
    }

    operation = operation.strip()

    if operation in corrections:
        if corrections[operation] is None:
            raise ValueError(
                f"Invalid operation '{operation}' - found placeholder instead of actual operation names"
            )
        else:
            corrected = corrections[operation]
            print(f"⚠️  Auto-correcting '{operation}' to '{corrected}'")
            return corrected

    if operation in valid_operations:
        print(f"  ✓ Valid operation: {operation}")
        return operation
    else:
        raise ValueError(
            f"Invalid operation '{operation}'. Valid operations: {', '.join(valid_operations)}"
        )


def determine_operations(
    operations_input: str, event_name: str, has_issue_updates: bool
) -> List[str]:
    """Determine which operations to run"""
    operations = []

    # If operations explicitly provided and not 'auto'
    if operations_input != "auto":
        print(f"🎯 Using explicit operations: {operations_input}")
        ops = [op.strip() for op in operations_input.split(",")]

        validated_operations = []
        for op in ops:
            try:
                validated_op = validate_operation(op)
                validated_operations.append(validated_op)
            except ValueError as e:
                print(f"❌ Error: {e}")
                sys.exit(1)

        operations = validated_operations

    # Auto-detect based on event and context
    else:
        print(f"🔍 Auto-detecting operations based on event: {event_name}")

        # Issue updates file exists
        if has_issue_updates:
            operations.append("update-issues")
            print("  ✓ Added update-issues (issue updates file found)")

        # Copilot events
        if event_name.startswith("pull_request"):
            operations.append("copilot-tickets")
            print("  ✓ Added copilot-tickets (PR event detected)")

        # Scheduled events
        if event_name == "schedule":
            operations.extend(["close-duplicates", "codeql-alerts"])
            print("  ✓ Added close-duplicates and codeql-alerts (scheduled event)")

        # Workflow dispatch can run all operations
        if event_name == "workflow_dispatch":
            if not operations:  # Only add defaults if none were auto-detected
                operations.extend(
                    [
                        "update-issues",
                        "copilot-tickets",
                        "close-duplicates",
                        "codeql-alerts",
                        "update-permalinks",
                    ]
                )
                print("  ✓ Added all operations (manual workflow dispatch)")

        # Default fallback if no operations determined
        if not operations:
            print("  ⚠️  No operations auto-detected, using default set")
            operations.append("copilot-tickets")

    return operations


def create_operations_json(operations: List[str]) -> str:
    """Create JSON array for operations matrix"""
    if not operations:
        return "[]"

    # Create JSON array safely
    return json.dumps(operations)


def set_github_output(key: str, value: str) -> None:
    """Set GitHub Actions output"""
    github_output = os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a", encoding="utf-8") as f:
            f.write(f"{key}={value}\n")
    else:
        print(f"Would set output: {key}={value}")


def main():
    """Main entry point"""
    if len(sys.argv) > 1 and sys.argv[1] == "--help":
        print(__doc__)
        print("\nUsage:")
        print("  determine_operations.py")
        print("\nEnvironment variables:")
        print("  OPERATIONS_INPUT - Operations input string")
        print("  EVENT_NAME - GitHub event name")
        print("  ISSUE_UPDATES_FILE - Path to legacy issue updates file")
        print("  ISSUE_UPDATES_DIRECTORY - Path to issue updates directory")
        print("\nOutputs:")
        print("  operations - JSON array of operations to run")
        print("  has_issue_updates - whether issue updates were found")
        return

    # Get inputs from environment
    operations_input = os.environ.get("OPERATIONS_INPUT", "auto")
    event_name = os.environ.get("EVENT_NAME", "workflow_dispatch")
    issue_updates_file = os.environ.get("ISSUE_UPDATES_FILE", "issue_updates.json")
    issue_updates_directory = os.environ.get(
        "ISSUE_UPDATES_DIRECTORY", ".github/issue-updates"
    )

    print(f"🔧 Determining operations for event: {event_name}")
    print(f"📋 Operations input: {operations_input}")

    # Check for files
    file_check = check_for_files(issue_updates_file, issue_updates_directory)

    # Determine operations
    operations = determine_operations(
        operations_input, event_name, file_check["has_issue_updates"]
    )

    # Create JSON output
    operations_json = create_operations_json(operations)

    # Set outputs
    set_github_output("operations", operations_json)
    set_github_output("has_issue_updates", str(file_check["has_issue_updates"]).lower())

    if operations:
        print(f"🎯 Final operations to run: {', '.join(operations)}")
    else:
        print("🎯 No operations to run")

    print(f"📤 Operations JSON: {operations_json}")


if __name__ == "__main__":
    main()
