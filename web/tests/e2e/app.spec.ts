// file: tests/e2e/app.spec.ts
// version: 1.0.0
// guid: 1f2a3b4c-5d6e-7f8a-9b0c-1d2e3f4a5b6c

import { test, expect } from '@playwright/test';

const mockApi = async (page: import('@playwright/test').Page) => {
  await page.addInitScript(() => {
    // Avoid real SSE connections during tests
    class MockEventSource {
      url: string;
      constructor(url: string) {
        this.url = url;
      }
      addEventListener() {}
      removeEventListener() {}
      close() {}
    }
    // @ts-ignore
    window.EventSource = MockEventSource;
  });

  await page.route('**/api/v1/system/status', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        status: 'ok',
        library: { book_count: 0, folder_count: 1, total_size: 0 },
        import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
        memory: {},
        runtime: {},
        operations: { recent: [] },
      }),
    });
  });

  await page.route('**/api/v1/import-paths', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ importPaths: [] }),
    });
  });

  await page.route('**/api/v1/audiobooks', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ audiobooks: [] }),
    });
  });

  await page.route('**/api/v1/health', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    });
  });

  await page.route('**/api/**', (route) => {
    // Default fallback for any other API call
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({}),
    });
  });
};

test.describe('App smoke', () => {
  test.beforeEach(async ({ page }) => {
    await mockApi(page);
    await page.addInitScript(() => {
      localStorage.setItem('welcome_wizard_completed', 'true');
    });
  });

  test('loads dashboard and shows title', async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByText('Audiobook Organizer', { exact: true }).first()
    ).toBeVisible();
    await expect(page.getByText('Dashboard', { exact: true }).first()).toBeVisible();
  });

  test('shows import path empty state on Library page', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByText('Library', { exact: true }).first().click();
    await expect(page).toHaveURL(/.*\/library/);
  });
});
