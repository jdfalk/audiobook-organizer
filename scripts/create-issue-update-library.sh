#!/bin/bash
# file: scripts/create-issue-update-library.sh
# version: 2.0.1
# guid: 4a81c3e0-5f7b-4e2a-b92d-6c8a7c1d3e5f
#
# Enhanced Library for creating GitHub issue update files with enhanced timestamp format v2.0
# This file is meant to be sourced by other scripts, not executed directly
#

# Function to generate legacy GUID for backward compatibility
generate_legacy_guid() {
    local action="$1"
    local title_or_number="$2"
    local date=$(date +%Y-%m-%d)

    case "$action" in
        "create")
            # For create actions, use title
            local clean_title=$(echo "$title_or_number" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^\-|-$//g')
            echo "create-${clean_title}-${date}"
            ;;
        "update"|"comment"|"close")
            # For other actions, use issue number
            echo "${action}-issue-${title_or_number}-${date}"
            ;;
        *)
            echo "${action}-${date}"
            ;;
    esac
}

# Function to generate UUID
generate_uuid() {
    if command -v uuidgen >/dev/null 2>&1; then
        uuidgen | tr '[:upper:]' '[:lower:]'
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import uuid; print(str(uuid.uuid4()))"
    else
        echo "Error: Neither uuidgen nor python3 is available for UUID generation" >&2
        exit 1
    fi
}

# Function to check if issue already exists on GitHub
check_github_issue_exists() {
    local title="$1"
    local repo=""

    # Determine repository from environment or git remote if available
    if [[ -n "${GITHUB_REPOSITORY:-}" ]]; then
        repo="$GITHUB_REPOSITORY"
    else
        repo=$(git remote get-url origin 2>/dev/null | \
            sed 's/.*github.com[:/]\(.*\)\.git/\1/') || true
    fi

    local token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

    # If we don't have the necessary info, skip the GitHub check gracefully
    if [[ -z "$token" ]]; then
        echo "Warning: No GitHub token found. Skipping GitHub issue check." >&2
        return 1
    fi

    if [[ -z "$repo" ]]; then
        echo "Warning: No git remote 'origin' found. Skipping GitHub issue check." >&2
        return 1
    fi

    # Search for issues with matching title
    local search_result=$(curl -s -H "Authorization: token $token" \
        "https://api.github.com/repos/$repo/issues?state=all" | \
        python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    title = '''$title'''

    # Check if data is a list (successful API call) or dict (error response)
    if isinstance(data, dict):
        if 'message' in data:
            print(f'api_error:{data.get(\"message\", \"Unknown error\")}')
            sys.exit(1)
        else:
            print('not_found')
            sys.exit(0)

    # Data should be a list of issues
    if not isinstance(data, list):
        print('api_error:Unexpected response format')
        sys.exit(1)

    for issue in data:
        if isinstance(issue, dict) and issue.get('title', '').strip() == title.strip():
            print(f\"exists:{issue['number']}:{issue['state']}:{issue['html_url']}\")
            sys.exit(0)
    print('not_found')
except json.JSONDecodeError as e:
    print(f'json_error:{str(e)}')
    sys.exit(1)
except Exception as e:
    print(f'error:{str(e)}')
    sys.exit(1)
")

    if [[ "$search_result" == exists:* ]]; then
        IFS=':' read -r _ issue_number state url <<< "$search_result"
        echo "⚠️  Issue already exists: #$issue_number (state: $state)" >&2
        echo "   URL: $url" >&2
        return 0
    elif [[ "$search_result" == api_error:* ]]; then
        echo "⚠️  GitHub API error: ${search_result#api_error:}" >&2
        return 1
    elif [[ "$search_result" == json_error:* ]]; then
        echo "⚠️  JSON parsing error: ${search_result#json_error:}" >&2
        return 1
    elif [[ "$search_result" == error:* ]]; then
        echo "⚠️  General error: ${search_result#error:}" >&2
        return 1
    fi

    return 1
}

# Function to check if GUID is unique across the project
check_guid_unique() {
    local guid="$1"
    local project_root="$(pwd)"

    # Check if Python smart migration script exists
    if [[ -f "scripts/smart-issue-migration.py" ]]; then
        # Use Python script to validate uniqueness
        python3 -c "
import sys
sys.path.append('scripts')
from smart_issue_migration import validate_guid_uniqueness_in_project, SmartMigrator
import os

migrator = SmartMigrator('$project_root')
analysis = migrator.analyze_guid_duplicates()

# Check if the GUID already exists
if '$guid' in analysis['guid_map']:
    print('duplicate')
    sys.exit(1)
else:
    print('unique')
    sys.exit(0)
" 2>/dev/null || {
            # Fallback if Python script fails
            if grep -r "\"$guid\"" .github/issue-updates/ >/dev/null 2>&1 || \
               grep -r "\"$guid\"" issue_updates.json >/dev/null 2>&1; then
                echo "duplicate"
                return 1
            else
                echo "unique"
                return 0
            fi
        }
    else
        # Fallback: simple grep search
        if grep -r "\"$guid\"" .github/issue-updates/ >/dev/null 2>&1 || \
           grep -r "\"$guid\"" issue_updates.json >/dev/null 2>&1; then
            echo "duplicate"
            return 1
        else
            echo "unique"
            return 0
        fi
    fi
}

# Function to generate a guaranteed unique GUID
generate_unique_guid() {
    local max_attempts=10
    local attempt=1

    while [[ $attempt -le $max_attempts ]]; do
        local new_guid
        new_guid=$(generate_uuid)

        if check_guid_unique "$new_guid" | grep -q "unique"; then
            echo "$new_guid"
            return 0
        fi

        echo "⚠️  GUID collision detected (attempt $attempt/$max_attempts), generating new one..." >&2
        ((attempt++))
    done

    echo "❌ Failed to generate unique GUID after $max_attempts attempts" >&2
    exit 1
}

# Function to create JSON file
create_issue_file() {
    local action="$1"
    local uuid="$2"
    local file_path=".github/issue-updates/${uuid}.json"

    # Ensure directory exists
    mkdir -p ".github/issue-updates"

    case "$action" in
        "create")
            local title="$3"
            local body="$4"
            local labels="$5"
            local parent_issue="$6"  # New parameter for sub-issues
            local repo="$7"
            local guid="$uuid"  # Use the passed-in UUID as the GUID
            local legacy_guid
            local timestamp
            timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")

            # Note: GitHub duplicate check is handled by the workflow processing the JSON files
            # This script should only create JSON files, not make API calls

            legacy_guid=$(generate_legacy_guid "create" "$title")

            # Build JSON with enhanced timestamp format v2.0
            local json_content="{
  \"action\": \"create\",
  \"title\": \"$title\",
  \"body\": \"$body\",
  \"labels\": [$(echo "$labels" | sed 's/,/", "/g' | sed 's/^/"/;s/$/"/')],
  \"guid\": \"$guid\",
  \"legacy_guid\": \"$legacy_guid\",
  \"created_at\": \"$timestamp\",
  \"processed_at\": null,
  \"failed_at\": null,
  \"sequence\": 0,
  \"parent_guid\": null"

            # Add parent_issue if provided
            if [[ -n "$parent_issue" ]]; then
                json_content+=",
  \"parent_issue\": $parent_issue"
            fi
            if [[ -n "$repo" ]]; then
                json_content+=",
  \"repo\": \"$repo\""
            fi

            json_content+="
}"

            echo "$json_content" > "$file_path"
            ;;

        "update")
            local number="$3"
            local body="$4"
            local labels="$5"
            local parent_guid="$6"
            local repo="$7"
            local guid="$uuid"  # Use the passed-in UUID as the GUID
            local legacy_guid
            local timestamp
            timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
            legacy_guid=$(generate_legacy_guid "update" "$number")

            local num_field
            if [[ -z "$number" || "$number" == "null" ]]; then
                num_field="null"
            else
                num_field="$number"
            fi

            local json_content="{\n  \"action\": \"update\",\n  \"number\": $num_field,\n  \"body\": \"$body\",\n  \"labels\": [$(echo "$labels" | sed 's/,/\", \"/g' | sed 's/^/\"/;s/$/\"/')],\n  \"guid\": \"$guid\",\n  \"legacy_guid\": \"$legacy_guid\",\n  \"created_at\": \"$timestamp\",\n  \"processed_at\": null,\n  \"failed_at\": null,\n  \"sequence\": 0,\n  \"parent_guid\": null"

            if [[ -n "$parent_guid" ]]; then
                json_content+=" ,\n  \"parent\": \"$parent_guid\""
            fi
            if [[ -n "$repo" ]]; then
                json_content+=" ,\n  \"repo\": \"$repo\""
            fi

            json_content+="\n}"

            echo -e "$json_content" > "$file_path"
            ;;

        "comment")
            local number="$3"
            local body="$4"
            local parent_guid="$5"
            local repo="$6"
            local guid="$uuid"  # Use the passed-in UUID as the GUID
            local legacy_guid
            local timestamp
            timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
            legacy_guid=$(generate_legacy_guid "comment" "$number")

            local num_field
            if [[ -z "$number" || "$number" == "null" ]]; then
                num_field="null"
            else
                num_field="$number"
            fi

            local json_content="{\n  \"action\": \"comment\",\n  \"number\": $num_field,\n  \"body\": \"$body\",\n  \"guid\": \"$guid\",\n  \"legacy_guid\": \"$legacy_guid\",\n  \"created_at\": \"$timestamp\",\n  \"processed_at\": null,\n  \"failed_at\": null,\n  \"sequence\": 0,\n  \"parent_guid\": null"

            if [[ -n "$parent_guid" ]]; then
                json_content+=" ,\n  \"parent\": \"$parent_guid\""
            fi
            if [[ -n "$repo" ]]; then
                json_content+=" ,\n  \"repo\": \"$repo\""
            fi

            json_content+="\n}"

            echo -e "$json_content" > "$file_path"
            ;;

        "close")
            local number="$3"
            local state_reason="${4:-completed}"
            local parent_guid="$5"
            local repo="$6"
            local guid="$uuid"  # Use the passed-in UUID as the GUID
            local legacy_guid
            local timestamp
            timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.%3NZ")
            legacy_guid=$(generate_legacy_guid "close" "$number")

            local num_field
            if [[ -z "$number" || "$number" == "null" ]]; then
                num_field="null"
            else
                num_field="$number"
            fi

            local json_content="{\n  \"action\": \"close\",\n  \"number\": $num_field,\n  \"state_reason\": \"$state_reason\",\n  \"guid\": \"$guid\",\n  \"legacy_guid\": \"$legacy_guid\",\n  \"created_at\": \"$timestamp\",\n  \"processed_at\": null,\n  \"failed_at\": null,\n  \"sequence\": 0,\n  \"parent_guid\": null"

            if [[ -n "$parent_guid" ]]; then
                json_content+=" ,\n  \"parent\": \"$parent_guid\""
            fi
            if [[ -n "$repo" ]]; then
                json_content+=" ,\n  \"repo\": \"$repo\""
            fi

            json_content+="\n}"

            echo -e "$json_content" > "$file_path"
            ;;

        *)
            echo "Error: Unknown action '$action'" >&2
            echo "Supported actions: create, update, comment, close" >&2
            exit 1
            ;;
    esac

    echo "✅ Created: $file_path"
    echo "📄 UUID: $uuid"
    echo "🔧 Action: $action"
}

# Main function to handle command line arguments
run_issue_update() {
    if [[ $# -lt 2 ]]; then
        echo "Usage:"
        echo "  $0 create \"Title\" \"Body\" \"label1,label2\" [parent_issue] [repo]"
        echo "  $0 update NUMBER \"Body\" \"label1,label2\" [parent_guid] [repo]"
        echo "  $0 comment NUMBER \"Comment text\" [parent_guid] [repo]"
        echo "  $0 close NUMBER [state_reason] [parent_guid] [repo]"
        return 1
    fi

    local action="$1"
    local uuid=$(generate_uuid)

    case "$action" in
        "create")
            if [[ $# -lt 4 ]]; then
                echo "Error: create requires title, body, and labels" >&2
                echo "Usage: $0 create \"Title\" \"Body\" \"label1,label2\" [parent_issue] [repo]" >&2
                return 1
            fi
            create_issue_file "$action" "$uuid" "$2" "$3" "$4" "${5:-}" "${6:-}"
            ;;
        "update")
            if [[ $# -lt 4 ]]; then
                echo "Error: update requires number, body, and labels" >&2
                return 1
            fi
            create_issue_file "$action" "$uuid" "$2" "$3" "$4" "${5:-}" "${6:-}"
            ;;
        "comment")
            if [[ $# -lt 3 ]]; then
                echo "Error: comment requires number and body" >&2
                return 1
            fi
            create_issue_file "$action" "$uuid" "$2" "$3" "${4:-}" "${5:-}"
            ;;
        "close")
            if [[ $# -lt 2 ]]; then
                echo "Error: close requires number" >&2
                return 1
            fi
            create_issue_file "$action" "$uuid" "$2" "${3:-completed}" "${4:-}" "${5:-}"
            ;;
        *)
            echo "Error: Unknown action '$action'" >&2
            echo "Supported actions: create, update, comment, close" >&2
            return 1
            ;;
    esac
}
