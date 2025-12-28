<!-- file: docs/MANUAL_TEST_PLAN.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->

# Manual Test Plan - Audiobook Organizer

## Overview

This document provides comprehensive manual test scenarios for the
audiobook-organizer application. These tests complement automated E2E tests and
focus on user-facing workflows that require human validation.

**Target Audience**: QA Engineers, Developers, Product Managers

**Prerequisites**:

- Application running locally or in test environment
- Sample audiobook files available (see Test Data Setup Guide)
- Basic understanding of application features
- Browser developer tools for debugging (optional)

**Related Documentation**:

- [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md) - Critical scenarios for PR
  #79 merge
- [Test Data Setup Guide](./TEST_DATA_SETUP_GUIDE.md) - How to create test
  audiobook files
- [E2E Test Coverage](../web/tests/e2e/TEST_COVERAGE_SUMMARY.md) - Automated
  test scenarios

---

## Test Environment Setup

### Prerequisites

1. **Application Running**

   ```bash
   cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
   go build -o audiobook-organizer
   ./audiobook-organizer serve --port 8888 --dir /path/to/test/audiobooks
   ```

2. **Browser Access**

   - Navigate to: http://localhost:8888
   - Recommended: Chrome or Firefox (latest versions)
   - Enable browser console for debugging

3. **Test Data Ready**
   - At least 3 sample audiobook files in various formats (m4b, mp3)
   - Known file hashes for blocked hash testing
   - Sample metadata values for override testing

### Test Environment Reset

To start fresh between test sessions:

```bash
# Stop application
# Delete test database
rm -f audiobooks.pebble

# Restart application
./audiobook-organizer serve --port 8888 --dir /path/to/test/audiobooks
```

---

## Test Scenarios by Feature Area

### 1. Metadata Provenance (PR #79)

**Priority**: P0 (Critical for PR #79 merge)

#### 1.1 Provenance Data Display

**Objective**: Verify that metadata sources are correctly displayed in the UI

**Prerequisites**:

- At least one audiobook imported with file metadata
- Book has been fetched from metadata API (or mock data available)

**Test Steps**:

1. Navigate to Library page
2. Click on an audiobook to open Book Detail page
3. Click on the "Tags" tab
4. Verify the following for each metadata field:
   - Effective value is displayed prominently
   - Source chip is visible (file/fetched/stored/override)
   - Chip color matches source type:
     - `file`: Blue/primary color
     - `fetched`: Green/success color
     - `stored`: Grey/default color
     - `override`: Orange/warning color
   - Lock icon appears only for locked override fields

**Expected Results**:

- All metadata fields display correct effective values
- Source chips accurately reflect data source
- Visual hierarchy is clear (value > source > lock status)
- No console errors in browser developer tools

**Pass/Fail Criteria**:

- ✅ PASS: All fields display correct values and sources
- ❌ FAIL: Missing values, incorrect sources, or console errors

**Screenshot Guidance**: Capture full Tags tab with multiple fields showing
different sources

---

#### 1.2 Override from File Value

**Objective**: Apply file metadata as override and verify persistence

**Prerequisites**:

- Book with file metadata that differs from stored value
- Example: File has title "Book Title (File)", DB has "Book Title (Stored)"

**Test Steps**:

1. Open Book Detail page for test book
2. Click "Compare" tab
3. Locate a field with different file vs stored value (e.g., title)
4. Note the current effective value in the page heading
5. Click "Use File" button for the target field
6. Observe:
   - Effective value updates immediately in heading
   - Source chip changes to "override"
   - UI shows confirmation (snackbar or similar)
7. Click "Tags" tab
8. Verify:
   - Field shows "override" source chip
   - Effective value matches file value
9. Refresh browser (F5)
10. Verify:
    - Override persists after page reload
    - Values remain correct

**Expected Results**:

- Override applies instantly without page reload
- Source chip updates to "override"
- Data persists across page refresh
- API request succeeds (check Network tab: `PATCH /api/v1/audiobooks/:id`)

**Pass/Fail Criteria**:

- ✅ PASS: Override applies, persists, and displays correctly
- ❌ FAIL: Override doesn't apply, values incorrect, or doesn't persist

**Edge Cases to Test**:

- Apply override to field with null file value (button should be disabled)
- Apply override to already-overridden field (should update)
- Apply multiple overrides in sequence

---

#### 1.3 Override from Fetched Value

**Objective**: Apply metadata API value as override

**Prerequisites**:

- Book with fetched metadata available
- Field where fetched value differs from current effective value

**Test Steps**:

1. Open Book Detail page
2. Navigate to Compare tab
3. Find field with fetched value (e.g., publisher from metadata API)
4. Click "Use Fetched" button
5. Verify:
   - Effective value updates to fetched value
   - Source chip changes to "override"
   - Confirmation message appears
6. Navigate to Tags tab
7. Verify override persists and displays correctly
8. Refresh page and revalidate

**Expected Results**:

- Fetched value applies as override
- Source priority: override > fetched (confirms override took precedence)
- Persistence across navigation and refresh

**Pass/Fail Criteria**:

- ✅ PASS: Fetched override applies and persists correctly
- ❌ FAIL: Override fails, incorrect value, or doesn't persist

---

#### 1.4 Clear Override

**Objective**: Remove override and revert to next priority source

**Prerequisites**:

- Field with active override

**Test Steps**:

1. Open Book Detail → Compare tab
2. Locate field with override (orange "override" chip)
3. Note the stored/fetched/file value that will become effective after clearing
4. Click "Clear Override" or equivalent action button
5. Verify:
   - Effective value changes to next priority source
   - Source chip updates (stored/fetched/file)
   - Override no longer listed in Tags tab
6. Refresh page
7. Verify override remains cleared

**Expected Results**:

- Override clears successfully
- Value reverts to correct source based on priority (stored > fetched > file)
- Clearing persists after refresh
- API request succeeds: `PATCH /api/v1/audiobooks/:id` with overrides removed

**Pass/Fail Criteria**:

- ✅ PASS: Override clears and correct source value displays
- ❌ FAIL: Override doesn't clear, wrong value displays, or doesn't persist

---

#### 1.5 Lock/Unlock Metadata Field

**Objective**: Lock override to prevent automatic updates

**Prerequisites**:

- Field with override applied

**Test Steps**:

1. Open Book Detail → Tags tab
2. Locate field with override
3. Click lock icon or toggle lock control
4. Verify:
   - Lock icon changes to "locked" state
   - Visual indicator shows field is locked
5. Navigate away and return
6. Verify lock status persists
7. Click lock icon again to unlock
8. Verify unlock state persists

**Expected Results**:

- Lock toggles on/off successfully
- Locked state prevents field updates from metadata fetch (future test)
- Lock status visible in both Tags and Compare tabs
- Lock persists across sessions

**Pass/Fail Criteria**:

- ✅ PASS: Lock toggles correctly and persists
- ❌ FAIL: Lock doesn't toggle, incorrect state, or doesn't persist

**Note**: Full lock functionality validation requires metadata fetch feature
(future test)

---

#### 1.6 Source Priority Validation

**Objective**: Verify correct source priority: override > stored > fetched >
file

**Prerequisites**:

- Create test book with all four source types for a field:
  - file: "Title from File"
  - fetched: "Title from API"
  - stored: "Title Stored"
  - override: None initially

**Test Steps**:

1. Open Book Detail → Compare tab
2. Verify initial effective value is "Title Stored" (stored takes priority)
3. Note source chip shows "stored"
4. Apply override from file value
5. Verify effective value changes to "Title from File"
6. Verify source chip shows "override"
7. Clear override
8. Verify effective value returns to "Title Stored"
9. Remove stored value via database manipulation (advanced test)
10. Verify effective value falls back to "Title from API" (fetched)
11. Remove fetched value
12. Verify effective value falls back to "Title from File" (file)

**Expected Results**:

- Priority order strictly enforced
- Each source level correctly takes precedence
- UI accurately reflects current source

**Pass/Fail Criteria**:

- ✅ PASS: All priority transitions work correctly
- ❌ FAIL: Wrong priority order, incorrect fallback values

**Advanced**: Use database tools to manipulate source values directly

---

### 2. Blocked Hashes Management (PR #69)

**Priority**: P0 (Critical - Manual verification required)

#### 2.1 View Blocked Hashes

**Objective**: Display blocked hashes in Settings tab

**Prerequisites**:

- Fresh application instance (or cleared blocked hashes)

**Test Steps**:

1. Navigate to Settings page
2. Click "Blocked Hashes" tab
3. Verify empty state displays:
   - Informative message: "No Blocked Hashes"
   - Explanation of feature
   - "Add Blocked Hash" button visible
4. Add a test hash (see next test)
5. Return to Blocked Hashes tab
6. Verify table displays:
   - Hash (truncated to first 12 characters)
   - Full hash visible on hover (tooltip or click)
   - Reason text
   - Blocked date (formatted)
   - Delete/unblock button

**Expected Results**:

- Empty state is clear and helpful
- Table displays all hash details correctly
- UI is responsive and accessible
- Date formatting is locale-appropriate

**Pass/Fail Criteria**:

- ✅ PASS: Empty state and table display correctly
- ❌ FAIL: Missing elements, incorrect data, layout issues

**Screenshot Guidance**: Capture empty state and populated table

---

#### 2.2 Add Blocked Hash

**Objective**: Add file hash to blocklist with validation

**Prerequisites**:

- Have a test SHA256 hash ready:
  `a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd`

**Test Steps**:

1. Navigate to Settings → Blocked Hashes
2. Click "Add Blocked Hash" button
3. Verify dialog opens with:
   - Hash input field (64 character SHA256)
   - Reason input field (text)
   - Validation hints
4. **Test Validation**: Enter invalid hash (too short, non-hex characters)
   - Enter: `abc123`
   - Click Save
   - Verify error message: "Hash must be 64 hexadecimal characters (SHA256)"
5. **Test Validation**: Enter hash without reason
   - Enter valid 64-char hash
   - Leave reason blank
   - Click Save
   - Verify error: "Hash and reason are required"
6. **Valid Entry**:
   - Enter valid hash:
     `a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd`
   - Enter reason: "Test blocked hash for manual QA"
   - Click Save
7. Verify:
   - Dialog closes
   - Success snackbar: "Hash blocked successfully"
   - New entry appears in table
   - API request succeeds: `POST /api/v1/blocked-hashes`

**Expected Results**:

- Validation catches all invalid inputs
- Valid hash saves successfully
- UI provides clear feedback
- Data persists (visible after page refresh)

**Pass/Fail Criteria**:

- ✅ PASS: Validation works, valid hash saves successfully
- ❌ FAIL: Validation fails, can't save valid hash, no feedback

**Edge Cases**:

- Try adding duplicate hash (should update or show error)
- Test with uppercase/lowercase hex (should accept both)
- Very long reason text (test UI handling)

---

#### 2.3 Delete Blocked Hash

**Objective**: Remove hash from blocklist

**Prerequisites**:

- At least one blocked hash in system

**Test Steps**:

1. Navigate to Settings → Blocked Hashes
2. Locate test hash in table
3. Click delete/unblock button (trash icon)
4. Verify confirmation dialog appears:
   - Shows full hash (not truncated)
   - Shows reason
   - Clear warning message
   - Cancel and Confirm buttons
5. Click Cancel
   - Verify dialog closes
   - Hash remains in table
6. Click delete button again
7. Click Confirm in dialog
8. Verify:
   - Dialog closes
   - Success message: "Hash unblocked successfully"
   - Hash removed from table
   - API request: `DELETE /api/v1/blocked-hashes/:hash`
9. Refresh page
10. Verify hash remains deleted

**Expected Results**:

- Confirmation dialog prevents accidental deletion
- Hash deletes successfully
- Deletion persists across refresh
- UI updates immediately

**Pass/Fail Criteria**:

- ✅ PASS: Confirmation works, deletion succeeds and persists
- ❌ FAIL: No confirmation, deletion fails, or doesn't persist

**Screenshot Guidance**: Capture delete confirmation dialog with full hash
visible

---

#### 2.4 Blocked Hash Prevents Reimport

**Objective**: Verify blocked hash prevents file import during scan

**Prerequisites**:

- Audiobook file with known hash
- Hash added to blocklist

**Test Steps**:

1. Get hash of test audiobook file:
   ```bash
   shasum -a 256 /path/to/test/audiobook.m4b
   ```
2. Add hash to blocklist via Settings tab with reason: "Manual test - prevent
   reimport"
3. Place audiobook file in import directory
4. Trigger import scan:
   ```bash
   curl -X POST http://localhost:8888/api/v1/operations/scan
   ```
5. Monitor scan logs:
   ```bash
   tail -f logs/audiobook-organizer.log
   ```
6. Verify log message:
   `Skipping file: hash blocked: <hash> (reason: Manual test - prevent reimport)`
7. Check Library page - verify file was NOT imported
8. Check dashboard stats - verify count unchanged

**Expected Results**:

- Scanner detects blocked hash
- File skipped with logged reason
- File does not appear in library
- No duplicate entries created

**Pass/Fail Criteria**:

- ✅ PASS: File skipped, logged correctly, not in library
- ❌ FAIL: File imported despite blocked hash

**Advanced Test**: Remove hash from blocklist, re-scan, verify file now imports

---

### 3. State Transitions & Delete Flows (PR #70)

**Priority**: P0 (Critical - Manual verification required)

#### 3.1 Import → Organized State Transition

**Objective**: Verify book state transitions through lifecycle

**Prerequisites**:

- New audiobook file not yet imported

**Test Steps**:

1. Place audiobook in import directory
2. Trigger scan operation
3. After scan completes, find book in Library
4. Open Book Detail page
5. Check book state (via browser console or API):
   ```javascript
   // In browser console
   fetch('/api/v1/audiobooks/<book-id>')
     .then(r => r.json())
     .then(d => console.log('State:', d.library_state));
   ```
6. Verify state is `imported`
7. Trigger organize operation for the book
8. After organize completes, check state again
9. Verify state changed to `organized`
10. Check file moved to organized directory
11. Verify book still appears in Library

**Expected Results**:

- Initial state: `imported`
- After organize: `organized`
- State persists in database
- File physically moved to organized location
- Book remains visible in Library

**Pass/Fail Criteria**:

- ✅ PASS: State transitions correctly, file organized
- ❌ FAIL: State doesn't change, file not moved, or errors occur

**API Validation**:

```bash
curl http://localhost:8888/api/v1/audiobooks/<book-id> | jq '.library_state'
```

---

#### 3.2 Soft Delete

**Objective**: Mark book for deletion without removing from database

**Prerequisites**:

- Book in `organized` state

**Test Steps**:

1. Navigate to Library page
2. Locate test book
3. Click delete button/icon
4. Verify delete dialog opens with options:
   - "Delete" or "Soft Delete" confirmation
   - Checkbox: "Prevent reimporting" (optional)
   - Reason text field (if prevent reimporting checked)
   - Cancel and Confirm buttons
5. **Test Soft Delete WITHOUT blocking hash**:
   - Uncheck "Prevent reimporting"
   - Click Confirm
6. Verify:
   - Book removed from Library list
   - Success message appears
   - Soft-delete count updates (if displayed)
   - API request: `DELETE /api/v1/audiobooks/<book-id>` (soft delete)
7. Check book state via API:
   ```bash
   curl http://localhost:8888/api/v1/audiobooks/<book-id> | jq '{library_state, marked_for_deletion, marked_for_deletion_at}'
   ```
8. Verify:
   - `library_state`: `deleted`
   - `marked_for_deletion`: `true`
   - `marked_for_deletion_at`: recent timestamp
9. Verify file still exists on filesystem

**Expected Results**:

- Book marked as deleted in database
- Book hidden from Library list
- File NOT deleted from filesystem
- State properly tracked
- Deletion reversible (see restore test)

**Pass/Fail Criteria**:

- ✅ PASS: Soft delete completes, state correct, file intact
- ❌ FAIL: Hard delete occurs, state wrong, or file removed

---

#### 3.3 Soft Delete with Hash Blocking

**Objective**: Soft delete and add hash to blocklist

**Prerequisites**:

- Book in `organized` state with known hash

**Test Steps**:

1. Open delete dialog for test book
2. Check "Prevent reimporting" checkbox
3. Enter reason: "Low quality version - testing"
4. Click Confirm
5. Verify:
   - Book soft-deleted (removed from Library)
   - Success message includes hash blocking confirmation
6. Navigate to Settings → Blocked Hashes
7. Verify new entry appears:
   - Hash matches book's library_hash or original_hash
   - Reason matches entered text
   - Blocked date is today
8. Verify via API:
   ```bash
   curl http://localhost:8888/api/v1/blocked-hashes | jq '.items[] | select(.reason | contains("Low quality"))'
   ```
9. Attempt to reimport same file (copy to import dir and scan)
10. Verify file is skipped due to blocked hash

**Expected Results**:

- Soft delete completes
- Hash added to blocklist
- Reason stored correctly
- Reimport blocked by hash check

**Pass/Fail Criteria**:

- ✅ PASS: Delete and hash blocking both succeed
- ❌ FAIL: Hash not blocked, reimport not prevented

**Edge Case**: Test with book that has both original_hash and library_hash
(should block both or library_hash only based on implementation)

---

#### 3.4 Restore Soft-Deleted Book

**Objective**: Undelete book and return to library

**Prerequisites**:

- Book in soft-deleted state

**Test Steps**:

1. Navigate to Library page
2. Look for soft-delete indicator or button:
   - Example: "Show Deleted (3)" button
   - Or: separate "Deleted Books" section
3. Click to view soft-deleted books
4. Locate test book in deleted list
5. Click "Restore" button
6. Verify:
   - Confirmation dialog (optional)
   - Book removed from deleted list
   - Book appears in main Library
   - Success message: "Book restored successfully"
7. Open Book Detail page for restored book
8. Verify state via API:
   ```bash
   curl http://localhost:8888/api/v1/audiobooks/<book-id> | jq '{library_state, marked_for_deletion}'
   ```
9. Verify:
   - `library_state`: `organized` (or `imported`)
   - `marked_for_deletion`: `false`

**Expected Results**:

- Book restored to library
- State correctly updated
- Book fully functional after restore
- Deletion metadata cleared

**Pass/Fail Criteria**:

- ✅ PASS: Restore succeeds, state correct, book usable
- ❌ FAIL: Restore fails, state wrong, book remains deleted

**Note**: If hash was blocked during delete, it remains blocked after restore
(expected behavior - test separately if unblocking is needed)

---

#### 3.5 Purge Soft-Deleted Books

**Objective**: Permanently delete soft-deleted books

**Prerequisites**:

- Multiple books in soft-deleted state

**Test Steps**:

1. Navigate to Library page
2. View soft-deleted books list
3. Verify purge button is visible (e.g., "Purge Deleted Books")
4. Note count of soft-deleted books
5. Click purge button
6. Verify purge confirmation dialog:
   - Shows count of books to purge
   - Warning message about permanent deletion
   - Option: "Delete files from disk" checkbox
   - Cancel and Confirm buttons
7. **Test 1: Purge WITHOUT deleting files**:
   - Uncheck "Delete files from disk"
   - Click Confirm
8. Verify:
   - Progress indicator or loading state
   - Success message with count: "Purged X books"
   - Soft-deleted list now empty
   - Books no longer queryable via API
   - Files still exist on filesystem
9. **Test 2: Purge WITH deleting files**:
   - Soft-delete another book
   - Navigate to purge dialog
   - Check "Delete files from disk"
   - Click Confirm
10. Verify:
    - Book purged from database
    - File deleted from filesystem
    - Success message indicates file deletion

**Expected Results**:

- Purge removes books from database permanently
- File deletion option works as expected
- UI correctly updates counts
- No orphaned database entries remain

**Pass/Fail Criteria**:

- ✅ PASS: Purge succeeds, files handled per option, UI updates
- ❌ FAIL: Purge fails, files incorrectly deleted/retained, database errors

**Safety Check**: Verify purge is irreversible (books cannot be restored after
purge)

**API Validation**:

```bash
# Before purge
curl http://localhost:8888/api/v1/audiobooks/soft-deleted | jq '.items | length'

# After purge (should return 0 or empty)
curl http://localhost:8888/api/v1/audiobooks/soft-deleted | jq '.items | length'
```

---

#### 3.6 Auto-Purge After Retention Period

**Objective**: Verify automatic purge of old soft-deleted books

**Prerequisites**:

- Application configured with purge retention period:
  ```bash
  # In config or environment
  PURGE_SOFT_DELETED_AFTER_DAYS=30
  ```
- Books soft-deleted more than 30 days ago (requires database manipulation or
  time travel)

**Test Steps** (Advanced):

1. **Setup**: Manually update `marked_for_deletion_at` in database to simulate
   old deletion:
   ```sql
   UPDATE books
   SET marked_for_deletion_at = datetime('now', '-35 days')
   WHERE id = '<test-book-id>';
   ```
2. Restart application or trigger auto-purge job:
   ```bash
   # If auto-purge runs on startup or cron
   # Check application logs for purge execution
   ```
3. Monitor logs:
   ```bash
   grep "Auto-purge" logs/audiobook-organizer.log
   ```
4. Verify log message indicates purge:
   ```
   [INFO] Auto-purge soft-deleted books: attempted=1 purged=1 files_deleted=0 errors=0
   ```
5. Check book no longer in database:
   ```bash
   curl http://localhost:8888/api/v1/audiobooks/<book-id>
   # Should return 404 Not Found
   ```
6. Verify file handling based on config (`PURGE_SOFT_DELETED_DELETE_FILES`)

**Expected Results**:

- Auto-purge runs on schedule
- Old soft-deleted books purged automatically
- Logs show purge activity
- File deletion follows configuration

**Pass/Fail Criteria**:

- ✅ PASS: Auto-purge runs, old books removed, logs accurate
- ❌ FAIL: Purge doesn't run, books remain, or errors occur

**Note**: This test may require advanced setup (database manipulation, time
mocking). Mark as "Optional - Advanced" if time-constrained.

---

### 4. Book Detail Page Functionality

**Priority**: P1 (High)

#### 4.1 Navigation and Layout

**Objective**: Verify book detail page loads and displays correctly

**Test Steps**:

1. Navigate to Library page
2. Click on any book to open detail page
3. Verify page layout:
   - Book title in header
   - Tab navigation: Tags, Compare, (other tabs)
   - Tab content area
   - Back button or navigation breadcrumbs
4. Click each tab
5. Verify tab content loads without errors
6. Use browser back button
7. Verify returns to Library page

**Expected Results**:

- Page loads within 2 seconds
- All tabs accessible and functional
- Navigation smooth without errors
- Responsive design on different screen sizes

**Pass/Fail Criteria**:

- ✅ PASS: All elements render, tabs work, navigation functional
- ❌ FAIL: Missing elements, tabs don't load, navigation broken

---

#### 4.2 Media Info Display

**Objective**: Verify file metadata displays correctly

**Prerequisites**:

- Book with rich file metadata (duration, bitrate, codec, etc.)

**Test Steps**:

1. Open Book Detail page
2. Navigate to Tags tab
3. Locate "Media Info" section (or similar)
4. Verify displayed information:
   - Duration (formatted as HH:MM:SS)
   - File size (formatted in MB/GB)
   - Bitrate (e.g., "128 kbps")
   - Codec/format (e.g., "AAC", "MP3")
   - Sample rate (e.g., "44.1 kHz")
   - Channels (e.g., "Stereo", "Mono")
5. Compare with actual file metadata:
   ```bash
   ffprobe /path/to/audiobook.m4b 2>&1 | grep -E "Duration|bitrate|codec|Hz"
   ```
6. Verify values match

**Expected Results**:

- All media info fields populated
- Values accurate compared to file
- Formatting human-readable
- No "unknown" or "N/A" for available data

**Pass/Fail Criteria**:

- ✅ PASS: Media info accurate and well-formatted
- ❌ FAIL: Missing info, incorrect values, poor formatting

---

### 5. Import and Scan Operations

**Priority**: P1 (High)

#### 5.1 Basic Import Scan

**Objective**: Import audiobooks via scan operation

**Prerequisites**:

- Clean import directory with 2-3 test audiobook files

**Test Steps**:

1. Navigate to Library page (or Import page if separate)
2. Click "Scan" or "Import" button
3. Verify:
   - Scan operation starts
   - Progress indicator appears
   - Real-time status updates (optional)
4. Wait for scan completion
5. Verify:
   - Success message with count: "Imported X books"
   - New books appear in Library
   - Book metadata populated from files
6. Check each imported book:
   - Open Book Detail page
   - Verify title, author extracted correctly
   - Verify file path correct
   - Verify state is `imported`

**Expected Results**:

- All files detected and imported
- Metadata extracted accurately
- No errors in logs
- Books immediately visible in library

**Pass/Fail Criteria**:

- ✅ PASS: All files imported successfully with correct metadata
- ❌ FAIL: Files missed, metadata wrong, or errors occur

**Edge Cases**:

- Import directory empty (should report 0 imported)
- Import duplicate file (should detect and skip)
- Import file with missing metadata tags

---

### 6. Settings and Configuration

**Priority**: P2 (Medium)

#### 6.1 Retention Settings

**Objective**: Configure auto-purge retention period

**Prerequisites**:

- Access to Settings page

**Test Steps**:

1. Navigate to Settings page
2. Locate "Soft Delete Retention" or similar section
3. Verify current retention period displayed (e.g., 30 days)
4. Change retention period:
   - Enter new value (e.g., 60 days)
   - Toggle "Delete files when purging" checkbox
5. Click Save
6. Verify:
   - Success message appears
   - Settings persist after page refresh
7. Check configuration via API or database:
   ```bash
   curl http://localhost:8888/api/v1/settings | jq '.purge_soft_deleted_after_days'
   ```

**Expected Results**:

- Settings save successfully
- Values persist across sessions
- Configuration affects auto-purge behavior (validate in separate test)

**Pass/Fail Criteria**:

- ✅ PASS: Settings save and persist correctly
- ❌ FAIL: Settings don't save, values revert, or no feedback

---

## Accessibility Testing

**Priority**: P2 (Medium)

### Keyboard Navigation

**Test Steps**:

1. Use Tab key to navigate through UI
2. Verify focus indicators visible on all interactive elements
3. Use Enter/Space to activate buttons
4. Use Escape to close dialogs
5. Test form inputs with keyboard only

**Expected Results**:

- All interactive elements keyboard-accessible
- Focus order logical
- No keyboard traps
- Shortcuts documented (if any)

### Screen Reader Testing (Optional)

**Prerequisites**: Screen reader software (VoiceOver, NVDA, JAWS)

**Test Steps**:

1. Enable screen reader
2. Navigate through key pages
3. Verify labels and descriptions read correctly
4. Test form inputs with screen reader
5. Verify table data announced properly

---

## Performance Testing

**Priority**: P2 (Medium)

### Large Library Performance

**Objective**: Verify performance with large number of books

**Prerequisites**:

- 100+ books in library (use database seeding script)

**Test Steps**:

1. Navigate to Library page
2. Measure page load time (should be <3 seconds)
3. Scroll through list
4. Verify smooth scrolling (60fps)
5. Use search/filter features
6. Measure filter response time (<1 second)
7. Open Book Detail for random book
8. Measure detail page load (<2 seconds)

**Expected Results**:

- Acceptable load times even with large dataset
- UI remains responsive
- No browser freezing or lag

---

## Error Handling

**Priority**: P2 (Medium)

### Network Error Scenarios

**Test Steps**:

1. Start application
2. Open browser DevTools → Network tab
3. Simulate offline mode
4. Try to load Library page
5. Verify error message displayed
6. Try to perform operations (should fail gracefully)
7. Restore network
8. Verify application recovers

**Expected Results**:

- Clear error messages for network failures
- No silent failures
- Application recovers when network restored
- Data integrity maintained

---

## Test Data Cleanup

After completing test sessions:

```bash
# Remove test blocked hashes
curl -X DELETE http://localhost:8888/api/v1/blocked-hashes/<test-hash>

# Purge soft-deleted test books
curl -X DELETE http://localhost:8888/api/v1/audiobooks/purge-soft-deleted?delete_files=false

# Or reset database completely
rm -f audiobooks.pebble
```

---

## Test Reporting

### Issue Template

When reporting issues discovered during testing, use this template:

```markdown
**Test Scenario**: [e.g., 1.2 Override from File Value] **Priority**: [P0/P1/P2]
**Status**: FAIL

**Description**: [Brief description of the issue]

**Steps to Reproduce**:

1. [Step 1]
2. [Step 2]
3. [Step 3]

**Expected Result**: [What should happen]

**Actual Result**: [What actually happened]

**Environment**:

- OS: [macOS/Linux/Windows]
- Browser: [Chrome 120 / Firefox 121]
- Application Version: [commit hash or version]

**Screenshots/Logs**: [Attach relevant screenshots or log excerpts]

**Severity**: [Critical/High/Medium/Low]
```

### Test Summary Report Template

```markdown
# Manual Test Session Report

**Date**: YYYY-MM-DD **Tester**: [Name] **Build**: [commit hash] **Duration**:
[hours]

## Summary

- Total Tests: X
- Passed: X
- Failed: X
- Blocked: X
- Pass Rate: X%

## Critical Issues (P0)

1. [Issue #1 - Brief description]
2. [Issue #2 - Brief description]

## High Priority Issues (P1)

[List issues]

## Notes

[Additional observations, blockers, recommendations]

## Recommendations

- [ ] [Action item 1]
- [ ] [Action item 2]
```

---

## Appendix A: Browser Console Commands

Useful commands for testing via browser console:

```javascript
// Get current book state
fetch('/api/v1/audiobooks/<book-id>')
  .then(r => r.json())
  .then(d =>
    console.table({
      state: d.library_state,
      deleted: d.marked_for_deletion,
      deleted_at: d.marked_for_deletion_at,
    })
  );

// List blocked hashes
fetch('/api/v1/blocked-hashes')
  .then(r => r.json())
  .then(d => console.table(d.items));

// Check soft-deleted books
fetch('/api/v1/audiobooks/soft-deleted')
  .then(r => r.json())
  .then(d => console.log('Soft-deleted count:', d.items.length));
```

---

## Appendix B: Test Data Examples

### Sample SHA256 Hashes for Testing

```
# Test Hash 1 (valid format)
a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd

# Test Hash 2 (valid format)
1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef

# Invalid Hash (too short)
abc123def456

# Invalid Hash (non-hex characters)
xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Sample Override Scenarios

| Field     | File Value          | Fetched Value      | Stored Value          | Override To Test |
| --------- | ------------------- | ------------------ | --------------------- | ---------------- |
| Title     | "Book Title (File)" | "Book Title (API)" | "Book Title (Stored)" | File             |
| Author    | "John Doe"          | "John R. Doe"      | "J. Doe"              | Fetched          |
| Narrator  | "Jane Smith"        | "Jane A. Smith"    | null                  | File             |
| Publisher | null                | "Penguin Books"    | "Unknown Publisher"   | Fetched          |
| Year      | 2020                | 2021               | 2022                  | Stored (clear)   |

---

## Version History

- **1.0.0** (2025-12-28): Initial manual test plan created
  - Comprehensive scenarios for PR #79 (Metadata Provenance)
  - Scenarios for PR #69 (Blocked Hashes) and PR #70 (State Transitions)
  - Test reporting templates and appendices

---

**Next Steps**: See [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md) for
critical scenarios required before PR #79 merge.
