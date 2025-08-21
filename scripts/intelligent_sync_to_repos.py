#!/usr/bin/env python3
# file: scripts/intelligent_sync_to_repos.py
# version: 1.0.0
# guid: a1b2c3d4-e5f6-7890-1234-567890abcdef

"""
Intelligent sync script that understands the new modular .github structure.

This script:
1. Syncs the new modular structure (.github/instructions/, .github/prompts/)
2. Cleans up old files that have been moved/restructured
3. Preserves repo-specific files
4. Creates proper VS Code symlinks for Copilot integration
"""

import os
import sys
import subprocess
import shutil
import tempfile
import argparse
from typing import List
import logging

logging.basicConfig(level=logging.INFO, format="%(message)s")

# Files managed by ghcommon that should be synced
MANAGED_FILES = {
    # Core instruction system
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
    # Linter configurations (for GitHub Actions workflows)
    ".github/linters/.eslintrc.json",
    ".github/linters/.markdownlint.json",
    ".github/linters/.pylintrc",
    ".github/linters/.python-black",
    ".github/linters/.stylelintrc.json",
    ".github/linters/.yaml-lint.yml",
    ".github/linters/README.md",
    ".github/linters/ruff.toml",
    # Prompts
    ".github/prompts/ai-architecture.prompt.md",
    ".github/prompts/ai-changelog.prompt.md",
    ".github/prompts/ai-contribution.prompt.md",
    ".github/prompts/ai-issue-triage.prompt.md",
    ".github/prompts/ai-migration.prompt.md",
    ".github/prompts/ai-rebase-system.prompt.md",
    ".github/prompts/ai-refactor.prompt.md",
    ".github/prompts/ai-release-notes.prompt.md",
    ".github/prompts/ai-roadmap.prompt.md",
    ".github/prompts/bug-report.prompt.md",
    ".github/prompts/code-review.prompt.md",
    ".github/prompts/commit-message.prompt.md",
    ".github/prompts/documentation.prompt.md",
    ".github/prompts/feature-request.prompt.md",
    ".github/prompts/onboarding.prompt.md",
    ".github/prompts/pull-request.prompt.md",
    ".github/prompts/security-review.prompt.md",
    ".github/prompts/test-generation.prompt.md",
    # Core documentation (versioned)
    ".github/commit-messages.md",
    ".github/pull-request-descriptions.md",
    ".github/test-generation.md",
    ".github/AGENTS.md",
}

# Old files that should be cleaned up (moved to new structure)
OLD_FILES_TO_REMOVE = {
    # These have been moved to .github/instructions/
    ".github/code-style-general.md",
    ".github/code-style-go.md",
    ".github/code-style-python.md",
    ".github/code-style-javascript.md",
    ".github/code-style-typescript.md",
    ".github/code-style-markdown.md",
    ".github/code-style-json.md",
    ".github/code-style-shell.md",
    ".github/code-style-html-css.md",
    ".github/code-style-protobuf.md",
    ".github/code-style-r.md",
    ".github/code-style-github-actions.md",
}

# Repo-specific files that should NOT be overwritten
REPO_SPECIFIC_FILES = {
    ".github/dependabot.yml",
    ".github/pull_request_template.md",
    ".github/ISSUE_TEMPLATE/",
    ".github/labeler.yml",  # May have repo-specific rules
    ".github/workflows/",  # May have repo-specific workflows
}


def parse_args():
    parser = argparse.ArgumentParser(
        description="Intelligently sync .github structure to target repos."
    )
    parser.add_argument(
        "--repos", required=True, help="Comma-separated list of target repos"
    )
    parser.add_argument("--branch", required=True, help="Branch name to push to")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be done without executing",
    )
    return parser.parse_args()


def run(cmd: List[str], cwd=None, check=True, dry_run=False):
    logging.info(f"$ {' '.join(cmd)}")
    if dry_run:
        logging.info("  [DRY RUN] Command not executed")
        return None
    result = subprocess.run(cmd, cwd=cwd, check=check, capture_output=True, text=True)
    if result.stdout:
        logging.info(result.stdout)
    if result.stderr:
        logging.info(result.stderr)
    return result


def create_vscode_copilot_symlinks(repo_dir: str, dry_run: bool):
    """Create VS Code Copilot symlinks in .vscode/copilot/ pointing to .github/instructions/"""
    vscode_copilot_dir = os.path.join(repo_dir, ".vscode", "copilot")
    github_instructions_dir = os.path.join(repo_dir, ".github", "instructions")

    if not os.path.exists(github_instructions_dir):
        logging.info(
            "  [SKIP] .github/instructions/ not found, skipping VS Code symlinks"
        )
        return

    if dry_run:
        logging.info("  [DRY RUN] Would create .vscode/copilot/ symlinks")
        return

    os.makedirs(vscode_copilot_dir, exist_ok=True)

    # Create symlinks for all instruction files
    for file in os.listdir(github_instructions_dir):
        if file.endswith(".instructions.md"):
            src = os.path.join("..", "..", ".github", "instructions", file)
            dst = os.path.join(vscode_copilot_dir, file)

            # Remove existing file/symlink
            if os.path.exists(dst) or os.path.islink(dst):
                os.remove(dst)

            os.symlink(src, dst)
            logging.info(
                f"  Created symlink: .vscode/copilot/{file} -> ../../.github/instructions/{file}"
            )


def sync_to_repo(
    repo: str, branch: str, gh_token: str, summary: List[str], dry_run: bool
):
    logging.info(f"\n=== Syncing to {repo} ===")

    if dry_run:
        # Show detailed dry-run preview
        logging.info("  DRY RUN - Would make these changes:")

        # Check what old files would be removed
        for old_file in OLD_FILES_TO_REMOVE:
            logging.info(f"    REMOVE: {old_file} (if exists)")

        # Check what managed files would be synced
        for managed_file in MANAGED_FILES:
            src = os.path.abspath(managed_file)
            if os.path.exists(src):
                logging.info(f"    SYNC: {managed_file}")
            else:
                logging.info(f"    SKIP: {managed_file} (source not found)")

        # Show VS Code symlinks that would be created
        logging.info("    VS Code Copilot symlinks:")
        for file in MANAGED_FILES:
            if file.startswith(".github/instructions/") and file.endswith(
                ".instructions.md"
            ):
                basename = os.path.basename(file)
                logging.info(
                    f"      CREATE: .vscode/copilot/{basename} -> ../../{file}"
                )

        summary.append(
            f"[DRY RUN] {repo}: Would sync {len(MANAGED_FILES)} files and clean up {len(OLD_FILES_TO_REMOVE)} old files"
        )
        return

    repo_url = f"https://{gh_token}:x-oauth-basic@github.com/{repo}.git"

    with tempfile.TemporaryDirectory() as tmpdir:
        # Clone the target repo
        run(["git", "clone", "--depth=1", repo_url, tmpdir], dry_run=dry_run)

        # Create/checkout branch
        try:
            run(["git", "checkout", branch], cwd=tmpdir, check=False, dry_run=dry_run)
        except subprocess.CalledProcessError:
            run(["git", "checkout", "-b", branch], cwd=tmpdir, dry_run=dry_run)

        changes_made = False

        # 1. Remove old files that have been moved
        for old_file in OLD_FILES_TO_REMOVE:
            old_path = os.path.join(tmpdir, old_file)
            if os.path.exists(old_path):
                logging.info(f"  Removing old file: {old_file}")
                if os.path.isdir(old_path):
                    shutil.rmtree(old_path)
                else:
                    os.remove(old_path)
                changes_made = True

        # 2. Sync managed files from ghcommon
        for managed_file in MANAGED_FILES:
            src = os.path.abspath(managed_file)
            dst = os.path.join(tmpdir, managed_file)

            if not os.path.exists(src):
                logging.info(f"  [WARN] Source file not found: {managed_file}")
                continue

            # Create directory if needed
            os.makedirs(os.path.dirname(dst), exist_ok=True)

            # Copy file
            if os.path.isdir(src):
                if os.path.exists(dst):
                    shutil.rmtree(dst)
                shutil.copytree(src, dst)
            else:
                shutil.copy2(src, dst)

            logging.info(f"  Synced: {managed_file}")
            changes_made = True

        # 3. Create VS Code Copilot symlinks
        create_vscode_copilot_symlinks(tmpdir, dry_run)
        changes_made = True

        # 4. Commit and push changes
        if changes_made:
            run(["git", "add", "."], cwd=tmpdir, dry_run=dry_run)
            run(
                ["git", "config", "user.name", "ghcommon-sync-bot"],
                cwd=tmpdir,
                dry_run=dry_run,
            )
            run(
                [
                    "git",
                    "config",
                    "user.email",
                    "ghcommon-sync-bot@users.noreply.github.com",
                ],
                cwd=tmpdir,
                dry_run=dry_run,
            )

            commit_msg = """chore(sync): sync .github structure from ghcommon

- Updated to new modular instruction system
- Synced .github/instructions/ and .github/prompts/
- Removed old code-style-*.md files (moved to instructions/)
- Created VS Code Copilot symlinks for instruction files

This maintains the centralized coding standards while supporting
the new VS Code Copilot customization features."""

            try:
                run(["git", "commit", "-m", commit_msg], cwd=tmpdir, dry_run=dry_run)
                run(
                    ["git", "push", "-u", "origin", branch, "--force"],
                    cwd=tmpdir,
                    dry_run=dry_run,
                )
                summary.append(f"[OK] {repo}: Synced to branch {branch}")
            except subprocess.CalledProcessError:
                summary.append(f"[SKIP] {repo}: No changes to commit")
        else:
            summary.append(f"[SKIP] {repo}: No changes needed")


def main():
    args = parse_args()

    if not args.dry_run:
        gh_token = os.environ.get("GH_TOKEN")
        if not gh_token:
            print("GH_TOKEN environment variable is required.", file=sys.stderr)
            sys.exit(1)
    else:
        gh_token = "dummy-for-dry-run"

    repos = [r.strip() for r in args.repos.split(",") if r.strip()]
    branch = args.branch
    summary = []

    logging.info(f"Syncing to {len(repos)} repositories:")
    for repo in repos:
        logging.info(f"  - {repo}")

    logging.info(f"\nManaged files to sync: {len(MANAGED_FILES)}")
    logging.info(f"Old files to clean up: {len(OLD_FILES_TO_REMOVE)}")

    if args.dry_run:
        logging.info("\n*** DRY RUN MODE - No changes will be made ***\n")

    for repo in repos:
        try:
            sync_to_repo(repo, branch, gh_token, summary, args.dry_run)
        except Exception as e:
            summary.append(f"[FAIL] {repo}: {str(e)}")
            logging.error(f"Failed to sync {repo}: {e}")

    # Write summary
    summary_file = "intelligent-sync-summary.log"
    with open(summary_file, "w") as f:
        for line in summary:
            f.write(line + "\n")

    logging.info("\n=== SYNC SUMMARY ===")
    for line in summary:
        logging.info(line)


if __name__ == "__main__":
    main()
