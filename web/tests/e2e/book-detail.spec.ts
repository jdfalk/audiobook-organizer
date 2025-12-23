// file: tests/e2e/book-detail.spec.ts
// version: 1.0.0
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
  let book: BookState = createInitialBook();
  let purged = false;

  await page.route(`**/api/v1/audiobooks/${bookId}`, (route) => {
    const method = route.request().method();
    const url = new URL(route.request().url());
    if (method === 'GET') {
      if (purged) {
        return route.fulfill({ status: 404, body: '{}' });
      }
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(book),
      });
    }
    if (method === 'DELETE') {
      const softDelete = url.searchParams.get('soft_delete') === 'true';
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
      return route.fulfill({ status: 200, body: '{}' });
    }
    return route.fulfill({ status: 200, body: '{}' });
  });

  await page.route(`**/api/v1/audiobooks/${bookId}/versions`, (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        versions: [
          { ...book, is_primary_version: true },
          {
            ...book,
            id: 'book-2',
            title: 'Second Version',
            is_primary_version: false,
          },
        ],
      }),
    });
  });

  await page.route(`**/api/v1/audiobooks/${bookId}/restore`, (route) => {
    book = {
      ...book,
      library_state: 'organized',
      marked_for_deletion: false,
      marked_for_deletion_at: undefined,
    };
    route.fulfill({ status: 200, body: '{}' });
  });

  await page.route(`**/api/v1/audiobooks/${bookId}/fetch-metadata`, (route) => {
    book = { ...book, title: 'Refreshed Title' };
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ message: 'refreshed', book, source: 'test' }),
    });
  });

  await page.route(`**/api/v1/audiobooks/${bookId}/parse-with-ai`, (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        message: 'parsed',
        book: { ...book, description: 'AI parsed desc' },
        confidence: 'high',
      }),
    });
  });

  await page.route('**/api/v1/system/status', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        status: 'ok',
        library: { book_count: 1, folder_count: 1, total_size: 0 },
        import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
        memory: {},
        runtime: {},
        operations: { recent: [] },
      }),
    });
  });

  await page.route('**/api/v1/health', (route) => {
    route.fulfill({ status: 200, body: JSON.stringify({ status: 'ok' }) });
  });

  await page.route('**/api/**', (route) => {
    route.fulfill({ status: 200, body: '{}' });
  });

  return {
    getBook: () => book,
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
    await expect(page.getByText('Versions')).toBeVisible();
    await expect(page.getByText('Second Version')).toBeVisible();
  });

  test('soft delete, restore, and purge flow', async ({ page }) => {
    const state = await setupRoutes(page);
    await page.goto(`/library/${bookId}`);

    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Soft Delete' }).click();

    await expect(page.getByText('Audiobook marked for deletion.')).toBeVisible();
    await expect(page.getByText('Soft Deleted')).toBeVisible();

    await page.getByRole('button', { name: 'Restore' }).click();
    await expect(page.getByText('Audiobook restored.')).toBeVisible();
    await expect(page.getByText('Soft Deleted')).not.toBeVisible();

    // Soft delete again to open purge dialog
    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Soft Delete' }).click();
    await page.getByRole('button', { name: 'Purge' }).click();
    await expect(page.getByRole('dialog', { name: 'Purge Audiobook' })).toBeVisible();
    await page.getByRole('button', { name: 'Purge Permanently' }).click();
    await expect(page).toHaveURL(/\/library$/);
    expect(state.getBook().marked_for_deletion).toBeTruthy();
  });

  test('metadata refresh and AI parse actions', async ({ page }) => {
    await setupRoutes(page);
    await page.goto(`/library/${bookId}`);

    await page.getByRole('button', { name: 'Fetch Metadata' }).click();
    await expect(page.getByText(/Metadata refreshed|refreshed/)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Refreshed Title' })).toBeVisible();

    await page.getByRole('button', { name: 'Parse with AI' }).click();
    await expect(page.getByText(/AI parsing completed|parsed/)).toBeVisible();
  });
});
