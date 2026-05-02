# Complete Service Layer Refactoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor all HTTP handlers in the server package to use a clean service layer, moving all business logic out of handlers so they only deal with HTTP parsing and responses.

**Architecture:** Follow the AudiobookService pattern already established. Each domain (audiobooks, works, authors, series, filesystem, imports, operations) gets its own service class. Services contain all business logic and database interaction. Handlers become thin HTTP adapters that parse requests, call services, and format responses.

**Tech Stack:** Go, Gin web framework, existing database layer, service pattern

---

## Task 1: Batch Update Audiobooks Service

**Files:**
- Modify: `internal/server/server.go:1248-1310` (batchUpdateAudiobooks handler - extract business logic)
- Create: `internal/server/batch_service.go` (new service for batch operations)
- Create: `internal/server/batch_service_test.go` (tests)

**Step 1: Create batch_service.go with BatchUpdateAudiobooks method**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/batch_service.go`:

```go
// file: internal/server/batch_service.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type BatchService struct {
	db database.Store
}

func NewBatchService(db database.Store) *BatchService {
	return &BatchService{db: db}
}

type BatchUpdateRequest struct {
	IDs     []string               `json:"ids"`
	Updates map[string]interface{} `json:"updates"`
}

type BatchUpdateResult struct {
	ID      string      `json:"id"`
	Success bool        `json:"success,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type BatchUpdateResponse struct {
	Results []BatchUpdateResult `json:"results"`
	Success int                 `json:"success"`
	Failed  int                 `json:"failed"`
	Total   int                 `json:"total"`
}

func (bs *BatchService) UpdateAudiobooks(req *BatchUpdateRequest) *BatchUpdateResponse {
	resp := &BatchUpdateResponse{
		Results: []BatchUpdateResult{},
		Total:   len(req.IDs),
	}

	if len(req.IDs) == 0 {
		return resp
	}

	for _, id := range req.IDs {
		book, err := bs.db.GetBookByID(id)
		if err != nil {
			resp.Results = append(resp.Results, BatchUpdateResult{
				ID:    id,
				Error: "not found",
			})
			resp.Failed++
			continue
		}

		// Apply updates
		if title, ok := req.Updates["title"].(string); ok {
			book.Title = title
		}
		if format, ok := req.Updates["format"].(string); ok {
			book.Format = format
		}
		if authorID, ok := req.Updates["author_id"].(float64); ok {
			aid := int(authorID)
			book.AuthorID = &aid
		}
		if seriesID, ok := req.Updates["series_id"].(float64); ok {
			sid := int(seriesID)
			book.SeriesID = &sid
		}
		if seriesSeq, ok := req.Updates["series_sequence"].(float64); ok {
			seq := int(seriesSeq)
			book.SeriesSequence = &seq
		}

		if _, err := bs.db.UpdateBook(id, book); err != nil {
			resp.Results = append(resp.Results, BatchUpdateResult{
				ID:    id,
				Error: err.Error(),
			})
			resp.Failed++
		} else {
			resp.Results = append(resp.Results, BatchUpdateResult{
				ID:      id,
				Success: true,
			})
			resp.Success++
		}
	}

	return resp
}
```

**Step 2: Write test for batchUpdateAudiobooks**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/batch_service_test.go`:

```go
// file: internal/server/batch_service_test.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestBatchUpdateAudiobooks_EmptyBatch(t *testing.T) {
	mockDB := &database.MockStore{}
	bs := NewBatchService(mockDB)

	req := &BatchUpdateRequest{
		IDs:     []string{},
		Updates: map[string]interface{}{},
	}

	resp := bs.UpdateAudiobooks(req)

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if resp.Success != 0 {
		t.Errorf("expected success 0, got %d", resp.Success)
	}
	if resp.Failed != 0 {
		t.Errorf("expected failed 0, got %d", resp.Failed)
	}
}

func TestBatchUpdateAudiobooks_SingleBook(t *testing.T) {
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{
				ID:    id,
				Title: "Original Title",
			}, nil
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
	}
	bs := NewBatchService(mockDB)

	req := &BatchUpdateRequest{
		IDs: []string{"book1"},
		Updates: map[string]interface{}{
			"title": "Updated Title",
		},
	}

	resp := bs.UpdateAudiobooks(req)

	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
	if resp.Success != 1 {
		t.Errorf("expected success 1, got %d", resp.Success)
	}
	if resp.Failed != 0 {
		t.Errorf("expected failed 0, got %d", resp.Failed)
	}
}
```

**Step 3: Run tests to verify they pass**

Run: `make test`
Expected: Tests pass (will need MockStore implementation in database package)

**Step 4: Update batchUpdateAudiobooks handler to use service**

Modify handler at `internal/server/server.go:1248-1310`:

```go
func (s *Server) batchUpdateAudiobooks(c *gin.Context) {
	var req BatchUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := s.batchService.UpdateAudiobooks(&req)
	c.JSON(http.StatusOK, resp)
}
```

**Step 5: Add batchService to Server struct**

Modify `internal/server/server.go` struct (around line 468):

```go
type Server struct {
	// ... existing fields ...
	audiobookService *AudiobookService
	batchService     *BatchService  // Add this line
	// ... rest of fields ...
}
```

Initialize in NewServer (around line 468):

```go
batchService: NewBatchService(database.GlobalStore),
```

**Step 6: Commit**

```bash
git add internal/server/batch_service.go internal/server/batch_service_test.go internal/server/server.go
git commit -m "feat(batch_service): extract batch update logic into service layer"
```

---

## Task 2: Work Management Service

**Files:**
- Create: `internal/server/work_service.go` (new service)
- Create: `internal/server/work_service_test.go` (tests)
- Modify: `internal/server/server.go:1314-1412` (refactor 5 work handlers)

**Step 1: Create work_service.go**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/work_service.go`:

```go
// file: internal/server/work_service.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type WorkService struct {
	db database.Store
}

func NewWorkService(db database.Store) *WorkService {
	return &WorkService{db: db}
}

type WorkListResponse struct {
	Items []database.Work `json:"items"`
	Count int             `json:"count"`
}

func (ws *WorkService) ListWorks() (*WorkListResponse, error) {
	works, err := ws.db.GetAllWorks()
	if err != nil {
		return nil, err
	}
	if works == nil {
		works = []database.Work{}
	}
	return &WorkListResponse{
		Items: works,
		Count: len(works),
	}, nil
}

func (ws *WorkService) CreateWork(work *database.Work) (*database.Work, error) {
	if strings.TrimSpace(work.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	return ws.db.CreateWork(work)
}

func (ws *WorkService) GetWork(id string) (*database.Work, error) {
	work, err := ws.db.GetWorkByID(id)
	if err != nil {
		return nil, err
	}
	if work == nil {
		return nil, fmt.Errorf("work not found")
	}
	return work, nil
}

func (ws *WorkService) UpdateWork(id string, work *database.Work) (*database.Work, error) {
	if strings.TrimSpace(work.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	updated, err := ws.db.UpdateWork(id, work)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (ws *WorkService) DeleteWork(id string) error {
	return ws.db.DeleteWork(id)
}
```

**Step 2: Write tests for WorkService**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/work_service_test.go`:

```go
// file: internal/server/work_service_test.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestWorkService_ListWorks_Empty(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllWorksFunc: func() ([]database.Work, error) {
			return nil, nil
		},
	}
	ws := NewWorkService(mockDB)

	resp, err := ws.ListWorks()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Count)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

func TestWorkService_CreateWork_Success(t *testing.T) {
	mockDB := &database.MockStore{
		CreateWorkFunc: func(w *database.Work) (*database.Work, error) {
			return w, nil
		},
	}
	ws := NewWorkService(mockDB)

	work := &database.Work{Title: "Test Work"}
	result, err := ws.CreateWork(work)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.Title != "Test Work" {
		t.Errorf("expected title 'Test Work', got %q", result.Title)
	}
}

func TestWorkService_CreateWork_MissingTitle(t *testing.T) {
	mockDB := &database.MockStore{}
	ws := NewWorkService(mockDB)

	work := &database.Work{Title: ""}
	_, err := ws.CreateWork(work)

	if err == nil {
		t.Error("expected error for missing title")
	}
}

func TestWorkService_GetWork_NotFound(t *testing.T) {
	mockDB := &database.MockStore{
		GetWorkByIDFunc: func(id string) (*database.Work, error) {
			return nil, nil
		},
	}
	ws := NewWorkService(mockDB)

	_, err := ws.GetWork("nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent work")
	}
}
```

**Step 3: Run tests**

Run: `make test`
Expected: Tests pass

**Step 4: Add workService to Server struct**

Modify `internal/server/server.go`:

```go
type Server struct {
	// ... existing fields ...
	workService *WorkService  // Add this line
	// ... rest ...
}
```

Initialize in NewServer:

```go
workService: NewWorkService(database.GlobalStore),
```

**Step 5: Refactor 5 work handlers**

Replace handlers at `internal/server/server.go:1314-1412`:

```go
func (s *Server) listWorks(c *gin.Context) {
	resp, err := s.workService.ListWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) createWork(c *gin.Context) {
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := s.workService.CreateWork(&work)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Server) getWork(c *gin.Context) {
	id := c.Param("id")
	work, err := s.workService.GetWork(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, work)
}

func (s *Server) updateWork(c *gin.Context) {
	id := c.Param("id")
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := s.workService.UpdateWork(id, &work)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteWork(c *gin.Context) {
	id := c.Param("id")
	if err := s.workService.DeleteWork(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
```

**Step 6: Commit**

```bash
git add internal/server/work_service.go internal/server/work_service_test.go internal/server/server.go
git commit -m "feat(work_service): extract work management logic into service layer"
```

---

## Task 3: Author & Series Service

**Files:**
- Create: `internal/server/author_series_service.go` (new service for both)
- Create: `internal/server/author_series_service_test.go` (tests)
- Modify: `internal/server/server.go:1431-1469` (refactor 2 handlers)

**Step 1: Create author_series_service.go**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/author_series_service.go`:

```go
// file: internal/server/author_series_service.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type AuthorSeriesService struct {
	db database.Store
}

func NewAuthorSeriesService(db database.Store) *AuthorSeriesService {
	return &AuthorSeriesService{db: db}
}

type AuthorListResponse struct {
	Items []database.Author `json:"items"`
	Count int               `json:"count"`
}

type SeriesListResponse struct {
	Items []database.Series `json:"items"`
	Count int               `json:"count"`
}

func (as *AuthorSeriesService) ListAuthors() (*AuthorListResponse, error) {
	authors, err := as.db.GetAllAuthors()
	if err != nil {
		return nil, err
	}
	if authors == nil {
		authors = []database.Author{}
	}
	return &AuthorListResponse{
		Items: authors,
		Count: len(authors),
	}, nil
}

func (as *AuthorSeriesService) ListSeries() (*SeriesListResponse, error) {
	series, err := as.db.GetAllSeries()
	if err != nil {
		return nil, err
	}
	if series == nil {
		series = []database.Series{}
	}
	return &SeriesListResponse{
		Items: series,
		Count: len(series),
	}, nil
}
```

**Step 2: Write tests**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/author_series_service_test.go`:

```go
// file: internal/server/author_series_service_test.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestAuthorSeriesService_ListAuthors_Empty(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return nil, nil
		},
	}
	as := NewAuthorSeriesService(mockDB)

	resp, err := as.ListAuthors()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Count)
	}
}

func TestAuthorSeriesService_ListSeries_Empty(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return nil, nil
		},
	}
	as := NewAuthorSeriesService(mockDB)

	resp, err := as.ListSeries()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Count)
	}
}
```

**Step 3: Run tests**

Run: `make test`
Expected: Tests pass

**Step 4: Add authorSeriesService to Server struct and initialize it**

**Step 5: Refactor listAuthors and listSeries handlers**

Replace at `internal/server/server.go:1431-1469`:

```go
func (s *Server) listAuthors(c *gin.Context) {
	resp, err := s.authorSeriesService.ListAuthors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) listSeries(c *gin.Context) {
	resp, err := s.authorSeriesService.ListSeries()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
```

**Step 6: Commit**

```bash
git add internal/server/author_series_service.go internal/server/author_series_service_test.go internal/server/server.go
git commit -m "feat(author_series_service): extract author/series list logic into service layer"
```

---

## Task 4: Filesystem Service

**Files:**
- Create: `internal/server/filesystem_service.go` (new service)
- Create: `internal/server/filesystem_service_test.go` (tests)
- Modify: `internal/server/server.go:1482-1620` (refactor browse/exclusion handlers)

**Step 1: Create filesystem_service.go**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/filesystem_service.go`:

```go
// file: internal/server/filesystem_service.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"fmt"
	"os"
	"path/filepath"
)

type FilesystemService struct{}

func NewFilesystemService() *FilesystemService {
	return &FilesystemService{}
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"mod_time,omitempty"`
	Excluded bool   `json:"excluded"`
}

type BrowseResult struct {
	Path     string     `json:"path"`
	Items    []FileInfo `json:"items"`
	Count    int        `json:"count"`
	DiskInfo map[string]interface{} `json:"disk_info"`
}

func (fs *FilesystemService) BrowseDirectory(path string) (*BrowseResult, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	items := []FileInfo{}
	for _, entry := range entries {
		fullPath := filepath.Join(absPath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		excluded := false
		if entry.IsDir() {
			jabExcludePath := filepath.Join(fullPath, ".jabexclude")
			if _, err := os.Stat(jabExcludePath); err == nil {
				excluded = true
			}
		}

		item := FileInfo{
			Name:     entry.Name(),
			Path:     fullPath,
			IsDir:    entry.IsDir(),
			Excluded: excluded,
		}

		if !entry.IsDir() {
			item.Size = info.Size()
			item.ModTime = info.ModTime().Unix()
		}

		items = append(items, item)
	}

	diskInfo := map[string]interface{}{}
	if stat, err := os.Stat(absPath); err == nil {
		diskInfo = map[string]interface{}{
			"exists":   true,
			"readable": stat.Mode().Perm()&0400 != 0,
			"writable": stat.Mode().Perm()&0200 != 0,
		}
	}

	return &BrowseResult{
		Path:     absPath,
		Items:    items,
		Count:    len(items),
		DiskInfo: diskInfo,
	}, nil
}

func (fs *FilesystemService) CreateExclusion(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path not found or inaccessible: %w", err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("path must be a directory")
	}

	excludeFile := filepath.Join(absPath, ".jabexclude")
	return os.WriteFile(excludeFile, []byte(""), 0644)
}

func (fs *FilesystemService) RemoveExclusion(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	excludeFile := filepath.Join(absPath, ".jabexclude")
	if err := os.Remove(excludeFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("exclusion not found")
		}
		return fmt.Errorf("failed to remove exclusion: %w", err)
	}
	return nil
}
```

**Step 2: Write tests**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/filesystem_service_test.go`:

```go
// file: internal/server/filesystem_service_test.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"os"
	"path/filepath"
	"testing"
	"io/ioutil"
)

func TestFilesystemService_BrowseDirectory_Empty(t *testing.T) {
	fs := NewFilesystemService()

	result, err := fs.BrowseDirectory("")

	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestFilesystemService_BrowseDirectory_InvalidPath(t *testing.T) {
	fs := NewFilesystemService()

	result, err := fs.BrowseDirectory("/nonexistent/path/that/does/not/exist")

	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestFilesystemService_CreateExclusion_Success(t *testing.T) {
	fs := NewFilesystemService()

	tmpDir, err := ioutil.TempDir("", "test-exclusion")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = fs.CreateExclusion(tmpDir)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	excludeFile := filepath.Join(tmpDir, ".jabexclude")
	if _, err := os.Stat(excludeFile); err != nil {
		t.Errorf("expected .jabexclude file to exist, got %v", err)
	}
}

func TestFilesystemService_RemoveExclusion_NotFound(t *testing.T) {
	fs := NewFilesystemService()

	tmpDir, err := ioutil.TempDir("", "test-remove-exclusion")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = fs.RemoveExclusion(tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent exclusion")
	}
}
```

**Step 3: Run tests**

Run: `make test`
Expected: Tests pass

**Step 4: Add filesystemService to Server struct**

**Step 5: Refactor browseFilesystem, createExclusion, removeExclusion handlers**

Replace at `internal/server/server.go:1482-1620`:

```go
func (s *Server) browseFilesystem(c *gin.Context) {
	path := c.Query("path")
	result, err := s.filesystemService.BrowseDirectory(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.filesystemService.CreateExclusion(req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "exclusion created"})
}

func (s *Server) removeExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.filesystemService.RemoveExclusion(req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
```

**Step 6: Commit**

```bash
git add internal/server/filesystem_service.go internal/server/filesystem_service_test.go internal/server/server.go
git commit -m "feat(filesystem_service): extract filesystem operations into service layer"
```

---

## Task 5: Import & File Management Service

**Files:**
- Create: `internal/server/import_service.go` (new service for file import)
- Create: `internal/server/import_service_test.go` (tests)
- Modify: `internal/server/server.go:2289-2450` (refactor importFile handler)

**Step 1: Create import_service.go**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/import_service.go`:

```go
// file: internal/server/import_service.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

type ImportService struct {
	db database.Store
}

func NewImportService(db database.Store) *ImportService {
	return &ImportService{db: db}
}

type ImportFileRequest struct {
	FilePath string `json:"file_path" binding:"required"`
	Organize bool   `json:"organize"`
}

type ImportFileResponse struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

func (is *ImportService) ImportFile(req *ImportFileRequest) (*ImportFileResponse, error) {
	// Validate file exists and is supported
	fileInfo, err := os.Stat(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("file not found or inaccessible: %w", err)
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	// Check if file extension is supported
	ext := strings.ToLower(filepath.Ext(req.FilePath))
	supported := false
	for _, supportedExt := range config.AppConfig.SupportedExtensions {
		if ext == supportedExt {
			supported = true
			break
		}
	}

	if !supported {
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}

	// Extract metadata
	meta, err := metadata.ExtractMetadata(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Create book record
	book := &database.Book{
		Title:            meta.Title,
		FilePath:         req.FilePath,
		OriginalFilename: stringPtr(filepath.Base(req.FilePath)),
	}

	// Set author if available
	if meta.Artist != "" {
		author, err := is.db.GetAuthorByName(meta.Artist)
		if err != nil {
			// Create new author
			author, err = is.db.CreateAuthor(meta.Artist)
			if err != nil {
				return nil, fmt.Errorf("failed to create author: %w", err)
			}
		}
		if author != nil {
			book.AuthorID = &author.ID
		}
	}

	// Set series if available
	if meta.Series != "" && book.AuthorID != nil {
		series, err := is.db.GetSeriesByName(meta.Series, book.AuthorID)
		if err != nil {
			// Create new series
			series, err = is.db.CreateSeries(meta.Series, book.AuthorID)
			if err != nil {
				return nil, fmt.Errorf("failed to create series: %w", err)
			}
		}
		if series != nil {
			book.SeriesID = &series.ID
			if meta.SeriesIndex > 0 {
				book.SeriesSequence = &meta.SeriesIndex
			}
		}
	}

	// Set additional metadata
	if meta.Album != "" && book.Title == "" {
		book.Title = meta.Album
	}
	if meta.Genre != "" {
		book.Genre = &meta.Genre
	}
	if meta.Year > 0 {
		book.ReleaseYear = &meta.Year
	}
	if meta.Comment != "" {
		book.Description = &meta.Comment
	}

	// Create book in database
	created, err := is.db.CreateBook(book)
	if err != nil {
		return nil, fmt.Errorf("failed to create book: %w", err)
	}

	return &ImportFileResponse{
		ID:       created.ID,
		Title:    created.Title,
		FilePath: created.FilePath,
	}, nil
}
```

**Step 2: Write tests**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/import_service_test.go`:

```go
// file: internal/server/import_service_test.go
// version: 1.0.0
// guid: [generate-new-guid]

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestImportService_ImportFile_MissingFile(t *testing.T) {
	mockDB := &database.MockStore{}
	is := NewImportService(mockDB)

	req := &ImportFileRequest{
		FilePath: "/nonexistent/file.m4b",
	}

	_, err := is.ImportFile(req)

	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestImportService_ImportFile_UnsupportedExtension(t *testing.T) {
	mockDB := &database.MockStore{}
	is := NewImportService(mockDB)

	req := &ImportFileRequest{
		FilePath: "test.txt",
	}

	_, err := is.ImportFile(req)

	if err == nil {
		t.Error("expected error for unsupported file type")
	}
}
```

**Step 3: Run tests**

Run: `make test`
Expected: Tests pass (may need to adjust based on metadata package behavior)

**Step 4: Add importService to Server struct**

**Step 5: Refactor importFile handler**

Replace at `internal/server/server.go:2289-2450`:

```go
func (s *Server) importFile(c *gin.Context) {
	var req ImportFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := s.importService.ImportFile(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}
```

**Step 6: Commit**

```bash
git add internal/server/import_service.go internal/server/import_service_test.go internal/server/server.go
git commit -m "feat(import_service): extract file import logic into service layer"
```

---

## Task 6: Cleanup - Update Server Struct and Remove Helper Functions

**Files:**
- Modify: `internal/server/server.go` (consolidate struct, clean up helpers)

**Step 1: Update Server struct with all service instances**

Modify `internal/server/server.go` Server struct (around line 440-470):

```go
type Server struct {
	// Services
	audiobookService      *AudiobookService
	batchService          *BatchService
	workService           *WorkService
	authorSeriesService   *AuthorSeriesService
	filesystemService     *FilesystemService
	importService         *ImportService

	// Existing fields (leave as-is)
	router          *gin.Engine
	realtimeManager *realtime.Manager
	// ... other existing fields ...
}
```

**Step 2: Update NewServer initialization**

Ensure all services are initialized in NewServer:

```go
s := &Server{
	// ... existing fields ...
	audiobookService:    NewAudiobookService(database.GlobalStore),
	batchService:        NewBatchService(database.GlobalStore),
	workService:         NewWorkService(database.GlobalStore),
	authorSeriesService: NewAuthorSeriesService(database.GlobalStore),
	filesystemService:   NewFilesystemService(),
	importService:       NewImportService(database.GlobalStore),
	// ... rest of initialization ...
}
```

**Step 3: Verify all handlers now use services**

Run grep to ensure no handlers still have database.GlobalStore calls:

Run: `grep -n "database.GlobalStore" internal/server/server.go | grep -v "://"`

Expected: Only comments and service initialization, no handler code with GlobalStore calls

**Step 4: Remove redundant database nil checks from handlers**

Since all handlers now delegate to services, they don't need to check if database.GlobalStore is nil (services handle that).

**Step 5: Update file version header**

Update `internal/server/server.go` version from 1.41.0 to 1.50.0 (significant refactoring)

**Step 6: Commit**

```bash
git add internal/server/server.go
git commit -m "refactor(server): consolidate service layer initialization and update version"
```

---

## Task 7: Verify Complete Refactoring

**Files:**
- No new files
- Modify: Testing and verification only

**Step 1: Run all tests**

Run: `make test`
Expected: All tests pass

**Step 2: Run full build**

Run: `make build-api`
Expected: Build succeeds with no errors

**Step 3: Quick smoke test of handlers**

If the app starts, verify a few endpoints work (list audiobooks, list works, etc.)

**Step 4: Code review of refactoring**

Verify:
- [ ] All handlers are thin (5-15 lines max)
- [ ] All business logic is in services
- [ ] Services are well-named and cohesive
- [ ] No duplicate logic between handlers and services
- [ ] Error handling is consistent
- [ ] All services follow the same pattern

**Step 5: Final commit message summarizing entire refactoring**

```bash
git log --oneline | head -6
```

Expected output showing all 6 commits in order.

**Step 6: Optional - Create summary document**

Create `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/docs/SERVICE_LAYER_ARCHITECTURE.md` (optional enhancement):

Summarize:
- Services created and their responsibilities
- Handler responsibilities (HTTP only)
- How to add new features using this pattern

---

## Testing Strategy

Each service has unit tests. Integration testing happens at handler level with real HTTP requests.

For each commit, verify:
1. Unit tests pass: `make test`
2. Code compiles: `make build-api`
3. No regressions in handler behavior

---

## Notes for Implementation

- **MockStore**: Some tests require `database.MockStore` with mocked methods. Ensure the database package has a MockStore type or create it.
- **Pointer helpers**: The existing `stringPtr()` and `intPtrHelper()` functions in server.go are used in import service - keep them.
- **Error handling**: Services return errors with context; handlers translate to HTTP status codes.
- **No breaking changes**: This refactoring maintains backward compatibility - API responses don't change.

