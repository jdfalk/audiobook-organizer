<!-- file: docs/TASK-7-CORE-TESTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3d7e9f2c-8a4b-4c5d-9f7e-1a8b2c3d4e5f -->

# Task 7: Core E2E Test Suite Validation

This file defines the core flow to build, run, and validate the E2E test suite.

## Phase 0: Infrastructure Check (Read-Only)

```bash
echo "=== Checking E2E Infrastructure ==="

# Check for test directory
ls -la tests/e2e/

# Check for Dockerfile
cat tests/Dockerfile.test

# Check for docker-compose
cat docker-compose.test.yml

# Check for pytest config
cat tests/pytest.ini tests/conftest.py 2>/dev/null
```

Expected:

- `tests/e2e/` contains test files: `test_dashboard.py`, `test_library.py`, etc.
- `Dockerfile.test` includes Selenium, pytest, browser drivers
- `docker-compose.test.yml` defines test service with dependencies

## Phase 1: Docker Image Build

```bash
echo "=== Building Test Image ==="

# Build from project root
docker build -f tests/Dockerfile.test -t audiobook-organizer-test:latest .

# Verify image
docker images | grep audiobook-organizer-test
```

Pass criteria:

- Image builds without errors
- Size reasonable (<2GB)
- Contains Python 3.x, pytest, selenium, browser

## Phase 2: Test Execution (Local)

```bash
echo "=== Running E2E Tests ==="

# Run via docker-compose
docker-compose -f docker-compose.test.yml up --abort-on-container-exit

# Or run directly
docker run --rm \
  --network host \
  -v $(pwd)/tests:/tests \
  -v $(pwd)/test-results:/test-results \
  audiobook-organizer-test:latest \
  pytest -v tests/e2e/

# Check exit code
echo "Exit code: $?"
```

Pass criteria:

- All tests pass (exit code 0)
- No crashes or hangs
- Test output shows clear pass/fail status

## Phase 3: Test Coverage Verification

```bash
echo "=== Checking Test Coverage ==="

# List test files
ls -1 tests/e2e/test_*.py

# Expected test files:
# - test_dashboard.py (dashboard load, stats)
# - test_library.py (library view, book list)
# - test_scan.py (trigger scan, progress)
# - test_import.py (import file, metadata)
# - test_organize.py (organize operation)
# - test_metadata.py (edit metadata)
# - test_delete.py (delete with prevention)
# - test_settings.py (configure settings)
```

## Phase 4: Individual Test Scenarios

### Test 1: Dashboard Load

```bash
echo "=== Test: Dashboard Loads ==="

docker run --rm \
  --network host \
  -v $(pwd)/tests:/tests \
  audiobook-organizer-test:latest \
  pytest -v tests/e2e/test_dashboard.py::test_dashboard_loads
```

Expected:

- Dashboard page loads without errors
- Key elements visible: book count, folder count, stats cards

### Test 2: Library Scan

```bash
echo "=== Test: Library Scan ==="

docker run --rm \
  --network host \
  -v $(pwd)/tests:/tests \
  audiobook-organizer-test:latest \
  pytest -v tests/e2e/test_scan.py::test_trigger_scan
```

Expected:

- Scan button works
- Progress shows file count
- Scan completes successfully

### Test 3: Import File

```bash
echo "=== Test: Import File ==="

docker run --rm \
  --network host \
  -v $(pwd)/tests:/tests \
  -v /tmp/test-audiobooks:/test-data \
  audiobook-organizer-test:latest \
  pytest -v tests/e2e/test_import.py::test_import_single_file
```

Expected:

- Import form accepts file path
- File imported with metadata
- Book appears in library

### Test 4: Delete with Prevention

```bash
echo "=== Test: Delete with Reimport Prevention ==="

docker run --rm \
  --network host \
  -v $(pwd)/tests:/tests \
  audiobook-organizer-test:latest \
  pytest -v tests/e2e/test_delete.py::test_delete_with_prevention
```

Expected:

- Delete dialog shows checkbox
- Confirmation shows hashes
- Book soft-deleted, hashes blocked

## Phase 5: VS Code Task Creation

```bash
echo "=== Creating VS Code Task ==="

# Task definition (to be added to .vscode/tasks.json)
cat << 'EOF'
{
  "label": "Run E2E Tests in Docker",
  "type": "shell",
  "command": "docker-compose",
  "args": ["-f", "docker-compose.test.yml", "up", "--abort-on-container-exit"],
  "group": "test",
  "presentation": {
    "reveal": "always",
    "panel": "dedicated"
  }
}
EOF
```

## Phase 6: Failure Artifacts

```bash
echo "=== Checking Failure Artifacts ==="

# On test failure, check for screenshots
ls -la test-results/screenshots/

# Check for test logs
ls -la test-results/logs/

# View last failure
cat test-results/logs/last_failure.log
```

## Phase 7: CI Integration (GitHub Actions)

```yaml
# .github/workflows/e2e-tests.yml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build test image
        run: docker build -f tests/Dockerfile.test -t audiobook-organizer-test .
      - name: Run E2E tests
        run: docker-compose -f docker-compose.test.yml up --abort-on-container-exit
      - name: Upload artifacts on failure
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: test-results
          path: test-results/
```

If tests fail, switch to `TASK-7-TROUBLESHOOTING.md`.
