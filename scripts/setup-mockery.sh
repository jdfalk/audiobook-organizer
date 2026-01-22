#!/bin/bash
# file: scripts/setup-mockery.sh
# version: 1.2.0
# guid: c3d4e5f6-a7b8-9012-cdef-345678901abc

# Setup script for integrating mockery v3 into the project

set -euo pipefail

echo "🔧 Setting up mockery for improved test coverage..."

# Check if mockery is installed
if ! command -v mockery &> /dev/null; then
    echo "📦 Installing mockery..."
    go install github.com/vektra/mockery/v2@latest
fi

echo "✅ Mockery is installed"
mockery --version

# Generate mocks using configuration
echo "🔨 Generating mocks for Store interface..."
mockery --config .mockery.yaml

echo ""
echo "✨ Setup complete!"
echo ""
echo "Generated mocks:"
ls -la internal/database/mocks/*.go 2>/dev/null || echo "  (check internal/database/mocks/)"
echo ""
echo "Next steps:"
echo "1. Review the generated mock in internal/database/mocks/"
echo "2. Update server tests to use the mock (see server_mockery_example_test.go.example)"
echo "3. Run: go test ./internal/server -v"
echo "4. Add 'make mocks' target to Makefile for CI/CD"
echo ""
echo "Expected coverage improvement: 66% → 85%+"
