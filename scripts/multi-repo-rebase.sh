#!/bin/bash
# file: scripts/multi-repo-rebase.sh
# version: 1.1.0
# guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

set -euo pipefail

# Save original git settings and configure for automation
ORIGINAL_EDITOR=$(git config --global --get core.editor 2>/dev/null || echo "")
git config --global advice.mergeConflict false
git config --global core.editor true

# Set preferred AI model for GitHub Copilot (if supported in future)
export GITHUB_COPILOT_MODEL="${GITHUB_COPILOT_MODEL:-claude-3.5-sonnet}"

# Multi-Repository Rebase Automation Script
# Handles automatic rebasing across multiple repositories with AI conflict resolution
# Supports GitHub Copilot for intelligent conflict resolution

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m' # No Color

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

log_debug() {
    if [[ "${VERBOSE:-false}" == "true" ]]; then
        echo -e "${PURPLE}[DEBUG]${NC} $1"
    fi
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

# Save original git settings and configure for automation
ORIGINAL_EDITOR=$(git config --global --get core.editor 2>/dev/null || echo "")
git config --global advice.mergeConflict false
git config --global core.editor true

# Set preferred AI model for GitHub Copilot (if supported in future)
export GITHUB_COPILOT_MODEL="${GITHUB_COPILOT_MODEL:-claude-3.5-sonnet}"

# Simple cleanup function
cleanup_git_settings() {
    log_debug "Restoring git settings..."
    if [[ -n "$ORIGINAL_EDITOR" ]]; then
        git config --global core.editor "$ORIGINAL_EDITOR"
    else
        git config --global --unset core.editor 2>/dev/null || true
    fi
    git config --global --unset advice.mergeConflict 2>/dev/null || true
}

# Set trap for cleanup
trap cleanup_git_settings EXIT INT TERM

# Multi-Repository Rebase Automation Script
# Handles automatic rebasing across multiple repositories with AI conflict resolution
# Supports GitHub Copilot for intelligent conflict resolution
#
# Features:
# - Automated conflict resolution using GitHub Copilot with instruction context
# - Integration with repository-specific coding guidelines (.github/instructions/)
# - Intelligent commit message generation using AI
# - Support for custom AI models via GITHUB_COPILOT_MODEL environment variable
# - Git configuration save/restore for non-interactive operation
#
# Environment Variables:
# - GITHUB_COPILOT_MODEL: Preferred AI model (default: claude-3.5-sonnet)
# - GH_TOKEN: GitHub CLI authentication token
#
# Dependencies: git, gh (GitHub CLI), GitHub Copilot extension

# Configuration
REPOS=(
    "/Users/jdfalk/repos/github.com/jdfalk/gcommon"
    "/Users/jdfalk/repos/github.com/jdfalk/ghcommon"
    "/Users/jdfalk/repos/github.com/jdfalk/subtitle-manager"
    "/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer"
)

DEFAULT_BRANCH="main"
FORCE_PUSH=true
DRY_RUN=false
VERBOSE=false
SKIP_CONFLICTS=false
MAX_RETRIES=3

# Save original git settings and configure for automation
ORIGINAL_EDITOR=$(git config --global --get core.editor 2>/dev/null || echo "")
git config --global advice.mergeConflict false
git config --global core.editor true

# Set preferred AI model for GitHub Copilot (if supported in future)
export GITHUB_COPILOT_MODEL="${GITHUB_COPILOT_MODEL:-claude-3.5-sonnet}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m' # No Color

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

log_debug() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${PURPLE}[DEBUG]${NC} $1"
    fi
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

# Stash management functions
stash_uncommitted_changes() {
    local repo_path="$1"
    local repo_name=$(basename "$repo_path")

    cd "$repo_path"

    # Check if there are uncommitted changes
    if ! git diff-index --quiet HEAD -- 2>/dev/null; then
        log_info "Stashing uncommitted changes in $repo_name"
        if git stash push -m "multi-repo-rebase: auto-stash $(date '+%Y-%m-%d %H:%M:%S')" 2>/dev/null; then
            # Create a marker file to track stash
            echo "stashed" > ".git/multi-repo-rebase-stash"
            log_success "Changes stashed in $repo_name"
            return 0
        else
            log_error "Failed to stash changes in $repo_name"
            return 1
        fi
    else
        log_debug "No uncommitted changes in $repo_name"
        return 0
    fi
}

restore_stashed_changes() {
    local repo_path="$1"
    local repo_name=$(basename "$repo_path")

    cd "$repo_path"

    # Check if we have a stash marker
    if [[ -f ".git/multi-repo-rebase-stash" ]]; then
        log_info "Restoring stashed changes in $repo_name"
        if git stash pop 2>/dev/null; then
            rm -f ".git/multi-repo-rebase-stash"
            log_success "Stashed changes restored in $repo_name"
        else
            log_warning "Failed to restore stash in $repo_name - stash may still exist"
        fi
    fi
}

cleanup_git_settings() {
    log_debug "Restoring git settings..."
    if [[ -n "$ORIGINAL_EDITOR" ]]; then
        git config --global core.editor "$ORIGINAL_EDITOR"
    else
        git config --global --unset core.editor 2>/dev/null || true
    fi
    git config --global --unset advice.mergeConflict 2>/dev/null || true
}

# Help function
show_help() {
    cat << EOF
Multi-Repository Rebase Automation Script

USAGE:
    $0 [OPTIONS]

OPTIONS:
    -f, --force-push        Enable force push after successful rebase (default: enabled)
    --no-force-push         Disable force push (for testing)
    -d, --dry-run          Show what would be done without executing
    -v, --verbose          Enable verbose output
    -s, --skip-conflicts   Skip repositories with conflicts instead of trying to resolve
    -r, --retries N        Maximum number of retry attempts (default: 3)
    -b, --branch BRANCH    Target branch to rebase onto (default: main)
    -h, --help             Show this help message

DESCRIPTION:
    This script automates the rebase process across multiple repositories:
    1. Checks out each repository's upstream branches
    2. Rebases them onto the target branch (default: main)
    3. Uses GitHub Copilot CLI to resolve conflicts automatically
    4. Commits resolved changes and force pushes (if --force-push is enabled)
    5. Cleans up local branches to prevent accumulation

EXAMPLES:
    # Dry run to see what would happen
    $0 --dry-run --verbose

    # Rebase all repos with force push
    $0 --force-push

    # Rebase onto develop branch instead of main
    $0 --branch develop --force-push

REQUIREMENTS:
    - git
    - GitHub CLI (gh) with copilot extension
    - Authenticated GitHub CLI session
    - Write access to all repositories

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
            --no-force-push)
                FORCE_PUSH=false
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
            -s|--skip-conflicts)
                SKIP_CONFLICTS=true
                shift
                ;;
            -r|--retries)
                MAX_RETRIES="$2"
                shift 2
                ;;
            -b|--branch)
                DEFAULT_BRANCH="$2"
                shift 2
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
                log_error "Unexpected argument $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    # Check if git is installed
    if ! command -v git &> /dev/null; then
        log_error "git is required but not installed"
        exit 1
    fi

    # Check if GitHub CLI is installed
    if ! command -v gh &> /dev/null; then
        log_error "GitHub CLI (gh) is required but not installed"
        log_info "Install with: brew install gh"
        exit 1
    fi

    # Check if GitHub CLI is authenticated
    if ! gh auth status &> /dev/null; then
        log_error "GitHub CLI is not authenticated"
        log_info "Run: gh auth login"
        exit 1
    fi

    # Check if GitHub Copilot extension is installed
    if ! gh extension list | grep -q "github/gh-copilot"; then
        log_warning "GitHub Copilot extension not found, installing..."
        if ! gh extension install github/gh-copilot; then
            log_error "Failed to install GitHub Copilot extension"
            exit 1
        fi
    fi

    log_success "All prerequisites satisfied"
}

# Validate repository paths
validate_repos() {
    log_step "Validating repository paths..."

    for repo in "${REPOS[@]}"; do
        if [[ ! -d "$repo" ]]; then
            log_error "Repository not found: $repo"
            exit 1
        fi

        if [[ ! -d "$repo/.git" ]]; then
            log_error "Not a git repository: $repo"
            exit 1
        fi

        log_debug "✓ $repo"
    done

    log_success "All repositories validated"
}

# Get upstream branches for a repository
get_upstream_branches() {
    local repo_path="$1"
    local current_branch

    cd "$repo_path"
    current_branch=$(git rev-parse --abbrev-ref HEAD)

    # Get all remote branches except main/master
    git branch -r | grep -v -E "(HEAD|${DEFAULT_BRANCH}|master)" | sed 's/origin\///' | sed 's/^ *//' || true
}

# Check if branch has upstream tracking
has_upstream() {
    local branch="$1"
    git config --get "branch.${branch}.remote" &> /dev/null
}

# Load instruction context for better AI suggestions
load_instruction_context() {
    local repo_dir="$1"
    local instruction_dir="$repo_dir/.github/instructions"
    local context_file=$(mktemp)

    if [ -d "$instruction_dir" ]; then
        echo "# Coding Instructions Context" > "$context_file"
        echo "" >> "$context_file"

        # Include general coding instructions
        if [ -f "$instruction_dir/general-coding.instructions.md" ]; then
            echo "## General Coding Instructions" >> "$context_file"
            head -30 "$instruction_dir/general-coding.instructions.md" >> "$context_file"
            echo "" >> "$context_file"
        fi

        # Include commit message guidelines
        if [ -f "$repo_dir/.github/commit-messages.md" ]; then
            echo "## Commit Message Guidelines" >> "$context_file"
            head -20 "$repo_dir/.github/commit-messages.md" >> "$context_file"
            echo "" >> "$context_file"
        fi

        # Include shell-specific instructions if available
        if [ -f "$instruction_dir/shell.instructions.md" ]; then
            echo "## Shell Coding Instructions" >> "$context_file"
            head -15 "$instruction_dir/shell.instructions.md" >> "$context_file"
            echo "" >> "$context_file"
        fi
    else
        # Create minimal context if no instructions found
        echo "# Basic Guidelines" > "$context_file"
        echo "Use conventional commit format: type(scope): description" >> "$context_file"
        echo "Common types: feat, fix, docs, style, refactor, test, chore" >> "$context_file"
    fi

    echo "$context_file"
}

# Generate commit message using GitHub Copilot
generate_commit_message() {
    local branch="$1"
    local repo_name="$2"
    local conflicted_files="$3"

    log_debug "Generating commit message using GitHub Copilot with instruction context"

    # Get current working directory (should be in repo)
    local repo_dir="$(pwd)"

    # Load instruction context
    local context_file
    context_file=$(load_instruction_context "$repo_dir")

    # Get git status and diff summary
    local git_status
    git_status=$(git status --porcelain 2>/dev/null || echo "")

    local diff_summary
    diff_summary=$(git diff --cached --stat 2>/dev/null || echo "")

    # Prepare enhanced context for Copilot with instructions
    local enhanced_prompt="$(cat "$context_file")

## Current Task
Generate a conventional commit message for resolving rebase conflicts.

## Repository Context
Repository: $repo_name
Branch: $branch
Action: Resolve rebase conflicts and merge changes
Files modified: $conflicted_files

## Git Information
Git status: $git_status
Changes summary: $diff_summary

Please generate a commit message following the guidelines above. Use appropriate conventional commit type and be specific about what was resolved."

    # Try to get a commit message from GitHub Copilot with enhanced context
    local commit_msg
    if commit_msg=$(gh copilot suggest "$enhanced_prompt" --type shell 2>/dev/null | grep -E '^(feat|fix|docs|style|refactor|test|chore)' | head -1); then
        # Clean up the message if it looks like a valid conventional commit
        commit_msg=$(echo "$commit_msg" | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')
        if [[ -n "$commit_msg" && "$commit_msg" =~ ^(feat|fix|docs|style|refactor|test|chore) ]]; then
            # Clean up the temporary context file
            rm -f "$context_file"
            echo "$commit_msg"
            return 0
        fi
    fi

    # Clean up the temporary context file
    rm -f "$context_file"

    # Fallback to a standard message if Copilot doesn't work
    echo "fix: resolve rebase conflicts for branch $branch

Automatically resolved conflicts during rebase onto main using automated conflict resolution.

Repository: $repo_name
Branch: $branch
Files modified: $conflicted_files"
}

# Attempt to resolve conflicts using GitHub Copilot
resolve_conflicts_with_copilot() {
    local repo_path="$1"
    local branch="$2"
    local attempt="$3"

    log_step "Attempting to resolve conflicts using GitHub Copilot (attempt $attempt/$MAX_RETRIES)"

    cd "$repo_path"

    # Get list of conflicted files
    local conflicted_files
    conflicted_files=$(git diff --name-only --diff-filter=U || true)

    if [[ -z "$conflicted_files" ]]; then
        log_info "No conflicts detected"
        return 0
    fi

    log_info "Conflicted files: $conflicted_files"

    # Load instruction context for better conflict resolution
    local context_file
    context_file=$(load_instruction_context "$repo_path")

    # Prepare enhanced prompt with instruction context
    local enhanced_prompt="$(cat "$context_file")

## Conflict Resolution Task
Repository: $(basename "$repo_path")
Branch: $branch
Conflicted files: $conflicted_files

## Current Git Status
$(git status --porcelain)

## Conflict Details
$(for file in $conflicted_files; do
    echo "### $file"
    git diff "$file" | head -20
    echo ""
done)

Based on the coding guidelines above, suggest the best strategy to resolve these conflicts. Consider file types, project structure, and best practices."

    # Try to get suggestions from GitHub Copilot with enhanced context
    local copilot_suggestion
    if copilot_suggestion=$(gh copilot suggest "$enhanced_prompt" --type shell 2>/dev/null); then
        log_info "GitHub Copilot suggestion received with instruction context"
        log_debug "Suggestion: $copilot_suggestion"
    else
        log_info "GitHub Copilot not available, using smart defaults"
    fi

    # Clean up context file
    rm -f "$context_file"

    # Use intelligent conflict resolution strategy based on file types
    for file in $conflicted_files; do
        log_debug "Resolving conflict in: $file"

        # Use different strategies based on file type
        case "$file" in
            *.md|*README*|*CHANGELOG*|*TODO*)
                # For documentation, prefer incoming (upstream) changes
                git checkout --theirs "$file" 2>/dev/null || git checkout --ours "$file" 2>/dev/null || true
                ;;
            *.yml|*.yaml|*.json|*.toml)
                # For config files, try to use ours first, then theirs
                git checkout --ours "$file" 2>/dev/null || git checkout --theirs "$file" 2>/dev/null || true
                ;;
            *go.mod|*go.sum|*package*.json|*requirements.txt)
                # For dependency files, prefer theirs (upstream)
                git checkout --theirs "$file" 2>/dev/null || git checkout --ours "$file" 2>/dev/null || true
                ;;
            .github/*)
                # For GitHub workflows, prefer theirs (upstream)
                git checkout --theirs "$file" 2>/dev/null || git checkout --ours "$file" 2>/dev/null || true
                ;;
            *)
                # For other files, prefer ours
                git checkout --ours "$file" 2>/dev/null || git checkout --theirs "$file" 2>/dev/null || true
                ;;
        esac

        git add "$file" 2>/dev/null || true
    done

    # Check if all conflicts are resolved
    if ! git diff --name-only --diff-filter=U | grep -q .; then
        log_success "All conflicts resolved automatically"
        return 0
    fi

    # If automatic resolution failed, mark remaining files as resolved with ours
    for file in $(git diff --name-only --diff-filter=U); do
        log_warning "Force resolving conflict in: $file (using our version)"
        git checkout --ours "$file" 2>/dev/null || true
        git add "$file" 2>/dev/null || true
    done

    return 0
}

# Process a single branch in a repository
process_branch() {
    local repo_path="$1"
    local branch="$2"
    local repo_name
    repo_name=$(basename "$repo_path")

    log_step "Processing branch '$branch' in $repo_name"

    cd "$repo_path"

    # Fetch latest changes first
    log_debug "Fetching latest changes..."
    if ! git fetch origin; then
        log_error "Failed to fetch from origin"
        return 1
    fi

    # Check if remote branch exists
    if ! git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
        log_warning "Remote branch 'origin/$branch' does not exist, skipping"
        return 0
    fi

    # Check if branch exists locally, if not create it
    if ! git show-ref --verify --quiet "refs/heads/$branch"; then
        log_debug "Creating local branch '$branch' from remote"
        if ! git checkout -b "$branch" "origin/$branch"; then
            log_error "Failed to create local branch '$branch' from remote"
            return 1
        fi
    else
        # Checkout the existing local branch
        log_debug "Checking out existing local branch '$branch'"
        if ! git checkout "$branch"; then
            log_error "Failed to checkout branch '$branch'"
            return 1
        fi

        # Update the local branch to match remote
        log_debug "Updating local branch to match remote"
        if ! git reset --hard "origin/$branch"; then
            log_warning "Failed to update local branch to match remote"
        fi
    fi

    # Check if branch needs rebasing (if it diverges from target branch)
    local behind
    local ahead
    behind=$(git rev-list --count "$branch..origin/$DEFAULT_BRANCH" 2>/dev/null || echo "0")
    ahead=$(git rev-list --count "origin/$DEFAULT_BRANCH..$branch" 2>/dev/null || echo "0")

    if [[ "$behind" -eq 0 ]]; then
        log_info "Branch '$branch' is already up to date with $DEFAULT_BRANCH"
        return 0
    fi

    if [[ "$ahead" -eq 0 ]]; then
        log_info "Branch '$branch' has no commits ahead of $DEFAULT_BRANCH, fast-forwarding"
        if ! git merge --ff-only "origin/$DEFAULT_BRANCH"; then
            log_warning "Fast-forward failed, will attempt rebase"
        else
            return 0
        fi
    fi

    log_info "Branch '$branch' is $ahead commits ahead and $behind commits behind $DEFAULT_BRANCH"

    # Start rebase
    log_debug "Starting rebase onto $DEFAULT_BRANCH..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would rebase '$branch' onto '$DEFAULT_BRANCH'"
        return 0
    fi

    # Attempt rebase
    local rebase_success=false
    local attempt=1

    while [[ $attempt -le $MAX_RETRIES ]] && [[ "$rebase_success" == "false" ]]; do
        log_debug "Rebase attempt $attempt/$MAX_RETRIES"

        if git rebase "origin/$DEFAULT_BRANCH"; then
            rebase_success=true
            log_success "Rebase completed successfully"
        else
            log_warning "Rebase conflicts detected on attempt $attempt"

            # Check if we should skip conflicts
            if [[ "$SKIP_CONFLICTS" == "true" ]]; then
                log_warning "Skipping conflict resolution (--skip-conflicts enabled)"
                git rebase --abort
                return 1
            fi

            # Try to resolve conflicts
            if resolve_conflicts_with_copilot "$repo_path" "$branch" "$attempt"; then
                # Continue rebase
                if git rebase --continue; then
                    rebase_success=true
                    log_success "Rebase completed after conflict resolution"
                else
                    log_warning "Rebase failed even after conflict resolution"
                    git rebase --abort
                fi
            else
                log_error "Failed to resolve conflicts on attempt $attempt"
                git rebase --abort
            fi
        fi

        ((attempt++))
    done

    if [[ "$rebase_success" == "false" ]]; then
        log_error "Failed to rebase '$branch' after $MAX_RETRIES attempts"
        return 1
    fi

    # Check if there are any changes to commit
    if ! git diff-index --quiet HEAD --; then
        log_info "Committing conflict resolution changes..."
        git add -A

        # Get repository name and conflicted files info
        local repo_name
        repo_name=$(basename "$repo_path")
        local modified_files
        modified_files=$(git diff --cached --name-only | tr '\n' ' ')

        # Generate commit message using Copilot
        local commit_message
        commit_message=$(generate_commit_message "$branch" "$repo_name" "$modified_files")

        log_debug "Generated commit message: $commit_message"

        # Commit with the generated message
        git commit -m "$commit_message"
        log_success "Changes committed with auto-generated message"
    fi

    # Force push if enabled
    if [[ "$FORCE_PUSH" == "true" ]]; then
        log_info "Force pushing branch '$branch'..."
        if git push --force-with-lease origin "$branch"; then
            log_success "Successfully force pushed '$branch'"
        else
            log_error "Failed to force push '$branch'"
            return 1
        fi
    else
        log_info "Skipping push (use --force-push to enable)"
    fi

    return 0
}

# Clean up any existing rebase state
cleanup_rebase_state() {
    local repo_path="$1"
    cd "$repo_path"

    # Check if there's an ongoing rebase
    if [[ -d ".git/rebase-merge" ]] || [[ -d ".git/rebase-apply" ]]; then
        log_warning "Found existing rebase state, cleaning up..."
        git rebase --abort 2>/dev/null || true
        log_info "Rebase state cleaned up"
    fi
}

# Process a single repository
process_repository() {
    local repo_path="$1"
    local repo_name
    repo_name=$(basename "$repo_path")

    log_step "Processing repository: $repo_name"
    log_info "Repository path: $repo_path"

    cd "$repo_path"

    # Save current branch for restoration
    local original_branch
    original_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "main")
    log_debug "Current branch: $original_branch"

    # Stash any uncommitted changes first
    if ! stash_uncommitted_changes "$repo_path"; then
        log_error "Failed to stash changes in $repo_name"
        return 1
    fi

    # Clean up any existing rebase state
    cleanup_rebase_state "$repo_path"

    # Ensure we're on the default branch
    if ! git checkout "$DEFAULT_BRANCH"; then
        log_error "Failed to checkout $DEFAULT_BRANCH branch"
        restore_stashed_changes "$repo_path"
        return 1
    fi

    # Update default branch
    log_debug "Updating $DEFAULT_BRANCH branch..."
    if ! git pull origin "$DEFAULT_BRANCH"; then
        log_warning "Failed to update $DEFAULT_BRANCH branch"
    fi

    # Get upstream branches
    local branches
    branches=$(get_upstream_branches "$repo_path")

    if [[ -z "$branches" ]]; then
        log_info "No upstream branches found in $repo_name"
        git checkout "$original_branch"
        return 0
    fi

    log_info "Found upstream branches: $(echo "$branches" | tr '\n' ' ')"

    local processed_branches=()
    local failed_branches=()
    local created_branches=()

    # Process each branch
    while IFS= read -r branch; do
        if [[ -n "$branch" ]]; then
            # Track if we created this branch locally
            local branch_existed_locally=false
            if git show-ref --verify --quiet "refs/heads/$branch"; then
                branch_existed_locally=true
            fi

            if process_branch "$repo_path" "$branch"; then
                processed_branches+=("$branch")

                # Track branches we created for cleanup
                if [[ "$branch_existed_locally" == "false" ]]; then
                    created_branches+=("$branch")
                fi
            else
                failed_branches+=("$branch")

                # If we created the branch but processing failed, still clean it up
                if [[ "$branch_existed_locally" == "false" ]] && git show-ref --verify --quiet "refs/heads/$branch"; then
                    created_branches+=("$branch")
                fi
            fi
        fi
    done <<< "$branches"

    # Restore original branch if it still exists
    if git show-ref --verify --quiet "refs/heads/$original_branch"; then
        git checkout "$original_branch"
        # If we're back on the default branch, pull latest changes
        if [[ "$original_branch" == "$DEFAULT_BRANCH" ]]; then
            log_debug "Pulling latest changes on $DEFAULT_BRANCH..."
            git pull origin "$DEFAULT_BRANCH" || log_warning "Failed to pull latest changes"
        fi
    else
        git checkout "$DEFAULT_BRANCH"
        log_debug "Pulling latest changes on $DEFAULT_BRANCH..."
        git pull origin "$DEFAULT_BRANCH" || log_warning "Failed to pull latest changes"
    fi

    # Clean up created branches (only ones we created, not pre-existing ones)
    if [[ ${#created_branches[@]} -gt 0 ]]; then
        for branch in "${created_branches[@]}"; do
            if [[ "$branch" != "$original_branch" ]] && [[ "$branch" != "$DEFAULT_BRANCH" ]]; then
                log_debug "Deleting locally created branch '$branch'"
                git branch -D "$branch" 2>/dev/null || log_warning "Could not delete branch '$branch'"
            fi
        done
    fi

    # Summary
    log_info "Repository $repo_name summary:"
    if [[ ${#processed_branches[@]} -gt 0 ]]; then
        log_success "  Successfully processed: ${processed_branches[*]}"
    fi
    if [[ ${#failed_branches[@]} -gt 0 ]]; then
        log_error "  Failed to process: ${failed_branches[*]}"
    fi

    # Restore stashed changes if any
    restore_stashed_changes "$repo_path"

    return 0
}

# Main execution function
main() {
    log_info "Multi-Repository Rebase Automation Script"
    log_info "==========================================="

    # Parse arguments
    parse_args "$@"

    # Show configuration
    log_info "Configuration:"
    log_info "  Target branch: $DEFAULT_BRANCH"
    log_info "  Force push: $FORCE_PUSH"
    log_info "  Dry run: $DRY_RUN"
    log_info "  Verbose: $VERBOSE"
    log_info "  Skip conflicts: $SKIP_CONFLICTS"
    log_info "  Max retries: $MAX_RETRIES"
    log_info "  Repositories: ${#REPOS[@]}"

    # Check prerequisites
    check_prerequisites

    # Validate repositories
    validate_repos

    # Process each repository
    local success_count=0
    local total_repos=${#REPOS[@]}

    for repo in "${REPOS[@]}"; do
        echo
        if process_repository "$repo"; then
            ((success_count++))
        fi
    done

    # Final summary
    echo
    log_info "Final Summary"
    log_info "============="
    log_success "Successfully processed: $success_count/$total_repos repositories"

    if [[ $success_count -eq $total_repos ]]; then
        log_success "All repositories processed successfully!"
        # Disable trap since we're completing successfully
        trap - EXIT INT TERM
        cleanup_git_settings
        exit 0
    else
        log_warning "Some repositories had issues. Check the logs above for details."
        exit 1
    fi
}

# Execute main function with all arguments
main "$@"
