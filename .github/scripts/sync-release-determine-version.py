#!/usr/bin/env python3
# file: .github/scripts/sync-release-determine-version.py
# version: 1.0.0
# guid: f6a7b8c9-d0e1-2f3a-4b5c-6d7e8f9a0b1c

"""
Determine version for release based on manual input or semantic-release analysis.
Usage: sync-release-determine-version.py <language> <manual_release_type> <github_token>
"""

import sys
import re
import subprocess
import json
from pathlib import Path


def get_current_version_rust():
    """Get current version from Cargo.toml."""
    try:
        with open("Cargo.toml", "r") as f:
            content = f.read()

        match = re.search(r'^version = "([^"]+)"', content, re.MULTILINE)
        if match:
            return match.group(1)
        else:
            raise ValueError("Could not find version in Cargo.toml")
    except FileNotFoundError:
        raise ValueError("Cargo.toml not found")


def get_current_version_go():
    """Get current version from VERSION file or git tags."""
    # Try VERSION file first
    version_file = Path("VERSION")
    if version_file.exists():
        return version_file.read_text().strip()

    # Fall back to git tags
    try:
        result = subprocess.run(
            ["git", "describe", "--tags", "--abbrev=0"],
            capture_output=True,
            text=True,
            check=True,
        )
        tag = result.stdout.strip()
        # Remove 'v' prefix if present
        return tag.lstrip("v")
    except subprocess.CalledProcessError:
        return "0.0.0"


def get_current_version_python():
    """Get current version from pyproject.toml or setup.py."""
    # Try pyproject.toml first
    pyproject_file = Path("pyproject.toml")
    if pyproject_file.exists():
        content = pyproject_file.read_text()
        match = re.search(r'version = "([^"]+)"', content)
        if match:
            return match.group(1)

    # Try setup.py
    setup_file = Path("setup.py")
    if setup_file.exists():
        content = setup_file.read_text()
        match = re.search(r'version=["\']([^"\']+)["\']', content)
        if match:
            return match.group(1)

    # Try __init__.py
    init_file = Path("__init__.py")
    if init_file.exists():
        content = init_file.read_text()
        match = re.search(r'__version__ = ["\']([^"\']+)["\']', content)
        if match:
            return match.group(1)

    return "0.0.0"


def get_current_version_js_ts():
    """Get current version from package.json."""
    try:
        with open("package.json", "r") as f:
            package_data = json.load(f)
        return package_data.get("version", "0.0.0")
    except FileNotFoundError:
        return "0.0.0"


def increment_version(version, release_type):
    """Increment version based on release type."""
    parts = version.split(".")
    if len(parts) != 3:
        raise ValueError(f"Invalid version format: {version}")

    major, minor, patch = map(int, parts)

    if release_type == "major":
        return f"{major + 1}.0.0"
    elif release_type == "minor":
        return f"{major}.{minor + 1}.0"
    elif release_type == "patch":
        return f"{major}.{minor}.{patch + 1}"
    else:
        raise ValueError(f"Invalid release type: {release_type}")


def run_semantic_release_dry_run(github_token):
    """Run semantic-release in dry-run mode to determine version."""
    env = subprocess.os.environ.copy()
    env["GITHUB_TOKEN"] = github_token

    try:
        result = subprocess.run(
            ["npx", "semantic-release", "--dry-run"],
            capture_output=True,
            text=True,
            env=env,
        )

        output = result.stdout + result.stderr

        # Look for version in output
        version_match = re.search(r"The next release version is ([0-9.]+)", output)
        if version_match:
            return version_match.group(1)

        return None

    except subprocess.CalledProcessError as e:
        print(f"Warning: semantic-release dry-run failed: {e}", file=sys.stderr)
        return None


def set_github_output(key, value):
    """Set GitHub Actions output variable."""
    print(f"{key}={value}")

    # Also write to GITHUB_OUTPUT file if it exists
    github_output = subprocess.os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a") as f:
            f.write(f"{key}={value}\n")


def main():
    """Main function to determine release version."""
    if len(sys.argv) != 4:
        print("Error: Missing required parameters", file=sys.stderr)
        print(
            f"Usage: {sys.argv[0]} <language> <manual_release_type> <github_token>",
            file=sys.stderr,
        )
        sys.exit(1)

    language = sys.argv[1].lower()
    manual_release_type = sys.argv[2]
    github_token = sys.argv[3]

    # Get current version based on language
    version_getters = {
        "rust": get_current_version_rust,
        "go": get_current_version_go,
        "python": get_current_version_python,
        "javascript": get_current_version_js_ts,
        "typescript": get_current_version_js_ts,
    }

    if language not in version_getters:
        print(f"Error: Unsupported language: {language}", file=sys.stderr)
        sys.exit(1)

    try:
        current_version = version_getters[language]()
        print(f"Current version: {current_version}")

        if manual_release_type and manual_release_type != "auto":
            # Manual release
            print(f"Manual release type specified: {manual_release_type}")

            new_version = increment_version(current_version, manual_release_type)

            set_github_output("version", new_version)
            set_github_output("tag", f"v{new_version}")
            set_github_output("should-release", "true")
            set_github_output("changelog", f"Manual {manual_release_type} release")

            print(f"Manual release version: {new_version}")

        else:
            # Automatic release via semantic-release
            print("Running semantic-release dry-run to determine version...")

            new_version = run_semantic_release_dry_run(github_token)

            if new_version:
                set_github_output("version", new_version)
                set_github_output("tag", f"v{new_version}")
                set_github_output("should-release", "true")
                set_github_output("changelog", "Automatic semantic release")

                print(f"Semantic release version: {new_version}")

            else:
                set_github_output("version", current_version)
                set_github_output("tag", f"v{current_version}")
                set_github_output("should-release", "false")
                set_github_output("changelog", "No release needed")

                print("No release needed based on semantic analysis")

    except Exception as e:
        print(f"Error determining version: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
