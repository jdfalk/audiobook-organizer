#!/bin/bash
# file: scripts/migrate-instruction-files.sh
# version: 1.0.0
# guid: mig12345-e89b-12d3-a456-426614174000

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GHCOMMON_DIR="$(dirname "$SCRIPT_DIR")"
GHCOMMON_INSTRUCTIONS_DIR="$GHCOMMON_DIR/.github/instructions"

# Target repositories
REPOS=(
    "/Users/jdfalk/repos/github.com/jdfalk/subtitle-manager"
    "/Users/jdfalk/repos/github.com/jdfalk/gcommon"
    "/Users/jdfalk/repos/github.com/jdfalk/codex-cli"
)

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

# Function to backup old files
backup_old_files() {
    local repo_dir="$1"
    local backup_dir="$repo_dir/.github/instructions-backup-$(date +%Y%m%d-%H%M%S)"

    # Backup old standalone files if they exist
    local old_files=(
        "$repo_dir/.github/commit-messages.md"
        "$repo_dir/.github/pull-request-descriptions.md"
        "$repo_dir/.github/test-generation.md"
        "$repo_dir/.github/security-guidelines.md"
        "$repo_dir/.github/repository-setup.md"
        "$repo_dir/.github/review-selection.md"
        "$repo_dir/.github/workflow-usage.md"
    )

    local backup_needed=false
    for file in "${old_files[@]}"; do
        if [[ -f "$file" ]]; then
            backup_needed=true
            break
        fi
    done

    if [[ "$backup_needed" == "true" ]]; then
        log_info "Creating backup in $backup_dir"
        mkdir -p "$backup_dir"
        for file in "${old_files[@]}"; do
            if [[ -f "$file" ]]; then
                cp "$file" "$backup_dir/"
                log_info "Backed up $(basename "$file")"
            fi
        done
    else
        log_info "No old standalone files found to backup"
    fi

    # Backup duplicate instruction files in root .github if they exist
    if [[ -d "$repo_dir/.github/instructions" ]]; then
        local found_duplicates=false

        # Check if there are any duplicates before creating backup dir
        while IFS= read -r -d '' file; do
            basename_file=$(basename "$file")
            if [[ -f "$repo_dir/.github/instructions/$basename_file" ]]; then
                found_duplicates=true
                break
            fi
        done < <(find "$repo_dir/.github" -maxdepth 1 -name "*.instructions.md" -type f -print0 2>/dev/null)

        if [[ "$found_duplicates" == "true" ]]; then
            if [[ "$backup_needed" == "false" ]]; then
                log_info "Creating backup in $backup_dir"
                mkdir -p "$backup_dir"
            fi

            # Find instruction files in root .github that also exist in .github/instructions
            find "$repo_dir/.github" -maxdepth 1 -name "*.instructions.md" -type f 2>/dev/null | while read -r file; do
                basename_file=$(basename "$file")
                if [[ -f "$repo_dir/.github/instructions/$basename_file" ]]; then
                    cp "$file" "$backup_dir/"
                    log_info "Backed up duplicate root file: $basename_file"
                fi
            done
        else
            log_info "No duplicate instruction files found to backup"
        fi
    else
        log_info "No .github/instructions directory found - skipping duplicate check"
    fi
}

# Function to remove old files
remove_old_files() {
    local repo_dir="$1"

    log_info "Removing old standalone files from $repo_dir"

    # Remove old standalone files
    local old_files=(
        "$repo_dir/.github/commit-messages.md"
        "$repo_dir/.github/pull-request-descriptions.md"
        "$repo_dir/.github/test-generation.md"
        "$repo_dir/.github/security-guidelines.md"
        "$repo_dir/.github/repository-setup.md"
        "$repo_dir/.github/review-selection.md"
        "$repo_dir/.github/workflow-usage.md"
    )

    local files_removed=0
    for file in "${old_files[@]}"; do
        if [[ -f "$file" ]]; then
            rm "$file"
            log_success "Removed $(basename "$file")"
            ((files_removed++))
        fi
    done

    if [[ $files_removed -eq 0 ]]; then
        log_info "No old standalone files found to remove"
    fi

    # Remove duplicate instruction files from root .github
    if [[ -d "$repo_dir/.github/instructions" ]]; then
        local duplicates_removed=0
        find "$repo_dir/.github" -maxdepth 1 -name "*.instructions.md" -type f 2>/dev/null | while read -r file; do
            basename_file=$(basename "$file")
            if [[ -f "$repo_dir/.github/instructions/$basename_file" ]]; then
                rm "$file"
                log_success "Removed duplicate root file: $basename_file"
                ((duplicates_removed++))
            fi
        done

        if [[ $duplicates_removed -eq 0 ]]; then
            log_info "No duplicate instruction files found to remove"
        fi
    else
        log_info "No .github/instructions directory found - skipping duplicate removal"
    fi
}

# Function to copy instruction files from ghcommon
copy_instruction_files() {
    local repo_dir="$1"
    local target_instructions_dir="$repo_dir/.github/instructions"

    log_info "Copying instruction files to $repo_dir"

    # Create instructions directory if it doesn't exist
    mkdir -p "$target_instructions_dir"

    # Copy all instruction files from ghcommon
    if [[ -d "$GHCOMMON_INSTRUCTIONS_DIR" ]]; then
        local files_copied=0
        local files_updated=0
        local files_skipped=0

        find "$GHCOMMON_INSTRUCTIONS_DIR" -name "*.instructions.md" -type f | while read -r file; do
            basename_file=$(basename "$file")
            target_file="$target_instructions_dir/$basename_file"

            if [[ -f "$target_file" ]]; then
                # File exists, check if it needs updating
                if ! cmp -s "$file" "$target_file"; then
                    cp "$file" "$target_file"
                    log_success "Updated $basename_file"
                    ((files_updated++))
                else
                    log_info "Skipped $basename_file (already up-to-date)"
                    ((files_skipped++))
                fi
            else
                # New file
                cp "$file" "$target_file"
                log_success "Copied $basename_file"
                ((files_copied++))
            fi
        done

        if [[ $files_copied -eq 0 && $files_updated -eq 0 && $files_skipped -gt 0 ]]; then
            log_info "All instruction files are already up-to-date"
        fi
    else
        log_error "Source instructions directory not found: $GHCOMMON_INSTRUCTIONS_DIR"
        return 1
    fi
}

# Function to update copilot-instructions.md if needed
update_copilot_instructions() {
    local repo_dir="$1"
    local copilot_file="$repo_dir/.github/copilot-instructions.md"
    local source_file="$GHCOMMON_DIR/.github/copilot-instructions.md"

    if [[ ! -f "$source_file" ]]; then
        log_error "Source copilot-instructions.md not found: $source_file"
        return 1
    fi

    if [[ -f "$copilot_file" ]]; then
        # File exists, check if it needs updating
        if ! cmp -s "$source_file" "$copilot_file"; then
            log_info "Updating copilot-instructions.md in $repo_dir"
            cp "$source_file" "$copilot_file"
            log_success "Updated copilot-instructions.md"
        else
            log_info "copilot-instructions.md is already up-to-date"
        fi
    else
        log_info "Creating copilot-instructions.md in $repo_dir"
        cp "$source_file" "$copilot_file"
        log_success "Created copilot-instructions.md"
    fi
}

# Function to process a single repository
process_repository() {
    local repo_dir="$1"
    local repo_name=$(basename "$repo_dir")

    log_info "Processing repository: $repo_name"

    if [[ ! -d "$repo_dir" ]]; then
        log_error "Repository directory not found: $repo_dir"
        return 1
    fi

    # Create .github directory if it doesn't exist
    mkdir -p "$repo_dir/.github"

    # Backup old files
    backup_old_files "$repo_dir"

    # Remove old files
    remove_old_files "$repo_dir"

    # Copy instruction files
    copy_instruction_files "$repo_dir"

    # Update copilot-instructions.md
    update_copilot_instructions "$repo_dir"

    log_success "Completed processing $repo_name"
}

# Main execution
main() {
    log_info "Starting instruction file migration from ghcommon"
    log_info "Source: $GHCOMMON_INSTRUCTIONS_DIR"

    # Verify source directory exists
    if [[ ! -d "$GHCOMMON_INSTRUCTIONS_DIR" ]]; then
        log_error "Source instructions directory not found: $GHCOMMON_INSTRUCTIONS_DIR"
        exit 1
    fi

    # List available instruction files
    log_info "Available instruction files in ghcommon:"
    find "$GHCOMMON_INSTRUCTIONS_DIR" -name "*.instructions.md" -type f | while read -r file; do
        echo "  - $(basename "$file")"
    done

    # Process each repository
    for repo in "${REPOS[@]}"; do
        echo
        process_repository "$repo"
    done

    echo
    log_success "Migration completed successfully!"
    log_info "All repositories now use centralized instruction files from ghcommon"
    log_info "Old files have been backed up with timestamps"
}

# Run the script
main "$@"
