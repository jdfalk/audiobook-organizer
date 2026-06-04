// file: internal/database/sqlite_store_users.go
// version: 1.2.0
// guid: c3d4e5f6-a7b8-9012-cdef-g34567890123
// last-edited: 2026-06-04

package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ulid "github.com/oklog/ulid/v2"
)

// ErrSQLiteRBACUnsupported is returned by every SQLiteStore role method. The
// SQLite backend has no roles schema, so role-based access control cannot work
// there. Previously these methods returned silent success / empty results,
// which let bootstrap "succeed" while creating an admin whose role never
// resolved — so every authenticated request then 403'd with no explanation
// (pen-test finding HIGH-4b). Callers should branch on
// errors.Is(err, ErrSQLiteRBACUnsupported) and direct operators to PebbleDB.
var ErrSQLiteRBACUnsupported = errors.New("SQLite backend does not support RBAC; use PebbleDB for multi-user/role features")

func (s *SQLiteStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error) {
	id := ulid.Make().String()
	now := time.Now()
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal roles: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO users (id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		id, username, email, passwordHashAlgo, passwordHash, string(rolesJSON), status, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &User{
		ID: id, Username: username, Email: email,
		PasswordHashAlgo: passwordHashAlgo, PasswordHash: passwordHash,
		Roles: roles, Status: status, CreatedAt: now, UpdatedAt: now, Version: 1,
	}, nil
}

func (s *SQLiteStore) scanUser(row rowScanner) (*User, error) {
	var u User
	var rolesJSON string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHashAlgo, &u.PasswordHash,
		&rolesJSON, &u.Status, &u.CreatedAt, &u.UpdatedAt, &u.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(rolesJSON), &u.Roles)
	return &u, nil
}

func (s *SQLiteStore) GetUserByID(id string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version FROM users WHERE id = ?`, id))
}

func (s *SQLiteStore) GetUserByUsername(username string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version FROM users WHERE username = ?`, username))
}

func (s *SQLiteStore) GetUserByEmail(email string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, email, password_hash_algo, password_hash, roles, status, created_at, updated_at, version FROM users WHERE email = ?`, email))
}

func (s *SQLiteStore) UpdateUser(user *User) error {
	rolesJSON, _ := json.Marshal(user.Roles)
	user.UpdatedAt = time.Now()
	user.Version++
	_, err := s.db.Exec(
		`UPDATE users SET username=?, email=?, password_hash_algo=?, password_hash=?, roles=?, status=?, updated_at=?, version=? WHERE id=?`,
		user.Username, user.Email, user.PasswordHashAlgo, user.PasswordHash,
		string(rolesJSON), user.Status, user.UpdatedAt, user.Version, user.ID,
	)
	return err
}

// ---- Sessions ----

func (s *SQLiteStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	id := ulid.Make().String()
	now := time.Now()
	expiresAt := now.Add(ttl)
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at, ip, user_agent, revoked, version) VALUES (?, ?, ?, ?, ?, ?, 0, 1)`,
		id, userID, now, expiresAt, ip, userAgent,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return &Session{
		ID: id, UserID: userID, CreatedAt: now, ExpiresAt: expiresAt,
		IP: ip, UserAgent: userAgent, Revoked: false, Version: 1,
	}, nil
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var sess Session
	var revoked int
	err := s.db.QueryRow(
		`SELECT id, user_id, created_at, expires_at, ip, user_agent, revoked, version FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UserAgent, &revoked, &sess.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.Revoked = revoked != 0
	return &sess, nil
}

func (s *SQLiteStore) RevokeSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET revoked = 1 WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListUserSessions(userID string) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, created_at, expires_at, ip, user_agent, revoked, version FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var sess Session
		var revoked int
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UserAgent, &revoked, &sess.Version); err != nil {
			return nil, err
		}
		sess.Revoked = revoked != 0
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *SQLiteStore) DeleteExpiredSessions(now time.Time) (int, error) {
	result, err := s.db.Exec(`DELETE FROM sessions WHERE revoked = 1 OR expires_at <= ?`, now)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(rows), nil
}

func (s *SQLiteStore) ListUsers() ([]User, error) { return nil, nil }

func (s *SQLiteStore) CountUsers() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ---- Roles ----
//
// SQLite has no roles schema. The SQLite backend is deprecated for production
// use (PebbleDB is canonical) and adding a roles schema + migration there is
// not worth the cost. Rather than silently succeed — which made RBAC appear to
// work while granting no permissions (pen-test finding HIGH-4b) — every role
// method now fails loudly with ErrSQLiteRBACUnsupported so callers (bootstrap,
// role seeding, permission resolution) can surface a clear "use PebbleDB" error.

func (s *SQLiteStore) GetRoleByID(id string) (*Role, error) {
	return nil, ErrSQLiteRBACUnsupported
}

func (s *SQLiteStore) GetRoleByName(name string) (*Role, error) {
	return nil, ErrSQLiteRBACUnsupported
}

func (s *SQLiteStore) ListRoles() ([]Role, error) {
	return nil, ErrSQLiteRBACUnsupported
}

func (s *SQLiteStore) CreateRole(role *Role) (*Role, error) {
	return nil, ErrSQLiteRBACUnsupported
}

func (s *SQLiteStore) UpdateRole(role *Role) error {
	return ErrSQLiteRBACUnsupported
}

func (s *SQLiteStore) DeleteRole(id string) error {
	return ErrSQLiteRBACUnsupported
}

// ---- User positions + book state (SQLite no-op stubs, spec 3.6) ----

func (s *SQLiteStore) SetUserPosition(userID, bookID, segmentID string, positionSeconds float64) error {
	return nil
}
func (s *SQLiteStore) GetUserPosition(userID, bookID string) (*UserPosition, error) {
	return nil, nil
}
func (s *SQLiteStore) ListUserPositionsForBook(userID, bookID string) ([]UserPosition, error) {
	return nil, nil
}
func (s *SQLiteStore) ClearUserPositions(userID, bookID string) error { return nil }
func (s *SQLiteStore) SetUserBookState(state *UserBookState) error    { return nil }
func (s *SQLiteStore) GetUserBookState(userID, bookID string) (*UserBookState, error) {
	return nil, nil
}
func (s *SQLiteStore) ListUserBookStatesByStatus(userID, status string, limit, offset int) ([]UserBookState, error) {
	return nil, nil
}
func (s *SQLiteStore) ListUserPositionsSince(userID string, t time.Time) ([]UserPosition, error) {
	return nil, nil
}

// ---- Book versions (SQLite no-op stubs, spec 3.1) ----

func (s *SQLiteStore) CreateBookVersion(v *BookVersion) (*BookVersion, error) { return v, nil }
func (s *SQLiteStore) GetBookVersion(id string) (*BookVersion, error)         { return nil, nil }
func (s *SQLiteStore) GetBookVersionsByBookID(bookID string) ([]BookVersion, error) {
	return nil, nil
}
func (s *SQLiteStore) GetActiveVersionForBook(bookID string) (*BookVersion, error) {
	return nil, nil
}
func (s *SQLiteStore) UpdateBookVersion(v *BookVersion) error { return nil }
func (s *SQLiteStore) DeleteBookVersion(id string) error      { return nil }
func (s *SQLiteStore) GetBookVersionByTorrentHash(hash string) (*BookVersion, error) {
	return nil, nil
}
func (s *SQLiteStore) ListTrashedBookVersions() ([]BookVersion, error) { return nil, nil }
func (s *SQLiteStore) ListPurgedBookVersions() ([]BookVersion, error)  { return nil, nil }

// ---- User playlists (SQLite no-op stubs, spec 3.4) ----

func (s *SQLiteStore) CreateUserPlaylist(pl *UserPlaylist) (*UserPlaylist, error) { return pl, nil }
func (s *SQLiteStore) GetUserPlaylist(id string) (*UserPlaylist, error)           { return nil, nil }
func (s *SQLiteStore) GetUserPlaylistByName(name string) (*UserPlaylist, error)   { return nil, nil }
func (s *SQLiteStore) GetUserPlaylistByITunesPID(pid string) (*UserPlaylist, error) {
	return nil, nil
}
func (s *SQLiteStore) ListUserPlaylists(playlistType string, limit, offset int) ([]UserPlaylist, int, error) {
	return nil, 0, nil
}
func (s *SQLiteStore) ListUserPlaylistsForUser(userID, playlistType string, limit, offset int) ([]UserPlaylist, int, error) {
	return nil, 0, nil
}
func (s *SQLiteStore) UpdateUserPlaylist(pl *UserPlaylist) error       { return nil }
func (s *SQLiteStore) DeleteUserPlaylist(id string) error              { return nil }
func (s *SQLiteStore) ListDirtyUserPlaylists() ([]UserPlaylist, error) { return nil, nil }

// ---- API keys + Invites (SQLite no-op stubs) ----

func (s *SQLiteStore) CreateAPIKey(key *APIKey) (*APIKey, error)                    { return key, nil }
func (s *SQLiteStore) GetAPIKey(id string) (*APIKey, error)                         { return nil, nil }
func (s *SQLiteStore) GetAPIKeyByHash(hash string) (*APIKey, error)                 { return nil, nil }
func (s *SQLiteStore) ListAPIKeysForUser(userID string) ([]APIKey, error)           { return nil, nil }
func (s *SQLiteStore) ListAllAPIKeys() ([]APIKey, error)                            { return nil, nil }
func (s *SQLiteStore) RevokeAPIKey(id string) error                                 { return nil }
func (s *SQLiteStore) SetAPIKeyStatus(id, status string, at time.Time) error        { return nil }
func (s *SQLiteStore) TouchAPIKeyLastUsed(id string, at time.Time, ip string) error { return nil }
func (s *SQLiteStore) CreateInvite(invite *Invite) (*Invite, error)                 { return invite, nil }
func (s *SQLiteStore) GetInvite(token string) (*Invite, error)                      { return nil, nil }
func (s *SQLiteStore) ListActiveInvites() ([]Invite, error)                         { return nil, nil }
func (s *SQLiteStore) DeleteInvite(token string) error                              { return nil }
func (s *SQLiteStore) ConsumeInvite(token, algo, hash string) (*User, error) {
	return nil, fmt.Errorf("invites not supported on SQLite backend")
}

// ---- Per-User Preferences ----

func (s *SQLiteStore) SetUserPreferenceForUser(userID, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_preferences (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		fmt.Sprintf("user:%s:%s", userID, key), value, time.Now(),
	)
	return err
}

func (s *SQLiteStore) GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error) {
	var pref UserPreferenceKV
	var rawValue sql.NullString
	err := s.db.QueryRow(
		`SELECT value, updated_at FROM user_preferences WHERE key = ?`,
		fmt.Sprintf("user:%s:%s", userID, key),
	).Scan(&rawValue, &pref.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pref.UserID = userID
	pref.Key = key
	if rawValue.Valid {
		pref.Value = rawValue.String
	}
	return &pref, nil
}

func (s *SQLiteStore) GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error) {
	prefix := fmt.Sprintf("user:%s:", userID)
	rows, err := s.db.Query(`SELECT key, value, updated_at FROM user_preferences WHERE key LIKE ?`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prefs []UserPreferenceKV
	for rows.Next() {
		var fullKey string
		var rawValue sql.NullString
		var pref UserPreferenceKV
		if err := rows.Scan(&fullKey, &rawValue, &pref.UpdatedAt); err != nil {
			return nil, err
		}
		pref.UserID = userID
		pref.Key = strings.TrimPrefix(fullKey, prefix)
		if rawValue.Valid {
			pref.Value = rawValue.String
		}
		prefs = append(prefs, pref)
	}
	return prefs, rows.Err()
}
func (s *SQLiteStore) GetUserPreference(key string) (*UserPreference, error) {
	var pref UserPreference
	err := s.db.QueryRow("SELECT id, key, value, updated_at FROM user_preferences WHERE key = ?", key).
		Scan(&pref.ID, &pref.Key, &pref.Value, &pref.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

func (s *SQLiteStore) SetUserPreference(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO user_preferences (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?`,
		key, value, time.Now(), value, time.Now())
	return err
}

func (s *SQLiteStore) GetAllUserPreferences() ([]UserPreference, error) {
	rows, err := s.db.Query("SELECT id, key, value, updated_at FROM user_preferences ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preferences []UserPreference
	for rows.Next() {
		var pref UserPreference
		if err := rows.Scan(&pref.ID, &pref.Key, &pref.Value, &pref.UpdatedAt); err != nil {
			return nil, err
		}
		preferences = append(preferences, pref)
	}
	return preferences, rows.Err()
}
