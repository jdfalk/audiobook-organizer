<!-- file: PROJECT_IMPROVEMENT_ROADMAP.md -->
<!-- version: 1.0.0 -->
<!-- guid: f9e8d7c6-b5a4-3210-9876-fedcba098765 -->
<!-- last-edited: 2026-01-22 -->

# Audiobook Organizer - Comprehensive Improvement Roadmap

## Executive Summary

**Project Status**: 75-85% MVP Complete
**Current Health**: B+ (Good with critical blockers)
**Test Coverage**: 77.9% (Target: 80%)
**Time to MVP**: 16-26 hours focused work
**Critical Blockers**: 3 (all fixable within 5 hours)

This roadmap provides a comprehensive analysis of the audiobook-organizer project and actionable steps to reach project completion, focusing on test coverage, mockery implementation, module design, and overall code quality.

---

## Table of Contents

1. [Critical Issues](#critical-issues)
2. [Test Coverage Strategy](#test-coverage-strategy)
3. [Mockery Implementation Plan](#mockery-implementation-plan)
4. [Module Design Improvements](#module-design-improvements)
5. [Code Quality Enhancements](#code-quality-enhancements)
6. [MVP Completion Path](#mvp-completion-path)
7. [Post-MVP Roadmap](#post-mvp-roadmap)
8. [Optimization Opportunities](#optimization-opportunities)
9. [Architectural Recommendations](#architectural-recommendations)
10. [Risk Mitigation](#risk-mitigation)

---

## Critical Issues

### 🔴 CRITICAL #1: Database Test Failures

**Status**: Blocking all database package tests
**Impact**: Cannot measure coverage for core package
**Severity**: Critical

#### Root Causes

1. **Duplicate Mock Declarations**
   - `MockStore` declared in both `mock_store.go` and `mocks_test.go`
   - `NewMockStore` declared in both files
   - Compiler cannot resolve which implementation to use

2. **Missing Dependencies**

   ```
   Error: github.com/stretchr/objx is not in your go.mod file
   Error: missing go.sum entry for module providing package github.com/stretchr/objx
   ```

3. **Import Failures**
   - `github.com/stretchr/testify/mock` cannot be imported
   - Methods like `mock.Mock`, `mock.AssertExpectations`, `_mock.Called` are undefined

#### Affected Files

```
internal/database/mock_store.go:14:6     - MockStore redeclared
internal/database/mock_store.go:54:6     - NewMockStore redeclared
internal/database/mocks_test.go:15:6     - NewMockStore redeclared
internal/database/mocks_test.go:28:6     - MockStore redeclared
internal/database/mocks_test.go:10:2     - import error
go.mod:1:1                               - missing objx dependency
go.mod:17:2                              - missing go.sum entry
```

#### Solution (Estimated Time: 1 hour)

**Step 1: Fix Dependencies (5 minutes)**

```bash
# Add missing dependency
go get github.com/stretchr/testify/mock@v1.11.1

# Clean up go.mod and go.sum
go mod tidy

# Verify
go test ./internal/database -v
```

**Step 2: Choose Mocking Strategy (See [Mockery Implementation Plan](#mockery-implementation-plan))**

**Option A: Keep Manual MockStore (Recommended for immediate fix)**

```bash
# Remove mockery-generated file
rm internal/database/mocks_test.go

# Verify tests pass
go test ./internal/database -v
```

**Option B: Adopt Mockery (Recommended for long-term)**

```bash
# Remove manual mock
rm internal/database/mock_store.go

# Fix imports in tests to use generated mocks
# Update test files to use mocks package

# Regenerate mocks
mockery --all
```

**Step 3: Verify (5 minutes)**

```bash
# Run all tests
go test ./... -v -cover

# Verify database tests pass
go test ./internal/database -v -cover
```

---

### 🔴 CRITICAL #2: Server Package Coverage Gap

**Status**: 66.0% coverage (Target: 80%+)
**Impact**: Quality gate for MVP
**Severity**: High

#### Coverage Gaps (from MOCKERY_SUMMARY.md)

1. **Error Paths** (~30% missing)
   - Database connection failures
   - Validation errors
   - Network timeouts
   - Invalid input handling

2. **Edge Cases**
   - Empty result sets
   - Null/missing data
   - Concurrent access scenarios
   - Version conflict handling

3. **Complex Scenarios**
   - Version linking workflows
   - Metadata override/lock chains
   - Soft delete + restore flows
   - Bulk operations

#### Solution (Estimated Time: 2-3 hours)

**See [Test Coverage Strategy](#test-coverage-strategy) for detailed plan**

Quick wins to reach 80%:

1. Add error injection tests using mocks (30 min)
2. Add edge case coverage (empty lists, nulls) (30 min)
3. Add validation error tests (30 min)
4. Add complex scenario tests (version linking, overrides) (60 min)

**Projected Coverage**: 66% → 85%+ (19% improvement)

---

### 🔴 CRITICAL #3: CMD Package Coverage

**Status**: 78.6% coverage (Target: 80%+)
**Impact**: Minor blocker for MVP
**Severity**: Medium

#### Missing Coverage

- Error handling in commands
- Flag validation
- Help text generation paths

#### Solution (Estimated Time: 30-60 minutes)

```go
// Add tests for error scenarios
func TestScanCommandErrors(t *testing.T) {
    // Test missing directory flag
    // Test invalid directory path
    // Test permission errors
}

func TestDiagnosticsCommandErrors(t *testing.T) {
    // Test database connection failures
    // Test invalid query syntax
    // Test cleanup dry-run scenarios
}
```

**Projected Coverage**: 78.6% → 82%+ (3.4% improvement)

---

## Test Coverage Strategy

### Current State Analysis

#### Coverage Breakdown

```
Tier                    Coverage Range    Packages    Avg Coverage
------------------------------------------------------------------------
Perfect                 100%              1           100.0%
Excellent               90-99%            5           94.5%
Good                    80-89%            8           85.1%
Acceptable              70-79%            1           78.6%
Below Target            <70%              1           66.0%
Failing                 N/A               1           N/A
------------------------------------------------------------------------
TOTAL                   77.9% overall     17 packages
```

### Path to 80% Overall Coverage

#### Priority 1: Fix Database Tests (CRITICAL)

**Current**: FAIL → **Target**: 85%
**Effort**: 1-2 hours
**Impact**: +7% to overall coverage

**Actions**:

1. ✅ Fix dependencies (`go mod tidy`)
2. ✅ Resolve mock duplicates
3. ✅ Run existing tests to get baseline
4. ✅ Add tests for uncovered Store methods

#### Priority 2: Boost Server Coverage

**Current**: 66.0% → **Target**: 85%
**Effort**: 2-3 hours
**Impact**: +3-4% to overall coverage

**Actions**:

1. ✅ Integrate mockery for database mocking
2. ✅ Add error injection tests (database errors, network errors)
3. ✅ Add edge case tests (empty results, missing data)
4. ✅ Add complex scenario tests (version linking, metadata provenance)
5. ✅ Add validation error tests

**Test Categories to Add**:

```go
// 1. Error Injection Tests (30 min)
func TestListAudiobooksDBError(t *testing.T)
func TestUpdateBookDBError(t *testing.T)
func TestFetchMetadataNetworkError(t *testing.T)

// 2. Edge Case Tests (30 min)
func TestListAudiobooksEmptyDB(t *testing.T)
func TestGetBookNotFound(t *testing.T)
func TestUpdateBookNilFields(t *testing.T)

// 3. Validation Tests (30 min)
func TestUpdateBookInvalidID(t *testing.T)
func TestBulkFetchEmptyList(t *testing.T)

// 4. Complex Scenarios (60 min)
func TestVersionLinkingWorkflow(t *testing.T)
func TestMetadataOverrideChain(t *testing.T)
func TestSoftDeleteRestoreFlow(t *testing.T)
```

#### Priority 3: Boost CMD Coverage

**Current**: 78.6% → **Target**: 82%
**Effort**: 30-60 min
**Impact**: +0.5% to overall coverage

**Actions**:

1. ✅ Add command error tests
2. ✅ Add flag validation tests
3. ✅ Add help text tests

### Test Infrastructure Improvements

#### Recommended Testing Tools

**1. Mockery (already in progress)**

```yaml
# .mockery.yaml
packages:
  github.com/jdfalk/audiobook-organizer/internal/database:
    interfaces:
      Store:
        mockery-version: v2
```

**Benefits**:

- Automatic mock generation
- Type-safe mocks
- Call verification
- Argument matchers

**2. testify/suite**

```go
// Use test suites for setup/teardown
type ServerTestSuite struct {
    suite.Suite
    server *Server
    mockDB *mocks.MockStore
}

func (s *ServerTestSuite) SetupTest() {
    s.mockDB = mocks.NewMockStore(s.T())
    s.server = NewServer(s.mockDB)
}
```

**3. Table-Driven Tests**

```go
// Efficient coverage of multiple scenarios
tests := []struct {
    name    string
    input   *Book
    mockSetup func(*mocks.MockStore)
    wantErr bool
}{
    {"success", validBook, successMock, false},
    {"db_error", validBook, errorMock, true},
    {"nil_book", nil, nilMock, true},
}
```

---

## Mockery Implementation Plan

### Current Situation

**Problem**: Three competing mock implementations

1. `mock_store.go` (1,110 lines) - Manual implementation
2. `mocks_test.go` (56,398 lines!) - Mockery-generated
3. `mock.go` + `mock_test.go` - Additional mock code

**Result**: Build failures, duplicate declarations, confusion

### Recommended Solution: Adopt Mockery

#### Why Mockery?

**Advantages** ✅:

- ✅ Auto-generated (zero maintenance)
- ✅ Type-safe (compiler catches errors)
- ✅ Call verification built-in
- ✅ Argument matchers
- ✅ Industry standard
- ✅ Works with testify

**Disadvantages** ❌:

- ⚠️ Requires tool installation
- ⚠️ Extra build step
- ⚠️ Large generated files

**Verdict**: Benefits outweigh costs

#### Implementation Steps

**Phase 1: Setup (30 minutes)**

```bash
# 1. Install mockery
go install github.com/vektra/mockery/v2@latest

# 2. Verify installation
mockery --version

# 3. Create mockery config
cat > .mockery.yaml <<EOF
packages:
  github.com/jdfalk/audiobook-organizer/internal/database:
    interfaces:
      Store:
        output: internal/database/mocks
        outpkg: mocks
        filename: mock_store.go
EOF

# 4. Generate mocks
mockery --all

# 5. Verify generation
ls -lh internal/database/mocks/
```

**Phase 2: Migration (1-2 hours)**

```bash
# 1. Backup existing mocks
mv internal/database/mock_store.go internal/database/mock_store.go.bak
mv internal/database/mocks_test.go internal/database/mocks_test.go.bak

# 2. Update imports in test files
# Before:
import "github.com/jdfalk/audiobook-organizer/internal/database"
mockStore := database.NewMockStore()

# After:
import "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
mockStore := mocks.NewMockStore(t)

# 3. Update test setup
# Before:
mockStore := database.NewMockStore()
mockStore.Books["123"] = &database.Book{ID: "123"}

# After:
mockStore := mocks.NewMockStore(t)
mockStore.EXPECT().
    GetBookByID("123").
    Return(&database.Book{ID: "123"}, nil).
    Once()
```

**Phase 3: Test Conversion (2-3 hours)**

Convert existing tests to use mockery mocks:

```go
// Example: server_test.go

// BEFORE (manual mock)
func TestListAudiobooks(t *testing.T) {
    mockStore := database.NewMockStore()
    mockStore.Books["1"] = &database.Book{ID: "1", Title: "Test"}

    server := NewServer(mockStore)
    books, err := server.ListBooks()

    assert.NoError(t, err)
    assert.Len(t, books, 1)
}

// AFTER (mockery)
func TestListAudiobooks(t *testing.T) {
    mockStore := mocks.NewMockStore(t)
    mockStore.EXPECT().
        GetAllBooks(10, 0).
        Return([]database.Book{{ID: "1", Title: "Test"}}, nil).
        Once()

    server := NewServer(mockStore)
    books, err := server.ListBooks()

    assert.NoError(t, err)
    assert.Len(t, books, 1)
    mockStore.AssertExpectations(t) // Automatic verification!
}
```

**Phase 4: CI/CD Integration (15 minutes)**

```bash
# 1. Add make target
cat >> Makefile <<EOF
.PHONY: mocks
mocks:
 @echo "Generating mocks..."
 @mockery --all
 @echo "Mocks generated successfully"

.PHONY: test
test: mocks
 @echo "Running tests..."
 @go test ./... -v -cover
EOF

# 2. Update GitHub Actions
# .github/workflows/ci.yml
- name: Generate mocks
  run: make mocks

- name: Run tests
  run: make test
```

**Phase 5: Verification (30 minutes)**

```bash
# 1. Run all tests
go test ./... -v -cover

# 2. Check coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# 3. Verify no regressions
# Compare coverage before/after
```

### Alternative: Keep Manual Mocks

**When to choose this**:

- Need to ship MVP ASAP (minimal changes)
- Team unfamiliar with mockery
- Don't want tool dependency

**Steps**:

1. ✅ Remove `mocks_test.go` (mockery-generated)
2. ✅ Keep `mock_store.go` (manual)
3. ✅ Fix `go.mod` dependencies
4. ✅ Verify tests pass

**Trade-offs**:

- ✅ Faster to implement (30 min vs 3-4 hours)
- ❌ Manual maintenance burden
- ❌ No automatic verification
- ❌ Harder to add new tests

---

## Module Design Improvements

### Issue #1: Database Package Bloat

**Current State**:

- **29 files** in `internal/database/`
- **Unclear separation** of concerns
- **Large generated file** (mocks_test.go: 56,398 lines)

**Impact**:

- Hard to navigate
- Difficult to onboard new developers
- Build times affected
- Unclear ownership

#### Recommended Structure

**Option A: Separate Mocks (Minimal Change)**

```
internal/database/
├── audiobooks.go          # Book CRUD
├── audiobooks_test.go
├── database.go            # Init and config
├── interface.go           # Store interface
├── migrations.go          # Schema migrations
├── pebble_store.go        # PebbleDB impl
├── pebble_store_test.go
├── settings.go            # Settings CRUD
├── settings_test.go
├── sqlite_store.go        # SQLite impl
├── sqlite_store_test.go
├── store.go               # Common store logic
├── store_test.go
├── web.go                 # Web-specific queries
└── mocks/                 # ← NEW: Separate directory
    └── mock_store.go      # Generated mocks
```

**Benefits**:

- Clear separation of mocks from production code
- Generated files don't pollute main package
- Easy to gitignore generated mocks

**Implementation**:

```bash
# 1. Create mocks directory
mkdir -p internal/database/mocks

# 2. Update .mockery.yaml
output: internal/database/mocks

# 3. Regenerate mocks
mockery --all

# 4. Update .gitignore
echo "internal/database/mocks/" >> .gitignore

# 5. Update test imports
# Replace: import "...database"
# With: import "...database/mocks"
```

**Option B: Decompose Database Package (Better Long-Term)**

```
internal/
├── database/
│   ├── database.go        # Init and config
│   ├── interface.go       # Store interface
│   └── migrations/        # ← NEW
│       ├── migrations.go
│       └── migrations_test.go
├── store/                 # ← NEW
│   ├── store.go          # Common logic
│   ├── sqlite/           # ← NEW
│   │   ├── sqlite_store.go
│   │   ├── sqlite_store_test.go
│   │   └── queries.go
│   ├── pebble/           # ← NEW
│   │   ├── pebble_store.go
│   │   └── pebble_store_test.go
│   └── mocks/            # ← NEW
│       └── mock_store.go  # Generated
├── models/
│   ├── audiobook.go      # Moved from database/
│   ├── author.go
│   ├── series.go
│   └── work.go
```

**Benefits**:

- Clear package boundaries
- Easier to understand responsibilities
- Better testability (smaller units)
- Supports future growth (e.g., PostgreSQL store)

**Trade-offs**:

- More upfront refactoring work (4-6 hours)
- Import path changes throughout codebase
- Potential merge conflicts if active development

**Recommendation**: Start with Option A, plan Option B for post-MVP

### Issue #2: Server Package Test Gap

**Current State**:

- **12 files** in `internal/server/`
- **66.0% coverage** (need 80%+)
- Missing error path coverage

**Recommended Structure**:

```
internal/server/
├── server.go              # Core server setup
├── server_test.go         # Integration tests
├── server_config_test.go  # Config tests
├── handlers/              # ← NEW: Group by feature
│   ├── audiobooks.go      # Book endpoints
│   ├── audiobooks_test.go
│   ├── authors.go         # Author endpoints
│   ├── authors_test.go
│   ├── metadata.go        # Metadata endpoints
│   ├── metadata_test.go
│   ├── operations.go      # Operation endpoints
│   ├── operations_test.go
│   └── system.go          # System/health endpoints
├── middleware/            # ← NEW: Middleware
│   ├── auth.go           # Future: Authentication
│   ├── cors.go           # CORS handling
│   ├── logging.go        # Request logging
│   └── recovery.go       # Panic recovery
└── static/               # ← Existing
    ├── static_embed.go
    └── static_nonembed.go
```

**Benefits**:

- Smaller, focused files
- Easier to test individual features
- Clear handler organization
- Supports middleware expansion

**Implementation Path**:

1. Extract handlers to `handlers/` package (2-3 hours)
2. Update imports
3. Move tests alongside handlers
4. Add missing test coverage

---

## Code Quality Enhancements

### 1. Reduce Code Duplication

**Identified Duplications**:

#### A. Store Interface Implementations

**Problem**: SQLiteStore and PebbleStore have duplicate logic

**Solution**: Extract common operations

```go
// internal/store/common.go
type BaseStore struct {}

func (b *BaseStore) validateBook(book *Book) error {
    // Common validation logic
}

func (b *BaseStore) generateID() string {
    // Common ID generation
}

// internal/store/sqlite/sqlite_store.go
type SQLiteStore struct {
    BaseStore
    db *sql.DB
}

// Inherit common methods, override specific ones
```

#### B. Test Setup Code

**Problem**: Repeated setup in test files

**Solution**: Use test helpers

```go
// internal/database/testhelpers/helpers.go
func NewTestStore(t *testing.T) *SQLiteStore {
    db, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)

    store, err := NewSQLiteStore(db)
    require.NoError(t, err)

    t.Cleanup(func() { store.Close() })

    return store
}

// Usage in tests
func TestGetBook(t *testing.T) {
    store := testhelpers.NewTestStore(t)
    // Test logic...
}
```

### 2. Improve Error Handling

**Current Issues**:

- Generic error messages
- No error wrapping for context
- Hard to debug production issues

**Recommended Patterns**:

```go
// Before
func (s *SQLiteStore) GetBookByID(id string) (*Book, error) {
    row := s.db.QueryRow("SELECT * FROM books WHERE id = ?", id)
    var book Book
    err := row.Scan(&book.ID, &book.Title)
    if err != nil {
        return nil, err  // ❌ Lost context
    }
    return &book, nil
}

// After
func (s *SQLiteStore) GetBookByID(id string) (*Book, error) {
    row := s.db.QueryRow("SELECT * FROM books WHERE id = ?", id)
    var book Book
    err := row.Scan(&book.ID, &book.Title)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, fmt.Errorf("book not found: %s", id)
        }
        return nil, fmt.Errorf("failed to get book %s: %w", id, err)
    }
    return &book, nil
}
```

**Benefits**:

- Better error messages
- Easier debugging
- Error type checking with errors.Is/As

### 3. Add Static Analysis

**Recommended Tools**:

```bash
# 1. golangci-lint (comprehensive linter)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# .golangci.yml
linters:
  enable:
    - gofmt
    - goimports
    - govet
    - errcheck
    - staticcheck
    - unused
    - gosimple
    - ineffassign

# 2. Run in CI
golangci-lint run ./...

# 3. gosec (security analysis)
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...

# 4. gocyclo (complexity)
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
gocyclo -over 15 .
```

### 4. Improve Code Documentation

**Current State**: Some packages well-documented, others sparse

**Recommended Standards**:

```go
// Package server provides HTTP API endpoints for audiobook management.
//
// The server exposes a RESTful API for:
//   - Audiobook CRUD operations
//   - Metadata fetching and editing
//   - File organization
//   - Operation tracking
//
// Example usage:
//   store, _ := database.NewSQLiteStore(db)
//   server := server.NewServer(store, config)
//   server.Start(":8080")
package server

// GetBookByID retrieves an audiobook by its unique identifier.
//
// Returns an error if:
//   - Book does not exist (ErrNotFound)
//   - Database connection fails
//   - ID is invalid
//
// Example:
//   book, err := store.GetBookByID("01HV8Z...")
//   if errors.Is(err, database.ErrNotFound) {
//       // Handle missing book
//   }
func (s *SQLiteStore) GetBookByID(id string) (*Book, error) {
    // Implementation...
}
```

**Tools for Documentation**:

```bash
# Generate HTML docs
godoc -http=:6060

# Check documentation coverage
gocover -func=coverage.out
```

### 5. Enforce Code Style

**Add to CI Pipeline**:

```yaml
# .github/workflows/ci.yml
- name: Format check
  run: |
    gofmt -l . | grep . && exit 1 || exit 0

- name: Imports check
  run: |
    go install golang.org/x/tools/cmd/goimports@latest
    goimports -l . | grep . && exit 1 || exit 0

- name: Lint
  run: |
    golangci-lint run ./...
```

---

## MVP Completion Path

### Current Status: 75-85% Complete

**Remaining Work**: 16-26 hours

### Phase 1: Critical Fixes (4-5 hours)

#### 1.1 Fix Database Tests (1-2 hours)

- [ ] Run `go mod tidy` to fix dependencies (5 min)
- [ ] Remove duplicate mock declarations (15 min)
- [ ] Choose and implement mocking strategy (30-60 min)
- [ ] Verify all database tests pass (15 min)
- [ ] Measure baseline coverage (5 min)

**Deliverable**: Database tests passing, coverage measured

#### 1.2 Boost Server Coverage (2-3 hours)

- [ ] Setup mockery (if chosen) (30 min)
- [ ] Add error injection tests (30 min)
- [ ] Add edge case tests (30 min)
- [ ] Add validation tests (30 min)
- [ ] Add complex scenario tests (60 min)

**Deliverable**: Server coverage ≥ 80%

#### 1.3 Boost CMD Coverage (30-60 min)

- [ ] Add command error tests (20 min)
- [ ] Add flag validation tests (20 min)
- [ ] Add help text tests (20 min)

**Deliverable**: CMD coverage ≥ 80%

**Phase 1 Exit Criteria**:
✅ All tests passing
✅ Overall coverage ≥ 80%
✅ Database package green
✅ Server package ≥ 80%
✅ CMD package ≥ 80%

### Phase 2: Manual QA (2-3 hours)

**From TODO.md P0**:

- [ ] Library workflows (search/sort, import path CRUD, scan operations)
- [ ] Book Detail (all tabs: info, files, versions, tags, compare)
- [ ] Metadata editing (edit/fetch, AI parse, bulk fetch)
- [ ] Soft delete + block hash
- [ ] Settings (blocked hashes tab, config persistence, system info)
- [ ] Dashboard (stats accuracy, navigation)
- [ ] State transitions (import → organized → deleted → purged)
- [ ] Version management (link versions, set primary, quality indicators)

**Deliverable**: QA checklist completed, bugs logged

### Phase 3: E2E Tests (4-6 hours)

**From TODO.md P0**:

- [ ] Expand Playwright coverage beyond smoke tests
- [ ] Library interactions (search/sort/pagination)
- [ ] Metadata fetch and AI parse trigger
- [ ] Book Detail flows (tab navigation, soft delete, version linking)
- [ ] Settings workflows (add/remove import paths end-to-end)
- [ ] Soft-deleted list (restore and purge actions)

**Deliverable**: E2E test suite covering critical workflows

### Phase 4: Release Pipeline (2-3 hours)

**From TODO.md P0**:

- [ ] Fix prerelease workflow token permissions
- [ ] Confirm GoReleaser publish works
- [ ] Verify Docker frontend build
- [ ] Replace local changelog stub
- [ ] Test full release pipeline

**Deliverable**: Automated release pipeline working

### Phase 5: Documentation (2-3 hours)

**From TODO.md P1**:

- [ ] Capture manual verification notes
- [ ] Update CHANGELOG with Phase 1-4 changes
- [ ] Update TODO with completion status
- [ ] Update README if needed
- [ ] Create deployment guide

**Deliverable**: Documentation up-to-date

---

## Post-MVP Roadmap

### Quarter 1: Stability & Performance (6-8 weeks)

#### Week 1-2: Architecture Refinement

- [ ] Decompose database package (Option B from Module Design)
- [ ] Extract server handlers to `handlers/` package
- [ ] Implement middleware layer
- [ ] Add request/response logging

#### Week 3-4: Performance Optimization

- [ ] Implement parallel scanning (goroutine pool)
- [ ] Add caching layer (LRU for book queries)
- [ ] Implement debounced library size computation
- [ ] Add batch metadata fetch pipeline

#### Week 5-6: Observability

- [ ] Add Prometheus metrics endpoint
- [ ] Implement structured logging
- [ ] Add operation timing summaries
- [ ] Create Grafana dashboards

#### Week 7-8: Testing & Quality

- [ ] Achieve 85%+ coverage across all packages
- [ ] Add load tests (1000+ file scan)
- [ ] Add fuzz tests for filename parser
- [ ] Implement chaos testing

### Quarter 2: Features & Integration (6-8 weeks)

#### Week 1-2: Metadata Enhancement

- [ ] Add multiple metadata source support
- [ ] Implement metadata confidence scoring
- [ ] Add batch AI parse queue
- [ ] Create metadata merge policy editor

#### Week 3-4: External Integrations

- [ ] Calibre metadata export
- [ ] OPDS feed generation
- [ ] Plex/Jellyfin library sync
- [ ] iTunes library integration

#### Week 5-6: Advanced Features

- [ ] Audio transcoding (MP3 → M4B)
- [ ] Chapter detection and metadata
- [ ] Cover art enhancement
- [ ] Duplicate detection improvements

#### Week 7-8: UX Improvements

- [ ] Dark mode
- [ ] Keyboard shortcuts
- [ ] Progressive loading
- [ ] Mobile responsiveness

### Quarter 3: Multi-User & Security (6-8 weeks)

#### Week 1-2: Authentication

- [ ] User registration/login
- [ ] JWT token management
- [ ] Role-based access control
- [ ] Session management

#### Week 3-4: Per-User Features

- [ ] User-specific libraries
- [ ] Playback progress tracking
- [ ] Personal notes and annotations
- [ ] Favorites and ratings

#### Week 5-6: Security Hardening

- [ ] TLS/HTTPS support
- [ ] Let's Encrypt integration
- [ ] API key authentication
- [ ] Audit logging

#### Week 7-8: Performance & Scale

- [ ] Database optimization for multi-user
- [ ] Connection pooling
- [ ] Caching improvements
- [ ] Load balancing support

### Quarter 4: Enterprise Features (6-8 weeks)

#### Week 1-2: Advanced Deployment

- [ ] Docker multi-arch builds
- [ ] Kubernetes Helm chart
- [ ] Database migration tools
- [ ] Backup/restore automation

#### Week 3-4: Monitoring & Alerting

- [ ] Health check improvements
- [ ] Error aggregation dashboard
- [ ] Slow operation detector
- [ ] Alert configuration

#### Week 5-6: Integration & Automation

- [ ] Webhook system
- [ ] BitTorrent client integration
- [ ] Automated file organization
- [ ] Scheduled tasks

#### Week 7-8: Documentation & Support

- [ ] Architecture diagrams
- [ ] API documentation (Swagger UI)
- [ ] Operations handbook
- [ ] Video tutorials

---

## Optimization Opportunities

### 1. Database Query Optimization

**Current Issues**:

- No query caching
- Full table scans for some queries
- N+1 query problems

**Solutions**:

```go
// Add caching layer
type CachedStore struct {
    store Store
    cache *lru.Cache
}

func (c *CachedStore) GetBookByID(id string) (*Book, error) {
    // Check cache first
    if cached, ok := c.cache.Get(id); ok {
        return cached.(*Book), nil
    }

    // Fetch from store
    book, err := c.store.GetBookByID(id)
    if err != nil {
        return nil, err
    }

    // Cache result
    c.cache.Add(id, book)
    return book, nil
}

// Add database indices
// migrations/011_add_indices.sql
CREATE INDEX IF NOT EXISTS idx_books_author_id ON books(author_id);
CREATE INDEX IF NOT EXISTS idx_books_series_id ON books(series_id);
CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash);
CREATE INDEX IF NOT EXISTS idx_books_library_state ON books(library_state);

// Optimize queries with JOINs
SELECT b.*, a.name as author_name, s.name as series_name
FROM books b
LEFT JOIN authors a ON b.author_id = a.id
LEFT JOIN series s ON b.series_id = s.id
WHERE b.library_state != 'deleted'
LIMIT ? OFFSET ?
```

### 2. Scanner Performance

**Current Issues**:

- Single-threaded scanning
- No progress checkpointing
- Full re-scan on interruption

**Solutions**:

```go
// Parallel scanner with worker pool
type ParallelScanner struct {
    workers    int
    fileQueue  chan string
    resultChan chan *Book
}

func (s *ParallelScanner) ScanDirectory(path string) error {
    // Create worker pool
    var wg sync.WaitGroup
    for i := 0; i < s.workers; i++ {
        wg.Add(1)
        go s.worker(&wg)
    }

    // Walk directory and queue files
    go s.walkDirectory(path)

    // Collect results
    go s.collectResults()

    wg.Wait()
    return nil
}

// Add checkpointing
type ScanCheckpoint struct {
    OperationID string
    LastPath    string
    ProcessedCount int
    TotalCount  int
}

func (s *ParallelScanner) SaveCheckpoint(cp *ScanCheckpoint) error {
    // Save to database for resume
}
```

### 3. Memory Optimization

**Current Issues**:

- No memory limits
- Large result sets loaded entirely
- No streaming for big operations

**Solutions**:

```go
// Implement pagination everywhere
func (s *SQLiteStore) GetAllBooks(limit, offset int) ([]Book, error) {
    // Already implemented ✓
}

// Add memory monitoring
type MemoryMonitor struct {
    threshold uint64
}

func (m *MemoryMonitor) Check() error {
    var mem runtime.MemStats
    runtime.ReadMemStats(&mem)

    if mem.Alloc > m.threshold {
        runtime.GC() // Trigger garbage collection
        log.Warn("Memory threshold exceeded, triggered GC")
    }
    return nil
}

// Stream large results
func (s *SQLiteStore) StreamBooks(ctx context.Context) (<-chan *Book, error) {
    bookChan := make(chan *Book, 100)

    go func() {
        defer close(bookChan)
        // Query and stream results
        rows, _ := s.db.QueryContext(ctx, "SELECT * FROM books")
        defer rows.Close()

        for rows.Next() {
            var book Book
            rows.Scan(&book)

            select {
            case bookChan <- &book:
            case <-ctx.Done():
                return
            }
        }
    }()

    return bookChan, nil
}
```

### 4. Frontend Performance

**Current Issues**:

- No virtualization for long lists
- Full re-renders on updates
- No code splitting

**Solutions**:

```tsx
// Add virtual scrolling
import { FixedSizeList } from 'react-window';

function AudiobookList({ books }: { books: Book[] }) {
  const Row = ({ index, style }: { index: number; style: any }) => (
    <div style={style}>
      <AudiobookCard book={books[index]} />
    </div>
  );

  return (
    <FixedSizeList
      height={800}
      itemCount={books.length}
      itemSize={200}
      width="100%"
    >
      {Row}
    </FixedSizeList>
  );
}

// Add React.memo for expensive components
const AudiobookCard = React.memo(({ book }: { book: Book }) => {
  // Component logic
});

// Implement code splitting
const BookDetail = lazy(() => import('./pages/BookDetail'));
const Settings = lazy(() => import('./pages/Settings'));

// Add query caching with React Query
import { useQuery } from '@tanstack/react-query';

function useBooks() {
  return useQuery({
    queryKey: ['books'],
    queryFn: fetchBooks,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}
```

---

## Architectural Recommendations

### 1. Adopt Hexagonal Architecture

**Current**: Monolithic structure with tight coupling

**Proposed**:

```
internal/
├── core/                  # Business logic (domain)
│   ├── audiobook/
│   │   ├── audiobook.go   # Entity
│   │   ├── service.go     # Business logic
│   │   └── ports.go       # Interfaces
│   ├── metadata/
│   └── operation/
├── adapters/              # External integrations
│   ├── http/             # HTTP server
│   ├── store/            # Database
│   └── external/         # APIs (OpenLibrary, OpenAI)
└── infrastructure/       # Cross-cutting concerns
    ├── config/
    ├── logging/
    └── metrics/
```

**Benefits**:

- Clear boundaries
- Easy to test (mock ports)
- Swappable implementations
- Better separation of concerns

### 2. Implement Repository Pattern

**Current**: Store interface is too broad

**Proposed**:

```go
// Domain layer (core)
type AudiobookRepository interface {
    Save(ctx context.Context, book *Audiobook) error
    FindByID(ctx context.Context, id string) (*Audiobook, error)
    FindAll(ctx context.Context, filter Filter) ([]Audiobook, error)
    Delete(ctx context.Context, id string) error
}

// Infrastructure layer (adapters/store)
type SQLiteAudiobookRepository struct {
    db *sql.DB
}

func (r *SQLiteAudiobookRepository) Save(ctx context.Context, book *Audiobook) error {
    // SQLite-specific implementation
}

// Benefits:
// - Clear API surface
// - Easy to mock
// - Technology-agnostic domain
```

### 3. Add Domain Events

**Current**: Direct coupling between components

**Proposed**:

```go
// Domain events
type BookScanned struct {
    BookID    string
    FilePath  string
    Timestamp time.Time
}

type MetadataFetched struct {
    BookID   string
    Source   string
    Metadata map[string]interface{}
}

// Event bus
type EventBus interface {
    Publish(event interface{})
    Subscribe(eventType reflect.Type, handler EventHandler)
}

// Usage
bus.Subscribe(BookScanned{}, func(e interface{}) {
    event := e.(BookScanned)
    // Trigger metadata fetch
    // Update statistics
    // Send SSE notification
})

// Benefits:
// - Decoupled components
// - Easy to add features
// - Event sourcing possible
```

### 4. Implement CQRS

**Current**: Same models for read and write

**Proposed**:

```go
// Command side (writes)
type CreateBookCommand struct {
    FilePath string
    Title    string
    Author   string
}

func (h *BookCommandHandler) Handle(cmd CreateBookCommand) error {
    book := NewBook(cmd)
    return h.repo.Save(book)
}

// Query side (reads)
type BookListQuery struct {
    Filters []Filter
    Page    int
    PerPage int
}

type BookListView struct {
    ID       string
    Title    string
    Author   string
    CoverURL string
}

func (h *BookQueryHandler) Handle(q BookListQuery) ([]BookListView, error) {
    // Optimized read model
}

// Benefits:
// - Optimized queries
// - Scalable reads
// - Clear intent
```

---

## Risk Mitigation

### 1. Test Coverage Regression

**Risk**: Coverage drops after hitting 80%

**Mitigation**:

- [ ] Add coverage gate in CI (minimum 80%)
- [ ] Set up coverage tracking (coveralls.io or codecov)
- [ ] Add pre-commit hook for coverage check
- [ ] Review coverage in PRs

```yaml
# .github/workflows/ci.yml
- name: Check coverage
  run: |
    go test ./... -coverprofile=coverage.out
    coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$coverage < 80" | bc -l) )); then
      echo "Coverage $coverage% is below 80%"
      exit 1
    fi
```

### 2. Database Migration Failures

**Risk**: Schema changes break production

**Mitigation**:

- [ ] Add migration tests
- [ ] Implement rollback capability
- [ ] Version migrations clearly
- [ ] Test migrations on production-like data

```go
// Test migrations
func TestMigration011(t *testing.T) {
    // Setup database at version 10
    db := setupTestDB(t)
    applyMigration(db, 10)

    // Insert test data
    insertTestData(db)

    // Apply migration 11
    err := applyMigration(db, 11)
    require.NoError(t, err)

    // Verify data integrity
    verifyData(db)

    // Test rollback
    err = rollbackMigration(db, 11)
    require.NoError(t, err)
}
```

### 3. Mock-Reality Mismatch

**Risk**: Mocks diverge from actual behavior

**Mitigation**:

- [ ] Add integration tests with real database
- [ ] Use contract testing
- [ ] Regenerate mocks frequently
- [ ] Add make target to verify mocks are up-to-date

```bash
# Makefile
.PHONY: verify-mocks
verify-mocks:
 @echo "Regenerating mocks..."
 @mockery --all
 @echo "Checking for differences..."
 @git diff --exit-code internal/database/mocks/ || \
  (echo "Mocks are out of date! Run 'make mocks' and commit." && exit 1)
```

### 4. Performance Regressions

**Risk**: New features slow down the app

**Mitigation**:

- [ ] Add benchmark tests
- [ ] Run benchmarks in CI
- [ ] Track performance metrics over time
- [ ] Set performance budgets

```go
// Benchmark scan performance
func BenchmarkScanDirectory(b *testing.B) {
    scanner := NewScanner(db)
    dir := setupTestDirectory(b)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        scanner.ScanDirectory(dir)
    }
}

// Run in CI
- name: Run benchmarks
  run: |
    go test -bench=. -benchmem ./... | tee benchmark.txt
    # Compare with baseline
```

### 5. Dependency Vulnerabilities

**Risk**: Security vulnerabilities in dependencies

**Mitigation**:

- [ ] Enable Dependabot
- [ ] Add gosec to CI
- [ ] Regular dependency updates
- [ ] Security scanning in pipeline

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"

  - package-ecosystem: "npm"
    directory: "/web"
    schedule:
      interval: "weekly"
```

---

## Conclusion

### Summary of Recommendations

**Immediate (This Week)**:

1. ✅ Fix database test failures (1 hour)
2. ✅ Adopt mockery for testing (3-4 hours)
3. ✅ Boost server coverage to 80%+ (2-3 hours)
4. ✅ Achieve 80% overall coverage (4-5 hours total)

**Short-term (Next 2 Weeks)**:

1. ✅ Complete MVP P0 items (16-26 hours)
2. ✅ Reorganize database package (2-3 hours)
3. ✅ Add static analysis to CI (1 hour)
4. ✅ Implement code quality tools (2-3 hours)

**Medium-term (Next Month)**:

1. ✅ Decompose monolithic packages
2. ✅ Implement performance optimizations
3. ✅ Add comprehensive observability
4. ✅ Expand E2E test coverage

**Long-term (Next Quarter)**:

1. ✅ Adopt hexagonal architecture
2. ✅ Implement multi-user features
3. ✅ Add external integrations
4. ✅ Harden security

### Expected Outcomes

**After Critical Fixes** (Week 1):

- ✅ All tests passing
- ✅ 80%+ test coverage
- ✅ Stable build pipeline
- ✅ Clear mocking strategy

**After MVP Completion** (Weeks 2-4):

- ✅ Production-ready v1.0
- ✅ Automated release pipeline
- ✅ Comprehensive E2E tests
- ✅ Complete documentation

**After Q1 Improvements** (Months 2-3):

- ✅ 85%+ test coverage
- ✅ Optimized performance
- ✅ Full observability
- ✅ Clean architecture

**After Q2-Q3 Features** (Months 4-9):

- ✅ Multi-user support
- ✅ External integrations
- ✅ Advanced features
- ✅ Enterprise-ready

### Success Metrics

**Quality**:

- Test coverage: 80%+ (current: 77.9%)
- Code duplication: <5% (measure with gocyclo)
- Bug count: <10 open critical/high bugs
- Documentation coverage: 100% of public APIs

**Performance**:

- Scan time: <1s per 100 files
- API response time: <100ms p95
- Memory usage: <500MB for 10k books
- UI load time: <2s initial load

**Maintainability**:

- Cyclomatic complexity: <15 per function
- Package size: <2000 LOC per package
- Test-to-code ratio: >1:1
- Documentation-to-code ratio: >0.2:1

### Next Steps

1. **Review this roadmap** with team/stakeholders
2. **Prioritize recommendations** based on business needs
3. **Create tickets** for Phase 1 critical fixes
4. **Schedule work** for MVP completion
5. **Set up tracking** for success metrics
6. **Begin implementation** with database test fixes

**Estimated Time to Green**: 4-5 hours focused work
**Estimated Time to MVP**: 16-26 hours focused work
**Estimated Time to Production**: 4-6 weeks

---

## Appendix

### A. Tool Installation Guide

```bash
# Mockery
go install github.com/vektra/mockery/v2@latest

# golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# gosec
go install github.com/securego/gosec/v2/cmd/gosec@latest

# gocyclo
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

# goimports
go install golang.org/x/tools/cmd/goimports@latest

# Verify installations
mockery --version
golangci-lint --version
gosec --version
gocyclo --version
goimports --version
```

### B. Useful Make Targets

```makefile
.PHONY: all
all: mocks lint test

.PHONY: mocks
mocks:
 @mockery --all

.PHONY: lint
lint:
 @golangci-lint run ./...
 @gosec ./...

.PHONY: test
test:
 @go test ./... -v -cover -race

.PHONY: coverage
coverage:
 @go test ./... -coverprofile=coverage.out
 @go tool cover -html=coverage.out -o coverage.html
 @echo "Coverage report: coverage.html"

.PHONY: bench
bench:
 @go test -bench=. -benchmem ./...

.PHONY: clean
clean:
 @rm -f coverage.out coverage.html
 @rm -rf internal/database/mocks/

.PHONY: ci
ci: mocks lint test coverage
```

### C. CI/CD Checklist

**Pre-commit**:

- [ ] Run `gofmt`
- [ ] Run `goimports`
- [ ] Run tests
- [ ] Check coverage

**Pull Request**:

- [ ] All tests pass
- [ ] Coverage ≥ 80%
- [ ] Linting passes
- [ ] Security scan passes
- [ ] Documentation updated

**Release**:

- [ ] All PR checks pass
- [ ] E2E tests pass
- [ ] Manual QA completed
- [ ] CHANGELOG updated
- [ ] Version bumped

### D. Contact & Resources

**Documentation**:

- Main README: `/README.md`
- Technical Design: `/docs/technical_design.md`
- MVP Specification: `/docs/mvp-specification.md`

**Key Files**:

- TODO List: `/TODO.md`
- Changelog: `/CHANGELOG.md`
- Mockery Config: `/.mockery.yaml`

**Issue Tracking**:

- GitHub Issues: `https://github.com/jdfalk/audiobook-organizer/issues`

---

*Document Version: 1.0.0*
*Last Updated: 2026-01-22*
*Next Review: After Phase 1 completion*
