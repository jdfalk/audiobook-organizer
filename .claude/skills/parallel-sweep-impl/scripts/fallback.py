# file: .claude/skills/parallel-sweep-impl/scripts/fallback.py
# version: 1.1.0
# guid: 1b2c3d4e-5f6a-7b8c-9d0e-1f2a3b4c5d6e

"""Opus file-copy cherry-pick fallback for /parallel-sweep.

When a sibling rebase produces conflicts that exceed the trivial threshold
(>30 markers OR >3 files — see ``conflict_resolver.assess_conflict``), the
trivial Sonnet resolver path is skipped and the coordinator dispatches THIS
fallback. Same trigger when Sonnet returned ``EXIT_REASON: uncertain``.

The procedure (plan §7), in commit-history-preserving form:

1. Abort the in-progress rebase (clean state).
2. Capture the branch's commit list between base (origin/main) and the
   pre-rebase tip. These are the commits we must end up with on top of
   the new main, in the same order, with the same author/message.
3. Reset the worktree to base (origin/main).
4. For each captured commit, in order:
   a. ``git cherry-pick`` the commit.
   b. If clean → next commit.
   c. If conflict: for each conflicted file, dispatch an Opus subagent
      with both full file versions side by side. Get back the merged
      content. Write + ``git add``.
   d. ``git cherry-pick --continue`` (preserves the original commit
      message and author via cherry-pick's defaults).
5. End state: branch has N commits on top of new main, same N as before,
   conflicts resolved.
6. If any per-file dispatch returns "uncertain" or any cherry-pick fails
   irrecoverably: ``git cherry-pick --abort`` (or ``--quit`` if --abort
   isn't applicable), mark the task ``rebase_blocked``, escalate.

Why per-commit cherry-pick instead of squash:

- This repo uses rebase/FF-only merges; no squash anywhere.
- Preserves commit-history granularity (the per-commit messages, separate
  author lines, individual reviews on `git log`).
- Mirrors what ``git rebase --continue`` would have done if the conflicts
  were trivial enough for the Sonnet path.

Design notes:

- One Opus dispatch per conflicted file per commit. A single big-bang
  3-way merge of the entire branch isn't enough: each commit may touch
  different parts of the same file, and the right resolution depends on
  the commit's intent. Per-commit-per-file is more dispatches but each
  is bounded and well-scoped.
- Unit-test layer ships now; live Opus spike on a real non-trivial
  conflict is deferred to step 9's full coordinator smoke where the
  verification pairs naturally with the end-to-end run.
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Callable

# Same test seam pattern as the other modules.
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


# --- Pre-flight -------------------------------------------------------------


@dataclass
class FallbackContext:
    """Snapshot taken before reset; used to drive the per-commit replay."""

    worktree: Path
    branch: str  # the task branch (e.g. refactor/audiobooks)
    base_sha: str  # origin/main when fallback was prepared
    branch_tip_sha: str  # branch HEAD pre-reset (the branch's pre-fallback tip)
    commits: list[str] = field(default_factory=list)  # SHAs to replay, in order


def prepare_fallback(
    worktree: Path,
    *,
    branch: str,
    base_ref: str = "origin/main",
) -> FallbackContext:
    """Capture commit list, then reset to base.

    Sequencing:
    1. Abort any in-progress rebase first.
    2. Capture branch_tip_sha BEFORE the reset.
    3. List commits between base and tip in *chronological* order — that's
       the order we must replay them in.
    4. Reset worktree HEAD to base so the cherry-picks land on a clean main.
    """
    # Step 1: clean state. cherry-pick in progress is also possible from a
    # previous failed fallback attempt — clean both.
    _git(worktree, "rebase", "--abort", check=False)
    _git(worktree, "cherry-pick", "--abort", check=False)

    # Step 2: capture the pre-reset tip.
    branch_tip_sha = _git(worktree, "rev-parse", "HEAD").stdout.strip()

    # Step 3: list commits oldest → newest. --reverse gives chronological.
    log = _git(
        worktree,
        "log",
        "--reverse",
        "--format=%H",
        f"{base_ref}..{branch_tip_sha}",
    )
    commits = [line for line in log.stdout.splitlines() if line]

    # Step 4: reset to base so cherry-picks have a clean target.
    base_sha = _git(worktree, "rev-parse", base_ref).stdout.strip()
    _git(worktree, "reset", "--hard", base_sha)

    return FallbackContext(
        worktree=worktree,
        branch=branch,
        base_sha=base_sha,
        branch_tip_sha=branch_tip_sha,
        commits=commits,
    )


# --- Per-file content extraction --------------------------------------------


def read_file_at_ref(worktree: Path, ref: str, path: str) -> str | None:
    """Return the file's content at ``ref``, or None if it didn't exist there.

    Used to build the per-file Opus prompt: we need both the branch's
    commit version and the worktree's current (pre-cherry-pick) version
    side by side.
    """
    result = _git(worktree, "show", f"{ref}:{path}", check=False)
    if result.returncode != 0:
        return None
    return result.stdout


def list_conflict_files(worktree: Path) -> list[str]:
    """Files with unmerged paths — same shape as conflict_resolver's helper."""
    result = _git(worktree, "diff", "--name-only", "--diff-filter=U")
    return [line for line in result.stdout.splitlines() if line]


# --- Prompt building --------------------------------------------------------


_FALLBACK_PROMPT_TEMPLATE = """\
You are an Opus file-copy fallback resolver in a /parallel-sweep run. The
trivial Sonnet path was skipped (or returned uncertain) for this conflict.
Your job is to produce the **merged content** for one specific file, for
one specific commit being cherry-picked.

# Context

- Worktree: {worktree}
- Task branch: {branch}
- Replaying commit {commit_sha} ("{commit_subject}") onto the new main.
- File path (relative to worktree root): {path}
- This is conflict file #{IDX} of {N_CONFLICTS} for this commit.

# The two versions

## What this commit wanted the file to look like (post-cherry-pick intent)

```
{commit_content}
```

## Current state in the worktree (main + any commits already replayed)

```
{base_content}
```

# What to do

1. Read both versions carefully. The "commit" version shows what THIS
   commit was trying to leave the file as. The "current" version is
   where the worktree is now (main, plus any earlier commits from this
   same branch already cherry-picked).
   Your job is to produce the file's content as it should be AFTER this
   cherry-pick lands, accounting for whatever the worktree side already
   reflects.

2. Output ONLY the merged file content, in a single fenced code block.
   Do not include any explanation outside the code block. Do not edit
   any file directly — your output IS the merged content; the
   coordinator will write it.

3. If you cannot confidently merge (semantic conflicts you can't resolve,
   data loss risk, ambiguity about intent), output ONE LINE outside any
   code block:

       UNCERTAIN: <one-line reason>

   The coordinator will mark the task rebase_blocked and escalate to the
   user. Better to escalate than to produce a wrong merge.

# Format

Reply with EXACTLY one of:

- A single fenced code block containing the full merged file content
  (one block, no extra prose).
- A single ``UNCERTAIN: <reason>`` line.

Nothing else.
"""


def build_fallback_prompt(
    *,
    ctx: FallbackContext,
    commit_sha: str,
    commit_subject: str,
    path: str,
    index: int,
    total_conflicts: int,
) -> str:
    """Build the per-commit-per-file Opus prompt.

    Args:
      ctx: the prepared FallbackContext.
      commit_sha: the commit being cherry-picked right now.
      commit_subject: the commit's subject line (from `git log -1 --format=%s`).
      path: the specific file being merged this dispatch.
      index: 1-based index of this conflict file within the current commit.
      total_conflicts: total conflict files in the current commit.
    """
    commit_content = read_file_at_ref(ctx.worktree, commit_sha, path)
    # Worktree current state: read directly (the conflicted file may not
    # match any single ref because cherry-pick wrote the merge markers).
    worktree_path = ctx.worktree / path
    if worktree_path.is_file():
        # Strip conflict markers for the prompt — we want the agent to see
        # the SEMANTIC sides, not the literal `<<<<<<<` mess.
        base_content = read_file_at_ref(ctx.worktree, "HEAD", path)
        if base_content is None:
            # Fall back to the literal disk content if HEAD doesn't have it.
            base_content = worktree_path.read_text(encoding="utf-8", errors="replace")
    else:
        base_content = read_file_at_ref(ctx.worktree, "HEAD", path)
    return _FALLBACK_PROMPT_TEMPLATE.format(
        worktree=ctx.worktree,
        branch=ctx.branch,
        commit_sha=commit_sha[:12],
        commit_subject=commit_subject,
        path=path,
        IDX=index,
        N_CONFLICTS=total_conflicts,
        commit_content=commit_content
        if commit_content is not None
        else "<file did not exist in this commit>",
        base_content=base_content
        if base_content is not None
        else "<file did not exist on HEAD>",
    )


# --- Reply parsing ----------------------------------------------------------


class FallbackExit(str, Enum):
    SUCCESS = "success"
    UNCERTAIN = "uncertain"


@dataclass
class FallbackReply:
    """Parsed view of an Opus fallback dispatch."""

    exit_reason: FallbackExit
    merged_content: str | None
    uncertain_reason: str | None


def parse_fallback_reply(text: str) -> FallbackReply:
    """Extract the merged-content code block OR the UNCERTAIN line.

    UNCERTAIN takes priority — if any line starts with ``UNCERTAIN:`` the
    reply is uncertain regardless of what else is in it.
    """
    for line in text.splitlines():
        if line.strip().startswith("UNCERTAIN:"):
            return FallbackReply(
                exit_reason=FallbackExit.UNCERTAIN,
                merged_content=None,
                uncertain_reason=line.strip()[len("UNCERTAIN:") :].strip(),
            )

    lines = text.splitlines()
    in_block = False
    block: list[str] = []
    for raw in lines:
        if raw.startswith("```"):
            if in_block:
                return FallbackReply(
                    exit_reason=FallbackExit.SUCCESS,
                    merged_content="\n".join(block) + "\n",
                    uncertain_reason=None,
                )
            in_block = True
            continue
        if in_block:
            block.append(raw)

    return FallbackReply(
        exit_reason=FallbackExit.UNCERTAIN,
        merged_content=None,
        uncertain_reason="reply contained neither a fenced code block nor an UNCERTAIN line",
    )


# --- Per-commit cherry-pick orchestration -----------------------------------


def cherry_pick(worktree: Path, commit_sha: str) -> bool:
    """``git cherry-pick <sha>``. Returns True if clean, False on conflict.

    Uses GIT_EDITOR=true so cherry-pick doesn't open an editor for the
    commit message — it reuses the original.
    """
    import os

    result = _run(
        ["git", "-C", str(worktree), "cherry-pick", commit_sha],
        check=False,
        capture_output=True,
        text=True,
        env={
            "GIT_EDITOR": "true",
            "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        },
    )
    return result.returncode == 0


def cherry_pick_continue(worktree: Path) -> bool:
    """``git cherry-pick --continue`` after writing merged files + git add."""
    import os

    result = _run(
        ["git", "-C", str(worktree), "cherry-pick", "--continue"],
        check=False,
        capture_output=True,
        text=True,
        env={
            "GIT_EDITOR": "true",
            "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        },
    )
    return result.returncode == 0


def cherry_pick_abort(worktree: Path) -> None:
    """Best-effort cleanup; tries --abort then --quit."""
    abort = _git(worktree, "cherry-pick", "--abort", check=False)
    if abort.returncode != 0:
        _git(worktree, "cherry-pick", "--quit", check=False)


def commit_subject(worktree: Path, commit_sha: str) -> str:
    """Return the commit's subject line for the prompt."""
    result = _git(worktree, "log", "-1", "--format=%s", commit_sha, check=False)
    return result.stdout.strip() if result.returncode == 0 else ""


# --- Application + top-level orchestrator -----------------------------------


def write_merged_files(
    *,
    worktree: Path,
    file_contents: dict[str, str],
) -> tuple[bool, str]:
    """Write merged content to disk and ``git add``."""
    for rel, content in file_contents.items():
        target = worktree / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
    if not file_contents:
        return True, "no files to write"
    add = _git(worktree, "add", *file_contents.keys(), check=False)
    if add.returncode != 0:
        return False, f"git add failed: {add.stderr.strip()[:200]}"
    return True, f"wrote and staged {len(file_contents)} file(s)"


@dataclass
class FallbackOutcome:
    """End-to-end fallback result. Coordinator translates to state."""

    status: str  # "merged" | "rebase_blocked"
    detail: str
    commits_replayed: list[str] = field(default_factory=list)
    files_resolved: list[str] = field(default_factory=list)
    files_uncertain: list[str] = field(default_factory=list)


# Type alias for the dispatch callable: takes (commit_sha, commit_subject,
# file_path, index, total_conflicts), returns the resolver's reply. Lets the
# coordinator inject the real Agent dispatch and unit tests inject a fake.
DispatchFn = Callable[[str, str, str, int, int], FallbackReply]


def run_fallback(
    *,
    ctx: FallbackContext,
    dispatch: DispatchFn,
) -> FallbackOutcome:
    """Replay the captured commits one at a time, resolving conflicts.

    On any UNCERTAIN reply or unrecoverable cherry-pick failure: stop,
    leave the worktree on the last successfully replayed commit (after
    aborting any in-progress cherry-pick), return rebase_blocked. Better
    to escalate than to commit a half-merge.
    """
    commits_replayed: list[str] = []
    files_resolved: list[str] = []
    files_uncertain: list[str] = []

    for commit_sha in ctx.commits:
        if cherry_pick(ctx.worktree, commit_sha):
            commits_replayed.append(commit_sha)
            continue

        # Conflict: gather the conflicted files, dispatch per file.
        conflict_files = list_conflict_files(ctx.worktree)
        if not conflict_files:
            cherry_pick_abort(ctx.worktree)
            return FallbackOutcome(
                status="rebase_blocked",
                detail=f"cherry-pick of {commit_sha[:12]} failed without conflict files",
                commits_replayed=commits_replayed,
                files_resolved=files_resolved,
                files_uncertain=files_uncertain,
            )

        subject = commit_subject(ctx.worktree, commit_sha)
        per_file_merged: dict[str, str] = {}
        for i, rel in enumerate(conflict_files, start=1):
            reply = dispatch(commit_sha, subject, rel, i, len(conflict_files))
            if reply.exit_reason == FallbackExit.UNCERTAIN:
                files_uncertain.append(
                    f"{rel} @ {commit_sha[:12]}: {reply.uncertain_reason}"
                )
                cherry_pick_abort(ctx.worktree)
                return FallbackOutcome(
                    status="rebase_blocked",
                    detail=f"Opus uncertain on {rel} during {commit_sha[:12]}",
                    commits_replayed=commits_replayed,
                    files_resolved=files_resolved,
                    files_uncertain=files_uncertain,
                )
            assert reply.merged_content is not None
            per_file_merged[rel] = reply.merged_content
            files_resolved.append(f"{rel} @ {commit_sha[:12]}")

        ok, detail = write_merged_files(
            worktree=ctx.worktree, file_contents=per_file_merged
        )
        if not ok:
            cherry_pick_abort(ctx.worktree)
            return FallbackOutcome(
                status="rebase_blocked",
                detail=f"failed to write merged files for {commit_sha[:12]}: {detail}",
                commits_replayed=commits_replayed,
                files_resolved=files_resolved,
                files_uncertain=files_uncertain,
            )

        if not cherry_pick_continue(ctx.worktree):
            cherry_pick_abort(ctx.worktree)
            return FallbackOutcome(
                status="rebase_blocked",
                detail=f"cherry-pick --continue failed for {commit_sha[:12]}",
                commits_replayed=commits_replayed,
                files_resolved=files_resolved,
                files_uncertain=files_uncertain,
            )
        commits_replayed.append(commit_sha)

    return FallbackOutcome(
        status="merged",
        detail=f"replayed {len(commits_replayed)} commit(s), "
        f"resolved {len(files_resolved)} file(s)",
        commits_replayed=commits_replayed,
        files_resolved=files_resolved,
        files_uncertain=files_uncertain,
    )
