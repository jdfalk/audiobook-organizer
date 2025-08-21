#!/bin/bash
# file: scripts/rebase.sh

# Smart Git Rebase Tool - Shell Implementation
#
# A lightweight shell-based rebase automation tool that provides basic
# intelligent conflict resolution and backup management. This is the
# fallback implementation for environments where Python is not available.
#
# Features:
# - Basic conflict resolution based on file patterns
# - Automatic backup branch creation
# - Simple logging and summary generation
# - Multiple operation modes (interactive, automated, smart)
# - Dry-run support

set -euo pipefail

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

# Global variables
VERBOSE=false
DRY_RUN=false
MODE="smart"
FORCE_PUSH=false
TARGET_BRANCH=""
SOURCE_BRANCH=""
BACKUP_BRANCH=""
START_TIME=""
SUMMARY_FILE=""
CONFLICTS_RESOLVED=0
TOTAL_CONFLICTS=0

# Arrays for tracking
declare -a RESOLVED_FILES=()
declare -a ERROR_MESSAGES=()
declare -a RECOVERY_INSTRUCTIONS=()

# Logging functions
log_info() {
    echo -e "${BLUE}[REBASE]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[REBASE]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[REBASE]${NC} $1"
}

log_error() {
    echo -e "${RED}[REBASE]${NC} $1"
}

log_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo -e "${CYAN}[VERBOSE]${NC} $1"
    fi
}

# Show help message
show_help() {
    cat << 'EOF'
Smart Git Rebase Tool - Shell Implementation

A lightweight shell-based rebase automation tool with basic intelligent
conflict resolution and backup management.

Usage: rebase.sh [OPTIONS] <target-branch>

OPTIONS:
    --mode MODE         Rebase mode: interactive, automated, smart (default: smart)
    -f, --force-push    Force push after successful rebase
    -d, --dry-run       Show what would be done without executing
    -v, --verbose       Enable verbose output
    -h, --help          Show this help message

EXAMPLES:
    rebase.sh main                    # Smart rebase onto main
    rebase.sh --mode automated main   # Fully automated rebase
    rebase.sh --force-push main       # Rebase and force push
    rebase.sh --dry-run main          # Preview what would happen

MODES:
    interactive  - User-driven with prompts for conflicts
    automated    - Fully automated (AI/CI friendly)
    smart        - Basic automation with simple fallbacks (default)

CONFLICT RESOLUTION:
The shell version provides basic conflict resolution strategies:
- Documentation files (*.md, docs/*): Prefer incoming changes
- Build/CI files (.github/*, Dockerfile*, etc.): Prefer incoming changes
- Package files (go.mod, package.json, etc.): Prefer incoming changes
- Other files: Prefer incoming changes as safe default

RECOVERY:
If something goes wrong, the tool creates backup branches and provides
recovery instructions in the generated summary file.
EOF
}

# Run command with optional output capture
run_command() {
    local cmd=("$@")
    log_verbose "Running command: ${cmd[*]}"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would execute: ${cmd[*]}"
        return 0
    fi

    if [[ "$VERBOSE" == true ]]; then
        "${cmd[@]}"
    else
        "${cmd[@]}" >/dev/null 2>&1
    fi
}

# Run command and capture output
run_command_output() {
    local cmd=("$@")
    log_verbose "Running command with output: ${cmd[*]}"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would execute: ${cmd[*]}"
        echo ""  # Return empty output for dry run
        return 0
    fi

    "${cmd[@]}" 2>/dev/null || true
}

# Get current Git branch
get_current_branch() {
    run_command_output git branch --show-current
}

# Check if branch exists
branch_exists() {
    local branch_name="$1"
    git show-ref --verify --quiet "refs/heads/$branch_name" 2>/dev/null
}

# Create backup branch
create_backup_branch() {
    local source_branch="$1"
    local timestamp
    timestamp=$(date +"%Y%m%d_%H%M%S")
    BACKUP_BRANCH="backup/${source_branch}/${timestamp}"

    if [[ "$DRY_RUN" == false ]]; then
        git branch "$BACKUP_BRANCH" "$source_branch"
    fi

    log_success "Created backup branch: $BACKUP_BRANCH"
    RECOVERY_INSTRUCTIONS+=("To restore backup: git checkout $BACKUP_BRANCH")
}

# Get list of conflicted files
get_conflicted_files() {
    if [[ "$DRY_RUN" == true ]]; then
        echo ""
        return 0
    fi

    git diff --name-only --diff-filter=U 2>/dev/null || true
}

# Determine conflict resolution strategy for a file
get_conflict_strategy() {
    local file_path="$1"

    # Documentation files - prefer incoming
    if [[ "$file_path" =~ \.md$ ]] || [[ "$file_path" =~ ^docs/ ]] || \
       [[ "$file_path" =~ ^README ]] || [[ "$file_path" =~ ^CHANGELOG ]] || \
       [[ "$file_path" =~ ^TODO ]]; then
        echo "incoming"
        return 0
    fi

    # Build and CI files - prefer incoming
    if [[ "$file_path" =~ ^\.github/ ]] || [[ "$file_path" =~ ^Dockerfile ]] || \
       [[ "$file_path" =~ ^docker- ]] || [[ "$file_path" == "Makefile" ]] || \
       [[ "$file_path" =~ \.yml$ ]] || [[ "$file_path" =~ \.yaml$ ]]; then
        echo "incoming"
        return 0
    fi

    # Package management files - prefer incoming
    if [[ "$file_path" == "go.mod" ]] || [[ "$file_path" == "go.sum" ]] || \
       [[ "$file_path" == "package.json" ]] || [[ "$file_path" == "package-lock.json" ]] || \
       [[ "$file_path" == "requirements.txt" ]] || [[ "$file_path" =~ ^Pipfile ]]; then
        echo "incoming"
        return 0
    fi

    # Default strategy - prefer incoming as safe choice
    echo "incoming"
}

# Resolve conflict using specified strategy
resolve_conflict() {
    local file_path="$1"
    local strategy="$2"

    case "$strategy" in
        "incoming")
            if run_command git checkout --theirs "$file_path"; then
                if run_command git add "$file_path"; then
                    log_verbose "Resolved $file_path using incoming changes"
                    RESOLVED_FILES+=("$file_path:$strategy")
                    return 0
                fi
            fi
            ;;
        "current")
            if run_command git checkout --ours "$file_path"; then
                if run_command git add "$file_path"; then
                    log_verbose "Resolved $file_path using current changes"
                    RESOLVED_FILES+=("$file_path:$strategy")
                    return 0
                fi
            fi
            ;;
        *)
            log_error "Unknown strategy: $strategy"
            return 1
            ;;
    esac

    log_error "Failed to resolve $file_path with strategy $strategy"
    ERROR_MESSAGES+=("Failed to resolve conflict in $file_path")
    return 1
}

# Resolve all conflicts based on mode
resolve_conflicts() {
    local mode="$1"
    local conflicted_files

    conflicted_files=$(get_conflicted_files)

    if [[ -z "$conflicted_files" ]]; then
        log_info "No conflicts to resolve"
        return 0
    fi

    # Convert to array
    local files_array
    IFS=$'\n' read -rd '' -a files_array <<< "$conflicted_files" || true
    TOTAL_CONFLICTS=${#files_array[@]}

    log_info "Found $TOTAL_CONFLICTS conflicted files"

    local resolved_count=0
    for file_path in "${files_array[@]}"; do
        [[ -n "$file_path" ]] || continue

        log_info "Resolving conflict in: $file_path"

        if [[ "$mode" == "interactive" ]]; then
            log_warning "Conflict in $file_path - please resolve manually"
            read -p "Continue after resolving? (y/n): " -r response
            if [[ "$response" != "y" && "$response" != "Y" ]]; then
                return 1
            fi
            ((resolved_count++))
        else
            # Automated resolution
            local strategy
            strategy=$(get_conflict_strategy "$file_path")
            log_verbose "Using strategy $strategy for $file_path"

            if resolve_conflict "$file_path" "$strategy"; then
                ((resolved_count++))
            else
                log_error "Failed to resolve conflict in $file_path"
                return 1
            fi
        fi
    done

    CONFLICTS_RESOLVED=$resolved_count
    log_success "Resolved $resolved_count/$TOTAL_CONFLICTS conflicts"
    return 0
}

# Perform the Git rebase operation
perform_rebase() {
    local target_branch="$1"
    local mode="$2"

    log_info "Starting rebase onto $target_branch in $mode mode"

    # Start the rebase
    local rebase_cmd=("git" "rebase")
    if [[ "$mode" == "interactive" ]]; then
        rebase_cmd+=("-i")
    fi
    rebase_cmd+=("$target_branch")

    # Try to start rebase (allow it to fail)
    if [[ "$DRY_RUN" == false ]]; then
        if ! "${rebase_cmd[@]}" 2>/dev/null; then
            log_verbose "Rebase encountered conflicts or issues"
        fi
    else
        log_info "[DRY RUN] Would execute: ${rebase_cmd[*]}"
    fi

    # Check for conflicts
    local conflicted_files
    conflicted_files=$(get_conflicted_files)

    if [[ -n "$conflicted_files" ]]; then
        log_warning "Rebase has conflicts, attempting resolution"

        if resolve_conflicts "$mode"; then
            # Continue the rebase
            if [[ "$DRY_RUN" == false ]]; then
                if git rebase --continue; then
                    log_success "Rebase completed successfully"
                    return 0
                else
                    log_error "Failed to continue rebase after conflict resolution"
                    return 1
                fi
            else
                log_info "[DRY RUN] Would continue rebase"
                return 0
            fi
        else
            log_error "Failed to resolve all conflicts"
            RECOVERY_INSTRUCTIONS+=("To abort rebase: git rebase --abort")
            RECOVERY_INSTRUCTIONS+=("Review conflicted files and resolve manually")
            return 1
        fi
    else
        # Check if rebase completed successfully
        if [[ "$DRY_RUN" == false ]]; then
            if git status --porcelain | grep -q .; then
                log_warning "Rebase may not have completed cleanly"
                return 1
            else
                log_success "Rebase completed successfully"
                return 0
            fi
        else
            log_info "[DRY RUN] Rebase would complete successfully"
            return 0
        fi
    fi
}

# Force push changes
force_push_changes() {
    local branch_name="$1"

    log_info "Force pushing $branch_name to remote"

    if run_command git push --force-with-lease origin "$branch_name"; then
        log_success "Force push completed successfully"
        return 0
    else
        log_error "Force push failed"
        ERROR_MESSAGES+=("Force push to origin/$branch_name failed")
        return 1
    fi
}

# Generate summary file
generate_summary() {
    local result="$1"
    local end_time
    end_time=$(date "+%Y-%m-%dT%H:%M:%S")
    local execution_time="unknown"

    local timestamp
    timestamp=$(date +"%Y%m%d_%H%M%S")
    SUMMARY_FILE="rebase-summary-$timestamp.md"

    if [[ "$result" == "success" ]]; then
        RECOVERY_INSTRUCTIONS+=("Rebase completed successfully. No recovery needed.")
    elif [[ "$result" == "conflicts" ]]; then
        RECOVERY_INSTRUCTIONS+=("Rebase stopped due to unresolved conflicts.")
        RECOVERY_INSTRUCTIONS+=("To abort: git rebase --abort")
        RECOVERY_INSTRUCTIONS+=("Review conflicted files and resolve manually, then run: git rebase --continue")
    elif [[ "$result" == "failed" ]]; then
        RECOVERY_INSTRUCTIONS+=("Rebase failed to complete.")
        RECOVERY_INSTRUCTIONS+=("To abort: git rebase --abort")
        RECOVERY_INSTRUCTIONS+=("Check git status for current state")
    fi

    if [[ "$DRY_RUN" == false ]]; then
        cat > "$SUMMARY_FILE" << EOF
# Git Rebase Summary - $end_time

## Operation Details

- **Mode**: $MODE
- **Target Branch**: $TARGET_BRANCH
- **Source Branch**: $SOURCE_BRANCH
- **Result**: $result
- **Execution Time**: ${execution_time}
- **Backup Branch**: $BACKUP_BRANCH

## Conflicts Resolved ($CONFLICTS_RESOLVED/$TOTAL_CONFLICTS)

EOF

        for resolved_file in "${RESOLVED_FILES[@]}"; do
            echo "- **${resolved_file%%:*}**: ${resolved_file##*:}" >> "$SUMMARY_FILE"
        done

        if [[ ${#ERROR_MESSAGES[@]} -gt 0 ]]; then
            echo "" >> "$SUMMARY_FILE"
            echo "## Errors (${#ERROR_MESSAGES[@]})" >> "$SUMMARY_FILE"
            echo "" >> "$SUMMARY_FILE"
            for error in "${ERROR_MESSAGES[@]}"; do
                echo "- $error" >> "$SUMMARY_FILE"
            done
        fi

        echo "" >> "$SUMMARY_FILE"
        echo "## Recovery Instructions" >> "$SUMMARY_FILE"
        echo "" >> "$SUMMARY_FILE"
        for instruction in "${RECOVERY_INSTRUCTIONS[@]}"; do
            echo "- $instruction" >> "$SUMMARY_FILE"
        done
    fi

    log_success "Summary generated: $SUMMARY_FILE"
}

# Main rebase execution
run_rebase() {
    local target_branch="$1"
    local mode="$2"
    local force_push="$3"

    # Get current branch
    SOURCE_BRANCH=$(get_current_branch)
    if [[ -z "$SOURCE_BRANCH" ]]; then
        log_error "Could not determine current branch"
        return 1
    fi

    TARGET_BRANCH="$target_branch"
    MODE="$mode"
    FORCE_PUSH="$force_push"

    log_info "Rebasing $SOURCE_BRANCH onto $target_branch"

    # Validate target branch exists
    if [[ "$DRY_RUN" == false ]] && ! branch_exists "$target_branch"; then
        log_error "Target branch '$target_branch' does not exist"
        ERROR_MESSAGES+=("Target branch '$target_branch' does not exist")
        return 1
    fi

    # Create backup branch
    create_backup_branch "$SOURCE_BRANCH"

    # Fetch latest changes
    log_info "Fetching latest changes from remote"
    if ! run_command git fetch origin; then
        log_warning "Failed to fetch from remote, continuing anyway"
    fi

    # Perform the rebase
    local result="failed"
    if perform_rebase "$target_branch" "$mode"; then
        result="success"

        # Force push if requested and rebase succeeded
        if [[ "$force_push" == true ]]; then
            if ! force_push_changes "$SOURCE_BRANCH"; then
                log_warning "Rebase succeeded but force push failed"
            fi
        fi
    else
        # Check if it was conflicts or complete failure
        local conflicted_files
        conflicted_files=$(get_conflicted_files)
        if [[ -n "$conflicted_files" ]]; then
            result="conflicts"
        else
            result="failed"
        fi
    fi

    # Generate summary
    generate_summary "$result"

    case "$result" in
        "success")
            log_success "Rebase completed successfully"
            return 0
            ;;
        "conflicts")
            log_error "Rebase stopped due to conflicts"
            log_info "See $SUMMARY_FILE for recovery instructions"
            return 1
            ;;
        "failed")
            log_error "Rebase failed"
            log_info "See $SUMMARY_FILE for recovery instructions"
            return 2
            ;;
    esac
}

# Parse command line arguments
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --mode)
                MODE="$2"
                case "$MODE" in
                    interactive|automated|smart)
                        ;;
                    *)
                        log_error "Invalid mode: $MODE"
                        echo "Valid modes: interactive, automated, smart"
                        exit 1
                        ;;
                esac
                shift 2
                ;;
            -f|--force-push)
                FORCE_PUSH=true
                shift
                ;;
            -d|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            -*)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
            *)
                if [[ -z "$TARGET_BRANCH" ]]; then
                    TARGET_BRANCH="$1"
                else
                    log_error "Unexpected argument: $1"
                    show_help
                    exit 1
                fi
                shift
                ;;
        esac
    done

    # Validate required arguments
    if [[ -z "$TARGET_BRANCH" ]]; then
        log_error "Target branch is required"
        show_help
        exit 1
    fi
}

# Main function
main() {
    START_TIME=$(date "+%Y-%m-%dT%H:%M:%S")

    # Ensure we're in a Git repository
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        log_error "Not in a Git repository"
        exit 1
    fi

    # Parse arguments
    parse_arguments "$@"

    log_info "Smart Git Rebase Tool - Shell Implementation"

    # Run the rebase
    if run_rebase "$TARGET_BRANCH" "$MODE" "$FORCE_PUSH"; then
        exit 0
    else
        exit $?
    fi
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
