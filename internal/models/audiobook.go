// file: internal/models/audiobook.go
// version: 1.2.1
// guid: 6e7f8a9b-0c1d-2e3f-4a5b-6c7d8e9f0a1b

package models

import "time"

// Author represents an audiobook author
type Author struct {
	ID   int    `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
}

// Series represents an audiobook series
type Series struct {
	ID       int     `json:"id" db:"id"`
	Name     string  `json:"name" db:"name"`
	AuthorID *int    `json:"author_id" db:"author_id"`
	Author   *Author `json:"author,omitempty"`
}

// Audiobook represents an audiobook with all its metadata
type Audiobook struct {
	ID                   int     `json:"id" db:"id"`
	Title                string  `json:"title" db:"title"`
	AuthorID             *int    `json:"author_id" db:"author_id"`
	SeriesID             *int    `json:"series_id" db:"series_id"`
	SeriesSequence       *int    `json:"series_sequence" db:"series_sequence"`
	FilePath             string  `json:"file_path" db:"file_path"`
	OriginalFilename     *string `json:"original_filename" db:"original_filename"`
	Format               string  `json:"format" db:"format"`
	Duration             *int    `json:"duration" db:"duration"`
	Narrator             *string `json:"narrator" db:"narrator"`
	Edition              *string `json:"edition" db:"edition"`
	Language             *string `json:"language" db:"language"`
	Publisher            *string `json:"publisher" db:"publisher"`
	PrintYear            *int    `json:"print_year" db:"print_year"`
	AudiobookReleaseYear *int    `json:"audiobook_release_year" db:"audiobook_release_year"`
	ISBN10               *string `json:"isbn10" db:"isbn10"`
	ISBN13               *string `json:"isbn13" db:"isbn13"`

	// Media info fields (parsed from file metadata)
	Bitrate    *int    `json:"bitrate_kbps" db:"bitrate_kbps"`     // Bitrate in kbps
	Codec      *string `json:"codec" db:"codec"`                   // e.g., 'AAC', 'MP3', 'FLAC'
	SampleRate *int    `json:"sample_rate_hz" db:"sample_rate_hz"` // Sample rate in Hz
	Channels   *int    `json:"channels" db:"channels"`             // Number of audio channels
	BitDepth   *int    `json:"bit_depth" db:"bit_depth"`           // Bit depth (for lossless formats)
	Quality    *string `json:"quality" db:"quality"`               // Human-readable quality string

	// Version management
	IsPrimaryVersion *bool   `json:"is_primary_version" db:"is_primary_version"` // Mark preferred version
	VersionGroupID   *string `json:"version_group_id" db:"version_group_id"`     // Links versions together
	VersionNotes     *string `json:"version_notes" db:"version_notes"`           // e.g., 'Remastered 2020'

	// Related objects (populated via joins)
	Author *Author `json:"author,omitempty"`
	Series *Series `json:"series,omitempty"`
}

// AudiobookListRequest represents pagination and filtering for audiobook list
type AudiobookListRequest struct {
	Page    int    `json:"page" form:"page"`
	Limit   int    `json:"limit" form:"limit"`
	Search  string `json:"search" form:"search"`
	Author  string `json:"author" form:"author"`
	Series  string `json:"series" form:"series"`
	Format  string `json:"format" form:"format"`
	SortBy  string `json:"sort_by" form:"sort_by"`
	SortDir string `json:"sort_dir" form:"sort_dir"`
}

// AudiobookListResponse represents paginated audiobook list response
type AudiobookListResponse struct {
	Audiobooks []Audiobook `json:"audiobooks"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	Pages      int         `json:"pages"`
}

// AudiobookUpdateRequest represents an audiobook update request
type AudiobookUpdateRequest struct {
	Title          *string `json:"title,omitempty"`
	Author         *string `json:"author,omitempty"`
	Series         *string `json:"series,omitempty"`
	SeriesSequence *int    `json:"series_sequence,omitempty"`
	Format         *string `json:"format,omitempty"`
	Duration       *int    `json:"duration,omitempty"`
}

// BatchUpdateRequest represents a batch update request
type BatchUpdateRequest struct {
	AudiobookIDs []int                  `json:"audiobook_ids"`
	Updates      AudiobookUpdateRequest `json:"updates"`
}

// FileSystemItem represents a file or directory in the filesystem
type FileSystemItem struct {
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	IsDirectory    bool      `json:"is_directory"`
	Size           int64     `json:"size,omitempty"`
	ModTime        time.Time `json:"mod_time"`
	IsExcluded     bool      `json:"is_excluded"`
	AudiobookCount int       `json:"audiobook_count,omitempty"`
}

// BrowseRequest represents a filesystem browse request
type BrowseRequest struct {
	Path string `json:"path" form:"path"`
}

// ExclusionRequest represents a folder exclusion request
type ExclusionRequest struct {
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
}

// SystemStatus represents current system status
type SystemStatus struct {
	Version          string           `json:"version"`
	Uptime           string           `json:"uptime"`
	DatabasePath     string           `json:"database_path"`
	TotalBooks       int              `json:"total_books"`
	TotalAuthors     int              `json:"total_authors"`
	TotalSeries      int              `json:"total_series"`
	ImportPaths      int              `json:"import_paths"`
	ActiveOperations int              `json:"active_operations"`
	DiskUsage        map[string]int64 `json:"disk_usage"`
}

// LogEntry represents a system log entry
type LogEntry struct {
	Level     string    `json:"level"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Source    string    `json:"source,omitempty"`
}
