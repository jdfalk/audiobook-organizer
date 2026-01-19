<!-- file: web/tests/e2e/TEST_COVERAGE_SUMMARY.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7d6c5b4a-3e2f-1d0c-9b8a-7f6e5d4c3b2a -->
<!-- last-edited: 2026-01-19 -->

# E2E Test Coverage Summary

## Quick Reference

| Test File                                                    | Test Count | Focus Area              | Status      |
| ------------------------------------------------------------ | ---------- | ----------------------- | ----------- |
| [app.spec.ts](./app.spec.ts)                                 | 4          | App smoke tests         | ✅ Existing |
| [import-paths.spec.ts](./import-paths.spec.ts)               | ~8         | Import paths UI         | ✅ Existing |
| [book-detail.spec.ts](./book-detail.spec.ts)                 | 6          | Book detail CRUD        | ✅ Existing |
| [metadata-provenance.spec.ts](./metadata-provenance.spec.ts) | **13**     | **Provenance tracking** | ✅ **NEW**  |

**Total E2E Tests**: ~31 **New Tests Added**: 13 **Coverage Increase**: +42% for
book detail features

---

## Metadata Provenance Tests (NEW)

**File**: [metadata-provenance.spec.ts](./metadata-provenance.spec.ts)
**Added**: 2024-12-28 **Version**: 1.0.0

### Test Categories

**Source Display & Accuracy** (3 tests)

- displays provenance data in Tags tab
- shows correct effective source for different fields
- shows media info in Tags tab

**Override Operations** (3 tests)

- applies override from file value
- applies override from fetched value
- clears override and reverts to stored value

**UI Interaction** (4 tests)

- displays all source columns in Compare tab
- disables action buttons when source value is null
- shows correct effective source chip colors and styling
- updates effective value when applying different source

**Edge Cases** (3 tests)

- handles field with only fetched source
- applies override with numeric value (audiobook_release_year)
- lock toggle persists across page reloads

---

## Running Tests

### All E2E Tests

```bash
cd web
npm run test:e2e
```

### Provenance Tests Only

```bash
cd web
npx playwright test metadata-provenance.spec.ts
```

### Debug Mode

```bash
cd web
npx playwright test metadata-provenance.spec.ts --ui
```

### Single Test

```bash
cd web
npx playwright test -g "displays provenance data"
```

---

## Coverage Gaps (Future Work)

**High Priority**:

- [ ] Error handling when override application fails
- [ ] Fetch Metadata integration with provenance update

**Medium Priority**:

- [ ] AI Parse provenance tracking
- [ ] Keyboard navigation accessibility
- [ ] Visual regression tests

**Low Priority**:

- [ ] Bulk override operations
- [ ] Concurrent modification conflicts
- [ ] Mobile responsive display

---

## Documentation

**Comprehensive Guide**:
[METADATA_PROVENANCE_TESTS.md](./METADATA_PROVENANCE_TESTS.md) **Full Report**:
[docs/TEST_ORCHESTRATOR_REPORT_PROVENANCE.md](../../docs/TEST_ORCHESTRATOR_REPORT_PROVENANCE.md)
**Playwright Config**: [playwright.config.ts](./playwright.config.ts)

---

**Last Updated**: 2024-12-28 **Maintained By**: Test Orchestrator Agent
