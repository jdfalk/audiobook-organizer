// file: internal/server/response_types.go
// version: 2.0.0
// guid: 7f8a9b0c-1d2e-3f4a-5b6c-7d8e9f0a1b2c
// last-edited: 2026-05-01

package server

// AudiobookResponse provides a consistent format for audiobook responses.
type AudiobookResponse struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Author              string   `json:"author,omitempty"`
	Series              string   `json:"series,omitempty"`
	SeriesSequence      *int     `json:"series_sequence,omitempty"`
	FilePath            string   `json:"file_path,omitempty"`
	Format              string   `json:"format,omitempty"`
	Duration            int64    `json:"duration,omitempty"`
	ReleaseYear         *int     `json:"release_year,omitempty"`
	Genre               string   `json:"genre,omitempty"`
	Narrators           string   `json:"narrators,omitempty"`
	Publisher           string   `json:"publisher,omitempty"`
	Language            string   `json:"language,omitempty"`
	CoverArtPath        string   `json:"cover_art_path,omitempty"`
	Description         string   `json:"description,omitempty"`
	Rating              *float64 `json:"rating,omitempty"`
	TagList             []string `json:"tags,omitempty"`
	IsMarkedForDeletion bool     `json:"is_marked_for_deletion,omitempty"`
	IsAudiobook         bool     `json:"is_audiobook,omitempty"`
}

// WorkResponse provides a consistent format for work responses.
type WorkResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	BookCount   int    `json:"book_count,omitempty"`
}

// AuthorResponse provides a consistent format for author responses.
type AuthorResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"book_count,omitempty"`
}

// SeriesResponse provides a consistent format for series responses.
type SeriesResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	AuthorID  int    `json:"author_id"`
	BookCount int    `json:"book_count,omitempty"`
}

// DuplicateGroup represents a group of duplicate audiobooks.
type DuplicateGroup struct {
	Key     string          `json:"key"`
	Items   int             `json:"items"`
	Details []DuplicateItem `json:"details,omitempty"`
}

// DuplicateItem represents a single item in a duplicate group.
type DuplicateItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

// DuplicatesResponse provides a consistent format for duplicates responses.
type DuplicatesResponse struct {
	Groups         []DuplicateGroup `json:"groups"`
	GroupCount     int              `json:"group_count"`
	DuplicateCount int              `json:"duplicate_count"`
}

// HealthResponse provides a consistent format for health check responses.
type HealthResponse struct {
	Status    string `json:"status"`
	Uptime    int64  `json:"uptime_seconds"`
	Timestamp int64  `json:"timestamp"`
}
