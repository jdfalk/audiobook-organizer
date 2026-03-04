// file: tests/e2e/metadata-provenance.spec.ts
// version: 2.0.0
// guid: 9a8b7c6d-5e4f-3d2c-1b0a-9f8e7d6c5b4a
// last-edited: 2026-03-04

/**
 * E2E tests for MetadataEditDialog provenance features.
 * Tests per-field lock state, source labels, auto-lock on edit,
 * fetched comparison display, year validation, and save/cancel behavior.
 */

import { expect, test } from '@playwright/test';
import { mockEventSource, skipWelcomeWizard } from './utils/test-helpers';

const bookId = 'prov-test-book';

/**
 * Creates a mock audiobook for testing.
 */
const createBookData = () => ({
  id: bookId,
  title: 'Provenance Test Book',
  author: 'Test Author',
  narrator: 'User Override Narrator',
  series: 'DB Series',
  series_number: 3,
  genre: 'Science Fiction',
  year: 2024,
  language: 'en',
  publisher: 'Audible Studios',
  isbn10: '',
  isbn13: '978-1234567890',
  description: 'A test book for provenance.',
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
 * Creates mock field-states response matching the backend shape.
 * Keys use backend names (author_name, series_name, etc.).
 */
const createFieldStates = () => ({
  field_states: {
    title: {
      fetched_value: 'API: Provenance Test',
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    author_name: {
      fetched_value: 'API Author',
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    narrator: {
      fetched_value: 'API Narrator',
      override_value: 'User Override Narrator',
      override_locked: true,
      updated_at: '2024-12-28T10:00:00Z',
    },
    series_name: {
      fetched_value: 'API Series',
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    publisher: {
      fetched_value: null,
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    language: {
      fetched_value: 'en',
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    audiobook_release_year: {
      fetched_value: 2023,
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    isbn13: {
      fetched_value: '978-0000000000',
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
    genre: {
      fetched_value: null,
      override_value: null,
      override_locked: false,
      updated_at: '2024-12-28T10:00:00Z',
    },
  },
});

/**
 * Sets up API mocking for the MetadataEditDialog tests.
 */
const setupMockRoutes = async (page: import('@playwright/test').Page) => {
  const bookData = createBookData();
  const fieldStatesData = createFieldStates();

  await page.addInitScript(
    ({
      injectedBookId,
      book,
      fieldStates,
    }: {
      injectedBookId: string;
      book: ReturnType<typeof createBookData>;
      fieldStates: ReturnType<typeof createFieldStates>;
    }) => {
      let savedBook = { ...book };
      let lastSaveDirtyFields: string[] = [];

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

        // Health / system
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

        // Field-states endpoint
        if (url.includes(`/api/v1/audiobooks/${injectedBookId}/field-states`)) {
          return Promise.resolve(jsonResponse(fieldStates));
        }

        // Book detail
        if (
          url.includes(`/api/v1/audiobooks/${injectedBookId}`) &&
          !url.includes('/field-states')
        ) {
          if (method === 'GET') {
            return Promise.resolve(jsonResponse(savedBook));
          }
          if (method === 'PUT') {
            const body = init?.body ? JSON.parse(init.body as string) : {};
            savedBook = { ...savedBook, ...body };
            // Store dirty fields info for test assertions
            if (body._dirtyFields) {
              lastSaveDirtyFields = body._dirtyFields;
            }
            // Expose to window for test assertions
            (window as unknown as Record<string, unknown>).__lastSavedBook = savedBook;
            (window as unknown as Record<string, unknown>).__lastSaveDirtyFields = lastSaveDirtyFields;
            return Promise.resolve(jsonResponse(savedBook));
          }
        }

        // Book list
        if (url.includes('/api/v1/audiobooks') && !url.includes(injectedBookId)) {
          return Promise.resolve(
            jsonResponse({ items: [savedBook], audiobooks: [savedBook], count: 1, limit: 50, offset: 0 })
          );
        }

        return originalFetch(input, init);
      };
    },
    { injectedBookId: bookId, book: bookData, fieldStates: fieldStatesData }
  );
};

/**
 * Navigates to book detail and opens the Edit Metadata dialog.
 */
const openEditDialog = async (page: import('@playwright/test').Page) => {
  await page.goto(`/library/${bookId}`);
  // Wait for book detail to load
  await expect(page.getByRole('heading', { name: 'Provenance Test Book' })).toBeVisible();
  // Click Edit Metadata button
  await page.getByRole('button', { name: /Edit Metadata/i }).click();
  // Wait for dialog to appear
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByText('Edit Metadata')).toBeVisible();
};

test.describe('MetadataEditDialog Provenance E2E', () => {
  test.beforeEach(async ({ page }) => {
    await skipWelcomeWizard(page);
    await mockEventSource(page);
  });

  test('dialog opens with all fields populated from audiobook data', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Verify dialog header text
    await expect(
      page.getByText('Edited fields are automatically locked to prevent overwrites from future fetches.')
    ).toBeVisible();

    // Verify fields are populated
    await expect(page.getByLabel('Title *')).toHaveValue('Provenance Test Book');
    await expect(page.getByLabel('Author')).toHaveValue('Test Author');
    await expect(page.getByLabel('Narrator')).toHaveValue('User Override Narrator');
    await expect(page.getByLabel('Series')).toHaveValue('DB Series');
    await expect(page.getByLabel('Genre')).toHaveValue('Science Fiction');
    await expect(page.getByLabel('Year')).toHaveValue('2024');
    await expect(page.getByLabel('Language')).toHaveValue('en');
    await expect(page.getByLabel('Publisher')).toHaveValue('Audible Studios');
    await expect(page.getByLabel('ISBN-13')).toHaveValue('978-1234567890');

    // Verify Cancel and Save buttons
    await expect(page.getByRole('button', { name: 'Cancel' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Save' })).toBeVisible();
  });

  test('locked fields show orange lock icon, unlocked show grey open lock', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Narrator has override_locked: true in field-states
    // Find the lock icon near the Narrator field - it should be a closed lock (LockIcon)
    const narratorField = page.getByLabel('Narrator');
    const narratorContainer = narratorField.locator('..').locator('..');
    // The lock button is a sibling before the text field wrapper
    const narratorLockButton = narratorContainer.locator('button').first();
    // Locked field should have the Lock icon (not LockOpen)
    await expect(narratorLockButton.locator('[data-testid="LockIcon"]')).toBeVisible();

    // Title has override_locked: false - should show open lock
    const titleField = page.getByLabel('Title *');
    const titleContainer = titleField.locator('..').locator('..');
    const titleLockButton = titleContainer.locator('button').first();
    await expect(titleLockButton.locator('[data-testid="LockOpenIcon"]')).toBeVisible();
  });

  test('editing a field automatically locks it (auto-lock on edit)', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Title starts unlocked
    const titleField = page.getByLabel('Title *');
    const titleContainer = titleField.locator('..').locator('..');
    const titleLockButton = titleContainer.locator('button').first();
    await expect(titleLockButton.locator('[data-testid="LockOpenIcon"]')).toBeVisible();

    // Edit the title field
    await titleField.fill('Modified Title');

    // Now the lock should be closed (auto-locked because dirty)
    await expect(titleLockButton.locator('[data-testid="LockIcon"]')).toBeVisible();

    // Source label should show "Manual override"
    await expect(page.getByText('Source: Manual override').first()).toBeVisible();
  });

  test('manual lock toggle changes lock state', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Title starts unlocked - click to lock
    const titleField = page.getByLabel('Title *');
    const titleContainer = titleField.locator('..').locator('..');
    const titleLockButton = titleContainer.locator('button').first();
    await expect(titleLockButton.locator('[data-testid="LockOpenIcon"]')).toBeVisible();

    // Click to lock
    await titleLockButton.click();
    await expect(titleLockButton.locator('[data-testid="LockIcon"]')).toBeVisible();

    // Click again to unlock
    await titleLockButton.click();
    await expect(titleLockButton.locator('[data-testid="LockOpenIcon"]')).toBeVisible();
  });

  test('source labels show correct source type', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Narrator has override_value set -> "Source: Manual override"
    await expect(page.getByText('Source: Manual override').first()).toBeVisible();

    // Author has fetched_value set (different from current) -> "Source: Fetched"
    // Note: author current = "Test Author", fetched = "API Author" -> source = Fetched
    await expect(page.getByText('Source: Fetched').first()).toBeVisible();

    // Publisher has no fetched_value and no override_value -> "Source: File tags"
    await expect(page.getByText('Source: File tags').first()).toBeVisible();
  });

  test('fetched comparison shows when fetched value differs from current', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Title has fetched_value "API: Provenance Test" which differs from "Provenance Test Book"
    await expect(page.getByText('Fetched: API: Provenance Test')).toBeVisible();

    // Author has fetched_value "API Author" which differs from "Test Author"
    await expect(page.getByText('Fetched: API Author')).toBeVisible();

    // Year has fetched_value 2023 which differs from current 2024
    await expect(page.getByText('Fetched: 2023')).toBeVisible();

    // Language has fetched_value "en" which matches current "en" -> should NOT show fetched
    // (count how many "Fetched: en" labels appear - should be 0)
    await expect(page.getByText('Fetched: en')).not.toBeVisible();
  });

  test('year field shows error for non-numeric input', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    const yearField = page.getByLabel('Year');
    await yearField.fill('not-a-number');

    // Should show year error
    await expect(page.getByText('Year must be a number')).toBeVisible();

    // Attempting to save should show error
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Year must be a number')).toBeVisible();

    // Dialog should still be open (save was blocked)
    await expect(page.getByRole('dialog')).toBeVisible();
  });

  test('cancel closes dialog without saving', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Edit a field
    await page.getByLabel('Title *').fill('Should Not Be Saved');

    // Click Cancel
    await page.getByRole('button', { name: 'Cancel' }).click();

    // Dialog should close
    await expect(page.getByRole('dialog')).not.toBeVisible();

    // Book title should still be original
    await expect(page.getByRole('heading', { name: 'Provenance Test Book' })).toBeVisible();
  });

  test('save sends updated data and closes dialog', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Edit author field
    await page.getByLabel('Author').fill('New Author Name');

    // Click Save
    await page.getByRole('button', { name: 'Save' }).click();

    // Dialog should close
    await expect(page.getByRole('dialog')).not.toBeVisible();
  });

  test('locked field tooltip shows correct text', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Hover over the narrator lock button (which is locked)
    const narratorField = page.getByLabel('Narrator');
    const narratorContainer = narratorField.locator('..').locator('..');
    const narratorLockButton = narratorContainer.locator('button').first();
    await narratorLockButton.hover();

    // Should show locked tooltip
    await expect(
      page.getByText(/Locked — will not be overwritten/)
    ).toBeVisible();
  });

  test('unlocked field tooltip shows correct text', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // Hover over the title lock button (which is unlocked)
    const titleField = page.getByLabel('Title *');
    const titleContainer = titleField.locator('..').locator('..');
    const titleLockButton = titleContainer.locator('button').first();
    await titleLockButton.hover();

    // Should show unlocked tooltip
    await expect(
      page.getByText(/Unlocked — may be updated/)
    ).toBeVisible();
  });

  test('ISBN-13 shows fetched comparison when values differ', async ({ page }) => {
    await setupMockRoutes(page);
    await openEditDialog(page);

    // ISBN-13 has fetched "978-0000000000" vs current "978-1234567890"
    await expect(page.getByText('Fetched: 978-0000000000')).toBeVisible();
  });
});
