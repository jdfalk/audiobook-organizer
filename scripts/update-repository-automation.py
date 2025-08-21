#!/usr/bin/env python3
# file: scripts/update-repository-automation.py
# version: 1.2.0
# guid: 7f8a9b0c-1d2e-3f4a-5b6c-7d8e9f0a1b2c

"""
Repository Automation Update Script

This script updates repositories to use the new unified automation configuration
system with enhanced duplicate prevention and standardized settings.

Usage:
    python scripts/update-repository-automation.py --repo owner/repo-name
    python scripts/update-repository-automation.py --config repos.txt --dry-run
    python scripts/update-repository-automation.py --config repos.txt --force-update
"""

import argparse
import json
import os
import sys
from typing import Dict, List

import requests


class RepositoryAutomationUpdater:
    """Updates repositories to use the new unified automation system."""

    def __init__(
        self,
        token: str,
        dry_run: bool = False,
        enable_cleanup: bool = True,
        force_update: bool = False,
    ):
        self.token = token
        self.dry_run = dry_run
        self.enable_cleanup = enable_cleanup
        self.force_update = force_update
        self.session = requests.Session()
        self.session.headers.update(
            {
                "Authorization": f"token {token}",
                "Accept": "application/vnd.github.v3+json",
                "User-Agent": "ghcommon-automation-updater",
            }
        )

        # Define patterns for old automation files that should be cleaned up
        self.old_automation_patterns = [
            "issue-management",
            "docs-update",
            "labeler",
            "super-linter",
            "intelligent-labeling",
            "label-sync",
            "sync-labels",
            "enhanced-issue-management",
            "enhanced-docs-update",
            "ai-rebase",
            "rebase",
            "unified-automation",  # Old unified automation workflow to be replaced
        ]

    def update_repository(self, repo: str) -> bool:
        """Update a single repository to use the new automation system."""
        print(f"🔄 Updating repository: {repo}")

        try:
            # Check if repository exists and we have access
            if not self._check_repository_access(repo):
                print(f"❌ Cannot access repository: {repo}")
                return False

            # Get the default branch for this repository
            default_branch = self._get_default_branch(repo)
            print(f"📋 Using default branch: {default_branch}")

            # Get current workflow files
            workflows = self._get_workflow_files(repo)

            # Check if new unified automation system is already configured
            # The new system has a workflow that calls reusable workflows
            has_new_unified = self._has_new_unified_automation(repo, workflows)

            if has_new_unified and not self.force_update:
                print(f"✅ Repository {repo} already has new unified automation system")
                cleanup_success = True
                if self.enable_cleanup:
                    cleanup_success = self._cleanup_old_automation_files(
                        repo, workflows, default_branch
                    )
                update_success = self._update_existing_configuration(
                    repo, workflows, default_branch
                )
                return cleanup_success and update_success
            else:
                if has_new_unified and self.force_update:
                    print(f"� Force updating unified automation in {repo}")
                else:
                    print(f"�📝 Adding unified automation to {repo}")

                # First, clean up old automation files (including old unified-automation.yml)
                cleanup_success = True
                if self.enable_cleanup:
                    cleanup_success = self._cleanup_old_automation_files(
                        repo, workflows, default_branch
                    )

                # Then add/update the unified automation system
                if cleanup_success:
                    add_success = self._add_unified_automation(repo, default_branch)
                    return add_success
                else:
                    return False

        except Exception as e:
            print(f"❌ Error updating {repo}: {e}")
            return False

    def _check_repository_access(self, repo: str) -> bool:
        """Check if we have access to the repository."""
        response = self.session.get(f"https://api.github.com/repos/{repo}")
        return response.status_code == 200

    def _get_default_branch(self, repo: str) -> str:
        """Get the default branch for the repository."""
        response = self.session.get(f"https://api.github.com/repos/{repo}")
        if response.status_code == 200:
            return response.json().get("default_branch", "main")
        return "main"

    def _get_workflow_files(self, repo: str) -> List[Dict]:
        """Get all workflow files in the repository."""
        response = self.session.get(
            f"https://api.github.com/repos/{repo}/contents/.github/workflows"
        )

        if response.status_code == 404:
            return []

        response.raise_for_status()
        return response.json()

    def _has_new_unified_automation(self, repo: str, workflows: List[Dict]) -> bool:
        """Check if repository has the new unified automation system."""
        # The new system should have a workflow that calls reusable workflows
        # and it should have a configuration file

        # Check for workflow that uses reusable workflows
        has_reusable_workflow = False
        for workflow in workflows:
            if "unified-automation" in workflow["name"]:
                # Get the workflow content to check if it calls reusable workflows
                try:
                    content_response = self.session.get(workflow["download_url"])
                    if content_response.status_code == 200:
                        content = content_response.text
                        if (
                            "jdfalk/ghcommon/.github/workflows/reusable-unified-automation.yml"
                            in content
                        ):
                            has_reusable_workflow = True
                            break
                except Exception:
                    continue

        # Check for configuration file
        has_config = False
        try:
            config_response = self.session.get(
                f"https://api.github.com/repos/{repo}/contents/.github/unified-automation-config.json"
            )
            has_config = config_response.status_code == 200
        except Exception:
            has_config = False

        return has_reusable_workflow and has_config

    def _cleanup_old_automation_files(
        self, repo: str, workflows: List[Dict], default_branch: str
    ) -> bool:
        """Clean up old automation workflow files that are now redundant."""
        files_to_remove = []

        # Find workflow files that match old automation patterns
        for workflow in workflows:
            workflow_name = workflow["name"].lower()

            # Check if it matches any old automation patterns
            for pattern in self.old_automation_patterns:
                if pattern in workflow_name and not workflow_name.startswith(
                    "reusable-"
                ):
                    files_to_remove.append(workflow)
                    break
                    break

        if not files_to_remove:
            print(f"✅ No old automation files to clean up in {repo}")
            return True

        print(
            f"🧹 Found {len(files_to_remove)} old automation files to remove in {repo}"
        )

        success_count = 0
        for workflow_file in files_to_remove:
            file_path = workflow_file["path"]
            file_name = workflow_file["name"]

            if self.dry_run:
                print(f"[DRY RUN] Would remove old workflow: {file_name}")
                success_count += 1
            else:
                if self._remove_file(
                    repo,
                    file_path,
                    workflow_file["sha"],
                    f"Remove redundant workflow: {file_name}",
                    default_branch,
                ):
                    print(f"🗑️  Removed old workflow: {file_name}")
                    success_count += 1
                else:
                    print(f"❌ Failed to remove old workflow: {file_name}")

        return success_count == len(files_to_remove)

    def _update_existing_configuration(
        self, repo: str, workflows: List[Dict], default_branch: str
    ) -> bool:
        """Update existing unified automation configuration."""
        # Check if configuration file exists
        config_response = self.session.get(
            f"https://api.github.com/repos/{repo}/contents/.github/unified-automation-config.json"
        )

        if config_response.status_code == 404:
            print(f"📄 Adding default configuration to {repo}")
            return self._add_default_configuration(repo, default_branch)
        else:
            print(f"📄 Configuration file already exists in {repo}")
            return self._validate_existing_configuration(
                repo, config_response.json(), default_branch
            )

    def _add_unified_automation(self, repo: str, default_branch: str) -> bool:
        """Add unified automation to a repository that doesn't have it."""
        # Add the complete workflow template
        workflow_content = self._get_complete_workflow_template(default_branch)

        if self.dry_run:
            print(f"[DRY RUN] Would add unified-automation.yml to {repo}")
            print(f"[DRY RUN] Would add unified-automation-config.json to {repo}")
            return True

        # Create workflow file
        workflow_success = self._create_file(
            repo,
            ".github/workflows/unified-automation.yml",
            workflow_content,
            "Add unified automation workflow",
            default_branch,
        )

        # Create configuration file
        config_content = self._get_default_configuration()
        config_success = self._create_file(
            repo,
            ".github/unified-automation-config.json",
            config_content,
            "Add unified automation configuration",
            default_branch,
        )

        return workflow_success and config_success

    def _add_default_configuration(self, repo: str, default_branch: str) -> bool:
        """Add default configuration to repository."""
        config_content = self._get_default_configuration()

        if self.dry_run:
            print(f"[DRY RUN] Would add unified-automation-config.json to {repo}")
            return True

        return self._create_file(
            repo,
            ".github/unified-automation-config.json",
            config_content,
            "Add unified automation configuration",
            default_branch,
        )

    def _validate_existing_configuration(
        self, repo: str, config_file: Dict, default_branch: str
    ) -> bool:
        """Validate and potentially update existing configuration."""
        try:
            import base64

            content = base64.b64decode(config_file["content"]).decode("utf-8")
            config = json.loads(content)

            # Check if config has the new duplicate prevention settings
            has_duplicate_settings = (
                "issue_management" in config
                and "enable_duplicate_prevention" in config["issue_management"]
            )

            if has_duplicate_settings:
                print(f"✅ Configuration in {repo} is up to date")
                return True
            else:
                print(f"🔄 Configuration in {repo} needs updating")
                return self._update_configuration(
                    repo, config, config_file["sha"], default_branch
                )

        except Exception as e:
            print(f"⚠️  Error validating configuration in {repo}: {e}")
            return False

    def _update_configuration(
        self, repo: str, current_config: Dict, sha: str, default_branch: str
    ) -> bool:
        """Update existing configuration with new settings."""
        # Merge with default configuration
        default_config = json.loads(self._get_default_configuration())

        # Update issue_management section
        if "issue_management" not in current_config:
            current_config["issue_management"] = {}

        # Add new duplicate prevention settings
        issue_mgmt = current_config["issue_management"]
        default_issue_mgmt = default_config["issue_management"]

        for key, value in default_issue_mgmt.items():
            if key not in issue_mgmt:
                issue_mgmt[key] = value

        updated_content = json.dumps(current_config, indent=2)

        if self.dry_run:
            print(f"[DRY RUN] Would update configuration in {repo}")
            return True

        return self._update_file(
            repo,
            ".github/unified-automation-config.json",
            updated_content,
            sha,
            "Update configuration with duplicate prevention settings",
            default_branch,
        )

    def _get_complete_workflow_template(self, default_branch: str) -> str:
        """Get the complete workflow template."""
        # First try to read local template if available
        local_template_path = "examples/workflows/unified-automation-complete.yml"
        if os.path.exists(local_template_path):
            try:
                with open(local_template_path, "r") as f:
                    return f.read()
            except Exception:
                pass

        # Fallback to remote template
        try:
            response = requests.get(
                "https://raw.githubusercontent.com/jdfalk/ghcommon/main/examples/workflows/unified-automation-complete.yml"
            )
            response.raise_for_status()
            return response.text
        except Exception:
            # Fallback to a basic template
            return self._get_basic_workflow_template(default_branch)

    def _get_basic_workflow_template(self, default_branch: str) -> str:
        """Get a basic workflow template as fallback."""
        return f"""name: Unified Automation

permissions:
  contents: write
  issues: write
  pull-requests: write
  security-events: read
  repository-projects: write
  actions: read
  checks: write

on:
  workflow_dispatch:
    inputs:
      operation:
        description: "Which operation(s) to run"
        required: false
        default: "all"
        type: choice
        options:
          - "all"
          - "issues"
          - "docs"
          - "label"
          - "lint"

  push:
    branches: [{default_branch}]
    paths:
      - '.github/issue-updates/**'
      - '.github/doc-updates/**'

jobs:
  automation:
    uses: jdfalk/ghcommon/.github/workflows/reusable-unified-automation.yml@main
    with:
      operation: ${{{{ github.event.inputs.operation || 'all' }}}}
    secrets: inherit
"""

    def _get_default_configuration(self) -> str:
        """Get the default configuration."""
        try:
            response = requests.get(
                "https://raw.githubusercontent.com/jdfalk/ghcommon/main/.github/unified-automation-config.json"
            )
            response.raise_for_status()
            return response.text
        except Exception:
            # Fallback to a minimal configuration
            return json.dumps(
                {
                    "issue_management": {
                        "operations": "auto",
                        "enable_duplicate_prevention": True,
                        "enable_duplicate_closure": True,
                        "duplicate_prevention_method": "guid_and_title",
                    }
                },
                indent=2,
            )

    def _create_file(
        self, repo: str, path: str, content: str, message: str, default_branch: str
    ) -> bool:
        """Create a file in the repository. If file exists, delete it first."""
        import base64

        # Check if file already exists and delete it first
        existing_response = self.session.get(
            f"https://api.github.com/repos/{repo}/contents/{path}"
        )

        if existing_response.status_code == 200:
            # File exists, delete it first
            existing_file = existing_response.json()
            delete_data = {
                "message": f"Remove existing {path}",
                "sha": existing_file["sha"],
                "branch": default_branch,
            }
            delete_response = self.session.delete(
                f"https://api.github.com/repos/{repo}/contents/{path}", json=delete_data
            )
            if delete_response.status_code != 200:
                print(
                    f"⚠️  Failed to delete existing {path} in {repo}: {delete_response.text}"
                )
                # Continue anyway, might still be able to create

        # Create the new file
        data = {
            "message": message,
            "content": base64.b64encode(content.encode()).decode(),
            "branch": default_branch,
        }

        response = self.session.put(
            f"https://api.github.com/repos/{repo}/contents/{path}", json=data
        )

        if response.status_code == 201:
            print(f"✅ Created {path} in {repo}")
            return True
        else:
            print(f"❌ Failed to create {path} in {repo}: {response.text}")
            return False

    def _update_file(
        self,
        repo: str,
        path: str,
        content: str,
        sha: str,
        message: str,
        default_branch: str,
    ) -> bool:
        """Update a file in the repository."""
        import base64

        data = {
            "message": message,
            "content": base64.b64encode(content.encode()).decode(),
            "sha": sha,
            "branch": default_branch,
        }

        response = self.session.put(
            f"https://api.github.com/repos/{repo}/contents/{path}", json=data
        )

        if response.status_code == 200:
            print(f"✅ Updated {path} in {repo}")
            return True
        else:
            print(f"❌ Failed to update {path} in {repo}: {response.text}")
            return False

    def _remove_file(
        self, repo: str, path: str, sha: str, message: str, default_branch: str
    ) -> bool:
        """Remove a file from the repository."""
        data = {"message": message, "sha": sha, "branch": default_branch}

        response = self.session.delete(
            f"https://api.github.com/repos/{repo}/contents/{path}", json=data
        )

        if response.status_code == 200:
            return True
        else:
            print(f"❌ Failed to remove {path} from {repo}: {response.text}")
            return False


def main():
    parser = argparse.ArgumentParser(
        description="Update repositories to use new unified automation"
    )
    parser.add_argument(
        "--repo", help="Single repository to update (format: owner/repo)"
    )
    parser.add_argument(
        "--config", help="File containing list of repositories to update"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be done without making changes",
    )
    parser.add_argument(
        "--no-cleanup", action="store_true", help="Skip cleanup of old automation files"
    )
    parser.add_argument(
        "--force-update",
        action="store_true",
        help="Force update even if new automation already exists",
    )
    parser.add_argument("--token", help="GitHub token (or set GITHUB_TOKEN env var)")

    args = parser.parse_args()

    # Get GitHub token
    token = args.token or os.getenv("GITHUB_TOKEN")
    if not token:
        print("❌ GitHub token required. Set GITHUB_TOKEN env var or use --token")
        sys.exit(1)

    # Get repositories to update
    repositories = []

    if args.repo:
        repositories.append(args.repo)
    elif args.config:
        if not os.path.exists(args.config):
            print(f"❌ Config file not found: {args.config}")
            sys.exit(1)

        with open(args.config, "r") as f:
            repositories = [
                line.strip() for line in f if line.strip() and not line.startswith("#")
            ]
    else:
        print("❌ Must specify either --repo or --config")
        sys.exit(1)

    print(f"🚀 Updating {len(repositories)} repositories")
    if args.dry_run:
        print("🔍 DRY RUN MODE - No changes will be made")
    if args.no_cleanup:
        print("🚫 CLEANUP DISABLED - Old automation files will not be removed")
    if args.force_update:
        print("💪 FORCE UPDATE MODE - Will update workflows even if they already exist")

    updater = RepositoryAutomationUpdater(
        token, args.dry_run, not args.no_cleanup, args.force_update
    )

    success_count = 0
    for repo in repositories:
        if updater.update_repository(repo):
            success_count += 1
        print()  # Empty line between repositories

    print(
        f"📊 Summary: {success_count}/{len(repositories)} repositories updated successfully"
    )


if __name__ == "__main__":
    main()
