# Phase 2 Handler Refactoring - Status & Next Steps

> **For Claude:** This document tracks Phase 2 completion status and defines the next handler integration phase.

**Date Started:** 2026-02-03
**Date Phase 2 Services Completed:** 2026-02-03
**Status:** ✅ Service Creation Complete | ⏳ Handler Integration Pending

---

## Executive Summary

**Phase 2 is 50% complete.** All 4 priority services have been successfully extracted, fully tested, and committed. The next phase requires integrating these services into the handlers to complete the thin adapter pattern refactoring.

### Phase 2 Services - COMPLETED ✅

| Service | Lines | Tests | Status |
|---------|-------|-------|--------|
| AudiobookUpdateService | 133 | 8 | ✅ Complete |
| ImportPathService | 62 | 4 | ✅ Complete |
| ConfigUpdateService | 96 | 5 | ✅ Complete |
| SystemService | 108 | 4 | ✅ Complete |
| **Total** | **399** | **21** | **✅ All Passing** |

### Handler Integration - PENDING ⏳

| Handler | Current Lines | Target Lines | Service | Status |
|---------|---------------|--------------|---------|--------|
| updateAudiobook | 141 | 15 | AudiobookUpdateService | ⏳ Pending |
| addImportPath | 152 | ~50 | ImportPathService | ⏳ Pending |
| updateConfig | 167 | 22 | ConfigUpdateService | ⏳ Pending |
| getSystemStatus | 94 | 10 | SystemService | ⏳ Pending |
| getSystemLogs | 111 | 25 | SystemService | ⏳ Pending |

---

## Phase 2A: Service Creation - COMPLETED ✅

### What Was Done

Four service classes were created to extract business logic from handlers:

#### 1. AudiobookUpdateService
**File:** `internal/server/audiobook_update_service.go`
**Purpose:** Extract JSON parsing and field transformation logic from updateAudiobook handler
**Key Methods:**
- `ValidateRequest(id string, payload map[string]any)` - Validates ID and payload
- `ExtractStringField(payload map[string]any, key string)` - Safely extracts strings
- `ExtractIntField(payload map[string]any, key string)` - Safely extracts integers (handles JSON float64)
- `ExtractBoolField(payload map[string]any, key string)` - Safely extracts booleans
- `ApplyUpdatesToBook(book *database.Book, updates map[string]any)` - Applies field updates
- `UpdateAudiobook(id string, payload map[string]any)` - Main orchestration method

**Test Coverage:** 8 unit tests covering validation, field extraction, and book updates

**Book Fields Handled:**
- title, author_id, series_id, narrator, publisher, language, audiobook_release_year, isbn10, isbn13

#### 2. ImportPathService
**File:** `internal/server/import_path_service.go`
**Purpose:** Extract path validation and creation logic from addImportPath handler
**Key Methods:**
- `ValidatePath(path string)` - Validates path is not empty
- `CreateImportPath(path, name string)` - Creates import path in database
- `UpdateImportPathEnabled(id int, enabled bool)` - Updates enabled status
- `GetImportPath(id int)` - Retrieves import path by ID

**Test Coverage:** 4 unit tests covering path validation and creation

**Database Interface Used:**
- `CreateImportPath(path, name string)` - Takes path and name strings
- `GetImportPathByID(id int)` - Takes integer ID
- `UpdateImportPath(id int, importPath *ImportPath)` - Takes integer ID

#### 3. ConfigUpdateService
**File:** `internal/server/config_update_service.go`
**Purpose:** Extract config field mapping and validation logic from updateConfig handler
**Key Methods:**
- `ValidateUpdate(payload map[string]any)` - Validates payload is not empty
- `ExtractStringField(payload map[string]any, key string)` - Safely extracts strings
- `ExtractBoolField(payload map[string]any, key string)` - Safely extracts booleans
- `ExtractIntField(payload map[string]any, key string)` - Safely extracts integers
- `ApplyUpdates(payload map[string]any)` - Applies all updates to AppConfig
- `MaskSecrets(cfg *config.Config)` - Returns safe config for response (removes sensitive fields)

**Test Coverage:** 5 unit tests covering field extraction and config application

**Config Fields Handled:**
- root_dir, auto_organize, concurrent_scans, exclude_patterns, supported_extensions

#### 4. SystemService
**File:** `internal/server/system_service.go`
**Purpose:** Extract status aggregation and log filtering logic from getSystemStatus and getSystemLogs handlers
**Key Methods:**
- `CollectSystemStatus()` - Gathers system status information
- `FilterLogsBySearch(logs []database.OperationLog, searchTerm string)` - Case-insensitive search
- `SortLogsByTimestamp(logs []database.OperationLog)` - Sorts logs by CreatedAt descending
- `PaginateLogs(logs []database.OperationLog, page, pageSize int)` - Returns page of logs
- `GetFormattedUptime(startTime time.Time)` - Formats uptime duration

**Test Coverage:** 4 unit tests covering status collection, filtering, and pagination

**Log Type:** Uses `database.OperationLog` (ID, OperationID, Level, Message, Details, CreatedAt)

### Commits Made

```
84830e7 fix(audiobook_update_service): update to use any instead of interface{}
25627a8 feat: create remaining Phase 2 services (ImportPath, ConfigUpdate, System)
```

### Test Results

**All tests passing:** ✅ 21 new tests, 100% pass rate
**Build status:** ✅ Clean, no errors or warnings
**Code coverage:** 64.5% (maintained from Phase 1)
**Go 1.25 compliance:** ✅ All services use `any` keyword

---

## Phase 2B: Handler Integration - NEXT STEPS ⏳

### Overview

This phase involves integrating the 4 newly created services into the 5 high-priority handlers. Each handler will be refactored from a complex business logic handler (40-167 lines) to a thin HTTP adapter (10-25 lines) that simply:

1. Parses the HTTP request
2. Calls the appropriate service method
3. Formats and returns the response

### Prerequisites for Next Phase

Before starting handler integration, ensure:

- ✅ All 4 services are created and tested (DONE)
- ✅ All services are committed (DONE)
- ✅ All tests pass (DONE)
- ✅ Git is clean (verify with `git status`)
- ✅ Full build succeeds (run `make build-api`)

### Task Breakdown

The handler integration consists of 5 refactoring tasks:

#### Task 2B.1: Refactor updateAudiobook Handler

**Current State:**
- File: `internal/server/server.go` lines 1097-1237
- Lines of code: 141
- Lines of business logic: ~110

**Changes:**
1. Add `audiobookUpdateService *AudiobookUpdateService` field to Server struct (around line 468)
2. Initialize service in `NewServer()`: `audiobookUpdateService: NewAudiobookUpdateService(database.GlobalStore),`
3. Replace entire updateAudiobook handler (lines 1097-1237) with thin adapter:

```go
func (s *Server) updateAudiobook(c *gin.Context) {
	id := c.Param("id")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := s.audiobookUpdateService.UpdateAudiobook(id, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}
```

**Result:** Reduce from 141 lines to 15 lines

**Testing:** Run `go test ./internal/server -run "TestUpdateAudiobook" -v` to verify

#### Task 2B.2: Refactor updateConfig Handler

**Current State:**
- File: `internal/server/server.go` lines 2019-2185
- Lines of code: 167
- Lines of business logic: ~130

**Changes:**
1. Add `configUpdateService *ConfigUpdateService` field to Server struct
2. Initialize service in `NewServer()`: `configUpdateService: NewConfigUpdateService(),`
3. Replace entire updateConfig handler with thin adapter:

```go
func (s *Server) updateConfig(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.configUpdateService.ValidateUpdate(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.configUpdateService.ApplyUpdates(payload)

	if err := config.SaveConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	maskedConfig := s.configUpdateService.MaskSecrets(&config.AppConfig)
	c.JSON(http.StatusOK, maskedConfig)
}
```

**Result:** Reduce from 167 lines to 22 lines

**Testing:** Run `go test ./internal/server -run "TestUpdateConfig" -v` to verify

#### Task 2B.3: Refactor getSystemStatus and getSystemLogs Handlers

**Current State:**
- getSystemStatus: `internal/server/server.go` lines 1800-1893 (94 lines, 75 logic)
- getSystemLogs: `internal/server/server.go` lines 1895-2005 (111 lines, 95 logic)

**Changes for getSystemStatus:**
1. Add `systemService *SystemService` field to Server struct
2. Initialize service in `NewServer()`: `systemService: NewSystemService(database.GlobalStore),`
3. Replace getSystemStatus handler:

```go
func (s *Server) getSystemStatus(c *gin.Context) {
	status, err := s.systemService.CollectSystemStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}
```

**Result:** Reduce from 94 to 10 lines

**Changes for getSystemLogs:**
1. Replace getSystemLogs handler with thin adapter that:
   - Parses query parameters (search, page, page_size)
   - Retrieves all operation logs from database
   - Calls service methods to filter, sort, and paginate
   - Returns formatted response

```go
func (s *Server) getSystemLogs(c *gin.Context) {
	searchQuery := c.Query("search")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page := 1
	pageSize := 20

	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
		pageSize = ps
	}

	// Retrieve all operation logs
	allOperations, err := database.GlobalStore.GetAllOperations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var allLogs []database.OperationLog
	for _, op := range allOperations {
		logs, err := database.GlobalStore.GetOperationLogs(op.ID)
		if err == nil && logs != nil {
			allLogs = append(allLogs, logs...)
		}
	}

	// Use service methods to filter, sort, and paginate
	filteredLogs := s.systemService.FilterLogsBySearch(allLogs, searchQuery)
	sortedLogs := s.systemService.SortLogsByTimestamp(filteredLogs)
	paginatedLogs := s.systemService.PaginateLogs(sortedLogs, page, pageSize)

	c.JSON(http.StatusOK, gin.H{
		"logs":  paginatedLogs,
		"page":  page,
		"size":  pageSize,
		"total": len(filteredLogs),
	})
}
```

**Result:** Reduce from 111 to 25 lines

**Testing:** Run `go test ./internal/server -run "TestGetSystemStatus|TestGetSystemLogs" -v` to verify

#### Task 2B.4: Partial Refactor of addImportPath Handler

**Current State:**
- File: `internal/server/server.go` lines 1449-1600
- Lines of code: 152
- Lines of business logic: ~130

**Changes:**
1. Add `importPathService *ImportPathService` field to Server struct
2. Initialize service in `NewServer()`: `importPathService: NewImportPathService(database.GlobalStore),`
3. Replace path creation section of handler to use service:

```go
// In addImportPath handler, replace path creation with:
createdPath, err := s.importPathService.CreateImportPath(newPath.Path, newPath.Name)
if err != nil {
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	return
}
```

**Note:** The auto-scan orchestration logic (lines 1477-1597) will remain in the handler for now. This can be extracted into a separate `AutoScanOrchestrator` service in Phase 3.

**Result:** Reduce path creation logic, refactor auto-scan orchestration later

**Testing:** Run `go test ./internal/server -run "TestAddImportPath" -v` to verify

### Implementation Instructions

For each task (2B.1 - 2B.4):

1. **Update Server struct** - Add service field around line 468
2. **Initialize in NewServer()** - Add service initialization around line 490
3. **Refactor handler** - Replace handler implementation with thin adapter
4. **Run tests** - `go test ./internal/server -v` to verify nothing broke
5. **Commit** - Create atomic commit with message like: `refactor(updateAudiobook): thin HTTP adapter using service layer`

### Expected Outcomes

After Phase 2B completion:

- **Handler lines reduced:** ~630 lines → ~130 lines (79% reduction)
- **Business logic:** All moved to testable services
- **Test coverage:** Should improve to 70%+ (more logic in testable services)
- **Code quality:** Handlers follow thin adapter pattern consistently
- **Thin adapters:** All 67 handlers will be <30 lines (uniform pattern)

### Git Workflow

```bash
# For each handler refactoring task:
1. git checkout main  # Start from clean state
2. # Make handler changes
3. go test ./internal/server -v  # Verify tests pass
4. make build-api  # Verify clean build
5. git add internal/server/server.go
6. git commit -m "refactor(handlerName): thin HTTP adapter using service layer"
7. git push origin main
```

### Quality Checklist

Before considering Phase 2B complete:

- ✅ All 5 handlers refactored to thin adapters
- ✅ All handler tests pass
- ✅ Full build succeeds (`make build-api`)
- ✅ Full test suite passes (`make test`)
- ✅ Test coverage improved to 70%+ for server package
- ✅ Git log shows atomic commits for each handler
- ✅ No business logic remains in any handler
- ✅ All services properly initialized in NewServer()

---

## Phase 3: Remaining Handler Refactoring - FUTURE

After Phase 2 completes, Phase 3 would address:

- Remaining 62 handlers with moderate/light business logic
- Auto-scan orchestration extraction (separate service)
- Health check metric gathering refactoring
- Additional service layer refinements

---

## Reference Documents

- **Phase 2 Service Creation Plan:** `docs/plans/2026-02-03-phase2-handler-refactoring.md` (Tasks 1-4)
- **Phase 1 Completion:** Completed with ScanService, OrganizeService, MetadataFetchService
- **Complete Service Layer Refactoring Plan:** `docs/plans/2026-02-03-complete-service-layer-refactoring.md` (7 total services)

---

## Handoff Checklist

For passing to another AI instance:

- ✅ Phase 2A services created and committed
- ✅ All 21 service unit tests passing
- ✅ Clear documentation of what was done
- ✅ Exact step-by-step handler refactoring instructions (Phase 2B)
- ✅ Code examples for each handler refactoring
- ✅ Database method signatures documented
- ✅ Testing commands provided
- ✅ Git workflow documented
- ✅ Quality checklist provided

**Ready for handoff:** YES ✅

To the next AI instance: Start with Task 2B.1 (refactor updateAudiobook handler) and follow the instructions in the "Phase 2B: Handler Integration" section above. All services are ready and tested.
