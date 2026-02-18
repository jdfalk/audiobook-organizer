// file: internal/metadata/source.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-e1f2a3b4c5d6

package metadata

// MetadataSource is a pluggable metadata provider.
type MetadataSource interface {
	Name() string
	SearchByTitle(title string) ([]BookMetadata, error)
	SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error)
}
