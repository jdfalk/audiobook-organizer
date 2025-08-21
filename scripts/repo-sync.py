#!/usr/bin/env python3
# file: scripts/repo-sync.py
# version: 1.1.0
# guid: 9a8b7c6d-5e4f-3d2c-1b0a-9c8b7a6d5e4f

"""
Repository Synchronization Tool

This script synchronizes files across repositories to ensure consistency.
It can copy missing files, update outdated files, and maintain version control.

Usage:
    python3 repo-sync.py --base-path /path/to/repos --source-repo ghcommon [options]

The script uses a source repository (default: ghcommon) as the canonical source
for all shared files and propagates them to target repositories.
"""

import argparse
import hashlib
import json
import re
import shutil
import sys
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple


@dataclass
class SyncOperation:
    """Represents a single file synchronization operation"""

    repo_name: str
    file_path: str
    operation_type: str  # 'copy', 'update', 'skip'
    source_version: str
    target_version: Optional[str]
    reason: str


@dataclass
class SyncReport:
    """Summary of synchronization operations"""

    timestamp: str
    source_repo: str
    total_repos: int
    operations: List[SyncOperation]
    summary: Dict[str, int]


class RepoSynchronizer:
    """Repository synchronization manager"""

    # Files to track and synchronize
    TRACKED_FILES = [
        ".github/AGENTS.md",
        ".github/commit-messages.md",
        ".github/copilot-instructions.md",
        ".github/instructions/general-coding.instructions.md",
        ".github/instructions/github-actions.instructions.md",
        ".github/instructions/go.instructions.md",
        ".github/instructions/html-css.instructions.md",
        ".github/instructions/javascript.instructions.md",
        ".github/instructions/json.instructions.md",
        ".github/instructions/markdown.instructions.md",
        ".github/instructions/protobuf.instructions.md",
        ".github/instructions/python.instructions.md",
        ".github/instructions/r.instructions.md",
        ".github/instructions/shell.instructions.md",
        ".github/instructions/typescript.instructions.md",
        ".github/pull-request-descriptions.md",
        ".github/test-generation.md",
    ]

    def __init__(
        self, base_path: Path, source_repo: str = "ghcommon", dry_run: bool = False
    ):
        self.base_path = Path(base_path)
        self.source_repo = source_repo
        self.dry_run = dry_run
        self.operations: List[SyncOperation] = []

        # Validate source repository
        self.source_path = self.base_path / source_repo
        if not self.source_path.exists():
            raise ValueError(f"Source repository not found: {self.source_path}")

        # Load source files
        self.source_files = self._load_source_files()

    def _load_source_files(self) -> Dict[str, Tuple[str, str, str]]:
        """Load source files with their versions and content"""
        source_files = {}

        for file_path in self.TRACKED_FILES:
            full_path = self.source_path / file_path
            if full_path.exists():
                try:
                    content = full_path.read_text(encoding="utf-8")
                    version, guid = self._extract_version_and_guid(content)
                    content_hash = hashlib.md5(content.encode()).hexdigest()
                    source_files[file_path] = (version, guid, content_hash)
                except Exception as e:
                    print(f"Warning: Could not load {file_path} from source: {e}")

        return source_files

    def _extract_version_and_guid(self, content: str) -> Tuple[str, str]:
        """Extract version and GUID from file content"""
        version_match = re.search(r"version:\s*([^\s]+)", content)
        guid_match = re.search(r"guid:\s*([^\s]+)", content)

        version = version_match.group(1) if version_match else "no-version"
        guid = guid_match.group(1) if guid_match else "no-guid"

        return version, guid

    def _get_repositories(self) -> List[Path]:
        """Get list of Git repositories to synchronize"""
        repositories = []

        for item in self.base_path.iterdir():
            if item.is_dir() and (item / ".git").exists():
                # Skip the source repository
                if item.name != self.source_repo:
                    repositories.append(item)

        return sorted(repositories)

    def _analyze_target_file(
        self, repo_path: Path, file_path: str
    ) -> Tuple[str, str, str, bool]:
        """Analyze target file status"""
        full_path = repo_path / file_path

        if not full_path.exists():
            return "missing", "no-version", "no-guid", False

        try:
            content = full_path.read_text(encoding="utf-8")
            version, guid = self._extract_version_and_guid(content)
            content_hash = hashlib.md5(content.encode()).hexdigest()

            # Check if content matches source
            source_info = self.source_files.get(file_path)
            if source_info and source_info[2] == content_hash:
                return "current", version, guid, True

            return "outdated", version, guid, False

        except Exception:
            return "error", "no-version", "no-guid", False

    def _should_sync_file(
        self, file_path: str, target_status: str, target_version: str
    ) -> Tuple[bool, str]:
        """Determine if file should be synchronized"""
        if file_path not in self.source_files:
            return False, "File not available in source repository"

        if target_status == "missing":
            return True, "File is missing in target repository"

        if target_status == "error":
            return True, "File has read errors in target repository"

        if target_status == "current":
            return False, "File is already current"

        # For outdated files, check version
        source_version = self.source_files[file_path][0]
        if target_version == "no-version" and source_version != "no-version":
            return True, "Target file lacks version, source has version"

        if source_version != "no-version" and target_version != "no-version":
            try:
                # Simple version comparison (assumes semantic versioning)
                source_parts = [
                    int(x) for x in source_version.replace("v", "").split(".")
                ]
                target_parts = [
                    int(x) for x in target_version.replace("v", "").split(".")
                ]

                # Pad to same length
                max_len = max(len(source_parts), len(target_parts))
                source_parts.extend([0] * (max_len - len(source_parts)))
                target_parts.extend([0] * (max_len - len(target_parts)))

                if source_parts > target_parts:
                    return (
                        True,
                        f"Source version {source_version} is newer than target {target_version}",
                    )

            except ValueError:
                # If version parsing fails, sync anyway to be safe
                return (
                    True,
                    f"Cannot compare versions: source={source_version}, target={target_version}",
                )

        return True, "File content differs from source"

    def _copy_file(self, source_file: str, target_repo: Path) -> bool:
        """Copy file from source to target repository"""
        source_path = self.source_path / source_file
        target_path = target_repo / source_file

        if self.dry_run:
            print(f"    [DRY RUN] Would copy {source_file} to {target_repo.name}")
            return True

        try:
            # Create target directory if it doesn't exist
            target_path.parent.mkdir(parents=True, exist_ok=True)

            # Copy file
            shutil.copy2(source_path, target_path)
            print(f"    ✓ Copied {source_file} to {target_repo.name}")
            return True

        except Exception as e:
            print(f"    ✗ Failed to copy {source_file} to {target_repo.name}: {e}")
            return False

    def sync_repository(self, repo_path: Path) -> List[SyncOperation]:
        """Synchronize a single repository"""
        print(f"  Synchronizing {repo_path.name}...")
        repo_operations = []

        for file_path in self.TRACKED_FILES:
            target_status, target_version, target_guid, is_current = (
                self._analyze_target_file(repo_path, file_path)
            )

            should_sync, reason = self._should_sync_file(
                file_path, target_status, target_version
            )

            if should_sync:
                operation_type = "copy" if target_status == "missing" else "update"
                success = self._copy_file(file_path, repo_path)

                if not success and not self.dry_run:
                    operation_type = "failed"

                source_version = self.source_files.get(
                    file_path, ("unknown", "unknown", "unknown")
                )[0]

                operation = SyncOperation(
                    repo_name=repo_path.name,
                    file_path=file_path,
                    operation_type=operation_type,
                    source_version=source_version,
                    target_version=target_version
                    if target_version != "no-version"
                    else None,
                    reason=reason,
                )

                repo_operations.append(operation)

            else:
                # File doesn't need sync
                source_version = self.source_files.get(
                    file_path, ("unknown", "unknown", "unknown")
                )[0]

                operation = SyncOperation(
                    repo_name=repo_path.name,
                    file_path=file_path,
                    operation_type="skip",
                    source_version=source_version,
                    target_version=target_version
                    if target_version != "no-version"
                    else None,
                    reason=reason,
                )

                repo_operations.append(operation)

        return repo_operations

    def sync_all_repositories(self) -> SyncReport:
        """Synchronize all repositories"""
        print(f"Synchronizing repositories from source: {self.source_repo}")
        print(f"Base path: {self.base_path}")

        if self.dry_run:
            print("🔍 DRY RUN MODE - No files will be modified")

        print(f"Source repository has {len(self.source_files)} tracked files")
        print()

        repositories = self._get_repositories()
        all_operations = []

        for repo_path in repositories:
            repo_operations = self.sync_repository(repo_path)
            all_operations.extend(repo_operations)

        # Generate summary
        summary = {
            "copy": sum(1 for op in all_operations if op.operation_type == "copy"),
            "update": sum(1 for op in all_operations if op.operation_type == "update"),
            "skip": sum(1 for op in all_operations if op.operation_type == "skip"),
            "failed": sum(1 for op in all_operations if op.operation_type == "failed"),
        }

        report = SyncReport(
            timestamp=datetime.now().isoformat(),
            source_repo=self.source_repo,
            total_repos=len(repositories),
            operations=all_operations,
            summary=summary,
        )

        return report

    def save_report(self, report: SyncReport, output_path: Path):
        """Save synchronization report to file"""
        output_path.parent.mkdir(parents=True, exist_ok=True)

        with open(output_path, "w", encoding="utf-8") as f:
            json.dump(asdict(report), f, indent=2, ensure_ascii=False)


def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(
        description="Synchronize repository files across multiple repositories",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Dry run to see what would be synchronized
  python3 repo-sync.py --base-path ~/repos/github.com/jdfalk/ --dry-run

  # Synchronize all repositories from ghcommon
  python3 repo-sync.py --base-path ~/repos/github.com/jdfalk/ --source-repo ghcommon

  # Use different source repository
  python3 repo-sync.py --base-path ~/repos/github.com/jdfalk/ --source-repo gcommon

  # Save detailed report
  python3 repo-sync.py --base-path ~/repos/github.com/jdfalk/ --output-dir ./sync-reports
        """,
    )

    parser.add_argument(
        "--base-path",
        type=Path,
        required=True,
        help="Base path containing all repositories",
    )

    parser.add_argument(
        "--source-repo",
        type=str,
        default="ghcommon",
        help="Source repository name (default: ghcommon)",
    )

    parser.add_argument(
        "--output-dir", type=Path, help="Directory to save synchronization reports"
    )

    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be done without making changes",
    )

    parser.add_argument(
        "--force",
        action="store_true",
        help="Force synchronization even for files that appear current",
    )

    parser.add_argument(
        "--include-repos", nargs="+", help="Only synchronize specified repositories"
    )

    parser.add_argument(
        "--exclude-repos",
        nargs="+",
        help="Exclude specified repositories from synchronization",
    )

    args = parser.parse_args()

    try:
        # Initialize synchronizer
        synchronizer = RepoSynchronizer(
            base_path=args.base_path, source_repo=args.source_repo, dry_run=args.dry_run
        )

        # Run synchronization
        report = synchronizer.sync_all_repositories()

        # Print summary
        print("\n" + "=" * 80)
        print("SYNCHRONIZATION SUMMARY")
        print("=" * 80)
        print(f"Source repository: {report.source_repo}")
        print(f"Total repositories processed: {report.total_repos}")
        print(f"Files copied: {report.summary['copy']}")
        print(f"Files updated: {report.summary['update']}")
        print(f"Files skipped: {report.summary['skip']}")
        print(f"Failed operations: {report.summary['failed']}")

        if args.dry_run:
            print("\n🔍 This was a dry run - no files were actually modified")

        # Save detailed report if requested
        if args.output_dir:
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            report_file = args.output_dir / f"sync-report-{timestamp}.json"
            synchronizer.save_report(report, report_file)
            print(f"\nDetailed report saved to: {report_file}")

        # Show any failed operations
        failed_ops = [op for op in report.operations if op.operation_type == "failed"]
        if failed_ops:
            print(f"\n⚠️  {len(failed_ops)} operations failed:")
            for op in failed_ops:
                print(f"  - {op.repo_name}: {op.file_path} ({op.reason})")

        return 0 if report.summary["failed"] == 0 else 1

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
