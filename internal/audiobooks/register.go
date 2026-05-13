// file: internal/audiobooks/register.go
// version: 1.1.0

package audiobooks

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "audiobook",
		Needs: []string{"store"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return NewAudiobookService(store), nil
		},
	})
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "organize",
		Needs: []string{"store"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return NewOrganizeService(store), nil
		},
	})
}
