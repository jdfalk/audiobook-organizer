#!/bin/bash
# file: .github/scripts/ci-status.sh
# version: 1.0.0
# guid: 9e1a2b3c-4d5e-6f7a-8b9c-0d1e2f3a4b5c

# Standard output/status functions for CI workflows
print_status() {
  echo "::notice ::[STATUS] $1"
}
print_error() {
  echo "::error ::[ERROR] $1" >&2
}
print_success() {
  echo "::notice ::[SUCCESS] $1"
}
print_summary() {
  echo "::notice ::[SUMMARY] $1"
}

# Usage: print_status "Starting job..."
# Usage: print_error "Failed to process file"
# Usage: print_success "Job completed successfully"
# Usage: print_summary "Processed 5 files, 2 errors"
