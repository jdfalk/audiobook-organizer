<!-- file: docs/MANUAL_QA_GUIDE.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->
<!-- last-edited: 2026-01-27 -->

# Manual QA Testing Guide

**Purpose**: Step-by-step instructions for manually validating all core user workflows before MVP release
**Priority**: P0 - Must complete before v1.0.0 release
**Est. Time**: 2-3 hours for complete validation

---

## Prerequisites

### Start the Application

```bash
# Terminal 1: Start the backend server
cd /path/to/audiobook-organizer
go run main.go serve

# Terminal 2: Start the frontend dev server (if not using embedded)
cd web
npm run dev

# Open browser to http://localhost:3000
```

### Test Data Setup

For comprehensive testing, you'll need:
- [ ] At least 5-10 audiobook files in various formats (M4B, MP3, FLAC)
- [ ] Files with good metadata (some with missing metadata)
- [ ] At least one multi-file audiobook (multiple MP3s for same book)
- [ ] Test import folder (e.g., `~/Downloads/test-audiobooks`)
- [ ] Test library folder (e.g., `~/Audiobooks`)

---

## Test Suite 1: Dashboard & Navigation (15 minutes)

### 1.1 Dashboard Page

**Navigate to**: `http://localhost:3000/`

**Verify**:
- [ ] Dashboard loads without errors
- [ ] Library statistics display correct numbers:
  - Total audiobooks count
  - Total authors count
  - Total series count
  - Import folders count
- [ ] Storage usage shows correct disk space
- [ ] Recent operations section exists (may be empty initially)
- [ ] Quick action buttons visible:
  - "Start Scan" button
  - "Organize All" button

**Take Screenshot**: `dashboard-overview.png`

**Expected Result**: Clean dashboard with accurate statistics

---

## Test Suite 2: Import Path Management (20 minutes)

### 2.1 Navigate to Settings

**Navigate to**: Settings → Import Paths tab

**Verify**:
- [ ] Import paths table loads
- [ ] "Add Import Path" button visible
- [ ] Empty state shows if no paths configured

**Take Screenshot**: `settings-import-paths-empty.png`

### 2.2 Add Import Path

**Steps**:
1. Click "Add Import Path" button
2. Browse to test folder (e.g., `~/Downloads/test-audiobooks`)
3. Click "Select This Folder"
4. Confirm addition

**Verify**:
- [ ] Path appears in import paths table
- [ ] Shows path, book count (0 initially), total size
- [ ] "Scan" and "Remove" action buttons available

**Take Screenshot**: `settings-import-path-added.png`

### 2.3 Trigger Scan Operation

**Steps**:
1. Click "Scan" button for the import path
2. Observe progress indicators

**Verify**:
- [ ] Scan operation starts
- [ ] Progress indicator shows scanning status
- [ ] Book count updates after scan completes
- [ ] Can navigate to other pages while scan runs

**Take Screenshot**: `scan-operation-in-progress.png`

**Expected Result**: Import path shows correct book count after scan

---

## Test Suite 3: Library View & Search (25 minutes)

### 3.1 Navigate to Library

**Navigate to**: Library page

**Verify**:
- [ ] Library view loads
- [ ] Books display in grid or list view
- [ ] Each book card shows:
  - Title
  - Author
  - Cover art placeholder
  - Format badge (M4B, MP3, etc.)
  - Action menu (three dots)

**Take Screenshot**: `library-grid-view.png`

### 3.2 Search Functionality

**Steps**:
1. Enter book title in search box
2. Observe filtered results
3. Clear search
4. Search by author name
5. Clear search

**Verify**:
- [ ] Search filters books correctly
- [ ] Results update in real-time
- [ ] Clear button removes filter
- [ ] Empty state shows if no results

**Take Screenshot**: `library-search-results.png`

### 3.3 Sort & Filter

**Steps**:
1. Click sort dropdown
2. Try each option:
   - Sort by Title (A-Z)
   - Sort by Author (A-Z)
   - Sort by Date Added (Newest first)
   - Sort by Date Modified (Newest first)

**Verify**:
- [ ] Books re-order correctly for each sort option
- [ ] Sort indicator updates

**Take Screenshot**: `library-sorted-by-title.png`

### 3.4 Pagination

**Steps** (if you have 20+ books):
1. Scroll to bottom of page
2. Click "Load More" or next page button
3. Observe additional books loading

**Verify**:
- [ ] Pagination works smoothly
- [ ] Page numbers/indicators update
- [ ] Books don't duplicate

**Expected Result**: Library provides intuitive browsing experience

---

## Test Suite 4: Book Detail Page (30 minutes)

### 4.1 Open Book Detail

**Steps**:
1. Click on any audiobook card in Library
2. Book detail page loads

**Verify**:
- [ ] Detail page loads without errors
- [ ] Tabs visible: Info, Files, Versions, Tags, Compare
- [ ] Book title and metadata display

**Take Screenshot**: `book-detail-info-tab.png`

### 4.2 Info Tab

**Verify**:
- [ ] Title, Author, Series display correctly
- [ ] Narrator, Publisher, Year show (if available)
- [ ] Duration, File size, Format correct
- [ ] Quality information (bitrate, codec) if available
- [ ] "Edit Metadata" button visible
- [ ] "Fetch Metadata" button visible
- [ ] "Parse with AI" button visible (if OpenAI configured)

### 4.3 Files Tab

**Steps**:
1. Click "Files" tab

**Verify**:
- [ ] File list shows all audiobook files
- [ ] Each file shows:
  - Filename
  - File path
  - File size
  - SHA256 hash
  - Media info (codec, bitrate)
- [ ] Copy hash button works (copies to clipboard)

**Take Screenshot**: `book-detail-files-tab.png`

### 4.4 Metadata Editing

**Steps**:
1. Go back to Info tab
2. Click "Edit Metadata" button
3. Edit dialog opens
4. Change title to "Test Title (Modified)"
5. Change author if needed
6. Click "Save"

**Verify**:
- [ ] Edit dialog opens with current values pre-filled
- [ ] Can modify all fields (title, author, series, narrator, etc.)
- [ ] Save button enabled when changes made
- [ ] Success message shows after save
- [ ] Book detail page updates with new values
- [ ] Library view reflects changes

**Take Screenshot**: `book-detail-edit-metadata-dialog.png`

### 4.5 Fetch Metadata from Open Library

**Steps**:
1. Click "Fetch Metadata" button
2. Observe progress indicator
3. Wait for fetch to complete

**Verify**:
- [ ] Fetch operation starts
- [ ] Progress/loading indicator visible
- [ ] Metadata updates if match found
- [ ] Fields update: description, publisher, year, ISBN
- [ ] Success or "no match found" message shows

**Take Screenshot**: `book-detail-after-metadata-fetch.png`

### 4.6 Tags & Compare Tabs

**Steps**:
1. Click "Tags" tab
2. View raw metadata
3. Click "Compare" tab
4. View metadata comparison

**Verify Tags tab**:
- [ ] Shows embedded file tags
- [ ] Shows media info (bitrate, codec, sample rate)
- [ ] Provenance indicators (where each field came from)
- [ ] Lock/unlock buttons for fields

**Take Screenshot**: `book-detail-tags-tab.png`

**Verify Compare tab**:
- [ ] Shows comparison: File Tags vs Stored vs Fetched
- [ ] Effective source highlighted
- [ ] "Use File Value" / "Use Fetched Value" buttons work
- [ ] Can apply different source to field

**Take Screenshot**: `book-detail-compare-tab.png`

### 4.7 Soft Delete Workflow

**Steps**:
1. Go back to Info tab
2. Click "Delete" button (or action menu → Delete)
3. Delete dialog opens
4. Enable "Soft Delete" option
5. Enable "Block Hash" option (prevent reimport)
6. Click "Confirm Delete"

**Verify**:
- [ ] Confirmation dialog shows with options:
  - Soft Delete checkbox
  - Block Hash checkbox
  - Warning about permanent deletion if not soft delete
- [ ] After delete:
  - Returns to Library view
  - Book no longer in main list
  - Success message shows

**Take Screenshot**: `book-detail-delete-dialog.png`

### 4.8 Restore Soft-Deleted Book

**Steps**:
1. In Library, look for "Soft-Deleted" section or filter
2. Find the deleted book
3. Click "Restore" button

**Verify**:
- [ ] Soft-deleted books show in separate list/section
- [ ] Restore button available
- [ ] After restore, book returns to main library
- [ ] Success message shows

**Take Screenshot**: `library-soft-deleted-list.png`

**Expected Result**: Book detail page provides comprehensive book management

---

## Test Suite 5: Batch Operations (15 minutes)

### 5.1 Select Multiple Books

**Steps**:
1. Go to Library page
2. Enable selection mode (checkbox appears on cards)
3. Select 3-5 books by clicking checkboxes
4. Observe bulk action buttons appear

**Verify**:
- [ ] Checkboxes appear on book cards
- [ ] Selection count shows (e.g., "3 selected")
- [ ] Bulk action buttons visible:
  - Bulk Fetch Metadata
  - Bulk Update
  - Bulk Delete

**Take Screenshot**: `library-bulk-selection.png`

### 5.2 Bulk Metadata Fetch

**Steps**:
1. With books selected, click "Bulk Fetch Metadata"
2. Confirmation dialog shows
3. Click "Confirm"
4. Observe progress

**Verify**:
- [ ] Confirmation shows number of books to fetch
- [ ] Progress indicator shows as books are processed
- [ ] Success/failure count updates
- [ ] Can cancel operation mid-progress
- [ ] Selection clears after completion

**Take Screenshot**: `library-bulk-fetch-progress.png`

**Expected Result**: Bulk operations work smoothly for multiple books

---

## Test Suite 6: Settings & Configuration (20 minutes)

### 6.1 Library Settings

**Navigate to**: Settings → Library tab

**Verify**:
- [ ] Library path setting (root_dir) displays
- [ ] Organization mode dropdown (auto/copy/hardlink/reflink)
- [ ] Folder naming pattern field
- [ ] File naming pattern field
- [ ] Pattern preview shows example output
- [ ] "Save Settings" button

**Steps**:
1. Modify folder pattern (e.g., `{author}/{series}/{title}`)
2. Observe preview update
3. Click "Save Settings"

**Verify**:
- [ ] Preview updates immediately
- [ ] Save success message shows
- [ ] Settings persist after page refresh

**Take Screenshot**: `settings-library-organization.png`

### 6.2 Metadata Settings

**Navigate to**: Settings → Metadata tab

**Verify**:
- [ ] Open Library API enable checkbox
- [ ] OpenAI API key field (if AI parsing enabled)
- [ ] "Test Connection" button
- [ ] Auto-fetch metadata checkbox

**Steps** (if OpenAI key configured):
1. Enter API key
2. Click "Test Connection"
3. Observe result

**Verify**:
- [ ] Test shows success or error
- [ ] Settings save correctly

**Take Screenshot**: `settings-metadata.png`

### 6.3 Blocked Hashes

**Navigate to**: Settings → Blocked Hashes tab

**Verify**:
- [ ] Blocked hashes table loads
- [ ] Shows hash, reason, date added
- [ ] "Add Hash" button
- [ ] Remove button for each entry
- [ ] Empty state if no blocked hashes

**Steps**:
1. Click "Add Hash"
2. Enter a test hash (64 hex characters)
3. Enter reason (e.g., "Test blocked hash")
4. Click "Add"

**Verify**:
- [ ] Hash validation (must be 64 hex characters)
- [ ] Hash appears in table after add
- [ ] Can remove hash with confirmation

**Take Screenshot**: `settings-blocked-hashes.png`

### 6.4 System Info

**Navigate to**: Settings → System tab (or System page)

**Verify**:
- [ ] System information displays:
  - OS and version
  - Go version
  - App version
  - Database path
  - Memory usage
  - CPU count
  - Uptime

**Take Screenshot**: `system-info.png`

**Expected Result**: All settings save and persist correctly

---

## Test Suite 7: Organize Workflow (25 minutes)

### 7.1 Trigger Organize Operation

**Steps**:
1. Go to Library page
2. Find books in "import" state (newly scanned)
3. Click "Organize" button (global or per-book)
4. Observe progress

**Verify**:
- [ ] Organize operation starts
- [ ] Progress shows files being processed
- [ ] Files move/copy to organized structure
- [ ] Book state changes to "organized"
- [ ] File paths update to new locations

**Take Screenshot**: `organize-operation-progress.png`

### 7.2 Verify Organized Files

**Steps**:
1. Open Finder/File Explorer
2. Navigate to library path
3. Verify folder structure matches pattern

**Verify**:
- [ ] Folders created with correct names (author/series/title)
- [ ] Files moved to correct locations
- [ ] Original files remain in import path (if using copy mode)
- [ ] File integrity maintained (hashes match)

**Take Screenshot**: `organized-files-filesystem.png`

**Expected Result**: Audiobooks organize into clean folder structure

---

## Test Suite 8: Version Management (15 minutes)

### 8.1 Link Versions

**Steps**:
1. Import two versions of same audiobook (e.g., different quality)
2. Open first book in Book Detail
3. Click "Manage Versions" or similar button
4. Link to second book

**Verify**:
- [ ] Version management dialog opens
- [ ] Can search for book to link
- [ ] Link creates version group
- [ ] Both books show "Multiple Versions" badge
- [ ] Version quality indicators show (bitrate, codec)

**Take Screenshot**: `version-management-dialog.png`

### 8.2 Set Primary Version

**Steps**:
1. In version management, mark one as primary
2. Observe visual indicator

**Verify**:
- [ ] Primary version shows star or badge
- [ ] Primary version displays first in links
- [ ] Can switch primary version

**Take Screenshot**: `versions-with-primary.png`

**Expected Result**: Version management helps organize duplicate books

---

## Test Suite 9: State Transitions (10 minutes)

### 9.1 Verify Full Lifecycle

**Test the complete workflow**:
```
New file → Import → Organized → Soft Delete → Purge
```

**Steps**:
1. Add new audiobook to import path
2. Scan import path (state: "import")
3. Organize book (state: "organized")
4. Soft delete book (state: "deleted", marked_for_deletion: true)
5. View in soft-deleted list
6. Either restore (back to "organized") or purge (permanently removed)

**Verify**:
- [ ] Each state transition works correctly
- [ ] States displayed accurately in UI
- [ ] Filters work for each state
- [ ] Counts update correctly

**Expected Result**: State machine handles full audiobook lifecycle

---

## Test Suite 10: Error Handling (10 minutes)

### 10.1 Network Errors

**Steps**:
1. Stop backend server
2. Try actions in frontend
3. Observe error messages

**Verify**:
- [ ] Clear error messages show
- [ ] Reconnection attempts visible
- [ ] No app crashes
- [ ] Graceful degradation

### 10.2 Invalid Input

**Steps**:
1. Try to add import path with invalid/nonexistent path
2. Try to save metadata with invalid values
3. Try to add blocked hash with invalid format

**Verify**:
- [ ] Validation errors show inline
- [ ] Helpful error messages
- [ ] Form prevents invalid submission

**Expected Result**: App handles errors gracefully with helpful messages

---

## Post-Testing Checklist

### Verify Data Integrity

- [ ] All books in database have correct metadata
- [ ] File paths are accurate
- [ ] Hashes match actual files
- [ ] No orphaned database entries
- [ ] No corrupted data

### Performance Check

- [ ] Library loads quickly (< 2 seconds for 100 books)
- [ ] Scans complete in reasonable time
- [ ] No memory leaks (check with long-running session)
- [ ] No browser console errors

### Cross-Browser (if time permits)

- [ ] Chrome/Edge (primary)
- [ ] Firefox (secondary)
- [ ] Safari (macOS only)

---

## Issues Found Template

Use this template to document any issues found during QA:

```markdown
### Issue #X: [Brief Description]

**Severity**: Critical | High | Medium | Low
**Component**: Dashboard | Library | Settings | Book Detail | etc.
**Steps to Reproduce**:
1. Step 1
2. Step 2
3. Step 3

**Expected Result**: What should happen

**Actual Result**: What actually happens

**Screenshot**: `issue-X-description.png`

**Workaround**: If any

**Notes**: Additional context
```

---

## Sign-Off

**Tester Name**: _______________________
**Date**: _______________________
**Version Tested**: _______________________
**Overall Assessment**: ☐ Pass ☐ Pass with Issues ☐ Fail
**Ready for Release**: ☐ Yes ☐ No ☐ With Fixes

**Comments**:


---

**End of Manual QA Guide**
**Status**: Ready for testing
**Last Updated**: 2026-01-27
