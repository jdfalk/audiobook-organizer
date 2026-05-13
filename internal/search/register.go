// file: internal/search/register.go
// version: 2.0.0
// guid: 7b4e2c1a-9f3d-4a82-b6e5-1d0c8f5a3e72
//
// Registers the BleveIndex as the "searchindex" service in the
// serviceregistry. IndexService satisfies Starter and Stopper so
// the container manages its lifecycle once Container.Start/Stop
// is wired (SERVER-LIFECYCLE-FLIP).
//
// Path convention mirrors server_lifecycle.go:
//
//	{dirname(config.DatabasePath)}/library.bleve

package search

import (
	"context"
	"log"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// IndexService wraps a *BleveIndex and defers the actual file-system
// open to Start so that test code that never calls Start doesn't leak
// Bleve file handles.
//
// Exported (renamed from bleveIndexService in v2.0.0) so server-package
// code can fetch it from the container via TryGet[*search.IndexService]
// and pull the underlying index via Index().
type IndexService struct {
	cfg  *config.Config
	idx  *BleveIndex
	path string
}

// Start currently a no-op. Bleve open stays inline in
// server_lifecycle.go because moving it here exposes a pre-existing
// race between the stripMovementAtoms / remuxMalformedM4BFiles
// goroutines (which read s.store) and the indexedStore decorator
// install. Once that race is fixed independently, the OpenInline
// helper below can move into this Start and Container.Start drives
// the open.
//
// Until then, Stop is a no-op too — the inline path also drives Close.
func (b *IndexService) Start(_ context.Context) error {
	return nil
}

// OpenInline opens the on-disk Bleve index. Idempotent — if already
// open, returns the existing handle. Called by server_lifecycle.go's
// inline open path. Open failures are downgraded to nil + warning so
// the server can run without search.
func (b *IndexService) OpenInline() *BleveIndex {
	if b == nil {
		return nil
	}
	if b.idx != nil {
		return b.idx
	}
	if b.cfg == nil || b.cfg.DatabasePath == "" {
		return nil
	}
	indexPath := filepath.Join(filepath.Dir(b.cfg.DatabasePath), "library.bleve")
	idx, err := Open(indexPath)
	if err != nil {
		log.Printf("[WARN] Failed to open search index: %v", err)
		return nil
	}
	b.idx = idx
	b.path = indexPath
	log.Printf("[INFO] Search index opened at %s", indexPath)
	return idx
}

// Stop currently a no-op. Bleve close stays inline in
// server_lifecycle.go alongside the inline open. When the
// store-install race is fixed and Start/Stop drive open/close, this
// becomes b.idx.Close() + b.idx = nil.
func (b *IndexService) Stop(_ context.Context) error {
	return nil
}

// Index returns the live *BleveIndex, or nil if Start has not been
// called or the service was stopped.
func (b *IndexService) Index() *BleveIndex {
	if b == nil {
		return nil
	}
	return b.idx
}

// Path returns the on-disk path the index was opened from, or "" if
// not yet started.
func (b *IndexService) Path() string {
	if b == nil {
		return ""
	}
	return b.path
}

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "searchindex",
		Needs:  []string{"config"},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			return &IndexService{cfg: cfg}, nil
		},
	})
}
