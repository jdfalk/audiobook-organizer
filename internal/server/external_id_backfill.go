// file: internal/server/external_id_backfill.go
// version: 1.0.0
// guid: a3b4c5d6-e7f8-4a9b-0c1d-2e3f4a5b6c7d

package server

import (
	"log"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ExternalIDStore defines the external ID mapping operations.
// Store implementations should satisfy this interface. Integration code
// performs a runtime type assertion: if the underlying store does not yet
// implement these methods the calls gracefully no-op.
type ExternalIDStore interface {
	CreateExternalIDMapping(mapping *database.ExternalIDMapping) error
	GetBookByExternalID(source, externalID string) (string, error)
	GetExternalIDsForBook(bookID string) ([]database.ExternalIDMapping, error)
	IsExternalIDTombstoned(source, externalID string) (bool, error)
	TombstoneExternalID(source, externalID string) error
	ReassignExternalIDs(oldBookID, newBookID string) error
	BulkCreateExternalIDMappings(mappings []database.ExternalIDMapping) error
}

// asExternalIDStore returns the ExternalIDStore if the given Store implements
// it, or nil otherwise. Callers should check for nil before using.
func asExternalIDStore(s database.Store) ExternalIDStore {
	if s == nil {
		return nil
	}
	if eid, ok := s.(ExternalIDStore); ok {
		return eid
	}
	return nil
}

// backfillExternalIDs scans all books and creates external ID mappings for any
// book that has an iTunes PersistentID set. This is idempotent — it checks the
// setting "external_id_backfill_done" and only runs once.
func (s *Server) backfillExternalIDs() {
	store := database.GlobalStore
	if store == nil {
		return
	}

	eidStore := asExternalIDStore(store)
	if eidStore == nil {
		log.Printf("[DEBUG] backfillExternalIDs: store does not implement ExternalIDStore, skipping")
		return
	}

	// Check if backfill has already been performed
	if setting, err := store.GetSetting("external_id_backfill_done"); err == nil && setting != nil && setting.Value == "true" {
		return
	}

	offset := 0
	backfilled := 0
	for {
		books, err := store.GetAllBooks(10000, offset)
		if err != nil || len(books) == 0 {
			break
		}
		for _, book := range books {
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				_ = eidStore.CreateExternalIDMapping(&database.ExternalIDMapping{
					Source:     "itunes",
					ExternalID: *book.ITunesPersistentID,
					BookID:     book.ID,
				})
				backfilled++
			}
		}
		offset += 10000
	}

	log.Printf("[INFO] Backfilled %d external ID mappings", backfilled)

	// Mark backfill as done so it doesn't re-run
	_ = store.SetSetting("external_id_backfill_done", "true", "bool", false)
}
