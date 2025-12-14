// file: internal/operations/queue_progress_test.go
// version: 1.0.0
// guid: 2f9d4c3b-1a0e-4b5c-9d7f-abcdef123456

package operations

import (
	"testing"
	"time"
)

func TestUpdateProgressNotifiesListeners(t *testing.T) {
	q := &OperationQueue{listeners: make(map[string][]ProgressListener)}

	progressCh := make(chan OperationProgress, 1)
	q.listeners["op-1"] = []ProgressListener{
		func(operationID string, progress OperationProgress) {
			if operationID != "op-1" {
				t.Errorf("unexpected operation id: %s", operationID)
			}
			progressCh <- progress
		},
	}

	reporter := &operationProgressReporter{operationID: "op-1", queue: q}

	if err := reporter.UpdateProgress(2, 5, "processing"); err != nil {
		t.Fatalf("UpdateProgress returned error: %v", err)
	}

	select {
	case got := <-progressCh:
		if got.Current != 2 || got.Total != 5 || got.Message != "processing" {
			t.Fatalf("unexpected progress payload: %+v", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("did not receive progress notification")
	}

	if reporter.current != 2 || reporter.total != 5 {
		t.Fatalf("reporter state not updated: current=%d total=%d", reporter.current, reporter.total)
	}
}
