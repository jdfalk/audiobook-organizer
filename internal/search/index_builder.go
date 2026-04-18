// file: internal/search/index_builder.go
// version: 1.1.0
// guid: 8a1c2f4d-5b3e-4f70-b7d6-2e8d0f1b9a57
//
// Helpers that project a database.Book (with its author, series,
// and tag relations resolved) into a BookDocument ready for
// indexing. Used by both the startup full-build path and the
// incremental indexing hooks that fire on book create/update/
// delete.

package search

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// BookToDoc resolves a Book's related rows through the Store and
// returns the flat BookDocument for indexing. Missing relations are
// silently skipped — the document is built best-effort.
func BookToDoc(store interface { database.AuthorReader; database.BookReader; database.SeriesReader; database.TagStore }, book *database.Book) BookDocument {
	doc := BookDocument{
		BookID: book.ID,
		Type:   BookDocType,
		Title:  book.Title,
	}
	if book.Narrator != nil {
		doc.Narrator = *book.Narrator
	}
	if book.Publisher != nil {
		doc.Publisher = *book.Publisher
	}
	if book.Description != nil {
		doc.Description = *book.Description
	}
	doc.FilePath = book.FilePath
	doc.Format = book.Format
	if book.Genre != nil {
		doc.Genre = *book.Genre
	}
	if book.Language != nil {
		doc.Language = *book.Language
	}
	if book.LibraryState != nil {
		doc.LibraryState = *book.LibraryState
	}
	if book.ISBN10 != nil {
		doc.ISBN10 = *book.ISBN10
	}
	if book.ISBN13 != nil {
		doc.ISBN13 = *book.ISBN13
	}
	if book.ASIN != nil {
		doc.ASIN = *book.ASIN
	}
	if book.AudiobookReleaseYear != nil {
		doc.Year = *book.AudiobookReleaseYear
	} else if book.PrintYear != nil {
		doc.Year = *book.PrintYear
	}
	if book.SeriesSequence != nil {
		doc.SeriesNumber = *book.SeriesSequence
	}
	if book.Duration != nil {
		doc.DurationSec = *book.Duration
	}
	if book.Bitrate != nil {
		doc.BitrateKbps = *book.Bitrate
	}
	if book.SampleRate != nil {
		doc.SampleRateHz = *book.SampleRate
	}
	if book.Channels != nil {
		doc.Channels = *book.Channels
	}
	if book.BitDepth != nil {
		doc.BitDepth = *book.BitDepth
	}
	if book.FileSize != nil {
		doc.FileSizeBytes = *book.FileSize
	}
	doc.HasCover = book.CoverURL != nil && *book.CoverURL != ""

	// Resolve author name.
	if store != nil && book.AuthorID != nil {
		if author, err := store.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
			doc.Author = author.Name
		}
	}
	// Resolve series name.
	if store != nil && book.SeriesID != nil {
		if series, err := store.GetSeriesByID(*book.SeriesID); err == nil && series != nil {
			doc.Series = series.Name
		}
	}
	// Resolve tags (user + system). Tags on a book come from the
	// existing BookTag / BookUserTag APIs.
	if store != nil {
		if tags, err := store.GetBookTags(book.ID); err == nil {
			doc.Tags = tags
		}
	}
	return doc
}

// ReindexBookByID convenience: load the book + project + index.
// Used by the update/create hook path; callers that already have a
// Book struct should call BookToDoc + IndexBook directly.
func ReindexBookByID(store interface { database.AuthorReader; database.BookReader; database.SeriesReader; database.TagStore }, idx *BleveIndex, bookID string) error {
	if store == nil || idx == nil {
		return nil
	}
	book, err := store.GetBookByID(bookID)
	if err != nil || book == nil {
		return err
	}
	return idx.IndexBook(BookToDoc(store, book))
}
