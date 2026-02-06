<!-- file: docs/plans/2026-02-05-itunes-bidirectional-sync-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: e1f2a3b4-c5d6-7890-efgh-i1j2k3l4m5n6 -->

# iTunes Bidirectional Sync - Design Document

**Goal:** Implement comprehensive bidirectional sync between iTunes and the audiobook organizer with conflict resolution UI, extensive testing, and demonstration in the comprehensive demo.

**Priority Order:** (1) Tests, (2) Force-sync UI buttons, (3) Demo integration

**Architecture:** iTunes sync operates bidirectionally with three main flows:
- **Import Flow** (iTunes → Organizer): Validate library → Import metadata → Store in DB
- **Write-Back Flow** (Organizer → iTunes): Collect changes → Detect conflicts → Sync to XML
- **Conflict Resolution** (Both directions): When changes exist in both systems, user explicitly picks winning version

**Infrastructure:**
- Real test library at `testdata/itunes/` with thousands of books
- Conflict detection: Track metadata source and last-modified timestamps
- UI dialogs for force override during conflicts
- Selective sync: Let users choose which books/fields to include
- Data sync state per book: Track iTunes sync dates and organizer edit dates
- Metadata provenance: Store which system has authoritative version
- Operation logging for audit trail

---

## Task 1: Comprehensive Bidirectional Sync Tests

**Test Data Strategy:**

Using the real iTunes test library (~1000s of books), create isolated test scenarios:
- **Subset A:** Books only in iTunes (new imports)
- **Subset B:** Books only in organizer (manually added)
- **Subset C:** Books in both, iTunes has newer metadata
- **Subset D:** Books in both, organizer has newer metadata
- **Subset E:** Books in both, both changed different fields (mergeable)

**Six Main Test Scenarios:**

1. **Import from iTunes (happy path)**
   - Validate iTunes library
   - Import Subset A books
   - Verify books appear in organizer with correct metadata
   - Verify sync state recorded

2. **Organizer edits then write-back**
   - Import books from Subset C
   - Edit book metadata in organizer (comments, tags, ratings)
   - Click "Force Sync to iTunes"
   - Verify iTunes XML updated with organizer changes
   - Verify timestamps updated

3. **iTunes conflict - newer iTunes data**
   - Detect Subset D has newer iTunes metadata than organizer
   - Trigger sync operation
   - Conflict dialog appears
   - User selects "Use iTunes version"
   - Verify organizer updated with iTunes data
   - Verify organizer DB reflects iTunes as source

4. **Organizer conflict - newer organizer data**
   - Detect Subset D has newer organizer metadata than iTunes
   - Trigger write-back sync
   - Conflict dialog appears
   - User selects "Use Organizer version"
   - Verify iTunes XML written with organizer data
   - Verify sync state updated

5. **Selective sync**
   - Import from iTunes but exclude certain books/fields
   - Verify excluded books not imported
   - Verify only selected fields synced
   - Verify partial imports don't corrupt data

6. **Retry failed sync**
   - Simulate sync failure (bad path, permission error)
   - Manually trigger "Retry Failed Sync" button
   - Verify retry uses same conflict choices as before
   - Verify successful completion

**Test Assertions:**
- Verify data consistency in organizer DB after each operation
- Verify iTunes XML matches expected state after write-back
- Verify sync timestamps accurate
- Verify metadata provenance recorded correctly
- Verify operation logs complete and accurate

**Files:**
- Create: `web/tests/e2e/itunes-bidirectional-sync.spec.ts` (new comprehensive test file)
- Modify: `web/tests/e2e/itunes-import.spec.ts` (existing tests still pass)

---

## Task 2: Force-Sync UI Buttons & Conflict Resolution

**Conflict Detection & Dialog Flow:**

When any sync operation (import or write-back) detects conflicts:
1. Sync operation pauses automatically
2. Conflict dialog appears with detailed information
3. User reviews each conflict and selects winning version
4. User applies choices and sync resumes
5. All choices logged for audit trail

**Conflict Dialog Component:**

Create new component: `web/src/components/settings/ITunesConflictDialog.tsx`

**UI Layout:**
- Title: "Sync Conflicts Detected (N conflicts)"
- Subtitle: "Review changes and choose which version to keep"
- Table with columns:
  - Book Title
  - Field Name (title, author, comments, etc.)
  - iTunes Version (with "Last Modified" timestamp)
  - Organizer Version (with "Last Modified" timestamp)
  - Your Choice (radio buttons: "iTunes" | "Organizer")
- Bulk Actions:
  - Button: "Use iTunes for all remaining"
  - Button: "Use Organizer for all remaining"
- Action Buttons:
  - "Apply & Sync" (primary)
  - "Cancel Sync" (secondary)

**Force-Sync Buttons in ITunesImport Component:**

Add three new buttons to iTunes settings:

1. **"Force Import from iTunes"**
   - Clears any pending organizer changes
   - Imports all from iTunes (no conflicts)
   - Shows confirmation dialog: "This will overwrite organizer changes"
   - Useful for: Rebuilding from authoritative iTunes

2. **"Force Sync to iTunes"**
   - Clears any pending iTunes changes
   - Writes all organizer data to iTunes (no conflicts)
   - Shows confirmation dialog: "This will overwrite iTunes metadata"
   - Useful for: Publishing organizer state to iTunes

3. **"Retry Failed Sync"**
   - Retries last failed sync operation
   - Uses previous conflict choices (if any)
   - Shows operation log
   - Visible only if last sync failed

**Error Handling:**
- All sync failures store state for retry
- Log all conflict resolutions with timestamps
- Preserve user's conflict choices for next sync
- Show operation history in settings

**Files:**
- Create: `web/src/components/settings/ITunesConflictDialog.tsx`
- Modify: `web/src/components/settings/ITunesImport.tsx` (add buttons + state management)
- Modify: `web/src/services/api.ts` (add conflict handling endpoints if needed)

---

## Task 3: iTunes Demo Integration

**Demo Extension: PHASE 7 (Screenshots 15-20)**

Add to existing `web/tests/e2e/demo-full-workflow.spec.ts` after PHASE 6 (persistence verification):

**PHASE 7: iTunes Integration & Bidirectional Sync**

1. **Navigate to Settings** (screenshot 15)
   - Use humanMove() to navigate to Settings page
   - Click iTunes Import tab
   - Show Settings UI with iTunes controls
   - Human cursor visible in screenshot

2. **Validate iTunes Library** (screenshot 16)
   - Use humanType() to enter iTunes library path (from testdata)
   - Click "Validate Library" button with humanMove()
   - Wait for validation to complete
   - Screenshot shows validation results (book count, status)

3. **Import from iTunes** (screenshot 17)
   - Click "Import Library" button with human cursor movement
   - Show progress bar animating (wait for import)
   - Wait 3-5 seconds for import to complete
   - Screenshot shows import success message

4. **Verify Books Imported** (screenshot 18)
   - Navigate back to Library view
   - Show newly imported books appear in library
   - Search for specific imported book (human search interaction)
   - Screenshot shows search results with imported book

5. **Edit Imported Book Metadata** (screenshot 19)
   - Click on imported book to open detail view
   - Edit comments field with humanType()
   - Show human cursor movement over form
   - Save changes
   - Screenshot shows edited metadata

6. **Write-Back to iTunes** (screenshot 20)
   - Navigate back to iTunes settings
   - Click "Force Sync to iTunes" button
   - Show success confirmation
   - Log verification message: "Changes written back to iTunes"
   - Screenshot shows sync completion

**Timing & Realism:**
- Each phase has 1-2 second delays between actions
- All clicks use humanMove() for visible cursor
- All text input uses humanType() for realistic typing
- Progress waits realistic (not instant)
- Total new runtime: ~90 seconds

**Demo Data:**
- Uses real iTunes test library (testdata/itunes/)
- Creates temporary organizer state
- Cleans up after demo completes

**Files:**
- Modify: `web/tests/e2e/demo-full-workflow.spec.ts` (add PHASE 7)
- Update: `web/tests/e2e/utils/demo-helpers.ts` (if needed for new UI interactions)

---

## Implementation Order

1. **Task 1 First:** Implement comprehensive tests
   - Tests validate the sync logic works correctly
   - Tests provide confidence for UI changes
   - Tests document expected behavior

2. **Task 2 Second:** Add UI buttons and conflict dialog
   - Buttons depend on sync logic (tested in Task 1)
   - UI changes won't break existing tests
   - Force-sync buttons ready to use in demo

3. **Task 3 Last:** Integrate into comprehensive demo
   - Demo depends on both tests passing and UI working
   - Demo shows complete workflow end-to-end
   - Demo validates all features work together

---

## Success Criteria

**Task 1 (Tests):**
- ✅ All 6 test scenarios pass
- ✅ Tests use real iTunes test library data
- ✅ Conflict resolution tested in both directions
- ✅ Selective sync tested
- ✅ Retry mechanism tested
- ✅ Test coverage includes error cases

**Task 2 (UI Buttons):**
- ✅ Conflict dialog appears during sync conflicts
- ✅ "Force Import from iTunes" button works
- ✅ "Force Sync to iTunes" button works
- ✅ "Retry Failed Sync" button works
- ✅ Confirmation dialogs prevent accidental overwrites
- ✅ Operation history logged and visible

**Task 3 (Demo):**
- ✅ All 6 demo phases execute successfully
- ✅ 6 new screenshots generated (15-20)
- ✅ Video shows complete iTunes workflow
- ✅ Metadata changes persist from organizer to iTunes
- ✅ Demo takes ~90 seconds (total demo ~2m 10s)
- ✅ Temp directories cleaned up
