// file: internal/metadata/taglib_stub.go
// version: 1.1.0
// guid: 4f3e2d1c-0b9a-8d7e-6c5b-4a3f2e1d0c9b

//go:build !taglib

package metadata

import (
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
)

// taglibAvailable false when not built with taglib
var taglibAvailable = false

// writeMetadataWithTaglib stub when taglib not compiled in
func writeMetadataWithTaglib(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	return ErrTaglibUnavailable
}
