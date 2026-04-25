# file: .claude/skills/parallel-sweep-impl/scripts/dispatch.py
# version: 1.0.0
# guid: 9b0c1d2e-3f4a-5b6c-7d8e-9f0a1b2c3d4e

"""Dispatch helpers for /parallel-sweep.

Two responsibilities:

1. Render the per-worktree ``.claude/settings.local.json`` containing the
   PreToolUse hook that *would* block Edit/Write outside the worktree's
   absolute path. **Important:** the 2026-04-25 hook spike (see
   ``docs/superpowers/notes/2026-04-25-parallel-sweep-hook-spike.md``) found
   that sub-agents inherit the parent session's hook config and do NOT pick
   up project-scope hooks from their working directory. The settings file is
   kept anyway as cheap forward-compatible decoration — if Claude Code ever
   changes sub-agent hook inheritance, the file is already in place.

2. Run the post-hoc cross-check: after a child reports done, list every file
   that has changed (working tree + index) in EVERY repo path the coordinator
   knows about, and flag any change that landed outside the child's own
   worktree. **This is the load-bearing worktree-isolation barrier** — the
   only enforcement happens here, after the fact, so the coordinator MUST
   call cross_check_isolation before opening a PR.

Why the two halves are split here instead of at call sites: rendering the
settings file is a pure data transform that's easy to unit-test, and the
cross-check is a concrete shell-out that has to be tested against real git
state. Keeping both in one file lets the coordinator import a single module.
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path
from typing import Iterable

# The PreToolUse hook command. The {worktree_root} placeholder is filled when
# the settings file is rendered; the result is a one-liner shell command that
# bash -c will evaluate on every Edit/Write call.
#
# Why bash and jq: the hook receives the tool input as the env var TOOL_INPUT
# (a JSON blob); jq is the cleanest way to extract .file_path. If the path is
# missing or doesn't start with the worktree root, exit non-zero — Claude
# treats a non-zero PreToolUse exit as a block.
_HOOK_COMMAND_TEMPLATE = (
    "file=$(echo \"$TOOL_INPUT\" | jq -r '.file_path // empty'); "
    'root="{worktree_root}"; '
    '[[ "$file" == "$root"/* || -z "$file" ]] && exit 0; '
    'echo "BLOCKED: $file is outside this worktree ($root)" >&2; '
    "exit 1"
)


def render_worktree_settings(worktree_root: Path) -> dict:
    """Return the dict that should be written to <worktree>/.claude/settings.local.json.

    The settings file is per-worktree and gitignored. It installs a PreToolUse
    hook scoped to that worktree's absolute path.
    """
    abs_root = str(worktree_root.resolve())
    return {
        "hooks": {
            "PreToolUse": [
                {
                    "matcher": "Edit|Write",
                    "hooks": [
                        {
                            "type": "command",
                            "command": _HOOK_COMMAND_TEMPLATE.format(
                                worktree_root=abs_root
                            ),
                        }
                    ],
                }
            ]
        }
    }


def write_worktree_settings(worktree_root: Path) -> Path:
    """Write the settings.local.json into <worktree>/.claude/ and return its path."""
    abs_root = worktree_root.resolve()
    settings_dir = abs_root / ".claude"
    settings_dir.mkdir(parents=True, exist_ok=True)
    settings_path = settings_dir / "settings.local.json"
    settings_path.write_text(
        json.dumps(render_worktree_settings(abs_root), indent=2) + "\n",
        encoding="utf-8",
    )
    return settings_path


def list_changed_files(repo_path: Path) -> list[str]:
    """Return the porcelain status entries for repo_path, one per changed file.

    Includes both staged and unstaged changes plus untracked files. Returns an
    empty list if there are no changes. Raises CalledProcessError if git fails
    (e.g. the path isn't a repo) so the coordinator can decide how to handle.
    """
    result = subprocess.run(
        ["git", "-C", str(repo_path), "status", "--porcelain"],
        check=True,
        capture_output=True,
        text=True,
    )
    return [line[3:] for line in result.stdout.splitlines() if line]


def cross_check_isolation(
    *,
    expected_worktree: Path,
    sibling_paths: Iterable[Path],
) -> list[str]:
    """Verify the child only modified files inside expected_worktree.

    Returns a list of human-readable violation strings. Empty list = clean.

    Concretely: changes inside ``expected_worktree`` are fine (that's where
    the child was supposed to work). Changes anywhere in ``sibling_paths``
    (the main checkout + every other worktree) are violations — they mean
    the child wrote outside its sandbox.

    The caller is the coordinator; on a non-empty result it should mark the
    task ``failed`` and skip merging. See coordinator-prompt.md hard
    constraint #5.
    """
    violations: list[str] = []
    for sibling in sibling_paths:
        sibling_abs = sibling.resolve()
        if sibling_abs == expected_worktree.resolve():
            # Same path — not a sibling, skip.
            continue
        try:
            changed = list_changed_files(sibling_abs)
        except subprocess.CalledProcessError as exc:
            violations.append(
                f"could not check {sibling_abs}: git status failed ({exc})"
            )
            continue
        if changed:
            violations.append(
                f"{sibling_abs}: {len(changed)} unexpected file change(s) — "
                f"first: {changed[0]}"
            )
    return violations


def main(argv: list[str] | None = None) -> int:
    """CLI entry point for ad-hoc invocations.

    Subcommands:
      render   — print the rendered settings.local.json for a given worktree
      write    — write the settings.local.json into a worktree
      check    — run the cross-check; print violations and exit non-zero on any
    """
    import argparse

    parser = argparse.ArgumentParser(prog="dispatch.py")
    sub = parser.add_subparsers(dest="cmd", required=True)

    p_render = sub.add_parser("render", help="print rendered settings.local.json")
    p_render.add_argument("worktree", type=Path)

    p_write = sub.add_parser("write", help="write settings.local.json into a worktree")
    p_write.add_argument("worktree", type=Path)

    p_check = sub.add_parser("check", help="cross-check worktree isolation")
    p_check.add_argument("--expected", type=Path, required=True)
    p_check.add_argument("--sibling", type=Path, action="append", required=True)

    args = parser.parse_args(argv)
    if args.cmd == "render":
        print(json.dumps(render_worktree_settings(args.worktree), indent=2))
        return 0
    if args.cmd == "write":
        path = write_worktree_settings(args.worktree)
        print(f"wrote {path}")
        return 0
    if args.cmd == "check":
        violations = cross_check_isolation(
            expected_worktree=args.expected,
            sibling_paths=args.sibling,
        )
        if violations:
            for v in violations:
                print(f"VIOLATION: {v}")
            return 1
        print("clean")
        return 0
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
