# file: .claude/skills/parallel-sweep-impl/scripts/test_fallback.py
# version: 1.1.0
# guid: 2c3d4e5f-6a7b-8c9d-0e1f-2a3b4c5d6e7f

"""Unit tests for fallback.py — per-commit cherry-pick fallback.

Same fixture pattern as test_conflict_resolver.py: build real local git
state with a real conflict, then exercise the fallback functions. The
Opus dispatch is replaced by a callable in the orchestrator tests so we
can simulate SUCCESS / UNCERTAIN replies deterministically.

Run from this directory:
    python3 -m unittest test_fallback.py -v
"""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path

import fallback


def _git(cwd: Path, *args: str, check: bool = True) -> str:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        check=check,
        capture_output=True,
        text=True,
    ).stdout


def _make_conflict_repo(
    tmp: Path,
    *,
    files: int = 2,
    branch_commits: int = 1,
) -> tuple[Path, str]:
    """Build a repo where ``feat`` branched off main, both touched the same
    files, and a rebase produced conflicts.

    branch_commits controls how many commits the branch has — useful for
    verifying multi-commit cherry-pick replay.

    Returns (repo, "feat" branch name). The repo is left mid-rebase.
    """
    repo = tmp / "repo"
    repo.mkdir()
    _git(repo, "init", "-b", "main")
    _git(repo, "config", "user.email", "t@e")
    _git(repo, "config", "user.name", "t")

    for i in range(files):
        (repo / f"f{i}.txt").write_text("BASE\n")
    _git(repo, "add", ".")
    _git(repo, "commit", "-m", "init")

    # Pretend "main" advanced via a sibling — write MAIN to every file.
    for i in range(files):
        (repo / f"f{i}.txt").write_text("MAIN\n")
    _git(repo, "add", ".")
    _git(repo, "commit", "-m", "sibling change")

    # feat: branch off init, write FEAT to every file across N commits.
    _git(repo, "checkout", "-b", "feat", "HEAD~1")
    for n in range(branch_commits):
        for i in range(files):
            (repo / f"f{i}.txt").write_text(f"FEAT-{n}\n")
        _git(repo, "add", ".")
        _git(repo, "commit", "-m", f"feat: feature change {n}")

    # Trigger the conflict by attempting the rebase.
    subprocess.run(
        ["git", "-C", str(repo), "rebase", "main"],
        capture_output=True,
        text=True,
    )
    return repo, "feat"


class PrepareFallbackTests(unittest.TestCase):
    def test_aborts_rebase_and_captures_commits(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=2, branch_commits=2)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")
        self.assertEqual(len(ctx.commits), 2)
        self.assertTrue(ctx.base_sha)
        self.assertTrue(ctx.branch_tip_sha)
        self.assertNotEqual(ctx.base_sha, ctx.branch_tip_sha)
        self.assertEqual(ctx.branch, "feat")

    def test_resets_worktree_to_base(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=1)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")
            head = _git(repo, "rev-parse", "HEAD").strip()
            self.assertEqual(head, ctx.base_sha)
            self.assertEqual(_git(repo, "status", "--porcelain"), "")

    def test_commits_in_chronological_order(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=1, branch_commits=3)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")
            # First captured commit should be the oldest on the branch.
            first_subject = fallback.commit_subject(repo, ctx.commits[0])
            last_subject = fallback.commit_subject(repo, ctx.commits[-1])
        self.assertEqual(first_subject, "feat: feature change 0")
        self.assertEqual(last_subject, "feat: feature change 2")


class ReadFileAtRefTests(unittest.TestCase):
    def test_returns_file_content_at_ref(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=1)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")
            branch_version = fallback.read_file_at_ref(
                repo, ctx.branch_tip_sha, "f0.txt"
            )
            main_version = fallback.read_file_at_ref(repo, ctx.base_sha, "f0.txt")
        self.assertEqual(branch_version, "FEAT-0\n")
        self.assertEqual(main_version, "MAIN\n")

    def test_returns_none_when_file_missing(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=1)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")
            self.assertIsNone(
                fallback.read_file_at_ref(repo, ctx.branch_tip_sha, "nonexistent.txt")
            )


class ParseReplyTests(unittest.TestCase):
    def test_success_returns_code_block_content(self) -> None:
        reply = fallback.parse_fallback_reply(
            "Here is the merged file:\n"
            "\n"
            "```\n"
            "merged content line 1\n"
            "merged content line 2\n"
            "```\n"
        )
        self.assertEqual(reply.exit_reason, fallback.FallbackExit.SUCCESS)
        self.assertEqual(
            reply.merged_content, "merged content line 1\nmerged content line 2\n"
        )

    def test_uncertain_takes_priority_over_code_block(self) -> None:
        reply = fallback.parse_fallback_reply(
            "UNCERTAIN: cannot reconcile renamed type with new caller\n"
            "\n"
            "```\n"
            "this should be ignored\n"
            "```\n"
        )
        self.assertEqual(reply.exit_reason, fallback.FallbackExit.UNCERTAIN)
        self.assertIn("renamed type", reply.uncertain_reason)
        self.assertIsNone(reply.merged_content)

    def test_no_code_block_no_uncertain_returns_uncertain(self) -> None:
        reply = fallback.parse_fallback_reply("just some prose, no fenced block\n")
        self.assertEqual(reply.exit_reason, fallback.FallbackExit.UNCERTAIN)
        self.assertIn("neither", reply.uncertain_reason)


class RunFallbackTests(unittest.TestCase):
    def test_single_commit_replay_preserves_message(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=2, branch_commits=1)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")

            def fake_dispatch(commit_sha, subject, path, idx, total):
                return fallback.FallbackReply(
                    exit_reason=fallback.FallbackExit.SUCCESS,
                    merged_content=f"MERGED-{path}\n",
                    uncertain_reason=None,
                )

            outcome = fallback.run_fallback(ctx=ctx, dispatch=fake_dispatch)

            self.assertEqual(outcome.status, "merged")
            self.assertEqual(len(outcome.commits_replayed), 1)
            # Original commit message preserved (not squashed).
            log = _git(repo, "log", "--oneline", "-3").strip().splitlines()
            self.assertIn("feature change 0", log[0])
            # File contents reflect the merge.
            self.assertEqual((repo / "f0.txt").read_text(), "MERGED-f0.txt\n")
            self.assertEqual((repo / "f1.txt").read_text(), "MERGED-f1.txt\n")

    def test_multi_commit_replay_produces_one_commit_per_original(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=1, branch_commits=3)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")

            # Vary the merged content per commit so we can verify each
            # cherry-pick produced its own commit (not a squash).
            def fake_dispatch(commit_sha, subject, path, idx, total):
                return fallback.FallbackReply(
                    exit_reason=fallback.FallbackExit.SUCCESS,
                    merged_content=f"MERGED-{subject}\n",
                    uncertain_reason=None,
                )

            outcome = fallback.run_fallback(ctx=ctx, dispatch=fake_dispatch)
            self.assertEqual(outcome.status, "merged")
            self.assertEqual(len(outcome.commits_replayed), 3)
            # Three new commits on top of base, original messages preserved
            # in chronological order.
            log = _git(repo, "log", "--oneline", "-6").strip().splitlines()
            # First three lines (newest first) should be the replayed commits.
            replayed_messages = [line for line in log[:3]]
            self.assertIn("feature change 2", replayed_messages[0])
            self.assertIn("feature change 1", replayed_messages[1])
            self.assertIn("feature change 0", replayed_messages[2])
            # File reflects the LAST commit's merge (each commit re-wrote
            # the file).
            self.assertEqual(
                (repo / "f0.txt").read_text(), "MERGED-feat: feature change 2\n"
            )

    def test_uncertain_blocks_at_first_failure(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo, branch = _make_conflict_repo(Path(td), files=2, branch_commits=1)
            ctx = fallback.prepare_fallback(repo, branch=branch, base_ref="main")

            calls: list[str] = []

            def fake_dispatch(commit_sha, subject, path, idx, total):
                calls.append(path)
                if idx == 1:
                    return fallback.FallbackReply(
                        exit_reason=fallback.FallbackExit.SUCCESS,
                        merged_content=f"OK-{path}\n",
                        uncertain_reason=None,
                    )
                return fallback.FallbackReply(
                    exit_reason=fallback.FallbackExit.UNCERTAIN,
                    merged_content=None,
                    uncertain_reason="too complex for a flat merge",
                )

            outcome = fallback.run_fallback(ctx=ctx, dispatch=fake_dispatch)
            self.assertEqual(outcome.status, "rebase_blocked")
            self.assertIn("Opus uncertain", outcome.detail)
            self.assertEqual(len(calls), 2)
            self.assertEqual(len(outcome.files_uncertain), 1)
            # Cherry-pick should have been aborted, leaving worktree clean.
            self.assertEqual(_git(repo, "status", "--porcelain"), "")


if __name__ == "__main__":
    unittest.main()
