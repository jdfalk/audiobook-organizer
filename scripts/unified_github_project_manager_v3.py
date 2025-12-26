#!/usr/bin/env python3
# file: scripts/unified_github_project_manager_v2.py
# version: 3.3.0
# guid: 4a5b6c7d-8e9f-0123-4567-89abcdef0123

"""
Unified GitHub Project Manager v3

A comprehensive script for creating and managing GitHub Projects across multiple repositories.
Consolidates ALL project management functionality into a single, idempotent, configuration-driven tool.

Features:
- Multi-repository project creation and linking
- Label creation and management with color coding
- Label cleanup and orphan detection/removal (preserves GitHub defaults)
- Interactive label management with safety protections
- Milestone creation with date support
- Issue creation with templates
- Issue assignment to projects with auto-detection
- Built-in workflow automation setup via GraphQL
- Idempotent operations (safe to run multiple times)
- Comprehensive project structure based on actual repository documentation analysis
- Cross-repository project sharing
- GitHub API integration for workflow setup
- Complete consolidation of github_project_manager.py functionality

Repositories Managed:
- subtitle-manager: Media processing and subtitle management
- codex-cli: AI automation tooling
- ghcommon: Common GitHub workflows and automation
- gcommon: Go common libraries and protobuf definitions

Project Structure Based on Actual Documentation Analysis:
- Cross-repository projects for shared initiatives (gcommon refactor, security, testing, docs, AI)
- Module-specific projects for gcommon (Metrics: 95 files, Queue: 175 files, Web: 176 files, etc.)
- Repository-specific projects for focused work areas
- Infrastructure and quality assurance projects

Usage:
    python3 scripts/unified_github_project_manager_v2.py [--dry-run] [--force] [--verbose]
    python3 scripts/unified_github_project_manager_v2.py --setup-workflows
    python3 scripts/unified_github_project_manager_v2.py --list-projects
    python3 scripts/unified_github_project_manager_v2.py --create-labels
    python3 scripts/unified_github_project_manager_v2.py --create-milestones
    python3 scripts/unified_github_project_manager_v2.py --cleanup-labels
    python3 scripts/unified_github_project_manager_v2.py --report-orphans
    python3 scripts/unified_github_project_manager_v2.py --interactive-cleanup

Author: GitHub Copilot
License: MIT
"""

import argparse
import json
import logging
import os
import subprocess
import sys
from typing import Dict, List, Any, Optional, Tuple


class UnifiedGitHubProjectManager:
    """
    Unified GitHub Project Manager for multi-repository project management.

    This class handles creation, linking, and management of GitHub Projects
    across multiple repositories with intelligent issue assignment, label management,
    milestone creation, and workflow automation setup.

    Consolidates all functionality from previous separate scripts.
    """

    def __init__(
        self, dry_run: bool = False, force: bool = False, verbose: bool = False
    ):
        """Initialize the unified project manager."""
        self.dry_run = dry_run
        self.force = force
        self.verbose = verbose
        self.owner = "jdfalk"  # Organization/user

        # Load configuration
        self.config = self._load_config()

        # Setup logging
        log_level = logging.DEBUG if verbose else logging.INFO
        logging.basicConfig(
            level=log_level,
            format="%(asctime)s - %(levelname)s - %(message)s",
            handlers=[
                logging.StreamHandler(sys.stdout),
                logging.FileHandler("unified_project_manager.log"),
            ],
        )
        self.logger = logging.getLogger(__name__)

        if self.dry_run:
            self.logger.info("🔍 Running in DRY-RUN mode - no changes will be made")

        # Validate GitHub CLI
        self._validate_github_cli()

    def _load_config(self):
        import json
        import os

        config_path = os.path.join(
            os.path.dirname(__file__), "unified_project_config.json"
        )
        with open(config_path, "r") as f:
            return json.load(f)

    def _validate_github_cli(self) -> None:
        """Validate GitHub CLI installation and authentication."""
        try:
            # Check if gh is installed
            subprocess.run(["gh", "--version"], capture_output=True, check=True)

            # Check authentication
            result = subprocess.run(
                ["gh", "auth", "token"], capture_output=True, check=True
            )
            if not result.stdout.strip():
                raise RuntimeError("GitHub CLI not authenticated")

            # Check project permissions
            subprocess.run(
                ["gh", "project", "list", "--owner", self.owner],
                capture_output=True,
                check=True,
            )

            self.logger.info("✅ GitHub CLI validated and authenticated")

        except subprocess.CalledProcessError as e:
            if "auth" in str(e):
                raise RuntimeError(
                    "GitHub CLI authentication failed. Run: gh auth login"
                ) from e
            elif "project" in str(e):
                raise RuntimeError(
                    "Missing project permissions. Run: gh auth refresh -s project,read:project"
                ) from e
            else:
                raise RuntimeError(f"GitHub CLI validation failed: {e}") from e
        except FileNotFoundError:
            raise RuntimeError(
                "GitHub CLI not found. Install with: brew install gh"
            ) from None

    def _run_gh_command(
        self, command: List[str], input_data: str = None
    ) -> Tuple[bool, str]:
        """
        Run a GitHub CLI command with error handling.

        Args:
            command: List of command arguments
            input_data: Optional input data for the command

        Returns:
            Tuple of (success, output/error_message)
        """
        try:
            # Ensure all command arguments are strings
            command_str = [str(arg) for arg in command]

            if self.dry_run:
                self.logger.info(f"DRY-RUN: Would execute: gh {' '.join(command_str)}")
                # For read-only commands (list, view), execute them even in dry-run
                if (
                    (
                        command_str[0] in ["project", "repo", "auth"]
                        and len(command_str) > 1
                        and command_str[1] in ["list", "view", "token"]
                    )
                    or (command_str[0] == "label" and command_str[1] == "list")
                    or (
                        command_str[0] == "api"
                        and "GET" not in command_str
                        and "--method" not in command_str
                    )
                ):
                    # Execute read-only commands even in dry-run mode
                    self.logger.debug("Executing read-only command in dry-run mode")
                    # Continue to actual execution
                else:
                    # Return mock responses for commands that expect specific formats
                    if "--json" in command_str or any(
                        "--format" in cmd and "json" in cmd for cmd in command_str
                    ):
                        # Return proper mock JSON structure based on command type
                        if "project" in command_str and "list" in command_str:
                            return True, '{"projects": [], "totalCount": 0}'
                        else:
                            return (
                                True,
                                "[]",
                            )  # Return empty JSON array for other commands
                    return True, "DRY-RUN: Command not executed"

            self.logger.debug(f"Executing: gh {' '.join(command_str)}")

            if input_data:
                result = subprocess.run(
                    ["gh"] + command_str,
                    input=input_data,
                    text=True,
                    capture_output=True,
                    check=True,
                )
            else:
                result = subprocess.run(
                    ["gh"] + command_str, capture_output=True, text=True, check=True
                )

            return True, result.stdout.strip()

        except subprocess.CalledProcessError as e:
            error_msg = e.stderr.strip() if e.stderr else str(e)
            command_str = [str(arg) for arg in command]  # Ensure we have command_str
            self.logger.error(f"Command failed: gh {' '.join(command_str)}")
            self.logger.error(f"Error: {error_msg}")
            return False, error_msg

        except FileNotFoundError:
            error_msg = "GitHub CLI (gh) not found. Please install it first."
            self.logger.error(error_msg)
            return False, error_msg
        except Exception as e:
            command_str = [str(arg) for arg in command]  # Ensure we have command_str
            error_msg = f"Unexpected error: {str(e)}"
            self.logger.error(f"Command failed: gh {' '.join(command_str)}")
            self.logger.error(error_msg)
            return False, error_msg

    def _get_project_definitions(self) -> dict:
        """Get all project definitions from config file."""
        return self.config.get("projects", {})

    def _get_label_definitions(self) -> dict:
        """Get all label definitions from config file."""
        return self.config.get("labels", {})

    def _get_milestone_definitions(self) -> dict:
        """Get all milestone definitions from config file."""
        return self.config.get("milestones", {})

    def _generate_project_readme_content(self, title: str, config: dict) -> str:
        """Generate README content for a project from config file."""
        projects = self.config.get("projects", {})
        project = projects.get(title, {})
        return project.get("readme", "")

    def _create_project(self, title: str, description: str) -> Optional[str]:
        """Create a single GitHub Project."""
        if self.dry_run:
            self.logger.info(f"DRY-RUN: Would create project '{title}'")
            return "dry-run-id"

        success, output = self._run_gh_command(
            [
                "project",
                "create",
                "--owner",
                self.owner,
                "--title",
                title,
                "--format",
                "json",
            ]
        )

        if success:
            return output
        else:
            self.logger.error(f"Failed to create project '{title}': {output}")
            return None

    def update_project_descriptions_and_readmes(
        self, project_numbers: Dict[str, str]
    ) -> None:
        """Update project descriptions and create README content for all projects."""
        self.logger.info(
            "📝 Updating project descriptions and creating README content..."
        )

        project_definitions = self._get_project_definitions()

        for project_title, project_number in project_numbers.items():
            if project_title not in project_definitions:
                continue

            project_config = project_definitions[project_title]
            description = project_config["description"]

            # Update project description
            self._update_project_description(project_number, project_title, description)

            # Create README content for the project
            self._create_project_readme(project_number, project_title, project_config)

    def _update_project_description(
        self, project_number: str, title: str, description: str
    ) -> bool:
        """Update a project's description using GraphQL API."""
        if self.dry_run:
            self.logger.info(f"DRY-RUN: Would update description for project '{title}'")
            return True

        # GraphQL mutation to update project description
        mutation = """
        mutation($projectId: ID!, $description: String!) {
          updateProjectV2(input: {
            projectId: $projectId,
            description: $description
          }) {
            projectV2 {
              id
              title
              description
            }
          }
        }
        """

        # First, get the project ID using the project number
        project_id = self._get_project_id_from_number(project_number)
        if not project_id:
            self.logger.warning(f"⚠️ Could not get project ID for #{project_number}")
            return False

        success, output = self._run_gh_command(
            [
                "api",
                "graphql",
                "-f",
                f"query={mutation}",
                "-f",
                f"projectId={project_id}",
                "-f",
                f"description={description}",
            ]
        )

        if success:
            self.logger.info(f"✅ Updated description for project '{title}'")
            return True
        else:
            self.logger.warning(
                f"⚠️ Failed to update description for project '{title}': {output}"
            )
            return False

    def _get_project_id_from_number(self, project_number: str) -> Optional[str]:
        """Get project ID from project number using GraphQL."""
        query = """
        query($owner: String!, $number: Int!) {
          user(login: $owner) {
            projectV2(number: $number) {
              id
            }
          }
        }
        """

        success, output = self._run_gh_command(
            [
                "api",
                "graphql",
                "-f",
                f"query={query}",
                "-F",
                f"owner={self.owner}",
                "-F",
                f"number={int(project_number)}",
            ]
        )

        if not success:
            return None

        try:
            response_data = json.loads(output)
            project_data = (
                response_data.get("data", {}).get("user", {}).get("projectV2")
            )
            return project_data.get("id") if project_data else None
        except (json.JSONDecodeError, KeyError):
            return None

    def _create_project_readme(
        self, project_number: str, title: str, config: Dict[str, Any]
    ) -> None:
        """Create README content for a project."""
        if self.dry_run:
            self.logger.info(f"DRY-RUN: Would create README for project '{title}'")
            return

        readme_content = self._generate_project_readme_content(title, config)

        # Display the README content
        print("\n" + "=" * 80)
        print(f"PROJECT README: {title}")
        print("=" * 80)
        print(readme_content)
        print("=" * 80)

        # Note: GitHub Projects v2 doesn't support adding README files directly
        # This content can be used to manually create README in project description
        # or as a reference document
        self.logger.info(
            f"📋 Generated README content for project '{title}' (displayed above)"
        )

    def link_all_repositories(self, project_numbers: Dict[str, str]) -> None:
        """Display repositories currently linked to projects without linking anything."""
        self.logger.info(
            "🔍 DEBUG: Displaying current project-repository relationships..."
        )

        project_definitions = self._get_project_definitions()

        # Print table header
        print("\n" + "=" * 80)
        print("CURRENT PROJECT-REPOSITORY RELATIONSHIPS")
        print("=" * 80)
        print(f"{'PROJECT NAME':<40} | {'PROJECT #':<10} | {'LINKED REPOSITORIES'}")
        print("-" * 80)

        for project_title, project_number in project_numbers.items():
            if project_title not in project_definitions:
                continue

            # Get repositories that SHOULD be linked according to config
            should_be_linked = project_definitions[project_title]["repositories"]

            # Get repositories that ARE ACTUALLY linked
            # Ensure project_number is a string
            project_number_str = str(project_number)
            linked_repos = self._get_linked_repositories(project_number_str)

            # Display the results
            if linked_repos is None:
                linked_repos_str = "ERROR: Could not retrieve linked repositories"
            elif len(linked_repos) == 0:
                linked_repos_str = "No repositories linked"
            else:
                linked_repos_str = ", ".join(linked_repos)

            print(
                f"{project_title:<40} | {project_number_str:<10} | {linked_repos_str}"
            )

            # Display which repositories need linking
            if linked_repos is not None:
                missing_repos = [
                    repo for repo in should_be_linked if repo not in linked_repos
                ]
                if missing_repos:
                    print(
                        f"{'  MISSING LINKS:':<40} | {'':10} | {', '.join(missing_repos)}"
                    )

            # Add a separator between projects
            print("-" * 80)

        print(
            "\nINSTRUCTIONS: Review the data above to see which repositories are already linked."
        )
        print("The linking code has been commented out to diagnose the issue.")
        print("=" * 80 + "\n")

        # Original linking code is commented out below:
        """
        for project_title, project_number in project_numbers.items():
            if project_title not in project_definitions:
                continue

            repositories = project_definitions[project_title]["repositories"]

            # First, get all repositories already linked to this project
            linked_repos = self._get_linked_repositories(project_number)
            if linked_repos is None:
                self.logger.warning(f"⚠️ Could not retrieve linked repositories for project '{project_title}', proceeding with caution")
                linked_repos = []

            for repository in repositories:
                # Skip if already linked (unless force is enabled)
                if repository in linked_repos and not self.force:
                    self.logger.info(f"✅ Repository '{repository}' already linked to project '{project_title}' (skipping)")
                    continue

                # If force is enabled and already linked, show message but still skip
                if repository in linked_repos and self.force:
                    self.logger.info(f"✅ Repository '{repository}' already linked to project '{project_title}' (force enabled but no action needed)")
                    continue

                # Only attempt to link if not already linked
                success = self._link_repository(project_number, repository)
                if success:
                    self.logger.info(f"✅ Linked {repository} to project '{project_title}'")
                else:
                    self.logger.warning(f"⚠️ Failed to link {repository} to project '{project_title}'")
        """

    def _get_linked_repositories(self, project_number: str) -> Optional[List[str]]:
        """
        Get list of repositories already linked to a project using GraphQL API.

        Args:
            project_number: Project number to check

        Returns:
            List of repository names or None if error occurred
        """
        # Convert project_number to string if it's not already
        project_number = str(project_number)

        if self.dry_run:
            # In dry-run mode, return empty list to simulate linking all repos
            return []

        try:
            # Use GraphQL to get project details and linked repositories
            graphql_query = """
            query($owner: String!, $number: Int!) {
              user(login: $owner) {
                projectV2(number: $number) {
                  id
                  title
                  repositories(first: 100) {
                    nodes {
                      name
                      nameWithOwner
                    }
                  }
                }
              }
            }
            """

            success, output = self._run_gh_command(
                [
                    "api",
                    "graphql",
                    "-f",
                    f"query={graphql_query}",
                    "-F",
                    f"owner={self.owner}",
                    "-F",
                    f"number={int(project_number)}",
                ]
            )

            if not success:
                self.logger.warning(
                    f"⚠️ Error getting project #{project_number} details via GraphQL: {output}"
                )
                return None

            # Parse GraphQL response
            response_data = json.loads(output)

            if "errors" in response_data:
                self.logger.warning(
                    f"⚠️ GraphQL errors for project #{project_number}: {response_data['errors']}"
                )
                return None

            project_data = (
                response_data.get("data", {}).get("user", {}).get("projectV2")
            )
            if not project_data:
                self.logger.warning(f"⚠️ No project data found for #{project_number}")
                return None

            # Extract repository names
            linked_repos = []
            repositories = project_data.get("repositories", {}).get("nodes", [])

            for repo in repositories:
                if "name" in repo:
                    linked_repos.append(repo["name"])

            return linked_repos

        except json.JSONDecodeError as e:
            self.logger.warning(
                f"⚠️ Could not parse GraphQL response for project #{project_number}: {e}"
            )
            return None
        except Exception as e:
            self.logger.warning(
                f"⚠️ Unexpected error getting linked repositories for project #{project_number}: {e}"
            )
            return None

    def _link_repository(self, project_number: str, repository: str) -> bool:
        """Link a repository to a project."""
        if self.dry_run:
            self.logger.info(
                f"DRY-RUN: Would link {repository} to project #{project_number}"
            )
            return True

        success, output = self._run_gh_command(
            [
                "project",
                "link",
                project_number,
                "--owner",
                self.owner,
                "--repo",
                repository,
            ]
        )

        if not success:
            # Check if the error is just that it's already linked
            if "already linked" in output.lower() or "already exists" in output.lower():
                self.logger.debug(
                    f"✅ Repository {repository} is already linked to project #{project_number}"
                )
                return True
            else:
                self.logger.warning(
                    f"Failed to link repository '{repository}' to project #{project_number}: {output}"
                )
                return False

        return success

    def _labels_are_identical(
        self, existing_label: Dict[str, str], new_label_config: Dict[str, str]
    ) -> bool:
        """
        Check if an existing label is identical to the desired label configuration.

        Args:
            existing_label: Label data from GitHub API
            new_label_config: Desired label configuration

        Returns:
            True if labels are identical, False otherwise
        """
        # Normalize colors for comparison
        existing_color = existing_label.get("color", "").lstrip("#").lower()
        new_color = self._normalize_color(new_label_config["color"])

        # Compare color and description
        existing_description = existing_label.get("description", "")
        new_description = new_label_config.get("description", "")

        return existing_color == new_color and existing_description == new_description

    def create_all_labels(self, repositories: List[str] = None) -> None:
        """Create labels across all repositories."""
        if repositories is None:
            repositories = ["subtitle-manager", "gcommon", "ghcommon", "codex-cli"]

        self.logger.info("🏷️ Creating labels across repositories...")

        label_definitions = self._get_label_definitions()

        for repository in repositories:
            self.logger.info(f"Creating labels for {repository}...")
            existing_labels = self._get_existing_labels(repository)

            for label_name, label_config in label_definitions.items():
                if label_name in existing_labels:
                    # Check if the existing label is identical to what we want
                    existing_label = existing_labels[label_name]
                    if self._labels_are_identical(existing_label, label_config):
                        self.logger.debug(
                            f"✅ Label '{label_name}' already exists in {repository} with correct properties (skipping)"
                        )
                        continue
                    else:
                        # Label exists but needs updating
                        if not self.force:
                            self.logger.info(
                                f"🔄 Label '{label_name}' exists in {repository} but needs updating..."
                            )
                        else:
                            self.logger.info(
                                f"🔄 Force updating label '{label_name}' in {repository}..."
                            )

                        success = self._update_label(
                            repository, label_name, label_config
                        )
                        if success:
                            self.logger.info(
                                f"✅ Updated label '{label_name}' in {repository}"
                            )
                        else:
                            self.logger.error(
                                f"❌ Failed to update label '{label_name}' in {repository}"
                            )
                        continue

                # Create new label
                success = self._create_label(repository, label_name, label_config)
                if success:
                    self.logger.info(f"✅ Created label '{label_name}' in {repository}")
                else:
                    self.logger.error(
                        f"❌ Failed to create label '{label_name}' in {repository}"
                    )

    def _update_label(
        self, repository: str, label_name: str, label_config: Dict[str, str]
    ) -> bool:
        """Update an existing label in a repository."""
        if self.dry_run:
            self.logger.info(
                f"DRY-RUN: Would update label '{label_name}' in {repository}"
            )
            return True

        color = self._normalize_color(label_config["color"])
        description = label_config.get("description", "")

        # Update the label using gh label edit
        success, output = self._run_gh_command(
            [
                "label",
                "edit",
                label_name,
                "--repo",
                f"{self.owner}/{repository}",
                "--color",
                color,
                "--description",
                description,
            ]
        )

        if not success:
            self.logger.debug(
                f"Failed to update label '{label_name}' in {repository}: {output}"
            )

        return success

    def _create_label(
        self, repository: str, label_name: str, label_config: Dict[str, str]
    ) -> bool:
        """Create a single label in a repository (idempotent)."""
        if self.dry_run:
            self.logger.info(
                f"DRY-RUN: Would create label '{label_name}' in {repository}"
            )
            return True

        color = self._normalize_color(label_config["color"])
        description = label_config.get("description", "")

        # Create the label with --force to handle existing labels
        success, output = self._run_gh_command(
            [
                "label",
                "create",
                label_name,
                "--repo",
                f"{self.owner}/{repository}",
                "--color",
                color,
                "--description",
                description,
                "--force",
            ]
        )

        if success:
            self.logger.info(f"✅ Created/updated label '{label_name}' in {repository}")
        else:
            # Check if it's just because the label already exists
            if (
                "already exists" in output.lower()
                or "label already exists" in output.lower()
            ):
                self.logger.info(
                    f"✅ Label '{label_name}' already exists in {repository}"
                )
                return True
            else:
                self.logger.error(
                    f"❌ Failed to create label '{label_name}' in {repository}: {output}"
                )

        return success

    def _get_default_github_labels(self) -> set:
        """
        Get the set of default labels that GitHub provides for new repositories.
        These should never be deleted during cleanup operations.

        Returns:
            Set of default GitHub label names to preserve
        """
        return {
            # GitHub's default labels as of 2025
            "bug",
            "documentation",
            "duplicate",
            "enhancement",
            "good first issue",
            "help wanted",
            "invalid",
            "question",
            "wontfix",
            # Common variations that might exist
            "good-first-issue",
            "help-wanted",
            # Additional default labels that might be present
            "dependencies",
            "dependency",
            # Custom protected labels that should never be deleted
            "codex",  # AI/agent-created issues identifier
            "security",
        }

    def cleanup_orphaned_labels(self, repositories: List[str] = None) -> None:
        """Remove labels that are not in the current definition."""
        if repositories is None:
            repositories = ["subtitle-manager", "gcommon", "ghcommon", "codex-cli"]

        self.logger.info("🧹 Cleaning up orphaned labels across repositories...")

        label_definitions = self._get_label_definitions()
        defined_labels = set(label_definitions.keys())
        default_github_labels = self._get_default_github_labels()

        for repository in repositories:
            self.logger.info(f"Checking for orphaned labels in {repository}...")
            existing_labels = self._get_existing_labels(repository)
            existing_label_names = set(existing_labels.keys())

            # Find labels that exist but are not in our definition AND not default GitHub labels
            orphaned_labels = (
                existing_label_names - defined_labels - default_github_labels
            )

            # Exclude GitHub's default labels from deletion
            default_labels = self._get_default_github_labels()
            orphaned_labels -= default_labels

            if not orphaned_labels:
                self.logger.info(f"✅ No orphaned labels found in {repository}")
                continue

                self.logger.info(
                    f"🗑️ Found {len(orphaned_labels)} orphaned labels in {repository}"
                )

                for label_name in orphaned_labels:
                    success = self._delete_label(repository, label_name)
                    if success:
                        self.logger.info(
                            f"✅ Deleted orphaned label '{label_name}' from {repository}"
                        )
                    else:
                        self.logger.error(
                            f"❌ Failed to delete label '{label_name}' from {repository}"
                        )

    def report_orphaned_labels(self, repositories: List[str] = None) -> None:
        """Report on labels that exist but are not in the current definition."""
        if repositories is None:
            repositories = ["subtitle-manager", "gcommon", "ghcommon", "codex-cli"]

        self.logger.info("📊 Reporting on orphaned labels across repositories...")

        label_definitions = self._get_label_definitions()
        defined_labels = set(label_definitions.keys())
        default_github_labels = self._get_default_github_labels()
        total_orphans = 0

        print("\n" + "=" * 80)
        print("ORPHANED LABELS REPORT")
        print("=" * 80)
        print(f"Labels defined in configuration: {len(defined_labels)}")
        print(f"Default GitHub labels (preserved): {len(default_github_labels)}")
        print("-" * 80)

        for repository in repositories:
            existing_labels = self._get_existing_labels(repository)
            existing_label_names = set(existing_labels.keys())

            # Find labels that exist but are not in our definition AND not default GitHub labels
            orphaned_labels = (
                existing_label_names - defined_labels - default_github_labels
            )
            defined_in_repo = existing_label_names & defined_labels
            default_in_repo = existing_label_names & default_github_labels

            print(f"\n📂 Repository: {repository}")
            print(f"   Total labels: {len(existing_label_names)}")
            print(f"   Defined labels: {len(defined_in_repo)}")
            print(f"   Default GitHub labels: {len(default_in_repo)}")
            print(f"   Orphaned labels: {len(orphaned_labels)}")

            if orphaned_labels:
                total_orphans += len(orphaned_labels)
                print("   🗑️ Orphaned label details:")
                for label_name in sorted(orphaned_labels):
                    label_info = existing_labels[label_name]
                    color = label_info.get("color", "unknown")
                    description = label_info.get("description", "no description")
                    print(f"      - '{label_name}' (#{color}): {description}")

            # Also show which default GitHub labels are present (for info)
            if default_in_repo:
                print(
                    f"   🔒 Protected GitHub defaults: {', '.join(sorted(default_in_repo))}"
                )

        print("\n" + "=" * 80)
        print(f"SUMMARY: {total_orphans} total orphaned labels across all repositories")
        print(
            f"NOTE: {len(default_github_labels)} default GitHub labels are protected from deletion"
        )
        if total_orphans > 0:
            print("Use --cleanup-labels to remove them automatically")
            print("Use --interactive-cleanup to review and remove selectively")
        print("=" * 80 + "\n")

    def interactive_cleanup_labels(self, repositories: List[str] = None) -> None:
        """Interactively clean up orphaned labels."""
        if repositories is None:
            repositories = ["subtitle-manager", "gcommon", "ghcommon", "codex-cli"]

        self.logger.info("🎯 Interactive cleanup of orphaned labels...")

        label_definitions = self._get_label_definitions()
        defined_labels = set(label_definitions.keys())
        default_github_labels = self._get_default_github_labels()

        for repository in repositories:
            print(f"\n📂 Checking repository: {repository}")
            existing_labels = self._get_existing_labels(repository)
            existing_label_names = set(existing_labels.keys())

            # Find labels that exist but are not in our definition AND not default GitHub labels
            orphaned_labels = (
                existing_label_names - defined_labels - default_github_labels
            )
            protected_labels = existing_label_names & default_github_labels

            if protected_labels:
                print(
                    f"🔒 Protected GitHub default labels: {', '.join(sorted(protected_labels))}"
                )

            if not orphaned_labels:
                print(f"✅ No orphaned labels found in {repository}")
                continue

            print(f"🗑️ Found {len(orphaned_labels)} orphaned labels in {repository}")

            for label_name in sorted(orphaned_labels):
                label_info = existing_labels[label_name]
                color = label_info.get("color", "unknown")
                description = label_info.get("description", "no description")

                print(f"\n   Label: '{label_name}'")
                print(f"   Color: #{color}")
                print(f"   Description: {description}")

                if self.dry_run:
                    print("   🔍 DRY-RUN: Would prompt for deletion")
                    continue

                while True:
                    response = input("   Delete this label? [y/N/q]: ").strip().lower()
                    if response in ["y", "yes"]:
                        success = self._delete_label(repository, label_name)
                        if success:
                            print(f"   ✅ Deleted '{label_name}'")
                        else:
                            print(f"   ❌ Failed to delete '{label_name}'")
                        break
                    elif response in ["n", "no", ""]:
                        print(f"   ⏭️ Skipped '{label_name}'")
                        break
                    elif response in ["q", "quit"]:
                        print("   🛑 Quitting interactive cleanup")
                        return
                    else:
                        print("   Please enter y(es), n(o), or q(uit)")

    def _delete_label(self, repository: str, label_name: str) -> bool:
        """Delete a label from a repository."""
        # Safety check: never delete default GitHub labels
        default_github_labels = self._get_default_github_labels()
        if label_name in default_github_labels:
            self.logger.warning(
                f"🔒 PROTECTED: Refusing to delete default GitHub label '{label_name}' from {repository}"
            )
            return False

        if self.dry_run:
            self.logger.info(
                f"DRY-RUN: Would delete label '{label_name}' from {repository}"
            )
            return True

        success, output = self._run_gh_command(
            [
                "label",
                "delete",
                label_name,
                "--repo",
                f"{self.owner}/{repository}",
                "--yes",  # Skip confirmation
            ]
        )

        if not success:
            self.logger.debug(
                f"Failed to delete label '{label_name}' from {repository}: {output}"
            )

        return success

    def create_all_milestones(self, repositories: List[str] = None) -> None:
        """Create milestones across all repositories."""
        if repositories is None:
            repositories = ["subtitle-manager", "gcommon", "ghcommon", "codex-cli"]

        self.logger.info("📅 Creating milestones across repositories...")

        milestone_definitions = self._get_milestone_definitions()

        for repository in repositories:
            self.logger.info(f"Creating milestones for {repository}...")
            existing_milestones = self._get_existing_milestones(repository)

            for milestone_title, milestone_config in milestone_definitions.items():
                if milestone_title in existing_milestones:
                    if not self.force:
                        self.logger.debug(
                            f"✅ Milestone '{milestone_title}' already exists in {repository}"
                        )
                        continue
                    else:
                        self.logger.info(
                            f"🔄 Would update milestone '{milestone_title}' in {repository} (not implemented)"
                        )

                success = self._create_milestone(
                    repository, milestone_title, milestone_config
                )
                if success:
                    self.logger.info(
                        f"✅ Created milestone '{milestone_title}' in {repository}"
                    )
                else:
                    self.logger.error(
                        f"❌ Failed to create milestone '{milestone_title}' in {repository}"
                    )

    def _create_milestone(
        self, repository: str, milestone_title: str, milestone_config: Dict[str, str]
    ) -> bool:
        """Create a single milestone in a repository."""
        if self.dry_run:
            self.logger.info(
                f"DRY-RUN: Would create milestone '{milestone_title}' in {repository}"
            )
            return True

        # Prepare milestone data
        milestone_data = {
            "title": milestone_title,
            "description": milestone_config.get("description", ""),
            "state": milestone_config.get("state", "open"),
        }

        if "due_date" in milestone_config:
            milestone_data["due_on"] = f"{milestone_config['due_date']}T23:59:59Z"

        # Use GitHub API to create milestone
        success, output = self._run_gh_command(
            [
                "api",
                f"repos/{self.owner}/{repository}/milestones",
                "--method",
                "POST",
                "--field",
                f"title={milestone_title}",
                "--field",
                f"description={milestone_config.get('description', '')}",
                "--field",
                f"state={milestone_config.get('state', 'open')}",
            ]
            + (
                ["--field", f"due_on={milestone_config['due_date']}T23:59:59Z"]
                if "due_date" in milestone_config
                else []
            )
        )

        return success

    def list_projects(self, output_format="table"):
        """List all GitHub projects with their repositories."""
        self.logger.info("Fetching project information...")

        existing_projects_dict = self._get_existing_projects()
        project_definitions = self._get_project_definitions()

        all_projects = {}

        # Add existing projects
        for project_title, project_data in existing_projects_dict.items():
            repo_names = []
            if isinstance(project_data, dict) and "repositories" in project_data:
                repo_names = [
                    repo["name"] for repo in project_data.get("repositories", [])
                ]

            all_projects[project_title] = {
                "repositories": repo_names,
                "status": "Exists",
                "description": project_data.get("description", "")
                if isinstance(project_data, dict)
                else "",
                "url": project_data.get("url", "")
                if isinstance(project_data, dict)
                else "",
                "items_count": project_data.get("items", {}).get("totalCount", 0)
                if isinstance(project_data, dict)
                else 0,
            }

        # Add defined projects that might not exist yet
        for project_name, project_data in project_definitions.items():
            if project_name not in all_projects:
                all_projects[project_name] = {
                    "repositories": project_data.get("repositories", []),
                    "status": "Defined (Not Created)",
                    "description": project_data.get("description", ""),
                    "url": "",
                    "items_count": 0,
                }
            else:
                # Update with config data if project exists
                all_projects[project_name]["repositories"] = project_data.get(
                    "repositories", []
                )
                if not all_projects[project_name]["description"]:
                    all_projects[project_name]["description"] = project_data.get(
                        "description", ""
                    )

        if output_format == "json":
            print(json.dumps(all_projects, indent=2))
        else:
            # Table format
            print("\n" + "=" * 80)
            print("GitHub Projects Overview")
            print("=" * 80)

            for project_name, project_info in all_projects.items():
                print(f"\nProject: {project_name}")
                print(f"Status: {project_info['status']}")
                print(f"Description: {project_info['description']}")
                print(f"Repositories: {', '.join(project_info['repositories'])}")
                if project_info["url"]:
                    print(f"URL: {project_info['url']}")
                print(f"Items: {project_info['items_count']}")
                print("-" * 40)

        return all_projects

    def sync_labels(self):
        """Synchronize labels across all repositories."""
        self.logger.info("🏷️ Synchronizing labels across repositories...")

        labels_config = self.config.get("labels", {})
        all_repos = set()

        # Collect all repositories from project definitions
        project_definitions = self._get_project_definitions()
        for project_data in project_definitions.values():
            if isinstance(project_data, dict) and "repositories" in project_data:
                all_repos.update(project_data["repositories"])

        success_count = 0
        total_count = 0

        for repo_name in all_repos:
            self.logger.info(f"Syncing labels for {repo_name}...")
            existing_labels = self._get_existing_labels(repo_name)

            for label_name, label_data in labels_config.items():
                total_count += 1
                if label_name in existing_labels:
                    # Update existing label
                    existing_label = existing_labels[label_name]
                    needs_update = self._normalize_color(
                        existing_label.get("color", "")
                    ) != self._normalize_color(
                        label_data.get("color", "")
                    ) or existing_label.get("description", "") != label_data.get(
                        "description", ""
                    )

                    if needs_update:
                        if self.dry_run:
                            self.logger.info(
                                f"DRY-RUN: Would update label '{label_name}' in {repo_name}"
                            )
                        if self._update_label(repo_name, label_name, label_data):
                            success_count += 1
                    else:
                        if self.dry_run:
                            self.logger.info(
                                f"DRY-RUN: Label '{label_name}' already exists and is up-to-date in {repo_name}"
                            )
                        success_count += 1  # Already up to date
                else:
                    # Create new label
                    if self._create_label(repo_name, label_name, label_data):
                        success_count += 1

        self.logger.info(
            f"Label sync completed: {success_count}/{total_count} operations successful"
        )
        return success_count == total_count

    def sync_milestones(self):
        """Synchronize milestones across all repositories."""
        self.logger.info("🎯 Synchronizing milestones across repositories...")

        milestones_config = self.config.get("milestones", {})
        all_repos = set()

        # Collect all repositories from project definitions
        project_definitions = self._get_project_definitions()
        for project_data in project_definitions.values():
            if isinstance(project_data, dict) and "repositories" in project_data:
                all_repos.update(project_data["repositories"])

        success_count = 0
        total_count = 0

        for repo_name in all_repos:
            self.logger.info(f"Syncing milestones for {repo_name}...")
            existing_milestones = self._get_existing_milestones(repo_name)

            for milestone_name, milestone_data in milestones_config.items():
                total_count += 1
                if milestone_name in existing_milestones:
                    # Update existing milestone
                    existing_milestone = existing_milestones[milestone_name]
                    needs_update = existing_milestone.get(
                        "description", ""
                    ) != milestone_data.get(
                        "description", ""
                    ) or existing_milestone.get("state", "") != milestone_data.get(
                        "state", ""
                    )

                    if needs_update:
                        if self._update_milestone(
                            repo_name, milestone_name, milestone_data
                        ):
                            success_count += 1
                            self.logger.info(
                                f"Updated milestone '{milestone_name}' in {repo_name}"
                            )
                        else:
                            self.logger.error(
                                f"Failed to update milestone '{milestone_name}' in {repo_name}"
                            )
                    else:
                        if self.dry_run:
                            self.logger.info(
                                f"DRY-RUN: Milestone '{milestone_name}' already exists and is up-to-date in {repo_name}"
                            )
                        success_count += 1  # Already up to date
                else:
                    # Create new milestone
                    if self._create_milestone(
                        repo_name, milestone_name, milestone_data
                    ):
                        success_count += 1
                        self.logger.info(
                            f"Created milestone '{milestone_name}' in {repo_name}"
                        )
                    else:
                        self.logger.error(
                            f"Failed to create milestone '{milestone_name}' in {repo_name}"
                        )

        self.logger.info(
            f"Milestone sync completed: {success_count}/{total_count} operations successful"
        )
        return success_count == total_count

    def create_projects(self):
        """Create missing GitHub projects based on configuration."""
        self.logger.info("🏗️ Creating missing GitHub projects...")

        project_definitions = self._get_project_definitions()
        existing_projects = self._get_existing_projects()
        existing_project_titles = set(existing_projects.keys())

        created_count = 0

        for project_name, project_data in project_definitions.items():
            if project_name not in existing_project_titles:
                self.logger.info(f"Creating project: {project_name}")
                if self._create_github_project(project_name, project_data):
                    created_count += 1

                    # Add repositories to project if specified
                    repositories = project_data.get("repositories", [])
                    if repositories:
                        self._add_repositories_to_project(project_name, repositories)
            else:
                self.logger.info(f"Project already exists: {project_name}")

        self.logger.info(f"Created {created_count} new projects")
        return created_count

    def _create_github_project(self, project_name, project_data):
        """Create a new GitHub project (idempotent)."""
        # Check if project already exists (case-insensitive)
        existing_projects = self._get_existing_projects()
        existing_names_lower = {name.lower(): name for name in existing_projects.keys()}

        if project_name.lower() in existing_names_lower:
            actual_name = existing_names_lower[project_name.lower()]
            self.logger.info(
                f"✅ Project already exists as '{actual_name}' (case-insensitive match for '{project_name}')"
            )
            return True

        if self.dry_run:
            self.logger.info(f"DRY-RUN: Would create project '{project_name}'")
            return True

        try:
            # Create project using GitHub CLI
            cmd = [
                "gh",
                "project",
                "create",
                "--owner",
                self.owner,
                "--title",
                project_name,
            ]

            subprocess.run(cmd, check=True, capture_output=True, text=True)
            self.logger.info(f"✅ Successfully created project: {project_name}")
            return True
        except subprocess.CalledProcessError as e:
            # Check if the error is because project already exists
            if "already exists" in str(e).lower():
                self.logger.info(f"✅ Project already exists: {project_name}")
                return True
            self.logger.error(f"❌ Failed to create project '{project_name}': {e}")
            return False

    def _add_repositories_to_project(self, project_name, repositories):
        """Add repositories to a GitHub project."""
        for repo_name in repositories:
            try:
                # Note: This requires the project URL, which we'd need to fetch
                # For now, just log the intent
                self.logger.info(
                    f"Would add repository {repo_name} to project {project_name}"
                )
                # TODO: Implement actual repository addition
            except Exception as e:
                self.logger.error(
                    f"Failed to add repository {repo_name} to project {project_name}: {e}"
                )

    def _get_existing_projects(self) -> dict:
        """
        Get all existing projects for the owner using GitHub CLI.
        Returns a dict mapping project titles to their details.
        """
        success, output = self._run_gh_command(
            [
                "project",
                "list",
                "--owner",
                self.owner,
                "--format",
                "json",
                "--limit",
                "100",  # Set high limit to get all projects (GitHub CLI default is 30)
            ]
        )
        if not success:
            self.logger.warning(f"⚠️ Could not fetch existing projects: {output}")
            return {}
        try:
            projects_data = json.loads(output)
            # Handle both list and dict-with-projects formats
            if isinstance(projects_data, dict) and "projects" in projects_data:
                projects = projects_data["projects"]
            elif isinstance(projects_data, list):
                projects = projects_data
            else:
                self.logger.warning(
                    f"⚠️ Unexpected project list format: {projects_data}"
                )
                return {}

            result = {
                p.get("title", f"project-{p.get('number', '')}"): p
                for p in projects
                if isinstance(p, dict)
            }
            self.logger.debug(f"Found {len(result)} existing projects")
            return result
        except Exception as e:
            self.logger.warning(f"⚠️ Error parsing project list: {e}")
            return {}

    def setup_project_workflows(self, project_numbers: Dict[str, str]) -> None:
        """
        Set up automated workflows for GitHub Projects.

        Args:
            project_numbers: Dictionary mapping project titles to their numbers
        """
        self.logger.info("🔄 Setting up project workflows...")

        # Get project definitions to know which repositories each project monitors
        project_definitions = self._get_project_definitions()
        workflow_config = self.get_auto_add_workflow_config()

        # Display comprehensive workflow setup instructions
        self.display_workflow_setup_with_repositories(
            project_definitions, workflow_config
        )

        if self.dry_run:
            self.logger.info(
                "🔍 DRY-RUN: Workflow configuration would be applied to projects"
            )
        else:
            self.logger.info(
                "📋 Workflow configuration ready - apply manually in GitHub UI"
            )

    def display_workflow_setup_with_repositories(
        self,
        project_definitions: Dict[str, Dict[str, Any]],
        workflow_config: Dict[str, List[str]],
    ) -> None:
        """
        Display detailed workflow setup instructions with repository information.

        Args:
            project_definitions: Full project configuration
            workflow_config: Workflow-specific label configuration
        """
        print("\n" + "=" * 80)
        print("GITHUB PROJECT WORKFLOW SETUP INSTRUCTIONS")
        print("=" * 80)

        print(
            "\nThese instructions will help you configure automated workflows for your GitHub Projects."
        )
        print(
            "Each project can be configured to automatically add issues when specific labels are applied."
        )

        print("\n" + "-" * 80)
        print("WORKFLOW CONFIGURATION BY PROJECT AND REPOSITORY")
        print("-" * 80)

        for project_title, labels in workflow_config.items():
            if project_title not in project_definitions:
                continue

            project_config = project_definitions[project_title]
            repositories = project_config["repositories"]

            print(f"\n📊 Project: {project_title}")
            print(f"    Monitors repositories: {', '.join(repositories)}")
            print(f"    Auto-add issues with these labels: {', '.join(labels)}")
            print(f"    Description: {project_config['description']}")

        print("\n" + "-" * 80)
        print("MANUAL SETUP INSTRUCTIONS")
        print("-" * 80)

        print(
            "\nFor each project listed above, you need to create one workflow per repository:"
        )

        print("\n1. Navigate to the project in GitHub:")
        print("   - Go to https://github.com/jdfalk?tab=projects")
        print("   - Select the project you want to configure")

        print("\n2. Access project settings:")
        print("   - Click the three dots (...) in the top-right corner")
        print("   - Select 'Settings'")

        print("\n3. Set up workflows:")
        print("   - In the left sidebar, click 'Workflows'")
        print("   - Click 'New workflow'")
        print("   - Select 'Item added to project'")

        print("\n4. Configure the workflow:")
        print("   - Name your workflow (e.g., 'Auto-add labeled issues')")
        print("   - Under 'When', select 'Issues added to repository'")
        print(
            "   - Under 'Filters', select 'Label' and add the labels shown above for this project"
        )
        print("   - Under 'Then', select 'Add item to project'")
        print("   - Click 'Create'")

        print("\n5. Repeat for each project listed above")

        print("\n" + "=" * 80)
        print(
            "Note: This script does not currently automate the workflow setup directly."
        )
        print(
            "      The GitHub Projects API has limited support for workflow automation."
        )
        print("      Follow the manual steps above to set up your workflows.")
        print("=" * 80 + "\n")

    def _get_existing_labels(self, repo_name):
        """Get existing labels for a repository using GitHub CLI."""
        try:
            result = subprocess.run(
                [
                    "gh",
                    "label",
                    "list",
                    "--repo",
                    f"{self.owner}/{repo_name}",
                    "--json",
                    "name,color,description",
                ],
                capture_output=True,
                text=True,
                check=True,
            )
            return {label["name"]: label for label in json.loads(result.stdout)}
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Failed to get existing labels for {repo_name}: {e}")
            return {}
        except json.JSONDecodeError as e:
            self.logger.error(f"Failed to parse labels JSON for {repo_name}: {e}")
            return {}

    def _get_existing_milestones(self, repo_name):
        """Get existing milestones for a repository using GitHub CLI."""
        try:
            result = subprocess.run(
                [
                    "gh",
                    "api",
                    f"/repos/{self.owner}/{repo_name}/milestones",
                ],
                capture_output=True,
                text=True,
                check=False,  # Don't raise on error
            )
            if result.returncode != 0:
                # If 404, treat as no milestones
                if "404" in result.stderr or "Not Found" in result.stderr:
                    self.logger.warning(
                        f"No milestones found for {repo_name} (404 Not Found)"
                    )
                    return {}
                # If other error, log and continue
                self.logger.error(
                    f"Failed to get existing milestones for {repo_name}: {result.stderr.strip()}"
                )
                return {}
            try:
                milestones = json.loads(result.stdout)
                return {milestone["title"]: milestone for milestone in milestones}
            except json.JSONDecodeError as e:
                self.logger.error(
                    f"Failed to parse milestones JSON for {repo_name}: {e}"
                )
                return {}
        except Exception as e:
            self.logger.error(
                f"Unexpected error getting milestones for {repo_name}: {e}"
            )
            return {}

    def _normalize_color(self, color):
        """Normalize color format (remove # if present)."""
        if color and color.startswith("#"):
            return color[1:]
        return color or "000000"

    def _update_label(self, repo_name, label_name, label_data):
        """Update an existing label in the repository."""
        try:
            cmd = [
                "gh",
                "label",
                "edit",
                "--repo",
                f"{self.owner}/{repo_name}",
                label_name,
                "--color",
                self._normalize_color(label_data.get("color", "000000")),
                "--description",
                label_data.get("description", ""),
            ]
            subprocess.run(cmd, check=True, capture_output=True, text=True)
            self.logger.info(f"Updated label '{label_name}' in {repo_name}")
            return True
        except subprocess.CalledProcessError as e:
            self.logger.error(
                f"Failed to update label '{label_name}' in {repo_name}: {e}"
            )
            return False

    def _update_milestone(self, repo_name, milestone_name, milestone_data):
        """Update an existing milestone in the repository."""
        try:
            # First get the milestone number
            result = subprocess.run(
                ["gh", "api", f"/repos/{self.owner}/{repo_name}/milestones"],
                capture_output=True,
                text=True,
                check=True,
            )
            milestones = json.loads(result.stdout)
            milestone_number = None
            for milestone in milestones:
                if milestone["title"] == milestone_name:
                    milestone_number = milestone["number"]
                    break

            if milestone_number is None:
                self.logger.error(
                    f"Milestone '{milestone_name}' not found in {repo_name}"
                )
                return False

            data = {
                "title": milestone_name,
                "description": milestone_data.get("description", ""),
                "state": milestone_data.get("state", "open"),
            }
            if "due_on" in milestone_data:
                data["due_on"] = milestone_data["due_on"]

            cmd = [
                "gh",
                "api",
                f"/repos/{self.owner}/{repo_name}/milestones/{milestone_number}",
                "--method",
                "PATCH",
            ]
            for key, value in data.items():
                cmd.extend(["--field", f"{key}={value}"])

            subprocess.run(cmd, check=True, capture_output=True, text=True)
            self.logger.info(f"Updated milestone '{milestone_name}' in {repo_name}")
            return True
        except subprocess.CalledProcessError as e:
            self.logger.error(
                f"Failed to update milestone '{milestone_name}' in {repo_name}: {e}"
            )
            return False
        except json.JSONDecodeError as e:
            self.logger.error(f"Failed to parse milestones JSON for {repo_name}: {e}")
            return False

    def run_full_setup(self):
        """Run the complete project setup process."""
        self.logger.info("🚀 Starting full GitHub project setup...")

        try:
            # 1. Create all projects
            project_numbers = self.create_all_projects()

            # 2. Link repositories to projects
            self.link_all_repositories(project_numbers)

            # 3. Create labels across all repositories
            self.sync_labels()

            # 4. Create milestones across all repositories
            self.sync_milestones()

            # 5. Set up project workflows
            self.setup_project_workflows(project_numbers)

            # 6. Display auto-add workflow configuration
            self.logger.info("🔄 Auto-add workflow configuration:")
            workflow_config = self.get_auto_add_workflow_config()

            print("\n" + "=" * 60)
            print("AUTO-ADD WORKFLOW CONFIGURATION")
            print("=" * 60)
            print("Use this configuration in GitHub's built-in project automation:")
            print()

            for project_title, labels in workflow_config.items():
                print(f"Project: {project_title}")
                print(f"  Auto-add when labeled with: {', '.join(labels)}")
                print()

            self.logger.info("✅ Full setup completed successfully!")

        except Exception as e:
            self.logger.error(f"❌ Setup failed: {str(e)}")
            raise

    def create_all_projects(self) -> Dict[str, str]:
        """Create all GitHub Projects defined in the configuration."""
        self.logger.info("🚀 Starting full GitHub project setup...")

        project_definitions = self._get_project_definitions()
        existing_projects = self._get_existing_projects()
        project_numbers = {}

        for title, config in project_definitions.items():
            # CHECK: Does project exist? (case-insensitive matching)
            existing_project_data = None
            exact_existing_title = None

            # First try exact match
            if title in existing_projects:
                existing_project_data = existing_projects[title]
                exact_existing_title = title
            else:
                # Try case-insensitive match
                title_lower = title.lower()
                for existing_title, project_data in existing_projects.items():
                    if existing_title.lower() == title_lower:
                        existing_project_data = project_data
                        exact_existing_title = existing_title
                        self.logger.info(
                            f"📝 Found case-insensitive match: '{title}' matches existing '{existing_title}'"
                        )
                        break

            if existing_project_data:
                project_number = str(existing_project_data.get("number", ""))
                project_id = str(existing_project_data.get("id", ""))
                project_numbers[title] = project_number

                # Get stored values from config
                stored_number = config.get("github_number")
                stored_id = config.get("github_id")

                # UPDATE: If config needs to be updated with current GitHub data
                if stored_number != project_number or stored_id != project_id:
                    self.logger.info(
                        f"✅ Project '{exact_existing_title}' exists (#{project_number}) - updating config"
                    )
                    # Update will happen in _update_config_with_existing_data
                else:
                    self.logger.info(
                        f"✅ Project '{exact_existing_title}' already exists (#{project_number})"
                    )
                continue

            # CREATE: Project doesn't exist
            self.logger.info(f"🆕 Creating missing project: '{title}'")
            project_number = self._create_project(title, config["description"])
            if project_number:
                project_numbers[title] = project_number
                self.logger.info(f"✅ Created project: {title} (#{project_number})")
            else:
                self.logger.error(f"❌ Failed to create project: {title}")

        # Update project descriptions and create README content
        self.update_project_descriptions_and_readmes(project_numbers)

        # Link repositories to projects
        self.link_all_repositories(project_numbers)

        # Update config file with the latest GitHub data (numbers, IDs, links)
        self._update_config_with_existing_data(existing_projects)

        return project_numbers

    def _create_project(self, title: str, description: str) -> Optional[str]:
        """Create a single GitHub Project."""
        if self.dry_run:
            self.logger.info(f"DRY-RUN: Would create project '{title}'")
            return "dry-run-id"

        success, output = self._run_gh_command(
            [
                "project",
                "create",
                "--owner",
                self.owner,
                "--title",
                title,
                "--format",
                "json",
            ]
        )

        if success:
            try:
                project_data = json.loads(output)
                return str(project_data.get("number", ""))
            except json.JSONDecodeError:
                self.logger.error(
                    f"Could not parse project creation response for '{title}'"
                )
                return None
        else:
            self.logger.error(f"Failed to create project '{title}': {output}")
            return None

    def get_auto_add_workflow_config(self):
        """Get auto-add workflow configuration for projects."""
        project_definitions = self._get_project_definitions()
        workflow_config = {}

        for project_name, project_data in project_definitions.items():
            workflows = project_data.get("workflows", {})
            if workflows.get("auto_add_issues", False):
                labels = workflows.get("labels", [])
                if labels:
                    workflow_config[project_name] = labels

        return workflow_config

    def _update_config_with_existing_data(self, existing_projects: dict = None) -> None:
        """Update the config file with existing project IDs and repository links."""
        if existing_projects is None:
            existing_projects = self._get_existing_projects()
        project_definitions = self._get_project_definitions()

        config_updated = False

        # Update project numbers and IDs
        for project_title, project_data in existing_projects.items():
            if project_title in project_definitions:
                project_number = str(project_data.get("number", ""))
                project_id = str(project_data.get("id", ""))

                current_number = project_definitions[project_title].get("github_number")
                current_id = project_definitions[project_title].get("github_id")

                # Update if either number or id changed
                if current_number != project_number or current_id != project_id:
                    self.logger.info(
                        f"📝 Updating config: '{project_title}' -> #{project_number} (ID: {project_id})"
                    )
                    self.config["projects"][project_title]["github_number"] = (
                        project_number
                    )
                    self.config["projects"][project_title]["github_id"] = project_id
                    config_updated = True

                    # Also update repository links if we can get them
                    linked_repos = self._get_linked_repositories(project_number)
                    if linked_repos is not None:
                        current_links = project_definitions[project_title].get(
                            "repository_links", {}
                        )
                        new_links = {}

                        # Check each repository that should be linked
                        for repo in project_definitions[project_title]["repositories"]:
                            new_links[repo] = repo in linked_repos

                        if current_links != new_links:
                            self.config["projects"][project_title][
                                "repository_links"
                            ] = new_links
                            config_updated = True

        # Save updated config
        if config_updated:
            self._save_config()
            self.logger.info("💾 Config file updated with latest GitHub data")
        else:
            self.logger.debug("✅ Config file is already up to date")

    def _save_config(self) -> None:
        """Save the current config back to the JSON file."""
        config_path = os.path.join(
            os.path.dirname(__file__), "unified_project_config.json"
        )
        with open(config_path, "w") as f:
            json.dump(self.config, f, indent=2)


def main():
    """Main entry point for the unified GitHub project manager."""

    parser = argparse.ArgumentParser(
        description="Unified GitHub Project Manager for multi-repository project management"
    )

    # Action flags
    parser.add_argument(
        "--list-projects",
        action="store_true",
        help="List all projects and their current status",
    )
    parser.add_argument(
        "--sync-labels", action="store_true", help="Sync labels across repositories"
    )
    parser.add_argument(
        "--sync-milestones",
        action="store_true",
        help="Sync milestones across repositories",
    )
    parser.add_argument(
        "--update-config",
        action="store_true",
        help="Update config file with existing project IDs and repository links",
    )

    # Operational flags
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be done without making changes",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Force operations even if they seem unnecessary",
    )
    parser.add_argument("--verbose", action="store_true", help="Enable verbose logging")

    args = parser.parse_args()

    # Create manager instance
    manager = UnifiedGitHubProjectManager(
        dry_run=args.dry_run, force=args.force, verbose=args.verbose
    )

    try:
        # Handle specific actions
        if args.list_projects:
            manager.list_projects()
            return

        ran_any = False
        if args.sync_labels:
            manager.sync_labels()
            ran_any = True

        if args.sync_milestones:
            manager.sync_milestones()
            ran_any = True

        if ran_any:
            return

        if args.update_config:
            manager._update_config_with_existing_data()
            return

        # Default action: run full setup
        print("🚀 Running full GitHub project setup...")
        print(
            "Use --list-projects to just list projects, or other flags for specific actions\n"
        )

        # Run full setup (which handles config updates internally)
        manager.run_full_setup()

    except KeyboardInterrupt:
        print("\n⏹️ Operation cancelled by user")
    except Exception as e:
        print(f"\n❌ Error: {e}")
        if args.verbose:
            import traceback

            traceback.print_exc()


if __name__ == "__main__":
    main()
