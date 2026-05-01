// file: internal/operations/state.go
// version: 1.5.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-03

package operations

import (
	"encoding/json"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// OperationState is the resumable checkpoint for any operation.
type OperationState struct {
	OperationID string    `json:"operation_id"`
	Type        string    `json:"type"`        // "scan", "organize", "itunes_import"
	Phase       string    `json:"phase"`       // e.g. "grouping", "importing", "enriching", "organizing"
	PhaseIndex  int       `json:"phase_index"` // current item index within phase
	PhaseTotal  int       `json:"phase_total"` // total items in current phase
	Status      string    `json:"status"`      // "running", "interrupted"
	UpdatedAt   time.Time `json:"updated_at"`
}

// ITunesImportParams stores the immutable parameters for an iTunes import operation.
type ITunesImportParams struct {
	LibraryXMLPath string            `json:"library_xml_path"`
	LibraryPath    string            `json:"library_path"`
	ImportMode     string            `json:"import_mode"`
	PathMappings   map[string]string `json:"path_mappings,omitempty"`
	SkipDuplicates bool              `json:"skip_duplicates"`
	EnrichMetadata bool              `json:"enrich_metadata"`
	AutoOrganize   bool              `json:"auto_organize"`
}

// ScanParams stores the immutable parameters for a scan operation.
type ScanParams struct {
	FolderPath  *string `json:"folder_path,omitempty"`
	ForceUpdate bool    `json:"force_update"`
}

// OrganizeParams stores the immutable parameters for an organize operation.
type OrganizeParams struct {
	Strategy string `json:"strategy,omitempty"`
}

// BulkWriteBackParams stores the immutable parameters for a bulk write-back operation.
// Resume diffs the book IDs against the operation's checkpoint PhaseIndex.
type BulkWriteBackParams struct {
	BookIDs []string `json:"book_ids"`
	Rename  bool     `json:"rename"`
}

// IsbnEnrichmentParams stores the immutable parameters for an ISBN enrichment operation.
// BookIDs is the list to enrich; on resume, the checkpoint PhaseIndex is the next index.
type IsbnEnrichmentParams struct {
	BookIDs []string `json:"book_ids"`
}

// MetadataRefreshParams stores the immutable parameters for a metadata refresh operation.
type MetadataRefreshParams struct {
	BookIDs []string `json:"book_ids"`
	Source  string   `json:"source,omitempty"`
}

// ComposerScanParams stores the immutable parameters for a composer-tag scan operation.
type ComposerScanParams struct {
	DryRun  bool   `json:"dry_run"`
	FixMode string `json:"fix_mode"` // "set_narrator" or "clear"
}

// BulkMetadataFetchParams stores the immutable parameters for a bulk metadata fetch operation.
// PreferAudible=true moves Audible to the front of the source chain.
// SkipCached=true skips books that already have a valid (non-expired) cache entry.
type BulkMetadataFetchParams struct {
	PreferAudible bool `json:"prefer_audible"`
	SkipCached    bool `json:"skip_cached"`
}

// MissingFileRepairParams stores the immutable parameters for a missing-file path-repair operation.
// DryRun=true reports what would be fixed without writing. SearchRoots is the ordered list of
// directory trees to search when the PID lookup fails.
type MissingFileRepairParams struct {
	DryRun      bool     `json:"dry_run"`
	SearchRoots []string `json:"search_roots,omitempty"`
}

// BulkImportDelugeParams stores the immutable parameters for a bulk-import-deluge operation.
// DryRun=true reports what would be imported without making any changes.
// MaxBooks limits the number of books imported in one run (0 = unlimited).
type BulkImportDelugeParams struct {
	DryRun   bool `json:"dry_run"`
	MaxBooks int  `json:"max_books,omitempty"`
}

// SaveCheckpoint persists an operation's progress checkpoint.
func SaveCheckpoint(store database.OperationStore, opID, opType, phase string, index, total int) error {
	state := OperationState{
		OperationID: opID,
		Type:        opType,
		Phase:       phase,
		PhaseIndex:  index,
		PhaseTotal:  total,
		Status:      "running",
		UpdatedAt:   time.Now(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return store.SaveOperationState(opID, data)
}

// LoadCheckpoint loads an operation's progress checkpoint. Returns nil if none exists.
func LoadCheckpoint(store database.OperationStore, opID string) (*OperationState, error) {
	data, err := store.GetOperationState(opID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var state OperationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveParams persists an operation's immutable parameters.
func SaveParams(store database.OperationStore, opID string, params any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return store.SaveOperationParams(opID, data)
}

// LoadParams loads and deserializes an operation's parameters into the given pointer.
func LoadParams[T any](store database.Store, opID string) (*T, error) {
	data, err := store.GetOperationParams(opID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var params T
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// LoadRawParams loads an operation's raw JSON parameters.
// Returns nil if no params are stored.
func LoadRawParams(store database.OperationStore, opID string) (json.RawMessage, error) {
	data, err := store.GetOperationParams(opID)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SaveRawParams persists raw JSON parameters for an operation.
func SaveRawParams(store database.OperationStore, opID string, raw json.RawMessage) error {
	return store.SaveOperationParams(opID, raw)
}

// ClearState removes all persisted state for an operation (called on completion/failure).
func ClearState(store database.OperationStore, opID string) error {
	return store.DeleteOperationState(opID)
}
