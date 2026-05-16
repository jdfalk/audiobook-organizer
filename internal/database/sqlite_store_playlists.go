// file: internal/database/sqlite_store_playlists.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-ef01-567890123456
// last-edited: 2026-05-01

package database

import (
	"database/sql"
	"time"
)

func (s *SQLiteStore) AddPlaybackEvent(event *PlaybackEvent) error {
	event.CreatedAt = time.Now()
	event.Version = 1
	_, err := s.db.Exec(
		`INSERT INTO playback_events (user_id, book_id, segment_id, position_seconds, event_type, play_speed, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.UserID, event.BookID, event.SegmentID, event.PositionSec, event.EventType, event.PlaySpeed, event.CreatedAt, event.Version,
	)
	return err
}

func (s *SQLiteStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error) {
	rows, err := s.db.Query(
		`SELECT user_id, book_id, segment_id, position_seconds, event_type, play_speed, created_at, version
		 FROM playback_events WHERE user_id = ? AND book_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, bookNumericID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []PlaybackEvent
	for rows.Next() {
		var e PlaybackEvent
		if err := rows.Scan(&e.UserID, &e.BookID, &e.SegmentID, &e.PositionSec, &e.EventType, &e.PlaySpeed, &e.CreatedAt, &e.Version); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) UpdatePlaybackProgress(progress *PlaybackProgress) error {
	progress.UpdatedAt = time.Now()
	progress.Version++
	_, err := s.db.Exec(
		`INSERT INTO playback_progress (user_id, book_id, segment_id, position_seconds, percent_complete, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, book_id) DO UPDATE SET segment_id=excluded.segment_id, position_seconds=excluded.position_seconds,
		 percent_complete=excluded.percent_complete, updated_at=excluded.updated_at, version=excluded.version`,
		progress.UserID, progress.BookID, progress.SegmentID, progress.PositionSec, progress.Percent, progress.UpdatedAt, progress.Version,
	)
	return err
}

func (s *SQLiteStore) GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error) {
	var p PlaybackProgress
	err := s.db.QueryRow(
		`SELECT user_id, book_id, segment_id, position_seconds, percent_complete, updated_at, version
		 FROM playback_progress WHERE user_id = ? AND book_id = ?`, userID, bookNumericID,
	).Scan(&p.UserID, &p.BookID, &p.SegmentID, &p.PositionSec, &p.Percent, &p.UpdatedAt, &p.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- Stats ----

func (s *SQLiteStore) IncrementBookPlayStats(bookNumericID int, seconds int) error {
	_, err := s.db.Exec(
		`INSERT INTO book_stats (book_id, play_count, listen_seconds, version) VALUES (?, 1, ?, 1)
		 ON CONFLICT(book_id) DO UPDATE SET play_count = play_count + 1, listen_seconds = listen_seconds + ?, version = version + 1`,
		bookNumericID, seconds, seconds,
	)
	return err
}

func (s *SQLiteStore) GetBookStats(bookNumericID int) (*BookStats, error) {
	var bs BookStats
	err := s.db.QueryRow(
		`SELECT book_id, play_count, listen_seconds, version FROM book_stats WHERE book_id = ?`, bookNumericID,
	).Scan(&bs.BookID, &bs.PlayCount, &bs.ListenSeconds, &bs.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &bs, nil
}

func (s *SQLiteStore) IncrementUserListenStats(userID string, seconds int) error {
	_, err := s.db.Exec(
		`INSERT INTO user_stats (user_id, listen_seconds, version) VALUES (?, ?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET listen_seconds = listen_seconds + ?, version = version + 1`,
		userID, seconds, seconds,
	)
	return err
}

func (s *SQLiteStore) GetUserStats(userID string) (*UserStats, error) {
	var us UserStats
	err := s.db.QueryRow(
		`SELECT user_id, listen_seconds, version FROM user_stats WHERE user_id = ?`, userID,
	).Scan(&us.UserID, &us.ListenSeconds, &us.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &us, nil
}
func (s *SQLiteStore) CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error) {
	result, err := s.db.Exec("INSERT INTO playlists (name, series_id, file_path) VALUES (?, ?, ?)",
		name, seriesID, filePath)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Playlist{
		ID:       int(id),
		Name:     name,
		SeriesID: seriesID,
		FilePath: filePath,
	}, nil
}

func (s *SQLiteStore) GetPlaylistByID(id int) (*Playlist, error) {
	var playlist Playlist
	err := s.db.QueryRow("SELECT id, name, series_id, file_path FROM playlists WHERE id = ?", id).
		Scan(&playlist.ID, &playlist.Name, &playlist.SeriesID, &playlist.FilePath)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &playlist, nil
}

func (s *SQLiteStore) GetPlaylistBySeriesID(seriesID int) (*Playlist, error) {
	var playlist Playlist
	err := s.db.QueryRow("SELECT id, name, series_id, file_path FROM playlists WHERE series_id = ?", seriesID).
		Scan(&playlist.ID, &playlist.Name, &playlist.SeriesID, &playlist.FilePath)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &playlist, nil
}

func (s *SQLiteStore) AddPlaylistItem(playlistID, bookID, position int) error {
	_, err := s.db.Exec("INSERT INTO playlist_items (playlist_id, book_id, position) VALUES (?, ?, ?)",
		playlistID, bookID, position)
	return err
}

func (s *SQLiteStore) GetPlaylistItems(playlistID int) ([]PlaylistItem, error) {
	rows, err := s.db.Query(`SELECT id, playlist_id, book_id, position
		FROM playlist_items WHERE playlist_id = ? ORDER BY position`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PlaylistItem
	for rows.Next() {
		var item PlaylistItem
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.BookID, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
