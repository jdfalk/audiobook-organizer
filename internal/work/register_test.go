// file: internal/work/register_test.go
// version: 1.0.0

package work_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
	"github.com/jdfalk/audiobook-organizer/internal/work"
)

func TestWorkRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Include("work")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*work.WorkService](c, "work")
	if svc == nil {
		t.Fatal("WorkService is nil")
	}
}
