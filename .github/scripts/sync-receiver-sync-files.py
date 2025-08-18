#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-sync-files.py
# version: 1.0.0
# guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

"""
Sync files from the ghcommon source to the target repository.
"""

import sys
import shutil
import stat
from pathlib import Path


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
    """Sync workflow files."""
    print("Syncing workflows...")

    # Copy specific workflows (avoid sync workflows to prevent recursion)
    workflows = [
        "pr-automation.yml",
        "release.yml",
        "release-rust.yml",
        "release-go.yml",
        "release-python.yml",
        "release-javascript.yml",
        "release-typescript.yml",
        "release-docker.yml",
    ]

    for workflow in workflows:
        src = f"ghcommon-source/.github/workflows/{workflow}"
        dst = f".github/workflows/{workflow}"
        copy_file_safe(src, dst)


def sync_instructions():
    """Sync instruction files."""
    print("Syncing instructions...")

    # Copy main instructions file
    copy_file_safe(
        "ghcommon-source/.github/copilot-instructions.md",
        ".github/copilot-instructions.md",
    )

    # Copy instructions directory
    src_dir = Path("ghcommon-source/.github/instructions")
    if src_dir.exists():
        for instruction_file in src_dir.glob("*"):
            if instruction_file.is_file():
                copy_file_safe(
                    str(instruction_file),
                    f".github/instructions/{instruction_file.name}",
                )


def sync_prompts():
    """Sync prompt files."""
    print("Syncing prompts...")
    copy_directory_safe("ghcommon-source/.github/prompts", ".github/prompts")


def sync_scripts():
    """Sync script files."""
    print("Syncing scripts...")

    # Copy root scripts
    copy_directory_safe("ghcommon-source/scripts", "scripts")

    # Copy GitHub scripts
    src_dir = Path("ghcommon-source/.github/scripts")
    if src_dir.exists():
        for script_file in src_dir.glob("*"):
            if script_file.is_file():
                copy_file_safe(str(script_file), f".github/scripts/{script_file.name}")

    # Make sync scripts executable
    make_scripts_executable("sync-*.sh")
    make_scripts_executable("sync-*.py")


def sync_linters():
    """Sync linter configuration files."""
    print("Syncing linters...")
    copy_directory_safe("ghcommon-source/.github/linters", ".github/linters")


def sync_labels():
    """Sync label files."""
    print("Syncing labels...")
    copy_file_safe("ghcommon-source/labels.json", "labels.json")
    copy_file_safe("ghcommon-source/labels.md", "labels.md")


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

    print(f"✅ Sync completed for type: {sync_type}")


if __name__ == "__main__":
    main()
