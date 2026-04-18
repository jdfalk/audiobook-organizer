// file: internal/server/version_fingerprint.go
// version: 1.1.0
// guid: 8d5e7f4c-9c5a-4a70-b8c5-3d7e0f1b9a99
//
// Thin wrappers delegating to internal/versions package.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/versions"
)

// FingerprintMatch is an alias for versions.FingerprintMatch.
type FingerprintMatch = versions.FingerprintMatch

// CheckFingerprint delegates to versions.CheckFingerprint.
func CheckFingerprint(store database.Store, torrentHash string, fileHashes []string) *FingerprintMatch {
	return versions.CheckFingerprint(store, torrentHash, fileHashes)
}
