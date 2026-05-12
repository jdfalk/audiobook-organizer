// file: internal/batch/register.go
// version: 1.0.0

package batch

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "batch",
		Needs: []string{"store"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return NewBatchService(store), nil
		},
	})
}
