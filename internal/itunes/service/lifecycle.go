// file: internal/itunes/service/lifecycle.go
// version: 1.0.0

// PostInit method on *Service. Wires the activity writer into the
// PathRepairer sub-component so repair runs can emit per-book activity
// events. Replaces the inline SetActivityWriter call from NewServer's
// activity fan-out block.

package itunesservice

import (
	"context"

	"github.com/falkcorp/audiobook-organizer/internal/activity"
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the activity writer into the path-repair sub-component.
// activitywriter is DatabasePath-gated; the disabled service path (Repair
// nil) and missing-writer path are both no-ops.
func (s *Service) PostInit(_ context.Context, c *serviceregistry.Container) error {
	if s == nil || !s.Enabled() || s.Repair == nil {
		return nil
	}
	if aw, ok := serviceregistry.TryGet[*activity.Writer](c, "activitywriter"); ok && aw != nil {
		s.Repair.SetActivityWriter(aw)
	}
	return nil
}
