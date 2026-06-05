// file: internal/quarantine/register.go
// version: 1.0.0

package quarantine

import (
	"github.com/falkcorp/audiobook-organizer/internal/config"
	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/plugin"
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "quarantine",
		Needs:  []string{"store", "config", "eventbus"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			cfg := serviceregistry.Get[*config.Config](c, "config")
			bus := serviceregistry.Get[*plugin.EventBus](c, "eventbus")
			return NewQuarantineService(store, cfg, bus), nil
		},
	})
}
