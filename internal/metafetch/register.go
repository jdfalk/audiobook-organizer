// file: internal/metafetch/register.go
// version: 1.2.0

package metafetch

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "metadatastate",
		Needs:  []string{"store"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return NewMetadataStateService(store), nil
		},
	})

	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "metafetch",
		Needs:  []string{"store"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return NewService(store), nil
		},
	})

	// olservice — Open Library data-dump lifecycle wrapper. No build-time
	// deps; the underlying OL store is opened lazily on first EnsureStore.
	// metafetch.Service.PostInit pulls this to wire SetOLStore.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "olservice",
		Needs:  []string{},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			return NewOpenLibraryService(), nil
		},
	})
}
