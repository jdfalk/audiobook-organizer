// file: web/tests/e2e/dynamic-ui-interactions.spec.ts
// version: 1.1.0
// guid: 9f8e7d6c-5b4a-3210-fedc-ba9876543210
// last-edited: 2026-02-04

/**
 * E2E tests for dynamic UI interactions with in-place loading states
 * Tests the Sonarr/Radarr-style button spinners and smooth updates
 */

import { test, expect, type Page } from '@playwright/test';
import { setupPhase1ApiDriven, mockEventSource } from './utils/test-helpers';

test.describe('Dynamic UI - BookDetail Page', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);

    // Mock API responses
    await page.route('**/api/v1/audiobooks/*', async (route) => {
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
        // Simulate delay to test spinner
        await new Promise((resolve) => setTimeout(resolve, 1000));
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
      } else if (route.request().method() === 'PATCH') {
        // Handle metadata override updates
        await new Promise((resolve) => setTimeout(resolve, 500));
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

    await page.route('**/api/v1/audiobooks/*/versions', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });

    await page.goto('/book/test-book-id');
    await page.waitForLoadState('networkidle');
  });

  test('Fetch Metadata button shows spinner during fetch', async ({ page }) => {
    // Find the Fetch Metadata button
    const fetchButton = page.getByRole('button', { name: /fetch metadata/i });
    await expect(fetchButton).toBeVisible();
    await expect(fetchButton).toHaveText('Fetch Metadata');

    // Click the button
    await fetchButton.click();

    // Should show spinner and "Fetching..." text
    await expect(fetchButton).toBeDisabled();
    await expect(fetchButton).toHaveText('Fetching...');

    // Verify spinner is present
    const spinner = fetchButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();

    // Wait for completion
    await expect(fetchButton).toBeEnabled({ timeout: 5000 });
    await expect(fetchButton).toHaveText('Fetch Metadata');
  });

  test('Parse with AI button shows spinner during parse', async ({ page }) => {
    const parseButton = page.getByRole('button', { name: /parse with ai/i });
    await expect(parseButton).toBeVisible();
    await expect(parseButton).toHaveText('Parse with AI');

    await parseButton.click();

    // Should show spinner and "Parsing..." text
    await expect(parseButton).toBeDisabled();
    await expect(parseButton).toHaveText('Parsing...');

    const spinner = parseButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();

    await expect(parseButton).toBeEnabled({ timeout: 5000 });
    await expect(parseButton).toHaveText('Parse with AI');
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
    await expect(useFetchedButton).toHaveText('Use Fetched');

    await useFetchedButton.click();

    // Should show spinner and "Applying..." text
    await expect(useFetchedButton).toBeDisabled();
    await expect(useFetchedButton).toHaveText('Applying...');

    const spinner = useFetchedButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();

    // Should return to normal after completion
    await expect(useFetchedButton).toBeEnabled({ timeout: 3000 });
  });

  test('Compare tab - other buttons remain clickable during action', async ({ page }) => {
    await page.getByRole('tab', { name: /compare/i }).click();
    await page.waitForLoadState('networkidle');

    const row = page.getByRole('row', { name: /audiobook release year/i });
    const useFetchedButton = row.getByRole('button', { name: /use fetched/i });
    const useFileButton = row.getByRole('button', { name: /use file/i });

    await useFetchedButton.click();

    // Use Fetched button should be disabled
    await expect(useFetchedButton).toBeDisabled();

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

    // Wait for fetch to complete
    await expect(fetchButton).toBeEnabled({ timeout: 5000 });

    // Should still be on Info tab (not switched away)
    await expect(infoTab).toHaveAttribute('aria-selected', 'true');
  });
});

test.describe('Dynamic UI - Library Page', () => {
  test.beforeEach(async ({ page }) => {
    // Mock library API
    await page.route('**/api/v1/audiobooks*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            {
              id: 'book-1',
              title: 'Test Book 1',
              author: 'Test Author',
            },
          ],
          page: 1,
          total_pages: 1,
          total_items: 1,
        }),
      });
    });

    await page.route('**/api/v1/library/folders', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            id: 1,
            path: '/test/path',
            book_count: 5,
          },
        ]),
      });
    });

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

    await page.route('**/api/v1/system/status', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          library: {
            book_count: 1,
            folder_count: 1,
          },
        }),
      });
    });

    await page.route('**/api/v1/authors', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });

    await page.route('**/api/v1/series', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });

    // Mock SSE
    await page.route('**/api/events', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: '',
      });
    });

    await page.goto('/library');
    await page.waitForLoadState('networkidle');
  });

  test('Scan All button shows spinner during scan', async ({ page }) => {
    // Expand import paths section
    const importPathsSection = page.getByText('Import Paths (1)');
    if (await importPathsSection.isVisible()) {
      await importPathsSection.click();
    }

    const scanAllButton = page.getByRole('button', { name: /scan all/i }).first();
    await expect(scanAllButton).toBeVisible();
    await expect(scanAllButton).toHaveText('Scan All');

    await scanAllButton.click();

    // Should show spinner and "Scanning..." text
    await expect(scanAllButton).toBeDisabled();
    await expect(scanAllButton).toHaveText('Scanning...');

    const spinner = scanAllButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();
  });

  test('Organize Library button shows spinner', async ({ page }) => {
    const organizeButton = page.getByRole('button', { name: /organize library/i });
    await expect(organizeButton).toBeVisible();

    await organizeButton.click();

    await expect(organizeButton).toBeDisabled();
    await expect(organizeButton).toHaveText('Organizing…');

    const spinner = organizeButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();
  });

  test('Individual path scan button shows spinner', async ({ page }) => {
    // Expand import paths
    const importPathsHeader = page.getByText('Import Paths (1)');
    await importPathsHeader.click();

    // Find the refresh icon button for the individual path
    const pathItem = page.getByText('/test/path');
    const refreshButton = pathItem.locator('..').locator('..').getByRole('button').first();

    // Mock the individual scan
    await page.route('**/api/v1/operations/scan*', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 800));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'op-scan-path-123',
          type: 'scan',
          status: 'running',
        }),
      });
    });

    await refreshButton.click();

    // Should show spinner (the icon changes to CircularProgress)
    const spinner = refreshButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();
    await expect(refreshButton).toBeDisabled();
  });

  test('Remove path button shows spinner', async ({ page }) => {
    // Expand import paths
    const importPathsHeader = page.getByText('Import Paths (1)');
    await importPathsHeader.click();

    // Find the delete icon button
    const pathItem = page.getByText('/test/path');
    const deleteButton = pathItem.locator('..').locator('..').getByRole('button').last();

    // Mock the remove operation
    await page.route('**/api/v1/library/folders/*', async (route) => {
      if (route.request().method() === 'DELETE') {
        await new Promise((resolve) => setTimeout(resolve, 500));
        await route.fulfill({ status: 200 });
      }
    });

    await deleteButton.click();

    // Should show spinner
    const spinner = deleteButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();
    await expect(deleteButton).toBeDisabled();
  });
});

test.describe('Dynamic UI - Dashboard Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/api/v1/system/status', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          library: {
            book_count: 10,
            folder_count: 2,
            total_size: 1000000,
          },
        }),
      });
    });

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
    await expect(scanButton).toHaveText('Scan All Import Paths');

    await scanButton.click();

    // Should show spinner and "Starting Scan..." text
    await expect(scanButton).toBeDisabled();
    await expect(scanButton).toHaveText('Starting Scan...');

    const spinner = scanButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();
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

    // Should show spinner
    await expect(confirmButton).toBeDisabled();
    await expect(confirmButton).toHaveText('Organizing...');

    const spinner = confirmButton.locator('[role="progressbar"]');
    await expect(spinner).toBeVisible();
  });
});

test.describe('Visual Regression - Button States', () => {
  test('Button loading states visual check', async ({ page }) => {
    // This test captures screenshots of button states for visual regression testing

    await page.route('**/api/**', async (route) => {
      // Delay all API calls to capture loading states
      await new Promise((resolve) => setTimeout(resolve, 2000));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({}),
      });
    });

    await page.goto('/');

    const scanButton = page.getByRole('button', { name: /scan all import paths/i });
    await scanButton.click();

    // Wait a bit for spinner to appear
    await page.waitForTimeout(100);

    // Take screenshot of loading state
    await expect(scanButton).toHaveScreenshot('scan-button-loading.png');
  });
});
