// file: tests/e2e/itunes-import.spec.ts
// version: 1.3.0
// guid: 8d0eb913-029f-42f1-ad7b-984aa66a6fdc
// last-edited: 2026-03-02

import { test, expect } from '@playwright/test';
import { setupMockApi } from './utils/test-helpers';

test.describe('iTunes Import', () => {
  test.beforeEach(async ({ page }) => {
    await setupMockApi(page);
  });

  test('validates iTunes library', async ({ page }) => {
    // Act
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();
    await page
      .getByLabel('iTunes Library Path')
      .fill('/path/to/test/library.xml');
    await page.getByRole('button', { name: 'Validate Import' }).click();

    // Assert
    await expect(page.getByText('Validation Results')).toBeVisible();
  });

  test('imports iTunes library', async ({ page }) => {
    // Act
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'iTunes Import' }).click();
    await page
      .getByLabel('iTunes Library Path')
      .fill('/path/to/test/library.xml');
    await page.getByRole('button', { name: 'Validate Import' }).click();
    await expect(page.getByText('Validation Results')).toBeVisible();

    await page.getByRole('button', { name: 'Import Library' }).click();

    // Assert - mock returns completed immediately so progressbar may be brief
    // Just verify the import completes successfully
    await expect(page.getByText('Import Complete', { exact: true })).toBeVisible({ timeout: 10000 });
  });
});
