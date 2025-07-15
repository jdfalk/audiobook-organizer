#!/bin/bash
# file: copilot/validate-setup.sh
# version: 1.0.0
# guid: 9e2f5a8b-3c6d-4e9f-5a2b-8e3f5a9e2f5a

# Repository Validation Script
# This script validates that a repository is properly configured for the reusable workflows

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0
WARNINGS=0

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED++))
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    ((WARNINGS++))
}

# Check if we're in a git repository
check_git_repo() {
    log_info "Checking git repository..."
    if git rev-parse --git-dir > /dev/null 2>&1; then
        log_pass "Git repository detected"
    else
        log_fail "Not in a git repository"
        return 1
    fi
}

# Check for workflow files
check_workflows() {
    log_info "Checking workflow files..."

    if [ -d ".github/workflows" ]; then
        log_pass ".github/workflows directory exists"

        # Check for workflow files
        local workflow_count=$(find .github/workflows -name "*.yml" -o -name "*.yaml" | wc -l)
        if [ "$workflow_count" -gt 0 ]; then
            log_pass "Found $workflow_count workflow file(s)"

            # List workflow files
            find .github/workflows -name "*.yml" -o -name "*.yaml" | while read -r file; do
                echo "  - $file"
            done
        else
            log_fail "No workflow files found in .github/workflows"
        fi
    else
        log_fail ".github/workflows directory does not exist"
    fi
}

# Check for required files
check_required_files() {
    log_info "Checking required files..."

    # Check for version files
    local version_files=("package.json" "version.txt" "__init__.py" "Cargo.toml" "go.mod" "setup.py")
    local found_version_file=false

    for file in "${version_files[@]}"; do
        if [ -f "$file" ]; then
            log_pass "Version file found: $file"
            found_version_file=true
        fi
    done

    if [ "$found_version_file" = false ]; then
        log_warning "No version files found. Consider adding package.json or version.txt"
    fi

    # Check for Dockerfile if workflows suggest container builds
    if grep -q "buildah-multiarch\|container" .github/workflows/*.yml 2>/dev/null || \
       grep -q "buildah-multiarch\|container" .github/workflows/*.yaml 2>/dev/null; then
        if [ -f "Dockerfile" ]; then
            log_pass "Dockerfile found for container builds"
        else
            log_fail "Dockerfile not found but container workflow detected"
        fi
    fi

    # Check for README
    if [ -f "README.md" ] || [ -f "README.rst" ] || [ -f "README.txt" ]; then
        log_pass "README file found"
    else
        log_warning "No README file found"
    fi

    # Check for LICENSE
    if [ -f "LICENSE" ] || [ -f "LICENSE.md" ] || [ -f "LICENSE.txt" ]; then
        log_pass "LICENSE file found"
    else
        log_warning "No LICENSE file found"
    fi
}

# Check git configuration
check_git_config() {
    log_info "Checking git configuration..."

    # Check if git user is configured
    if git config user.name > /dev/null 2>&1 && git config user.email > /dev/null 2>&1; then
        log_pass "Git user configuration found"
    else
        log_warning "Git user not configured. Run: git config --global user.name 'Your Name' && git config --global user.email 'your.email@example.com'"
    fi

    # Check for remote origin
    if git remote get-url origin > /dev/null 2>&1; then
        local remote_url=$(git remote get-url origin)
        log_pass "Git remote origin configured: $remote_url"

        # Check if it's a GitHub repository
        if [[ "$remote_url" == *"github.com"* ]]; then
            log_pass "GitHub repository detected"
        else
            log_warning "Not a GitHub repository. Some features may not work."
        fi
    else
        log_fail "No git remote origin configured"
    fi
}

# Check conventional commits
check_conventional_commits() {
    log_info "Checking recent commits for conventional commit format..."

    # Get last 10 commit messages
    local commit_messages=$(git log --oneline -10 --pretty=format:"%s" 2>/dev/null || echo "")

    if [ -z "$commit_messages" ]; then
        log_warning "No commit history found"
        return
    fi

    local conventional_count=0
    local total_count=0

    while IFS= read -r message; do
        ((total_count++))
        if echo "$message" | grep -E "^(feat|fix|docs|style|refactor|perf|test|chore|build|ci|revert)(\(.+\))?\!?:" > /dev/null; then
            ((conventional_count++))
        fi
    done <<< "$commit_messages"

    if [ "$conventional_count" -gt 0 ]; then
        local percentage=$((conventional_count * 100 / total_count))
        if [ "$percentage" -ge 80 ]; then
            log_pass "$conventional_count/$total_count commits follow conventional format ($percentage%)"
        else
            log_warning "$conventional_count/$total_count commits follow conventional format ($percentage%). Consider using conventional commits for better versioning."
        fi
    else
        log_warning "No conventional commits found. Consider using conventional commit format for automatic versioning."
    fi
}

# Check workflow syntax
check_workflow_syntax() {
    log_info "Checking workflow syntax..."

    # Check if GitHub CLI is available
    if command -v gh > /dev/null 2>&1; then
        # Find workflow files
        find .github/workflows -name "*.yml" -o -name "*.yaml" | while read -r file; do
            if gh workflow view "$(basename "$file" .yml)" > /dev/null 2>&1 || \
               gh workflow view "$(basename "$file" .yaml)" > /dev/null 2>&1; then
                log_pass "Workflow syntax valid: $file"
            else
                # Try basic YAML validation
                if python3 -c "import yaml; yaml.safe_load(open('$file'))" 2>/dev/null; then
                    log_pass "YAML syntax valid: $file"
                else
                    log_fail "Invalid YAML syntax: $file"
                fi
            fi
        done
    else
        log_warning "GitHub CLI not available. Install 'gh' for workflow validation."

        # Try basic YAML validation with Python
        if command -v python3 > /dev/null 2>&1; then
            find .github/workflows -name "*.yml" -o -name "*.yaml" | while read -r file; do
                if python3 -c "import yaml; yaml.safe_load(open('$file'))" 2>/dev/null; then
                    log_pass "YAML syntax valid: $file"
                else
                    log_fail "Invalid YAML syntax: $file"
                fi
            done
        else
            log_warning "Python3 not available. Cannot validate YAML syntax."
        fi
    fi
}

# Check for security best practices
check_security() {
    log_info "Checking security best practices..."

    # Check for secrets in code
    local secret_patterns=("password" "secret" "key" "token" "api_key")
    local issues_found=false

    for pattern in "${secret_patterns[@]}"; do
        if git log --all -p | grep -i "$pattern" | grep -E "(=|:)" > /dev/null 2>&1; then
            if [ "$issues_found" = false ]; then
                log_warning "Potential secrets found in git history. Review carefully:"
                issues_found=true
            fi
        fi
    done

    if [ "$issues_found" = false ]; then
        log_pass "No obvious secrets found in git history"
    fi

    # Check .gitignore for common secret files
    if [ -f ".gitignore" ]; then
        local gitignore_patterns=(".env" "*.key" "*.pem" "config.json")
        local protected_count=0

        for pattern in "${gitignore_patterns[@]}"; do
            if grep -q "$pattern" .gitignore; then
                ((protected_count++))
            fi
        done

        if [ "$protected_count" -gt 2 ]; then
            log_pass "Common secret file patterns found in .gitignore"
        else
            log_warning "Consider adding common secret file patterns to .gitignore (.env, *.key, *.pem, etc.)"
        fi
    else
        log_warning ".gitignore file not found"
    fi
}

# Check dependencies and tools
check_dependencies() {
    log_info "Checking available tools..."

    local tools=("git" "curl" "jq")
    local optional_tools=("gh" "docker" "npm" "python3" "go")

    # Check required tools
    for tool in "${tools[@]}"; do
        if command -v "$tool" > /dev/null 2>&1; then
            log_pass "$tool is available"
        else
            log_fail "$tool is not available (required)"
        fi
    done

    # Check optional tools
    for tool in "${optional_tools[@]}"; do
        if command -v "$tool" > /dev/null 2>&1; then
            log_pass "$tool is available"
        else
            log_info "$tool is not available (optional)"
        fi
    done
}

# Generate summary report
generate_summary() {
    echo
    echo "=================================="
    echo "         VALIDATION SUMMARY"
    echo "=================================="
    echo -e "${GREEN}Passed: $PASSED${NC}"
    echo -e "${YELLOW}Warnings: $WARNINGS${NC}"
    echo -e "${RED}Failed: $FAILED${NC}"
    echo "=================================="
    echo

    if [ "$FAILED" -eq 0 ] && [ "$WARNINGS" -eq 0 ]; then
        echo -e "${GREEN}✅ Repository is ready for reusable workflows!${NC}"
        return 0
    elif [ "$FAILED" -eq 0 ]; then
        echo -e "${YELLOW}⚠️  Repository is mostly ready with some recommendations.${NC}"
        return 0
    else
        echo -e "${RED}❌ Repository needs attention before using reusable workflows.${NC}"
        return 1
    fi
}

# Main function
main() {
    echo "🔍 Validating repository setup for reusable workflows..."
    echo

    # Run all checks
    check_git_repo
    check_workflows
    check_required_files
    check_git_config
    check_conventional_commits
    check_workflow_syntax
    check_security
    check_dependencies

    # Generate summary
    generate_summary
}

# Run main function
main "$@"
