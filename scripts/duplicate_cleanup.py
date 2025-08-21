#!/usr/bin/env python3
# file: scripts/duplicate_cleanup.py
# version: 1.0.0
# guid: a7b8c9d0-e1f2-3456-7890-abcdef123456
"""
Advanced Duplicate Issue Cleanup Manager

This script identifies closed duplicate issues that are not properly labeled as duplicates
and generates issue update files to delete them with appropriate "Duplicate of #xx" messages.

Features:
- Identifies closed issues with same titles but missing duplicate labels
- Finds original (canonical) issues to reference
- Generates delete actions with "Duplicate of #xx" messages
- Uses proper GUID tracking for delete operations
- Supports both dry-run and execution modes

Usage:
    python scripts/duplicate_cleanup.py scan --dry-run
    python scripts/duplicate_cleanup.py scan --generate-deletes
    python scripts/duplicate_cleanup.py scan --help
"""

import argparse
import json
import os
import re
import sys
import uuid
from collections import defaultdict
from datetime import datetime
from typing import Dict, List, Any, Optional

# Add the scripts directory to the path for imports
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

try:
    from issue_manager import GitHubAPI, OperationSummary
except ImportError:
    print(
        "Error: Could not import required modules from issue_manager.py",
        file=sys.stderr,
    )
    sys.exit(1)


class DuplicateCleanupManager:
    """Advanced manager for identifying and cleaning up duplicate issues."""

    def __init__(self, github_api: GitHubAPI):
        """
        Initialize the duplicate cleanup manager.

        Args:
            github_api: GitHub API client instance
        """
        self.api = github_api
        self.summary = OperationSummary("duplicate-cleanup")

    def scan_and_cleanup(
        self,
        dry_run: bool = True,
        generate_deletes: bool = False,
        output_directory: str = ".github/issue-updates",
    ) -> Dict[str, Any]:
        """
        Scan for duplicate issues and optionally generate cleanup actions.

        Args:
            dry_run: If True, only identify duplicates without generating actions
            generate_deletes: If True, generate delete action files
            output_directory: Directory to store generated update files

        Returns:
            Dictionary with scan results and statistics
        """
        print("🔍 Scanning for closed duplicate issues...")

        # Get all closed issues
        closed_issues = self.api.get_all_issues(state="closed")
        print(f"📋 Found {len(closed_issues)} closed issues")

        # Get all open issues for canonical reference
        open_issues = self.api.get_all_issues(state="open")
        print(f"📋 Found {len(open_issues)} open issues")

        # Combine all issues for title matching
        all_issues = closed_issues + open_issues

        # Find duplicate groups
        duplicate_groups = self._find_duplicate_groups(all_issues)
        print(f"🔍 Found {len(duplicate_groups)} groups with duplicate titles")

        # Identify closed duplicates that need cleanup
        cleanup_candidates = self._identify_cleanup_candidates(duplicate_groups)
        print(f"🧹 Found {len(cleanup_candidates)} closed duplicates needing cleanup")

        if not cleanup_candidates:
            print("✅ No duplicate cleanup needed")
            return {
                "total_groups": len(duplicate_groups),
                "cleanup_candidates": 0,
                "files_generated": 0,
                "dry_run": dry_run,
            }

        # Print summary of findings
        self._print_cleanup_summary(cleanup_candidates, dry_run)

        files_generated = 0
        if generate_deletes and not dry_run:
            files_generated = self._generate_delete_files(
                cleanup_candidates, output_directory
            )

        return {
            "total_groups": len(duplicate_groups),
            "cleanup_candidates": len(cleanup_candidates),
            "files_generated": files_generated,
            "dry_run": dry_run,
        }

    def _find_duplicate_groups(
        self, issues: List[Dict[str, Any]]
    ) -> Dict[str, List[Dict[str, Any]]]:
        """
        Group issues by title to find duplicates.

        Args:
            issues: List of all issues (open and closed)

        Returns:
            Dictionary mapping normalized titles to lists of issues
        """
        title_groups = defaultdict(list)

        for issue in issues:
            # Skip pull requests
            if "pull_request" in issue:
                continue

            title = self._normalize_title(issue["title"])
            if title:  # Skip empty titles
                simplified_issue = {
                    "number": issue["number"],
                    "title": issue["title"],
                    "state": issue["state"],
                    "html_url": issue["html_url"],
                    "created_at": issue["created_at"],
                    "labels": [label["name"] for label in issue.get("labels", [])],
                    "body": issue.get("body", ""),
                }
                title_groups[title].append(simplified_issue)

        # Filter to only groups with multiple issues
        return {
            title: issues for title, issues in title_groups.items() if len(issues) > 1
        }

    def _normalize_title(self, title: str) -> str:
        """
        Normalize issue title for comparison.

        Args:
            title: Raw issue title

        Returns:
            Normalized title string
        """
        if not title:
            return ""

        # Convert to lowercase and strip whitespace
        normalized = title.lower().strip()

        # Remove common punctuation and extra spaces
        normalized = re.sub(r"[^\w\s]", " ", normalized)
        normalized = re.sub(r"\s+", " ", normalized)

        return normalized.strip()

    def _identify_cleanup_candidates(
        self, duplicate_groups: Dict[str, List[Dict[str, Any]]]
    ) -> List[Dict[str, Any]]:
        """
        Identify closed duplicate issues that need cleanup.

        Args:
            duplicate_groups: Groups of issues with same titles

        Returns:
            List of cleanup candidate dictionaries
        """
        cleanup_candidates = []

        for title, issues in duplicate_groups.items():
            # Sort issues by creation date (oldest first)
            issues.sort(key=lambda x: x["created_at"])

            # Find the canonical issue (prefer open, then oldest)
            canonical_issue = self._find_canonical_issue(issues)

            if not canonical_issue:
                continue

            # Find closed duplicates that aren't properly labeled
            for issue in issues:
                if (
                    issue["number"] != canonical_issue["number"]
                    and issue["state"] == "closed"
                    and not self._has_duplicate_label(issue["labels"])
                    and not self._has_duplicate_comment(
                        issue["number"], canonical_issue["number"]
                    )
                ):
                    cleanup_candidates.append(
                        {
                            "duplicate_issue": issue,
                            "canonical_issue": canonical_issue,
                            "title": title,
                        }
                    )

        return cleanup_candidates

    def _find_canonical_issue(
        self, issues: List[Dict[str, Any]]
    ) -> Optional[Dict[str, Any]]:
        """
        Find the canonical (original) issue from a group of duplicates.

        Args:
            issues: List of issues with the same title

        Returns:
            The canonical issue, or None if none found
        """
        # Prefer open issues
        open_issues = [issue for issue in issues if issue["state"] == "open"]
        if open_issues:
            # Return the oldest open issue
            return min(open_issues, key=lambda x: x["created_at"])

        # If no open issues, return the oldest closed issue
        closed_issues = [issue for issue in issues if issue["state"] == "closed"]
        if closed_issues:
            return min(closed_issues, key=lambda x: x["created_at"])

        return None

    def _has_duplicate_label(self, labels: List[str]) -> bool:
        """
        Check if issue has a duplicate-related label.

        Args:
            labels: List of label names

        Returns:
            True if has duplicate label
        """
        duplicate_labels = {"duplicate", "duplicates", "duplicate-issue", "wontfix"}
        return any(label.lower() in duplicate_labels for label in labels)

    def _has_duplicate_comment(self, issue_number: int, canonical_number: int) -> bool:
        """
        Check if issue already has a "Duplicate of #xx" comment.

        Args:
            issue_number: Issue number to check
            canonical_number: Canonical issue number to look for

        Returns:
            True if duplicate comment exists
        """
        try:
            import requests

            url = f"https://api.github.com/repos/{self.api.repo}/issues/{issue_number}/comments"
            response = requests.get(url, headers=self.api.headers, timeout=10)

            if response.status_code != 200:
                return False

            comments = response.json()

            # Look for "Duplicate of #xx" pattern in comments
            duplicate_pattern = re.compile(r"duplicate\s+of\s+#(\d+)", re.IGNORECASE)

            for comment in comments:
                comment_body = comment.get("body", "")
                if duplicate_pattern.search(comment_body):
                    return True

            return False

        except Exception as e:
            print(f"⚠️  Error checking comments for issue #{issue_number}: {e}")
            return False

    def _print_cleanup_summary(
        self, cleanup_candidates: List[Dict[str, Any]], dry_run: bool
    ) -> None:
        """
        Print a summary of cleanup candidates.

        Args:
            cleanup_candidates: List of issues that need cleanup
            dry_run: Whether this is a dry run
        """
        print(f"\n{'🧪 DRY RUN - ' if dry_run else ''}🧹 DUPLICATE CLEANUP SUMMARY")
        print("=" * 60)

        if not cleanup_candidates:
            print("✅ No duplicate cleanup needed")
            return

        print(
            f"📋 Found {len(cleanup_candidates)} closed duplicates that need cleanup:"
        )

        # Group by canonical issue for better display
        by_canonical = defaultdict(list)
        for candidate in cleanup_candidates:
            canonical_num = candidate["canonical_issue"]["number"]
            by_canonical[canonical_num].append(candidate)

        for canonical_num, candidates in by_canonical.items():
            canonical = candidates[0]["canonical_issue"]
            print(
                f"\n📌 Canonical Issue #{canonical_num}: {canonical['title'][:50]}..."
            )
            print(f"   Status: {canonical['state'].upper()}")
            print(f"   URL: {canonical['html_url']}")

            print(f"   🗑️  Duplicates to delete ({len(candidates)}):")
            for candidate in candidates:
                dup = candidate["duplicate_issue"]
                print(f"      • #{dup['number']}: {dup['title'][:40]}...")
                print(f"        Created: {dup['created_at'][:10]}")
                print(f"        URL: {dup['html_url']}")

        if dry_run:
            print("\n💡 To generate delete files, run with --generate-deletes")

        print("=" * 60)

    def _generate_delete_files(
        self, cleanup_candidates: List[Dict[str, Any]], output_directory: str
    ) -> int:
        """
        Generate individual delete action files for cleanup candidates.

        Args:
            cleanup_candidates: List of issues that need cleanup
            output_directory: Directory to store generated files

        Returns:
            Number of files generated
        """
        print(f"\n📁 Generating delete files in: {output_directory}")

        # Create output directory if it doesn't exist
        os.makedirs(output_directory, exist_ok=True)

        files_generated = 0

        for candidate in cleanup_candidates:
            duplicate_issue = candidate["duplicate_issue"]
            canonical_issue = candidate["canonical_issue"]

            # Generate file content
            delete_action = self._create_delete_action(duplicate_issue, canonical_issue)

            # Generate filename with UUID
            action_uuid = str(uuid.uuid4())
            filename = (
                f"delete-duplicate-{duplicate_issue['number']}-{action_uuid[:8]}.json"
            )
            file_path = os.path.join(output_directory, filename)

            try:
                with open(file_path, "w", encoding="utf-8") as f:
                    json.dump(delete_action, f, indent=2)

                print(f"📄 Created: {filename}")
                files_generated += 1

                # Record in summary
                self.summary.add_file_processed(file_path)

            except Exception as e:
                error_msg = f"Failed to create file {filename}: {e}"
                print(f"❌ {error_msg}")
                self.summary.add_error(error_msg)

        print(f"\n✅ Generated {files_generated} delete action files")
        return files_generated

    def _create_delete_action(
        self, duplicate_issue: Dict[str, Any], canonical_issue: Dict[str, Any]
    ) -> Dict[str, Any]:
        """
        Create a delete action object for a duplicate issue.

        Args:
            duplicate_issue: The duplicate issue to delete
            canonical_issue: The canonical issue to reference

        Returns:
            Delete action dictionary
        """
        # Generate UUIDs for tracking
        action_guid = str(uuid.uuid4())
        legacy_guid = f"delete-duplicate-{duplicate_issue['number']}-{datetime.now().strftime('%Y-%m-%d')}"

        delete_action = {
            "action": "delete",
            "number": duplicate_issue["number"],
            "guid": action_guid,
            "legacy_guid": legacy_guid,
            "reason": f"Duplicate of #{canonical_issue['number']}",
            "duplicate_of": canonical_issue["number"],
            "metadata": {
                "duplicate_title": duplicate_issue["title"],
                "canonical_title": canonical_issue["title"],
                "canonical_url": canonical_issue["html_url"],
                "duplicate_created": duplicate_issue["created_at"],
                "canonical_created": canonical_issue["created_at"],
                "cleanup_generated": datetime.now().isoformat(),
                "cleanup_reason": "Closed duplicate without proper labeling",
            },
        }

        return delete_action

    def generate_comment_actions(
        self,
        cleanup_candidates: List[Dict[str, Any]],
        output_directory: str = ".github/issue-updates",
    ) -> int:
        """
        Generate comment actions to add "Duplicate of #xx" comments before deletion.

        Args:
            cleanup_candidates: List of issues that need cleanup
            output_directory: Directory to store generated files

        Returns:
            Number of comment files generated
        """
        print("\n💬 Generating comment files for duplicate references...")

        # Create output directory if it doesn't exist
        os.makedirs(output_directory, exist_ok=True)

        files_generated = 0

        for candidate in cleanup_candidates:
            duplicate_issue = candidate["duplicate_issue"]
            canonical_issue = candidate["canonical_issue"]

            # Generate comment action
            comment_action = self._create_comment_action(
                duplicate_issue, canonical_issue
            )

            # Generate filename with UUID
            action_uuid = str(uuid.uuid4())
            filename = (
                f"comment-duplicate-{duplicate_issue['number']}-{action_uuid[:8]}.json"
            )
            file_path = os.path.join(output_directory, filename)

            try:
                with open(file_path, "w", encoding="utf-8") as f:
                    json.dump(comment_action, f, indent=2)

                print(f"💬 Created: {filename}")
                files_generated += 1

                # Record in summary
                self.summary.add_file_processed(file_path)

            except Exception as e:
                error_msg = f"Failed to create comment file {filename}: {e}"
                print(f"❌ {error_msg}")
                self.summary.add_error(error_msg)

        print(f"\n✅ Generated {files_generated} comment action files")
        return files_generated

    def _create_comment_action(
        self, duplicate_issue: Dict[str, Any], canonical_issue: Dict[str, Any]
    ) -> Dict[str, Any]:
        """
        Create a comment action object for a duplicate reference.

        Args:
            duplicate_issue: The duplicate issue to comment on
            canonical_issue: The canonical issue to reference

        Returns:
            Comment action dictionary
        """
        # Generate UUIDs for tracking
        action_guid = str(uuid.uuid4())
        legacy_guid = f"comment-duplicate-{duplicate_issue['number']}-{datetime.now().strftime('%Y-%m-%d')}"

        comment_body = f"Duplicate of #{canonical_issue['number']}\n\nThis issue is being marked as a duplicate and will be deleted to clean up the issue tracker."

        comment_action = {
            "action": "comment",
            "number": duplicate_issue["number"],
            "body": comment_body,
            "guid": action_guid,
            "legacy_guid": legacy_guid,
            "metadata": {
                "duplicate_of": canonical_issue["number"],
                "canonical_url": canonical_issue["html_url"],
                "comment_purpose": "Mark as duplicate before deletion",
                "cleanup_generated": datetime.now().isoformat(),
            },
        }

        return comment_action


def create_github_api() -> GitHubAPI:
    """Create and verify GitHub API client."""
    # Get GitHub token from environment
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if not token:
        print(
            "Error: GITHUB_TOKEN or GH_TOKEN environment variable required",
            file=sys.stderr,
        )
        sys.exit(1)

    # Get repository from environment or default
    repo = os.environ.get("GITHUB_REPOSITORY")
    if not repo:
        print("Error: GITHUB_REPOSITORY environment variable required", file=sys.stderr)
        print("       Format: owner/repository-name", file=sys.stderr)
        sys.exit(1)

    # Create and test API client
    api = GitHubAPI(token, repo)
    if not api.test_access():
        print("Error: Failed to access GitHub API", file=sys.stderr)
        sys.exit(1)

    return api


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Advanced Duplicate Issue Cleanup Manager",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Scan for duplicates (dry run)
  python scripts/duplicate_cleanup.py scan --dry-run

  # Generate delete files for duplicates
  python scripts/duplicate_cleanup.py scan --generate-deletes

  # Generate both comments and deletes
  python scripts/duplicate_cleanup.py scan --generate-deletes --add-comments

Environment Variables:
  GITHUB_TOKEN      GitHub personal access token
  GITHUB_REPOSITORY Repository in owner/name format
        """,
    )

    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Scan command
    scan_parser = subparsers.add_parser("scan", help="Scan for duplicate issues")
    scan_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Only identify duplicates without generating actions (default: True)",
    )
    scan_parser.add_argument(
        "--generate-deletes",
        action="store_true",
        help="Generate delete action files for identified duplicates",
    )
    scan_parser.add_argument(
        "--add-comments",
        action="store_true",
        help="Also generate comment actions to mark duplicates before deletion",
    )
    scan_parser.add_argument(
        "--output-dir",
        default=".github/issue-updates",
        help="Directory to store generated update files (default: .github/issue-updates)",
    )

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        sys.exit(1)

    # Create GitHub API client
    try:
        api = create_github_api()
    except Exception as e:
        print(f"Error creating GitHub API client: {e}", file=sys.stderr)
        sys.exit(1)

    # Execute command
    if args.command == "scan":
        manager = DuplicateCleanupManager(api)

        # Determine if this is really a dry run
        is_dry_run = args.dry_run or not args.generate_deletes

        try:
            # Scan for duplicates
            results = manager.scan_and_cleanup(
                dry_run=is_dry_run,
                generate_deletes=args.generate_deletes,
                output_directory=args.output_dir,
            )

            # Generate comment actions if requested
            if args.add_comments and args.generate_deletes and not is_dry_run:
                # This would require refactoring to return candidates
                print("💡 Comment generation would be implemented here")

            # Print final summary
            print("\n🎯 DUPLICATE CLEANUP RESULTS")
            print("=" * 40)
            print(f"📊 Total duplicate groups: {results['total_groups']}")
            print(f"🧹 Cleanup candidates: {results['cleanup_candidates']}")
            print(f"📄 Files generated: {results['files_generated']}")
            print(f"🧪 Dry run mode: {results['dry_run']}")

            if results["cleanup_candidates"] > 0 and results["dry_run"]:
                print("\n💡 To generate action files, run:")
                print("   python scripts/duplicate_cleanup.py scan --generate-deletes")

        except Exception as e:
            print(f"Error during duplicate cleanup: {e}", file=sys.stderr)
            sys.exit(1)

    else:
        print(f"Unknown command: {args.command}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
