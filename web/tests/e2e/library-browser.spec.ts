// file: web/tests/e2e/library-browser.spec.ts
// version: 1.2.0
// guid: b2c3d4e5-f6a7-8901-bcde-f2a3b4c5d6e7

import { test, expect } from '@playwright/test';
import {
  mockEventSource,
  skipWelcomeWizard,
  generateTestBooks,
  setupLibraryWithBooks,
} from './utils/test-helpers';

test.describe('Library Browser', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);
    await skipWelcomeWizard(page);
  });

  test('loads library page and displays books in grid', async ({ page }) => {
    // GIVEN: Database has 25 audiobooks
    const books = generateTestBooks(25);
    await setupLibraryWithBooks(page, books);

    // WHEN: User navigates to /library
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // THEN: Grid displays books with title and author
    await expect(
      page.getByRole('heading', { name: 'Test Book 1', exact: true })
    ).toBeVisible();
    await expect(page.getByText('Brandon Sanderson').first()).toBeVisible();

    // AND: Shows pagination controls
    await expect(
      page.getByRole('button', { name: /page 2/i })
    ).toBeVisible();
  });

  test('switches between grid and list view', async ({ page }) => {
    // GIVEN: Library page is loaded
    const books = generateTestBooks(3);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User clicks "List View" button
    await page.getByRole('button', { name: /list view/i }).click();

    // THEN: Display changes to list layout
    await expect(page.getByRole('columnheader', { name: 'Title' })).toBeVisible();

    // WHEN: User clicks "Grid View" button
    await page.getByRole('button', { name: /grid view/i }).click();

    // THEN: Display changes back to grid layout
    await expect(
      page.getByRole('heading', { name: 'Test Book 1', exact: true })
    ).toBeVisible();
  });

  test('sorts books by title ascending', async ({ page }) => {
    // GIVEN: Library page with books
    const books = [
      { ...generateTestBooks(1)[0], id: 'book-1', title: 'Zebra Book' },
      { ...generateTestBooks(1)[0], id: 'book-2', title: 'Apple Book' },
      { ...generateTestBooks(1)[0], id: 'book-3', title: 'Mango Book' },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Title" from sort dropdown
    await page.getByLabel('Sort by').click();
    await page.getByRole('option', { name: 'Title' }).click();

    // THEN: Books are ordered alphabetically by title
    const titleLocator = page.locator('h2').filter({ hasText: /Book/ });
    const firstTitle = await titleLocator.first().textContent();
    expect(firstTitle).toContain('Apple Book');
  });

  test('sorts books by title descending', async ({ page }) => {
    // GIVEN: Library page with books
    const books = [
      { ...generateTestBooks(1)[0], id: 'book-1', title: 'Zebra Book' },
      { ...generateTestBooks(1)[0], id: 'book-2', title: 'Apple Book' },
      { ...generateTestBooks(1)[0], id: 'book-3', title: 'Mango Book' },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Title" and "Descending"
    await page.getByLabel('Sort by').click();
    await page.getByRole('option', { name: 'Title' }).click();
    await page.getByLabel('Order').click();
    await page.getByRole('option', { name: 'Descending' }).click();

    // THEN: Books are ordered reverse alphabetically
    const titleLocator = page.locator('h2').filter({ hasText: /Book/ });
    const firstTitle = await titleLocator.first().textContent();
    expect(firstTitle).toContain('Zebra Book');
  });

  test('sorts books by author', async ({ page }) => {
    // GIVEN: Library page with books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Book A',
        author_name: 'Zelda Author',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Book B',
        author_name: 'Alice Author',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-3',
        title: 'Book C',
        author_name: 'Mike Author',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Author" from sort dropdown
    await page.getByLabel('Sort by').click();
    await page.getByRole('option', { name: 'Author' }).click();

    // THEN: Books are ordered by author name
    const titleLocator = page.locator('h2').filter({ hasText: /Book/ });
    const firstTitle = await titleLocator.first().textContent();
    expect(firstTitle).toContain('Book B');
  });

  test('sorts books by date added', async ({ page }) => {
    // GIVEN: Library page with books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-old',
        title: 'Old Book',
        created_at: '2022-01-01T00:00:00Z',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-new',
        title: 'New Book',
        created_at: '2024-12-31T00:00:00Z',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Date Added" from sort dropdown
    await page.getByLabel('Sort by').click();
    await page.getByRole('option', { name: 'Date Added' }).click();

    // THEN: Books are reordered by created_at (newest first)
    const titleLocator = page.locator('h2').filter({ hasText: /Book/ });
    const firstTitle = await titleLocator.first().textContent();
    expect(firstTitle).toContain('New Book');
  });

  test('filters books by organized state', async ({ page }) => {
    // GIVEN: Library has organized and import books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Organized Book',
        library_state: 'organized',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Import Book',
        library_state: 'import',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Organized" filter
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Organized' }).click();

    // THEN: Only organized books are shown
    await expect(
      page.getByRole('heading', { name: 'Organized Book', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Import Book', exact: true })
    ).not.toBeVisible();
  });

  test('filters books by import state', async ({ page }) => {
    // GIVEN: Library has organized and import books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Organized Book',
        library_state: 'organized',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Import Book',
        library_state: 'import',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Import" filter
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Import' }).click();

    // THEN: Only import books are shown
    await expect(
      page.getByRole('heading', { name: 'Import Book', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Organized Book', exact: true })
    ).not.toBeVisible();
  });

  test('filters books by soft-deleted state', async ({ page }) => {
    // GIVEN: Library has deleted and active books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Deleted Book',
        marked_for_deletion: true,
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Active Book',
        marked_for_deletion: false,
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Deleted" filter
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Deleted' }).click();

    // THEN: Only deleted books are shown
    await expect(
      page.getByRole('heading', { name: 'Deleted Book', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Active Book', exact: true })
    ).not.toBeVisible();
  });

  test('filters books by author', async ({ page }) => {
    // GIVEN: Library has multiple authors
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Book 1',
        author_name: 'Brandon Sanderson',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Book 2',
        author_name: 'J.R.R. Tolkien',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User filters by author
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Author').click();
    await page.getByRole('option', { name: 'Brandon Sanderson' }).click();

    // THEN: Only books by that author are shown
    await expect(
      page.getByRole('heading', { name: 'Book 1', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Book 2', exact: true })
    ).not.toBeVisible();
  });

  test('filters books by series', async ({ page }) => {
    // GIVEN: Library has multiple series
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'book-1',
        title: 'Book 1',
        series_name: 'Stormlight Archive',
      },
      {
        ...generateTestBooks(1)[0],
        id: 'book-2',
        title: 'Book 2',
        series_name: 'Foundation',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User filters by series
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Series').click();
    await page.getByRole('option', { name: 'Stormlight Archive' }).click();

    // THEN: Only books in that series are shown
    await expect(
      page.getByRole('heading', { name: 'Book 1', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Book 2', exact: true })
    ).not.toBeVisible();
  });

  test('combines multiple filters', async ({ page }) => {
    // GIVEN: Library page loaded
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
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "Organized" state filter
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Organized' }).click();

    // AND: User selects "Brandon Sanderson" author filter
    await page.getByLabel('Author').click();
    await page.getByRole('option', { name: 'Brandon Sanderson' }).click();

    // THEN: Only organized books by Brandon Sanderson are shown
    await expect(
      page.getByRole('heading', { name: 'Book 1', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Book 2', exact: true })
    ).not.toBeVisible();
  });

  test('clears all filters', async ({ page }) => {
    // GIVEN: Multiple filters applied
    const books = generateTestBooks(6);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Organized' }).click();

    // WHEN: User clicks "Clear All" button
    await page.getByRole('button', { name: /clear all/i }).click();

    // THEN: Filters are cleared and books are shown again
    await expect(page.getByRole('heading', { name: 'Test Book 1' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Test Book 2' })).toBeVisible();
  });

  test('changes items per page', async ({ page }) => {
    // GIVEN: Library showing default items per page
    const books = generateTestBooks(60);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects "50" from items-per-page dropdown
    await page.getByLabel('Items per page').click();
    await page.getByRole('option', { name: '50' }).click();

    // THEN: Page reloads showing 50 items
    await expect(
      page.getByRole('heading', { name: 'Test Book 49', exact: true })
    ).toBeVisible();
  });

  test('navigates to next page', async ({ page }) => {
    // GIVEN: Library has 50 books, showing page 1
    const books = generateTestBooks(50);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User clicks "Next" pagination button
    await expect(
      page.getByRole('heading', { name: 'Test Book 1', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Test Book 25', exact: true })
    ).not.toBeVisible();

    await page.getByRole('button', { name: /next page/i }).click();

    // THEN: Page 2 is loaded
    await expect(
      page.getByRole('heading', { name: 'Test Book 25', exact: true })
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Test Book 1', exact: true })
    ).not.toBeVisible();
  });

  test('navigates to previous page', async ({ page }) => {
    // GIVEN: User is on page 2
    const books = generateTestBooks(50);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /page 2/i }).click();
    await expect(
      page.getByRole('heading', { name: 'Test Book 25', exact: true })
    ).toBeVisible();

    // WHEN: User clicks "Previous" pagination button
    await page.getByRole('button', { name: /previous page/i }).click();

    // THEN: Page 1 is loaded
    await expect(
      page.getByRole('heading', { name: 'Test Book 1', exact: true })
    ).toBeVisible();
  });

  test('jumps to specific page', async ({ page }) => {
    // GIVEN: User is on page 1
    const books = generateTestBooks(60);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User clicks page "3" button
    await page.getByRole('button', { name: /page 3/i }).click();

    // THEN: Page 3 is loaded
    await expect(
      page.getByRole('heading', { name: 'Test Book 49', exact: true })
    ).toBeVisible();
  });

  test('clicks book card to navigate to detail page', async ({ page }) => {
    // GIVEN: Library page with books
    const books = [
      {
        ...generateTestBooks(1)[0],
        id: 'test-book-123',
        title: 'The Way of Kings',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User clicks on a book card
    await page
      .getByRole('heading', { name: 'The Way of Kings', exact: true })
      .click();

    // THEN: Navigates to /library/{bookId}
    await page.waitForLoadState('networkidle');
    const url = page.url();
    expect(url).toContain('/library/test-book-123');
  });

  test('shows empty state when no books match filters', async ({ page }) => {
    // GIVEN: Library page loaded
    const books = generateTestBooks(5);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User filters by deleted state with no deleted books
    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Deleted' }).click();

    // THEN: Shows empty state
    await expect(page.getByText(/no audiobooks found/i)).toBeVisible();
  });

  test('shows empty state when library is completely empty', async ({ page }) => {
    // GIVEN: Database has zero audiobooks
    await setupLibraryWithBooks(page, []);

    // WHEN: User navigates to /library
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // THEN: Shows "No Audiobooks Found" message
    await expect(page.getByText('No Audiobooks Found')).toBeVisible();
  });

  test('persists sort and filter settings across page reloads', async ({ page }) => {
    // GIVEN: Library page loaded
    const books = generateTestBooks(10);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User selects sort and filter
    await page.getByLabel('Sort by').click();
    await page.getByRole('option', { name: 'Author' }).click();
    await page.getByLabel('Order').click();
    await page.getByRole('option', { name: 'Descending' }).click();

    await page.getByRole('button', { name: /filters/i }).click();
    await page.getByLabel('Library State').click();
    await page.getByRole('option', { name: 'Organized' }).click();

    // WHEN: User reloads the page
    await page.reload();

    // THEN: URL contains sort/filter params
    const url = page.url();
    expect(url).toContain('sort=author');
    expect(url).toContain('order=desc');
    expect(url).toContain('state=organized');
  });
});
