// file: internal/audiobooks/lifecycle.go
// version: 1.1.0

// PostInit method on *AudiobookService. Pulls optional deps from the
// container — replaces inline Set* calls from NewServer's activity
// fan-out and write-back fan-out blocks.

package audiobooks

import (
	"context"

	"github.com/falkcorp/audiobook-organizer/internal/activity"
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires:
//   - activity service (snapshot fallback in GetAudiobookTags;
//     DatabasePath-gated; missing → legacy non-snapshot path).
//   - iTunes write-back enqueuer (ITunesEnqueuer interface;
//     itunes-disabled path → typed-nil; left unset).
func (svc *AudiobookService) PostInit(_ context.Context, c *serviceregistry.Container) error {
	if svc == nil {
		return nil
	}
	if as, ok := serviceregistry.TryGet[*activity.Service](c, "activity"); ok && as != nil {
		svc.SetActivityService(as)
	}
	// "writebackbatcher" is registered as *itunesservice.WriteBackBatcher
	// which satisfies the local ITunesEnqueuer interface (EnqueueRemove).
	// Type-assert via the interface so this file doesn't have to import
	// internal/itunes/service.
	if enq, ok := serviceregistry.TryGet[ITunesEnqueuer](c, "writebackbatcher"); ok && enq != nil {
		svc.SetITunesEnqueuer(enq)
	}
	return nil
}
