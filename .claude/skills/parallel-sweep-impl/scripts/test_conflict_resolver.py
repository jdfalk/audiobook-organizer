# file: .claude/skills/parallel-sweep-impl/scripts/test_conflict_resolver.py
# version: 1.0.0
# guid: 9e0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b

"""Unit tests for conflict_resolver.py.

Uses real local git fixtures that produce real rebase conflicts (same
pattern as test_rebase.py). The test produces conflicts deliberately by
having two branches that touch the same lines of the same file.

Run from this directory:
    python3 -m unittest test_conflict_resolver.py -v
"""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path

import conflict_resolver as cr


def _git(cwd: Path, *args: str, check: bool = True) -> str:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        check=check,
        capture_output=True,
        text=True,
    ).stdout


def _make_conflict_repo(tmp: Path, *, files: int = 1) -> Path:
    """Set up a repo with two branches that conflict on `files` files.

    Returns the repo path, with HEAD detached on a feature branch and a
    rebase in progress with conflicts ready to inspect.
    """
    repo = tmp / "repo"
    repo.mkdir()
    _git(repo, "init", "-b", "main")
    _git(repo, "config", "user.email", "t@e")
    _git(repo, "config", "user.name", "t")
    # Initial commit with `files` files, each containing "BASE".
    for i in range(files):
        (repo / f"f{i}.txt").write_text("BASE\n")
    _git(repo, "add", ".")
    _git(repo, "commit", "-m", "init")

    # main: change every file from BASE to MAIN.
    for i in range(files):
        (repo / f"f{i}.txt").write_text("MAIN\n")
    _git(repo, "add", ".")
    _git(repo, "commit", "-m", "main change")

    # feature: branch off init, change every file from BASE to FEAT.
    _git(repo, "checkout", "-b", "feat", "HEAD~1")
    for i in range(files):
        (repo / f"f{i}.txt").write_text("FEAT\n")
    _git(repo, "add", ".")
    _git(repo, "commit", "-m", "feat change")

    # Rebase feat onto main → conflicts in every file.
    rebase_result = subprocess.run(
        ["git", "-C", str(repo), "rebase", "main"],
        capture_output=True,
        text=True,
    )
    # Expected to fail with conflict markers.
    assert rebase_result.returncode != 0, (
        f"expected rebase to conflict, got rc={rebase_result.returncode}"
    )
    return repo


class ConflictInspectionTests(unittest.TestCase):
    def test_list_conflict_files_finds_all(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=3)
            files = cr.list_conflict_files(repo)
        self.assertEqual(sorted(files), ["f0.txt", "f1.txt", "f2.txt"])

    def test_list_conflict_files_empty_on_clean_repo(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = Path(td) / "clean"
            repo.mkdir()
            _git(repo, "init", "-b", "main")
            _git(repo, "config", "user.email", "t@e")
            _git(repo, "config", "user.name", "t")
            (repo / "x.txt").write_text("x\n")
            _git(repo, "add", ".")
            _git(repo, "commit", "-m", "init")
            self.assertEqual(cr.list_conflict_files(repo), [])

    def test_count_conflict_markers(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=2)
            files = cr.list_conflict_files(repo)
            count = cr.count_conflict_markers(repo, files)
        # Each conflicting file has exactly one marker block.
        self.assertEqual(count, 2)


class AssessConflictTests(unittest.TestCase):
    def test_trivial_when_one_file_one_marker(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=1)
            assessment = cr.assess_conflict(repo)
        self.assertTrue(assessment.is_trivial)
        self.assertEqual(assessment.files, ["f0.txt"])
        self.assertEqual(assessment.marker_count, 1)
        self.assertIn("trivial", assessment.reason)

    def test_not_trivial_when_too_many_files(self) -> None:
        # Generate a conflict with 4 files (above TRIVIAL_FILE_THRESHOLD=3).
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=4)
            assessment = cr.assess_conflict(repo)
        self.assertFalse(assessment.is_trivial)
        self.assertEqual(len(assessment.files), 4)
        self.assertIn("exceeds trivial threshold", assessment.reason)

    def test_returns_no_conflicts_on_clean_tree(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = Path(td) / "clean"
            repo.mkdir()
            _git(repo, "init", "-b", "main")
            _git(repo, "config", "user.email", "t@e")
            _git(repo, "config", "user.name", "t")
            (repo / "x.txt").write_text("x\n")
            _git(repo, "add", ".")
            _git(repo, "commit", "-m", "init")
            assessment = cr.assess_conflict(repo)
        self.assertFalse(assessment.is_trivial)
        self.assertEqual(assessment.files, [])
        self.assertIn("no conflicts found", assessment.reason)


class BuildResolverPromptTests(unittest.TestCase):
    def test_substitutes_all_placeholders(self) -> None:
        template = (
            Path(__file__).parent.parent / "references" / "conflict-resolver-prompt.md"
        )
        with tempfile.TemporaryDirectory() as td:
            wt = Path(td)
            prompt = cr.build_resolver_prompt(
                worktree_root=wt,
                task_branch="refactor/handlers",
                conflict_files=["a.go", "b.go"],
                marker_count=4,
                template_path=template,
            )
        self.assertIn(str(wt.resolve()), prompt)
        self.assertIn("refactor/handlers", prompt)
        self.assertIn("`a.go`", prompt)
        self.assertIn("`b.go`", prompt)
        self.assertIn("4", prompt)
        # Sanity: leftover placeholders would mean a typo in the template
        # or extractor.
        self.assertNotIn("{{WORKTREE_PATH}}", prompt)
        self.assertNotIn("{{TASK_BRANCH}}", prompt)
        self.assertNotIn("{{CONFLICT_FILES_LIST}}", prompt)
        self.assertNotIn("{{MARKER_COUNT}}", prompt)


class ParseResolverReportTests(unittest.TestCase):
    def test_success_report(self) -> None:
        report = cr.parse_resolver_report(
            "RESOLVED_FILES:\n"
            "- a.go: combined both intents\n"
            "- b.go: kept main's signature, added branch's call\n"
            "\n"
            "UNRESOLVED_FILES:\n"
            "- none\n"
            "\n"
            "EXIT_REASON: success\n"
        )
        self.assertEqual(report.exit_reason, cr.ResolverExit.SUCCESS)
        self.assertEqual(len(report.resolved_files), 2)
        self.assertEqual(report.unresolved_files, [])

    def test_uncertain_report(self) -> None:
        report = cr.parse_resolver_report(
            "RESOLVED_FILES:\n"
            "- a.go: combined both intents\n"
            "\n"
            "UNRESOLVED_FILES:\n"
            "- b.go: semantic conflict — main renamed Foo, branch added a use of Foo\n"
            "\n"
            "EXIT_REASON: uncertain — see UNRESOLVED_FILES\n"
        )
        self.assertEqual(report.exit_reason, cr.ResolverExit.UNCERTAIN)
        self.assertEqual(len(report.unresolved_files), 1)
        self.assertIn("renamed Foo", report.unresolved_files[0])

    def test_missing_exit_reason_treated_as_uncertain(self) -> None:
        # Conservative default: anything not explicitly success → uncertain.
        report = cr.parse_resolver_report("RESOLVED_FILES:\n- a.go: ok\n")
        self.assertEqual(report.exit_reason, cr.ResolverExit.UNCERTAIN)


class ApplyResolverSuccessTests(unittest.TestCase):
    """End-to-end: resolver edits the file, then we apply the success path."""

    def test_continues_rebase_when_markers_resolved(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=1)
            # Simulate the resolver edit: pick the FEAT side.
            (repo / "f0.txt").write_text("FEAT\n")
            ok, detail = cr.apply_resolver_success(repo)
        self.assertTrue(ok, detail)
        self.assertIn("continued", detail)

    def test_refuses_when_markers_remain(self) -> None:
        # Resolver claimed success but didn't actually edit out the markers.
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=1)
            ok, detail = cr.apply_resolver_success(repo)
        self.assertFalse(ok)
        self.assertIn("markers remain", detail)


class AbortRebaseTests(unittest.TestCase):
    def test_aborts_in_progress_rebase(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = _make_conflict_repo(Path(td), files=1)
            cr.abort_rebase(repo)
            # After abort, no conflicts and no rebase in progress.
            self.assertEqual(cr.list_conflict_files(repo), [])

    def test_aborts_when_no_rebase_in_progress_does_not_raise(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            repo = Path(td) / "clean"
            repo.mkdir()
            _git(repo, "init", "-b", "main")
            _git(repo, "config", "user.email", "t@e")
            _git(repo, "config", "user.name", "t")
            (repo / "x.txt").write_text("x\n")
            _git(repo, "add", ".")
            _git(repo, "commit", "-m", "init")
            # Should not raise even though there's no rebase to abort.
            cr.abort_rebase(repo)


if __name__ == "__main__":
    unittest.main()
