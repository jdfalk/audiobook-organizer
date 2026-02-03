<!-- file: docs/SERVICE_LAYER_ARCHITECTURE.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d -->
<!-- last-edited: 2026-02-03 -->

# Service Layer Architecture

## Overview

The audiobook-organizer server has been refactored to use a clean service layer architecture. All HTTP handlers are now thin adapters that parse requests and format responses, with all business logic encapsulated in dedicated service classes.

## Architecture Pattern

```
HTTP Request
    ↓
Handler (thin adapter, 5-15 lines)
    ├─ Parse JSON request
    ├─ Call service method
    ├─ Translate errors to HTTP status codes
    └─ Return JSON response
    ↓
Service (business logic)
    ├─ Validate inputs
    ├─ Implement business rules
    ├─ Interact with database
    └─ Return typed responses
    ↓
Database Store (data access)
```

## Services

### 1. AudiobookService (`audiobook_service.go`)

**Responsibilities:** Full CRUD operations for audiobook records, metadata handling, and duplicate detection.

**Key Methods:**
- `GetAudiobooks()` - List audiobooks with filtering (search, author_id, series_id)
- `GetAudiobook(id)` - Retrieve single audiobook with metadata provenance
- `GetAudiobookTags()` - Get metadata tags and media info
- `UpdateAudiobook(id, updates)` - Update metadata with override handling
- `DeleteAudiobook(id, options)` - Hard or soft delete with optional hash blocking
- `GetDuplicateBooks()` - Detect audiobooks with identical content
- `RestoreAudiobook(id)` - Restore soft-deleted audiobooks
- `PurgeSoftDeletedBooks()` - Permanently delete soft-deleted audiobooks

**Database Access:** Uses injected `database.Store` interface

### 2. BatchService (`batch_service.go`)

**Responsibilities:** Batch operations on multiple audiobooks.

**Key Methods:**
- `UpdateAudiobooks(req)` - Apply updates to multiple books in a single operation

**Supported Updates:**
- title
- format
- author_id
- series_id
- series_sequence

**Database Access:** Uses injected `database.Store` interface

### 3. WorkService (`work_service.go`)

**Responsibilities:** Management of work collections (logical groupings of audiobooks).

**Key Methods:**
- `ListWorks()` - Retrieve all works
- `CreateWork(work)` - Create new work with title validation
- `GetWork(id)` - Retrieve single work
- `UpdateWork(id, work)` - Update work with validation
- `DeleteWork(id)` - Remove work

**Validation:** Ensures work titles are not empty

**Database Access:** Uses injected `database.Store` interface

### 4. AuthorSeriesService (`author_series_service.go`)

**Responsibilities:** Listing operations for authors and series.

**Key Methods:**
- `ListAuthors()` - Retrieve all authors, returns empty array if nil
- `ListSeries()` - Retrieve all series, returns empty array if nil

**Database Access:** Uses injected `database.Store` interface

### 5. FilesystemService (`filesystem_service.go`)

**Responsibilities:** Filesystem operations and directory browsing.

**Key Methods:**
- `BrowseDirectory(path)` - List directory contents with metadata
- `CreateExclusion(path)` - Create `.jabexclude` file to exclude directory from scanning
- `RemoveExclusion(path)` - Remove exclusion file

**Security:** Validates paths and prevents directory traversal attacks

**Database Access:** None (file system only)

### 6. ImportService (`import_service.go`)

**Responsibilities:** File import with metadata extraction and database creation.

**Key Methods:**
- `ImportFile(req)` - Import audiobook file with metadata extraction

**Import Process:**
1. Validates file exists and is supported format
2. Extracts metadata from file tags
3. Creates or links to existing author
4. Creates or links to existing series
5. Creates book record in database

**Supported Formats:** Configured in `config.AppConfig.SupportedExtensions`

**Database Access:** Uses injected `database.Store` interface

## Handler Refactoring Examples

### Before (Business Logic in Handler)

```go
func (s *Server) listAudiobooks(c *gin.Context) {
    if database.GlobalStore == nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
        return
    }

    // Business logic embedded in handler
    audiobooks, err := database.GlobalStore.GetAllAudiobooks()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Additional processing...
    if audiobooks == nil {
        audiobooks = []database.Audiobook{}
    }

    c.JSON(http.StatusOK, gin.H{"items": audiobooks, "count": len(audiobooks)})
}
```

### After (Handler as Thin Adapter)

```go
func (s *Server) listAudiobooks(c *gin.Context) {
    resp, err := s.audiobookService.GetAudiobooks(/* filters */)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, resp)
}
```

## Testing Strategy

### Unit Tests

Each service has dedicated unit tests using `MockStore` for database interactions:

```go
func TestBatchService_UpdateAudiobooks_SingleBook(t *testing.T) {
    mockDB := &database.MockStore{
        GetBookByIDFunc: func(id string) (*database.Book, error) {
            return &database.Book{ID: id, Title: "Original"}, nil
        },
        UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
            return book, nil
        },
    }

    bs := NewBatchService(mockDB)
    req := &BatchUpdateRequest{
        IDs: []string{"book1"},
        Updates: map[string]interface{}{"title": "Updated"},
    }

    resp := bs.UpdateAudiobooks(req)
    assert.Equal(t, 1, resp.Success)
}
```

### Integration Tests

Handler tests verify the complete request/response cycle:

```go
func TestBatchUpdateAudiobooksEndpoint(t *testing.T) {
    srv := NewServer()
    srv.router.POST("/api/audiobooks/batch-update", srv.batchUpdateAudiobooks)

    payload := `{"ids": ["book1"], "updates": {"title": "Updated"}}`
    resp := makeRequest(t, srv.router, "POST", "/api/audiobooks/batch-update", payload)

    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
```

### MockStore

The `MockStore` in `internal/database/mock_store.go` provides:

- Function fields for every database method (e.g., `GetBookByIDFunc`, `UpdateBookFunc`)
- Default implementations that return nil/error
- Ability to set up behavior per-test without external mocks

## Adding New Features

### Adding a New Service

1. Create `internal/server/new_feature_service.go`:

```go
package server

import (
    "github.com/jdfalk/audiobook-organizer/internal/database"
)

type FeatureService struct {
    db database.Store
}

func NewFeatureService(db database.Store) *FeatureService {
    return &FeatureService{db: db}
}

func (fs *FeatureService) DoSomething(input string) (string, error) {
    // Business logic here
    return fs.db.SomeOperation(input)
}
```

2. Add tests in `internal/server/new_feature_service_test.go`

3. Add service to `Server` struct in `server.go`:

```go
type Server struct {
    // ... existing fields ...
    featureService *FeatureService
}
```

4. Initialize in `NewServer()`:

```go
featureService: NewFeatureService(database.GlobalStore),
```

5. Create handler that uses the service:

```go
func (s *Server) doSomething(c *gin.Context) {
    var req struct {
        Input string `json:"input"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    result, err := s.featureService.DoSomething(req.Input)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"result": result})
}
```

6. Register route in `setupRoutes()`:

```go
r.POST("/api/feature/do-something", s.doSomething)
```

## Refactoring Metrics

### Code Changes

- **5 New Service Files**: 769 total lines (batch, work, author/series, filesystem, import)
- **5 Service Test Files**: 381 total lines
- **1 MockStore**: 750 lines (comprehensive database mock)
- **Server Modifications**: 401 lines removed, 70 lines added (net -331 lines)
- **Net Result**: Cleaner separation of concerns with minimal overall size increase

### Handler Improvements

- **Average Handler Size**: Reduced from 30-40 lines → 5-15 lines
- **Business Logic Removed**: ~420 lines extracted to service layer
- **Handlers Refactored**: 11 handlers now use service layer
- **Test Coverage**: Easy to mock and test

### Dependency Injection

- All services receive `database.Store` interface in constructor
- Enables easy testing with MockStore
- Improves testability and code reusability

## Error Handling

### Service Layer

Services return errors with context:

```go
if book == nil {
    return nil, fmt.Errorf("book not found")
}
```

### Handler Layer

Handlers translate service errors to HTTP status codes:

```go
result, err := s.service.GetBook(id)
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
    } else {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    }
    return
}
```

## Future Improvements

### Services Not Yet Refactored

The following handlers could benefit from service extraction:

- **Operation Management**: startScan, startOrganize, getOperationStatus, cancelOperation
- **System Management**: getSystemStatus, getSystemLogs, getConfig, updateConfig
- **Import Path Management**: listImportPaths, addImportPath, removeImportPath

### Potential Next Steps

1. Extract operation management into `OperationService`
2. Extract system management into `SystemService`
3. Extract configuration management into `ConfigService`
4. Add logging service across all operations
5. Add metrics service for performance tracking

## References

- **Implementation Plan**: `docs/plans/2026-02-03-complete-service-layer-refactoring.md`
- **AudiobookService Example**: `internal/server/audiobook_service.go`
- **Server Initialization**: `internal/server/server.go` (NewServer function)
- **Test Examples**: `internal/server/*_test.go`

