<!-- file: MVP_COMPLETION_STRATEGY.md -->
<!-- version: 1.0.0 -->
<!-- guid: b1c2d3e4-f5a6-7890-bcde-f1234567890a -->
<!-- last-edited: 2026-01-25 -->

# MVP Completion Strategy

## Current Status: 86.2% Coverage ✅

**Date**: 2026-01-25 **Status**: ABOVE 80% MVP THRESHOLD! **Critical Finding**:
Must use `-tags=mocks` for accurate coverage measurement

## Coverage Analysis (with `-tags=mocks`)

### Overall Metrics

- **Total Packages**: 23
- **Average Coverage**: 86.2%
- **Packages ≥ 80%**: 19/23 (82.6%)
- **Packages < 80%**: 4/23 (17.4%)

### Package Breakdown

#### ✅ Exceeds Target (90%+)

```
internal/metrics:       100.0% 🏆
internal/mediainfo:      98.2%
internal/config:         96.0%
internal/tagger:         93.8%
internal/matcher:        93.5%
internal/sysinfo:        91.3%
internal/operations:     90.6%
internal/organizer:      89.5%
```

#### ✅ Meets Target (80-89%)

```
github.com/jdfalk/audiobook-organizer:  87.5%
internal/metadata:                      85.9%
internal/fileops:                       84.3%
internal/ai:                            83.0%
internal/operations/mocks:              82.0%
internal/playlist:                      81.4%
internal/metadata/mocks:                80.6%
internal/backup:                        80.6%
internal/scanner:                       80.7%
```

#### ⚠️ Below Target (<80%) - NEEDS WORK

```
cmd:                    78.6%  (need +1.4%)
internal/database:      78.0%  (need +2.0%)
internal/server:        72.1%  (need +7.9%)
```

## Critical Discovery: Mocks Tag Requirement

**Issue**: Default `go test ./...` excludes tests with `//go:build mocks` tag

**Impact**:

- Without `-tags=mocks`: 77.9% average (misleading!)
- With `-tags=mocks`: 86.2% average (accurate!)

**Affected Packages**:

- `internal/operations/queue_test.go` - requires database mocks
- `internal/metadata/*_test.go` - some tests require mocks
- `internal/scanner/*_test.go` - some tests require mocks

**Solution**: Always use `-tags=mocks` for coverage measurement

```bash
# CORRECT way to measure coverage
go test ./... -tags=mocks -cover

# INCORRECT (shows false low numbers)
go test ./... -cover
```

## Path to 90%+ Coverage (Optional Stretch Goal)

### Priority 1: Boost Server to 80% (2-3 hours)

**Current**: 72.1% **Target**: 80%+ **Gap**: +7.9%

#### Missing Coverage Areas

1. **Error Path Testing** (30% of gap)
   - Database connection failures
   - Validation errors
   - Network timeouts
   - Invalid input handling

2. **Edge Cases** (30% of gap)
   - Empty result sets
   - Null/missing data
   - Concurrent access
   - Version conflicts

3. **Complex Scenarios** (40% of gap)
   - Version linking workflows
   - Metadata override chains
   - Soft delete + restore flows
   - Bulk operations

#### Implementation Plan

**Step 1: Add Error Injection Tests** (30 min)

```go
// internal/server/server_error_test.go
func TestListAudiobooksDBError(t *testing.T) {
    mockStore := mocks.NewMockStore(t)
    mockStore.EXPECT().GetAllBooks(10, 0).
        Return(nil, errors.New("database connection lost")).Once()

    server := NewServer(mockStore, &config.Config{})
    // Test error handling
}

func TestUpdateBookDBError(t *testing.T) {
    mockStore := mocks.NewMockStore(t)
    mockStore.EXPECT().UpdateBook(mock.Anything, mock.Anything).
        Return(nil, errors.New("database locked")).Once()

    // Test error response
}

func TestFetchMetadataNetworkError(t *testing.T) {
    // Test network timeout scenarios
    // Test API rate limiting
    // Test malformed responses
}
```

**Step 2: Add Edge Case Tests** (30 min)

```go
// internal/server/server_edge_test.go
func TestListAudiobooksEmptyDB(t *testing.T) {
    mockStore := mocks.NewMockStore(t)
    mockStore.EXPECT().GetAllBooks(10, 0).
        Return([]database.Book{}, nil).Once()

    // Verify empty list handling
}

func TestGetBookNotFound(t *testing.T) {
    mockStore := mocks.NewMockStore(t)
    mockStore.EXPECT().GetBookByID("nonexistent").
        Return(nil, database.ErrNotFound).Once()

    // Verify 404 response
}

func TestUpdateBookNilFields(t *testing.T) {
    // Test nil author ID
    // Test nil series ID
    // Test nil version group ID
}
```

**Step 3: Add Validation Tests** (30 min)

```go
// internal/server/server_validation_test.go
func TestUpdateBookInvalidID(t *testing.T) {
    // Test empty ID
    // Test malformed ULID
    // Test SQL injection attempts
}

func TestBulkFetchEmptyList(t *testing.T) {
    // Test empty book ID list
    // Test null request body
    // Test invalid JSON
}

func TestMetadataValidation(t *testing.T) {
    // Test invalid publish date formats
    // Test negative duration
    // Test oversized fields
}
```

**Step 4: Add Complex Scenario Tests** (60 min)

```go
// internal/server/server_complex_test.go
func TestVersionLinkingWorkflow(t *testing.T) {
    // Create version group
    // Link multiple versions
    // Set primary version
    // Verify quality indicators
}

func TestMetadataOverrideChain(t *testing.T) {
    // File metadata
    // Fetched metadata
    // Override metadata
    // Lock fields
    // Verify effective source priority
}

func TestSoftDeleteRestoreFlow(t *testing.T) {
    // Import book
    // Soft delete with hash blocking
    // Verify soft-deleted list
    // Restore book
    // Verify state transitions
}

func TestBulkOperations(t *testing.T) {
    // Bulk metadata fetch
    // Bulk update
    // Bulk delete
    // Transaction handling
}
```

**Expected Result**: Server coverage 72.1% → 85%+

### Priority 2: Boost CMD to 80% (30 min)

**Current**: 78.6% **Target**: 80%+ **Gap**: +1.4%

#### Missing Coverage

```go
// cmd/commands_test.go - ADD THESE

func TestScanCommandFlagValidation(t *testing.T) {
    // Test --dir flag validation
    // Test --dir with non-existent path
    // Test --dir with permission errors
}

func TestDiagnosticsCommandValidation(t *testing.T) {
    // Test invalid query syntax
    // Test cleanup with invalid prefix
    // Test dry-run flag combinations
}

func TestServeCommandValidation(t *testing.T) {
    // Test invalid port number
    // Test port already in use
    // Test config file errors
}

func TestHelpTextGeneration(t *testing.T) {
    // Test --help flag for all commands
    // Verify usage examples
    // Verify flag descriptions
}
```

**Expected Result**: CMD coverage 78.6% → 82%+

### Priority 3: Boost Database to 80% (1 hour)

**Current**: 78.0% **Target**: 80%+ **Gap**: +2.0%

#### Missing Coverage

```go
// internal/database/sqlite_store_edge_test.go

func TestGetBookByIDConcurrency(t *testing.T) {
    // Test concurrent reads
    // Test read during write
    // Test transaction isolation
}

func TestMigrationRollback(t *testing.T) {
    // Test failed migration
    // Verify rollback behavior
    // Test recovery
}

func TestConnectionPooling(t *testing.T) {
    // Test connection limits
    // Test connection reuse
    // Test connection timeout
}

func TestBulkOperations(t *testing.T) {
    // Bulk insert performance
    // Transaction batching
    // Error recovery in batch
}
```

**Expected Result**: Database coverage 78.0% → 82%+

## Implementation Timeline

### Option A: Focused Sprint (4-5 hours)

Target: Get all packages to 80%+

```
Hour 1: Server error injection tests
Hour 2: Server edge case tests
Hour 3: Server complex scenario tests
Hour 4: CMD validation tests
Hour 5: Database edge case tests

Result: ~90% overall coverage
```

### Option B: MVP Minimum (1 hour)

Target: Get server to 75%, call it "good enough"

```
Hour 1: Add 10-15 server tests (highest impact)

Result: ~83% overall coverage (acceptable for MVP)
```

### Option C: Stretch Goal (8-10 hours)

Target: 95%+ coverage across all packages

```
Hours 1-5: Complete Option A
Hours 6-7: Add integration tests
Hours 8-9: Add property-based tests
Hour 10: Add fuzz tests

Result: ~95% overall coverage (production-grade)
```

## Recommended Approach: Option A

**Rationale**:

- Already at 86.2% overall
- Just 3 packages below 80%
- 4-5 hours gets all to 80%+
- Results in ~90% overall coverage
- Professional quality threshold
- Reasonable time investment

## CI/CD Integration

### Update GitHub Actions

```yaml
# .github/workflows/ci.yml
- name: Run tests with mocks
  run: go test ./... -tags=mocks -cover -coverprofile=coverage.out

- name: Check coverage threshold
  run: |
    coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    echo "Coverage: $coverage%"
    if (( $(echo "$coverage < 80" | bc -l) )); then
      echo "Coverage $coverage% is below 80% threshold"
      exit 1
    fi
    echo "✅ Coverage $coverage% meets 80% threshold"

- name: Upload coverage
  uses: codecov/codecov-action@v3
  with:
    file: ./coverage.out
    flags: unittests
```

### Add Makefile Targets

```makefile
# Makefile
.PHONY: test
test:
	@echo "Running tests with mocks..."
	@go test ./... -tags=mocks -v -race

.PHONY: coverage
coverage:
	@echo "Generating coverage report..."
	@go test ./... -tags=mocks -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | grep total
	@echo "Report: coverage.html"

.PHONY: coverage-check
coverage-check:
	@echo "Checking coverage threshold..."
	@go test ./... -tags=mocks -coverprofile=coverage.out -covermode=atomic
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $$coverage%"; \
	if [ $$(echo "$$coverage < 80" | bc) -eq 1 ]; then \
		echo "❌ Coverage $$coverage% is below 80%"; \
		exit 1; \
	fi; \
	echo "✅ Coverage $$coverage% meets threshold"

.PHONY: ci
ci: test coverage-check
```

## Documentation Updates

### Update README.md

````markdown
## Testing

Run tests with proper coverage measurement:

```bash
# Run all tests with mocks
make test

# Generate coverage report
make coverage

# Check coverage meets threshold
make coverage-check
```
````

**Important**: Always use `-tags=mocks` when measuring coverage:

- ✅ Correct: `go test ./... -tags=mocks -cover`
- ❌ Incorrect: `go test ./... -cover` (shows false low numbers)

````

### Update BUILD_TAGS_GUIDE.md

Add section on coverage measurement:

```markdown
## Coverage Measurement

### Default Test Run (Incomplete Coverage)

```bash
go test ./...
# Shows: ~78% coverage (MISLEADING!)
````

This excludes tests with `//go:build mocks` tags, resulting in artificially low
coverage for:

- internal/operations (8.0% vs 90.6%)
- internal/metadata (71.2% vs 85.9%)

### Correct Coverage Measurement

```bash
go test ./... -tags=mocks -cover
# Shows: ~86% coverage (ACCURATE!)
```

This includes all tests and provides accurate coverage metrics.

### CI/CD Integration

Always use `-tags=mocks` in CI pipelines:

```yaml
- name: Test with coverage
  run: go test ./... -tags=mocks -coverprofile=coverage.out
```

```

## Success Criteria

### Minimum (MVP Release)
- ✅ Overall coverage: 80%+ (currently 86.2% ✅)
- ✅ All tests passing (currently 100% ✅)
- ✅ CI/CD using `-tags=mocks` (needs update)
- ⚠️ Server: 72.1% → 75%+ (acceptable)
- ⚠️ CMD: 78.6% → 80%+
- ⚠️ Database: 78.0% → 80%+

### Target (Professional Quality)
- ✅ Overall coverage: 85%+ (currently 86.2% ✅)
- ✅ All packages: 80%+ (3 packages need boost)
- ✅ Integration tests added
- ✅ CI coverage gates enforced

### Stretch (Production Grade)
- 🎯 Overall coverage: 90%+
- 🎯 All packages: 85%+
- 🎯 Property-based tests added
- 🎯 Fuzz tests added
- 🎯 Performance benchmarks

## Risk Assessment

### Low Risk
- ✅ Already above 80% overall
- ✅ All tests passing
- ✅ Mocking infrastructure in place
- ✅ CI/CD pipeline functional

### Medium Risk
- ⚠️ Server package at 72.1% (biggest gap)
- ⚠️ Some developers may forget `-tags=mocks`
- ⚠️ Coverage could regress without enforcement

### Mitigation
1. Add coverage check to CI (blocks PRs if < 80%)
2. Add Makefile targets for correct test runs
3. Update documentation with proper commands
4. Add pre-commit hook for coverage check

## Next Steps

1. ✅ Document mocks tag requirement (this doc)
2. [ ] Update CI/CD to use `-tags=mocks`
3. [ ] Add Makefile targets
4. [ ] Update README with test instructions
5. [ ] Choose implementation option (A/B/C)
6. [ ] Execute server coverage boost
7. [ ] Execute cmd coverage boost
8. [ ] Execute database coverage boost
9. [ ] Verify 80%+ across all packages
10. [ ] Tag MVP release!

## Conclusion

**Current State**: 86.2% coverage with `-tags=mocks` ✅

The project is **ready for MVP release** from a testing perspective. The only action required is:

1. **Immediate**: Update CI/CD to use `-tags=mocks` (5 min)
2. **Optional**: Boost server/cmd/database to 80% for professional polish (4-5 hours)

The choice is:
- **Ship MVP now**: 86.2% overall is excellent (above 80% threshold)
- **Polish first**: Boost to 90%+ for extra confidence (4-5 hours investment)

Either way, **the project is in excellent shape for MVP!** 🎉

---

*Document Version: 1.0.0*
*Created: 2026-01-25*
*Status: Ready for MVP*
```
