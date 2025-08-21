#!/bin/bash
# file: scripts/create-doc-update.sh
# version: 3.0.0
# guid: 4db28a8c-33e2-4853-9f0c-1d8283720bd1

set -euo pipefail

# Enhanced Documentation Update Script v3.0 with Enhanced Timestamp Format v2.0
#
# This script creates structured JSON files for documentation updates that can be
# processed by the reusable-docs-update.yml workflow. It supports multiple modes
# of operation and provides templates for common documentation updates.
#
# Enhanced Features:
# - Enhanced timestamp format v2.0 with lifecycle tracking (created_at, processed_at, failed_at)
# - Chronological processing support with sequence numbers
# - Parent GUID tracking for dependency management
# - Full backwards compatibility with existing workflows
#
# Usage examples:
#   ./scripts/create-doc-update.sh README.md "## New Feature\nAdded amazing functionality" append
#   ./scripts/create-doc-update.sh CHANGELOG.md "### Added\n- New feature" changelog-entry
#   ./scripts/create-doc-update.sh TODO.md "- [ ] Fix bug #123" task-add
#   ./scripts/create-doc-update.sh --template changelog-fix "Fixed critical bug in authentication"

print_usage() {
  cat >&2 << 'EOF'
Enhanced Documentation Update Script

Usage:
  $0 FILE CONTENT [MODE] [OPTIONS]
  $0 --template TEMPLATE_TYPE CONTENT [OPTIONS]
  $0 --list-templates
  $0 --help

Arguments:
  FILE                 Target file to update (e.g., README.md, CHANGELOG.md, TODO.md)
  CONTENT             Content to add/update
  MODE                Update mode (default: append)

Modes:
  append              Append content to end of file
  prepend             Prepend content to beginning of file
  replace             Replace entire file content
  replace-section     Replace specific section (requires --section)
  insert-after        Insert after specific line (requires --after)
  insert-before       Insert before specific line (requires --before)
  changelog-entry     Add changelog entry with proper formatting
  task-add            Add task to TODO list
  task-complete       Mark task as complete (requires --task-id)
  update-badge        Update README badge (requires --badge-name)

Templates:
  changelog-fix       Add bug fix entry to CHANGELOG.md
  changelog-feature   Add feature entry to CHANGELOG.md
  changelog-breaking  Add breaking change entry to CHANGELOG.md
  todo-task           Add new task to TODO.md
  todo-epic           Add new epic/section to TODO.md
  readme-badge        Update or add badge to README.md
  readme-section      Add new section to README.md

Options:
  --section SECTION   Section name for replace-section mode
  --after TEXT        Text to insert after
  --before TEXT       Text to insert before
  --task-id ID        Task ID for task operations
  --badge-name NAME   Badge name for badge operations
  --priority HIGH|MED|LOW  Task priority (default: MED)
  --category CAT      Category for organization
  --dry-run           Show what would be created without creating it
  --interactive       Interactive mode for complex updates

Examples:
  # Basic append
  $0 README.md "## New Section\nContent here" append

  # Changelog entry using template
  $0 --template changelog-feature "Added user authentication system"

  # Add TODO task with priority
  $0 TODO.md "Implement OAuth2 integration" task-add --priority HIGH

  # Replace specific section
  $0 README.md "Updated installation instructions" replace-section --section "Installation"

  # Interactive mode
  $0 --interactive
EOF
}

print_templates() {
  cat >&2 << 'EOF'
Available Templates:

changelog-fix       - Add bug fix entry to CHANGELOG.md
changelog-feature   - Add new feature entry to CHANGELOG.md
changelog-breaking  - Add breaking change entry to CHANGELOG.md
todo-task          - Add new task to TODO.md
todo-epic          - Add new epic/section to TODO.md
readme-badge       - Update or add badge to README.md
readme-section     - Add new section to README.md

Usage: $0 --template TEMPLATE_TYPE "Description"
EOF
}

generate_uuid() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]'
  else
    python3 -c 'import uuid; print(uuid.uuid4())'
  fi
}

get_timestamp() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

create_changelog_entry() {
  local type="$1"
  local description="$2"
  local timestamp="$(get_timestamp)"
  local date_only="${timestamp:0:10}"

  case "$type" in
    "fix")
      echo "### Fixed"
      echo ""
      echo "- $description"
      ;;
    "feature")
      echo "### Added"
      echo ""
      echo "- $description"
      ;;
    "breaking")
      echo "### Changed"
      echo ""
      echo "- **BREAKING**: $description"
      ;;
    *)
      echo "### Changed"
      echo ""
      echo "- $description"
      ;;
  esac
}

create_todo_task() {
  local description="$1"
  local priority="${2:-MED}"
  local category="${3:-General}"

  local priority_icon=""
  case "$priority" in
    "HIGH") priority_icon="🔴" ;;
    "MED") priority_icon="🟡" ;;
    "LOW") priority_icon="🟢" ;;
  esac

  echo "- [ ] $priority_icon **$category**: $description"
}

create_readme_badge() {
  local badge_name="$1"
  local content="$2"

  echo "Badge update for: $badge_name"
  echo "Content: $content"
}

interactive_mode() {
  echo "🔧 Interactive Documentation Update Mode"
  echo "========================================"

  echo "Select target file:"
  select file in "README.md" "CHANGELOG.md" "TODO.md" "Custom"; do
    case $file in
      "Custom")
        read -p "Enter custom filename: " file
        break
        ;;
      "")
        echo "Invalid selection"
        ;;
      *)
        break
        ;;
    esac
  done

  echo "Select update mode:"
  select mode in "append" "prepend" "changelog-entry" "task-add" "replace-section"; do
    case $mode in
      "")
        echo "Invalid selection"
        ;;
      *)
        break
        ;;
    esac
  done

  read -p "Enter content: " content

  case $mode in
    "task-add")
      echo "Select priority:"
      select priority in "HIGH" "MED" "LOW"; do
        case $priority in
          "")
            echo "Invalid selection"
            ;;
          *)
            break
            ;;
        esac
      done

      read -p "Enter category (optional): " category
      content="$(create_todo_task "$content" "$priority" "${category:-General}")"
      ;;
    "changelog-entry")
      echo "Select entry type:"
      select entry_type in "feature" "fix" "breaking"; do
        case $entry_type in
          "")
            echo "Invalid selection"
            ;;
          *)
            break
            ;;
        esac
      done

      content="$(create_changelog_entry "$entry_type" "$content")"
      ;;
  esac

  echo "Creating update for: $file"
  echo "Mode: $mode"
  echo "Content preview:"
  echo "----------------"
  echo "$content"
  echo "----------------"

  read -p "Proceed? [y/N]: " confirm
  if [[ "$confirm" =~ ^[Yy]$ ]]; then
    create_update "$file" "$content" "$mode"
  else
    echo "Cancelled."
    exit 0
  fi
}

create_update() {
  local file="$1"
  local content="$2"
  local mode="${3:-append}"
  local uuid="$(generate_uuid)"
  local timestamp="$(get_timestamp)"
  local dir=".github/doc-updates"

  mkdir -p "$dir"
  local path="$dir/${uuid}.json"

  # Create comprehensive update file with enhanced timestamp format v2.0
  jq -n \
    --arg file "$file" \
    --arg mode "$mode" \
    --arg content "$content" \
    --arg guid "$uuid" \
    --arg timestamp "$timestamp" \
    --arg section "$SECTION" \
    --arg after "$AFTER" \
    --arg before "$BEFORE" \
    --arg task_id "$TASK_ID" \
    --arg badge_name "$BADGE_NAME" \
    --arg priority "$PRIORITY" \
    --arg category "$CATEGORY" \
    '{
      file: $file,
      mode: $mode,
      content: $content,
      guid: $guid,
      created_at: $timestamp,
      processed_at: null,
      failed_at: null,
      sequence: 0,
      parent_guid: null,
      options: {
        section: (if $section != "" then $section else null end),
        after: (if $after != "" then $after else null end),
        before: (if $before != "" then $before else null end),
        task_id: (if $task_id != "" then $task_id else null end),
        badge_name: (if $badge_name != "" then $badge_name else null end),
        priority: (if $priority != "" then $priority else null end),
        category: (if $category != "" then $category else null end)
      }
    }' > "$path"

  echo "✅ Created doc update: $path"
  echo "   File: $file"
  echo "   Mode: $mode"
  echo "   GUID: $uuid"

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "   (Dry run - file would be created)"
    rm "$path"
  fi
}

process_template() {
  local template="$1"
  local description="$2"

  case "$template" in
    "changelog-fix")
      create_update "CHANGELOG.md" "$(create_changelog_entry "fix" "$description")" "changelog-entry"
      ;;
    "changelog-feature")
      create_update "CHANGELOG.md" "$(create_changelog_entry "feature" "$description")" "changelog-entry"
      ;;
    "changelog-breaking")
      create_update "CHANGELOG.md" "$(create_changelog_entry "breaking" "$description")" "changelog-entry"
      ;;
    "todo-task")
      create_update "TODO.md" "$(create_todo_task "$description" "${PRIORITY:-MED}" "${CATEGORY:-General}")" "task-add"
      ;;
    "todo-epic")
      local epic_content="## $description\n\n**Status**: 🔴 Not Started\n**Target**: TBD\n**Dependencies**: None\n\n### Tasks\n\n- [ ] TODO: Define specific tasks"
      create_update "TODO.md" "$epic_content" "append"
      ;;
    "readme-badge")
      create_update "README.md" "$(create_readme_badge "${BADGE_NAME:-$description}" "$description")" "update-badge"
      ;;
    "readme-section")
      local section_content="## $description\n\nTODO: Add content for this section"
      create_update "README.md" "$section_content" "append"
      ;;
    *)
      echo "❌ Unknown template: $template" >&2
      print_templates
      exit 1
      ;;
  esac
}

# Parse command line arguments
SECTION=""
AFTER=""
BEFORE=""
TASK_ID=""
BADGE_NAME=""
PRIORITY=""
CATEGORY=""
DRY_RUN="false"
INTERACTIVE="false"

while [[ $# -gt 0 ]]; do
  case $1 in
    --help|-h)
      print_usage
      exit 0
      ;;
    --list-templates)
      print_templates
      exit 0
      ;;
    --template)
      if [[ $# -lt 3 ]]; then
        echo "❌ Template requires template type and description" >&2
        exit 1
      fi
      process_template "$2" "$3"
      exit 0
      ;;
    --interactive)
      INTERACTIVE="true"
      shift
      ;;
    --section)
      SECTION="$2"
      shift 2
      ;;
    --after)
      AFTER="$2"
      shift 2
      ;;
    --before)
      BEFORE="$2"
      shift 2
      ;;
    --task-id)
      TASK_ID="$2"
      shift 2
      ;;
    --badge-name)
      BADGE_NAME="$2"
      shift 2
      ;;
    --priority)
      PRIORITY="$2"
      shift 2
      ;;
    --category)
      CATEGORY="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -*)
      echo "❌ Unknown option: $1" >&2
      print_usage
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

# Handle interactive mode
if [[ "$INTERACTIVE" == "true" ]]; then
  interactive_mode
  exit 0
fi

# Validate required arguments for non-template mode
if [[ $# -lt 2 ]]; then
  print_usage
  exit 1
fi

FILE="$1"
CONTENT="$2"
MODE="${3:-append}"

# Validate mode-specific required options
case "$MODE" in
  "insert-after")
    if [[ -z "$AFTER" ]]; then
      echo "❌ --after option is required for insert-after mode" >&2
      exit 1
    fi
    ;;
  "insert-before")
    if [[ -z "$BEFORE" ]]; then
      echo "❌ --before option is required for insert-before mode" >&2
      exit 1
    fi
    ;;
  "replace-section")
    if [[ -z "$SECTION" ]]; then
      echo "❌ --section option is required for replace-section mode" >&2
      exit 1
    fi
    ;;
  "task-complete")
    if [[ -z "$TASK_ID" ]]; then
      echo "❌ --task-id option is required for task-complete mode" >&2
      exit 1
    fi
    ;;
  "update-badge")
    if [[ -z "$BADGE_NAME" ]]; then
      echo "❌ --badge-name option is required for update-badge mode" >&2
      exit 1
    fi
    ;;
esac

create_update "$FILE" "$CONTENT" "$MODE"
