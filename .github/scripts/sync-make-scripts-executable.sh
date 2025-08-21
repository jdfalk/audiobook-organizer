#!/bin/bash
# file: .github/scripts/sync-make-scripts-executable.sh
# version: 1.0.0
# guid: c9d0e1f2-a3b4-5c6d-7e8f-9a0b1c2d3e4f

set -euo pipefail

echo "Making all Python scripts in .github/scripts executable..."

# Find all Python scripts and make them executable
find .github/scripts -name "*.py" -type f -exec chmod +x {} \;

echo "Making all shell scripts in .github/scripts executable..."

# Find all shell scripts and make them executable
find .github/scripts -name "*.sh" -type f -exec chmod +x {} \;

echo "All scripts are now executable."
