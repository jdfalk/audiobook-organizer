# file: .claude/skills/parallel-sweep-impl/scripts/test_pr_merge.py
# version: 1.0.0
# guid: 4f5a6b7c-8d9e-0f1a-2b3c-4d5e6f7a8b9c

"""Unit tests for pr_merge.py.

Mocks subprocess.run so the tests don't need real gh / GitHub state. The
trade-off: these tests verify the helper's CONTROL FLOW (which gates fire,
in what order, with what arguments), not whether `gh pr merge` actually
merges. The real verification is the live coordinator smoke in step 5.

Run from this directory:
    python3 -m unittest test_pr_merge.py -v
"""

from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

import pr_merge


@dataclass
class FakeRunner:
    """Records subprocess calls and returns scripted responses.

    Each entry in ``responses`` is a (predicate, result) pair. The first
    predicate that matches a given call provides the result.
    """

    responses: list[tuple[Callable[[list[str]], bool], subprocess.CompletedProcess]] = (
        field(default_factory=list)
    )
    calls: list[list[str]] = field(default_factory=list)

    def __call__(self, cmd, **kwargs) -> subprocess.CompletedProcess:
        self.calls.append(list(cmd))
        for pred, result in self.responses:
            if pred(list(cmd)):
                return result
        # Default: success with empty output. Lets tests omit responses for
        # commands they don't care about (e.g. gh pr ready).
        return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")


def _ok(stdout: str = "", returncode: int = 0) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(
        args=[], returncode=returncode, stdout=stdout, stderr=""
    )


def _matches(*needles: str) -> Callable[[list[str]], bool]:
    """Return a predicate that matches any cmd containing all of `needles`."""

    def pred(cmd: list[str]) -> bool:
        joined = " ".join(cmd)
        return all(n in joined for n in needles)

    return pred


class FakeRunnerScope:
    """Context manager that swaps in a FakeRunner for the test body."""

    def __init__(self, runner: FakeRunner):
        self.runner = runner

    def __enter__(self) -> FakeRunner:
        pr_merge._set_runner(self.runner)
        # cross_check_isolation runs git via dispatch.py's own subprocess.run
        # (not pr_merge's swappable _run), so we leave dispatch alone in unit
        # tests; the few tests that exercise the integrated merge_task use a
        # real temp repo and only mock pr_merge's runner.
        return self.runner

    def __exit__(self, *exc: Any) -> None:
        pr_merge._reset_runner()


class RunLocalCITests(unittest.TestCase):
    def test_returns_true_on_zero_exit(self) -> None:
        runner = FakeRunner(responses=[(_matches("make", "ci"), _ok(returncode=0))])
        with tempfile.TemporaryDirectory() as td:
            log = Path(td) / "ci.log"
            with FakeRunnerScope(runner):
                ok = pr_merge.run_local_ci(worktree_root=Path(td), log_path=log)
            self.assertTrue(ok)
            self.assertTrue(log.exists())
            # Sanity: the log should record the command we ran.
            self.assertIn("make ci", log.read_text())

    def test_returns_false_on_nonzero_exit(self) -> None:
        runner = FakeRunner(responses=[(_matches("make", "ci"), _ok(returncode=1))])
        with tempfile.TemporaryDirectory() as td:
            log = Path(td) / "ci.log"
            with FakeRunnerScope(runner):
                ok = pr_merge.run_local_ci(worktree_root=Path(td), log_path=log)
        self.assertFalse(ok)


class OpenPRTests(unittest.TestCase):
    def test_parses_pr_number_from_url(self) -> None:
        runner = FakeRunner(
            responses=[
                (
                    _matches("gh", "pr", "create"),
                    _ok(
                        stdout="https://github.com/jdfalk/audiobook-organizer/pull/447\n"
                    ),
                )
            ]
        )
        with tempfile.TemporaryDirectory() as td, FakeRunnerScope(runner):
            n = pr_merge.open_pr(
                worktree_root=Path(td),
                branch="feat/x",
                title="t",
                body="b",
            )
        self.assertEqual(n, 447)

    def test_raises_on_unparseable_output(self) -> None:
        runner = FakeRunner(
            responses=[(_matches("gh", "pr", "create"), _ok(stdout="???\n"))]
        )
        with tempfile.TemporaryDirectory() as td, FakeRunnerScope(runner):
            with self.assertRaises(RuntimeError):
                pr_merge.open_pr(
                    worktree_root=Path(td),
                    branch="feat/x",
                    title="t",
                    body="b",
                )


class PollCITests(unittest.TestCase):
    def _rollup(self, *checks: dict) -> str:
        return json.dumps({"statusCheckRollup": list(checks)})

    def test_returns_green_when_all_succeed(self) -> None:
        runner = FakeRunner(
            responses=[
                (
                    _matches("gh", "pr", "view"),
                    _ok(
                        stdout=self._rollup(
                            {
                                "name": "Build",
                                "status": "COMPLETED",
                                "conclusion": "SUCCESS",
                            },
                            {
                                "name": "Test",
                                "status": "COMPLETED",
                                "conclusion": "SUCCESS",
                            },
                        )
                    ),
                )
            ]
        )
        with FakeRunnerScope(runner):
            res = pr_merge.poll_ci(pr_number=1, sleep=lambda _s: None, now=lambda: 0.0)
        self.assertTrue(res.green)
        self.assertEqual(res.failed_checks, [])
        self.assertFalse(res.timed_out)

    def test_returns_red_with_failed_check_names(self) -> None:
        runner = FakeRunner(
            responses=[
                (
                    _matches("gh", "pr", "view"),
                    _ok(
                        stdout=self._rollup(
                            {
                                "name": "Build",
                                "status": "COMPLETED",
                                "conclusion": "SUCCESS",
                            },
                            {
                                "name": "Test",
                                "status": "COMPLETED",
                                "conclusion": "FAILURE",
                            },
                        )
                    ),
                )
            ]
        )
        with FakeRunnerScope(runner):
            res = pr_merge.poll_ci(pr_number=1, sleep=lambda _s: None, now=lambda: 0.0)
        self.assertFalse(res.green)
        self.assertEqual(res.failed_checks, ["Test"])

    def test_skipped_checks_count_as_success(self) -> None:
        # GitHub workflows often SKIP jobs (e.g., no Go changes -> Go CI skipped).
        # Skipped is not failure.
        runner = FakeRunner(
            responses=[
                (
                    _matches("gh", "pr", "view"),
                    _ok(
                        stdout=self._rollup(
                            {
                                "name": "Go CI",
                                "status": "COMPLETED",
                                "conclusion": "SKIPPED",
                            },
                            {
                                "name": "Python CI",
                                "status": "COMPLETED",
                                "conclusion": "SUCCESS",
                            },
                        )
                    ),
                )
            ]
        )
        with FakeRunnerScope(runner):
            res = pr_merge.poll_ci(pr_number=1, sleep=lambda _s: None, now=lambda: 0.0)
        self.assertTrue(res.green)

    def test_polls_until_complete(self) -> None:
        # First two polls return IN_PROGRESS; third returns SUCCESS.
        rollups = [
            self._rollup({"name": "Build", "status": "IN_PROGRESS"}),
            self._rollup({"name": "Build", "status": "IN_PROGRESS"}),
            self._rollup(
                {"name": "Build", "status": "COMPLETED", "conclusion": "SUCCESS"}
            ),
        ]
        idx = {"i": 0}

        def runner(cmd, **kwargs):
            i = idx["i"]
            idx["i"] += 1
            return _ok(stdout=rollups[i])

        sleep_calls: list[float] = []
        with FakeRunnerScope(FakeRunner()):
            pr_merge._set_runner(runner)
            res = pr_merge.poll_ci(
                pr_number=1,
                poll_interval_s=5,
                sleep=sleep_calls.append,
                now=lambda: 0.0,
            )
        self.assertTrue(res.green)
        self.assertEqual(sleep_calls, [5, 5])  # slept twice between three polls

    def test_returns_timed_out_when_deadline_passes(self) -> None:
        # now() advances by 1000s on each call; deadline is 100s; first poll
        # IN_PROGRESS, then deadline expires.
        runner = FakeRunner(
            responses=[
                (
                    _matches("gh", "pr", "view"),
                    _ok(
                        stdout=self._rollup({"name": "Build", "status": "IN_PROGRESS"})
                    ),
                )
            ]
        )
        clock = {"t": 0.0}

        def now() -> float:
            t = clock["t"]
            clock["t"] += 1000.0
            return t

        with FakeRunnerScope(runner):
            res = pr_merge.poll_ci(
                pr_number=1,
                timeout_s=100,
                sleep=lambda _s: None,
                now=now,
            )
        self.assertTrue(res.timed_out)
        self.assertFalse(res.green)
        self.assertEqual(res.failed_checks, ["Build"])


class MergeTaskTests(unittest.TestCase):
    """End-to-end merge_task tests with a real temp repo + mocked gh."""

    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.worktree = self.root / "worktree"
        self.sibling = self.root / "sibling"
        for repo in (self.worktree, self.sibling):
            repo.mkdir()
            subprocess.run(
                ["git", "-C", str(repo), "init", "-b", "main"],
                check=True,
                capture_output=True,
            )
            subprocess.run(
                ["git", "-C", str(repo), "config", "user.email", "t@e"], check=True
            )
            subprocess.run(
                ["git", "-C", str(repo), "config", "user.name", "t"], check=True
            )
            (repo / "README.md").write_text("init\n")
            subprocess.run(
                ["git", "-C", str(repo), "add", "README.md"],
                check=True,
                capture_output=True,
            )
            subprocess.run(
                ["git", "-C", str(repo), "commit", "-m", "init"],
                check=True,
                capture_output=True,
            )
        self.log = self.root / "ci.log"

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def _run_with(self, runner: FakeRunner) -> pr_merge.TaskOutcome:
        with FakeRunnerScope(runner):
            return pr_merge.merge_task(
                worktree_root=self.worktree,
                branch="feat/x",
                pr_title="t",
                pr_body="b",
                local_ci_log=self.log,
                sibling_paths=[self.sibling],
            )

    def test_returns_failed_when_isolation_violated(self) -> None:
        # Leak a file into the sibling — pre-merge isolation check fails.
        (self.sibling / "leaked.txt").write_text("oops\n")
        outcome = self._run_with(FakeRunner())
        self.assertEqual(outcome.status, "failed")
        self.assertIn("isolation violation", outcome.failure)

    def test_returns_failed_when_local_ci_fails(self) -> None:
        runner = FakeRunner(responses=[(_matches("make", "ci"), _ok(returncode=1))])
        outcome = self._run_with(runner)
        self.assertEqual(outcome.status, "failed")
        self.assertIn("local make ci failed", outcome.failure)

    def test_returns_merged_on_full_happy_path(self) -> None:
        rollup_ok = json.dumps(
            {
                "statusCheckRollup": [
                    {"name": "Build", "status": "COMPLETED", "conclusion": "SUCCESS"}
                ]
            }
        )
        runner = FakeRunner(
            responses=[
                (_matches("make", "ci"), _ok(returncode=0)),
                (_matches("git", "push"), _ok()),
                (
                    _matches("gh", "pr", "create"),
                    _ok(stdout="https://github.com/x/y/pull/123\n"),
                ),
                (_matches("gh", "pr", "view"), _ok(stdout=rollup_ok)),
                (_matches("gh", "pr", "ready"), _ok()),
                (_matches("gh", "pr", "merge"), _ok()),
            ]
        )
        outcome = self._run_with(runner)
        self.assertEqual(outcome.status, "merged")
        self.assertEqual(outcome.pr_number, 123)
        self.assertIsNone(outcome.failure)

    def test_returns_failed_on_red_github_ci(self) -> None:
        rollup_red = json.dumps(
            {
                "statusCheckRollup": [
                    {"name": "Build", "status": "COMPLETED", "conclusion": "FAILURE"}
                ]
            }
        )
        runner = FakeRunner(
            responses=[
                (_matches("make", "ci"), _ok(returncode=0)),
                (_matches("git", "push"), _ok()),
                (
                    _matches("gh", "pr", "create"),
                    _ok(stdout="https://github.com/x/y/pull/124\n"),
                ),
                (_matches("gh", "pr", "view"), _ok(stdout=rollup_red)),
            ]
        )
        outcome = self._run_with(runner)
        self.assertEqual(outcome.status, "failed")
        self.assertEqual(outcome.pr_number, 124)
        self.assertIn("Build", outcome.failure)

    def test_returns_pr_opened_on_admin_merge_failure(self) -> None:
        # CI green but the merge itself fails (e.g., main moved). Outcome
        # should leave the PR open for retry rather than mark it failed.
        rollup_ok = json.dumps(
            {
                "statusCheckRollup": [
                    {"name": "Build", "status": "COMPLETED", "conclusion": "SUCCESS"}
                ]
            }
        )
        base = FakeRunner(
            responses=[
                (_matches("make", "ci"), _ok(returncode=0)),
                (_matches("git", "push"), _ok()),
                (
                    _matches("gh", "pr", "create"),
                    _ok(stdout="https://github.com/x/y/pull/125\n"),
                ),
                (_matches("gh", "pr", "view"), _ok(stdout=rollup_ok)),
                (_matches("gh", "pr", "ready"), _ok()),
            ]
        )

        # Wrap base in a function so `gh pr merge` raises while everything
        # else delegates. We can't monkey-patch base.__call__ on the
        # instance — Python resolves __call__ via the type — so we install
        # the wrapper as the runner directly.
        def runner(cmd, **kwargs):
            if "gh pr merge" in " ".join(cmd):
                raise subprocess.CalledProcessError(
                    1, cmd, output="", stderr="conflict"
                )
            return base(cmd, **kwargs)

        with FakeRunnerScope(base):
            pr_merge._set_runner(runner)
            outcome = pr_merge.merge_task(
                worktree_root=self.worktree,
                branch="feat/x",
                pr_title="t",
                pr_body="b",
                local_ci_log=self.log,
                sibling_paths=[self.sibling],
            )
        self.assertEqual(outcome.status, "pr_opened")
        self.assertEqual(outcome.pr_number, 125)
        self.assertIn("admin merge failed", outcome.failure)


if __name__ == "__main__":
    unittest.main()
