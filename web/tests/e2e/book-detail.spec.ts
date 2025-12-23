// file: tests/e2e/book-detail.spec.ts
// version: 1.2.0
// guid: 2a3b4c5d-6e7f-8a9b-0c1d-2e3f4a5b6c7d

import { expect, test } from '@playwright/test';

const bookId = 'book-1';

type BookState = {
  id: string;
  title: string;
  author_name: string;
  file_path: string;
  file_hash: string;
  original_file_hash: string;
  organized_file_hash: string;
  library_state: string;
  marked_for_deletion: boolean;
  marked_for_deletion_at?: string;
  created_at: string;
  updated_at: string;
};

const createInitialBook = (): BookState => ({
  id: bookId,
  title: 'The Test Book',
  author_name: 'Jane Tester',
  file_path: '/library/test-book.m4b',
  file_hash: 'hash-file',
  original_file_hash: 'hash-original',
  organized_file_hash: 'hash-organized',
  library_state: 'organized',
  marked_for_deletion: false,
  created_at: new Date('2024-01-01T12:00:00Z').toISOString(),
  updated_at: new Date('2024-01-02T12:00:00Z').toISOString(),
});

const mockEventSource = async (page: import('@playwright/test').Page) => {
  await page.addInitScript(() => {
    class MockEventSource {
      url: string;
      constructor(url: string) {
        this.url = url;
      }
      addEventListener() {}
      removeEventListener() {}
      close() {}
    }
    (window as unknown as { EventSource: typeof EventSource }).EventSource =
      MockEventSource as unknown as typeof EventSource;
  });
};

const setupRoutes = async (page: import('@playwright/test').Page) => {
  const initialBook = createInitialBook();
  await page.addInitScript(
    ({ bookId: injectedBookId, bookData }: { bookId: string; bookData: BookState }) => {
      let book = { ...bookData };
      let purged = false;

      const jsonResponse = (body: unknown, status = 200) =>
        new Response(JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        });

      const originalFetch = window.fetch.bind(window);
      window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : input.url;
        const method = (init?.method || 'GET').toUpperCase();

        // Health/system
        if (url.includes('/api/v1/health')) {
          return Promise.resolve(jsonResponse({ status: 'ok' }));
        }
        if (url.includes('/api/v1/system/status')) {
          return Promise.resolve(
            jsonResponse({
              status: 'ok',
              library: { book_count: 1, folder_count: 1, total_size: 0 },
              import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
              memory: {},
              runtime: {},
              operations: { recent: [] },
            })
          );
        }

        // Book detail + list
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}`)) {
          if (method === 'GET') {
            if (purged) {
              return Promise.resolve(jsonResponse({}, 404));
            }
            return Promise.resolve(jsonResponse(book));
          }
          if (method === 'DELETE') {
            const softDelete = url.includes('soft_delete=true');
            if (softDelete) {
              book = {
                ...book,
                library_state: 'deleted',
                marked_for_deletion: true,
                marked_for_deletion_at: new Date().toISOString(),
              };
            } else {
              purged = true;
            }
            return Promise.resolve(jsonResponse({}));
          }
        }

        if (url.endsWith('/api/v1/audiobooks')) {
          return Promise.resolve(jsonResponse({ items: [book], audiobooks: [book] }));
        }

        // Versions
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/versions`)) {
          return Promise.resolve(
            jsonResponse({
              versions: [
                { ...book, is_primary_version: true },
                { ...book, id: 'book-2', title: 'Second Version', is_primary_version: false },
              ],
            })
          );
        }

        // Restore
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/restore`)) {
          book = {
            ...book,
            library_state: 'organized',
            marked_for_deletion: false,
            marked_for_deletion_at: undefined,
          };
          return Promise.resolve(jsonResponse({}));
        }

        // Metadata refresh
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/fetch-metadata`)) {
          book = { ...book, title: 'Refreshed Title' };
          return Promise.resolve(jsonResponse({ message: 'refreshed', book, source: 'test' }));
        }

        // AI parse
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/parse-with-ai`)) {
          book = { ...book, description: 'AI parsed desc' };
          return Promise.resolve(jsonResponse({ message: 'parsed', book, confidence: 'high' }));
        }

        // Fallback
        return originalFetch(input, init);
      };
    },
    { bookId, bookData: initialBook }
  );

  return {
    // Simple accessor for assertions if needed later
    getBook: () => initialBook,
  };
};

test.describe('Book Detail page', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await page.addInitScript(() => {
      localStorage.setItem('welcome_wizard_completed', 'true');
    });
  });

  test('renders info, files, and versions tabs', async ({ page }) => {
    await setupRoutes(page);
    await page.goto(`/library/${bookId}`);

    await expect(page.getByRole('heading', { name: 'The Test Book' })).toBeVisible();
    await page.getByRole('tab', { name: 'Files' }).click();
    await expect(page.getByText('File Hash')).toBeVisible();

    await page.getByRole('tab', { name: /Versions/ }).click();
    await expect(page.getByText(/Versions/).first()).toBeVisible();
    await expect(
      page.getByText(/Second Version|No additional versions linked yet/i)
    ).toBeVisible();
  });

  test('soft delete, restore, and purge flow', async ({ page }) => {
    const state = await setupRoutes(page);
    await page.goto(`/library/${bookId}`);

    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Soft Delete' }).click();

    await expect(page.getByText('Audiobook marked for deletion.')).toBeVisible();
    await expect(page.getByText('Soft Deleted')).toBeVisible();

    await page.getByRole('button', { name: /^Restore$/ }).last().click();
    await expect(page.getByText('Audiobook restored.')).toBeVisible();
    await expect(page.getByText('Soft Deleted')).not.toBeVisible();

    // Soft delete again to open purge dialog
    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Soft Delete' }).click();
    await page.getByRole('button', { name: 'Purge' }).click();
    await expect(page.getByRole('dialog', { name: 'Purge Audiobook' })).toBeVisible();
    await page.getByRole('button', { name: 'Purge Permanently' }).click();
    await expect(page).toHaveURL(/\/library$/);
  });

  test('metadata refresh and AI parse actions', async ({ page }) => {
    await setupRoutes(page);
    await page.goto(`/library/${bookId}`);

    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await expect(page.getByRole('heading', { name: 'Refreshed Title' })).toBeVisible();

    await page.getByRole('button', { name: 'Parse with AI' }).click();
    await expect(page.getByText('AI parsed desc')).toBeVisible();
  });
});
