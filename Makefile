# file: Makefile
# version: 1.0.0
# guid: c1d2e3f4-g5h6-7890-ijkl-m1234567890n

.PHONY: all test coverage coverage-check ci clean help

# Default target
all: test

## help: Show this help message
help:
	@echo "Available targets:"
	@echo "  make test           - Run all tests with mocks"
	@echo "  make coverage       - Generate HTML coverage report"
	@echo "  make coverage-check - Check coverage meets 80% threshold"
	@echo "  make ci             - Run all CI checks (test + coverage-check)"
	@echo "  make clean          - Remove generated files"
	@echo "  make help           - Show this help message"

## test: Run all tests
test:
	@echo "🧪 Running tests..."
	@go test ./... -v -race
	@echo "✅ All tests passed!"

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

## ci: Run all CI checks
ci: test coverage-check
	@echo "✅ All CI checks passed!"

## clean: Remove generated files
clean:
	@echo "🧹 Cleaning up..."
	@rm -f coverage.out coverage.html
	@echo "✅ Clean complete!"

# Quick aliases
.PHONY: t c
t: test
c: coverage
