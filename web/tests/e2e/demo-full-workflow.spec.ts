// file: web/tests/e2e/demo-full-workflow.spec.ts
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { test, expect } from '@playwright/test';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { setupPhase2Interactive } from './utils/setup-modes';

// Consistent demo artifacts directory
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DEMO_ARTIFACTS_DIR = join(__dirname, '../../..', 'demo_artifacts');

test.describe('Full End-to-End Demo Workflow', () => {
  test('Complete audiobook workflow: from empty library to organized books', async ({ page }) => {
    // Increase timeout for this recording test (video capture takes time)
    test.setTimeout(60 * 1000); // 60 seconds for full demo capture

    // Setup Phase 2: Interactive mode with mocked APIs
    // This starts with an empty library and 30 mock books
    await setupPhase2Interactive(page, 'http://127.0.0.1:4173', {
      books: [], // Start with empty library
      config: {
        root_dir: '/library',
        auto_organize: false,
      },
    });

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
    // STEP 3: Simulate Adding Books (update mock data)
    // ==============================================
    console.log('=== STEP 3: Books Added to Library ===');

    // Inject books into the mock API
    await page.evaluate(() => {
      const mockBooks = Array.from({ length: 20 }, (_, i) => ({
        id: `book-${i + 1}`,
        title: `The ${['Hobbit', 'Fellowship', 'Two Towers', 'Return'][i % 4]}`,
        author_name: `J.R.R. Tolkien`,
        series_name: `The Lord of the Rings`,
        series_position: (i % 3) + 1,
        library_state: i % 2 === 0 ? 'organized' : 'import',
        marked_for_deletion: false,
        language: 'en',
        file_path: `/library/book-${i + 1}.m4b`,
        file_hash: `hash-${i + 1}`,
        original_file_hash: `orig-${i + 1}`,
        organized_file_hash: i % 2 === 0 ? `org-${i + 1}` : null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        duration: 36000 + i * 1000,
        file_size: 100000000 + i * 5000000,
        publisher: 'George Allen & Unwin',
        description: 'An epic fantasy adventure',
      }));

      if (window.__apiMock?.setBooks) {
        window.__apiMock.setBooks(mockBooks);
      }
    });

    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2000);

    // Screenshot library with books
    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_03_library_with_books.png`, fullPage: true });
    console.log('✓ Books now visible in library');
    await page.waitForTimeout(2000);

    // ==============================================
    // STEP 4: Scroll Through Library
    // ==============================================
    console.log('=== STEP 4: Browse Books List ===');
    await page.evaluate(() => window.scrollBy(0, 400));
    await page.waitForTimeout(1000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_04_library_scrolled.png`, fullPage: true });
    console.log('✓ Showing more books in list');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 5: Click a Book for Details
    // ==============================================
    console.log('=== STEP 5: Book Detail View ===');
    await page.evaluate(() => window.scrollBy(0, -400)); // Scroll back up
    await page.waitForTimeout(500);

    // Click first book to open detail
    const bookLink = page.locator('[data-testid*="book"], a >> text=/The/').first();
    if (await bookLink.isVisible().catch(() => false)) {
      await bookLink.click();
      await page.waitForURL(/.*\/books\/.*/);
      await page.waitForTimeout(2000);

      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_05_book_detail.png`, fullPage: true });
      console.log('✓ Book detail view displayed');
      await page.waitForTimeout(1500);

      // Scroll to show more details
      await page.evaluate(() => window.scrollBy(0, 300));
      await page.waitForTimeout(800);
      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_06_book_detail_metadata.png`, fullPage: true });
      console.log('✓ Metadata details visible');
      await page.waitForTimeout(1500);
    }

    // ==============================================
    // STEP 6: Back to Library
    // ==============================================
    console.log('=== STEP 6: Return to Library Overview ===');
    await page.goto('/library');
    await page.waitForTimeout(1500);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/full_demo_07_library_final.png`, fullPage: true });
    console.log('✓ Back to organized library view');
    await page.waitForTimeout(2000);

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
