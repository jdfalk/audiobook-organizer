// file: pkg/plugin/sdk/events.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-ef01-23456789abcd
// last-edited: 2026-05-06

package sdk

import "github.com/falkcorp/audiobook-organizer/internal/operations/registry"

// EventSubscription wires an event name to a handler on the OperationDef.
type EventSubscription = registry.EventSubscription

// Bus is the event publishing interface. A nil Bus is safe; all Publish calls
// are skipped when the bus has not been wired.
type Bus = registry.Bus
