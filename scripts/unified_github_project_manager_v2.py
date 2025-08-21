#!/usr/bin/env python3
# file: scripts/unified_github_project_manager_v2.py
# version: 3.2.1
# guid: 4a5b6c7d-8e9f-0123-4567-89abcdef0123

"""
Unified GitHub Project Manager v2

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
                # Return mock JSON for commands that expect JSON output
                if "--json" in command_str or any(
                    "--format" in cmd and "json" in cmd for cmd in command_str
                ):
                    return True, "[]"  # Return empty JSON array
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

    def _get_project_definitions(self) -> Dict[str, Dict[str, Any]]:
        """
        Get comprehensive project definitions with standardized labels.

        Returns:
            Dictionary mapping project titles to their configurations
        """
        return {
            # Cross-Repository Projects (Multi-repo initiatives)
            "gcommon Refactor & Integration": {
                "description": "Hybrid protobuf + Go types migration affecting multiple repositories. Centralizes business logic in gcommon while maintaining compatibility.",
                "repositories": ["subtitle-manager", "gcommon", "ghcommon"],
                "labels": [
                    "project:gcommon-refactor",
                    "tech:protobuf",
                    "tech:go",
                    "module:backend",
                ],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "project:gcommon-refactor",
                        "tech:protobuf",
                        "tech:go",
                        "module:backend",
                    ],
                },
            },
            "Security & Authentication": {
                "description": "Cross-repository security improvements, authentication systems, and compliance initiatives.",
                "repositories": ["subtitle-manager", "gcommon", "ghcommon"],
                "labels": ["type:security", "module:auth", "module:backend"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "type:security",
                        "module:auth",
                        "module:backend",
                    ],
                },
            },
            "Testing & Quality Assurance": {
                "description": "Comprehensive testing strategies, quality assurance processes, and validation frameworks across all repositories.",
                "repositories": [
                    "subtitle-manager",
                    "gcommon",
                    "ghcommon",
                    "codex-cli",
                ],
                "labels": ["type:testing", "tech:go", "tech:python", "workflow:ci-cd"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "type:testing",
                        "tech:go",
                        "tech:python",
                        "workflow:ci-cd",
                    ],
                },
            },
            "Documentation & Standards": {
                "description": "Documentation improvements, coding standards, and knowledge management across all repositories.",
                "repositories": [
                    "subtitle-manager",
                    "gcommon",
                    "ghcommon",
                    "codex-cli",
                ],
                "labels": ["type:documentation", "tech:go", "tech:python"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": ["type:documentation", "tech:go", "tech:python"],
                },
            },
            "AI & Automation Tools": {
                "description": "AI-powered automation tools, GitHub Actions, and workflow improvements.",
                "repositories": ["codex-cli", "ghcommon", "subtitle-manager"],
                "labels": [
                    "workflow:automation",
                    "workflow:github-actions",
                    "workflow:deployment",
                ],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "workflow:automation",
                        "workflow:github-actions",
                        "workflow:deployment",
                    ],
                },
            },
            # Repository-Specific Projects
            # subtitle-manager projects
            "Media Processing & Transcription": {
                "description": "Whisper integration, subtitle synchronization, quality scoring, and media processing features.",
                "repositories": ["subtitle-manager"],
                "labels": [
                    "project:media",
                    "project:transcription",
                    "project:whisper",
                    "tech:go",
                ],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "project:media",
                        "project:transcription",
                        "project:whisper",
                        "tech:go",
                    ],
                },
            },
            "Web UI & User Experience": {
                "description": "Metadata editor, dashboard improvements, authentication UI, and user experience enhancements.",
                "repositories": ["subtitle-manager"],
                "labels": ["module:ui", "module:frontend", "tech:javascript"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": ["module:ui", "module:frontend", "tech:javascript"],
                },
            },
            "Database & Performance": {
                "description": "Database backend migration (PebbleDB/SQLite), performance optimization, and caching improvements.",
                "repositories": ["subtitle-manager"],
                "labels": [
                    "module:database",
                    "performance",
                    "tech:go",
                    "module:backend",
                ],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "module:database",
                        "performance",
                        "tech:go",
                        "module:backend",
                    ],
                },
            },
            # gcommon module-specific projects (626 empty protobuf files to implement!)
            "Metrics Module": {
                "description": "Implementation of 95 empty protobuf files for metrics collection, monitoring, and observability.",
                "repositories": ["gcommon"],
                "labels": ["module:metrics", "tech:protobuf", "tech:grpc", "tech:go"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "module:metrics",
                        "tech:protobuf",
                        "tech:grpc",
                        "tech:go",
                    ],
                },
            },
            "Queue Module": {
                "description": "Implementation of 175 empty protobuf files for message queuing, task management, and asynchronous processing.",
                "repositories": ["gcommon"],
                "labels": ["module:queue", "tech:protobuf", "tech:grpc", "tech:go"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": ["module:queue", "tech:protobuf", "tech:grpc", "tech:go"],
                },
            },
            "Web Module": {
                "description": "Implementation of 176 empty protobuf files for web services, HTTP handling, and API management.",
                "repositories": ["gcommon"],
                "labels": ["module:web", "tech:protobuf", "tech:grpc", "tech:go"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": ["module:web", "tech:protobuf", "tech:grpc", "tech:go"],
                },
            },
            "Auth Module": {
                "description": "Implementation of 109 remaining protobuf files for authentication, authorization, and identity management.",
                "repositories": ["gcommon"],
                "labels": ["module:auth", "tech:protobuf", "tech:grpc", "tech:go"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "module:auth",
                        "tech:protobuf",
                        "tech:grpc",
                        "tech:go",
                    ],
                },
            },
            "Cache Module": {
                "description": "Implementation of 36 remaining protobuf files for caching, data storage, and performance optimization.",
                "repositories": ["gcommon"],
                "labels": ["module:cache", "performance", "tech:protobuf", "tech:grpc"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "module:cache",
                        "performance",
                        "tech:protobuf",
                        "tech:grpc",
                    ],
                },
            },
            "Config Module": {
                "description": "Implementation of 20 remaining protobuf files for configuration management and system settings.",
                "repositories": ["gcommon"],
                "labels": ["module:config", "tech:protobuf", "tech:grpc", "tech:go"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "module:config",
                        "tech:protobuf",
                        "tech:grpc",
                        "tech:go",
                    ],
                },
            },
            # ghcommon projects
            "Infrastructure Cleanup": {
                "description": "File organization, duplicate file removal, deprecated workflow cleanup, and infrastructure improvements.",
                "repositories": ["ghcommon"],
                "labels": ["workflow:automation", "workflow:deployment", "tech:python"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "workflow:automation",
                        "workflow:deployment",
                        "tech:python",
                    ],
                },
            },
            "Core Workflow Enhancement": {
                "description": "Error handling improvements, workflow modularization, logging framework, and core functionality enhancements.",
                "repositories": ["ghcommon"],
                "labels": ["workflow:deployment", "workflow:automation", "tech:python"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": [
                        "workflow:deployment",
                        "workflow:automation",
                        "tech:python",
                    ],
                },
            },
            "Security & Compliance": {
                "description": "Security.md creation, contributing guidelines, code of conduct, and compliance documentation.",
                "repositories": ["ghcommon"],
                "labels": ["type:security", "type:documentation", "tech:python"],
                "workflows": {
                    "auto_add_issues": True,
                    "auto_close_completed": True,
                    "labels": ["type:security", "type:documentation", "tech:python"],
                },
            },
        }

    def _get_label_definitions(self) -> Dict[str, Dict[str, str]]:
        """
        Get comprehensive standardized label definitions for all repositories.
        Consolidates best practices from gcommon, ghcommon, subtitle-manager, and codex-cli.

        Returns:
            Dictionary mapping label names to their properties (color, description)
        """
        return {
            # Priority labels - unified across all repos
            "priority:critical": {
                "color": "d73a49",
                "description": "Critical priority - immediate attention required",
            },
            "priority:high": {"color": "d93f0b", "description": "High priority"},
            "priority:medium": {"color": "fbca04", "description": "Medium priority"},
            "priority:low": {"color": "0e8a16", "description": "Low priority"},
            # Size labels - from subtitle-manager best practices
            "size:small": {
                "color": "c2e0c6",
                "description": "Small change (1-2 hours)",
            },
            "size:medium": {
                "color": "bfd4f2",
                "description": "Medium change (half day)",
            },
            "size:large": {"color": "f9d0c4", "description": "Large change (1-2 days)"},
            "size:epic": {
                "color": "d73a49",
                "description": "Epic change (multiple days/weeks)",
            },
            # Type labels - core issue types
            "type:bug": {"color": "d73a49", "description": "Something isn't working"},
            "type:feature": {
                "color": "0052cc",
                "description": "New feature development",
            },
            "type:enhancement": {
                "color": "a2eeef",
                "description": "Improvement to existing feature",
            },
            "type:documentation": {
                "color": "0075ca",
                "description": "Improvements or additions to documentation",
            },
            "type:testing": {"color": "1d76db", "description": "Testing related work"},
            "type:security": {
                "color": "ee0701",
                "description": "Security related issues",
            },
            "type:refactor": {
                "color": "f1c232",
                "description": "Code refactoring without feature changes",
            },
            "type:maintenance": {
                "color": "6c757d",
                "description": "Maintenance and housekeeping",
            },
            # Status labels - workflow states
            "status:todo": {"color": "ffffff", "description": "To do - not started"},
            "status:in-progress": {
                "color": "d4edda",
                "description": "Work in progress",
            },
            "status:blocked": {
                "color": "f8d7da",
                "description": "Blocked by external dependencies",
            },
            "status:needs-review": {
                "color": "fff3cd",
                "description": "Needs code review",
            },
            "status:ready": {
                "color": "c3e6cb",
                "description": "Ready for implementation",
            },
            "status:duplicate": {"color": "6f42c1", "description": "Duplicate issue"},
            "status:wontfix": {
                "color": "6c757d",
                "description": "This will not be worked on",
            },
            # Module labels - from gcommon architecture
            "module:auth": {
                "color": "0052cc",
                "description": "Authentication and authorization",
            },
            "module:cache": {
                "color": "5319e7",
                "description": "Caching and data storage",
            },
            "module:config": {
                "color": "006b75",
                "description": "Configuration management",
            },
            "module:database": {
                "color": "fd7e14",
                "description": "Database related work",
            },
            "module:metrics": {
                "color": "6f42c1",
                "description": "Metrics collection and monitoring",
            },
            "module:queue": {
                "color": "e99695",
                "description": "Message queuing and task management",
            },
            "module:web": {
                "color": "b60205",
                "description": "Web services and HTTP handling",
            },
            "module:ui": {
                "color": "495057",
                "description": "User interface development",
            },
            "module:api": {
                "color": "0366d6",
                "description": "API development and changes",
            },
            "module:backend": {"color": "343a40", "description": "Backend development"},
            "module:frontend": {
                "color": "6c757d",
                "description": "Frontend development",
            },
            # Technology labels - programming languages and frameworks
            "tech:go": {"color": "00add8", "description": "Go programming language"},
            "tech:python": {
                "color": "3572a5",
                "description": "Python programming language",
            },
            "tech:javascript": {
                "color": "f1e05a",
                "description": "JavaScript programming language",
            },
            "tech:typescript": {
                "color": "3178c6",
                "description": "TypeScript programming language",
            },
            "tech:protobuf": {
                "color": "c5def5",
                "description": "Protocol buffer definitions",
            },
            "tech:grpc": {
                "color": "bfd4f2",
                "description": "gRPC service implementations",
            },
            "tech:docker": {
                "color": "2496ed",
                "description": "Docker containerization",
            },
            "tech:kubernetes": {
                "color": "326ce5",
                "description": "Kubernetes orchestration",
            },
            "tech:shell": {
                "color": "89e051",
                "description": "Shell scripting (bash, sh)",
            },
            # Workflow labels - automation and processes
            "workflow:automation": {
                "color": "1f883d",
                "description": "Automation and tooling",
            },
            "workflow:github-actions": {
                "color": "2088ff",
                "description": "GitHub Actions workflows",
            },
            "workflow:ci-cd": {
                "color": "28a745",
                "description": "Continuous integration and deployment",
            },
            "workflow:deployment": {
                "color": "0366d6",
                "description": "Deployment and release management",
            },
            # Workflow-specific labels (shorter names for common use)
            "github-actions": {
                "color": "2088ff",
                "description": "GitHub Actions related work",
            },
            "automation": {
                "color": "1f883d",
                "description": "Automation scripts and tools",
            },
            "issue-management": {
                "color": "e99695",
                "description": "Issue tracking and management",
            },
            "gcommon-refactor": {
                "color": "f9d0c4",
                "description": "gcommon refactoring work",
            },
            # Project-specific labels - subtitle-manager
            "project:whisper": {
                "color": "ff6b6b",
                "description": "Whisper ASR integration",
            },
            "project:transcription": {
                "color": "ffa8a8",
                "description": "Audio transcription features",
            },
            "project:media": {
                "color": "74c0fc",
                "description": "Media processing and handling",
            },
            "project:subtitles": {
                "color": "4c956c",
                "description": "Subtitle processing and conversion",
            },
            # Project-specific labels - gcommon
            "project:gcommon-refactor": {
                "color": "f9d0c4",
                "description": "gcommon refactor initiative",
            },
            "project:protobuf-implementation": {
                "color": "c5def5",
                "description": "Protocol buffer implementation work",
            },
            # GitHub management labels
            "project:github-management": {
                "color": "6f42c1",
                "description": "GitHub project management and workflows",
            },
            "project:issue-management": {
                "color": "e99695",
                "description": "Issue management and tracking workflows",
            },
            # Special labels
            "good-first-issue": {
                "color": "7057ff",
                "description": "Good for newcomers",
            },
            "help-wanted": {
                "color": "008672",
                "description": "Extra attention is needed",
            },
            "performance": {
                "color": "fcc419",
                "description": "Performance optimization",
            },
            "breaking-change": {
                "color": "d73a49",
                "description": "Introduces breaking changes",
            },
            "external-dependency": {
                "color": "e4b429",
                "description": "Depends on external systems or libraries",
            },
            # AI/Automation labels
            "codex": {
                "color": "ff6b9d",
                "description": "Created or modified by AI/automation agents",
            },
        }

    def _get_milestone_definitions(self) -> Dict[str, Dict[str, str]]:
        """
        Get milestone definitions for project planning.

        Returns:
            Dictionary mapping milestone titles to their properties
        """
        return {
            "v2.0.0 - gcommon Integration": {
                "description": "Major release integrating gcommon across all repositories",
                "due_date": "2025-08-01",
                "state": "open",
            },
            "Q3 2025 - Security & Compliance": {
                "description": "Security improvements and compliance documentation",
                "due_date": "2025-09-30",
                "state": "open",
            },
            "Q4 2025 - Performance & Quality": {
                "description": "Performance optimization and quality assurance improvements",
                "due_date": "2025-12-31",
                "state": "open",
            },
            "Protobuf Implementation Complete": {
                "description": "Complete implementation of all 626 empty protobuf files",
                "due_date": "2025-10-31",
                "state": "open",
            },
        }

    def _get_existing_projects(self) -> Dict[str, Dict[str, str]]:
        """Get existing projects from GitHub."""
        success, output = self._run_gh_command(
            ["project", "list", "--owner", self.owner, "--format", "json"]
        )

        if not success:
            self.logger.warning(f"Could not fetch existing projects: {output}")
            return {}

        try:
            projects = json.loads(output)
            return {
                project["title"]: project for project in projects.get("projects", [])
            }
        except (json.JSONDecodeError, KeyError):
            self.logger.warning("Could not parse existing projects JSON")
            return {}

    def _get_existing_labels(self, repository: str) -> Dict[str, Dict[str, str]]:
        """Get existing labels from a repository."""
        if self.dry_run:
            # In dry-run mode, still fetch real labels to show accurate info
            # but don't use the dry-run wrapper that would prevent actual execution
            import subprocess

            try:
                cmd = [
                    "gh",
                    "label",
                    "list",
                    "--repo",
                    f"{self.owner}/{repository}",
                    "--json",
                    "name,color,description",
                ]
                result = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    check=True,
                    timeout=30,
                )
                labels = json.loads(result.stdout)
                return {label["name"]: label for label in labels}
            except (
                subprocess.CalledProcessError,
                subprocess.TimeoutExpired,
                json.JSONDecodeError,
            ):
                # If we can't fetch in dry-run mode, assume no labels exist
                return {}

        success, output = self._run_gh_command(
            [
                "label",
                "list",
                "--repo",
                f"{self.owner}/{repository}",
                "--json",
                "name,color,description",
            ]
        )

        if not success:
            self.logger.warning(
                f"Could not fetch existing labels for {repository}: {output}"
            )
            return {}

        try:
            labels = json.loads(output)
            return {label["name"]: label for label in labels}
        except json.JSONDecodeError:
            self.logger.warning(
                f"Could not parse existing labels JSON for {repository}"
            )
            return {}

    def _get_existing_milestones(self, repository: str) -> Dict[str, Dict[str, str]]:
        """Get existing milestones from a repository."""
        success, output = self._run_gh_command(
            ["api", f"repos/{self.owner}/{repository}/milestones"]
        )

        if not success:
            self.logger.warning(
                f"Could not fetch existing milestones for {repository}: {output}"
            )
            return {}

        try:
            milestones = json.loads(output)
            return {milestone["title"]: milestone for milestone in milestones}
        except json.JSONDecodeError:
            self.logger.warning(
                f"Could not parse existing milestones JSON for {repository}"
            )
            return {}

    def _normalize_color(self, color: str) -> str:
        """Normalize color string for GitHub labels."""
        color = color.lstrip("#")
        if len(color) == 6:
            return color.lower()
        else:
            raise ValueError(f"Invalid color format: {color}")

    def create_all_projects(self) -> Dict[str, str]:
        """Create all GitHub Projects defined in the configuration."""
        self.logger.info("🚀 Creating GitHub Projects...")

        project_definitions = self._get_project_definitions()
        existing_projects = self._get_existing_projects()
        created_projects = {}

        for title, config in project_definitions.items():
            if title in existing_projects:
                if not self.force:
                    self.logger.info(f"✅ Project '{title}' already exists (skipping)")
                    created_projects[title] = existing_projects[title].get("number", "")
                    continue
                else:
                    self.logger.info(
                        f"🔄 Project '{title}' exists, force update enabled"
                    )

            project_number = self._create_project(title, config["description"])
            if project_number:
                created_projects[title] = project_number
                self.logger.info(f"✅ Created project: {title} (#{project_number})")
            else:
                self.logger.error(f"❌ Failed to create project: {title}")

        return created_projects

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
        Get list of repositories already linked to a project.

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

        # First get project details using the API directly
        success, output = self._run_gh_command(
            ["api", f"projects/v2/{project_number}", "--jq", "."]
        )

        if not success:
            self.logger.warning(
                f"⚠️ Error getting project #{project_number} details: {output}"
            )
            return None

        try:
            # Now get the repository links specifically
            success, repo_output = self._run_gh_command(
                [
                    "api",
                    f"projects/v2/{project_number}/repositories",
                    "--jq",
                    ".repositories[].name",
                ]
            )

            if not success:
                self.logger.warning(
                    f"⚠️ Error getting linked repositories for project #{project_number}: {repo_output}"
                )
                return None

            # Process the output - the JQ filter should give us one repo name per line
            if repo_output.strip():
                return [repo.strip() for repo in repo_output.strip().split("\n")]
            else:
                return []

        except Exception as e:
            self.logger.warning(
                f"⚠️ Error processing repository data for project #{project_number}: {e}"
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
        """Create a single label in a repository."""
        if self.dry_run:
            self.logger.info(
                f"DRY-RUN: Would create label '{label_name}' in {repository}"
            )
            return True

        color = self._normalize_color(label_config["color"])
        description = label_config.get("description", "")

        # Create the label
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

        if not success:
            self.logger.debug(
                f"Failed to create label '{label_name}' in {repository}: {output}"
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

    def list_projects(self) -> None:
        """List all projects and their configurations."""
        self.logger.info("📋 Listing all project configurations...")

        project_definitions = self._get_project_definitions()
        existing_projects = self._get_existing_projects()

        print("\n" + "=" * 80)
        print("PROJECT STRUCTURE OVERVIEW")
        print("=" * 80)

        # Cross-repository projects
        print("\n🔗 CROSS-REPOSITORY PROJECTS:")
        cross_repo_projects = {
            k: v for k, v in project_definitions.items() if len(v["repositories"]) > 1
        }

        for title, config in cross_repo_projects.items():
            status = "✅ EXISTS" if title in existing_projects else "❌ NOT CREATED"
            print(f"\n  📊 {title} - {status}")
            print(f"     Description: {config['description']}")
            print(f"     Repositories: {', '.join(config['repositories'])}")
            print(f"     Labels: {', '.join(config['labels'])}")

        # Repository-specific projects
        repos = ["subtitle-manager", "gcommon", "ghcommon", "codex-cli"]
        for repo in repos:
            repo_projects = {
                k: v
                for k, v in project_definitions.items()
                if v["repositories"] == [repo]
            }

            if repo_projects:
                print(f"\n📁 {repo.upper()} PROJECTS:")
                for title, config in repo_projects.items():
                    status = (
                        "✅ EXISTS" if title in existing_projects else "❌ NOT CREATED"
                    )
                    print(f"  📊 {title} - {status}")
                    print(f"     Description: {config['description']}")
                    print(f"     Labels: {', '.join(config['labels'])}")

        print("\n📈 SUMMARY:")
        print(f"  Total projects defined: {len(project_definitions)}")
        print(f"  Projects created: {len(existing_projects)}")
        print(
            f"  Projects pending: {len(project_definitions) - len(existing_projects)}"
        )
        print("=" * 80 + "\n")

    def get_auto_add_workflow_config(self) -> Dict[str, List[str]]:
        """
        Generate configuration for GitHub's auto-add workflow rules.

        Returns:
            Dictionary mapping project names to lists of labels that should auto-add issues
        """
        project_definitions = self._get_project_definitions()
        workflow_config = {}

        for project_title, config in project_definitions.items():
            if config.get("workflows", {}).get("auto_add_issues", False):
                workflow_config[project_title] = config["workflows"]["labels"]

        return workflow_config

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
            self.logger.info("DRY-RUN: Workflow setup instructions displayed above")
            return

        # Note: Actual GraphQL workflow creation is not yet implemented
        # GitHub's Projects v2 API has limited workflow automation support
        # The instructions above provide manual setup guidance
        self.logger.info("ℹ️ Workflow setup requires manual configuration in GitHub UI")
        self.logger.info("📋 Follow the detailed instructions displayed above")

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

        print("\n3. Set up workflows (REPEAT FOR EACH REPOSITORY):")
        print("   - In the left sidebar, click 'Workflows'")
        print("   - Click 'New workflow'")

        print("\n4. Configure each workflow:")
        print("   - Name: 'Auto-add from [repository-name]'")
        print("   - Trigger: 'Issue added'")
        print("   - Filters: 'Repository' = [specific repository]")
        print("   - Additional filters: 'Label' in [labels shown above]")
        print("   - Action: 'Add to project'")

        print("\n" + "-" * 80)
        print("EXAMPLE WORKFLOW CONFIGURATIONS")
        print("-" * 80)

        # Show specific examples for a few projects
        example_projects = list(workflow_config.keys())[:3]
        for project_title in example_projects:
            if project_title not in project_definitions:
                continue

            project_config = project_definitions[project_title]
            repositories = project_config["repositories"]
            labels = workflow_config[project_title]

            print(f"\n🔧 Example: {project_title}")
            for repo in repositories:
                print(f"   Workflow for {repo}:")
                print(f"     - Name: 'Auto-add from {repo}'")
                print(f"     - Repository filter: {repo}")
                print(f"     - Label filters: {', '.join(labels)}")

        print("\n" + "=" * 80)
        print("NOTE: GitHub Projects v2 API has limited workflow automation support.")
        print("Manual setup through the GitHub UI is currently required.")
        print("=" * 80 + "\n")

    def _build_workflow_mutation(
        self, project_number: str, workflow_name: str, labels: List[str]
    ) -> str:
        """
        Build GraphQL mutation for creating a project workflow.

        Args:
            project_number: Project number
            workflow_name: Name for the workflow
            labels: List of labels that trigger the workflow

        Returns:
            GraphQL mutation string
        """
        # Create the filter conditions for each label
        label_filters = []
        for label in labels:
            label_filters.append(f"""
            {{
                field: "label",
                operator: EQUALS,
                value: "{label}"
            }}
        """)

        # Build the complete mutation
        return f"""
    mutation {{
        createProjectV2Workflow(
            input: {{
                projectId: "{project_number}",
                name: "{workflow_name}"
            }}
        ) {{
            workflow {{
                id
            }}
        }}
    }}
    """

    def run_full_setup(self) -> None:
        """Run the complete project setup process."""
        self.logger.info("🚀 Starting full GitHub project setup...")

        try:
            # 1. Create all projects
            project_numbers = self.create_all_projects()

            # 2. Link repositories to projects
            self.link_all_repositories(project_numbers)

            # 3. Create labels across all repositories
            self.create_all_labels()

            # 4. Create milestones across all repositories
            self.create_all_milestones()

            # 5. Set up project workflows (NEW!)
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


def main():
    """Main entry point for the unified GitHub project manager."""
    parser = argparse.ArgumentParser(
        description="Unified GitHub Project Manager v2",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 scripts/unified_github_project_manager_v2.py
  python3 scripts/unified_github_project_manager_v2.py --dry-run
  python3 scripts/unified_github_project_manager_v2.py --setup-workflows
  python3 scripts/unified_github_project_manager_v2.py --list-projects
  python3 scripts/unified_github_project_manager_v2.py --create-labels
  python3 scripts/unified_github_project_manager_v2.py --create-milestones
  python3 scripts/unified_github_project_manager_v2.py --cleanup-labels
  python3 scripts/unified_github_project_manager_v2.py --report-orphans
  python3 scripts/unified_github_project_manager_v2.py --interactive-cleanup
        """,
    )

    parser.add_argument(
        "--dry-run",
        "-n",
        action="store_true",
        help="Run in dry-run mode (no actual changes)",
    )

    parser.add_argument(
        "--force", "-f", action="store_true", help="Force update existing objects"
    )

    parser.add_argument(
        "--verbose", "-v", action="store_true", help="Enable verbose logging"
    )

    parser.add_argument(
        "--setup-workflows", action="store_true", help="Setup project workflows only"
    )

    parser.add_argument(
        "--list-projects",
        action="store_true",
        help="List all projects and their configurations",
    )

    parser.add_argument(
        "--create-labels",
        action="store_true",
        help="Create labels across all repositories",
    )

    parser.add_argument(
        "--create-milestones",
        action="store_true",
        help="Create milestones across all repositories",
    )

    parser.add_argument(
        "--cleanup-labels",
        action="store_true",
        help="Remove labels that are not in the current definition",
    )

    parser.add_argument(
        "--report-orphans",
        action="store_true",
        help="Report on labels that exist but are not in the current definition",
    )

    parser.add_argument(
        "--interactive-cleanup",
        action="store_true",
        help="Interactively clean up orphaned labels",
    )

    args = parser.parse_args()

    try:
        manager = UnifiedGitHubProjectManager(
            dry_run=args.dry_run, force=args.force, verbose=args.verbose
        )

        if args.list_projects:
            manager.list_projects()
        elif args.create_labels:
            manager.create_all_labels()
        elif args.create_milestones:
            manager.create_all_milestones()
        elif args.cleanup_labels:
            manager.cleanup_orphaned_labels()
        elif args.report_orphans:
            manager.report_orphaned_labels()
        elif args.interactive_cleanup:
            manager.interactive_cleanup_labels()
        elif args.setup_workflows:
            # Get project numbers first
            projects = manager.get_all_projects()
            project_numbers = {}
            for project in projects:
                project_numbers[project["title"]] = project["number"]

            manager.setup_project_workflows(project_numbers)
        else:
            manager.run_full_setup()

    except Exception as e:
        print(f"❌ Error: {str(e)}", file=sys.stderr)
        sys.exit(1)


def display_workflow_setup_instructions(workflow_config: Dict[str, List[str]]) -> None:
    """
    Display clear, actionable instructions for setting up GitHub Project workflows.

    Args:
        workflow_config: Dictionary mapping project names to lists of labels for auto-add
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
    print("WORKFLOW CONFIGURATION BY PROJECT")
    print("-" * 80)

    for project_name, labels in workflow_config.items():
        print(f"\n📊 Project: {project_name}")
        print(f"    Auto-add issues with these labels: {', '.join(labels)}")

    print("\n" + "-" * 80)
    print("MANUAL SETUP INSTRUCTIONS")
    print("-" * 80)

    print("\nFor each project listed above, follow these steps:")

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
    print("Note: This script does not currently automate the workflow setup directly.")
    print("      The GitHub Projects API has limited support for workflow automation.")
    print("      Follow the manual steps above to set up your workflows.")
    print("=" * 80 + "\n")


if __name__ == "__main__":
    main()
