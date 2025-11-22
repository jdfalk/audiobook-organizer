<!-- file: tests/e2e/README.md -->
<!-- version: 1.2.0 -->
<!-- guid: 5d6e7f89-0123-4567-89ab-cdef01234567 -->

# End-to-End Tests for Audiobook Organizer

Selenium-based end-to-end tests for complete UI workflows.

## Setup

```bash
# Install Python dependencies
pip install -r requirements.txt

# Ensure Chrome/Chromium is installed
# ChromeDriver will be auto-installed via webdriver-manager
```

## Running Tests

```bash
# Run all tests
pytest tests/e2e/ -v

# Run specific test file
pytest tests/e2e/test_settings_workflow.py -v

# Run with HTML report
pytest tests/e2e/ --html=test_results/e2e_report.html --self-contained-html

# Run in non-headless mode (see browser)
# Edit conftest.py and remove "--headless" from chrome_options
```

## Test Coverage

- **test_settings_workflow.py**: Settings page interactions
  - Navigation to Settings
  - Browse server filesystem dialog
  - Tab switching
  - Save button visibility

- **test_dashboard_workflow.py**: Dashboard page verification
  - Dashboard loads
  - Import folders count display
  - Library statistics display

- **test_organize_workflow.py**: Organize files operation
  - Organize button exists
  - Shows real book count (not 0/0)

## Configuration

Set environment variable `TEST_BASE_URL` to test against different instances:

```bash
export TEST_BASE_URL=http://localhost:8080
pytest tests/e2e/ -v
```

Bundled sample audiobook fixtures live under `testdata/audio/librivox/` (first six
tracks from several Librivox releases, checked in via Git LFS). Point your test
server at these fixtures for deterministic metadata/organize flows.

## Dockerized Test Image

Build the standardized test image (contains Go 1.23, Node.js 22, Chromium, and
Python/Selenium tooling):

```bash
docker build -f Dockerfile.test -t audiobook-organizer-test .
```

Then run any combination of test suites inside the container:

```bash
# Go + frontend unit tests
docker run --rm audiobook-organizer-test bash -lc "go test ./... && npm test --prefix web"

# E2E tests (requires the server running on the host)
docker run --rm --network host \
  -e TEST_BASE_URL=http://host.docker.internal:8080 \
  audiobook-organizer-test \
  bash -lc "xvfb-run -a pytest tests/e2e -v"
```

`xvfb-run` ensures Chromium has a display when the container lacks a GUI. You
can adjust `TEST_BASE_URL` or add extra pytest flags as needed.

## Debugging

- Remove `--headless` from chrome_options in conftest.py to see browser
- Add time.sleep() calls to pause and inspect UI state
- Check screenshots saved on failure (if configured)
- Review HTML test reports in test_results/
