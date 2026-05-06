// file: internal/maintenance/jobs/testhelpers_test.go
// version: 1.0.0
// guid: c8d9e0f1-a2b3-4567-cdef-890123456789
// last-edited: 2026-05-05

// Package jobs_test shared test helpers.
// This file is the single source of truth for shared test helpers used by all
// jobs_test files. Definitions here replace duplicate copies that previously
// lived in fix_read_by_narrator_test.go, fix_author_narrator_swap_test.go, and
// cleanup_empty_folders_test.go.
package jobs_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs" // register all jobs
)

// noopReporter satisfies maintenance.ProgressReporter for test use.
// The logs field captures log messages emitted by the job under test.
type noopReporter struct {
	logs []string
}

func (r *noopReporter) SetTotal(_ int)                      {}
func (r *noopReporter) Increment()                          {}
func (r *noopReporter) Log(_ string, msg string, _ *string) { r.logs = append(r.logs, msg) }

// assertJobRegistered verifies the job is present in the global registry.
func assertJobRegistered(t *testing.T, id string) {
	t.Helper()
	j, err := maintenance.Get(id)
	require.NoError(t, err, "job %q should be registered", id)
	assert.Equal(t, id, j.ID())
}
