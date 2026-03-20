// file: tests/e2e/files-history.spec.ts
// version: 1.1.0
// guid: bd99e21f-38d1-4976-8ac2-43060c5fc17a

import { expect, test } from '@playwright/test';
import { mockEventSource } from './utils/test-helpers';

const bookId = 'book-fh-1';

const createBook = (overrides: Record<string, unknown> = {}) => ({
  id: bookId,
  title: 'Files History Test Book',
  author_name: 'Test Author',
  file_path: '/library/test-book.m4b',
  file_hash: 'hash-1',
  original_file_hash: 'hash-orig',
  organized_file_hash: 'hash-org',
  library_state: 'organized',
  format: 'm4b',
  codec: 'AAC',
  bitrate: 128,
  duration: 7200,
  file_size: 52428800,
  marked_for_deletion: false,
  is_primary_version: true,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-02T00:00:00Z',
  ...overrides,
});

const createVersions = () => [
  createBook({ is_primary_version: true }),
  {
    ...createBook({
      id: 'book-fh-2',
      title: 'MP3 Version',
      format: 'mp3',
      codec: 'MP3',
      bitrate: 192,
      duration: 7200,
      file_size: 78643200,
      file_path: '/library/test-book-mp3/chapter01.mp3',
      is_primary_version: false,
    }),
  },
];

const createTags = () => ({
  media_info: { codec: 'AAC', bitrate: 128, sample_rate: 44100, channels: 2 },
  tags: {
    title: {
      file_value: 'Files History Test Book',
      stored_value: 'Files History Test Book',
      effective_value: 'Files History Test Book',
      effective_source: 'stored',
    },
    author_name: {
      file_value: 'Test Author',
      stored_value: 'Test Author',
      effective_value: 'Test Author',
      effective_source: 'stored',
    },
    narrator: {
      file_value: null,
      stored_value: null,
      effective_value: null,
      effective_source: '',
    },
    series_name: {
      file_value: null,
      stored_value: null,
      effective_value: null,
      effective_source: '',
    },
    publisher: {
      file_value: 'Test Publisher',
      stored_value: 'Test Publisher',
      effective_value: 'Test Publisher',
      effective_source: 'stored',
    },
    language: {
      file_value: 'en',
      stored_value: 'en',
      effective_value: 'en',
      effective_source: 'stored',
    },
    isbn13: {
      file_value: null,
      stored_value: null,
      effective_value: null,
      effective_source: '',
    },
  },
});

const createChangelog = () => ({
  entries: [
    {
      timestamp: '2025-01-02T12:00:00Z',
      type: 'tag_write',
      summary: 'Tags written — title, author',
    },
    {
      timestamp: '2025-01-01T10:00:00Z',
      type: 'import',
      summary: 'Imported from /imports/test-book.m4b',
    },
  ],
});

const setupRoutes = async (page: import('@playwright/test').Page) => {
  await mockEventSource(page);

  await page.addInitScript(() => {
    localStorage.setItem('welcome_wizard_completed', 'true');
  });

  const book = createBook();
  const versions = createVersions();
  const tags = createTags();
  const changelog = createChangelog();

  await page.addInitScript(
    ({
      bookId: injectedBookId,
      bookData,
      versionsData,
      tagsData,
      changelogData,
    }: {
      bookId: string;
      bookData: typeof book;
      versionsData: typeof versions;
      tagsData: typeof tags;
      changelogData: typeof changelog;
    }) => {
      const jsonResponse = (body: unknown, status = 200) =>
        new Response(JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        });

      const originalFetch = window.fetch.bind(window);
      window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : input.url;

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

        // Changelog
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/changelog`)) {
          return Promise.resolve(jsonResponse(changelogData));
        }

        // Tags (with optional compare_id or snapshot_ts)
        if (url.includes('/tags')) {
          const hasCompare = url.includes('compare_id=') || url.includes('snapshot_ts=');
          if (hasCompare) {
            // Add comparison_value to each tag
            const compTags = JSON.parse(JSON.stringify(tagsData));
            for (const key of Object.keys(compTags.tags)) {
              compTags.tags[key].comparison_value = `Compared ${key}`;
            }
            return Promise.resolve(jsonResponse(compTags));
          }
          return Promise.resolve(jsonResponse(tagsData));
        }

        // Versions
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/versions`)) {
          return Promise.resolve(jsonResponse({ versions: versionsData }));
        }

        // Segments
        if (url.includes('/segments')) {
          return Promise.resolve(jsonResponse([]));
        }

        // External IDs
        if (url.includes('/external-ids')) {
          return Promise.resolve(
            jsonResponse({ itunes_linked: false, total: 0, ids: [] })
          );
        }

        // Book detail
        if (
          url.includes(`/api/v1/audiobooks/${injectedBookId}`) &&
          !url.includes('/tags') &&
          !url.includes('/versions') &&
          !url.includes('/changelog') &&
          !url.includes('/segments') &&
          !url.includes('/external-ids')
        ) {
          return Promise.resolve(jsonResponse(bookData));
        }

        // Book list
        if (url.endsWith('/api/v1/audiobooks')) {
          return Promise.resolve(
            jsonResponse({ items: [bookData], count: 1 })
          );
        }

        return originalFetch(input, init);
      };
    },
    {
      bookId,
      bookData: book,
      versionsData: versions,
      tagsData: tags,
      changelogData: changelog,
    }
  );
};

test.describe('Files & History tab', () => {
  test.beforeEach(async ({ page }) => {
    await setupRoutes(page);
    await page.goto(`/library/${bookId}?tab=files`);
  });

  test('tab shows "Files & History" label', async ({ page }) => {
    const tab = page.getByRole('tab', { name: /files & history/i });
    await expect(tab).toBeVisible();
  });

  test('format trays render grouped by format', async ({ page }) => {
    // Should have M4B and MP3 format trays
    const m4bTray = page.locator('[data-testid="format-tray-m4b"]');
    const mp3Tray = page.locator('[data-testid="format-tray-mp3"]');

    await expect(m4bTray).toBeVisible();
    await expect(mp3Tray).toBeVisible();

    // M4B tray should show Primary badge
    await expect(m4bTray.getByText('Primary')).toBeVisible();

    // MP3 tray should show format info
    await expect(mp3Tray.getByText(/MP3/)).toBeVisible();
  });

  test('tag comparison toggle and dropdown work', async ({ page }) => {
    // Expand the M4B format tray
    const m4bTray = page.locator('[data-testid="format-tray-m4b"]');
    await m4bTray.click();

    // Wait for tag comparison to load
    const toggle = page.getByTestId('tag-comparison-toggle').first();
    await expect(toggle).toBeVisible();

    // Should show key tag badges
    await expect(page.getByText(/\u2713 title/i).first()).toBeVisible();

    // Click to expand full comparison
    await toggle.click();

    // Tag table should now be visible
    await expect(page.getByText('File Value').first()).toBeVisible();
    await expect(page.getByText('DB Value').first()).toBeVisible();
  });

  test('change log section renders', async ({ page }) => {
    // Changelog section should be visible
    const changelogSection = page.getByTestId('changelog-section');
    await expect(changelogSection).toBeVisible();
    await expect(changelogSection.getByText('Change Log')).toBeVisible();

    // Should show timeline entries
    const timeline = page.getByTestId('changelog-timeline');
    await expect(timeline).toBeVisible();

    // Should have entries for tag_write and import
    await expect(page.getByText(/Tags written/)).toBeVisible();
    await expect(page.getByText(/Imported from/)).toBeVisible();

    // tag_write entry should have "Compare snapshot" link
    await expect(page.getByText(/Compare snapshot/)).toBeVisible();
  });
});
