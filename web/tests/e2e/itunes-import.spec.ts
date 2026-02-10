// file: tests/e2e/itunes-import.spec.ts
// version: 1.2.0
// guid: 8d0eb913-029f-42f1-ad7b-984aa66a6fdc

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

    // Assert
    await expect(page.getByRole('progressbar')).toBeVisible();
    await expect(page.getByText('Import Complete', { exact: true })).toBeVisible({ timeout: 10000 });
  });
});
