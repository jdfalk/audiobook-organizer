// file: internal/server/metadata_state_service.go
// version: 1.0.0
// guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// MetadataStateService handles metadata field state operations
type MetadataStateService struct {
	db database.Store
}

// NewMetadataStateService creates a new metadata state service
func NewMetadataStateService(db database.Store) *MetadataStateService {
	return &MetadataStateService{db: db}
}

// metadataFieldState represents the state of a single metadata field
type metadataFieldState struct {
	FetchedValue   any       `json:"fetched_value,omitempty"`
	OverrideValue  any       `json:"override_value,omitempty"`
	OverrideLocked bool      `json:"override_locked"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LoadMetadataState loads the complete metadata state for a book
func (mss *MetadataStateService) LoadMetadataState(bookID string) (map[string]metadataFieldState, error) {
	state := map[string]metadataFieldState{}
	if mss.db == nil {
		return state, fmt.Errorf("database not initialized")
	}

	stored, err := mss.db.GetMetadataFieldStates(bookID)
	if err != nil {
		return state, err
	}

	for _, entry := range stored {
		state[entry.Field] = metadataFieldState{
			FetchedValue:   decodeMetadataValue(entry.FetchedValue),
			OverrideValue:  decodeMetadataValue(entry.OverrideValue),
			OverrideLocked: entry.OverrideLocked,
			UpdatedAt:      entry.UpdatedAt,
		}
	}

	if len(state) > 0 {
		return state, nil
	}

	// Try legacy metadata state
	legacy, err := mss.loadLegacyMetadataState(bookID)
	if err != nil {
		return state, err
	}

	if len(legacy) == 0 {
		return state, nil
	}

	// Migrate legacy state
	if err := mss.SaveMetadataState(bookID, legacy); err != nil {
		log.Printf("[WARN] failed to migrate legacy metadata state for %s: %v", bookID, err)
	}

	return legacy, nil
}

// SaveMetadataState persists metadata state to the database
func (mss *MetadataStateService) SaveMetadataState(bookID string, state map[string]metadataFieldState) error {
	if mss.db == nil {
		return fmt.Errorf("database not initialized")
	}

	existing, err := mss.db.GetMetadataFieldStates(bookID)
	if err != nil {
		return err
	}

	existingFields := make(map[string]struct{})
	for _, entry := range existing {
		existingFields[entry.Field] = struct{}{}
	}

	now := time.Now()
	for field, entry := range state {
		fetched, err := encodeMetadataValue(entry.FetchedValue)
		if err != nil {
			return fmt.Errorf("failed to encode fetched metadata for %s: %w", field, err)
		}
		override, err := encodeMetadataValue(entry.OverrideValue)
		if err != nil {
			return fmt.Errorf("failed to encode override metadata for %s: %w", field, err)
		}

		if entry.UpdatedAt.IsZero() {
			entry.UpdatedAt = now
		}

		dbState := database.MetadataFieldState{
			BookID:         bookID,
			Field:          field,
			FetchedValue:   fetched,
			OverrideValue:  override,
			OverrideLocked: entry.OverrideLocked,
			UpdatedAt:      entry.UpdatedAt,
		}

		if err := mss.db.UpsertMetadataFieldState(&dbState); err != nil {
			return fmt.Errorf("failed to persist metadata state for %s: %w", field, err)
		}
		delete(existingFields, field)
	}

	// Clean up fields that are no longer in the state
	for field := range existingFields {
		if err := mss.db.DeleteMetadataFieldState(bookID, field); err != nil {
			return fmt.Errorf("failed to clean up metadata state for %s: %w", field, err)
		}
	}

	return nil
}

// UpdateFetchedMetadata updates the fetched values in metadata state
func (mss *MetadataStateService) UpdateFetchedMetadata(bookID string, values map[string]any) error {
	state, err := mss.LoadMetadataState(bookID)
	if err != nil {
		return err
	}

	if state == nil {
		state = make(map[string]metadataFieldState)
	}

	for field, value := range values {
		entry := state[field]
		entry.FetchedValue = value
		entry.UpdatedAt = time.Now()
		state[field] = entry
	}

	return mss.SaveMetadataState(bookID, state)
}

// SetOverride sets an override value for a metadata field
func (mss *MetadataStateService) SetOverride(bookID string, field string, value any, locked bool) error {
	state, err := mss.LoadMetadataState(bookID)
	if err != nil {
		return err
	}

	if state == nil {
		state = make(map[string]metadataFieldState)
	}

	entry := state[field]
	entry.OverrideValue = value
	entry.OverrideLocked = locked
	entry.UpdatedAt = time.Now()
	state[field] = entry

	return mss.SaveMetadataState(bookID, state)
}

// UnlockOverride unlocks an override without changing its value
func (mss *MetadataStateService) UnlockOverride(bookID string, field string) error {
	state, err := mss.LoadMetadataState(bookID)
	if err != nil {
		return err
	}

	if entry, exists := state[field]; exists {
		entry.OverrideLocked = false
		entry.UpdatedAt = time.Now()
		state[field] = entry
		return mss.SaveMetadataState(bookID, state)
	}

	return fmt.Errorf("field %s not found in metadata state", field)
}

// ClearOverride removes an override for a metadata field
func (mss *MetadataStateService) ClearOverride(bookID string, field string) error {
	state, err := mss.LoadMetadataState(bookID)
	if err != nil {
		return err
	}

	if _, exists := state[field]; exists {
		delete(state, field)
		return mss.SaveMetadataState(bookID, state)
	}

	return fmt.Errorf("field %s not found in metadata state", field)
}

// GetEffectiveValue returns the effective value for a field (override > fetched > empty)
func (mss *MetadataStateService) GetEffectiveValue(bookID string, field string) (any, error) {
	state, err := mss.LoadMetadataState(bookID)
	if err != nil {
		return nil, err
	}

	if entry, exists := state[field]; exists {
		if entry.OverrideValue != nil {
			return entry.OverrideValue, nil
		}
		if entry.FetchedValue != nil {
			return entry.FetchedValue, nil
		}
	}

	return nil, nil
}

// loadLegacyMetadataState loads metadata state from the legacy storage system
func (mss *MetadataStateService) loadLegacyMetadataState(bookID string) (map[string]metadataFieldState, error) {
	state := map[string]metadataFieldState{}

	pref, err := mss.db.GetUserPreference(metadataStateKey(bookID))
	if err != nil {
		return state, err
	}

	if pref == nil || pref.Value == nil || *pref.Value == "" {
		return state, nil
	}

	if err := json.Unmarshal([]byte(*pref.Value), &state); err != nil {
		return state, fmt.Errorf("failed to parse legacy metadata state: %w", err)
	}

	return state, nil
}
