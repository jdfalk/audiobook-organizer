// file: pkg/plugin/sdk/plugin.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-f012-3456789abcde
// last-edited: 2026-05-06

package sdk

// DisableMode controls how a plugin is shut down.
type DisableMode int

const (
	// DisableImmediate stops the plugin and all in-flight operations immediately.
	DisableImmediate DisableMode = iota
	// DisableWhenIdle waits for all in-flight operations to complete before stopping.
	DisableWhenIdle
)

// Plugin is the entry point for an audiobook-organizer plugin.
type Plugin interface {
	// ID returns the plugin's globally unique identifier (e.g., "acoustid", "openai").
	ID() string
	// Name returns the human-readable plugin name.
	Name() string
	// Version returns the plugin's version string (semver recommended).
	Version() string
	// Register is called at startup to register operations. Implementations MUST
	// call r.RegisterOp for each operation defined by the plugin.
	Register(r Registry) error
}
