#!/usr/bin/env python3
# file: scripts/sync-repo-setup.py
# version: 1.0.0
# guid: f1a2b3c4-d5e6-f7a8-b9c0-d1e2f3a4b5c6

"""
Sync repository setup files from ghcommon to other repositories.

This script copies:
1. .github/instructions/ directory
2. .github/copilot-instructions.md
3. .github/AGENTS.md and related AI files
4. .github/dependabot.yml (with intelligent detection)
5. Other setup files

The script detects the programming languages in each repository and
customizes the dependabot.yml configuration accordingly.
"""

import shutil
from pathlib import Path
from typing import Dict, List, Any, Optional, Tuple
import argparse
import logging
import yaml
import re

# Configure logging
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


def extract_version_from_content(content: str) -> Optional[str]:
    """Extract version from file content."""
    # Look for version patterns in various comment formats
    patterns = [
        r"<!-- version: ([\d.]+) -->",  # HTML/Markdown comments
        r"# version: ([\d.]+)",  # Shell/Python comments
        r"// version: ([\d.]+)",  # JavaScript/Go comments
        r"/\* version: ([\d.]+) \*/",  # CSS comments
    ]

    for pattern in patterns:
        match = re.search(pattern, content)
        if match:
            return match.group(1)

    return None


def compare_versions(version1: str, version2: str) -> int:
    """Compare semantic versions. Returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2."""

    def parse_version(v):
        return [int(x) for x in v.split(".")]

    try:
        v1_parts = parse_version(version1)
        v2_parts = parse_version(version2)

        # Pad with zeros to same length
        max_len = max(len(v1_parts), len(v2_parts))
        v1_parts.extend([0] * (max_len - len(v1_parts)))
        v2_parts.extend([0] * (max_len - len(v2_parts)))

        for a, b in zip(v1_parts, v2_parts):
            if a < b:
                return -1
            elif a > b:
                return 1

        return 0
    except (ValueError, AttributeError):
        # If version parsing fails, treat as equal
        return 0


def should_overwrite_file(source_file: Path, target_file: Path) -> Tuple[bool, str]:
    """Check if source file should overwrite target file based on version."""
    if not target_file.exists():
        return True, "target file doesn't exist"

    try:
        # Read both files
        with open(source_file, "r", encoding="utf-8") as f:
            source_content = f.read()
        with open(target_file, "r", encoding="utf-8") as f:
            target_content = f.read()

        # Extract versions
        source_version = extract_version_from_content(source_content)
        target_version = extract_version_from_content(target_content)

        # If no version info, check content equality
        if not source_version and not target_version:
            if source_content == target_content:
                return False, "files are identical"
            else:
                return True, "files differ (no version info)"

        # If only source has version, overwrite
        if source_version and not target_version:
            return True, f"source has version {source_version}, target has none"

        # If only target has version, don't overwrite
        if not source_version and target_version:
            return False, f"target has version {target_version}, source has none"

        # Both have versions - compare
        comparison = compare_versions(source_version, target_version)
        if comparison > 0:
            return (
                True,
                f"source version {source_version} > target version {target_version}",
            )
        elif comparison < 0:
            return (
                False,
                f"target version {target_version} > source version {source_version}",
            )
        else:
            # Same version - check content
            if source_content == target_content:
                return False, f"same version {source_version}, identical content"
            else:
                return True, f"same version {source_version}, content differs"

    except UnicodeDecodeError:
        # For binary files, fall back to simple comparison
        try:
            with open(source_file, "rb") as f:
                source_bytes = f.read()
            with open(target_file, "rb") as f:
                target_bytes = f.read()

            if source_bytes == target_bytes:
                return False, "binary files are identical"
            else:
                return True, "binary files differ"
        except Exception:
            return True, "unable to compare files"

    except Exception as e:
        logger.warning(f"Error comparing versions for {target_file}: {e}")
        return True, f"comparison error: {e}"


logger = logging.getLogger(__name__)


def extract_version_from_file(file_path: Path) -> str:
    """Extract version from file header comments."""
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            content = f.read()

        # Look for version in various comment formats
        patterns = [
            r"<!-- version: ([^\s]+) -->",  # HTML comments
            r"# version: ([^\s]+)",  # Shell/Python comments
            r"// version: ([^\s]+)",  # JavaScript/Go comments
            r"/\* version: ([^\s]+) \*/",  # CSS/C comments
        ]

        for pattern in patterns:
            match = re.search(pattern, content)
            if match:
                return match.group(1)

        return "0.0.0"  # Default if no version found

    except Exception:
        return "0.0.0"


class RepositoryAnalyzer:
    """Analyze repositories to determine programming languages and dependencies."""

    def __init__(self):
        self.language_indicators = {
            "go": ["go.mod", "go.sum", "*.go"],
            "python": ["requirements.txt", "setup.py", "pyproject.toml", "*.py"],
            "nodejs": [
                "package.json",
                "package-lock.json",
                "yarn.lock",
                "*.js",
                "*.ts",
            ],
            "docker": ["Dockerfile", "docker-compose.yml", "docker-compose.yaml"],
            "github-actions": [".github/workflows/*.yml", ".github/workflows/*.yaml"],
        }

    def detect_languages(self, repo_path: Path) -> Dict[str, bool]:
        """Detect programming languages used in a repository."""
        languages = {}

        for language, indicators in self.language_indicators.items():
            languages[language] = self._check_indicators(repo_path, indicators)

        return languages

    def _check_indicators(self, repo_path: Path, indicators: List[str]) -> bool:
        """Check if any of the indicators exist in the repository."""
        for indicator in indicators:
            if "*" in indicator:
                # Use glob pattern
                if list(repo_path.glob(indicator)):
                    return True
                # Also check subdirectories for certain patterns
                if list(repo_path.glob(f"**/{indicator}")):
                    return True
            else:
                # Check exact file
                if (repo_path / indicator).exists():
                    return True

        return False

    def get_directory_structure(self, repo_path: Path) -> Dict[str, Any]:
        """Get basic directory structure information."""
        structure = {
            "has_github_dir": (repo_path / ".github").exists(),
            "has_workflows": (repo_path / ".github" / "workflows").exists(),
            "has_instructions": (repo_path / ".github" / "instructions").exists(),
            "has_dependabot": (repo_path / ".github" / "dependabot.yml").exists(),
        }

        return structure


class DependabotGenerator:
    """Generate dependabot.yml configurations based on detected languages."""

    def __init__(self):
        self.base_config = {"version": 2, "updates": []}

    def generate_config(
        self, languages: Dict[str, bool], repo_name: str
    ) -> Dict[str, Any]:
        """Generate dependabot configuration based on detected languages."""
        config = self.base_config.copy()
        config["updates"] = []

        # Add comment at the top
        comment = f"# Smart Dependabot configuration for {repo_name}\n"
        comment += f"# Automatically detected: {', '.join([lang for lang, detected in languages.items() if detected])}\n"

        if languages.get("nodejs"):
            config["updates"].append(self._get_nodejs_config())

        if languages.get("python"):
            config["updates"].append(self._get_python_config())

        if languages.get("go"):
            config["updates"].append(self._get_go_config())

        if languages.get("docker"):
            config["updates"].append(self._get_docker_config())

        if languages.get("github-actions"):
            config["updates"].append(self._get_github_actions_config())

        return config, comment

    def _get_nodejs_config(self) -> Dict[str, Any]:
        """Get Node.js/npm dependabot configuration."""
        return {
            "package-ecosystem": "npm",
            "directory": "/",
            "schedule": {
                "interval": "weekly",
                "day": "monday",
                "time": "09:00",
                "timezone": "America/New_York",
            },
            "open-pull-requests-limit": 8,
            "commit-message": {"prefix": "npm", "include": "scope"},
            "labels": ["dependencies", "priority-low"],
            "allow": [{"dependency-type": "direct"}, {"dependency-type": "indirect"}],
            "ignore": [
                {
                    "dependency-name": "*",
                    "update-types": ["version-update:semver-major"],
                }
            ],
            "groups": {
                "dev-dependencies": {
                    "patterns": ["*eslint*", "*prettier*", "*commitlint*", "@types/*"],
                    "update-types": ["minor", "patch"],
                }
            },
        }

    def _get_python_config(self) -> Dict[str, Any]:
        """Get Python/pip dependabot configuration."""
        return {
            "package-ecosystem": "pip",
            "directory": "/",
            "schedule": {
                "interval": "weekly",
                "day": "tuesday",
                "time": "09:00",
                "timezone": "America/New_York",
            },
            "open-pull-requests-limit": 3,
            "commit-message": {"prefix": "python", "include": "scope"},
            "labels": ["dependencies", "priority-medium"],
            "allow": [{"dependency-type": "direct"}, {"dependency-type": "indirect"}],
        }

    def _get_go_config(self) -> Dict[str, Any]:
        """Get Go modules dependabot configuration."""
        return {
            "package-ecosystem": "gomod",
            "directory": "/",
            "schedule": {
                "interval": "weekly",
                "day": "wednesday",
                "time": "09:00",
                "timezone": "America/New_York",
            },
            "open-pull-requests-limit": 5,
            "commit-message": {"prefix": "go", "include": "scope"},
            "labels": ["dependencies", "priority-medium"],
        }

    def _get_docker_config(self) -> Dict[str, Any]:
        """Get Docker dependabot configuration."""
        return {
            "package-ecosystem": "docker",
            "directory": "/",
            "schedule": {
                "interval": "weekly",
                "day": "thursday",
                "time": "09:00",
                "timezone": "America/New_York",
            },
            "open-pull-requests-limit": 3,
            "commit-message": {"prefix": "docker", "include": "scope"},
            "labels": ["dependencies", "priority-low"],
        }

    def _get_github_actions_config(self) -> Dict[str, Any]:
        """Get GitHub Actions dependabot configuration."""
        return {
            "package-ecosystem": "github-actions",
            "directory": "/",
            "schedule": {
                "interval": "weekly",
                "day": "friday",
                "time": "09:00",
                "timezone": "America/New_York",
            },
            "open-pull-requests-limit": 5,
            "commit-message": {"prefix": "ci", "include": "scope"},
            "labels": ["dependencies", "priority-high"],
        }


class RepoSetupSyncer:
    """Main class for syncing repository setup files."""

    def __init__(self, source_repo: Path, dry_run: bool = False):
        self.source_repo = source_repo
        self.dry_run = dry_run
        self.analyzer = RepositoryAnalyzer()
        self.dependabot_generator = DependabotGenerator()

        # Files and directories to sync
        self.sync_files = [
            ".github/copilot-instructions.md",
            ".github/AGENTS.md",
            ".github/CLAUDE.md",
            ".github/repository-setup.md",
            ".github/security-guidelines.md",
            ".github/workflow-usage.md",
            ".github/review-selection.md",
            ".prettierrc",
            ".prettierignore",
            ".yamllint",
        ]

        self.sync_directories = [
            ".github/instructions",
            ".github/copilot",
            ".github/prompts",
            ".github/ISSUE_TEMPLATE",
            ".github/PULL_REQUEST_TEMPLATE",
        ]

    def find_target_repositories(self) -> List[Path]:
        """Find all target repositories to sync to using GitHub CLI."""
        import subprocess
        import json

        repos = []

        try:
            # Use GitHub CLI to get repositories owned by the user (not forks)
            logger.info("Fetching your repositories from GitHub...")
            cmd = [
                "gh",
                "repo",
                "list",
                "--json",
                "name,owner,isFork,isPrivate",
                "--limit",
                "100",
            ]

            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            github_repos = json.loads(result.stdout)

            # Get current user
            user_cmd = ["gh", "api", "user", "--jq", ".login"]
            user_result = subprocess.run(
                user_cmd, capture_output=True, text=True, check=True
            )
            current_user = user_result.stdout.strip()

            logger.info(
                f"Found {len(github_repos)} repositories on GitHub for user: {current_user}"
            )

            # Filter to only repos owned by the user and not forks
            owned_repos = [
                repo
                for repo in github_repos
                if repo["owner"]["login"] == current_user and not repo["isFork"]
            ]

            logger.info(
                f"Filtered to {len(owned_repos)} owned repositories (excluding forks)"
            )

            # Find local clones of these repositories
            # Look in common Git hosting directory structures
            possible_base_dirs = [
                Path.home() / "repos" / "github.com" / current_user,
                Path.home() / "repos" / current_user,
                Path.home() / "code" / current_user,
                Path.home() / "git" / current_user,
                Path.home() / "workspace" / current_user,
                self.source_repo.parent,  # Same directory as current repo
            ]

            # Also check if we're in a nested structure like /Users/user/repos/github.com/user/
            current_path = self.source_repo.absolute()
            for i in range(len(current_path.parts)):
                potential_base = Path(*current_path.parts[: i + 1])
                if (
                    potential_base.name == current_user
                    and potential_base.parent.exists()
                ):
                    possible_base_dirs.append(potential_base)

            logger.debug(
                f"Searching for local repositories in: {[str(d) for d in possible_base_dirs]}"
            )

            for repo_info in owned_repos:
                repo_name = repo_info["name"]

                # Skip the current repository
                if repo_name == self.source_repo.name:
                    continue

                # Look for local clone
                found_local = False
                for base_dir in possible_base_dirs:
                    if not base_dir.exists():
                        continue

                    local_repo_path = base_dir / repo_name
                    if local_repo_path.exists() and (local_repo_path / ".git").exists():
                        repos.append(local_repo_path)
                        logger.debug(f"  Found local clone: {local_repo_path}")
                        found_local = True
                        break

                if not found_local:
                    logger.debug(f"  No local clone found for: {repo_name}")

            logger.info(f"Found {len(repos)} local repository clones to sync")

        except subprocess.CalledProcessError as e:
            logger.error(f"Error running GitHub CLI: {e}")
            logger.error("Make sure 'gh' is installed and you're authenticated")
            logger.info("Falling back to local directory scan...")

            # Fallback to original directory-based discovery
            parent_dir = self.source_repo.parent
            logger.debug(f"Looking for repositories in: {parent_dir}")

            for repo_dir in parent_dir.iterdir():
                logger.debug(f"Checking: {repo_dir.name}")
                if (
                    repo_dir.is_dir()
                    and repo_dir.name != self.source_repo.name
                    and (repo_dir / ".git").exists()
                ):
                    repos.append(repo_dir)
                    logger.debug(f"  Added: {repo_dir.name}")

        except Exception as e:
            logger.error(f"Unexpected error: {e}")
            return []

        return sorted(repos, key=lambda x: x.name)

    def sync_repository(self, target_repo: Path) -> Dict[str, Any]:
        """Sync setup files to a target repository."""
        logger.info(f"Syncing to: {target_repo.name}")

        result = {
            "repo_name": target_repo.name,
            "languages": {},
            "structure": {},
            "files_synced": [],
            "files_skipped": [],
            "directories_synced": [],
            "directories_skipped": [],
            "dependabot_generated": False,
            "errors": [],
        }

        try:
            # Analyze the repository
            result["languages"] = self.analyzer.detect_languages(target_repo)
            result["structure"] = self.analyzer.get_directory_structure(target_repo)

            # Ensure .github directory exists
            github_dir = target_repo / ".github"
            if not github_dir.exists():
                if not self.dry_run:
                    github_dir.mkdir(parents=True)
                logger.info("  Created .github directory")

            # Sync individual files
            for file_path in self.sync_files:
                source_file = self.source_repo / file_path
                target_file = target_repo / file_path

                if source_file.exists():
                    synced, reason = self._sync_file(source_file, target_file)
                    if synced:
                        result["files_synced"].append(file_path)
                    else:
                        result["files_skipped"].append((file_path, reason))

            # Sync directories
            for dir_path in self.sync_directories:
                source_dir = self.source_repo / dir_path
                target_dir = target_repo / dir_path

                if source_dir.exists():
                    synced, reason = self._sync_directory(source_dir, target_dir)
                    if synced:
                        result["directories_synced"].append(dir_path)
                    else:
                        result["directories_skipped"].append((dir_path, reason))

            # Generate dependabot.yml
            if self._should_generate_dependabot(result["languages"]):
                if self._generate_dependabot(target_repo, result["languages"]):
                    result["dependabot_generated"] = True

        except Exception as e:
            result["errors"].append(str(e))
            logger.error(f"Error syncing {target_repo.name}: {e}")

        return result

    def _sync_file(self, source_file: Path, target_file: Path) -> Tuple[bool, str]:
        """Sync a single file with version checking."""
        try:
            # Create parent directory if needed
            target_file.parent.mkdir(parents=True, exist_ok=True)

            # Check if we should overwrite the file
            should_overwrite, reason = should_overwrite_file(source_file, target_file)

            if not should_overwrite:
                logger.debug(
                    f"  Skipped file ({reason}): {target_file.relative_to(target_file.parents[1])}"
                )
                return (
                    False,
                    reason,
                )  # Return False and reason to indicate no sync was needed

            # Files should be overwritten
            if not self.dry_run:
                shutil.copy2(source_file, target_file)

            action = "Would sync" if self.dry_run else "Synced"
            logger.info(
                f"  {action} file ({reason}): {target_file.relative_to(target_file.parents[1])}"
            )
            return True, reason

        except Exception as e:
            logger.error(f"  Error syncing file {source_file}: {e}")
            return False, f"error: {e}"

    def _sync_directory(self, source_dir: Path, target_dir: Path) -> Tuple[bool, str]:
        """Sync a directory recursively."""
        try:
            # Check if directories are identical
            if target_dir.exists():
                try:
                    # Use filecmp to compare directory trees
                    import filecmp

                    comparison = filecmp.dircmp(str(source_dir), str(target_dir))

                    def dirs_identical(dcmp):
                        """Recursively check if directories are identical."""
                        if (
                            dcmp.left_only
                            or dcmp.right_only
                            or dcmp.diff_files
                            or dcmp.funny_files
                        ):
                            return False

                        for sub_dcmp in dcmp.subdirs.values():
                            if not dirs_identical(sub_dcmp):
                                return False

                        return True

                    if dirs_identical(comparison):
                        logger.debug(
                            f"  Skipped directory (already identical): {target_dir.relative_to(target_dir.parents[1])}"
                        )
                        return (
                            False,
                            "already identical",
                        )  # Return False and reason to indicate no sync was needed

                except Exception:
                    # If comparison fails, proceed with sync
                    pass

            # Directories are different or target doesn't exist, so sync
            if not self.dry_run:
                if target_dir.exists():
                    shutil.rmtree(target_dir)
                shutil.copytree(source_dir, target_dir)

            action = "Would sync" if self.dry_run else "Synced"
            logger.info(
                f"  {action} directory: {target_dir.relative_to(target_dir.parents[1])}"
            )
            return True, "directories differ"

        except Exception as e:
            logger.error(f"  Error syncing directory {source_dir}: {e}")
            return False, f"error: {e}"

    def _should_generate_dependabot(self, languages: Dict[str, bool]) -> bool:
        """Check if dependabot.yml should be generated."""
        # Generate if any supported language is detected
        supported_languages = ["nodejs", "python", "go", "docker", "github-actions"]
        return any(languages.get(lang, False) for lang in supported_languages)

    def _generate_dependabot(
        self, target_repo: Path, languages: Dict[str, bool]
    ) -> bool:
        """Generate dependabot.yml configuration."""
        try:
            config, comment = self.dependabot_generator.generate_config(
                languages, target_repo.name
            )

            target_file = target_repo / ".github" / "dependabot.yml"

            # Create the YAML content with comment
            yaml_content = yaml.dump(config, default_flow_style=False, sort_keys=False)
            new_content = comment + "\n" + yaml_content

            # Check if the content is already identical
            if target_file.exists():
                try:
                    with open(target_file, "r", encoding="utf-8") as f:
                        existing_content = f.read()

                    if existing_content == new_content:
                        logger.debug("  Skipped dependabot.yml (already up-to-date)")
                        return (
                            False  # Return False to indicate no generation was needed
                        )
                except Exception:
                    # If we can't read the file, proceed with generation
                    pass

            # Content is different or file doesn't exist, so generate
            if not self.dry_run:
                target_file.parent.mkdir(parents=True, exist_ok=True)
                with open(target_file, "w") as f:
                    f.write(new_content)

            action = "Would generate" if self.dry_run else "Generated"
            logger.info(f"  {action} dependabot.yml")
            return True

        except Exception as e:
            logger.error(f"  Error generating dependabot.yml: {e}")
            return False

    def sync_all_repositories(self) -> Dict[str, Any]:
        """Sync setup files to all target repositories."""
        target_repos = self.find_target_repositories()

        logger.info(f"Found {len(target_repos)} target repositories")
        if self.dry_run:
            logger.info("DRY RUN MODE - No files will be modified")

        results = {}

        for repo in target_repos:
            results[repo.name] = self.sync_repository(repo)

        return results

    def print_summary(self, results: Dict[str, Any]) -> None:
        """Print a summary of the sync operation."""
        print("\n" + "=" * 70)
        print("REPOSITORY SETUP SYNC SUMMARY")
        print("=" * 70)

        total_repos = len(results)
        successful_repos = 0
        total_files_synced = 0
        total_files_skipped = 0
        total_dirs_synced = 0
        total_dirs_skipped = 0
        dependabot_generated = 0

        for repo_name, result in results.items():
            if not result["errors"]:
                successful_repos += 1

            total_files_synced += len(result["files_synced"])
            total_files_skipped += len(result["files_skipped"])
            total_dirs_synced += len(result["directories_synced"])
            total_dirs_skipped += len(result["directories_skipped"])

            if result["dependabot_generated"]:
                dependabot_generated += 1

            # Print repo details
            status = "✅" if not result["errors"] else "❌"
            print(f"\n{status} {repo_name}")

            # Show detected languages
            detected_langs = [
                lang for lang, detected in result["languages"].items() if detected
            ]
            if detected_langs:
                print(f"  Languages: {', '.join(detected_langs)}")

            # Show what was synced
            files_count = len(result["files_synced"])
            files_skipped_count = len(result["files_skipped"])
            dirs_count = len(result["directories_synced"])
            dirs_skipped_count = len(result["directories_skipped"])

            if files_count > 0:
                print(f"  Files synced: {files_count}")
            else:
                print("  Files: all up-to-date")

            if files_skipped_count > 0:
                print(f"  Files already correct: {files_skipped_count}")

            if dirs_count > 0:
                print(f"  Directories synced: {dirs_count}")
            else:
                print("  Directories: all up-to-date")

            if dirs_skipped_count > 0:
                print(f"  Directories already correct: {dirs_skipped_count}")

            if result["dependabot_generated"]:
                print("  ✓ Generated dependabot.yml")
            else:
                print("  Dependabot: up-to-date")

            # Show errors
            if result["errors"]:
                for error in result["errors"]:
                    print(f"  ❌ Error: {error}")

        print("\nOverall Summary:")
        print(f"Total repositories: {total_repos}")
        print(f"Successful: {successful_repos}")
        print(f"Files synced: {total_files_synced}")
        print(f"Files already correct: {total_files_skipped}")
        print(f"Directories synced: {total_dirs_synced}")
        print(f"Directories already correct: {total_dirs_skipped}")
        print(f"Dependabot configs generated: {dependabot_generated}")

        if self.dry_run:
            print("\nThis was a dry run. Remove --dry-run to make actual changes.")


def main():
    parser = argparse.ArgumentParser(
        description="Sync repository setup files from ghcommon to other repositories"
    )
    parser.add_argument(
        "--source-repo",
        type=Path,
        default=Path("."),
        help="Source repository (ghcommon) path",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be synced without making changes",
    )
    parser.add_argument(
        "--verbose", "-v", action="store_true", help="Enable verbose output"
    )

    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    # Initialize syncer
    syncer = RepoSetupSyncer(args.source_repo, dry_run=args.dry_run)

    # Run the sync
    results = syncer.sync_all_repositories()

    # Print summary
    syncer.print_summary(results)

    return 0


if __name__ == "__main__":
    exit(main())
