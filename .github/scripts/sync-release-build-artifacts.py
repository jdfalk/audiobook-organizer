#!/usr/bin/env python3
# file: .github/scripts/sync-release-build-artifacts.py
# version: 1.0.0
# guid: f2a3b4c5-d6e7-8f9a-0b1c-2d3e4f5a6b7c

"""
Build Artifacts Script

Handles building release artifacts for different programming languages.
Replaces embedded bash build scripts with reliable Python-based build logic.
"""

import os
import sys
import json
import subprocess
import platform
from pathlib import Path


def log(message: str, level: str = "INFO") -> None:
    """Log a message with timestamp and level."""
    print(f"[{level}] {message}")


def run_command(cmd: list, cwd: str = None, env: dict = None) -> tuple:
    """Run a command and return (success, stdout, stderr)."""
    try:
        result = subprocess.run(
            cmd, cwd=cwd, env=env, capture_output=True, text=True, check=False
        )
        return result.returncode == 0, result.stdout, result.stderr
    except Exception as e:
        return False, "", str(e)


def build_rust_artifacts() -> bool:
    """Build Rust artifacts for multiple targets."""
    log("Building Rust artifacts...")

    # Standard Rust targets for cross-compilation
    targets = [
        "x86_64-unknown-linux-gnu",
        "x86_64-unknown-linux-musl",
        "aarch64-unknown-linux-gnu",
        "x86_64-apple-darwin",
        "aarch64-apple-darwin",
        "x86_64-pc-windows-gnu",
    ]

    # Get binary name from Cargo.toml
    binary_name = "unknown"
    try:
        with open("Cargo.toml", "r") as f:
            content = f.read()
            # Look for [[bin]] section first
            if "[[bin]]" in content:
                lines = content.split("\n")
                in_bin_section = False
                for line in lines:
                    if "[[bin]]" in line:
                        in_bin_section = True
                    elif in_bin_section and line.startswith("name"):
                        binary_name = line.split("=")[1].strip().strip('"')
                        break
            else:
                # Fall back to package name
                for line in content.split("\n"):
                    if line.startswith("name"):
                        binary_name = line.split("=")[1].strip().strip('"')
                        break
    except Exception as e:
        log(f"Could not determine binary name: {e}", "WARN")

    log(f"Building binary: {binary_name}")

    # Create releases directory
    releases_dir = Path("releases")
    releases_dir.mkdir(exist_ok=True)

    success_count = 0
    for target in targets:
        log(f"Building for target: {target}")

        # Install target if needed
        install_success, _, _ = run_command(["rustup", "target", "add", target])
        if not install_success:
            log(f"Failed to install target {target}", "WARN")
            continue

        # Build for target
        build_success, stdout, stderr = run_command(
            ["cargo", "build", "--release", "--target", target]
        )

        if build_success:
            log(f"Successfully built for {target}")
            success_count += 1

            # Create archive
            binary_path = f"target/{target}/release/{binary_name}"
            if target.endswith("windows-gnu"):
                binary_path += ".exe"

            if Path(binary_path).exists():
                # Create appropriate archive
                if target.endswith("windows-gnu"):
                    archive_name = f"{binary_name}-{target}.zip"
                    # Use PowerShell for Windows
                    run_command(
                        [
                            "powershell",
                            "-Command",
                            f"Compress-Archive -Path {binary_path} -DestinationPath releases/{archive_name}",
                        ]
                    )
                else:
                    archive_name = f"{binary_name}-{target}.tar.gz"
                    run_command(
                        [
                            "tar",
                            "-czf",
                            f"releases/{archive_name}",
                            "-C",
                            f"target/{target}/release",
                            binary_name,
                        ]
                    )

                log(f"Created archive: {archive_name}")
            else:
                log(f"Binary not found at {binary_path}", "WARN")
        else:
            log(f"Failed to build for {target}: {stderr}", "ERROR")

    log(f"Built successfully for {success_count}/{len(targets)} targets")
    return success_count > 0


def build_go_artifacts() -> bool:
    """Build Go artifacts for multiple platforms."""
    log("Building Go artifacts...")

    platforms = [
        ("linux", "amd64"),
        ("linux", "arm64"),
        ("darwin", "amd64"),
        ("darwin", "arm64"),
        ("windows", "amd64"),
    ]

    # Get module name
    module_name = "unknown"
    try:
        with open("go.mod", "r") as f:
            first_line = f.readline().strip()
            if first_line.startswith("module"):
                module_name = Path(first_line.split()[1]).name
    except Exception as e:
        log(f"Could not determine module name: {e}", "WARN")

    log(f"Building module: {module_name}")

    # Create releases directory
    releases_dir = Path("releases")
    releases_dir.mkdir(exist_ok=True)

    success_count = 0
    for goos, goarch in platforms:
        log(f"Building for {goos}/{goarch}")

        binary_name = module_name
        if goos == "windows":
            binary_name += ".exe"

        env = os.environ.copy()
        env["GOOS"] = goos
        env["GOARCH"] = goarch
        env["CGO_ENABLED"] = "0"

        build_success, stdout, stderr = run_command(
            ["go", "build", "-o", f"releases/{binary_name}", "."], env=env
        )

        if build_success:
            log(f"Successfully built for {goos}/{goarch}")
            success_count += 1

            # Create archive
            if goos == "windows":
                archive_name = f"{module_name}-{goos}-{goarch}.zip"
                run_command(
                    ["zip", f"releases/{archive_name}", f"releases/{binary_name}"]
                )
            else:
                archive_name = f"{module_name}-{goos}-{goarch}.tar.gz"
                run_command(
                    [
                        "tar",
                        "-czf",
                        f"releases/{archive_name}",
                        "-C",
                        "releases",
                        binary_name,
                    ]
                )

            # Remove the binary (keep only archive)
            Path(f"releases/{binary_name}").unlink(missing_ok=True)
            log(f"Created archive: {archive_name}")
        else:
            log(f"Failed to build for {goos}/{goarch}: {stderr}", "ERROR")

    log(f"Built successfully for {success_count}/{len(platforms)} platforms")
    return success_count > 0


def build_python_artifacts() -> bool:
    """Build Python wheel artifacts."""
    log("Building Python artifacts...")

    # Create releases directory
    releases_dir = Path("releases")
    releases_dir.mkdir(exist_ok=True)

    # Build wheel
    build_success, stdout, stderr = run_command(
        ["python", "-m", "build", "--wheel", "--outdir", "releases"]
    )

    if build_success:
        log("Successfully built Python wheel")
        return True
    else:
        log(f"Failed to build Python wheel: {stderr}", "ERROR")
        return False


def build_javascript_artifacts() -> bool:
    """Build JavaScript/Node.js artifacts."""
    log("Building JavaScript artifacts...")

    # Install dependencies
    install_success, _, stderr = run_command(["npm", "install"])
    if not install_success:
        log(f"Failed to install dependencies: {stderr}", "ERROR")
        return False

    # Build if build script exists
    try:
        with open("package.json", "r") as f:
            package_data = json.load(f)
            scripts = package_data.get("scripts", {})

            if "build" in scripts:
                build_success, stdout, stderr = run_command(["npm", "run", "build"])
                if build_success:
                    log("Successfully built JavaScript project")
                    return True
                else:
                    log(f"Build failed: {stderr}", "ERROR")
                    return False
            else:
                log("No build script found, assuming library project")
                return True

    except Exception as e:
        log(f"Error processing package.json: {e}", "ERROR")
        return False


def main():
    """Main execution function."""
    language = (
        sys.argv[1] if len(sys.argv) > 1 else os.environ.get("LANGUAGE", "unknown")
    )

    log(f"Building artifacts for language: {language}")

    success = False

    if language == "rust":
        success = build_rust_artifacts()
    elif language == "go":
        success = build_go_artifacts()
    elif language == "python":
        success = build_python_artifacts()
    elif language in ["javascript", "typescript"]:
        success = build_javascript_artifacts()
    else:
        log(f"Unsupported language: {language}", "ERROR")
        sys.exit(1)

    if success:
        log("Artifact building completed successfully")
        sys.exit(0)
    else:
        log("Artifact building failed", "ERROR")
        sys.exit(1)


if __name__ == "__main__":
    main()
