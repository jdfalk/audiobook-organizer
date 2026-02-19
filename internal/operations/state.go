// file: internal/operations/state.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

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

// SaveCheckpoint persists an operation's progress checkpoint.
func SaveCheckpoint(store database.Store, opID, opType, phase string, index, total int) error {
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
func LoadCheckpoint(store database.Store, opID string) (*OperationState, error) {
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
func SaveParams(store database.Store, opID string, params any) error {
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

// ClearState removes all persisted state for an operation (called on completion/failure).
func ClearState(store database.Store, opID string) error {
	return store.DeleteOperationState(opID)
}
