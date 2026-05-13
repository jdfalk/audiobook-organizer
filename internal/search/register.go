// file: internal/search/register.go
// version: 1.0.0
// guid: 7b4e2c1a-9f3d-4a82-b6e5-1d0c8f5a3e72
//
// Registers the BleveIndex as the "searchindex" service in the
// serviceregistry. The wrapper type (bleveIndexService) satisfies
// Starter and Stopper so the container can manage its lifecycle.
//
// Path convention mirrors server_lifecycle.go:
//
//	{dirname(config.DatabasePath)}/library.bleve

package search

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// bleveIndexService wraps a *BleveIndex and defers the actual file-system
// open to Start so that test code that never calls Start doesn't leak
// Bleve file handles (same discipline the server uses).
type bleveIndexService struct {
	cfg  *config.Config
	idx  *BleveIndex
	path string
}

// Start opens the on-disk Bleve index. Idempotent — if the index is already
// open, Start is a no-op and returns nil.
func (b *bleveIndexService) Start(_ context.Context) error {
	if b.idx != nil {
		return nil
	}
	if b.cfg.DatabasePath == "" {
		return fmt.Errorf("searchindex: DatabasePath not configured")
	}
	indexPath := filepath.Join(filepath.Dir(b.cfg.DatabasePath), "library.bleve")
	idx, err := Open(indexPath)
	if err != nil {
		return fmt.Errorf("searchindex Start: %w", err)
	}
	b.idx = idx
	b.path = indexPath
	return nil
}

// Stop closes the underlying Bleve index. Safe to call multiple times or
// before Start has been called.
func (b *bleveIndexService) Stop(_ context.Context) error {
	if b.idx == nil {
		return nil
	}
	err := b.idx.Close()
	b.idx = nil
	return err
}

// Index returns the live *BleveIndex, or nil if Start has not been called
// or the service was stopped. Callers that need the raw index (e.g. a
// PostInit wiring step) may type-assert the container value to
// *bleveIndexService and call Index().
func (b *bleveIndexService) Index() *BleveIndex {
	return b.idx
}

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "searchindex",
		Needs: []string{"config"},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			return &bleveIndexService{cfg: cfg}, nil
		},
	})
}
