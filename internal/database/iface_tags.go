// file: internal/database/iface_tags.go
// version: 1.0.0
// guid: 9129bad9-0aa9-4eda-82fb-b945f0393674

package database

// TagStore covers book/author/series tag operations (source-tracked).
// Matches the "Tags" section of the legacy Store interface.
type TagStore interface {
	// Book tags
	AddBookTag(bookID, tag string) error
	AddBookTagWithSource(bookID, tag, source string) error
	RemoveBookTag(bookID, tag string) error
	RemoveBookTagsByPrefix(bookID, prefix, source string) error
	GetBookTags(bookID string) ([]string, error)
	GetBookTagsDetailed(bookID string) ([]BookTag, error)
	SetBookTags(bookID string, tags []string) error
	ListAllTags() ([]TagWithCount, error)
	GetBooksByTag(tag string) ([]string, error)

	// Author tags
	AddAuthorTag(authorID int, tag string) error
	AddAuthorTagWithSource(authorID int, tag, source string) error
	RemoveAuthorTag(authorID int, tag string) error
	RemoveAuthorTagsByPrefix(authorID int, prefix, source string) error
	GetAuthorTags(authorID int) ([]string, error)
	GetAuthorTagsDetailed(authorID int) ([]BookTag, error)
	SetAuthorTags(authorID int, tags []string) error
	ListAllAuthorTags() ([]TagWithCount, error)
	GetAuthorsByTag(tag string) ([]int, error)

	// Series tags
	AddSeriesTag(seriesID int, tag string) error
	AddSeriesTagWithSource(seriesID int, tag, source string) error
	RemoveSeriesTag(seriesID int, tag string) error
	RemoveSeriesTagsByPrefix(seriesID int, prefix, source string) error
	GetSeriesTags(seriesID int) ([]string, error)
	GetSeriesTagsDetailed(seriesID int) ([]BookTag, error)
	SetSeriesTags(seriesID int, tags []string) error
	ListAllSeriesTags() ([]TagWithCount, error)
	GetSeriesByTag(tag string) ([]int, error)
}

// UserTagStore covers free-form per-book user tags (the *BookUserTag* variants).
type UserTagStore interface {
	GetBookUserTags(bookID string) ([]string, error)
	SetBookUserTags(bookID string, tags []string) error
	AddBookUserTag(bookID string, tag string) error
	RemoveBookUserTag(bookID string, tag string) error
}
