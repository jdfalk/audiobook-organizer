#!/usr/bin/env python3
# file: .github/scripts/sync-release-create-package-json.py
# version: 1.0.0
# guid: f6a7b8c9-d0e1-2f3a-4b5c-6d7e8f9a0b1c

"""
Create package.json for semantic-release based on language type.
Usage: sync-release-create-package-json.py <language>
"""

import json
import sys


def get_base_dependencies():
    """Get base dependencies common to most language projects."""
    return {
        "@semantic-release/changelog": "^6.0.3",
        "@semantic-release/git": "^10.0.1",
        "@semantic-release/github": "^9.2.6",
        "@semantic-release/exec": "^6.0.3",
        "semantic-release": "^22.0.12",
        "conventional-changelog-conventionalcommits": "^7.0.2",
    }


def create_package_json(language):
    """Create package.json for the specified language."""
    base_deps = get_base_dependencies()

    package_configs = {
        "rust": {
            "name": "rust-project",
            "devDependencies": base_deps,
        },
        "go": {
            "name": "go-project",
            "devDependencies": base_deps,
        },
        "python": {
            "name": "python-project",
            "devDependencies": base_deps,
        },
        "javascript": {
            "name": "js-project",
            "devDependencies": {
                **base_deps,
                "@semantic-release/npm": "^11.0.3",
            },
        },
        "typescript": {
            "name": "ts-project",
            "devDependencies": {
                **base_deps,
                "@semantic-release/npm": "^11.0.3",
            },
        },
    }

    if language not in package_configs:
        raise ValueError(f"Unsupported language: {language}")

    config = package_configs[language]

    package_json = {
        "name": config["name"],
        "version": "0.0.0-development",
        "private": True,
        "devDependencies": config["devDependencies"],
    }

    # Write package.json with proper formatting
    with open("package.json", "w") as f:
        json.dump(package_json, f, indent=2)

    print(f"Package.json created successfully for {language}")


def main():
    """Main entry point."""
    if len(sys.argv) != 2:
        print("Error: Language parameter required")
        print("Usage: sync-release-create-package-json.py <language>")
        sys.exit(1)

    language = sys.argv[1]

    try:
        create_package_json(language)
    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
