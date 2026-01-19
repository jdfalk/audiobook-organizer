<!-- file: web/tests/e2e/NEXT_STEPS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9c8d7e6f-5a4b-3c2d-1e0f-9a8b7c6d5e4f -->
<!-- last-edited: 2026-01-19 -->

# Metadata Provenance E2E Tests - Next Steps

## ✅ Completed (2024-12-28)

- [x] Created comprehensive test file with 13 scenarios
- [x] Implemented AAA pattern for all tests
- [x] Documented test scenarios and mock data
- [x] Created test report and coverage analysis
- [x] All files pass linting and type checking

## 🚀 Immediate Actions (Before Merge)

### 1. Run Tests Locally

```bash
cd web
npm install
npx playwright install --with-deps
npx playwright test metadata-provenance.spec.ts
```

**Expected Outcome**: All 13 tests pass in ~15-20 seconds

### 2. Review Test File

**File to Review**: [metadata-provenance.spec.ts](./metadata-provenance.spec.ts)

**Review Checklist**:

- [ ] Test scenarios match product requirements
- [ ] Mock data reflects actual API responses
- [ ] AAA pattern is clear and readable
- [ ] No hardcoded values that should be configurable
- [ ] Error messages are descriptive

### 3. Update CI/CD Workflow

**File**: `.github/workflows/web-tests.yml` (or similar)

**Add to existing test job**:

```yaml
- name: Run E2E Tests
  run: |
    cd web
    npm run test:e2e
```

**Expected Outcome**: Tests run on every PR/push to main

## 📋 Short-Term Priorities (Next Sprint)

### Priority 1: Error Handling Tests

**Why**: Validates resilience when API calls fail

**Implementation**:

```typescript
test('handles API error when applying override', async ({ page }) => {
  // Arrange: Mock PUT failure
  await page.route('**/api/v1/audiobooks/*/tags', route => {
    route.fulfill({
      status: 500,
      body: JSON.stringify({ error: 'Internal error' }),
    });
  });

  // Act: Attempt to apply override
  await page.goto(`/library/${bookId}`);
  await page.getByRole('tab', { name: 'Compare' }).click();
  const titleRow = page.locator('tr').filter({ hasText: /^title$/i });
  await titleRow.getByRole('button', { name: 'Use File' }).click();

  // Assert: Error message displays
  await expect(page.getByText(/failed|error/i)).toBeVisible();
});
```

### Priority 2: Fetch Metadata Integration

**Why**: Validates that metadata refresh updates provenance

**Implementation**:

```typescript
test('updates fetched_value after metadata refresh', async ({ page }) => {
  // Arrange: Mock initial state
  await setupProvenanceRoutes(page);
  await page.goto(`/library/${bookId}`);

  // Mock fetch-metadata endpoint to return new data
  await page.route('**/api/v1/audiobooks/*/fetch-metadata', route => {
    route.fulfill({
      status: 200,
      body: JSON.stringify({
        message: 'refreshed',
        book: { title: 'New Fetched Title' },
      }),
    });
  });

  // Act: Trigger fetch metadata
  await page.getByRole('button', { name: 'Fetch Metadata' }).click();

  // Assert: Fetched value updates in Compare tab
  await page.getByRole('tab', { name: 'Compare' }).click();
  const titleRow = page.locator('tr').filter({ hasText: /^title$/i });
  await expect(titleRow.getByText('New Fetched Title')).toBeVisible();
});
```

### Priority 3: Visual Regression Baseline

**Why**: Catches unintended UI changes

**Implementation**:

```bash
# Generate baseline screenshots
npx playwright test metadata-provenance.spec.ts --update-snapshots

# Add to test file:
test('provenance UI matches baseline', async ({ page }) => {
  await setupProvenanceRoutes(page);
  await page.goto(`/library/${bookId}`);
  await page.getByRole('tab', { name: 'Tags' }).click();
  await expect(page).toHaveScreenshot('tags-tab-provenance.png');
});
```

## 📊 Medium-Term Goals (Future Sprints)

### Goal 1: Accessibility Validation

**Implementation**:

```typescript
test('provenance UI is keyboard accessible', async ({ page }) => {
  await setupProvenanceRoutes(page);
  await page.goto(`/library/${bookId}`);
  await page.getByRole('tab', { name: 'Compare' }).click();

  // Tab to first action button
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');

  // Assert: Override applied via keyboard
  await expect(
    page.getByRole('heading', { name: 'File: Provenance Test' })
  ).toBeVisible();
});
```

### Goal 2: Performance Benchmarks

**Implementation**:

```typescript
test('provenance data loads in <500ms', async ({ page }) => {
  await setupProvenanceRoutes(page);
  const startTime = Date.now();

  await page.goto(`/library/${bookId}`);
  await page.getByRole('tab', { name: 'Tags' }).click();
  await page.getByText('Provenance Test Book').waitFor();

  const loadTime = Date.now() - startTime;
  expect(loadTime).toBeLessThan(500);
});
```

### Goal 3: Real Backend Integration

**Implementation**:

1. Configure test environment with real Go backend
2. Set up test database with fixtures
3. Run tests against actual API endpoints
4. Validate SSE updates work correctly

**Config Update** (playwright.config.ts):

```typescript
webServer: {
  command: '../audiobook-organizer serve --port 8080',
  url: 'http://127.0.0.1:8080',
  reuseExistingServer: false,
}
```

## 🔍 Code Review Checklist

### Before Merging

- [ ] All 13 tests pass locally
- [ ] No TypeScript errors
- [ ] No ESLint warnings
- [ ] Documentation is complete and accurate
- [ ] Test names are descriptive
- [ ] AAA pattern is clear in each test
- [ ] Mock data matches API contract
- [ ] Edge cases are covered
- [ ] Comments explain "why" not "what"

### After Merging

- [ ] Tests run successfully in CI/CD
- [ ] Test results appear in PR checks
- [ ] Team is aware of new test coverage
- [ ] Documentation is accessible to team

## 📚 Reference Documentation

**Primary Files**:

- [metadata-provenance.spec.ts](./metadata-provenance.spec.ts) - Test
  implementation
- [METADATA_PROVENANCE_TESTS.md](./METADATA_PROVENANCE_TESTS.md) - Test
  documentation
- [TEST_COVERAGE_SUMMARY.md](./TEST_COVERAGE_SUMMARY.md) - Quick reference
- [../../docs/TEST_ORCHESTRATOR_REPORT_PROVENANCE.md](../../docs/TEST_ORCHESTRATOR_REPORT_PROVENANCE.md) -
  Full report

**External Resources**:

- [Playwright Documentation](https://playwright.dev/)
- [AAA Pattern Guide](https://automationpanda.com/2020/07/07/arrange-act-assert-a-pattern-for-writing-good-tests/)
- [Backend API Documentation](../../docs/api-endpoints.md) (if exists)

## 🐛 Known Issues and Workarounds

### Issue 1: Mock EventSource Required

**Problem**: Real EventSource connections cause test timeouts

**Solution**: `mockEventSource()` function stubs out SSE

**Code**:

```typescript
await page.addInitScript(() => {
  class MockEventSource {
    url: string;
    constructor(url: string) {
      this.url = url;
    }
    addEventListener() {}
    removeEventListener() {}
    close() {}
  }
  (window as unknown as { EventSource: typeof EventSource }).EventSource =
    MockEventSource as unknown as typeof EventSource;
});
```

### Issue 2: State Resets Between Tests

**Problem**: Need fresh state for each test

**Solution**: Each test calls `setupProvenanceRoutes()` independently

**Best Practice**: Never rely on state from previous tests

## 💡 Tips for Extending Tests

### Adding a New Test

1. Copy existing test as template
2. Update test name to describe behavior
3. Modify mock data in Arrange section
4. Change actions in Act section
5. Update assertions in Assert section
6. Run test: `npx playwright test -g "your test name"`

### Adding a New Field to Provenance

1. Update `TagsData` type definition
2. Add field to `createTagsData()` function
3. Add field to test assertions where relevant
4. Update Compare table field list in tests

### Debugging a Failing Test

```bash
# Run with UI
npx playwright test metadata-provenance.spec.ts --ui

# Run with debug mode
npx playwright test metadata-provenance.spec.ts --debug

# Generate trace
npx playwright test metadata-provenance.spec.ts --trace on
```

## 📞 Support and Questions

**Test Issues**: Review
[METADATA_PROVENANCE_TESTS.md](./METADATA_PROVENANCE_TESTS.md) **API
Questions**: Check backend documentation or API contract **Playwright Help**:
See [Playwright Docs](https://playwright.dev/)

---

**Last Updated**: 2024-12-28 **Next Review**: After first test run or merge to
main
