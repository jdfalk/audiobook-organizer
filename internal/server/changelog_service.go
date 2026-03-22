// file: internal/server/changelog_service.go
// version: 1.1.0
// guid: 93167949-a587-41e9-8ef9-92d03f86aea6

package server

import (
	"fmt"
	"sort"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ChangeLogEntry represents a single entry in a book's changelog timeline.
type ChangeLogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"` // tag_write, rename, metadata_apply, import, transcode
	Summary   string         `json:"summary"`
	Details   map[string]any `json:"details,omitempty"`
}

// ChangelogService merges history data from multiple sources into a unified changelog.
type ChangelogService struct {
	db database.Store
}

// NewChangelogService creates a new ChangelogService instance.
func NewChangelogService(db database.Store) *ChangelogService {
	return &ChangelogService{db: db}
}

// maxChangelogEntries is the maximum number of entries returned by GetBookChangelog.
const maxChangelogEntries = 50

// GetBookChangelog returns a merged, time-sorted changelog for the given book.
// It pulls data from book_path_history (renames), metadata_change_history
// (metadata applies/tag writes), and operation_changes (imports/transcodes).
func (svc *ChangelogService) GetBookChangelog(bookID string) ([]ChangeLogEntry, error) {
	if svc.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var entries []ChangeLogEntry

	// 1. Path history → rename entries
	pathHistory, err := svc.db.GetBookPathHistory(bookID)
	if err != nil {
		// Non-fatal: log and continue
		_ = err
	} else {
		for _, ph := range pathHistory {
			entries = append(entries, ChangeLogEntry{
				Timestamp: ph.CreatedAt,
				Type:      "rename",
				Summary:   fmt.Sprintf("Renamed — %s → %s", ph.OldPath, ph.NewPath),
				Details: map[string]any{
					"old_path":    ph.OldPath,
					"new_path":    ph.NewPath,
					"change_type": ph.ChangeType,
				},
			})
		}
	}

	// 2. Metadata change history → metadata_apply and tag_write entries
	metaHistory, err := svc.db.GetBookChangeHistory(bookID, 100)
	if err != nil {
		_ = err
	} else {
		for _, mh := range metaHistory {
			entryType := "metadata_apply"
			summary := fmt.Sprintf("Metadata applied — %s: %s (%s)", mh.Field, derefStrDisplay(mh.NewValue), mh.Source)

			if mh.ChangeType == "override" || mh.ChangeType == "clear" || mh.ChangeType == "undo" {
				entryType = "tag_write"
				summary = fmt.Sprintf("Tag written — %s set to %s (%s)", mh.Field, derefStrDisplay(mh.NewValue), mh.ChangeType)
			}

			details := map[string]any{
				"field":       mh.Field,
				"change_type": mh.ChangeType,
				"source":      mh.Source,
			}
			if mh.PreviousValue != nil {
				details["previous_value"] = *mh.PreviousValue
			}
			if mh.NewValue != nil {
				details["new_value"] = *mh.NewValue
			}

			entries = append(entries, ChangeLogEntry{
				Timestamp: mh.ChangedAt,
				Type:      entryType,
				Summary:   summary,
				Details:   details,
			})
		}
	}

	// 3. Operation changes → import and transcode entries
	opChanges, err := svc.db.GetBookChanges(bookID)
	if err != nil {
		_ = err
	} else {
		for _, oc := range opChanges {
			entryType := "import"
			summary := fmt.Sprintf("Operation change — %s: %s → %s", oc.FieldName, oc.OldValue, oc.NewValue)

			switch oc.ChangeType {
			case "file_move":
				entryType = "rename"
				summary = fmt.Sprintf("File moved — %s → %s", oc.OldValue, oc.NewValue)
			case "tag_write":
				entryType = "tag_write"
				summary = fmt.Sprintf("Tags written — %s: %s → %s", oc.FieldName, oc.OldValue, oc.NewValue)
			case "metadata_update":
				entryType = "metadata_apply"
				summary = fmt.Sprintf("Metadata updated — %s: %s → %s", oc.FieldName, oc.OldValue, oc.NewValue)
			}

			details := map[string]any{
				"operation_id": oc.OperationID,
				"change_type":  oc.ChangeType,
				"field_name":   oc.FieldName,
				"old_value":    oc.OldValue,
				"new_value":    oc.NewValue,
			}
			if oc.RevertedAt != nil {
				details["reverted_at"] = oc.RevertedAt
			}

			entries = append(entries, ChangeLogEntry{
				Timestamp: oc.CreatedAt,
				Type:      entryType,
				Summary:   summary,
				Details:   details,
			})
		}
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	// Limit to maxChangelogEntries
	if len(entries) > maxChangelogEntries {
		entries = entries[:maxChangelogEntries]
	}

	return entries, nil
}

// derefStrDisplay safely dereferences a *string, returning "<nil>" for nil pointers (display-oriented).
func derefStrDisplay(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
