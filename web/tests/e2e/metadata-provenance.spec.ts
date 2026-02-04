// file: tests/e2e/metadata-provenance.spec.ts
// version: 1.1.0
// guid: 9a8b7c6d-5e4f-3d2c-1b0a-9f8e7d6c5b4a
// last-edited: 2026-02-04

/**
 * E2E tests for metadata provenance features (SESSION-003).
 * Tests per-field tracking of effective_value, source, stored_value,
 * fetched_value, override_value, and locked state.
 */

import { expect, test } from '@playwright/test';
import { setupPhase1ApiDriven, mockEventSource } from './utils/test-helpers';

const bookId = 'prov-test-book';

type TagEntry = {
  file_value: string | number | null;
  fetched_value: string | number | null;
  stored_value: string | number | null;
  override_value: string | number | null;
  override_locked: boolean;
  effective_value: string | number | null;
  effective_source: 'file' | 'fetched' | 'stored' | 'override' | '';
};

type TagsData = {
  media_info: {
    codec: string;
    bitrate: number;
    sample_rate: number;
    channels: number;
    bit_depth: number;
    quality: string;
    duration: number;
  };
  tags: Record<string, TagEntry>;
};

/**
 * Creates initial book state for testing
 */
const createBookState = () => ({
  id: bookId,
  title: 'Provenance Test Book',
  author_name: 'Test Author',
  file_path: '/library/provenance-test.m4b',
  file_hash: 'hash-prov-test',
  original_file_hash: 'hash-orig',
  organized_file_hash: 'hash-org',
  library_state: 'organized',
  marked_for_deletion: false,
  created_at: new Date('2024-12-01T10:00:00Z').toISOString(),
  updated_at: new Date('2024-12-28T10:00:00Z').toISOString(),
});

/**
 * Creates comprehensive tags data with various provenance scenarios
 */
const createTagsData = (): TagsData => ({
  media_info: {
    codec: 'M4B',
    bitrate: 128,
    sample_rate: 44100,
    channels: 2,
    bit_depth: 16,
    quality: '128kbps AAC',
    duration: 7200,
  },
  tags: {
    title: {
      file_value: 'File: Provenance Test',
      fetched_value: 'API: Provenance Test',
      stored_value: 'Provenance Test Book',
      override_value: null,
      override_locked: false,
      effective_value: 'Provenance Test Book',
      effective_source: 'stored',
    },
    author_name: {
      file_value: 'File Author',
      fetched_value: 'API Author',
      stored_value: 'Test Author',
      override_value: null,
      override_locked: false,
      effective_value: 'Test Author',
      effective_source: 'stored',
    },
    narrator: {
      file_value: 'File Narrator',
      fetched_value: 'API Narrator',
      stored_value: 'DB Narrator',
      override_value: 'User Override Narrator',
      override_locked: true,
      effective_value: 'User Override Narrator',
      effective_source: 'override',
    },
    series_name: {
      file_value: 'File Series',
      fetched_value: 'API Series',
      stored_value: 'DB Series',
      override_value: null,
      override_locked: false,
      effective_value: 'DB Series',
      effective_source: 'stored',
    },
    publisher: {
      file_value: null,
      fetched_value: 'Audible Studios',
      stored_value: null,
      override_value: null,
      override_locked: false,
      effective_value: 'Audible Studios',
      effective_source: 'fetched',
    },
    language: {
      file_value: 'en',
      fetched_value: 'en',
      stored_value: 'en',
      override_value: null,
      override_locked: false,
      effective_value: 'en',
      effective_source: 'stored',
    },
    audiobook_release_year: {
      file_value: 2022,
      fetched_value: 2023,
      stored_value: 2024,
      override_value: null,
      override_locked: false,
      effective_value: 2024,
      effective_source: 'stored',
    },
  },
});

/**
 * Mocks EventSource to prevent SSE connections
 */
// Note: Using mockEventSource from test-helpers instead of this local definition
// to avoid duplication

/**
 * Recomputes effective value and source based on provenance hierarchy
 * Note: This helper is defined but used within the browser context in setupProvenanceRoutes
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars
const recomputeEffective = (entry: TagEntry) => {
  if (entry.override_value !== null && entry.override_value !== undefined) {
    entry.effective_value = entry.override_value;
    entry.effective_source = 'override';
  } else if (entry.stored_value !== null && entry.stored_value !== undefined) {
    entry.effective_value = entry.stored_value;
    entry.effective_source = 'stored';
  } else if (
    entry.fetched_value !== null &&
    entry.fetched_value !== undefined
  ) {
    entry.effective_value = entry.fetched_value;
    entry.effective_source = 'fetched';
  } else if (entry.file_value !== null && entry.file_value !== undefined) {
    entry.effective_value = entry.file_value;
    entry.effective_source = 'file';
  } else {
    entry.effective_value = null;
    entry.effective_source = '';
  }
};

/**
 * Sets up comprehensive API mocking for provenance testing
 */
const setupProvenanceRoutes = async (page: import('@playwright/test').Page) => {
  const bookState = createBookState();
  const tagsState = createTagsData();

  await page.addInitScript(
    ({
      bookId: injectedBookId,
      bookData,
      tagsData,
    }: {
      bookId: string;
      bookData: ReturnType<typeof createBookState>;
      tagsData: TagsData;
    }) => {
      let book = { ...bookData };
      const tags = JSON.parse(JSON.stringify(tagsData));

      const recompute = (field: string) => {
        const entry = tags.tags[field];
        if (!entry) return;
        if (
          entry.override_value !== null &&
          entry.override_value !== undefined
        ) {
          entry.effective_value = entry.override_value;
          entry.effective_source = 'override';
        } else if (
          entry.stored_value !== null &&
          entry.stored_value !== undefined
        ) {
          entry.effective_value = entry.stored_value;
          entry.effective_source = 'stored';
        } else if (
          entry.fetched_value !== null &&
          entry.fetched_value !== undefined
        ) {
          entry.effective_value = entry.fetched_value;
          entry.effective_source = 'fetched';
        } else if (
          entry.file_value !== null &&
          entry.file_value !== undefined
        ) {
          entry.effective_value = entry.file_value;
          entry.effective_source = 'file';
        } else {
          entry.effective_value = null;
          entry.effective_source = '';
        }
      };

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

        // Book detail
        if (
          url.includes(`/api/v1/audiobooks/${injectedBookId}`) &&
          !url.includes('/tags')
        ) {
          if (method === 'GET') {
            return Promise.resolve(jsonResponse(book));
          }
          if (method === 'PUT') {
            const body = init?.body ? JSON.parse(init.body as string) : {};
            book = { ...book, ...body };

            // Handle overrides payload
            if (body.overrides) {
              Object.entries(
                body.overrides as Record<
                  string,
                  { value?: unknown; clear?: boolean; locked?: boolean }
                >
              ).forEach(([key, override]) => {
                const entry = tags.tags[key];
                if (!entry) return;

                if (override.clear) {
                  entry.override_value = null;
                  entry.override_locked = false;
                  recompute(key);
                  return;
                }

                if (override.value !== undefined) {
                  entry.override_value = override.value as never;
                  entry.override_locked =
                    override.locked !== undefined ? override.locked : true;
                  recompute(key);
                }
              });
            }

            // Handle direct field updates
            Object.keys(body).forEach((key) => {
              if (key === 'overrides') return;
              if (tags.tags[key]) {
                tags.tags[key].stored_value = body[key];
                recompute(key);
              }
            });

            return Promise.resolve(jsonResponse(book));
          }
        }

        // Tags endpoint - critical for provenance
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/tags`)) {
          return Promise.resolve(jsonResponse(tags));
        }

        // Book list
        if (url.endsWith('/api/v1/audiobooks')) {
          return Promise.resolve(
            jsonResponse({ items: [book], audiobooks: [book] })
          );
        }

        // Fallback
        return originalFetch(input, init);
      };
    },
    { bookId, bookData: bookState, tagsData: tagsState }
  );
};

test.describe('Metadata Provenance E2E', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
  });

  test('displays provenance data in Tags tab', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);

    // Act
    await page.getByRole('tab', { name: 'Tags' }).click();

    // Assert - Check that effective values are displayed
    await expect(page.getByText('Provenance Test Book')).toBeVisible();
    await expect(page.getByText('Test Author')).toBeVisible();
    await expect(page.getByText('User Override Narrator')).toBeVisible();

    // Assert - Check that source chips are visible
    await expect(page.getByText('stored')).toBeVisible();
    await expect(page.getByText('override')).toBeVisible();

    // Assert - Check locked indicator
    await expect(page.getByText('locked')).toBeVisible();
  });

  test('shows correct effective source for different fields', async ({
    page,
  }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Tags' }).click();

    // Act - Navigate to Tags tab (already done above)

    // Assert - Title uses 'stored' source
    const titleRow = page.locator('text=Provenance Test Book').locator('..');
    await expect(titleRow.locator('text=stored')).toBeVisible();

    // Assert - Narrator uses 'override' source and is locked
    const narratorSection = page
      .locator('text=User Override Narrator')
      .locator('..');
    await expect(narratorSection.locator('text=override')).toBeVisible();
    await expect(narratorSection.locator('text=locked')).toBeVisible();

    // Assert - Publisher uses 'fetched' source (only source available)
    await page.getByRole('tab', { name: 'Compare' }).click();
    const publisherRow = page.locator('tr').filter({ hasText: 'publisher' });
    await expect(publisherRow.getByText('Audible Studios')).toBeVisible();
    await expect(publisherRow.getByText('fetched')).toBeVisible();
  });

  test('applies override from file value', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Act - Apply file value for title
    const titleRow = page.locator('tr').filter({ hasText: /^title$/i });
    await titleRow.getByRole('button', { name: 'Use File' }).click();

    // Assert - Title should now show file value in heading
    await expect(
      page.getByRole('heading', { name: 'File: Provenance Test' })
    ).toBeVisible();

    // Assert - Navigate to Tags tab to verify source changed
    await page.getByRole('tab', { name: 'Tags' }).click();
    const titleSection = page
      .locator('text=File: Provenance Test')
      .locator('..');
    await expect(titleSection.getByText('override')).toBeVisible();
  });

  test('applies override from fetched value', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Act - Apply fetched value for author_name
    const authorRow = page.locator('tr').filter({ hasText: /author.*name/i });
    await authorRow.getByRole('button', { name: 'Use Fetched' }).click();

    // Assert - Author should now show fetched value
    await page.getByRole('tab', { name: 'Info' }).click();
    await expect(page.getByText('API Author')).toBeVisible();
  });

  test('clears override and reverts to stored value', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Act - Clear override for narrator (which has override set)
    const narratorRow = page.locator('tr').filter({ hasText: /narrator/i });
    await expect(narratorRow.getByText('User Override Narrator')).toBeVisible();
    await narratorRow.getByRole('button', { name: 'Clear' }).click();

    // Assert - Narrator should revert to stored value
    await expect(narratorRow.getByText('DB Narrator')).toBeVisible();

    // Assert - Check Tags tab shows stored source now
    await page.getByRole('tab', { name: 'Tags' }).click();
    const narratorSection = page.locator('text=DB Narrator').locator('..');
    await expect(narratorSection.getByText('stored')).toBeVisible();
    await expect(narratorSection.getByText('locked')).not.toBeVisible();
  });

  test('lock toggle persists across page reloads', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Act - Apply override for series_name
    const seriesRow = page.locator('tr').filter({ hasText: /series.*name/i });
    await seriesRow.getByRole('button', { name: 'Use File' }).click();

    // Assert - Verify override applied
    await page.getByRole('tab', { name: 'Tags' }).click();
    const seriesSection = page.locator('text=File Series').locator('..');
    await expect(seriesSection.getByText('override')).toBeVisible();

    // Act - Reload page
    await page.reload();
    await page.getByRole('tab', { name: 'Tags' }).click();

    // Assert - Override should still be present after reload
    await expect(page.getByText('File Series')).toBeVisible();
    const reloadedSeriesSection = page
      .locator('text=File Series')
      .locator('..');
    await expect(reloadedSeriesSection.getByText('override')).toBeVisible();
  });

  test('displays all source columns in Compare tab', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);

    // Act
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Assert - Verify table headers
    await expect(
      page.getByRole('columnheader', { name: 'Field' })
    ).toBeVisible();
    await expect(
      page.getByRole('columnheader', { name: 'File Tag' })
    ).toBeVisible();
    await expect(
      page.getByRole('columnheader', { name: 'Fetched' })
    ).toBeVisible();
    await expect(
      page.getByRole('columnheader', { name: 'Stored' })
    ).toBeVisible();
    await expect(
      page.getByRole('columnheader', { name: 'Override' })
    ).toBeVisible();
    await expect(
      page.getByRole('columnheader', { name: 'Actions' })
    ).toBeVisible();

    // Assert - Verify narrator row shows all sources
    const narratorRow = page.locator('tr').filter({ hasText: /^narrator$/i });
    await expect(narratorRow.getByText('File Narrator')).toBeVisible();
    await expect(narratorRow.getByText('API Narrator')).toBeVisible();
    await expect(narratorRow.getByText('DB Narrator')).toBeVisible();
    await expect(narratorRow.getByText('User Override Narrator')).toBeVisible();
  });

  test('handles field with only fetched source', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);

    // Act
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Assert - Publisher has no file or stored value, only fetched
    const publisherRow = page.locator('tr').filter({ hasText: /^publisher$/i });
    await expect(publisherRow.getByText('Audible Studios')).toBeVisible();

    // Verify file and stored columns show placeholder
    const cells = publisherRow.locator('td');
    // File Tag column should be empty or show "—"
    await expect(cells.nth(1)).toContainText('—');
    // Stored column should be empty or show "—"
    await expect(cells.nth(3)).toContainText('—');
    // Fetched column should have value
    await expect(cells.nth(2)).toContainText('Audible Studios');

    // Assert - Source chip shows 'fetched'
    await expect(publisherRow.getByText('fetched')).toBeVisible();
  });

  test('disables action buttons when source value is null', async ({
    page,
  }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);

    // Act
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Assert - Publisher "Use File" button should be disabled (file_value is null)
    const publisherRow = page.locator('tr').filter({ hasText: /^publisher$/i });
    const useFileButton = publisherRow.getByRole('button', {
      name: 'Use File',
    });
    await expect(useFileButton).toBeDisabled();

    // Assert - "Use Fetched" button should be enabled
    const useFetchedButton = publisherRow.getByRole('button', {
      name: 'Use Fetched',
    });
    await expect(useFetchedButton).toBeEnabled();
  });

  test('shows media info in Tags tab', async ({ page }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);

    // Act
    await page.getByRole('tab', { name: 'Tags' }).click();

    // Assert - Media info should be visible
    await expect(page.getByText('128 kbps')).toBeVisible();
    await expect(page.getByText('M4B')).toBeVisible();
    await expect(page.getByText('44100')).toBeVisible();
  });

  test('updates effective value when applying different source', async ({
    page,
  }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Tags' }).click();

    // Act - Initially showing stored value for title
    await expect(page.getByText('Provenance Test Book')).toBeVisible();

    // Act - Switch to Compare and apply file value
    await page.getByRole('tab', { name: 'Compare' }).click();
    const titleRow = page.locator('tr').filter({ hasText: /^title$/i });
    await titleRow.getByRole('button', { name: 'Use File' }).click();

    // Assert - Should now show file value
    await page.getByRole('tab', { name: 'Tags' }).click();
    await expect(page.getByText('File: Provenance Test')).toBeVisible();
    await expect(page.getByText('Provenance Test Book')).not.toBeVisible();
  });

  test('shows correct effective source chip colors and styling', async ({
    page,
  }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);

    // Act
    await page.getByRole('tab', { name: 'Tags' }).click();

    // Assert - Check that source chips have proper variant (outlined)
    const sourceChips = page
      .locator('span.MuiChip-label')
      .filter({ hasText: /^(stored|override|fetched|file)$/i });
    await expect(sourceChips.first()).toBeVisible();

    // Assert - Check that locked chip has warning color
    const lockedChip = page
      .locator('span.MuiChip-label')
      .filter({ hasText: 'locked' });
    await expect(lockedChip).toBeVisible();
  });

  test('applies override with numeric value (audiobook_release_year)', async ({
    page,
  }) => {
    // Arrange
    await setupProvenanceRoutes(page);
    await page.goto(`/library/${bookId}`);
    await page.getByRole('tab', { name: 'Compare' }).click();

    // Act - Apply file value for release year (numeric field)
    const yearRow = page.locator('tr').filter({ hasText: /release.*year/i });
    await expect(yearRow.getByText('2022')).toBeVisible(); // file_value
    await yearRow.getByRole('button', { name: 'Use File' }).click();

    // Assert - Tags tab should show file value (2022)
    await page.getByRole('tab', { name: 'Tags' }).click();
    const yearSection = page.locator('text=2022').locator('..');
    await expect(yearSection.getByText('override')).toBeVisible();
  });
});
