// file: internal/server/system_service.go
// version: 1.3.0
// guid: h8i9j0k1-l2m3-n4o5-p6q7-r8s9t0u1v2w3

package server

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/sysinfo"
)

type SystemService struct {
	db database.Store
}

func NewSystemService(db database.Store) *SystemService {
	return &SystemService{db: db}
}

type SystemStatus struct {
	Status           string               `json:"status"`
	Version          string               `json:"version"`
	LibraryBookCount int                  `json:"library_book_count"`
	ImportBookCount  int                  `json:"import_book_count"`
	TotalBookCount   int                  `json:"total_book_count"`
	AuthorCount      int                  `json:"author_count"`
	SeriesCount      int                  `json:"series_count"`
	LibrarySizeBytes int64                `json:"library_size_bytes"`
	ImportSizeBytes  int64                `json:"import_size_bytes"`
	TotalSizeBytes   int64                `json:"total_size_bytes"`
	RootDirectory    string               `json:"root_directory"`
	Library          SystemLibraryStatus  `json:"library"`
	ImportPaths      SystemImportStatus   `json:"import_paths"`
	Memory           SystemMemoryStatus   `json:"memory"`
	Runtime          SystemRuntimeStatus  `json:"runtime"`
	Operations       SystemOperationsInfo `json:"operations"`
}

type SystemLibraryStatus struct {
	BookCount   int    `json:"book_count"`
	FolderCount int    `json:"folder_count"`
	TotalSize   int64  `json:"total_size"`
	Path        string `json:"path"`
}

type SystemImportStatus struct {
	BookCount   int   `json:"book_count"`
	FolderCount int   `json:"folder_count"`
	TotalSize   int64 `json:"total_size"`
}

type SystemMemoryStatus struct {
	AllocBytes      uint64 `json:"alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
	NumGC           uint32 `json:"num_gc"`
	HeapAlloc       uint64 `json:"heap_alloc"`
	HeapSys         uint64 `json:"heap_sys"`
	SystemTotal     uint64 `json:"system_total"`
}

type SystemRuntimeStatus struct {
	GoVersion    string `json:"go_version"`
	NumGoroutine int    `json:"num_goroutine"`
	NumCPU       int    `json:"num_cpu"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
}

type SystemOperationsInfo struct {
	Recent []database.Operation `json:"recent"`
}

type SystemLogEntry struct {
	OperationID string    `json:"operation_id"`
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	Details     *string   `json:"details,omitempty"`
}

// CollectSystemStatus gathers system status information
func (ss *SystemService) CollectSystemStatus() (*SystemStatus, error) {
	if ss.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	importFolders, err := ss.db.GetAllImportPaths()
	if err != nil {
		importFolders = []database.ImportPath{}
	}

	rootDir := config.AppConfig.RootDir
	libraryBookCount, importBookCount, err := ss.db.GetBookCountsByLocation(rootDir)
	if err != nil {
		libraryBookCount = 0
		importBookCount = 0
	}

	authorCount, _ := ss.db.CountAuthors()
	seriesCount, _ := ss.db.CountSeries()

	recentOps, err := ss.db.GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)
	// Fall back to DB file sizes when filesystem walk returns 0 (e.g. paths don't exist on this host)
	if librarySize+importSize == 0 {
		if dbLib, dbImp, err := ss.db.GetBookSizesByLocation(rootDir); err == nil {
			librarySize = dbLib
			importSize = dbImp
		}
	}
	totalSize := librarySize + importSize

	status := &SystemStatus{
		Status:           "running",
		Version:          appVersion,
		LibraryBookCount: libraryBookCount,
		ImportBookCount:  importBookCount,
		TotalBookCount:   libraryBookCount + importBookCount,
		AuthorCount:      authorCount,
		SeriesCount:      seriesCount,
		LibrarySizeBytes: librarySize,
		ImportSizeBytes:  importSize,
		TotalSizeBytes:   totalSize,
		RootDirectory:    rootDir,
		Library: SystemLibraryStatus{
			BookCount:   libraryBookCount,
			FolderCount: 1,
			TotalSize:   librarySize,
			Path:        rootDir,
		},
		ImportPaths: SystemImportStatus{
			BookCount:   importBookCount,
			FolderCount: len(importFolders),
			TotalSize:   importSize,
		},
		Memory: SystemMemoryStatus{
			AllocBytes:      memStats.Alloc,
			TotalAllocBytes: memStats.TotalAlloc,
			SysBytes:        memStats.Sys,
			NumGC:           memStats.NumGC,
			HeapAlloc:       memStats.HeapAlloc,
			HeapSys:         memStats.HeapSys,
			SystemTotal:     sysinfo.GetTotalMemory(),
		},
		Runtime: SystemRuntimeStatus{
			GoVersion:    runtime.Version(),
			NumGoroutine: runtime.NumGoroutine(),
			NumCPU:       runtime.NumCPU(),
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
		},
		Operations: SystemOperationsInfo{
			Recent: recentOps,
		},
	}

	return status, nil
}

// FilterLogsBySearch filters logs by search term (case-insensitive)
func (ss *SystemService) FilterLogsBySearch(logs []database.OperationLog, searchTerm string) []database.OperationLog {
	if searchTerm == "" {
		return logs
	}

	searchLower := strings.ToLower(searchTerm)
	filtered := make([]database.OperationLog, 0)

	for _, log := range logs {
		if strings.Contains(strings.ToLower(log.Message), searchLower) {
			filtered = append(filtered, log)
		}
	}

	return filtered
}

// SortLogsByTimestamp sorts logs by timestamp (descending)
func (ss *SystemService) SortLogsByTimestamp(logs []database.OperationLog) []database.OperationLog {
	sorted := make([]database.OperationLog, len(logs))
	copy(sorted, logs)

	// Bubble sort for small sets
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j].CreatedAt.Before(sorted[j+1].CreatedAt) {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	return sorted
}

// PaginateLogs returns a subset of logs for the given page
func (ss *SystemService) PaginateLogs(logs []database.OperationLog, page, pageSize int) []database.OperationLog {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	start := (page - 1) * pageSize
	if start >= len(logs) {
		return []database.OperationLog{}
	}

	end := start + pageSize
	if end > len(logs) {
		end = len(logs)
	}

	return logs[start:end]
}

// GetFormattedUptime returns uptime as a formatted string
func (ss *SystemService) GetFormattedUptime(startTime time.Time) string {
	return time.Now().Sub(startTime).String()
}

// CollectSystemLogs gathers logs for recent operations with filtering and pagination.
func (ss *SystemService) CollectSystemLogs(level, search string, limit, offset int) ([]SystemLogEntry, int, error) {
	if ss.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	operations, err := ss.db.GetRecentOperations(50)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch operations")
	}

	var allLogs []SystemLogEntry
	searchLower := strings.ToLower(search)

	for _, op := range operations {
		logs, err := ss.db.GetOperationLogs(op.ID)
		if err != nil {
			continue
		}

		for _, logEntry := range logs {
			if level != "" && logEntry.Level != level {
				continue
			}

			if searchLower != "" {
				found := strings.Contains(strings.ToLower(logEntry.Message), searchLower)
				if !found && logEntry.Details != nil {
					found = strings.Contains(strings.ToLower(*logEntry.Details), searchLower)
				}
				if !found {
					continue
				}
			}

			allLogs = append(allLogs, SystemLogEntry{
				OperationID: op.ID,
				Timestamp:   logEntry.CreatedAt,
				Level:       logEntry.Level,
				Message:     logEntry.Message,
				Details:     logEntry.Details,
			})
		}
	}

	for i := 0; i < len(allLogs)-1; i++ {
		for j := i + 1; j < len(allLogs); j++ {
			if allLogs[j].Timestamp.After(allLogs[i].Timestamp) {
				allLogs[i], allLogs[j] = allLogs[j], allLogs[i]
			}
		}
	}

	total := len(allLogs)
	start := offset
	end := offset + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	return allLogs[start:end], total, nil
}
