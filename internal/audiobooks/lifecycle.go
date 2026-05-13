// file: internal/audiobooks/lifecycle.go
// version: 1.0.0

// PostInit method on *AudiobookService that the serviceregistry container
// picks up via interface satisfaction. Wires the optional activity service
// dependency by pulling it from the container — replaces the inline
// SetActivityService call that used to live in NewServer's activity
// fan-out block.

package audiobooks

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the activity service (used for snapshot fallback in
// GetAudiobookTags). Activity is DatabasePath-gated — when absent, the
// fallback path simply returns the legacy tag map without snapshots.
func (svc *AudiobookService) PostInit(_ context.Context, c *serviceregistry.Container) error {
	if svc == nil {
		return nil
	}
	if as, ok := serviceregistry.TryGet[*activity.Service](c, "activity"); ok && as != nil {
		svc.SetActivityService(as)
	}
	return nil
}
