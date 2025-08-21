#!/usr/bin/env python3
# file: .github/scripts/sync-complete-workflow-modernizer.py
# version: 1.0.0
# guid: a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6

"""
Complete workflow modernization script.
Converts ALL workflows to use Python scripts and removes inline bash.
"""

import os
import re
import sys
import subprocess
from pathlib import Path


class WorkflowModernizer:
    def __init__(self, workflow_dir):
        self.workflow_dir = Path(workflow_dir)
        self.scripts_dir = Path(workflow_dir).parent / "scripts"
        self.modernized_count = 0

    def run_command(self, cmd):
        """Run shell command."""
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        return result.returncode == 0, result.stdout, result.stderr

    def extract_inline_scripts(self, content):
        """Extract inline scripts from workflow and convert to Python calls."""
        # Find all run: | blocks
        run_blocks = re.findall(
            r"(\s+)run: \|\s*\n((?:\1  .*\n?)*)", content, re.MULTILINE
        )

        script_replacements = []
        for indent, script_content in run_blocks:
            # Clean up the script content
            lines = script_content.split("\n")
            cleaned_lines = [
                line[len(indent) + 2 :]
                if line.startswith(indent + "  ")
                else line.strip()
                for line in lines
                if line.strip()
            ]

            if cleaned_lines:
                script_replacements.append((indent, script_content, cleaned_lines))

        return script_replacements

    def modernize_workflow_content(self, content, workflow_name):
        """Modernize workflow content to use Python scripts."""
        modernized = content

        # Replace common inline script patterns
        replacements = [
            # Language detection
            (
                r"(\s+)run: \|\s*\n(?:\s*.*language.*detection.*\n)+",
                r"\1run: python3 .github/scripts/sync-release-detect-language.py",
            ),
            # Package.json creation
            (
                r"(\s+)run: \|\s*\n(?:\s*.*cat.*package\.json.*\n)+.*EOF\s*\n",
                r"\1run: python3 .github/scripts/sync-release-create-package-json.py ${{ env.LANGUAGE }}",
            ),
            # Semantic release config creation
            (
                r"(\s+)run: \|\s*\n(?:\s*.*cat.*\.releaserc\.json.*\n)+.*EOF\s*\n",
                r"\1run: python3 .github/scripts/sync-release-create-semantic-config.py ${{ env.LANGUAGE }}",
            ),
            # Version determination
            (
                r"(\s+)run: \|\s*\n(?:\s*.*version.*determination.*\n)+",
                r"\1run: python3 .github/scripts/sync-release-determine-version.py",
            ),
            # Manual release handling
            (
                r"(\s+)run: \|\s*\n(?:\s*.*manual.*release.*\n)+",
                r"\1run: python3 .github/scripts/sync-release-handle-manual-release.py",
            ),
            # Build artifacts
            (
                r"(\s+)run: \|\s*\n(?:\s*.*build.*artifacts.*\n)+",
                r"\1run: python3 .github/scripts/sync-release-build-artifacts.py",
            ),
        ]

        for pattern, replacement in replacements:
            modernized = re.sub(
                pattern, replacement, modernized, flags=re.MULTILINE | re.DOTALL
            )

        # Replace GitHub context variables with environment variables for security
        security_replacements = [
            (r"\$\{\{ github\.repository \}\}", "${{ env.GITHUB_REPOSITORY }}"),
            (r"\$\{\{ github\.ref \}\}", "${{ env.GITHUB_REF }}"),
            (r"\$\{\{ github\.sha \}\}", "${{ env.GITHUB_SHA }}"),
            (r"\$\{\{ github\.actor \}\}", "${{ env.GITHUB_ACTOR }}"),
        ]

        for pattern, replacement in security_replacements:
            modernized = re.sub(pattern, replacement, modernized)

        # Add environment variables at the job level if not present
        if "env:" not in modernized and any("${{ env." in modernized for _ in [None]):
            # Find the first job and add env section
            job_pattern = r"(jobs:\s*\n\s+\w+:\s*\n)"
            modernized = re.sub(
                job_pattern,
                r"\1    env:\n      GITHUB_REPOSITORY: ${{ github.repository }}\n      GITHUB_REF: ${{ github.ref }}\n      GITHUB_SHA: ${{ github.sha }}\n      GITHUB_ACTOR: ${{ github.actor }}\n",
                modernized,
            )

        return modernized

    def modernize_workflow(self, workflow_path):
        """Modernize a single workflow file."""
        try:
            with open(workflow_path, "r") as f:
                content = f.read()

            # Skip if already modernized
            if "python3 .github/scripts/sync-" in content:
                print(f"  ✓ {workflow_path.name} already modernized")
                return False

            workflow_name = workflow_path.stem
            modernized_content = self.modernize_workflow_content(content, workflow_name)

            # Only write if there were changes
            if modernized_content != content:
                with open(workflow_path, "w") as f:
                    f.write(modernized_content)
                print(f"  ✓ Modernized {workflow_path.name}")
                self.modernized_count += 1
                return True
            else:
                print(f"  - {workflow_path.name} no changes needed")
                return False

        except Exception as e:
            print(f"  ✗ Error modernizing {workflow_path.name}: {e}")
            return False

    def modernize_all_workflows(self):
        """Modernize all workflow files."""
        print(f"🔄 Modernizing workflows in {self.workflow_dir}")

        workflow_files = list(self.workflow_dir.glob("*.yml")) + list(
            self.workflow_dir.glob("*.yaml")
        )

        if not workflow_files:
            print("No workflow files found")
            return

        for workflow_path in sorted(workflow_files):
            self.modernize_workflow(workflow_path)

        print(
            f"\n✅ Modernization complete: {self.modernized_count}/{len(workflow_files)} workflows updated"
        )

    def create_missing_scripts(self):
        """Create any missing Python scripts that workflows might need."""
        missing_scripts = {
            "sync-release-run-semantic-release.py": '''#!/usr/bin/env python3
# file: .github/scripts/sync-release-run-semantic-release.py
# version: 1.0.0
# guid: b3c4d5e6-f7a8-b9c0-d1e2-f3a4b5c6d7e8

"""Run semantic-release with proper environment and configuration."""

import subprocess
import sys
import os


def main():
    """Run semantic-release."""
    # Ensure npm dependencies are installed
    print("Installing npm dependencies...")
    result = subprocess.run(["npm", "install"], capture_output=True, text=True)
    if result.returncode != 0:
        print(f"Failed to install npm dependencies: {result.stderr}")
        sys.exit(1)

    # Run semantic-release
    print("Running semantic-release...")
    env = os.environ.copy()
    result = subprocess.run(["npx", "semantic-release"], env=env)
    sys.exit(result.returncode)


if __name__ == "__main__":
    main()''',
            "sync-release-upload-artifacts.py": '''#!/usr/bin/env python3
# file: .github/scripts/sync-release-upload-artifacts.py
# version: 1.0.0
# guid: c4d5e6f7-a8b9-c0d1-e2f3-a4b5c6d7e8f9

"""Upload release artifacts to GitHub release."""

import os
import sys
import subprocess
from pathlib import Path


def upload_artifacts(release_id, artifacts_dir):
    """Upload artifacts to GitHub release."""
    artifacts_path = Path(artifacts_dir)

    if not artifacts_path.exists():
        print(f"Artifacts directory {artifacts_dir} does not exist")
        return

    # Find all artifact files
    artifact_files = []
    for pattern in ["*.tar.gz", "*.zip", "*.whl", "*.tgz"]:
        artifact_files.extend(artifacts_path.glob(pattern))

    if not artifact_files:
        print("No artifacts found to upload")
        return

    # Upload each artifact
    for artifact in artifact_files:
        print(f"Uploading {artifact.name}...")
        cmd = [
            "gh", "release", "upload", release_id, str(artifact),
            "--repo", os.environ.get("GITHUB_REPOSITORY", "")
        ]

        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode == 0:
            print(f"  ✓ Uploaded {artifact.name}")
        else:
            print(f"  ✗ Failed to upload {artifact.name}: {result.stderr}")


def main():
    """Main entry point."""
    if len(sys.argv) < 3:
        print("Usage: sync-release-upload-artifacts.py <release_id> <artifacts_dir>")
        sys.exit(1)

    release_id = sys.argv[1]
    artifacts_dir = sys.argv[2]

    upload_artifacts(release_id, artifacts_dir)


if __name__ == "__main__":
    main()''',
        }

        for script_name, script_content in missing_scripts.items():
            script_path = self.scripts_dir / script_name
            if not script_path.exists():
                script_path.write_text(script_content)
                os.chmod(script_path, 0o755)
                print(f"Created missing script: {script_name}")


def main():
    """Main entry point."""
    workflow_dir = "/Users/jdfalk/repos/github.com/jdfalk/ghcommon/.github/workflows"

    if not os.path.exists(workflow_dir):
        print(f"Workflow directory {workflow_dir} does not exist")
        sys.exit(1)

    modernizer = WorkflowModernizer(workflow_dir)

    # Create any missing scripts first
    modernizer.create_missing_scripts()

    # Modernize all workflows
    modernizer.modernize_all_workflows()

    # Auto-commit if there were changes
    if modernizer.modernized_count > 0:
        print(
            f"\n🚀 Auto-committing {modernizer.modernized_count} modernized workflows..."
        )

        # Add files
        subprocess.run(["git", "add", "."], cwd=os.path.dirname(workflow_dir))

        # Commit
        commit_msg = f"""feat(workflows): complete workflow modernization phase 2

Modernized {modernizer.modernized_count} additional workflows to use Python scripts.
- Replaced inline bash scripts with dedicated Python modules
- Enhanced security by using environment variables instead of direct GitHub context
- Improved maintainability and reliability
- Added missing utility scripts for artifact management

Part of comprehensive workflow system overhaul."""

        result = subprocess.run(
            ["git", "commit", "-m", commit_msg], cwd=os.path.dirname(workflow_dir)
        )

        if result.returncode == 0:
            print("✓ Changes committed successfully")

            # Push changes
            result = subprocess.run(["git", "push"], cwd=os.path.dirname(workflow_dir))
            if result.returncode == 0:
                print("✓ Changes pushed successfully")
            else:
                print("✗ Failed to push changes")
        else:
            print("✗ Failed to commit changes")


if __name__ == "__main__":
    main()
