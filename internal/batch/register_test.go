// file: internal/batch/register_test.go
// version: 1.0.0

package batch_test

import (
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/batch"
	"github.com/falkcorp/audiobook-organizer/internal/database/mocks"
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

func TestBatchRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Include("batch")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*batch.BatchService](c, "batch")
	if svc == nil {
		t.Fatal("BatchService is nil")
	}
}
