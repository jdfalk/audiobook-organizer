# file: .claude/skills/parallel-sweep-impl/scripts/test_dispatch.py
# version: 1.0.0
# guid: 1c2d3e4f-5a6b-7c8d-9e0f-1a2b3c4d5e6f

"""Unit tests for dispatch.py.

Run from this directory:
    python3 -m unittest test_dispatch.py -v
"""

from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path

import dispatch


def _git(cwd: Path, *args: str) -> str:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        check=True,
        capture_output=True,
        text=True,
    ).stdout


def _init_repo(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)
    _git(path, "init", "-b", "main")
    _git(path, "config", "user.email", "test@example.com")
    _git(path, "config", "user.name", "Test")
    (path / "README.md").write_text("init\n")
    _git(path, "add", "README.md")
    _git(path, "commit", "-m", "init")


class RenderSettingsTests(unittest.TestCase):
    def test_render_includes_absolute_worktree_path(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            wt = Path(td) / "worktrees" / "task-a"
            wt.mkdir(parents=True)
            data = dispatch.render_worktree_settings(wt)
        self.assertIn("PreToolUse", data["hooks"])
        cmd = data["hooks"]["PreToolUse"][0]["hooks"][0]["command"]
        self.assertIn(str(wt.resolve()), cmd)
        self.assertIn("BLOCKED", cmd)
        self.assertIn("Edit|Write", data["hooks"]["PreToolUse"][0]["matcher"])

    def test_write_creates_settings_file(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            wt = Path(td) / "worktrees" / "task-a"
            wt.mkdir(parents=True)
            path = dispatch.write_worktree_settings(wt)
            expected = wt.resolve() / ".claude" / "settings.local.json"
            self.assertEqual(path.resolve(), expected)
            self.assertTrue(path.exists())
            loaded = json.loads(path.read_text())
            self.assertEqual(loaded, dispatch.render_worktree_settings(wt))

    def test_render_handles_path_with_spaces(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            wt = Path(td) / "with spaces" / "task-a"
            wt.mkdir(parents=True)
            data = dispatch.render_worktree_settings(wt)
            cmd = data["hooks"]["PreToolUse"][0]["hooks"][0]["command"]
            # The path should be embedded verbatim in the command's "root="
            # variable. The hook script wraps it in double quotes, so spaces
            # are safe at runtime — what we want to verify here is that the
            # absolute path made it in.
            self.assertIn(str(wt.resolve()), cmd)


class CrossCheckTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.main_repo = self.root / "main"
        self.worktree = self.root / "worktree"
        self.sibling = self.root / "sibling"
        for repo in (self.main_repo, self.worktree, self.sibling):
            _init_repo(repo)

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def test_clean_when_no_unexpected_changes(self) -> None:
        # Modify only the expected worktree.
        (self.worktree / "expected.txt").write_text("ok\n")
        violations = dispatch.cross_check_isolation(
            expected_worktree=self.worktree,
            sibling_paths=[self.main_repo, self.sibling],
        )
        self.assertEqual(violations, [])

    def test_violation_when_sibling_modified(self) -> None:
        (self.worktree / "expected.txt").write_text("ok\n")
        # Simulate a child agent leaking into a sibling worktree.
        (self.sibling / "leaked.go").write_text("package x\n")
        violations = dispatch.cross_check_isolation(
            expected_worktree=self.worktree,
            sibling_paths=[self.main_repo, self.sibling],
        )
        self.assertEqual(len(violations), 1)
        self.assertIn(str(self.sibling.resolve()), violations[0])
        self.assertIn("leaked.go", violations[0])

    def test_violation_when_main_checkout_modified(self) -> None:
        # The most common defect: child wrote into main instead of worktree.
        (self.main_repo / "wrong.txt").write_text("oops\n")
        violations = dispatch.cross_check_isolation(
            expected_worktree=self.worktree,
            sibling_paths=[self.main_repo, self.sibling],
        )
        self.assertEqual(len(violations), 1)
        self.assertIn("wrong.txt", violations[0])

    def test_self_path_in_siblings_is_ignored(self) -> None:
        # If the caller accidentally passes the expected worktree among
        # siblings, it should be a no-op rather than a false positive.
        (self.worktree / "expected.txt").write_text("ok\n")
        violations = dispatch.cross_check_isolation(
            expected_worktree=self.worktree,
            sibling_paths=[self.worktree, self.main_repo],
        )
        self.assertEqual(violations, [])

    def test_non_repo_sibling_yields_violation(self) -> None:
        not_a_repo = self.root / "not-a-repo"
        not_a_repo.mkdir()
        violations = dispatch.cross_check_isolation(
            expected_worktree=self.worktree,
            sibling_paths=[not_a_repo],
        )
        self.assertEqual(len(violations), 1)
        self.assertIn("could not check", violations[0])

    def test_staged_changes_count_too(self) -> None:
        # A child that staged a write outside its worktree but didn't commit
        # would still be a violation.
        leak = self.sibling / "leaked.go"
        leak.write_text("package x\n")
        _git(self.sibling, "add", "leaked.go")
        violations = dispatch.cross_check_isolation(
            expected_worktree=self.worktree,
            sibling_paths=[self.sibling],
        )
        self.assertEqual(len(violations), 1)


class CLITests(unittest.TestCase):
    def test_render_subcommand(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            wt = Path(td) / "task-a"
            wt.mkdir()
            rc = dispatch.main(["render", str(wt)])
            self.assertEqual(rc, 0)

    def test_check_subcommand_clean_returns_zero(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            wt = root / "worktree"
            sib = root / "sibling"
            for r in (wt, sib):
                _init_repo(r)
            (wt / "ok.txt").write_text("ok\n")
            rc = dispatch.main(["check", "--expected", str(wt), "--sibling", str(sib)])
            self.assertEqual(rc, 0)

    def test_check_subcommand_violation_returns_nonzero(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            wt = root / "worktree"
            sib = root / "sibling"
            for r in (wt, sib):
                _init_repo(r)
            (sib / "bad.txt").write_text("bad\n")
            rc = dispatch.main(["check", "--expected", str(wt), "--sibling", str(sib)])
            self.assertEqual(rc, 1)


if __name__ == "__main__":
    unittest.main()
