// file: internal/server/dashboard_service.go
// version: 1.0.0
// guid: 9c0d1e2f-3a4b-5c6d-7e8f-9a0b1c2d3e4f

package server

import (
	"fmt"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// DashboardService handles dashboard statistics and metrics collection
type DashboardService struct {
	db database.Store
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(db database.Store) *DashboardService {
	return &DashboardService{db: db}
}

// DashboardMetrics represents aggregated dashboard metrics
type DashboardMetrics struct {
	Books     int   `json:"books"`
	Authors   int   `json:"authors"`
	Series    int   `json:"series"`
	Playlists int   `json:"playlists"`
	Timestamp int64 `json:"timestamp"`
}

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
	Status       string            `json:"status"`
	Timestamp    int64             `json:"timestamp"`
	Version      string            `json:"version"`
	DatabaseType string            `json:"database_type"`
	Metrics      DashboardMetrics  `json:"metrics"`
	PartialError string            `json:"partial_error,omitempty"`
}

// CollectDashboardMetrics gathers all dashboard metrics
func (ds *DashboardService) CollectDashboardMetrics() (*DashboardMetrics, error) {
	metrics := &DashboardMetrics{
		Timestamp: time.Now().Unix(),
	}

	if ds.db == nil {
		return metrics, fmt.Errorf("database not initialized")
	}

	// Collect book count
	if bc, err := ds.db.CountBooks(); err == nil {
		metrics.Books = bc
	}

	// Collect author count
	if authors, err := ds.db.GetAllAuthors(); err == nil {
		metrics.Authors = len(authors)
	}

	// Collect series count
	if series, err := ds.db.GetAllSeries(); err == nil {
		metrics.Series = len(series)
	}

	// Collect playlist count (legacy, placeholder for now)
	if playlists, err := ds.db.GetPlaylistBySeriesID(0); err == nil && playlists != nil {
		metrics.Playlists = 1
	}

	return metrics, nil
}

// GetHealthCheckResponse generates a health check response with metrics
func (ds *DashboardService) GetHealthCheckResponse(version string) *HealthCheckResponse {
	metrics, dbErr := ds.CollectDashboardMetrics()
	if metrics == nil {
		metrics = &DashboardMetrics{
			Timestamp: time.Now().Unix(),
		}
	}

	response := &HealthCheckResponse{
		Status:       "ok",
		Timestamp:    time.Now().Unix(),
		Version:      version,
		DatabaseType: config.AppConfig.DatabaseType,
		Metrics:      *metrics,
	}

	if dbErr != nil {
		response.PartialError = dbErr.Error()
		response.Status = "degraded"
	}

	return response
}

// GetLibraryStats returns library statistics for the dashboard
type LibraryStats struct {
	TotalBooks      int `json:"total_books"`
	TotalAuthors    int `json:"total_authors"`
	TotalSeries     int `json:"total_series"`
	OrganizedBooks  int `json:"organized_books"`
	UnorganizedBooks int `json:"unorganized_books"`
}

// CollectLibraryStats gathers detailed library statistics
func (ds *DashboardService) CollectLibraryStats() (*LibraryStats, error) {
	if ds.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	stats := &LibraryStats{}

	// Count total books
	if bc, err := ds.db.CountBooks(); err == nil {
		stats.TotalBooks = bc
	}

	// Count authors
	if authors, err := ds.db.GetAllAuthors(); err == nil {
		stats.TotalAuthors = len(authors)
	}

	// Count series
	if series, err := ds.db.GetAllSeries(); err == nil {
		stats.TotalSeries = len(series)
	}

	// Count organized vs unorganized books (fetch in batches)
	limit := 1000
	offset := 0
	if books, err := ds.db.GetAllBooks(limit, offset); err == nil {
		for _, book := range books {
			// Assuming a book is organized if it has a library_state = "organized"
			if book.LibraryState != nil && *book.LibraryState == "organized" {
				stats.OrganizedBooks++
			} else {
				stats.UnorganizedBooks++
			}
		}
	}

	return stats, nil
}

// GetQuickMetrics returns a quick set of metrics for display
type QuickMetrics struct {
	BookCount   int `json:"book_count"`
	AuthorCount int `json:"author_count"`
	SeriesCount int `json:"series_count"`
}

// CollectQuickMetrics gathers metrics for quick display
func (ds *DashboardService) CollectQuickMetrics() *QuickMetrics {
	metrics := &QuickMetrics{}

	if ds.db == nil {
		return metrics
	}

	if bc, err := ds.db.CountBooks(); err == nil {
		metrics.BookCount = bc
	}

	if authors, err := ds.db.GetAllAuthors(); err == nil {
		metrics.AuthorCount = len(authors)
	}

	if series, err := ds.db.GetAllSeries(); err == nil {
		metrics.SeriesCount = len(series)
	}

	return metrics
}
