// file: internal/server/import_service.go
// version: 1.3.0
// guid: d0e1f2a3-b4c5-6d7e-8f9a-0b1c2d3e4f5a

package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// importServiceStore is the narrow slice of database.Store this service uses.
// Includes the transitive surfaces required by forwarded helpers:
// CreateIngestVersion needs BookVersionStore + BookFileStore; ProvisionITLTracksForBook
// needs ExternalIDStore (plus the AuthorReader + BookFileStore already present).
type importServiceStore interface {
	database.AuthorReader
	database.AuthorWriter
	database.BookWriter
	database.SeriesReader
	database.SeriesWriter
	database.BookVersionStore
	database.BookFileStore
	database.ExternalIDStore
}


type ImportService struct {
	db importServiceStore
	writeBackBatcher *WriteBackBatcher
}

// SetWriteBackBatcher sets the iTunes write-back batcher.
func (is *ImportService) SetWriteBackBatcher(b *WriteBackBatcher) {
	is.writeBackBatcher = b
}

func NewImportService(db importServiceStore) *ImportService {
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

	// Extract metadata — use folder-aware assembly for generic part filenames.
	var meta metadata.Metadata
	if metadata.IsGenericPartFilename(req.FilePath) {
		dirPath := filepath.Dir(req.FilePath)
		firstFile := metadata.FindFirstAudioFile(dirPath, config.AppConfig.SupportedExtensions)
		if firstFile == "" {
			firstFile = req.FilePath
		}
		bm, bmErr := metadata.AssembleBookMetadata(dirPath, firstFile, 0, 0)
		if bmErr != nil {
			return nil, fmt.Errorf("failed to assemble metadata: %w", bmErr)
		}
		meta = metadata.Metadata{
			Title:       bm.Title,
			Artist:      bm.PrimaryAuthor(),
			Series:      bm.SeriesName,
			SeriesIndex: bm.SeriesPosition,
			Narrator:    bm.Narrator,
			Language:    bm.Language,
			Publisher:   bm.Publisher,
			ISBN10:      bm.ISBN10,
			ISBN13:      bm.ISBN13,
		}
	} else {
		var err error
		meta, err = metadata.ExtractMetadata(req.FilePath, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to extract metadata: %w", err)
		}
	}

	// Create book record
	book := &database.Book{
		Title:            meta.Title,
		FilePath:         req.FilePath,
		OriginalFilename: stringPtr(filepath.Base(req.FilePath)),
	}

	// Set author if available
	if meta.Artist != "" {
		normalizedArtist := NormalizeAuthorName(meta.Artist)
		author, err := is.db.GetAuthorByName(normalizedArtist)
		if err != nil {
			author, err = is.db.CreateAuthor(normalizedArtist)
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
	if meta.Narrator != "" {
		book.Narrator = stringPtr(meta.Narrator)
	}
	if meta.Language != "" {
		book.Language = stringPtr(meta.Language)
	}
	if meta.Publisher != "" {
		book.Publisher = stringPtr(meta.Publisher)
	}
	if meta.ISBN10 != "" {
		book.ISBN10 = stringPtr(meta.ISBN10)
	}
	if meta.ISBN13 != "" {
		book.ISBN13 = stringPtr(meta.ISBN13)
	}

	// Create book in database
	created, err := is.db.CreateBook(book)
	if err != nil {
		return nil, fmt.Errorf("failed to create book: %w", err)
	}

	// Create version row for the imported file (spec 3.1).
	if _, verErr := CreateIngestVersion(is.db, IngestVersionParams{
		BookID: created.ID, FilePath: created.FilePath,
		Format: created.Format, Source: "imported",
	}); verErr != nil {
		log.Printf("[WARN] create ingest version for %s: %v", created.ID, verErr)
	}

	// Provision ITL track (generates PID, stores in external_id_map, enqueues add)
	if err := ProvisionITLTracksForBook(is.db, created, is.writeBackBatcher); err != nil {
		// Non-fatal: book was created, ITL provisioning can be retried
		log.Printf("[WARN] ITL track provisioning failed for %s: %v", created.ID, err)
	}

	return &ImportFileResponse{
		ID:       created.ID,
		Title:    created.Title,
		FilePath: created.FilePath,
	}, nil
}
