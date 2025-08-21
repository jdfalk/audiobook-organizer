#!/usr/bin/env python3
# file: scripts/cleanup-archived-repos.py
# version: 1.0.1
# guid: f1a2b3c4-d5e6-7890-abcd-ef1234567890

"""
GitHub Repository Cleanup Script

Automatically detects archived or read-only repositories in the local file system
and optionally removes their local directories to free up disk space.

Featu                self.logger.info(f"Archived: {repo_info.get('isArchived', 'Unknown')}")
                self.logger.info(f"Private: {repo_info.get('isPrivate', 'Unknown')}")
                self.logger.info(f"Visibility: {repo_info.get('visibility', 'Unknown')}")s:
- Scans a specified directory for Git repositories
- Uses GitHub CLI to check repository status (archived, disabled, read-only)
- Provides interactive and non-interactive modes
- Generates detailed reports of actions taken
- Includes dry-run mode for safety
- Comprehensive logging and error handling
"""

import argparse
import json
import logging
import shutil
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple


class RepositoryCleanup:
    """Main class for repository cleanup operations."""

    def __init__(self, base_path: str, dry_run: bool = True, interactive: bool = True):
        """
        Initialize the repository cleanup tool.

        Args:
            base_path: Base directory containing repositories
            dry_run: If True, only show what would be done without making changes
            interactive: If True, prompt for confirmation before each action
        """
        self.base_path = Path(base_path).expanduser().resolve()
        self.dry_run = dry_run
        self.interactive = interactive
        self.log_file = (
            Path.home()
            / "logs"
            / f"repo-cleanup-{datetime.now().strftime('%Y%m%d_%H%M%S')}.log"
        )

        # Ensure logs directory exists
        self.log_file.parent.mkdir(exist_ok=True)

        # Setup logging
        self._setup_logging()

        # Statistics
        self.stats = {
            "total_repos": 0,
            "archived_repos": 0,
            "removed_repos": 0,
            "errors": 0,
            "skipped": 0,
        }

    def _setup_logging(self) -> None:
        """Configure logging to both file and console with broken pipe handling."""

        # Create a custom stream handler that ignores broken pipe errors
        class BrokenPipeHandler(logging.StreamHandler):
            def emit(self, record):
                try:
                    super().emit(record)
                except BrokenPipeError:
                    # Ignore broken pipe errors (occurs when output is piped to head, etc.)
                    pass
                except Exception:
                    # Handle other exceptions normally
                    self.handleError(record)

        logging.basicConfig(
            level=logging.INFO,
            format="%(asctime)s - %(levelname)s - %(message)s",
            handlers=[
                logging.FileHandler(self.log_file),
                BrokenPipeHandler(sys.stdout),
            ],
        )
        self.logger = logging.getLogger(__name__)

    def _run_command(
        self, cmd: List[str], cwd: Optional[Path] = None
    ) -> Tuple[bool, str, str]:
        """
        Run a shell command and return success status and output.

        Args:
            cmd: Command and arguments to run
            cwd: Working directory for the command

        Returns:
            Tuple of (success, stdout, stderr)
        """
        try:
            result = subprocess.run(
                cmd, cwd=cwd, capture_output=True, text=True, timeout=30
            )
            return result.returncode == 0, result.stdout.strip(), result.stderr.strip()
        except subprocess.TimeoutExpired:
            return False, "", "Command timed out"
        except Exception as e:
            return False, "", str(e)

    def _is_git_repository(self, path: Path) -> bool:
        """Check if a directory is a Git repository."""
        return (path / ".git").exists()

    def _get_remote_url(self, repo_path: Path) -> Optional[str]:
        """Get the remote URL for a Git repository."""
        success, stdout, stderr = self._run_command(
            ["git", "remote", "get-url", "origin"], repo_path
        )
        if success and stdout:
            return stdout.strip()
        return None

    def _parse_github_url(self, url: str) -> Optional[Tuple[str, str]]:
        """
        Parse GitHub URL to extract owner and repository name.

        Args:
            url: Git remote URL

        Returns:
            Tuple of (owner, repo_name) or None if not a GitHub URL
        """
        if not url:
            return None

        # Handle different URL formats
        if url.startswith("git@github.com:"):
            # SSH format: git@github.com:owner/repo.git
            parts = url.replace("git@github.com:", "").replace(".git", "").split("/")
        elif "github.com" in url:
            # HTTPS format: https://github.com/owner/repo.git
            parts = url.split("github.com/")[-1].replace(".git", "").split("/")
        else:
            return None

        if len(parts) >= 2:
            return parts[0], parts[1]
        return None

    def _get_repository_info(self, owner: str, repo: str) -> Optional[Dict]:
        """
        Get repository information from GitHub API using gh CLI.

        Args:
            owner: Repository owner
            repo: Repository name

        Returns:
            Dictionary with repository information or None on error
        """
        cmd = [
            "gh",
            "repo",
            "view",
            f"{owner}/{repo}",
            "--json",
            "isArchived,isPrivate,visibility,pushedAt,updatedAt",
        ]

        success, stdout, stderr = self._run_command(cmd)
        if success and stdout:
            try:
                return json.loads(stdout)
            except json.JSONDecodeError as e:
                self.logger.error(f"Failed to parse JSON for {owner}/{repo}: {e}")
                return None
        else:
            self.logger.warning(f"Failed to get info for {owner}/{repo}: {stderr}")
            return None

    def _should_remove_repository(
        self, repo_info: Dict, repo_path: Path
    ) -> Tuple[bool, str]:
        """
        Determine if a repository should be removed based on its status.

        Args:
            repo_info: Repository information from GitHub API
            repo_path: Local path to the repository

        Returns:
            Tuple of (should_remove, reason)
        """
        if repo_info.get("isArchived", False):
            return True, "Repository is archived"

        # Note: GitHub API doesn't have a 'disabled' field in the standard repo view response
        # We can check visibility instead for private/disabled repositories
        visibility = repo_info.get("visibility", "")
        if visibility == "private" and repo_info.get("isPrivate", False):
            # Additional check - if repo hasn't been updated in a very long time and is private,
            # it might be effectively disabled/abandoned
            updated_at = repo_info.get("updatedAt")
            if updated_at:
                try:
                    from datetime import timezone

                    if updated_at.endswith("Z"):
                        updated_at = updated_at[:-1] + "+00:00"
                    updated_date = datetime.fromisoformat(updated_at)
                    days_since_update = (datetime.now(timezone.utc) - updated_date).days
                    if days_since_update > 1095:  # 3 years for private repos
                        return (
                            True,
                            f"Private repository hasn't been updated in {days_since_update} days (appears abandoned)",
                        )
                except Exception as e:
                    self.logger.warning(
                        f"Failed to parse update date {updated_at}: {e}"
                    )

        # Check if repository hasn't been updated in over 2 years
        updated_at = repo_info.get("updatedAt")
        if updated_at:
            try:
                # Simple date parsing without external dependencies
                from datetime import timezone

                # Parse ISO format: 2023-01-15T12:30:45Z
                if updated_at.endswith("Z"):
                    updated_at = updated_at[:-1] + "+00:00"
                updated_date = datetime.fromisoformat(updated_at)
                days_since_update = (datetime.now(timezone.utc) - updated_date).days
                if days_since_update > 730:  # 2 years
                    return (
                        True,
                        f"Repository hasn't been updated in {days_since_update} days",
                    )
            except Exception as e:
                self.logger.warning(f"Failed to parse update date {updated_at}: {e}")

        return False, "Repository is active"

    def _remove_repository(self, repo_path: Path) -> bool:
        """
        Remove a repository directory.

        Args:
            repo_path: Path to the repository to remove

        Returns:
            True if successful, False otherwise
        """
        if self.dry_run:
            self.logger.info(f"[DRY RUN] Would remove: {repo_path}")
            return True

        try:
            shutil.rmtree(repo_path)
            self.logger.info(f"Removed repository: {repo_path}")
            return True
        except Exception as e:
            self.logger.error(f"Failed to remove {repo_path}: {e}")
            return False

    def _confirm_action(self, message: str) -> bool:
        """
        Ask for user confirmation in interactive mode.

        Args:
            message: Confirmation message to display

        Returns:
            True if user confirms, False otherwise
        """
        if not self.interactive:
            return True

        while True:
            response = input(f"{message} (y/n/q): ").lower().strip()
            if response == "y":
                return True
            elif response == "n":
                return False
            elif response == "q":
                self.logger.info("User requested to quit")
                sys.exit(0)
            else:
                print("Please enter 'y' for yes, 'n' for no, or 'q' to quit")

    def scan_repositories(self) -> List[Path]:
        """
        Scan the base directory for Git repositories.

        Returns:
            List of paths to Git repositories
        """
        repositories = []

        if not self.base_path.exists():
            self.logger.error(f"Base path does not exist: {self.base_path}")
            return repositories

        self.logger.info(f"Scanning for repositories in: {self.base_path}")

        for item in self.base_path.iterdir():
            if item.is_dir() and self._is_git_repository(item):
                repositories.append(item)
                self.logger.debug(f"Found repository: {item.name}")

        self.logger.info(f"Found {len(repositories)} Git repositories")
        return repositories

    def process_repositories(self, repositories: List[Path]) -> None:
        """
        Process each repository and determine cleanup actions.

        Args:
            repositories: List of repository paths to process
        """
        self.stats["total_repos"] = len(repositories)

        for repo_path in repositories:
            self.logger.info(f"\n{'=' * 60}")
            self.logger.info(f"Processing: {repo_path.name}")
            self.logger.info(f"{'=' * 60}")

            try:
                # Get remote URL
                remote_url = self._get_remote_url(repo_path)
                if not remote_url:
                    self.logger.warning(f"No remote URL found for {repo_path.name}")
                    self.stats["skipped"] += 1
                    continue

                # Parse GitHub URL
                github_info = self._parse_github_url(remote_url)
                if not github_info:
                    self.logger.info(f"Not a GitHub repository: {remote_url}")
                    self.stats["skipped"] += 1
                    continue

                owner, repo_name = github_info
                self.logger.info(f"GitHub repository: {owner}/{repo_name}")

                # Get repository information
                repo_info = self._get_repository_info(owner, repo_name)
                if not repo_info:
                    self.logger.error(
                        f"Failed to get repository information for {owner}/{repo_name}"
                    )
                    self.stats["errors"] += 1
                    continue

                # Display repository status
                self.logger.info(f"Archived: {repo_info.get('archived', 'Unknown')}")
                self.logger.info(f"Disabled: {repo_info.get('disabled', 'Unknown')}")
                self.logger.info(
                    f"Visibility: {repo_info.get('visibility', 'Unknown')}"
                )
                self.logger.info(
                    f"Last updated: {repo_info.get('updatedAt', 'Unknown')}"
                )

                # Determine if repository should be removed
                should_remove, reason = self._should_remove_repository(
                    repo_info, repo_path
                )

                if should_remove:
                    self.stats["archived_repos"] += 1
                    self.logger.warning(f"CLEANUP CANDIDATE: {reason}")

                    if self._confirm_action(
                        f"Remove repository '{repo_path.name}'? Reason: {reason}"
                    ):
                        if self._remove_repository(repo_path):
                            self.stats["removed_repos"] += 1
                        else:
                            self.stats["errors"] += 1
                    else:
                        self.logger.info(f"Skipped removal of {repo_path.name}")
                        self.stats["skipped"] += 1
                else:
                    self.logger.info(f"KEEP: {reason}")

            except Exception as e:
                self.logger.error(f"Error processing {repo_path.name}: {e}")
                self.stats["errors"] += 1

    def generate_report(self) -> None:
        """Generate a summary report of the cleanup operation."""
        self.logger.info(f"\n{'=' * 60}")
        self.logger.info("CLEANUP SUMMARY")
        self.logger.info(f"{'=' * 60}")
        self.logger.info(f"Total repositories scanned: {self.stats['total_repos']}")
        self.logger.info(
            f"Archived/outdated repositories found: {self.stats['archived_repos']}"
        )
        self.logger.info(f"Repositories removed: {self.stats['removed_repos']}")
        self.logger.info(f"Repositories skipped: {self.stats['skipped']}")
        self.logger.info(f"Errors encountered: {self.stats['errors']}")
        self.logger.info(f"Log file: {self.log_file}")

        if self.dry_run:
            self.logger.info("\n⚠️  This was a DRY RUN - no changes were made")
        elif self.stats["removed_repos"] > 0:
            self.logger.info(
                f"\n✅ Successfully cleaned up {self.stats['removed_repos']} repositories"
            )

    def run(self) -> None:
        """Main entry point for the cleanup operation."""
        self.logger.info("Starting repository cleanup")
        self.logger.info(f"Base path: {self.base_path}")
        self.logger.info(f"Dry run: {self.dry_run}")
        self.logger.info(f"Interactive: {self.interactive}")

        # Check if gh CLI is available
        success, stdout, stderr = self._run_command(["gh", "--version"])
        if not success:
            self.logger.error(
                "GitHub CLI (gh) is not available. Please install it first."
            )
            sys.exit(1)

        self.logger.info(
            f"GitHub CLI version: {stdout.split()[0] if stdout else 'Unknown'}"
        )

        # Scan for repositories
        repositories = self.scan_repositories()
        if not repositories:
            self.logger.info("No repositories found to process")
            return

        # Process repositories
        self.process_repositories(repositories)

        # Generate report
        self.generate_report()


def main():
    """Main function with command-line argument parsing."""
    parser = argparse.ArgumentParser(
        description="Cleanup archived GitHub repositories from local filesystem",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Dry run (safe, shows what would be done)
  python cleanup-archived-repos.py

  # Interactive cleanup with confirmation prompts
  python cleanup-archived-repos.py --no-dry-run

  # Non-interactive cleanup (removes all archived repos)
  python cleanup-archived-repos.py --no-dry-run --no-interactive

  # Custom path
  python cleanup-archived-repos.py --path ~/my-repos
        """,
    )

    parser.add_argument(
        "--path",
        default="~/repos/github.com/jdfalk",
        help="Base path to scan for repositories (default: ~/repos/github.com/jdfalk)",
    )

    parser.add_argument(
        "--dry-run",
        action="store_true",
        default=True,
        help="Show what would be done without making changes (default: True)",
    )

    parser.add_argument(
        "--no-dry-run",
        action="store_false",
        dest="dry_run",
        help="Actually perform the cleanup operations",
    )

    parser.add_argument(
        "--interactive",
        action="store_true",
        default=True,
        help="Prompt for confirmation before each action (default: True)",
    )

    parser.add_argument(
        "--no-interactive",
        action="store_false",
        dest="interactive",
        help="Run without confirmation prompts",
    )

    parser.add_argument("--verbose", action="store_true", help="Enable verbose logging")

    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    # Create and run cleanup tool
    cleanup = RepositoryCleanup(
        base_path=args.path, dry_run=args.dry_run, interactive=args.interactive
    )

    try:
        cleanup.run()
    except KeyboardInterrupt:
        print("\n\nOperation cancelled by user")
        sys.exit(1)
    except Exception as e:
        print(f"\nUnexpected error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
