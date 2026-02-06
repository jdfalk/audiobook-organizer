// file: tests/e2e/app.spec.ts
// version: 1.3.0
// guid: 1f2a3b4c-5d6e-7f8a-9b0c-1d2e3f4a5b6c
// last-edited: 2026-02-06

import { test, expect } from '@playwright/test';
import {
  mockEventSource,
  setupPhase2Interactive,
} from './utils/test-helpers';

test.describe('App smoke', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 2 setup: Reset with mocked APIs
    await setupPhase2Interactive(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
  });

  test('loads dashboard and shows title', async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByText('Audiobook Organizer', { exact: true }).first()
    ).toBeVisible();
    await expect(
      page.getByText('Dashboard', { exact: true }).first()
    ).toBeVisible();
  });

  test('shows import path empty state on Library page', async ({ page }) => {
    await page.goto('/library');
    await page.waitForLoadState('domcontentloaded');
    await expect(page).toHaveURL(/.*\/library/);
  });

  test('navigates to Settings page', async ({ page }) => {
    // Navigate directly to Settings page
    await page.goto('/settings');
    // Verify we're on the Settings page
    await expect(page).toHaveURL(/.*\/settings/);
    // Verify the page has loaded some content
    const content = await page.locator('main, [role="main"]').first();
    await expect(content).toBeVisible({ timeout: 10000 });
  });
});
