// file: internal/fileops/register_test.go
// version: 1.0.0

package fileops_test

import (
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database/mocks"
	"github.com/falkcorp/audiobook-organizer/internal/fileops"
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

func TestFilesystemRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Include("filesystem")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*fileops.FilesystemService](c, "filesystem")
	if svc == nil {
		t.Fatal("FilesystemService is nil")
	}
}
