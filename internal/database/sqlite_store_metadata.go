// file: internal/database/sqlite_store_metadata.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-f012-678901234567
// last-edited: 2026-05-01

package database

import (
"database/sql"
"fmt"
"time"

ulid "github.com/oklog/ulid/v2"
)

func (s *SQLiteStore) GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error) {
	rows, err := s.db.Query(`SELECT book_id, field, fetched_value, override_value, override_locked, updated_at
		FROM metadata_states WHERE book_id = ? ORDER BY field`, bookID)
	if err != nil {
		return nil, fmt.Errorf("failed to query metadata_states: %w", err)
	}
	defer rows.Close()

	var states []MetadataFieldState
	for rows.Next() {
		var state MetadataFieldState
		var fetchedVal, overrideVal sql.NullString

		if err := rows.Scan(&state.BookID, &state.Field, &fetchedVal, &overrideVal, &state.OverrideLocked, &state.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan metadata_state: %w", err)
		}

		if fetchedVal.Valid {
			state.FetchedValue = &fetchedVal.String
		}
		if overrideVal.Valid {
			state.OverrideValue = &overrideVal.String
		}

		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *SQLiteStore) UpsertMetadataFieldState(state *MetadataFieldState) error {
	if state == nil {
		return fmt.Errorf("metadata state cannot be nil")
	}
	if state.BookID == "" || state.Field == "" {
		return fmt.Errorf("book_id and field are required")
	}

	_, err := s.db.Exec(`INSERT INTO metadata_states (book_id, field, fetched_value, override_value, override_locked, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(book_id, field) DO UPDATE SET
			fetched_value = excluded.fetched_value,
			override_value = excluded.override_value,
			override_locked = excluded.override_locked,
			updated_at = CURRENT_TIMESTAMP`,
		state.BookID, state.Field, state.FetchedValue, state.OverrideValue, state.OverrideLocked)
	return err
}

func (s *SQLiteStore) DeleteMetadataFieldState(bookID, field string) error {
	if bookID == "" || field == "" {
		return fmt.Errorf("book_id and field are required")
	}
	_, err := s.db.Exec("DELETE FROM metadata_states WHERE book_id = ? AND field = ?", bookID, field)
	return err
}

// Metadata change history operations

func (s *SQLiteStore) RecordMetadataChange(record *MetadataChangeRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO metadata_changes_history (book_id, field, previous_value, new_value, change_type, source, changed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.BookID, record.Field, record.PreviousValue, record.NewValue,
		record.ChangeType, record.Source, record.ChangedAt,
	)
	return err
}

func (s *SQLiteStore) GetMetadataChangeHistory(bookID string, field string, limit int) ([]MetadataChangeRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, book_id, field, previous_value, new_value, change_type, source, changed_at
		 FROM metadata_changes_history WHERE book_id = ? AND field = ? ORDER BY changed_at DESC LIMIT ?`,
		bookID, field, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MetadataChangeRecord
	for rows.Next() {
		var r MetadataChangeRecord
		var prevVal, newVal, source sql.NullString
		if err := rows.Scan(&r.ID, &r.BookID, &r.Field, &prevVal, &newVal, &r.ChangeType, &source, &r.ChangedAt); err != nil {
			return nil, err
		}
		if prevVal.Valid {
			r.PreviousValue = &prevVal.String
		}
		if newVal.Valid {
			r.NewValue = &newVal.String
		}
		if source.Valid {
			r.Source = source.String
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) GetBookChangeHistory(bookID string, limit int) ([]MetadataChangeRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, book_id, field, previous_value, new_value, change_type, source, changed_at
		 FROM metadata_changes_history WHERE book_id = ? ORDER BY changed_at DESC LIMIT ?`,
		bookID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MetadataChangeRecord
	for rows.Next() {
		var r MetadataChangeRecord
		var prevVal, newVal, source sql.NullString
		if err := rows.Scan(&r.ID, &r.BookID, &r.Field, &prevVal, &newVal, &r.ChangeType, &source, &r.ChangedAt); err != nil {
			return nil, err
		}
		if prevVal.Valid {
			r.PreviousValue = &prevVal.String
		}
		if newVal.Valid {
			r.NewValue = &newVal.String
		}
		if source.Valid {
			r.Source = source.String
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// Playlist operations
func (s *SQLiteStore) AddMetadataRejection(r MetadataRejection) error {
	if r.ID == "" {
		r.ID = ulid.Make().String()
	}
	if r.RejectedAt.IsZero() {
		r.RejectedAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO metadata_rejections
			(id, book_id, source, candidate_asin, candidate_isbn, candidate_title, candidate_author,
			 rejection_reason, score, rejected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.BookID, r.Source,
		nullableStringVal(r.CandidateASIN), nullableStringVal(r.CandidateISBN),
		nullableStringVal(r.CandidateTitle), nullableStringVal(r.CandidateAuthor),
		r.RejectionReason, r.Score, r.RejectedAt,
	)
	if err != nil {
		return fmt.Errorf("AddMetadataRejection: %w", err)
	}
	return nil
}

// GetMetadataRejections returns all rejection records for a book, newest first.
func (s *SQLiteStore) GetMetadataRejections(bookID string) ([]MetadataRejection, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, source,
			COALESCE(candidate_asin,''), COALESCE(candidate_isbn,''),
			COALESCE(candidate_title,''), COALESCE(candidate_author,''),
			rejection_reason, COALESCE(score,0), rejected_at
		FROM metadata_rejections WHERE book_id = ? ORDER BY rejected_at DESC`,
		bookID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetMetadataRejections: %w", err)
	}
	defer rows.Close()
	var out []MetadataRejection
	for rows.Next() {
		var r MetadataRejection
		if err := rows.Scan(
			&r.ID, &r.BookID, &r.Source,
			&r.CandidateASIN, &r.CandidateISBN,
			&r.CandidateTitle, &r.CandidateAuthor,
			&r.RejectionReason, &r.Score, &r.RejectedAt,
		); err != nil {
			return nil, fmt.Errorf("GetMetadataRejections scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteMetadataRejections removes all rejection records for a book.
func (s *SQLiteStore) DeleteMetadataRejections(bookID string) error {
	_, err := s.db.Exec(`DELETE FROM metadata_rejections WHERE book_id = ?`, bookID)
	if err != nil {
		return fmt.Errorf("DeleteMetadataRejections: %w", err)
	}
	return nil
}
