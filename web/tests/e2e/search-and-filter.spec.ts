// file: web/tests/e2e/search-and-filter.spec.ts
// version: 1.1.2
// guid: c3d4e5f6-a7b8-9012-cdef-a3b4c5d6e7f8

import { test, expect } from '@playwright/test';
import {
  mockEventSource,
  skipWelcomeWizard,
  setupLibraryWithBooks,
  generateTestBooks,
} from './utils/test-helpers';

test.describe('Search and Filter Functionality', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await skipWelcomeWizard(page);
  });

  test('searches books by exact title match', async ({ page }) => {
    // GIVEN: Library has book titled "The Way of Kings"
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'The Way of Kings',
        author_name: 'Brandon Sanderson',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "The Way of Kings" in search box
    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('The Way of Kings');
    await page.waitForTimeout(400); // Debounce

    // THEN: Shows only books matching that title
    await expect(
      page.getByRole('heading', { name: 'The Way of Kings', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).not.toBeVisible();
  });

  test('searches books by partial title match', async ({ page }) => {
    // GIVEN: Library has books with "Foundation" in title
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Foundation',
        author_name: 'Isaac Asimov',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Foundation and Empire',
        author_name: 'Isaac Asimov',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-3',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "Found" in search box
    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('Found');
    await page.waitForTimeout(400);

    // THEN: Shows all books with "Found" in title
    await expect(
      page.getByRole('heading', { name: 'Foundation', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Foundation and Empire', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).not.toBeVisible();
  });

  test('searches books by author name', async ({ page }) => {
    // GIVEN: Library has books by multiple authors
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'The Way of Kings',
        author_name: 'Brandon Sanderson',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-3',
        title: 'Words of Radiance',
        author_name: 'Brandon Sanderson',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "Sanderson" in search
    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('Sanderson');
    await page.waitForTimeout(400);

    // THEN: Shows all books by authors matching "Sanderson"
    await expect(
      page.getByRole('heading', { name: 'The Way of Kings', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Words of Radiance', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).not.toBeVisible();
  });

  test('searches books by series name', async ({ page }) => {
    // GIVEN: Library has "Stormlight Archive" series
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'The Way of Kings',
        author_name: 'Brandon Sanderson',
        series_name: 'The Stormlight Archive',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Words of Radiance',
        author_name: 'Brandon Sanderson',
        series_name: 'The Stormlight Archive',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-3',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
        series_name: null,
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "Stormlight" in search box
    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('Stormlight');
    await page.waitForTimeout(400);

    // THEN: Shows all books in series matching "Stormlight"
    await expect(
      page.getByRole('heading', { name: 'The Way of Kings', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Words of Radiance', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).not.toBeVisible();
  });

  test('search is case-insensitive', async ({ page }) => {
    // GIVEN: Library has "The Hobbit"
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "the hobbit" (lowercase) in search
    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('the hobbit');
    await page.waitForTimeout(400);

    // THEN: Shows "The Hobbit" book
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).toBeVisible();
  });

  test('shows no results message when search matches nothing', async ({
    page,
  }) => {
    // GIVEN: Library loaded
    const books = generateTestBooks(5);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "zzznonexistent" in search
    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('zzznonexistent');
    await page.waitForTimeout(400);

    // THEN: Shows "No audiobooks found" message
    await expect(page.getByText(/no audiobooks found/i)).toBeVisible();
  });

  test('clears search with clear button', async ({ page }) => {
    // GIVEN: User has typed "Foundation" in search
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Foundation',
        author_name: 'Isaac Asimov',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('Foundation');
    await page.waitForTimeout(400);

    // AND: Results are filtered
    await expect(
      page.getByRole('heading', { name: 'Foundation', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).not.toBeVisible();

    // WHEN: User clicks "X" (clear search) button
    await page
      .locator('button', {
        has: page.locator('svg[data-testid="ClearIcon"]'),
      })
      .click();

    // THEN: Search input is cleared
    await expect(searchInput).toHaveValue('');

    // AND: All books are shown again
    await expect(
      page.getByRole('heading', { name: 'Foundation', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).toBeVisible();
  });

  test('clears search with backspace to empty', async ({ page }) => {
    // GIVEN: User has typed "Foundation" in search
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Foundation',
        author_name: 'Isaac Asimov',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    const searchInput = page.getByPlaceholder(/search audiobooks/i);
    await searchInput.fill('Foundation');
    await page.waitForTimeout(400);

    // Verify filtered
    await expect(
      page.getByRole('heading', { name: 'Foundation', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).not.toBeVisible();

    // WHEN: User backspaces to empty string
    await searchInput.clear();
    await page.waitForTimeout(400);

    // THEN: All books are shown again
    await expect(
      page.getByRole('heading', { name: 'Foundation', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'The Hobbit', exact: true })
    ).toBeVisible();
  });

  test.skip('search works with other filters combined', async ({ page }) => {
    // TODO: Filters drawer needs author/state filters wired into data load.
    const books = generateTestBooks(2);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');
  });

  test.skip('search persists across page navigation', async ({ page }) => {
    // TODO: Persist search term in URL or global state.
    const books = generateTestBooks(2);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');
  });

  test.skip('search updates URL with query parameter', async ({ page }) => {
    // TODO: Add query param sync for search term.
    const books = generateTestBooks(2);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');
  });

  test.skip('search debounces input to avoid excessive requests', async ({
    page,
  }) => {
    // TODO: Track search API requests for debounce verification.
    const books = generateTestBooks(2);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');
  });
});
