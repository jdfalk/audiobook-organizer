// file: internal/sysinfo/register_test.go
// version: 1.0.0

package sysinfo_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
	"github.com/jdfalk/audiobook-organizer/internal/sysinfo"
)

func TestDashboardRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Include("dashboard")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*sysinfo.DashboardService](c, "dashboard")
	if svc == nil {
		t.Fatal("DashboardService is nil")
	}
}
