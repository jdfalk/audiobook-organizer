#!/bin/bash
# file: scripts/run-all-tests.sh
# version: 1.0.0
# guid: f1e2d3c4-b5a6-7980-1234-567890abcdef
# description: Run all tests (Go backend + Frontend E2E + Frontend unit) and generate reports

set -e

echo "🧪 Running Comprehensive Test Suite"
echo "===================================="
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track results
GO_TESTS_PASSED=false
FRONTEND_UNIT_PASSED=false
E2E_TESTS_PASSED=false

# Create reports directory
mkdir -p test-reports

echo "📊 Step 1: Running Go Backend Tests"
echo "------------------------------------"
if go test -v -coverprofile=test-reports/go-coverage.out ./... 2>&1 | tee test-reports/go-tests.log; then
    echo -e "${GREEN}✅ Go tests passed${NC}"
    GO_TESTS_PASSED=true

    # Generate HTML coverage report
    go tool cover -html=test-reports/go-coverage.out -o test-reports/go-coverage.html
    echo "📈 Coverage report: test-reports/go-coverage.html"
else
    echo -e "${RED}❌ Go tests failed${NC}"
fi
echo ""

echo "🎨 Step 2: Running Frontend Unit Tests"
echo "---------------------------------------"
cd web
if npm test -- --coverage --run 2>&1 | tee ../test-reports/frontend-unit.log; then
    echo -e "${GREEN}✅ Frontend unit tests passed${NC}"
    FRONTEND_UNIT_PASSED=true
else
    echo -e "${RED}❌ Frontend unit tests failed${NC}"
fi
cd ..
echo ""

echo "🌐 Step 3: Running E2E Tests (Playwright)"
echo "------------------------------------------"
cd web
if npm run test:e2e 2>&1 | tee ../test-reports/e2e-tests.log; then
    echo -e "${GREEN}✅ E2E tests passed${NC}"
    E2E_TESTS_PASSED=true
else
    echo -e "${RED}❌ E2E tests failed (check video recordings in web/test-results/)${NC}"
fi

# Generate HTML report
if command -v npx &> /dev/null; then
    npx playwright show-report --host 127.0.0.1 --port 9323 &
    echo "📊 E2E Report available at: http://127.0.0.1:9323"
fi
cd ..
echo ""

echo "📋 Summary Report"
echo "================="
echo ""

if [ "$GO_TESTS_PASSED" = true ]; then
    echo -e "${GREEN}✅ Go Backend Tests: PASSED${NC}"
else
    echo -e "${RED}❌ Go Backend Tests: FAILED${NC}"
fi

if [ "$FRONTEND_UNIT_PASSED" = true ]; then
    echo -e "${GREEN}✅ Frontend Unit Tests: PASSED${NC}"
else
    echo -e "${RED}❌ Frontend Unit Tests: FAILED${NC}"
fi

if [ "$E2E_TESTS_PASSED" = true ]; then
    echo -e "${GREEN}✅ E2E Tests: PASSED${NC}"
else
    echo -e "${RED}❌ E2E Tests: FAILED${NC}"
fi

echo ""
echo "📁 Test Artifacts:"
echo "  - Go Coverage: test-reports/go-coverage.html"
echo "  - Frontend Coverage: web/coverage/"
echo "  - E2E Videos/Screenshots: web/test-results/"
echo "  - E2E Report: http://127.0.0.1:9323 (if server running)"
echo ""

# Exit with error if any tests failed
if [ "$GO_TESTS_PASSED" = true ] && [ "$FRONTEND_UNIT_PASSED" = true ] && [ "$E2E_TESTS_PASSED" = true ]; then
    echo -e "${GREEN}🎉 All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}❌ Some tests failed. Review logs and artifacts above.${NC}"
    exit 1
fi
