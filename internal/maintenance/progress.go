// file: internal/maintenance/progress.go
// version: 1.0.0
// guid: a3b4c5d6-e7f8-9012-3456-7890abcdef12
// last-edited: 2026-05-11

package maintenance

import (
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// ProgressAdapter adapts operations.ProgressReporter to maintenance.ProgressReporter.
// It bridges the operations layer's richer progress API to the simpler interface
// that maintenance jobs consume.
type ProgressAdapter struct {
	Ops   operations.ProgressReporter
	cur   int
	total int
}

// SetTotal sets the expected total item count for progress reporting.
func (a *ProgressAdapter) SetTotal(n int) { a.total = n }

// Increment advances the current count by one and propagates progress to the
// underlying operations reporter.
func (a *ProgressAdapter) Increment() {
	a.cur++
	_ = a.Ops.UpdateProgress(a.cur, a.total, "")
}

// Log forwards a log entry to the underlying operations reporter.
func (a *ProgressAdapter) Log(level, message string, details *string) {
	_ = a.Ops.Log(level, message, details)
}
