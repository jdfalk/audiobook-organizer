// file: internal/server/metadata_upgrade.go
// version: 2.0.0
// guid: 4a3b2c1d-0e9f-8a7b-6c5d-4e3f2a1b0c9d
// last-edited: 2026-05-11
//
// Re-exports MetadataUpgradeService from internal/metabatch so existing
// server wiring code continues to compile without changes. All logic
// has moved to internal/metabatch/upgrade.go.

package server

import "github.com/jdfalk/audiobook-organizer/internal/metabatch"

// MetadataUpgradeService is a server-local alias for the metabatch type,
// preserving backward compatibility for any server wiring that references it.
type MetadataUpgradeService = metabatch.MetadataUpgradeService

// NewMetadataUpgradeService is a convenience alias for the constructor in
// internal/metabatch.
var NewMetadataUpgradeService = metabatch.NewMetadataUpgradeService
