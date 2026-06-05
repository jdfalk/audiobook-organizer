// file: internal/plugin/register.go
// version: 1.0.0

package plugin

import (
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

func init() {
	// eventbus: shared plugin event bus. Plugins and services publish here;
	// the bus has no external dependencies, so it's a leaf.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "eventbus",
		Needs:  []string{},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			return NewEventBus(), nil
		},
	})
}
