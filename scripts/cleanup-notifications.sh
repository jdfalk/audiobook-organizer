#!/bin/bash
# file: scripts/cleanup-notifications.sh
# version: 1.0.0
# guid: c3d4e5f6-a7b8-9012-cdef-345678901234

# Simple wrapper script for GitHub notifications cleanup
# Usage: ./scripts/cleanup-notifications.sh [OPTIONS]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_SCRIPT="$SCRIPT_DIR/mark_old_notifications_done.py"

# Check if Python script exists
if [ ! -f "$PYTHON_SCRIPT" ]; then
    echo "Error: Python script not found at $PYTHON_SCRIPT" >&2
    exit 1
fi

# Check if Python 3 is available
if ! command -v python3 &> /dev/null; then
    echo "Error: python3 is not installed or not in PATH" >&2
    exit 1
fi

# Check if required packages are installed
if ! python3 -c "import requests, dotenv" &> /dev/null; then
    echo "Installing required packages..."
    pip3 install -r "$SCRIPT_DIR/requirements-notifications.txt"
fi

# Run the Python script with all arguments passed through
exec python3 "$PYTHON_SCRIPT" "$@"
