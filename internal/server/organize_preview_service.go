// file: internal/server/organize_preview_service.go
// version: 1.3.0
// guid: f1a2b3c4-d5e6-7890-abcd-ef1234567890

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// OrganizePreviewStep describes a single step in the organize preview.
type OrganizePreviewStep struct {
	Action      string                 `json:"action"`
	Description string                 `json:"description"`
	From        string                 `json:"from,omitempty"`
	To          string                 `json:"to,omitempty"`
	Files       []string               `json:"files,omitempty"`
	Tags        map[string]interface{} `json:"tags,omitempty"`
	CoverURL    string                 `json:"cover_url,omitempty"`
	Warning     string                 `json:"warning,omitempty"`
}

// OrganizePreviewResponse is the full response for a preview-organize request.
type OrganizePreviewResponse struct {
	Steps         []OrganizePreviewStep `json:"steps"`
	NeedsCopy     bool                  `json:"needs_copy"`
	NeedsRename   bool                  `json:"needs_rename"`
	CurrentPath   string                `json:"current_path"`
	TargetPath    string                `json:"target_path"`
	IsProtected   bool                  `json:"is_protected"`
	HasBookFiles  bool                  `json:"has_book_files"`
	BookFileCount int                   `json:"book_file_count"`
}

// OrganizePreviewService builds a read-only preview of what a single-book organize would do.
type OrganizePreviewService struct {
	db database.Store
}

// NewOrganizePreviewService creates a new OrganizePreviewService.
func NewOrganizePreviewService(db database.Store) *OrganizePreviewService {
	return &OrganizePreviewService{db: db}
}

// PreviewOrganize computes all steps without executing them.
func (ops *OrganizePreviewService) PreviewOrganize(bookID string) (*OrganizePreviewResponse, error) {
	book, err := ops.db.GetBookByID(bookID)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found: %s", bookID)
	}

	org := organizer.NewOrganizer(&config.AppConfig)
	currentPath := book.FilePath

	// Always fetch book_files — this is the authoritative source for what files
	// belong to this book. We use it for both directory books and single-file books
	// (a single-file book has exactly one book_file entry).
	var bookFiles []database.BookFile
	var activeBookFiles []database.BookFile
	bookFiles, _ = ops.db.GetBookFiles(bookID)
	for _, bf := range bookFiles {
		if bf.FilePath != "" && !bf.Missing {
			activeBookFiles = append(activeBookFiles, bf)
		}
	}

	// Determine whether this is a directory-based (multi-file) book.
	// Prefer book_files count over path extension / os.Stat.
	isDirectoryBook := len(activeBookFiles) > 1 || isDirectoryPath(currentPath)

	// Compute the target path. Directory books use the folder naming pattern only.
	var targetPath string
	if isDirectoryBook {
		targetPath, err = org.GenerateTargetDirPath(book)
	} else {
		targetPath, err = org.GenerateTargetPath(book)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to compute target path: %w", err)
	}

	protected := isProtectedPath(currentPath)
	alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(currentPath, config.AppConfig.RootDir)
	needsCopy := false
	needsRename := currentPath != targetPath

	// Determine if copy is needed: file is outside the library root
	if !alreadyInRoot {
		needsCopy = true
	}

	// If the file is already at the target, no copy or rename needed
	if currentPath == targetPath {
		needsCopy = false
		needsRename = false
	}

	var steps []OrganizePreviewStep

	// Step 1: Protected path warning (only for books outside RootDir)
	if protected && !alreadyInRoot {
		if isDirectoryBook && len(activeBookFiles) > 0 {
			steps = append(steps, OrganizePreviewStep{
				Action:      "warning",
				Description: fmt.Sprintf("Source is a flat iTunes author directory shared by multiple books. Only the %d files belonging to this book will be copied; the original directory and other books' files will not be modified.", len(activeBookFiles)),
				Warning:     "flat_itunes_directory",
			})
		} else {
			steps = append(steps, OrganizePreviewStep{
				Action:      "warning",
				Description: "Source file is in a protected path (import/iTunes). A copy will be created; the original will not be modified.",
				Warning:     "protected_path",
			})
		}
	}

	// Step 2: Copy to library (only for books outside RootDir)
	if needsCopy && !alreadyInRoot {
		if isDirectoryBook && len(activeBookFiles) > 0 {
			// Flat iTunes directory: list the specific files that will be copied
			var fileNames []string
			for _, bf := range activeBookFiles {
				fileNames = append(fileNames, filepath.Base(bf.FilePath))
			}
			desc := fmt.Sprintf("Copy %d file(s) from flat iTunes author directory to library folder", len(activeBookFiles))
			steps = append(steps, OrganizePreviewStep{
				Action:      "copy",
				Description: desc,
				From:        currentPath,
				To:          targetPath,
				Files:       fileNames,
			})
		} else {
			desc := "Copy to library folder"
			if protected {
				desc = "Copy from protected source to library folder"
			}
			steps = append(steps, OrganizePreviewStep{
				Action:      "copy",
				Description: desc,
				From:        currentPath,
				To:          targetPath,
			})
		}
	}

	// Step 3: Rename — for books already in RootDir that need a path update,
	// or for non-copy cases where the path differs.
	if needsRename && !needsCopy {
		desc := "Rename file"
		if alreadyInRoot {
			desc = "Rename to match updated metadata"
		}
		steps = append(steps, OrganizePreviewStep{
			Action:      "rename",
			Description: desc,
			From:        currentPath,
			To:          targetPath,
		})
	}

	// Step 4: Write tags — build the tag map
	rs := &RenameService{db: ops.db}
	authorName, _ := resolveAuthorAndSeriesNames(book)
	narratorStr := rs.resolveNarratorNames(bookID, book)
	tagMeta := rs.buildTagMetadata(book, authorName, narratorStr)

	if len(tagMeta) > 0 {
		steps = append(steps, OrganizePreviewStep{
			Action:      "write_tags",
			Description: "Write metadata tags",
			Tags:        tagMeta,
		})
	}

	// Step 5: Embed cover
	if book.CoverURL != nil && *book.CoverURL != "" {
		steps = append(steps, OrganizePreviewStep{
			Action:      "embed_cover",
			Description: "Embed cover art",
			CoverURL:    *book.CoverURL,
		})
	}

	return &OrganizePreviewResponse{
		Steps:         steps,
		NeedsCopy:     needsCopy,
		NeedsRename:   needsRename,
		CurrentPath:   currentPath,
		TargetPath:    targetPath,
		IsProtected:   protected,
		HasBookFiles:  len(activeBookFiles) > 0,
		BookFileCount: len(activeBookFiles),
	}, nil
}

// isDirectoryPath returns true if the given path looks like a directory.
// It checks the path extension first (no audio extension → likely a directory),
// then falls back to os.Stat if needed.
func isDirectoryPath(path string) bool {
	if path == "" {
		return false
	}
	audioExts := map[string]bool{
		".m4b": true, ".m4a": true, ".mp3": true, ".flac": true,
		".ogg": true, ".opus": true, ".wma": true, ".aac": true,
		".wav": true,
	}
	ext := strings.ToLower(filepath.Ext(path))
	if audioExts[ext] {
		return false
	}
	// No audio extension — confirm via stat if path exists
	if info, err := os.Stat(path); err == nil {
		return info.IsDir()
	}
	// Path doesn't exist yet (unlikely in preview) — treat no-ext paths as dirs
	return ext == ""
}
