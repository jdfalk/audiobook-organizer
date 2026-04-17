// file: internal/operations/activity.go
// version: 1.0.0
// guid: a1b2c3d4-5e6f-7a8b-9c0d-e1f2a3b4c5d6

package operations

import "github.com/jdfalk/audiobook-organizer/internal/database"

// ActivityLogger receives activity entries for audit logging.
// Implementations must be safe for concurrent use.
type ActivityLogger interface {
	RecordActivity(entry database.ActivityEntry)
}
