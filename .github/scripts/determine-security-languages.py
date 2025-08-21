#!/usr/bin/env python3
# file: .github/scripts/determine-security-languages.py
# version: 1.0.0
# guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

"""
Determine which languages need security scanning based on file changes.

This script reads environment variables for change detection outputs and generates
a JSON matrix for CodeQL security scanning, only including languages that have
actual file changes.
"""

import json
import os
import sys
from typing import Dict


def get_env_bool(key: str, default: bool = False) -> bool:
    """Safely get a boolean environment variable."""
    value = os.environ.get(key, "").lower()
    return value in ("true", "1", "yes", "on")


def determine_security_languages() -> Dict[str, any]:
    """
    Determine which languages need security scanning based on file changes.

    Returns:
        Dict containing matrix configuration and metadata
    """
    # Language mapping: env var name -> CodeQL language name
    language_mapping = {
        "GO_CHANGED": "go",
        "FRONTEND_CHANGED": "javascript",
        "PYTHON_CHANGED": "python",
        "RUST_CHANGED": "rust",
    }

    # Collect languages that have changes
    languages_with_changes = []

    for env_var, codeql_language in language_mapping.items():
        if get_env_bool(env_var):
            languages_with_changes.append(codeql_language)
            print(f"✓ {codeql_language} has changes")
        else:
            print(f"- {codeql_language} has no changes")

    # Generate matrix configuration
    if not languages_with_changes:
        matrix = {"include": []}
        has_languages = False
        print("No languages with changes detected for security scanning")
    else:
        matrix = {"language": languages_with_changes}
        has_languages = True
        print(f"Languages for security scanning: {', '.join(languages_with_changes)}")

    result = {
        "matrix": matrix,
        "has_languages": has_languages,
        "language_count": len(languages_with_changes),
        "languages": languages_with_changes,
    }

    print(f"Generated matrix: {json.dumps(matrix)}")
    return result


def write_github_output(key: str, value: str) -> None:
    """Write output to GitHub Actions output file."""
    github_output = os.environ.get("GITHUB_OUTPUT")
    if not github_output:
        print(f"Warning: GITHUB_OUTPUT not set, would write {key}={value}")
        return

    try:
        with open(github_output, "a", encoding="utf-8") as f:
            f.write(f"{key}={value}\n")
        print(f"✓ Wrote {key}={value} to GITHUB_OUTPUT")
    except Exception as e:
        print(f"Error writing to GITHUB_OUTPUT: {e}")
        sys.exit(1)


def main() -> None:
    """Main entry point."""
    try:
        print("🔍 Determining security scan languages...")

        # Determine security languages
        result = determine_security_languages()

        # Write outputs for GitHub Actions
        write_github_output("matrix", json.dumps(result["matrix"]))
        write_github_output("has-languages", str(result["has_languages"]).lower())
        write_github_output("language-count", str(result["language_count"]))
        write_github_output("languages", ",".join(result["languages"]))

        print("✅ Security language determination completed successfully")

    except Exception as e:
        print(f"❌ Error determining security languages: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
