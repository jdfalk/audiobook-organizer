<!-- file: docs/e2e-tests/TEST_INFRASTRUCTURE_ANALYSIS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7b8c9d0e-1f2a-3b4c-5d6e-7f8a9b0c1d2e -->

# E2E Test Infrastructure Analysis

## Executive Summary

**Status**: 🔴 **INFRASTRUCTURE REQUIRED**

The new metadata provenance E2E tests (`metadata-provenance.spec.ts`) are
well-designed but require running backend infrastructure to execute. All 13
tests failed with "Cannot navigate to invalid URL" errors because they attempt
to navigate to actual application URLs (`/library/${bookId}`).

**Impact**: Tests cannot run in isolation without:

1. Running backend server (Go application on <http://localhost:8080>)
2. Test database with seed data
3. Mock API routes properly configured

---

## Test Execution Results

### Run Summary

```bash
npx playwright test metadata-provenance.spec.ts --reporter=list
```

**Results**:

- Total Tests: 13
- Passed: 0 ✘
- Failed: 13 ✘
- Duration: ~1090ms

**Failure Pattern**: All tests failed with identical error:

```
Error: page.goto: Protocol error (Page.navigate): Cannot navigate to invalid URL
Call log:
  - navigating to "/library/prov-test-book", waiting until "load"
```

---

## Root Cause Analysis

### Problem Identification

The tests use `page.goto('/library/${bookId}')` expecting:

1. A running web server at the base URL
2. Valid routes configured for `/library/*` paths
3. Backend API responding to metadata requests

**Test Architecture**: Integration tests, not unit tests

- Tests interact with actual DOM rendered by React Router
- Tests navigate between tabs (Tags, Compare, Details)
- Tests trigger API calls via route handlers (mocked)

### Current Test Setup

```typescript
// File: web/tests/e2e/metadata-provenance.spec.ts

// Mock setup (INLINE API mocks)
async function setupProvenanceRoutes(page: Page) {
  await page.route('**/api/books/*/metadata-state', async route => {
    await route.fulfill({
      status: 200,
      body: JSON.stringify(mockMetadataState),
    });
  });
}

// Test execution (REQUIRES RUNNING APP)
await page.goto(`/library/${bookId}`); // ❌ Fails: no server running
await page.getByRole('tab', { name: 'Tags' }).click();
```

---

## Infrastructure Options

### Option 1: Backend Server Integration (RECOMMENDED)

**Setup**:

1. Start Go backend server before tests: `go run main.go`
2. Configure Playwright `baseURL` in `playwright.config.ts`
3. Seed test database with test data
4. Run tests against live application

**Pros**:

- Tests full integration stack
- Validates real API interactions
- Catches routing and navigation issues
- Most accurate representation of production

**Cons**:

- Requires backend running (CI/CD complexity)
- Slower test execution
- Test data management overhead
- Database state cleanup needed

**Implementation**:

```typescript
// playwright.config.ts
export default defineConfig({
  use: {
    baseURL: 'http://localhost:8080', // Go backend
  },
  webServer: {
    command: 'go run main.go',
    port: 8080,
    timeout: 120 * 1000,
    reuseExistingServer: !process.env.CI,
  },
});
```

**Effort**: 4-6 hours

- Backend server start script: 1 hour
- Test data seed script: 2 hours
- CI/CD integration: 1-2 hours
- Documentation: 1 hour

---

### Option 2: Mock Service Worker (MSW) (ALTERNATIVE)

**Setup**:

1. Install MSW: `npm install msw@latest --save-dev`
2. Create MSW handlers for all API routes
3. Start MSW server in Playwright global setup
4. Run frontend dev server with MSW intercepting API calls

**Pros**:

- No backend required for E2E tests
- Faster test execution
- Easier to control test scenarios
- Simpler CI/CD setup

**Cons**:

- Doesn't test real backend integration
- Mock drift risk (mocks vs actual API)
- Additional tooling overhead
- Frontend-only validation

**Implementation**:

```typescript
// web/tests/mocks/handlers.ts
import { http, HttpResponse } from 'msw';

export const handlers = [
  http.get('/api/books/:id/metadata-state', () => {
    return HttpResponse.json(mockMetadataState);
  }),
  // ... more handlers
];

// web/tests/global-setup.ts
import { setupServer } from 'msw/node';
import { handlers } from './mocks/handlers';

const server = setupServer(...handlers);
server.listen();
```

**Effort**: 3-4 hours

- MSW setup: 1 hour
- Handler creation: 1-2 hours
- Test updates: 30 minutes
- Documentation: 30 minutes

---

### Option 3: Component Testing (PRAGMATIC)

**Setup**:

1. Convert E2E tests to component tests with `@playwright/experimental-ct-react`
2. Test individual components in isolation
3. Mock React Router and API hooks
4. Faster, more focused tests

**Pros**:

- No backend required
- Fastest test execution
- Focused component validation
- Simpler test maintenance

**Cons**:

- Doesn't test routing integration
- Doesn't validate full page interactions
- More granular (less comprehensive)
- Different test paradigm

**Implementation**:

```typescript
// web/tests/component/BookDetailTabs.spec.tsx
import { test, expect } from '@playwright/experimental-ct-react';
import { BookDetailTabs } from '@/components/BookDetailTabs';

test('displays provenance in Tags tab', async ({ mount }) => {
  const component = await mount(
    <BookDetailTabs book={mockBook} metadataState={mockMetadataState} />
  );

  await component.getByRole('tab', { name: 'Tags' }).click();
  await expect(component.getByText('Effective Source: stored')).toBeVisible();
});
```

**Effort**: 5-7 hours

- Component test framework setup: 2 hours
- Test conversion: 2-3 hours
- Mock setup: 1-2 hours
- Documentation: 1 hour

---

## Recommendation Matrix

| Criterion            | Option 1 (Backend)    | Option 2 (MSW)          | Option 3 (Component)   |
| -------------------- | --------------------- | ----------------------- | ---------------------- |
| **Test Coverage**    | ⭐⭐⭐⭐⭐ Full stack | ⭐⭐⭐⭐ Frontend + API | ⭐⭐⭐ Components only |
| **Setup Complexity** | ⭐⭐ Moderate-High    | ⭐⭐⭐ Moderate         | ⭐⭐⭐⭐ Low-Moderate  |
| **Execution Speed**  | ⭐⭐ Slow             | ⭐⭐⭐⭐ Fast           | ⭐⭐⭐⭐⭐ Very Fast   |
| **CI/CD Ease**       | ⭐⭐ Complex          | ⭐⭐⭐⭐ Simple         | ⭐⭐⭐⭐⭐ Very Simple |
| **Maintenance**      | ⭐⭐⭐ Moderate       | ⭐⭐⭐ Moderate         | ⭐⭐⭐⭐ Low           |
| **Value for Effort** | ⭐⭐⭐⭐⭐ Highest    | ⭐⭐⭐⭐ High           | ⭐⭐⭐ Moderate        |

**Recommended Approach**: **Option 1 (Backend Server Integration)**

**Rationale**:

1. Provides most comprehensive test coverage
2. Validates full integration stack (frontend + backend)
3. Catches issues that isolated tests miss
4. Aligns with test intent (E2E validation)
5. Higher effort but highest confidence

---

## Implementation Plan (Option 1 - Backend Integration)

### Phase 1: Local Development Setup (Day 1)

**Tasks**:

1. ✅ Create backend start script
2. ✅ Configure Playwright `baseURL`
3. ✅ Create test data seed script
4. ✅ Update test documentation

**Deliverables**:

- `scripts/start-test-backend.sh` - Starts Go server with test config
- `scripts/seed-test-data.sh` - Seeds database with test audiobooks
- `playwright.config.ts` update - Adds webServer config
- `docs/e2e-tests/SETUP_GUIDE.md` - Step-by-step instructions

### Phase 2: Test Execution (Day 2)

**Tasks**:

1. ✅ Run tests locally with backend
2. ✅ Fix any test failures
3. ✅ Validate all 13 tests pass
4. ✅ Document any known issues

**Deliverables**:

- Test execution report
- Fixed tests (if needed)
- Known issues documented

### Phase 3: CI/CD Integration (Day 3)

**Tasks**:

1. ✅ Add backend start to GitHub Actions workflow
2. ✅ Configure test database in CI
3. ✅ Add E2E test step to CI pipeline
4. ✅ Validate tests pass in CI

**Deliverables**:

- `.github/workflows/e2e-tests.yml` - New workflow
- CI test execution report
- Troubleshooting guide

---

## Quick Start (Temporary Workaround)

Until infrastructure is set up, run tests against running dev environment:

```bash
# Terminal 1: Start backend
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go run main.go

# Terminal 2: Start frontend dev server
cd web
npm run dev

# Terminal 3: Run E2E tests
cd web
npx playwright test metadata-provenance.spec.ts
```

**Expected Behavior**:

- Tests should navigate to `http://localhost:5173/library/prov-test-book`
- React Router should handle routing
- API mocks should intercept `/api/books/*/metadata-state` calls
- All 13 tests should pass

---

## Next Steps

### Immediate Actions (This Week)

1. **Create Backend Start Script** (1 hour)
   - Script to start Go server with test configuration
   - Add signal handling for graceful shutdown
   - Configure test database path

2. **Create Test Data Seed Script** (2 hours)
   - Seed audiobook with `bookId = "prov-test-book"`
   - Create metadata_state entries matching mock data
   - Add cleanup script for test data

3. **Update Playwright Config** (30 minutes)
   - Add `baseURL: 'http://localhost:8080'`
   - Add `webServer` configuration
   - Configure test timeout and retries

4. **Run Tests Locally** (1 hour)
   - Execute full test suite
   - Document any failures
   - Update tests if needed

### Medium-Term Actions (Next Sprint)

5. **Integrate into CI/CD** (2-3 hours)
   - Add E2E test workflow
   - Configure database in CI
   - Add test artifacts upload

6. **Expand Test Coverage** (4-6 hours)
   - Add error scenario tests
   - Add accessibility tests
   - Add visual regression tests

7. **Documentation** (2 hours)
   - Complete setup guide
   - Add troubleshooting section
   - Document test data management

---

## Resources

### Files Referenced

- `web/tests/e2e/metadata-provenance.spec.ts` - Test file (663 lines)
- `web/playwright.config.ts` - Playwright configuration
- `main.go` - Backend entry point
- `internal/database/sqlite.go` - Database implementation

### Documentation

- [Playwright Web Server Guide](https://playwright.dev/docs/test-webserver)
- [Playwright Best Practices](https://playwright.dev/docs/best-practices)
- [MSW Documentation](https://mswjs.io/) - If choosing Option 2

### Related Files

- `docs/MANUAL_TEST_PLAN.md` - Manual testing scenarios
- `docs/TEST_DATA_SETUP_GUIDE.md` - Test data creation guide
- `docs/e2e-tests/test-orchestrator-report.md` - Test creation report

---

## Decision Log

| Date         | Decision                                    | Rationale                                      |
| ------------ | ------------------------------------------- | ---------------------------------------------- |
| Dec 28, 2025 | Tests require infrastructure setup          | All 13 tests failed with URL navigation errors |
| Dec 28, 2025 | Recommend Option 1 (Backend Integration)    | Highest test confidence, validates full stack  |
| Dec 28, 2025 | Defer CI integration until local tests pass | Reduce complexity, validate approach first     |

---

**Status**: 🟡 **BLOCKED** - Tests cannot run without backend infrastructure

**Next Action**: Create backend start script and test data seed script

**Owner**: Development Team

**Priority**: P1 (High) - Required for E2E test validation

**Estimated Effort**: 6-8 hours total (spread over 2-3 days)

---

**Last Updated**: December 28, 2025 **Next Review**: After backend
infrastructure setup complete
