<!-- file: CI_E2E_INTEGRATION.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-01-26 -->

# CI/CD E2E Test Integration Guide

**Status**: E2E tests implemented but **NOT running in CI** ⚠️

---

## Current Situation

### ✅ What's Done
1. **MUI Icon Import Issues Fixed**: All imports updated to include `.js` extension
   - `src/components/ErrorBoundary.tsx`
   - `src/components/layout/Sidebar.tsx`
   - `src/components/layout/TopBar.tsx`
   - `src/pages/BookDetail.tsx`

2. **E2E Tests Implemented**: 141 tests across 15 files (92% coverage)

3. **CI Infrastructure Exists**: `frontend-ci.yml` workflow calls reusable workflow

### ⚠️ What's Missing
**E2E tests are NOT running in CI!**

The current CI workflow only runs:
- `npm run test` (vitest unit tests)
- `npm run build` (build step)
- `npm run lint` (linting)

But **NOT**:
- `npm run test:e2e` (Playwright E2E tests)

---

## Current CI Architecture

### Workflow Flow
```
audiobook-organizer/.github/workflows/frontend-ci.yml
    ↓
jdfalk/ghcommon/.github/workflows/reusable-ci.yml
    ↓
frontend-ci job (lines 600-688)
    ↓
Test frontend step (lines 680-688) → runs "npm run test" (vitest)
```

### Current Frontend Test Step
```yaml
- name: Test frontend
  if: ${{ !inputs.skip-tests }}
  uses: jdfalk/ci-workflow-helpers-action@71e09093947c478e54a01855baf76a43f36843a4
  with:
    command: frontend-run
    frontend-working-dir: ${{ steps.frontend-dir.outputs.dir }}
    frontend-script: test  # ← Runs "npm run test" (vitest only)
    frontend-success-message: '✅ Tests passed'
    frontend-failure-message: 'ℹ️ No tests configured'
```

---

## Solution: Add E2E Test Step to Reusable Workflow

### Option 1: Add Separate E2E Test Step (Recommended)

**Location**: `jdfalk/ghcommon/.github/workflows/reusable-ci.yml`
**After**: Line 688 (after "Test frontend" step)

```yaml
- name: Install Playwright browsers
  if: ${{ !inputs.skip-tests }}
  working-directory: ${{ steps.frontend-dir.outputs.dir }}
  run: npx playwright install --with-deps chromium webkit

- name: Run E2E tests
  if: ${{ !inputs.skip-tests }}
  uses: jdfalk/ci-workflow-helpers-action@71e09093947c478e54a01855baf76a43f36843a4
  with:
    command: frontend-run
    frontend-working-dir: ${{ steps.frontend-dir.outputs.dir }}
    frontend-script: test:e2e
    frontend-success-message: '✅ E2E tests passed'
    frontend-failure-message: 'ℹ️ No E2E tests configured'

- name: Upload Playwright report
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: playwright-report
    path: ${{ steps.frontend-dir.outputs.dir }}/playwright-report/
    retention-days: 30
```

### Option 2: Modify Existing Test Step

**Pros**: Uses existing infrastructure
**Cons**: Mixes unit and E2E tests in one step

```yaml
- name: Test frontend (unit tests)
  if: ${{ !inputs.skip-tests }}
  uses: jdfalk/ci-workflow-helpers-action@71e09093947c478e54a01855baf76a43f36843a4
  with:
    command: frontend-run
    frontend-working-dir: ${{ steps.frontend-dir.outputs.dir }}
    frontend-script: test
    frontend-success-message: '✅ Unit tests passed'
    frontend-failure-message: 'ℹ️ No unit tests configured'

# Then add the E2E step as shown in Option 1
```

---

## Implementation Steps

### 1. Update Reusable Workflow (ghcommon)

**File**: `jdfalk/ghcommon/.github/workflows/reusable-ci.yml`
**Changes**: Add E2E test steps after line 688

### 2. Test Locally First

```bash
cd web

# Fix any remaining issues
npm run test:e2e
```

### 3. Push to ghcommon

```bash
cd ../ghcommon
git checkout -b feature/add-e2e-tests
# Add E2E test steps to reusable-ci.yml
git add .github/workflows/reusable-ci.yml
git commit -m "feat(ci): add Playwright E2E test step to frontend CI

- Install Playwright browsers (chromium, webkit)
- Run npm run test:e2e for E2E tests
- Upload Playwright report artifacts
- Add separate step for clear separation of unit vs E2E tests"
git push origin feature/add-e2e-tests
# Create PR and merge to main
```

### 4. Update audiobook-organizer to Use New ghcommon Version

**File**: `audiobook-organizer/.github/workflows/frontend-ci.yml`
**Line 51**: Update ref to point to new ghcommon commit

```yaml
uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@<new-commit-sha>
```

### 5. Test in CI

Push a change to `audiobook-organizer/web/**` and verify:
- Unit tests run (`npm run test`)
- E2E tests run (`npm run test:e2e`)
- Playwright report uploaded as artifact

---

## Configuration Notes

### Playwright Config

**File**: `web/tests/e2e/playwright.config.ts`

Current config:
- Base URL: `http://127.0.0.1:4173`
- Web server: `npm run dev -- --host --port 4173`
- Browsers: chromium, webkit
- Workers: 2
- Timeout: 30s

**Issue**: Web server command uses `npm run dev` which might not be production-like.

**Recommendation**: Consider using `npm run preview` instead:
```typescript
webServer: {
  command: 'npm run build && npm run preview -- --port 4173',
  url: 'http://127.0.0.1:4173',
  reuseExistingServer: !process.env.CI,
}
```

### Package.json Scripts

Current scripts:
```json
{
  "test": "vitest",           // Unit tests
  "test:e2e": "playwright test -c tests/e2e/playwright.config.ts"  // E2E tests
}
```

---

## Verification Checklist

After CI integration:

- [ ] E2E tests run in CI on every push to main/develop
- [ ] E2E tests run in CI on every PR
- [ ] Playwright report artifacts uploaded on failure
- [ ] CI fails if E2E tests fail
- [ ] Test execution time is reasonable (< 10 minutes)
- [ ] Flaky tests identified and fixed
- [ ] All 141 tests pass consistently

---

## Expected CI Output

Successful CI run should show:

```
✅ Lint frontend code
✅ Build frontend
✅ Test frontend (unit tests) - 12 tests passed
✅ Install Playwright browsers
✅ Run E2E tests - 141 tests passed
✅ Upload Playwright report
```

---

## Troubleshooting

### Common Issues

1. **Module Import Errors** (✅ Fixed)
   - Error: `Cannot find module '@mui/icons-material/Dashboard'`
   - Solution: Added `.js` extensions to all MUI icon imports

2. **Playwright Browsers Not Installed**
   - Error: `Executable doesn't exist at ...`
   - Solution: Add `npx playwright install --with-deps` step before tests

3. **Web Server Not Starting**
   - Error: `waiting for http://127.0.0.1:4173 failed`
   - Solution: Check `webServer` config in playwright.config.ts

4. **Tests Timeout in CI**
   - Error: `Test timeout of 30000ms exceeded`
   - Solution: Increase timeout or optimize slow tests

5. **Flaky Tests**
   - Error: Tests pass locally but fail in CI
   - Solution: Use proper wait strategies, avoid hardcoded delays

---

## Performance Considerations

### Current Test Suite
- **141 tests** across **15 files**
- **Estimated time**: 5-8 minutes (not yet verified)
- **Workers**: 2 parallel workers
- **Browsers**: chromium + webkit

### Optimization Options

1. **Run only chromium in CI** (fastest)
   ```typescript
   projects: [
     {
       name: 'chromium',
       use: { ...devices['Desktop Chrome'] },
     },
     // Remove webkit for CI, keep for local testing
   ],
   ```

2. **Increase workers** (if CI has resources)
   ```typescript
   workers: process.env.CI ? 4 : 2,
   ```

3. **Shard tests** (for very large test suites)
   ```bash
   npm run test:e2e -- --shard=1/2
   npm run test:e2e -- --shard=2/2
   ```

---

## Maintenance

### Regular Tasks

1. **Weekly**: Review failed E2E tests and fix flakiness
2. **Monthly**: Update Playwright to latest version
3. **Per Sprint**: Add E2E tests for new features
4. **Before Release**: Run full E2E suite including webkit

### Monitoring

- Track E2E test execution time trends
- Monitor flaky test rate (should be < 1%)
- Review Playwright report artifacts for failures

---

## Next Steps

1. **Immediate** (Today):
   - ✅ Fix MUI icon imports (DONE)
   - ⏳ Add E2E test step to ghcommon reusable workflow
   - ⏳ Test locally to verify all tests pass

2. **This Week**:
   - Integrate E2E tests into CI pipeline
   - Run full test suite and verify stability
   - Create documentation for team

3. **Follow-up**:
   - Monitor CI performance
   - Address any flaky tests
   - Optimize test execution time if needed

---

**Last Updated**: 2026-01-26
**Author**: Claude Code
**Status**: Ready for CI integration
