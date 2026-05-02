<!-- file: docs/CODE_DUPLICATION_ANALYSIS.md -->
<!-- version: 1.0.0 -->

# Code Duplication Analysis & Refactoring Recommendations

## Overview

This document identifies patterns of code duplication across the service layer and handlers, with recommendations for consolidation. The analysis was performed on the Phase 3 service layer implementations.

## Identified Duplication Patterns

### 1. Empty/Nil List Handling (HIGH FREQUENCY)

**Current Pattern:**
```go
// In AuthorSeriesService
if authors == nil {
    authors = []database.Author{}
}
return &AuthorListResponse{
    Items: authors,
    Count: len(authors),
}, nil

// In WorkService (same pattern)
if works == nil {
    works = []database.Work{}
}
return &WorkListResponse{
    Items: works,
    Count: len(works),
}, nil
```

**Recommendation:** Create a shared helper function in `internal/server/helpers.go`:
```go
func EnsureNonNilSlice[T any](slice []T) []T {
    if slice == nil {
        return []T{}
    }
    return slice
}
```

**Impact:** Appears in 6+ service methods. Consolidation saves ~30 lines of code.

---

### 2. Required Field Validation

**Current Pattern:**
```go
// WorkService.CreateWork
if strings.TrimSpace(work.Title) == "" {
    return nil, fmt.Errorf("title is required")
}

// ImportService (implied validation)
// Validators.ValidateTitle() (new)
```

**Issue:** Validation logic scattered across:
- Service layer (business logic)
- Handler layer (input validation via validators)
- Handlers (inline checks)

**Recommendation:** Implement layered validation approach:
1. **Handler layer** (validators package): Basic input validation
2. **Service layer**: Business logic validation only
3. Avoid duplicate checks

**Example integration:**
```go
// In handler
if err := ValidateTitle(work.Title, 1, 500); err != nil {
    RespondWithValidationError(c, "title", err.Error())
    return
}

// In service - only business logic checks
if !isValidForUpdate(work) {
    return nil, fmt.Errorf("cannot update archived work")
}
```

**Impact:** Reduces validation duplication in 8+ handlers and services.

---

### 3. Error Response Formatting

**Current Pattern:**
```go
// Repeated across 35+ handlers
if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
}

if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}
```

**Status:** RESOLVED ✓

The error_handler.go module consolidates this into:
```go
RespondWithBadRequest(c, message)
RespondWithInternalError(c, message)
```

---

### 4. Query Parameter Parsing

**Current Pattern:**
```go
// In listAudiobooks
limitStr := c.DefaultQuery("limit", "50")
offsetStr := c.DefaultQuery("offset", "0")
limit, _ := strconv.Atoi(limitStr)
offset, _ := strconv.Atoi(offsetStr)

// In listSoftDeletedAudiobooks (same pattern)
limitStr := c.DefaultQuery("limit", "50")
offsetStr := c.DefaultQuery("offset", "0")
limit, _ := strconv.Atoi(limitStr)
offset, _ := strconv.Atoi(offsetStr)
```

**Status:** RESOLVED ✓

The error_handler.go module provides:
```go
ParseQueryInt(c, "limit", 50)
ParseQueryInt(c, "offset", 0)
ParseQueryIntPtr(c, "author_id")
ParseQueryBool(c, "delete_files", false)
```

---

### 5. Database Error Handling Pattern

**Current Pattern:**
```go
// Inconsistent nil vs error handling
work, err := ws.db.GetWorkByID(id)
if err != nil {
    return nil, err
}
if work == nil {
    return nil, fmt.Errorf("work not found")
}

// VS. sometimes just one check
book, err := bs.db.GetBookByID(id)
if err != nil {
    return errors.Wrap(err, "failed to get book")
}
```

**Issue:** Database methods sometimes return:
- (obj, error) - error on DB failure
- (obj, error) - nil obj when not found
- (nil, nil) - when not found

**Recommendation:** Standardize database interface to always return error for not-found:

```go
// Current
GetWorkByID(id string) (*Work, error) // Returns (*nil, nil) when not found

// Proposed
GetWorkByID(id string) (*Work, error) // Returns (nil, ErrNotFound) when not found
```

**Alternative:** Implement helper in service layer:
```go
func (ws *WorkService) GetWorkOrError(id string) (*database.Work, error) {
    work, err := ws.db.GetWorkByID(id)
    if err != nil {
        return nil, err
    }
    if work == nil {
        return nil, fmt.Errorf("work not found")
    }
    return work, nil
}
```

**Impact:** Eliminates 4+ nil checks across services.

---

### 6. Response Type Wrapping

**Current Pattern:**
```go
c.JSON(http.StatusOK, gin.H{
    "items":  items,
    "count":  count,
    "limit":  limit,
    "offset": offset,
})

// vs. another handler
c.JSON(http.StatusOK, gin.H{
    "data":   item,
})

// vs. yet another
c.JSON(http.StatusOK, item)
```

**Status:** RESOLVED ✓

The response_types.go module provides:
```go
NewListResponse(items, count, limit, offset)
NewItemResponse(data)
type AudiobookResponse struct { ... }
```

---

### 7. Service Constructor Duplication

**Current Pattern:**
```go
type BatchService struct {
    db database.Store
}

func NewBatchService(db database.Store) *BatchService {
    return &BatchService{db: db}
}

// Repeated 7+ times across all services
type WorkService struct {
    db database.Store
}

func NewWorkService(db database.Store) *WorkService {
    return &WorkService{db: db}
}
```

**Recommendation:** Create a base service struct:

```go
// In internal/server/service_base.go
type BaseService struct {
    db database.Store
}

// Then services can embed it
type WorkService struct {
    *BaseService
}

func NewWorkService(db database.Store) *WorkService {
    return &WorkService{
        BaseService: &BaseService{db: db},
    }
}
```

**Impact:** Saves ~15 lines per service × 7 services = 105 lines.

---

### 8. List Response Pattern

**Current Pattern:**
```go
// In multiple services
return &ListResponse{
    Items: items,
    Count: len(items),
}, nil

// But inconsistent - sometimes no count
return items, nil
```

**Status:** RESOLVED ✓

The response_types.go provides consistent `ListResponse` with factory functions.

---

### 9. Logging Pattern

**Current Pattern:**
Mostly missing - handlers don't log operations systematically.

**Status:** RESOLVED ✓

The logger.go module provides:
- `OperationLogger` - tracks handler lifecycle
- `ServiceLogger` - service operations
- `RequestLogger` - HTTP request tracking
- Specialized loggers for DB operations, cache hits, etc.

---

## Summary of Changes Made

| Pattern | Status | Module | Impact |
|---------|--------|--------|--------|
| Error response formatting | ✓ | error_handler.go | 35+ handlers simplified |
| Query parameter parsing | ✓ | error_handler.go | 8+ handlers simplified |
| Response type wrapping | ✓ | response_types.go | Improved type safety |
| Logging | ✓ | logger.go | Full observability |
| Validation | ✓ | validators.go | Consolidated validators |

## Remaining Opportunities

### High Priority

1. **Empty list handling** - Consolidate nil checks
   - Effort: 1 hour
   - Impact: ~30 lines saved
   - Files: 6+ services

2. **Database error consistency** - Standardize not-found handling
   - Effort: 2-3 hours (requires DB interface changes)
   - Impact: ~20 lines saved, better consistency
   - Files: All services

3. **Service constructor base** - Extract common pattern
   - Effort: 30 minutes
   - Impact: ~105 lines saved
   - Files: 7 services

### Medium Priority

4. **Validation layering** - Separate input vs business logic
   - Effort: 2 hours
   - Impact: Better separation of concerns
   - Files: 8+ handlers, 6+ services

5. **Response factory pattern** - Standardize response creation
   - Effort: 1 hour
   - Impact: ~40 lines saved
   - Files: All handlers

### Low Priority

6. **Metadata extraction** - DRY up tag/metadata updates
   - Effort: 2-3 hours
   - Impact: ~80 lines saved
   - Files: audiobook_service.go, import_service.go

## Recommended Implementation Order

1. **Week 1:** Empty list handling (quick win)
2. **Week 1:** Response factory consolidation (medium complexity)
3. **Week 2:** Validation layering (higher impact)
4. **Week 2:** Database error consistency (more complex)
5. **Week 3:** Service constructor base (refactoring)

## Testing Impact

- Each consolidation should have comprehensive tests
- Current test coverage: ~70% across services
- Target post-consolidation: 85%+

## Code Metrics

Current codebase (Phase 3 complete):
- Server package: ~3,500 lines
- Service files: ~1,200 lines
- Handler files: ~2,000 lines (excluding test methods)
- Duplication ratio: ~15% (estimated)

Post-consolidation target:
- Server package: ~3,000 lines (14% reduction)
- Duplication ratio: ~5%

## Conclusion

The Phase 3 service layer refactoring has successfully extracted business logic into services. Additional consolidation opportunities exist with high ROI, particularly:
- Empty list handling (quick)
- Validation layering (higher impact)
- Database error consistency (better DX)

These follow the principle of progressive consolidation - optimize after achieving correct structure.
