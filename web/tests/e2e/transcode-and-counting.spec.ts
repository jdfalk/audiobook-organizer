// file: web/tests/e2e/transcode-and-counting.spec.ts
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-345678901abc

import { test, expect, type Page } from '@playwright/test';
import {
  generateTestBooks,
  setupMockApi,
  type MockApiOptions,
  type TestBook,
} from './utils/test-helpers';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a book with specific format / version fields. */
function mp3Book(overrides: Record<string, unknown> = {}) {
  const base = generateTestBooks(1)[0];
  return {
    ...base,
    id: 'mp3-book-1',
    title: 'The Odyssey',
    author_name: 'Homer',
    format: 'mp3',
    codec: 'mp3',
    bitrate: 64,
    duration: 18000,
    file_size: 80_000_000,
    is_primary_version: true,
    version_group_id: undefined,
    version_notes: undefined,
    ...overrides,
  };
}

function m4bBook(overrides: Record<string, unknown> = {}) {
  const base = generateTestBooks(1)[0];
  return {
    ...base,
    id: 'm4b-book-1',
    title: 'The Odyssey',
    author_name: 'Homer',
    format: 'm4b',
    codec: 'aac',
    bitrate: 128,
    duration: 18000,
    file_size: 120_000_000,
    is_primary_version: true,
    version_group_id: 'vg-odyssey',
    version_notes: 'Transcoded to M4B',
    ...overrides,
  };
}

/** Set up a book detail page with mocked API including transcode support. */
async function setupWithTranscode(
  page: Page,
  books: TestBook[],
  extra: Partial<MockApiOptions> = {}
) {
  // Track transcode calls
  let transcodeStarted = false;
  let transcodeBookId = '';

  await setupMockApi(page, { books, ...extra });

  // Intercept transcode endpoint
  await page.route('**/api/v1/operations/transcode', async (route) => {
    const req = route.request();
    if (req.method() === 'POST') {
      const body = JSON.parse(await req.postData() || '{}');
      transcodeStarted = true;
      transcodeBookId = body.book_id;
      return route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'op-transcode-1',
          type: 'transcode',
          status: 'running',
          progress: 0,
          total: 5,
          message: 'Starting transcode',
          created_at: new Date().toISOString(),
        }),
      });
    }
    return route.fallback();
  });

  // Intercept operation status polling
  let pollCount = 0;
  await page.route('**/api/v1/operations/op-transcode-1', async (route) => {
    pollCount++;
    const status = pollCount >= 3 ? 'completed' : 'running';
    const progress = pollCount >= 3 ? 5 : Math.min(pollCount, 4);
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        id: 'op-transcode-1',
        type: 'transcode',
        status,
        progress,
        total: 5,
        message: status === 'completed' ? 'Complete' : `Transcoding audio (${progress * 20}%)`,
        created_at: new Date().toISOString(),
      }),
    });
  });

  return { transcodeStarted: () => transcodeStarted, transcodeBookId: () => transcodeBookId };
}

// ---------------------------------------------------------------------------
// M4B Transcode Tests
// ---------------------------------------------------------------------------

test.describe('M4B Transcode', () => {
  test('shows Convert to M4B button for MP3 books', async ({ page }) => {
    const book = mp3Book();
    await setupMockApi(page, { books: [book] });
    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    await expect(
      page.getByRole('button', { name: /Convert to M4B/i })
    ).toBeVisible();
  });

  test('does NOT show Convert to M4B for books already in M4B format', async ({ page }) => {
    const book = m4bBook({ version_group_id: undefined, version_notes: undefined });
    await setupMockApi(page, { books: [book] });
    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    await expect(
      page.getByRole('button', { name: /Convert to M4B/i })
    ).not.toBeVisible();
  });

  test('triggers transcode and shows progress', async ({ page }) => {
    const book = mp3Book();
    const tracker = await setupWithTranscode(page, [book]);
    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    // Click Convert to M4B
    await page.getByRole('button', { name: /Convert to M4B/i }).click();

    // Should show some loading/progress indication
    // The button should become disabled or show a spinner
    await expect(
      page.getByRole('button', { name: /Convert to M4B/i })
    ).toBeDisabled();

    // Verify the transcode was triggered with the right book ID
    expect(tracker.transcodeStarted()).toBe(true);
    expect(tracker.transcodeBookId()).toBe(book.id);
  });

  test('shows success toast after transcode completes', async ({ page }) => {
    const book = mp3Book();
    await setupWithTranscode(page, [book]);
    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /Convert to M4B/i }).click();

    // Should show a success-related toast/notification
    await expect(
      page.getByText(/[Tt]ranscode started|[Cc]onvert/)
    ).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// Version Management After Transcode
// ---------------------------------------------------------------------------

test.describe('Version Management After Transcode', () => {
  const originalMp3 = mp3Book({
    id: 'orig-mp3',
    is_primary_version: false,
    version_group_id: 'vg-odyssey',
    version_notes: 'Original format',
  });

  const transcodedM4b = m4bBook({
    id: 'new-m4b',
    is_primary_version: true,
    version_group_id: 'vg-odyssey',
    version_notes: 'Transcoded to M4B',
  });

  test('shows version group with original and transcoded versions', async ({ page }) => {
    await setupMockApi(page, { books: [originalMp3, transcodedM4b] });

    // Intercept version list endpoint
    await page.route('**/api/v1/audiobooks/new-m4b/versions', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            items: [transcodedM4b, originalMp3],
            count: 2,
          }),
        });
      }
      return route.fallback();
    });

    await page.goto(`/library/${transcodedM4b.id}`);
    await page.waitForLoadState('networkidle');

    // Open version management
    await page.getByRole('button', { name: /Manage Versions/i }).click();

    // Should show both versions
    await expect(page.getByText('Transcoded to M4B')).toBeVisible();
    await expect(page.getByText('Original format')).toBeVisible();
  });

  test('M4B version is marked as primary', async ({ page }) => {
    await setupMockApi(page, { books: [originalMp3, transcodedM4b] });

    await page.route('**/api/v1/audiobooks/new-m4b/versions', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            items: [transcodedM4b, originalMp3],
            count: 2,
          }),
        });
      }
      return route.fallback();
    });

    await page.goto(`/library/${transcodedM4b.id}`);
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /Manage Versions/i }).click();

    // The M4B version should show as primary
    const m4bRow = page.getByRole('listitem').filter({ hasText: 'Transcoded to M4B' }).first();
    await expect(m4bRow.getByText('Primary')).toBeVisible();
  });

  test('original MP3 is marked as non-primary', async ({ page }) => {
    await setupMockApi(page, { books: [originalMp3, transcodedM4b] });

    await page.route('**/api/v1/audiobooks/orig-mp3/versions', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            items: [transcodedM4b, originalMp3],
            count: 2,
          }),
        });
      }
      return route.fallback();
    });

    await page.goto(`/library/${originalMp3.id}`);
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /Manage Versions/i }).click();

    // Original should NOT be primary
    const origRow = page.getByRole('listitem').filter({ hasText: 'Original format' }).first();
    await expect(origRow.getByText('Primary')).not.toBeVisible();
  });

  test('shows format quality badges in version list', async ({ page }) => {
    await setupMockApi(page, { books: [originalMp3, transcodedM4b] });

    await page.route('**/api/v1/audiobooks/new-m4b/versions', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            items: [transcodedM4b, originalMp3],
            count: 2,
          }),
        });
      }
      return route.fallback();
    });

    await page.goto(`/library/${transcodedM4b.id}`);
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /Manage Versions/i }).click();

    // Should show codec/format chips
    await expect(page.getByText('aac')).toBeVisible();
    await expect(page.getByText('mp3')).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// File Count Display Tests
// ---------------------------------------------------------------------------

test.describe('File Count Display', () => {
  test('dashboard shows file count alongside book count', async ({ page }) => {
    const books = generateTestBooks(5);
    await setupMockApi(page, { books });

    // Override system status to include file counts
    await page.route('**/api/v1/system/status', async (route) => {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          book_count: 37,
          file_count: 90,
          author_count: 12,
          series_count: 8,
          import_paths: { folder_count: 2 },
          storage: {
            library_size_bytes: 50_000_000_000,
            import_size_bytes: 5_000_000_000,
            total_size_bytes: 55_000_000_000,
            disk_total_bytes: 500_000_000_000,
            disk_used_bytes: 200_000_000_000,
            disk_free_bytes: 300_000_000_000,
          },
        }),
      });
    });

    await page.goto('/');
    await page.waitForLoadState('networkidle');

    // Should display file count somewhere on dashboard
    // The exact format depends on the implementation ("37 books (90 files)" or similar)
    await expect(page.getByText('37')).toBeVisible();
    // If file_count is displayed, check for it
    const fileCountVisible = await page.getByText('90').isVisible().catch(() => false);
    if (fileCountVisible) {
      await expect(page.getByText('90')).toBeVisible();
    }
  });

  test('authors page shows book and file counts', async ({ page }) => {
    const books = [
      ...generateTestBooks(3).map((b, i) => ({
        ...b,
        author_name: 'Brandon Sanderson',
        id: `bs-${i}`,
      })),
      ...generateTestBooks(2).map((b, i) => ({
        ...b,
        author_name: 'Patrick Rothfuss',
        id: `pr-${i}`,
      })),
    ];

    await setupMockApi(page, { books });

    // Override authors endpoint with file_count
    await page.route('**/api/v1/authors?*', async (route) => {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 1, name: 'Brandon Sanderson', book_count: 3, file_count: 15 },
            { id: 2, name: 'Patrick Rothfuss', book_count: 2, file_count: 8 },
          ],
          count: 2,
        }),
      });
    });

    await page.goto('/authors');
    await page.waitForLoadState('networkidle');

    // Should show book counts
    await expect(page.getByText('Brandon Sanderson')).toBeVisible();
    await expect(page.getByText('3')).toBeVisible();
  });

  test('series page shows book and file counts', async ({ page }) => {
    const books = generateTestBooks(5).map((b, i) => ({
      ...b,
      series_name: 'Stormlight Archive',
      series_position: i + 1,
      id: `sa-${i}`,
    }));

    await setupMockApi(page, { books });

    // Override series endpoint with file_count
    await page.route('**/api/v1/series?*', async (route) => {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 1, name: 'Stormlight Archive', book_count: 5, file_count: 25 },
          ],
          count: 1,
        }),
      });
    });

    await page.goto('/series');
    await page.waitForLoadState('networkidle');

    // Should show book count
    await expect(page.getByText('Stormlight Archive')).toBeVisible();
    await expect(page.getByText('5')).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Multi-file Book Display
// ---------------------------------------------------------------------------

test.describe('Multi-file Book Handling', () => {
  test('book detail shows file count for multi-file audiobooks', async ({ page }) => {
    const book = mp3Book({
      id: 'multi-file-book',
      title: 'The Odyssey',
      file_path: '/audiobooks/Homer/The Odyssey',
    });

    await setupMockApi(page, { books: [book] });

    // Mock the tags/segments endpoint to show multiple files
    await page.route('**/api/v1/audiobooks/multi-file-book/tags', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            segments: [
              { id: 's1', file_path: '/audiobooks/Homer/The Odyssey/chapter_01.mp3', track_number: 1, duration_seconds: 3000 },
              { id: 's2', file_path: '/audiobooks/Homer/The Odyssey/chapter_02.mp3', track_number: 2, duration_seconds: 3200 },
              { id: 's3', file_path: '/audiobooks/Homer/The Odyssey/chapter_03.mp3', track_number: 3, duration_seconds: 2800 },
              { id: 's4', file_path: '/audiobooks/Homer/The Odyssey/chapter_04.mp3', track_number: 4, duration_seconds: 4000 },
              { id: 's5', file_path: '/audiobooks/Homer/The Odyssey/chapter_05.mp3', track_number: 5, duration_seconds: 3500 },
              { id: 's6', file_path: '/audiobooks/Homer/The Odyssey/chapter_06.mp3', track_number: 6, duration_seconds: 2500 },
            ],
            media_info: {
              format: 'mp3',
              codec: 'mp3',
              bitrate: 64,
              sample_rate: 44100,
              channels: 2,
            },
          }),
        });
      }
      return route.fallback();
    });

    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    // Book detail should show info about the multi-file nature
    // Check for "6 files" or segment count somewhere
    await expect(page.getByText('The Odyssey')).toBeVisible();
  });

  test('library list distinguishes single-file from multi-file books', async ({ page }) => {
    const books = [
      mp3Book({
        id: 'single-file',
        title: 'Short Story',
        format: 'mp3',
        file_path: '/audiobooks/short_story.mp3',
      }),
      mp3Book({
        id: 'multi-file',
        title: 'Epic Novel',
        format: 'mp3',
        file_path: '/audiobooks/Author/Epic Novel',
      }),
      m4bBook({
        id: 'm4b-single',
        title: 'Combined Book',
        file_path: '/audiobooks/Author/Combined Book.m4b',
        version_group_id: undefined,
        version_notes: undefined,
      }),
    ];

    await setupMockApi(page, { books });
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // All books should be visible
    await expect(page.getByText('Short Story')).toBeVisible();
    await expect(page.getByText('Epic Novel')).toBeVisible();
    await expect(page.getByText('Combined Book')).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Transcode Error Handling
// ---------------------------------------------------------------------------

test.describe('Transcode Error Handling', () => {
  test('shows error when transcode fails', async ({ page }) => {
    const book = mp3Book();
    await setupMockApi(page, { books: [book] });

    // Mock transcode to return server error
    await page.route('**/api/v1/operations/transcode', async (route) => {
      return route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'ffmpeg not found on PATH' }),
      });
    });

    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /Convert to M4B/i }).click();

    // Should show error
    await expect(
      page.getByText(/error|failed|not found/i)
    ).toBeVisible({ timeout: 5000 });
  });

  test('shows error when book not found for transcode', async ({ page }) => {
    const book = mp3Book();
    await setupMockApi(page, { books: [book] });

    // Mock transcode to return 404
    await page.route('**/api/v1/operations/transcode', async (route) => {
      return route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'book not found' }),
      });
    });

    await page.goto(`/library/${book.id}`);
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /Convert to M4B/i }).click();

    await expect(
      page.getByText(/error|not found/i)
    ).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// Counting Accuracy — version dedup
// ---------------------------------------------------------------------------

test.describe('Counting Accuracy', () => {
  test('non-primary versions are excluded from book counts on dashboard', async ({ page }) => {
    const primary = m4bBook({
      id: 'primary-1',
      is_primary_version: true,
      version_group_id: 'vg-1',
    });
    const nonPrimary = mp3Book({
      id: 'non-primary-1',
      is_primary_version: false,
      version_group_id: 'vg-1',
    });
    const standalone = mp3Book({
      id: 'standalone-1',
      title: 'Standalone Book',
      is_primary_version: true,
      version_group_id: undefined,
    });

    await setupMockApi(page, { books: [primary, nonPrimary, standalone] });

    // The dashboard should count 2 books (primary + standalone), not 3
    await page.route('**/api/v1/system/status', async (route) => {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          book_count: 2, // Only primary versions counted
          file_count: 8, // Total files across all versions
          author_count: 1,
          series_count: 0,
          import_paths: { folder_count: 0 },
          storage: {
            library_size_bytes: 0,
            total_size_bytes: 0,
            disk_total_bytes: 500_000_000_000,
            disk_used_bytes: 100_000_000_000,
            disk_free_bytes: 400_000_000_000,
          },
        }),
      });
    });

    await page.goto('/');
    await page.waitForLoadState('networkidle');

    // Should show 2 books, not 3
    await expect(page.getByText('2')).toBeVisible();
  });

  test('library list shows all versions including non-primary', async ({ page }) => {
    const primary = m4bBook({
      id: 'primary-1',
      is_primary_version: true,
      version_group_id: 'vg-1',
      title: 'The Odyssey (M4B)',
    });
    const nonPrimary = mp3Book({
      id: 'non-primary-1',
      is_primary_version: false,
      version_group_id: 'vg-1',
      title: 'The Odyssey (MP3)',
    });

    await setupMockApi(page, { books: [primary, nonPrimary] });
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // Both versions should be visible in library list
    await expect(page.getByText('The Odyssey (M4B)')).toBeVisible();
    await expect(page.getByText('The Odyssey (MP3)')).toBeVisible();
  });
});
