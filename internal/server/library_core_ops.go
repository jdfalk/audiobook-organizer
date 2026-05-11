// file: internal/server/library_core_ops.go
// version: 1.1.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

// library_core_ops registers the scan, organize, and transcode OperationDefs
// that previously went through the legacy BridgeQueue.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/transcode"
	ulid "github.com/oklog/ulid/v2"
)

type libraryScanParams struct {
	FolderPath  *string `json:"folder_path,omitempty"`
	ForceUpdate *bool   `json:"force_update,omitempty"`
}

type libraryOrganizeParams struct {
	FolderPath         *string  `json:"folder_path,omitempty"`
	BookIDs            []string `json:"book_ids,omitempty"`
	FetchMetadataFirst bool     `json:"fetch_metadata_first"`
	SyncITunesFirst    bool     `json:"sync_itunes_first"`
}

type libraryTranscodeParams struct {
	BookID       string `json:"book_id"`
	OutputFormat string `json:"output_format"`
	Bitrate      int    `json:"bitrate"`
	KeepOriginal bool   `json:"keep_original"`
}

// RegisterLibraryScanOp registers the "library.scan" v2 OperationDef.
func (s *Server) RegisterLibraryScanOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "library.scan",
		Plugin:          "library",
		DisplayName:     "Library Scan",
		Description:     "Scan the library root directory for new, changed, or removed audiobook files.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "library.scan",
		Permissions:     []auth.Permission{auth.PermScanTrigger},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p libraryScanParams
			if len(rawParams) > 0 {
				_ = json.Unmarshal(rawParams, &p)
			}
			scanReq := &scanner.ScanRequest{
				FolderPath:  p.FolderPath,
				ForceUpdate: p.ForceUpdate,
			}
			progress := registryProgressAdapter{r: reporter}
			return s.scanService.PerformScan(ctx, scanReq, operations.LoggerFromReporter(progress))
		},
	})
}

// RegisterLibraryOrganizeOp registers the "library.organize" v2 OperationDef.
func (s *Server) RegisterLibraryOrganizeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "library.organize",
		Plugin:          "library",
		DisplayName:     "Organize Library",
		Description:     "Move audiobook files into the canonical directory structure based on current metadata.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "library.organize",
		Permissions:     []auth.Permission{auth.PermScanTrigger},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapFilesWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p libraryOrganizeParams
			if len(rawParams) > 0 {
				_ = json.Unmarshal(rawParams, &p)
			}
			opID := ulid.Make().String()
			progress := registryProgressAdapter{r: reporter}
			organizeReq := &OrganizeRequest{
				FolderPath:         p.FolderPath,
				BookIDs:            p.BookIDs,
				FetchMetadataFirst: p.FetchMetadataFirst,
				SyncITunesFirst:    p.SyncITunesFirst,
				OperationID:        opID,
			}
			return s.organizeService.PerformOrganize(ctx, organizeReq, operations.LoggerFromReporter(progress))
		},
	})
}

// RegisterLibraryTranscodeOp registers the "library.transcode" v2 OperationDef.
func (s *Server) RegisterLibraryTranscodeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "library.transcode",
		Plugin:          "library",
		DisplayName:     "Transcode to M4B",
		Description:     "Transcode an audiobook file to M4B format and register it as a new version.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         6 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "", // transcodes can run in parallel
		Permissions:     []auth.Permission{auth.PermLibraryOrganize},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapFilesWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p libraryTranscodeParams
			if len(rawParams) > 0 {
				if err := json.Unmarshal(rawParams, &p); err != nil {
					return fmt.Errorf("transcode: decode params: %w", err)
				}
			}
			if p.BookID == "" {
				return fmt.Errorf("transcode: book_id is required")
			}

			progress := registryProgressAdapter{r: reporter}

			opts := transcode.TranscodeOpts{
				BookID:       p.BookID,
				OutputFormat: p.OutputFormat,
				Bitrate:      p.Bitrate,
				KeepOriginal: p.KeepOriginal,
			}

			outputPath, err := transcode.Transcode(ctx, opts, s.Store(), progress)
			if err != nil {
				return err
			}

			originalBook, err := s.Store().GetBookByID(p.BookID)
			if err != nil {
				return fmt.Errorf("failed to get original book: %w", err)
			}

			groupID := ""
			if originalBook.VersionGroupID != nil && *originalBook.VersionGroupID != "" {
				groupID = *originalBook.VersionGroupID
			} else {
				groupID = ulid.Make().String()
			}

			notPrimary := false
			origNotes := "Original format"
			originalBook.IsPrimaryVersion = &notPrimary
			originalBook.VersionGroupID = &groupID
			originalBook.VersionNotes = &origNotes
			if _, err := s.Store().UpdateBook(p.BookID, originalBook); err != nil {
				progress.Log("warn", fmt.Sprintf("Failed to update original book version info: %v", err), nil)
			}

			m4bFormat := "m4b"
			aacCodec := "aac"
			bitrateVal := opts.Bitrate
			if bitrateVal <= 0 {
				bitrateVal = 128
			}
			isPrimary := true
			m4bNotes := "Transcoded to M4B"

			newBook := &database.Book{
				ID:                   ulid.Make().String(),
				Title:                originalBook.Title,
				FilePath:             outputPath,
				Format:               m4bFormat,
				Codec:                &aacCodec,
				Bitrate:              &bitrateVal,
				AuthorID:             originalBook.AuthorID,
				SeriesID:             originalBook.SeriesID,
				SeriesSequence:       originalBook.SeriesSequence,
				Duration:             originalBook.Duration,
				Narrator:             originalBook.Narrator,
				Publisher:            originalBook.Publisher,
				PrintYear:            originalBook.PrintYear,
				AudiobookReleaseYear: originalBook.AudiobookReleaseYear,
				ISBN10:               originalBook.ISBN10,
				ISBN13:               originalBook.ISBN13,
				ASIN:                 originalBook.ASIN,
				Language:             originalBook.Language,
				CoverURL:             originalBook.CoverURL,
				IsPrimaryVersion:     &isPrimary,
				VersionGroupID:       &groupID,
				VersionNotes:         &m4bNotes,
			}
			if _, err := s.Store().CreateBook(newBook); err != nil {
				progress.Log("warn", fmt.Sprintf("Failed to create M4B version record, updating original: %v", err), nil)
				isPrim := true
				fallbackNotes := fmt.Sprintf("Transcoded to M4B (in-place, original was at %s)", originalBook.FilePath)
				originalBook.FilePath = outputPath
				originalBook.Format = m4bFormat
				originalBook.Codec = &aacCodec
				originalBook.Bitrate = &bitrateVal
				originalBook.IsPrimaryVersion = &isPrim
				originalBook.VersionGroupID = &groupID
				originalBook.VersionNotes = &fallbackNotes
				if _, updateErr := s.Store().UpdateBook(p.BookID, originalBook); updateErr != nil {
					return updateErr
				}
				return nil
			}

			progress.Log("info", fmt.Sprintf("Created M4B version %s (group %s), original %s demoted to non-primary", newBook.ID, groupID, p.BookID), nil)

			if !config.AppConfig.ITLWriteBackEnabled &&
				originalBook.ITunesPersistentID != nil &&
				*originalBook.ITunesPersistentID != "" {
				if err := s.Store().CreateDeferredITunesUpdate(
					originalBook.ID,
					*originalBook.ITunesPersistentID,
					originalBook.FilePath,
					newBook.FilePath,
					"transcode",
				); err != nil {
					progress.Log("warn", fmt.Sprintf("Failed to create deferred iTunes update: %v", err), nil)
				} else {
					progress.Log("info", "M4B created. iTunes library update deferred until write-back is enabled.", nil)
				}
			}

			return nil
		},
	})
}

func init() {
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterLibraryScanOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterLibraryOrganizeOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterLibraryTranscodeOp(reg) })
}
