// file: internal/server/version_ingest.go
// version: 1.1.0
// guid: 3e1f2a9b-4c5d-4a70-b8c5-3d7e0f1b9a99
//
// Thin wrappers delegating to internal/versions package.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/versions"
)

// IngestVersionParams is an alias for versions.IngestVersionParams.
type IngestVersionParams = versions.IngestVersionParams

// CreateIngestVersion delegates to versions.CreateIngestVersion.
func CreateIngestVersion(store database.Store, params IngestVersionParams) (*database.BookVersion, error) {
	return versions.CreateIngestVersion(store, params)
}

// hashFile delegates to versions.HashFile.
func hashFile(path string) (string, error) {
	return versions.HashFile(path)
}
