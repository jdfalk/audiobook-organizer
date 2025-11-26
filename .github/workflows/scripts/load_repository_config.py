#!/usr/bin/env python3
# file: .github/workflows/scripts/load_repository_config.py
# version: 1.0.0
# guid: load-repo-config-audiobook-organizer-001

"""Load repository configuration and emit workflow outputs."""

from __future__ import annotations

import json
import os
from pathlib import Path

try:
    import yaml
except ImportError:  # pragma: no cover
    yaml = None


def emit_output(name: str, value: str) -> None:
    """Emit GitHub Actions output."""
    output_path = os.environ.get("GITHUB_OUTPUT")
    if output_path:
        with open(output_path, "a", encoding="utf-8") as handle:
            handle.write(f"{name}={value}\n")


def main() -> None:
    """Main entry point."""
    config_file = Path(os.environ.get("CONFIG_FILE", ".github/repository-config.yml"))

    if not config_file.exists() or yaml is None:
        emit_output("has-config", "false")
        emit_output("config", "{}")
        return

    try:
        data = yaml.safe_load(config_file.read_text()) or {}
    except Exception as exc:  # pragma: no cover
        print(f"::warning::Unable to parse {config_file}: {exc}")
        emit_output("has-config", "false")
        emit_output("config", "{}")
        return

    emit_output("has-config", "true")
    emit_output("config", json.dumps(data, separators=(",", ":")))


if __name__ == "__main__":
    main()
