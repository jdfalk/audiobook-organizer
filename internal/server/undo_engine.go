// file: internal/server/undo_engine.go
// version: 1.3.0
// guid: 0b8c9d6e-1f7a-4a70-b8c5-3d7e0f1b9a99
// last-edited: 2026-05-11
//
// Backward-compatibility wrapper for the undo engine, now in internal/undo.
// This file re-exports the public API from internal/undo with server-specific
// callback handling (deluge.NotifyDelugeAfterUndo).
//
// The actual undo implementation lives in internal/undo/engine.go

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/undo"
)

// UndoResult is re-exported from the undo package.
type UndoResult = undo.UndoResult

// UndoConflictReport is re-exported from the undo package.
type UndoConflictReport = undo.UndoConflictReport

// UndoConflictItem is re-exported from the undo package.
type UndoConflictItem = undo.UndoConflictItem

// RunUndoOperation wraps the undo engine with server-specific callback
// for Deluge integration. It loads the changes for targetOpID, walks them
// in reverse order, and applies the inverse of each change.
func RunUndoOperation(
	store interface {
		database.BookStore
		database.BookVersionStore
		database.OperationStore
	},
	targetOpID string,
	progress func(step string, pct int),
) (*UndoResult, error) {
	return undo.RunUndoOperation(
		store,
		targetOpID,
		progress,
		deluge.NotifyDelugeAfterUndo,
	)
}

// PreflightUndoConflicts is re-exported from the undo package.
func PreflightUndoConflicts(store database.Store, operationID string) (*UndoConflictReport, error) {
	return undo.PreflightUndoConflicts(store, operationID)
}
