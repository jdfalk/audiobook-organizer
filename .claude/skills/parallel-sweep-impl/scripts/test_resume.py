# file: .claude/skills/parallel-sweep-impl/scripts/test_resume.py
# version: 1.0.0
# guid: 4e5f6a7b-8c9d-0e1f-2a3b-4c5d6e7f8a9b

"""Unit tests for resume.py.

Run from this directory:
    python3 -m unittest test_resume.py -v
"""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path

import resume
import state as state_mod


def _git(cwd: Path, *args: str, check: bool = True) -> str:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        check=check,
        capture_output=True,
        text=True,
    ).stdout


def _setup_state_with_worktrees(
    tmp: Path, *, slugs: list[str]
) -> tuple[state_mod.State, dict[str, Path]]:
    """Build a state file with N tasks, each pointing at a real local repo
    that simulates a worktree (one clone per slug).

    Returns (state, {slug: worktree_path}).
    """
    state_dir = tmp / ".claude" / "state"

    # Bare remote with main branch + advance commit, so origin/main exists.
    remote = tmp / "remote.git"
    subprocess.run(
        ["git", "init", "--bare", "-b", "main", str(remote)],
        check=True,
        capture_output=True,
    )
    seed = tmp / "seed"
    subprocess.run(
        ["git", "clone", str(remote), str(seed)], check=True, capture_output=True
    )
    _git(seed, "config", "user.email", "t@e")
    _git(seed, "config", "user.name", "t")
    (seed / "README.md").write_text("init\n")
    _git(seed, "add", "README.md")
    _git(seed, "commit", "-m", "init")
    _git(seed, "push", "origin", "main")

    worktrees: dict[str, Path] = {}
    for slug in slugs:
        wt = tmp / f"wt-{slug}"
        subprocess.run(
            ["git", "clone", str(remote), str(wt)], check=True, capture_output=True
        )
        _git(wt, "config", "user.email", "t@e")
        _git(wt, "config", "user.name", "t")
        # Put the worktree on its task branch with an in-progress commit,
        # simulating a child agent that committed before being killed.
        _git(wt, "checkout", "-b", f"refactor/{slug}")
        (wt / f"{slug}.txt").write_text(f"{slug} in-progress\n")
        _git(wt, "add", f"{slug}.txt")
        _git(wt, "commit", "-m", f"feat({slug}): wip")
        worktrees[slug] = wt

    state = state_mod.State.create(
        state_dir,
        "2026-04-25-1200-test-resume",
        "test prompt",
        [
            {"slug": slug, "description": f"task {slug}", "model": "haiku"}
            for slug in slugs
        ],
    )
    # Record worktree paths and put each task in_progress.
    for slug, wt in worktrees.items():
        state.update_task(
            slug,
            worktreePath=str(wt),
            status="in_progress",
            agentID=f"agt-{slug}",
        )
    state.set_status("paused")
    return state, worktrees


class LoadForResumeTests(unittest.TestCase):
    def test_classifies_tasks_by_status(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, _ = _setup_state_with_worktrees(Path(td), slugs=["a", "b", "c"])
            # Mix statuses.
            state.update_task("a", status="merged")
            state.update_task("b", status="rebase_blocked")
            # c stays in_progress.
            ctx = resume.load_for_resume(state.state_dir, state.data["runID"])
        self.assertEqual(ctx.in_flight_slugs, ["c"])
        self.assertEqual(ctx.completed_slugs, ["a"])
        self.assertEqual(ctx.skipped_slugs, ["b"])

    def test_refuses_when_status_is_running(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, _ = _setup_state_with_worktrees(Path(td), slugs=["a"])
            state.set_status("running")
            with self.assertRaises(resume.ResumeRefused):
                resume.load_for_resume(state.state_dir, state.data["runID"])

    def test_force_overrides_running_refusal(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, _ = _setup_state_with_worktrees(Path(td), slugs=["a"])
            state.set_status("running")
            ctx = resume.load_for_resume(
                state.state_dir, state.data["runID"], force=True
            )
            self.assertIn("a", ctx.in_flight_slugs)


class ResetInFlightTests(unittest.TestCase):
    def test_resets_worktree_to_main_and_flips_pending(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, worktrees = _setup_state_with_worktrees(Path(td), slugs=["a", "b"])
            ctx = resume.load_for_resume(state.state_dir, state.data["runID"])
            results = resume.reset_in_flight(ctx, main_ref="origin/main")

            self.assertEqual(len(results), 2)
            for r in results:
                self.assertTrue(r.reset_succeeded, r.detail)
                self.assertEqual(r.previous_status, "in_progress")
            for slug in ("a", "b"):
                self.assertEqual(ctx.state.task(slug)["status"], "pending")
                self.assertIsNone(ctx.state.task(slug)["agentID"])
            for slug, wt in worktrees.items():
                log = _git(wt, "log", "--oneline", "-3")
                self.assertNotIn("wip", log, f"worktree for {slug} not reset to main")

    def test_handles_task_with_no_worktree_path(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, _ = _setup_state_with_worktrees(Path(td), slugs=["a"])
            # Simulate a task that was dispatched but never had a worktree
            # (e.g., dispatch failed before worktree creation).
            state.update_task("a", worktreePath=None, status="dispatched")
            ctx = resume.load_for_resume(state.state_dir, state.data["runID"])
            results = resume.reset_in_flight(ctx)

        self.assertEqual(len(results), 1)
        self.assertTrue(results[0].reset_succeeded)
        self.assertIn("no worktree", results[0].detail)
        self.assertEqual(ctx.state.task("a")["status"], "pending")

    def test_failed_reset_records_error_and_continues(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, worktrees = _setup_state_with_worktrees(Path(td), slugs=["a", "b"])
            # Break worktree a's remote so fetch fails. b should still reset.
            _git(worktrees["a"], "remote", "set-url", "origin", str(Path(td) / "nope"))
            ctx = resume.load_for_resume(state.state_dir, state.data["runID"])
            results = resume.reset_in_flight(ctx)

        self.assertEqual(len(results), 2)
        # Find each result by slug.
        by_slug = {r.slug: r for r in results}
        self.assertFalse(by_slug["a"].reset_succeeded)
        self.assertTrue(by_slug["b"].reset_succeeded)
        # Task a should still be in_progress (not flipped); error recorded.
        self.assertEqual(ctx.state.task("a")["status"], "in_progress")
        self.assertGreater(len(ctx.state.task("a")["errors"]), 0)
        # Task b should be pending.
        self.assertEqual(ctx.state.task("b")["status"], "pending")

    def test_aborts_in_progress_rebase_before_reset(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, worktrees = _setup_state_with_worktrees(Path(td), slugs=["a"])
            # Force the worktree into a mid-rebase state by triggering a
            # conflict against an unrelated commit.
            wt = worktrees["a"]
            (wt / "README.md").write_text("CONFLICT\n")
            _git(wt, "add", "README.md")
            _git(wt, "commit", "-m", "wip conflict")
            # Build a divergent main: a separate commit on a fake "main".
            _git(wt, "checkout", "main")
            (wt / "README.md").write_text("OTHER\n")
            _git(wt, "add", "README.md")
            _git(wt, "commit", "-m", "main divergent")
            _git(wt, "checkout", "refactor/a")
            subprocess.run(
                ["git", "-C", str(wt), "rebase", "main"],
                capture_output=True,
                text=True,
            )
            # Now should be mid-rebase.
            ctx = resume.load_for_resume(state.state_dir, state.data["runID"])
            results = resume.reset_in_flight(ctx)

            self.assertTrue(results[0].reset_succeeded, results[0].detail)
            self.assertEqual(_git(worktrees["a"], "status", "--porcelain"), "")


class MarkResumedTests(unittest.TestCase):
    def test_flips_status_to_running(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state, _ = _setup_state_with_worktrees(Path(td), slugs=["a"])
            ctx = resume.load_for_resume(state.state_dir, state.data["runID"])
            resume.mark_resumed(ctx)
        self.assertEqual(ctx.state.data["status"], "running")


if __name__ == "__main__":
    unittest.main()
