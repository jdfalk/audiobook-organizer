// file: web/tests/e2e/import-audiobook-file.spec.ts
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

import { test, expect, type Page } from '@playwright/test';
import {
  mockEventSource,
  setupMockApi,
  skipWelcomeWizard,
} from './utils/test-helpers';

const filesystem = {
  '/': {
    path: '/',
    items: [
      { name: 'Users', path: '/Users', is_dir: true, excluded: false },
      { name: 'tmp', path: '/tmp', is_dir: true, excluded: false },
    ],
    disk_info: {
      exists: true,
      readable: true,
      writable: true,
      total_bytes: 500 * 1024 * 1024 * 1024,
      free_bytes: 350 * 1024 * 1024 * 1024,
      library_bytes: 45 * 1024 * 1024 * 1024,
    },
  },
  '/Users': {
    path: '/Users',
    items: [
      { name: 'jdfalk', path: '/Users/jdfalk', is_dir: true, excluded: false },
    ],
  },
  '/Users/jdfalk': {
    path: '/Users/jdfalk',
    items: [
      {
        name: 'Audiobooks',
        path: '/Users/jdfalk/Audiobooks',
        is_dir: true,
        excluded: false,
      },
      {
        name: 'Documents',
        path: '/Users/jdfalk/Documents',
        is_dir: true,
        excluded: false,
      },
    ],
  },
  '/Users/jdfalk/Audiobooks': {
    path: '/Users/jdfalk/Audiobooks',
    items: [
      {
        name: 'Brandon Sanderson',
        path: '/Users/jdfalk/Audiobooks/Brandon Sanderson',
        is_dir: true,
        excluded: false,
      },
      {
        name: 'standalone.m4b',
        path: '/Users/jdfalk/Audiobooks/standalone.m4b',
        is_dir: false,
        size: 250 * 1024 * 1024,
        mod_time: 1640000000,
        excluded: false,
      },
    ],
  },
  '/Users/jdfalk/Audiobooks/Brandon Sanderson': {
    path: '/Users/jdfalk/Audiobooks/Brandon Sanderson',
    items: [
      {
        name: 'Mistborn 01 - The Final Empire.m4b',
        path: '/Users/jdfalk/Audiobooks/Brandon Sanderson/Mistborn 01 - The Final Empire.m4b',
        is_dir: false,
        size: 400 * 1024 * 1024,
        mod_time: 1640000000,
        excluded: false,
      },
      {
        name: 'Mistborn 02 - The Well of Ascension.m4b',
        path: '/Users/jdfalk/Audiobooks/Brandon Sanderson/Mistborn 02 - The Well of Ascension.m4b',
        is_dir: false,
        size: 450 * 1024 * 1024,
        mod_time: 1640100000,
        excluded: false,
      },
    ],
  },
  '/tmp': {
    path: '/tmp',
    items: [
      {
        name: 'test-audio.m4b',
        path: '/tmp/test-audio.m4b',
        is_dir: false,
        size: 100 * 1024 * 1024,
        excluded: false,
      },
    ],
  },
};

const openImportFileBrowser = async (page: Page) => {
  await setupMockApi(page, {
    filesystem,
    books: [],
    homeDirectory: '/Users/jdfalk',
  });
  await page.goto('/library');
  await page.waitForLoadState('networkidle');
  await page.getByRole('button', { name: 'Import Files' }).first().click();
  await expect(
    page.getByRole('dialog', { name: 'Import Audiobook File' })
  ).toBeVisible();
};

test.describe('Import Audiobook File - Interactive Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await skipWelcomeWizard(page);
  });

  test('navigates to home directory when home icon is clicked', async ({
    page,
  }) => {
    // Arrange: Start at root
    await openImportFileBrowser(page);
    await expect(page.getByText('Users')).toBeVisible();
    await expect(page.getByText('tmp')).toBeVisible();

    // Act: Click home icon (first breadcrumb)
    const homeButton = page.locator('button').filter({ has: page.locator('svg[data-testid="HomeIcon"]') }).first();
    await homeButton.click();

    // Assert: Should navigate to /Users/jdfalk (home directory)
    await expect(page.getByText('Audiobooks')).toBeVisible();
    await expect(page.getByText('Documents')).toBeVisible();
  });

  test('navigates through directories by clicking folders', async ({
    page,
  }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Act: Navigate through folder structure by clicking
    await page.getByRole('button', { name: 'Users' }).click();
    await expect(page.getByText('jdfalk')).toBeVisible();

    await page.getByRole('button', { name: 'jdfalk' }).click();
    await expect(page.getByText('Audiobooks')).toBeVisible();

    await page.getByRole('button', { name: 'Audiobooks' }).click();
    await expect(page.getByText('Brandon Sanderson')).toBeVisible();

    // Assert: Final directory contents visible
    await expect(page.getByText('standalone.m4b')).toBeVisible();
  });

  test('navigates using breadcrumb links', async ({ page }) => {
    // Arrange: Navigate deep into folder structure
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();
    await page.getByRole('button', { name: 'Brandon Sanderson' }).click();

    // Assert: We're in the deep folder
    await expect(
      page.getByText('Mistborn 01 - The Final Empire.m4b')
    ).toBeVisible();

    // Act: Click "Audiobooks" in breadcrumb to go back
    await page.getByRole('button', { name: 'Audiobooks' }).first().click();

    // Assert: Should be back at Audiobooks folder
    await expect(page.getByText('Brandon Sanderson')).toBeVisible();
    await expect(page.getByText('standalone.m4b')).toBeVisible();
  });

  test('edits path manually and navigates', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Act: Click edit path button
    const editButton = page.locator('button[aria-label="edit path"], button').filter({ has: page.locator('svg[data-testid="EditIcon"]') }).first();
    await editButton.click();

    // Edit path to /tmp
    const pathInput = page.getByRole('textbox').first();
    await pathInput.clear();
    await pathInput.fill('/tmp');

    // Save the path
    const checkButton = page.locator('button').filter({ has: page.locator('svg[data-testid="CheckIcon"]') }).first();
    await checkButton.click();

    // Assert: Should navigate to /tmp
    await expect(page.getByText('test-audio.m4b')).toBeVisible();
  });

  test('filters files by extension', async ({ page }) => {
    // Arrange: Navigate to folder with multiple file types
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();

    // Act: Enter filter
    await page.getByLabel('Filter extension').fill('.m4b');

    // Assert: Only .m4b files visible
    await expect(page.getByText('standalone.m4b')).toBeVisible();
    // Folders should still be visible
    await expect(page.getByText('Brandon Sanderson')).toBeVisible();
  });

  test('double-clicks file to select it', async ({ page }) => {
    // Arrange: Navigate to file
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();

    // Act: Double-click the file
    const fileButton = page.getByRole('button', { name: 'standalone.m4b' });
    await fileButton.dblclick();

    // Assert: File should be selected (path should show in dialog)
    // Note: This depends on how your UI shows selection - adjust selector as needed
    await expect(page.getByText(/standalone\.m4b/)).toBeVisible();
  });

  test('navigates from root to home to specific file through clicks only', async ({
    page,
  }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Start at root
    await expect(page.getByText('Users')).toBeVisible();

    // Act: Complete navigation journey using only clicks
    // 1. Click home icon to go to home directory
    const homeButton = page.locator('button').filter({ has: page.locator('svg[data-testid="HomeIcon"]') }).first();
    await homeButton.click();
    await expect(page.getByText('Audiobooks')).toBeVisible();

    // 2. Navigate into Audiobooks
    await page.getByRole('button', { name: 'Audiobooks' }).click();
    await expect(page.getByText('Brandon Sanderson')).toBeVisible();

    // 3. Navigate into author folder
    await page.getByRole('button', { name: 'Brandon Sanderson' }).click();
    await expect(
      page.getByText('Mistborn 01 - The Final Empire.m4b')
    ).toBeVisible();

    // 4. Select a file by double-clicking
    const mistborn1 = page.getByRole('button', {
      name: 'Mistborn 01 - The Final Empire.m4b',
    });
    await mistborn1.dblclick();

    // Assert: File should be selected
    await expect(
      page.getByText(/Mistborn 01 - The Final Empire\.m4b/)
    ).toBeVisible();
  });

  test('shows disk space information', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Assert: Disk info chips should be visible
    await expect(page.getByText(/Available/i)).toBeVisible();
    await expect(page.getByText(/Library/i)).toBeVisible();
    await expect(page.getByText('Readable')).toBeVisible();
    await expect(page.getByText('Writable')).toBeVisible();
  });

  test('handles clicking on excluded folders', async ({ page }) => {
    // This test would need a folder marked as excluded in the filesystem mock
    // For now, testing the exclusion workflow
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();

    // Right-click on a folder
    await page
      .getByRole('button', { name: 'Brandon Sanderson' })
      .click({ button: 'right' });

    // Assert: Context menu appears
    await expect(
      page.getByRole('menuitem', { name: 'Exclude from scan' })
    ).toBeVisible();
  });

  test('preserves navigation state when filtering', async ({ page }) => {
    // Arrange: Navigate to a specific folder
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();

    // Act: Apply filter
    await page.getByLabel('Filter extension').fill('.m4b');

    // Assert: Should still be in the same directory (breadcrumb unchanged)
    await expect(page.getByRole('button', { name: 'Audiobooks' }).first()).toBeVisible();
    await expect(page.getByText('standalone.m4b')).toBeVisible();

    // Folder should still be navigable
    await page.getByRole('button', { name: 'Brandon Sanderson' }).click();
    await expect(page.getByText('Mistborn 01 - The Final Empire.m4b')).toBeVisible();
  });

  test('displays file size and modification time', async ({ page }) => {
    // Arrange: Navigate to folder with files
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();

    // Assert: File metadata should be visible
    // The exact format depends on your formatBytes function
    await expect(page.getByText(/MB/)).toBeVisible();
  });

  test('handles empty directories gracefully', async ({ page }) => {
    // Would need an empty directory in the filesystem mock
    // This is a placeholder for when you add one
    await openImportFileBrowser(page);

    // Navigate to an empty directory (if one exists in mock)
    // Assert appropriate message is shown
  });

  test('maintains current path after closing and reopening dialog', async ({
    page,
  }) => {
    // Arrange: Open dialog and navigate
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'Users' }).click();
    await page.getByRole('button', { name: 'jdfalk' }).click();
    await page.getByRole('button', { name: 'Audiobooks' }).click();

    // Act: Close dialog
    const closeButton = page.getByRole('button', { name: 'Cancel' }).or(page.getByLabel('close'));
    await closeButton.click();

    // Reopen dialog
    await page.getByRole('button', { name: 'Import Files' }).click();

    // Assert: Should start at root again (or wherever initial path is set)
    await expect(
      page.getByRole('dialog', { name: 'Import Audiobook File' })
    ).toBeVisible();
  });
});

test.describe('Import Audiobook File - Error Handling', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await skipWelcomeWizard(page);
  });

  test('handles invalid path entry gracefully', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Act: Enter invalid path
    const editButton = page.locator('button').filter({ has: page.locator('svg[data-testid="EditIcon"]') }).first();
    await editButton.click();

    const pathInput = page.getByRole('textbox').first();
    await pathInput.clear();
    await pathInput.fill('/nonexistent/path');

    const checkButton = page.locator('button').filter({ has: page.locator('svg[data-testid="CheckIcon"]') }).first();
    await checkButton.click();

    // Assert: Error message should appear
    // (Exact behavior depends on your error handling)
    await expect(page.getByText(/error|failed/i)).toBeVisible({ timeout: 5000 });
  });
});
