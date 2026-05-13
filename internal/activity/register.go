// file: internal/activity/register.go
// version: 1.0.0

package activity

import (
	"fmt"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	// activitystore: NutsDB-backed sidecar store for activity log entries.
	// Lives in {dirname(DatabasePath)}/activity.nutsdb so it sits next to the
	// main Pebble DB. Returns nil (Build returns an error, but the container
	// will fail Resolve before reaching here if DatabasePath is empty) when
	// DatabasePath is unset — host code must Override "activitystore" with a
	// pre-built instance in that case (test paths).
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "activitystore",
		Needs: []string{"config"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			if cfg.DatabasePath == "" {
				return nil, fmt.Errorf("activitystore: DatabasePath not configured")
			}
			activityDir := filepath.Join(filepath.Dir(cfg.DatabasePath), "activity.nutsdb")
			return database.NewNutsActivityStore(activityDir)
		},
	})

	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "activity",
		Needs: []string{"activitystore"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[*database.NutsActivityStore](c, "activitystore")
			return NewService(store), nil
		},
	})
}
