#!/bin/bash
# file: scripts/cleanup-repos.sh
# version: 1.0.0
# guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

# Simple wrapper script for the Python repository cleanup tool

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_SCRIPT="$SCRIPT_DIR/cleanup-archived-repos.py"

# Check if Python script exists
if [[ ! -f "$PYTHON_SCRIPT" ]]; then
    echo "Error: Python script not found at $PYTHON_SCRIPT"
    exit 1
fi

# Check if gh CLI is available
if ! command -v gh &> /dev/null; then
    echo "Error: GitHub CLI (gh) is required but not installed."
    echo "Please install it from: https://cli.github.com/"
    exit 1
fi

# Check if user is authenticated with gh
if ! gh auth status &> /dev/null; then
    echo "Error: You need to authenticate with GitHub CLI first."
    echo "Run: gh auth login"
    exit 1
fi

echo "🧹 Repository Cleanup Tool"
echo "=========================="
echo ""

# Run the Python script with all provided arguments
python3 "$PYTHON_SCRIPT" "$@"
