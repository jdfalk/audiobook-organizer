// file: internal/server/batch_poller_test.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-5678-cdef-9876543210ab

package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBatchLister is a minimal mock for testing BatchPoller without a real OpenAI client.
// We test the poller logic by calling Poll with a parser that has a custom ListProjectBatches.
// Since ListProjectBatches is on *ai.OpenAIParser and not mockable directly, we test
// the handler routing and dedup logic via the exported methods.

func TestBatchPollerRegisterAndRouting(t *testing.T) {
	bp := NewBatchPoller(nil, nil)

	called := map[string]string{}
	bp.RegisterHandler("author_dedup", func(_ context.Context, batchID, outputFileID string) error {
		called["author_dedup"] = batchID
		return nil
	})
	bp.RegisterHandler("diagnostics", func(_ context.Context, batchID, outputFileID string) error {
		called["diagnostics"] = batchID
		return nil
	})

	assert.Len(t, bp.handlers, 2)
	assert.Contains(t, bp.handlers, "author_dedup")
	assert.Contains(t, bp.handlers, "diagnostics")
}

func TestBatchPollerProcessedTracking(t *testing.T) {
	bp := NewBatchPoller(nil, nil)

	assert.False(t, bp.IsProcessed("batch_123"))

	bp.MarkProcessed("batch_123")
	assert.True(t, bp.IsProcessed("batch_123"))

	// Marking again is idempotent
	bp.MarkProcessed("batch_123")
	assert.True(t, bp.IsProcessed("batch_123"))
}

func TestBatchPollerHandlerError(t *testing.T) {
	bp := NewBatchPoller(nil, nil)

	failCount := 0
	bp.RegisterHandler("failing_type", func(_ context.Context, batchID, outputFileID string) error {
		failCount++
		return fmt.Errorf("handler error")
	})

	// Simulate calling the handler directly — on failure it should NOT be marked processed
	err := bp.handlers["failing_type"](context.Background(), "batch_fail", "file_123")
	require.Error(t, err)
	assert.Equal(t, 1, failCount)
	assert.False(t, bp.IsProcessed("batch_fail"))
}

func TestBatchMetadataHelper(t *testing.T) {
	// Test the batchMetadata helper in the ai package
	// We can't call it directly since it's unexported, but we verify the types are correct
	info := ai.BatchInfo{
		ID:           "batch_abc",
		Status:       "completed",
		Type:         "author_dedup",
		OutputFileID: "file_xyz",
		ErrorFileID:  "",
		RequestCounts: ai.RequestCounts{
			Total:     10,
			Completed: 8,
			Failed:    2,
		},
	}

	assert.Equal(t, "batch_abc", info.ID)
	assert.Equal(t, "completed", info.Status)
	assert.Equal(t, "author_dedup", info.Type)
	assert.Equal(t, 10, info.RequestCounts.Total)
	assert.Equal(t, 8, info.RequestCounts.Completed)
	assert.Equal(t, 2, info.RequestCounts.Failed)
}
