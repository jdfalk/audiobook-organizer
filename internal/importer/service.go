// file: internal/importer/service.go
// version: 1.0.1
// guid: d0e1f2a3-b4c5-6d7e-8f9a-0b1c2d3e4f5b
// last-edited: 2026-05-16

package importer

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/versions"
)

// Store is the narrow slice of database.Store this service uses.
// Temporarily widened to database.Store because versions.CreateIngestVersion
// requires the full Store interface. A future ISP pass on the versions package
// will re-narrow this.
type Store = database.Store

type ImportService struct {
	db          Store
	provisioner *itunesservice.TrackProvisioner
	dedupEngine *dedup.Engine
}

// SetTrackProvisioner wires the iTunes track provisioner for newly-imported
// books. Pass nil to disable ITL track provisioning (e.g. in tests).
func (is *ImportService) SetTrackProvisioner(p *itunesservice.TrackProvisioner) {
	is.provisioner = p
}

func (is *ImportService) SetDedupEngine(e *dedup.Engine) {
	is.dedupEngine = e
}

func NewImportService(db Store) *ImportService {
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
		normalizedArtist := dedup.NormalizeAuthorName(meta.Artist)
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
	if _, verErr := versions.CreateIngestVersion(is.db, versions.IngestVersionParams{
		BookID: created.ID, FilePath: created.FilePath,
		Format: created.Format, Source: "imported",
	}); verErr != nil {
		log.Printf("[WARN] create ingest version for %s: %v", created.ID, verErr)
	}

	// Provision ITL track via the injected iTunes service.
	// Nil provisioner → iTunes disabled or not wired; book is still created.
	if is.provisioner != nil {
		if err := is.provisioner.ProvisionAll(created); err != nil {
			log.Printf("[WARN] ITL track provisioning failed for %s: %v", created.ID, err)
		}
	}

	// Fire dedup check on import if dedup engine is wired. Run async so we don't
	// block the import API; dedup engine will create pending candidates for review.
	if is.dedupEngine != nil {
		go func(id string) {
			if _, err := is.dedupEngine.CheckBook(context.Background(), id); err != nil {
				log.Printf("[WARN] dedup-on-import CheckBook(%s): %v", id, err)
			}
		}(created.ID)
	}

	return &ImportFileResponse{
		ID:       created.ID,
		Title:    created.Title,
		FilePath: created.FilePath,
	}, nil
}

func stringPtr(s string) *string {
	return &s
}
