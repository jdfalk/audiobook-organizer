#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-sync-files.py
#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-sync-files.py
# version: 3.0.0
# guid: 8d9e1f2a-3b4c-5d6e-7f8a-9b0c1d2e3f4a

"""
Sync receiver script for copying files from ghcommon to target repositories.
This script performs the actual file copying operations based on sync_type.
Now reads workflow-config.yaml to determine what files to sync.
"""

import os
import shutil
import stat
import subprocess
import sys
import yaml
from pathlib import Path


def load_sync_config():
    """Load sync configuration from workflow-config.yaml."""
    config_file = Path(".github/workflow-config.yaml")
    if not config_file.exists():
        print("⚠️  No workflow-config.yaml found, using defaults")
        return {"sync": {"sync_paths": [], "exclude_files": []}}

    try:
        with open(config_file, "r") as f:
            config = yaml.safe_load(f)
            print(f"✅ Loaded sync configuration from {config_file}")
            return config
    except yaml.YAMLError as e:
        print(f"❌ Error loading workflow-config.yaml: {e}")
        return {"sync": {"sync_paths": [], "exclude_files": []}}


def is_file_excluded(file_path, exclude_list):
    """Check if a file should be excluded from sync."""
    file_path = str(file_path)

    for exclude_pattern in exclude_list:
        if exclude_pattern in file_path or file_path.endswith(exclude_pattern):
            return True
    return False


def ensure_directory(path):
    """Ensure a directory exists."""
    Path(path).mkdir(parents=True, exist_ok=True)


def copy_file_safe(src, dst):
    """Copy a file safely, creating directories as needed."""
    try:
        dst_path = Path(dst)
        ensure_directory(dst_path.parent)
        shutil.copy2(src, dst)
        print(f"✅ Copied {src} -> {dst}")
        return True
    except FileNotFoundError:
        print(f"⚠️  Source file not found: {src}")
        return False
    except Exception as e:
        print(f"❌ Error copying {src} -> {dst}: {e}")
        return False


def copy_directory_safe(src, dst):
    """Copy a directory safely."""
    try:
        src_path = Path(src)
        if not src_path.exists():
            print(f"⚠️  Source directory not found: {src}")
            return False

        dst_path = Path(dst)
        ensure_directory(dst_path.parent)

        if dst_path.exists():
            shutil.rmtree(dst_path)

        shutil.copytree(src, dst)
        print(f"✅ Copied directory {src} -> {dst}")
        return True
    except Exception as e:
        print(f"❌ Error copying directory {src} -> {dst}: {e}")
        return False


def make_scripts_executable(pattern):
    """Make scripts matching pattern executable."""
    try:
        for script in Path(".github/scripts").glob(pattern):
            current_mode = script.stat().st_mode
            script.chmod(current_mode | stat.S_IEXEC)
        print(f"✅ Made {pattern} scripts executable")
    except Exception as e:
        print(f"❌ Error making scripts executable: {e}")


def sync_workflows():
    """Sync workflow files based on configuration."""
    print("🔄 Processing workflows section...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    # Filter for workflow files
    workflow_files = [
        path
        for path in sync_paths
        if path.startswith(".github/workflows/") and path.endswith(".yml")
    ]

    if not workflow_files:
        print("⚠️  No workflow files configured for sync")
        return

    print(f"📋 Found {len(workflow_files)} workflow files configured for sync")

    success_count = 0
    for workflow_file in workflow_files:
        workflow_name = Path(workflow_file).name

        # Check if file is excluded
        if is_file_excluded(workflow_file, exclude_files):
            print(f"⏭️  Skipping excluded workflow: {workflow_name}")
            continue

        src_path = f"ghcommon-source/{workflow_file}"
        dst_path = workflow_file

        print(f"ℹ️  Copying workflow {workflow_name}: {src_path} -> {dst_path}")

        if copy_file_safe(src_path, dst_path):
            print(f"✅ Successfully copied workflow {workflow_name}")
            success_count += 1
        else:
            print(f"❌ Failed to copy workflow {workflow_name}")

    print(f"📊 Workflows sync: {success_count}/{len(workflow_files)} files copied")


def sync_instructions():
    """Sync instruction files based on configuration."""
    print("🔄 Processing instructions section...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    # Copy main copilot-instructions.md if configured
    copilot_instructions = ".github/copilot-instructions.md"
    if copilot_instructions in sync_paths and not is_file_excluded(
        copilot_instructions, exclude_files
    ):
        print(
            f"ℹ️  Copying main copilot-instructions.md: ghcommon-source/{copilot_instructions} -> {copilot_instructions}"
        )
        if copy_file_safe(
            f"ghcommon-source/{copilot_instructions}", copilot_instructions
        ):
            print("✅ Successfully copied main copilot-instructions.md")
        else:
            print("❌ Failed to copy main copilot-instructions.md")

    # Copy instructions directory if configured
    instructions_dir = ".github/instructions/"
    if instructions_dir in sync_paths and not is_file_excluded(
        instructions_dir, exclude_files
    ):
        src_dir = Path("ghcommon-source/.github/instructions")
        if src_dir.exists():
            instruction_files = list(src_dir.glob("*"))
            print(f"📋 Copying {len(instruction_files)} instruction files...")

            success_count = 0
            for instruction_file in instruction_files:
                if instruction_file.is_file():
                    if copy_file_safe(
                        str(instruction_file),
                        f".github/instructions/{instruction_file.name}",
                    ):
                        success_count += 1
                    else:
                        print(
                            f"❌ Failed to copy instruction file {instruction_file.name}"
                        )

            print(
                f"✅ Copied {success_count}/{len(instruction_files)} instruction files"
            )
        else:
            print(f"⚠️  Source not found for instruction files: {src_dir}/*")


def sync_prompts():
    """Sync prompt files based on configuration."""
    print("🔄 Processing prompts section...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    # Check if prompts directory is configured for sync
    prompts_dir = ".github/prompts/"
    if prompts_dir not in sync_paths:
        print("⚠️  Prompts directory not configured for sync")
        return

    if is_file_excluded(prompts_dir, exclude_files):
        print("⏭️  Prompts directory excluded from sync")
        return

    # List prompts files to copy first
    src_dir = Path("ghcommon-source/.github/prompts")
    if src_dir.exists():
        print("📋 Prompts files to copy:")
        subprocess.run(["ls", "-la", str(src_dir)], check=False)

        for prompt_file in src_dir.glob("*"):
            if prompt_file.is_file():
                print(
                    f"ℹ️  Copying prompt file {prompt_file.name}: {prompt_file} -> .github/prompts/"
                )
                if copy_file_safe(
                    str(prompt_file), f".github/prompts/{prompt_file.name}"
                ):
                    print(f"✅ Successfully copied prompt file {prompt_file.name}")
                else:
                    print(f"❌ Failed to copy prompt file {prompt_file.name}")
    else:
        print(f"⚠️  Source not found for prompts files: {src_dir}/*")


def sync_scripts():
    """Sync script files based on configuration."""
    print("🔄 Processing scripts section...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    # Show initial .github tree structure
    print("📁 Current .github structure:")
    subprocess.run(["tree", ".github", "-I", "logs|*.tmp"], check=False)
    print()

    # Copy root scripts if configured
    if any(path.startswith("scripts/") for path in sync_paths):
        src_dir = Path("ghcommon-source/scripts")
        if src_dir.exists():
            print("📋 Copying root scripts...")
            copy_directory_safe("ghcommon-source/scripts", "scripts")
            print("✅ Root scripts copied")
        else:
            print(f"⚠️  Source not found for root scripts: {src_dir}/*")

    # Scripts to exclude from sync (master dispatcher scripts)
    excluded_scripts = {
        "sync-determine-target-repos.py",
        "sync-dispatch-events.py",
        "sync-generate-summary.py",
    }

    # Copy GitHub scripts individually based on configuration
    github_scripts_configured = any(
        path.startswith(".github/scripts/") for path in sync_paths
    )

    if github_scripts_configured:
        src_dir = Path("ghcommon-source/.github/scripts")
        if src_dir.exists():
            script_files = [
                f
                for f in src_dir.glob("*")
                if f.is_file() and f.name not in excluded_scripts
            ]
            print(f"📋 Copying {len(script_files)} GitHub scripts...")

            success_count = 0
            for script_file in script_files:
                script_path = f".github/scripts/{script_file.name}"
                if not is_file_excluded(script_path, exclude_files):
                    if copy_file_safe(str(script_file), script_path):
                        success_count += 1
                    else:
                        print(f"❌ Failed to copy GitHub script {script_file.name}")

            excluded_count = len(
                [
                    f
                    for f in src_dir.glob("*")
                    if f.is_file() and f.name in excluded_scripts
                ]
            )
            print(
                f"✅ Copied {success_count}/{len(script_files)} GitHub scripts ({excluded_count} excluded)"
            )
        else:
            print(f"⚠️  Source not found for GitHub scripts: {src_dir}/*")

    # Make sync scripts executable
    make_scripts_executable("sync-*.sh")
    make_scripts_executable("sync-*.py")


def sync_linters():
    """Sync linter configuration files based on configuration."""
    print("🔄 Processing linters section...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    # Check if linters directory is configured for sync
    linters_dir = ".github/linters/"
    if linters_dir not in sync_paths:
        print("⚠️  Linters directory not configured for sync")
        return

    if is_file_excluded(linters_dir, exclude_files):
        print("⏭️  Linters directory excluded from sync")
        return

    # List linter files to copy first
    src_dir = Path("ghcommon-source/.github/linters")
    if src_dir.exists():
        linter_files = list(src_dir.glob("*"))
        print(f"📋 Copying {len(linter_files)} linter files...")
        copy_directory_safe("ghcommon-source/.github/linters", ".github/linters")
        print("✅ Linter files copied")
    else:
        print(f"⚠️  Source not found for linter files: {src_dir}/*")


def sync_labels():
    """Sync label files based on configuration."""
    print("🔄 Processing labels section...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    success_count = 0
    total_files = 0

    # Check each label file
    label_files = ["labels.json", "labels.md"]

    for label_file in label_files:
        if label_file in sync_paths and not is_file_excluded(label_file, exclude_files):
            total_files += 1
            print(f"ℹ️  Copying {label_file}: ghcommon-source/{label_file} -> .")
            if copy_file_safe(f"ghcommon-source/{label_file}", label_file):
                print(f"✅ Successfully copied {label_file}")
                success_count += 1
            else:
                print(f"❌ Failed to copy {label_file}")

    # Copy GitHub labels sync script if configured
    labels_script = "scripts/sync-github-labels.py"
    if labels_script in sync_paths and not is_file_excluded(
        labels_script, exclude_files
    ):
        total_files += 1
        print(
            f"ℹ️  Copying GitHub labels sync script: ghcommon-source/{labels_script} -> {labels_script}"
        )
        if copy_file_safe(f"ghcommon-source/{labels_script}", labels_script):
            print("✅ Successfully copied GitHub labels sync script")
            success_count += 1
        else:
            print("❌ Failed to copy GitHub labels sync script")

    if total_files > 0:
        print(f"📊 Labels sync: {success_count}/{total_files} files copied")
    else:
        print("⚠️  No label files configured for sync")


def sync_other_files():
    """Sync other individual files based on configuration."""
    print("🔄 Processing other configured files...")

    config = load_sync_config()
    sync_paths = config.get("sync", {}).get("sync_paths", [])
    exclude_files = config.get("sync", {}).get("exclude_files", [])

    # Files that aren't handled by other sync functions
    handled_patterns = [
        ".github/workflows/",
        ".github/instructions/",
        ".github/prompts/",
        ".github/linters/",
        "scripts/",
        ".github/scripts/",
        "labels.json",
        "labels.md",
    ]

    other_files = [
        path
        for path in sync_paths
        if not any(
            path.startswith(pattern) or path == pattern for pattern in handled_patterns
        )
    ]

    if not other_files:
        print("⚠️  No other files configured for sync")
        return

    print(f"📋 Found {len(other_files)} other files configured for sync")

    success_count = 0
    for file_path in other_files:
        if is_file_excluded(file_path, exclude_files):
            print(f"⏭️  Skipping excluded file: {file_path}")
            continue

        src_path = f"ghcommon-source/{file_path}"
        dst_path = file_path

        print(f"ℹ️  Copying file: {src_path} -> {dst_path}")

        if copy_file_safe(src_path, dst_path):
            print(f"✅ Successfully copied {file_path}")
            success_count += 1
        else:
            print(f"❌ Failed to copy {file_path}")

    print(f"📊 Other files sync: {success_count}/{len(other_files)} files copied")


def main():
    """Main entry point."""
    sync_type = sys.argv[1] if len(sys.argv) > 1 else "all"

    print(f"Performing sync of type: {sync_type}")

    # Create necessary directories
    directories = [
        ".github/workflows",
        ".github/instructions",
        ".github/prompts",
        ".github/scripts",
        ".github/linters",
        "scripts",
    ]

    for directory in directories:
        ensure_directory(directory)

    # Perform sync based on type
    if sync_type in ["all", "workflows"]:
        sync_workflows()

    if sync_type in ["all", "instructions"]:
        sync_instructions()

    if sync_type in ["all", "prompts"]:
        sync_prompts()

    if sync_type in ["all", "scripts"]:
        sync_scripts()

    if sync_type in ["all", "linters"]:
        sync_linters()

    if sync_type in ["all", "labels"]:
        sync_labels()

    # Sync other configured files not handled by specific functions
    if sync_type == "all":
        sync_other_files()

    print(f"✅ Sync completed for type: {sync_type}")


if __name__ == "__main__":
    main()
