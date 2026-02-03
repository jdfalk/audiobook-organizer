// file: internal/server/import_service.go
// version: 1.1.0
// guid: d0e1f2a3-b4c5-6d7e-8f9a-0b1c2d3e4f5a

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

type ImportService struct {
	db database.Store
}

func NewImportService(db database.Store) *ImportService {
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

	// Extract metadata
	meta, err := metadata.ExtractMetadata(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Create book record
	book := &database.Book{
		Title:            meta.Title,
		FilePath:         req.FilePath,
		OriginalFilename: stringPtr(filepath.Base(req.FilePath)),
	}

	// Set author if available
	if meta.Artist != "" {
		author, err := is.db.GetAuthorByName(meta.Artist)
		if err != nil {
			// Create new author
			author, err = is.db.CreateAuthor(meta.Artist)
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
			// Create new series
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

	return &ImportFileResponse{
		ID:       created.ID,
		Title:    created.Title,
		FilePath: created.FilePath,
	}, nil
}
