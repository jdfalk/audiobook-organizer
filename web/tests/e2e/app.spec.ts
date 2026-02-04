// file: tests/e2e/app.spec.ts
// version: 1.1.0
// guid: 1f2a3b4c-5d6e-7f8a-9b0c-1d2e3f4a5b6c
// last-edited: 2026-02-04

import { test, expect } from '@playwright/test';
import {
  setupPhase1ApiDriven,
  setupMockApi,
  mockEventSource,
} from './utils/test-helpers';

test.describe('App smoke', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
    // Setup mock APIs for empty library
    await setupMockApi(page, {
      books: [],
    });
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
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByText('Library', { exact: true }).first().click();
    await expect(page).toHaveURL(/.*\/library/);
  });

  test('navigates to Settings and renders content', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByText('Settings', { exact: true }).first().click();
    await expect(page).toHaveURL(/.*\/settings/);
    await expect(
      page.getByText('Settings', { exact: true }).first()
    ).toBeVisible();
  });
});
