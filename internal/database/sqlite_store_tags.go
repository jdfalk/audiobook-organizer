// file: internal/database/sqlite_store_tags.go
// version: 1.1.0
// guid: b7c8d9e0-f1a2-3b4c-5d6e-7f8a9b0c1d2e
// last-edited: 2026-05-02

package database

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/util"
)

// Hash Blocklist Methods

// IsHashBlocked checks if a hash is in the blocklist
func (s *SQLiteStore) IsHashBlocked(hash string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM do_not_import WHERE hash = ?", hash).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddBookTag adds a user-sourced tag to a book. Server code that
// auto-applies tags should call AddBookTagWithSource so provenance
// is preserved (see migration 47 namespace notes).
func (s *SQLiteStore) AddBookTag(bookID, tag string) error {
	return s.AddBookTagWithSource(bookID, tag, "user")
}

// AddBookTagWithSource upserts a tag on a book with an explicit
// source ("user" or "system"). Later writes overwrite the source
// so a user can claim a system tag or vice versa.
func (s *SQLiteStore) AddBookTagWithSource(bookID, tag, source string) error {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	if source == "" {
		source = "user"
	}
	_, err := s.db.Exec(
		`INSERT INTO book_tags (book_id, tag, source) VALUES (?, ?, ?)
		 ON CONFLICT(book_id, tag) DO UPDATE SET source = excluded.source`,
		bookID, tag, source,
	)
	return err
}

// RemoveBookTag removes a tag from a book regardless of source.
func (s *SQLiteStore) RemoveBookTag(bookID, tag string) error {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	_, err := s.db.Exec(
		`DELETE FROM book_tags WHERE book_id = ? AND tag = ?`,
		bookID, tag,
	)
	return err
}

// RemoveBookTagsByPrefix removes every tag on a book whose name
// begins with `prefix`, optionally scoped to a specific source.
// Used to clear a namespace before writing a fresh system tag —
// e.g., re-applying metadata from a new source clears any existing
// `metadata:source:*` system tags first so each book has exactly
// one source tag at a time. Empty `source` matches all sources.
func (s *SQLiteStore) RemoveBookTagsByPrefix(bookID, prefix, source string) error {
	prefix = util.NormalizeString(prefix)
	if prefix == "" {
		return fmt.Errorf("prefix cannot be empty")
	}
	if source == "" {
		_, err := s.db.Exec(
			`DELETE FROM book_tags WHERE book_id = ? AND tag LIKE ?`,
			bookID, prefix+"%",
		)
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM book_tags WHERE book_id = ? AND tag LIKE ? AND source = ?`,
		bookID, prefix+"%", source,
	)
	return err
}

// GetBookTags returns all tag strings for a book, sorted
// alphabetically. Both user and system tags are returned —
// callers that need provenance should use GetBookTagsDetailed.
func (s *SQLiteStore) GetBookTags(bookID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT tag FROM book_tags WHERE book_id = ? ORDER BY tag`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// GetBookTagsDetailed returns tags with source and creation time.
// Used by the frontend to render system tags differently from
// user tags (outlined chip + not-deletable by default).
func (s *SQLiteStore) GetBookTagsDetailed(bookID string) ([]BookTag, error) {
	rows, err := s.db.Query(
		`SELECT tag, source, created_at FROM book_tags WHERE book_id = ? ORDER BY source, tag`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookTag
	for rows.Next() {
		var bt BookTag
		if err := rows.Scan(&bt.Tag, &bt.Source, &bt.CreatedAt); err != nil {
			return nil, err
		}
		bt.BookID = bookID
		out = append(out, bt)
	}
	return out, rows.Err()
}

// SetBookTags replaces all USER tags on a book with the given set.
// System tags (dedup:*, metadata:source:*, ...) survive the bulk
// replace so the user-facing operation doesn't clobber server-
// applied provenance.
func (s *SQLiteStore) SetBookTags(bookID string, tags []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Only delete user-sourced tags so system tags survive.
	if _, err := tx.Exec(
		`DELETE FROM book_tags WHERE book_id = ? AND source = 'user'`,
		bookID,
	); err != nil {
		return err
	}

	for _, tag := range tags {
		tag = util.NormalizeString(tag)
		if tag == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO book_tags (book_id, tag, source) VALUES (?, ?, 'user')`,
			bookID, tag,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListAllTags returns all unique book tags with their usage counts.
// `source` is populated only when every row sharing the tag string
// has the same source — otherwise it's empty (mixed provenance).
func (s *SQLiteStore) ListAllTags() ([]TagWithCount, error) {
	rows, err := s.db.Query(
		`SELECT tag,
		        COUNT(*) AS count,
		        CASE WHEN COUNT(DISTINCT source) = 1 THEN MAX(source) ELSE '' END AS source
		 FROM book_tags
		 GROUP BY tag
		 ORDER BY tag`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TagWithCount
	for rows.Next() {
		var tc TagWithCount
		if err := rows.Scan(&tc.Tag, &tc.Count, &tc.Source); err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// GetBooksByTag returns all book IDs that have the given tag.
func (s *SQLiteStore) GetBooksByTag(tag string) ([]string, error) {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}

	rows, err := s.db.Query(
		`SELECT book_id FROM book_tags WHERE tag = ?`,
		tag,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		bookIDs = append(bookIDs, id)
	}
	return bookIDs, rows.Err()
}

// ---------- Author / Series tag helpers ----------
//
// Author and series tags share the exact same shape as book_tags
// (see migrations 47 and 48), so instead of duplicating SQL across
// 18 parallel methods we parameterize by table name and ID column.
// `idCol` is "author_id" or "series_id"; the caller passes the
// integer ID directly. Helpers stay unexported — the Store
// interface exposes typed wrappers below.

func (s *SQLiteStore) addTagGeneric(table, idCol string, id any, tag, source string) error {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	if source == "" {
		source = "user"
	}
	stmt := fmt.Sprintf(
		`INSERT INTO %s (%s, tag, source) VALUES (?, ?, ?)
		 ON CONFLICT(%s, tag) DO UPDATE SET source = excluded.source`,
		table, idCol, idCol,
	)
	_, err := s.db.Exec(stmt, id, tag, source)
	return err
}

func (s *SQLiteStore) removeTagGeneric(table, idCol string, id any, tag string) error {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	stmt := fmt.Sprintf(`DELETE FROM %s WHERE %s = ? AND tag = ?`, table, idCol)
	_, err := s.db.Exec(stmt, id, tag)
	return err
}

func (s *SQLiteStore) removeTagsByPrefixGeneric(table, idCol string, id any, prefix, source string) error {
	prefix = util.NormalizeString(prefix)
	if prefix == "" {
		return fmt.Errorf("prefix cannot be empty")
	}
	if source == "" {
		stmt := fmt.Sprintf(`DELETE FROM %s WHERE %s = ? AND tag LIKE ?`, table, idCol)
		_, err := s.db.Exec(stmt, id, prefix+"%")
		return err
	}
	stmt := fmt.Sprintf(
		`DELETE FROM %s WHERE %s = ? AND tag LIKE ? AND source = ?`,
		table, idCol,
	)
	_, err := s.db.Exec(stmt, id, prefix+"%", source)
	return err
}

func (s *SQLiteStore) getTagsGeneric(table, idCol string, id any) ([]string, error) {
	stmt := fmt.Sprintf(`SELECT tag FROM %s WHERE %s = ? ORDER BY tag`, table, idCol)
	rows, err := s.db.Query(stmt, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *SQLiteStore) getTagsDetailedGeneric(table, idCol string, id any) ([]BookTag, error) {
	stmt := fmt.Sprintf(
		`SELECT tag, source, created_at FROM %s WHERE %s = ? ORDER BY source, tag`,
		table, idCol,
	)
	rows, err := s.db.Query(stmt, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookTag
	for rows.Next() {
		var bt BookTag
		if err := rows.Scan(&bt.Tag, &bt.Source, &bt.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, bt)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) setTagsGeneric(table, idCol string, id any, tags []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delStmt := fmt.Sprintf(`DELETE FROM %s WHERE %s = ? AND source = 'user'`, table, idCol)
	if _, err := tx.Exec(delStmt, id); err != nil {
		return err
	}

	insStmt := fmt.Sprintf(
		`INSERT OR IGNORE INTO %s (%s, tag, source) VALUES (?, ?, 'user')`,
		table, idCol,
	)
	for _, tag := range tags {
		tag = util.NormalizeString(tag)
		if tag == "" {
			continue
		}
		if _, err := tx.Exec(insStmt, id, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) listAllTagsGeneric(table string) ([]TagWithCount, error) {
	stmt := fmt.Sprintf(
		`SELECT tag, COUNT(*) AS count,
		        CASE WHEN COUNT(DISTINCT source) = 1 THEN MAX(source) ELSE '' END AS source
		 FROM %s
		 GROUP BY tag
		 ORDER BY tag`,
		table,
	)
	rows, err := s.db.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TagWithCount
	for rows.Next() {
		var tc TagWithCount
		if err := rows.Scan(&tc.Tag, &tc.Count, &tc.Source); err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// ---------- Author tag wrappers ----------

func (s *SQLiteStore) AddAuthorTag(authorID int, tag string) error {
	return s.addTagGeneric("author_tags", "author_id", authorID, tag, "user")
}
func (s *SQLiteStore) AddAuthorTagWithSource(authorID int, tag, source string) error {
	return s.addTagGeneric("author_tags", "author_id", authorID, tag, source)
}
func (s *SQLiteStore) RemoveAuthorTag(authorID int, tag string) error {
	return s.removeTagGeneric("author_tags", "author_id", authorID, tag)
}
func (s *SQLiteStore) RemoveAuthorTagsByPrefix(authorID int, prefix, source string) error {
	return s.removeTagsByPrefixGeneric("author_tags", "author_id", authorID, prefix, source)
}
func (s *SQLiteStore) GetAuthorTags(authorID int) ([]string, error) {
	return s.getTagsGeneric("author_tags", "author_id", authorID)
}
func (s *SQLiteStore) GetAuthorTagsDetailed(authorID int) ([]BookTag, error) {
	return s.getTagsDetailedGeneric("author_tags", "author_id", authorID)
}
func (s *SQLiteStore) SetAuthorTags(authorID int, tags []string) error {
	return s.setTagsGeneric("author_tags", "author_id", authorID, tags)
}
func (s *SQLiteStore) ListAllAuthorTags() ([]TagWithCount, error) {
	return s.listAllTagsGeneric("author_tags")
}
func (s *SQLiteStore) GetAuthorsByTag(tag string) ([]int, error) {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}
	rows, err := s.db.Query(`SELECT author_id FROM author_tags WHERE tag = ?`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---------- Series tag wrappers ----------

func (s *SQLiteStore) AddSeriesTag(seriesID int, tag string) error {
	return s.addTagGeneric("series_tags", "series_id", seriesID, tag, "user")
}
func (s *SQLiteStore) AddSeriesTagWithSource(seriesID int, tag, source string) error {
	return s.addTagGeneric("series_tags", "series_id", seriesID, tag, source)
}
func (s *SQLiteStore) RemoveSeriesTag(seriesID int, tag string) error {
	return s.removeTagGeneric("series_tags", "series_id", seriesID, tag)
}
func (s *SQLiteStore) RemoveSeriesTagsByPrefix(seriesID int, prefix, source string) error {
	return s.removeTagsByPrefixGeneric("series_tags", "series_id", seriesID, prefix, source)
}
func (s *SQLiteStore) GetSeriesTags(seriesID int) ([]string, error) {
	return s.getTagsGeneric("series_tags", "series_id", seriesID)
}
func (s *SQLiteStore) GetSeriesTagsDetailed(seriesID int) ([]BookTag, error) {
	return s.getTagsDetailedGeneric("series_tags", "series_id", seriesID)
}
func (s *SQLiteStore) SetSeriesTags(seriesID int, tags []string) error {
	return s.setTagsGeneric("series_tags", "series_id", seriesID, tags)
}
func (s *SQLiteStore) ListAllSeriesTags() ([]TagWithCount, error) {
	return s.listAllTagsGeneric("series_tags")
}
func (s *SQLiteStore) GetSeriesByTag(tag string) ([]int, error) {
	tag = util.NormalizeString(tag)
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}
	rows, err := s.db.Query(`SELECT series_id FROM series_tags WHERE tag = ?`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
