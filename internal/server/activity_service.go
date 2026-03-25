// file: internal/server/activity_service.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ActivityService wraps the ActivityStore and provides business-level methods
// for recording and querying unified activity log entries.
type ActivityService struct {
	store *database.ActivityStore
}

// NewActivityService creates a new ActivityService backed by the given store.
func NewActivityService(store *database.ActivityStore) *ActivityService {
	return &ActivityService{store: store}
}

// Record inserts an activity entry into the store. The entry ID is discarded;
// callers that need it should call the store directly.
func (s *ActivityService) Record(entry database.ActivityEntry) error {
	_, err := s.store.Record(entry)
	return err
}

// Query returns entries matching the filter plus the total matching count.
func (s *ActivityService) Query(filter database.ActivityFilter) ([]database.ActivityEntry, int, error) {
	return s.store.Query(filter)
}

// Summarize collapses old entries in the given tier that are older than olderThan.
// Returns the count of original rows deleted.
func (s *ActivityService) Summarize(olderThan time.Time, tier string) (int, error) {
	return s.store.Summarize(olderThan, tier)
}

// Prune hard-deletes all entries of the given tier older than olderThan.
// Returns the number of rows deleted.
func (s *ActivityService) Prune(olderThan time.Time, tier string) (int, error) {
	return s.store.Prune(olderThan, tier)
}

// Store returns the underlying ActivityStore (e.g. for direct access or close).
func (s *ActivityService) Store() *database.ActivityStore {
	return s.store
}
