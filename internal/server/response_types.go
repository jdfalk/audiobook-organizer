// file: internal/server/response_types.go
// version: 1.0.0
// guid: 7f8a9b0c-1d2e-3f4a-5b6c-7d8e9f0a1b2c

package server

// ListResponse provides a consistent format for paginated list responses
type ListResponse struct {
	Items  any `json:"items"`
	Count  int `json:"count"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total,omitempty"`
}

// ItemResponse provides a consistent format for single item responses
type ItemResponse struct {
	Data any `json:"data"`
}

// CreateResponse provides a consistent format for resource creation responses
type CreateResponse struct {
	ID   string `json:"id"`
	Data any    `json:"data,omitempty"`
}

// MessageResponse provides a consistent format for status messages
type MessageResponse struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// DeleteResponse provides a consistent format for deletion responses
type DeleteResponse struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}

// BulkResponse provides a consistent format for bulk operation responses
type BulkResponse struct {
	Total     int         `json:"total"`
	Succeeded int         `json:"succeeded"`
	Failed    int         `json:"failed"`
	Results   []BulkItem  `json:"results"`
}

// BulkItem represents a single item in a bulk operation response
type BulkItem struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // "success", "failed"
	Error    string `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// StatusResponse provides a consistent format for status check responses
type StatusResponse struct {
	Status string `json:"status"` // "ok", "degraded", "error"
	Code   string `json:"code,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// PaginationParams holds common pagination parameters
type PaginationParams struct {
	Limit  int
	Offset int
	Search string
}

// SortParams holds common sort parameters
type SortParams struct {
	Field string
	Order string // "asc" or "desc"
}

// NewListResponse creates a new ListResponse with pagination info
func NewListResponse(items any, count int, limit int, offset int) *ListResponse {
	return &ListResponse{
		Items:  items,
		Count:  count,
		Limit:  limit,
		Offset: offset,
		Total:  count, // Set total equal to count by default
	}
}

// NewListResponseWithTotal creates a new ListResponse with a distinct total
func NewListResponseWithTotal(items any, count int, limit int, offset int, total int) *ListResponse {
	return &ListResponse{
		Items:  items,
		Count:  count,
		Limit:  limit,
		Offset: offset,
		Total:  total,
	}
}

// NewBulkResponse creates a new BulkResponse
func NewBulkResponse(total int, results []BulkItem) *BulkResponse {
	succeeded := 0
	failed := 0
	for _, item := range results {
		if item.Status == "success" {
			succeeded++
		} else if item.Status == "failed" {
			failed++
		}
	}
	return &BulkResponse{
		Total:     total,
		Succeeded: succeeded,
		Failed:    failed,
		Results:   results,
	}
}

// NewMessageResponse creates a new MessageResponse
func NewMessageResponse(message string, code string) *MessageResponse {
	return &MessageResponse{
		Message: message,
		Code:    code,
	}
}

// NewStatusResponse creates a new StatusResponse
func NewStatusResponse(status string, data any) *StatusResponse {
	return &StatusResponse{
		Status: status,
		Data:   data,
	}
}

// AudiobookResponse provides a consistent format for audiobook responses
type AudiobookResponse struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Author          string  `json:"author,omitempty"`
	Series          string  `json:"series,omitempty"`
	SeriesSequence  *int    `json:"series_sequence,omitempty"`
	FilePath        string  `json:"file_path,omitempty"`
	Format          string  `json:"format,omitempty"`
	Duration        int64   `json:"duration,omitempty"`
	ReleaseYear     *int    `json:"release_year,omitempty"`
	Genre           string  `json:"genre,omitempty"`
	Narrators       string  `json:"narrators,omitempty"`
	Publisher       string  `json:"publisher,omitempty"`
	Language        string  `json:"language,omitempty"`
	CoverArtPath    string  `json:"cover_art_path,omitempty"`
	Description     string  `json:"description,omitempty"`
	Rating          *float64 `json:"rating,omitempty"`
	TagList         []string `json:"tags,omitempty"`
	IsMarkedForDeletion bool `json:"is_marked_for_deletion,omitempty"`
	IsAudiobook     bool    `json:"is_audiobook,omitempty"`
}

// WorkResponse provides a consistent format for work responses
type WorkResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	BookCount   int    `json:"book_count,omitempty"`
}

// AuthorResponse provides a consistent format for author responses
type AuthorResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"book_count,omitempty"`
}

// SeriesResponse provides a consistent format for series responses
type SeriesResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	AuthorID  int    `json:"author_id"`
	BookCount int    `json:"book_count,omitempty"`
}

// DuplicateGroup represents a group of duplicate audiobooks
type DuplicateGroup struct {
	Key     string `json:"key"`
	Items   int    `json:"items"`
	Details []DuplicateItem `json:"details,omitempty"`
}

// DuplicateItem represents a single item in a duplicate group
type DuplicateItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

// DuplicatesResponse provides a consistent format for duplicates responses
type DuplicatesResponse struct {
	Groups         []DuplicateGroup `json:"groups"`
	GroupCount     int              `json:"group_count"`
	DuplicateCount int              `json:"duplicate_count"`
}

// HealthResponse provides a consistent format for health check responses
type HealthResponse struct {
	Status   string `json:"status"`
	Uptime   int64  `json:"uptime_seconds"`
	Timestamp int64 `json:"timestamp"`
}
