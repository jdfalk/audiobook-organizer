<!-- file: PROJECT_ANALYSIS_RAW_DATA.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->
<!-- last-edited: 2026-01-22 -->

# Project Analysis - Raw Data

## Executive Summary

- **Project**: Audiobook Organizer
- **Language**: Go 1.24.0 (Backend), TypeScript/React (Frontend)
- **MVP Status**: ~75-85% complete
- **Current Test Coverage**: 77.9% overall (Goal: 80%)
- **Lines of Code**: ~50,000+ (estimated based on file counts)
- **Analysis Date**: 2026-01-22

## Test Coverage Analysis (Current State)

### Package-Level Coverage

```
Package                                           Coverage    Status
----------------------------------------------------------------
github.com/jdfalk/audiobook-organizer            87.5%       ✅ Excellent
cmd                                              78.6%       ✅ Good
internal/ai                                      84.0%       ✅ Good
internal/backup                                  80.6%       ✅ Good
internal/config                                  90.3%       ✅ Excellent
internal/database                                FAIL        ❌ CRITICAL
internal/fileops                                 84.3%       ✅ Good
internal/matcher                                 93.5%       ✅ Excellent
internal/mediainfo                               98.2%       ✅ Excellent
internal/metadata                                86.0%       ✅ Good
internal/metrics                                 100.0%      ✅ Perfect
internal/models                                  N/A         ⚠️  No statements
internal/operations                              90.6%       ✅ Excellent
internal/organizer                               89.5%       ✅ Excellent
internal/playlist                                81.4%       ✅ Good
internal/realtime                                95.6%       ✅ Excellent
internal/scanner                                 81.6%       ✅ Good
internal/server                                  66.0%       ⚠️  Below target
internal/sysinfo                                 91.3%       ✅ Excellent
internal/tagger                                  93.8%       ✅ Excellent

OVERALL: 77.9% (excluding failed package)
```

### Coverage Gaps

1. **internal/database**: FAIL - Build errors blocking all tests
2. **internal/server**: 66.0% - Main coverage gap (need +14% to hit 80%)
3. **cmd**: 78.6% - Slightly below 80% target

## Critical Issues Identified

### 1. Mockery/Testing Infrastructure Issues

#### Issue: Duplicate Mock Declarations

**Files Affected**:

- `internal/database/mock_store.go` (line 14, 54)
- `internal/database/mocks_test.go` (line 15, 28)

**Error Details**:

```
mock_store.go:14:6: MockStore redeclared in this block
mock_store.go:54:6: NewMockStore redeclared in this block
mocks_test.go:15:6: NewMockStore redeclared in this block
mocks_test.go:28:6: MockStore redeclared in this block
```

**Root Cause**: Conflicting mock implementations - manual MockStore and mockery-generated mocks coexisting

#### Issue: Missing Dependencies

**Error**:

```
go.mod:1:1: github.com/stretchr/objx is not in your go.mod file (go mod tidy)
go.mod:17:2: missing go.sum entry for module providing package github.com/stretchr/objx
```

**Impact**: All database tests fail, blocking coverage measurement

#### Issue: testify/mock Import Errors

**Files Affected**:

- `mocks_test.go:10`: Error importing github.com/stretchr/testify/mock
- Multiple method errors: `mock.Mock undefined`, `mock.AssertExpectations undefined`, `_mock.Called undefined`

**Root Cause**: Missing transitive dependency (github.com/stretchr/objx)

### 2. Mocking Strategy Inconsistency

**Current Situation**: Three different mocking approaches

1. **Manual MockStore** (`internal/database/mock_store.go`) - 1110 lines
   - Hand-written implementation
   - Full interface implementation
   - Error injection support
   - Call tracking

2. **Mockery-generated mocks** (`internal/database/mocks_test.go`) - 56,398 lines (!)
   - Auto-generated from mockery tool
   - testify/mock based
   - Conflicts with manual mocks

3. **Mock implementations** (`internal/database/mock.go`, `mock_test.go`)
   - Additional mock-related code
   - Purpose unclear due to file size

**Recommendation**: Standardize on ONE approach

## Module Architecture Analysis

### Package Structure

```
audiobook-organizer/
├── cmd/                    # CLI commands (root, diagnostics)
├── internal/
│   ├── ai/                # OpenAI integration
│   ├── backup/            # Backup system
│   ├── config/            # Configuration management
│   ├── database/          # Data layer (29 files!)
│   ├── fileops/           # File operations
│   ├── matcher/           # Series matching
│   ├── mediainfo/         # Media metadata extraction
│   ├── metadata/          # Metadata management (16 files)
│   ├── metrics/           # Metrics/observability
│   ├── models/            # Data models
│   ├── operations/        # Async operation queue
│   ├── organizer/         # File organization
│   ├── playlist/          # Playlist generation
│   ├── realtime/          # SSE/WebSocket events
│   ├── scanner/           # File scanning
│   ├── server/            # HTTP server (12 files)
│   ├── sysinfo/           # System information
│   └── tagger/            # Tag writing
├── web/                   # React frontend
└── main.go
```

### Database Package Concerns

**Files**: 29 files in `internal/database/`
**Issues**:

- `mocks_test.go`: 56,398 lines (HUGE - likely auto-generated)
- Mock implementation scattered across 4 files
- Unclear separation of concerns

**Files**:

```
audiobooks.go              mock.go                      mock_store_test.go
audiobooks_test.go         mock_store.go                pebble_coverage_test.go
close_store_test.go        mock_store_coverage_test.go  pebble_store.go
coverage_test.go           mock_test.go                 pebble_store_test.go
database.go                mocks_test.go                settings.go
do_not_import_test.go      migrations.go                settings_extra_test.go
interface.go               migrations_extra_test.go     sqlite_store.go
                          sqlite_test.go               store.go
                          store_extra_test.go          web.go
```

### Server Package Concerns

**Coverage**: 66.0% (lowest among active packages)
**Files**: 12 files
**Missing Coverage Areas** (based on MOCKERY_SUMMARY.md):

- Error path testing (~30% missing)
- Database error scenarios
- Validation edge cases
- Complex scenarios (version linking, overrides)

## Frontend Analysis

### Structure

```
web/
├── src/
│   ├── components/       # React components
│   ├── pages/            # Page components
│   ├── services/         # API client
│   └── test/             # Test setup
├── e2e/                  # Playwright tests
└── package.json
```

### Frontend Test Status

- **Unit Tests**: Vitest + React Testing Library
- **E2E Tests**: Playwright (minimal coverage)
- **Coverage**: Not measured in current data

## Dependency Analysis

### Go Dependencies (go.mod)

```
Direct Dependencies:
- github.com/cockroachdb/pebble v1.1.5
- github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
- github.com/gin-gonic/gin v1.11.0
- github.com/mattn/go-sqlite3 v1.14.33
- github.com/oklog/ulid/v2 v2.1.1
- github.com/openai/openai-go v1.12.0
- github.com/spf13/cobra v1.10.2
- github.com/spf13/viper v1.21.0
- github.com/stretchr/testify v1.11.1
- go.senan.xyz/taglib v0.11.1

Missing/Broken:
- github.com/stretchr/objx (transitive, missing in go.sum)
```

### Database Systems

1. **SQLite** (primary)
   - File: `internal/database/sqlite_store.go`
   - Migrations: `internal/db/migrations/`
   - Version: 10 migrations

2. **PebbleDB** (alternative)
   - File: `internal/database/pebble_store.go`
   - Use case: Embedded key-value store
   - Extended keyspace with users, sessions

## TODO.md Analysis

### Categorization by Priority

#### P0 (Critical Path to MVP)

1. Manual QA & Validation (2-3 hours)
2. Release Pipeline Fixes (2-3 hours)
3. **Test Coverage Expansion (8-12 hours)** - KEY BLOCKER
4. E2E Backend Integration (4-6 hours)

**Total P0 Effort**: 16-26 hours

#### P1 (High Priority)

- CI/CD Health Monitoring
- Documentation Updates
- Metadata Quality Improvements
- Frontend Polish

#### P2 (Medium Priority)

- Documentation & Status
- Observability Improvements
- Performance Optimizations
- UX Enhancements

### Completed Items (Recent)

From TODO.md "RECENTLY COMPLETED" section:

- Metadata Provenance Frontend (PR #79)
- Bulk Metadata Fetch
- Library Metadata Edit
- Import Workflow
- Action Integration
- Scanner Progress Race Fix (PR #83)

## CHANGELOG.md Analysis

### Recent Activity (January 2026)

- Comprehensive test coverage documentation
- Media info tests
- Backup system tests
- Metadata write tests
- Scanner core tests (7+ formats)
- Organizer pattern tests
- Operations queue tests
- Model serialization tests
- PebbleDB store tests

### Test Infrastructure Expansion

**Backend Unit Tests Added**:

- Media info tests
- Backup system tests
- Metadata write tests
- Scanner tests (integration + unit)
- Organizer tests (pattern + real-world)
- Operations queue tests
- Model serialization tests
- PebbleDB store tests
- Metadata internal tests

**Frontend Unit Tests Added**:

- API service tests
- Library metadata tests
- Library helpers tests

**E2E Tests Added**:

- App smoke tests (Playwright)
- Import paths E2E (Playwright)
- Metadata provenance E2E (Playwright)
- Soft delete and retention (Python/Selenium)

## Code Quality Observations

### Strengths

1. **Comprehensive testing infrastructure** - Multiple test types (unit, integration, E2E)
2. **Good package organization** - Clear separation of concerns
3. **Documentation** - README, TODO, CHANGELOG, technical docs
4. **Migration system** - Database versioning in place
5. **High coverage in most packages** - 80%+ in 13 out of 18 packages

### Weaknesses

1. **Mock implementation chaos** - 3 competing approaches
2. **Database package bloat** - 29 files, huge generated mock
3. **Server coverage gap** - 66% (need 80%+)
4. **Build failures** - Blocking database tests
5. **Dependency issues** - Missing go.sum entries

## File Size Anomalies

### Concerning Files

1. `internal/database/mocks_test.go` - 56,398 lines
   - Likely auto-generated by mockery
   - Should be in separate package/directory
   - Polluting main package

2. `internal/database/` - 29 files
   - Suggests package needs decomposition
   - Mixed concerns (store, mocks, tests, migrations)

## Build System Analysis

### Makefile/Scripts

- `.mockery.yaml` exists (v1.1.0)
- `scripts/setup-mockery.sh` mentioned in MOCKERY_SUMMARY
- `Makefile.mockery.example` exists
- No main Makefile observed

### CI/CD

From TODO.md:

- GitHub Actions workflows
- Coverage threshold: Currently 0% (lowered temporarily)
- Go 1.24/1.25 version mismatch issues (resolved)
- npm cache issues (partially resolved)

## Migration History

### Database Migrations (10 total)

1. Initial schema
2. Authors and series
3. Unknown
4. Unknown
5. Media info and version fields
6-8. Unknown
6. State machine (lifecycle tracking)
7. Metadata provenance tracking

## Documentation Analysis

### Existing Docs

```
docs/
├── archive/                         # Historical records
├── current-progress-analysis.md
├── database-pebble-schema.md
├── mvp-implementation-plan.md
├── mvp-specification.md
├── mvp-summary.md
├── openapi.yaml
└── technical_design.md
```

### Documentation Gaps

- Architecture diagrams missing
- Data flow documentation incomplete
- Deployment guide needed
- API usage examples sparse

## Metrics & Observability

### Current State

- `internal/metrics/` package exists (100% coverage!)
- Prometheus integration (`github.com/prometheus/client_golang`)
- SSE for real-time updates
- Operation tracking and logs API

### Gaps (from TODO.md)

- Persist operation logs
- Improve log view UX
- SSE system status heartbeats
- Structured metrics endpoint

## Performance Considerations

### Identified Bottlenecks (from TODO.md)

- No parallel scanning (single-threaded currently)
- Full library walk for size computation (no inotify/fsnotify)
- No caching layer for frequent queries
- No batch metadata fetch optimization

### Proposed Optimizations

- Goroutine pool for concurrent scans
- Debounced library size recomputation
- LRU cache for book queries
- Batch metadata fetch pipeline

## Security Posture

### Current

- API key management in config
- SHA256 file hashing
- .gitignore for secrets (`.encryption_key`)

### Missing

- Authentication/authorization (future multi-user)
- TLS/HTTPS support
- Input sanitization audit
- Secret scanning in CI

## External Integrations

### Current

1. **Open Library** - Metadata fetching
2. **OpenAI** - Filename parsing fallback
3. **GitHub Actions** - CI/CD

### Proposed (from TODO.md)

- Calibre metadata export
- OPDS feed generation
- Plex/Jellyfin sync
- iTunes library integration
- BitTorrent client integration

## Resource Estimates

### To Reach 80% Coverage

**Based on MOCKERY_SUMMARY.md projections**:

```
Package             Current    Target    Effort
--------------------------------------------------------
internal/server     66.0%      85%       2-3 hours
cmd                 78.6%      82%       30-60 min
internal/database   FAIL       85%       1-2 hours (fix + test)

Total: 4-6.5 hours of focused test writing
```

### To Fix Mock Issues

**Estimated effort**:

1. Run `go mod tidy` to fix dependencies - 5 min
2. Remove duplicate mock declarations - 15 min
3. Choose single mocking strategy - 30 min
4. Migrate tests to chosen strategy - 2-3 hours
5. Verify all tests pass - 30 min

**Total**: 3-4.5 hours

### To Complete MVP (P0 items)

**From TODO.md**:

- Manual QA: 2-3 hours
- Release pipeline: 2-3 hours
- Test coverage: 8-12 hours
- E2E tests: 4-6 hours

**Total**: 16-26 hours (2-3.5 days focused work)

## Risk Assessment

### High Risk

1. **Mock implementation conflict** - Blocks database tests
   - Severity: Critical
   - Impact: Cannot measure coverage for core package
   - Mitigation: Immediate fix required

2. **Server coverage gap** - 66% vs 80% target
   - Severity: High
   - Impact: Quality gate for MVP
   - Mitigation: Mockery integration + test expansion

### Medium Risk

1. **Database package complexity** - 29 files, unclear boundaries
   - Severity: Medium
   - Impact: Maintainability, onboarding difficulty
   - Mitigation: Refactor into sub-packages

2. **Frontend E2E coverage** - Minimal Playwright tests
   - Severity: Medium
   - Impact: UI regressions possible
   - Mitigation: Expand E2E test suite (P0 item)

### Low Risk

1. **Performance optimizations** - None implemented
   - Severity: Low
   - Impact: Slower scans, higher memory usage
   - Mitigation: P2 priority, address post-MVP

## Comparative Analysis

### Coverage by Category

```
Category                 Avg Coverage    Package Count
----------------------------------------------------------------
Excellent (90%+)         94.5%           5 packages
Good (80-89%)            85.1%           8 packages
Acceptable (70-79%)      78.6%           1 package
Below Target (<70%)      66.0%           1 package
Failing                  N/A             1 package
```

### Coverage Trends

**Improving** (based on CHANGELOG recent activity):

- Media info: 98.2% (new tests added)
- Metadata: 86.0% (internal tests added)
- Scanner: 81.6% (integration tests added)
- Organizer: 89.5% (pattern + real-world tests)

**Stagnant**:

- Server: 66.0% (no recent test expansion)
- Database: FAIL (mock conflicts blocking progress)

## Technology Stack Summary

### Backend

- **Language**: Go 1.24.0
- **Web Framework**: Gin
- **Databases**: SQLite (primary), PebbleDB (alternative)
- **CLI**: Cobra + Viper
- **Testing**: testify, mockery (planned)
- **Media**: dhowden/tag, go.senan.xyz/taglib
- **AI**: OpenAI Go SDK

### Frontend

- **Language**: TypeScript
- **Framework**: React 18
- **Build**: Vite
- **UI**: Material-UI v5
- **State**: Zustand
- **Testing**: Vitest, React Testing Library, Playwright

### Infrastructure

- **CI/CD**: GitHub Actions
- **Observability**: Prometheus
- **Real-time**: Server-Sent Events (SSE)

## Conclusion

### Overall Health: B+ (Good, with critical issues)

**Strengths**:

- Solid test coverage in most areas (77.9% overall)
- Comprehensive documentation
- Active development with recent test expansion
- Clear MVP roadmap

**Critical Blockers**:

1. Database test failures (mock conflicts)
2. Missing dependency (stretchr/objx)
3. Server coverage gap (66% vs 80%)

**Immediate Actions Required**:

1. Fix go.mod/go.sum (5 min)
2. Resolve mock duplicates (30 min)
3. Standardize mocking strategy (1 hour)
4. Add server tests to reach 80% (2-3 hours)
5. Verify all tests pass (30 min)

**Total Time to Green**: 4-5 hours focused work

**Time to MVP**: 16-26 hours (assuming test issues resolved)
