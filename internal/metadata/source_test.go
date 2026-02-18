// file: internal/metadata/source_test.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2f3a-4b5c-d6e7f8a9b0c1

package metadata

import "testing"

// TestInterfaceCompliance verifies all clients implement MetadataSource.
func TestInterfaceCompliance(t *testing.T) {
	var _ MetadataSource = (*OpenLibraryClient)(nil)
	var _ MetadataSource = (*GoogleBooksClient)(nil)
	var _ MetadataSource = (*AudnexusClient)(nil)
}
