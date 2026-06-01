// file: internal/server/handlers/itunes.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-123456789012
// last-edited: 2026-06-01

package handlers

import "github.com/jdfalk/audiobook-organizer/internal/itunes"

// ITunesValidateRequest represents a validation request for an iTunes library.
type ITunesValidateRequest struct {
	LibraryPath  string               `json:"library_path" binding:"required"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesValidateResponse summarizes validation results for an iTunes library.
type ITunesValidateResponse struct {
	TotalTracks     int      `json:"total_tracks"`
	AudiobookTracks int      `json:"audiobook_tracks"`
	AudiobookCount  int      `json:"audiobook_count"`
	FilesFound      int      `json:"files_found"`
	FilesMissing    int      `json:"files_missing"`
	MissingPaths    []string `json:"missing_paths,omitempty"`
	PathPrefixes    []string `json:"path_prefixes,omitempty"`
	DuplicateCount  int      `json:"duplicate_count"`
	EstimatedTime   string   `json:"estimated_import_time"`
}

// ITunesImportRequest represents a request to import an iTunes library.
type ITunesImportRequest struct {
	LibraryPath      string               `json:"library_path" binding:"required"`
	ImportMode       string               `json:"import_mode" binding:"required,oneof=organized import organize"`
	PreserveLocation bool                 `json:"preserve_location"`
	ImportPlaylists  bool                 `json:"import_playlists"`
	SkipDuplicates   bool                 `json:"skip_duplicates"`
	FetchMetadata    bool                 `json:"fetch_metadata"`
	PathMappings     []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesImportResponse acknowledges an iTunes import operation.
type ITunesImportResponse struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// ITunesWriteBackRequest represents a write-back request for iTunes ITL updates.
type ITunesWriteBackRequest struct {
	LibraryPath  string               `json:"library_path"`
	AudiobookIDs []string             `json:"audiobook_ids"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesWriteBackResponse reports the result of an ITL write-back.
type ITunesWriteBackResponse struct {
	Success      bool   `json:"success"`
	UpdatedCount int    `json:"updated_count"`
	Message      string `json:"message"`
}

// ITunesBookMapping is a single book-to-iTunes-path mapping used in preview.
//
// Four path columns surface the full picture so users can see exactly what
// is currently in iTunes vs what AO has on disk vs what AO would write back:
//
//   - ITunesPath               — what iTunes currently has, e.g. W:/foo/bar.m4b
//   - ITunesPathTranslated     — local equivalent of ITunesPath after applying
//     forward path mappings (so users can stat it)
//   - AOPath                   — where AO has the file on disk (book.FilePath)
//   - AOITunesTranslatedPath   — what AO will write into the iTunes ITL when
//     write-back runs (ReverseRemapPath of AOPath)
//
// PathDiffers is true iff AOITunesTranslatedPath != ITunesPath — i.e. the
// thing AO wants to write does not match what iTunes already has.
//
// Backwards compatibility: LocalPath is preserved as an alias of AOPath so
// older clients keep working through the migration. Remove once no caller
// reads it.
type ITunesBookMapping struct {
	BookID                 string `json:"book_id"`
	Title                  string `json:"title"`
	Author                 string `json:"author"`
	ITunesPersistentID     string `json:"itunes_persistent_id"`
	ITunesPath             string `json:"itunes_path,omitempty"`
	ITunesPathTranslated   string `json:"itunes_path_translated,omitempty"`
	AOPath                 string `json:"ao_path"`
	AOITunesTranslatedPath string `json:"ao_itunes_translated_path,omitempty"`
	PathDiffers            bool   `json:"path_differs,omitempty"`

	// LocalPath duplicates AOPath for backwards compatibility with the
	// previous response shape. Will be removed once no caller reads it.
	LocalPath string `json:"local_path"`
}

// ITunesWriteBackPreviewRequest is the wire type for POST /itunes/write-back-preview.
//
// LibraryPath is now optional — when empty, the handler uses the configured
// ITunesLibraryReadPath. The dialog used to require the user to type this
// path on every preview, which was confusing because the actual write-back
// always targets the configured ITunesLibraryWritePath (.itl) regardless.
type ITunesWriteBackPreviewRequest struct {
	LibraryPath string   `json:"library_path,omitempty"`
	BookIDs     []string `json:"book_ids,omitempty"`
}

// ITunesWriteBackPreviewResponse is returned by POST /itunes/write-back-preview.
type ITunesWriteBackPreviewResponse struct {
	Items []ITunesBookMapping `json:"items"`
	Total int                 `json:"total"`
}

// ITunesImportStatusResponse is returned by GET /itunes/import/:id.
type ITunesImportStatusResponse struct {
	OperationID string   `json:"operation_id"`
	Status      string   `json:"status"`
	Progress    int      `json:"progress"`
	Message     string   `json:"message"`
	TotalBooks  int      `json:"total_books"`
	Processed   int      `json:"processed"`
	Imported    int      `json:"imported"`
	Skipped     int      `json:"skipped"`
	Failed      int      `json:"failed"`
	Errors      []string `json:"errors,omitempty"`
}

// ITunesTestMappingRequest tests a single path mapping against the library.
type ITunesTestMappingRequest struct {
	LibraryPath string `json:"library_path" binding:"required"`
	From        string `json:"from" binding:"required"`
	To          string `json:"to" binding:"required"`
}

// ITunesTestMappingResponse returns sample results from testing a mapping.
type ITunesTestMappingResponse struct {
	Tested   int                `json:"tested"`
	Found    int                `json:"found"`
	Examples []ITunesTestExample `json:"examples"`
}

// ITunesTestExample is a single found file example.
type ITunesTestExample struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

// ITunesSyncRequest is the wire type for POST /itunes/sync.
type ITunesSyncRequest struct {
	LibraryPath  string               `json:"library_path,omitempty"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
	Force        bool                 `json:"force,omitempty"`
}

// ITunesSyncResponse acknowledges a sync operation.
type ITunesSyncResponse struct {
	OperationID string `json:"operation_id"`
	Message     string `json:"message"`
}
