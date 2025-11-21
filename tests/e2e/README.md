<!-- file: tests/e2e/README.md -->
<!-- version: 1.1.0 -->
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

## Debugging

- Remove `--headless` from chrome_options in conftest.py to see browser
- Add time.sleep() calls to pause and inspect UI state
- Check screenshots saved on failure (if configured)
- Review HTML test reports in test_results/
