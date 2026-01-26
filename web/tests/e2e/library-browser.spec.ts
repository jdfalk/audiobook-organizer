// file: tests/e2e/library-browser.spec.ts
// version: 1.0.0
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

    // THEN: Grid displays books with title, author
    await expect(page.getByText('Test Book 1')).toBeVisible();
    await expect(page.getByText('Brandon Sanderson')).toBeVisible();

    // AND: Shows pagination controls (default 20 per page, so we should see page controls)
    const bookCards = page.locator('[data-testid="audiobook-card"]').or(
      page.locator('article').filter({ hasText: 'Test Book' })
    );
    const count = await bookCards.count();
    expect(count).toBeGreaterThan(0);
    expect(count).toBeLessThanOrEqual(20);
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

    // WHEN: User selects "Title (A-Z)" from sort dropdown
    const sortButton = page.getByRole('button', { name: /sort/i }).or(
      page.locator('button').filter({ hasText: /sort|Title/i })
    );
    await sortButton.click();

    const sortOptionAZ = page.getByRole('menuitem', { name: /title.*a.*z/i }).or(
      page.getByText(/title.*a.*z/i)
    );
    await sortOptionAZ.click();

    // THEN: Books are reordered alphabetically by title
    await page.waitForTimeout(500); // Wait for re-render

    const bookTitles = page.locator('[data-testid="book-title"]').or(
      page.locator('h6, h5, h4').filter({ hasText: /Book/ })
    );

    const firstTitle = await bookTitles.first().textContent();
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

    // WHEN: User selects "Title (Z-A)" from sort dropdown
    const sortButton = page.getByRole('button', { name: /sort/i }).or(
      page.locator('button').filter({ hasText: /sort|Title/i })
    );
    await sortButton.click();

    const sortOptionZA = page.getByRole('menuitem', { name: /title.*z.*a/i }).or(
      page.getByText(/title.*z.*a/i)
    );
    await sortOptionZA.click();

    // THEN: Books are reordered reverse alphabetically
    await page.waitForTimeout(500);

    const bookTitles = page.locator('[data-testid="book-title"]').or(
      page.locator('h6, h5, h4').filter({ hasText: /Book/ })
    );

    const firstTitle = await bookTitles.first().textContent();
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
    const sortButton = page.getByRole('button', { name: /sort/i }).or(
      page.locator('button').filter({ hasText: /sort|Author/i })
    );
    await sortButton.click();

    const sortByAuthor = page.getByRole('menuitem', { name: /author/i }).or(
      page.getByText(/^Author$/i)
    );
    await sortByAuthor.click();

    // THEN: Books are reordered by author_name
    await page.waitForTimeout(500);

    const authors = page.locator('[data-testid="book-author"]').or(
      page.locator('text=/Author$/i')
    );

    const firstAuthor = await authors.first().textContent();
    expect(firstAuthor).toContain('Alice Author');
  });

  test('filters books by organized state', async ({ page }) => {
    // GIVEN: Library has organized and unorganized books
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
    const filterButton = page.getByRole('button', { name: /filter|state/i });
    await filterButton.click();

    const organizedFilter = page
      .getByRole('menuitem', { name: /organized/i })
      .or(page.getByText(/^Organized$/i));
    await organizedFilter.click();

    // THEN: Only books with library_state='organized' are shown
    await page.waitForTimeout(500);

    await expect(page.getByText('Organized Book')).toBeVisible();
    await expect(page.getByText('Import Book')).not.toBeVisible();
  });

  test('filters books by import state', async ({ page }) => {
    // GIVEN: Library has books in various states
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
    const filterButton = page.getByRole('button', { name: /filter|state/i });
    await filterButton.click();

    const importFilter = page
      .getByRole('menuitem', { name: /import/i })
      .or(page.getByText(/^Import$/i));
    await importFilter.click();

    // THEN: Only books with library_state='import' are shown
    await page.waitForTimeout(500);

    await expect(page.getByText('Import Book')).toBeVisible();
    await expect(page.getByText('Organized Book')).not.toBeVisible();
  });

  test('searches books by title', async ({ page }) => {
    // GIVEN: Library has books with various titles
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
        title: 'Foundation',
        author_name: 'Isaac Asimov',
      },
    ];
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User types "Hobbit" in search box
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Hobbit');
    await page.waitForTimeout(500); // Debounce

    // THEN: Shows only books matching "Hobbit"
    await expect(page.getByText('The Hobbit')).toBeVisible();
    await expect(page.getByText('The Way of Kings')).not.toBeVisible();
    await expect(page.getByText('Foundation')).not.toBeVisible();
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
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Sanderson');
    await page.waitForTimeout(500);

    // THEN: Shows all books by authors matching "Sanderson"
    await expect(page.getByText('The Way of Kings')).toBeVisible();
    await expect(page.getByText('Words of Radiance')).toBeVisible();
    await expect(page.getByText('The Hobbit')).not.toBeVisible();
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
    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('zzznonexistent');
    await page.waitForTimeout(500);

    // THEN: Shows "No books found" message
    await expect(
      page.getByText(/no.*books.*found|no.*results/i)
    ).toBeVisible();
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

    const searchInput = page.getByPlaceholder(/search/i).or(
      page.locator('input[type="text"]').first()
    );
    await searchInput.fill('Foundation');
    await page.waitForTimeout(500);

    // AND: Results are filtered
    await expect(page.getByText('Foundation')).toBeVisible();
    await expect(page.getByText('The Hobbit')).not.toBeVisible();

    // WHEN: User clicks "X" (clear search) button
    const clearButton = page
      .getByRole('button', { name: /clear/i })
      .or(page.locator('button[aria-label*="clear"]'));
    await clearButton.click();

    // THEN: Search input is cleared
    await expect(searchInput).toHaveValue('');

    // AND: All books are shown again
    await expect(page.getByText('Foundation')).toBeVisible();
    await expect(page.getByText('The Hobbit')).toBeVisible();
  });

  test('navigates to next page', async ({ page }) => {
    // GIVEN: Library has 50 books, showing page 1 (20 books per page)
    const books = generateTestBooks(50);
    await setupLibraryWithBooks(page, books);

    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // WHEN: User clicks "Next" pagination button
    const nextButton = page.getByRole('button', { name: /next/i }).or(
      page.locator('button[aria-label*="next"]')
    );
    await nextButton.click();

    // THEN: Page 2 is loaded
    await page.waitForTimeout(500);

    // AND: URL updates to ?page=2 (or similar)
    const url = page.url();
    expect(url).toMatch(/page=2|\/library\/2/);
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
    const bookCard = page.getByText('The Way of Kings').locator('..');
    await bookCard.click();

    // THEN: Navigates to /library/{bookId}
    await page.waitForLoadState('networkidle');
    const url = page.url();
    expect(url).toContain('/library/test-book-123');
  });

  test('shows empty state when library is completely empty', async ({
    page,
  }) => {
    // GIVEN: Database has zero audiobooks
    await setupLibraryWithBooks(page, []);

    // WHEN: User navigates to /library
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // THEN: Shows "Library is empty" message
    await expect(
      page.getByText(/library.*empty|no.*audiobooks/i)
    ).toBeVisible();

    // AND: Shows "Scan for books" call-to-action (optional)
    const scanButton = page.getByRole('button', { name: /scan/i });
    if (await scanButton.isVisible()) {
      await expect(scanButton).toBeVisible();
    }
  });
});
