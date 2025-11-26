#!/usr/bin/env python3
# file: .github/workflows/scripts/workflow_common.py
# version: 1.0.0
# guid: 6310ec6e-4513-4e0e-9f9b-5a100a305266

"""Shared helpers for GitHub workflow scripts."""

from __future__ import annotations

import json
import os
from collections.abc import Iterable
from pathlib import Path
from typing import Any

_CONFIG_CACHE: dict[str, Any] | None = None


def append_to_file(path_env: str, content: str) -> None:
    """Append content to a GitHub Actions environment file."""
    file_path = os.environ.get(path_env)
    if not file_path:
        return
    path = Path(file_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(content)


def write_output(name: str, value: str) -> None:
    """Write an output value for downstream steps."""
    append_to_file("GITHUB_OUTPUT", f"{name}={value}\n")


def append_summary(text: str) -> None:
    """Append Markdown text to the GitHub Actions step summary."""
    append_to_file("GITHUB_STEP_SUMMARY", text)


def append_summary_line(line: str) -> None:
    """Append a single line to the GitHub Actions step summary."""
    append_summary(f"{line}\n")


def format_summary_table(rows: Iterable[tuple[str, str]]) -> str:
    """Return a Markdown table for summary rows."""
    table = ["| Item | Value |", "| --- | --- |"]
    table.extend(f"| {key} | {value} |" for key, value in rows)
    return "\n".join(table)


def log_notice(message: str) -> None:
    """Emit a GitHub Actions notice message."""
    print(f"::notice::{message}")


def log_warning(message: str) -> None:
    """Emit a GitHub Actions warning message."""
    print(f"::warning::{message}")


def log_error(message: str) -> None:
    """Emit a GitHub Actions error message."""
    print(f"::error::{message}")


def get_repository_config() -> dict[str, Any]:
    """Return cached repository configuration parsed from environment."""
    global _CONFIG_CACHE  # noqa: PLW0603
    if _CONFIG_CACHE is not None:
        return _CONFIG_CACHE

    raw = os.environ.get("REPOSITORY_CONFIG")
    if not raw:
        _CONFIG_CACHE = {}
        return _CONFIG_CACHE

    try:
        _CONFIG_CACHE = json.loads(raw)
    except json.JSONDecodeError:
        log_warning(
            "Unable to parse REPOSITORY_CONFIG; falling back to defaults",
        )
        _CONFIG_CACHE = {}
    return _CONFIG_CACHE


def reset_repository_config(cache: dict[str, Any] | None = None) -> None:
    """Reset the cached repository configuration (useful for tests)."""
    global _CONFIG_CACHE  # noqa: PLW0603
    _CONFIG_CACHE = cache


def config_path(default: Any, *path: str) -> Any:
    """Navigate configuration keys and return a value or default."""
    current: Any = get_repository_config()
    for key in path:
        if not isinstance(current, dict) or key not in current:
            return default
        current = current[key]
    return current


def registry_enabled(registry: str) -> bool:
    """Return True if the given package registry is enabled."""
    registries = config_path({}, "packages", "registries")
    if not isinstance(registries, dict):
        return False
    return bool(registries.get(registry, False))


def build_release_summary(context: dict[str, Any]) -> str:
    """Generate a Markdown summary for release job results."""
    components = context.get("components", {})
    lines = [
        "# 🚀 Release Build Results",
        "",
        f"**Project Type:** {context.get('primary_language', 'unknown')}",
        f"**Build Target:** {context.get('build_target', 'all')}",
        f"**Release Tag:** {context.get('release_tag', 'n/a')}",
        f"**Release Strategy:** {context.get('release_strategy', 'n/a')}",
        f"**Branch:** {context.get('branch', 'n/a')}",
        "",
        "| Component | Status |",
        "|-----------|--------|",
    ]
    for component, status in components.items():
        lines.append(f"| {component} | {status} |")

    if context.get("release_created"):
        lines.extend(
            [
                "",
                f"🎉 **Release created: {context.get('release_tag', 'n/a')}**",
            ]
        )
        if context.get("auto_prerelease"):
            lines.append("⚠️ **Pre-release** - for testing purposes")
        elif context.get("auto_draft"):
            lines.append("📝 **Draft release** - review before publishing")
        else:
            lines.append("🚀 **Stable release** - ready for production")

    lines.append("")
    return "\n".join(lines)
