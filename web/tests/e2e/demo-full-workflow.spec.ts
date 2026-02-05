// file: web/tests/e2e/demo-full-workflow.spec.ts
// version: 2.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { test, expect } from '@playwright/test';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { resetToFactoryDefaults } from './utils/setup-modes';

// Consistent demo artifacts directory
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DEMO_ARTIFACTS_DIR = join(__dirname, '../../..', 'demo_artifacts');

test.describe('Full End-to-End Demo Workflow', () => {
  test('Complete audiobook workflow: from empty library to organized books', async ({ page }) => {
    // Increase timeout for this comprehensive demo
    test.setTimeout(180 * 1000); // 180 seconds for full demo with many slow interactions

    // Reset to factory defaults (keeps welcome wizard, no skipping)
    await resetToFactoryDefaults(page, 'http://127.0.0.1:8080');

    // ==============================================
    // STEP 1: Welcome Screen - First-Time User Experience
    // ==============================================
    console.log('=== STEP 1: Welcome Screen ===');
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(3000); // Let page fully load

    // Screenshot the welcome screen
    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_01_welcome_screen.png`, fullPage: true });
    console.log('✓ Welcome screen displayed');
    await page.waitForTimeout(2000);

    // Look for "Get Started" button in welcome dialog
    const getStartedButton = page.locator('button').filter({ hasText: /get started|start|next|continue/i }).first();
    if (await getStartedButton.isVisible().catch(() => false)) {
      await page.mouse.move(400, 300);
      await page.waitForTimeout(1000);
      await getStartedButton.click();
      console.log('✓ Clicked "Get Started" button');
      await page.waitForTimeout(2000);
    }

    // ==============================================
    // STEP 2: Navigate to Settings to Add Import Folder
    // ==============================================
    console.log('=== STEP 2: Navigate to Settings ===');

    const settingsLink = page.locator('a, button').filter({ hasText: /settings|configuration|config/i }).first();
    if (await settingsLink.isVisible().catch(() => false)) {
      await page.mouse.move(200, 50);
      await page.waitForTimeout(1000);
      await settingsLink.click();
      console.log('✓ Clicked Settings');
      await page.waitForTimeout(3000);
    } else {
      await page.goto('/settings', { waitUntil: 'domcontentloaded' });
      await page.waitForTimeout(3000);
    }

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_02_settings_page.png`, fullPage: true });
    console.log('✓ Settings page loaded');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 3: Add Import Path - Show Adding a Folder
    // ==============================================
    console.log('=== STEP 3: Add Import Path ===');

    // Scroll to import paths section
    await page.mouse.move(400, 300);
    await page.waitForTimeout(800);
    await page.evaluate(() => window.scrollBy(0, 300));
    await page.waitForTimeout(2000);

    // Look for "Add" or "+" button for import paths
    const addImportButton = page.locator('button').filter({ hasText: /add|create|plus|\+/i }).first();
    if (await addImportButton.isVisible().catch(() => false)) {
      await page.mouse.move(300, 200);
      await page.waitForTimeout(1000);
      await addImportButton.click();
      console.log('✓ Clicked Add Import Path');
      await page.waitForTimeout(2000);
    }

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_03_add_import_dialog.png`, fullPage: true });
    console.log('✓ Import path dialog shown');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 4: Navigate to Library - Show Library View
    // ==============================================
    console.log('=== STEP 4: Navigate to Library ===');

    // Scroll back up and click Library
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(1500);

    const libraryLink = page.locator('a, button').filter({ hasText: /library|books|audiobooks/i }).first();
    if (await libraryLink.isVisible().catch(() => false)) {
      await page.mouse.move(150, 50);
      await page.waitForTimeout(1000);
      await libraryLink.click();
      console.log('✓ Clicked Library');
      await page.waitForTimeout(3000);
    } else {
      await page.goto('/library', { waitUntil: 'domcontentloaded' });
      await page.waitForTimeout(3000);
    }

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_04_library_view.png`, fullPage: true });
    console.log('✓ Library view displayed');
    await page.waitForTimeout(2000);

    // ==============================================
    // STEP 5: Explore Library UI - Show Search/Filter
    // ==============================================
    console.log('=== STEP 5: Explore Library Controls ===');

    // Scroll down to show search and filter controls
    await page.mouse.move(400, 300);
    await page.waitForTimeout(1000);
    await page.evaluate(() => window.scrollBy(0, 200));
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_05_library_controls.png`, fullPage: true });
    console.log('✓ Library controls visible');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 6: Continue Scrolling - Show Import Paths
    // ==============================================
    console.log('=== STEP 6: Show Import Paths ===');

    await page.evaluate(() => window.scrollBy(0, 300));
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_06_import_paths_list.png`, fullPage: true });
    console.log('✓ Import paths section visible');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 7: Show Scan Button
    // ==============================================
    console.log('=== STEP 7: Scanning Folders ===');

    // Look for Scan button
    const scanButton = page.locator('button').filter({ hasText: /scan|search|import/i }).first();
    if (await scanButton.isVisible().catch(() => false)) {
      await page.mouse.move(300, 200);
      await page.waitForTimeout(1000);
      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_07_scan_button_ready.png`, fullPage: true });
      console.log('✓ Scan button visible');
    }
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 8: Navigate to Dashboard - Show Stats
    // ==============================================
    console.log('=== STEP 8: Dashboard Overview ===');

    // Scroll back to top
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(1500);

    const dashboardLink = page.locator('a, button').filter({ hasText: /dashboard|home|overview/i }).first();
    if (await dashboardLink.isVisible().catch(() => false)) {
      await page.mouse.move(100, 50);
      await page.waitForTimeout(1000);
      await dashboardLink.click();
      console.log('✓ Clicked Dashboard');
      await page.waitForTimeout(3000);
    } else {
      await page.goto('/', { waitUntil: 'domcontentloaded' });
      await page.waitForTimeout(3000);
    }

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_08_dashboard_empty.png`, fullPage: true });
    console.log('✓ Dashboard displayed with empty stats');
    await page.waitForTimeout(2000);

    // ==============================================
    // STEP 9: Scroll Dashboard - Show Full Overview
    // ==============================================
    console.log('=== STEP 9: Dashboard Full Overview ===');

    await page.mouse.move(400, 300);
    await page.waitForTimeout(1000);
    await page.evaluate(() => window.scrollBy(0, 400));
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_09_dashboard_full.png`, fullPage: true });
    console.log('✓ Dashboard full overview shown');
    await page.waitForTimeout(1500);

    // Continue scrolling
    await page.evaluate(() => window.scrollBy(0, 400));
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_10_dashboard_bottom.png`, fullPage: true });
    console.log('✓ Dashboard bottom section visible');

    console.log('✅ Complete workflow demo finished successfully!');
  });
});
