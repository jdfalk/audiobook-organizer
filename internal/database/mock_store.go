// file: internal/database/mock_store.go
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package database

import (
	"time"
)

// MockStore is a simple mock implementation for testing services
type MockStore struct {
	// Book methods
	GetBookByIDFunc    func(id string) (*Book, error)
	GetBookByFilePathFunc func(path string) (*Book, error)
	GetAllBooksFunc    func(limit, offset int) ([]Book, error)
	GetBooksByWorkIDFunc func(workID string) ([]Book, error)
	GetBooksBySeriesIDFunc func(seriesID int) ([]Book, error)
	GetBooksByAuthorIDFunc func(authorID int) ([]Book, error)
	GetBookByFileHashFunc func(hash string) (*Book, error)
	GetBookByOriginalHashFunc func(hash string) (*Book, error)
	GetBookByOrganizedHashFunc func(hash string) (*Book, error)
	GetDuplicateBooksFunc func() ([][]Book, error)
	CreateBookFunc     func(book *Book) (*Book, error)
	UpdateBookFunc     func(id string, book *Book) (*Book, error)
	DeleteBookFunc     func(id string) error
	SearchBooksFunc    func(query string, limit, offset int) ([]Book, error)
	CountBooksFunc     func() (int, error)
	ListSoftDeletedBooksFunc func(limit, offset int, olderThan *time.Time) ([]Book, error)

	// Work methods
	GetAllWorksFunc    func() ([]Work, error)
	GetWorkByIDFunc    func(id string) (*Work, error)
	CreateWorkFunc     func(work *Work) (*Work, error)
	UpdateWorkFunc     func(id string, work *Work) (*Work, error)
	DeleteWorkFunc     func(id string) error

	// Author methods
	GetAllAuthorsFunc  func() ([]Author, error)
	GetAuthorByIDFunc  func(id int) (*Author, error)
	GetAuthorByNameFunc func(name string) (*Author, error)
	CreateAuthorFunc   func(name string) (*Author, error)

	// Series methods
	GetAllSeriesFunc   func() ([]Series, error)
	GetSeriesByIDFunc  func(id int) (*Series, error)
	GetSeriesByNameFunc func(name string, authorID *int) (*Series, error)
	CreateSeriesFunc   func(name string, authorID *int) (*Series, error)

	// Metadata
	GetMetadataFieldStatesFunc func(bookID string) ([]MetadataFieldState, error)
	UpsertMetadataFieldStateFunc func(state *MetadataFieldState) error
	DeleteMetadataFieldStateFunc func(bookID, field string) error

	// Import Paths
	GetAllImportPathsFunc func() ([]ImportPath, error)
	GetImportPathByIDFunc func(id int) (*ImportPath, error)
	GetImportPathByPathFunc func(path string) (*ImportPath, error)
	CreateImportPathFunc func(path, name string) (*ImportPath, error)
	UpdateImportPathFunc func(id int, importPath *ImportPath) error
	DeleteImportPathFunc func(id int) error

	// Operations
	CreateOperationFunc func(id, opType string, folderPath *string) (*Operation, error)
	GetOperationByIDFunc func(id string) (*Operation, error)
	GetRecentOperationsFunc func(limit int) ([]Operation, error)
	UpdateOperationStatusFunc func(id, status string, progress, total int, message string) error
	UpdateOperationErrorFunc func(id, errorMessage string) error

	// Operation Logs
	AddOperationLogFunc func(operationID, level, message string, details *string) error
	GetOperationLogsFunc func(operationID string) ([]OperationLog, error)

	// User Preferences
	GetUserPreferenceFunc func(key string) (*UserPreference, error)
	SetUserPreferenceFunc func(key, value string) error
	GetAllUserPreferencesFunc func() ([]UserPreference, error)

	// Settings
	GetSettingFunc     func(key string) (*Setting, error)
	SetSettingFunc     func(key, value, typ string, isSecret bool) error
	GetAllSettingsFunc func() ([]Setting, error)
	DeleteSettingFunc  func(key string) error

	// Playlists
	CreatePlaylistFunc func(name string, seriesID *int, filePath string) (*Playlist, error)
	GetPlaylistByIDFunc func(id int) (*Playlist, error)
	GetPlaylistBySeriesIDFunc func(seriesID int) (*Playlist, error)
	AddPlaylistItemFunc func(playlistID, bookID, position int) error
	GetPlaylistItemsFunc func(playlistID int) ([]PlaylistItem, error)

	// Users
	CreateUserFunc     func(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error)
	GetUserByIDFunc    func(id string) (*User, error)
	GetUserByUsernameFunc func(username string) (*User, error)
	GetUserByEmailFunc func(email string) (*User, error)
	UpdateUserFunc     func(user *User) error

	// Sessions
	CreateSessionFunc  func(userID, ip, userAgent string, ttl time.Duration) (*Session, error)
	GetSessionFunc     func(id string) (*Session, error)
	RevokeSessionFunc  func(id string) error
	ListUserSessionsFunc func(userID string) ([]Session, error)

	// Per-user preferences
	SetUserPreferenceForUserFunc func(userID, key, value string) error
	GetUserPreferenceForUserFunc func(userID, key string) (*UserPreferenceKV, error)
	GetAllPreferencesForUserFunc func(userID string) ([]UserPreferenceKV, error)

	// Book segments
	CreateBookSegmentFunc func(bookNumericID int, segment *BookSegment) (*BookSegment, error)
	ListBookSegmentsFunc func(bookNumericID int) ([]BookSegment, error)
	MergeBookSegmentsFunc func(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error

	// Playback events
	AddPlaybackEventFunc func(event *PlaybackEvent) error
	ListPlaybackEventsFunc func(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error)
	UpdatePlaybackProgressFunc func(progress *PlaybackProgress) error
	GetPlaybackProgressFunc func(userID string, bookNumericID int) (*PlaybackProgress, error)

	// Stats
	IncrementBookPlayStatsFunc func(bookNumericID int, seconds int) error
	GetBookStatsFunc func(bookNumericID int) (*BookStats, error)
	IncrementUserListenStatsFunc func(userID string, seconds int) error
	GetUserStatsFunc func(userID string) (*UserStats, error)

	// Hash blocklist
	IsHashBlockedFunc func(hash string) (bool, error)
	AddBlockedHashFunc func(hash, reason string) error
	RemoveBlockedHashFunc func(hash string) error
	GetAllBlockedHashesFunc func() ([]DoNotImport, error)
	GetBlockedHashByHashFunc func(hash string) (*DoNotImport, error)

	// Version Management
	GetBooksByVersionGroupFunc func(groupID string) ([]Book, error)

	// Lifecycle
	CloseFunc func() error
	ResetFunc func() error
}

// Implement all Store interface methods

func (m *MockStore) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockStore) GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error) {
	if m.GetMetadataFieldStatesFunc != nil {
		return m.GetMetadataFieldStatesFunc(bookID)
	}
	return nil, nil
}

func (m *MockStore) UpsertMetadataFieldState(state *MetadataFieldState) error {
	if m.UpsertMetadataFieldStateFunc != nil {
		return m.UpsertMetadataFieldStateFunc(state)
	}
	return nil
}

func (m *MockStore) DeleteMetadataFieldState(bookID, field string) error {
	if m.DeleteMetadataFieldStateFunc != nil {
		return m.DeleteMetadataFieldStateFunc(bookID, field)
	}
	return nil
}

func (m *MockStore) GetAllAuthors() ([]Author, error) {
	if m.GetAllAuthorsFunc != nil {
		return m.GetAllAuthorsFunc()
	}
	return nil, nil
}

func (m *MockStore) GetAuthorByID(id int) (*Author, error) {
	if m.GetAuthorByIDFunc != nil {
		return m.GetAuthorByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetAuthorByName(name string) (*Author, error) {
	if m.GetAuthorByNameFunc != nil {
		return m.GetAuthorByNameFunc(name)
	}
	return nil, nil
}

func (m *MockStore) CreateAuthor(name string) (*Author, error) {
	if m.CreateAuthorFunc != nil {
		return m.CreateAuthorFunc(name)
	}
	return nil, nil
}

func (m *MockStore) GetAllSeries() ([]Series, error) {
	if m.GetAllSeriesFunc != nil {
		return m.GetAllSeriesFunc()
	}
	return nil, nil
}

func (m *MockStore) GetSeriesByID(id int) (*Series, error) {
	if m.GetSeriesByIDFunc != nil {
		return m.GetSeriesByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetSeriesByName(name string, authorID *int) (*Series, error) {
	if m.GetSeriesByNameFunc != nil {
		return m.GetSeriesByNameFunc(name, authorID)
	}
	return nil, nil
}

func (m *MockStore) CreateSeries(name string, authorID *int) (*Series, error) {
	if m.CreateSeriesFunc != nil {
		return m.CreateSeriesFunc(name, authorID)
	}
	return nil, nil
}

func (m *MockStore) GetAllWorks() ([]Work, error) {
	if m.GetAllWorksFunc != nil {
		return m.GetAllWorksFunc()
	}
	return nil, nil
}

func (m *MockStore) GetWorkByID(id string) (*Work, error) {
	if m.GetWorkByIDFunc != nil {
		return m.GetWorkByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) CreateWork(work *Work) (*Work, error) {
	if m.CreateWorkFunc != nil {
		return m.CreateWorkFunc(work)
	}
	return nil, nil
}

func (m *MockStore) UpdateWork(id string, work *Work) (*Work, error) {
	if m.UpdateWorkFunc != nil {
		return m.UpdateWorkFunc(id, work)
	}
	return nil, nil
}

func (m *MockStore) DeleteWork(id string) error {
	if m.DeleteWorkFunc != nil {
		return m.DeleteWorkFunc(id)
	}
	return nil
}

func (m *MockStore) GetBooksByWorkID(workID string) ([]Book, error) {
	if m.GetBooksByWorkIDFunc != nil {
		return m.GetBooksByWorkIDFunc(workID)
	}
	return nil, nil
}

func (m *MockStore) GetAllBooks(limit, offset int) ([]Book, error) {
	if m.GetAllBooksFunc != nil {
		return m.GetAllBooksFunc(limit, offset)
	}
	return nil, nil
}

func (m *MockStore) GetBookByID(id string) (*Book, error) {
	if m.GetBookByIDFunc != nil {
		return m.GetBookByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetBookByFilePath(path string) (*Book, error) {
	if m.GetBookByFilePathFunc != nil {
		return m.GetBookByFilePathFunc(path)
	}
	return nil, nil
}

func (m *MockStore) GetBookByFileHash(hash string) (*Book, error) {
	if m.GetBookByFileHashFunc != nil {
		return m.GetBookByFileHashFunc(hash)
	}
	return nil, nil
}

func (m *MockStore) GetBookByOriginalHash(hash string) (*Book, error) {
	if m.GetBookByOriginalHashFunc != nil {
		return m.GetBookByOriginalHashFunc(hash)
	}
	return nil, nil
}

func (m *MockStore) GetBookByOrganizedHash(hash string) (*Book, error) {
	if m.GetBookByOrganizedHashFunc != nil {
		return m.GetBookByOrganizedHashFunc(hash)
	}
	return nil, nil
}

func (m *MockStore) GetDuplicateBooks() ([][]Book, error) {
	if m.GetDuplicateBooksFunc != nil {
		return m.GetDuplicateBooksFunc()
	}
	return nil, nil
}

func (m *MockStore) GetBooksBySeriesID(seriesID int) ([]Book, error) {
	if m.GetBooksBySeriesIDFunc != nil {
		return m.GetBooksBySeriesIDFunc(seriesID)
	}
	return nil, nil
}

func (m *MockStore) GetBooksByAuthorID(authorID int) ([]Book, error) {
	if m.GetBooksByAuthorIDFunc != nil {
		return m.GetBooksByAuthorIDFunc(authorID)
	}
	return nil, nil
}

func (m *MockStore) CreateBook(book *Book) (*Book, error) {
	if m.CreateBookFunc != nil {
		return m.CreateBookFunc(book)
	}
	return nil, nil
}

func (m *MockStore) UpdateBook(id string, book *Book) (*Book, error) {
	if m.UpdateBookFunc != nil {
		return m.UpdateBookFunc(id, book)
	}
	return nil, nil
}

func (m *MockStore) DeleteBook(id string) error {
	if m.DeleteBookFunc != nil {
		return m.DeleteBookFunc(id)
	}
	return nil
}

func (m *MockStore) SearchBooks(query string, limit, offset int) ([]Book, error) {
	if m.SearchBooksFunc != nil {
		return m.SearchBooksFunc(query, limit, offset)
	}
	return nil, nil
}

func (m *MockStore) CountBooks() (int, error) {
	if m.CountBooksFunc != nil {
		return m.CountBooksFunc()
	}
	return 0, nil
}

func (m *MockStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error) {
	if m.ListSoftDeletedBooksFunc != nil {
		return m.ListSoftDeletedBooksFunc(limit, offset, olderThan)
	}
	return nil, nil
}

func (m *MockStore) GetBooksByVersionGroup(groupID string) ([]Book, error) {
	if m.GetBooksByVersionGroupFunc != nil {
		return m.GetBooksByVersionGroupFunc(groupID)
	}
	return nil, nil
}

func (m *MockStore) GetAllImportPaths() ([]ImportPath, error) {
	if m.GetAllImportPathsFunc != nil {
		return m.GetAllImportPathsFunc()
	}
	return nil, nil
}

func (m *MockStore) GetImportPathByID(id int) (*ImportPath, error) {
	if m.GetImportPathByIDFunc != nil {
		return m.GetImportPathByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetImportPathByPath(path string) (*ImportPath, error) {
	if m.GetImportPathByPathFunc != nil {
		return m.GetImportPathByPathFunc(path)
	}
	return nil, nil
}

func (m *MockStore) CreateImportPath(path, name string) (*ImportPath, error) {
	if m.CreateImportPathFunc != nil {
		return m.CreateImportPathFunc(path, name)
	}
	return nil, nil
}

func (m *MockStore) UpdateImportPath(id int, importPath *ImportPath) error {
	if m.UpdateImportPathFunc != nil {
		return m.UpdateImportPathFunc(id, importPath)
	}
	return nil
}

func (m *MockStore) DeleteImportPath(id int) error {
	if m.DeleteImportPathFunc != nil {
		return m.DeleteImportPathFunc(id)
	}
	return nil
}

func (m *MockStore) CreateOperation(id, opType string, folderPath *string) (*Operation, error) {
	if m.CreateOperationFunc != nil {
		return m.CreateOperationFunc(id, opType, folderPath)
	}
	return nil, nil
}

func (m *MockStore) GetOperationByID(id string) (*Operation, error) {
	if m.GetOperationByIDFunc != nil {
		return m.GetOperationByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetRecentOperations(limit int) ([]Operation, error) {
	if m.GetRecentOperationsFunc != nil {
		return m.GetRecentOperationsFunc(limit)
	}
	return nil, nil
}

func (m *MockStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	if m.UpdateOperationStatusFunc != nil {
		return m.UpdateOperationStatusFunc(id, status, progress, total, message)
	}
	return nil
}

func (m *MockStore) UpdateOperationError(id, errorMessage string) error {
	if m.UpdateOperationErrorFunc != nil {
		return m.UpdateOperationErrorFunc(id, errorMessage)
	}
	return nil
}

func (m *MockStore) AddOperationLog(operationID, level, message string, details *string) error {
	if m.AddOperationLogFunc != nil {
		return m.AddOperationLogFunc(operationID, level, message, details)
	}
	return nil
}

func (m *MockStore) GetOperationLogs(operationID string) ([]OperationLog, error) {
	if m.GetOperationLogsFunc != nil {
		return m.GetOperationLogsFunc(operationID)
	}
	return nil, nil
}

func (m *MockStore) GetUserPreference(key string) (*UserPreference, error) {
	if m.GetUserPreferenceFunc != nil {
		return m.GetUserPreferenceFunc(key)
	}
	return nil, nil
}

func (m *MockStore) SetUserPreference(key, value string) error {
	if m.SetUserPreferenceFunc != nil {
		return m.SetUserPreferenceFunc(key, value)
	}
	return nil
}

func (m *MockStore) GetAllUserPreferences() ([]UserPreference, error) {
	if m.GetAllUserPreferencesFunc != nil {
		return m.GetAllUserPreferencesFunc()
	}
	return nil, nil
}

func (m *MockStore) GetSetting(key string) (*Setting, error) {
	if m.GetSettingFunc != nil {
		return m.GetSettingFunc(key)
	}
	return nil, nil
}

func (m *MockStore) SetSetting(key, value, typ string, isSecret bool) error {
	if m.SetSettingFunc != nil {
		return m.SetSettingFunc(key, value, typ, isSecret)
	}
	return nil
}

func (m *MockStore) GetAllSettings() ([]Setting, error) {
	if m.GetAllSettingsFunc != nil {
		return m.GetAllSettingsFunc()
	}
	return nil, nil
}

func (m *MockStore) DeleteSetting(key string) error {
	if m.DeleteSettingFunc != nil {
		return m.DeleteSettingFunc(key)
	}
	return nil
}

func (m *MockStore) CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error) {
	if m.CreatePlaylistFunc != nil {
		return m.CreatePlaylistFunc(name, seriesID, filePath)
	}
	return nil, nil
}

func (m *MockStore) GetPlaylistByID(id int) (*Playlist, error) {
	if m.GetPlaylistByIDFunc != nil {
		return m.GetPlaylistByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetPlaylistBySeriesID(seriesID int) (*Playlist, error) {
	if m.GetPlaylistBySeriesIDFunc != nil {
		return m.GetPlaylistBySeriesIDFunc(seriesID)
	}
	return nil, nil
}

func (m *MockStore) AddPlaylistItem(playlistID, bookID, position int) error {
	if m.AddPlaylistItemFunc != nil {
		return m.AddPlaylistItemFunc(playlistID, bookID, position)
	}
	return nil
}

func (m *MockStore) GetPlaylistItems(playlistID int) ([]PlaylistItem, error) {
	if m.GetPlaylistItemsFunc != nil {
		return m.GetPlaylistItemsFunc(playlistID)
	}
	return nil, nil
}

func (m *MockStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(username, email, passwordHashAlgo, passwordHash, roles, status)
	}
	return nil, nil
}

func (m *MockStore) GetUserByID(id string) (*User, error) {
	if m.GetUserByIDFunc != nil {
		return m.GetUserByIDFunc(id)
	}
	return nil, nil
}

func (m *MockStore) GetUserByUsername(username string) (*User, error) {
	if m.GetUserByUsernameFunc != nil {
		return m.GetUserByUsernameFunc(username)
	}
	return nil, nil
}

func (m *MockStore) GetUserByEmail(email string) (*User, error) {
	if m.GetUserByEmailFunc != nil {
		return m.GetUserByEmailFunc(email)
	}
	return nil, nil
}

func (m *MockStore) UpdateUser(user *User) error {
	if m.UpdateUserFunc != nil {
		return m.UpdateUserFunc(user)
	}
	return nil
}

func (m *MockStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	if m.CreateSessionFunc != nil {
		return m.CreateSessionFunc(userID, ip, userAgent, ttl)
	}
	return nil, nil
}

func (m *MockStore) GetSession(id string) (*Session, error) {
	if m.GetSessionFunc != nil {
		return m.GetSessionFunc(id)
	}
	return nil, nil
}

func (m *MockStore) RevokeSession(id string) error {
	if m.RevokeSessionFunc != nil {
		return m.RevokeSessionFunc(id)
	}
	return nil
}

func (m *MockStore) ListUserSessions(userID string) ([]Session, error) {
	if m.ListUserSessionsFunc != nil {
		return m.ListUserSessionsFunc(userID)
	}
	return nil, nil
}

func (m *MockStore) SetUserPreferenceForUser(userID, key, value string) error {
	if m.SetUserPreferenceForUserFunc != nil {
		return m.SetUserPreferenceForUserFunc(userID, key, value)
	}
	return nil
}

func (m *MockStore) GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error) {
	if m.GetUserPreferenceForUserFunc != nil {
		return m.GetUserPreferenceForUserFunc(userID, key)
	}
	return nil, nil
}

func (m *MockStore) GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error) {
	if m.GetAllPreferencesForUserFunc != nil {
		return m.GetAllPreferencesForUserFunc(userID)
	}
	return nil, nil
}

func (m *MockStore) CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error) {
	if m.CreateBookSegmentFunc != nil {
		return m.CreateBookSegmentFunc(bookNumericID, segment)
	}
	return nil, nil
}

func (m *MockStore) ListBookSegments(bookNumericID int) ([]BookSegment, error) {
	if m.ListBookSegmentsFunc != nil {
		return m.ListBookSegmentsFunc(bookNumericID)
	}
	return nil, nil
}

func (m *MockStore) MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error {
	if m.MergeBookSegmentsFunc != nil {
		return m.MergeBookSegmentsFunc(bookNumericID, newSegment, supersedeIDs)
	}
	return nil
}

func (m *MockStore) AddPlaybackEvent(event *PlaybackEvent) error {
	if m.AddPlaybackEventFunc != nil {
		return m.AddPlaybackEventFunc(event)
	}
	return nil
}

func (m *MockStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error) {
	if m.ListPlaybackEventsFunc != nil {
		return m.ListPlaybackEventsFunc(userID, bookNumericID, limit)
	}
	return nil, nil
}

func (m *MockStore) UpdatePlaybackProgress(progress *PlaybackProgress) error {
	if m.UpdatePlaybackProgressFunc != nil {
		return m.UpdatePlaybackProgressFunc(progress)
	}
	return nil
}

func (m *MockStore) GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error) {
	if m.GetPlaybackProgressFunc != nil {
		return m.GetPlaybackProgressFunc(userID, bookNumericID)
	}
	return nil, nil
}

func (m *MockStore) IncrementBookPlayStats(bookNumericID int, seconds int) error {
	if m.IncrementBookPlayStatsFunc != nil {
		return m.IncrementBookPlayStatsFunc(bookNumericID, seconds)
	}
	return nil
}

func (m *MockStore) GetBookStats(bookNumericID int) (*BookStats, error) {
	if m.GetBookStatsFunc != nil {
		return m.GetBookStatsFunc(bookNumericID)
	}
	return nil, nil
}

func (m *MockStore) IncrementUserListenStats(userID string, seconds int) error {
	if m.IncrementUserListenStatsFunc != nil {
		return m.IncrementUserListenStatsFunc(userID, seconds)
	}
	return nil
}

func (m *MockStore) GetUserStats(userID string) (*UserStats, error) {
	if m.GetUserStatsFunc != nil {
		return m.GetUserStatsFunc(userID)
	}
	return nil, nil
}

func (m *MockStore) IsHashBlocked(hash string) (bool, error) {
	if m.IsHashBlockedFunc != nil {
		return m.IsHashBlockedFunc(hash)
	}
	return false, nil
}

func (m *MockStore) AddBlockedHash(hash, reason string) error {
	if m.AddBlockedHashFunc != nil {
		return m.AddBlockedHashFunc(hash, reason)
	}
	return nil
}

func (m *MockStore) RemoveBlockedHash(hash string) error {
	if m.RemoveBlockedHashFunc != nil {
		return m.RemoveBlockedHashFunc(hash)
	}
	return nil
}

func (m *MockStore) GetAllBlockedHashes() ([]DoNotImport, error) {
	if m.GetAllBlockedHashesFunc != nil {
		return m.GetAllBlockedHashesFunc()
	}
	return nil, nil
}

func (m *MockStore) GetBlockedHashByHash(hash string) (*DoNotImport, error) {
	if m.GetBlockedHashByHashFunc != nil {
		return m.GetBlockedHashByHashFunc(hash)
	}
	return nil, nil
}

func (m *MockStore) Reset() error {
	if m.ResetFunc != nil {
		return m.ResetFunc()
	}
	return nil
}
