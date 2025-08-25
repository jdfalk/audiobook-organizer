#!/usr/bin/env python3
# file: scripts/post-tag-hook.py
# version: 1.0.0
# guid: 2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e

"""
Post-tag hook script for creating module-specific tags.

This script can be called by workflows after creating a version tag to
automatically create module-specific tags for Go SDK packages.

Usage:
    python3 scripts/post-tag-hook.py <version_tag>
    python3 scripts/post-tag-hook.py v1.3.0

Environment Variables:
    PUSH_TAGS: Set to 'true' to automatically push tags to origin
    CI: Automatically detected in CI environments
"""

import sys
from pathlib import Path

# Add the scripts directory to the path so we can import the module tagger
sys.path.insert(0, str(Path(__file__).parent))

try:
    # Import our module tagging script functionality
    import subprocess

    def run_module_tagging(version_tag):
        """Run the module tagging script."""
        script_path = Path(__file__).parent / "create-module-tags.py"

        if not script_path.exists():
            print(f"⚠️  Module tagging script not found: {script_path}")
            print("ℹ️  This is normal for repositories without Go SDK modules")
            return True

        print(f"🏷️  Running module tagging for {version_tag}")

        # Run the module tagging script
        result = subprocess.run(
            [sys.executable, str(script_path), version_tag],
            capture_output=True,
            text=True,
        )

        if result.returncode == 0:
            print("✅ Module tagging completed successfully")
            if result.stdout:
                print(result.stdout)
            return True
        else:
            print("❌ Module tagging failed")
            if result.stderr:
                print(f"Error: {result.stderr}")
            if result.stdout:
                print(f"Output: {result.stdout}")
            return False

    def main():
        """Main function."""
        if len(sys.argv) != 2:
            print("Usage: python3 scripts/post-tag-hook.py <version_tag>")
            print("Example: python3 scripts/post-tag-hook.py v1.3.0")
            return 1

        version_tag = sys.argv[1]

        print(f"📋 Post-tag hook processing version: {version_tag}")

        # Check if we're in a repository with Go SDK modules
        sdk_path = Path("sdks/go/v1")
        if not sdk_path.exists():
            print("ℹ️  No Go SDK modules found - skipping module tagging")
            print("   This is normal for repositories without protobuf SDKs")
            return 0

        # Run module tagging
        success = run_module_tagging(version_tag)

        if success:
            print("🎉 Post-tag hook completed successfully")
            return 0
        else:
            print("❌ Post-tag hook failed")
            return 1

except ImportError as e:
    print(f"⚠️  Import error: {e}")
    print("ℹ️  Module tagging functionality not available")
    print("   This is normal for repositories without the required scripts")
    sys.exit(0)

if __name__ == "__main__":
    sys.exit(main())
