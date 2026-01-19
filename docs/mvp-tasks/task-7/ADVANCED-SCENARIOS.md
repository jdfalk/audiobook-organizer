<!-- file: docs/TASK-7-ADVANCED-SCENARIOS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4e8f9d3c-7a5b-4c6d-9f7e-1a8b2c3d4e5f -->
<!-- last-edited: 2026-01-19 -->

# Task 7: Advanced Scenarios & Code Deep Dive (E2E Tests)

Use these scenarios when core tests pass but edge conditions or optimization
needed.

## 🚀 Performance Testing

**Scenario:** Large library (1000+ books) performance validation.

```python
@pytest.mark.slow
def test_large_library_performance():
    # Seed DB with 1000 books
    # Measure page load time
    # Assert < 3 seconds
```

- Use pytest marks: `@pytest.mark.slow`, `@pytest.mark.performance`.
- Run separately: `pytest -m performance`.

## 🌐 Cross-Browser Testing

**Scenario:** Validate in Chrome, Firefox, Safari.

```python
@pytest.fixture(params=["chrome", "firefox"])
def browser(request):
    if request.param == "chrome":
        return webdriver.Chrome()
    elif request.param == "firefox":
        return webdriver.Firefox()
```

- Parametrize browser fixture.
- Skip Safari on Linux CI (use local only).

## 🔁 Flaky Test Mitigation

**Scenario:** Tests fail intermittently due to timing issues.

```python
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

# Wait for element instead of sleep
wait = WebDriverWait(driver, 10)
element = wait.until(EC.presence_of_element_located((By.ID, "book-list")))
```

- Use explicit waits, not `time.sleep()`.
- Retry flaky tests: `@pytest.mark.flaky(reruns=3)`.

## 📸 Screenshot on Failure

**Scenario:** Capture UI state when test fails.

```python
@pytest.hookimpl(tryfirst=True, hookwrapper=True)
def pytest_runtest_makereport(item, call):
    outcome = yield
    report = outcome.get_result()
    if report.when == "call" and report.failed:
        driver = item.funcargs.get("driver")
        if driver:
            driver.save_screenshot(f"test-results/screenshots/{item.name}.png")
```

- Add to `conftest.py`.
- Screenshots saved to `test-results/screenshots/`.

## 🧹 Test Data Cleanup

**Scenario:** Tests leave stale data in DB/filesystem.

```python
@pytest.fixture(scope="function", autouse=True)
def cleanup():
    yield
    # Cleanup after each test
    requests.delete("http://localhost:8888/api/v1/test/reset")
```

- Implement `/api/v1/test/reset` endpoint (test mode only).
- Reset DB, clear temp files after each test.

## 🐳 Docker Networking

**Scenario:** Tests can't reach server running in separate container.

```yaml
# docker-compose.test.yml
services:
  app:
    build: .
    ports:
      - '8888:8888'
    networks:
      - test-network

  tests:
    build:
      context: .
      dockerfile: tests/Dockerfile.test
    depends_on:
      - app
    environment:
      - APP_URL=http://app:8888
    networks:
      - test-network

networks:
  test-network:
```

- Use service name (`app`) instead of `localhost`.
- Set `APP_URL` env var for tests.

## 🧰 Backend Code Checklist

- Test mode flag: `--test-mode` enables reset endpoint, mock data.
- Seed data script: `scripts/seed-test-data.sh` creates sample books.
- Test fixtures: JSON files with sample audiobook metadata.

## 🪛 Frontend Checklist

- Test IDs on elements: `data-testid="book-card"` for stable selectors.
- Avoid brittle selectors (class names change, use IDs).
- Mock API responses in unit tests, real API in E2E.

## 🔬 Performance Considerations

- Parallel test execution: `pytest -n auto` (pytest-xdist).
- Headless mode: `--headless` flag for faster runs.
- Reuse browser sessions: fixture scope='module' for related tests.

When an edge condition is identified, document in `TASK-7-TROUBLESHOOTING.md`.
