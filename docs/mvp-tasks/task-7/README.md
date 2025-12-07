<!-- file: docs/TASK-7-README.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2c6e9f3d-7a4b-4c5d-9f7e-1a8b2c3d4e5f -->

# Task 7: E2E Test Suite - Complete Documentation

## 📖 Overview

This task implements and validates a containerized end-to-end test suite using Selenium/pytest to ensure all critical user workflows function correctly. Core requirement: automated tests run in Docker container with consistent results, covering all MVP features.

**Deliverables:**

- Dockerized E2E test environment with Selenium, pytest, and browser drivers.
- Test suite covering: library setup, scan, import, organize, metadata edit, delete workflows.
- VS Code task to run tests inside container for consistent automation.
- Tests pass reliably without flakiness (retry logic, proper waits).
- CI integration ready (tests can run in GitHub Actions).

## 📂 Document Set

| Document                       | Purpose                                              |
| ------------------------------ | ---------------------------------------------------- |
| `TASK-7-CORE-TESTING.md`       | Core validation flow, setup, execution               |
| `TASK-7-ADVANCED-SCENARIOS.md` | Edge cases (performance, cross-browser, flaky tests) |
| `TASK-7-TROUBLESHOOTING.md`    | Issues, root causes, and fixes                       |
| `TASK-7-README.md` (this file) | Overview, navigation, quick commands                 |

**Reading order:** README → Core → Advanced → Troubleshooting.

## 🎯 Success Criteria

- Docker image builds successfully with all test dependencies.
- `docker-compose up test` or VS Code task runs full E2E suite.
- Tests cover: dashboard load, library scan, import file, organize, metadata edit, delete with prevention.
- All tests pass with green status; no false failures.
- Test logs and screenshots captured on failure for debugging.
- README documents how to run tests locally and in CI.

## 🚀 Quick Start

```bash
# Check if E2E infrastructure exists
ls -la tests/e2e tests/Dockerfile.test docker-compose.test.yml

# Build test image
docker build -f tests/Dockerfile.test -t audiobook-organizer-test .

# Run E2E suite
docker-compose -f docker-compose.test.yml up --abort-on-container-exit

# Or via VS Code task
# Task: "Run E2E Tests in Docker"
```

## 🔐 Multi-AI Safety

- E2E tests run in isolated containers; no production data affected.
- Tests use temporary library/import folders; cleaned up after run.
- Capture screenshots and logs on failure for debugging.

## 🧭 Navigation

- Need the main flow? → `TASK-7-CORE-TESTING.md`
- Handling edge cases? → `TASK-7-ADVANCED-SCENARIOS.md`
- Something broken? → `TASK-7-TROUBLESHOOTING.md`

## 🧩 Current State (from TODO)

- Priority: High (New Requirement, MVP-blocking for quality assurance)
- Status: Partial (Docker test image exists but tests failing/incomplete)
- Depends on: All other MVP tasks (tests validate integrated workflows)

## ✅ Next Actions

1. Review existing `tests/e2e/` directory and Dockerfile.test.
2. Expand test scenarios to cover all MVP workflows.
3. Fix failing tests (element locators, timing, API responses).
4. Create VS Code task for running tests.
5. Document test setup and execution in README.
6. Run Core Phases to validate end-to-end coverage.

---
