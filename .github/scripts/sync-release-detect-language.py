#!/usr/bin/env python3
# file: .github/scripts/sync-release-detect-language.py
# version: 1.2.0
# guid: a7b8c9d0-e1f2-3a4b-5c6d-7e8f9a0b1c2d

"""
Detect programming languages and determine if release should be triggered.
Usage: sync-release-detect-language.py [force_language]
"""

import os
import json
import sys
import subprocess
from pathlib import Path
from typing import Dict, List


def check_file_exists(filename):
    """Check if a file exists in the current directory."""
    return Path(filename).exists()


def has_changes_since_last_release():
    """Check if there are changes since the last release tag."""
    try:
        # Get the latest tag
        result = subprocess.run(
            ["git", "describe", "--tags", "--abbrev=0"],
            capture_output=True,
            text=True,
            check=False,
        )

        if result.returncode != 0:
            # No tags found, so there are changes
            return True

        latest_tag = result.stdout.strip()

        # Check if there are commits since the latest tag
        result = subprocess.run(
            ["git", "rev-list", f"{latest_tag}..HEAD", "--count"],
            capture_output=True,
            text=True,
            check=True,
        )

        commit_count = int(result.stdout.strip())
        return commit_count > 0

    except (subprocess.CalledProcessError, ValueError):
        # If git commands fail, assume there are changes
        return True


def detect_languages() -> Dict[str, bool]:
    """Detect all programming languages present in the project."""
    langs = {
        "rust": False,
        "go": False,
        "python": False,
        "javascript": False,
        "typescript": False,
        "docker": False,
    }

    # Fast file-based checks
    if check_file_exists("Cargo.toml") or list(Path("src").glob("**/*.rs")):
        langs["rust"] = True
    if check_file_exists("go.mod") or list(Path(".").glob("**/*.go")):
        langs["go"] = True
    if check_file_exists("pyproject.toml") or check_file_exists("setup.py"):
        langs["python"] = True
    if check_file_exists("package.json") or list(Path(".").glob("**/*.js")):
        langs["javascript"] = True
    # TypeScript refinement
    if check_file_exists("tsconfig.json") or list(Path(".").glob("**/*.ts")):
        langs["typescript"] = True
        # If TS present, JS may be build output; keep both true when both exist
    # Docker
    dockerfiles = list(Path(".").glob("**/Dockerfile*"))
    if dockerfiles:
        langs["docker"] = True

    return langs


def should_release():
    """Determine if a release should be triggered."""
    # Check for changes since last release
    has_changes = has_changes_since_last_release()

    # For manual dispatch, always allow release
    if os.getenv("GITHUB_EVENT_NAME") == "workflow_dispatch":
        return True

    # For push events, only release if there are changes
    if os.getenv("GITHUB_EVENT_NAME") == "push":
        return has_changes

    # Default: no release
    return False


def set_github_output(name, value):
    """Set GitHub Actions output variable."""
    github_output = os.getenv("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a") as f:
            f.write(f"{name}={value}\n")


def main():
    """Main entry point."""
    # Prefer argv if provided; fall back to FORCE_LANGUAGE env for workflow_dispatch compatibility
    force_language = sys.argv[1] if len(sys.argv) > 1 else os.getenv("FORCE_LANGUAGE")
    if force_language:
        force_language = force_language.strip().lower()

    # Detect languages
    if force_language and force_language != "auto":
        # Back-compat: force single primary language
        language = force_language
        languages = {
            "rust": False,
            "go": False,
            "python": False,
            "javascript": False,
            "typescript": False,
            "docker": False,
        }
        languages[language] = True
        print(f"Language forced to: {language}")
    else:
        languages = detect_languages()
        # Choose a primary for back-compat; priority order
        priority: List[str] = ["rust", "go", "typescript", "javascript", "python"]
        language = next((lang for lang in priority if languages.get(lang)), "unknown")
        print(
            f"Detected languages: {', '.join([k for k, v in languages.items() if v]) or 'none'}"
        )

    # Determine if release should happen
    should_rel = should_release()
    print(f"Should release: {should_rel}")

    # Set outputs for GitHub Actions (back-compat + matrix)
    set_github_output("language", language)
    set_github_output("should-release", "true" if should_rel else "false")
    # New multi-language outputs
    set_github_output("languages", ",".join([k for k, v in languages.items() if v]))
    set_github_output("languages_json", json.dumps(languages))
    # Ordered matrix array (TS before GO to allow codegen, then others)
    ordered = [
        lang
        for lang in ["typescript", "go", "rust", "javascript", "python"]
        if languages.get(lang)
    ]
    set_github_output("languages_matrix", json.dumps({"language": ordered}))

    # Print summary
    print("\nLanguage Detection Summary:")
    print(f"  Primary: {language}")
    print(f"  All: {', '.join([k for k, v in languages.items() if v]) or 'none'}")
    print(f"  Matrix JSON: {json.dumps(languages)}")
    print(f"  Should Release: {should_rel}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
