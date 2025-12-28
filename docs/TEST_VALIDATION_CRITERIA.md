<!-- file: docs/TEST_VALIDATION_CRITERIA.md -->
<!-- version: 1.0.0 -->
<!-- guid: d4e5f6a7-b8c9-0123-def1-234567890123 -->

# Test Validation Criteria & Issue Reporting

## Overview

This document defines pass/fail criteria for manual testing scenarios and provides templates for reporting issues discovered during QA.

**Purpose**: Ensure consistent test evaluation and clear issue communication

**Target Audience**: QA Engineers, Developers, Project Managers

---

## Pass/Fail Criteria by Feature

### 1. Metadata Provenance (PR #79)

#### 1.1 Provenance Data Display

**PASS Criteria** ✅:
- All metadata fields display effective value prominently
- Source chip visible for each field (file/fetched/stored/override)
- Source chip color accurate:
  - `file`: Blue/primary
  - `fetched`: Green/success
  - `stored`: Grey/default
  - `override`: Orange/warning
- Lock icon appears only for locked fields
- No console errors in browser developer tools
- Tags and Compare tabs load within 2 seconds
- Responsive layout on desktop and mobile

**FAIL Criteria** ❌:
- Missing effective values for any field
- Source chip incorrect or missing
- Wrong chip color for source type
- Lock icon shown for non-locked fields
- Console errors present
- Page load time >5 seconds
- Layout broken on any viewport size

**Severity Thresholds**:
- **Critical**: Missing values, incorrect source priority, app crash
- **High**: Wrong chip colors, missing chips, console errors
- **Medium**: Slow load times (3-5s), minor layout issues
- **Low**: Tooltip text issues, inconsistent spacing

---

#### 1.2 Apply Override from File Value

**PASS Criteria** ✅:
- Override applies instantly (<1 second)
- Effective value updates in page heading
- Source chip changes to "override" with correct color
- Success notification displays
- Override persists in Tags tab
- Override persists after page refresh
- API request succeeds: `PATCH /api/v1/audiobooks/:id` (status 200)
- Database reflects change (verifiable via API)

**FAIL Criteria** ❌:
- Override doesn't apply
- Effective value doesn't update
- Source chip doesn't change
- No user feedback (notification)
- Override lost after navigation
- Override lost after page refresh
- API request fails (4xx/5xx)
- Database not updated

**Severity Thresholds**:
- **Critical**: Override doesn't apply, data loss, API failure
- **High**: No user feedback, doesn't persist refresh
- **Medium**: Slow apply (1-3s), inconsistent UI updates
- **Low**: Notification text issues, animation glitches

**Edge Cases** (Must Pass):
- Apply override to field with null file value → Button disabled ✅
- Apply override to already-overridden field → Updates correctly ✅
- Apply multiple overrides in sequence → All persist ✅

---

#### 1.3 Clear Override

**PASS Criteria** ✅:
- Override clears successfully
- Effective value changes to next priority source:
  - Override removed → stored (if exists)
  - No stored → fetched (if exists)
  - No fetched → file (if exists)
- Source chip updates to correct new source
- Success notification displays
- Clear persists after page refresh
- API request succeeds: `PATCH /api/v1/audiobooks/:id` with override removed
- Database updated (override null/deleted)

**FAIL Criteria** ❌:
- Override doesn't clear
- Wrong source becomes effective (incorrect priority)
- Source chip doesn't update
- No user feedback
- Clear doesn't persist
- API request fails
- Database still shows override

**Severity Thresholds**:
- **Critical**: Can't clear override, wrong fallback source
- **High**: Doesn't persist, API failure
- **Medium**: Wrong notification text, slow clear
- **Low**: Minor UI inconsistencies

**Source Priority Validation**:
```
override > stored > fetched > file
```
Must strictly follow this order.

---

#### 1.4 Lock Toggle

**PASS Criteria** ✅:
- Lock toggles on/off on click
- Lock icon changes visibly (locked ↔ unlocked)
- Lock state persists across navigation
- Lock state persists after page refresh
- API request succeeds on toggle
- Locked field prevents automatic metadata updates (future test)
- Lock works in both Tags and Compare tabs

**FAIL Criteria** ❌:
- Lock doesn't toggle
- Icon doesn't change
- Lock state lost after navigation
- Lock state lost after refresh
- API request fails
- Lock has no effect on field behavior

**Severity Thresholds**:
- **Critical**: Lock doesn't work, state not persisted
- **High**: Lock lost after refresh, API failure
- **Medium**: Icon doesn't update, slow toggle
- **Low**: Tooltip/visual polish issues

---

### 2. Blocked Hashes Management (PR #69)

#### 2.1 Add Blocked Hash

**PASS Criteria** ✅:
- **Validation**: Rejects invalid hash formats:
  - Too short: `abc123` → Error: "Hash must be 64 hexadecimal characters"
  - Non-hex: `xxxx...` → Error: "Hash must be 64 hexadecimal characters"
  - Empty reason: Valid hash, blank reason → Error: "Hash and reason are required"
- **Valid Entry**: Accepts 64-char hex hash with reason
- Dialog closes after successful save
- Success notification: "Hash blocked successfully"
- New entry appears in Blocked Hashes table
- Hash truncated in table (first 12 chars), full hash on hover/click
- API request succeeds: `POST /api/v1/blocked-hashes` (status 201)
- Entry persists after page refresh

**FAIL Criteria** ❌:
- Validation doesn't catch invalid inputs
- Valid hash rejected
- No success feedback
- Hash doesn't appear in table
- API request fails
- Entry lost after refresh

**Severity Thresholds**:
- **Critical**: Can't add valid hash, API failure, no validation
- **High**: Entry doesn't persist, wrong validation messages
- **Medium**: Poor UX feedback, slow save
- **Low**: Tooltip/formatting issues

**Edge Cases** (Must Pass):
- Duplicate hash → Update existing or show error ✅
- Uppercase hex → Accepted and normalized ✅
- Very long reason (500+ chars) → Accepted or truncated ✅

---

#### 2.2 Delete Blocked Hash

**PASS Criteria** ✅:
- Delete button visible for each hash
- Click opens confirmation dialog
- Confirmation shows:
  - Full hash (not truncated)
  - Reason
  - Clear warning message
  - Cancel and Confirm buttons
- Cancel → Dialog closes, hash remains
- Confirm → Hash deleted
- Success notification: "Hash unblocked successfully"
- Hash removed from table immediately
- API request succeeds: `DELETE /api/v1/blocked-hashes/:hash` (status 200)
- Deletion persists after refresh

**FAIL Criteria** ❌:
- No delete button
- No confirmation dialog (dangerous!)
- Confirmation missing critical info
- Cancel doesn't work
- Hash not deleted after confirm
- No feedback
- API request fails
- Deletion doesn't persist

**Severity Thresholds**:
- **Critical**: No confirmation dialog, can't delete, API failure
- **High**: Wrong hash deleted, doesn't persist
- **Medium**: Poor confirmation UX, slow delete
- **Low**: Button styling, notification text

---

#### 2.3 Blocked Hash Prevents Reimport

**PASS Criteria** ✅:
- Scanner detects file hash before import
- Hash checked against blocklist
- Match found → File skipped
- Log message: `Skipping file: hash blocked: <hash> (reason: <reason>)`
- File NOT imported to library
- Book count unchanged
- Scanner completes without errors

**FAIL Criteria** ❌:
- File imported despite blocked hash
- No log message
- Scanner crashes/errors
- Book appears in library
- Incorrect hash matching

**Severity Thresholds**:
- **Critical**: File imported despite block (core feature broken)
- **High**: No logging, hash matching broken
- **Medium**: Misleading log messages, confusing UX
- **Low**: Log formatting issues

---

### 3. State Transitions & Delete Flows (PR #70)

#### 3.1 Import → Organized Transition

**PASS Criteria** ✅:
- New file scan sets initial state: `imported`
- `quantity` field set to 1
- After organize operation:
  - State changes to: `organized`
  - File physically moved to organized directory
  - Book remains in library
  - All metadata intact
- State persists in database
- State queryable via API
- Transitions logged

**FAIL Criteria** ❌:
- Wrong initial state
- State doesn't change after organize
- File not moved
- Book disappears from library
- Metadata lost
- State not persisted

**Severity Thresholds**:
- **Critical**: State doesn't transition, file not moved, data loss
- **High**: State not persisted, metadata lost
- **Medium**: Logging issues, slow transition
- **Low**: Minor state field inconsistencies

**API Validation**:
```bash
# After import
curl .../audiobooks/<id> | jq '.library_state' # "imported"

# After organize
curl .../audiobooks/<id> | jq '.library_state' # "organized"
```

---

#### 3.2 Soft Delete

**PASS Criteria** ✅:
- Delete dialog offers soft delete option
- Optional "Prevent reimporting" checkbox
- Confirmation required
- After soft delete:
  - `library_state`: `deleted`
  - `marked_for_deletion`: `true`
  - `marked_for_deletion_at`: recent timestamp
  - Book removed from main library list
  - File still exists on filesystem (NOT deleted)
  - Book queryable via soft-deleted endpoint
- Success notification
- API request: `DELETE /api/v1/audiobooks/:id` (with soft delete flag)

**FAIL Criteria** ❌:
- Hard delete occurs (file deleted)
- State not updated
- Book completely removed (not queryable)
- No confirmation
- API failure

**Severity Thresholds**:
- **Critical**: File deleted when should be soft (data loss), can't restore
- **High**: State wrong, book not queryable
- **Medium**: No confirmation, poor UX
- **Low**: Notification text issues

---

#### 3.3 Soft Delete with Hash Blocking

**PASS Criteria** ✅:
- All soft delete criteria (above) PLUS:
- Hash added to blocklist:
  - `POST /api/v1/blocked-hashes` succeeds
  - Hash appears in Settings → Blocked Hashes
  - Reason matches user input
  - Blocked date is today
- Reimport attempt:
  - Same file placed in import dir
  - Scan operation triggered
  - File skipped (hash blocked)
  - Log message confirms skip
  - File NOT imported

**FAIL Criteria** ❌:
- Hash not blocked
- Reimport succeeds (hash check failed)
- Wrong hash blocked
- Reason not saved

**Severity Thresholds**:
- **Critical**: Reimport not prevented (core feature broken)
- **High**: Wrong hash blocked, hash check logic broken
- **Medium**: Reason not saved, logging issues
- **Low**: UI feedback issues

---

#### 3.4 Restore Soft-Deleted Book

**PASS Criteria** ✅:
- Soft-deleted books list accessible
- Restore button visible per book
- Click restore (optional confirmation)
- After restore:
  - `library_state`: `organized` or `imported`
  - `marked_for_deletion`: `false`
  - `marked_for_deletion_at`: null or cleared
  - Book appears in main library
  - Book fully functional
  - All metadata intact
- Success notification
- API request: `PATCH /api/v1/audiobooks/:id/restore` or equivalent

**FAIL Criteria** ❌:
- Can't access soft-deleted list
- No restore button
- Restore fails
- State not updated
- Book doesn't return to library
- Metadata lost
- Book not functional

**Severity Thresholds**:
- **Critical**: Can't restore, data loss, API failure
- **High**: Metadata lost, wrong state after restore
- **Medium**: No confirmation, slow restore
- **Low**: UI/UX polish issues

**Note**: Blocked hash (if created during delete) remains blocked after restore (expected behavior).

---

#### 3.5 Purge Soft-Deleted Books

**PASS Criteria** ✅:
- Purge button visible with soft-deleted count
- Click opens confirmation dialog:
  - Shows count to be purged
  - Warning: "This action is permanent"
  - Checkbox: "Delete files from disk"
  - Cancel and Confirm buttons
- **Purge WITHOUT file deletion**:
  - Books removed from database permanently
  - Files remain on filesystem
  - API: `DELETE /api/v1/audiobooks/purge-soft-deleted?delete_files=false`
- **Purge WITH file deletion**:
  - Books removed from database
  - Files deleted from filesystem
  - API: `DELETE /api/v1/audiobooks/purge-soft-deleted?delete_files=true`
- Success notification with count
- Soft-deleted list empty after purge
- Purged books not queryable (404)

**FAIL Criteria** ❌:
- No confirmation (dangerous!)
- Files deleted when shouldn't be
- Files not deleted when should be
- Books still queryable after purge
- API failure
- Partial purge (some remain)

**Severity Thresholds**:
- **Critical**: Wrong files deleted, no confirmation, data loss
- **High**: Purge incomplete, API failure
- **Medium**: Confusing UX, no count display
- **Low**: Notification text, styling

**Safety Validation**: Purged books cannot be restored ⚠️

---

## Accessibility Validation Criteria

### Keyboard Navigation

**PASS Criteria** ✅:
- Tab key navigates through all interactive elements
- Focus indicators visible (outline or highlight)
- Enter/Space activates buttons and links
- Escape closes dialogs
- No keyboard traps
- Tab order logical (top→bottom, left→right)
- Skip links available for main content

**FAIL Criteria** ❌:
- Elements not keyboard-accessible
- No visible focus indicators
- Keyboard shortcuts don't work
- Keyboard traps exist
- Illogical tab order

**Severity**: Critical (WCAG 2.1 Level A requirement)

### Screen Reader Support (Optional - Advanced)

**PASS Criteria** ✅:
- All form inputs have labels
- Buttons have descriptive text or aria-label
- Images have alt text
- Table data announced properly
- Status messages announced (aria-live)
- Semantic HTML used (headings, lists, etc.)

---

## Performance Validation Criteria

### Page Load Times

| Metric                  | Target | Acceptable | Unacceptable |
|-------------------------|--------|------------|--------------|
| Library page load       | <2s    | 2-3s       | >3s          |
| Book detail page load   | <1s    | 1-2s       | >2s          |
| Settings tab load       | <1s    | 1-2s       | >2s          |
| API response time       | <500ms | 500ms-1s   | >1s          |
| Override apply          | <500ms | 500ms-1s   | >1s          |

**Test Conditions**: 100+ books in library, desktop browser, local network

### Responsiveness

**PASS Criteria** ✅:
- Scrolling smooth (60fps)
- No UI freezing
- Actions feel instant (<100ms perceived latency)
- Progress indicators for long operations (>1s)

---

## Issue Reporting Templates

### Template 1: Bug Report

```markdown
# Bug Report: [Short Title]

## Test Scenario
**Scenario ID**: [e.g., 1.2 Apply Override from File Value]
**Priority**: [P0 / P1 / P2]
**Severity**: [Critical / High / Medium / Low]

## Description
[Clear, concise description of the bug]

## Steps to Reproduce
1. [First step]
2. [Second step]
3. [Third step]
4. [Observe result]

## Expected Behavior
[What should happen according to pass criteria]

## Actual Behavior
[What actually happened]

## Impact
[How this affects users or system]

## Environment
- **OS**: [e.g., macOS 14.2]
- **Browser**: [e.g., Chrome 120.0.6099.109]
- **Application Version**: [commit hash or tag]
- **Test Data**: [e.g., test-book-001.m4b]

## Evidence
**Screenshots**: [Attach images]
**Logs**: 
```
[Paste relevant log excerpts]
```
**API Response**:
```json
[Paste API response if relevant]
```

## Reproducibility
- [ ] 100% (Always occurs)
- [ ] >75% (Frequent)
- [ ] 50-75% (Occasional)
- [ ] <50% (Rare)

## Related Issues
[Link to related bugs or features]

## Suggested Fix (Optional)
[If you have insights into root cause or fix]
```

### Template 2: Test Failure Report

```markdown
# Test Failure: [Scenario Name]

**Test ID**: [e.g., P0-Test-002]
**Date**: [YYYY-MM-DD]
**Tester**: [Name]

## Failure Summary
| Aspect            | Expected        | Actual          | Status |
|-------------------|-----------------|-----------------|--------|
| [Aspect 1]        | [Expected]      | [Actual]        | ❌ FAIL |
| [Aspect 2]        | [Expected]      | [Actual]        | ✅ PASS |

## Details
[Explain what went wrong]

## Blocker Status
- [ ] **BLOCKS PR MERGE**: Critical issue, must fix before merge
- [ ] **HIGH PRIORITY**: Should fix before merge, can be addressed quickly
- [ ] **MEDIUM PRIORITY**: Should fix soon, but doesn't block merge
- [ ] **LOW PRIORITY**: Nice to fix, can be tracked as tech debt

## Recommended Action
[What should be done next]
```

### Template 3: Enhancement/Improvement

```markdown
# Test Observation: [Enhancement Title]

**Category**: [UX / Performance / Accessibility / Documentation]
**Priority**: [P1 / P2 / P3]

## Observation
[What you noticed during testing]

## Current Behavior
[How it works now]

## Suggested Improvement
[How it could be better]

## User Benefit
[Why this would improve the experience]

## Effort Estimate
- [ ] Quick win (<1 hour)
- [ ] Small (1-4 hours)
- [ ] Medium (1-2 days)
- [ ] Large (>2 days)
```

---

## Issue Severity Guidelines

### Critical (P0) - Blocks Release
- Data loss or corruption
- Security vulnerability
- Core feature completely broken
- Application crashes/unusable
- Cannot complete primary user flow

**Action**: Immediate fix required, blocks PR merge

### High (P1) - Significant Impact
- Major feature broken but workaround exists
- Significant UX degradation
- Performance severely impacted
- Affects many users
- Incorrect data displayed

**Action**: Fix before release, may not block PR merge if tracked

### Medium (P2) - Moderate Impact
- Minor feature issues
- Cosmetic UX problems
- Edge case failures
- Affects some users
- Workaround readily available

**Action**: Fix in next sprint, does not block release

### Low (P3) - Minor Impact
- Polish/refinement issues
- Documentation gaps
- Rare edge cases
- Minimal user impact

**Action**: Track as tech debt, fix when convenient

---

## Test Sign-Off Checklist

Before approving PR #79 merge, verify:

- [ ] All P0 tests passed (10/10 from checklist)
- [ ] No critical or high-severity issues remain open
- [ ] All automated E2E tests pass (13/13)
- [ ] Performance within acceptable thresholds
- [ ] Accessibility requirements met (keyboard navigation)
- [ ] Documentation updated (CHANGELOG, README)
- [ ] Test evidence collected (screenshots, logs)
- [ ] Known issues documented with severity assigned

**Approval**: ☐ APPROVED | ☐ APPROVED WITH CONDITIONS | ☐ REJECTED

**Approver**: _______________ **Date**: _______________

---

## Appendix: Quick Severity Decision Tree

```
Issue occurs → Does it cause data loss?
  ├─ YES → CRITICAL
  └─ NO → Does it block primary user flow?
      ├─ YES → CRITICAL
      └─ NO → Is there a workaround?
          ├─ NO → HIGH
          └─ YES → Does it affect many users?
              ├─ YES → HIGH
              └─ NO → Does it affect core features?
                  ├─ YES → MEDIUM
                  └─ NO → LOW
```

---

## Version History

- **1.0.0** (2025-12-28): Initial validation criteria and issue templates created
  - Pass/fail criteria for all P0 scenarios
  - Severity thresholds defined
  - Issue reporting templates
  - Test sign-off checklist

---

**Related Documents**:
- [Manual Test Plan](./MANUAL_TEST_PLAN.md)
- [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md)
- [Test Data Setup Guide](./TEST_DATA_SETUP_GUIDE.md)
