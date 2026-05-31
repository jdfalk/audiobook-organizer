// file: internal/scanner/register_test.go
// version: 1.0.0

package scanner_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func TestScanRegistration(t *testing.T) {
	c := serviceregistry.NewContainer().
		Override("store", mocks.NewMockStore(t)).
		Override("embeddingstore", (*database.EmbeddingStore)(nil)).
		Include("scan")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := serviceregistry.Get[*scanner.ScanService](c, "scan")
	if svc == nil {
		t.Fatal("ScanService is nil")
	}
}
