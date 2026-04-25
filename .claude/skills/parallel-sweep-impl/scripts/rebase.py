# file: .claude/skills/parallel-sweep-impl/scripts/rebase.py
# version: 1.0.0
# guid: 5a6b7c8d-9e0f-1a2b-3c4d-5e6f7a8b9c0d

"""Sibling rebase loop for /parallel-sweep.

After every successful merge, the coordinator must rebase every still-unmerged
sibling worktree onto the new ``origin/main``. This module owns that loop.

The clean case (this step):
- ``git fetch origin main``
- ``git rebase origin/main``
- If the rebase succeeds with no conflict markers, the sibling is up-to-date
  and ready for its own merge gate.

The conflict cases (steps 6/7) are deliberately NOT handled here yet:
- Trivial conflicts (≤30 markers, ≤3 files) → Sonnet conflict-resolver subagent.
- Larger conflicts → Opus file-copy cherry-pick fallback.
- Both will add new functions to this module without touching the clean path.

Why a separate module from pr_merge.py: pr_merge handles ONE task's
post-completion pipeline (its own isolation, CI, merge). Rebase is a
cross-task operation triggered by an unrelated task's merge — different
trigger, different scope, different failure modes (a single sibling rebase
failure shouldn't block other siblings).
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Callable

# Same test seam as pr_merge.py — lets unit tests inject a fake subprocess.
_run: Callable[..., subprocess.CompletedProcess] = subprocess.run


def _set_runner(runner: Callable[..., subprocess.CompletedProcess]) -> None:
    global _run
    _run = runner


def _reset_runner() -> None:
    global _run
    _run = subprocess.run


class RebaseOutcome(str, Enum):
    """Distinct outcomes the coordinator may need to act on differently."""

    CLEAN = "clean"  # rebase succeeded, no conflicts
    UP_TO_DATE = "up_to_date"  # nothing to do — sibling already has all of main
    CONFLICT = "conflict"  # rebase left the worktree in mid-rebase state
    DIRTY_TREE = "dirty_tree"  # refused — worktree had uncommitted changes
    FETCH_FAILED = "fetch_failed"  # `git fetch` failed (network / auth)


@dataclass
class SiblingRebaseResult:
    """Per-sibling outcome aggregated by rebase_siblings()."""

    slug: str
    worktree: Path
    outcome: RebaseOutcome
    detail: str  # human-readable; one-liner suitable for logs / state.errors


def _git(repo: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess:
    return _run(
        ["git", "-C", str(repo), *args],
        check=check,
        capture_output=True,
        text=True,
    )


def _has_uncommitted_changes(worktree: Path) -> bool:
    """True iff git status --porcelain has any output."""
    result = _git(worktree, "status", "--porcelain")
    return bool(result.stdout.strip())


def _is_up_to_date(worktree: Path, base_ref: str) -> bool:
    """True iff HEAD has no commits ahead of base_ref AND no commits behind.

    Specifically: the symmetric-difference count is zero. This is the case
    where a rebase would be a literal no-op.
    """
    result = _git(worktree, "rev-list", "--count", f"HEAD...{base_ref}")
    return result.stdout.strip() == "0"


def _rebase_in_progress(worktree: Path) -> bool:
    """True iff git is currently mid-rebase in this worktree.

    Detection: the .git/rebase-merge or .git/rebase-apply directory exists.
    For worktrees, .git is a file pointing to the gitdir, so we resolve it.
    """
    git_dir_result = _git(worktree, "rev-parse", "--git-dir", check=False)
    if git_dir_result.returncode != 0:
        return False
    git_dir = Path(git_dir_result.stdout.strip())
    if not git_dir.is_absolute():
        git_dir = (worktree / git_dir).resolve()
    return (git_dir / "rebase-merge").exists() or (git_dir / "rebase-apply").exists()


def fetch_main(worktree: Path, *, remote: str = "origin", branch: str = "main") -> bool:
    """``git fetch <remote> <branch>``. Returns True on success.

    Only fetches the one branch the coordinator cares about — keeps the
    network footprint tight on slow networks during long sweeps.
    """
    result = _git(worktree, "fetch", remote, branch, check=False)
    return result.returncode == 0


def rebase_onto_main(
    worktree: Path,
    *,
    remote: str = "origin",
    branch: str = "main",
) -> SiblingRebaseResult:
    """Rebase ``worktree``'s branch onto ``<remote>/<branch>``.

    Returns a SiblingRebaseResult with the outcome enum + a one-line detail
    describing what happened. The slug field is left empty — rebase_siblings()
    fills it in. The worktree field is set to the resolved path.

    Pre-flight checks:
    - Refuse if the worktree has uncommitted changes (DIRTY_TREE). The child
      agent should have committed before reporting `completed`; if it didn't,
      that's a child contract violation, not a rebase problem.
    - If the sibling is already up to date with the base (UP_TO_DATE), skip
      the rebase entirely — saves time and avoids spurious "rewriting same
      commits" output.
    """
    resolved = worktree.resolve()

    if _has_uncommitted_changes(resolved):
        return SiblingRebaseResult(
            slug="",
            worktree=resolved,
            outcome=RebaseOutcome.DIRTY_TREE,
            detail="worktree has uncommitted changes; coordinator should mark task failed",
        )

    if not fetch_main(resolved, remote=remote, branch=branch):
        return SiblingRebaseResult(
            slug="",
            worktree=resolved,
            outcome=RebaseOutcome.FETCH_FAILED,
            detail=f"git fetch {remote} {branch} failed",
        )

    base_ref = f"{remote}/{branch}"
    if _is_up_to_date(resolved, base_ref):
        return SiblingRebaseResult(
            slug="",
            worktree=resolved,
            outcome=RebaseOutcome.UP_TO_DATE,
            detail=f"already up to date with {base_ref}",
        )

    rebase_result = _git(resolved, "rebase", base_ref, check=False)
    if rebase_result.returncode == 0:
        return SiblingRebaseResult(
            slug="",
            worktree=resolved,
            outcome=RebaseOutcome.CLEAN,
            detail=f"rebased cleanly onto {base_ref}",
        )

    # Non-zero exit = either conflict or some other rebase failure. Detect
    # conflict via the rebase-in-progress state. If we're not mid-rebase,
    # something else broke (e.g., bad ref) — surface as CONFLICT for now since
    # the coordinator's response is the same either way (don't merge, escalate).
    if _rebase_in_progress(resolved):
        # Leave the worktree in mid-rebase state for the resolver subagent
        # (steps 6/7) to inspect. Capture the conflict files for the detail
        # so the coordinator can decide trivial-vs-fallback.
        diff = _git(resolved, "diff", "--name-only", "--diff-filter=U", check=False)
        files = [f for f in diff.stdout.splitlines() if f]
        return SiblingRebaseResult(
            slug="",
            worktree=resolved,
            outcome=RebaseOutcome.CONFLICT,
            detail=f"rebase conflict in {len(files)} file(s): {', '.join(files[:5])}",
        )

    return SiblingRebaseResult(
        slug="",
        worktree=resolved,
        outcome=RebaseOutcome.CONFLICT,
        detail=f"git rebase failed (exit {rebase_result.returncode}): "
        f"{rebase_result.stderr.strip()[:200]}",
    )


def rebase_siblings(
    siblings: list[tuple[str, Path]],
    *,
    remote: str = "origin",
    branch: str = "main",
) -> list[SiblingRebaseResult]:
    """Rebase every sibling in turn; aggregate outcomes.

    ``siblings`` is a list of (slug, worktree_path) pairs in the order they
    should be processed. The coordinator typically passes the sibling rebase
    queue from the state file.

    A failure on one sibling does NOT block the others — the coordinator
    handles each task independently. The returned list mirrors the input
    order so the caller can correlate results back to slugs.
    """
    results: list[SiblingRebaseResult] = []
    for slug, worktree in siblings:
        result = rebase_onto_main(worktree, remote=remote, branch=branch)
        result.slug = slug
        results.append(result)
    return results
