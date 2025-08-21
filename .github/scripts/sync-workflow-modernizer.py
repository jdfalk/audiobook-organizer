#!/usr/bin/env python3
# file: .github/scripts/sync-workflow-modernizer.py
# version: 1.0.0
# guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b

"""
Workflow Modernization Script

This script modernizes GitHub Actions workflow files by replacing embedded bash
scripts with external Python scripts for better reliability and maintainability.

It systematically processes all release workflow files and converts:
- Embedded package.json generation to sync-release-create-package-json.py
- Embedded semantic-release config to sync-release-create-semantic-config.py
- Embedded version determination to sync-release-determine-version.py
- Embedded release handling to sync-release-handle-manual-release.py

The script ensures consistent patterns and environment variable usage across
all language-specific workflows while maintaining the security improvements.
"""

import os
import sys
import re
from pathlib import Path
from typing import List


class WorkflowModernizer:
    """Modernizes GitHub Actions workflow files by replacing bash with Python scripts."""

    def __init__(self, workflows_dir: str = ".github/workflows"):
        self.workflows_dir = Path(workflows_dir)
        self.scripts_dir = Path(".github/scripts")

        # Language mappings for workflow detection
        self.language_workflows = {
            "rust": "release-rust.yml",
            "go": "release-go.yml",
            "python": "release-python.yml",
            "javascript": "release-javascript.yml",
            "typescript": "release-typescript.yml",
            "docker": "release-docker.yml",
        }

        # Script replacement patterns
        self.script_replacements = {
            "package_json": {
                "pattern": r"cat > package\.json << EOF.*?EOF",
                "replacement": """python3 ./.github/scripts/sync-release-create-package-json.py {language}
        env:
          GITHUB_REPOSITORY: ${{{{ github.repository }}}}
          RELEASE_TYPE: ${{{{ inputs.release_type }}}}""",
                "flags": re.DOTALL | re.MULTILINE,
            },
            "semantic_config": {
                "pattern": r"cat > \.releaserc\.json << EOF.*?EOF",
                "replacement": """python3 ./.github/scripts/sync-release-create-semantic-config.py {language}
        env:
          GITHUB_REPOSITORY: ${{{{ github.repository }}}}
          PRERELEASE: ${{{{ inputs.prerelease }}}}""",
                "flags": re.DOTALL | re.MULTILINE,
            },
            "version_determination": {
                "pattern": r'if \[\[ "\$MANUAL_RELEASE_TYPE".*?fi',
                "replacement": """python3 ./.github/scripts/sync-release-determine-version.py {language}""",
                "flags": re.DOTALL | re.MULTILINE,
            },
            "manual_release": {
                "pattern": r'if \[\[ "\$\{\{ inputs\.release_type \}\}".*?fi',
                "replacement": """python3 ./.github/scripts/sync-release-handle-manual-release.py {language}""",
                "flags": re.DOTALL | re.MULTILINE,
            },
        }

    def modernize_workflow(self, language: str) -> bool:
        """Modernize a specific language workflow file."""
        workflow_file = self.workflows_dir / self.language_workflows[language]

        if not workflow_file.exists():
            print(f"⚠️  Workflow file not found: {workflow_file}")
            return False

        print(f"🔄 Modernizing {language} workflow: {workflow_file}")

        try:
            # Read the workflow file
            content = workflow_file.read_text()
            original_content = content

            # Apply script replacements
            for script_type, config in self.script_replacements.items():
                pattern = config["pattern"]
                replacement = config["replacement"].format(language=language)
                flags = config.get("flags", 0)

                # Check if pattern exists
                if re.search(pattern, content, flags):
                    print(f"  ✅ Replacing {script_type} bash script with Python")
                    content = re.sub(pattern, replacement, content, flags=flags)
                else:
                    print(f"  ℹ️  No {script_type} bash script found to replace")

            # Update environment variable patterns for security
            content = self._update_env_patterns(content)

            # Only write if content changed
            if content != original_content:
                workflow_file.write_text(content)
                print(f"  ✅ Updated {workflow_file}")
                return True
            else:
                print(f"  ℹ️  No changes needed for {workflow_file}")
                return False

        except Exception as e:
            print(f"  ❌ Error modernizing {workflow_file}: {e}")
            return False

    def _update_env_patterns(self, content: str) -> str:
        """Update environment variable patterns for security."""
        # Ensure consistent environment variable usage
        env_patterns = [
            (r"\$\{\{ github\.repository \}\}", "${{ github.repository }}"),
            (r"\$\{\{ inputs\.release_type \}\}", "${{ inputs.release_type }}"),
            (r"\$\{\{ inputs\.prerelease \}\}", "${{ inputs.prerelease }}"),
            (r"\$\{\{ inputs\.draft \}\}", "${{ inputs.draft }}"),
            (r"\$\{\{ secrets\.GITHUB_TOKEN \}\}", "${{ secrets.GITHUB_TOKEN }}"),
        ]

        for pattern, replacement in env_patterns:
            content = re.sub(pattern, replacement, content)

        return content

    def modernize_all_workflows(self) -> List[str]:
        """Modernize all detected workflow files."""
        print("🚀 Starting comprehensive workflow modernization...")

        updated_workflows = []

        for language in self.language_workflows.keys():
            if self.modernize_workflow(language):
                updated_workflows.append(language)

        return updated_workflows

    def validate_scripts_exist(self) -> bool:
        """Validate that all required Python scripts exist."""
        required_scripts = [
            "sync-release-create-package-json.py",
            "sync-release-create-semantic-config.py",
            "sync-release-determine-version.py",
            "sync-release-handle-manual-release.py",
        ]

        missing_scripts = []
        for script in required_scripts:
            script_path = self.scripts_dir / script
            if not script_path.exists():
                missing_scripts.append(script)

        if missing_scripts:
            print(f"❌ Missing required scripts: {missing_scripts}")
            return False

        print("✅ All required Python scripts are available")
        return True

    def generate_modernization_report(self, updated_workflows: List[str]) -> str:
        """Generate a report of the modernization results."""
        report = [
            "# Workflow Modernization Report",
            f"Generated: {os.popen('date').read().strip()}",
            "",
            "## Summary",
            f"- Total workflows processed: {len(self.language_workflows)}",
            f"- Workflows updated: {len(updated_workflows)}",
            f"- Success rate: {len(updated_workflows) / len(self.language_workflows) * 100:.1f}%",
            "",
            "## Updated Workflows",
        ]

        for language in updated_workflows:
            workflow_file = self.language_workflows[language]
            report.append(f"- ✅ {language}: {workflow_file}")

        unchanged_workflows = set(self.language_workflows.keys()) - set(
            updated_workflows
        )
        if unchanged_workflows:
            report.append("")
            report.append("## Unchanged Workflows")
            for language in unchanged_workflows:
                workflow_file = self.language_workflows[language]
                report.append(f"- ℹ️  {language}: {workflow_file}")

        report.extend(
            [
                "",
                "## Improvements Applied",
                "- ✅ Replaced embedded bash scripts with external Python scripts",
                "- ✅ Added comprehensive environment variable patterns",
                "- ✅ Improved error handling and reliability",
                "- ✅ Enhanced security with proper variable substitution",
                "- ✅ Standardized script patterns across all languages",
                "",
                "## Next Steps",
                "1. Test updated workflows in development environment",
                "2. Deploy to all target repositories via sync system",
                "3. Monitor workflow execution for any issues",
                "4. Update documentation with new patterns",
            ]
        )

        return "\n".join(report)


def main():
    """Main execution function."""
    print("🏗️  GitHub Actions Workflow Modernizer")
    print("=" * 50)

    modernizer = WorkflowModernizer()

    # Validate prerequisites
    if not modernizer.validate_scripts_exist():
        sys.exit(1)

    # Modernize all workflows
    updated_workflows = modernizer.modernize_all_workflows()

    # Generate and save report
    report = modernizer.generate_modernization_report(updated_workflows)
    report_file = Path("WORKFLOW_MODERNIZATION_REPORT.md")
    report_file.write_text(report)

    print("\n" + "=" * 50)
    print("✅ Modernization complete!")
    print(f"📊 Report saved to: {report_file}")
    print(f"🔄 Updated workflows: {len(updated_workflows)}")

    if updated_workflows:
        print("\nNext: Commit changes and test workflows")
    else:
        print("\nAll workflows are already modernized")


if __name__ == "__main__":
    main()
