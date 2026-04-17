// file: internal/database/iface_misc.go
// version: 1.0.0
// guid: 473781a7-1a31-4914-b7c7-8efc91f9f7e6

package database

import "time"

// LifecycleStore covers store startup/teardown.
type LifecycleStore interface {
	Close() error
	Reset() error
}

// NarratorStore covers narrators + book-narrator joins.
type NarratorStore interface {
	CreateNarrator(name string) (*Narrator, error)
	GetNarratorByID(id int) (*Narrator, error)
	GetNarratorByName(name string) (*Narrator, error)
	ListNarrators() ([]Narrator, error)
	GetBookNarrators(bookID string) ([]BookNarrator, error)
	SetBookNarrators(bookID string, narrators []BookNarrator) error
}

// WorkStore covers Work CRUD.
type WorkStore interface {
	GetAllWorks() ([]Work, error)
	GetWorkByID(id string) (*Work, error)
	CreateWork(work *Work) (*Work, error)
	UpdateWork(id string, work *Work) (*Work, error)
	DeleteWork(id string) error
	GetBooksByWorkID(workID string) ([]Book, error)
}

// SessionStore covers authenticated session CRUD.
type SessionStore interface {
	CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error)
	GetSession(id string) (*Session, error)
	RevokeSession(id string) error
	ListUserSessions(userID string) ([]Session, error)
	DeleteExpiredSessions(now time.Time) (int, error)
}

// RoleStore covers Role CRUD.
type RoleStore interface {
	GetRoleByID(id string) (*Role, error)
	GetRoleByName(name string) (*Role, error)
	ListRoles() ([]Role, error)
	CreateRole(role *Role) (*Role, error)
	UpdateRole(role *Role) error
	DeleteRole(id string) error
}

// APIKeyStore covers APIKey CRUD and revocation.
type APIKeyStore interface {
	CreateAPIKey(key *APIKey) (*APIKey, error)
	GetAPIKey(id string) (*APIKey, error)
	ListAPIKeysForUser(userID string) ([]APIKey, error)
	RevokeAPIKey(id string) error
	TouchAPIKeyLastUsed(id string, at time.Time) error
}

// InviteStore covers Invite CRUD and atomic consume.
type InviteStore interface {
	CreateInvite(invite *Invite) (*Invite, error)
	GetInvite(token string) (*Invite, error)
	ListActiveInvites() ([]Invite, error)
	DeleteInvite(token string) error
	ConsumeInvite(token, passwordHashAlgo, passwordHash string) (*User, error)
}

// UserPreferenceStore covers both global and per-user preferences.
type UserPreferenceStore interface {
	GetUserPreference(key string) (*UserPreference, error)
	SetUserPreference(key, value string) error
	GetAllUserPreferences() ([]UserPreference, error)
	SetUserPreferenceForUser(userID, key, value string) error
	GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error)
	GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error)
}

// UserPositionStore covers per-user position + derived book state.
type UserPositionStore interface {
	SetUserPosition(userID, bookID, segmentID string, positionSeconds float64) error
	GetUserPosition(userID, bookID string) (*UserPosition, error)
	ListUserPositionsForBook(userID, bookID string) ([]UserPosition, error)
	ClearUserPositions(userID, bookID string) error
	SetUserBookState(state *UserBookState) error
	GetUserBookState(userID, bookID string) (*UserBookState, error)
	ListUserBookStatesByStatus(userID, status string, limit, offset int) ([]UserBookState, error)
	ListUserPositionsSince(userID string, t time.Time) ([]UserPosition, error)
}

// BookVersionStore covers version CRUD, lifecycle, and lookups.
type BookVersionStore interface {
	CreateBookVersion(v *BookVersion) (*BookVersion, error)
	GetBookVersion(id string) (*BookVersion, error)
	GetBookVersionsByBookID(bookID string) ([]BookVersion, error)
	GetActiveVersionForBook(bookID string) (*BookVersion, error)
	UpdateBookVersion(v *BookVersion) error
	DeleteBookVersion(id string) error
	GetBookVersionByTorrentHash(hash string) (*BookVersion, error)
	ListTrashedBookVersions() ([]BookVersion, error)
	ListPurgedBookVersions() ([]BookVersion, error)
}

// BookFileStore covers the canonical BookFile surface.
type BookFileStore interface {
	CreateBookFile(file *BookFile) error
	UpdateBookFile(id string, file *BookFile) error
	GetBookFiles(bookID string) ([]BookFile, error)
	GetBookFileByID(bookID, fileID string) (*BookFile, error)
	GetBookFileByPID(itunesPID string) (*BookFile, error)
	GetBookFileByPath(filePath string) (*BookFile, error)
	DeleteBookFile(id string) error
	DeleteBookFilesForBook(bookID string) error
	UpsertBookFile(file *BookFile) error
	BatchUpsertBookFiles(files []*BookFile) error
	MoveBookFilesToBook(fileIDs []string, sourceBookID, targetBookID string) error
}

// BookSegmentStore covers the deprecated segment surface, kept until
// the segment-removal PR.
type BookSegmentStore interface {
	CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error)
	UpdateBookSegment(segment *BookSegment) error
	ListBookSegments(bookNumericID int) ([]BookSegment, error)
	MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error
	GetBookSegmentByID(segmentID string) (*BookSegment, error)
	MoveSegmentsToBook(segmentIDs []string, targetBookNumericID int) error
}

// PlaylistStore covers the legacy series-playlist auto-generator.
type PlaylistStore interface {
	CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error)
	GetPlaylistByID(id int) (*Playlist, error)
	GetPlaylistBySeriesID(seriesID int) (*Playlist, error)
	AddPlaylistItem(playlistID, bookID, position int) error
	GetPlaylistItems(playlistID int) ([]PlaylistItem, error)
}

// UserPlaylistStore covers smart + static user playlists (spec 3.4).
type UserPlaylistStore interface {
	CreateUserPlaylist(pl *UserPlaylist) (*UserPlaylist, error)
	GetUserPlaylist(id string) (*UserPlaylist, error)
	GetUserPlaylistByName(name string) (*UserPlaylist, error)
	GetUserPlaylistByITunesPID(pid string) (*UserPlaylist, error)
	ListUserPlaylists(playlistType string, limit, offset int) ([]UserPlaylist, int, error)
	UpdateUserPlaylist(pl *UserPlaylist) error
	DeleteUserPlaylist(id string) error
	ListDirtyUserPlaylists() ([]UserPlaylist, error)
}

// ImportPathStore covers managed import path CRUD.
type ImportPathStore interface {
	GetAllImportPaths() ([]ImportPath, error)
	GetImportPathByID(id int) (*ImportPath, error)
	GetImportPathByPath(path string) (*ImportPath, error)
	CreateImportPath(path, name string) (*ImportPath, error)
	UpdateImportPath(id int, importPath *ImportPath) error
	DeleteImportPath(id int) error
}

// MetadataStore covers MetadataFieldState, change history, and
// alternative titles.
type MetadataStore interface {
	GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error)
	UpsertMetadataFieldState(state *MetadataFieldState) error
	DeleteMetadataFieldState(bookID, field string) error
	RecordMetadataChange(record *MetadataChangeRecord) error
	GetMetadataChangeHistory(bookID string, field string, limit int) ([]MetadataChangeRecord, error)
	GetBookChangeHistory(bookID string, limit int) ([]MetadataChangeRecord, error)
	GetBookAlternativeTitles(bookID string) ([]BookAlternativeTitle, error)
	AddBookAlternativeTitle(bookID, title, source, language string) error
	RemoveBookAlternativeTitle(bookID, title string) error
	SetBookAlternativeTitles(bookID string, titles []BookAlternativeTitle) error
}

// HashBlocklistStore covers DoNotImport entries.
type HashBlocklistStore interface {
	IsHashBlocked(hash string) (bool, error)
	AddBlockedHash(hash, reason string) error
	RemoveBlockedHash(hash string) error
	GetAllBlockedHashes() ([]DoNotImport, error)
	GetBlockedHashByHash(hash string) (*DoNotImport, error)
}

// RawKVStore covers the low-level key-value escape hatch.
type RawKVStore interface {
	SetRaw(key string, value []byte) error
	GetRaw(key string) ([]byte, error)
	DeleteRaw(key string) error
	ScanPrefix(prefix string) ([]KVPair, error)
}

// PlaybackStore covers playback events, progress, and stats.
type PlaybackStore interface {
	AddPlaybackEvent(event *PlaybackEvent) error
	ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error)
	UpdatePlaybackProgress(progress *PlaybackProgress) error
	GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error)
	IncrementBookPlayStats(bookNumericID int, seconds int) error
	GetBookStats(bookNumericID int) (*BookStats, error)
	IncrementUserListenStats(userID string, seconds int) error
	GetUserStats(userID string) (*UserStats, error)
}

// SettingsStore covers persistent encrypted configuration.
type SettingsStore interface {
	GetSetting(key string) (*Setting, error)
	SetSetting(key, value, typ string, isSecret bool) error
	GetAllSettings() ([]Setting, error)
	DeleteSetting(key string) error
}

// StatsStore covers aggregate counts and dashboard metrics.
type StatsStore interface {
	CountFiles() (int, error)
	CountAuthors() (int, error)
	CountSeries() (int, error)
	GetBookCountsByLocation(rootDir string) (library, import_ int, err error)
	GetBookSizesByLocation(rootDir string) (librarySize, importSize int64, err error)
	GetDashboardStats() (*DashboardStats, error)
}

// MaintenanceStore covers database maintenance and scan-cache.
type MaintenanceStore interface {
	Optimize() error
	GetScanCacheMap() (map[string]ScanCacheEntry, error)
	UpdateScanCache(bookID string, mtime int64, size int64) error
	MarkNeedsRescan(bookID string) error
	GetDirtyBookFolders() ([]string, error)
}

// SystemActivityStore covers cross-cutting system activity log.
type SystemActivityStore interface {
	AddSystemActivityLog(source, level, message string) error
	GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error)
	PruneSystemActivityLogs(olderThan time.Time) (int, error)
}
