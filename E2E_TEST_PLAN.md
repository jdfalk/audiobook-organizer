<!-- file: E2E_TEST_PLAN.md -->
<!-- version: 1.0.0 -->
<!-- guid: f1e2d3c4-b5a6-7890-cdef-1a2b3c4d5e6f -->
<!-- last-edited: 2026-01-25 -->

# Comprehensive E2E Test Plan

**Date**: 2026-01-25
**Purpose**: Document ALL possible user workflows and map to Playwright test scenarios
**Scope**: Complete end-to-end testing for MVP release

---

## Overview

### Current State
- **Existing E2E tests**: 4 files, 21 test cases
- **Current coverage**: ~25% of user workflows
- **Target coverage**: 90%+ of critical user workflows

### Goals
1. Document **every possible user workflow** in the application
2. Map each workflow to specific Playwright test scenarios
3. Prioritize tests by user impact and workflow criticality
4. Provide implementation guide for each test

---

## Test Organization

### Test File Structure

```
web/tests/e2e/
├── app.spec.ts                          # ✅ Basic navigation (existing)
├── import-paths.spec.ts                 # ✅ Import path management (existing)
├── metadata-provenance.spec.ts          # ✅ Metadata provenance (existing)
├── book-detail.spec.ts                  # ✅ Book detail page (existing)
├── library-browser.spec.ts              # ❌ NEW - Library browsing workflows
├── search-and-filter.spec.ts            # ❌ NEW - Search and filtering
├── batch-operations.spec.ts             # ❌ NEW - Batch metadata operations
├── scan-import-organize.spec.ts         # ❌ NEW - Complete import workflow
├── settings-configuration.spec.ts       # ❌ NEW - Settings management
├── file-browser.spec.ts                 # ❌ NEW - Filesystem browsing
├── operation-monitoring.spec.ts         # ❌ NEW - Operation tracking
├── version-management.spec.ts           # ❌ NEW - Version linking workflows
├── backup-restore.spec.ts               # ❌ NEW - Backup/restore workflows
├── error-handling.spec.ts               # ❌ NEW - Error scenarios
└── dashboard.spec.ts                    # ❌ NEW - Dashboard workflows
```

---

## Part 1: Critical User Workflows (P0)

### 1. Library Browser Workflows

**File**: `library-browser.spec.ts`

#### User Stories
- As a user, I want to view all my audiobooks in a grid/list
- As a user, I want to sort books by different fields
- As a user, I want to filter books by state/author/series
- As a user, I want to navigate through pages of books
- As a user, I want to click a book to see details

#### Test Scenarios

```typescript
describe('Library Browser', () => {

  test('loads library page and displays books in grid', async ({ page }) => {
    // GIVEN: Database has 25 audiobooks
    // WHEN: User navigates to /library
    // THEN: Grid displays books with title, author, cover
    // AND: Shows pagination controls
  });

  test('switches between grid and list view', async ({ page }) => {
    // GIVEN: Library page is loaded
    // WHEN: User clicks "List View" button
    // THEN: Display changes to list layout
    // WHEN: User clicks "Grid View" button
    // THEN: Display changes back to grid layout
  });

  test('sorts books by title ascending', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User selects "Title (A-Z)" from sort dropdown
    // THEN: Books are reordered alphabetically by title
    // AND: First book title starts with A
  });

  test('sorts books by title descending', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User selects "Title (Z-A)" from sort dropdown
    // THEN: Books are reordered reverse alphabetically
    // AND: First book title starts with Z
  });

  test('sorts books by author', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User selects "Author" from sort dropdown
    // THEN: Books are reordered by author_name
  });

  test('sorts books by date added', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User selects "Date Added" from sort dropdown
    // THEN: Books are reordered by created_at (newest first)
  });

  test('filters books by organized state', async ({ page }) => {
    // GIVEN: Library has organized and unorganized books
    // WHEN: User selects "Organized" filter
    // THEN: Only books with library_state='organized' are shown
  });

  test('filters books by import state', async ({ page }) => {
    // GIVEN: Library has books in various states
    // WHEN: User selects "Import" filter
    // THEN: Only books with library_state='import' are shown
  });

  test('filters books by soft-deleted state', async ({ page }) => {
    // GIVEN: Library has soft-deleted books
    // WHEN: User selects "Deleted" filter
    // THEN: Only books with marked_for_deletion=true are shown
  });

  test('filters books by author', async ({ page }) => {
    // GIVEN: Library has books by multiple authors
    // WHEN: User types author name in filter input
    // THEN: Only books by that author are shown
  });

  test('filters books by series', async ({ page }) => {
    // GIVEN: Library has books in multiple series
    // WHEN: User types series name in filter input
    // THEN: Only books in that series are shown
  });

  test('combines multiple filters', async ({ page }) => {
    // GIVEN: Library page loaded
    // WHEN: User selects "Organized" state filter
    // AND: User types "Brandon Sanderson" in author filter
    // THEN: Only organized books by Brandon Sanderson are shown
  });

  test('clears all filters', async ({ page }) => {
    // GIVEN: Multiple filters applied
    // WHEN: User clicks "Clear Filters" button
    // THEN: All filters are removed
    // AND: All books are shown again
  });

  test('navigates to next page', async ({ page }) => {
    // GIVEN: Library has 100 books, showing page 1 (20 books per page)
    // WHEN: User clicks "Next" pagination button
    // THEN: Page 2 is loaded with books 21-40
    // AND: URL updates to ?page=2
  });

  test('navigates to previous page', async ({ page }) => {
    // GIVEN: User is on page 3
    // WHEN: User clicks "Previous" pagination button
    // THEN: Page 2 is loaded
    // AND: URL updates to ?page=2
  });

  test('jumps to specific page', async ({ page }) => {
    // GIVEN: User is on page 1
    // WHEN: User clicks page "5" button
    // THEN: Page 5 is loaded
    // AND: URL updates to ?page=5
  });

  test('changes items per page', async ({ page }) => {
    // GIVEN: Library showing 20 items per page
    // WHEN: User selects "50" from items-per-page dropdown
    // THEN: Page reloads showing 50 items
    // AND: Pagination controls update
  });

  test('clicks book card to navigate to detail page', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User clicks on a book card
    // THEN: Navigates to /library/{bookId}
    // AND: Book detail page loads
  });

  test('shows empty state when no books match filters', async ({ page }) => {
    // GIVEN: Library page loaded
    // WHEN: User applies filters that match no books
    // THEN: Shows "No books found" message
    // AND: Shows "Clear filters" button
  });

  test('shows empty state when library is completely empty', async ({ page }) => {
    // GIVEN: Database has zero audiobooks
    // WHEN: User navigates to /library
    // THEN: Shows "Library is empty" message
    // AND: Shows "Scan for books" call-to-action
  });

  test('persists sort and filter settings across page reloads', async ({ page }) => {
    // GIVEN: User has selected "Author" sort and "Organized" filter
    // WHEN: User reloads the page
    // THEN: Same sort and filter are applied
    // AND: URL contains sort/filter params
  });
});
```

**Estimated Implementation Time**: 6-8 hours

---

### 2. Search Functionality

**File**: `search-and-filter.spec.ts`

#### User Stories
- As a user, I want to search for books by title
- As a user, I want to search for books by author
- As a user, I want to see search results instantly
- As a user, I want to clear my search easily

#### Test Scenarios

```typescript
describe('Search Functionality', () => {

  test('searches books by exact title match', async ({ page }) => {
    // GIVEN: Library has book titled "The Way of Kings"
    // WHEN: User types "The Way of Kings" in search box
    // THEN: Shows only books matching that title
  });

  test('searches books by partial title match', async ({ page }) => {
    // GIVEN: Library has books with "Foundation" in title
    // WHEN: User types "Found" in search box
    // THEN: Shows all books with "Found" in title
  });

  test('searches books by author name', async ({ page }) => {
    // GIVEN: Library has books by "Brandon Sanderson"
    // WHEN: User types "Sanderson" in search box
    // THEN: Shows all books by authors matching "Sanderson"
  });

  test('searches books by series name', async ({ page }) => {
    // GIVEN: Library has "Stormlight Archive" series
    // WHEN: User types "Stormlight" in search box
    // THEN: Shows all books in series matching "Stormlight"
  });

  test('search is case-insensitive', async ({ page }) => {
    // GIVEN: Library has "The Hobbit"
    // WHEN: User types "the hobbit" (lowercase) in search
    // THEN: Shows "The Hobbit" book
  });

  test('shows no results message when search matches nothing', async ({ page }) => {
    // GIVEN: Library loaded
    // WHEN: User types "zzznonexistent" in search
    // THEN: Shows "No books found matching 'zzznonexistent'"
    // AND: Shows "Clear search" button
  });

  test('clears search with clear button', async ({ page }) => {
    // GIVEN: User has typed "Foundation" in search
    // AND: Results are filtered
    // WHEN: User clicks "X" (clear search) button
    // THEN: Search input is cleared
    // AND: All books are shown again
  });

  test('clears search with backspace to empty', async ({ page }) => {
    // GIVEN: User has typed "Foundation" in search
    // WHEN: User backspaces to empty string
    // THEN: All books are shown again
  });

  test('search works with other filters combined', async ({ page }) => {
    // GIVEN: User has selected "Organized" state filter
    // WHEN: User types "Sanderson" in search
    // THEN: Shows only organized books by Sanderson
  });

  test('search persists across page navigation', async ({ page }) => {
    // GIVEN: User has searched for "Foundation"
    // WHEN: User clicks a book to view details
    // AND: User clicks browser back button
    // THEN: Search term "Foundation" is still in search box
    // AND: Results are still filtered
  });

  test('search updates URL with query parameter', async ({ page }) => {
    // GIVEN: Library page loaded
    // WHEN: User types "Hobbit" in search
    // THEN: URL updates to ?search=Hobbit
  });

  test('search debounces input to avoid excessive requests', async ({ page }) => {
    // GIVEN: Library page loaded
    // WHEN: User types "Foundation" quickly
    // THEN: Search request fires only after typing stops
    // AND: Does not fire for each character
  });
});
```

**Estimated Implementation Time**: 3-4 hours

---

### 3. Batch Operations

**File**: `batch-operations.spec.ts`

#### User Stories
- As a user, I want to select multiple books
- As a user, I want to fetch metadata for multiple books at once
- As a user, I want to update metadata for multiple books
- As a user, I want to delete multiple books

#### Test Scenarios

```typescript
describe('Batch Operations', () => {

  test('selects single book with checkbox', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User clicks checkbox on one book card
    // THEN: Book is selected (checkbox checked)
    // AND: Selection toolbar appears showing "1 selected"
  });

  test('selects multiple books with individual checkboxes', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: User clicks checkboxes on 5 different books
    // THEN: All 5 books are selected
    // AND: Selection toolbar shows "5 selected"
  });

  test('selects all books on current page', async ({ page }) => {
    // GIVEN: Library page showing 20 books
    // WHEN: User clicks "Select All" checkbox in header
    // THEN: All 20 books on page are selected
    // AND: Selection toolbar shows "20 selected"
  });

  test('deselects all books', async ({ page }) => {
    // GIVEN: 10 books are selected
    // WHEN: User clicks "Deselect All" button
    // THEN: All checkboxes are unchecked
    // AND: Selection toolbar disappears
  });

  test('selection persists across page navigation', async ({ page }) => {
    // GIVEN: User selects 5 books on page 1
    // WHEN: User navigates to page 2
    // AND: User navigates back to page 1
    // THEN: Same 5 books are still selected
  });

  test('bulk fetches metadata for selected books', async ({ page }) => {
    // GIVEN: User has selected 3 books
    // WHEN: User clicks "Fetch Metadata" button in selection toolbar
    // THEN: Confirmation dialog appears
    // WHEN: User confirms
    // THEN: Bulk fetch operation starts
    // AND: Progress indicator appears
    // AND: Shows "Fetching metadata for 3 books..."
  });

  test('monitors bulk fetch progress', async ({ page }) => {
    // GIVEN: Bulk fetch operation started for 10 books
    // WHEN: Operation progresses
    // THEN: Progress bar updates (e.g., "3/10 completed")
    // AND: Shows which books are done, pending, failed
  });

  test('bulk fetch completes successfully', async ({ page }) => {
    // GIVEN: Bulk fetch operation started
    // WHEN: All books complete successfully
    // THEN: Success toast appears "Metadata fetched for 3 books"
    // AND: Selection is cleared
    // AND: Book cards update with new metadata
  });

  test('bulk fetch handles partial failures', async ({ page }) => {
    // GIVEN: Bulk fetch operation started for 5 books
    // WHEN: 3 succeed, 2 fail
    // THEN: Shows "3 succeeded, 2 failed"
    // AND: Lists which books failed with error messages
  });

  test('cancels bulk fetch operation', async ({ page }) => {
    // GIVEN: Bulk fetch operation in progress
    // WHEN: User clicks "Cancel" button
    // THEN: Operation stops
    // AND: Shows "Bulk fetch cancelled"
    // AND: Books processed so far keep their fetched metadata
  });

  test('batch updates metadata field for selected books', async ({ page }) => {
    // GIVEN: User has selected 5 books
    // WHEN: User clicks "Batch Edit" button
    // THEN: Batch edit dialog opens
    // WHEN: User selects "Language" field
    // AND: User sets value to "en"
    // AND: User clicks "Apply"
    // THEN: All 5 books update language to "en"
    // AND: Success toast appears
  });

  test('batch soft-deletes selected books', async ({ page }) => {
    // GIVEN: User has selected 4 books
    // WHEN: User clicks "Delete Selected" button
    // THEN: Confirmation dialog appears
    // WHEN: User confirms soft delete
    // THEN: All 4 books marked for deletion
    // AND: Books move to "Deleted" state
    // AND: Success toast appears
  });

  test('batch restores soft-deleted books', async ({ page }) => {
    // GIVEN: User has selected 3 soft-deleted books
    // WHEN: User clicks "Restore Selected" button
    // THEN: All 3 books restored to previous state
    // AND: marked_for_deletion flag cleared
    // AND: Success toast appears
  });

  test('disables batch operations when no books selected', async ({ page }) => {
    // GIVEN: Library page with books
    // WHEN: No books are selected
    // THEN: Batch operation buttons are disabled
    // AND: Tooltip shows "Select books first"
  });

  test('shows different batch actions based on selection state', async ({ page }) => {
    // GIVEN: User has selected mix of organized and soft-deleted books
    // THEN: Shows "Restore" button (for deleted books)
    // AND: Shows "Delete" button (for non-deleted books)
  });
});
```

**Estimated Implementation Time**: 4-5 hours

---

### 4. Scan/Import/Organize Complete Workflow

**File**: `scan-import-organize.spec.ts`

#### User Stories
- As a user, I want to add a folder to scan for books
- As a user, I want to scan that folder
- As a user, I want to see progress of the scan
- As a user, I want to see discovered books in import state
- As a user, I want to organize those books into my library
- As a user, I want to see the books move to organized state

#### Test Scenarios

```typescript
describe('Scan/Import/Organize Workflow', () => {

  test('complete workflow: add import path → scan → organize', async ({ page }) => {
    // GIVEN: Empty library, no import paths

    // WHEN: User navigates to Settings → Import Paths
    await page.goto('/settings');
    await page.getByText('Import Paths').click();

    // AND: User clicks "Add Import Path"
    await page.getByRole('button', { name: 'Add Import Path' }).click();

    // AND: User enters path "/test/audiobooks"
    await page.getByPlaceholder('/path/to/downloads').fill('/test/audiobooks');

    // AND: User clicks "Add"
    await page.getByRole('button', { name: 'Add' }).click();

    // THEN: Import path appears in list
    await expect(page.getByText('/test/audiobooks')).toBeVisible();

    // WHEN: User clicks "Scan" button for this import path
    await page.getByRole('button', { name: 'Scan' }).click();

    // THEN: Scan operation starts
    await expect(page.getByText('Scanning...')).toBeVisible();

    // AND: Progress indicator appears
    await expect(page.getByText(/Scanned \d+ files/)).toBeVisible();

    // WHEN: Scan completes
    await page.waitForFunction(
      () => !document.body.textContent.includes('Scanning...'),
      { timeout: 30000 }
    );

    // THEN: Success message appears
    await expect(page.getByText(/Scan complete.*found \d+ audiobooks/)).toBeVisible();

    // WHEN: User navigates to Library page
    await page.goto('/library');

    // AND: User filters by "Import" state
    await page.getByRole('button', { name: 'Filter' }).click();
    await page.getByRole('option', { name: 'Import' }).click();

    // THEN: Shows discovered books in import state
    await expect(page.getByText('import')).toBeVisible();

    // WHEN: User selects all import books
    await page.getByRole('checkbox', { name: 'Select All' }).click();

    // AND: User clicks "Organize" button
    await page.getByRole('button', { name: 'Organize Selected' }).click();

    // THEN: Organize confirmation dialog appears
    await expect(page.getByRole('dialog', { name: /Organize/i })).toBeVisible();

    // WHEN: User confirms organize
    await page.getByRole('button', { name: 'Organize' }).click();

    // THEN: Organize operation starts
    await expect(page.getByText('Organizing...')).toBeVisible();

    // AND: Progress indicator shows files being moved
    await expect(page.getByText(/Organized \d+ of \d+/)).toBeVisible();

    // WHEN: Organize completes
    await page.waitForFunction(
      () => !document.body.textContent.includes('Organizing...'),
      { timeout: 60000 }
    );

    // THEN: Success message appears
    await expect(page.getByText(/Successfully organized \d+ audiobooks/)).toBeVisible();

    // WHEN: User changes filter to "Organized"
    await page.getByRole('button', { name: 'Filter' }).click();
    await page.getByRole('option', { name: 'Organized' }).click();

    // THEN: Shows books in organized state
    await expect(page.getByText('organized')).toBeVisible();

    // AND: Books are no longer in import state
    await page.getByRole('button', { name: 'Filter' }).click();
    await page.getByRole('option', { name: 'Import' }).click();
    await expect(page.getByText('No books found')).toBeVisible();
  });

  test('scan operation: start, monitor progress, complete', async ({ page }) => {
    // GIVEN: Import path "/test/books" exists with 50 audiobook files
    // WHEN: User triggers scan for this path
    // THEN: Scan operation creates operation record
    // AND: SSE events stream progress updates
    // AND: UI shows "Scanning... 10/50 files"
    // AND: Progress bar updates to 20%, 40%, 60%, etc.
    // WHEN: Scan completes
    // THEN: Shows "Scan complete. Found 50 audiobooks."
    // AND: Operation status changes to "completed"
  });

  test('scan operation: cancel in progress', async ({ page }) => {
    // GIVEN: Scan operation running (scanned 20/100 files)
    // WHEN: User clicks "Cancel Scan" button
    // THEN: Confirmation dialog appears
    // WHEN: User confirms cancellation
    // THEN: Scan operation stops
    // AND: Shows "Scan cancelled. Processed 20 files."
    // AND: Already discovered books remain in database
  });

  test('scan operation: handles errors gracefully', async ({ page }) => {
    // GIVEN: Import path "/test/corrupt" with some corrupt files
    // WHEN: Scan runs and encounters corrupt files
    // THEN: Shows warnings for corrupt files
    // AND: Continues scanning valid files
    // WHEN: Scan completes
    // THEN: Shows "Scan complete. Found 45 audiobooks, 5 errors."
    // AND: Provides button to "View Errors"
  });

  test('organize operation: moves files to library root', async ({ page }) => {
    // GIVEN: 10 books in "import" state
    // WHEN: User selects all and clicks "Organize"
    // THEN: Organize operation starts
    // AND: Shows progress "Organizing... 2/10"
    // WHEN: Each file completes
    // THEN: File is copied/moved to library root
    // AND: Book state changes from "import" to "organized"
    // AND: organized_file_hash is updated
  });

  test('organize operation: handles duplicate files', async ({ page }) => {
    // GIVEN: Import book with hash matching existing organized book
    // WHEN: User tries to organize this book
    // THEN: Shows duplicate detection warning
    // AND: Offers options: "Skip", "Link as Version", "Replace"
    // WHEN: User selects "Link as Version"
    // THEN: Books are linked as versions
    // AND: Original remains primary
  });

  test('organize operation: rollback on error', async ({ page }) => {
    // GIVEN: Organize operation for 5 books
    // WHEN: Third book fails (e.g., disk full)
    // THEN: Shows error "Failed to organize Book 3"
    // AND: Provides "Rollback" option
    // WHEN: User clicks "Rollback"
    // THEN: First 2 books are reverted to import state
    // AND: Files are moved back to import paths
  });

  test('rescan existing import path', async ({ page }) => {
    // GIVEN: Import path previously scanned with 30 books
    // WHEN: User adds 10 new files to this path
    // AND: User clicks "Rescan" for this import path
    // THEN: Scan discovers 10 new books
    // AND: Does not duplicate existing 30 books
    // AND: Shows "Found 10 new audiobooks"
  });

  test('remove import path with books', async ({ page }) => {
    // GIVEN: Import path with 20 books in "import" state
    // WHEN: User clicks "Remove" for this import path
    // THEN: Warning dialog appears
    // AND: Shows "20 books will remain in database"
    // WHEN: User confirms removal
    // THEN: Import path is removed
    // AND: Books remain in database (orphaned)
  });

  test('shows real-time SSE updates during scan', async ({ page }) => {
    // GIVEN: Scan operation started
    // WHEN: SSE events are emitted from server
    // THEN: UI updates without page refresh
    // AND: Shows current file being scanned
    // AND: Shows files scanned count incrementing
    // AND: Shows books discovered count incrementing
  });

  test('shows real-time SSE updates during organize', async ({ page }) => {
    // GIVEN: Organize operation started for 20 books
    // WHEN: SSE events are emitted
    // THEN: UI shows "Organizing Book 5 of 20"
    // AND: Shows current book being processed
    // AND: Updates progress bar (25%, 50%, 75%)
  });
});
```

**Estimated Implementation Time**: 6-8 hours

---

### 5. Settings Configuration

**File**: `settings-configuration.spec.ts`

#### User Stories
- As a user, I want to configure my library root directory
- As a user, I want to configure my OpenAI API key for AI parsing
- As a user, I want to configure scan settings
- As a user, I want to save settings and have them persist

#### Test Scenarios

```typescript
describe('Settings Configuration', () => {

  test('loads settings page with all sections', async ({ page }) => {
    // GIVEN: User is logged in
    // WHEN: User navigates to /settings
    // THEN: Shows all settings sections:
    //   - Library Configuration
    //   - Import Paths
    //   - Scan Settings
    //   - API Keys
    //   - Blocked Hashes
    //   - System Info
  });

  test('updates library root directory', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User clicks on "Library Configuration" section
    // AND: User updates "Root Directory" to "/new/library/path"
    // AND: User clicks "Save"
    // THEN: Settings are saved
    // AND: Success toast appears
    // WHEN: User reloads page
    // THEN: Root directory shows "/new/library/path"
  });

  test('browses for library root directory', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User clicks "Browse" button next to root directory
    // THEN: File browser dialog opens
    // WHEN: User navigates to /home/user/audiobooks
    // AND: User clicks "Select"
    // THEN: Root directory field updates to "/home/user/audiobooks"
  });

  test('updates OpenAI API key', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User navigates to "API Keys" section
    // AND: User enters API key "sk-test1234..."
    // AND: User clicks "Save"
    // THEN: API key is saved (masked)
    // AND: Success toast appears
  });

  test('tests OpenAI connection', async ({ page }) => {
    // GIVEN: OpenAI API key is configured
    // WHEN: User clicks "Test Connection" button
    // THEN: Test request is sent to OpenAI
    // AND: Shows loading indicator
    // WHEN: Connection succeeds
    // THEN: Shows "✅ Connection successful"
    // AND: Shows OpenAI model info
  });

  test('handles OpenAI connection failure', async ({ page }) => {
    // GIVEN: Invalid OpenAI API key is configured
    // WHEN: User clicks "Test Connection" button
    // THEN: Test request fails
    // AND: Shows "❌ Connection failed: Invalid API key"
  });

  test('configures scan settings: file extensions', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User navigates to "Scan Settings"
    // AND: User adds ".opus" to file extensions
    // AND: User clicks "Save"
    // THEN: Scan settings updated
    // WHEN: Next scan runs
    // THEN: .opus files are included
  });

  test('configures scan settings: exclude patterns', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User adds "*_preview.m4b" to exclude patterns
    // AND: User clicks "Save"
    // THEN: Exclude patterns updated
    // WHEN: Next scan runs
    // THEN: Files matching *_preview.m4b are skipped
  });

  test('adds import path from settings', async ({ page }) => {
    // See import-paths.spec.ts (already covered)
    // Verified working in existing test
  });

  test('removes import path from settings', async ({ page }) => {
    // See import-paths.spec.ts (already covered)
    // Verified working in existing test
  });

  test('views blocked hashes list', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User navigates to "Blocked Hashes" section
    // THEN: Shows list of blocked file hashes
    // AND: Shows reason for each block
    // AND: Shows date blocked
  });

  test('adds hash to blocked list manually', async ({ page }) => {
    // GIVEN: User is on "Blocked Hashes" section
    // WHEN: User enters hash "abc123..." and reason "Duplicate"
    // AND: User clicks "Block Hash"
    // THEN: Hash is added to blocked list
    // AND: Success toast appears
    // WHEN: File with this hash is scanned
    // THEN: File is automatically rejected
  });

  test('removes hash from blocked list', async ({ page }) => {
    // GIVEN: Blocked hash "abc123..." exists
    // WHEN: User clicks "Remove" for this hash
    // THEN: Confirmation dialog appears
    // WHEN: User confirms
    // THEN: Hash is removed from blocked list
    // AND: File with this hash can now be imported
  });

  test('views system information', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User navigates to "System Info" section
    // THEN: Shows:
    //   - Application version
    //   - Go version
    //   - Database location
    //   - Library path
    //   - Total books count
    //   - Total storage used
  });

  test('exports settings configuration', async ({ page }) => {
    // GIVEN: Settings configured
    // WHEN: User clicks "Export Settings"
    // THEN: Downloads JSON file with settings
    // AND: File contains root_dir, api_keys, scan settings
  });

  test('imports settings configuration', async ({ page }) => {
    // GIVEN: User has exported settings JSON file
    // WHEN: User clicks "Import Settings"
    // AND: User selects settings.json file
    // THEN: Settings are loaded from file
    // AND: Confirmation dialog shows changes
    // WHEN: User confirms import
    // THEN: Settings are applied
    // AND: Page reloads with new settings
  });

  test('resets settings to defaults', async ({ page }) => {
    // GIVEN: User has customized settings
    // WHEN: User clicks "Reset to Defaults" button
    // THEN: Confirmation dialog appears
    // WHEN: User confirms reset
    // THEN: All settings revert to default values
    // AND: Success toast appears
  });

  test('shows unsaved changes warning', async ({ page }) => {
    // GIVEN: User has modified root directory
    // BUT: User has not clicked "Save"
    // WHEN: User tries to navigate away
    // THEN: Warning dialog appears
    // AND: Shows "You have unsaved changes"
    // AND: Offers "Save", "Discard", "Cancel" options
  });

  test('validates root directory exists', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User enters non-existent path "/fake/path"
    // AND: User clicks "Save"
    // THEN: Validation error appears
    // AND: Shows "Directory does not exist"
    // AND: Settings are not saved
  });

  test('validates API key format', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User enters invalid API key "invalid"
    // AND: User clicks "Save"
    // THEN: Validation error appears
    // AND: Shows "Invalid API key format"
  });
});
```

**Estimated Implementation Time**: 4-5 hours

---

## Part 2: Important User Workflows (P1)

### 6. File Browser

**File**: `file-browser.spec.ts`

#### Test Scenarios

```typescript
describe('File Browser', () => {

  test('browses root filesystem', async ({ page }) => {
    // GIVEN: Settings page loaded
    // WHEN: User opens file browser
    // THEN: Shows root directory contents
    // AND: Shows folders and files
    // AND: Shows folder icon for directories
  });

  test('navigates into subdirectory', async ({ page }) => {
    // GIVEN: File browser showing /home/user
    // WHEN: User double-clicks "audiobooks" folder
    // THEN: Navigates to /home/user/audiobooks
    // AND: Shows contents of audiobooks folder
    // AND: Breadcrumb updates to "home > user > audiobooks"
  });

  test('navigates up directory hierarchy', async ({ page }) => {
    // GIVEN: File browser showing /home/user/audiobooks
    // WHEN: User clicks "user" in breadcrumb
    // THEN: Navigates to /home/user
    // AND: Shows parent directory contents
  });

  test('creates .jabexclude file', async ({ page }) => {
    // GIVEN: File browser showing /home/user/audiobooks/temp
    // WHEN: User right-clicks on folder
    // AND: User selects "Exclude from scan"
    // THEN: .jabexclude file is created in /temp
    // AND: Folder shows "excluded" indicator
    // AND: Success toast appears
  });

  test('removes .jabexclude file', async ({ page }) => {
    // GIVEN: Folder /temp has .jabexclude file
    // AND: Folder shows "excluded" indicator
    // WHEN: User right-clicks on folder
    // AND: User selects "Include in scan"
    // THEN: .jabexclude file is deleted
    // AND: "excluded" indicator disappears
  });

  test('shows disk space information', async ({ page }) => {
    // GIVEN: File browser open
    // THEN: Shows available disk space for current volume
    // AND: Shows total disk space
    // AND: Shows space used by library
  });

  test('filters files by extension', async ({ page }) => {
    // GIVEN: Directory with .m4b, .mp3, .txt files
    // WHEN: User applies ".m4b" filter
    // THEN: Shows only .m4b files
    // AND: Hides .mp3 and .txt files
  });

  test('selects folder as import path', async ({ page }) => {
    // GIVEN: File browser showing /downloads/audiobooks
    // WHEN: User clicks "Add as Import Path" button
    // THEN: This folder is added to import paths
    // AND: User is taken to Import Paths settings
    // AND: New path appears in list
  });
});
```

**Estimated Implementation Time**: 3-4 hours

---

### 7. Operation Monitoring

**File**: `operation-monitoring.spec.ts`

#### Test Scenarios

```typescript
describe('Operation Monitoring', () => {

  test('views active operations list', async ({ page }) => {
    // GIVEN: 2 scan operations and 1 organize operation running
    // WHEN: User navigates to Dashboard or Operations page
    // THEN: Shows list of 3 active operations
    // AND: Shows operation type, status, progress for each
  });

  test('monitors operation progress in real-time', async ({ page }) => {
    // GIVEN: Scan operation running (20/100 files scanned)
    // WHEN: SSE events update progress to 21, 22, 23...
    // THEN: Progress bar updates in real-time
    // AND: Shows "21/100 files scanned"
    // AND: No page refresh needed
  });

  test('views operation logs', async ({ page }) => {
    // GIVEN: Active scan operation
    // WHEN: User clicks "View Logs" button
    // THEN: Log viewer dialog opens
    // AND: Shows timestamped log entries
    // AND: Shows "Scanning file: book1.m4b"
    // AND: Logs auto-scroll to bottom
  });

  test('views completed operation logs', async ({ page }) => {
    // GIVEN: Completed scan operation
    // WHEN: User clicks "View Logs" button
    // THEN: Shows full operation log
    // AND: Shows summary: "Completed. Found 50 books, 2 errors."
  });

  test('filters operation logs by level', async ({ page }) => {
    // GIVEN: Log viewer open with info, warning, error logs
    // WHEN: User selects "Errors only" filter
    // THEN: Shows only error-level log entries
    // AND: Hides info and warning entries
  });

  test('cancels running operation', async ({ page }) => {
    // GIVEN: Organize operation running (5/20 books completed)
    // WHEN: User clicks "Cancel" button
    // THEN: Confirmation dialog appears
    // WHEN: User confirms cancellation
    // THEN: Operation is cancelled
    // AND: Shows "Operation cancelled" status
    // AND: Shows "5/20 books organized before cancellation"
  });

  test('retries failed operation', async ({ page }) => {
    // GIVEN: Scan operation failed (network error)
    // WHEN: User clicks "Retry" button
    // THEN: New scan operation starts
    // AND: Uses same parameters as failed operation
    // AND: Shows progress indicator
  });

  test('clears completed operations', async ({ page }) => {
    // GIVEN: 10 completed operations in list
    // WHEN: User clicks "Clear Completed" button
    // THEN: Completed operations are removed from list
    // AND: Active operations remain
  });

  test('shows operation error details', async ({ page }) => {
    // GIVEN: Failed operation with error
    // WHEN: User clicks on failed operation
    // THEN: Error details dialog opens
    // AND: Shows error message
    // AND: Shows stack trace (if available)
    // AND: Shows failed files list
  });

  test('operation history pagination', async ({ page }) => {
    // GIVEN: 100 historical operations
    // WHEN: User navigates to operation history
    // THEN: Shows first 20 operations
    // WHEN: User clicks "Next"
    // THEN: Shows operations 21-40
  });
});
```

**Estimated Implementation Time**: 3-4 hours

---

### 8. Version Management

**File**: `version-management.spec.ts`

#### Test Scenarios

```typescript
describe('Version Management', () => {

  test('links two books as versions', async ({ page }) => {
    // GIVEN: Book A and Book B (same title, different narrators)
    // WHEN: User opens Book A detail page
    // AND: User navigates to "Versions" tab
    // AND: User clicks "Link Version"
    // THEN: Book selection dialog opens
    // WHEN: User searches for and selects Book B
    // AND: User clicks "Link"
    // THEN: Books are linked as versions
    // AND: Book A shows "1 additional version"
    // AND: Versions tab shows Book B
  });

  test('sets primary version', async ({ page }) => {
    // GIVEN: Book A and Book B linked as versions
    // AND: Book A is currently primary
    // WHEN: User opens Book B detail page
    // AND: User clicks "Set as Primary"
    // THEN: Book B becomes primary version
    // AND: Book A shows "Book B is primary version"
  });

  test('unlinks version', async ({ page }) => {
    // GIVEN: Book A and Book B linked as versions
    // WHEN: User opens Book A Versions tab
    // AND: User clicks "Unlink" for Book B
    // THEN: Confirmation dialog appears
    // WHEN: User confirms
    // THEN: Books are no longer linked
    // AND: Each shows "No additional versions"
  });

  test('navigates between versions', async ({ page }) => {
    // GIVEN: Book A (primary) with Book B, Book C as versions
    // WHEN: User is on Book A detail page
    // AND: User clicks on Book B in Versions list
    // THEN: Navigates to Book B detail page
    // AND: Shows "Book A is primary version"
  });

  test('shows version group information', async ({ page }) => {
    // GIVEN: 3 books linked as versions
    // WHEN: User views any version
    // THEN: Shows "Part of version group with 3 books"
    // AND: Lists all versions with indicators:
    //   - Primary version (crown icon)
    //   - Current version (highlighted)
  });

  test('prevents circular version links', async ({ page }) => {
    // GIVEN: Book A and Book B already linked
    // WHEN: User tries to link Book B to Book C
    // AND: Book C is already linked to Book A
    // THEN: Shows error "Cannot create circular version links"
    // AND: Link is not created
  });
});
```

**Estimated Implementation Time**: 2-3 hours

---

### 9. Backup and Restore

**File**: `backup-restore.spec.ts`

#### Test Scenarios

```typescript
describe('Backup and Restore', () => {

  test('creates manual backup', async ({ page }) => {
    // GIVEN: Settings page, Backup section
    // WHEN: User clicks "Create Backup"
    // THEN: Backup operation starts
    // AND: Shows progress indicator
    // WHEN: Backup completes
    // THEN: Success toast appears
    // AND: New backup appears in backups list
    // AND: Shows filename, size, date
  });

  test('lists existing backups', async ({ page }) => {
    // GIVEN: 5 backups exist
    // WHEN: User navigates to Backup section
    // THEN: Shows all 5 backups ordered by date (newest first)
    // AND: Shows backup size, date, status
  });

  test('downloads backup file', async ({ page }) => {
    // GIVEN: Backup "backup-2026-01-25.db.gz" exists
    // WHEN: User clicks "Download" button
    // THEN: Backup file is downloaded
    // AND: File is compressed .gz format
  });

  test('restores from backup', async ({ page }) => {
    // GIVEN: Backup file exists
    // WHEN: User clicks "Restore" button
    // THEN: Warning dialog appears
    // AND: Shows "This will replace current database"
    // WHEN: User confirms restore
    // THEN: Restore operation starts
    // AND: Shows progress indicator
    // WHEN: Restore completes
    // THEN: Database is restored from backup
    // AND: Page reloads with restored data
  });

  test('deletes backup file', async ({ page }) => {
    // GIVEN: Backup file exists
    // WHEN: User clicks "Delete" button
    // THEN: Confirmation dialog appears
    // WHEN: User confirms deletion
    // THEN: Backup file is deleted
    // AND: Removed from backups list
  });

  test('validates backup before restore', async ({ page }) => {
    // GIVEN: Corrupt backup file selected
    // WHEN: User attempts to restore
    // THEN: Validation runs
    // AND: Shows error "Backup file is corrupt"
    // AND: Restore is cancelled
  });

  test('automatic backup on schedule', async ({ page }) => {
    // GIVEN: Automatic backup enabled (daily at 2am)
    // WHEN: Scheduled time arrives
    // THEN: Backup is created automatically
    // AND: Appears in backups list with "Auto" tag
  });
});
```

**Estimated Implementation Time**: 2-3 hours

---

## Part 3: Secondary Workflows (P2)

### 10. Error Handling

**File**: `error-handling.spec.ts`

#### Test Scenarios

```typescript
describe('Error Handling', () => {

  test('handles network timeout gracefully', async ({ page }) => {
    // GIVEN: Server is slow to respond
    // WHEN: API request times out
    // THEN: Shows error toast "Request timed out"
    // AND: Provides "Retry" button
  });

  test('handles 404 not found errors', async ({ page }) => {
    // GIVEN: User tries to access non-existent book
    // WHEN: GET /audiobooks/invalid-id returns 404
    // THEN: Shows "Book not found" page
    // AND: Provides link to return to library
  });

  test('handles 500 server errors', async ({ page }) => {
    // GIVEN: Server encounters internal error
    // WHEN: API returns 500 error
    // THEN: Shows error toast "Server error occurred"
    // AND: Logs error details for debugging
  });

  test('handles invalid form input', async ({ page }) => {
    // GIVEN: Book detail edit mode
    // WHEN: User enters invalid year "abcd"
    // AND: User tries to save
    // THEN: Validation error appears
    // AND: Shows "Year must be a number"
    // AND: Save is prevented
  });

  test('handles concurrent edit conflicts', async ({ page }) => {
    // GIVEN: User A and User B editing same book
    // WHEN: User A saves changes
    // AND: User B tries to save (stale data)
    // THEN: Shows conflict warning
    // AND: Shows "Book was updated by another user"
    // AND: Provides options: "Reload", "Overwrite"
  });

  test('handles session expiration', async ({ page }) => {
    // GIVEN: User session expired
    // WHEN: User tries to perform action
    // THEN: Shows "Session expired" message
    // AND: Redirects to login page
  });

  test('recovers from SSE connection loss', async ({ page }) => {
    // GIVEN: SSE connection active
    // WHEN: Network disconnects
    // THEN: Shows "Connection lost" indicator
    // WHEN: Network reconnects
    // THEN: SSE connection re-establishes
    // AND: Shows "Connection restored"
    // AND: Syncs missed updates
  });

  test('handles file upload errors', async ({ page }) => {
    // GIVEN: User uploading file
    // WHEN: Upload fails (disk full)
    // THEN: Shows error "Upload failed: Disk full"
    // AND: Clears upload progress
    // AND: File is not saved
  });
});
```

**Estimated Implementation Time**: 2-3 hours

---

### 11. Dashboard

**File**: `dashboard.spec.ts`

#### Test Scenarios

```typescript
describe('Dashboard', () => {

  test('displays library statistics', async ({ page }) => {
    // GIVEN: Library has 150 books
    // WHEN: User navigates to Dashboard
    // THEN: Shows "150 Total Books"
    // AND: Shows breakdown by state (organized, import, deleted)
  });

  test('displays import paths statistics', async ({ page }) => {
    // GIVEN: 3 import paths with 25 books total
    // WHEN: User views Dashboard
    // THEN: Shows "3 Import Paths"
    // AND: Shows "25 Books Pending Import"
  });

  test('displays recent operations', async ({ page }) => {
    // GIVEN: 5 recent operations completed
    // WHEN: User views Dashboard
    // THEN: Shows last 5 operations
    // AND: Shows type, status, date for each
  });

  test('displays storage usage', async ({ page }) => {
    // GIVEN: Library uses 45GB of 500GB available
    // WHEN: User views Dashboard
    // THEN: Shows storage usage chart
    // AND: Shows "45 GB / 500 GB (9%)"
  });

  test('quick action: start scan', async ({ page }) => {
    // GIVEN: Dashboard loaded
    // WHEN: User clicks "Scan All Import Paths" button
    // THEN: Scan operation starts for all import paths
    // AND: Redirects to operation monitoring
  });

  test('quick action: organize all import books', async ({ page }) => {
    // GIVEN: 30 books in import state
    // WHEN: User clicks "Organize All" button
    // THEN: Confirmation dialog appears
    // WHEN: User confirms
    // THEN: Organize operation starts
  });
});
```

**Estimated Implementation Time**: 2-3 hours

---

## Part 4: Implementation Priority & Timeline

### Phase 1: Critical Workflows (2-3 days)
**Total**: ~25 hours

1. **Library Browser** (6-8 hours) - P0
   - Sorting, filtering, pagination
   - Essential for navigation

2. **Search Functionality** (3-4 hours) - P0
   - Basic search by title/author/series
   - Critical user feature

3. **Batch Operations** (4-5 hours) - P0
   - Select multiple books
   - Bulk fetch metadata
   - Validates major workflow

4. **Scan/Import/Organize** (6-8 hours) - P0
   - Complete workflow from scan to organized
   - Most critical user journey

5. **Settings Configuration** (4-5 hours) - P0
   - Configure root dir, API keys
   - Essential for app functionality

**After Phase 1**: 60-70% E2E coverage achieved

---

### Phase 2: Important Workflows (1-2 days)
**Total**: ~15 hours

6. **File Browser** (3-4 hours) - P1
   - Browse filesystem
   - Create/remove .jabexclude

7. **Operation Monitoring** (3-4 hours) - P1
   - View active operations
   - Monitor progress, view logs

8. **Version Management** (2-3 hours) - P1
   - Link/unlink versions
   - Set primary version

9. **Backup and Restore** (2-3 hours) - P1
   - Create backup
   - Restore from backup

10. **Dashboard** (2-3 hours) - P1
    - View statistics
    - Quick actions

**After Phase 2**: 80-85% E2E coverage achieved

---

### Phase 3: Secondary Workflows (1 day)
**Total**: ~5 hours

11. **Error Handling** (2-3 hours) - P2
    - Network errors
    - Validation errors
    - Session handling

12. **Edge Cases & Polish** (2-3 hours) - P2
    - Empty states
    - Loading states
    - Accessibility

**After Phase 3**: 90%+ E2E coverage achieved

---

## Part 5: Test Infrastructure Setup

### Prerequisites

```bash
# Install Playwright
cd web
npm install -D @playwright/test
npx playwright install chromium

# Update playwright.config.ts if needed
```

### Common Test Utilities

Create shared utilities for all tests:

**File**: `web/tests/e2e/utils/test-helpers.ts`

```typescript
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
    (window as any).EventSource = MockEventSource;
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
export async function waitForToast(page: Page, text: string, timeout = 5000) {
  await page.waitForSelector(`text=${text}`, { timeout });
}

/**
 * Generate test audiobooks
 */
export function generateTestBooks(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    id: `book-${i + 1}`,
    title: `Test Book ${i + 1}`,
    author_name: `Author ${(i % 5) + 1}`,
    series_name: i % 3 === 0 ? `Test Series ${(i % 3) + 1}` : null,
    library_state: i % 4 === 0 ? 'import' : 'organized',
    file_path: `/library/book${i + 1}.m4b`,
    file_hash: `hash-${i + 1}`,
    created_at: new Date(2024, 0, i + 1).toISOString(),
  }));
}
```

---

## Part 6: Success Metrics

### E2E Coverage Goals

| Category | Current | Phase 1 | Phase 2 | Phase 3 | Target |
|----------|---------|---------|---------|---------|--------|
| Navigation | 5% | 100% | 100% | 100% | 100% |
| Library Browser | 0% | 80% | 90% | 100% | 100% |
| Search & Filter | 0% | 80% | 90% | 100% | 100% |
| Batch Operations | 0% | 70% | 80% | 90% | 90% |
| Import Workflow | 10% | 90% | 95% | 100% | 100% |
| Book Detail | 60% | 70% | 80% | 90% | 90% |
| Metadata Provenance | 90% | 90% | 95% | 100% | 100% |
| Settings | 0% | 80% | 90% | 100% | 100% |
| File Browser | 0% | 0% | 80% | 90% | 90% |
| Operations | 0% | 0% | 80% | 90% | 90% |
| **OVERALL** | **25%** | **60-70%** | **80-85%** | **90%+** | **90%+** |

### Quality Gates

- [ ] **Phase 1 Complete**: All P0 workflows have E2E tests
- [ ] **Manual QA**: All critical paths validated manually
- [ ] **Phase 2 Complete**: 80%+ E2E coverage achieved
- [ ] **Phase 3 Complete**: 90%+ E2E coverage achieved
- [ ] **CI Integration**: All E2E tests pass in CI pipeline
- [ ] **Performance**: E2E tests complete in < 10 minutes

---

## Conclusion

This comprehensive E2E test plan documents **every possible user workflow** in the audiobook-organizer application and maps them to specific Playwright test scenarios.

### Summary

- **Total test files to create**: 11 new files
- **Total test scenarios**: ~120+ test cases
- **Current coverage**: 25% (21 tests)
- **Target coverage**: 90%+ (140+ tests)
- **Implementation time**: 4-6 days
  - Phase 1 (Critical): 2-3 days
  - Phase 2 (Important): 1-2 days
  - Phase 3 (Secondary): 1 day

### Recommended Next Steps

1. **Immediate** (Day 1-3): Implement Phase 1 tests
   - Library browser, search, batch operations
   - Scan/import/organize workflow
   - Settings configuration

2. **Short-term** (Day 4-5): Implement Phase 2 tests
   - File browser, operation monitoring
   - Version management, backup/restore
   - Dashboard workflows

3. **Final** (Day 6): Implement Phase 3 tests
   - Error handling
   - Edge cases
   - Accessibility

4. **Validation** (Day 7): Manual QA
   - Walk through all workflows
   - Verify E2E tests match reality
   - Identify any remaining gaps

### Success Criteria

The audiobook-organizer will be **MVP-ready** when:
- ✅ 90%+ of user workflows have E2E coverage
- ✅ All critical paths validated (scan/import/organize, library browser, settings)
- ✅ Manual QA confirms tests match real user experience
- ✅ All tests pass in CI pipeline

---

*Plan created*: 2026-01-25
*Status*: Ready for implementation
*Next step*: Begin Phase 1 test implementation
