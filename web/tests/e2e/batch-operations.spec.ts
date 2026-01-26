// file: web/tests/e2e/batch-operations.spec.ts
// version: 1.0.2
// guid: 5d6e7f80-9a0b-1c2d-3e4f-5a6b7c8d9e0f

import { test, expect, type Page } from '@playwright/test';
import {
  generateTestBooks,
  mockEventSource,
  setupCommonRoutes,
  setupLibraryWithBooks,
  skipWelcomeWizard,
  waitForToast,
} from './utils/test-helpers';

const arrangeLibrary = async (page: Page, count = 40) => {
  const books = generateTestBooks(count);
  await setupLibraryWithBooks(page, books);
  await page.goto('/library');
  await page.waitForLoadState('networkidle');
  return books;
};

test.describe('Batch Operations', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await skipWelcomeWizard(page);
    await setupCommonRoutes(page);
  });

  test('selects single book with checkbox', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);

    // Act
    await page.getByLabel('Select Test Book 1').click();

    // Assert
    await expect(page.getByText('1 selected')).toBeVisible();
  });

  test('selects multiple books with individual checkboxes', async ({
    page,
  }) => {
    // Arrange
    await arrangeLibrary(page);

    // Act
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();
    await page.getByLabel('Select Test Book 3').click();
    await page.getByLabel('Select Test Book 4').click();
    await page.getByLabel('Select Test Book 5').click();

    // Assert
    await expect(page.getByText('5 selected')).toBeVisible();
  });

  test('selects all books on current page', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);

    // Act
    await page.getByLabel('Select All').click();

    // Assert
    await expect(page.getByText('20 selected')).toBeVisible();
  });

  test('deselects all books', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select All').click();

    // Act
    await page.getByRole('button', { name: 'Deselect All' }).click();

    // Assert
    await expect(page.getByText('0 selected')).toBeVisible();
  });

  test('selection persists across page navigation', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select Test Book 1').click();

    // Act
    await page.getByRole('button', { name: '2' }).click();
    await page.getByRole('button', { name: '1' }).click();

    // Assert
    await expect(page.getByLabel('Select Test Book 1')).toBeChecked();
  });

  test('bulk fetches metadata for selected books', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();
    await page.getByLabel('Select Test Book 3').click();

    // Act
    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await page.getByRole('button', { name: 'Fetch Metadata' }).last().click();

    // Assert
    await expect(page.getByText('3 / 3 completed')).toBeVisible();
    await waitForToast(page, 'Metadata fetched for 3 books.');
  });

  test('monitors bulk fetch progress', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select All').click();

    // Act
    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await page.getByRole('button', { name: 'Fetch Metadata' }).last().click();

    // Assert
    await expect(page.getByText('20 / 20 completed')).toBeVisible();
    await expect(page.getByText('Test Book 1')).toBeVisible();
  });

  test('bulk fetch completes successfully and clears selection', async ({
    page,
  }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select Test Book 1').click();

    // Act
    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await page.getByRole('button', { name: 'Fetch Metadata' }).last().click();

    // Assert
    await waitForToast(page, 'Metadata fetched for 1 books.');
    await expect(page.getByText('0 selected')).toBeVisible();
  });

  test('bulk fetch handles partial failures', async ({ page }) => {
    // Arrange
    const books = generateTestBooks(10);
    books[1].fetch_metadata_error = true;
    books[3].fetch_metadata_error = true;
    await setupLibraryWithBooks(page, books);
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();
    await page.getByLabel('Select Test Book 3').click();
    await page.getByLabel('Select Test Book 4').click();
    await page.getByLabel('Select Test Book 5').click();

    // Act
    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await page.getByRole('button', { name: 'Fetch Metadata' }).last().click();

    // Assert
    await waitForToast(page, '3 succeeded, 2 failed.');
    await expect(page.getByText('Failed')).toBeVisible();
  });

  test('cancels bulk fetch operation', async ({ page }) => {
    // Arrange
    const books = generateTestBooks(15).map((book) => ({
      ...book,
      fetch_metadata_delay_ms: 200,
    }));
    await setupLibraryWithBooks(page, books);
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Select All').click();

    // Act
    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await page.getByRole('button', { name: 'Fetch Metadata' }).last().click();
    await page.getByRole('button', { name: 'Cancel' }).click();

    // Assert
    await waitForToast(page, 'Bulk fetch cancelled.');
  });

  test('batch updates metadata field for selected books', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();
    await page.getByLabel('Select Test Book 3').click();

    // Act
    await page.getByRole('button', { name: 'Batch Edit' }).click();
    await page.getByRole('checkbox', { name: 'Language' }).check();
    await page.getByPlaceholder('Language').fill('en');
    await page.getByRole('button', { name: 'Update 3 audiobooks' }).click();

    // Assert
    await waitForToast(page, 'Updated metadata for 3 audiobooks.');
  });

  test('batch soft-deletes selected books', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();

    // Act
    await page.getByRole('button', { name: 'Delete Selected' }).click();
    await page.getByRole('button', { name: 'Delete Selected' }).last().click();

    // Assert
    await waitForToast(page, 'Soft deleted 2 selected audiobooks.');
  });

  test('batch restores soft-deleted books', async ({ page }) => {
    // Arrange
    const books = generateTestBooks(5).map((book, index) => ({
      ...book,
      marked_for_deletion: index < 3,
    }));
    await setupLibraryWithBooks(page, books);
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();
    await page.getByLabel('Select Test Book 3').click();

    // Act
    await page.getByRole('button', { name: 'Restore Selected' }).click();

    // Assert
    await waitForToast(page, 'Restored 3 selected audiobooks.');
  });

  test('disables batch operations when no books selected', async ({ page }) => {
    // Arrange
    await arrangeLibrary(page);

    // Act
    await page.getByText('Batch Edit').hover();

    // Assert
    await expect(page.getByText('Select books first')).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Batch Edit' })
    ).toBeDisabled();
    await expect(
      page.getByRole('button', { name: 'Fetch Metadata' })
    ).toBeDisabled();
  });

  test('shows different batch actions based on selection state', async ({
    page,
  }) => {
    // Arrange
    const books = generateTestBooks(6).map((book, index) => ({
      ...book,
      marked_for_deletion: index % 2 === 0,
    }));
    await setupLibraryWithBooks(page, books);
    await page.goto('/library');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Select Test Book 1').click();
    await page.getByLabel('Select Test Book 2').click();

    // Assert
    await expect(
      page.getByRole('button', { name: 'Delete Selected' })
    ).toBeEnabled();
    await expect(
      page.getByRole('button', { name: 'Restore Selected' })
    ).toBeEnabled();
  });
});
