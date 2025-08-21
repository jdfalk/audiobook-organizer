#!/bin/bash
# file: scripts/smart-rebase.sh

# Smart Git Rebase Script with Intelligent Conflict Resolution
# This script handles rebasing with automatic conflict resolution strategies

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
FORCE_PUSH=${FORCE_PUSH:-false}
DRY_RUN=${DRY_RUN:-false}
VERBOSE=${VERBOSE:-false}
BACKUP_DIR="$(pwd)/.rebase-backup"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${BLUE}[VERBOSE]${NC} $1"
    fi
}

# Help function
show_help() {
    cat << EOF
Smart Git Rebase Script

Usage: $0 [OPTIONS] <target-branch>

OPTIONS:
    -f, --force-push    Force push after successful rebase
    -d, --dry-run       Show what would be done without executing
    -v, --verbose       Enable verbose output
    -h, --help          Show this help message

EXAMPLES:
    $0 main                    # Rebase current branch onto main
    $0 -f main                 # Rebase and force push
    $0 -v -f origin/main       # Verbose rebase and force push
    $0 --dry-run main          # Preview what would happen

ENVIRONMENT VARIABLES:
    FORCE_PUSH=true           # Enable force push
    DRY_RUN=true             # Enable dry run mode
    VERBOSE=true             # Enable verbose output

The script includes intelligent conflict resolution for common scenarios:
- Documentation conflicts (prefer incoming for docs/)
- Build/CI conflicts (prefer incoming for .github/, Dockerfile, etc.)
- Code conflicts (save both versions for manual review)
- JSON/config conflicts (attempt smart merge)
EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
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
                log_error "Unknown option $1"
                show_help
                exit 1
                ;;
            *)
                TARGET_BRANCH="$1"
                shift
                ;;
        esac
    done

    if [[ -z "${TARGET_BRANCH:-}" ]]; then
        log_error "Target branch is required"
        show_help
        exit 1
    fi
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check if we're in a git repository
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "Not in a git repository"
        exit 1
    fi

    # Check if target branch exists
    if ! git rev-parse --verify "$TARGET_BRANCH" > /dev/null 2>&1; then
        log_error "Target branch '$TARGET_BRANCH' does not exist"
        exit 1
    fi

    # Check for uncommitted changes
    if ! git diff-index --quiet HEAD --; then
        log_warning "You have uncommitted changes. They will be stashed."
        if [[ "$DRY_RUN" == "false" ]]; then
            git stash push -m "Auto-stash before smart rebase $(date)"
        fi
    fi

    # Get current branch
    CURRENT_BRANCH=$(git branch --show-current)
    if [[ "$CURRENT_BRANCH" == "$TARGET_BRANCH" ]]; then
        log_error "Cannot rebase branch onto itself"
        exit 1
    fi

    log_success "Prerequisites check passed"
}

# Set up remote if it doesn't exist (for CI environments)
setup_remote() {
    if ! git remote get-url origin >/dev/null 2>&1; then
        log_info "No 'origin' remote found, configuring for jdfalk/subtitle-manager"
        git remote add origin https://github.com/jdfalk/subtitle-manager.git
        log_info "Added origin remote: https://github.com/jdfalk/subtitle-manager.git"
    else
        log_info "Origin remote already configured: $(git remote get-url origin)"
    fi
}

# Create backup
create_backup() {
    log_info "Creating backup..."

    if [[ "$DRY_RUN" == "false" ]]; then
        mkdir -p "$BACKUP_DIR"

        # Backup current branch state
        git rev-parse HEAD > "$BACKUP_DIR/original-head.txt"
        echo "$CURRENT_BRANCH" > "$BACKUP_DIR/original-branch.txt"

        # Create a backup branch
        BACKUP_BRANCH="backup-$(date +%Y%m%d-%H%M%S)-$CURRENT_BRANCH"
        git branch "$BACKUP_BRANCH"
        echo "$BACKUP_BRANCH" > "$BACKUP_DIR/backup-branch.txt"

        log_success "Backup created: $BACKUP_BRANCH"
    else
        log_info "DRY RUN: Would create backup branch"
    fi
}

# Intelligent conflict resolution
resolve_conflict_intelligently() {
    local file="$1"
    local resolution_strategy=""

    log_verbose "Analyzing conflict in: $file"

    # Determine resolution strategy based on file type and location
    case "$file" in
        # Documentation files - prefer incoming (main branch)
        "README.md"|"CHANGELOG.md"|"TODO.md"|"*.md"|"docs/"*)
            resolution_strategy="incoming"
            log_info "Documentation conflict in $file - preferring incoming changes"
            ;;

        # Build and CI files - prefer incoming
        ".github/"*|"Dockerfile"*|"docker-compose"*|"Makefile"|".dockerignore"|".gitignore")
            resolution_strategy="incoming"
            log_info "Build/CI conflict in $file - preferring incoming changes"
            ;;

        # Package management files - prefer incoming
        "go.mod"|"go.sum"|"package.json"|"package-lock.json"|"requirements.txt"|"Cargo.toml"|"Cargo.lock")
            resolution_strategy="incoming"
            log_info "Package management conflict in $file - preferring incoming changes"
            ;;

        # Configuration files - attempt smart merge
        "*.json"|"*.yml"|"*.yaml"|"*.toml"|"*.ini"|"*.conf")
            resolution_strategy="smart_merge"
            log_info "Configuration conflict in $file - attempting smart merge"
            ;;

        # Code files - save both versions
        "*.go"|"*.js"|"*.ts"|"*.py"|"*.java"|"*.cpp"|"*.c"|"*.h"|"*.rs")
            resolution_strategy="save_both"
            log_info "Code conflict in $file - saving both versions"
            ;;

        # Default - save both versions
        *)
            resolution_strategy="save_both"
            log_info "Unknown file type conflict in $file - saving both versions"
            ;;
    esac

    case "$resolution_strategy" in
        "incoming")
            # Accept incoming changes (from target branch)
            git checkout --theirs "$file"
            git add "$file"
            ;;

        "current")
            # Keep current changes (from current branch)
            git checkout --ours "$file"
            git add "$file"
            ;;

        "smart_merge")
            # Attempt intelligent merge for configuration files
            if smart_merge_config "$file"; then
                git add "$file"
            else
                # Fall back to save_both if smart merge fails
                save_both_versions "$file"
            fi
            ;;

        "save_both")
            save_both_versions "$file"
            ;;
    esac

    return 0
}

# Smart merge for configuration files
smart_merge_config() {
    local file="$1"
    local temp_dir=$(mktemp -d)
    local current_file="$temp_dir/current"
    local incoming_file="$temp_dir/incoming"
    local merged_file="$temp_dir/merged"

    # Extract current and incoming versions
    git show :2:"$file" > "$current_file" 2>/dev/null || return 1
    git show :3:"$file" > "$incoming_file" 2>/dev/null || return 1

    # Attempt smart merge based on file type
    case "$file" in
        "*.json")
            if command -v jq >/dev/null 2>&1; then
                log_verbose "Attempting JSON smart merge for $file"
                if merge_json_files "$current_file" "$incoming_file" "$merged_file"; then
                    cp "$merged_file" "$file"
                    rm -rf "$temp_dir"
                    return 0
                fi
            fi
            ;;

        "*.yml"|"*.yaml")
            if command -v yq >/dev/null 2>&1; then
                log_verbose "Attempting YAML smart merge for $file"
                if merge_yaml_files "$current_file" "$incoming_file" "$merged_file"; then
                    cp "$merged_file" "$file"
                    rm -rf "$temp_dir"
                    return 0
                fi
            fi
            ;;
    esac

    rm -rf "$temp_dir"
    return 1
}

# Merge JSON files intelligently
merge_json_files() {
    local current="$1"
    local incoming="$2"
    local output="$3"

    # Simple strategy: merge objects at root level, prefer incoming for conflicts
    if jq -s '.[0] * .[1]' "$current" "$incoming" > "$output" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Merge YAML files intelligently
merge_yaml_files() {
    local current="$1"
    local incoming="$2"
    local output="$3"

    # Simple strategy: merge maps at root level, prefer incoming for conflicts
    if yq eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' "$current" "$incoming" > "$output" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Save both versions of conflicted files
save_both_versions() {
    local file="$1"
    local base_name="${file%.*}"
    local extension="${file##*.}"

    # Create filenames for both versions
    local current_version="${base_name}.${extension}.current"
    local incoming_version="${base_name}.${extension}.main.incoming"

    # Save current version (ours)
    git show :2:"$file" > "$current_version" 2>/dev/null || {
        log_warning "Could not extract current version of $file"
    }

    # Save incoming version (theirs)
    git show :3:"$file" > "$incoming_version" 2>/dev/null || {
        log_warning "Could not extract incoming version of $file"
    }

    # Keep current version as the resolved file
    git checkout --ours "$file"
    git add "$file"

    log_info "Saved conflict versions: $current_version, $incoming_version"
    log_info "Keeping current version in $file (review and merge manually)"
}

# Handle conflicts during rebase
handle_conflicts() {
    local conflicted_files
    conflicted_files=$(git diff --name-only --diff-filter=U)

    if [[ -z "$conflicted_files" ]]; then
        return 0
    fi

    log_info "Found conflicts in the following files:"
    echo "$conflicted_files" | while read -r file; do
        log_info "  - $file"
    done

    echo "$conflicted_files" | while read -r file; do
        if [[ -n "$file" ]]; then
            resolve_conflict_intelligently "$file"
        fi
    done

    # Continue rebase
    git rebase --continue
}

# Perform the rebase
perform_rebase() {
    log_info "Starting rebase of '$CURRENT_BRANCH' onto '$TARGET_BRANCH'..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would execute: git rebase $TARGET_BRANCH"
        log_info "DRY RUN: Current branch: $CURRENT_BRANCH"
        log_info "DRY RUN: Target branch: $TARGET_BRANCH"

        # Show what commits would be rebased
        log_info "Commits that would be rebased:"
        git log --oneline "$TARGET_BRANCH..$CURRENT_BRANCH" | head -10
        return 0
    fi

    # Start the rebase
    if git rebase "$TARGET_BRANCH"; then
        log_success "Rebase completed successfully without conflicts"
        return 0
    fi

    # Handle conflicts if they occur
    while git status --porcelain | grep -q "^UU\|^AA\|^DD"; do
        log_info "Handling conflicts..."
        handle_conflicts
    done

    log_success "Rebase completed with automatic conflict resolution"
}

# Push changes
push_changes() {
    if [[ "$FORCE_PUSH" == "true" ]]; then
        log_info "Force pushing changes..."

        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "DRY RUN: Would execute: git push --force-with-lease origin $CURRENT_BRANCH"
            return 0
        fi

        if git push --force-with-lease origin "$CURRENT_BRANCH"; then
            log_success "Force push completed successfully"
        else
            log_error "Force push failed. This might be due to:"
            log_error "1. Authentication issues (no push access)"
            log_error "2. Network connectivity problems"
            log_error "3. Branch protection rules"
            log_error "Remote URL: $(git remote get-url origin 2>/dev/null || echo 'Not configured')"
            return 1
        fi
    else
        log_info "Skipping push (use -f/--force-push to enable)"
    fi
}

# Cleanup function
cleanup_on_error() {
    log_error "Rebase failed. Cleaning up..."

    if [[ -f "$BACKUP_DIR/backup-branch.txt" ]]; then
        local backup_branch
        backup_branch=$(cat "$BACKUP_DIR/backup-branch.txt")
        log_info "You can recover using: git checkout $backup_branch"
    fi

    # Abort rebase if in progress
    if git status | grep -q "rebase in progress"; then
        log_info "Aborting rebase..."
        git rebase --abort
    fi
}

# Main function
main() {
    # Set trap for cleanup on error
    trap cleanup_on_error ERR

    parse_args "$@"

    log_info "Smart Rebase Script Starting..."
    log_info "Current branch: $(git branch --show-current)"
    log_info "Target branch: $TARGET_BRANCH"
    log_info "Force push: $FORCE_PUSH"
    log_info "Dry run: $DRY_RUN"
    log_info "Verbose: $VERBOSE"

    check_prerequisites
    setup_remote
    create_backup
    perform_rebase
    push_changes

    log_success "Smart rebase completed successfully!"

    if [[ -f "$BACKUP_DIR/backup-branch.txt" ]]; then
        local backup_branch
        backup_branch=$(cat "$BACKUP_DIR/backup-branch.txt")
        log_info "Backup branch created: $backup_branch"
        log_info "You can delete it with: git branch -D $backup_branch"
    fi
}

# Run main function with all arguments
main "$@"
