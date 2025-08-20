#!/usr/bin/env python3
# file: .github/scripts/docker-detect.py
# version: 1.0.0
# guid: d1e2f3g4-h5i6-j7k8-l9m0-n1o2p3q4r5s6

"""
Docker configuration detection script for matrix build system.
Detects Docker files, configurations, and determines build strategy.
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


def find_dockerfile():
    """Find the best Dockerfile to use."""
    dockerfile_options = [
        "Dockerfile",
        "Dockerfile.hybrid",
        "Dockerfile.prod",
        "Dockerfile.assets",
    ]

    for dockerfile in dockerfile_options:
        if os.path.exists(dockerfile):
            return dockerfile
    return None


def check_docker_compose():
    """Check for docker-compose files."""
    compose_files = [
        "docker-compose.yml",
        "docker-compose.yaml",
        "docker-stack.yml",
        "docker-stack-jf.yml",
    ]

    found_files = []
    for compose_file in compose_files:
        if os.path.exists(compose_file):
            found_files.append(compose_file)

    return found_files


def should_build_docker(event_name, ref):
    """Determine if Docker image should be built and pushed."""
    if event_name == "push" and ref == "refs/heads/main":
        return True
    elif event_name == "release":
        return True
    elif event_name == "workflow_dispatch":
        return True
    return False


def generate_docker_matrix():
    """Generate Docker build matrix."""
    dockerfile = find_dockerfile()
    if not dockerfile:
        return {"include": []}

    # Basic multi-platform matrix
    matrix = {
        "include": [
            {
                "platform": "linux/amd64",
                "os": "ubuntu-latest",
                "dockerfile": dockerfile,
                "primary": True,
            },
            {
                "platform": "linux/arm64",
                "os": "ubuntu-latest",
                "dockerfile": dockerfile,
                "primary": False,
            },
        ]
    }

    return matrix


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
        print("Detecting Docker configuration...")

        # Find Dockerfile
        dockerfile = find_dockerfile()
        has_dockerfile = dockerfile is not None

        # Check for compose files
        compose_files = check_docker_compose()
        has_compose = len(compose_files) > 0

        # Determine if we should build
        event_name = os.environ.get("GITHUB_EVENT_NAME", "")
        ref = os.environ.get("GITHUB_REF", "")
        should_build = should_build_docker(event_name, ref)

        # Generate matrix
        docker_matrix = generate_docker_matrix()

        # Set outputs
        set_github_output("has-dockerfile", has_dockerfile)
        set_github_output("dockerfile-path", dockerfile or "")
        set_github_output("has-compose", has_compose)
        set_github_output("compose-files", compose_files)
        set_github_output("should-build", should_build)
        set_github_output("docker-matrix", docker_matrix)

        # Print summary
        print(f"Dockerfile found: {dockerfile}")
        print(f"Compose files found: {compose_files}")
        print(f"Should build: {should_build}")
        print(f"Matrix entries: {len(docker_matrix['include'])}")

    except Exception as e:
        print(f"Error during Docker detection: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
