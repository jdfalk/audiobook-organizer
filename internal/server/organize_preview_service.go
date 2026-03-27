// file: internal/server/organize_preview_service.go
// version: 1.0.0
// guid: f1a2b3c4-d5e6-7890-abcd-ef1234567890

package server

import (
	"fmt"
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
	Tags        map[string]interface{} `json:"tags,omitempty"`
	CoverURL    string                 `json:"cover_url,omitempty"`
	Warning     string                 `json:"warning,omitempty"`
}

// OrganizePreviewResponse is the full response for a preview-organize request.
type OrganizePreviewResponse struct {
	Steps       []OrganizePreviewStep `json:"steps"`
	NeedsCopy   bool                  `json:"needs_copy"`
	NeedsRename bool                  `json:"needs_rename"`
	CurrentPath string                `json:"current_path"`
	TargetPath  string                `json:"target_path"`
	IsProtected bool                  `json:"is_protected"`
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
	targetPath, err := org.GenerateTargetPath(book)
	if err != nil {
		return nil, fmt.Errorf("failed to compute target path: %w", err)
	}

	currentPath := book.FilePath
	protected := isProtectedPath(currentPath)
	needsCopy := false
	needsRename := currentPath != targetPath

	// Determine if copy is needed: file is outside the library root
	if config.AppConfig.RootDir != "" && !strings.HasPrefix(currentPath, config.AppConfig.RootDir) {
		needsCopy = true
	}

	// If the file is already at the target, no copy or rename needed
	if currentPath == targetPath {
		needsCopy = false
		needsRename = false
	}

	var steps []OrganizePreviewStep

	// Step 1: Protected path warning
	if protected {
		steps = append(steps, OrganizePreviewStep{
			Action:      "warning",
			Description: "Source file is in a protected path (import/iTunes). A copy will be created; the original will not be modified.",
			Warning:     "protected_path",
		})
	}

	// Step 2: Copy to library
	if needsCopy {
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

	// Step 3: Rename (only if not copying — copy already places at target)
	if needsRename && !needsCopy {
		steps = append(steps, OrganizePreviewStep{
			Action:      "rename",
			Description: "Rename file",
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
		Steps:       steps,
		NeedsCopy:   needsCopy,
		NeedsRename: needsRename,
		CurrentPath: currentPath,
		TargetPath:  targetPath,
		IsProtected: protected,
	}, nil
}
