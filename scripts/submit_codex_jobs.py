#!/usr/bin/env python3
# file: scripts/submit_codex_jobs.py
# version: 1.0.0
# guid: 9c1d77e9-84a6-4b9b-9e02-d8f6c5bfa519
"""Submit codex jobs described in a JSON file.

Each job object must include:
- uuid: unique job identifier
- repo: repository in owner/name form
- instructions: text instructions for codex
- priority: job priority (passed through)
- repeat (optional): if true, submit even if uuid was submitted before

The script maintains a local ledger of submitted UUIDs in
``.codex_submitted_jobs.json`` to prevent accidental duplicates.
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Set

LEDGER_PATH = Path(".codex_submitted_jobs.json")

def load_ledger() -> Set[str]:
    """Load the set of previously submitted job UUIDs."""
    if LEDGER_PATH.exists():
        with LEDGER_PATH.open("r", encoding="utf-8") as handle:
            return set(json.load(handle))
    return set()

def save_ledger(ledger: Set[str]) -> None:
    """Persist the ledger of submitted job UUIDs."""
    with LEDGER_PATH.open("w", encoding="utf-8") as handle:
        json.dump(sorted(ledger), handle, indent=2)

def submit_job(job: Dict[str, Any]) -> None:
    """Submit a single job to the codex CLI."""
    repo = job["repo"]
    instructions = job["instructions"]
    priority = str(job.get("priority", "normal"))
    job_id = job["uuid"]
    cmd = [
        "codex",
        "submit",
        "--repo",
        repo,
        "--instructions",
        instructions,
        "--priority",
        priority,
        "--uuid",
        job_id,
    ]
    subprocess.run(cmd, check=True)

def process_jobs(jobs: List[Dict[str, Any]], ledger: Set[str]) -> None:
    """Process and submit all jobs."""
    for job in jobs:
        job_id = job["uuid"]
        repeat = bool(job.get("repeat", False))
        if job_id in ledger and not repeat:
            print(f"Skipping {job_id}: already submitted", file=sys.stderr)
            continue
        submit_job(job)
        ledger.add(job_id)

def main() -> int:
    """Program entry point."""
    parser = argparse.ArgumentParser(
        description="Submit codex jobs from a JSON file."
    )
    parser.add_argument("path", type=Path, help="Path to JSON job file.")
    args = parser.parse_args()

    with args.path.open("r", encoding="utf-8") as handle:
        jobs: List[Dict[str, Any]] = json.load(handle)

    ledger = load_ledger()
    process_jobs(jobs, ledger)
    save_ledger(ledger)
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
