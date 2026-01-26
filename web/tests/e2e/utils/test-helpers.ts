// file: web/tests/e2e/utils/test-helpers.ts
// version: 1.0.4
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { Page } from '@playwright/test';

/**
 * Mock EventSource to prevent SSE connections during tests
 */
export async function mockEventSource(page: Page) {
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
}

/**
 * Skip welcome wizard
 */
export async function skipWelcomeWizard(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('welcome_wizard_completed', 'true');
  });
}

/**
 * Setup common routes for all tests
 */
export async function setupCommonRoutes(page: Page) {
  await page.route('**/api/v1/health', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    });
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
}

/**
 * Wait for toast notification
 */
export async function waitForToast(
  page: Page,
  text: string,
  timeout = 5000
) {
  await page.waitForSelector(`text=${text}`, { timeout });
}

/**
 * Generate test audiobooks
 */
export function generateTestBooks(count: number) {
  const authors = ['Brandon Sanderson', 'J.R.R. Tolkien', 'Terry Pratchett', 'Isaac Asimov', 'Ursula K. Le Guin'];
  const series = ['The Stormlight Archive', 'The Lord of the Rings', 'Discworld', 'Foundation', 'Earthsea'];

  return Array.from({ length: count }, (_, i) => ({
    id: `book-${i + 1}`,
    title: `Test Book ${i + 1}`,
    author_name: authors[i % authors.length],
    series_name: i % 3 === 0 ? series[i % series.length] : null,
    series_position: i % 3 === 0 ? (i % 5) + 1 : null,
    library_state: i % 4 === 0 ? 'import' : 'organized',
    marked_for_deletion: i % 10 === 0,
    file_path: `/library/book${i + 1}.m4b`,
    file_hash: `hash-${i + 1}`,
    original_file_hash: `hash-orig-${i + 1}`,
    organized_file_hash: i % 4 !== 0 ? `hash-org-${i + 1}` : null,
    created_at: new Date(2024, 0, i + 1).toISOString(),
    updated_at: new Date(2024, 11, i + 1).toISOString(),
    duration: 3600 + (i * 100),
    file_size: 100000000 + (i * 1000000),
  }));
}

/**
 * Generate test audiobook with full metadata
 */
export function generateTestBook(overrides: Record<string, unknown> = {}) {
  return {
    id: 'test-book-1',
    title: 'The Way of Kings',
    author_name: 'Brandon Sanderson',
    narrator: 'Michael Kramer, Kate Reading',
    series_name: 'The Stormlight Archive',
    series_position: 1,
    publisher: 'Tor Books',
    audiobook_release_year: 2010,
    language: 'en',
    isbn: '9780765326355',
    description: 'Epic fantasy novel',
    genre: 'Fantasy',
    library_state: 'organized',
    marked_for_deletion: false,
    file_path: '/library/Brandon Sanderson/The Stormlight Archive/The Way of Kings.m4b',
    file_hash: 'hash-twok',
    original_file_hash: 'hash-orig-twok',
    organized_file_hash: 'hash-org-twok',
    created_at: '2024-01-01T12:00:00Z',
    updated_at: '2024-12-01T12:00:00Z',
    duration: 45600,
    file_size: 450000000,
    ...overrides,
  };
}

/**
 * Setup library page with mock books
 */
export async function setupLibraryWithBooks(
  page: Page,
  books: ReturnType<typeof generateTestBooks>
) {
  await page.addInitScript(
    ({ bookData }: { bookData: typeof books }) => {
      let libraryBooks = [...bookData];

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

        const applySearchFilter = (items: typeof libraryBooks, query: string) => {
          if (!query) return items;
          const searchLower = query.toLowerCase();
          return items.filter((b) => {
            const title = b.title?.toLowerCase() || '';
            const author = b.author_name?.toLowerCase() || '';
            const series = b.series_name?.toLowerCase() || '';
            return (
              title.includes(searchLower) ||
              author.includes(searchLower) ||
              series.includes(searchLower)
            );
          });
        };

        // Health/system
        if (pathname === '/api/v1/health') {
          return Promise.resolve(jsonResponse({ status: 'ok' }));
        }
        if (pathname === '/api/v1/operations/active' && method === 'GET') {
          return Promise.resolve(jsonResponse({ operations: [] }));
        }
        if (pathname === '/api/v1/system/status') {
          return Promise.resolve(
            jsonResponse({
              status: 'ok',
              library: {
                book_count: libraryBooks.length,
                folder_count: 1,
                total_size: libraryBooks.reduce((sum, b) => sum + (b.file_size || 0), 0),
              },
              import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
              memory: {},
              runtime: {},
              operations: { recent: [] },
            })
          );
        }

        if (pathname === '/api/v1/import-paths' && method === 'GET') {
          return Promise.resolve(jsonResponse({ importPaths: [] }));
        }

        if (pathname === '/api/v1/audiobooks/count' && method === 'GET') {
          return Promise.resolve(jsonResponse({ count: libraryBooks.length }));
        }

        if (pathname === '/api/v1/audiobooks/soft-deleted' && method === 'GET') {
          const deleted = libraryBooks.filter((book) => book.marked_for_deletion);
          return Promise.resolve(
            jsonResponse({
              items: deleted,
              total: deleted.length,
              count: deleted.length,
            })
          );
        }

        if (pathname === '/api/v1/audiobooks/search' && method === 'GET') {
          const query =
            urlObj.searchParams.get('q') ||
            urlObj.searchParams.get('search') ||
            '';
          const limit = parseInt(urlObj.searchParams.get('limit') || '50');
          const filtered = applySearchFilter([...libraryBooks], query);
          const paginated = filtered.slice(0, limit);
          return Promise.resolve(
            jsonResponse({
              items: paginated,
              audiobooks: paginated,
              total: filtered.length,
            })
          );
        }

        if (pathname.startsWith('/api/v1/audiobooks/') && method === 'GET') {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 1];
          if (bookId && bookId !== 'audiobooks') {
            const match = libraryBooks.find((book) => book.id === bookId);
            if (!match) {
              return Promise.resolve(
                jsonResponse({ error: 'Not found' }, 404)
              );
            }
            return Promise.resolve(jsonResponse(match));
          }
        }

        // Audiobooks list
        if (pathname === '/api/v1/audiobooks' && method === 'GET') {
          const limit = parseInt(urlObj.searchParams.get('limit') || '24');
          const offset = parseInt(urlObj.searchParams.get('offset') || '0');
          const page = parseInt(urlObj.searchParams.get('page') || '1');

          const effectiveOffset =
            offset > 0 ? offset : Math.max(0, (page - 1) * limit);

          const paginatedBooks = libraryBooks.slice(
            effectiveOffset,
            effectiveOffset + limit
          );

          return Promise.resolve(
            jsonResponse({
              items: paginatedBooks,
              audiobooks: paginatedBooks,
              total: libraryBooks.length,
              page,
              limit,
            })
          );
        }

        // Fallback
        return originalFetch(input, init);
      };
    },
    { bookData: books }
  );
}
