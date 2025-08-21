#!/bin/bash
# file: scripts/set-copilot-firewall.sh
# version: 2.0.0
# guid: b2c3d4e5-6f7g-8h9i-0j1k-2l3m4n5o6p7q
# Wrapper script for the Python-based Copilot Firewall Manager

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_TOOL_DIR="$SCRIPT_DIR/copilot-firewall"

# Check if Python is available
if ! command -v python3 &> /dev/null; then
    echo "Python 3 is not installed. Please install Python 3.8 or later."
    exit 1
fi

# Check if the Python tool directory exists
if [ ! -d "$PYTHON_TOOL_DIR" ]; then
    echo "Python tool directory not found: $PYTHON_TOOL_DIR"
    echo "Please ensure the copilot-firewall directory exists."
    exit 1
fi

# Change to the Python tool directory
cd "$PYTHON_TOOL_DIR" || exit 1

# Check if dependencies are installed
if ! python3 -c "import inquirer, rich" &> /dev/null; then
    echo "Installing required dependencies..."
    pip3 install inquirer rich
fi

# Run the Python tool with all arguments passed through
echo "Starting GitHub Copilot Firewall Allowlist Manager..."
python3 -m copilot_firewall.main "$@"
