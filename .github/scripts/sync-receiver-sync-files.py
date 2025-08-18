#!/usr/bin/env python3
# file: .github/scripts/sync-receiver-sync-files.py
# version: 1.2.0
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
    print("🔄 Processing instructions section...")

    # Copy main instructions file
    print(
        "ℹ️  Copying main copilot-instructions.md: ghcommon-source/.github/copilot-instructions.md -> .github/"
    )
    if copy_file_safe(
        "ghcommon-source/.github/copilot-instructions.md",
        ".github/copilot-instructions.md",
    ):
        print("✅ Successfully copied main copilot-instructions.md")
    else:
        print("❌ Failed to copy main copilot-instructions.md")

    # Copy instructions directory
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
                    print(f"❌ Failed to copy instruction file {instruction_file.name}")

        print(f"✅ Copied {success_count}/{len(instruction_files)} instruction files")
    else:
        print(f"⚠️  Source not found for instruction files: {src_dir}/*")


def sync_prompts():
    """Sync prompt files."""
    print("🔄 Processing prompts section...")

    # List prompts files to copy first
    src_dir = Path("ghcommon-source/.github/prompts")
    if src_dir.exists():
        print("📋 Prompts files to copy:")
        import subprocess

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
    """Sync script files."""
    print("🔄 Processing scripts section...")

    # Show initial .github tree structure
    print("📁 Current .github structure:")
    import subprocess

    subprocess.run(["tree", ".github", "-I", "logs|*.tmp"], check=False)
    print()

    # Copy root scripts
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

    # Copy GitHub scripts individually
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
            if copy_file_safe(str(script_file), f".github/scripts/{script_file.name}"):
                success_count += 1
            else:
                print(f"❌ Failed to copy GitHub script {script_file.name}")

        excluded_count = len(
            [f for f in src_dir.glob("*") if f.is_file() and f.name in excluded_scripts]
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
    """Sync linter configuration files."""
    print("🔄 Processing linters section...")

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
    """Sync label files."""
    print("🔄 Processing labels section...")

    success_count = 0
    total_files = 3

    print("ℹ️  Copying labels.json: ghcommon-source/labels.json -> .")
    if copy_file_safe("ghcommon-source/labels.json", "labels.json"):
        print("✅ Successfully copied labels.json")
        success_count += 1
    else:
        print("❌ Failed to copy labels.json")

    print("ℹ️  Copying labels.md: ghcommon-source/labels.md -> .")
    if copy_file_safe("ghcommon-source/labels.md", "labels.md"):
        print("✅ Successfully copied labels.md")
        success_count += 1
    else:
        print("❌ Failed to copy labels.md")

    # Copy GitHub labels sync script
    print(
        "ℹ️  Copying GitHub labels sync script: ghcommon-source/scripts/sync-github-labels.py -> scripts/"
    )
    if copy_file_safe(
        "ghcommon-source/scripts/sync-github-labels.py", "scripts/sync-github-labels.py"
    ):
        print("✅ Successfully copied GitHub labels sync script")
        success_count += 1
    else:
        print("❌ Failed to copy GitHub labels sync script")

    print(f"📊 Labels sync: {success_count}/{total_files} files copied")


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
