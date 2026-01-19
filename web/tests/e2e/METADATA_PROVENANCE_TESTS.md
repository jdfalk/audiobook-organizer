<!-- file: web/tests/e2e/METADATA_PROVENANCE_TESTS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-01-19 -->

# Metadata Provenance E2E Test Documentation

## Overview

This document describes the E2E test coverage for metadata provenance features
implemented in SESSION-003. The tests validate per-field tracking of metadata
sources and override functionality.

## Test Infrastructure

**Test File**:
[tests/e2e/metadata-provenance.spec.ts](./metadata-provenance.spec.ts)

**Framework**: Playwright 1.56.1

**Test Configuration**: [tests/e2e/playwright.config.ts](./playwright.config.ts)

## Provenance Data Model

Each metadata field tracks:

- `file_value` - Value extracted from audio file tags
- `fetched_value` - Value retrieved from metadata API
- `stored_value` - Value persisted in database
- `override_value` - User-set override value
- `override_locked` - Boolean indicating if field is locked
- `effective_value` - Current displayed value (computed)
- `effective_source` - Which source provides effective_value
  (file|fetched|stored|override)

**Hierarchy**: `override > stored > fetched > file`

## Test Scenarios Implemented

### 1. Provenance Data Display (Tags Tab)

**Test**: `displays provenance data in Tags tab`

**AAA Pattern**:

- **Arrange**: Mock API with comprehensive provenance data
- **Act**: Navigate to Tags tab
- **Assert**: Verify effective values, source chips, and lock indicators are
  visible

**Coverage**:

- ✅ Effective values display correctly
- ✅ Source chips render (stored, override, fetched, file)
- ✅ Locked indicator shows for override-locked fields

---

### 2. Effective Source Verification

**Test**: `shows correct effective source for different fields`

**AAA Pattern**:

- **Arrange**: Setup fields with different active sources
- **Act**: Navigate to Tags and Compare tabs
- **Assert**: Verify each field shows correct source chip

**Coverage**:

- ✅ 'stored' source for standard fields (title, author)
- ✅ 'override' source for user-overridden fields (narrator)
- ✅ 'fetched' source for API-only fields (publisher)
- ✅ Lock indicator appears only for locked overrides

---

### 3. Apply Override from File Value

**Test**: `applies override from file value`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab
- **Act**: Click "Use File" button for title field
- **Assert**: Verify title updates to file value and source changes to
  'override'

**Coverage**:

- ✅ Apply file value as override
- ✅ Effective value updates in heading
- ✅ Source chip changes to 'override'
- ✅ Override persists in Tags tab

---

### 4. Apply Override from Fetched Value

**Test**: `applies override from fetched value`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab
- **Act**: Click "Use Fetched" button for author_name
- **Assert**: Verify author updates to fetched value

**Coverage**:

- ✅ Apply fetched value as override
- ✅ Effective value visible in Info tab
- ✅ Source tracking updates correctly

---

### 5. Clear Override and Revert

**Test**: `clears override and reverts to stored value`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab for field with existing override
- **Act**: Click "Clear" button for narrator field
- **Assert**: Verify narrator reverts to stored value, source changes to
  'stored', locked indicator removed

**Coverage**:

- ✅ Clear override functionality
- ✅ Revert to stored value (next in hierarchy)
- ✅ Source chip updates to 'stored'
- ✅ Lock indicator disappears

---

### 6. Override Persistence Across Reloads

**Test**: `lock toggle persists across page reloads`

**AAA Pattern**:

- **Arrange**: Apply override for series_name
- **Act**: Reload page, navigate to Tags tab
- **Assert**: Verify override still present with correct source

**Coverage**:

- ✅ Override persists after page reload
- ✅ Source indicator persists
- ✅ State management validates persistence

---

### 7. Compare Tab Source Columns

**Test**: `displays all source columns in Compare tab`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab
- **Act**: Check table headers and narrator row
- **Assert**: Verify all source columns visible with correct values

**Coverage**:

- ✅ Table headers: Field, File Tag, Fetched, Stored, Override, Actions
- ✅ All source values display in respective columns
- ✅ Narrator row shows all four source values

---

### 8. Field with Single Source

**Test**: `handles field with only fetched source`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab
- **Act**: Locate publisher row (only fetched_value exists)
- **Assert**: Verify fetched value displays, empty columns show placeholder "—"

**Coverage**:

- ✅ Field with only one source displays correctly
- ✅ Empty source columns show "—" placeholder
- ✅ Source chip reflects 'fetched'
- ✅ Handles edge case of partial provenance data

---

### 9. Disabled Action Buttons

**Test**: `disables action buttons when source value is null`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab
- **Act**: Check publisher row buttons (file_value is null)
- **Assert**: "Use File" disabled, "Use Fetched" enabled

**Coverage**:

- ✅ Button disabled when source value null
- ✅ Button enabled when source value exists
- ✅ UX prevents invalid operations

---

### 10. Media Info Display

**Test**: `shows media info in Tags tab`

**AAA Pattern**:

- **Arrange**: Navigate to Tags tab
- **Act**: Check for media info fields
- **Assert**: Verify codec, bitrate, sample rate visible

**Coverage**:

- ✅ Media info renders in Tags tab
- ✅ Codec, bitrate, sample rate display correctly
- ✅ Separate from provenance data but same endpoint

---

### 11. Effective Value Transition

**Test**: `updates effective value when applying different source`

**AAA Pattern**:

- **Arrange**: Start with stored value for title
- **Act**: Apply file value via Compare tab
- **Assert**: Effective value changes from stored to file value

**Coverage**:

- ✅ Effective value updates dynamically
- ✅ Source transition tracked correctly
- ✅ UI reflects backend state change

---

### 12. Source Chip Styling

**Test**: `shows correct effective source chip colors and styling`

**AAA Pattern**:

- **Arrange**: Navigate to Tags tab
- **Act**: Locate source and locked chips
- **Assert**: Verify chips have proper styling

**Coverage**:

- ✅ Source chips use outlined variant
- ✅ Locked chip uses warning color
- ✅ Visual distinction between chip types

---

### 13. Numeric Field Overrides

**Test**: `applies override with numeric value (audiobook_release_year)`

**AAA Pattern**:

- **Arrange**: Navigate to Compare tab
- **Act**: Apply file value for release year (2022)
- **Assert**: Numeric override applies correctly, source shows 'override'

**Coverage**:

- ✅ Numeric field overrides work
- ✅ Year field (2022) applies correctly
- ✅ Type handling validated (string vs number)

---

## Test Execution

### Run All E2E Tests

```bash
cd web
npm run test:e2e
```

### Run Provenance Tests Only

```bash
cd web
npx playwright test metadata-provenance.spec.ts
```

### Run with UI (Debug Mode)

```bash
cd web
npx playwright test metadata-provenance.spec.ts --ui
```

### Run Specific Test

```bash
cd web
npx playwright test -g "displays provenance data in Tags tab"
```

## Mock Data Structure

The tests use comprehensive mock data in `setupProvenanceRoutes()`:

```typescript
tags: {
  title: {
    file_value: 'File: Provenance Test',
    fetched_value: 'API: Provenance Test',
    stored_value: 'Provenance Test Book',
    override_value: null,
    override_locked: false,
    effective_value: 'Provenance Test Book',
    effective_source: 'stored',
  },
  narrator: {
    file_value: 'File Narrator',
    fetched_value: 'API Narrator',
    stored_value: 'DB Narrator',
    override_value: 'User Override Narrator',
    override_locked: true,
    effective_value: 'User Override Narrator',
    effective_source: 'override',
  },
  // ... additional fields
}
```

## Coverage Gaps and Recommendations

### Current Coverage

✅ **13 comprehensive test scenarios** ✅ **AAA pattern strictly followed** ✅
**Source hierarchy validation** ✅ **Override apply/clear/persist** ✅ **UI
element verification** ✅ **Edge cases (null values, numeric fields)**

### Not Yet Covered (Future Work)

1. **Fetch Metadata Refresh**
   - Test that "Fetch Metadata" updates fetched_value
   - Verify effective value recalculates after fetch

2. **AI Parse Integration**
   - Test AI parse updates with provenance tracking
   - Verify which source AI values populate

3. **Bulk Operations**
   - Test applying multiple overrides simultaneously
   - Test clearing all overrides at once

4. **Error Handling**
   - Test API failure during override application
   - Test network error during tags endpoint call

5. **Multi-User Scenarios**
   - Test concurrent override modifications
   - Test conflict resolution

6. **History/Audit Trail**
   - If implemented, test provenance change history
   - Test rollback to previous source values

7. **Keyboard Navigation**
   - Test tab navigation through Compare table
   - Test Enter key applying overrides

8. **Mobile/Responsive**
   - Test provenance display on mobile viewport
   - Test touch interactions for source selection

## Integration with Existing Tests

The new provenance tests complement existing
[book-detail.spec.ts](./book-detail.spec.ts):

**Existing**: Basic book detail rendering, soft delete, restore, metadata
refresh **New**: Deep provenance tracking, source hierarchy, override
persistence

**Overlap**: Compare tab usage (both test suites use it, but for different
purposes)

## Testing Best Practices Applied

1. ✅ **AAA Pattern**: All tests follow Arrange-Act-Assert
2. ✅ **Descriptive Names**: Test names describe expected behavior
3. ✅ **Independent**: Each test can run standalone
4. ✅ **Deterministic**: No flaky timing or race conditions
5. ✅ **Focused**: Each test validates one specific behavior
6. ✅ **Comprehensive Mocking**: Full API surface mocked
7. ✅ **Type Safety**: Full TypeScript types for mock data

## Known Limitations

1. **No Backend Integration**: Tests use client-side mocking, not real backend
2. **No Database**: Persistence tested via mock state, not actual PebbleDB
3. **No SSE**: EventSource mocked to prevent connection errors
4. **Browser Context**: Tests run in Chromium and WebKit only

## Maintenance Notes

**Mock Data Location**: `createTagsData()` function in test file

**Update Trigger**: If backend provenance schema changes, update TagEntry type
and createTagsData()

**Version Alignment**: Test data should match API contract in
`web/src/services/api.ts`

## CI/CD Integration

These tests are designed to run in GitHub Actions workflows:

```yaml
- name: Run E2E Tests
  run: |
    cd web
    npm install
    npx playwright install --with-deps
    npm run test:e2e
```

**Expected Runtime**: ~2-3 minutes for full provenance test suite

## Related Documentation

- [Backend Provenance Implementation](../../../docs/SESSION-003_metadata-provenance-summary.md)
  (if exists)
- [API Endpoint Documentation](../../../docs/api-endpoints.md)
- [Frontend Component Guide](../../src/pages/BookDetail.tsx)

---

**Last Updated**: 2024-12-28 **Test File Version**: 1.0.0 **Playwright
Version**: 1.56.1
