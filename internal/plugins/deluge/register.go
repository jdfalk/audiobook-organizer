// file: internal/plugins/deluge/register.go
// version: 1.0.0

// Service registry registration for the deluge UOS plugin (W5).
//
// Build returns nil when Deluge is not configured (no global client OR
// no protected-path cache configured). Inline NewServer construction
// continues to be the production path; W7 migrates registration to
// PostInit.

package deluge

import (
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	delugeclient "github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "delugeplugin",
		Needs: []string{"store", "config"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			client := delugeclient.GetClient()
			if client == nil {
				return (*Plugin)(nil), nil
			}
			cache := delugeclient.NewProtectedPathCache(client, cfg.ProtectedPaths)
			store := serviceregistry.Get[database.Store](c, "store")
			return New(client, cache, store), nil
		},
	})
}
