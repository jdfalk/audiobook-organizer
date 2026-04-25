# file: .claude/skills/parallel-sweep-impl/scripts/conflict_resolver.py
# version: 1.0.0
# guid: 8d9e0f1a-2b3c-4d5e-6f7a-8b9c0d1e2f3a

"""Conflict-resolver helpers for /parallel-sweep — trivial-conflict path.

When a sibling rebase produces conflicts, this module:

1. Counts how big the conflict is (markers + files affected).
2. Decides whether the conflict is *trivial* enough for a Sonnet resolver
   subagent (vs. fallback to Opus file-copy cherry-pick — step 7).
3. Builds the resolver's prompt by filling the template in
   ``../references/conflict-resolver-prompt.md``.
4. Translates the resolver's structured outcome into git verbs:
   ``git add -u && git rebase --continue`` on success,
   ``git rebase --abort`` on uncertainty.

The resolver subagent itself is a Sonnet Agent dispatch — that lives in the
coordinator. This module doesn't dispatch agents; it sets them up and
processes their output.
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Callable

# --- Trivial-vs-fallback heuristic -------------------------------------------

# Empirical threshold from the envelope-migration sweep notes. Below: Sonnet
# resolves reliably. Above: dispatch goes to the Opus file-copy fallback.
# Re-tune after a few real conflicts in step 6+ if the boundary turns out to
# be wrong.
TRIVIAL_MARKER_THRESHOLD = 30
TRIVIAL_FILE_THRESHOLD = 3


# --- Test seam (same pattern as pr_merge.py / rebase.py) ----------------------

_run: Callable[..., subprocess.CompletedProcess] = subprocess.run


def _set_runner(runner: Callable[..., subprocess.CompletedProcess]) -> None:
    global _run
    _run = runner


def _reset_runner() -> None:
    global _run
    _run = subprocess.run


def _git(repo: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess:
    return _run(
        ["git", "-C", str(repo), *args],
        check=check,
        capture_output=True,
        text=True,
    )


# --- Conflict inspection -----------------------------------------------------


def list_conflict_files(worktree: Path) -> list[str]:
    """Files with unmerged paths, per ``git diff --name-only --diff-filter=U``.

    Returns relative paths. Empty list = no conflicts (worktree clean or
    rebase already completed).
    """
    result = _git(worktree, "diff", "--name-only", "--diff-filter=U")
    return [line for line in result.stdout.splitlines() if line]


def count_conflict_markers(worktree: Path, files: list[str]) -> int:
    """Sum of ``<<<<<<<`` line counts across the given conflict files.

    A "marker" here means one conflict region. Each region has exactly one
    ``<<<<<<<`` opener regardless of how big the region is, so this is the
    right thing to count for the trivial-vs-fallback heuristic.

    Files that don't exist (e.g. a deleted-vs-modified conflict where one
    side removed the file) contribute zero markers but still count toward
    the file threshold via list_conflict_files().
    """
    total = 0
    for rel in files:
        path = worktree / rel
        if not path.is_file():
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        # Each conflict region begins with a line that starts with `<<<<<<<`.
        for line in text.splitlines():
            if line.startswith("<<<<<<<"):
                total += 1
    return total


@dataclass
class ConflictAssessment:
    """Result of inspecting a mid-rebase worktree."""

    files: list[str]
    marker_count: int
    is_trivial: bool  # within both thresholds
    reason: str  # one-liner explaining why (for logs / state.errors)


def assess_conflict(worktree: Path) -> ConflictAssessment:
    """Inspect the mid-rebase worktree and decide trivial vs. fallback.

    Trivial = ``len(files) <= TRIVIAL_FILE_THRESHOLD AND
              marker_count <= TRIVIAL_MARKER_THRESHOLD``.
    """
    files = list_conflict_files(worktree)
    if not files:
        return ConflictAssessment(
            files=[],
            marker_count=0,
            is_trivial=False,
            reason="no conflicts found (worktree may be clean or rebase already done)",
        )
    markers = count_conflict_markers(worktree, files)
    if len(files) > TRIVIAL_FILE_THRESHOLD:
        return ConflictAssessment(
            files=files,
            marker_count=markers,
            is_trivial=False,
            reason=f"{len(files)} files exceeds trivial threshold "
            f"({TRIVIAL_FILE_THRESHOLD})",
        )
    if markers > TRIVIAL_MARKER_THRESHOLD:
        return ConflictAssessment(
            files=files,
            marker_count=markers,
            is_trivial=False,
            reason=f"{markers} markers exceeds trivial threshold "
            f"({TRIVIAL_MARKER_THRESHOLD})",
        )
    return ConflictAssessment(
        files=files,
        marker_count=markers,
        is_trivial=True,
        reason=f"{len(files)} file(s), {markers} marker(s) — within trivial threshold",
    )


# --- Prompt building ---------------------------------------------------------


def build_resolver_prompt(
    *,
    worktree_root: Path,
    task_branch: str,
    conflict_files: list[str],
    marker_count: int,
    template_path: Path,
) -> str:
    r"""Read the resolver-prompt template and fill in placeholders.

    The template is ``references/conflict-resolver-prompt.md`` — it's wrapped
    in a fenced code block in the markdown source. We extract the block
    between the first ``\`\`\`...\`\`\`` pair and substitute the placeholders.
    """
    text = template_path.read_text(encoding="utf-8")
    # Extract the outer fenced block. The template's prompt body itself
    # contains nested triple-backtick code samples (e.g. the resolver's
    # report format), so naively pairing the first fence with the next one
    # truncates the prompt. We use the FIRST fence as the opener and the
    # LAST fence as the closer.
    start = text.find("\n```")
    if start == -1:
        raise RuntimeError(
            f"resolver prompt template at {template_path} missing fenced block"
        )
    # Move past the fence opener line.
    start = text.find("\n", start + 1) + 1
    end = text.rfind("\n```")
    if end == -1 or end <= start:
        raise RuntimeError(
            f"resolver prompt template at {template_path} missing closing fence"
        )
    body = text[start:end]

    files_list = "\n".join(f"- `{f}`" for f in conflict_files)
    return (
        body.replace("{{WORKTREE_PATH}}", str(worktree_root.resolve()))
        .replace("{{TASK_BRANCH}}", task_branch)
        .replace("{{CONFLICT_FILES_LIST}}", files_list)
        .replace("{{MARKER_COUNT}}", str(marker_count))
    )


# --- Outcome handling --------------------------------------------------------


class ResolverExit(str, Enum):
    """Possible structured outcomes the resolver subagent reports."""

    SUCCESS = "success"
    UNCERTAIN = "uncertain"


@dataclass
class ResolverReport:
    """Parsed view of the resolver subagent's reply."""

    exit_reason: ResolverExit
    resolved_files: list[str]
    unresolved_files: list[str]
    raw: str


def parse_resolver_report(text: str) -> ResolverReport:
    """Parse the resolver's structured reply.

    Format (from the prompt):
        RESOLVED_FILES:
        - <path>: <summary>
        ...
        UNRESOLVED_FILES:
        - <path>: <reason>  (or "none")
        ...
        EXIT_REASON: <success | uncertain — ...>

    Permissive: tolerates extra whitespace, missing sections, case in EXIT_REASON.
    """
    lines = text.splitlines()
    section: str | None = None
    resolved: list[str] = []
    unresolved: list[str] = []
    exit_reason: ResolverExit | None = None

    for raw in lines:
        line = raw.strip()
        if not line:
            continue
        upper = line.upper()
        if upper.startswith("RESOLVED_FILES"):
            section = "resolved"
            continue
        if upper.startswith("UNRESOLVED_FILES"):
            section = "unresolved"
            continue
        if upper.startswith("EXIT_REASON"):
            value = line.split(":", 1)[1].strip().lower() if ":" in line else ""
            if value.startswith("success"):
                exit_reason = ResolverExit.SUCCESS
            else:
                # Anything that's not explicitly "success" is treated as
                # uncertain. Conservative: better to escalate than to merge
                # the wrong intent.
                exit_reason = ResolverExit.UNCERTAIN
            continue
        if line.startswith("-") and section is not None:
            entry = line[1:].strip()
            if section == "resolved":
                resolved.append(entry)
            else:
                # Skip the literal "none" placeholder.
                if entry.lower() not in {"none", "(none)"}:
                    unresolved.append(entry)
    if exit_reason is None:
        exit_reason = ResolverExit.UNCERTAIN
    return ResolverReport(
        exit_reason=exit_reason,
        resolved_files=resolved,
        unresolved_files=unresolved,
        raw=text,
    )


def apply_resolver_success(worktree: Path) -> tuple[bool, str]:
    """``git add -u && git rebase --continue``. Returns (success, detail).

    Verifies first that no literal ``<<<<<<<`` text remains in the
    previously-conflicted files. If the resolver said ``success`` but
    didn't actually edit the markers out, that's a defect we want to catch
    before continuing the rebase.

    Note: we check file CONTENT (not ``git diff --diff-filter=U`` which
    looks at index state) because the resolver edits files in place but
    doesn't ``git add`` — that's our job below. So the index will still
    show the files as unmerged at this point; the meaningful question is
    whether the on-disk text still has conflict markers.
    """
    files_with_markers = _files_still_with_markers(worktree)
    if files_with_markers:
        return (
            False,
            f"resolver claimed success but markers remain in: "
            f"{', '.join(files_with_markers)}",
        )

    add = _git(worktree, "add", "-u", check=False)
    if add.returncode != 0:
        return False, f"git add -u failed: {add.stderr.strip()[:200]}"

    # GIT_EDITOR=true makes rebase --continue not open an editor for the
    # commit message — it reuses the message from the original commit.
    cont = _run(
        ["git", "-C", str(worktree), "rebase", "--continue"],
        check=False,
        capture_output=True,
        text=True,
        env={"GIT_EDITOR": "true", "PATH": _env_path()},
    )
    if cont.returncode != 0:
        return False, f"git rebase --continue failed: {cont.stderr.strip()[:200]}"
    return True, "rebase continued cleanly"


def _files_still_with_markers(worktree: Path) -> list[str]:
    """Return file paths (relative to worktree) that still contain ``<<<<<<<``."""
    files = list_conflict_files(worktree)
    bad: list[str] = []
    for rel in files:
        path = worktree / rel
        if not path.is_file():
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        if any(line.startswith("<<<<<<<") for line in text.splitlines()):
            bad.append(rel)
    return bad


def abort_rebase(worktree: Path) -> None:
    """``git rebase --abort``. Best-effort; logs but does not raise."""
    _git(worktree, "rebase", "--abort", check=False)


def _env_path() -> str:
    """Minimal PATH for subprocesses we spawn ourselves.

    git rebase needs at least /usr/bin and /bin to find its own helpers
    (sh, sed). We don't inherit os.environ wholesale because the test seam
    sometimes wants tight control over what subprocesses see.
    """
    import os

    return os.environ.get("PATH", "/usr/bin:/bin")
