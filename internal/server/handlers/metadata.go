// file: internal/server/handlers/metadata.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-fabc-345678901234
// last-edited: 2026-06-01

package handlers

import (
	"encoding/json"

	"github.com/falkcorp/audiobook-organizer/internal/operations"
)

// BulkMetadataFetchV2Params is the JSON params for the v2 bulk_metadata_fetch op.
// Selection replaces the old BookIDs field: the client sends either
//   - book_ids: an explicit list of IDs (page-level selection), or
//   - filter: a FilterSpec that the server resolves to IDs at run time
//     with IsPrimaryVersion=true always applied.
type BulkMetadataFetchV2Params struct {
	Selection     operations.SelectionSpec `json:"selection"`
	PreferAudible bool                     `json:"prefer_audible"`
	SkipCached    bool                     `json:"skip_cached"`
}

// RatingPatchRequest is the JSON body for PATCH /api/v1/audiobooks/:id/rating.
// Each field is a json.RawMessage so the handler can distinguish null (clear)
// from absent (don't touch) from a numeric value.
type RatingPatchRequest struct {
	Overall     json.RawMessage `json:"overall"`
	Story       json.RawMessage `json:"story"`
	Performance json.RawMessage `json:"performance"`
	Notes       json.RawMessage `json:"notes"`
}
