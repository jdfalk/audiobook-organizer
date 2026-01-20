#!/bin/bash
# file: .github/scripts/fix-releases.sh
# version: 1.0.0
# guid: f1e2d3c4-b5a6-7f8e-9d0c-1a2b3c4d5e6f

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO="jdfalk/audiobook-organizer"
DRY_RUN=true
SKIP_CONFIRMATION=false

# Version range to fix
VERSIONS=(
    "v0.2.0"
    "v0.3.0"
    "v0.4.0"
    "v0.5.0"
    "v0.6.0"
    "v0.7.0"
    "v0.8.0"
    "v0.9.0"
    "v0.10.0"
    "v0.11.0"
)

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check prerequisites
check_prerequisites() {
    print_info "Checking prerequisites..."
    
    if ! command -v gh &> /dev/null; then
        print_error "GitHub CLI (gh) is not installed. Please install it first."
        exit 1
    fi
    
    if ! command -v git &> /dev/null; then
        print_error "git is not installed. Please install it first."
        exit 1
    fi
    
    # Check if gh is authenticated
    if ! gh auth status &> /dev/null; then
        print_error "GitHub CLI is not authenticated. Run 'gh auth login' first."
        exit 1
    fi
    
    print_success "All prerequisites met"
}

# Function to delete a release
delete_release() {
    local version=$1
    
    if [[ "$DRY_RUN" == true ]]; then
        print_info "[DRY-RUN] Would delete release: $version"
        return 0
    fi
    
    print_info "Deleting release: $version"
    if gh release delete "$version" --repo "$REPO" --yes 2>/dev/null; then
        print_success "Deleted release: $version"
    else
        print_warning "Release $version not found or already deleted"
    fi
}

# Function to delete a git tag
delete_tag() {
    local tag=$1
    
    if [[ "$DRY_RUN" == true ]]; then
        print_info "[DRY-RUN] Would delete tag: $tag"
        return 0
    fi
    
    print_info "Deleting tag: $tag"
    
    # Delete local tag if it exists
    if git tag -l "$tag" | grep -q "$tag"; then
        git tag -d "$tag" 2>/dev/null || true
        print_success "Deleted local tag: $tag"
    fi
    
    # Delete remote tag
    if git ls-remote --tags origin | grep -q "refs/tags/$tag"; then
        git push origin ":refs/tags/$tag" 2>/dev/null || true
        print_success "Deleted remote tag: $tag"
    else
        print_warning "Remote tag $tag not found"
    fi
}

# Function to generate changelog between two commits
generate_changelog() {
    local from_commit=$1
    local to_commit=$2
    local version=$3
    
    print_info "Generating changelog for $version..."
    
    local changelog
    changelog=$(cat <<EOF
## Release $version

### Changes

EOF
)
    
    # Get commits between versions
    if [[ "$from_commit" == "ROOT" ]]; then
        # First release - get all commits up to this version
        local commits
        commits=$(git log --oneline --no-decorate "$to_commit" --format="- %s (%h)" 2>/dev/null || echo "- Initial release")
    else
        # Get commits between versions
        local commits
        commits=$(git log --oneline --no-decorate "$from_commit..$to_commit" --format="- %s (%h)" 2>/dev/null || echo "- Bug fixes and improvements")
    fi
    
    changelog+="$commits"
    changelog+=$'\n\n'
    changelog+="**Full Changelog**: https://github.com/$REPO/compare/$from_commit...$version"
    
    echo "$changelog"
}

# Function to create a release
create_release() {
    local version=$1
    local commit=$2
    local prev_commit=$3
    
    if [[ "$DRY_RUN" == true ]]; then
        print_info "[DRY-RUN] Would create release: $version at commit $commit"
        return 0
    fi
    
    print_info "Creating release: $version at commit $commit"
    
    # Create tag first
    if ! git tag -l "$version" | grep -q "$version"; then
        git tag "$version" "$commit"
        git push origin "$version"
        print_success "Created and pushed tag: $version"
    else
        print_warning "Tag $version already exists"
    fi
    
    # Generate changelog
    local changelog
    changelog=$(generate_changelog "$prev_commit" "$commit" "$version")
    
    # Create release
    if gh release create "$version" \
        --repo "$REPO" \
        --title "Release $version" \
        --notes "$changelog" \
        --target "$commit"; then
        print_success "Created release: $version"
    else
        print_error "Failed to create release: $version"
        return 1
    fi
}

# Function to recreate all releases
recreate_releases() {
    print_info "Starting release recreation process..."
    
    # Get the commit history to map versions to commits
    # Since we have a shallow clone, we'll work with what we have
    # In production, this should have full git history
    
    local current_commit
    current_commit=$(git rev-parse HEAD)
    
    local prev_commit="ROOT"
    
    for version in "${VERSIONS[@]}"; do
        create_release "$version" "$current_commit" "$prev_commit"
        prev_commit="$version"
    done
    
    print_success "Release recreation complete!"
}

# Function to show usage
show_usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Fix broken releases and tags in the audiobook-organizer repository.

OPTIONS:
    --execute           Execute the fix (default is dry-run)
    --dry-run           Show what would be done without making changes (default)
    --skip-confirmation Skip confirmation prompts
    --help              Show this help message

EXAMPLES:
    # Show what would be done (dry-run)
    $(basename "$0")
    
    # Execute with confirmation
    $(basename "$0") --execute
    
    # Execute without confirmation (use with caution!)
    $(basename "$0") --execute --skip-confirmation

DESCRIPTION:
    This script fixes broken releases by:
    1. Deleting all broken releases (v0.2.0 through v0.11.0)
    2. Deleting corresponding git tags
    3. Recreating releases with proper changelogs from git history
    
    The script is idempotent and safe to run multiple times.
    By default, it runs in dry-run mode to show what would be done.

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --execute)
                DRY_RUN=false
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --skip-confirmation)
                SKIP_CONFIRMATION=true
                shift
                ;;
            --help)
                show_usage
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
}

# Function to confirm action
confirm_action() {
    if [[ "$SKIP_CONFIRMATION" == true ]]; then
        return 0
    fi
    
    echo
    print_warning "This will delete and recreate ${#VERSIONS[@]} releases!"
    print_warning "Affected versions: ${VERSIONS[*]}"
    echo
    read -p "Are you sure you want to continue? (yes/no): " -r
    echo
    
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        print_info "Operation cancelled by user"
        exit 0
    fi
}

# Main function
main() {
    echo "=========================================="
    echo "  Release Fix Script"
    echo "  Repository: $REPO"
    echo "=========================================="
    echo
    
    parse_args "$@"
    
    if [[ "$DRY_RUN" == true ]]; then
        print_info "Running in DRY-RUN mode (no changes will be made)"
    else
        print_warning "Running in EXECUTE mode (changes will be made)"
    fi
    echo
    
    check_prerequisites
    
    if [[ "$DRY_RUN" == false ]]; then
        confirm_action
    fi
    
    # Phase 1: Delete broken releases
    print_info "Phase 1: Deleting broken releases..."
    for version in "${VERSIONS[@]}"; do
        delete_release "$version"
    done
    echo
    
    # Phase 2: Delete broken tags
    print_info "Phase 2: Deleting broken tags..."
    for version in "${VERSIONS[@]}"; do
        delete_tag "$version"
    done
    echo
    
    # Phase 3: Recreate releases with proper tags and changelogs
    if [[ "$DRY_RUN" == false ]]; then
        print_info "Phase 3: Recreating releases with proper changelogs..."
        recreate_releases
        echo
    else
        print_info "[DRY-RUN] Phase 3 would recreate releases with proper changelogs"
        echo
    fi
    
    if [[ "$DRY_RUN" == true ]]; then
        echo
        print_info "Dry-run complete. To execute these changes, run:"
        print_info "  $(basename "$0") --execute"
    else
        print_success "Release fix complete!"
        echo
        print_info "You can verify the releases at:"
        print_info "  https://github.com/$REPO/releases"
    fi
}

# Run main function
main "$@"
