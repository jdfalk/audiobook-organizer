<!-- file: GHCOMMON_E2E_PATCH.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9c8d7e6f-5a4b-3c2d-1e0f-9a8b7c6d5e4f -->
<!-- last-edited: 2026-01-26 -->

# Patch: Add E2E Tests to ghcommon Reusable CI Workflow

**Target File**: `jdfalk/ghcommon/.github/workflows/reusable-ci.yml`
**Location**: After line 688 (after "Test frontend" step)
**Version Bump**: 1.9.0 → 1.10.0

---

## Changes Required

### 1. Add Input Parameter for E2E Tests (Optional)

**Location**: After line 43 (with other inputs)

```yaml
skip-e2e-tests:
  description: 'Skip running E2E tests'
  required: false
  type: boolean
  default: false
```

### 2. Add E2E Test Steps to frontend-ci Job

**Location**: After line 688 (after "Test frontend" step)

```yaml
      # E2E Tests with Playwright
      - name: Install Playwright browsers
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests }}
        working-directory: ${{ steps.frontend-dir.outputs.dir }}
        run: |
          npx playwright install --with-deps chromium webkit
        shell: bash

      - name: Run E2E tests
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests }}
        uses: jdfalk/ci-workflow-helpers-action@71e09093947c478e54a01855baf76a43f36843a4 # v1.1.3
        with:
          command: frontend-run
          frontend-working-dir: ${{ steps.frontend-dir.outputs.dir }}
          frontend-script: test:e2e
          frontend-success-message: '✅ E2E tests passed'
          frontend-failure-message: 'ℹ️ No E2E tests configured or test:e2e script not found'
        continue-on-error: false

      - name: Upload Playwright report
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests && always() }}
        uses: actions/upload-artifact@b7c566a772e6b6bfb58ed0dc250532a479d7789f # v6.0.0
        with:
          name: playwright-report-${{ matrix.os }}-node-${{ matrix.node-version }}
          path: ${{ steps.frontend-dir.outputs.dir }}/playwright-report/
          retention-days: 30
          if-no-files-found: ignore

      - name: Upload Playwright test results
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests && always() }}
        uses: actions/upload-artifact@b7c566a772e6b6bfb58ed0dc250532a479d7789f # v6.0.0
        with:
          name: playwright-results-${{ matrix.os }}-node-${{ matrix.node-version }}
          path: ${{ steps.frontend-dir.outputs.dir }}/test-results/
          retention-days: 7
          if-no-files-found: ignore
```

---

## Complete Patch (Insert After Line 688)

```yaml
      - name: Test frontend
        if: ${{ !inputs.skip-tests }}
        uses: jdfalk/ci-workflow-helpers-action@71e09093947c478e54a01855baf76a43f36843a4 # v1.1.3
        with:
          command: frontend-run
          frontend-working-dir: ${{ steps.frontend-dir.outputs.dir }}
          frontend-script: test
          frontend-success-message: '✅ Tests passed'
          frontend-failure-message: 'ℹ️ No tests configured'

      # ============================================================
      # NEW: E2E Tests with Playwright
      # ============================================================

      - name: Install Playwright browsers
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests }}
        working-directory: ${{ steps.frontend-dir.outputs.dir }}
        run: |
          npx playwright install --with-deps chromium webkit
        shell: bash

      - name: Run E2E tests
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests }}
        uses: jdfalk/ci-workflow-helpers-action@71e09093947c478e54a01855baf76a43f36843a4 # v1.1.3
        with:
          command: frontend-run
          frontend-working-dir: ${{ steps.frontend-dir.outputs.dir }}
          frontend-script: test:e2e
          frontend-success-message: '✅ E2E tests passed'
          frontend-failure-message: 'ℹ️ No E2E tests configured or test:e2e script not found'
        continue-on-error: false

      - name: Upload Playwright report
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests && always() }}
        uses: actions/upload-artifact@b7c566a772e6b6bfb58ed0dc250532a479d7789f # v6.0.0
        with:
          name: playwright-report-${{ matrix.os }}-node-${{ matrix.node-version }}
          path: ${{ steps.frontend-dir.outputs.dir }}/playwright-report/
          retention-days: 30
          if-no-files-found: ignore

      - name: Upload Playwright test results
        if: ${{ !inputs.skip-tests && !inputs.skip-e2e-tests && always() }}
        uses: actions/upload-artifact@b7c566a772e6b6bfb58ed0dc250532a479d7789f # v6.0.0
        with:
          name: playwright-results-${{ matrix.os }}-node-${{ matrix.node-version }}
          path: ${{ steps.frontend-dir.outputs.dir }}/test-results/
          retention-days: 7
          if-no-files-found: ignore

      # ============================================================
      # END: E2E Tests
      # ============================================================
```

---

## Update Workflow Version

**Location**: Line 2

```yaml
# file: .github/workflows/reusable-ci.yml
# version: 1.10.0  # ← Update from 1.9.0
```

---

## Update CI Summary Job (Optional)

**Location**: Line 763 (in ci-summary job)

Add E2E test result to summary:

```yaml
env:
  JOB_DETECT_CHANGES: ${{ needs.detect-changes.result || 'skipped' }}
  JOB_WORKFLOW_LINT: ${{ needs.workflow-lint.result || 'skipped' }}
  JOB_WORKFLOW_SCRIPTS: ${{ needs.workflow-scripts.result || 'skipped' }}
  JOB_GO: ${{ needs.go-ci.result || 'skipped' }}
  JOB_PYTHON: ${{ needs.python-ci.result || 'skipped' }}
  JOB_RUST: ${{ needs.rust-ci.result || 'skipped' }}
  JOB_FRONTEND: ${{ needs.frontend-ci.result || 'skipped' }}
  JOB_FRONTEND_E2E: ${{ needs.frontend-ci.result || 'skipped' }}  # ← Add
  JOB_DOCKER: skipped
  JOB_DOCS: skipped
```

---

## Testing Instructions

### 1. Apply Patch to ghcommon

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/ghcommon

# Create feature branch
git checkout -b feature/add-playwright-e2e-tests

# Edit .github/workflows/reusable-ci.yml
# Apply changes shown above

# Commit changes
git add .github/workflows/reusable-ci.yml
git commit -m "feat(ci): add Playwright E2E test support to frontend CI

- Add skip-e2e-tests input parameter for flexibility
- Install Playwright browsers (chromium, webkit) with deps
- Run npm run test:e2e for E2E tests
- Upload Playwright report and test-results artifacts
- Support matrix testing (multiple OS/Node versions)
- Continue-on-error: false to fail CI on E2E failures

Closes: #<issue-number>
"

# Push and create PR
git push origin feature/add-playwright-e2e-tests
```

### 2. Test with audiobook-organizer

**Option A: Test with PR (Recommended)**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Update frontend-ci.yml to use PR branch
# Edit .github/workflows/frontend-ci.yml line 51:
uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@feature/add-playwright-e2e-tests
```

**Option B: Test locally first**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web

# Install dependencies
npm install

# Run E2E tests locally
npm run test:e2e

# Verify all 141 tests pass
```

### 3. Merge and Update Reference

After PR is approved and merged to main:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Update to use main branch with new commit
# Edit .github/workflows/frontend-ci.yml line 51:
uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@<new-commit-sha>

git add .github/workflows/frontend-ci.yml
git commit -m "ci: enable E2E tests in frontend CI

Updates ghcommon reference to include Playwright E2E test support.
This will run 141 E2E tests on every push/PR to web/**
"
git push
```

---

## Expected Behavior

### Before Patch
```
Frontend CI
├── Install dependencies
├── Lint frontend code
├── Build frontend
└── Test frontend (vitest unit tests only)
```

### After Patch
```
Frontend CI
├── Install dependencies
├── Lint frontend code
├── Build frontend
├── Test frontend (vitest unit tests)
├── Install Playwright browsers  ← NEW
├── Run E2E tests               ← NEW (141 tests)
├── Upload Playwright report    ← NEW (on failure/always)
└── Upload test results         ← NEW (on failure/always)
```

---

## Verification

After CI runs, verify:

1. **GitHub Actions UI** shows:
   - ✅ Install Playwright browsers (passed)
   - ✅ Run E2E tests (passed, ~5-8 minutes)
   - ✅ Upload Playwright report (artifacts available)

2. **Artifacts** uploaded:
   - `playwright-report-ubuntu-latest-node-22` (HTML report)
   - `playwright-results-ubuntu-latest-node-22` (test results)

3. **CI fails** if any E2E test fails (continue-on-error: false)

---

## Rollback Plan

If E2E tests cause CI issues:

1. **Quick disable**: Pass `skip-e2e-tests: true` in frontend-ci.yml
   ```yaml
   uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@main
   with:
     node-version: '22'
     skip-e2e-tests: true  ← Add this
   ```

2. **Revert ghcommon**: Revert to previous commit
   ```bash
   cd ghcommon
   git revert <commit-sha>
   git push
   ```

3. **Fix issues locally** before re-enabling

---

## Notes

- **Performance**: E2E tests add 5-8 minutes to CI time (estimated)
- **Cost**: Playwright browsers ~500MB download, happens once per CI run
- **Reliability**: Tests should be stable (current: 141 tests, 92% coverage)
- **Artifacts**: Playwright reports retained for 30 days (helps debugging)

---

**Created**: 2026-01-26
**Status**: Ready for implementation
**Priority**: High (required for MVP release confidence)
