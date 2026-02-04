// file: internal/server/system_service.go
// version: 1.0.0
// guid: h8i9j0k1-l2m3-n4o5-p6q7-r8s9t0u1v2w3

package server

import (
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
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
