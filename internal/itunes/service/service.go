// file: internal/itunes/service/service.go
// version: 1.1.0
// guid: 81ccaec6-42b0-4828-83c8-7a96680112d9

package itunesservice

import (
	"context"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
)

// Deps is the explicit dependency set for Service. No globals, no Server,
// no config.AppConfig — everything the service needs is passed in.
type Deps struct {
	Store      Store
	OpQueue    operations.Queue
	ActivityFn func(database.ActivityEntry)
	Realtime   *realtime.EventHub // may be nil; means no SSE push
	Config     Config
	Logger     logger.Logger
}

// Sub-component placeholder types. Real definitions land in Phase 2 M1
// as each sub-component is moved out of internal/server/. Kept as empty
// structs here so Service can declare typed fields without a forward
// reference cycle. Each placeholder is deleted and replaced by a real
// definition in its own file when the corresponding sub-component moves.
//
// Status after Phase 2 M1 step 1 (this PR):
//   - TrackProvisioner: real (track_provisioner.go)
//   - All others: placeholder
type (
	// Importer runs the iTunes import pipeline. Placeholder until moved.
	Importer struct{}
	// WriteBackBatcher batches ITL write-backs. Placeholder until moved.
	WriteBackBatcher struct{}
	// PositionSync syncs playback positions with iTunes. Placeholder until moved.
	PositionSync struct{}
	// PathReconciler reconciles iTunes-vs-library paths. Placeholder until moved.
	PathReconciler struct{}
	// PlaylistSync syncs iTunes playlists. Placeholder until moved.
	PlaylistSync struct{}
	// TransferService transfers ITL files. Placeholder until moved.
	TransferService struct{}
)

// Service owns the iTunes integration. Prefer a single *Service on the
// Server struct — it composes the seven sub-components below with shared
// lifecycle (Start / Shutdown).
type Service struct {
	deps Deps

	// Sub-components. Nil when the service is disabled; populated by New.
	Importer    *Importer
	Batcher     *WriteBackBatcher
	Positions   *PositionSync
	Paths       *PathReconciler
	Playlists   *PlaylistSync
	Provisioner *TrackProvisioner
	Transfer    *TransferService
}

// New constructs a fully-wired iTunes service. Returns ErrITunesDisabled
// equivalent (cfg.Enabled == false) routes through NewDisabled instead —
// callers should branch on cfg.Enabled at the construction site.
func New(deps Deps) (*Service, error) {
	if !deps.Config.Enabled {
		return NewDisabled(), nil
	}
	if deps.Logger == nil {
		deps.Logger = logger.New("itunes")
	}
	svc := &Service{
		deps: deps,
	}

	// Wire real sub-components as they land. M1 step 1: Provisioner.
	// The enqueuer (batcher) lives on Server until M1 step 2 — server
	// calls Provisioner.SetEnqueuer after New() to complete wiring.
	svc.Provisioner = newTrackProvisioner(deps.Store, nil /*enqueuer TBD*/, deps.Config)

	return svc, nil
}

// NewDisabled constructs a Service whose methods all return
// ErrITunesDisabled. Use when cfg.Enabled == false so the rest of the
// server can still wire a non-nil *Service and avoid nil guards at every
// call site.
func NewDisabled() *Service {
	return &Service{}
}

// Enabled reports whether the service has active sub-components wired.
// A disabled service returns false; a real service returns true once
// Start has run (or immediately — PR 2 decides per component).
func (s *Service) Enabled() bool {
	// PR 2 will refine: "enabled and started" vs "enabled but not yet
	// started". For now, Enabled == cfg.Enabled.
	return s.deps.Config.Enabled
}

// Start launches any long-lived sub-component goroutines (currently just
// the WriteBackBatcher, wired in PR 2's step 2f). No-op when disabled.
func (s *Service) Start(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	// Sub-component Start calls added in PR 2. This skeleton is a no-op
	// so PR 1 can ship without behavior change.
	return nil
}

// Shutdown flushes any long-lived sub-components and waits up to timeout
// for graceful completion. No-op when disabled.
func (s *Service) Shutdown(timeout time.Duration) error {
	if !s.Enabled() {
		return nil
	}
	// Sub-component Shutdown calls added in PR 2.
	return nil
}
