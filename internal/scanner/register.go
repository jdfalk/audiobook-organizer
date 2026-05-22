// file: internal/scanner/register.go
// version: 1.0.0

package scanner

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "scan",
		Needs:  []string{"store", "embeddingstore"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			scanSvc := NewScanService(store)
			// Wire in EmbeddingStore for metadata hash dedup detection
			if es := serviceregistry.Get[*database.EmbeddingStore](c, "embeddingstore"); es != nil {
				scanSvc.SetEmbeddingStore(es)
			}
			return scanSvc, nil
		},
	})
}
