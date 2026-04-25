# file: .claude/skills/parallel-sweep-impl/scripts/resume.py
# version: 1.0.0
# guid: 3d4e5f6a-7b8c-9d0e-1f2a-3b4c5d6e7f8a

"""Resume support for /parallel-sweep.

When a sweep is killed mid-flight (SIGTERM, usage limit, crash) and the
user re-invokes ``/parallel-sweep --resume <runID>``, this module:

1. Loads the state file.
2. Verifies it's resumable (status is paused/failed, not running — unless
   the user passes --force, indicating they're certain the previous
   coordinator process is dead).
3. Identifies in-flight tasks (status in {dispatched, in_progress}) and
   resets each back to ``pending`` after ``git reset --hard origin/main``
   on its worktree.
4. Sets state status back to ``running`` so a coordinator can pick up.

Locked decision Q3 (plan §13): granularity = last completed task. A task
that was mid-edit when the coordinator died gets re-dispatched from
scratch on a clean tree. The agent's narrative work is lost; the worktree
state is reset. One code path, no special cases.

Why ``git reset --hard origin/main`` (not the original base):

- The task hasn't merged yet (it's in_flight at resume).
- Sibling tasks may have merged in the original sweep, advancing main.
- The resumed task should start from CURRENT main, not stale base — no
  point in re-doing a rebase later when we can land on current main now.

Open PRs from previous attempts are not closed by resume — the new
re-dispatched attempt opens a fresh PR. The orphan PR may still be open
on GitHub; the coordinator's Phase 6 cleanup (step 9) handles those.
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

import state as state_mod

# Test seam.
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


# Tasks in these states need to be reset and re-dispatched.
_IN_FLIGHT_STATES = {"dispatched", "in_progress", "committed", "pr_opened"}


class ResumeRefused(Exception):
    """The state file is not safely resumable.

    Raised when the existing run's status is ``running`` and the caller
    didn't pass ``force=True``. The right response is for the user to
    manually verify no other coordinator is alive (check ``ps``, the state
    file's ``lastCheckpointAt`` timestamp), then re-invoke with --force.
    """


@dataclass
class ResetResult:
    """Per-task outcome of the resume reset pass."""

    slug: str
    worktree: Path | None
    previous_status: str
    reset_succeeded: bool
    detail: str


@dataclass
class ResumeContext:
    """What load_for_resume returns. The coordinator uses this to drive
    Phase 1 (re-dispatch the now-pending tasks)."""

    state: state_mod.State
    in_flight_slugs: list[str]
    pending_slugs: list[str]  # tasks that were already pending — no reset needed
    completed_slugs: list[str]  # merged or failed — left alone
    skipped_slugs: list[str]  # rebase_blocked — needs user, leave alone


def load_for_resume(
    state_dir: Path,
    run_id: str,
    *,
    force: bool = False,
) -> ResumeContext:
    """Load the state file and classify tasks for the resume pass.

    Raises ResumeRefused if the run's status is ``running`` and force is
    False — that's the most likely sign that another coordinator is still
    alive. The user can re-invoke with --force after verifying.
    """
    state = state_mod.State.load(state_dir, run_id)

    if state.data["status"] == "running" and not force:
        raise ResumeRefused(
            f"run {run_id} is still marked status=running. Another coordinator "
            f"may be alive. Verify (e.g., ps, last checkpoint timestamp) and "
            f"re-invoke with force=True if certain it's dead."
        )

    in_flight: list[str] = []
    pending: list[str] = []
    completed: list[str] = []
    skipped: list[str] = []
    for task in state.data["tasks"]:
        status = task["status"]
        slug = task["slug"]
        if status in _IN_FLIGHT_STATES:
            in_flight.append(slug)
        elif status == "pending":
            pending.append(slug)
        elif status in {"merged", "failed"}:
            completed.append(slug)
        elif status == "rebase_blocked":
            skipped.append(slug)
        else:
            # Defensive: unknown status. Treat as in_flight so we reset it.
            in_flight.append(slug)

    return ResumeContext(
        state=state,
        in_flight_slugs=in_flight,
        pending_slugs=pending,
        completed_slugs=completed,
        skipped_slugs=skipped,
    )


def reset_in_flight(
    ctx: ResumeContext,
    *,
    main_ref: str = "origin/main",
) -> list[ResetResult]:
    """Reset every in-flight task's worktree to ``main_ref`` and mark pending.

    For tasks with no worktreePath (i.e., never made it past the dispatch
    step), no git reset is needed — just flip status back to pending.

    Per-task failures are recorded in the result list and also as task
    errors via state.append_task_error, but they don't block the rest of
    the reset pass. The coordinator can decide what to do with tasks
    whose reset failed (most likely: leave them in_flight and let the
    next resume try again).
    """
    results: list[ResetResult] = []
    for slug in ctx.in_flight_slugs:
        task = ctx.state.task(slug)
        previous_status = task["status"]
        worktree_path_str = task.get("worktreePath")
        worktree = Path(worktree_path_str) if worktree_path_str else None

        if worktree is None or not worktree.exists():
            # Nothing to reset — task never got a worktree. Just flip pending.
            ctx.state.update_task(slug, status="pending", agentID=None)
            results.append(
                ResetResult(
                    slug=slug,
                    worktree=worktree,
                    previous_status=previous_status,
                    reset_succeeded=True,
                    detail="no worktree to reset; marked pending",
                )
            )
            continue

        # Fetch first so origin/main reflects what's actually upstream.
        fetch = _git(worktree, "fetch", "origin", "main", check=False)
        if fetch.returncode != 0:
            ctx.state.append_task_error(
                slug, f"resume: git fetch failed; leaving status={previous_status}"
            )
            results.append(
                ResetResult(
                    slug=slug,
                    worktree=worktree,
                    previous_status=previous_status,
                    reset_succeeded=False,
                    detail=f"git fetch failed: {fetch.stderr.strip()[:200]}",
                )
            )
            continue

        # Abort any in-progress rebase / cherry-pick before reset.
        _git(worktree, "rebase", "--abort", check=False)
        _git(worktree, "cherry-pick", "--abort", check=False)

        reset = _git(worktree, "reset", "--hard", main_ref, check=False)
        if reset.returncode != 0:
            ctx.state.append_task_error(
                slug, f"resume: git reset --hard {main_ref} failed"
            )
            results.append(
                ResetResult(
                    slug=slug,
                    worktree=worktree,
                    previous_status=previous_status,
                    reset_succeeded=False,
                    detail=f"git reset failed: {reset.stderr.strip()[:200]}",
                )
            )
            continue

        ctx.state.update_task(slug, status="pending", agentID=None, prNumber=None)
        results.append(
            ResetResult(
                slug=slug,
                worktree=worktree,
                previous_status=previous_status,
                reset_succeeded=True,
                detail=f"reset to {main_ref}, marked pending",
            )
        )
    return results


def mark_resumed(ctx: ResumeContext) -> None:
    """Flip top-level status back to ``running``.

    Called after reset_in_flight succeeds so the coordinator can pick up.
    Idempotent — safe to call even if status was already running.
    """
    ctx.state.set_status("running")
