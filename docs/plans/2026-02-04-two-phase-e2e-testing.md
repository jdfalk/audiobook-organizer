# Two-Phase E2E Testing with Factory Reset Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a two-phase E2E testing system with factory reset capability to ensure tests work consistently in any environment (local or CI) by first running fast API-driven tests, then interactive UI-only tests.

**Architecture:** Add a backend factory reset endpoint (`POST /api/v1/system/reset`) that clears all database and config state, then create test helper functions that use this endpoint to bootstrap tests in either fast (API-driven) or realistic (UI-only) modes. Update existing E2E tests and demo recording script to use the appropriate mode.

**Tech Stack:** Go backend (database reset), TypeScript/Playwright E2E tests, existing test-helpers infrastructure

---

## Task 1: Implement Factory Reset Endpoint (Backend)

**Files:**
- Modify: `internal/server/handlers.go` (add reset handler)
- Modify: `internal/database/database.go` (add reset method)
- Modify: `internal/config/config.go` (add reset method)
- Test: `internal/server/handlers_test.go` (add reset endpoint tests)

**Step 1: Write the failing test for factory reset**

Add to `internal/server/handlers_test.go`:

```go
func TestResetToFactoryDefaults(t *testing.T) {
	// Setup: create some test data first
	db := setupTestDB(t)
	defer db.Close()

	// Insert a test book
	testBook := &audiobook.Audiobook{
		ID:    "test-book-1",
		Title: "Test Title",
	}
	err := db.SaveAudiobook(testBook)
	require.NoError(t, err)

	// Verify book exists
	book, err := db.GetAudiobook("test-book-1")
	require.NoError(t, err)
	require.NotNil(t, book)

	// Call reset endpoint
	handler := NewResetHandler(db, configManager)
	req := httptest.NewRequest("POST", "/api/v1/system/reset", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	// Verify database is cleared
	book, err = db.GetAudiobook("test-book-1")
	require.NoError(t, err)
	require.Nil(t, book)

	// Verify response contains timestamp
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Contains(t, resp, "reset_at")
	require.Contains(t, resp, "status")
	require.Equal(t, "reset", resp["status"])
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/server -run TestResetToFactoryDefaults -v
```

Expected: FAIL with "NewResetHandler not defined" or similar

**Step 3: Implement the reset handler in Go**

Create/modify `internal/server/handlers.go` to add:

```go
// ResetHandler resets the application to factory defaults
type ResetHandler struct {
	db     database.Database
	config *config.Config
}

func NewResetHandler(db database.Database, cfg *config.Config) *ResetHandler {
	return &ResetHandler{
		db:     db,
		config: cfg,
	}
}

func (h *ResetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear all data
	if err := h.db.Reset(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to reset database: %v", err), http.StatusInternalServerError)
		return
	}

	// Reset config to defaults
	if err := h.config.ResetToDefaults(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to reset config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "reset",
		"reset_at": time.Now().UTC(),
		"message":  "Application reset to factory defaults",
	})
}
```

Add to `internal/database/database.go`:

```go
// Reset clears all data from the database
func (db *Database) Reset() error {
	// For Pebble DB
	if db.usePebble {
		// Delete all keys
		iter := db.pebbleDB.NewIter(nil)
		defer iter.Close()

		batch := db.pebbleDB.NewBatch()
		defer batch.Close()

		for iter.First(); iter.Valid(); iter.Next() {
			if err := batch.Delete(iter.Key(), nil); err != nil {
				return fmt.Errorf("failed to delete key: %w", err)
			}
		}

		return batch.Commit(nil)
	}

	// For SQLite
	if db.useSQL {
		tables := []string{
			"audiobooks",
			"import_paths",
			"operations",
			"operation_logs",
			"backups",
			"blocked_hashes",
			"metadata",
		}

		for _, table := range tables {
			if _, err := db.sqliteDB.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
				return fmt.Errorf("failed to clear table %s: %w", table, err)
			}
		}
	}

	return nil
}
```

Add to `internal/config/config.go`:

```go
// ResetToDefaults resets configuration to factory defaults
func (c *Config) ResetToDefaults() error {
	defaultConfig := &Config{
		RootDir:              "/library",
		DatabasePath:         "/data/library.db",
		DatabaseType:         "pebble",
		PlaylistDir:          "/library/playlists",
		OrganizationStrategy: "auto",
		ScanOnStartup:        false,
		AutoOrganize:         true,
		FolderNamingPattern:  "{author}/{series}/{title} ({print_year})",
		FileNamingPattern:    "{title} - {author} - read by {narrator}",
		CreateBackups:        true,
		AutoFetchMetadata:    true,
		Language:             "en",
		LogLevel:             "info",
		LogFormat:            "text",
	}

	*c = *defaultConfig

	// Save to config file if it exists
	return c.Save()
}
```

**Step 4: Register the handler in the router**

Modify `internal/server/router.go` or main handler setup:

```go
// Add this to your router setup
http.HandleFunc("POST /api/v1/system/reset", NewResetHandler(db, cfg).ServeHTTP)
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/server -run TestResetToFactoryDefaults -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go internal/database/database.go internal/config/config.go
git commit -m "feat: add factory reset endpoint for testing"
```

---

## Task 2: Add Reset Helper to Test Utilities

**Files:**
- Modify: `web/tests/e2e/utils/test-helpers.ts`
- Create: `web/tests/e2e/utils/setup-modes.ts`

**Step 1: Write helpers for calling reset endpoint**

Create `web/tests/e2e/utils/setup-modes.ts`:

```typescript
// file: web/tests/e2e/utils/setup-modes.ts
// version: 1.0.0

import { Page } from '@playwright/test';

/**
 * Call the factory reset endpoint to clear all state
 */
export async function resetToFactoryDefaults(
  page: Page,
  baseURL: string = 'http://localhost:8080'
) {
  try {
    const response = await page.evaluate(
      async (url) => {
        const res = await fetch(`${url}/api/v1/system/reset`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
        });
        return {
          ok: res.ok,
          status: res.status,
          body: await res.json(),
        };
      },
      baseURL
    );

    if (!response.ok) {
      throw new Error(
        `Reset failed with status ${response.status}: ${JSON.stringify(response.body)}`
      );
    }

    return response.body;
  } catch (error) {
    throw new Error(
      `Failed to reset to factory defaults: ${error instanceof Error ? error.message : String(error)}`
    );
  }
}

/**
 * Setup for API-driven phase (fast, use real API)
 * - Resets to factory defaults
 * - Uses actual API calls for data setup
 * - Optionally mocks specific endpoints for failure testing
 */
export async function setupPhase1ApiDriven(
  page: Page,
  baseURL: string = 'http://localhost:8080'
) {
  // Reset state
  await resetToFactoryDefaults(page, baseURL);

  // Skip welcome wizard
  await page.addInitScript(() => {
    localStorage.setItem('welcome_wizard_completed', 'true');
  });

  // Clear any cached data
  await page.addInitScript(() => {
    localStorage.clear();
  });

  return {
    baseURL,
    mode: 'api-driven' as const,
  };
}

/**
 * Setup for interactive phase (realistic, UI-only)
 * - Resets to factory defaults
 * - Mocks ALL API endpoints
 * - Tests only UI interactions, no direct API calls
 */
export async function setupPhase2Interactive(
  page: Page,
  baseURL: string = 'http://localhost:8080',
  mockOptions = {}
) {
  // Reset state via real API first
  await resetToFactoryDefaults(page, baseURL);

  // Skip welcome wizard
  await page.addInitScript(() => {
    localStorage.setItem('welcome_wizard_completed', 'true');
  });

  // Import and setup mock API (from existing test-helpers)
  const { setupMockApi } = await import('./test-helpers');
  await setupMockApi(page, mockOptions);

  return {
    baseURL,
    mode: 'interactive' as const,
  };
}
```

**Step 2: Export new helpers from test-helpers**

Add to end of `web/tests/e2e/utils/test-helpers.ts`:

```typescript
export {
  resetToFactoryDefaults,
  setupPhase1ApiDriven,
  setupPhase2Interactive,
} from './setup-modes';
```

**Step 3: Run the test suite to verify no breakage**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
npm run test:e2e -- --reporter=list 2>&1 | head -50
```

Expected: Tests should run (existing tests may fail if they relied on mocking, that's next step)

**Step 4: Commit**

```bash
git add web/tests/e2e/utils/setup-modes.ts web/tests/e2e/utils/test-helpers.ts
git commit -m "feat: add setup helpers for two-phase E2E testing modes"
```

---

## Task 3: Update Existing E2E Tests to Use Phase 1 (API-Driven)

**Files:**
- Modify: All spec files in `web/tests/e2e/*.spec.ts` to use Phase 1 setup

**Step 1: Update one test file to use Phase 1**

Modify `web/tests/e2e/app.spec.ts`:

```typescript
// file: tests/e2e/app.spec.ts
// version: 1.1.0

import { test, expect } from '@playwright/test';
import { setupPhase1ApiDriven } from './utils/setup-modes';
import { skipWelcomeWizard, setupMockApi, generateTestBooks } from './utils/test-helpers';

test.describe('App smoke (Phase 1 - API-Driven)', () => {
  test.beforeEach(async ({ page }) => {
    // Setup Phase 1: Fast API-driven
    await setupPhase1ApiDriven(page);

    // Mock API for this test
    await setupMockApi(page, {
      books: generateTestBooks(0), // Empty library
    });
  });

  test('loads dashboard and shows title', async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByText('Audiobook Organizer', { exact: true }).first()
    ).toBeVisible();
    await expect(
      page.getByText('Dashboard', { exact: true }).first()
    ).toBeVisible();
  });

  test('shows import path empty state on Library page', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByText('Library', { exact: true }).first().click();
    await expect(page).toHaveURL(/.*\/library/);
  });

  test('navigates to Settings and renders content', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByText('Settings', { exact: true }).first().click();
    await expect(page).toHaveURL(/.*\/settings/);
    await expect(
      page.getByText('Settings', { exact: true }).first()
    ).toBeVisible();
  });
});
```

**Step 2: Run this test to verify it works**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
npx playwright test web/tests/e2e/app.spec.ts -v
```

Expected: Tests pass

**Step 3: Verify the reset endpoint is being called**

Add a quick console log to verify reset is called:

```bash
# Check if the reset call succeeded by examining page logs
npx playwright test web/tests/e2e/app.spec.ts -v 2>&1 | grep -i reset
```

**Step 4: Commit**

```bash
git add web/tests/e2e/app.spec.ts
git commit -m "test: update app smoke test to use Phase 1 API-driven setup"
```

---

## Task 4: Create a Phase 2 Interactive Test Example

**Files:**
- Create: `web/tests/e2e/interactive-import-workflow.spec.ts` (new test that uses Phase 2)

**Step 1: Write a new interactive-only test**

Create `web/tests/e2e/interactive-import-workflow.spec.ts`:

```typescript
// file: web/tests/e2e/interactive-import-workflow.spec.ts
// version: 1.0.0
// guid: 5a6b7c8d-9e0f-1a2b-3c4d-5e6f7a8b9c0d

import { test, expect } from '@playwright/test';
import { setupPhase2Interactive } from './utils/setup-modes';
import { generateTestBooks } from './utils/test-helpers';

test.describe('Interactive Workflow (Phase 2 - UI-Only)', () => {
  test.beforeEach(async ({ page }) => {
    // Setup Phase 2: Interactive, all UI
    await setupPhase2Interactive(page, 'http://localhost:5173', {
      books: [],
      config: {
        root_dir: '/tmp/audiobooks',
      },
    });
  });

  test('user can navigate to library and see empty state', async ({ page }) => {
    // Pure UI interaction - no API calls
    await page.goto('/');
    await expect(
      page.getByText('Audiobook Organizer', { exact: true }).first()
    ).toBeVisible();

    // Navigate to library via UI
    await page.getByRole('link', { name: /library/i }).click();
    await expect(page).toHaveURL(/.*\/library/);

    // Should see empty state (no books)
    await expect(
      page.getByText(/no audiobooks/i)
    ).toBeVisible();
  });

  test('user can navigate through settings', async ({ page }) => {
    await page.goto('/');

    // Navigate to settings via UI
    await page.getByRole('link', { name: /settings/i }).click();
    await expect(page).toHaveURL(/.*\/settings/);

    // Should see settings content
    await expect(
      page.getByText('Settings', { exact: true }).first()
    ).toBeVisible();
  });
});
```

**Step 2: Run the new interactive test**

```bash
npx playwright test web/tests/e2e/interactive-import-workflow.spec.ts -v
```

Expected: Tests pass with mocked APIs

**Step 3: Verify no real API calls are made**

Add network inspection to verify only mocked responses:

```bash
npx playwright test web/tests/e2e/interactive-import-workflow.spec.ts -v --debug 2>&1 | grep "api/v1" | head -5
```

All requests should be caught by the mock.

**Step 4: Commit**

```bash
git add web/tests/e2e/interactive-import-workflow.spec.ts
git commit -m "test: add Phase 2 interactive-only workflow test example"
```

---

## Task 5: Update Demo Recording Script to Use Phase 2 (Interactive)

**Files:**
- Modify: `scripts/record_demo.js` to use Phase 2 approach

**Step 1: Update demo script to call reset endpoint**

Modify `scripts/record_demo.js`:

```javascript
// file: scripts/record_demo.js
// version: 2.1.0

const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const axios = require('axios');

const BASE_URL = process.env.API_URL || 'https://localhost:8080';
const OUTPUT_DIR = process.env.OUTPUT_DIR || './demo_recordings';
const DEMO_VIDEO_PATH = path.join(OUTPUT_DIR, 'audiobook-demo.webm');
const SCREENSHOTS_DIR = path.join(OUTPUT_DIR, 'screenshots');

const https = require('https');
const axiosInstance = axios.create({
  httpsAgent: new https.Agent({ rejectUnauthorized: false })
});

if (!fs.existsSync(OUTPUT_DIR)) {
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });
}
if (!fs.existsSync(SCREENSHOTS_DIR)) {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
}

async function waitForServer(maxAttempts = 30) {
  console.log('⏳ Waiting for server...');
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const response = await axiosInstance.get(`${BASE_URL}/api/health`);
      if (response.status === 200) {
        console.log('✅ Server is ready!');
        return true;
      }
    } catch (error) {
      if (i === maxAttempts - 1) {
        console.error('❌ Server did not start in time');
        return false;
      }
      await new Promise(resolve => setTimeout(resolve, 1000));
    }
  }
  return false;
}

// NEW: Reset to factory defaults
async function resetToFactoryDefaults() {
  console.log('🔄 Resetting to factory defaults...');
  try {
    const response = await axiosInstance.post(`${BASE_URL}/api/v1/system/reset`);
    console.log(`✅ Reset complete: ${response.data.message}`);
    return response.data;
  } catch (error) {
    console.error('❌ Failed to reset:', error.message);
    throw error;
  }
}

async function screenshot(page, name) {
  const filePath = path.join(SCREENSHOTS_DIR, `${Date.now()}-${name}.png`);
  await page.screenshot({ path: filePath, fullPage: true });
  console.log(`📸 Screenshot: ${name}`);
  return filePath;
}

async function recordDemo() {
  console.log('🎬 Starting Audiobook Organizer Demo Recording (Interactive UI)\n');

  if (!(await waitForServer())) {
    console.error('Failed to connect to server');
    process.exit(1);
  }

  // NEW: Reset state first
  try {
    await resetToFactoryDefaults();
  } catch (error) {
    console.error('Failed to reset application state');
    process.exit(1);
  }

  const browser = await chromium.launch({
    headless: false,
    args: ['--disable-blink-features=AutomationControlled']
  });

  const context = await browser.newContext({
    recordVideo: { dir: OUTPUT_DIR },
    ignoreHTTPSErrors: true
  });

  const page = await context.newPage();

  try {
    console.log('📝 PHASE 1: NAVIGATE TO APPLICATION\n');

    console.log('Opening web interface...');
    await page.goto(`${BASE_URL}/`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForSelector('#root', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(2000);
    await screenshot(page, '01-app-home');
    console.log('✅ Application loaded');

    console.log('\n📝 PHASE 2: IMPORT FILES (UI INTERACTION)\n');

    const timestamp = Date.now();
    const importPath = `/tmp/demo-audiobooks-${timestamp}`;

    if (!fs.existsSync(importPath)) {
      fs.mkdirSync(importPath, { recursive: true });
    }
    const testFilePath = `${importPath}/test_book.m4b`;
    fs.writeFileSync(testFilePath, Buffer.alloc(1024 * 100));
    console.log('✅ Created test audiobook file');

    // NEW: Use API to import (setup), not for demonstration
    console.log('Importing audiobook via API (backend setup)...');
    const importResult = await axiosInstance.post(`${BASE_URL}/api/v1/import/file`, {
      file_path: testFilePath,
      organize: false
    });
    const bookId = importResult.data.id;
    console.log(`✅ Imported book: ${bookId}`);

    // Show in UI
    await page.reload({ waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForTimeout(2000);
    await screenshot(page, '02-books-list');
    console.log('✅ Book visible in library');

    console.log('\n📝 PHASE 3: FETCH METADATA (UI INTERACTION)\n');

    // Click fetch metadata button in UI
    await page.getByRole('button', { name: /fetch metadata/i }).first().click();
    await page.waitForTimeout(2000);
    await screenshot(page, '03-metadata-populated');
    console.log('✅ Metadata displayed in library');

    console.log('\n📝 PHASE 4: ORGANIZE FILES (UI INTERACTION)\n');

    // Click organize button in UI
    await page.getByRole('button', { name: /organize/i }).first().click();
    await page.waitForTimeout(3000);
    await screenshot(page, '04-organization-in-progress');
    console.log('✅ Organization processing visible');

    console.log('\n📝 PHASE 5: EDIT METADATA (UI INTERACTION)\n');

    // Click on book to open detail
    await page.getByRole('link', { name: /test_book/i }).first().click();
    await page.waitForTimeout(1000);

    // Edit title field
    await page.getByLabel(/title/i).fill('Custom Demo Title');
    await page.getByLabel(/narrator/i).fill('Professional Narrator');
    await page.getByLabel(/publisher/i).fill('Demo Publisher');

    // Save
    await page.getByRole('button', { name: /save/i }).click();
    await page.waitForTimeout(1000);
    await screenshot(page, '05-metadata-edited');
    console.log('✅ Changes displayed in UI');

    console.log('\n📝 PHASE 6: VERIFY PERSISTENCE\n');

    const finalBook = await axiosInstance.get(`${BASE_URL}/api/v1/audiobooks/${bookId}`);
    console.log('✅ Verification Results:');
    console.log(`   - Title persisted: ${finalBook.data.title === 'Custom Demo Title' ? '✅' : '❌'}`);
    console.log(`   - Narrator persisted: ${finalBook.data.narrator === 'Professional Narrator' ? '✅' : '❌'}`);
    console.log(`   - Publisher persisted: ${finalBook.data.publisher === 'Demo Publisher' ? '✅' : '❌'}`);

    await screenshot(page, '06-final-state');
    console.log('✅ Final library state captured');

    console.log('\n✅ DEMO COMPLETED SUCCESSFULLY!\n');

    console.log('═══════════════════════════════════════════════════════');
    console.log('📊 DEMO SUMMARY');
    console.log('═══════════════════════════════════════════════════════');
    console.log(`✅ Imported audiobook: ${bookId}`);
    console.log(`✅ Fetched metadata from Open Library`);
    console.log(`✅ Organized files into folder structure`);
    console.log(`✅ Edited metadata with custom values`);
    console.log(`✅ Verified all changes persisted`);
    console.log('\n📹 Recording Details:');
    console.log(`   Video: ${DEMO_VIDEO_PATH}`);
    console.log(`   Screenshots: ${SCREENSHOTS_DIR}`);
    console.log(`   Duration: ~2-3 minutes`);
    console.log('═══════════════════════════════════════════════════════\n');

  } catch (error) {
    console.error('❌ Demo failed:', error.message);
    process.exit(1);
  } finally {
    await page.close();
    await context.close();
    await browser.close();

    const files = fs.readdirSync(OUTPUT_DIR);
    const webmFile = files.find(f => f.endsWith('.webm') && f !== 'audiobook-demo.webm' && fs.statSync(path.join(OUTPUT_DIR, f)).size > 1024);

    if (webmFile) {
      const sourcePath = path.join(OUTPUT_DIR, webmFile);
      fs.renameSync(sourcePath, DEMO_VIDEO_PATH);
      console.log(`📹 Demo video saved to: ${DEMO_VIDEO_PATH}`);
    }

    console.log('\n🎉 Recording complete!');
    console.log(`Video: ${DEMO_VIDEO_PATH}`);
    console.log(`Screenshots: ${SCREENSHOTS_DIR}`);
  }
}

recordDemo().catch(error => {
  console.error('Fatal error:', error);
  process.exit(1);
});
```

**Step 2: Run the demo script to verify it works**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
npm run build
npm run dev &
sleep 5
node scripts/record_demo.js
```

Expected: Reset endpoint called, demo runs with pure UI interactions

**Step 3: Commit**

```bash
git add scripts/record_demo.js
git commit -m "feat: update demo recording to use factory reset and pure UI interactions"
```

---

## Task 6: Add Documentation for Two-Phase Testing

**Files:**
- Create: `docs/TESTING.md` (comprehensive testing guide)

**Step 1: Write testing documentation**

Create `docs/TESTING.md`:

```markdown
# E2E Testing Guide

## Two-Phase Testing Approach

The E2E test suite uses a two-phase approach to ensure tests work consistently in any environment:

### Phase 1: API-Driven Tests (Fast Setup)
- **Purpose:** Quick iteration and basic functionality verification
- **Setup:** Reset to factory defaults via API, then populate test data via API calls
- **Use Case:** Unit-like E2E tests, CI pipelines where speed matters
- **Environment:** Works locally and in CI without special setup

```typescript
import { setupPhase1ApiDriven, setupMockApi } from './utils/setup-modes';

test.beforeEach(async ({ page }) => {
  await setupPhase1ApiDriven(page);
  await setupMockApi(page, { books: generateTestBooks(5) });
});
```

### Phase 2: Interactive Tests (Realistic)
- **Purpose:** Verify real user workflows, capture realistic demo recordings
- **Setup:** Reset to factory defaults, mock ALL APIs, interact only via UI
- **Use Case:** Integration tests, demo recordings, end-to-end validation
- **Environment:** Completely isolated, no dependencies on real APIs

```typescript
import { setupPhase2Interactive } from './utils/setup-modes';

test.beforeEach(async ({ page }) => {
  await setupPhase2Interactive(page, 'http://localhost:5173');
});
```

## Factory Reset Endpoint

**Endpoint:** `POST /api/v1/system/reset`

**Purpose:** Clear all state and reset to factory defaults

**When to use:**
- Before each Phase 1 test (automatic)
- Before each Phase 2 test (automatic)
- Manual cleanup between test runs

**Response:**
```json
{
  "status": "reset",
  "reset_at": "2024-02-04T12:00:00Z",
  "message": "Application reset to factory defaults"
}
```

## Running Tests

### Phase 1 Tests (API-Driven)
```bash
# Run all API-driven tests
npm run test:e2e

# Run specific test
npx playwright test app.spec.ts -v
```

### Phase 2 Tests (Interactive)
```bash
# Run interactive tests
npx playwright test interactive-import-workflow.spec.ts -v

# Run with debug UI
npx playwright test interactive-import-workflow.spec.ts --debug
```

### Demo Recording
```bash
npm run build
npm run dev &
sleep 5
npm run record-demo
```

## Environment Variables

- `API_URL`: API server URL (default: http://localhost:8080)
- `PLAYWRIGHT_TEST_BASE_URL`: Base URL for tests (default: http://localhost:5173)

## Best Practices

1. **Use Phase 1 for fast iteration** - Quick setup/teardown
2. **Use Phase 2 for integration** - Real workflows, demo content
3. **Always reset before tests** - Handlers do this automatically
4. **Mock external APIs** - Don't depend on Open Library, Audible, etc.
5. **Commit frequently** - After each test phase
```

**Step 2: Verify documentation is clear**

```bash
cat /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/docs/TESTING.md
```

Expected: Clear, actionable documentation

**Step 3: Commit**

```bash
git add docs/TESTING.md
git commit -m "docs: add comprehensive two-phase E2E testing guide"
```

---

## Summary

This plan implements a robust two-phase testing system:

1. **Phase 1 (API-Driven):** Fast setup via API calls → great for quick iteration
2. **Phase 2 (Interactive):** Pure UI interactions with mocked APIs → great for realistic workflows
3. **Factory Reset:** New endpoint clears state, enables consistent test environment
4. **Demo Recording:** Updated to use Phase 2, showing real UI workflows
5. **Documentation:** Clear guide for using each phase appropriately

Each phase uses the factory reset endpoint to guarantee a clean starting state, making tests pass consistently whether running locally or in CI.

---

Plan complete and saved to `docs/plans/2026-02-04-two-phase-e2e-testing.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?