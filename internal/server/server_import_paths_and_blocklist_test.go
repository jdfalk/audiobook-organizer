// file: internal/server/server_import_paths_and_blocklist_test.go
// version: 1.1.1
// guid: 2f4a6b8c-0d1e-2f3a-4b5c-6d7e8f9a0b1c
// last-edited: 2026-02-03

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	qmock "github.com/jdfalk/audiobook-organizer/internal/operations/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	scannermocks "github.com/jdfalk/audiobook-organizer/internal/scanner/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type noopProgress struct{}

func (noopProgress) UpdateProgress(current, total int, message string) error { return nil }
func (noopProgress) Log(level, message string, details *string) error        { return nil }
func (noopProgress) IsCanceled() bool                                        { return false }

func TestListAuthorsAndSeries_ReturnsEmptyArrayWhenNil(t *testing.T) {
	store := dbmocks.NewMockStore(t)
	store.EXPECT().GetAllAuthors().Return(([]database.Author)(nil), nil)
	store.EXPECT().GetAllSeries().Return(([]database.Series)(nil), nil)

	server, cleanup := setupTestServerWithStore(t, store)
	defer cleanup()

	// Authors
	req := httptest.NewRequest(http.MethodGet, "/api/v1/authors", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var authorsResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &authorsResp))
	items, ok := authorsResp["items"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 0)

	// Series
	req = httptest.NewRequest(http.MethodGet, "/api/v1/series", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var seriesResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &seriesResp))
	items, ok = seriesResp["items"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 0)
}

func TestImportPaths_ListNilAndRemoveInvalidID(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	store := dbmocks.NewMockStore(t)
	store.EXPECT().GetAllImportPaths().Return(([]database.ImportPath)(nil), nil)
	store.EXPECT().DeleteImportPath(mock.Anything).Return(nil).Maybe()

	origStore := database.GlobalStore
	database.GlobalStore = store
	t.Cleanup(func() { database.GlobalStore = origStore })

	// listImportPaths should return [] not null.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/import-paths", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var listResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	paths, ok := listResp["importPaths"].([]interface{})
	require.True(t, ok)
	assert.Len(t, paths, 0)

	// removeImportPath invalid id
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/import-paths/not-an-int", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddImportPath_EnqueuesAndExecutesOperationFunc(t *testing.T) {
	// Ensure deterministic worker selection.
	origCfg := config.AppConfig
	t.Cleanup(func() { config.AppConfig = origCfg })
	config.AppConfig.ConcurrentScans = 1
	config.AppConfig.AutoOrganize = false
	config.AppConfig.RootDir = ""

	store := dbmocks.NewMockStore(t)
	origStore := database.GlobalStore
	server, cleanup := setupTestServerWithStore(t, store)
	defer cleanup()
	queue := qmock.NewMockQueue(t)
	scannerMock := scannermocks.NewMockScanner(t)

	origQueue := operations.GlobalQueue
	origScanner := scanner.GlobalScanner
	operations.GlobalQueue = queue
	scanner.GlobalScanner = scannerMock
	t.Cleanup(func() {
		database.GlobalStore = origStore
		operations.GlobalQueue = origQueue
		scanner.GlobalScanner = origScanner
	})

	importDir := t.TempDir()
	bookPath := filepath.Join(importDir, "The Hobbit - J.R.R. Tolkien.m4b")

	created := &database.ImportPath{ID: 123, Path: importDir, Name: "Test Import", Enabled: true}
	store.EXPECT().CreateImportPath(importDir, "Test Import").Return(created, nil)
	store.EXPECT().CreateOperation(mock.Anything, "scan", mock.Anything).Return(&database.Operation{ID: "op-1", Type: "scan"}, nil)
	store.EXPECT().UpdateImportPath(created.ID, mock.Anything).Return(nil)

	scannerMock.EXPECT().ScanDirectoryParallel(importDir, mock.AnythingOfType("int")).Return([]scanner.Book{{FilePath: bookPath, Format: ".m4b"}}, nil)
	scannerMock.EXPECT().ProcessBooksParallel(mock.Anything, mock.Anything, mock.AnythingOfType("int"), mock.Anything).Return(nil)

	queue.EXPECT().Enqueue("op-1", "scan", operations.PriorityNormal, mock.Anything).RunAndReturn(
		func(id, opType string, priority int, fn operations.OperationFunc) error {
			return fn(context.Background(), noopProgress{})
		},
	)

	body := bytes.NewBufferString(`{"path":"` + importDir + `","name":"Test Import"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-paths", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	// Confirm the operation func ran and attempted to set LastScan/BookCount.
	assert.Equal(t, 1, created.BookCount)
	if assert.NotNil(t, created.LastScan) {
		assert.WithinDuration(t, time.Now(), *created.LastScan, 5*time.Second)
	}
}

func TestBlockedHashes_CRUD(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	store := dbmocks.NewMockStore(t)
	origStore := database.GlobalStore
	database.GlobalStore = store
	t.Cleanup(func() { database.GlobalStore = origStore })

	// addBlockedHash invalid hash length
	bad := bytes.NewBufferString(`{"hash":"abc","reason":"nope"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocked-hashes", bad)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// listBlockedHashes and removeBlockedHash
	hashes := []database.DoNotImport{{Hash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Reason: "test"}}
	store.EXPECT().GetAllBlockedHashes().Return(hashes, nil)
	store.EXPECT().RemoveBlockedHash(hashes[0].Hash).Return(nil)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/blocked-hashes", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/blocked-hashes/"+hashes[0].Hash, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
