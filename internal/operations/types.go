// file: internal/operations/types.go
// version: 1.0.0
// guid: f1e2d3c4-b5a6-7890-abcd-ef1234567890
// last-edited: 2026-05-11
//
// SelectionSpec and related types for server-side bulk operation targeting.
// A SelectionSpec describes which books an operation targets without requiring
// the client to enumerate all IDs upfront.

package operations

// SelectionSpec describes which books an operation targets.
// Exactly one of BookIDs or Filter must be non-nil/non-empty.
// When Filter is set the server resolves it to book IDs at execution time
// with IsPrimaryVersion=true always applied.
type SelectionSpec struct {
	BookIDs []string    `json:"book_ids,omitempty"`
	Filter  *FilterSpec `json:"filter,omitempty"`
}

// FilterSpec mirrors the query params accepted by GET /api/v1/audiobooks.
// When Filter is set on a SelectionSpec, the server resolves it to book IDs
// at operation execution time with IsPrimaryVersion=true always applied.
type FilterSpec struct {
	Search       string        `json:"search,omitempty"`
	LibraryState string        `json:"library_state,omitempty"`
	Tag          string        `json:"tag,omitempty"`
	FieldFilters []FieldFilter `json:"field_filters,omitempty"`
	AuthorID     *int64        `json:"author_id,omitempty"`
	SeriesID     *int64        `json:"series_id,omitempty"`
}

// FieldFilter mirrors the server-side FieldFilter used for advanced search.
type FieldFilter struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Negated bool   `json:"negated"`
}
