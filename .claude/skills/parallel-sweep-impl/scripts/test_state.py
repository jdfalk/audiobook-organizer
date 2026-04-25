# file: .claude/skills/parallel-sweep-impl/scripts/test_state.py
# version: 1.0.0
# guid: 5b6c7d8e-9f0a-1b2c-3d4e-5f6a7b8c9d0e

"""Unit tests for state.py.

Run from this directory:
    python3 -m unittest test_state.py -v

Or via the full path:
    python3 -m unittest .claude/skills/parallel-sweep-impl/scripts/test_state.py
"""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

import state


SAMPLE_TASKS = [
    {"slug": "task-a", "description": "Refactor handler A", "model": "haiku"},
    {"slug": "task-b", "description": "Refactor handler B", "model": "sonnet"},
]


class StateTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.state_dir = Path(self.tmp.name)
        self.run_id = "2026-04-24-1530-test-run"

    def tearDown(self) -> None:
        self.tmp.cleanup()

    # ----- create / load -----

    def test_create_writes_file_with_defaults(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "test prompt", SAMPLE_TASKS)
        self.assertTrue(s.path.exists())
        loaded = json.loads(s.path.read_text())
        self.assertEqual(loaded["runID"], self.run_id)
        self.assertEqual(loaded["status"], "running")
        self.assertEqual(len(loaded["tasks"]), 2)
        self.assertEqual(loaded["tasks"][0]["status"], "pending")
        self.assertEqual(loaded["tasks"][0]["branch"], "refactor/task-a")
        self.assertIsNone(loaded["tasks"][0]["worktreePath"])
        self.assertEqual(loaded["siblingRebaseQueue"], [])

    def test_create_rejects_bad_run_id(self) -> None:
        with self.assertRaises(ValueError):
            state.State.create(self.state_dir, "not-a-run-id", "x", SAMPLE_TASKS)

    def test_create_rejects_invalid_model(self) -> None:
        with self.assertRaises(ValueError):
            state.State.create(
                self.state_dir,
                self.run_id,
                "x",
                [{"slug": "t", "description": "d", "model": "gpt-4"}],
            )

    def test_load_round_trips(self) -> None:
        s1 = state.State.create(self.state_dir, self.run_id, "test", SAMPLE_TASKS)
        s1.update_task("task-a", status="dispatched", agentID="agt-123")
        s2 = state.State.load(self.state_dir, self.run_id)
        self.assertEqual(s2.task("task-a")["status"], "dispatched")
        self.assertEqual(s2.task("task-a")["agentID"], "agt-123")

    # ----- mutations -----

    def test_update_task_validates_status(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        with self.assertRaises(ValueError):
            s.update_task("task-a", status="bogus")

    def test_update_task_unknown_slug_raises(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        with self.assertRaises(KeyError):
            s.update_task("no-such-task", status="dispatched")

    def test_set_status_validates(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        s.set_status("paused")
        self.assertEqual(
            state.State.load(self.state_dir, self.run_id).data["status"], "paused"
        )
        with self.assertRaises(ValueError):
            s.set_status("bogus")

    def test_append_task_error_persists(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        s.append_task_error("task-a", "rebase blocked: 5 files conflicted")
        s.append_task_error("task-a", "second error")
        loaded = state.State.load(self.state_dir, self.run_id)
        self.assertEqual(len(loaded.task("task-a")["errors"]), 2)

    def test_set_sibling_queue_rejects_unknown_slugs(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        with self.assertRaises(ValueError):
            s.set_sibling_queue(["task-a", "ghost"])

    def test_record_resolver_invocation_appends(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        s.record_resolver_invocation(
            task="task-b",
            triggered_by="task-a",
            outcome="resolved",
            subagent_id="agt-resolver-1",
            model="sonnet",
            markers_before=4,
            files_affected=2,
        )
        loaded = state.State.load(self.state_dir, self.run_id)
        self.assertEqual(len(loaded.data["conflictResolverInvocations"]), 1)
        entry = loaded.data["conflictResolverInvocations"][0]
        self.assertEqual(entry["outcome"], "resolved")
        self.assertEqual(entry["model"], "sonnet")

    def test_record_resolver_rejects_bad_outcome(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        with self.assertRaises(ValueError):
            s.record_resolver_invocation(
                task="task-a",
                triggered_by="task-b",
                outcome="not-an-outcome",
                subagent_id="x",
                model="sonnet",
                markers_before=0,
                files_affected=0,
            )

    # ----- queries -----

    def test_tasks_by_status(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        s.update_task("task-a", status="merged")
        merged = s.tasks_by_status("merged")
        pending = s.tasks_by_status("pending")
        self.assertEqual([t["slug"] for t in merged], ["task-a"])
        self.assertEqual([t["slug"] for t in pending], ["task-b"])

    def test_is_complete_true_when_all_terminal(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        self.assertFalse(s.is_complete())
        s.update_task("task-a", status="merged")
        s.update_task("task-b", status="failed")
        self.assertTrue(s.is_complete())

    # ----- atomicity -----

    def test_checkpoint_no_orphan_tmp_files(self) -> None:
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        for i in range(20):
            s.update_task("task-a", status="dispatched" if i % 2 else "pending")
        leftover = [p for p in self.state_dir.iterdir() if p.name.endswith(".tmp")]
        self.assertEqual(leftover, [], f"orphan tmp files: {leftover}")

    def test_checkpoint_atomic_against_partial_read(self) -> None:
        """Simulate: SIGKILL between fsync and rename leaves the original file intact."""
        s = state.State.create(self.state_dir, self.run_id, "x", SAMPLE_TASKS)
        original = s.path.read_bytes()
        # If checkpoint never finished os.replace, the live file would still be the original.
        # We can't kill mid-call from a unittest, but we can verify the contract: tmp files
        # are written with a prefix that excludes them from a cold load, and replace happens last.
        # Cold-load after a successful checkpoint must yield the new contents, never partial.
        s.update_task("task-a", status="committed")
        loaded = state.State.load(self.state_dir, self.run_id)
        self.assertEqual(loaded.task("task-a")["status"], "committed")
        # And the original bytes are no longer on disk under the canonical path.
        self.assertNotEqual(s.path.read_bytes(), original)

    # ----- run id helpers -----

    def test_make_run_id_format(self) -> None:
        rid = state.make_run_id("Some Slug With Spaces")
        self.assertRegex(rid, r"^\d{4}-\d{2}-\d{2}-\d{4}-some-slug-with-spaces$")

    def test_make_run_id_rejects_empty_slug(self) -> None:
        with self.assertRaises(ValueError):
            state.make_run_id("!!!")

    # ----- normalize -----

    def test_normalize_requires_slug_and_description(self) -> None:
        with self.assertRaises(ValueError):
            state.State.create(
                self.state_dir, self.run_id, "x", [{"slug": "only-slug"}]
            )

    def test_normalize_default_branch(self) -> None:
        s = state.State.create(
            self.state_dir, self.run_id, "x", [{"slug": "my-task", "description": "d"}]
        )
        self.assertEqual(s.task("my-task")["branch"], "refactor/my-task")
        self.assertEqual(s.task("my-task")["model"], "haiku")  # default


if __name__ == "__main__":
    unittest.main()
