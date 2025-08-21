#!/bin/bash
# file: scripts/create-issue-update.sh
# version: 2.0.0
# guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d
#
# Enhanced helper script to create new issue update files with enhanced timestamp format v2.0
#
# Usage:
#   ./scripts/create-issue-update.sh create "Issue Title" "Issue body" "label1,label2"
#   ./scripts/create-issue-update.sh update 123 "Updated body" "label1,label2"
#   ./scripts/create-issue-update.sh comment 123 "Comment text"
#   ./scripts/create-issue-update.sh close 123 "completed"

set -euo pipefail

# Source the library (this is the ghcommon repository, so library is local)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIBRARY_PATH="${SCRIPT_DIR}/create-issue-update-library.sh"

# Try to download from GitHub if not found locally
GITHUB_RAW_URL="https://raw.githubusercontent.com/jdfalk/ghcommon/main/scripts/create-issue-update-library.sh"
TEMP_LIBRARY_PATH="/tmp/create-issue-update-library-$$.sh"

# Source the library directly since we're in the ghcommon repository
if [[ -f "$LIBRARY_PATH" ]]; then
  source "$LIBRARY_PATH"
else
  echo "ERROR: Could not locate the create-issue-update-library.sh file at: $LIBRARY_PATH" >&2
  exit 1
fi

# Run the main function with all arguments
run_issue_update "$@"
