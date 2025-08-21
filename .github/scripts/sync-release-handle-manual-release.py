#!/usr/bin/env python3
# file: .github/scripts/sync-release-handle-manual-release.py
# version: 1.0.0
# guid: b8c9d0e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e

"""
Handle manual release version calculation.
Usage: sync-release-handle-manual-release.py <release_type> <language>
"""

import sys
import os
import re
import subprocess
from pathlib import Path


def get_current_version_rust():
    """Get current version from Cargo.toml."""
    cargo_toml = Path("Cargo.toml")
    if not cargo_toml.exists():
        raise ValueError("Cargo.toml not found")

    content = cargo_toml.read_text()
    match = re.search(r'^version = "([^"]+)"', content, re.MULTILINE)
    if not match:
        raise ValueError("Version not found in Cargo.toml")

    return match.group(1)


def get_current_version_go():
    """Get current version from git tags (Go modules use git tags)."""
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
    pyproject = Path("pyproject.toml")
    if pyproject.exists():
        content = pyproject.read_text()
        match = re.search(r'^version = "([^"]+)"', content, re.MULTILINE)
        if match:
            return match.group(1)

    # Try setup.py
    setup_py = Path("setup.py")
    if setup_py.exists():
        content = setup_py.read_text()
        match = re.search(r'version=["\']([^"\']+)["\']', content)
        if match:
            return match.group(1)

    # Try __init__.py
    init_py = Path("__init__.py")
    if init_py.exists():
        content = init_py.read_text()
        match = re.search(r'__version__ = ["\']([^"\']+)["\']', content)
        if match:
            return match.group(1)

    return "0.0.0"


def get_current_version_js_ts():
    """Get current version from package.json."""
    package_json = Path("package.json")
    if not package_json.exists():
        return "0.0.0"

    import json

    try:
        data = json.loads(package_json.read_text())
        return data.get("version", "0.0.0")
    except json.JSONDecodeError:
        return "0.0.0"


def get_current_version(language):
    """Get current version based on language."""
    version_getters = {
        "rust": get_current_version_rust,
        "go": get_current_version_go,
        "python": get_current_version_python,
        "javascript": get_current_version_js_ts,
        "typescript": get_current_version_js_ts,
    }

    getter = version_getters.get(language)
    if not getter:
        raise ValueError(f"Unsupported language: {language}")

    return getter()


def increment_version(current_version, release_type):
    """Increment version based on release type."""
    try:
        parts = current_version.split(".")
        if len(parts) != 3:
            raise ValueError(f"Invalid version format: {current_version}")

        major, minor, patch = [int(p) for p in parts]

        if release_type == "major":
            major += 1
            minor = 0
            patch = 0
        elif release_type == "minor":
            minor += 1
            patch = 0
        elif release_type == "patch":
            patch += 1
        else:
            raise ValueError(f"Invalid release type: {release_type}")

        return f"{major}.{minor}.{patch}"

    except ValueError as e:
        raise ValueError(f"Failed to increment version: {e}")


def set_github_output(name, value):
    """Set GitHub Actions output variable."""
    github_output = os.getenv("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a") as f:
            f.write(f"{name}={value}\n")


def main():
    """Main entry point."""
    if len(sys.argv) != 3:
        print("Error: Both release_type and language parameters required")
        print("Usage: sync-release-handle-manual-release.py <release_type> <language>")
        sys.exit(1)

    release_type = sys.argv[1]
    language = sys.argv[2]

    try:
        # Get current version
        current_version = get_current_version(language)
        print(f"Current version: {current_version}")

        # Calculate new version
        new_version = increment_version(current_version, release_type)
        print(f"New version: {new_version}")

        # Create tag with 'v' prefix
        tag = f"v{new_version}"

        # Set outputs
        set_github_output("version", new_version)
        set_github_output("tag", tag)
        set_github_output("should-release", "true")
        set_github_output("changelog", f"Manual {release_type} release")

        print(f"Manual release configured: {release_type} -> {new_version}")

    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
