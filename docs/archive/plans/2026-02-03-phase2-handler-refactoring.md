# Phase 2: Priority Handler Refactoring

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Extract business logic from 5 high-priority handlers (updateAudiobook, addImportPath, updateConfig, getSystemLogs, getSystemStatus) into service classes to achieve thin HTTP adapter pattern across all handlers.

**Architecture:** Create 4 new service classes (AudiobookUpdateService, ImportPathService, ConfigUpdateService, SystemService) that handle data transformation, validation, and business logic. Handlers become request parsers that delegate entirely to services. Services use dependency injection for database and other dependencies.

**Tech Stack:** Go, Gin web framework, service pattern with MockStore for testing

---

## Task 1: AudiobookUpdateService - Extract updateAudiobook handler logic

**Files:**
- Create: `internal/server/audiobook_update_service.go`
- Create: `internal/server/audiobook_update_service_test.go`
- Modify: `internal/server/server.go` - Add service field, update handler, update NewServer()

**Background:** The `updateAudiobook` handler (lines 1097-1237, ~141 lines) contains ~110 lines of JSON parsing/field extraction logic. It unmarshals the request into multiple nested structures and extracts individual fields with type conversions.

**Step 1: Create the test file with comprehensive tests**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/audiobook_update_service_test.go`:

```go
// file: internal/server/audiobook_update_service_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestAudiobookUpdateService_ValidateRequest_EmptyID(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	_, err := service.ValidateRequest("", map[string]interface{}{})

	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestAudiobookUpdateService_ValidateRequest_NoUpdates(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	_, err := service.ValidateRequest("book1", map[string]interface{}{})

	if err == nil {
		t.Error("expected error for empty updates")
	}
}

func TestAudiobookUpdateService_ExtractStringField(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{
		"title": "New Title",
	}

	result, ok := service.ExtractStringField(payload, "title")

	if !ok || result != "New Title" {
		t.Errorf("expected 'New Title', got %q (ok=%v)", result, ok)
	}
}

func TestAudiobookUpdateService_ExtractIntField(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{
		"author_id": float64(42),
	}

	result, ok := service.ExtractIntField(payload, "author_id")

	if !ok || result != 42 {
		t.Errorf("expected 42, got %d (ok=%v)", result, ok)
	}
}

func TestAudiobookUpdateService_ExtractStringField_NotFound(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{}

	_, ok := service.ExtractStringField(payload, "missing")

	if ok {
		t.Error("expected ok=false for missing field")
	}
}

func TestAudiobookUpdateService_ExtractOverrides_Success(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{
		"overrides": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}

	result, ok := service.ExtractOverrides(payload)

	if !ok || len(result) != 2 {
		t.Errorf("expected 2 overrides, got %d (ok=%v)", len(result), ok)
	}
}

func TestAudiobookUpdateService_ApplyUpdatesToBook(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	book := &database.Book{
		ID:    "book1",
		Title: "Original Title",
	}

	updates := map[string]interface{}{
		"title": "Updated Title",
	}

	service.ApplyUpdatesToBook(book, updates)

	if book.Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %q", book.Title)
	}
}

func TestAudiobookUpdateService_ApplyOverridesToBook(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	book := &database.Book{
		ID: "book1",
	}

	overrides := map[string]interface{}{
		"field1": "value1",
	}

	service.ApplyOverridesToBook(book, overrides)

	if book.Overrides == nil {
		t.Error("expected overrides to be set")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server -run TestAudiobookUpdateService -v`

Expected: FAIL - AudiobookUpdateService not defined

**Step 3: Create the service implementation**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/audiobook_update_service.go`:

```go
// file: internal/server/audiobook_update_service.go
// version: 1.0.0
// guid: b2c3d4e5-f6g7-h8i9-j0k1-l2m3n4o5p6q7

package server

import (
	"encoding/json"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type AudiobookUpdateService struct {
	db database.Store
}

func NewAudiobookUpdateService(db database.Store) *AudiobookUpdateService {
	return &AudiobookUpdateService{db: db}
}

// ValidateRequest checks if the update request has required fields
func (aus *AudiobookUpdateService) ValidateRequest(id string, payload map[string]interface{}) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("audiobook ID is required")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("no updates provided")
	}
	return payload, nil
}

// ExtractStringField extracts a string value from payload
func (aus *AudiobookUpdateService) ExtractStringField(payload map[string]interface{}, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// ExtractIntField extracts an int value from payload (handling JSON float64)
func (aus *AudiobookUpdateService) ExtractIntField(payload map[string]interface{}, key string) (int, bool) {
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	// JSON unmarshals numbers as float64
	f, ok := val.(float64)
	return int(f), ok
}

// ExtractBoolField extracts a bool value from payload
func (aus *AudiobookUpdateService) ExtractBoolField(payload map[string]interface{}, key string) (bool, bool) {
	val, ok := payload[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ExtractOverrides extracts and marshals the overrides map from payload
func (aus *AudiobookUpdateService) ExtractOverrides(payload map[string]interface{}) (map[string]interface{}, bool) {
	val, ok := payload["overrides"]
	if !ok {
		return nil, false
	}

	overridesMap, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}

	return overridesMap, true
}

// ApplyUpdatesToBook applies field updates to a book struct
func (aus *AudiobookUpdateService) ApplyUpdatesToBook(book *database.Book, updates map[string]interface{}) {
	if title, ok := aus.ExtractStringField(updates, "title"); ok {
		book.Title = title
	}
	if authorID, ok := aus.ExtractIntField(updates, "author_id"); ok {
		book.AuthorID = &authorID
	}
	if seriesID, ok := aus.ExtractIntField(updates, "series_id"); ok {
		book.SeriesID = &seriesID
	}
	if narrator, ok := aus.ExtractStringField(updates, "narrator"); ok {
		book.Narrator = narrator
	}
	if publisher, ok := aus.ExtractStringField(updates, "publisher"); ok {
		book.Publisher = &publisher
	}
	if language, ok := aus.ExtractStringField(updates, "language"); ok {
		book.Language = &language
	}
	if year, ok := aus.ExtractIntField(updates, "published_year"); ok {
		book.PublishedYear = &year
	}
	if isbn, ok := aus.ExtractStringField(updates, "isbn"); ok {
		book.ISBN = &isbn
	}
}

// ApplyOverridesToBook applies the overrides to a book struct
func (aus *AudiobookUpdateService) ApplyOverridesToBook(book *database.Book, overrides map[string]interface{}) error {
	if len(overrides) == 0 {
		return nil
	}

	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("failed to marshal overrides: %w", err)
	}

	book.Overrides = string(overridesJSON)
	return nil
}

// UpdateAudiobook is the main business logic method
func (aus *AudiobookUpdateService) UpdateAudiobook(id string, payload map[string]interface{}) (*database.Book, error) {
	// Validate request
	_, err := aus.ValidateRequest(id, payload)
	if err != nil {
		return nil, err
	}

	// Get the book from database
	book, err := aus.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	// Apply updates
	aus.ApplyUpdatesToBook(book, payload)

	// Apply overrides if present
	if overrides, ok := aus.ExtractOverrides(payload); ok {
		if err := aus.ApplyOverridesToBook(book, overrides); err != nil {
			return nil, err
		}
	}

	// Persist to database
	updated, err := aus.db.UpdateBook(id, book)
	if err != nil {
		return nil, fmt.Errorf("failed to update audiobook: %w", err)
	}

	return updated, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server -run TestAudiobookUpdateService -v`

Expected: PASS (all tests pass)

**Step 5: Add service to Server struct and refactor handler**

Modify `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go`:

Find the Server struct (around line 468) and add:
```go
audiobookUpdateService *AudiobookUpdateService
```

In NewServer() function (around line 490), add:
```go
audiobookUpdateService: NewAudiobookUpdateService(database.GlobalStore),
```

Replace the `updateAudiobook` handler (lines 1097-1237) with:

```go
func (s *Server) updateAudiobook(c *gin.Context) {
	id := c.Param("id")

	var payload map[string]interface{}
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

**Step 6: Run all tests to verify**

Run: `make test`

Expected: All tests pass including new AudiobookUpdateService tests

**Step 7: Commit**

```bash
git add internal/server/audiobook_update_service.go internal/server/audiobook_update_service_test.go internal/server/server.go
git commit -m "feat(audiobook_update_service): extract updateAudiobook handler logic into service layer

- Create AudiobookUpdateService with field extraction and update logic
- Extract 110 lines of JSON parsing and field transformation from handler
- Reduce updateAudiobook handler from 141 lines to 15 lines (thin adapter)
- Add comprehensive unit tests for field extraction and overrides handling
- All tests pass, clean build"
```

---

## Task 2: ImportPathService - Extract addImportPath handler logic

**Files:**
- Create: `internal/server/import_path_service.go`
- Create: `internal/server/import_path_service_test.go`
- Modify: `internal/server/server.go` - Add service field, update handler, update NewServer()

**Background:** The `addImportPath` handler (lines 1449-1600, ~152 lines) contains ~130 lines of import orchestration including path creation, auto-scan, and auto-organize logic.

**Step 1: Create comprehensive test file**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/import_path_service_test.go`:

```go
// file: internal/server/import_path_service_test.go
// version: 1.0.0
// guid: c3d4e5f6-g7h8-i9j0-k1l2-m3n4o5p6q7r8

package server

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func TestImportPathService_CreateImportPath_Success(t *testing.T) {
	mockDB := &database.MockStore{
		CreateImportPathFunc: func(path *database.ImportPath) (*database.ImportPath, error) {
			return path, nil
		},
	}
	service := NewImportPathService(mockDB)

	path := &database.ImportPath{Path: "/import/folder"}
	result, err := service.CreateImportPath(path)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result == nil {
		t.Error("expected result, got nil")
	}
}

func TestImportPathService_CreateImportPath_EmptyPath(t *testing.T) {
	service := NewImportPathService(&database.MockStore{})

	path := &database.ImportPath{Path: ""}
	_, err := service.CreateImportPath(path)

	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestImportPathService_ValidatePath_Success(t *testing.T) {
	service := NewImportPathService(&database.MockStore{})

	err := service.ValidatePath("/valid/path")

	if err != nil {
		t.Errorf("expected no error for valid path, got %v", err)
	}
}

func TestImportPathService_ValidatePath_Empty(t *testing.T) {
	service := NewImportPathService(&database.MockStore{})

	err := service.ValidatePath("")

	if err == nil {
		t.Error("expected error for empty path")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server -run TestImportPathService -v`

Expected: FAIL - ImportPathService not defined

**Step 3: Create the service implementation**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/import_path_service.go`:

```go
// file: internal/server/import_path_service.go
// version: 1.0.0
// guid: d4e5f6g7-h8i9-j0k1-l2m3-n4o5p6q7r8s9

package server

import (
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type ImportPathService struct {
	db database.Store
}

func NewImportPathService(db database.Store) *ImportPathService {
	return &ImportPathService{db: db}
}

// ValidatePath validates that an import path is not empty
func (ips *ImportPathService) ValidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("import path cannot be empty")
	}
	return nil
}

// CreateImportPath creates a new import path in the database
func (ips *ImportPathService) CreateImportPath(path *database.ImportPath) (*database.ImportPath, error) {
	if err := ips.ValidatePath(path.Path); err != nil {
		return nil, err
	}

	// Default to enabled if not explicitly set
	if path.Enabled == false && path.Path != "" {
		path.Enabled = true
	}

	return ips.db.CreateImportPath(path)
}

// UpdateImportPathEnabled updates the enabled status of an import path
func (ips *ImportPathService) UpdateImportPathEnabled(id string, enabled bool) error {
	path, err := ips.db.GetImportPathByID(id)
	if err != nil || path == nil {
		return fmt.Errorf("import path not found")
	}

	path.Enabled = enabled
	return ips.db.UpdateImportPath(id, path)
}

// GetImportPath retrieves an import path by ID
func (ips *ImportPathService) GetImportPath(id string) (*database.ImportPath, error) {
	path, err := ips.db.GetImportPathByID(id)
	if err != nil || path == nil {
		return nil, fmt.Errorf("import path not found")
	}
	return path, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server -run TestImportPathService -v`

Expected: PASS

**Step 5: Add service to Server struct and update handler**

Modify `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go`:

Add to Server struct:
```go
importPathService *ImportPathService
```

In NewServer(), add:
```go
importPathService: NewImportPathService(database.GlobalStore),
```

Refactor handler to simplify (extract auto-scan orchestration separately in a follow-up task). For now, simplify the path creation:

Find the `addImportPath` handler and replace the path creation section with:
```go
func (s *Server) addImportPath(c *gin.Context) {
	// ... existing request parsing code ...

	// Use service to create the import path
	createdPath, err := s.importPathService.CreateImportPath(&newPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ... keep existing auto-scan and auto-organize logic for now ...
	// (This will be further refactored in follow-up)

	c.JSON(http.StatusCreated, createdPath)
}
```

**Step 6: Run tests**

Run: `make test`

Expected: All tests pass

**Step 7: Commit**

```bash
git add internal/server/import_path_service.go internal/server/import_path_service_test.go internal/server/server.go
git commit -m "feat(import_path_service): extract import path management logic into service

- Create ImportPathService with path validation and creation
- Extract path validation and creation logic from handler
- Add comprehensive unit tests for path validation
- Handler now delegates path creation to service
- All tests pass"
```

---

## Task 3: ConfigUpdateService - Extract updateConfig handler logic

**Files:**
- Create: `internal/server/config_update_service.go`
- Create: `internal/server/config_update_service_test.go`
- Modify: `internal/server/server.go` - Add service field, update handler, update NewServer()

**Background:** The `updateConfig` handler (lines 2019-2185, ~167 lines) contains ~130 lines of config field mapping and validation logic.

**Step 1: Create test file**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/config_update_service_test.go`:

```go
// file: internal/server/config_update_service_test.go
// version: 1.0.0
// guid: e5f6g7h8-i9j0-k1l2-m3n4-o5p6q7r8s9t0

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

func TestConfigUpdateService_ValidateUpdate_EmptyPayload(t *testing.T) {
	service := NewConfigUpdateService()

	err := service.ValidateUpdate(map[string]interface{}{})

	if err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestConfigUpdateService_ExtractStringField(t *testing.T) {
	service := NewConfigUpdateService()

	payload := map[string]interface{}{
		"root_dir": "/library",
	}

	result, ok := service.ExtractStringField(payload, "root_dir")

	if !ok || result != "/library" {
		t.Errorf("expected '/library', got %q (ok=%v)", result, ok)
	}
}

func TestConfigUpdateService_ExtractBoolField(t *testing.T) {
	service := NewConfigUpdateService()

	payload := map[string]interface{}{
		"auto_organize": true,
	}

	result, ok := service.ExtractBoolField(payload, "auto_organize")

	if !ok || result != true {
		t.Errorf("expected true, got %v (ok=%v)", result, ok)
	}
}

func TestConfigUpdateService_ExtractIntField(t *testing.T) {
	service := NewConfigUpdateService()

	payload := map[string]interface{}{
		"concurrent_scans": float64(4),
	}

	result, ok := service.ExtractIntField(payload, "concurrent_scans")

	if !ok || result != 4 {
		t.Errorf("expected 4, got %d (ok=%v)", result, ok)
	}
}

func TestConfigUpdateService_ApplyUpdates_Success(t *testing.T) {
	service := NewConfigUpdateService()

	updates := map[string]interface{}{
		"root_dir": "/new/library",
	}

	originalDir := config.AppConfig.RootDir
	defer func() {
		config.AppConfig.RootDir = originalDir
	}()

	service.ApplyUpdates(updates)

	if config.AppConfig.RootDir != "/new/library" {
		t.Errorf("expected '/new/library', got %q", config.AppConfig.RootDir)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server -run TestConfigUpdateService -v`

Expected: FAIL - ConfigUpdateService not defined

**Step 3: Create the service implementation**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/config_update_service.go`:

```go
// file: internal/server/config_update_service.go
// version: 1.0.0
// guid: f6g7h8i9-j0k1-l2m3-n4o5-p6q7r8s9t0u1

package server

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

type ConfigUpdateService struct{}

func NewConfigUpdateService() *ConfigUpdateService {
	return &ConfigUpdateService{}
}

// ValidateUpdate checks if the update payload has required fields
func (cus *ConfigUpdateService) ValidateUpdate(payload map[string]interface{}) error {
	if len(payload) == 0 {
		return fmt.Errorf("no configuration updates provided")
	}
	return nil
}

// ExtractStringField extracts a string value from payload
func (cus *ConfigUpdateService) ExtractStringField(payload map[string]interface{}, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// ExtractBoolField extracts a bool value from payload
func (cus *ConfigUpdateService) ExtractBoolField(payload map[string]interface{}, key string) (bool, bool) {
	val, ok := payload[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ExtractIntField extracts an int value from payload (handling JSON float64)
func (cus *ConfigUpdateService) ExtractIntField(payload map[string]interface{}, key string) (int, bool) {
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	f, ok := val.(float64)
	return int(f), ok
}

// ApplyUpdates applies all updates from payload to AppConfig
func (cus *ConfigUpdateService) ApplyUpdates(payload map[string]interface{}) {
	if rootDir, ok := cus.ExtractStringField(payload, "root_dir"); ok {
		config.AppConfig.RootDir = rootDir
	}

	if autoOrganize, ok := cus.ExtractBoolField(payload, "auto_organize"); ok {
		config.AppConfig.AutoOrganize = autoOrganize
	}

	if concurrentScans, ok := cus.ExtractIntField(payload, "concurrent_scans"); ok {
		config.AppConfig.ConcurrentScans = concurrentScans
	}

	if excludePatterns, ok := payload["exclude_patterns"].([]interface{}); ok {
		patterns := make([]string, len(excludePatterns))
		for i, p := range excludePatterns {
			if s, ok := p.(string); ok {
				patterns[i] = s
			}
		}
		config.AppConfig.ExcludePatterns = patterns
	}

	if supportedExtensions, ok := payload["supported_extensions"].([]interface{}); ok {
		extensions := make([]string, len(supportedExtensions))
		for i, e := range supportedExtensions {
			if s, ok := e.(string); ok {
				extensions[i] = s
			}
		}
		config.AppConfig.SupportedExtensions = extensions
	}
}

// MaskSecrets removes sensitive fields from config for response
func (cus *ConfigUpdateService) MaskSecrets(cfg *config.Config) map[string]interface{} {
	result := map[string]interface{}{
		"root_dir":              cfg.RootDir,
		"auto_organize":         cfg.AutoOrganize,
		"concurrent_scans":      cfg.ConcurrentScans,
		"exclude_patterns":      cfg.ExcludePatterns,
		"supported_extensions":  cfg.SupportedExtensions,
	}
	return result
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server -run TestConfigUpdateService -v`

Expected: PASS

**Step 5: Add service to Server struct and refactor handler**

Modify `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go`:

Add to Server struct:
```go
configUpdateService *ConfigUpdateService
```

In NewServer(), add:
```go
configUpdateService: NewConfigUpdateService(),
```

Replace the `updateConfig` handler (lines 2019-2185) with:

```go
func (s *Server) updateConfig(c *gin.Context) {
	var payload map[string]interface{}
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

**Step 6: Run tests**

Run: `make test`

Expected: All tests pass

**Step 7: Commit**

```bash
git add internal/server/config_update_service.go internal/server/config_update_service_test.go internal/server/server.go
git commit -m "feat(config_update_service): extract updateConfig handler logic into service

- Create ConfigUpdateService with field extraction and validation
- Extract 130 lines of config mapping logic from handler
- Reduce updateConfig handler from 167 lines to 22 lines (thin adapter)
- Add comprehensive unit tests for field extraction and masking
- All tests pass, clean build"
```

---

## Task 4: SystemService - Extract getSystemStatus and getSystemLogs handlers

**Files:**
- Create: `internal/server/system_service.go`
- Create: `internal/server/system_service_test.go`
- Modify: `internal/server/server.go` - Add service field, update handlers, update NewServer()

**Background:** The `getSystemStatus` handler (94 lines, 75 lines logic) and `getSystemLogs` handler (111 lines, 95 lines logic) both perform data aggregation and filtering that should be in a service.

**Step 1: Create comprehensive test file**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/system_service_test.go`:

```go
// file: internal/server/system_service_test.go
// version: 1.0.0
// guid: g7h8i9j0-k1l2-m3n4-o5p6-q7r8s9t0u1v2

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func TestSystemService_CollectSystemStatus_Success(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllImportPathsFunc: func() ([]database.ImportPath, error) {
			return []database.ImportPath{}, nil
		},
	}
	service := NewSystemService(mockDB)

	status, err := service.CollectSystemStatus()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if status == nil {
		t.Error("expected status, got nil")
	}
}

func TestSystemService_FilterLogsBySearch_Match(t *testing.T) {
	service := NewSystemService(&database.MockStore{})

	logs := []operations.OperationLog{
		{
			Message: "Scanning folder /library",
		},
	}

	filtered := service.FilterLogsBySearch(logs, "Scanning")

	if len(filtered) != 1 {
		t.Errorf("expected 1 result, got %d", len(filtered))
	}
}

func TestSystemService_FilterLogsBySearch_NoMatch(t *testing.T) {
	service := NewSystemService(&database.MockStore{})

	logs := []operations.OperationLog{
		{
			Message: "Scanning folder",
		},
	}

	filtered := service.FilterLogsBySearch(logs, "Organizing")

	if len(filtered) != 0 {
		t.Errorf("expected 0 results, got %d", len(filtered))
	}
}

func TestSystemService_PaginateLogs_Success(t *testing.T) {
	service := NewSystemService(&database.MockStore{})

	logs := make([]operations.OperationLog, 100)
	for i := 0; i < 100; i++ {
		logs[i] = operations.OperationLog{Message: "Log"}
	}

	paginated := service.PaginateLogs(logs, 1, 20)

	if len(paginated) != 20 {
		t.Errorf("expected 20 logs, got %d", len(paginated))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server -run TestSystemService -v`

Expected: FAIL - SystemService not defined

**Step 3: Create the service implementation**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/system_service.go`:

```go
// file: internal/server/system_service.go
// version: 1.0.0
// guid: h8i9j0k1-l2m3-n4o5-p6q7-r8s9t0u1v2w3

package server

import (
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

type SystemService struct {
	db database.Store
}

func NewSystemService(db database.Store) *SystemService {
	return &SystemService{db: db}
}

type SystemStatus struct {
	RootDir              string                    `json:"root_dir"`
	ImportPaths          []database.ImportPath     `json:"import_paths"`
	TotalBooks           int                       `json:"total_books"`
	MemoryUsage          uint64                    `json:"memory_usage"`
	Uptime               string                    `json:"uptime"`
	RuntimeVersion       string                    `json:"go_version"`
	ActiveOperationCount int                       `json:"active_operations"`
}

// CollectSystemStatus gathers system status information
func (ss *SystemService) CollectSystemStatus() (*SystemStatus, error) {
	paths, err := ss.db.GetAllImportPaths()
	if err != nil {
		paths = []database.ImportPath{}
	}

	status := &SystemStatus{
		RootDir:     config.AppConfig.RootDir,
		ImportPaths: paths,
	}

	return status, nil
}

// FilterLogsBySearch filters logs by search term (case-insensitive)
func (ss *SystemService) FilterLogsBySearch(logs []operations.OperationLog, searchTerm string) []operations.OperationLog {
	if searchTerm == "" {
		return logs
	}

	searchLower := strings.ToLower(searchTerm)
	filtered := make([]operations.OperationLog, 0)

	for _, log := range logs {
		if strings.Contains(strings.ToLower(log.Message), searchLower) {
			filtered = append(filtered, log)
		}
	}

	return filtered
}

// SortLogsByTimestamp sorts logs by timestamp (descending)
func (ss *SystemService) SortLogsByTimestamp(logs []operations.OperationLog) []operations.OperationLog {
	sorted := make([]operations.OperationLog, len(logs))
	copy(sorted, logs)

	// Bubble sort for small sets (simple and reliable)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j].Timestamp.Before(sorted[j+1].Timestamp) {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	return sorted
}

// PaginateLogs returns a subset of logs for the given page
func (ss *SystemService) PaginateLogs(logs []operations.OperationLog, page, pageSize int) []operations.OperationLog {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	start := (page - 1) * pageSize
	if start >= len(logs) {
		return []operations.OperationLog{}
	}

	end := start + pageSize
	if end > len(logs) {
		end = len(logs)
	}

	return logs[start:end]
}

// GetFormattedUptime returns uptime as a formatted string
func (ss *SystemService) GetFormattedUptime(startTime time.Time) string {
	duration := time.Since(startTime)

	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	return time.Now().Sub(startTime).String()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/server -run TestSystemService -v`

Expected: PASS

**Step 5: Add service to Server struct and refactor handlers**

Modify `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go`:

Add to Server struct:
```go
systemService *SystemService
```

In NewServer(), add:
```go
systemService: NewSystemService(database.GlobalStore),
```

Replace the `getSystemStatus` handler (lines 1800-1893) with:

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

Replace the `getSystemLogs` handler (lines 1895-2005) with:

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

	// Get all operations and their logs (existing logic)
	allOperations, err := database.GlobalStore.GetAllOperations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var allLogs []operations.OperationLog
	for _, op := range allOperations {
		logs, err := database.GlobalStore.GetOperationLogs(op.ID)
		if err == nil && logs != nil {
			allLogs = append(allLogs, logs...)
		}
	}

	// Filter by search term
	filteredLogs := s.systemService.FilterLogsBySearch(allLogs, searchQuery)

	// Sort by timestamp
	sortedLogs := s.systemService.SortLogsByTimestamp(filteredLogs)

	// Paginate
	paginatedLogs := s.systemService.PaginateLogs(sortedLogs, page, pageSize)

	c.JSON(http.StatusOK, gin.H{
		"logs":  paginatedLogs,
		"page":  page,
		"size":  pageSize,
		"total": len(filteredLogs),
	})
}
```

**Step 6: Run tests**

Run: `make test`

Expected: All tests pass

**Step 7: Commit**

```bash
git add internal/server/system_service.go internal/server/system_service_test.go internal/server/server.go
git commit -m "feat(system_service): extract getSystemStatus and getSystemLogs handler logic

- Create SystemService with status collection and log filtering
- Extract 75 lines of status aggregation from getSystemStatus
- Extract 95 lines of filtering/sorting/pagination from getSystemLogs
- Reduce getSystemStatus handler from 94 to 10 lines (thin adapter)
- Reduce getSystemLogs handler from 111 to 25 lines (thin adapter)
- Add comprehensive unit tests for filtering and pagination
- All tests pass, clean build"
```

---

## Verification

After all 4 tasks are complete:

1. Run `make test` to verify all tests pass
2. Run `make build-api` to verify clean compilation
3. Run `go test ./internal/server -cover` to verify coverage improvement
4. Check that all handlers are now 20-30 lines max (thin adapters)
5. Verify all business logic is in service classes

Expected final state:
- All 67 handlers are thin HTTP adapters (request parsing + service call + response formatting)
- No business logic remains in any handler
- Server package coverage improved to 70%+
- Service layer classes handle all complex logic with unit tests
