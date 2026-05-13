// file: internal/scanner/lifecycle.go
// version: 1.0.0

// PostInit method on *ScanService that the serviceregistry container
// picks up via interface satisfaction. Wires the optional activity
// writer dependency from the container — replaces the inline
// SetActivityWriter call from NewServer's activity fan-out block.

package scanner

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the activity writer used to batch per-book scan
// events. Writer is DatabasePath-gated — when absent, scan events are
// dropped (the legacy behavior before the dual-write tap landed).
func (ss *ScanService) PostInit(_ context.Context, c *serviceregistry.Container) error {
	if ss == nil {
		return nil
	}
	if aw, ok := serviceregistry.TryGet[*activity.Writer](c, "activitywriter"); ok && aw != nil {
		ss.SetActivityWriter(aw)
	}
	return nil
}
