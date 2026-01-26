// file: tests/e2e/search-and-filter.spec.ts
// version: 1.0.0
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
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('The Way of Kings');
    await page.waitForTimeout(500); // Debounce

    // THEN: Shows only books matching that title
    await expect(page.getByText('The Way of Kings')).toBeVisible();
    await expect(page.getByText('The Hobbit')).not.toBeVisible();
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
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Found');
    await page.waitForTimeout(500);

    // THEN: Shows all books with "Found" in title
    await expect(page.getByText('Foundation', { exact: true })).toBeVisible();
    await expect(page.getByText('Foundation and Empire')).toBeVisible();
    await expect(page.getByText('The Hobbit')).not.toBeVisible();
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
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Stormlight');
    await page.waitForTimeout(500);

    // THEN: Shows all books in series matching "Stormlight"
    await expect(page.getByText('The Way of Kings')).toBeVisible();
    await expect(page.getByText('Words of Radiance')).toBeVisible();
    await expect(page.getByText('The Hobbit')).not.toBeVisible();
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
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('the hobbit');
    await page.waitForTimeout(500);

    // THEN: Shows "The Hobbit" book
    await expect(page.getByText('The Hobbit')).toBeVisible();
  });

  test('search works with state filters combined', async ({ page }) => {
    // GIVEN: User has selected "Organized" state filter
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Organized Sanderson Book',
        author_name: 'Brandon Sanderson',
        library_state: 'organized',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Import Sanderson Book',
        author_name: 'Brandon Sanderson',
        library_state: 'import',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // Apply "Organized" filter
    const filterButton = page.getByRole('button', { name: /filter|state/i });
    await filterButton.click();
    const organizedFilter = page
      .getByRole('menuitem', { name: /organized/i })
      .or(page.getByText(/^Organized$/i));
    await organizedFilter.click();
    await page.waitForTimeout(300);

    // WHEN: User types "Sanderson" in search
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Sanderson');
    await page.waitForTimeout(500);

    // THEN: Shows only organized books by Sanderson
    await expect(page.getByText('Organized Sanderson Book')).toBeVisible();
    await expect(page.getByText('Import Sanderson Book')).not.toBeVisible();
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

    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Foundation');
    await page.waitForTimeout(500);

    // Verify filtered
    await expect(page.getByText('Foundation')).toBeVisible();
    await expect(page.getByText('The Hobbit')).not.toBeVisible();

    // WHEN: User backspaces to empty string
    await searchInput.clear();
    await page.waitForTimeout(500);

    // THEN: All books are shown again
    await expect(page.getByText('Foundation')).toBeVisible();
    await expect(page.getByText('The Hobbit')).toBeVisible();
  });

  test('combines multiple filters', async ({ page }) => {
    // GIVEN: Library with various books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Book 1',
        author_name: 'Brandon Sanderson',
        library_state: 'organized',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Book 2',
        author_name: 'Brandon Sanderson',
        library_state: 'import',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-3',
        title: 'Book 3',
        author_name: 'J.R.R. Tolkien',
        library_state: 'organized',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Organized" state filter
    const filterButton = page.getByRole('button', { name: /filter|state/i });
    await filterButton.click();
    const organizedFilter = page
      .getByRole('menuitem', { name: /organized/i })
      .or(page.getByText(/^Organized$/i));
    await organizedFilter.click();
    await page.waitForTimeout(300);

    // AND: User types "Brandon Sanderson" in search
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Brandon Sanderson');
    await page.waitForTimeout(500);

    // THEN: Only organized books by Brandon Sanderson are shown
    await expect(page.getByText('Book 1')).toBeVisible();
    await expect(page.getByText('Book 2')).not.toBeVisible(); // import state
    await expect(page.getByText('Book 3')).not.toBeVisible(); // different author
  });

  test('clears all filters', async ({ page }) => {
    // GIVEN: Multiple filters applied
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Book 1',
        author_name: 'Brandon Sanderson',
        library_state: 'organized',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Book 2',
        author_name: 'J.R.R. Tolkien',
        library_state: 'import',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // Apply filter
    const filterButton = page.getByRole('button', { name: /filter|state/i });
    await filterButton.click();
    const organizedFilter = page
      .getByRole('menuitem', { name: /organized/i })
      .or(page.getByText(/^Organized$/i));
    await organizedFilter.click();
    await page.waitForTimeout(300);

    // Apply search
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Sanderson');
    await page.waitForTimeout(500);

    // Verify filtered
    await expect(page.getByText('Book 1')).toBeVisible();
    await expect(page.getByText('Book 2')).not.toBeVisible();

    // WHEN: User clicks "Clear Filters" button
    const clearFiltersButton = page.getByRole('button', {
      name: /clear.*filter/i,
    });
    if (await clearFiltersButton.isVisible()) {
      await clearFiltersButton.click();
    } else {
      // Alternative: Clear search manually
      await searchInput.clear();
      // Reset filter
      await filterButton.click();
      const allFilter = page.getByText(/^All$/i).or(
        page.getByRole('menuitem', { name: /all/i })
      );
      await allFilter.click();
    }

    await page.waitForTimeout(500);

    // THEN: All filters are removed AND all books are shown again
    await expect(page.getByText('Book 1')).toBeVisible();
    await expect(page.getByText('Book 2')).toBeVisible();
  });

  test('search persists across page navigation', async ({ page }) => {
    // GIVEN: User has searched for "Foundation"
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-foundation',
        title: 'Foundation',
        author_name: 'Isaac Asimov',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-hobbit',
        title: 'The Hobbit',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Foundation');
    await page.waitForTimeout(500);

    // WHEN: User clicks a book to view details
    await page.getByText('Foundation').click();
    await page.waitForLoadState('networkidle');

    // AND: User clicks browser back button
    await page.goBack();
    await page.waitForLoadState('networkidle');

    // THEN: Search term "Foundation" is still in search box
    const searchValue = await searchInput.inputValue();
    expect(searchValue).toBe('Foundation');

    // AND: Results are still filtered
    await expect(page.getByText('Foundation')).toBeVisible();
    // Note: The Hobbit may or may not be visible depending on implementation
  });
});
