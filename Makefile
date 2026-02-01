# file: Makefile
# version: 2.0.0
# guid: c1d2e3f4-g5h6-7890-ijkl-m1234567890n

BINARY := audiobook-organizer
WEB_DIR := web

.PHONY: all build build-api run run-api install clean help \
        web-install web-build web-dev web-test web-lint \
        test test-all test-e2e coverage coverage-check ci

# Default: full build (frontend + backend with embed)
all: build

## help: Show available targets
help:
	@echo "Build:"
	@echo "  make build          - Full build: frontend + Go binary with embedded UI"
	@echo "  make build-api      - Backend only (no embedded frontend, for quick iteration)"
	@echo "  make run            - Full build then serve"
	@echo "  make run-api        - Backend-only build then serve (API endpoints only)"
	@echo ""
	@echo "Frontend:"
	@echo "  make web-install    - Install npm dependencies"
	@echo "  make web-build      - Build frontend (outputs to web/dist)"
	@echo "  make web-dev        - Start Vite dev server"
	@echo "  make web-test       - Run frontend unit tests"
	@echo "  make web-lint       - Lint frontend code"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run Go backend tests"
	@echo "  make test-all       - Run all tests (backend + frontend)"
	@echo "  make test-e2e       - Run Playwright E2E tests"
	@echo "  make coverage       - Generate coverage report"
	@echo "  make coverage-check - Verify 80% coverage threshold"
	@echo "  make ci             - Full CI: all tests + coverage check"
	@echo ""
	@echo "Setup:"
	@echo "  make install        - Install dependencies (npm)"
	@echo "  make clean          - Remove build artifacts"

## install: Install all dependencies
install: web-install

# --- Build targets ---
# The binary embeds the React frontend via //go:embed web/dist (build tag:
# embed_frontend). This requires web/dist to exist, so web-build runs first.
# Use build-api for quick backend iteration when you don't need the UI.

## build: Full build with embedded frontend
build: web-build
	@echo "🔨 Building $(BINARY) with embedded frontend..."
	@go build -tags embed_frontend -o $(BINARY) .
	@echo "✅ Built ./$(BINARY)"

## build-api: Backend-only build (no frontend, serves placeholder at /)
build-api:
	@echo "🔨 Building $(BINARY) (API only)..."
	@go build -o $(BINARY) .
	@echo "✅ Built ./$(BINARY)"

## run: Full build and serve
run: build
	@./$(BINARY) serve

## run-api: API-only build and serve
run-api: build-api
	@./$(BINARY) serve

# --- Frontend targets ---

## web-install: Install npm dependencies
web-install:
	@echo "📦 Installing frontend dependencies..."
	@cd $(WEB_DIR) && npm install
	@echo "✅ Dependencies installed"

## web-build: Build frontend (produces web/dist for embedding)
web-build: web-install
	@echo "🌐 Building frontend..."
	@cd $(WEB_DIR) && npm run build
	@echo "✅ Frontend built (web/dist)"

## web-dev: Start Vite dev server
web-dev:
	@cd $(WEB_DIR) && npm run dev

## web-test: Run frontend unit tests
web-test:
	@echo "🧪 Running frontend tests..."
	@cd $(WEB_DIR) && npm run test
	@echo "✅ Frontend tests passed"

## web-lint: Lint frontend code
web-lint:
	@echo "🔍 Linting frontend..."
	@cd $(WEB_DIR) && npm run lint
	@echo "✅ Frontend lint passed"

# --- Testing targets ---

## test: Run Go backend tests
test:
	@echo "🧪 Running backend tests..."
	@go test ./... -v -race
	@echo "✅ Backend tests passed"

## test-all: Run all tests (backend + frontend)
test-all: test web-test

## test-e2e: Run Playwright E2E tests
test-e2e:
	@echo "🧪 Running E2E tests..."
	@cd $(WEB_DIR) && npm run test:e2e
	@echo "✅ E2E tests passed"

## coverage: Generate coverage report
coverage:
	@echo "📊 Generating coverage report..."
	@go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -html=coverage.out -o coverage.html
	@echo ""
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out | grep total | awk '{printf "  Total: %s\n", $$3}'
	@echo ""
	@echo "📄 Detailed report: coverage.html"

## coverage-check: Verify coverage meets 80% threshold
coverage-check:
	@echo "🎯 Checking coverage threshold..."
	@go test ./... -coverprofile=coverage.out -covermode=atomic >/dev/null 2>&1
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $$coverage%"; \
	if [ $$(echo "$$coverage < 80" | bc -l) -eq 1 ]; then \
		echo "❌ Coverage $$coverage% is below 80% threshold"; \
		exit 1; \
	fi; \
	echo "✅ Coverage $$coverage% meets 80% threshold"

## ci: Full CI check (all tests + coverage)
ci: test-all coverage-check
	@echo "✅ All CI checks passed!"

## clean: Remove build artifacts
clean:
	@echo "🧹 Cleaning..."
	@rm -f $(BINARY) coverage.out coverage.html
	@echo "✅ Clean complete"

# Quick aliases
.PHONY: t c b
t: test
c: coverage
b: build
