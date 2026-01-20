// file: internal/database/mock_store.go
// version: 1.0.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

package database

import (
	"fmt"
	"sync"
	"time"
)

// MockStore is a mock implementation of Store for testing
type MockStore struct {
	mu sync.RWMutex

	// Data stores
	Books       map[string]*Book
	Authors     map[int]*Author
	Series      map[int]*Series
	Works       map[string]*Work
	Operations  map[string]*Operation
	OpLogs      map[string][]OperationLog
	Settings    map[string]*Setting
	Preferences map[string]*UserPreference
	ImportPaths map[int]*ImportPath
	Playlists   map[int]*Playlist
	BlockedHash map[string]*DoNotImport
	FieldStates map[string][]MetadataFieldState

	// Counters for ID generation
	NextAuthorID     int
	NextSeriesID     int
	NextBookID       int
	NextOpLogID      int
	NextImportPathID int
	NextPlaylistID   int
	NextPrefID       int

	// Error injection
	ErrorOnNext map[string]error

	// Call tracking
	Calls []MockStoreCall
}

// MockStoreCall records a method call
type MockStoreCall struct {
	Method string
	Args   []interface{}
}

// NewMockStore creates a new mock store
func NewMockStore() *MockStore {
	return &MockStore{
		Books:            make(map[string]*Book),
		Authors:          make(map[int]*Author),
		Series:           make(map[int]*Series),
		Works:            make(map[string]*Work),
		Operations:       make(map[string]*Operation),
		OpLogs:           make(map[string][]OperationLog),
		Settings:         make(map[string]*Setting),
		Preferences:      make(map[string]*UserPreference),
		ImportPaths:      make(map[int]*ImportPath),
		Playlists:        make(map[int]*Playlist),
		BlockedHash:      make(map[string]*DoNotImport),
		FieldStates:      make(map[string][]MetadataFieldState),
		NextAuthorID:     1,
		NextSeriesID:     1,
		NextBookID:       1,
		NextOpLogID:      1,
		NextImportPathID: 1,
		NextPlaylistID:   1,
		NextPrefID:       1,
		ErrorOnNext:      make(map[string]error),
		Calls:            make([]MockStoreCall, 0),
	}
}

func (m *MockStore) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockStoreCall{Method: method, Args: args})
}

func (m *MockStore) checkError(method string) error {
	if err, ok := m.ErrorOnNext[method]; ok {
		delete(m.ErrorOnNext, method)
		return err
	}
	return nil
}

// Close implements Store.Close
func (m *MockStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("Close")
	return m.checkError("Close")
}

// GetMetadataFieldStates implements Store
func (m *MockStore) GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetMetadataFieldStates", bookID)
	if err := m.checkError("GetMetadataFieldStates"); err != nil {
		return nil, err
	}
	return m.FieldStates[bookID], nil
}

// UpsertMetadataFieldState implements Store
func (m *MockStore) UpsertMetadataFieldState(state *MetadataFieldState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("UpsertMetadataFieldState", state)
	if err := m.checkError("UpsertMetadataFieldState"); err != nil {
		return err
	}
	states := m.FieldStates[state.BookID]
	for i, s := range states {
		if s.Field == state.Field {
			states[i] = *state
			return nil
		}
	}
	m.FieldStates[state.BookID] = append(states, *state)
	return nil
}

// DeleteMetadataFieldState implements Store
func (m *MockStore) DeleteMetadataFieldState(bookID, field string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("DeleteMetadataFieldState", bookID, field)
	if err := m.checkError("DeleteMetadataFieldState"); err != nil {
		return err
	}
	states := m.FieldStates[bookID]
	for i, s := range states {
		if s.Field == field {
			m.FieldStates[bookID] = append(states[:i], states[i+1:]...)
			return nil
		}
	}
	return nil
}

// GetAllAuthors implements Store
func (m *MockStore) GetAllAuthors() ([]Author, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllAuthors")
	if err := m.checkError("GetAllAuthors"); err != nil {
		return nil, err
	}
	result := make([]Author, 0, len(m.Authors))
	for _, a := range m.Authors {
		result = append(result, *a)
	}
	return result, nil
}

// GetAuthorByID implements Store
func (m *MockStore) GetAuthorByID(id int) (*Author, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAuthorByID", id)
	if err := m.checkError("GetAuthorByID"); err != nil {
		return nil, err
	}
	if a, ok := m.Authors[id]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("author not found: %d", id)
}

// GetAuthorByName implements Store
func (m *MockStore) GetAuthorByName(name string) (*Author, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAuthorByName", name)
	if err := m.checkError("GetAuthorByName"); err != nil {
		return nil, err
	}
	for _, a := range m.Authors {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("author not found: %s", name)
}

// CreateAuthor implements Store
func (m *MockStore) CreateAuthor(name string) (*Author, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreateAuthor", name)
	if err := m.checkError("CreateAuthor"); err != nil {
		return nil, err
	}
	a := &Author{ID: m.NextAuthorID, Name: name}
	m.Authors[m.NextAuthorID] = a
	m.NextAuthorID++
	return a, nil
}

// GetAllSeries implements Store
func (m *MockStore) GetAllSeries() ([]Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllSeries")
	if err := m.checkError("GetAllSeries"); err != nil {
		return nil, err
	}
	result := make([]Series, 0, len(m.Series))
	for _, s := range m.Series {
		result = append(result, *s)
	}
	return result, nil
}

// GetSeriesByID implements Store
func (m *MockStore) GetSeriesByID(id int) (*Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetSeriesByID", id)
	if err := m.checkError("GetSeriesByID"); err != nil {
		return nil, err
	}
	if s, ok := m.Series[id]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("series not found: %d", id)
}

// GetSeriesByName implements Store
func (m *MockStore) GetSeriesByName(name string, authorID *int) (*Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetSeriesByName", name, authorID)
	if err := m.checkError("GetSeriesByName"); err != nil {
		return nil, err
	}
	for _, s := range m.Series {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, fmt.Errorf("series not found: %s", name)
}

// CreateSeries implements Store
func (m *MockStore) CreateSeries(name string, authorID *int) (*Series, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreateSeries", name, authorID)
	if err := m.checkError("CreateSeries"); err != nil {
		return nil, err
	}
	s := &Series{ID: m.NextSeriesID, Name: name, AuthorID: authorID}
	m.Series[m.NextSeriesID] = s
	m.NextSeriesID++
	return s, nil
}

// GetAllWorks implements Store
func (m *MockStore) GetAllWorks() ([]Work, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllWorks")
	if err := m.checkError("GetAllWorks"); err != nil {
		return nil, err
	}
	result := make([]Work, 0, len(m.Works))
	for _, w := range m.Works {
		result = append(result, *w)
	}
	return result, nil
}

// GetWorkByID implements Store
func (m *MockStore) GetWorkByID(id string) (*Work, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetWorkByID", id)
	if err := m.checkError("GetWorkByID"); err != nil {
		return nil, err
	}
	if w, ok := m.Works[id]; ok {
		return w, nil
	}
	return nil, fmt.Errorf("work not found: %s", id)
}

// CreateWork implements Store
func (m *MockStore) CreateWork(work *Work) (*Work, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreateWork", work)
	if err := m.checkError("CreateWork"); err != nil {
		return nil, err
	}
	if work.ID == "" {
		work.ID = fmt.Sprintf("work-%d", len(m.Works)+1)
	}
	m.Works[work.ID] = work
	return work, nil
}

// UpdateWork implements Store
func (m *MockStore) UpdateWork(id string, work *Work) (*Work, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("UpdateWork", id, work)
	if err := m.checkError("UpdateWork"); err != nil {
		return nil, err
	}
	work.ID = id
	m.Works[id] = work
	return work, nil
}

// DeleteWork implements Store
func (m *MockStore) DeleteWork(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("DeleteWork", id)
	if err := m.checkError("DeleteWork"); err != nil {
		return err
	}
	delete(m.Works, id)
	return nil
}

// GetBooksByWorkID implements Store
func (m *MockStore) GetBooksByWorkID(workID string) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBooksByWorkID", workID)
	if err := m.checkError("GetBooksByWorkID"); err != nil {
		return nil, err
	}
	var result []Book
	for _, b := range m.Books {
		if b.WorkID != nil && *b.WorkID == workID {
			result = append(result, *b)
		}
	}
	return result, nil
}

// GetAllBooks implements Store
func (m *MockStore) GetAllBooks(limit, offset int) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllBooks", limit, offset)
	if err := m.checkError("GetAllBooks"); err != nil {
		return nil, err
	}
	result := make([]Book, 0, len(m.Books))
	for _, b := range m.Books {
		result = append(result, *b)
	}
	return result, nil
}

// GetBookByID implements Store
func (m *MockStore) GetBookByID(id string) (*Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBookByID", id)
	if err := m.checkError("GetBookByID"); err != nil {
		return nil, err
	}
	if b, ok := m.Books[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("book not found: %s", id)
}

// GetBookByFilePath implements Store
func (m *MockStore) GetBookByFilePath(path string) (*Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBookByFilePath", path)
	if err := m.checkError("GetBookByFilePath"); err != nil {
		return nil, err
	}
	for _, b := range m.Books {
		if b.FilePath == path {
			return b, nil
		}
	}
	return nil, fmt.Errorf("book not found: %s", path)
}

// GetBookByFileHash implements Store
func (m *MockStore) GetBookByFileHash(hash string) (*Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBookByFileHash", hash)
	if err := m.checkError("GetBookByFileHash"); err != nil {
		return nil, err
	}
	for _, b := range m.Books {
		if b.FileHash != nil && *b.FileHash == hash {
			return b, nil
		}
	}
	return nil, fmt.Errorf("book not found by hash: %s", hash)
}

// GetBookByOriginalHash implements Store
func (m *MockStore) GetBookByOriginalHash(hash string) (*Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBookByOriginalHash", hash)
	if err := m.checkError("GetBookByOriginalHash"); err != nil {
		return nil, err
	}
	for _, b := range m.Books {
		if b.OriginalFileHash != nil && *b.OriginalFileHash == hash {
			return b, nil
		}
	}
	return nil, fmt.Errorf("book not found by original hash: %s", hash)
}

// GetBookByOrganizedHash implements Store
func (m *MockStore) GetBookByOrganizedHash(hash string) (*Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBookByOrganizedHash", hash)
	if err := m.checkError("GetBookByOrganizedHash"); err != nil {
		return nil, err
	}
	for _, b := range m.Books {
		if b.OrganizedFileHash != nil && *b.OrganizedFileHash == hash {
			return b, nil
		}
	}
	return nil, fmt.Errorf("book not found by organized hash: %s", hash)
}

// GetDuplicateBooks implements Store
func (m *MockStore) GetDuplicateBooks() ([][]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetDuplicateBooks")
	if err := m.checkError("GetDuplicateBooks"); err != nil {
		return nil, err
	}
	return [][]Book{}, nil
}

// GetBooksBySeriesID implements Store
func (m *MockStore) GetBooksBySeriesID(seriesID int) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBooksBySeriesID", seriesID)
	if err := m.checkError("GetBooksBySeriesID"); err != nil {
		return nil, err
	}
	var result []Book
	for _, b := range m.Books {
		if b.SeriesID != nil && *b.SeriesID == seriesID {
			result = append(result, *b)
		}
	}
	return result, nil
}

// GetBooksByAuthorID implements Store
func (m *MockStore) GetBooksByAuthorID(authorID int) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBooksByAuthorID", authorID)
	if err := m.checkError("GetBooksByAuthorID"); err != nil {
		return nil, err
	}
	var result []Book
	for _, b := range m.Books {
		if b.AuthorID != nil && *b.AuthorID == authorID {
			result = append(result, *b)
		}
	}
	return result, nil
}

// CreateBook implements Store
func (m *MockStore) CreateBook(book *Book) (*Book, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreateBook", book)
	if err := m.checkError("CreateBook"); err != nil {
		return nil, err
	}
	if book.ID == "" {
		book.ID = fmt.Sprintf("book-%d", m.NextBookID)
		m.NextBookID++
	}
	m.Books[book.ID] = book
	return book, nil
}

// UpdateBook implements Store
func (m *MockStore) UpdateBook(id string, book *Book) (*Book, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("UpdateBook", id, book)
	if err := m.checkError("UpdateBook"); err != nil {
		return nil, err
	}
	book.ID = id
	m.Books[id] = book
	return book, nil
}

// DeleteBook implements Store
func (m *MockStore) DeleteBook(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("DeleteBook", id)
	if err := m.checkError("DeleteBook"); err != nil {
		return err
	}
	delete(m.Books, id)
	return nil
}

// SearchBooks implements Store
func (m *MockStore) SearchBooks(query string, limit, offset int) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("SearchBooks", query, limit, offset)
	if err := m.checkError("SearchBooks"); err != nil {
		return nil, err
	}
	return []Book{}, nil
}

// CountBooks implements Store
func (m *MockStore) CountBooks() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("CountBooks")
	if err := m.checkError("CountBooks"); err != nil {
		return 0, err
	}
	return len(m.Books), nil
}

// ListSoftDeletedBooks implements Store
func (m *MockStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("ListSoftDeletedBooks", limit, offset, olderThan)
	if err := m.checkError("ListSoftDeletedBooks"); err != nil {
		return nil, err
	}
	return []Book{}, nil
}

// GetBooksByVersionGroup implements Store
func (m *MockStore) GetBooksByVersionGroup(groupID string) ([]Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBooksByVersionGroup", groupID)
	if err := m.checkError("GetBooksByVersionGroup"); err != nil {
		return nil, err
	}
	var result []Book
	for _, b := range m.Books {
		if b.VersionGroupID != nil && *b.VersionGroupID == groupID {
			result = append(result, *b)
		}
	}
	return result, nil
}

// GetAllImportPaths implements Store
func (m *MockStore) GetAllImportPaths() ([]ImportPath, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllImportPaths")
	if err := m.checkError("GetAllImportPaths"); err != nil {
		return nil, err
	}
	result := make([]ImportPath, 0, len(m.ImportPaths))
	for _, p := range m.ImportPaths {
		result = append(result, *p)
	}
	return result, nil
}

// GetImportPathByID implements Store
func (m *MockStore) GetImportPathByID(id int) (*ImportPath, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetImportPathByID", id)
	if err := m.checkError("GetImportPathByID"); err != nil {
		return nil, err
	}
	if p, ok := m.ImportPaths[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("import path not found: %d", id)
}

// GetImportPathByPath implements Store
func (m *MockStore) GetImportPathByPath(path string) (*ImportPath, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetImportPathByPath", path)
	if err := m.checkError("GetImportPathByPath"); err != nil {
		return nil, err
	}
	for _, p := range m.ImportPaths {
		if p.Path == path {
			return p, nil
		}
	}
	return nil, fmt.Errorf("import path not found: %s", path)
}

// CreateImportPath implements Store
func (m *MockStore) CreateImportPath(path, name string) (*ImportPath, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreateImportPath", path, name)
	if err := m.checkError("CreateImportPath"); err != nil {
		return nil, err
	}
	p := &ImportPath{ID: m.NextImportPathID, Path: path, Name: name, Enabled: true, CreatedAt: time.Now()}
	m.ImportPaths[m.NextImportPathID] = p
	m.NextImportPathID++
	return p, nil
}

// UpdateImportPath implements Store
func (m *MockStore) UpdateImportPath(id int, importPath *ImportPath) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("UpdateImportPath", id, importPath)
	if err := m.checkError("UpdateImportPath"); err != nil {
		return err
	}
	importPath.ID = id
	m.ImportPaths[id] = importPath
	return nil
}

// DeleteImportPath implements Store
func (m *MockStore) DeleteImportPath(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("DeleteImportPath", id)
	if err := m.checkError("DeleteImportPath"); err != nil {
		return err
	}
	delete(m.ImportPaths, id)
	return nil
}

// CreateOperation implements Store
func (m *MockStore) CreateOperation(id, opType string, folderPath *string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreateOperation", id, opType, folderPath)
	if err := m.checkError("CreateOperation"); err != nil {
		return nil, err
	}
	op := &Operation{
		ID:         id,
		Type:       opType,
		Status:     "pending",
		FolderPath: folderPath,
		CreatedAt:  time.Now(),
	}
	m.Operations[id] = op
	return op, nil
}

// GetOperationByID implements Store
func (m *MockStore) GetOperationByID(id string) (*Operation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetOperationByID", id)
	if err := m.checkError("GetOperationByID"); err != nil {
		return nil, err
	}
	if op, ok := m.Operations[id]; ok {
		return op, nil
	}
	return nil, fmt.Errorf("operation not found: %s", id)
}

// GetRecentOperations implements Store
func (m *MockStore) GetRecentOperations(limit int) ([]Operation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetRecentOperations", limit)
	if err := m.checkError("GetRecentOperations"); err != nil {
		return nil, err
	}
	result := make([]Operation, 0, len(m.Operations))
	for _, op := range m.Operations {
		result = append(result, *op)
	}
	return result, nil
}

// UpdateOperationStatus implements Store
func (m *MockStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("UpdateOperationStatus", id, status, progress, total, message)
	if err := m.checkError("UpdateOperationStatus"); err != nil {
		return err
	}
	if op, ok := m.Operations[id]; ok {
		op.Status = status
		op.Progress = progress
		op.Total = total
		op.Message = message
		return nil
	}
	// Create operation if it doesn't exist
	m.Operations[id] = &Operation{
		ID:        id,
		Status:    status,
		Progress:  progress,
		Total:     total,
		Message:   message,
		CreatedAt: time.Now(),
	}
	return nil
}

// UpdateOperationError implements Store
func (m *MockStore) UpdateOperationError(id, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("UpdateOperationError", id, errorMessage)
	if err := m.checkError("UpdateOperationError"); err != nil {
		return err
	}
	if op, ok := m.Operations[id]; ok {
		op.Status = "failed"
		op.ErrorMessage = &errorMessage
		return nil
	}
	return fmt.Errorf("operation not found: %s", id)
}

// AddOperationLog implements Store
func (m *MockStore) AddOperationLog(operationID, level, message string, details *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("AddOperationLog", operationID, level, message, details)
	if err := m.checkError("AddOperationLog"); err != nil {
		return err
	}
	log := OperationLog{
		ID:          m.NextOpLogID,
		OperationID: operationID,
		Level:       level,
		Message:     message,
		Details:     details,
		CreatedAt:   time.Now(),
	}
	m.NextOpLogID++
	m.OpLogs[operationID] = append(m.OpLogs[operationID], log)
	return nil
}

// GetOperationLogs implements Store
func (m *MockStore) GetOperationLogs(operationID string) ([]OperationLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetOperationLogs", operationID)
	if err := m.checkError("GetOperationLogs"); err != nil {
		return nil, err
	}
	return m.OpLogs[operationID], nil
}

// GetUserPreference implements Store
func (m *MockStore) GetUserPreference(key string) (*UserPreference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetUserPreference", key)
	if err := m.checkError("GetUserPreference"); err != nil {
		return nil, err
	}
	if p, ok := m.Preferences[key]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("preference not found: %s", key)
}

// SetUserPreference implements Store
func (m *MockStore) SetUserPreference(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("SetUserPreference", key, value)
	if err := m.checkError("SetUserPreference"); err != nil {
		return err
	}
	m.Preferences[key] = &UserPreference{ID: m.NextPrefID, Key: key, Value: &value, UpdatedAt: time.Now()}
	m.NextPrefID++
	return nil
}

// GetAllUserPreferences implements Store
func (m *MockStore) GetAllUserPreferences() ([]UserPreference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllUserPreferences")
	if err := m.checkError("GetAllUserPreferences"); err != nil {
		return nil, err
	}
	result := make([]UserPreference, 0, len(m.Preferences))
	for _, p := range m.Preferences {
		result = append(result, *p)
	}
	return result, nil
}

// GetSetting implements Store
func (m *MockStore) GetSetting(key string) (*Setting, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetSetting", key)
	if err := m.checkError("GetSetting"); err != nil {
		return nil, err
	}
	if s, ok := m.Settings[key]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("setting not found: %s", key)
}

// SetSetting implements Store
func (m *MockStore) SetSetting(key, value, typ string, isSecret bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("SetSetting", key, value, typ, isSecret)
	if err := m.checkError("SetSetting"); err != nil {
		return err
	}
	m.Settings[key] = &Setting{Key: key, Value: value, Type: typ, IsSecret: isSecret}
	return nil
}

// GetAllSettings implements Store
func (m *MockStore) GetAllSettings() ([]Setting, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllSettings")
	if err := m.checkError("GetAllSettings"); err != nil {
		return nil, err
	}
	result := make([]Setting, 0, len(m.Settings))
	for _, s := range m.Settings {
		result = append(result, *s)
	}
	return result, nil
}

// DeleteSetting implements Store
func (m *MockStore) DeleteSetting(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("DeleteSetting", key)
	if err := m.checkError("DeleteSetting"); err != nil {
		return err
	}
	delete(m.Settings, key)
	return nil
}

// CreatePlaylist implements Store
func (m *MockStore) CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("CreatePlaylist", name, seriesID, filePath)
	if err := m.checkError("CreatePlaylist"); err != nil {
		return nil, err
	}
	p := &Playlist{ID: m.NextPlaylistID, Name: name, SeriesID: seriesID, FilePath: filePath}
	m.Playlists[m.NextPlaylistID] = p
	m.NextPlaylistID++
	return p, nil
}

// GetPlaylistByID implements Store
func (m *MockStore) GetPlaylistByID(id int) (*Playlist, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetPlaylistByID", id)
	if err := m.checkError("GetPlaylistByID"); err != nil {
		return nil, err
	}
	if p, ok := m.Playlists[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("playlist not found: %d", id)
}

// GetPlaylistBySeriesID implements Store
func (m *MockStore) GetPlaylistBySeriesID(seriesID int) (*Playlist, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetPlaylistBySeriesID", seriesID)
	if err := m.checkError("GetPlaylistBySeriesID"); err != nil {
		return nil, err
	}
	for _, p := range m.Playlists {
		if p.SeriesID != nil && *p.SeriesID == seriesID {
			return p, nil
		}
	}
	return nil, fmt.Errorf("playlist not found for series: %d", seriesID)
}

// AddPlaylistItem implements Store
func (m *MockStore) AddPlaylistItem(playlistID, bookID, position int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("AddPlaylistItem", playlistID, bookID, position)
	return m.checkError("AddPlaylistItem")
}

// GetPlaylistItems implements Store
func (m *MockStore) GetPlaylistItems(playlistID int) ([]PlaylistItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetPlaylistItems", playlistID)
	if err := m.checkError("GetPlaylistItems"); err != nil {
		return nil, err
	}
	return []PlaylistItem{}, nil
}

// User & Auth stubs
func (m *MockStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error) {
	m.recordCall("CreateUser", username, email)
	return nil, m.checkError("CreateUser")
}
func (m *MockStore) GetUserByID(id string) (*User, error) {
	m.recordCall("GetUserByID", id)
	return nil, m.checkError("GetUserByID")
}
func (m *MockStore) GetUserByUsername(username string) (*User, error) {
	m.recordCall("GetUserByUsername", username)
	return nil, m.checkError("GetUserByUsername")
}
func (m *MockStore) GetUserByEmail(email string) (*User, error) {
	m.recordCall("GetUserByEmail", email)
	return nil, m.checkError("GetUserByEmail")
}
func (m *MockStore) UpdateUser(user *User) error {
	m.recordCall("UpdateUser", user)
	return m.checkError("UpdateUser")
}

// Session stubs
func (m *MockStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	m.recordCall("CreateSession", userID)
	return nil, m.checkError("CreateSession")
}
func (m *MockStore) GetSession(id string) (*Session, error) {
	m.recordCall("GetSession", id)
	return nil, m.checkError("GetSession")
}
func (m *MockStore) RevokeSession(id string) error {
	m.recordCall("RevokeSession", id)
	return m.checkError("RevokeSession")
}
func (m *MockStore) ListUserSessions(userID string) ([]Session, error) {
	m.recordCall("ListUserSessions", userID)
	return nil, m.checkError("ListUserSessions")
}

// Per-user preferences stubs
func (m *MockStore) SetUserPreferenceForUser(userID, key, value string) error {
	m.recordCall("SetUserPreferenceForUser", userID, key, value)
	return m.checkError("SetUserPreferenceForUser")
}
func (m *MockStore) GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error) {
	m.recordCall("GetUserPreferenceForUser", userID, key)
	return nil, m.checkError("GetUserPreferenceForUser")
}
func (m *MockStore) GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error) {
	m.recordCall("GetAllPreferencesForUser", userID)
	return nil, m.checkError("GetAllPreferencesForUser")
}

// Book segments stubs
func (m *MockStore) CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error) {
	m.recordCall("CreateBookSegment", bookNumericID)
	return nil, m.checkError("CreateBookSegment")
}
func (m *MockStore) ListBookSegments(bookNumericID int) ([]BookSegment, error) {
	m.recordCall("ListBookSegments", bookNumericID)
	return nil, m.checkError("ListBookSegments")
}
func (m *MockStore) MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error {
	m.recordCall("MergeBookSegments", bookNumericID)
	return m.checkError("MergeBookSegments")
}

// Playback stubs
func (m *MockStore) AddPlaybackEvent(event *PlaybackEvent) error {
	m.recordCall("AddPlaybackEvent", event)
	return m.checkError("AddPlaybackEvent")
}
func (m *MockStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error) {
	m.recordCall("ListPlaybackEvents", userID, bookNumericID)
	return nil, m.checkError("ListPlaybackEvents")
}
func (m *MockStore) UpdatePlaybackProgress(progress *PlaybackProgress) error {
	m.recordCall("UpdatePlaybackProgress", progress)
	return m.checkError("UpdatePlaybackProgress")
}
func (m *MockStore) GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error) {
	m.recordCall("GetPlaybackProgress", userID, bookNumericID)
	return nil, m.checkError("GetPlaybackProgress")
}

// Stats stubs
func (m *MockStore) IncrementBookPlayStats(bookNumericID int, seconds int) error {
	m.recordCall("IncrementBookPlayStats", bookNumericID, seconds)
	return m.checkError("IncrementBookPlayStats")
}
func (m *MockStore) GetBookStats(bookNumericID int) (*BookStats, error) {
	m.recordCall("GetBookStats", bookNumericID)
	return nil, m.checkError("GetBookStats")
}
func (m *MockStore) IncrementUserListenStats(userID string, seconds int) error {
	m.recordCall("IncrementUserListenStats", userID, seconds)
	return m.checkError("IncrementUserListenStats")
}
func (m *MockStore) GetUserStats(userID string) (*UserStats, error) {
	m.recordCall("GetUserStats", userID)
	return nil, m.checkError("GetUserStats")
}

// Hash blocklist
func (m *MockStore) IsHashBlocked(hash string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("IsHashBlocked", hash)
	if err := m.checkError("IsHashBlocked"); err != nil {
		return false, err
	}
	_, ok := m.BlockedHash[hash]
	return ok, nil
}

func (m *MockStore) AddBlockedHash(hash, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("AddBlockedHash", hash, reason)
	if err := m.checkError("AddBlockedHash"); err != nil {
		return err
	}
	m.BlockedHash[hash] = &DoNotImport{Hash: hash, Reason: reason, CreatedAt: time.Now()}
	return nil
}

func (m *MockStore) RemoveBlockedHash(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("RemoveBlockedHash", hash)
	if err := m.checkError("RemoveBlockedHash"); err != nil {
		return err
	}
	delete(m.BlockedHash, hash)
	return nil
}

func (m *MockStore) GetAllBlockedHashes() ([]DoNotImport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetAllBlockedHashes")
	if err := m.checkError("GetAllBlockedHashes"); err != nil {
		return nil, err
	}
	result := make([]DoNotImport, 0, len(m.BlockedHash))
	for _, h := range m.BlockedHash {
		result = append(result, *h)
	}
	return result, nil
}

func (m *MockStore) GetBlockedHashByHash(hash string) (*DoNotImport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.recordCall("GetBlockedHashByHash", hash)
	if err := m.checkError("GetBlockedHashByHash"); err != nil {
		return nil, err
	}
	if h, ok := m.BlockedHash[hash]; ok {
		return h, nil
	}
	return nil, fmt.Errorf("blocked hash not found: %s", hash)
}
