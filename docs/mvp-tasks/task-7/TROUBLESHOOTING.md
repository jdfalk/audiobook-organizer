<!-- file: docs/TASK-7-TROUBLESHOOTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5f9e2d4c-8b6a-4c7d-9f8e-1a9b2c3d4e5f -->

# Task 7: Troubleshooting - E2E Test Suite

Use this guide when E2E tests fail, hang, or produce inconsistent results.

## Quick Index

| Problem                  | Likely Causes                        | Fix                           | Reference |
| ------------------------ | ------------------------------------ | ----------------------------- | --------- |
| Docker build fails       | Missing dependencies, bad Dockerfile | Fix Dockerfile, install deps  | Issue 1   |
| Tests can't reach server | Network config, wrong URL            | Fix docker-compose networking | Issue 2   |
| Element not found        | Brittle selector, timing issue       | Use stable IDs, add waits     | Issue 3   |
| Tests flaky/intermittent | Race conditions, timing              | Add explicit waits, retries   | Issue 4   |

---

## Issue 1: Docker Build Fails

**Symptoms:** `docker build` exits with errors.

**Steps:**

```bash
# Build with verbose output
docker build -f tests/Dockerfile.test -t audiobook-organizer-test:latest . --progress=plain

# Check for common issues:
# - Base image not found
# - pip install failures
# - Selenium/browser driver issues
```

**Fix:**

Update `Dockerfile.test`:

```dockerfile
FROM python:3.11-slim

# Install browser and dependencies
RUN apt-get update && apt-get install -y \
    chromium chromium-driver \
    firefox-esr \
    && rm -rf /var/lib/apt/lists/*

# Install Python packages
COPY tests/requirements.txt /tmp/
RUN pip install --no-cache-dir -r /tmp/requirements.txt

WORKDIR /tests
```

## Issue 2: Tests Can't Reach Server

**Symptoms:** `ConnectionError`, `requests.exceptions.ConnectionError`.

**Steps:**

```bash
# Check if server running
curl http://localhost:8888/api/health

# Check docker-compose networking
docker-compose -f docker-compose.test.yml ps

# Check test environment variable
docker-compose -f docker-compose.test.yml config | grep APP_URL
```

**Fix:**

- If server in separate container, use service name: `APP_URL=http://app:8888`.
- If server on host, use `host.docker.internal:8888` (Mac/Windows) or `172.17.0.1:8888` (Linux).
- Update test config:

```python
# conftest.py
import os
APP_URL = os.getenv("APP_URL", "http://localhost:8888")
```

## Issue 3: Element Not Found

**Symptoms:** `NoSuchElementException`, element locator fails.

**Steps:**

```bash
# Run test with debug
pytest -v -s tests/e2e/test_dashboard.py::test_dashboard_loads

# Capture screenshot on failure (check test-results/screenshots/)
ls -la test-results/screenshots/
```

**Fix:**

- Use stable selectors (data-testid):

```python
# Bad: brittle class name
element = driver.find_element(By.CLASS_NAME, "MuiButton-root")

# Good: stable test ID
element = driver.find_element(By.CSS_SELECTOR, "[data-testid='scan-button']")
```

- Add explicit wait:

```python
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

wait = WebDriverWait(driver, 10)
element = wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "[data-testid='scan-button']")))
```

## Issue 4: Tests Flaky/Intermittent

**Symptoms:** Tests pass sometimes, fail other times.

**Steps:**

```bash
# Run test multiple times
for i in {1..10}; do
  echo "Run $i"
  pytest tests/e2e/test_scan.py::test_trigger_scan || echo "FAILED"
done
```

**Fix:**

- Replace `time.sleep()` with explicit waits.
- Add retry decorator:

```python
import pytest

@pytest.mark.flaky(reruns=3, reruns_delay=2)
def test_trigger_scan(driver):
    # Test code
```

- Wait for Ajax/API calls to complete:

```python
wait.until(lambda d: d.execute_script("return document.readyState") == "complete")
wait.until(lambda d: d.execute_script("return jQuery.active == 0"))  # If using jQuery
```

## Issue 5: Tests Hang Forever

**Symptoms:** Test run never completes, no output.

**Steps:**

```bash
# Run with timeout
timeout 300 pytest tests/e2e/

# Check for infinite waits
grep "WebDriverWait" tests/e2e/*.py
```

**Fix:**

- Add global timeout in `conftest.py`:

```python
@pytest.fixture
def driver():
    options = webdriver.ChromeOptions()
    options.add_argument("--no-sandbox")
    options.add_argument("--headless")
    driver = webdriver.Chrome(options=options)
    driver.set_page_load_timeout(30)  # 30 second page load timeout
    yield driver
    driver.quit()
```

## Cleanup

```bash
# Stop all containers
docker-compose -f docker-compose.test.yml down -v

# Remove test artifacts
rm -rf test-results/screenshots/* test-results/logs/*
```

If unresolved, capture full test output, screenshots, and docker logs for review.
