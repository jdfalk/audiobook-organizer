// file: internal/serviceregistry/registry.go
// version: 1.0.0

package serviceregistry

import "fmt"

// ServiceDef describes a registered service.
type ServiceDef struct {
	// Name is the registry key. Must be unique. Convention: lowercase,
	// dot-separated for grouping (e.g. "dedup", "metafetch", "itunes.batcher").
	Name string

	// Needs lists names of OTHER services this service's Build func will
	// Get[T]. The container enforces that Build can only Get services listed
	// here. Needs is the single source of truth for the build-time dep graph.
	Needs []string

	// Build constructs the service instance. May call Get[T](c, name) for
	// any name in Needs.
	Build func(c *Container) (any, error)
}

var registered = map[string]ServiceDef{}

// Register appends a ServiceDef to the package-level factory list.
// Called from init() in a domain package's register.go.
// Panics on duplicate Name or missing required field — caught at startup,
// never at runtime.
func Register(def ServiceDef) {
	if def.Name == "" {
		panic("serviceregistry: ServiceDef.Name is required")
	}
	if def.Build == nil {
		panic(fmt.Sprintf("serviceregistry: ServiceDef.Build is required (name=%q)", def.Name))
	}
	if _, dup := registered[def.Name]; dup {
		panic(fmt.Sprintf("serviceregistry: duplicate name: %q", def.Name))
	}
	registered[def.Name] = def
}

// ResetForTest clears the package-level factory list. ONLY for tests
// that need to register isolated ServiceDefs without polluting production
// registrations. Production code never calls this.
func ResetForTest() {
	registered = map[string]ServiceDef{}
}
