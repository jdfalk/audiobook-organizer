# file: .claude/skills/parallel-sweep-impl/scripts/pr_merge.py
# version: 1.0.0
# guid: 3e4f5a6b-7c8d-9e0f-1a2b-3c4d5e6f7a8b

"""PR + merge pipeline for /parallel-sweep.

Encapsulates the per-task post-completion workflow that the coordinator runs
after a child agent reports `completed`:

    isolation check → local make ci → push → open PR → poll GitHub CI → merge

Each step is a separate function so the coordinator can call them piecewise
(e.g. on resume, just re-poll CI for an already-open PR), and so the unit
tests can mock subprocess calls without touching real GitHub.

The two-gate merge rule (locked decision Q2 from the plan, §13):
- Both GitHub CI green AND local `make ci` exit-zero must hold.
- GitHub CI red → never merge.
- Local CI red but GitHub green → also never merge (local is authoritative
  on coverage / tests the workflow doesn't run).

This module deliberately does NOT handle sibling rebase or conflict
resolution — those land in step 5+ and likely warrant their own module.
"""

from __future__ import annotations

import json
import shlex
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

# Allow tests to inject a fake subprocess.run. Default is the real thing.
_run: Callable[..., subprocess.CompletedProcess] = subprocess.run


def _set_runner(runner: Callable[..., subprocess.CompletedProcess]) -> None:
    """Test seam: install a fake subprocess runner. Call _reset_runner to undo."""
    global _run
    _run = runner


def _reset_runner() -> None:
    global _run
    _run = subprocess.run


@dataclass
class CIResult:
    """Outcome of polling a PR's GitHub CI to completion."""

    green: bool
    failed_checks: list[str]
    timed_out: bool


def run_local_ci(
    *,
    worktree_root: Path,
    log_path: Path,
    make_target: str = "ci",
    timeout_s: int = 1800,
) -> bool:
    """Run ``make <target>`` in the worktree, tee output to log_path.

    Returns True on exit code 0. On non-zero, the log file contains the
    full output; the caller writes the tail into the state file's task
    errors and skips the merge.

    Why ``tee``-equivalent (not just capture): the output is often long
    and the user wants to be able to ``tail -f`` it during a real sweep.
    The full text also goes to log_path so the coordinator can reference
    it after the fact.
    """
    log_path.parent.mkdir(parents=True, exist_ok=True)
    cmd = ["make", make_target]
    with log_path.open("w", encoding="utf-8") as log_fh:
        log_fh.write(
            f"$ cd {worktree_root} && {' '.join(shlex.quote(c) for c in cmd)}\n"
        )
        log_fh.flush()
        try:
            result = _run(
                cmd,
                cwd=str(worktree_root),
                stdout=log_fh,
                stderr=subprocess.STDOUT,
                timeout=timeout_s,
                check=False,
            )
        except subprocess.TimeoutExpired:
            log_fh.write(f"\n[pr_merge] TIMEOUT after {timeout_s}s\n")
            return False
    return result.returncode == 0


def push_branch(*, worktree_root: Path, branch: str) -> None:
    """``git push -u origin <branch>``. Raises CalledProcessError on failure."""
    _run(
        ["git", "-C", str(worktree_root), "push", "-u", "origin", branch],
        check=True,
        capture_output=True,
        text=True,
    )


def open_pr(
    *,
    worktree_root: Path,
    branch: str,
    title: str,
    body: str,
    base: str = "main",
) -> int:
    """``gh pr create``. Returns the new PR number."""
    result = _run(
        [
            "gh",
            "pr",
            "create",
            "--base",
            base,
            "--head",
            branch,
            "--title",
            title,
            "--body",
            body,
        ],
        cwd=str(worktree_root),
        check=True,
        capture_output=True,
        text=True,
    )
    # gh pr create prints the PR URL on the last non-empty line.
    last = next(
        (line for line in reversed(result.stdout.splitlines()) if line.strip()),
        "",
    )
    pr_str = last.rsplit("/", 1)[-1]
    if not pr_str.isdigit():
        raise RuntimeError(
            f"could not parse PR number from gh output: {result.stdout!r}"
        )
    return int(pr_str)


def poll_ci(
    *,
    pr_number: int,
    poll_interval_s: int = 30,
    timeout_s: int = 1800,
    sleep: Callable[[float], None] = time.sleep,
    now: Callable[[], float] = time.monotonic,
) -> CIResult:
    """Poll ``gh pr checks <n> --json statusCheckRollup`` until all complete.

    Returns CIResult(green=..., failed_checks=[...], timed_out=...).

    The ``sleep`` and ``now`` injectables let unit tests run the loop
    instantly. Production callers use the defaults.
    """
    deadline = now() + timeout_s
    while True:
        result = _run(
            [
                "gh",
                "pr",
                "view",
                str(pr_number),
                "--json",
                "statusCheckRollup",
            ],
            check=True,
            capture_output=True,
            text=True,
        )
        rollup = json.loads(result.stdout).get("statusCheckRollup", [])
        running = [c for c in rollup if c.get("status") != "COMPLETED"]
        if not running:
            failed = [
                c.get("name", "<unnamed>")
                for c in rollup
                if c.get("conclusion") == "FAILURE"
            ]
            return CIResult(green=not failed, failed_checks=failed, timed_out=False)
        if now() >= deadline:
            return CIResult(
                green=False,
                failed_checks=[c.get("name", "<unnamed>") for c in running],
                timed_out=True,
            )
        sleep(poll_interval_s)


def mark_pr_ready(*, pr_number: int) -> None:
    """Idempotent: ``gh pr ready`` succeeds even if the PR is already non-draft."""
    _run(
        ["gh", "pr", "ready", str(pr_number)],
        check=False,
        capture_output=True,
        text=True,
    )


def admin_rebase_merge(*, pr_number: int) -> None:
    """``gh pr merge <n> --rebase --admin``. Raises on failure."""
    _run(
        ["gh", "pr", "merge", str(pr_number), "--rebase", "--admin"],
        check=True,
        capture_output=True,
        text=True,
    )


@dataclass
class TaskOutcome:
    """Final outcome of merge_task. The coordinator uses this to update state."""

    status: str  # "merged" | "failed" | "rebase_blocked" | "pr_opened"
    pr_number: int | None
    failure: str | None  # human-readable; None on success


def merge_task(
    *,
    worktree_root: Path,
    branch: str,
    pr_title: str,
    pr_body: str,
    local_ci_log: Path,
    sibling_paths: list[Path],
    poll_interval_s: int = 30,
    poll_timeout_s: int = 1800,
) -> TaskOutcome:
    """Run the full per-task post-completion pipeline for one task.

    Stops and returns at the first failed gate. The coordinator translates
    the returned TaskOutcome into a state.update_task call.

    Why the sibling_paths arg: cross_check_isolation lives in dispatch.py and
    requires the coordinator to enumerate every other repo path (main checkout
    + every other live worktree). Passing them in keeps this module ignorant
    of the coordinator's worktree bookkeeping.
    """
    # Lazy import — both scripts live in the same dir but we don't want a
    # cycle if dispatch.py ever needs to import pr_merge.
    from dispatch import cross_check_isolation

    # Gate 1: post-hoc isolation check (load-bearing per 2026-04-25 spike).
    violations = cross_check_isolation(
        expected_worktree=worktree_root,
        sibling_paths=sibling_paths,
    )
    if violations:
        return TaskOutcome(
            status="failed",
            pr_number=None,
            failure="worktree isolation violation: " + "; ".join(violations),
        )

    # Gate 2: local make ci.
    if not run_local_ci(worktree_root=worktree_root, log_path=local_ci_log):
        return TaskOutcome(
            status="failed",
            pr_number=None,
            failure=f"local make ci failed; see {local_ci_log}",
        )

    # Push and open PR.
    try:
        push_branch(worktree_root=worktree_root, branch=branch)
        pr_number = open_pr(
            worktree_root=worktree_root,
            branch=branch,
            title=pr_title,
            body=pr_body,
        )
    except subprocess.CalledProcessError as exc:
        return TaskOutcome(
            status="failed",
            pr_number=None,
            failure=f"push or PR open failed: {exc.stderr or exc!r}",
        )

    # Gate 3: GitHub CI.
    ci = poll_ci(
        pr_number=pr_number,
        poll_interval_s=poll_interval_s,
        timeout_s=poll_timeout_s,
    )
    if ci.timed_out:
        return TaskOutcome(
            status="pr_opened",
            pr_number=pr_number,
            failure=f"GitHub CI did not complete within {poll_timeout_s}s; "
            f"still-running: {', '.join(ci.failed_checks)}",
        )
    if not ci.green:
        return TaskOutcome(
            status="failed",
            pr_number=pr_number,
            failure=f"GitHub CI failed: {', '.join(ci.failed_checks)}",
        )

    # Both gates green → merge.
    mark_pr_ready(pr_number=pr_number)
    try:
        admin_rebase_merge(pr_number=pr_number)
    except subprocess.CalledProcessError as exc:
        return TaskOutcome(
            status="pr_opened",
            pr_number=pr_number,
            failure=f"admin merge failed (likely transient — main may have moved): "
            f"{exc.stderr or exc!r}",
        )

    return TaskOutcome(status="merged", pr_number=pr_number, failure=None)
