// file: internal/server/audiobooks_compat.go
// version: 1.0.0
// guid: b1c2d3e4-f5a6-7890-bcde-f01234560020
// last-edited: 2026-05-05
//
// Type aliases and function variables that let the rest of internal/server/
// continue using the old unqualified names after the seven service files
// were moved to internal/audiobooks/. This file is the only place that
// needs updating if a moved symbol is later renamed.

package server

import (
	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"strings"
	"time"
)

// --- AudiobookService -------------------------------------------------------

type (
	// AudiobookService is a type alias for the moved service.
	AudiobookService = audiobookspkg.AudiobookService

	// AudiobooksListResponse is re-exported from the audiobooks package.
	AudiobooksListResponse = audiobookspkg.AudiobooksListResponse

	// AudiobookDetail is re-exported from the audiobooks package.
	AudiobookDetail = audiobookspkg.AudiobookDetail

	// DuplicatesResult is re-exported from the audiobooks package.
	DuplicatesResult = audiobookspkg.DuplicatesResult

	// SoftDeletedBooksResponse is re-exported from the audiobooks package.
	SoftDeletedBooksResponse = audiobookspkg.SoftDeletedBooksResponse

	// PurgeResult is re-exported from the audiobooks package.
	PurgeResult = audiobookspkg.PurgeResult

	// AudiobookUpdate is re-exported from the audiobooks package.
	AudiobookUpdate = audiobookspkg.AudiobookUpdate

	// OverridePayload is re-exported from the audiobooks package.
	OverridePayload = audiobookspkg.OverridePayload

	// FieldFilter is re-exported from the audiobooks package.
	FieldFilter = audiobookspkg.FieldFilter

	// ListFilters is re-exported from the audiobooks package.
	ListFilters = audiobookspkg.ListFilters

	// UpdateAudiobookRequest is re-exported from the audiobooks package.
	UpdateAudiobookRequest = audiobookspkg.UpdateAudiobookRequest

	// DeleteAudiobookOptions is re-exported from the audiobooks package.
	DeleteAudiobookOptions = audiobookspkg.DeleteAudiobookOptions
)

// PerUserFieldNames is re-exported from the audiobooks package.
var PerUserFieldNames = audiobookspkg.PerUserFieldNames

// IsPerUserField delegates to the audiobooks package.
func IsPerUserField(field string) bool {
	return audiobookspkg.IsPerUserField(field)
}

// NewAudiobookService delegates to the audiobooks package.
func NewAudiobookService(store database.Store) *AudiobookService {
	return audiobookspkg.NewAudiobookService(store)
}

// --- AudiobookUpdateService -------------------------------------------------

type (
	// AudiobookUpdateService is re-exported from the audiobooks package.
	AudiobookUpdateService = audiobookspkg.AudiobookUpdateService
)

// NewAudiobookUpdateService delegates to the audiobooks package.
func NewAudiobookUpdateService(db database.Store) *AudiobookUpdateService {
	return audiobookspkg.NewAudiobookUpdateService(db)
}

// --- AuthorSeriesService ----------------------------------------------------

type (
	// AuthorSeriesService is re-exported from the audiobooks package.
	AuthorSeriesService = audiobookspkg.AuthorSeriesService

	// AuthorWithCount is re-exported from the audiobooks package.
	AuthorWithCount = audiobookspkg.AuthorWithCount

	// AuthorListResponse is re-exported from the audiobooks package.
	AuthorListResponse = audiobookspkg.AuthorListResponse

	// AuthorWithCountListResponse is re-exported from the audiobooks package.
	AuthorWithCountListResponse = audiobookspkg.AuthorWithCountListResponse

	// SeriesWithCount is re-exported from the audiobooks package.
	SeriesWithCount = audiobookspkg.SeriesWithCount

	// SeriesListResponse is re-exported from the audiobooks package.
	SeriesListResponse = audiobookspkg.SeriesListResponse

	// SeriesWithCountsResponse is re-exported from the audiobooks package.
	SeriesWithCountsResponse = audiobookspkg.SeriesWithCountsResponse
)

// NewAuthorSeriesService delegates to the audiobooks package.
func NewAuthorSeriesService(db database.Store) *AuthorSeriesService {
	return audiobookspkg.NewAuthorSeriesService(db)
}

// --- OrganizeService --------------------------------------------------------

type (
	// OrganizeService is re-exported from the audiobooks package.
	OrganizeService = audiobookspkg.OrganizeService

	// OrganizeRequest is re-exported from the audiobooks package.
	OrganizeRequest = audiobookspkg.OrganizeRequest

	// OrganizeStats is re-exported from the audiobooks package.
	OrganizeStats = audiobookspkg.OrganizeStats
)

// NewOrganizeService delegates to the audiobooks package.
func NewOrganizeService(db database.Store) *OrganizeService {
	return audiobookspkg.NewOrganizeService(db)
}

// --- OrganizePreviewService -------------------------------------------------

type (
	// OrganizePreviewStep is re-exported from the audiobooks package.
	OrganizePreviewStep = audiobookspkg.OrganizePreviewStep

	// OrganizePreviewResponse is re-exported from the audiobooks package.
	OrganizePreviewResponse = audiobookspkg.OrganizePreviewResponse

	// OrganizePreviewService is re-exported from the audiobooks package.
	OrganizePreviewService = audiobookspkg.OrganizePreviewService
)

// NewOrganizePreviewService delegates to the audiobooks package.
func NewOrganizePreviewService(db database.Store) *OrganizePreviewService {
	return audiobookspkg.NewOrganizePreviewService(db)
}

// --- RevertService ----------------------------------------------------------

type (
	// RevertService is re-exported from the audiobooks package.
	RevertService = audiobookspkg.RevertService
)

// NewRevertService delegates to the audiobooks package.
func NewRevertService(db database.Store) *RevertService {
	return audiobookspkg.NewRevertService(db)
}

// --- RenameService ----------------------------------------------------------

type (
	// RenameService is re-exported from the audiobooks package.
	RenameService = audiobookspkg.RenameService

	// TagChange is re-exported from the audiobooks package.
	TagChange = audiobookspkg.TagChange

	// RenamePreview is re-exported from the audiobooks package.
	RenamePreview = audiobookspkg.RenamePreview

	// RenameApplyResult is re-exported from the audiobooks package.
	RenameApplyResult = audiobookspkg.RenameApplyResult
)

// NewRenameService delegates to the audiobooks package.
func NewRenameService(db database.Store) *RenameService {
	return audiobookspkg.NewRenameService(db)
}

// applyOverrideToPayload delegates to the audiobooks package.
// Kept unexported so server-package whitebox tests can reference it directly.
var applyOverrideToPayload = audiobookspkg.ApplyOverrideToPayload

// --- Small helpers previously in audiobook_service.go -----------------------
// These are used by operations_handlers.go and playlist_evaluator.go which
// remain in the server package after the service files were extracted.

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func derefInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func cmpTime(a, b *time.Time) int {
	ta := time.Time{}
	tb := time.Time{}
	if a != nil {
		ta = *a
	}
	if b != nil {
		tb = *b
	}
	switch {
	case ta.Before(tb):
		return -1
	case ta.After(tb):
		return 1
	default:
		return 0
	}
}

func splitMultipleNames(name string) []string {
	parts := strings.Split(name, " & ")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{name}
	}
	return result
}
