#!/bin/bash
# file: scripts/install-pre-commit-hooks.sh

# Script to install comprehensive pre-commit hooks for automatic code quality checks
# This prevents broken code from being committed by running all CI checks locally

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}Installing comprehensive pre-commit hooks for subtitle-manager...${NC}"

# Check if we're in a git repository
if [ ! -d ".git" ]; then
    echo -e "${RED}Error: This script must be run from the root of the git repository${NC}"
    exit 1
fi

# Create the pre-commit hook
HOOK_FILE=".git/hooks/pre-commit"

cat > "$HOOK_FILE" << 'EOF'
#!/bin/bash
# file: .git/hooks/pre-commit
# Comprehensive pre-commit hook for code quality and consistency

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check and format Go code
check_go() {
    echo -e "${BLUE}🔍 Checking Go code...${NC}"

    # Find all staged Go files
    GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)

    # If no staged Go files found, check if there are any Go files to check
    if [ -z "$GO_FILES" ]; then
        GO_FILES=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | head -5)
        if [ -z "$GO_FILES" ]; then
            echo "No Go files found to check"
            return 0
        fi
    fi

    if [ -n "$GO_FILES" ]; then
        echo "Checking Go files: $GO_FILES"

        # 1. Check Go formatting
        echo -e "${YELLOW}📝 Checking Go formatting...${NC}"
        UNFORMATTED=$(echo "$GO_FILES" | xargs gofmt -s -l 2>/dev/null || true)
        if [ -n "$UNFORMATTED" ]; then
            echo -e "${YELLOW}⚠️  Auto-formatting Go files...${NC}"
            echo "$UNFORMATTED" | while IFS= read -r file; do
                if [ -f "$file" ]; then
                    gofmt -s -w "$file"
                    git add "$file"
                    echo "  ✅ Formatted: $file"
                fi
            done
        fi

        # 2. Run goimports if available
        if command_exists goimports; then
            echo -e "${YELLOW}📦 Fixing Go imports...${NC}"
            echo "$GO_FILES" | while IFS= read -r file; do
                if [ -f "$file" ]; then
                    goimports -w "$file" 2>/dev/null || echo "Warning: goimports failed for $file"
                    git add "$file"
                fi
            done
        else
            echo -e "${YELLOW}Installing goimports...${NC}"
            go install golang.org/x/tools/cmd/goimports@latest
            if command_exists goimports; then
                echo "$GO_FILES" | while IFS= read -r file; do
                    if [ -f "$file" ]; then
                        goimports -w "$file" 2>/dev/null || echo "Warning: goimports failed for $file"
                        git add "$file"
                    fi
                done
            fi
        fi

        # 3. Run go vet
        echo -e "${YELLOW}🔬 Running go vet...${NC}"
        if ! go vet ./...; then
            echo -e "${RED}❌ go vet found issues. Please fix them before committing.${NC}"
            return 1
        fi

        # 4. Run go mod tidy
        echo -e "${YELLOW}🧹 Running go mod tidy...${NC}"
        go mod tidy
        if ! git diff --quiet go.mod go.sum; then
            echo -e "${YELLOW}⚠️  go.mod or go.sum changed, adding to commit...${NC}"
            git add go.mod go.sum
        fi

        echo -e "${GREEN}✅ Go checks passed!${NC}"
    fi

    return 0
}

# Function to check and format frontend code
check_frontend() {
    echo -e "${BLUE}🎨 Checking frontend code...${NC}"

    # Check if we have frontend files to check
    FRONTEND_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep -E 'webui/.*\.(js|jsx|ts|tsx|css|json)$' || true)

    if [ -z "$FRONTEND_FILES" ] || [ ! -f "webui/package.json" ]; then
        echo "No frontend files to check or no webui directory found"
        return 0
    fi

    echo "Checking frontend files: $FRONTEND_FILES"

    cd webui

    # 1. Check if dependencies are installed
    if [ ! -d "node_modules" ]; then
        echo -e "${YELLOW}📦 Installing frontend dependencies...${NC}"
        npm install
    fi

    # 2. Run Prettier formatting
    echo -e "${YELLOW}💅 Running Prettier...${NC}"
    if npm list prettier > /dev/null 2>&1; then
        FRONTEND_FILES_RELATIVE=$(echo "$FRONTEND_FILES" | sed 's|webui/||g')
        echo "$FRONTEND_FILES_RELATIVE" | xargs npx prettier --write || {
            echo -e "${RED}❌ Prettier failed. Please fix formatting issues.${NC}"
            cd ..
            return 1
        }

        # Re-stage the formatted files
        cd ..
        git add $FRONTEND_FILES
        echo -e "${GREEN}✅ Frontend files formatted${NC}"
        cd webui
    else
        echo -e "${YELLOW}⚠️  Prettier not found, skipping formatting${NC}"
    fi

    # 3. Run ESLint
    echo -e "${YELLOW}🔍 Running ESLint...${NC}"
    if npm list eslint > /dev/null 2>&1; then
        if ! npm run lint; then
            echo -e "${RED}❌ ESLint found issues. Please fix them before committing.${NC}"
            echo -e "${YELLOW}💡 Try running: cd webui && npm run lint --fix${NC}"
            cd ..
            return 1
        fi
        echo -e "${GREEN}✅ ESLint checks passed!${NC}"
    else
        echo -e "${YELLOW}⚠️  ESLint not found, skipping linting${NC}"
    fi

    # 4. Check TypeScript (if tsconfig.json exists)
    if [ -f "tsconfig.json" ] && command_exists npx; then
        echo -e "${YELLOW}📘 Checking TypeScript...${NC}"
        if ! npx tsc --noEmit; then
            echo -e "${RED}❌ TypeScript check failed. Please fix type errors.${NC}"
            cd ..
            return 1
        fi
        echo -e "${GREEN}✅ TypeScript checks passed!${NC}"
    fi

    cd ..
    return 0
}

# Function to run quick tests on staged files
check_tests() {
    echo -e "${BLUE}🧪 Running quick tests...${NC}"

    # Check if there are any test files staged
    TEST_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep -E '(_test\.go|\.test\.|\.spec\.)' || true)

    if [ -n "$TEST_FILES" ]; then
        echo "Found test files in staging, running affected tests..."

        # Run Go tests for packages with staged test files
        GO_TEST_PACKAGES=$(echo "$TEST_FILES" | grep '_test\.go$' | xargs -I {} dirname {} | sort -u | xargs -I {} echo "./{}..." || true)
        if [ -n "$GO_TEST_PACKAGES" ]; then
            echo -e "${YELLOW}🧪 Running Go tests...${NC}"
            if ! go test $GO_TEST_PACKAGES; then
                echo -e "${RED}❌ Go tests failed. Please fix them before committing.${NC}"
                return 1
            fi
            echo -e "${GREEN}✅ Go tests passed!${NC}"
        fi

        # Run frontend tests if staging frontend test files
        FRONTEND_TEST_FILES=$(echo "$TEST_FILES" | grep -E 'webui/.*\.(test|spec)\.' || true)
        if [ -n "$FRONTEND_TEST_FILES" ] && [ -f "webui/package.json" ]; then
            echo -e "${YELLOW}🧪 Running frontend tests...${NC}"
            cd webui
            if ! npm test -- --passWithNoTests --watchAll=false; then
                echo -e "${RED}❌ Frontend tests failed. Please fix them before committing.${NC}"
                cd ..
                return 1
            fi
            echo -e "${GREEN}✅ Frontend tests passed!${NC}"
            cd ..
        fi
    else
        echo "No test files staged, skipping test run"
    fi

    return 0
}

# Main execution
echo -e "${GREEN}🚀 Running pre-commit checks...${NC}"

FAILED=0

# Check Go code
if command_exists go; then
    if ! check_go; then
        FAILED=1
    fi
else
    echo -e "${YELLOW}⚠️  Go not found, skipping Go checks${NC}"
fi

# Check frontend code
if command_exists npm; then
    if ! check_frontend; then
        FAILED=1
    fi
else
    echo -e "${YELLOW}⚠️  Node.js/npm not found, skipping frontend checks${NC}"
fi

# Run tests (optional, can be disabled for faster commits)
# Uncomment the following lines to enable test running on commit
# if ! check_tests; then
#     FAILED=1
# fi

if [ $FAILED -eq 1 ]; then
    echo -e "${RED}❌ Pre-commit checks failed! Please fix the issues above.${NC}"
    echo -e "${YELLOW}💡 Tip: You can bypass this hook with 'git commit --no-verify' but it's not recommended.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ All pre-commit checks passed! Ready to commit! 🎉${NC}"
exit 0
EOF

# Make the hook executable
chmod +x "$HOOK_FILE"

echo -e "${GREEN}✅ Comprehensive pre-commit hook installed successfully!${NC}"
echo ""
echo -e "${YELLOW}The hook will now run the following checks before each commit:${NC}"
echo -e "  ${BLUE}Go:${NC}"
echo "    • Auto-format code with gofmt"
echo "    • Fix imports with goimports"
echo "    • Run go vet for code analysis"
echo "    • Run go mod tidy"
echo -e "  ${BLUE}Frontend:${NC}"
echo "    • Auto-format with Prettier"
echo "    • Run ESLint for code quality"
echo "    • Check TypeScript compilation"
echo -e "  ${BLUE}Tests:${NC}"
echo "    • Run tests for staged files (optional - currently disabled)"
echo ""
echo -e "${YELLOW}To disable the hook temporarily, use:${NC}"
echo "  git commit --no-verify"
echo ""
echo -e "${YELLOW}To uninstall the hook, delete:${NC}"
echo "  .git/hooks/pre-commit"
echo ""
echo -e "${YELLOW}To enable test running on commit, edit the hook and uncomment the test section.${NC}"
echo ""
echo -e "${GREEN}This should prevent most CI failures by catching issues early! 🚀${NC}"
