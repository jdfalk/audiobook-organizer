#!/usr/bin/env python3
# file: scripts/create-module-tags.py
# version: 1.0.0
# guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

"""
Create module-specific Git tags for Go SDK packages.

This script automatically creates module-specific tags for each Go SDK package
when a new version tag is created. This is required for Go modules to properly
resolve dependencies when using local modules.

For version v1.3.0, it creates:
- sdks/go/v1/common/v1.3.0
- sdks/go/v1/config/v1.3.0
- sdks/go/v1/database/v1.3.0
- etc.

Usage:
    python3 scripts/create-module-tags.py <version>
    python3 scripts/create-module-tags.py v1.3.0
"""

import subprocess
import sys
from pathlib import Path


def run_command(cmd, check=True):
    """Run a command and return the result."""
    try:
        result = subprocess.run(
            cmd, shell=True, check=check, capture_output=True, text=True
        )
        return result.stdout.strip()
    except subprocess.CalledProcessError as e:
        print(f"❌ Command failed: {cmd}")
        print(f"   Error: {e.stderr.strip()}")
        if check:
            sys.exit(1)
        return None


def get_sdk_modules():
    """Get list of SDK modules that need tags."""
    sdk_path = Path("sdks/go/v1")
    if not sdk_path.exists():
        print(f"❌ SDK path does not exist: {sdk_path}")
        return []

    modules = []
    for item in sdk_path.iterdir():
        if item.is_dir() and (item / "go.mod").exists():
            modules.append(item.name)

    return sorted(modules)


def tag_exists(tag_name):
    """Check if a tag already exists."""
    result = run_command(f"git tag -l '{tag_name}'", check=False)
    return bool(result)


def create_module_tags(version):
    """Create module-specific tags for the given version."""
    # Normalize version (ensure it starts with 'v')
    if not version.startswith("v"):
        version = f"v{version}"

    print(f"🏷️  Creating module tags for version {version}")

    # Get the list of SDK modules
    modules = get_sdk_modules()
    if not modules:
        print("❌ No SDK modules found in sdks/go/v1/")
        sys.exit(1)

    print(f"📦 Found {len(modules)} SDK modules: {', '.join(modules)}")

    # Create tags for each module
    created_count = 0
    skipped_count = 0

    for module in modules:
        module_tag = f"sdks/go/v1/{module}/{version}"

        if tag_exists(module_tag):
            print(f"⏭️  Tag already exists: {module_tag}")
            skipped_count += 1
            continue

        # Create the tag at the same commit as the main version tag
        cmd = f"git tag {module_tag} {version}"
        result = run_command(cmd, check=False)

        if result is not None:
            print(f"✅ Created tag: {module_tag}")
            created_count += 1
        else:
            print(f"❌ Failed to create tag: {module_tag}")

    print(f"\n📊 Summary: {created_count} created, {skipped_count} skipped")

    if created_count > 0:
        print("💡 Don't forget to push the tags: git push origin --tags")


def main():
    """Main function."""
    if len(sys.argv) != 2:
        print("Usage: python3 scripts/create-module-tags.py <version>")
        print("Example: python3 scripts/create-module-tags.py v1.3.0")
        sys.exit(1)

    version = sys.argv[1]

    # Validate that we're in a git repository
    result = run_command("git rev-parse --git-dir", check=False)
    if result is None:
        print("❌ Not in a Git repository")
        sys.exit(1)

    # Validate that the main version tag exists
    version_normalized = version if version.startswith("v") else f"v{version}"
    if not tag_exists(version_normalized):
        print(f"❌ Version tag {version_normalized} does not exist")
        print("   Create the main version tag first")
        sys.exit(1)

    create_module_tags(version)


if __name__ == "__main__":
    main()
