#!/usr/bin/env python3
# file: scripts/workflow-debugger.py
# version: 2.1.0
# guid: 9a8b7c6d-5e4f-3a2b-1c0d-9e8f7a6b5c4d

"""
Enhanced Workflow Debugger
=========================

A comprehensive tool for debugging GitHub Actions workflows across repositories.
This script:

1. Discovers failed workflows across specified repositories
2. Downloads and analyzes workflow logs
3. Categorizes failures by type (build, test, dependency, etc.)
4. Generates detailed failure reports with suggested fixes
5. Creates JSON tasks for Copilot to implement fixes
6. Provides comprehensive debugging information

Usage:
    python workflow-debugger.py --repositories jdfalk/repo1,jdfalk/repo2
    python workflow-debugger.py --auto-discover --org jdfalk
    python workflow-debugger.py --all --max-failures 50
"""

import argparse
import json
import logging
import re
import subprocess
import sys
import uuid
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Tuple, Set, Any
from dataclasses import dataclass, asdict


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
    handlers=[logging.StreamHandler()],
)
logger = logging.getLogger(__name__)


@dataclass
class WorkflowRun:
    """Represents a workflow run."""

    id: str
    name: str
    status: str
    conclusion: str
    repository: str
    branch: str
    event: str
    created_at: str
    updated_at: str
    url: str
    run_number: int
    attempt: int


@dataclass
class WorkflowJob:
    """Represents a workflow job."""

    id: str
    name: str
    status: str
    conclusion: str
    url: str
    runner_name: str
    started_at: str
    completed_at: str
    steps: List[Dict[str, Any]]


@dataclass
class FailureAnalysis:
    """Analysis of a workflow failure."""

    workflow_run: WorkflowRun
    failed_jobs: List[WorkflowJob]
    error_patterns: List[str]
    failure_category: str
    root_cause: str
    suggested_fixes: List[str]
    log_snippets: List[str]
    severity: str  # critical, high, medium, low
    is_actionable: bool
    fix_complexity: str  # simple, moderate, complex


@dataclass
class FixTask:
    """Represents a task for Copilot to fix."""

    id: str
    title: str
    description: str
    repository: str
    workflow_file: str
    failure_type: str
    priority: str
    suggested_actions: List[str]
    code_changes_needed: List[Dict[str, str]]
    related_logs: List[str]
    estimated_effort: str
    created_at: str


class WorkflowDebugger:
    """Main workflow debugging class."""

    def __init__(self, output_dir: str = "workflow-debug-output"):
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(exist_ok=True)

        # Create subdirectories
        (self.output_dir / "logs").mkdir(exist_ok=True)
        (self.output_dir / "reports").mkdir(exist_ok=True)
        (self.output_dir / "fix-tasks").mkdir(exist_ok=True)

        self.repositories: Set[str] = set()
        self.workflow_runs: List[WorkflowRun] = []
        self.failure_analyses: List[FailureAnalysis] = []
        self.fix_tasks: List[FixTask] = []

        # Common error patterns and their fixes
        self.error_patterns = {
            "permission_denied": {
                "patterns": [
                    r"permission denied",
                    r"insufficient permissions",
                    r"GITHUB_TOKEN.*permissions",
                    r"403.*Forbidden",
                ],
                "category": "permissions",
                "fixes": [
                    "Add required permissions to workflow file",
                    "Check GITHUB_TOKEN scope",
                    "Verify repository settings",
                ],
            },
            "dependency_issue": {
                "patterns": [
                    r"go: module.*not found",
                    r"npm.*not found",
                    r"pip.*failed",
                    r"cargo.*failed to compile",
                    r"buf.*module.*not found",
                ],
                "category": "dependencies",
                "fixes": [
                    "Update go.mod dependencies",
                    "Run go mod tidy",
                    "Check package.json dependencies",
                    "Update requirements.txt",
                ],
            },
            "build_failure": {
                "patterns": [
                    r"build failed",
                    r"compilation error",
                    r"syntax error",
                    r"undefined reference",
                    r"cannot find symbol",
                ],
                "category": "build",
                "fixes": [
                    "Fix syntax errors in source code",
                    "Resolve missing imports",
                    "Update build configuration",
                    "Check compiler version compatibility",
                ],
            },
            "test_failure": {
                "patterns": [
                    r"test.*failed",
                    r"assertion.*failed",
                    r"expected.*got",
                    r"panic.*test",
                ],
                "category": "testing",
                "fixes": [
                    "Fix failing test cases",
                    "Update test expectations",
                    "Fix test setup/teardown",
                    "Review test data",
                ],
            },
            "timeout": {
                "patterns": [r"timeout", r"killed.*exceeded", r"operation timed out"],
                "category": "timeout",
                "fixes": [
                    "Increase timeout duration",
                    "Optimize slow operations",
                    "Add timeout handling",
                    "Split long-running tasks",
                ],
            },
            "docker_issue": {
                "patterns": [
                    r"docker.*failed",
                    r"container.*exit code",
                    r"image.*not found",
                    r"dockerfile.*error",
                ],
                "category": "docker",
                "fixes": [
                    "Fix Dockerfile syntax",
                    "Update base image",
                    "Check container configuration",
                    "Review docker-compose setup",
                ],
            },
        }

    def run_gh_command(self, args: List[str]) -> str:
        """Run a GitHub CLI command and return output."""
        try:
            cmd = ["gh"] + args
            logger.debug(f"Running: {' '.join(cmd)}")

            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            return result.stdout.strip()
        except subprocess.CalledProcessError as e:
            logger.error(f"GitHub CLI command failed: {e}")
            logger.error(f"stderr: {e.stderr}")
            return ""
        except FileNotFoundError:
            logger.error("GitHub CLI (gh) not found. Please install it first.")
            sys.exit(1)

    def discover_repositories(self, org: str = "jdfalk") -> List[str]:
        """Discover all repositories for the user/org."""
        logger.info(f"Discovering repositories for {org}...")

        output = self.run_gh_command(
            ["repo", "list", org, "--limit", "1000", "--json", "name,owner"]
        )

        if not output:
            return []

        repos = []
        try:
            repo_data = json.loads(output)
            for repo in repo_data:
                repo_name = f"{repo['owner']['login']}/{repo['name']}"
                repos.append(repo_name)
                self.repositories.add(repo_name)
        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse repository list: {e}")

        logger.info(f"Found {len(repos)} repositories")
        return repos

    def get_workflow_runs(
        self, repo: str, status: str = "failure", limit: int = 50
    ) -> List[WorkflowRun]:
        """Get workflow runs for a repository."""
        logger.info(f"Getting workflow runs for {repo} with status {status}...")

        output = self.run_gh_command(
            [
                "run",
                "list",
                "--repo",
                repo,
                "--status",
                status,
                "--limit",
                str(limit),
                "--json",
                "databaseId,name,status,conclusion,headBranch,event,createdAt,updatedAt,url,number,attempt",
            ]
        )

        if not output:
            return []

        runs = []
        try:
            run_data = json.loads(output)
            for run in run_data:
                workflow_run = WorkflowRun(
                    id=str(run["databaseId"]),
                    name=run["name"],
                    status=run["status"],
                    conclusion=run["conclusion"],
                    repository=repo,
                    branch=run["headBranch"],
                    event=run["event"],
                    created_at=run["createdAt"],
                    updated_at=run["updatedAt"],
                    url=run["url"],
                    run_number=run["number"],
                    attempt=run["attempt"],
                )
                runs.append(workflow_run)
        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse workflow runs: {e}")

        return runs

    def get_workflow_jobs(self, repo: str, run_id: str) -> List[WorkflowJob]:
        """Get jobs for a specific workflow run."""
        output = self.run_gh_command(
            ["run", "view", run_id, "--repo", repo, "--json", "jobs"]
        )

        if not output:
            return []

        jobs = []
        try:
            data = json.loads(output)
            for job_data in data.get("jobs", []):
                job = WorkflowJob(
                    id=str(job_data["databaseId"]),
                    name=job_data["name"],
                    status=job_data["status"],
                    conclusion=job_data["conclusion"],
                    url=job_data["url"],
                    runner_name=job_data.get("runnerName", ""),
                    started_at=job_data.get("startedAt", ""),
                    completed_at=job_data.get("completedAt", ""),
                    steps=job_data.get("steps", []),
                )
                jobs.append(job)
        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse workflow jobs: {e}")

        return jobs

    def download_logs(self, repo: str, run_id: str) -> str:
        """Download logs and artifacts for a workflow run."""
        log_file = self.output_dir / "logs" / f"{repo.replace('/', '_')}_{run_id}.log"

        if log_file.exists():
            logger.debug(f"Log file already exists: {log_file}")
            return str(log_file)

        logger.info(f"Downloading logs for run {run_id} in {repo}...")

        # First, get the jobs for this run
        jobs = self.get_workflow_jobs(repo, run_id)

        if not jobs:
            logger.warning(f"No jobs found for run {run_id}")
            return ""

        combined_logs = []

        # Try to download artifacts first
        try:
            import tempfile
            import os

            logger.debug(f"Attempting to download artifacts for run {run_id}")
            with tempfile.TemporaryDirectory() as temp_dir:
                artifact_result = subprocess.run(
                    [
                        "gh",
                        "run",
                        "download",
                        run_id,
                        "--repo",
                        repo,
                        "--dir",
                        temp_dir,
                    ],
                    capture_output=True,
                    text=True,
                )

                if artifact_result.returncode == 0:
                    combined_logs.append("\n=== DOWNLOADED ARTIFACTS ===\n")

                    # Read artifact files
                    for root, dirs, files in os.walk(temp_dir):
                        for file in files:
                            file_path = os.path.join(root, file)
                            try:
                                with open(
                                    file_path, "r", encoding="utf-8", errors="ignore"
                                ) as f:
                                    content = f.read()
                                    combined_logs.append(
                                        f"\n--- ARTIFACT FILE: {file} ---\n"
                                    )
                                    combined_logs.append(content)
                            except Exception as e:
                                logger.debug(
                                    f"Could not read artifact file {file}: {e}"
                                )
                else:
                    logger.debug(
                        f"No artifacts found or could not download: {artifact_result.stderr}"
                    )

        except Exception as e:
            logger.debug(f"Could not download artifacts: {e}")

        # Download logs for each job
        for job in jobs:
            logger.debug(f"Getting logs for job {job.name} ({job.id})")

            # Method 1: Use gh run view with --log flag
            result = subprocess.run(
                ["gh", "run", "view", run_id, "--repo", repo, "--job", job.id, "--log"],
                capture_output=True,
                text=True,
            )

            if result.returncode == 0 and result.stdout.strip():
                combined_logs.append(f"\n=== JOB: {job.name} ===\n")
                combined_logs.append(result.stdout)
            else:
                # Method 2: Try using gh api to get logs directly
                try:
                    api_result = subprocess.run(
                        ["gh", "api", f"/repos/{repo}/actions/jobs/{job.id}/logs"],
                        capture_output=True,
                        text=True,
                    )

                    if api_result.returncode == 0 and api_result.stdout.strip():
                        combined_logs.append(f"\n=== JOB: {job.name} ===\n")
                        combined_logs.append(api_result.stdout)
                    else:
                        logger.warning(
                            f"Could not get logs for job {job.name}: API returned {api_result.returncode}"
                        )
                        combined_logs.append(
                            f"\n=== JOB: {job.name} (FAILED TO GET LOGS) ===\n"
                        )

                        # Add step failure information
                        self._add_job_failure_info(combined_logs, job)

                except Exception as e:
                    logger.warning(
                        f"Failed to get logs via API for job {job.name}: {e}"
                    )
                    combined_logs.append(f"\n=== JOB: {job.name} (API ERROR) ===\n")
                    self._add_job_failure_info(combined_logs, job)

        # Save combined logs
        try:
            log_file.parent.mkdir(parents=True, exist_ok=True)
            with open(log_file, "w", encoding="utf-8") as f:
                f.write("\n".join(combined_logs))

            if combined_logs:
                logger.info(f"Saved logs to {log_file} ({len(combined_logs)} sections)")
                return str(log_file)
            else:
                logger.warning(f"No logs were downloaded for run {run_id}")
                return ""

        except Exception as e:
            logger.error(f"Failed to save combined logs: {e}")
            return ""

    def _add_job_failure_info(self, combined_logs: List[str], job: WorkflowJob):
        """Add failure information when logs are not available."""
        combined_logs.append(f"Job Status: {job.status}\n")
        combined_logs.append(f"Job Conclusion: {job.conclusion}\n")
        combined_logs.append(f"Job URL: {job.url}\n")

        if job.steps:
            combined_logs.append("Steps:\n")
            for step in job.steps:
                step_status = step.get("status", "unknown")
                step_conclusion = step.get("conclusion", "unknown")
                step_name = step.get("name", "Unknown Step")

                combined_logs.append(
                    f"  - {step_name}: {step_status}/{step_conclusion}\n"
                )

                if step_conclusion == "failure":
                    combined_logs.append("    ^^^ FAILED STEP ^^^\n")
        else:
            combined_logs.append("No step information available\n")

    def analyze_logs(self, log_file: str) -> Tuple[List[str], str, List[str]]:
        """Analyze logs to identify error patterns and root causes."""
        if not log_file or not Path(log_file).exists():
            return [], "unknown", []

        try:
            with open(log_file, "r", encoding="utf-8", errors="ignore") as f:
                content = f.read()
        except Exception as e:
            logger.error(f"Failed to read log file {log_file}: {e}")
            return [], "unknown", []

        found_patterns = []
        categories = set()
        log_snippets = []

        # Split content into lines for analysis
        lines = content.split("\n")

        # Find error lines (lines containing error keywords)
        error_lines = []
        for i, line in enumerate(lines):
            if any(
                keyword in line.lower()
                for keyword in ["error", "failed", "panic", "fatal"]
            ):
                # Include context (2 lines before and after)
                start = max(0, i - 2)
                end = min(len(lines), i + 3)
                context = "\n".join(lines[start:end])
                error_lines.append(context)

        # Limit to most relevant error snippets
        log_snippets = error_lines[:10]

        # Check for known patterns
        for pattern_name, pattern_info in self.error_patterns.items():
            for pattern in pattern_info["patterns"]:
                if re.search(pattern, content, re.IGNORECASE):
                    found_patterns.append(pattern_name)
                    categories.add(pattern_info["category"])

        # Determine primary category
        if categories:
            primary_category = list(categories)[0]  # Take first category
        else:
            primary_category = "unknown"

        return found_patterns, primary_category, log_snippets

    def analyze_failure(
        self, workflow_run: WorkflowRun, failed_jobs: List[WorkflowJob], log_file: str
    ) -> FailureAnalysis:
        """Analyze a workflow failure comprehensively."""
        logger.info(
            f"Analyzing failure for workflow {workflow_run.name} (run {workflow_run.id})"
        )

        error_patterns, failure_category, log_snippets = self.analyze_logs(log_file)

        # Determine root cause and suggested fixes
        root_cause = "Unknown failure"
        suggested_fixes = []
        severity = "medium"
        is_actionable = True
        fix_complexity = "moderate"

        if error_patterns:
            # Get fixes for the first pattern found
            first_pattern = error_patterns[0]
            if first_pattern in self.error_patterns:
                pattern_info = self.error_patterns[first_pattern]
                suggested_fixes = pattern_info["fixes"]
                root_cause = f"Detected {pattern_info['category']} issue"

                # Set severity based on category
                if pattern_info["category"] in ["permissions", "dependencies"]:
                    severity = "high"
                    fix_complexity = "simple"
                elif pattern_info["category"] in ["build", "testing"]:
                    severity = "medium"
                    fix_complexity = "moderate"
                else:
                    severity = "low"
                    fix_complexity = "complex"

        # Check if this is actionable (not infrastructure/runner issues)
        infrastructure_keywords = [
            "runner",
            "github actions",
            "internal server error",
            "service unavailable",
            "network",
            "timeout.*github",
        ]

        log_content = "\n".join(log_snippets).lower()
        if any(re.search(keyword, log_content) for keyword in infrastructure_keywords):
            is_actionable = False
            severity = "low"
            root_cause = "Infrastructure/runner issue (not actionable)"

        return FailureAnalysis(
            workflow_run=workflow_run,
            failed_jobs=failed_jobs,
            error_patterns=error_patterns,
            failure_category=failure_category,
            root_cause=root_cause,
            suggested_fixes=suggested_fixes,
            log_snippets=log_snippets,
            severity=severity,
            is_actionable=is_actionable,
            fix_complexity=fix_complexity,
        )

    def generate_fix_task(self, analysis: FailureAnalysis) -> FixTask:
        """Generate a fix task for Copilot based on failure analysis."""
        workflow_run = analysis.workflow_run

        task_id = str(uuid.uuid4())
        title = f"Fix {analysis.failure_category} issue in {workflow_run.name}"

        description = f"""
Workflow Failure Analysis
========================

**Workflow**: {workflow_run.name}
**Repository**: {workflow_run.repository}
**Run ID**: {workflow_run.id}
**Branch**: {workflow_run.branch}
**Status**: {workflow_run.conclusion}
**Created**: {workflow_run.created_at}

**Root Cause**: {analysis.root_cause}
**Category**: {analysis.failure_category}
**Severity**: {analysis.severity}
**Actionable**: {analysis.is_actionable}

**Error Patterns Detected**:
{chr(10).join(f"- {pattern}" for pattern in analysis.error_patterns)}

**Failed Jobs**:
{chr(10).join(f"- {job.name} ({job.conclusion})" for job in analysis.failed_jobs)}

**Log Snippets**:
```
{chr(10).join(analysis.log_snippets[:3])}
```

**Workflow URL**: {workflow_run.url}
        """.strip()

        # Generate specific code changes needed
        code_changes = []
        if analysis.failure_category == "permissions":
            code_changes.append(
                {
                    "file": ".github/workflows/*.yml",
                    "change": "Add required permissions block",
                    "example": """permissions:
  contents: read
  pull-requests: write
  issues: write""",
                }
            )
        elif analysis.failure_category == "dependencies":
            code_changes.append(
                {
                    "file": "go.mod",
                    "change": "Update dependencies",
                    "example": "Run 'go mod tidy' and update versions",
                }
            )

        # Determine priority based on severity and actionability
        if analysis.is_actionable and analysis.severity == "high":
            priority = "urgent"
        elif analysis.is_actionable and analysis.severity == "medium":
            priority = "high"
        elif analysis.is_actionable:
            priority = "medium"
        else:
            priority = "low"

        return FixTask(
            id=task_id,
            title=title,
            description=description,
            repository=workflow_run.repository,
            workflow_file=workflow_run.name,
            failure_type=analysis.failure_category,
            priority=priority,
            suggested_actions=analysis.suggested_fixes,
            code_changes_needed=code_changes,
            related_logs=[
                f"logs/{workflow_run.repository.replace('/', '_')}_{workflow_run.id}.log"
            ],
            estimated_effort=analysis.fix_complexity,
            created_at=datetime.now().isoformat(),
        )

    def scan_repositories(self, repositories: List[str], days_back: int = 7) -> None:
        """Scan repositories for failing workflows."""
        logger.info(
            f"Scanning {len(repositories)} repositories for failures in the last {days_back} days..."
        )

        for repo in repositories:
            logger.info(f"Scanning repository: {repo}")

            # Get failed workflow runs
            failed_runs = self.get_workflow_runs(repo, "failure", limit=20)

            # Filter by date if specified
            if days_back > 0:
                cutoff_date = datetime.now().replace(tzinfo=None) - timedelta(
                    days=days_back
                )
                failed_runs = [
                    run
                    for run in failed_runs
                    if datetime.fromisoformat(
                        run.created_at.replace("Z", "").replace("+00:00", "")
                    )
                    > cutoff_date
                ]

            if not failed_runs:
                logger.info(f"No recent failures found in {repo}")
                continue

            logger.info(f"Found {len(failed_runs)} recent failures in {repo}")
            self.workflow_runs.extend(failed_runs)

            # Analyze each failure
            for run in failed_runs:
                logger.info(f"Analyzing run {run.id}: {run.name}")

                # Get failed jobs
                all_jobs = self.get_workflow_jobs(repo, run.id)
                failed_jobs = [job for job in all_jobs if job.conclusion == "failure"]

                # Download logs
                log_file = self.download_logs(repo, run.id)

                # Analyze the failure
                analysis = self.analyze_failure(run, failed_jobs, log_file)
                self.failure_analyses.append(analysis)

                # Generate fix task if actionable
                if analysis.is_actionable:
                    fix_task = self.generate_fix_task(analysis)
                    self.fix_tasks.append(fix_task)

    def save_results(self) -> None:
        """Save all results to output files."""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

        # Save failure analyses
        analyses_file = (
            self.output_dir / "reports" / f"failure_analyses_{timestamp}.json"
        )
        with open(analyses_file, "w") as f:
            json.dump(
                [asdict(analysis) for analysis in self.failure_analyses], f, indent=2
            )
        logger.info(f"Saved failure analyses to {analyses_file}")

        # Save fix tasks
        tasks_file = (
            self.output_dir / "fix-tasks" / f"copilot_fix_tasks_{timestamp}.json"
        )
        with open(tasks_file, "w") as f:
            json.dump([asdict(task) for task in self.fix_tasks], f, indent=2)
        logger.info(f"Saved fix tasks to {tasks_file}")

        # Generate summary report
        self.generate_summary_report(timestamp)

    def generate_summary_report(self, timestamp: str) -> None:
        """Generate a human-readable summary report."""
        report_file = self.output_dir / "reports" / f"summary_report_{timestamp}.md"

        # Count failures by category
        category_counts = {}
        actionable_count = 0
        severity_counts = {"critical": 0, "high": 0, "medium": 0, "low": 0}

        for analysis in self.failure_analyses:
            category = analysis.failure_category
            category_counts[category] = category_counts.get(category, 0) + 1

            if analysis.is_actionable:
                actionable_count += 1

            severity_counts[analysis.severity] += 1

        # Generate markdown report
        report = f"""# Workflow Failure Analysis Report

**Generated**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}
**Repositories Scanned**: {len(self.repositories)}
**Total Failures**: {len(self.failure_analyses)}
**Actionable Failures**: {actionable_count}
**Fix Tasks Generated**: {len(self.fix_tasks)}

## Failure Categories

| Category | Count | Percentage |
|----------|-------|------------|
"""

        total_failures = len(self.failure_analyses)
        for category, count in sorted(category_counts.items()):
            percentage = (count / total_failures * 100) if total_failures > 0 else 0
            report += f"| {category.title()} | {count} | {percentage:.1f}% |\n"

        report += """
## Severity Distribution

| Severity | Count |
|----------|-------|
"""

        for severity, count in severity_counts.items():
            if count > 0:
                report += f"| {severity.title()} | {count} |\n"

        report += """
## Top Issues to Fix

"""

        # List top actionable issues
        actionable_analyses = [a for a in self.failure_analyses if a.is_actionable]
        actionable_analyses.sort(
            key=lambda x: (x.severity == "high", x.severity == "medium")
        )

        for i, analysis in enumerate(actionable_analyses[:10], 1):
            run = analysis.workflow_run
            report += f"""
### {i}. {run.name} in {run.repository}

- **Category**: {analysis.failure_category}
- **Severity**: {analysis.severity}
- **Root Cause**: {analysis.root_cause}
- **URL**: {run.url}

**Suggested Fixes**:
{chr(10).join(f"- {fix}" for fix in analysis.suggested_fixes)}
"""

        report += f"""
## Generated Fix Tasks

{len(self.fix_tasks)} fix tasks have been generated for Copilot to address the actionable failures.

**High Priority Tasks**: {len([t for t in self.fix_tasks if t.priority in ["urgent", "high"]])}
**Medium Priority Tasks**: {len([t for t in self.fix_tasks if t.priority == "medium"])}
**Low Priority Tasks**: {len([t for t in self.fix_tasks if t.priority == "low"])}

## Files Generated

- **Failure Analyses**: `reports/failure_analyses_{timestamp}.json`
- **Fix Tasks**: `fix-tasks/copilot_fix_tasks_{timestamp}.json`
- **Logs**: `logs/` directory
- **Summary**: `reports/summary_report_{timestamp}.md` (this file)

## Next Steps

1. Review the generated fix tasks in `fix-tasks/copilot_fix_tasks_{timestamp}.json`
2. Use these tasks with your Copilot workflow to automatically fix issues
3. Monitor workflow runs after fixes are applied
4. Re-run this analysis to verify fixes worked

---
*Generated by workflow-debugger.py*
"""

        with open(report_file, "w") as f:
            f.write(report)

        logger.info(f"Generated summary report: {report_file}")


def main():
    parser = argparse.ArgumentParser(
        description="Ultimate GitHub Workflow Debugger and Fixer"
    )

    # Main modes
    parser.add_argument(
        "--scan-all",
        action="store_true",
        help="Scan all repositories for workflow failures",
    )
    parser.add_argument("--repo", help="Scan specific repository (format: owner/repo)")
    parser.add_argument(
        "--recent-failures", action="store_true", help="Focus on recent failures only"
    )

    # Configuration
    parser.add_argument(
        "--days",
        type=int,
        default=7,
        help="Number of days back to scan for failures (default: 7)",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=50,
        help="Maximum number of workflow runs to analyze per repo (default: 50)",
    )
    parser.add_argument(
        "--org",
        default="jdfalk",
        help="GitHub organization/user to scan (default: jdfalk)",
    )
    parser.add_argument(
        "--output-dir",
        default="workflow-debug-output",
        help="Output directory for results (default: workflow-debug-output)",
    )

    # Options
    parser.add_argument(
        "--fix-tasks",
        action="store_true",
        help="Generate fix tasks for Copilot (default: true)",
    )
    parser.add_argument(
        "--actionable-only",
        action="store_true",
        help="Only analyze actionable failures (skip infrastructure issues)",
    )
    parser.add_argument("--verbose", action="store_true", help="Enable verbose logging")

    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    # Initialize debugger
    debugger = WorkflowDebugger(args.output_dir)

    # Determine repositories to scan
    repositories = []
    if args.scan_all:
        repositories = debugger.discover_repositories(args.org)
    elif args.repo:
        repositories = [args.repo]
    else:
        # Default to common repositories
        repositories = [
            f"{args.org}/gcommon",
            f"{args.org}/ghcommon",
            f"{args.org}/subtitle-manager",
            f"{args.org}/copilot-agent-util-rust",
        ]

    if not repositories:
        logger.error("No repositories to scan. Use --scan-all or --repo")
        return 1

    logger.info("Starting workflow failure analysis...")
    logger.info(f"Repositories: {repositories}")
    logger.info(f"Days back: {args.days}")
    logger.info(f"Output directory: {args.output_dir}")

    # Scan repositories
    debugger.scan_repositories(repositories, args.days)

    # Filter results if requested
    if args.actionable_only:
        debugger.failure_analyses = [
            a for a in debugger.failure_analyses if a.is_actionable
        ]
        debugger.fix_tasks = [
            t for t in debugger.fix_tasks if t.priority in ["urgent", "high", "medium"]
        ]

    # Save results
    debugger.save_results()

    # Print summary
    total_failures = len(debugger.failure_analyses)
    actionable_failures = len([a for a in debugger.failure_analyses if a.is_actionable])
    fix_tasks = len(debugger.fix_tasks)

    print(f"\n{'=' * 80}")
    print("WORKFLOW FAILURE ANALYSIS COMPLETE")
    print(f"{'=' * 80}")
    print(f"📊 Total Failures Analyzed: {total_failures}")
    print(f"🔧 Actionable Failures: {actionable_failures}")
    print(f"📝 Fix Tasks Generated: {fix_tasks}")
    print(f"📁 Output Directory: {args.output_dir}")
    print(f"{'=' * 80}")

    if fix_tasks > 0:
        print(f"\n✅ Generated {fix_tasks} fix tasks for Copilot!")
        print(f"📂 Check {args.output_dir}/fix-tasks/ for JSON task files")
        print(f"📋 Review {args.output_dir}/reports/ for detailed analysis")
    else:
        print("\n❌ No actionable failures found to generate fix tasks")

    return 0


if __name__ == "__main__":
    sys.exit(main())
