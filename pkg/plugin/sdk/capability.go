// file: pkg/plugin/sdk/capability.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-def0-123456789abc
// last-edited: 2026-05-06

package sdk

import "github.com/falkcorp/audiobook-organizer/internal/operations/registry"

// Capability is a coarse permission an OperationDef declares it needs.
type Capability = registry.Capability

// Capability constants.
const (
	CapLibraryRead  = registry.CapLibraryRead
	CapLibraryWrite = registry.CapLibraryWrite
	CapFilesRead    = registry.CapFilesRead
	CapFilesWrite   = registry.CapFilesWrite
	CapFilesExecute = registry.CapFilesExecute

	CapNetworkOpenAI      = registry.CapNetworkOpenAI
	CapNetworkAudible     = registry.CapNetworkAudible
	CapNetworkOpenLibrary = registry.CapNetworkOpenLibrary
	CapNetworkGoogleBooks = registry.CapNetworkGoogleBooks
	CapNetworkITunes      = registry.CapNetworkITunes
	CapNetworkGeneric     = registry.CapNetworkGeneric

	CapScheduleCron  = registry.CapScheduleCron
	CapScheduleEvent = registry.CapScheduleEvent

	CapSubprocessSpawn = registry.CapSubprocessSpawn
	CapDBMigrate       = registry.CapDBMigrate
)
