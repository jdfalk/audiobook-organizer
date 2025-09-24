#!/usr/bin/env python3
# file: .github/scripts/detect-build-matrix.py
# version: 1.1.0
# guid: a1b2c3d4-e5f6-7890-1234-56789abcdef0

"""
Detect build matrix requirements for the repository.
This script analyzes the repository to determine what technologies are used
and generates appropriate build matrices for GitHub Actions.
"""

import json
import os
import subprocess
import sys


def run_command(cmd, capture_output=True):
    """Run a shell command and return the result."""
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=capture_output, text=True
        )
        return result.returncode == 0, result.stdout.strip()
    except Exception:
        return False, ""


def check_file_exists(pattern):
    """Check if files matching pattern exist."""
    success, output = run_command(f"find . -name '{pattern}' | head -1")
    return success and output.strip() != ""


def detect_build_requirements():
    """Detect what build requirements are needed."""
    print("Detecting build matrix requirements...")

    # Initialize matrices
    matrices = {
        "go": {"include": []},
        "python": {"include": []},
        "frontend": {"include": []},
        "docker": {"include": []},
    }

    flags = {
        "has_go": False,
        "has_python": False,
        "has_frontend": False,
        "has_docker": False,
        "protobuf_needed": False,
    }

    # Check for Go projects
    if os.path.exists("go.mod") or check_file_exists("*.go"):
        print("Go project detected")
        flags["has_go"] = True
        matrices["go"] = {
            "include": [
                {
                    "go-version": "1.24",
                    "os": "ubuntu-latest",
                    "arch": "amd64",
                    "primary": True,
                },
                {"go-version": "1.24", "os": "macos-latest", "arch": "amd64"},
                {"go-version": "1.24", "os": "windows-latest", "arch": "amd64"},
            ]
        }

    # Check for Python projects
    if any(
        os.path.exists(f) for f in ["pyproject.toml", "requirements.txt", "setup.py"]
    ) or check_file_exists("*.py"):
        print("Python project detected")
        flags["has_python"] = True
        matrices["python"] = {
            "include": [
                {"python-version": "3.13", "os": "ubuntu-latest", "primary": True},
                {"python-version": "3.13", "os": "macos-latest", "primary": False},
                {"python-version": "3.13", "os": "windows-latest", "primary": False},
            ]
        }

    # Check for frontend projects
    if os.path.exists("package.json"):
        # Read package.json to determine if this is actually a frontend project
        try:
            with open("package.json", "r") as f:
                import json as json_lib

                pkg_data = json_lib.load(f)

            # Check for frontend indicators
            frontend_indicators = [
                # Dependencies that indicate frontend development
                "react",
                "vue",
                "angular",
                "@angular/core",
                "svelte",
                "solid-js",
                "next",
                "nuxt",
                "gatsby",
                "vite",
                "webpack",
                "rollup",
                "parcel",
                "typescript",
                "@types/node",
                "eslint",
                "prettier",
                # Build frameworks
                "create-react-app",
                "vue-cli",
                "@vue/cli",
                "@angular/cli",
                # UI libraries
                "bootstrap",
                "tailwindcss",
                "@mui/material",
                "antd",
                # Testing frameworks for frontend
                "jest",
                "cypress",
                "@testing-library/react",
                "vitest",
            ]

            # Build script indicators
            build_scripts = pkg_data.get("scripts", {})
            script_indicators = ["build", "serve", "dev", "start"]

            # Check dependencies and devDependencies
            all_deps = {
                **pkg_data.get("dependencies", {}),
                **pkg_data.get("devDependencies", {}),
            }

            has_frontend_deps = any(
                indicator in all_deps for indicator in frontend_indicators
            )
            has_build_scripts = any(
                script in build_scripts
                for script in script_indicators
                if script not in ["commitlint", "commitlint-ci", "lint", "test"]
            )

            # Only treat as frontend if we have actual frontend dependencies or meaningful build scripts
            if has_frontend_deps or has_build_scripts:
                print("Frontend project detected")
                flags["has_frontend"] = True
                matrices["frontend"] = {
                    "include": [
                        {"node-version": "20", "os": "ubuntu-latest"},
                        {"node-version": "22", "os": "ubuntu-latest", "primary": True},
                        {"node-version": "24", "os": "ubuntu-latest"},
                        {"node-version": "22", "os": "macos-latest"},
                        {"node-version": "22", "os": "windows-latest"},
                    ]
                }
            else:
                print(
                    "package.json found but no frontend project detected (likely build tooling only)"
                )

        except Exception as e:
            print(f"Warning: Could not parse package.json: {e}")
            # If we can't parse package.json, assume it's not a frontend project

    # Check for Docker projects
    if (
        check_file_exists("Dockerfile*")
        or check_file_exists("docker-compose*.yml")
        or check_file_exists("docker-compose*.yaml")
        or check_file_exists("docker-stack*.yml")
    ):
        print("Docker project detected")
        flags["has_docker"] = True
        # Use docker-detect.py script for detailed Docker configuration
        success, output = run_command("python3 .github/scripts/docker-detect.py")
        if success:
            print("Docker detection script completed successfully")
            # The docker-detect.py script will set its own outputs
        else:
            print("Docker detection script failed, using basic matrix")
            matrices["docker"] = {
                "include": [
                    {"platform": "linux/amd64", "os": "ubuntu-latest", "primary": True},
                    {"platform": "linux/arm64", "os": "ubuntu-latest"},
                ]
            }

    # Check for protobuf
    if any(
        os.path.exists(f) for f in ["buf.yaml", "buf.gen.yaml"]
    ) or check_file_exists("*.proto"):
        print("Protobuf project detected")
        flags["protobuf_needed"] = True

    return matrices, flags


def set_github_output(key, value):
    """Set GitHub Actions output."""
    if isinstance(value, (dict, list)):
        value = json.dumps(value)
    elif isinstance(value, bool):
        value = "true" if value else "false"

    # GitHub Actions output
    github_output = os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a") as f:
            f.write(f"{key}={value}\n")
    else:
        print(f"{key}={value}")


def main():
    """Main entry point."""
    try:
        matrices, flags = detect_build_requirements()

        # Set outputs
        set_github_output("go-matrix", matrices["go"])
        set_github_output("python-matrix", matrices["python"])
        set_github_output("frontend-matrix", matrices["frontend"])
        set_github_output("docker-matrix", matrices["docker"])
        set_github_output("protobuf-needed", flags["protobuf_needed"])
        set_github_output("has-go", flags["has_go"])
        set_github_output("has-python", flags["has_python"])
        set_github_output("has-frontend", flags["has_frontend"])
        set_github_output("has-docker", flags["has_docker"])

        print("Matrix detection complete")

    except Exception as e:
        print(f"Error during matrix detection: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
