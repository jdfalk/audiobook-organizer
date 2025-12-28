<!-- file: docs/MANUAL_TEST_CHECKLIST_P0.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-f12345678901 -->

# P0 Manual Test Checklist - PR #79 Merge Gate

## Overview

This checklist contains **critical manual tests** that MUST pass before merging
PR #79 (Metadata Provenance feature). These tests complement the 13 automated
E2E tests and focus on user-facing scenarios that require human validation.

**Status**: ⏳ PENDING EXECUTION **Target**: PR #79 merge approval **Estimated
Time**: 2-3 hours **Tester**: [Assign QA engineer or developer]

---

## Pre-Test Setup

### Environment Preparation

- [ ] Application running locally on port 8888
- [ ] Fresh database or known clean state
- [ ] At least 3 test audiobook files prepared
- [ ] Browser: Chrome or Firefox (latest version)
- [ ] Browser console open for debugging
- [ ] Test data folder: `/path/to/test/audiobooks`

### Verification Commands

```bash
# Verify application running
curl http://localhost:8888/api/v1/system/status

# Verify database accessible
curl http://localhost:8888/api/v1/audiobooks | jq '.books | length'

# Expected: Status 200 OK
```

---

## Critical Test Scenarios

### ✅ 1. Metadata Provenance Display (PR #79)

**Priority**: P0 - CRITICAL **Related E2E**: `metadata-provenance.spec.ts`
(tests 1-3) **Time**: 15 minutes

#### Test Steps

- [ ] **1.1** Navigate to Library → Select any book → Open Book Detail
- [ ] **1.2** Click "Tags" tab
- [ ] **1.3** Verify each metadata field displays:
  - [ ] Effective value (prominent display)
  - [ ] Source chip (file/fetched/stored/override)
  - [ ] Correct chip color per source type
  - [ ] Lock icon for locked fields only
- [ ] **1.4** Click "Compare" tab
- [ ] **1.5** Verify all columns visible:
  - [ ] Field name
  - [ ] Effective value
  - [ ] File value
  - [ ] Fetched value
  - [ ] Stored value
  - [ ] Action buttons (Use File, Use Fetched, Clear)

**Pass Criteria**: All fields render correctly, sources accurate, no console
errors

**Screenshot**: Capture Tags tab with multiple source types visible

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 2. Apply Override from File Value (PR #79)

**Priority**: P0 - CRITICAL **Related E2E**: `metadata-provenance.spec.ts`
(test 4) **Time**: 10 minutes

#### Test Steps

- [ ] **2.1** Open Book Detail → Compare tab
- [ ] **2.2** Identify field with different file vs effective value (e.g.,
      title)
- [ ] **2.3** Note current effective value in page heading
- [ ] **2.4** Click "Use File" button for that field
- [ ] **2.5** Verify immediate UI updates:
  - [ ] Effective value changes in heading
  - [ ] Source chip updates to "override"
  - [ ] Success notification appears
- [ ] **2.6** Navigate to Tags tab
- [ ] **2.7** Verify override persists:
  - [ ] Field shows "override" chip
  - [ ] Value matches file value
- [ ] **2.8** Refresh browser (F5)
- [ ] **2.9** Verify override still present after reload

**API Validation**:

```bash
curl http://localhost:8888/api/v1/audiobooks/<book-id> | jq '.overrides'
# Should show override for field
```

**Pass Criteria**: Override applies instantly, persists after refresh, API
reflects change

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 3. Clear Override and Source Fallback (PR #79)

**Priority**: P0 - CRITICAL **Related E2E**: `metadata-provenance.spec.ts`
(test 6) **Time**: 10 minutes

#### Test Steps

- [ ] **3.1** Use book from Test #2 (with override applied)
- [ ] **3.2** Open Book Detail → Compare tab
- [ ] **3.3** Note what the next priority source value is (stored/fetched/file)
- [ ] **3.4** Click "Clear Override" button
- [ ] **3.5** Verify:
  - [ ] Effective value changes to next source
  - [ ] Source chip updates (no longer "override")
  - [ ] Success notification
- [ ] **3.6** Navigate to Tags tab
- [ ] **3.7** Verify override removed
- [ ] **3.8** Refresh page
- [ ] **3.9** Verify clear persists

**Expected Fallback Order**: stored > fetched > file

**Pass Criteria**: Override clears, correct source takes precedence, change
persists

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 4. Lock Toggle Persistence (PR #79)

**Priority**: P0 - CRITICAL **Related E2E**: `metadata-provenance.spec.ts`
(test 13) **Time**: 5 minutes

#### Test Steps

- [ ] **4.1** Open Book Detail → Tags tab
- [ ] **4.2** Locate field with override
- [ ] **4.3** Click lock icon to lock
- [ ] **4.4** Verify lock icon changes to "locked" state
- [ ] **4.5** Navigate away (e.g., to Library)
- [ ] **4.6** Return to Book Detail → Tags tab
- [ ] **4.7** Verify lock status persisted
- [ ] **4.8** Click lock icon to unlock
- [ ] **4.9** Verify unlock persists after navigation

**Pass Criteria**: Lock toggles correctly, state persists across navigation

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 5. Blocked Hashes - Add with Validation (PR #69)

**Priority**: P0 - CRITICAL **Time**: 10 minutes

#### Test Steps

- [ ] **5.1** Navigate to Settings → Blocked Hashes tab
- [ ] **5.2** Click "Add Blocked Hash"
- [ ] **5.3** **Test validation** - Enter invalid hash: `abc123`
- [ ] **5.4** Click Save
- [ ] **5.5** Verify error: "Hash must be 64 hexadecimal characters (SHA256)"
- [ ] **5.6** **Test validation** - Enter valid hash but leave reason blank
- [ ] **5.7** Click Save
- [ ] **5.8** Verify error: "Hash and reason are required"
- [ ] **5.9** **Valid entry** - Enter:
  - Hash: `a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd`
  - Reason: `P0 Test - Manual QA`
- [ ] **5.10** Click Save
- [ ] **5.11** Verify:
  - [ ] Dialog closes
  - [ ] Success message appears
  - [ ] New entry in table
  - [ ] Hash truncated in table, full hash on hover
- [ ] **5.12** Refresh page
- [ ] **5.13** Verify hash persists

**Pass Criteria**: Validation catches invalid inputs, valid hash saves and
persists

**Screenshot**: Capture blocked hashes table with new entry

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 6. Blocked Hashes - Delete with Confirmation (PR #69)

**Priority**: P0 - CRITICAL **Time**: 5 minutes

#### Test Steps

- [ ] **6.1** Navigate to Settings → Blocked Hashes tab
- [ ] **6.2** Locate hash added in Test #5
- [ ] **6.3** Click delete/unblock button (trash icon)
- [ ] **6.4** Verify confirmation dialog:
  - [ ] Shows full hash (not truncated)
  - [ ] Shows reason
  - [ ] Cancel and Confirm buttons
- [ ] **6.5** Click Cancel
- [ ] **6.6** Verify hash remains in table
- [ ] **6.7** Click delete button again
- [ ] **6.8** Click Confirm
- [ ] **6.9** Verify:
  - [ ] Success message
  - [ ] Hash removed from table
- [ ] **6.10** Refresh page
- [ ] **6.11** Verify deletion persists

**Pass Criteria**: Confirmation prevents accidental deletion, delete succeeds
and persists

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 7. Soft Delete Without Hash Blocking (PR #70)

**Priority**: P0 - CRITICAL **Time**: 10 minutes

#### Test Steps

- [ ] **7.1** Navigate to Library
- [ ] **7.2** Select a test book
- [ ] **7.3** Click delete button
- [ ] **7.4** Verify delete dialog:
  - [ ] Soft delete option
  - [ ] "Prevent reimporting" checkbox (unchecked)
  - [ ] Cancel and Confirm buttons
- [ ] **7.5** Leave "Prevent reimporting" UNCHECKED
- [ ] **7.6** Click Confirm
- [ ] **7.7** Verify:
  - [ ] Book removed from Library list
  - [ ] Success message
  - [ ] Soft-delete count updates (if displayed)
- [ ] **7.8** Check book state via API:
  ```bash
  curl http://localhost:8888/api/v1/audiobooks/<book-id> | \
    jq '{library_state, marked_for_deletion, marked_for_deletion_at}'
  ```
- [ ] **7.9** Verify:
  - [ ] `library_state`: "deleted"
  - [ ] `marked_for_deletion`: true
  - [ ] `marked_for_deletion_at`: recent timestamp
- [ ] **7.10** Check file still exists on filesystem

**Pass Criteria**: Book soft-deleted, state correct, file NOT deleted, hash NOT
blocked

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 8. Soft Delete With Hash Blocking (PR #70)

**Priority**: P0 - CRITICAL **Time**: 15 minutes

#### Test Steps

- [ ] **8.1** Navigate to Library
- [ ] **8.2** Select another test book
- [ ] **8.3** Note the book's hash (visible in detail page or API)
- [ ] **8.4** Click delete button
- [ ] **8.5** CHECK "Prevent reimporting" checkbox
- [ ] **8.6** Enter reason: `P0 Test - Soft delete with hash block`
- [ ] **8.7** Click Confirm
- [ ] **8.8** Verify:
  - [ ] Book removed from Library
  - [ ] Success message mentions hash blocking
- [ ] **8.9** Navigate to Settings → Blocked Hashes
- [ ] **8.10** Verify new hash entry:
  - [ ] Hash matches book's hash
  - [ ] Reason matches entered text
  - [ ] Blocked date is today
- [ ] **8.11** Copy the test audiobook file to import directory
- [ ] **8.12** Trigger scan operation
- [ ] **8.13** Check logs:
  ```bash
  tail -f logs/audiobook-organizer.log | grep "Skipping file"
  ```
- [ ] **8.14** Verify file was skipped due to blocked hash
- [ ] **8.15** Verify book NOT reimported in Library

**Pass Criteria**: Delete blocks hash, reimport prevented, hash persists in
blocklist

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 9. Restore Soft-Deleted Book (PR #70)

**Priority**: P0 - CRITICAL **Time**: 10 minutes

#### Test Steps

- [ ] **9.1** Navigate to Library
- [ ] **9.2** Click "Show Deleted" or navigate to soft-deleted books section
- [ ] **9.3** Locate soft-deleted book from Test #7
- [ ] **9.4** Click "Restore" button
- [ ] **9.5** Verify:
  - [ ] Confirmation dialog (if applicable)
  - [ ] Success message
  - [ ] Book removed from deleted list
  - [ ] Book appears in main Library
- [ ] **9.6** Open Book Detail page for restored book
- [ ] **9.7** Verify book fully functional
- [ ] **9.8** Check state via API:
  ```bash
  curl http://localhost:8888/api/v1/audiobooks/<book-id> | \
    jq '{library_state, marked_for_deletion}'
  ```
- [ ] **9.9** Verify:
  - [ ] `library_state`: "organized" or "imported"
  - [ ] `marked_for_deletion`: false

**Pass Criteria**: Restore succeeds, state correct, book fully usable

**Observed Issues**:

```
[Record any issues here]
```

---

### ✅ 10. State Transition: Import → Organized (PR #70)

**Priority**: P0 - CRITICAL **Time**: 15 minutes

#### Test Steps

- [ ] **10.1** Place NEW audiobook file in import directory
- [ ] **10.2** Trigger scan operation
- [ ] **10.3** Wait for scan completion
- [ ] **10.4** Find new book in Library
- [ ] **10.5** Check state via API:
  ```bash
  curl http://localhost:8888/api/v1/audiobooks/<book-id> | jq '.library_state'
  ```
- [ ] **10.6** Verify state is `imported`
- [ ] **10.7** Trigger organize operation for the book
- [ ] **10.8** Wait for organize completion
- [ ] **10.9** Check state again via API
- [ ] **10.10** Verify state changed to `organized`
- [ ] **10.11** Verify file moved to organized directory
- [ ] **10.12** Open Book Detail page
- [ ] **10.13** Verify book fully accessible with all metadata

**Pass Criteria**: State transitions correctly, file organized, book remains
accessible

**Observed Issues**:

```
[Record any issues here]
```

---

## Post-Test Validation

### Automated Test Correlation

After manual tests, verify automated E2E tests align:

- [ ] Run E2E tests: `cd web && npm run test:e2e`
- [ ] Verify all 13 provenance tests pass
- [ ] Compare results with manual findings
- [ ] Document any discrepancies

### Log Review

- [ ] Check application logs for errors:
  ```bash
  grep -i error logs/audiobook-organizer.log
  ```
- [ ] Verify no unexpected warnings or panics
- [ ] Check API response times reasonable (<2s average)

### Database Integrity

- [ ] Verify no orphaned records:
  ```bash
  curl http://localhost:8888/api/v1/audiobooks | jq '.books | length'
  curl http://localhost:8888/api/v1/audiobooks/soft-deleted | jq '.items | length'
  curl http://localhost:8888/api/v1/blocked-hashes | jq '.items | length'
  ```
- [ ] Totals match expected counts

---

## Test Summary

**Completion Date**: ******\_\_\_****** **Tester**: ******\_\_\_******
**Build/Commit**: ******\_\_\_******

### Results

| Test # | Test Name                | Status | Issues Found |
| ------ | ------------------------ | ------ | ------------ |
| 1      | Provenance Display       | ☐      |              |
| 2      | Apply Override (File)    | ☐      |              |
| 3      | Clear Override           | ☐      |              |
| 4      | Lock Toggle              | ☐      |              |
| 5      | Add Blocked Hash         | ☐      |              |
| 6      | Delete Blocked Hash      | ☐      |              |
| 7      | Soft Delete (No Block)   | ☐      |              |
| 8      | Soft Delete (With Block) | ☐      |              |
| 9      | Restore Soft-Deleted     | ☐      |              |
| 10     | State Transition         | ☐      |              |

**Pass Rate**: **_/10 (_**%)

### Critical Issues Found

```
[List any P0 blocking issues that prevent PR #79 merge]

1.
2.
3.
```

### Recommendations

- [ ] **APPROVE MERGE**: All critical tests pass, no blocking issues
- [ ] **CONDITIONAL APPROVAL**: Minor issues found, can be addressed post-merge
- [ ] **BLOCK MERGE**: Critical issues found, must be fixed before merge

**Justification**:

```
[Explain recommendation]
```

---

## Sign-Off

**Tester Signature**: ******\_\_\_****** **Date**: ******\_\_\_******

**Reviewer Signature**: ******\_\_\_****** **Date**: ******\_\_\_******

---

## Appendix: Quick Reference Commands

### API Health Check

```bash
curl http://localhost:8888/api/v1/system/status
```

### Get Book State

```bash
curl http://localhost:8888/api/v1/audiobooks/<book-id> | \
  jq '{id, title, library_state, marked_for_deletion}'
```

### List Blocked Hashes

```bash
curl http://localhost:8888/api/v1/blocked-hashes | jq '.items[]'
```

### List Soft-Deleted Books

```bash
curl http://localhost:8888/api/v1/audiobooks/soft-deleted | jq '.items[] | {id, title}'
```

### Trigger Scan

```bash
curl -X POST http://localhost:8888/api/v1/operations/scan
```

### View Logs

```bash
tail -f logs/audiobook-organizer.log
```

---

## Version History

- **1.0.0** (2025-12-28): Initial P0 checklist created for PR #79 merge gate

---

**Related Documents**:

- [Full Manual Test Plan](./MANUAL_TEST_PLAN.md)
- [Test Data Setup Guide](./TEST_DATA_SETUP_GUIDE.md)
- [E2E Test Coverage Summary](../web/tests/e2e/TEST_COVERAGE_SUMMARY.md)
