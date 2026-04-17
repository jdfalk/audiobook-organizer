// file: internal/scanner/hooks.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package scanner

// ScanHooks provides optional callbacks for scan-time side effects.
// All methods must be safe for concurrent use. A nil ScanHooks value
// means no hooks fire — callers must nil-check before calling.
type ScanHooks interface {
	OnBookScanned(bookID, title string)
	OnImportDedup(bookID string)
}

var scanHooks ScanHooks

// SetScanHooks installs (or clears) the hook implementation used by
// the scanner's save-to-database path. Pass nil to disable hooks.
func SetScanHooks(hooks ScanHooks) {
	scanHooks = hooks
}
