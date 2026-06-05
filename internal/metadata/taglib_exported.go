// file: internal/metadata/taglib_exported.go
// version: 1.1.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f
//
// Exported wrappers around the internal taglib read/write functions.
// Both the WASM (taglib_support.go) and CGO (taglib_cgo.go) build
// variants define the same internal signatures; this file calls them
// without needing to duplicate the build-tag logic.

package metadata

import "github.com/falkcorp/audiobook-organizer/internal/tagger"

// packageSafeWriteDeps holds the optional Deluge-guard dependencies wired
// in at startup by the server via SetSafeWriteDeps. Zero value = no guard
// (writes proceed in-place as before).
var packageSafeWriteDeps tagger.SafeWriteDeps

// SetSafeWriteDeps installs the pre-flight guard dependencies for all taglib
// writes in this package. Must be called once at server startup, before any
// tag writes occur. Both fields of deps should be non-nil for the guard to be
// fully effective.
func SetSafeWriteDeps(deps tagger.SafeWriteDeps) {
	packageSafeWriteDeps = deps
}

// ReadRawTags returns the raw tag key→values map for a file via TagLib.
// Keys are uppercase (e.g. "COMPOSER", "ALBUMARTIST"). Useful for
// maintenance tools that need the actual on-disk value rather than the
// parsed Metadata struct produced by ExtractMetadata.
func ReadRawTags(filePath string) (map[string][]string, error) {
	return readTagsWithTaglib(filePath)
}

// WriteSingleTag writes one tag property to a file without disturbing
// any other tags. Pass value="" to clear the property.
func WriteSingleTag(filePath, tagName, value string) error {
	return writeSingleTagWithTaglib(filePath, tagName, value)
}
