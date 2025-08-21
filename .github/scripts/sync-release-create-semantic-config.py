#!/usr/bin/env python3
# file: .github/scripts/sync-release-create-semantic-config.py
# version: 1.0.0
# guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

"""
Create semantic-release configuration based on language type.
Usage: sync-release-create-semantic-config.py <language>
"""

import json
import sys


def get_base_config():
    """Get base configuration common to all languages."""
    return {
        "branches": ["main"],
        "plugins": [
            [
                "@semantic-release/commit-analyzer",
                {
                    "preset": "conventionalcommits",
                    "releaseRules": [
                        {"type": "feat", "release": "minor"},
                        {"type": "fix", "release": "patch"},
                        {"type": "perf", "release": "patch"},
                        {"type": "revert", "release": "patch"},
                        {"type": "docs", "release": False},
                        {"type": "style", "release": False},
                        {"type": "chore", "release": False},
                        {"type": "refactor", "release": "patch"},
                        {"type": "test", "release": False},
                        {"type": "build", "release": False},
                        {"type": "ci", "release": False},
                        {"breaking": True, "release": "major"},
                    ],
                },
            ],
            [
                "@semantic-release/release-notes-generator",
                {"preset": "conventionalcommits"},
            ],
        ],
    }


def get_rust_config():
    """Get Rust-specific semantic-release configuration."""
    config = get_base_config()
    config["plugins"].extend(
        [
            [
                "@semantic-release/exec",
                {
                    "prepareCmd": 'sed -i \'s/^version = ".*"/version = "${nextRelease.version}"/\' Cargo.toml'
                },
            ],
            ["@semantic-release/changelog", {"changelogFile": "CHANGELOG.md"}],
            [
                "@semantic-release/git",
                {
                    "assets": ["Cargo.toml", "CHANGELOG.md"],
                    "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
                },
            ],
            [
                "@semantic-release/github",
                {"assets": ["./releases/*.tar.gz", "./releases/*.zip"]},
            ],
        ]
    )
    return config


def get_go_config():
    """Get Go-specific semantic-release configuration."""
    config = get_base_config()
    config["plugins"].extend(
        [
            [
                "@semantic-release/exec",
                {"prepareCmd": 'echo "${nextRelease.version}" > VERSION'},
            ],
            ["@semantic-release/changelog", {"changelogFile": "CHANGELOG.md"}],
            [
                "@semantic-release/git",
                {
                    "assets": ["VERSION", "CHANGELOG.md"],
                    "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
                },
            ],
            [
                "@semantic-release/github",
                {"assets": ["./releases/*.tar.gz", "./releases/*.zip"]},
            ],
        ]
    )
    return config


def get_python_config():
    """Get Python-specific semantic-release configuration."""
    config = get_base_config()
    config["plugins"].extend(
        [
            [
                "@semantic-release/exec",
                {
                    "prepareCmd": "python scripts/update_version.py ${nextRelease.version}"
                },
            ],
            ["@semantic-release/changelog", {"changelogFile": "CHANGELOG.md"}],
            [
                "@semantic-release/git",
                {
                    "assets": [
                        "pyproject.toml",
                        "setup.py",
                        "__init__.py",
                        "CHANGELOG.md",
                    ],
                    "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
                },
            ],
            [
                "@semantic-release/github",
                {"assets": ["./dist/*.whl", "./dist/*.tar.gz"]},
            ],
        ]
    )
    return config


def get_javascript_config():
    """Get JavaScript-specific semantic-release configuration."""
    config = get_base_config()
    config["plugins"].extend(
        [
            "@semantic-release/npm",
            ["@semantic-release/changelog", {"changelogFile": "CHANGELOG.md"}],
            [
                "@semantic-release/git",
                {
                    "assets": ["package.json", "package-lock.json", "CHANGELOG.md"],
                    "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
                },
            ],
            "@semantic-release/github",
        ]
    )
    return config


def get_typescript_config():
    """Get TypeScript-specific semantic-release configuration."""
    config = get_base_config()
    config["plugins"].extend(
        [
            "@semantic-release/npm",
            ["@semantic-release/changelog", {"changelogFile": "CHANGELOG.md"}],
            [
                "@semantic-release/git",
                {
                    "assets": ["package.json", "package-lock.json", "CHANGELOG.md"],
                    "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
                },
            ],
            ["@semantic-release/github", {"assets": ["./artifacts/*.tgz"]}],
        ]
    )
    return config


def main():
    """Main function to create semantic-release configuration."""
    if len(sys.argv) != 2:
        print("Error: Language parameter required", file=sys.stderr)
        print(f"Usage: {sys.argv[0]} <language>", file=sys.stderr)
        sys.exit(1)

    language = sys.argv[1].lower()

    print(f"Creating semantic-release config for {language} project...")

    # Language-specific configuration mapping
    config_map = {
        "rust": get_rust_config,
        "go": get_go_config,
        "python": get_python_config,
        "javascript": get_javascript_config,
        "typescript": get_typescript_config,
    }

    if language not in config_map:
        print(f"Error: Unsupported language: {language}", file=sys.stderr)
        print(f"Supported languages: {', '.join(config_map.keys())}", file=sys.stderr)
        sys.exit(1)

    # Generate configuration
    config = config_map[language]()

    # Write to .releaserc.json
    try:
        with open(".releaserc.json", "w") as f:
            json.dump(config, f, indent=2)

        print(f"Semantic-release config created successfully for {language}")

    except IOError as e:
        print(f"Error writing .releaserc.json: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
