#!/usr/bin/env python3
# file: scripts/rebase.py

"""
Smart Git Rebase Tool - Python Implementation

A comprehensive Git rebase automation tool with intelligent conflict resolution,
backup management, and detailed logging. This is the primary implementation
that provides full-featured rebase capabilities for AI agents and developers.

Features:
- Intelligent conflict resolution based on file types
- Automatic backup branch creation
- Comprehensive logging and summary generation
- Multiple operation modes (interactive, automated, smart)
- Recovery instructions and rollback capabilities
- Support for dry-run operations
"""

import argparse
import json
import subprocess
import sys
from datetime import datetime
from enum import Enum
from typing import List


class RebaseMode(Enum):
    """Rebase operation modes"""

    INTERACTIVE = "interactive"
    AUTOMATED = "automated"
    SMART = "smart"


class ConflictStrategy(Enum):
    """Conflict resolution strategies"""

    PREFER_INCOMING = "prefer_incoming"
    PREFER_CURRENT = "prefer_current"
    SMART_MERGE = "smart_merge"
    MANUAL_REVIEW = "manual_review"
    SAVE_BOTH = "save_both"


class RebaseResult(Enum):
    """Rebase operation results"""

    SUCCESS = "success"
    CONFLICTS = "conflicts"
    FAILED = "failed"
    ABORTED = "aborted"


class GitRebaseError(Exception):
    """Custom exception for rebase operations"""

    pass


class SmartRebase:
    """
    Smart Git Rebase implementation with intelligent conflict resolution.

    This class provides comprehensive rebase functionality including:
    - Automatic conflict resolution based on file types
    - Backup management and recovery
    - Detailed logging and summary generation
    - Multiple operation modes for different use cases
    """

    def __init__(self, verbose: bool = False, dry_run: bool = False):
        """
        Initialize the SmartRebase instance.

        Args:
            verbose: Enable verbose logging output
            dry_run: Show what would be done without executing
        """
        self.verbose = verbose
        self.dry_run = dry_run
        self.start_time = datetime.now()
        self.summary = {
            "timestamp": self.start_time.isoformat(),
            "mode": None,
            "target_branch": None,
            "source_branch": None,
            "result": None,
            "conflicts_resolved": [],
            "files_modified": [],
            "backup_branch": None,
            "recovery_instructions": [],
            "execution_time": None,
            "errors": [],
        }

        # File type conflict resolution strategies
        self.file_strategies = {
            # Documentation files - prefer incoming changes
            r".*\.md$": ConflictStrategy.PREFER_INCOMING,
            r"^docs/.*": ConflictStrategy.PREFER_INCOMING,
            r"^README.*": ConflictStrategy.PREFER_INCOMING,
            r"^CHANGELOG.*": ConflictStrategy.PREFER_INCOMING,
            r"^TODO.*": ConflictStrategy.PREFER_INCOMING,
            # Build and CI files - prefer incoming changes
            r"^\.github/.*": ConflictStrategy.PREFER_INCOMING,
            r"^Dockerfile.*": ConflictStrategy.PREFER_INCOMING,
            r"^docker-.*": ConflictStrategy.PREFER_INCOMING,
            r"^Makefile$": ConflictStrategy.PREFER_INCOMING,
            r".*\.yml$": ConflictStrategy.PREFER_INCOMING,
            r".*\.yaml$": ConflictStrategy.PREFER_INCOMING,
            # Package management files - prefer incoming changes
            r"^go\.mod$": ConflictStrategy.PREFER_INCOMING,
            r"^go\.sum$": ConflictStrategy.PREFER_INCOMING,
            r"^package\.json$": ConflictStrategy.PREFER_INCOMING,
            r"^package-lock\.json$": ConflictStrategy.PREFER_INCOMING,
            r"^requirements\.txt$": ConflictStrategy.PREFER_INCOMING,
            r"^Pipfile.*": ConflictStrategy.PREFER_INCOMING,
            # Configuration files - smart merge or prefer incoming
            r".*\.json$": ConflictStrategy.SMART_MERGE,
            r".*\.toml$": ConflictStrategy.SMART_MERGE,
            r".*\.ini$": ConflictStrategy.SMART_MERGE,
            r".*\.conf$": ConflictStrategy.SMART_MERGE,
            # Code files - save both versions for manual review
            r".*\.go$": ConflictStrategy.SAVE_BOTH,
            r".*\.py$": ConflictStrategy.SAVE_BOTH,
            r".*\.js$": ConflictStrategy.SAVE_BOTH,
            r".*\.ts$": ConflictStrategy.SAVE_BOTH,
            r".*\.java$": ConflictStrategy.SAVE_BOTH,
            r".*\.cpp$": ConflictStrategy.SAVE_BOTH,
            r".*\.c$": ConflictStrategy.SAVE_BOTH,
            r".*\.h$": ConflictStrategy.SAVE_BOTH,
        }

    def log_info(self, message: str) -> None:
        """Log informational message"""
        print(f"\033[0;34m[REBASE]\033[0m {message}")

    def log_success(self, message: str) -> None:
        """Log success message"""
        print(f"\033[0;32m[REBASE]\033[0m {message}")

    def log_warning(self, message: str) -> None:
        """Log warning message"""
        print(f"\033[1;33m[REBASE]\033[0m {message}")

    def log_error(self, message: str) -> None:
        """Log error message"""
        print(f"\033[0;31m[REBASE]\033[0m {message}")

    def log_verbose(self, message: str) -> None:
        """Log verbose message (only if verbose mode enabled)"""
        if self.verbose:
            print(f"\033[0;36m[VERBOSE]\033[0m {message}")

    def run_command(
        self, cmd: List[str], capture_output: bool = True, check: bool = True
    ) -> subprocess.CompletedProcess:
        """
        Run a shell command with optional output capture.

        Args:
            cmd: Command and arguments to execute
            capture_output: Whether to capture stdout/stderr
            check: Whether to raise exception on non-zero exit

        Returns:
            CompletedProcess instance with command results

        Raises:
            GitRebaseError: If command fails and check=True
        """
        self.log_verbose(f"Running command: {' '.join(cmd)}")

        if self.dry_run:
            self.log_info(f"[DRY RUN] Would execute: {' '.join(cmd)}")
            return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")

        try:
            result = subprocess.run(
                cmd, capture_output=capture_output, text=True, check=check
            )

            if self.verbose and result.stdout:
                self.log_verbose(f"stdout: {result.stdout.strip()}")
            if self.verbose and result.stderr:
                self.log_verbose(f"stderr: {result.stderr.strip()}")

            return result
        except subprocess.CalledProcessError as e:
            error_msg = f"Command failed: {' '.join(cmd)}"
            if e.stderr:
                error_msg += f"\nError: {e.stderr.strip()}"
            self.summary["errors"].append(error_msg)
            raise GitRebaseError(error_msg) from e

    def get_current_branch(self) -> str:
        """Get the current Git branch name"""
        result = self.run_command(["git", "branch", "--show-current"])
        return result.stdout.strip()

    def branch_exists(self, branch_name: str) -> bool:
        """Check if a Git branch exists"""
        try:
            self.run_command(
                ["git", "show-ref", "--verify", f"refs/heads/{branch_name}"]
            )
            return True
        except GitRebaseError:
            return False

    def create_backup_branch(self, source_branch: str) -> str:
        """
        Create a backup branch before starting rebase.

        Args:
            source_branch: Source branch to backup

        Returns:
            Name of the created backup branch
        """
        timestamp = self.start_time.strftime("%Y%m%d_%H%M%S")
        backup_name = f"backup/{source_branch}/{timestamp}"

        if not self.dry_run:
            self.run_command(["git", "branch", backup_name, source_branch])

        self.log_success(f"Created backup branch: {backup_name}")
        self.summary["backup_branch"] = backup_name
        return backup_name

    def get_conflicted_files(self) -> List[str]:
        """Get list of files with merge conflicts"""
        try:
            result = self.run_command(["git", "diff", "--name-only", "--diff-filter=U"])
            return [f.strip() for f in result.stdout.split("\n") if f.strip()]
        except GitRebaseError:
            return []

    def determine_conflict_strategy(self, file_path: str) -> ConflictStrategy:
        """
        Determine the appropriate conflict resolution strategy for a file.

        Args:
            file_path: Path to the conflicted file

        Returns:
            ConflictStrategy to use for resolving conflicts
        """
        import re

        for pattern, strategy in self.file_strategies.items():
            if re.match(pattern, file_path):
                return strategy

        # Default strategy for unknown file types
        return ConflictStrategy.MANUAL_REVIEW

    def resolve_conflict_prefer_incoming(self, file_path: str) -> bool:
        """
        Resolve conflict by preferring incoming changes.

        Args:
            file_path: Path to the conflicted file

        Returns:
            True if conflict was resolved successfully
        """
        try:
            self.run_command(["git", "checkout", "--theirs", file_path])
            self.run_command(["git", "add", file_path])
            self.log_verbose(f"Resolved {file_path} using incoming changes")
            return True
        except GitRebaseError as e:
            self.log_error(f"Failed to resolve {file_path} with incoming: {e}")
            return False

    def resolve_conflict_prefer_current(self, file_path: str) -> bool:
        """
        Resolve conflict by preferring current changes.

        Args:
            file_path: Path to the conflicted file

        Returns:
            True if conflict was resolved successfully
        """
        try:
            self.run_command(["git", "checkout", "--ours", file_path])
            self.run_command(["git", "add", file_path])
            self.log_verbose(f"Resolved {file_path} using current changes")
            return True
        except GitRebaseError as e:
            self.log_error(f"Failed to resolve {file_path} with current: {e}")
            return False

    def resolve_conflict_save_both(self, file_path: str) -> bool:
        """
        Resolve conflict by saving both versions for manual review.

        Args:
            file_path: Path to the conflicted file

        Returns:
            True if both versions were saved successfully
        """
        try:
            # Save current version
            current_file = f"{file_path}.current"
            self.run_command(["git", "show", f"HEAD:{file_path}"], capture_output=True)
            result = self.run_command(["git", "show", f"HEAD:{file_path}"])

            if not self.dry_run:
                with open(current_file, "w") as f:
                    f.write(result.stdout)

            # Save incoming version
            incoming_file = f"{file_path}.incoming"
            result = self.run_command(["git", "show", f"MERGE_HEAD:{file_path}"])

            if not self.dry_run:
                with open(incoming_file, "w") as f:
                    f.write(result.stdout)

            # Use incoming version as default
            self.run_command(["git", "checkout", "--theirs", file_path])
            self.run_command(["git", "add", file_path])

            self.log_warning(f"Saved both versions: {current_file}, {incoming_file}")
            self.log_warning(f"Using incoming version for {file_path}")
            self.log_warning("Please review and merge manually if needed")

            return True
        except GitRebaseError as e:
            self.log_error(f"Failed to save both versions for {file_path}: {e}")
            return False

    def resolve_conflict_smart_merge(self, file_path: str) -> bool:
        """
        Resolve conflict using smart merge strategy.

        Args:
            file_path: Path to the conflicted file

        Returns:
            True if conflict was resolved successfully
        """
        # For now, prefer incoming changes for smart merge
        # This could be enhanced with more sophisticated merging logic
        return self.resolve_conflict_prefer_incoming(file_path)

    def resolve_conflicts(self, mode: RebaseMode) -> bool:
        """
        Resolve merge conflicts based on the rebase mode and file types.

        Args:
            mode: Rebase mode determining conflict resolution behavior

        Returns:
            True if all conflicts were resolved successfully
        """
        conflicted_files = self.get_conflicted_files()

        if not conflicted_files:
            self.log_info("No conflicts to resolve")
            return True

        self.log_info(f"Found {len(conflicted_files)} conflicted files")

        resolved_count = 0
        for file_path in conflicted_files:
            self.log_info(f"Resolving conflict in: {file_path}")

            if mode == RebaseMode.INTERACTIVE:
                # In interactive mode, prompt user for each conflict
                self.log_warning(f"Conflict in {file_path} - please resolve manually")
                response = input("Continue after resolving? (y/n): ")
                if response.lower() != "y":
                    return False
                resolved_count += 1
            else:
                # Automated resolution based on file type
                strategy = self.determine_conflict_strategy(file_path)
                self.log_verbose(f"Using strategy {strategy.value} for {file_path}")

                success = False
                if strategy == ConflictStrategy.PREFER_INCOMING:
                    success = self.resolve_conflict_prefer_incoming(file_path)
                elif strategy == ConflictStrategy.PREFER_CURRENT:
                    success = self.resolve_conflict_prefer_current(file_path)
                elif strategy == ConflictStrategy.SAVE_BOTH:
                    success = self.resolve_conflict_save_both(file_path)
                elif strategy == ConflictStrategy.SMART_MERGE:
                    success = self.resolve_conflict_smart_merge(file_path)
                else:
                    # Manual review - prefer incoming as safe default
                    self.log_warning(
                        f"Manual review needed for {file_path}, using incoming"
                    )
                    success = self.resolve_conflict_prefer_incoming(file_path)

                if success:
                    resolved_count += 1
                    self.summary["conflicts_resolved"].append(
                        {"file": file_path, "strategy": strategy.value}
                    )
                else:
                    self.log_error(f"Failed to resolve conflict in {file_path}")
                    return False

        self.log_success(f"Resolved {resolved_count}/{len(conflicted_files)} conflicts")
        return resolved_count == len(conflicted_files)

    def perform_rebase(self, target_branch: str, mode: RebaseMode) -> RebaseResult:
        """
        Perform the Git rebase operation.

        Args:
            target_branch: Target branch to rebase onto
            mode: Rebase mode determining behavior

        Returns:
            RebaseResult indicating the outcome
        """
        self.log_info(f"Starting rebase onto {target_branch} in {mode.value} mode")

        try:
            # Start the rebase
            if mode == RebaseMode.INTERACTIVE:
                cmd = ["git", "rebase", "-i", target_branch]
            else:
                cmd = ["git", "rebase", target_branch]

            self.run_command(cmd, check=False)

            # Check if rebase completed successfully
            try:
                self.run_command(["git", "status", "--porcelain"])
                # If we get here without conflicts, rebase succeeded
                return RebaseResult.SUCCESS
            except GitRebaseError:
                pass

            # Check for conflicts
            conflicted_files = self.get_conflicted_files()
            if conflicted_files:
                self.log_warning("Rebase has conflicts, attempting resolution")

                if self.resolve_conflicts(mode):
                    # Continue the rebase
                    self.run_command(["git", "rebase", "--continue"])
                    return RebaseResult.SUCCESS
                else:
                    self.log_error("Failed to resolve all conflicts")
                    return RebaseResult.CONFLICTS

            # Check rebase status
            result = self.run_command(["git", "status", "--porcelain"], check=False)
            if result.returncode == 0 and not result.stdout.strip():
                return RebaseResult.SUCCESS
            else:
                return RebaseResult.FAILED

        except GitRebaseError as e:
            self.log_error(f"Rebase failed: {e}")
            return RebaseResult.FAILED

    def force_push_changes(self, branch_name: str) -> bool:
        """
        Force push changes to remote repository.

        Args:
            branch_name: Branch name to push

        Returns:
            True if push was successful
        """
        try:
            self.log_info(f"Force pushing {branch_name} to remote")
            self.run_command(
                ["git", "push", "--force-with-lease", "origin", branch_name]
            )
            self.log_success("Force push completed successfully")
            return True
        except GitRebaseError as e:
            self.log_error(f"Force push failed: {e}")
            return False

    def generate_summary(self, result: RebaseResult) -> str:
        """
        Generate a comprehensive summary of the rebase operation.

        Args:
            result: Final result of the rebase operation

        Returns:
            Path to the generated summary file
        """
        self.summary["result"] = result.value
        self.summary["execution_time"] = (
            datetime.now() - self.start_time
        ).total_seconds()

        # Add recovery instructions based on result
        if result == RebaseResult.SUCCESS:
            self.summary["recovery_instructions"].append(
                "Rebase completed successfully. No recovery needed."
            )
        elif result == RebaseResult.CONFLICTS:
            self.summary["recovery_instructions"].extend(
                [
                    "Rebase stopped due to unresolved conflicts.",
                    "To abort: git rebase --abort",
                    f"To restore backup: git checkout {self.summary['backup_branch']}",
                    "Review conflicted files and resolve manually, then run: git rebase --continue",
                ]
            )
        elif result == RebaseResult.FAILED:
            self.summary["recovery_instructions"].extend(
                [
                    "Rebase failed to complete.",
                    "To abort: git rebase --abort",
                    f"To restore backup: git checkout {self.summary['backup_branch']}",
                    "Check git status for current state",
                ]
            )

        # Generate summary file
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        summary_file = f"rebase-summary-{timestamp}.md"

        if not self.dry_run:
            with open(summary_file, "w") as f:
                f.write(f"# Git Rebase Summary - {self.summary['timestamp']}\n\n")
                f.write("## Operation Details\n\n")
                f.write(f"- **Mode**: {self.summary['mode']}\n")
                f.write(f"- **Target Branch**: {self.summary['target_branch']}\n")
                f.write(f"- **Source Branch**: {self.summary['source_branch']}\n")
                f.write(f"- **Result**: {self.summary['result']}\n")
                f.write(
                    f"- **Execution Time**: {self.summary['execution_time']:.2f} seconds\n"
                )
                f.write(f"- **Backup Branch**: {self.summary['backup_branch']}\n\n")

                if self.summary["conflicts_resolved"]:
                    f.write(
                        f"## Conflicts Resolved ({len(self.summary['conflicts_resolved'])})\n\n"
                    )
                    for conflict in self.summary["conflicts_resolved"]:
                        f.write(f"- **{conflict['file']}**: {conflict['strategy']}\n")
                    f.write("\n")

                if self.summary["errors"]:
                    f.write(f"## Errors ({len(self.summary['errors'])})\n\n")
                    for error in self.summary["errors"]:
                        f.write(f"- {error}\n")
                    f.write("\n")

                f.write("## Recovery Instructions\n\n")
                for instruction in self.summary["recovery_instructions"]:
                    f.write(f"- {instruction}\n")
                f.write("\n")

                f.write("## Full Summary JSON\n\n")
                f.write(f"```json\n{json.dumps(self.summary, indent=2)}\n```\n")

        self.log_success(f"Summary generated: {summary_file}")
        return summary_file

    def run_rebase(
        self, target_branch: str, mode: RebaseMode, force_push: bool = False
    ) -> RebaseResult:
        """
        Main rebase execution method.

        Args:
            target_branch: Target branch to rebase onto
            mode: Rebase mode to use
            force_push: Whether to force push after successful rebase

        Returns:
            RebaseResult indicating the final outcome
        """
        try:
            # Get current branch
            source_branch = self.get_current_branch()
            if not source_branch:
                raise GitRebaseError("Could not determine current branch")

            self.summary["source_branch"] = source_branch
            self.summary["target_branch"] = target_branch
            self.summary["mode"] = mode.value

            self.log_info(f"Rebasing {source_branch} onto {target_branch}")

            # Validate target branch exists
            if not self.branch_exists(target_branch):
                raise GitRebaseError(f"Target branch '{target_branch}' does not exist")

            # Create backup branch
            self.create_backup_branch(source_branch)

            # Fetch latest changes
            self.log_info("Fetching latest changes from remote")
            self.run_command(["git", "fetch", "origin"])

            # Perform the rebase
            result = self.perform_rebase(target_branch, mode)

            # Force push if requested and rebase succeeded
            if result == RebaseResult.SUCCESS and force_push:
                if not self.force_push_changes(source_branch):
                    self.log_warning("Rebase succeeded but force push failed")

            return result

        except GitRebaseError as e:
            self.log_error(f"Rebase operation failed: {e}")
            return RebaseResult.FAILED
        except Exception as e:
            self.log_error(f"Unexpected error during rebase: {e}")
            self.summary["errors"].append(f"Unexpected error: {str(e)}")
            return RebaseResult.FAILED


def create_argument_parser() -> argparse.ArgumentParser:
    """
    Create and configure the argument parser.

    Returns:
        Configured ArgumentParser instance
    """
    parser = argparse.ArgumentParser(
        description="Smart Git Rebase Tool - Python Implementation",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s main                         # Smart rebase onto main
  %(prog)s --mode automated main        # Fully automated rebase
  %(prog)s --force-push main            # Rebase and force push
  %(prog)s --dry-run main               # Preview what would happen

Modes:
  interactive  - User-driven with prompts for conflicts
  automated    - Fully automated (AI/CI friendly)
  smart        - Intelligent automation with fallbacks (default)

This tool provides intelligent conflict resolution based on file types,
automatic backup creation, and comprehensive logging.
        """,
    )

    parser.add_argument("target_branch", help="Target branch to rebase onto")

    parser.add_argument(
        "--mode",
        choices=[mode.value for mode in RebaseMode],
        default=RebaseMode.SMART.value,
        help="Rebase mode (default: smart)",
    )

    parser.add_argument(
        "-f",
        "--force-push",
        action="store_true",
        help="Force push after successful rebase",
    )

    parser.add_argument(
        "-d",
        "--dry-run",
        action="store_true",
        help="Show what would be done without executing",
    )

    parser.add_argument(
        "-v", "--verbose", action="store_true", help="Enable verbose output"
    )

    return parser


def main() -> int:
    """
    Main entry point for the smart rebase tool.

    Returns:
        Exit code (0 for success, non-zero for failure)
    """
    parser = create_argument_parser()
    args = parser.parse_args()

    # Create rebase instance
    rebase = SmartRebase(verbose=args.verbose, dry_run=args.dry_run)

    try:
        # Convert mode string to enum
        mode = RebaseMode(args.mode)

        # Run the rebase
        result = rebase.run_rebase(
            target_branch=args.target_branch, mode=mode, force_push=args.force_push
        )

        # Generate summary
        summary_file = rebase.generate_summary(result)

        # Return appropriate exit code
        if result == RebaseResult.SUCCESS:
            rebase.log_success("Rebase completed successfully")
            return 0
        elif result == RebaseResult.CONFLICTS:
            rebase.log_error("Rebase stopped due to conflicts")
            rebase.log_info(f"See {summary_file} for recovery instructions")
            return 1
        else:
            rebase.log_error("Rebase failed")
            rebase.log_info(f"See {summary_file} for recovery instructions")
            return 2

    except KeyboardInterrupt:
        rebase.log_warning("Rebase interrupted by user")
        return 130
    except Exception as e:
        rebase.log_error(f"Unexpected error: {e}")
        return 3


if __name__ == "__main__":
    sys.exit(main())
