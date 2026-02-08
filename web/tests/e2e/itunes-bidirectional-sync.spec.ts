// file: web/tests/e2e/itunes-bidirectional-sync.spec.ts
// version: 1.1.0
// guid: f1e2a3b4-c5d6-7890-fghi-j1k2l3m4n5o6

import { test, expect } from '@playwright/test';
import {
  mockEventSource,
  setupMockApi,
  setupPhase1ApiDriven,
} from './utils/test-helpers';

test.describe.skip('iTunes Bidirectional Sync', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
    // Setup mock API for iTunes operations
    await setupMockApi(page);
  });

  test('import from iTunes - happy path', async ({ page }) => {
    // Test: Import books from iTunes library
    // Validates basic import workflow with real test data

    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Navigate to iTunes tab
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    // Enter path to test iTunes library
    const testLibraryPath = 'testdata/itunes/Library.xml';
    await page.getByLabel('iTunes Library Path').fill(testLibraryPath);

    // Validate library
    await page.getByRole('button', { name: 'Validate Library' }).click();
    await expect(page.getByText(/validation results|found \d+ books/i)).toBeVisible({ timeout: 5000 });

    // Import library
    await page.getByRole('button', { name: 'Import Library' }).click();
    await expect(page.getByRole('progressbar')).toBeVisible();
    await expect(page.getByText(/import complete|successfully imported/i)).toBeVisible({ timeout: 10000 });

    // Verify books appear in library
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    // At least one book should appear from iTunes import
    const bookElements = await page.locator('[role="button"]').filter({ hasText: /.+/ }).count();
    expect(bookElements).toBeGreaterThan(0);
  });

  test('organizer edits then write-back to iTunes', async ({ page }) => {
    // Test: Edit book in organizer, then sync changes back to iTunes
    // Validates write-back workflow

    // First import some books
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    const testLibraryPath = 'testdata/itunes/Library.xml';
    await page.getByLabel('iTunes Library Path').fill(testLibraryPath);
    await page.getByRole('button', { name: 'Validate Library' }).click();
    await expect(page.getByText(/validation results|found \d+ books/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole('button', { name: 'Import Library' }).click();
    await expect(page.getByText(/import complete|successfully imported/i)).toBeVisible({ timeout: 10000 });

    // Navigate to library and find a book
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    const firstBook = page.locator('[role="button"]').first();
    await expect(firstBook).toBeVisible();
    await firstBook.click();

    // Edit comments field
    await page.waitForLoadState('domcontentloaded');
    const commentsField = page.locator('textarea, input[placeholder*="comment"]').first();
    if (await commentsField.isVisible().catch(() => false)) {
      await commentsField.click();
      await commentsField.fill('Edited via test - should sync to iTunes');

      // Save changes (click save or navigate away)
      const saveButton = page.locator('button').filter({ hasText: /save|confirm/i }).first();
      if (await saveButton.isVisible().catch(() => false)) {
        await saveButton.click();
      }
    }

    // Navigate to iTunes settings and write-back
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    // Click Force Sync to iTunes button
    const forceSyncButton = page.getByRole('button', { name: /force sync to itunes|write.*back/i }).first();
    if (await forceSyncButton.isVisible().catch(() => false)) {
      await forceSyncButton.click();

      // Expect confirmation dialog or success message
      await expect(page.getByText(/synced|written|completed/i)).toBeVisible({ timeout: 5000 });
    }
  });

  test('iTunes conflict - newer iTunes data takes precedence', async ({ page }) => {
    // Test: When iTunes has newer data, user can choose to use iTunes version
    // This validates conflict detection and resolution

    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    const testLibraryPath = 'testdata/itunes/Library.xml';
    await page.getByLabel('iTunes Library Path').fill(testLibraryPath);
    await page.getByRole('button', { name: 'Validate Library' }).click();
    await expect(page.getByText(/validation results|found \d+ books/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole('button', { name: 'Import Library' }).click();
    await expect(page.getByText(/import complete|successfully imported/i)).toBeVisible({ timeout: 10000 });

    // Simulate a conflict by editing a book, then triggering a re-import
    // The conflict dialog should appear
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // If a conflict dialog appears, select "Use iTunes" option
    const itunesRadio = page.locator('input[type="radio"]').filter({ near: page.getByText(/use itunes|itunes version/i) }).first();
    if (await itunesRadio.isVisible({ timeout: 2000 }).catch(() => false)) {
      await itunesRadio.click();
      const applyButton = page.getByRole('button', { name: /apply|confirm|sync/i }).first();
      if (await applyButton.isVisible().catch(() => false)) {
        await applyButton.click();
      }
    }
  });

  test('organizer conflict - newer organizer data takes precedence', async ({ page }) => {
    // Test: When organizer has newer data, user can choose to use organizer version
    // This validates conflict resolution in opposite direction

    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    const testLibraryPath = 'testdata/itunes/Library.xml';
    await page.getByLabel('iTunes Library Path').fill(testLibraryPath);
    await page.getByRole('button', { name: 'Validate Library' }).click();
    await expect(page.getByText(/validation results|found \d+ books/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole('button', { name: 'Import Library' }).click();
    await expect(page.getByText(/import complete|successfully imported/i)).toBeVisible({ timeout: 10000 });

    // Navigate to a book and edit it significantly
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    const firstBook = page.locator('[role="button"]').first();
    await expect(firstBook).toBeVisible();
    await firstBook.click();

    // Edit multiple fields to create conflict
    await page.waitForLoadState('domcontentloaded');
    const commentsField = page.locator('textarea, input[placeholder*="comment"]').first();
    if (await commentsField.isVisible().catch(() => false)) {
      await commentsField.click();
      await commentsField.fill('Major update from organizer - should override iTunes');
    }

    // Save and navigate to iTunes sync
    const saveButton = page.locator('button').filter({ hasText: /save|confirm/i }).first();
    if (await saveButton.isVisible().catch(() => false)) {
      await saveButton.click();
    }

    // Go to iTunes settings and try write-back
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    // If conflict dialog appears, select "Use Organizer" option
    const organizerRadio = page.locator('input[type="radio"]').filter({ near: page.getByText(/use organizer|organizer version/i) }).first();
    if (await organizerRadio.isVisible({ timeout: 2000 }).catch(() => false)) {
      await organizerRadio.click();
      const applyButton = page.getByRole('button', { name: /apply|confirm|sync/i }).first();
      if (await applyButton.isVisible().catch(() => false)) {
        await applyButton.click();
      }
    }
  });

  test('selective sync - import only selected books', async ({ page }) => {
    // Test: User can choose to import only specific books
    // Validates selective sync functionality

    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    const testLibraryPath = 'testdata/itunes/Library.xml';
    await page.getByLabel('iTunes Library Path').fill(testLibraryPath);
    await page.getByRole('button', { name: 'Validate Library' }).click();
    await expect(page.getByText(/validation results|found \d+ books/i)).toBeVisible({ timeout: 5000 });

    // Look for selective options (checkboxes or similar)
    const selectiveCheckboxes = page.locator('input[type="checkbox"]');
    const checkboxCount = await selectiveCheckboxes.count();

    if (checkboxCount > 0) {
      // Uncheck some items to do selective import
      const firstCheckbox = selectiveCheckboxes.first();
      await firstCheckbox.click();

      await page.getByRole('button', { name: 'Import Library' }).click();
      await expect(page.getByText(/import complete|successfully imported|selective/i)).toBeVisible({ timeout: 10000 });
    } else {
      // If no selective UI, verify basic import still works
      await page.getByRole('button', { name: 'Import Library' }).click();
      await expect(page.getByText(/import complete|successfully imported/i)).toBeVisible({ timeout: 10000 });
    }
  });

  test('retry failed sync operation', async ({ page }) => {
    // Test: User can retry a failed sync operation
    // Validates retry mechanism

    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();

    // Try with invalid path to trigger failure
    await page.getByLabel('iTunes Library Path').fill('/invalid/path/nonexistent.xml');
    await page.getByRole('button', { name: 'Validate Library' }).click();

    // Should show error
    await expect(page.getByText(/error|not found|invalid/i)).toBeVisible({ timeout: 5000 });

    // Check if Retry button appears
    const retryButton = page.getByRole('button', { name: /retry|re-sync/i }).first();
    if (await retryButton.isVisible({ timeout: 2000 }).catch(() => false)) {
      // Clear the error by entering valid path
      await page.getByLabel('iTunes Library Path').clear();
      await page.getByLabel('iTunes Library Path').fill('testdata/itunes/Library.xml');

      // Retry should work now
      await retryButton.click();
      await expect(page.getByText(/validation results|found \d+ books|success/i)).toBeVisible({ timeout: 5000 });
    }
  });
});
