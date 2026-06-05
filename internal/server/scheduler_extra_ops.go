// file: internal/server/scheduler_extra_ops.go
// version: 2.0.0
// guid: f1e2d3c4-b5a6-7890-fedc-ba9876543210

// scheduler_extra_ops is a thin shim that wires the 13 ExtraOpsRegistrar
// methods (now living in internal/scheduler/extra_ops.go) into the server
// package's addOpRegistrar mechanism.
//
// The actual OperationDef logic has been extracted to
// internal/scheduler.ExtraOpsRegistrar as part of SERVER-THIN-RESIDUAL.

package server

import opsregistry "github.com/falkcorp/audiobook-organizer/internal/operations/registry"

// schedulerExtraOpParams carries the v1 operation ID from a legacy TriggerFn
// into a UOS v2 Run func. Kept in the server package so server_lifecycle.go
// can re-enqueue resumed operations with the correct legacy op ID.
type schedulerExtraOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

func init() {
	// All 13 Register* methods have moved to internal/scheduler.ExtraOpsRegistrar
	// (SERVER-THIN-RESIDUAL). These shims keep the addOpRegistrar contract intact
	// while delegating to s.extraOpsRegistrar which is constructed in NewServer.
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterDedupLLMReviewOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterTrashCleanupOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterArchiveSweepOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterMetadataUpgradeOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterAuthorSplitScanOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterDBOptimizeOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterCleanupOldBackupsOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterISBNEnrichmentOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterTempFileCleanupOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterPurgeDeletedOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterTombstoneCleanupOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterResolveProductionAuthorsOp(reg)
	})
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error {
		return s.extraOpsRegistrar.RegisterMetadataRefreshOp(reg)
	})
}
