// file: web/tests/e2e/dynamic-ui-interactions.spec.ts
// version: 1.2.0
// guid: 9f8e7d6c-5b4a-3210-fedc-ba9876543210
// last-edited: 2026-02-04

/**
 * E2E tests for dynamic UI interactions with in-place loading states
 * Tests the Sonarr/Radarr-style button spinners and smooth updates
 */

import { test, expect, type Page } from '@playwright/test';
import { setupMockApi, generateTestBooks } from './utils/test-helpers';

test.describe('Dynamic UI - BookDetail Page', () => {
  test.beforeEach(async ({ page }) => {
    // Set up base mock routes
    await setupMockApi(page);

    // Override audiobook routes with test-specific mocks (use ** to match sub-paths)
    await page.route('**/api/v1/audiobooks/**', async (route) => {
      const url = route.request().url();

      if (url.includes('/tags')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            media_info: {
              codec: 'aac',
              bitrate: 128,
              sample_rate: 44100,
              channels: 2,
            },
            tags: {
              title: {
                file_value: 'The Odyssey',
                fetched_value: 'The Odyssey: Homer',
                stored_value: 'The Odyssey',
                override_value: null,
                override_locked: false,
                effective_value: 'The Odyssey',
                effective_source: 'stored',
              },
              audiobook_release_year: {
                file_value: null,
                fetched_value: 2020,
                stored_value: null,
                override_value: null,
                override_locked: false,
                effective_value: null,
                effective_source: null,
              },
            },
          }),
        });
      } else if (url.includes('/fetch-metadata')) {
        // Simulate delay to test spinner (3s to ensure assertion catches disabled state)
        await new Promise((resolve) => setTimeout(resolve, 3000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            message: 'Metadata fetched successfully',
            source: 'Open Library',
            book: {
              id: 'test-book-id',
              title: 'The Odyssey: Homer',
              audiobook_release_year: 2020,
            },
          }),
        });
      } else if (url.includes('/parse-with-ai')) {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            message: 'AI parsing completed',
            book: {
              id: 'test-book-id',
              title: 'The Odyssey',
              author: 'Homer',
            },
          }),
        });
      } else if (route.request().method() === 'PUT' || route.request().method() === 'PATCH') {
        // Handle metadata override updates (updateBook uses PUT)
        await new Promise((resolve) => setTimeout(resolve, 2000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'test-book-id',
            title: 'The Odyssey',
          }),
        });
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'test-book-id',
            title: 'The Odyssey',
            author: 'Homer',
            file_path: '/library/test.m4b',
          }),
        });
      }
    });

    await page.route('**/api/v1/audiobooks/**/versions', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });

    await page.goto('/library/test-book-id');
    await page.waitForLoadState('networkidle');
  });

  test('Fetch Metadata button shows spinner during fetch', async ({ page }) => {
    // Find the Fetch Metadata button
    const fetchButton = page.getByRole('button', { name: /fetch metadata/i });
    await expect(fetchButton).toBeVisible();

    // Click the button
    await fetchButton.click();

    // After clicking, button text changes to "Fetching..." - find by new text
    const fetchingButton = page.getByRole('button', { name: /fetching/i });
    await expect(fetchingButton).toBeVisible();
    await expect(fetchingButton).toBeDisabled();

    // Wait for completion - button returns to "Fetch Metadata"
    await expect(page.getByRole('button', { name: /fetch metadata/i })).toBeEnabled({ timeout: 10000 });
  });

  test('Parse with AI button shows spinner during parse', async ({ page }) => {
    const parseButton = page.getByRole('button', { name: /parse with ai/i });
    await expect(parseButton).toBeVisible();

    await parseButton.click();

    // After clicking, button text changes to "Parsing..."
    const parsingButton = page.getByRole('button', { name: /parsing/i });
    await expect(parsingButton).toBeVisible();
    await expect(parsingButton).toBeDisabled();

    // Wait for completion
    await expect(page.getByRole('button', { name: /parse with ai/i })).toBeEnabled({ timeout: 10000 });
  });

  test('Compare tab - Use Fetched button shows spinner', async ({ page }) => {
    // Navigate to Compare tab
    await page.getByRole('tab', { name: /compare/i }).click();
    await page.waitForLoadState('networkidle');

    // Find Use Fetched button for audiobook_release_year
    const useFetchedButton = page
      .getByRole('row', { name: /audiobook release year/i })
      .getByRole('button', { name: /use fetched/i });

    await expect(useFetchedButton).toBeVisible();

    await useFetchedButton.click();

    // Should show spinner and "Applying..." text
    const applyingButton = page.getByRole('button', { name: /applying/i });
    await expect(applyingButton).toBeVisible();
    await expect(applyingButton).toBeDisabled();
  });

  test('Compare tab - other buttons remain clickable during action', async ({ page }) => {
    await page.getByRole('tab', { name: /compare/i }).click();
    await page.waitForLoadState('networkidle');

    const row = page.getByRole('row', { name: /audiobook release year/i });
    const useFetchedButton = row.getByRole('button', { name: /use fetched/i });

    await useFetchedButton.click();

    // After clicking, button shows "Applying..." and is disabled
    const applyingButton = row.getByRole('button', { name: /applying/i });
    await expect(applyingButton).toBeVisible();
    await expect(applyingButton).toBeDisabled();

    // But other buttons in different rows should still be clickable
    const titleRow = page.getByRole('row', { name: /^title/i });
    const titleUseFileButton = titleRow.getByRole('button', { name: /use file/i }).first();

    // This button should NOT be disabled (it's for a different field)
    await expect(titleUseFileButton).toBeEnabled();
  });

  test('No tab switching after fetch metadata', async ({ page }) => {
    // Start on Info tab
    const infoTab = page.getByRole('tab', { name: /^info$/i });
    await expect(infoTab).toHaveAttribute('aria-selected', 'true');

    // Click Fetch Metadata
    const fetchButton = page.getByRole('button', { name: /fetch metadata/i });
    await fetchButton.click();

    // Wait for fetch to complete - button returns to "Fetch Metadata"
    await expect(page.getByRole('button', { name: /fetch metadata/i })).toBeEnabled({ timeout: 10000 });

    // Should still be on Info tab (not switched away)
    await expect(infoTab).toHaveAttribute('aria-selected', 'true');
  });
});

test.describe('Dynamic UI - Library Page', () => {
  test.beforeEach(async ({ page }) => {
    // Pass books and import paths through setupMockApi to avoid route priority issues
    const books = generateTestBooks(1).map((b) => ({
      ...b,
      id: 'book-1',
      title: 'Test Book 1',
      author_name: 'Test Author',
    }));
    await setupMockApi(page, {
      books,
      importPaths: [
        {
          id: 1,
          path: '/test/path',
          book_count: 5,
        },
      ],
    });

    // Override scan and organize POST routes with delays for spinner testing
    await page.route('**/api/v1/operations/scan', async (route) => {
      if (route.request().method() === 'POST') {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'op-scan-123',
            type: 'scan',
            status: 'running',
          }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.route('**/api/v1/operations/organize', async (route) => {
      if (route.request().method() === 'POST') {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'op-organize-123',
            type: 'organize',
            status: 'running',
          }),
        });
      } else {
        await route.fallback();
      }
    });

    // Override operation status to keep returning 'running' for spinner tests
    await page.route('**/api/v1/operations/*/status', async (route) => {
      const opId = route.request().url().split('/').at(-2);
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: opId,
          type: opId?.includes('scan') ? 'scan' : 'organize',
          status: 'running',
          progress: 50,
        }),
      });
    });

    await page.goto('/library');
    await page.waitForLoadState('networkidle');
  });

  test('Scan All button shows spinner during scan', async ({ page }) => {
    // Find Scan All button (may be in header bar or import paths section)
    const scanAllButton = page.getByRole('button', { name: /scan all/i }).first();
    await expect(scanAllButton).toBeVisible();

    await scanAllButton.click();

    // After clicking, the button text changes to "Scanning..." - use text locator
    const scanningButton = page.getByRole('button', { name: /scanning/i }).first();
    await expect(scanningButton).toBeVisible();
    await expect(scanningButton).toBeDisabled();
  });

  test('Organize Library button shows spinner', async ({ page }) => {
    const organizeButton = page.getByRole('button', { name: /organize library/i });
    await expect(organizeButton).toBeVisible();

    await organizeButton.click();

    // After clicking, the button text changes to "Organizing…"
    const organizingButton = page.getByRole('button', { name: /organizing/i });
    await expect(organizingButton).toBeVisible();
    await expect(organizingButton).toBeDisabled();
  });

  test('Individual path scan button shows spinner', async ({ page }) => {
    // Expand import paths section
    const importPathsHeader = page.getByText('Import Paths (1)');
    await importPathsHeader.click();

    // Find the path list item and its refresh button (first icon button)
    const pathListItem = page.getByRole('listitem').filter({ hasText: '/test/path' });
    await expect(pathListItem).toBeVisible();
    const refreshButton = pathListItem.getByRole('button').first();

    // Mock the individual scan with delay
    await page.route('**/api/v1/operations/scan*', async (route) => {
      if (route.request().method() === 'POST') {
        await new Promise((resolve) => setTimeout(resolve, 2000));
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'op-scan-path-123',
            type: 'scan',
            status: 'running',
          }),
        });
      } else {
        await route.fallback();
      }
    });

    await refreshButton.click();

    // Should show spinner (the icon changes to CircularProgress)
    const spinner = pathListItem.locator('[role="progressbar"]').first();
    await expect(spinner).toBeVisible();
  });

  test('Remove path button shows spinner', async ({ page }) => {
    // Expand import paths section
    const importPathsHeader = page.getByText('Import Paths (1)');
    await importPathsHeader.click();

    // Find the path list item and its delete button (last icon button)
    const pathListItem = page.getByRole('listitem').filter({ hasText: '/test/path' });
    await expect(pathListItem).toBeVisible();
    const deleteButton = pathListItem.getByRole('button').last();

    // Mock the remove operation with delay
    await page.route('**/api/v1/import-paths/**', async (route) => {
      if (route.request().method() === 'DELETE') {
        await new Promise((resolve) => setTimeout(resolve, 2000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ message: 'Removed' }),
        });
      } else {
        await route.fallback();
      }
    });

    await deleteButton.click();

    // Should show spinner
    const spinner = pathListItem.locator('[role="progressbar"]').last();
    await expect(spinner).toBeVisible();
  });
});

test.describe('Dynamic UI - Dashboard Page', () => {
  test.beforeEach(async ({ page }) => {
    // Base mock setup
    await setupMockApi(page);

    // Override specific routes for spinner testing
    await page.route('**/api/v1/operations/scan', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'op-scan-123',
          type: 'scan',
          status: 'running',
        }),
      });
    });

    await page.route('**/api/v1/operations/organize', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'op-organize-123',
          type: 'organize',
          status: 'running',
        }),
      });
    });

    await page.goto('/');
    await page.waitForLoadState('networkidle');
  });

  test('Scan All Import Paths button shows spinner', async ({ page }) => {
    const scanButton = page.getByRole('button', { name: /scan all import paths/i });
    await expect(scanButton).toBeVisible();

    await scanButton.click();

    // After clicking, the button text changes to "Starting Scan..."
    const scanningButton = page.getByRole('button', { name: /starting scan/i });
    await expect(scanningButton).toBeVisible();
    await expect(scanningButton).toBeDisabled();
  });

  test('Organize All button opens dialog with spinner', async ({ page }) => {
    const organizeButton = page.getByRole('button', { name: /organize all/i });
    await expect(organizeButton).toBeVisible();

    await organizeButton.click();

    // Dialog should open
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Click the Organize button in the dialog
    const confirmButton = dialog.getByRole('button', { name: /^organize$/i });
    await confirmButton.click();

    // After clicking, the button text changes to "Organizing..."
    const organizingButton = dialog.getByRole('button', { name: /organizing/i });
    await expect(organizingButton).toBeVisible();
    await expect(organizingButton).toBeDisabled();
  });
});

test.describe('Visual Regression - Button States', () => {
  test('Button loading states visual check', async ({ page }) => {
    // This test captures screenshots of button states for visual regression testing
    await setupMockApi(page);

    // Override scan POST with delay to capture loading state
    await page.route('**/api/v1/operations/scan', async (route) => {
      if (route.request().method() === 'POST') {
        await new Promise((resolve) => setTimeout(resolve, 5000));
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({ id: 'op-vis-1', type: 'scan', status: 'running' }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/');
    await page.waitForLoadState('networkidle');

    const scanButton = page.getByRole('button', { name: /scan all import paths/i });
    await expect(scanButton).toBeVisible();
    await scanButton.click();

    // After clicking, find the button in loading state
    const loadingButton = page.getByRole('button', { name: /starting scan/i });
    await expect(loadingButton).toBeVisible();

    // Take screenshot of loading state
    await expect(loadingButton).toHaveScreenshot('scan-button-loading.png');
  });
});
