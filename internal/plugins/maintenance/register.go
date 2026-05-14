// file: internal/plugins/maintenance/register.go
// version: 2.0.0

// Service registry registration for the maintenance UOS plugin (W5).
//
// **Intentionally not registered.** The maintenance plugin's
// constructor takes a 25-method `ServerDeps` interface that *only*
// *server.Server implements. The interface aggregates scheduler hooks,
// dedup engine handles, AI scan store, activity writer, OpenLibrary
// service, plus a dozen other server-bound capabilities. Decomposing
// it into container services would mean registering ~15 new services
// with their own dependency wiring just so this one plugin can be
// built indirectly — a refactor that would touch every service the
// plugin reaches into.
//
// The actual maintenance OperationDefs are registered inline from
// `internal/server/server.go:~402`:
//
//	if err := maintenanceplugin.New(server).Register(server.opRegistry); err != nil { ... }
//
// That call runs after `wireServerFromContainer` has populated all of
// `*Server`'s fields, so `ServerDeps` is satisfied cleanly. The
// inline pattern is the canonical registration site for this plugin
// and won't move unless the ServerDeps interface itself is broken up.
//
// Earlier revisions of this file registered a typed-nil `*Plugin`
// stub in the container so consumers could `TryGet` it. Nothing ever
// consumed that nil value — it was a placeholder for a future where
// the plugin builds from the container. Until that future arrives,
// the empty registration is just noise; deleted.

package maintenance

// No init() — the maintenance plugin registers from
// internal/server/server.go directly. See file header.
