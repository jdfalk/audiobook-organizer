// file: internal/maintenance/job_test.go
// version: 1.0.0
// guid: 33333333-3333-3333-3333-333333333333
// last-edited: 2026-05-03

package maintenance_test

import (
	"context"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/maintenance"
)

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	if id := maintenance.OperationIDFromCtx(ctx); id != "" {
		t.Fatalf("expected empty, got %q", id)
	}
	ctx = maintenance.WithOperationID(ctx, "op-123")
	if id := maintenance.OperationIDFromCtx(ctx); id != "op-123" {
		t.Fatalf("expected op-123, got %q", id)
	}
}

func TestGetUnknown(t *testing.T) {
	_, err := maintenance.Get("does-not-exist-xyzabc")
	if err == nil {
		t.Fatal("expected error for unknown job ID")
	}
}
