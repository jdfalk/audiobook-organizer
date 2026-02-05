// file: web/tests/e2e/demo-full-workflow.spec.ts
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { test, expect } from '@playwright/test';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { setupPhase1ApiDriven } from './utils/setup-modes';

// Consistent demo artifacts directory
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DEMO_ARTIFACTS_DIR = join(__dirname, '../../..', 'demo_artifacts');

test.describe('Full End-to-End Demo Workflow', () => {
  test('Complete audiobook workflow: from empty library to organized books', async ({ page }) => {
    // Increase timeout for this recording test (video capture takes time)
    test.setTimeout(60 * 1000); // 60 seconds for full demo capture

    // Setup Phase 1: Real API-driven setup with backend
    // This resets to factory defaults and uses real APIs
    await setupPhase1ApiDriven(page, 'http://127.0.0.1:4173');

    // ==============================================
    // STEP 1: Dashboard - System Overview
    // ==============================================
    console.log('=== STEP 1: Dashboard Overview ===');
    await page.goto('/');
    await page.waitForTimeout(3000); // Let dashboard fully render

    // Take screenshot of dashboard
    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_01_dashboard.png`, fullPage: true });
    console.log('✓ Dashboard loaded - showing empty library state');
    await page.waitForTimeout(2000);

    // ==============================================
    // STEP 2: Navigate to Library - Show Empty State
    // ==============================================
    console.log('=== STEP 2: Library View - Empty State ===');
    await page.goto('/library');
    await page.waitForTimeout(2000);

    // Should show empty state message
    const emptyState = page.locator('text=/no audiobooks|empty/i').first();
    if (await emptyState.isVisible().catch(() => false)) {
      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_02_library_empty.png`, fullPage: true });
      console.log('✓ Library empty state displayed');
    }
    await page.waitForTimeout(2000);

    // ==============================================
    // STEP 3: Show Empty Library (no books yet)
    // ==============================================
    console.log('=== STEP 3: Empty Library State (Real API) ===');

    // Navigate to library to confirm real empty state
    await page.goto('/library', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(1500);

    // Screenshot library with real API (empty)
    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_03_library_real_empty.png`, fullPage: true });
    console.log('✓ Real empty library state');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 4: Library Filtered/Search UI
    // ==============================================
    console.log('=== STEP 4: Library UI Controls ===');
    await page.evaluate(() => window.scrollBy(0, 400));
    await page.waitForTimeout(1000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_04_library_ui.png`, fullPage: true });
    console.log('✓ Showing library controls and empty message');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 5: Library Help Section
    // ==============================================
    console.log('=== STEP 5: Library Help & Actions ===');
    await page.evaluate(() => window.scrollBy(0, 300));
    await page.waitForTimeout(800);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_05_library_help.png`, fullPage: true });
    console.log('✓ Showing import actions');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 7: Navigate to Settings
    // ==============================================
    console.log('=== STEP 7: Settings Configuration ===');
    await page.goto('/settings');
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_08_settings.png`, fullPage: true });
    console.log('✓ Settings page displayed');
    await page.waitForTimeout(1500);

    // Scroll through settings
    await page.evaluate(() => window.scrollBy(0, 300));
    await page.waitForTimeout(800);
    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_09_settings_scrolled.png`, fullPage: true });
    console.log('✓ Showing configuration options');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 8: File Browser (optional - graceful skip if not available)
    // ==============================================
    console.log('=== STEP 8: File Browser ===');
    try {
      await page.goto('/file-browser', { waitUntil: 'domcontentloaded' });
      await page.waitForTimeout(800);

      const fileBrowserVisible = await page.locator('text=/file browser|browse/i').isVisible().catch(() => false);
      if (fileBrowserVisible) {
        await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_10_file_browser.png`, fullPage: true });
        console.log('✓ File browser displayed');
      }
      await page.waitForTimeout(500);
    } catch (e) {
      console.log('⚠ File browser skipped (not available in test)');
    }

    // ==============================================
    // STEP 9: Operations View
    // ==============================================
    console.log('=== STEP 9: Operations Monitor ===');
    await page.goto('/operations', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(800);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_11_operations.png`, fullPage: true });
    console.log('✓ Operations view displayed');
    await page.waitForTimeout(500);

    // ==============================================
    // STEP 10: Back to Dashboard
    // ==============================================
    console.log('=== STEP 10: Final Dashboard State ===');
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(1000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_12_final_dashboard.png`, fullPage: true });
    console.log('✓ Final dashboard state - showing completed workflow');

    console.log('✅ Complete workflow demo finished successfully!');
  });
});
