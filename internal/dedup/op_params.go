// file: internal/dedup/op_params.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

// Package dedup: op_params.go defines the JSON-unmarshal parameter structs for
// each of the 8 async dedup OperationDef Run functions. They are kept here so
// the extracted logic functions can reference them directly and so server-side
// wrappers can unmarshal into the same types.
package dedup

// BookDedupScanParams are the parameters for the "dedup.book-scan" operation.
type BookDedupScanParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

// BookMergeParams are the parameters for the "dedup.book-merge" operation.
type BookMergeParams struct {
	LegacyOpID string   `json:"legacy_op_id"`
	KeepID     string   `json:"keep_id"`
	MergeIDs   []string `json:"merge_ids"`
	Detail     string   `json:"detail"`
}

// AuthorDedupScanParams are the parameters for the "dedup.author-scan" operation.
type AuthorDedupScanParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

// SeriesDedupScanParams are the parameters for the "dedup.series-scan" operation.
type SeriesDedupScanParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

// SeriesDedupParams are the parameters for the "dedup.series-dedup" operation.
type SeriesDedupParams struct {
	LegacyOpID string `json:"legacy_op_id"`
	Detail     string `json:"detail"`
}

// SeriesPruneParams are the parameters for the "dedup.series-prune" operation.
type SeriesPruneParams struct {
	LegacyOpID string `json:"legacy_op_id"`
	Detail     string `json:"detail"`
}

// SeriesMergeParams are the parameters for the "dedup.series-merge" operation.
type SeriesMergeParams struct {
	LegacyOpID string `json:"legacy_op_id"`
	KeepID     int    `json:"keep_id"`
	MergeIDs   []int  `json:"merge_ids"`
	CustomName string `json:"custom_name"`
	Detail     string `json:"detail"`
}

// SeriesNormalizeParams are the parameters for the "dedup.series-normalize" operation.
type SeriesNormalizeParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}
