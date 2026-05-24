// file: internal/database/chai_sync.go
// version: 1.1.0
// guid: f1e2d3c4-b5a6-4789-0abc-def012345678
// last-edited: 2026-05-24

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cockroachdb/pebble/v2"
)

// ── SQL null helpers ─────────────────────────────────────────────────────────
// Chai does not support parameterized queries, so we build SQL strings manually.
// All helpers return either NULL or a properly-quoted/formatted literal.

func chaiNullableString(s *string) string {
	if s == nil || *s == "" {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", strings.ReplaceAll(*s, "'", "''"))
}

func chaiNullableInt(i *int) string {
	if i == nil {
		return "NULL"
	}
	return fmt.Sprintf("%d", *i)
}

func chaiNullableInt64(i *int64) string {
	if i == nil {
		return "NULL"
	}
	return fmt.Sprintf("%d", *i)
}

func chaiNullableBool(b *bool) string {
	if b == nil {
		return "NULL"
	}
	if *b {
		return "true"
	}
	return "false"
}

func chaiNullableFloat64(f *float64) string {
	if f == nil {
		return "NULL"
	}
	return fmt.Sprintf("%g", *f)
}

func chaiNullableTime(t *time.Time) string {
	if t == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", t.UTC().Format("2006-01-02T15:04:05"))
}

func chaiEscapeStr(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ── UpsertBookToChaiDB ───────────────────────────────────────────────────────

// UpsertBookToChaiDB inserts or replaces a book and its relationships in the Chai
// SQL tables. It uses a DELETE-then-INSERT strategy within a single transaction
// because Chai does not support ON CONFLICT.
//
// Called on every CreateBook / UpdateBook; never hard-fails (Pebble is source of truth).
func (p *PebbleStore) UpsertBookToChaiDB(ctx context.Context, book *Book) error {
	if p.chai == nil {
		return nil
	}

	tx, err := p.chai.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("chai begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	bookID := chaiEscapeStr(book.ID)

	// Delete existing rows for this book (book_authors and book_files first, then books).
	for _, delSQL := range []string{
		fmt.Sprintf("DELETE FROM book_authors WHERE book_id = '%s'", bookID),
		fmt.Sprintf("DELETE FROM book_files WHERE book_id = '%s'", bookID),
		fmt.Sprintf("DELETE FROM books WHERE id = '%s'", bookID),
	} {
		if _, execErr := tx.ExecContext(ctx, delSQL); execErr != nil {
			return fmt.Errorf("chai delete books: %w", execErr)
		}
	}

	// Insert book row.
	insertBook := fmt.Sprintf(`INSERT INTO books (
		id, title, author_id, series_id, series_sequence,
		file_path, format, duration,
		work_id, narrator, edition, description, language, publisher, genre,
		print_year, audiobook_release_year, isbn10, isbn13, asin,
		open_library_id, hardcover_id, google_books_id,
		itunes_persistent_id, itunes_date_added, itunes_play_count,
		itunes_last_played, itunes_rating, itunes_bookmark,
		itunes_import_source, itunes_path, original_filename,
		bitrate_kbps, codec, sample_rate_hz, channels, bit_depth, quality,
		is_primary_version, version_group_id, version_notes,
		file_hash, file_size, original_file_hash, organized_file_hash,
		library_state, quantity, marked_for_deletion, marked_for_deletion_at,
		quarantine_reason, quarantined_at,
		created_at, updated_at, metadata_updated_at, last_written_at,
		last_organize_operation_id, last_organized_at,
		metadata_review_status, metadata_source,
		book_sig_v1, book_sig_segments, book_sig_built_at,
		book_sig_v1_mask, book_sig_coverage_pct,
		itunes_sync_status, audible_runtime_min, metadata_source_hash,
		merged_into_book_id,
		audible_rating_overall, audible_rating_performance, audible_rating_story,
		audible_rating_count, audible_num_reviews,
		google_rating_average, google_rating_count,
		user_rating_overall, user_rating_story, user_rating_performance,
		user_rating_notes, cover_url, narrators_json, source_import_path,
		last_scan_mtime, last_scan_size, needs_rescan
	) VALUES (
		'%s', '%s', %s, %s, %s,
		'%s', '%s', %s,
		%s, %s, %s, %s, %s, %s, %s,
		%s, %s, %s, %s, %s,
		%s, %s, %s,
		%s, %s, %s,
		%s, %s, %s,
		%s, %s, %s,
		%s, %s, %s, %s, %s, %s,
		%s, %s, %s,
		%s, %s, %s, %s,
		%s, %s, %s, %s,
		%s, %s,
		%s, %s, %s, %s,
		%s, %s,
		%s, %s,
		%s, %s, %s,
		%s, %s,
		%s, %s, %s,
		%s,
		%s, %s, %s,
		%s, %s,
		%s, %s,
		%s, %s, %s,
		%s, %s, %s, %s,
		%s, %s, %s
	)`,
		// id, title, author_id, series_id, series_sequence
		bookID,
		chaiEscapeStr(book.Title),
		chaiNullableInt(book.AuthorID),
		chaiNullableInt(book.SeriesID),
		chaiNullableInt(book.SeriesSequence),
		// file_path, format, duration
		chaiEscapeStr(book.FilePath),
		chaiEscapeStr(book.Format),
		chaiNullableInt(book.Duration),
		// work_id, narrator, edition, description, language, publisher, genre
		chaiNullableString(book.WorkID),
		chaiNullableString(book.Narrator),
		chaiNullableString(book.Edition),
		chaiNullableString(book.Description),
		chaiNullableString(book.Language),
		chaiNullableString(book.Publisher),
		chaiNullableString(book.Genre),
		// print_year, audiobook_release_year, isbn10, isbn13, asin
		chaiNullableInt(book.PrintYear),
		chaiNullableInt(book.AudiobookReleaseYear),
		chaiNullableString(book.ISBN10),
		chaiNullableString(book.ISBN13),
		chaiNullableString(book.ASIN),
		// external provider IDs
		chaiNullableString(book.OpenLibraryID),
		chaiNullableString(book.HardcoverID),
		chaiNullableString(book.GoogleBooksID),
		// iTunes fields
		chaiNullableString(book.ITunesPersistentID),
		chaiNullableTime(book.ITunesDateAdded),
		chaiNullableInt(book.ITunesPlayCount),
		chaiNullableTime(book.ITunesLastPlayed),
		chaiNullableInt(book.ITunesRating),
		chaiNullableInt64(book.ITunesBookmark),
		chaiNullableString(book.ITunesImportSource),
		chaiNullableString(book.ITunesPath),
		chaiNullableString(book.OriginalFilename),
		// media info
		chaiNullableInt(book.Bitrate),
		chaiNullableString(book.Codec),
		chaiNullableInt(book.SampleRate),
		chaiNullableInt(book.Channels),
		chaiNullableInt(book.BitDepth),
		chaiNullableString(book.Quality),
		// version management
		chaiNullableBool(book.IsPrimaryVersion),
		chaiNullableString(book.VersionGroupID),
		chaiNullableString(book.VersionNotes),
		// file hashes
		chaiNullableString(book.FileHash),
		chaiNullableInt64(book.FileSize),
		chaiNullableString(book.OriginalFileHash),
		chaiNullableString(book.OrganizedFileHash),
		// lifecycle
		chaiNullableString(book.LibraryState),
		chaiNullableInt(book.Quantity),
		chaiNullableBool(book.MarkedForDeletion),
		chaiNullableTime(book.MarkedForDeletionAt),
		chaiNullableString(book.QuarantineReason),
		chaiNullableTime(book.QuarantinedAt),
		// timestamps
		chaiNullableTime(book.CreatedAt),
		chaiNullableTime(book.UpdatedAt),
		chaiNullableTime(book.MetadataUpdatedAt),
		chaiNullableTime(book.LastWrittenAt),
		// organize tracking
		chaiNullableString(book.LastOrganizeOperationID),
		chaiNullableTime(book.LastOrganizedAt),
		// metadata review
		chaiNullableString(book.MetadataReviewStatus),
		chaiNullableString(book.MetadataSource),
		// book sig
		chaiNullableString(book.BookSigV1),
		chaiNullableInt(book.BookSigSegments),
		chaiNullableTime(book.BookSigBuiltAt),
		chaiNullableString(book.BookSigV1Mask),
		chaiNullableInt(book.BookSigCoveragePct),
		// iTunes sync status + audible runtime + metadata source hash + merged
		chaiNullableString(book.ITunesSyncStatus),
		chaiNullableInt(book.AudibleRuntimeMin),
		chaiNullableString(book.MetadataSourceHash),
		chaiNullableString(book.MergedIntoBookID),
		// Audible ratings
		chaiNullableFloat64(book.AudibleRatingOverall),
		chaiNullableFloat64(book.AudibleRatingPerformance),
		chaiNullableFloat64(book.AudibleRatingStory),
		chaiNullableInt(book.AudibleRatingCount),
		chaiNullableInt(book.AudibleNumReviews),
		// Google ratings
		chaiNullableFloat64(book.GoogleRatingAverage),
		chaiNullableInt(book.GoogleRatingCount),
		// User ratings
		chaiNullableFloat64(book.UserRatingOverall),
		chaiNullableFloat64(book.UserRatingStory),
		chaiNullableFloat64(book.UserRatingPerformance),
		chaiNullableString(book.UserRatingNotes),
		// cover/narrators
		chaiNullableString(book.CoverURL),
		chaiNullableString(book.NarratorsJSON),
		chaiNullableString(book.SourceImportPath),
		// scan cache
		chaiNullableInt64(book.LastScanMtime),
		chaiNullableInt64(book.LastScanSize),
		chaiNullableBool(book.NeedsRescan),
	)

	if _, execErr := tx.ExecContext(ctx, insertBook); execErr != nil {
		return fmt.Errorf("chai insert book: %w", execErr)
	}

	// Insert book_authors rows from the per-book JSON blob stored at book_authors:<bookID>.
	bookAuthors, baErr := p.GetBookAuthors(book.ID)
	if baErr != nil {
		slog.Warn("chai_sync: failed to load book_authors for book", "book_id", book.ID, "error", baErr)
	}

	// Fall back to AuthorID if no book_authors record exists (legacy books).
	if len(bookAuthors) == 0 && book.AuthorID != nil {
		bookAuthors = []BookAuthor{{
			BookID:   book.ID,
			AuthorID: *book.AuthorID,
			Role:     "author",
			Position: 0,
		}}
	}

	for i, ba := range bookAuthors {
		baID := fmt.Sprintf("%s_a%d", bookID, i)
		role := ba.Role
		if role == "" {
			role = "author"
		}
		insertBA := fmt.Sprintf(`INSERT INTO book_authors (id, book_id, author_id, role, position, marked_for_deletion) VALUES (
			'%s', '%s', %d, '%s', %d, false
		)`,
			baID,
			bookID,
			ba.AuthorID,
			chaiEscapeStr(role),
			ba.Position,
		)
		if _, execErr := tx.ExecContext(ctx, insertBA); execErr != nil {
			return fmt.Errorf("chai insert book_author: %w", execErr)
		}
	}

	// Insert book_files rows by iterating the Pebble key range for this book.
	// Key format: book_file:<bookID>:<fileID>
	// The Chai book_files table is intentionally slimmer — only columns needed
	// for aggregation (file count, duration, size, hash coverage).
	filePrefix := fmt.Sprintf("book_file:%s:", book.ID)
	fileUpper := fmt.Sprintf("book_file:%s;", book.ID)
	fileIter, fileIterErr := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(filePrefix),
		UpperBound: []byte(fileUpper),
	})
	if fileIterErr != nil {
		slog.Warn("chai_sync: failed to create book_files iterator", "book_id", book.ID, "error", fileIterErr)
	} else {
		defer fileIter.Close()
		for fileIter.First(); fileIter.Valid(); fileIter.Next() {
			rawFile, fileValErr := fileIter.ValueAndErr()
			if fileValErr != nil {
				slog.Warn("chai_sync: failed to read book_file value", "key", string(fileIter.Key()), "error", fileValErr)
				continue
			}
			// Clone before iterator advances.
			fileBytes := make([]byte, len(rawFile))
			copy(fileBytes, rawFile)

			var bf BookFile
			if jsonErr := json.Unmarshal(fileBytes, &bf); jsonErr != nil {
				slog.Warn("chai_sync: failed to unmarshal book_file", "key", string(fileIter.Key()), "error", jsonErr)
				continue
			}

			// Build nullable helpers for BookFile fields.
			var bfDuration *int
			if bf.Duration != 0 {
				bfDuration = &bf.Duration
			}
			var bfSize *int64
			if bf.FileSize != 0 {
				bfSize = &bf.FileSize
			}
			var bfHash *string
			if bf.FileHash != "" {
				bfHash = &bf.FileHash
			}
			var bfFormat *string
			if bf.Format != "" {
				bfFormat = &bf.Format
			}
			bfMissing := bf.Missing
			var bfCreatedAt *time.Time
			if !bf.CreatedAt.IsZero() {
				bfCreatedAt = &bf.CreatedAt
			}
			var bfUpdatedAt *time.Time
			if !bf.UpdatedAt.IsZero() {
				bfUpdatedAt = &bf.UpdatedAt
			}
			markedDel := false // BookFile has no MarkedForDeletion field

			insertBF := fmt.Sprintf(`INSERT INTO book_files (
				id, book_id, file_path, format, duration_ms, file_size_bytes,
				file_hash, missing, created_at, updated_at, marked_for_deletion
			) VALUES (
				'%s', '%s', '%s', %s, %s, %s,
				%s, %s, %s, %s, %s
			)`,
				chaiEscapeStr(bf.ID),
				chaiEscapeStr(bf.BookID),
				chaiEscapeStr(bf.FilePath),
				chaiNullableString(bfFormat),
				chaiNullableInt(bfDuration),
				chaiNullableInt64(bfSize),
				chaiNullableString(bfHash),
				chaiNullableBool(&bfMissing),
				chaiNullableTime(bfCreatedAt),
				chaiNullableTime(bfUpdatedAt),
				chaiNullableBool(&markedDel),
			)
			if _, execErr := tx.ExecContext(ctx, insertBF); execErr != nil {
				slog.Warn("chai_sync: failed to insert book_file", "file_id", bf.ID, "error", execErr)
				// Non-fatal: continue with remaining files.
			}
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("chai commit upsert: %w", commitErr)
	}
	committed = true
	return nil
}

// ── DeleteBookFromChaiDB ─────────────────────────────────────────────────────

// DeleteBookFromChaiDB removes a book and its relationships from Chai SQL tables.
// Called on every DeleteBook. Never hard-fails (Pebble is source of truth).
func (p *PebbleStore) DeleteBookFromChaiDB(ctx context.Context, bookID string) error {
	if p.chai == nil {
		return nil
	}

	tx, err := p.chai.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("chai begin tx for delete: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	escaped := chaiEscapeStr(bookID)
	for _, delSQL := range []string{
		fmt.Sprintf("DELETE FROM book_authors WHERE book_id = '%s'", escaped),
		fmt.Sprintf("DELETE FROM book_files WHERE book_id = '%s'", escaped),
		fmt.Sprintf("DELETE FROM books WHERE id = '%s'", escaped),
	} {
		if _, execErr := tx.ExecContext(ctx, delSQL); execErr != nil {
			return fmt.Errorf("chai delete: %w", execErr)
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("chai commit delete: %w", commitErr)
	}
	committed = true
	return nil
}

// ── BackfillChaiFromPebble ───────────────────────────────────────────────────

// BackfillChaiFromPebble iterates all books in Pebble and upserts each to Chai.
// Idempotent — safe to run multiple times. Logs progress every 1000 books.
// Returns the number of books successfully synced and any terminal error.
func (p *PebbleStore) BackfillChaiFromPebble(ctx context.Context) (synced int, err error) {
	if p.chai == nil {
		return 0, fmt.Errorf("chai database not initialized")
	}

	slog.Info("chai_sync: starting backfill from Pebble")

	iter, iterErr := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:"),
		UpperBound: []byte("book;"),
	})
	if iterErr != nil {
		return 0, fmt.Errorf("chai_sync backfill: failed to create iterator: %w", iterErr)
	}
	defer iter.Close()

	var failed int
	for iter.First(); iter.Valid(); iter.Next() {
		select {
		case <-ctx.Done():
			slog.Warn("chai_sync: backfill cancelled", "synced", synced, "failed", failed)
			return synced, ctx.Err()
		default:
		}

		key := string(iter.Key())

		// Skip secondary index keys — only canonical "book:<ULID>" keys have exactly
		// one colon in the suffix. Anything with an additional colon or slash is an index.
		suffix := strings.TrimPrefix(key, "book:")
		if strings.ContainsAny(suffix, ":/") {
			continue
		}

		rawVal, valErr := iter.ValueAndErr()
		if valErr != nil {
			slog.Warn("chai_sync: failed to read value", "key", key, "error", valErr)
			failed++
			continue
		}

		// Clone bytes before the iterator moves.
		val := make([]byte, len(rawVal))
		copy(val, rawVal)

		var book Book
		if jsonErr := json.Unmarshal(val, &book); jsonErr != nil {
			slog.Warn("chai_sync: failed to unmarshal book", "key", key, "error", jsonErr)
			failed++
			continue
		}

		if upsertErr := p.UpsertBookToChaiDB(ctx, &book); upsertErr != nil {
			slog.Warn("chai_sync: upsert failed", "book_id", book.ID, "error", upsertErr)
			failed++
			continue
		}

		synced++
		if synced%1000 == 0 {
			slog.Info("chai_sync: backfill progress", "synced", synced, "failed", failed)
		}
	}

	slog.Info("chai_sync: backfill complete", "synced", synced, "failed", failed)
	return synced, nil
}
