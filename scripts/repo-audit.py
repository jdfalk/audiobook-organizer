#!/usr/bin/env python3
# file: scripts/repo-audit.py
# version: 1.1.0
# guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

"""
Repository Audit Script
=======================

This script audits all repositories in the workspace to compare:
1. .github/instructions/ files and their versions
2. .github/ metadata files (copilot-instructions.md, commit-messages.md, etc.)
3. Missing files that should be present
4. Version mismatches

Generates a comprehensive comparison chart and identifies repositories that need updates.
"""

import argparse
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Optional, Tuple


@dataclass
class FileInfo:
    """Information about a file in a repository."""

    path: str
    version: Optional[str] = None
    guid: Optional[str] = None
    exists: bool = False
    size: int = 0


@dataclass
class RepoInfo:
    """Information about a repository."""

    name: str
    path: str
    files: Dict[str, FileInfo]
    is_git_repo: bool = False


class RepoAuditor:
    """Audits repositories for file consistency and versions."""

    def __init__(self, base_path: str):
        self.base_path = Path(base_path)
        self.repos: Dict[str, RepoInfo] = {}
        self.reference_repo = "ghcommon"  # Use ghcommon as the reference

        # Files to audit (relative to repo root)
        self.audit_files = [
            ".github/copilot-instructions.md",
            ".github/commit-messages.md",
            ".github/pull-request-descriptions.md",
            ".github/test-generation.md",
            ".github/AGENTS.md",
            ".github/instructions/general-coding.instructions.md",
            ".github/instructions/go.instructions.md",
            ".github/instructions/python.instructions.md",
            ".github/instructions/javascript.instructions.md",
            ".github/instructions/typescript.instructions.md",
            ".github/instructions/shell.instructions.md",
            ".github/instructions/markdown.instructions.md",
            ".github/instructions/json.instructions.md",
            ".github/instructions/html-css.instructions.md",
            ".github/instructions/protobuf.instructions.md",
            ".github/instructions/github-actions.instructions.md",
            ".github/instructions/r.instructions.md",
        ]

    def extract_version_and_guid(
        self, file_path: Path
    ) -> Tuple[Optional[str], Optional[str]]:
        """Extract version and GUID from file headers."""
        if not file_path.exists():
            return None, None

        try:
            with open(file_path, "r", encoding="utf-8") as f:
                content = f.read(1000)  # Read first 1KB to find headers

            # Look for version patterns
            version_patterns = [
                r"version:\s*([0-9]+\.[0-9]+\.[0-9]+)",
                r"<!-- version:\s*([0-9]+\.[0-9]+\.[0-9]+)\s*-->",
                r"# version:\s*([0-9]+\.[0-9]+\.[0-9]+)",
                r"// version:\s*([0-9]+\.[0-9]+\.[0-9]+)",
            ]

            # Look for GUID patterns
            guid_patterns = [
                r"guid:\s*([a-f0-9-]{36})",
                r"<!-- guid:\s*([a-f0-9-]{36})\s*-->",
                r"# guid:\s*([a-f0-9-]{36})",
                r"// guid:\s*([a-f0-9-]{36})",
            ]

            version = None
            guid = None

            for pattern in version_patterns:
                match = re.search(pattern, content, re.IGNORECASE)
                if match:
                    version = match.group(1)
                    break

            for pattern in guid_patterns:
                match = re.search(pattern, content, re.IGNORECASE)
                if match:
                    guid = match.group(1)
                    break

            return version, guid

        except Exception as e:
            print(f"Error reading {file_path}: {e}")
            return None, None

    def scan_repository(self, repo_path: Path) -> RepoInfo:
        """Scan a single repository for audit files."""
        repo_name = repo_path.name
        repo_info = RepoInfo(
            name=repo_name,
            path=str(repo_path),
            files={},
            is_git_repo=(repo_path / ".git").exists(),
        )

        for audit_file in self.audit_files:
            file_path = repo_path / audit_file
            version, guid = self.extract_version_and_guid(file_path)

            repo_info.files[audit_file] = FileInfo(
                path=str(file_path),
                version=version,
                guid=guid,
                exists=file_path.exists(),
                size=file_path.stat().st_size if file_path.exists() else 0,
            )

        return repo_info

    def scan_all_repositories(self) -> None:
        """Scan all repositories in the base path."""
        print(f"Scanning repositories in {self.base_path}...")

        for item in self.base_path.iterdir():
            if item.is_dir() and not item.name.startswith("."):
                # Skip certain directories
                skip_dirs = {
                    "ubuntu-autoinstall-webhook.bfg-report",
                    "subtitle-manager.old",
                }
                if item.name in skip_dirs:
                    continue

                print(f"  Scanning {item.name}...")
                repo_info = self.scan_repository(item)
                self.repos[item.name] = repo_info

    def generate_comparison_chart(self) -> str:
        """Generate a comparison chart showing file versions across repositories."""
        if not self.repos:
            return "No repositories found to compare."

        # Get all unique files across all repos
        all_files = set()
        for repo in self.repos.values():
            all_files.update(repo.files.keys())

        all_files = sorted(all_files)
        repo_names = sorted(self.repos.keys())

        # Create header
        chart = []
        header = ["File"] + repo_names
        chart.append(header)

        # Add separator
        chart.append(["-" * 50] + ["-" * 20] * len(repo_names))

        # Add file rows
        for file_name in all_files:
            row = [file_name.split("/")[-1]]  # Just filename for readability

            for repo_name in repo_names:
                repo = self.repos[repo_name]
                if file_name in repo.files:
                    file_info = repo.files[file_name]
                    if file_info.exists:
                        if file_info.version:
                            row.append(f"v{file_info.version}")
                        else:
                            row.append("no-version")
                    else:
                        row.append("missing")
                else:
                    row.append("missing")

            chart.append(row)

        # Format as table
        # Calculate column widths
        col_widths = []
        for i in range(len(chart[0])):
            max_width = max(len(str(row[i])) for row in chart)
            col_widths.append(max_width + 2)

        # Generate formatted output
        output = []
        for row in chart:
            formatted_row = ""
            for i, cell in enumerate(row):
                formatted_row += str(cell).ljust(col_widths[i])
                if i < len(row) - 1:
                    formatted_row += " | "
            output.append(formatted_row)

        return "\n".join(output)

    def generate_detailed_report(self) -> Dict:
        """Generate a detailed JSON report of all repositories."""
        report = {
            "scan_time": str(Path().cwd()),
            "reference_repo": self.reference_repo,
            "repositories": {},
            "summary": {
                "total_repos": len(self.repos),
                "total_files_tracked": len(self.audit_files),
                "repos_needing_updates": 0,
            },
        }

        reference_repo = self.repos.get(self.reference_repo)
        if not reference_repo:
            print(f"Warning: Reference repository '{self.reference_repo}' not found")

        repos_needing_updates = 0

        for repo_name, repo_info in self.repos.items():
            repo_data = {
                "path": repo_info.path,
                "is_git_repo": repo_info.is_git_repo,
                "files": {},
                "needs_update": False,
                "missing_files": [],
                "outdated_files": [],
            }

            for file_name, file_info in repo_info.files.items():
                repo_data["files"][file_name] = {
                    "exists": file_info.exists,
                    "version": file_info.version,
                    "guid": file_info.guid,
                    "size": file_info.size,
                }

                # Check if file needs update
                if not file_info.exists:
                    repo_data["missing_files"].append(file_name)
                    repo_data["needs_update"] = True
                elif reference_repo and file_name in reference_repo.files:
                    ref_file = reference_repo.files[file_name]
                    if (
                        ref_file.exists
                        and ref_file.version
                        and file_info.version != ref_file.version
                    ):
                        repo_data["outdated_files"].append(
                            {
                                "file": file_name,
                                "current": file_info.version,
                                "expected": ref_file.version,
                            }
                        )
                        repo_data["needs_update"] = True

            if repo_data["needs_update"]:
                repos_needing_updates += 1

            report["repositories"][repo_name] = repo_data

        report["summary"]["repos_needing_updates"] = repos_needing_updates
        return report

    def save_report(self, output_path: Path) -> None:
        """Save detailed report to JSON file."""
        report = self.generate_detailed_report()

        with open(output_path, "w", encoding="utf-8") as f:
            json.dump(report, f, indent=2, sort_keys=True)

        print(f"Detailed report saved to: {output_path}")


def main():
    parser = argparse.ArgumentParser(
        description="Audit repositories for file consistency"
    )
    parser.add_argument(
        "--base-path",
        default="/Users/jdfalk/repos/github.com/jdfalk",
        help="Base path containing repositories",
    )
    parser.add_argument("--output-dir", default=".", help="Directory to save reports")
    parser.add_argument(
        "--format",
        choices=["table", "json", "both"],
        default="both",
        help="Output format",
    )

    args = parser.parse_args()

    auditor = RepoAuditor(args.base_path)
    auditor.scan_all_repositories()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(exist_ok=True)

    if args.format in ["table", "both"]:
        chart = auditor.generate_comparison_chart()
        print("\n" + "=" * 80)
        print("REPOSITORY FILE COMPARISON CHART")
        print("=" * 80)
        print(chart)

        # Save table to file
        table_file = output_dir / "repo-comparison-chart.txt"
        with open(table_file, "w", encoding="utf-8") as f:
            f.write(chart)
        print(f"\nComparison chart saved to: {table_file}")

    if args.format in ["json", "both"]:
        json_file = output_dir / "repo-audit-report.json"
        auditor.save_report(json_file)

        # Print summary
        report = auditor.generate_detailed_report()
        print("\n" + "=" * 80)
        print("AUDIT SUMMARY")
        print("=" * 80)
        print(f"Total repositories scanned: {report['summary']['total_repos']}")
        print(f"Total files tracked: {report['summary']['total_files_tracked']}")
        print(
            f"Repositories needing updates: {report['summary']['repos_needing_updates']}"
        )

        print("\nRepositories needing updates:")
        for repo_name, repo_data in report["repositories"].items():
            if repo_data["needs_update"]:
                print(f"  - {repo_name}:")
                if repo_data["missing_files"]:
                    print(f"    Missing files: {len(repo_data['missing_files'])}")
                if repo_data["outdated_files"]:
                    print(f"    Outdated files: {len(repo_data['outdated_files'])}")


if __name__ == "__main__":
    main()
