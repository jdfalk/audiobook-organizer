// file: internal/importer/register_test.go
// version: 1.0.0

package importer_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func TestImportPathRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Include("importpath")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*importer.ImportPathService](c, "importpath")
	if svc == nil {
		t.Fatal("ImportPathService is nil")
	}
}
