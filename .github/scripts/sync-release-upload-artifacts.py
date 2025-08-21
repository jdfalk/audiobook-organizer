#!/usr/bin/env python3
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
    main()