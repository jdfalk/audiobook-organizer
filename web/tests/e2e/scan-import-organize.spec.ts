// file: tests/e2e/scan-import-organize.spec.ts
// version: 1.0.0
// guid: 6a7b8c9d-0e1f-2a3b-4c5d-6e7f8a9b0c1d

import { test, expect, type Page } from '@playwright/test';
import {
  mockEventSource,
  setupCommonRoutes,
  skipWelcomeWizard,
  waitForToast,
} from './utils/test-helpers';

type ScanMockOptions = {
  scanBooks: Array<Record<string, unknown>>;
  scanError?: boolean;
};

const setupScanWorkflow = async (page: Page, options: ScanMockOptions) => {
  await page.addInitScript(({ scanBooks, scanError }) => {
    let importPaths: Array<Record<string, unknown>> = [];
    let libraryBooks = [...scanBooks];

    const jsonResponse = (body: unknown, status = 200) =>
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      });

    const originalFetch = window.fetch.bind(window);
    window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.toString()
            : input.url;
      const method = (init?.method || 'GET').toUpperCase();
      const urlObj = new URL(url, window.location.origin);
      const pathname = urlObj.pathname;
      const body = typeof init?.body === 'string' ? init.body : '';
      const payload = body ? JSON.parse(body) : {};

      if (pathname === '/api/v1/import-paths' && method === 'GET') {
        return Promise.resolve(jsonResponse({ importPaths }));
      }
      if (pathname === '/api/v1/import-paths' && method === 'POST') {
        const newPath = {
          id: importPaths.length + 1,
          path: payload.path,
          name: payload.name,
          enabled: true,
          created_at: new Date().toISOString(),
          book_count: 0,
        };
        importPaths = [...importPaths, newPath];
        return Promise.resolve(jsonResponse({ importPath: newPath }));
      }
      if (
        pathname.startsWith('/api/v1/import-paths/') &&
        method === 'DELETE'
      ) {
        const id = Number(pathname.split('/').pop() || 0);
        importPaths = importPaths.filter((p) => p.id !== id);
        return Promise.resolve(jsonResponse({}));
      }
      if (pathname === '/api/v1/operations/scan' && method === 'POST') {
        if (scanError) {
          return Promise.resolve(jsonResponse({ error: 'Scan failed' }, 500));
        }
        libraryBooks = libraryBooks.map((book) => ({
          ...book,
          library_state: 'import',
        }));
        return Promise.resolve(
          jsonResponse({
            id: 'scan-op-1',
            type: 'scan',
            status: 'running',
            progress: 0,
            total: libraryBooks.length,
            message: 'Scanning',
            created_at: new Date().toISOString(),
          })
        );
      }
      if (pathname === '/api/v1/operations/organize' && method === 'POST') {
        const ids = Array.isArray(payload.book_ids) ? payload.book_ids : [];
        libraryBooks = libraryBooks.map((book) =>
          ids.includes(book.id)
            ? { ...book, library_state: 'organized' }
            : book
        );
        return Promise.resolve(
          jsonResponse({
            id: 'organize-op-1',
            type: 'organize',
            status: 'running',
            progress: 0,
            total: ids.length,
            message: 'Organizing',
            created_at: new Date().toISOString(),
          })
        );
      }
      if (pathname === '/api/v1/audiobooks/count' && method === 'GET') {
        return Promise.resolve(jsonResponse({ count: libraryBooks.length }));
      }
      if (pathname === '/api/v1/audiobooks/search' && method === 'GET') {
        const query = urlObj.searchParams.get('q') || '';
        const filtered = libraryBooks.filter((book) =>
          String(book.title || '')
            .toLowerCase()
            .includes(query.toLowerCase())
        );
        return Promise.resolve(
          jsonResponse({ items: filtered, audiobooks: filtered })
        );
      }
      if (pathname === '/api/v1/audiobooks' && method === 'GET') {
        return Promise.resolve(
          jsonResponse({ items: libraryBooks, audiobooks: libraryBooks })
        );
      }
      if (pathname === '/api/v1/system/status') {
        return Promise.resolve(
          jsonResponse({
            status: 'ok',
            library: {
              book_count: libraryBooks.length,
              folder_count: importPaths.length,
              total_size: 0,
            },
            import_paths: {
              book_count: libraryBooks.length,
              folder_count: importPaths.length,
              total_size: 0,
            },
            memory: {},
            runtime: {},
            operations: { recent: [] },
          })
        );
      }
      if (pathname === '/api/v1/operations/active' && method === 'GET') {
        return Promise.resolve(jsonResponse({ operations: [] }));
      }

      return originalFetch(input, init);
    };
  }, options);
};

test.describe('Scan/Import/Organize Workflow', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await skipWelcomeWizard(page);
    await setupCommonRoutes(page);
  });

  test('complete workflow: add import path → scan → organize', async ({
    page,
  }) => {
    // Arrange
    await setupScanWorkflow(page, {
      scanBooks: [
        {
          id: 'scan-1',
          title: 'Import Book 1',
          author_name: 'Test Author',
          file_path: '/test/audiobooks/book1.m4b',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
        {
          id: 'scan-2',
          title: 'Import Book 2',
          author_name: 'Test Author',
          file_path: '/test/audiobooks/book2.m4b',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
        {
          id: 'scan-3',
          title: 'Import Book 3',
          author_name: 'Test Author',
          file_path: '/test/audiobooks/book3.m4b',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
      ],
    });

    // Act: add import path and scan
    await page.goto('/settings');
    await page.getByRole('button', { name: 'Add Import Path' }).click();
    await page.getByLabel('Folder Path').fill('/test/audiobooks');
    await page.getByRole('button', { name: 'Add Path' }).click();
    await expect(page.getByText('/test/audiobooks')).toBeVisible();
    await page.getByRole('button', { name: 'Scan' }).click();

    // Assert: scan progress and completion
    await expect(page.getByText('Scanning...')).toBeVisible();
    await expect(page.getByText('Scan complete.')).toBeVisible();

    // Act: filter import books and organize
    await page.goto('/library');
    await page.getByRole('button', { name: 'Filter' }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Import' }).click();
    await expect(page.getByText('Import Book 1')).toBeVisible();
    await page.getByLabel('Select All').click();
    await page.getByRole('button', { name: 'Organize Selected' }).click();
    await page.getByRole('button', { name: 'Organize Selected' }).last().click();

    // Assert: organize progress and success
    await expect(page.getByText('Organized 3 of 3')).toBeVisible();
    await waitForToast(page, 'Successfully organized 3 audiobooks.');

    // Act: filter organized and confirm
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Organized' }).click();

    // Assert
    await expect(page.getByText('Import Book 1')).toBeVisible();

    // Act: verify import state is empty
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Import' }).click();

    // Assert
    await expect(page.getByText('No audiobooks found')).toBeVisible();
  });

  test('scan operation: start, monitor progress, complete', async ({
    page,
  }) => {
    // Arrange
    await setupScanWorkflow(page, { scanBooks: [] });
    await page.goto('/settings');
    await page.getByRole('button', { name: 'Add Import Path' }).click();
    await page.getByLabel('Folder Path').fill('/test/books');
    await page.getByRole('button', { name: 'Add Path' }).click();

    // Act
    await page.getByRole('button', { name: 'Scan' }).click();

    // Assert
    await expect(page.getByText('Scanning...')).toBeVisible();
    await expect(page.getByText('Scan complete.')).toBeVisible();
  });

  test('scan operation handles errors', async ({ page }) => {
    // Arrange
    await setupScanWorkflow(page, { scanBooks: [], scanError: true });
    await page.goto('/settings');
    await page.getByRole('button', { name: 'Add Import Path' }).click();
    await page.getByLabel('Folder Path').fill('/test/error-books');
    await page.getByRole('button', { name: 'Add Path' }).click();

    // Act
    await page.getByRole('button', { name: 'Scan' }).click();

    // Assert
    await expect(page.getByText('Scan failed.')).toBeVisible();
  });
});
