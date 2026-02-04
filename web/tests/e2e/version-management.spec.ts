// file: web/tests/e2e/version-management.spec.ts
// version: 1.1.0
// guid: 570ee522-c0f2-4d0c-ba5c-b5399cede9a9
// last-edited: 2026-02-04

import { test, expect, type Page } from '@playwright/test';
import {
  generateTestBooks,
  mockEventSource,
  setupMockApi,
  setupPhase1ApiDriven,
} from './utils/test-helpers';

const baseBook = generateTestBooks(1)[0];

const buildBook = (overrides: Record<string, unknown>) => ({
  ...baseBook,
  ...overrides,
});

const openBookDetail = async (page: Page, books: Record<string, unknown>[]) => {
  await setupMockApi(page, { books });
  await page.goto(`/library/${books[0].id}`);
  await page.waitForLoadState('networkidle');
};

test.describe('Version Management', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
  });

  test('links two books as versions', async ({ page }) => {
    // Arrange
    const bookA = buildBook({
      id: 'book-a',
      title: 'The Way of Kings',
      author_name: 'Brandon Sanderson',
    });
    const bookB = buildBook({
      id: 'book-b',
      title: 'The Way of Kings (MP3)',
      author_name: 'Brandon Sanderson',
    });
    await openBookDetail(page, [bookA, bookB]);

    // Act
    await page.getByRole('button', { name: 'Manage Versions' }).click();
    await page.getByRole('button', { name: 'Link Another Version' }).click();
    await page.getByLabel('Search by title or author').fill('Way of Kings');
    await page.getByText('The Way of Kings (MP3)').click();
    await page.getByRole('button', { name: 'Link Version' }).click();

    // Assert
    await expect(page.getByText('The Way of Kings (MP3)')).toBeVisible();
    await page.getByRole('button', { name: 'Close' }).click();
    await page.getByRole('tab', { name: /Versions/ }).click();
    await expect(
      page.getByText('Part of version group with 2 books.')
    ).toBeVisible();
  });

  test('sets primary version', async ({ page }) => {
    // Arrange
    const bookA = buildBook({
      id: 'book-a',
      title: 'The Way of Kings',
      author_name: 'Brandon Sanderson',
      version_group_id: 'group-1',
      is_primary_version: true,
    });
    const bookB = buildBook({
      id: 'book-b',
      title: 'The Way of Kings (MP3)',
      author_name: 'Brandon Sanderson',
      version_group_id: 'group-1',
      is_primary_version: false,
    });
    await setupMockApi(page, { books: [bookA, bookB] });
    await page.goto('/library/book-b');
    await page.waitForLoadState('networkidle');

    // Act
    await page.getByRole('button', { name: 'Manage Versions' }).click();
    await page
      .getByRole('button', { name: 'Set primary for The Way of Kings (MP3)' })
      .click();

    // Assert
    const currentRow = page
      .getByRole('listitem')
      .filter({ hasText: 'The Way of Kings (MP3)' })
      .first();
    await expect(currentRow.getByText('Primary')).toBeVisible();
  });

  test('unlinks version', async ({ page }) => {
    // Arrange
    const bookA = buildBook({
      id: 'book-a',
      title: 'The Way of Kings',
      author_name: 'Brandon Sanderson',
      version_group_id: 'group-1',
      is_primary_version: true,
    });
    const bookB = buildBook({
      id: 'book-b',
      title: 'The Way of Kings (MP3)',
      author_name: 'Brandon Sanderson',
      version_group_id: 'group-1',
      is_primary_version: false,
    });
    await openBookDetail(page, [bookA, bookB]);

    // Act
    await page.getByRole('button', { name: 'Manage Versions' }).click();
    const row = page
      .getByRole('listitem')
      .filter({ hasText: 'The Way of Kings (MP3)' })
      .first();
    await row.getByRole('button', { name: 'Unlink' }).click();
    await page.getByRole('button', { name: 'Unlink' }).last().click();

    // Assert
    await expect(page.getByText('No Additional Versions')).toBeVisible();
  });

  test('navigates between versions', async ({ page }) => {
    // Arrange
    const groupId = 'group-2';
    const bookA = buildBook({
      id: 'book-a',
      title: 'The Way of Kings',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: true,
    });
    const bookB = buildBook({
      id: 'book-b',
      title: 'The Way of Kings (MP3)',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: false,
    });
    const bookC = buildBook({
      id: 'book-c',
      title: 'The Way of Kings (FLAC)',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: false,
    });
    await openBookDetail(page, [bookA, bookB, bookC]);

    // Act
    await page.getByRole('tab', { name: /Versions/ }).click();
    await page.getByText('The Way of Kings (MP3)').click();

    // Assert
    await expect(page).toHaveURL(/\/library\/book-b/);
  });

  test('shows version group information', async ({ page }) => {
    // Arrange
    const groupId = 'group-3';
    const bookA = buildBook({
      id: 'book-a',
      title: 'The Way of Kings',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: true,
    });
    const bookB = buildBook({
      id: 'book-b',
      title: 'The Way of Kings (MP3)',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: false,
    });
    const bookC = buildBook({
      id: 'book-c',
      title: 'The Way of Kings (FLAC)',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: false,
    });
    await openBookDetail(page, [bookA, bookB, bookC]);

    // Act
    await page.getByRole('tab', { name: /Versions/ }).click();

    // Assert
    await expect(
      page.getByText('Part of version group with 3 books.')
    ).toBeVisible();
    await expect(page.getByText('(Current)')).toBeVisible();
  });

  test('prevents circular version links', async ({ page }) => {
    // Arrange
    const groupId = 'group-4';
    const bookA = buildBook({
      id: 'book-a',
      title: 'The Way of Kings',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: true,
    });
    const bookB = buildBook({
      id: 'book-b',
      title: 'The Way of Kings (MP3)',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: false,
    });
    const bookC = buildBook({
      id: 'book-c',
      title: 'The Way of Kings (FLAC)',
      author_name: 'Brandon Sanderson',
      version_group_id: groupId,
      is_primary_version: false,
    });
    await openBookDetail(page, [bookA, bookB, bookC]);

    // Act
    await page.getByRole('button', { name: 'Manage Versions' }).click();
    await page.getByRole('button', { name: 'Link Another Version' }).click();
    await page.getByLabel('Search by title or author').fill('FLAC');
    await page.getByText('The Way of Kings (FLAC)').click();
    await page.getByRole('button', { name: 'Link Version' }).click();

    // Assert
    await expect(
      page.getByText('Cannot create circular version links')
    ).toBeVisible();
  });
});
