// file: internal/config/register_test.go
// version: 1.0.0

package config_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func TestConfigUpdateRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Include("configupdate")
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*config.UpdateService](c, "configupdate")
	if svc == nil {
		t.Fatal("UpdateService is nil")
	}
}
