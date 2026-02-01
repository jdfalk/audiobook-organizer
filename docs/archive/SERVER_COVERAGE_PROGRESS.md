# Server Coverage Improvement Progress

**Date**: 2026-01-23 **Target**: 80%+ coverage for server package **Starting
Coverage**: 66.0% **Current Coverage**: 67.4% **Progress**: +1.4 percentage
points (10.6% toward goal)

## Strategy Evolution

### Phases 1-2: Database-Only Testing (Limited Progress)

- **Phase 1**: +0.9pp via error injection tests
- **Phase 2**: +0.5pp via targeted low-coverage functions
- **Total**: +1.4pp from 12 tests

**Problem Identified**: Many server functions depend on unmocked components
(queue, scanner, metadata), limiting what can be tested with database mocks
alone.

### Phase 3: Expanded Mockery Strategy (NEW APPROACH)

**Decision**: Expand mockery to mock Queue, Scanner, and Metadata dependencies
using same interface pattern as database.Store.

**Implementation**:

1. ✅ Created Queue interface (Enqueue, Cancel, ActiveOperations, Shutdown)
2. ✅ Generated MockQueue with mockery v3
3. ✅ Created 6 demonstration tests - all passing
4. ✅ Validated approach works seamlessly

**Coverage Impact**: +0.0pp (tested already-covered functions as proof of
concept)

**Next Steps**:

- Create Scanner and MetadataExtractor interfaces + mocks
- Test Enqueue-dependent functions (requires all 3 mocks)
- Expected to unlock 20-30 additional testable functions

## Completed Work

### Phase 1: Initial Error Injection Tests (✅ COMPLETED)

**File**: `internal/server/server_coverage_improvements_test.go` **Coverage
Gain**: 66.0% → 66.9% (+0.9pp)

Created 8 test functions with 11 test scenarios:

1. **TestCountAudiobooksError** - Database CountBooks() error → HTTP 500
2. **TestGetAudiobookError** - Database GetBookByID() error → HTTP 500
3. **TestGetAudiobookNotFound** - Database returns nil book → HTTP 404
4. **TestBatchUpdateAudiobooksErrors** - Invalid JSON and empty array validation
5. **TestExportMetadataError** - Database GetAllBooks() error → HTTP 500
6. **TestBulkFetchMetadataErrors** - Invalid JSON and empty book_ids validation
7. **TestCancelOperationErrors** - Nil database and queue checks
8. **TestCreateWorkErrors** - Invalid JSON and missing required fields

### Phase 2: Targeted Low-Coverage Functions (✅ COMPLETED)

**File**: `internal/server/server_coverage_phase2_test.go` **Coverage Gain**:
66.9% → 67.4% (+0.5pp)

Created 4 test functions with 9 test scenarios targeting functions with 50-70%
coverage:

1. **TestGetAudiobookTagsErrors** (2 subtests):
   - Database error scenario → HTTP 500
   - Book not found (nil return) → HTTP 404
   - **Function Coverage**: 59.3% → 74.1% (+14.8pp) ⭐

2. **TestAddBlockedHashErrors** (4 subtests):
   - Invalid JSON → HTTP 400
   - Missing hash field → HTTP 400
   - Missing reason field → HTTP 400
   - Hash too short (< 64 chars) → HTTP 400

3. **TestAddBlockedHashDatabaseError** (1 test):
   - Database AddBlockedHash() error → HTTP 500
   - **Function Coverage**: 60.0% → 86.7% (+26.7pp) ⭐⭐

4. **TestDeleteWorkErrors** (2 subtests):
   - Database DeleteWork() error → HTTP 500
   - Work not found → HTTP 404
   - **Function Coverage**: 63.6% → 81.8% (+18.2pp) ⭐

**Key Learnings from Phase 2**:

- IDs in this codebase are ULID strings (e.g., "01HQWKV1234567890ABCDEFGHJK"),
  not integers
- Gin field validation errors use capitalized field names ("Hash" not "hash")
- Hash validation requires exactly 64 characters (SHA256 format)
- Both hash and reason fields are required for blocked hashes
- Work IDs are also ULIDs, and DeleteWork checks for "work not found" string

## Combined Impact Analysis

**Total Tests Added**: 12 test functions, 20 individual test scenarios

- All tests passing: ✅
- Build tags: `//go:build mocks`
- Total coverage gain: +1.4 percentage points
- **Average function improvement**: ~20pp for targeted functions

## Remaining Work to Reach 80%

**Gap**: Need +12.6 percentage points more (80% - 67.4% = 12.6pp) **Progress**:
10.6% toward goal (1.4 / 13.2 \* 100)

Note: Functions getAudiobookTags, deleteWork, and addBlockedHash now exceed 70%
and meet coverage targets! 🎉

## Next Steps - Phase 3

Focus on remaining low-coverage functions to continue progress toward 80%
target.
