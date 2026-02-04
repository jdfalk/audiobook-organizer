// file: tests/e2e/itunes-import.spec.ts
// version: 1.0.0
// guid: 8d0eb913-029f-42f1-ad7b-984aa66a6fdc

import { test, expect } from '@playwright/test';
import {
  mockEventSource,
  setupMockApi,
  setupPhase1ApiDriven,
} from './utils/test-helpers';

test.describe('iTunes Import', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
  });

  test('validates iTunes library', async ({ page }) => {
    // Arrange
    await setupMockApi(page);

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
    // Arrange
    await setupMockApi(page);

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
    await expect(page.getByText('Import Complete')).toBeVisible();
  });
});
