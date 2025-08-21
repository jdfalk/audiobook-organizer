#!/usr/bin/env python3
# file: scripts/unified_operations_detector.py
# version: 1.0.0
# guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

"""
Unified Operations Detector

Intelligently determines which operations to run based on:
- Explicit input parameters
- Available files in the repository
- Event triggers (schedule, workflow_dispatch, etc.)

Used by the unified automation workflow to decide between:
- Issue management operations (update-issues, copilot-tickets, etc.)
- Documentation update operations (process doc update files)
"""

import os
import sys
import json
import logging
from pathlib import Path
from typing import List, Dict

# Configure logging
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


class UnifiedOperationsDetector:
    """Detects which operations need to be run for the unified workflow."""

    def __init__(self):
        self.operations_input = os.getenv("OPERATIONS_INPUT", "auto").strip()
        self.event_name = os.getenv("EVENT_NAME", "")
        self.issue_updates_file = os.getenv("ISSUE_UPDATES_FILE", "issue_updates.json")
        self.issue_updates_directory = os.getenv(
            "ISSUE_UPDATES_DIRECTORY", ".github/issue-updates"
        )
        self.doc_updates_directory = os.getenv(
            "DOC_UPDATES_DIRECTORY", ".github/doc-updates"
        )

        # Define all possible operations
        self.issue_operations = {
            "update-issues",
            "copilot-tickets",
            "close-duplicates",
            "codeql-alerts",
            "update-permalinks",
        }

        self.doc_operations = {"doc-updates"}

    def detect_operations(self) -> Dict[str, any]:
        """
        Detect which operations to run based on input and available files.

        Returns:
            Dict with detected operations and file availability
        """
        logger.info(f"🔍 Detecting operations for input: '{self.operations_input}'")
        logger.info(f"📅 Event: {self.event_name}")

        if self.operations_input == "auto":
            issue_ops, doc_ops = self._auto_detect_operations()
        else:
            issue_ops, doc_ops = self._parse_explicit_operations()

        # Check for available files
        has_issue_updates = self._check_issue_updates_available()
        has_doc_updates = self._check_doc_updates_available()

        # Filter operations based on file availability
        if not has_issue_updates:
            logger.info("📝 No issue updates found - removing issue operations")
            issue_ops = []

        if not has_doc_updates:
            logger.info("📝 No doc updates found - removing doc operations")
            doc_ops = []

        results = {
            "issue_operations": issue_ops,
            "doc_operations": doc_ops,
            "has_issue_updates": has_issue_updates,
            "has_doc_updates": has_doc_updates,
        }

        logger.info(f"✅ Detected operations: {results}")
        return results

    def _auto_detect_operations(self) -> tuple[List[str], List[str]]:
        """Auto-detect operations based on available files and event type."""
        issue_ops = []
        doc_ops = []

        logger.info("🤖 Auto-detecting operations...")

        # Check for issue-related files
        if self._check_issue_updates_available():
            logger.info("📋 Found issue update files - adding issue operations")
            # For auto-detection, include the most common operations
            issue_ops = ["update-issues", "update-permalinks"]

        # Check for doc-related files
        if self._check_doc_updates_available():
            logger.info("📝 Found doc update files - adding doc operations")
            doc_ops = ["doc-updates"]

        # Event-based detection
        if self.event_name == "schedule":
            logger.info("⏰ Scheduled event - adding maintenance operations")
            issue_ops.extend(["close-duplicates", "codeql-alerts"])

        # Remove duplicates while preserving order
        issue_ops = list(dict.fromkeys(issue_ops))
        doc_ops = list(dict.fromkeys(doc_ops))

        return issue_ops, doc_ops

    def _parse_explicit_operations(self) -> tuple[List[str], List[str]]:
        """Parse explicitly requested operations."""
        operations = [
            op.strip() for op in self.operations_input.split(",") if op.strip()
        ]

        issue_ops = [op for op in operations if op in self.issue_operations]
        doc_ops = [op for op in operations if op in self.doc_operations]

        unknown_ops = [
            op
            for op in operations
            if op not in self.issue_operations and op not in self.doc_operations
        ]

        if unknown_ops:
            logger.warning(f"⚠️ Unknown operations requested: {unknown_ops}")

        logger.info(f"📋 Explicit issue operations: {issue_ops}")
        logger.info(f"📝 Explicit doc operations: {doc_ops}")

        return issue_ops, doc_ops

    def _check_issue_updates_available(self) -> bool:
        """Check if issue update files are available."""
        # Check main issue updates file
        if Path(self.issue_updates_file).exists():
            logger.info(f"✅ Found issue updates file: {self.issue_updates_file}")
            return True

        # Check issue updates directory
        issue_dir = Path(self.issue_updates_directory)
        if issue_dir.exists():
            json_files = list(issue_dir.glob("*.json"))
            if json_files:
                logger.info(
                    f"✅ Found {len(json_files)} issue update files in {self.issue_updates_directory}"
                )
                return True

        logger.info("📝 No issue update files found")
        return False

    def _check_doc_updates_available(self) -> bool:
        """Check if documentation update files are available."""
        doc_dir = Path(self.doc_updates_directory)

        if not doc_dir.exists():
            logger.info(
                f"📝 Doc updates directory not found: {self.doc_updates_directory}"
            )
            return False

        # Look for JSON files in the main directory (not subdirectories)
        json_files = [f for f in doc_dir.glob("*.json") if f.is_file()]

        if json_files:
            logger.info(
                f"✅ Found {len(json_files)} doc update files in {self.doc_updates_directory}"
            )
            return True

        logger.info(f"📝 No doc update files found in {self.doc_updates_directory}")
        return False

    def set_github_outputs(self, results: Dict[str, any]) -> None:
        """Set GitHub Actions outputs."""
        github_output = os.getenv("GITHUB_OUTPUT")

        if not github_output:
            logger.warning(
                "⚠️ GITHUB_OUTPUT not set - outputs will be printed to stdout"
            )
            github_output = "/dev/stdout"

        with open(github_output, "a") as f:
            # Convert lists to JSON strings for GitHub Actions
            f.write(f"issue_operations={json.dumps(results['issue_operations'])}\n")
            f.write(f"doc_operations={json.dumps(results['doc_operations'])}\n")
            f.write(f"has_issue_updates={str(results['has_issue_updates']).lower()}\n")
            f.write(f"has_doc_updates={str(results['has_doc_updates']).lower()}\n")

        logger.info("✅ GitHub outputs set successfully")


def main():
    """Main entry point."""
    try:
        detector = UnifiedOperationsDetector()
        results = detector.detect_operations()
        detector.set_github_outputs(results)

        # Also print summary to stdout for debugging
        print("🔍 Unified Operations Detection Summary:")
        print(f"  - Issue Operations: {results['issue_operations']}")
        print(f"  - Doc Operations: {results['doc_operations']}")
        print(f"  - Has Issue Updates: {results['has_issue_updates']}")
        print(f"  - Has Doc Updates: {results['has_doc_updates']}")

        sys.exit(0)

    except Exception as e:
        logger.error(f"❌ Error detecting operations: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
