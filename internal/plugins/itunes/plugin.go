// file: internal/plugins/itunes/plugin.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-07

// Package itunes is the UOS plugin for iTunes/Music library operations.
// It wraps the internal iTunes service and registers OperationDefs through
// the public pkg/plugin/sdk interface.
package itunes

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// Plugin is the iTunes plugin. It wraps the shared iTunes service so that
// the Run functions can call service methods without importing internal packages.
type Plugin struct {
	svc   *itunesservice.Service
	store database.Store
}

// New constructs an iTunes Plugin. svc may be nil or disabled;
// the Register method will return nil (nil-guard pattern).
func New(svc *itunesservice.Service, store database.Store) *Plugin {
	return &Plugin{svc: svc, store: store}
}

// ID implements sdk.Plugin.
func (p *Plugin) ID() string { return "itunes" }

// Name implements sdk.Plugin.
func (p *Plugin) Name() string { return "iTunes/Music Library" }

// Version implements sdk.Plugin.
func (p *Plugin) Version() string { return "1.0.0" }

// Register registers all iTunes OperationDefs with the UOS registry.
// Returns nil if the service is nil or disabled (nil-guard pattern).
func (p *Plugin) Register(r sdk.Registry) error {
	// Nil-guard: don't register if service not configured or disabled.
	if p.svc == nil || !p.svc.Enabled() {
		return nil
	}

	defs := []sdk.OperationDef{
		p.syncDef(),
		p.importDef(),
		p.pathReconciledDef(),
		p.pathRepairDef(),
		p.positionSyncDef(),
	}

	for _, def := range defs {
		if err := r.RegisterOp(def); err != nil {
			return fmt.Errorf("register %s: %w", def.ID, err)
		}
	}

	return nil
}
