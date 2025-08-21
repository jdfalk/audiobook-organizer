#!/usr/bin/env python3
# file: scripts/sync-github-labels.py
# version: 1.2.0
# guid: b2c3d4e5-f6g7-8901-bcde-f23456789abc

"""
GitHub Labels Sync Script
This script manages GitHub repository labels via the GitHub API.
It reads labels.json and creates/updates actual repository labels.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional


class GitHubLabelsSync:
    """GitHub API-based label synchronization."""

    def __init__(self, owner: str, repo: str, token: str):
        self.owner = owner
        self.repo = repo
        self.token = token
        self.api_base = f"https://api.github.com/repos/{owner}/{repo}"

    def _make_request(
        self, method: str, endpoint: str, data: Optional[Dict] = None
    ) -> Dict[str, Any]:
        """Make authenticated GitHub API request."""
        url = f"{self.api_base}{endpoint}"
        headers = {
            "Authorization": f"token {self.token}",
            "Accept": "application/vnd.github.v3+json",
            "User-Agent": "GitHub-Labels-Sync/1.0",
        }

        if data:
            headers["Content-Type"] = "application/json"
            request_data = json.dumps(data).encode("utf-8")
        else:
            request_data = None

        request = urllib.request.Request(
            url, data=request_data, headers=headers, method=method
        )

        try:
            with urllib.request.urlopen(request) as response:
                return json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8")
            try:
                error_data = json.loads(error_body)
                raise Exception(
                    f"GitHub API error: {e.code} - {error_data.get('message', 'Unknown error')}"
                )
            except json.JSONDecodeError:
                raise Exception(f"GitHub API error: {e.code} - {error_body}")
        except urllib.error.URLError as e:
            raise Exception(f"Network error: {e.reason}")

    def get_existing_labels(self) -> List[Dict[str, Any]]:
        """Fetch all existing labels from the repository."""
        print("📋 Fetching existing labels...")
        labels = self._make_request("GET", "/labels")
        print(f"✅ Found {len(labels)} existing labels")
        return labels

    def create_label(self, name: str, color: str, description: str = "") -> bool:
        """Create a new label."""
        data = {"name": name, "color": color, "description": description}

        try:
            self._make_request("POST", "/labels", data)
            print(f"   ✅ Created label: {name}")
            return True
        except Exception as e:
            print(f"   ❌ Failed to create label '{name}': {e}")
            return False

    def update_label(self, name: str, color: str, description: str = "") -> bool:
        """Update an existing label."""
        data = {"name": name, "color": color, "description": description}

        try:
            encoded_name = urllib.parse.quote(name, safe="")
            self._make_request("PATCH", f"/labels/{encoded_name}", data)
            print(f"   ✅ Updated label: {name}")
            return True
        except Exception as e:
            print(f"   ❌ Failed to update label '{name}': {e}")
            return False

    def labels_are_identical(
        self, existing_label: Dict[str, Any], new_color: str, new_description: str
    ) -> bool:
        """Check if existing label is identical to the new one."""
        # Normalize colors (remove # prefix if present)
        existing_color = existing_label.get("color", "").lstrip("#")
        new_color = new_color.lstrip("#")

        # Normalize descriptions (empty string and None are considered the same)
        existing_desc = existing_label.get("description") or ""
        new_desc = new_description or ""

        return existing_color == new_color and existing_desc == new_desc

    def sync_labels(self, labels_file: str) -> bool:
        """Synchronize labels from JSON file to GitHub repository."""
        print("🏷️  GitHub Labels Sync")
        print(f"📁 Repository: {self.owner}/{self.repo}")
        print(f"📄 Labels file: {labels_file}")
        print()

        # Load labels from file
        try:
            with open(labels_file, "r", encoding="utf-8") as f:
                labels_data = json.load(f)
        except FileNotFoundError:
            print(f"❌ Labels file not found: {labels_file}")
            return False
        except json.JSONDecodeError as e:
            print(f"❌ Invalid JSON in labels file: {e}")
            return False

        # Get existing labels
        try:
            existing_labels = self.get_existing_labels()
        except Exception as e:
            print(f"❌ Failed to fetch existing labels: {e}")
            return False

        # Create lookup for existing labels
        existing_by_name = {label["name"]: label for label in existing_labels}

        # Process each label from the file
        print(f"🔄 Processing {len(labels_data)} labels from {labels_file}...")
        success_count = 0
        skipped_count = 0

        for label_config in labels_data:
            name = label_config.get("name")
            color = label_config.get("color", "")
            description = label_config.get("description", "")

            if not name:
                print("⚠️  Skipping label with missing name")
                continue

            print(f"🏷️  Processing label: {name}")

            if name in existing_by_name:
                # Check if the label is identical
                existing_label = existing_by_name[name]
                if self.labels_are_identical(existing_label, color, description):
                    print("   ⏭️  Skipping - label is identical")
                    skipped_count += 1
                    success_count += 1  # Count as success since no change needed
                else:
                    # Update existing label
                    print("   📝 Updating existing label...")
                    if self.update_label(name, color, description):
                        success_count += 1
            else:
                # Create new label
                print("   ➕ Creating new label...")
                if self.create_label(name, color, description):
                    success_count += 1

        print()
        if skipped_count > 0:
            print(f"⏭️  Skipped {skipped_count} identical labels")
        print(
            f"✅ GitHub labels sync completed! ({success_count}/{len(labels_data)} successful)"
        )
        print(f"🔗 View labels: https://github.com/{self.owner}/{self.repo}/labels")

        return success_count == len(labels_data)


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Synchronize GitHub repository labels from a JSON file",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s jdfalk gcommon
  %(prog)s jdfalk gcommon --labels-file custom-labels.json

Environment variables:
  GITHUB_TOKEN - Required GitHub personal access token
        """,
    )

    parser.add_argument("owner", help="Repository owner (e.g., jdfalk)")
    parser.add_argument("repo", help="Repository name (e.g., gcommon)")
    parser.add_argument(
        "--labels-file",
        default="labels.json",
        help="Path to labels JSON file (default: labels.json)",
    )

    args = parser.parse_args()

    # Get GitHub token (try PAT_TOKEN first, then GITHUB_TOKEN)
    github_token = os.environ.get("PAT_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not github_token:
        print("❌ GitHub token is required")
        print("   Set one of these environment variables:")
        print("   - PAT_TOKEN (recommended for enhanced permissions)")
        print("   - GITHUB_TOKEN (basic GitHub Actions token)")
        print("   Get a token from: https://github.com/settings/tokens")
        print("   Required scopes: repo")
        sys.exit(1)

    # Check if labels file exists
    if not os.path.isfile(args.labels_file):
        print(f"❌ Labels file not found: {args.labels_file}")
        sys.exit(1)

    # Perform sync
    sync = GitHubLabelsSync(args.owner, args.repo, github_token)

    try:
        success = sync.sync_labels(args.labels_file)
        sys.exit(0 if success else 1)
    except KeyboardInterrupt:
        print("\n⚠️  Operation cancelled by user")
        sys.exit(1)
    except Exception as e:
        print(f"❌ Unexpected error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
