// file: internal/plugins/itunes/plugin.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-4a5b-8c9d-0e1f2a4b5c6d
// last-edited: 2026-05-07

// Package itunes is the UOS plugin for iTunes operations.
// It wraps the internal iTunes service and registers OperationDefs through
// the public pkg/plugin/sdk interface.
package itunes

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// Plugin is the iTunes plugin. It wraps the iTunes service so that
// the Run functions can call service methods without importing internal packages.
type Plugin struct {
	svc   *itunesservice.Service
	store database.Store
}

// New constructs an iTunes Plugin.
func New(svc *itunesservice.Service, store database.Store) *Plugin {
	return &Plugin{svc: svc, store: store}
}

// ID implements sdk.Plugin.
func (p *Plugin) ID() string { return "itunes" }

// Name implements sdk.Plugin.
func (p *Plugin) Name() string { return "iTunes" }

// Version implements sdk.Plugin.
func (p *Plugin) Version() string { return "1.0.0" }

// Register registers all iTunes OperationDefs with the UOS registry.
// UOS-10 migrates itunes.import, itunes.sync, itunes.path-reconcile,
// itunes.path-repair, and itunes.position-sync to UOS.
func (p *Plugin) Register(r sdk.Registry) error {
	// Guard: if iTunes service is nil, skip registration.
	// This happens when iTunes is disabled or the service failed to initialize.
	if p.svc == nil {
		return nil
	}

	ops := []sdk.OperationDef{
		p.importDef(),
		p.syncDef(),
		p.pathReconcileDef(),
		p.pathRepairDef(),
		p.positionSyncDef(),
	}

	for _, op := range ops {
		if err := r.RegisterOp(op); err != nil {
			return err
		}
	}
	return nil
}
