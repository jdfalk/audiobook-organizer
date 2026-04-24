# file: .claude/skills/parallel-sweep-impl/scripts/state.py
# version: 1.0.0
# guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

"""State file CRUD for /parallel-sweep.

See ../references/state-schema.md for the schema. The only thing this module
guarantees beyond plain JSON I/O is atomic writes: every mutation goes through
checkpoint() which writes to a tmp file, fsyncs, and os.replace's into place.
"""

from __future__ import annotations

import json
import os
import re
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable

TOP_STATUSES = {"running", "paused", "complete", "failed"}
TASK_STATUSES = {
    "pending",
    "dispatched",
    "in_progress",
    "committed",
    "pr_opened",
    "merged",
    "rebase_blocked",
    "failed",
}
MODELS = {"haiku", "sonnet", "opus"}
RESOLVER_OUTCOMES = {"resolved", "escalated_to_fallback", "escalated_to_user"}

_RUN_ID_RE = re.compile(r"^\d{4}-\d{2}-\d{2}-\d{4}-[a-z0-9-]+$")


def _utcnow() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _state_path(state_dir: Path, run_id: str) -> Path:
    return state_dir / f"parallel-sweep-{run_id}.json"


def make_run_id(slug: str, now: datetime | None = None) -> str:
    """Generate a runID like 2026-04-24-1530-batch-api-migration."""
    when = now or datetime.now(timezone.utc)
    safe = re.sub(r"[^a-z0-9-]+", "-", slug.lower()).strip("-")
    if not safe:
        raise ValueError("slug must contain at least one alphanumeric character")
    return f"{when:%Y-%m-%d-%H%M}-{safe}"


class State:
    """Mutable in-memory mirror of the on-disk state file.

    Every public mutation calls self.checkpoint() implicitly. Reads are direct
    attribute access on self.data. Validation runs on every checkpoint so an
    invalid mutation is caught before it hits disk.
    """

    def __init__(self, state_dir: Path, data: dict[str, Any]):
        self.state_dir = state_dir
        self.data = data
        self.path = _state_path(state_dir, data["runID"])

    # ----- factories -----

    @classmethod
    def create(
        cls,
        state_dir: Path,
        run_id: str,
        user_prompt: str,
        tasks: list[dict[str, Any]],
    ) -> "State":
        if not _RUN_ID_RE.match(run_id):
            raise ValueError(f"runID {run_id!r} does not match expected format")
        state_dir.mkdir(parents=True, exist_ok=True)
        now = _utcnow()
        data = {
            "runID": run_id,
            "createdAt": now,
            "lastCheckpointAt": now,
            "status": "running",
            "userPrompt": user_prompt,
            "tasks": [_normalize_task(t) for t in tasks],
            "siblingRebaseQueue": [],
            "conflictResolverInvocations": [],
        }
        s = cls(state_dir, data)
        s.checkpoint()
        return s

    @classmethod
    def load(cls, state_dir: Path, run_id: str) -> "State":
        path = _state_path(state_dir, run_id)
        with path.open("r", encoding="utf-8") as fh:
            data = json.load(fh)
        s = cls(state_dir, data)
        s._validate()
        return s

    # ----- mutations -----

    def set_status(self, status: str) -> None:
        if status not in TOP_STATUSES:
            raise ValueError(f"invalid top-level status {status!r}")
        self.data["status"] = status
        self.checkpoint()

    def update_task(self, slug: str, **fields: Any) -> dict[str, Any]:
        task = self._task(slug)
        if "status" in fields and fields["status"] not in TASK_STATUSES:
            raise ValueError(f"invalid task status {fields['status']!r}")
        if "model" in fields and fields["model"] not in MODELS:
            raise ValueError(f"invalid model {fields['model']!r}")
        for k, v in fields.items():
            task[k] = v
        task["lastUpdate"] = _utcnow()
        self.checkpoint()
        return task

    def append_task_error(self, slug: str, message: str) -> None:
        task = self._task(slug)
        task.setdefault("errors", []).append(message)
        task["lastUpdate"] = _utcnow()
        self.checkpoint()

    def set_sibling_queue(self, slugs: Iterable[str]) -> None:
        known = {t["slug"] for t in self.data["tasks"]}
        bad = [s for s in slugs if s not in known]
        if bad:
            raise ValueError(f"unknown slugs in sibling queue: {bad}")
        self.data["siblingRebaseQueue"] = list(slugs)
        self.checkpoint()

    def record_resolver_invocation(
        self,
        *,
        task: str,
        triggered_by: str,
        outcome: str,
        subagent_id: str,
        model: str,
        markers_before: int,
        files_affected: int,
    ) -> None:
        if outcome not in RESOLVER_OUTCOMES:
            raise ValueError(f"invalid resolver outcome {outcome!r}")
        if model not in MODELS:
            raise ValueError(f"invalid model {model!r}")
        self.data["conflictResolverInvocations"].append(
            {
                "task": task,
                "triggeredBy": triggered_by,
                "outcome": outcome,
                "subagentID": subagent_id,
                "model": model,
                "markersBefore": markers_before,
                "filesAffected": files_affected,
                "at": _utcnow(),
            }
        )
        self.checkpoint()

    # ----- queries -----

    def task(self, slug: str) -> dict[str, Any]:
        return dict(self._task(slug))

    def tasks_by_status(self, status: str) -> list[dict[str, Any]]:
        if status not in TASK_STATUSES:
            raise ValueError(f"invalid task status {status!r}")
        return [dict(t) for t in self.data["tasks"] if t["status"] == status]

    def is_complete(self) -> bool:
        return all(t["status"] in {"merged", "failed"} for t in self.data["tasks"])

    # ----- internals -----

    def _task(self, slug: str) -> dict[str, Any]:
        for t in self.data["tasks"]:
            if t["slug"] == slug:
                return t
        raise KeyError(f"unknown task slug {slug!r}")

    def checkpoint(self) -> None:
        self.data["lastCheckpointAt"] = _utcnow()
        self._validate()
        # Atomic write: tmp + fsync + os.replace
        fd, tmp_path = tempfile.mkstemp(
            dir=self.state_dir, prefix=f".{self.path.name}.", suffix=".tmp"
        )
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                json.dump(self.data, fh, indent=2, sort_keys=True)
                fh.flush()
                os.fsync(fh.fileno())
            os.replace(tmp_path, self.path)
        except Exception:
            # Best-effort cleanup of orphaned tmp file.
            try:
                os.unlink(tmp_path)
            except FileNotFoundError:
                pass
            raise

    def _validate(self) -> None:
        d = self.data
        for required in ("runID", "createdAt", "lastCheckpointAt", "status", "userPrompt", "tasks"):
            if required not in d:
                raise ValueError(f"state missing required field {required!r}")
        if d["status"] not in TOP_STATUSES:
            raise ValueError(f"invalid top-level status {d['status']!r}")
        slugs: set[str] = set()
        for t in d["tasks"]:
            if t["slug"] in slugs:
                raise ValueError(f"duplicate task slug {t['slug']!r}")
            slugs.add(t["slug"])
            if t["status"] not in TASK_STATUSES:
                raise ValueError(f"invalid task status {t['status']!r} on {t['slug']}")
            if t["model"] not in MODELS:
                raise ValueError(f"invalid model {t['model']!r} on {t['slug']}")


def _normalize_task(raw: dict[str, Any]) -> dict[str, Any]:
    """Apply defaults to a user-supplied task dict."""
    if "slug" not in raw or "description" not in raw:
        raise ValueError("each task needs at least slug and description")
    model = raw.get("model", "haiku")
    if model not in MODELS:
        raise ValueError(f"invalid model {model!r} on task {raw['slug']!r}")
    return {
        "slug": raw["slug"],
        "description": raw["description"],
        "model": model,
        "worktreePath": raw.get("worktreePath"),
        "branch": raw.get("branch", f"refactor/{raw['slug']}"),
        "status": raw.get("status", "pending"),
        "agentID": raw.get("agentID"),
        "prNumber": raw.get("prNumber"),
        "lastUpdate": _utcnow(),
        "errors": list(raw.get("errors", [])),
    }
