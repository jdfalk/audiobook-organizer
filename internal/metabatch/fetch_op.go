// file: internal/metabatch/fetch_op.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a
// last-edited: 2026-05-11
//
// FetchOpParams holds the serializable parameters for the
// metadata.candidate-fetch v2 OperationDef. Kept here so the
// server package (which owns the handler and op registration) and
// any future recovery/replay tooling share the same type.

package metabatch

// FetchOpParams is the JSON params for the metadata.candidate-fetch
// v2 OperationDef. LegacyOpID is the v1 operation record ID written
// by the HTTP handler — OperationResult rows are keyed on this so
// that handleGetPendingReview, handleGetOperationResults, and the
// dedup scan in handleBatchFetchCandidates all continue working
// unchanged.
type FetchOpParams struct {
	LegacyOpID  string   `json:"legacy_op_id"`
	BookIDs     []string `json:"book_ids"`
	TotalBooks  int      `json:"total_books"`
	AlreadyDone int      `json:"already_done"` // used by resume path
}
