// file: internal/database/web.go
// version: 2.0.0
// guid: 5d6e7f8a-9b0c-1d2e-3f4a-5b6c7d8e9f0a
// last-edited: 2026-06-10

// Package database — this file previously implemented package-level
// GetImportPaths / CreateOperation / GetUserPreference etc. functions that
// queried the global *sql.DB directly. All of those operations are now
// implemented on PebbleStore (pebble_store.go). The SQLite global-DB
// wrappers were removed in fable5 TASK-022.

package database
