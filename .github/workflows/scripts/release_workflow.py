#!/usr/bin/env python3
# file: .github/workflows/scripts/release_workflow.py
# version: 1.0.0
# guid: 9f7e6d5c-4b3a-2f1e-0d9c-8b7a6f5e4d3c

"""
Minimal changelog generator for prerelease workflow.

The upstream reusable workflow expects a release_workflow.py script exposed via
GHCOMMON_SCRIPTS_DIR. We provide a lightweight implementation that emits a
synthetic changelog so the prerelease pipeline can complete without depending
on external scripts.
"""

from __future__ import annotations

import os
import sys
import textwrap


def write_output(key: str, value: str) -> None:
    """Append a multiline output value for GitHub Actions."""
    output_path = os.environ.get("GITHUB_OUTPUT")
    if not output_path:
        return
    with open(output_path, "a", encoding="utf-8") as handle:
        handle.write(f"{key}<<'EOF'\n{value}\nEOF\n")


def generate_changelog() -> None:
    """Emit a simple changelog block for prerelease builds."""
    content = textwrap.dedent(
        """\
        ## Changelog

        - Automated prerelease build (local changelog generator)
        """
    )
    write_output("changelog_content", content)


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: release_workflow.py <command>", file=sys.stderr)
        return 1

    command = argv[1]
    if command == "generate-changelog":
        generate_changelog()
        return 0

    print(f"unknown command: {command}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
