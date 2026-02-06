// file: web/tests/e2e/backup-restore.spec.ts
// version: 1.2.0
// guid: 467b5537-3e41-4190-938c-e2f2ccb2e127
// last-edited: 2026-02-06

import { test, expect, type Page } from '@playwright/test';
import {
  mockEventSource,
  setupPhase2Interactive,
} from './utils/test-helpers';

const backups = [
  {
    filename: 'backup-2026-01-25.db.gz',
    size: 25 * 1024 * 1024,
    created_at: '2026-01-25T12:00:00Z',
  },
  {
    filename: 'backup-2026-01-20.db.gz',
    size: 22 * 1024 * 1024,
    created_at: '2026-01-20T12:00:00Z',
    auto: true,
  },
];

const openBackupSettings = async (
  page: Page,
  backupsToUse: any[] = [],
  failures: Record<string, any> = {}
) => {
  // Phase 2 setup: Reset and skip welcome wizard with mocked APIs (must be before page.goto)
  await setupPhase2Interactive(page, undefined, { backups: backupsToUse, failures });
  // Mock EventSource to prevent SSE connections
  await mockEventSource(page);
  // Now navigate to settings
  await page.goto('/settings');
  await page.waitForLoadState('domcontentloaded');
};

test.describe('Backup and Restore', () => {
  test.beforeEach(async () => {
    // No setup in beforeEach - each test sets up its own mocks
    // This allows each test to use different mock data
  });

  test('creates manual backup', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, []);

    // Act
    await page.getByRole('button', { name: 'Create Backup' }).click();

    // Assert
    await expect(
      page.getByText('Backup created successfully.')
    ).toBeVisible();
    await expect(page.getByText(/backup-/i)).toBeVisible();
  });

  test('lists existing backups', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, backups);

    // Act + Assert
    await expect(page.getByText('backup-2026-01-25.db.gz')).toBeVisible();
    await expect(page.getByText('backup-2026-01-20.db.gz')).toBeVisible();
  });

  test('downloads backup file', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, backups);

    // Act
    const downloadLink = page
      .getByRole('link', { name: 'Download' })
      .first();

    // Assert
    await expect(downloadLink).toHaveAttribute(
      'href',
      '/api/v1/backup/backup-2026-01-25.db.gz'
    );
  });

  test('restores from backup', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, backups);

    // Act
    await page.getByRole('button', { name: 'Restore' }).first().click();
    await expect(page.getByText('Restore Backup')).toBeVisible();
    await page.getByRole('button', { name: 'Restore' }).last().click();

    // Assert
    await page.waitForLoadState('load');
    await expect(page.getByText('Backups')).toBeVisible();
  });

  test('deletes backup file', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, backups);

    // Act
    await page.getByRole('button', { name: 'Delete' }).first().click();
    await page.getByRole('button', { name: 'Delete' }).last().click();

    // Assert
    await expect(
      page.getByText('Backup deleted successfully.')
    ).toBeVisible();
  });

  test('validates backup before restore', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, backups, { restoreBackup: 500 });

    // Act
    await page.getByRole('button', { name: 'Restore' }).first().click();
    await page.getByRole('button', { name: 'Restore' }).last().click();

    // Assert
    await expect(page.getByText('Backup file is corrupt.')).toBeVisible();
  });

  test('automatic backup on schedule', async ({ page }) => {
    // Arrange
    await openBackupSettings(page, backups);

    // Act + Assert
    await expect(page.getByText('Auto')).toBeVisible();
  });
});
