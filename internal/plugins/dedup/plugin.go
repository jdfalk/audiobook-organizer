// file: internal/plugins/dedup/plugin.go
// version: 1.0.0
// guid: d1e2f3a4-b5c6-7890-abcd-ef1234567890
// last-edited: 2026-05-06

// Package dedup is the UOS plugin for deduplication operations.
// It wraps the internal dedup.Engine and registers OperationDefs through
// the public pkg/plugin/sdk interface.
package dedup

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// Plugin is the dedup plugin. It wraps the shared dedup.Engine so that
// the Run functions can call engine methods without importing internal packages.
type Plugin struct {
	engine *dedupengine.Engine
	store  database.Store
}

// New constructs a dedup Plugin. engine may be nil if embedding is disabled;
// the embed-scan op will return a descriptive error when run.
func New(engine *dedupengine.Engine, store database.Store) *Plugin {
	return &Plugin{engine: engine, store: store}
}

// ID implements sdk.Plugin.
func (p *Plugin) ID() string { return "dedup" }

// Name implements sdk.Plugin.
func (p *Plugin) Name() string { return "Deduplication" }

// Version implements sdk.Plugin.
func (p *Plugin) Version() string { return "1.0.0" }

// Register registers all dedup OperationDefs with the UOS registry.
// UOS-07 registers embed-scan only; additional ops are added in UOS-09.
func (p *Plugin) Register(r sdk.Registry) error {
	return r.RegisterOp(p.embedScanDef())
}
