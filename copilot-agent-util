#!/bin/bash
# file: tools/copilot-agent-util-rust/copilot-agent-util-wrapper.sh
# version: 1.0.0
# guid: d4e5f6a7-b8c9-0123-def0-456789012345

# Platform-detecting wrapper script for copilot-agent-util
# This script detects the current platform and executes the appropriate binary

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Detect platform
detect_platform() {
    local os arch

    # Detect OS
    case "$(uname -s)" in
        Darwin)  os="macos" ;;
        Linux)   os="linux" ;;
        *)       echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64)  arch="x86_64" ;;
        arm64)   arch="arm64" ;;
        aarch64) arch="arm64" ;;
        *)       echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac

    echo "${os}-${arch}"
}

# Get platform-specific binary
PLATFORM=$(detect_platform)
BINARY="$SCRIPT_DIR/bin/copilot-agent-util-${PLATFORM}"

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    echo "Error: Binary not found for platform $PLATFORM: $BINARY" >&2
    echo "Available binaries:" >&2
    ls -la "$SCRIPT_DIR/bin"/copilot-agent-util-* 2>/dev/null || echo "  No binaries found in $SCRIPT_DIR/bin/" >&2
    exit 1
fi

# Check if binary is executable
if [ ! -x "$BINARY" ]; then
    echo "Error: Binary is not executable: $BINARY" >&2
    exit 1
fi

# Execute the platform-specific binary with all arguments
exec "$BINARY" "$@"
