# file: .claude/skills/parallel-sweep-impl/scripts/test_rebase.py
# version: 1.0.0
# guid: 6b7c8d9e-0f1a-2b3c-4d5e-6f7a8b9c0d1e

"""Unit tests for rebase.py.

These tests use real local git fixtures (same pattern as test_dispatch.py)
because rebase semantics are too git-specific to be meaningfully mocked.
A "remote" is just another local directory we set up as the upstream.

Step 5 covers only the clean rebase outcomes:
- CLEAN: sibling has commits not in main; rebase succeeds
- UP_TO_DATE: sibling already has all of main; no-op
- DIRTY_TREE: refused — uncommitted changes
- FETCH_FAILED: network/auth error during fetch

The CONFLICT outcome is exercised in steps 6/7's tests for the resolver
subagents, where the conflict is the whole point.

Run from this directory:
    python3 -m unittest test_rebase.py -v
"""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path

import rebase


def _git(cwd: Path, *args: str, check: bool = True) -> str:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        check=check,
        capture_output=True,
        text=True,
    ).stdout


def _commit(repo: Path, filename: str, content: str, message: str) -> str:
    """Create or overwrite a file, commit it, return the new HEAD sha."""
    (repo / filename).write_text(content)
    _git(repo, "add", filename)
    _git(repo, "commit", "-m", message)
    return _git(repo, "rev-parse", "HEAD").strip()


def _setup_remote_and_clone(tmp: Path) -> tuple[Path, Path]:
    """Create a bare 'remote' repo + a clone of it. Returns (remote, clone)."""
    remote = tmp / "remote.git"
    subprocess.run(
        ["git", "init", "--bare", "-b", "main", str(remote)],
        check=True,
        capture_output=True,
    )
    clone = tmp / "clone"
    subprocess.run(
        ["git", "clone", str(remote), str(clone)], check=True, capture_output=True
    )
    _git(clone, "config", "user.email", "t@e")
    _git(clone, "config", "user.name", "t")
    _commit(clone, "README.md", "init\n", "init")
    _git(clone, "push", "origin", "main")
    return remote, clone


class FetchMainTests(unittest.TestCase):
    def test_returns_true_on_successful_fetch(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            _, clone = _setup_remote_and_clone(tmp)
            self.assertTrue(rebase.fetch_main(clone))

    def test_returns_false_on_bad_remote(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            _, clone = _setup_remote_and_clone(tmp)
            # Replace origin with a non-existent path.
            _git(clone, "remote", "set-url", "origin", str(tmp / "does-not-exist"))
            self.assertFalse(rebase.fetch_main(clone))


class RebaseOntoMainTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_ctx = tempfile.TemporaryDirectory()
        self.tmp = Path(self.tmp_ctx.name)
        self.remote, self.coordinator_clone = _setup_remote_and_clone(self.tmp)
        # Worktree: a separate clone simulating the per-task worktree.
        # (In production it'd be a real git worktree of the main checkout,
        # but a fresh clone is simpler to set up and exercises the same
        # rebase semantics.)
        self.worktree = self.tmp / "worktree"
        subprocess.run(
            ["git", "clone", str(self.remote), str(self.worktree)],
            check=True,
            capture_output=True,
        )
        _git(self.worktree, "config", "user.email", "t@e")
        _git(self.worktree, "config", "user.name", "t")
        # Branch the worktree onto a feature branch with one commit.
        _git(self.worktree, "checkout", "-b", "feat/x")
        _commit(self.worktree, "feat.txt", "feature\n", "feat: add feature")

    def tearDown(self) -> None:
        self.tmp_ctx.cleanup()

    def _advance_main(self, filename: str = "advance.txt") -> None:
        """Push a new commit on remote main from the coordinator's clone."""
        _git(self.coordinator_clone, "checkout", "main")
        _commit(self.coordinator_clone, filename, "advance\n", "feat: advance main")
        _git(self.coordinator_clone, "push", "origin", "main")

    def test_clean_rebase(self) -> None:
        self._advance_main()
        result = rebase.rebase_onto_main(self.worktree)
        self.assertEqual(result.outcome, rebase.RebaseOutcome.CLEAN)
        self.assertIn("rebased cleanly", result.detail)
        # Worktree HEAD should now have main's new commit as its parent.
        log = _git(self.worktree, "log", "--oneline", "-3")
        self.assertIn("advance", log)
        self.assertIn("feat: add feature", log)

    def test_up_to_date_when_no_new_main(self) -> None:
        # Worktree's branch IS main + one feature commit. After fetch, there's
        # nothing new to rebase onto.
        # First merge the feature branch into main so the symmetric-difference is 0.
        _git(self.worktree, "checkout", "main")
        _git(self.worktree, "merge", "--ff-only", "feat/x")
        _git(self.worktree, "push", "origin", "main")
        _git(self.worktree, "checkout", "feat/x")
        result = rebase.rebase_onto_main(self.worktree)
        self.assertEqual(result.outcome, rebase.RebaseOutcome.UP_TO_DATE)

    def test_dirty_tree_refused(self) -> None:
        # Leave a tracked-file modification uncommitted.
        (self.worktree / "feat.txt").write_text("dirty\n")
        result = rebase.rebase_onto_main(self.worktree)
        self.assertEqual(result.outcome, rebase.RebaseOutcome.DIRTY_TREE)
        self.assertIn("uncommitted", result.detail)

    def test_dirty_tree_refused_for_untracked_file(self) -> None:
        (self.worktree / "untracked.txt").write_text("oops\n")
        result = rebase.rebase_onto_main(self.worktree)
        self.assertEqual(result.outcome, rebase.RebaseOutcome.DIRTY_TREE)

    def test_fetch_failed_returns_specific_outcome(self) -> None:
        _git(self.worktree, "remote", "set-url", "origin", str(self.tmp / "nope"))
        result = rebase.rebase_onto_main(self.worktree)
        self.assertEqual(result.outcome, rebase.RebaseOutcome.FETCH_FAILED)
        self.assertIn("git fetch", result.detail)


class RebaseSiblingsTests(unittest.TestCase):
    """Batch test: process multiple siblings, verify per-sibling outcomes."""

    def setUp(self) -> None:
        self.tmp_ctx = tempfile.TemporaryDirectory()
        self.tmp = Path(self.tmp_ctx.name)
        self.remote, self.coordinator_clone = _setup_remote_and_clone(self.tmp)

        # Two siblings, each on its own feature branch.
        self.siblings: list[tuple[str, Path]] = []
        for slug in ("task-a", "task-b"):
            wt = self.tmp / f"wt-{slug}"
            subprocess.run(
                ["git", "clone", str(self.remote), str(wt)],
                check=True,
                capture_output=True,
            )
            _git(wt, "config", "user.email", "t@e")
            _git(wt, "config", "user.name", "t")
            _git(wt, "checkout", "-b", f"feat/{slug}")
            _commit(wt, f"{slug}.txt", f"{slug}\n", f"feat: {slug}")
            self.siblings.append((slug, wt))

        # Advance main so all siblings have something to rebase onto.
        _git(self.coordinator_clone, "checkout", "main")
        _commit(self.coordinator_clone, "advance.txt", "x\n", "feat: advance")
        _git(self.coordinator_clone, "push", "origin", "main")

    def tearDown(self) -> None:
        self.tmp_ctx.cleanup()

    def test_processes_all_siblings_with_clean_outcome(self) -> None:
        results = rebase.rebase_siblings(self.siblings)
        self.assertEqual(len(results), 2)
        self.assertEqual([r.slug for r in results], ["task-a", "task-b"])
        for r in results:
            self.assertEqual(r.outcome, rebase.RebaseOutcome.CLEAN)

    def test_one_sibling_failure_does_not_block_others(self) -> None:
        # Dirty the first sibling to force DIRTY_TREE; second should still proceed.
        slug_a, wt_a = self.siblings[0]
        (wt_a / "feat-a.txt").write_text("dirty\n")
        results = rebase.rebase_siblings(self.siblings)
        self.assertEqual(results[0].outcome, rebase.RebaseOutcome.DIRTY_TREE)
        self.assertEqual(results[1].outcome, rebase.RebaseOutcome.CLEAN)


if __name__ == "__main__":
    unittest.main()
