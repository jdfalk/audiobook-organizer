#!/bin/bash
# file: scripts/convert-legacy-issues.sh
# version: 1.0.0
# guid: 9a8b7c6d-5e4f-3a2b-1c9d-8e7f6a5b4c3d
#
# Convert legacy named issue update files to GUID format
#
# Usage:
#   ./scripts/convert-legacy-issues.sh

set -euo pipefail

echo "🔄 Converting legacy issue update files to GUID format..."

# Source our library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/create-issue-update-library.sh"

converted_count=0

# Process all legacy files
for file in .github/issue-updates/close-issue-*.json .github/issue-updates/update-issue-*.json; do
  if [[ -f "$file" ]]; then
    echo "📄 Processing: $(basename "$file")"

    # Extract the data from the file using jq if available, otherwise use grep
    if command -v jq >/dev/null 2>&1; then
      action=$(jq -r '.action' "$file")
      number=$(jq -r '.number' "$file")
      body=$(jq -r '.body // ""' "$file")
      state_reason=$(jq -r '.state_reason // "completed"' "$file")
      labels=$(jq -r '.labels[]? // empty' "$file" | tr '\n' ',' | sed 's/,$//')
    else
      # Fallback to grep parsing
      action=$(grep '"action"' "$file" | cut -d'"' -f4)
      number=$(grep '"number"' "$file" | cut -d':' -f2 | tr -d ', ')
      body=$(grep '"body"' "$file" | cut -d'"' -f4 | sed 's/\\n/\n/g' || echo "")
      state_reason=$(grep '"state_reason"' "$file" | cut -d'"' -f4 || echo "completed")
      labels=$(grep '"labels"' "$file" | cut -d'[' -f2 | cut -d']' -f1 | tr -d '"' | tr -d ' ' || echo "")
    fi

    echo "  ✓ Action: $action, Number: $number"

    # Generate new GUID for the file
    uuid=$(generate_unique_guid)
    new_file=".github/issue-updates/${uuid}.json"

    # Create the new GUID-format file with proper structure
    case "$action" in
      "close")
        guid=$(generate_unique_guid)
        legacy_guid=$(generate_legacy_guid "close" "$number")

        cat > "$new_file" << EOF
{
  "action": "close",
  "number": $number,
  "state_reason": "$state_reason",$(if [[ -n "$body" ]]; then echo "
  \"body\": \"$body\","; fi)
  "guid": "$guid",
  "legacy_guid": "$legacy_guid"
}
EOF
        ;;
      "update")
        guid=$(generate_unique_guid)
        legacy_guid=$(generate_legacy_guid "update" "$number")

        cat > "$new_file" << EOF
{
  "action": "update",
  "number": $number,
  "body": "$body",
  "labels": [$(echo "$labels" | sed 's/,/", "/g' | sed 's/^/"/;s/$/"/')],
  "guid": "$guid",
  "legacy_guid": "$legacy_guid"
}
EOF
        ;;
      "comment")
        guid=$(generate_unique_guid)
        legacy_guid=$(generate_legacy_guid "comment" "$number")

        cat > "$new_file" << EOF
{
  "action": "comment",
  "number": $number,
  "body": "$body",
  "guid": "$guid",
  "legacy_guid": "$legacy_guid"
}
EOF
        ;;
    esac

    # Remove the old file
    rm "$file"
    echo "  ✅ Converted: $(basename "$file") → $(basename "$new_file")"
    ((converted_count++))
  fi
done

if [[ $converted_count -eq 0 ]]; then
  echo "ℹ️  No legacy files found to convert"
else
  echo "🎉 Successfully converted $converted_count legacy files to GUID format"
fi
