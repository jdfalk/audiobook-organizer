#!/usr/bin/env python3
"""
# file: scripts/label_manager.py
# version: 1.0.0
# guid: a8b9c0d1-e2f3-4567-8901-234567890abc

GitHub label management script for standardizing labels across repositories.

This script provides functionality to:
1. Sync labels from a configuration file to target repositories
2. Create, update, or delete labels as needed
3. Support dry-run mode for testing
4. Handle multiple repositories in a single operation

Environment Variables:
  GH_TOKEN or GITHUB_TOKEN - GitHub token with repo access

Usage:
  export GH_TOKEN=$(gh auth token)
  python label_manager.py sync-labels --config labels.json --repos "owner/repo1,owner/repo2"
  python label_manager.py sync-labels --config labels.json --repos-file repos.txt --dry-run
"""

import argparse
import json
import os
import sys
from typing import Any, Dict, List, Optional
from urllib.parse import quote

try:
    import requests
except ImportError:
    print("Error: 'requests' module not found. Installing it now...", file=sys.stderr)
    import subprocess

    try:
        subprocess.check_call(["pip3", "install", "requests", "--quiet"])
        import requests

        print("✓ Successfully installed and imported 'requests' module")
    except subprocess.CalledProcessError as e:
        print(f"Failed to install 'requests' module: {e}", file=sys.stderr)
        sys.exit(1)

# GitHub API version
API_VERSION = "2022-11-28"


class LabelSyncResult:
    """Track results of label synchronization operation."""

    def __init__(self):
        self.created = []
        self.updated = []
        self.deleted = []
        self.skipped = []
        self.errors = []

    def add_created(self, label_name: str):
        self.created.append(label_name)

    def add_updated(self, label_name: str):
        self.updated.append(label_name)

    def add_deleted(self, label_name: str):
        self.deleted.append(label_name)

    def add_skipped(self, label_name: str, reason: str):
        self.skipped.append(f"{label_name} ({reason})")

    def add_error(self, error: str):
        self.errors.append(error)

    def get_summary(self) -> str:
        """Return a summary of the sync operation."""
        lines = []

        if self.created:
            lines.append(f"### ✅ Created ({len(self.created)})")
            for label in self.created:
                lines.append(f"- {label}")
            lines.append("")

        if self.updated:
            lines.append(f"### 🔄 Updated ({len(self.updated)})")
            for label in self.updated:
                lines.append(f"- {label}")
            lines.append("")

        if self.deleted:
            lines.append(f"### 🗑️ Deleted ({len(self.deleted)})")
            for label in self.deleted:
                lines.append(f"- {label}")
            lines.append("")

        if self.skipped:
            lines.append(f"### ⏭️ Skipped ({len(self.skipped)})")
            for item in self.skipped:
                lines.append(f"- {item}")
            lines.append("")

        if self.errors:
            lines.append(f"### ❌ Errors ({len(self.errors)})")
            for error in self.errors:
                lines.append(f"- {error}")
            lines.append("")

        return "\n".join(lines)


class GitHubLabelAPI:
    """GitHub API client for label management."""

    def __init__(self, token: str, repo: str):
        """
        Initialize GitHub API client.

        Args:
            token: GitHub personal access token
            repo: Repository in owner/name format
        """
        self.token = token
        self.repo = repo
        self.headers = self._get_headers()

    def _get_headers(self) -> Dict[str, str]:
        """Return HTTP headers for the GitHub API."""
        # Detect token type and set appropriate authorization header
        if self.token.startswith("github_pat_"):
            auth_header = f"token {self.token}"
        else:
            auth_header = f"Bearer {self.token}"

        return {
            "Authorization": auth_header,
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": API_VERSION,
        }

    def test_access(self) -> bool:
        """Test API access and permissions."""
        try:
            url = f"https://api.github.com/repos/{self.repo}"
            response = requests.get(url, headers=self.headers, timeout=10)

            if response.status_code == 401:
                print("Error: Invalid or expired GitHub token", file=sys.stderr)
                return False
            elif response.status_code == 404:
                print(
                    f"Error: Repository '{self.repo}' not found or not accessible",
                    file=sys.stderr,
                )
                return False

            response.raise_for_status()
            return True
        except requests.RequestException as e:
            print(f"Error testing GitHub API access: {e}", file=sys.stderr)
            return False

    def get_labels(self) -> Optional[List[Dict[str, Any]]]:
        """Get all labels from the repository."""
        try:
            url = f"https://api.github.com/repos/{self.repo}/labels"
            all_labels = []
            page = 1

            while True:
                response = requests.get(
                    url,
                    headers=self.headers,
                    params={"page": page, "per_page": 100},
                    timeout=10,
                )
                response.raise_for_status()

                labels = response.json()
                if not labels:
                    break

                all_labels.extend(labels)
                page += 1

            return all_labels
        except requests.RequestException as e:
            print(f"Error fetching labels: {e}", file=sys.stderr)
            return None

    def create_label(self, name: str, color: str, description: str = "") -> bool:
        """Create a new label."""
        try:
            url = f"https://api.github.com/repos/{self.repo}/labels"
            data = {"name": name, "color": color, "description": description}

            response = requests.post(url, headers=self.headers, json=data, timeout=10)
            if response.status_code == 201:
                return True
            else:
                print(
                    f"Failed to create label '{name}': {response.status_code}",
                    file=sys.stderr,
                )
                if response.status_code == 422:
                    error_data = response.json()
                    print(f"Details: {error_data}", file=sys.stderr)
                return False
        except requests.RequestException as e:
            print(f"Network error creating label '{name}': {e}", file=sys.stderr)
            return False

    def update_label(
        self,
        name: str,
        new_name: str = None,
        color: str = None,
        description: str = None,
    ) -> bool:
        """Update an existing label."""
        try:
            # URL encode the label name to handle special characters
            encoded_name = quote(name, safe="")
            url = f"https://api.github.com/repos/{self.repo}/labels/{encoded_name}"

            data = {}
            if new_name is not None:
                data["name"] = new_name
            if color is not None:
                data["color"] = color
            if description is not None:
                data["description"] = description

            response = requests.patch(url, headers=self.headers, json=data, timeout=10)
            if response.status_code == 200:
                return True
            else:
                print(
                    f"Failed to update label '{name}': {response.status_code}",
                    file=sys.stderr,
                )
                return False
        except requests.RequestException as e:
            print(f"Network error updating label '{name}': {e}", file=sys.stderr)
            return False

    def delete_label(self, name: str) -> bool:
        """Delete a label."""
        try:
            # URL encode the label name to handle special characters
            encoded_name = quote(name, safe="")
            url = f"https://api.github.com/repos/{self.repo}/labels/{encoded_name}"

            response = requests.delete(url, headers=self.headers, timeout=10)
            if response.status_code == 204:
                return True
            else:
                print(
                    f"Failed to delete label '{name}': {response.status_code}",
                    file=sys.stderr,
                )
                return False
        except requests.RequestException as e:
            print(f"Network error deleting label '{name}': {e}", file=sys.stderr)
            return False


class LabelManager:
    """Manage label synchronization across repositories."""

    def __init__(self, token: str, dry_run: bool = False):
        """
        Initialize label manager.

        Args:
            token: GitHub personal access token
            dry_run: If True, show what would be done without making changes
        """
        self.token = token
        self.dry_run = dry_run

    def load_label_config(self, config_file: str) -> Optional[List[Dict[str, Any]]]:
        """Load label configuration from JSON file."""
        try:
            with open(config_file, "r", encoding="utf-8") as f:
                labels = json.load(f)

            # Validate label structure
            for i, label in enumerate(labels):
                if not isinstance(label, dict):
                    print(f"Error: Label {i} is not a valid object", file=sys.stderr)
                    return None

                required_fields = ["name", "color"]
                for field in required_fields:
                    if field not in label:
                        print(
                            f"Error: Label {i} missing required field '{field}'",
                            file=sys.stderr,
                        )
                        return None

                # Ensure description exists
                if "description" not in label:
                    label["description"] = ""

            return labels
        except FileNotFoundError:
            print(
                f"Error: Configuration file '{config_file}' not found", file=sys.stderr
            )
            return None
        except json.JSONDecodeError as e:
            print(f"Error: Invalid JSON in configuration file: {e}", file=sys.stderr)
            return None

    def sync_labels_to_repo(
        self, repo: str, target_labels: List[Dict[str, Any]], delete_extra: bool = False
    ) -> LabelSyncResult:
        """
        Sync labels to a single repository.

        Args:
            repo: Repository in owner/name format
            target_labels: List of label configurations to sync
            delete_extra: Whether to delete labels not in target_labels

        Returns:
            LabelSyncResult with operation details
        """
        result = LabelSyncResult()

        print(f"\n🔄 Syncing labels to {repo}")
        if self.dry_run:
            print("   (DRY RUN - no changes will be made)")

        # Initialize API client
        api = GitHubLabelAPI(self.token, repo)

        # Test access
        if not api.test_access():
            result.add_error(f"Cannot access repository {repo}")
            return result

        # Get current labels
        current_labels = api.get_labels()
        if current_labels is None:
            result.add_error(f"Failed to fetch current labels from {repo}")
            return result

        # Create lookup maps
        current_label_map = {label["name"]: label for label in current_labels}
        target_label_map = {label["name"]: label for label in target_labels}

        # Process target labels (create or update)
        for target_label in target_labels:
            name = target_label["name"]
            color = target_label["color"]
            description = target_label.get("description", "")

            if name in current_label_map:
                # Check if update is needed
                current = current_label_map[name]
                needs_update = (
                    current["color"] != color
                    or current.get("description", "") != description
                )

                if needs_update:
                    if self.dry_run:
                        print(f"   Would update: {name}")
                        result.add_updated(name)
                    else:
                        if api.update_label(name, color=color, description=description):
                            print(f"   ✅ Updated: {name}")
                            result.add_updated(name)
                        else:
                            result.add_error(f"Failed to update label: {name}")
                else:
                    result.add_skipped(name, "no changes needed")
            else:
                # Create new label
                if self.dry_run:
                    print(f"   Would create: {name}")
                    result.add_created(name)
                else:
                    if api.create_label(name, color, description):
                        print(f"   ✅ Created: {name}")
                        result.add_created(name)
                    else:
                        result.add_error(f"Failed to create label: {name}")

        # Delete extra labels if requested
        if delete_extra:
            for current_name in current_label_map:
                if current_name not in target_label_map:
                    if self.dry_run:
                        print(f"   Would delete: {current_name}")
                        result.add_deleted(current_name)
                    else:
                        if api.delete_label(current_name):
                            print(f"   🗑️ Deleted: {current_name}")
                            result.add_deleted(current_name)
                        else:
                            result.add_error(f"Failed to delete label: {current_name}")

        return result

    def sync_labels_to_repos(
        self, repos: List[str], config_file: str, delete_extra: bool = False
    ) -> Dict[str, LabelSyncResult]:
        """
        Sync labels to multiple repositories.

        Args:
            repos: List of repositories in owner/name format
            config_file: Path to label configuration file
            delete_extra: Whether to delete labels not in configuration

        Returns:
            Dictionary mapping repo names to their sync results
        """
        # Load configuration
        target_labels = self.load_label_config(config_file)
        if target_labels is None:
            return {}

        print(f"📋 Loaded {len(target_labels)} labels from {config_file}")

        # Sync to each repository
        results = {}
        for repo in repos:
            try:
                results[repo] = self.sync_labels_to_repo(
                    repo, target_labels, delete_extra
                )
            except Exception as e:
                result = LabelSyncResult()
                result.add_error(f"Unexpected error: {e}")
                results[repo] = result

        return results


def parse_repo_list(repos_arg: str) -> List[str]:
    """Parse comma-separated repository list."""
    return [repo.strip() for repo in repos_arg.split(",") if repo.strip()]


def load_repos_from_file(file_path: str) -> List[str]:
    """Load repository list from file (one repo per line)."""
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            repos = [
                line.strip() for line in f if line.strip() and not line.startswith("#")
            ]
        return repos
    except FileNotFoundError:
        print(f"Error: Repository file '{file_path}' not found", file=sys.stderr)
        return []


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="GitHub label management script",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python label_manager.py sync-labels --config labels.json --repos "owner/repo1,owner/repo2"
  python label_manager.py sync-labels --config labels.json --repos-file repos.txt --dry-run
  python label_manager.py sync-labels --config labels.json --repos "owner/repo" --delete-extra
        """,
    )

    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Sync labels command
    sync_parser = subparsers.add_parser(
        "sync-labels", help="Sync labels to repositories"
    )
    sync_parser.add_argument(
        "--config", required=True, help="Path to labels configuration file"
    )
    sync_parser.add_argument(
        "--repos", help="Comma-separated list of repositories (owner/name)"
    )
    sync_parser.add_argument(
        "--repos-file", help="File containing repository list (one per line)"
    )
    sync_parser.add_argument(
        "--delete-extra", action="store_true", help="Delete labels not in configuration"
    )
    sync_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be done without making changes",
    )

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return

    # Get GitHub token
    token = os.getenv("GH_TOKEN") or os.getenv("GITHUB_TOKEN")
    if not token:
        print(
            "Error: GH_TOKEN or GITHUB_TOKEN environment variable required",
            file=sys.stderr,
        )
        sys.exit(1)

    if args.command == "sync-labels":
        # Parse repository list
        repos = []
        if args.repos:
            repos.extend(parse_repo_list(args.repos))
        if args.repos_file:
            repos.extend(load_repos_from_file(args.repos_file))

        if not repos:
            print(
                "Error: No repositories specified. Use --repos or --repos-file",
                file=sys.stderr,
            )
            sys.exit(1)

        # Initialize manager and sync
        manager = LabelManager(token, dry_run=args.dry_run)
        results = manager.sync_labels_to_repos(repos, args.config, args.delete_extra)

        # Print summary
        print("\n" + "=" * 60)
        print("📊 SYNC SUMMARY")
        print("=" * 60)

        total_created = 0
        total_updated = 0
        total_deleted = 0
        total_errors = 0

        for repo, result in results.items():
            print(f"\n🏛️ Repository: {repo}")
            if result.errors:
                print("❌ Sync failed")
                for error in result.errors:
                    print(f"   Error: {error}")
                total_errors += len(result.errors)
            else:
                print("✅ Sync completed")
                if result.created:
                    print(f"   Created: {len(result.created)} labels")
                    total_created += len(result.created)
                if result.updated:
                    print(f"   Updated: {len(result.updated)} labels")
                    total_updated += len(result.updated)
                if result.deleted:
                    print(f"   Deleted: {len(result.deleted)} labels")
                    total_deleted += len(result.deleted)
                if result.skipped:
                    print(f"   Skipped: {len(result.skipped)} labels")

        print("\n📈 Overall Summary:")
        print(f"   Repositories processed: {len(results)}")
        print(f"   Labels created: {total_created}")
        print(f"   Labels updated: {total_updated}")
        print(f"   Labels deleted: {total_deleted}")
        if total_errors > 0:
            print(f"   Errors: {total_errors}")
            sys.exit(1)
        else:
            print("   Status: All repositories synced successfully")


if __name__ == "__main__":
    main()
