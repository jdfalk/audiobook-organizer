// file: internal/server/server_extra_test.go
// version: 1.0.1
// guid: 61a2d3c4-80ab-4f6f-8c39-15a2ac5b7f0c

package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/stretchr/testify/require"
)

func TestServerHelpers(t *testing.T) {
	resetLibrarySizeCache()
	if got := *intPtr(7); got != 7 {
		t.Fatalf("expected int pointer 7, got %d", got)
	}
	if got := *boolPtr(true); got != true {
		t.Fatal("expected bool pointer true")
	}

	if stringFromSeries(nil) != nil {
		t.Fatal("expected nil series name")
	}
	if got := stringFromSeries(&database.Series{Name: "Series"}); got != "Series" {
		t.Fatalf("expected series name, got %v", got)
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "book.bin")
	require.NoError(t, os.WriteFile(filePath, []byte("audio"), 0o644))

	book := &database.Book{FilePath: filePath}
	applyOrganizedFileMetadata(book, filePath)
	if book.FileHash == nil || book.OrganizedFileHash == nil || book.OriginalFileHash == nil {
		t.Fatal("expected hashes to be set")
	}
	if book.FileSize == nil || *book.FileSize == 0 {
		t.Fatal("expected file size to be set")
	}

	resetLibrarySizeCache()
	rootDir := t.TempDir()
	importDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "root.txt"), []byte("1234"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(importDir, "import.txt"), []byte("12345"), 0o644))

	librarySize, importSize := calculateLibrarySizes(rootDir, []database.ImportPath{
		{Path: importDir, Enabled: true},
	})
	if librarySize == 0 || importSize == 0 {
		t.Fatalf("expected sizes to be non-zero, got %d/%d", librarySize, importSize)
	}
	cachedLibrary, cachedImport := calculateLibrarySizes(rootDir, []database.ImportPath{
		{Path: importDir, Enabled: true},
	})
	if cachedLibrary != librarySize || cachedImport != importSize {
		t.Fatalf("expected cached sizes to match, got %d/%d", cachedLibrary, cachedImport)
	}

	SetEmbeddedFS(embed.FS{})
}

func TestDuplicateAndSoftDeleteEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "dup1.m4b")
	file2 := filepath.Join(tempDir, "dup2.m4b")
	require.NoError(t, os.WriteFile(file1, []byte("audio"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("audio"), 0o644))

	hash := "dup-hash"
	_, err := database.GlobalStore.CreateBook(&database.Book{
		Title:             "Dup One",
		FilePath:          file1,
		FileHash:          &hash,
		OrganizedFileHash: &hash,
	})
	require.NoError(t, err)
	_, err = database.GlobalStore.CreateBook(&database.Book{
		Title:             "Dup Two",
		FilePath:          file2,
		FileHash:          &hash,
		OrganizedFileHash: &hash,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/duplicates", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	softFile := filepath.Join(tempDir, "soft.m4b")
	require.NoError(t, os.WriteFile(softFile, []byte("audio"), 0o644))
	marked := true
	deletedAt := time.Now().Add(-24 * time.Hour)
	_, err = database.GlobalStore.CreateBook(&database.Book{
		Title:               "Soft Delete",
		FilePath:            softFile,
		MarkedForDeletion:   &marked,
		MarkedForDeletionAt: &deletedAt,
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/soft-deleted?limit=10", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/audiobooks/purge-soft-deleted?delete_files=true", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	if _, err := os.Stat(softFile); !os.IsNotExist(err) {
		t.Fatalf("expected soft deleted file to be removed, got %v", err)
	}

	restoreFile := filepath.Join(tempDir, "restore.m4b")
	require.NoError(t, os.WriteFile(restoreFile, []byte("audio"), 0o644))
	restoreBook, err := database.GlobalStore.CreateBook(&database.Book{
		Title:               "Restore",
		FilePath:            restoreFile,
		MarkedForDeletion:   &marked,
		MarkedForDeletionAt: &deletedAt,
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+restoreBook.ID+"/restore", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/count", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestWorkEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{"title":"Test Work"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/works", createBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created database.Work
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	req = httptest.NewRequest(http.MethodGet, "/api/v1/works", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/works/"+created.ID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	updateBody := bytes.NewBufferString(`{"title":"Updated Work"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/works/"+created.ID, updateBody)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	bookPath := filepath.Join(t.TempDir(), "work.m4b")
	require.NoError(t, os.WriteFile(bookPath, []byte("audio"), 0o644))
	_, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Work Book",
		FilePath: bookPath,
		WorkID:   &created.ID,
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/works/"+created.ID+"/books", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/works/"+created.ID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestExclusionEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	dir := t.TempDir()
	payload := bytes.NewBufferString(`{"path":"` + dir + `","reason":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/filesystem/exclude", payload)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	if _, err := os.Stat(filepath.Join(dir, ".jabexclude")); err != nil {
		t.Fatalf("expected .jabexclude file: %v", err)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/filesystem/exclude", bytes.NewBufferString(`{"path":"`+dir+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestImportPathEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	origQueue := operations.GlobalQueue
	operations.GlobalQueue = nil
	defer func() {
		operations.GlobalQueue = origQueue
	}()

	dir := t.TempDir()
	payload := bytes.NewBufferString(`{"path":"` + dir + `","name":"Test Import","enabled":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-paths", payload)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	importPath := response["importPath"].(map[string]interface{})
	importID := int(importPath["id"].(float64))

	req = httptest.NewRequest(http.MethodGet, "/api/v1/import-paths", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/import-paths/"+strconv.Itoa(importID), nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestOperationEndpointsErrors(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	origQueue := operations.GlobalQueue
	operations.GlobalQueue = nil
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/scan", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/operations/active", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	operations.GlobalQueue = origQueue

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/operations/bad-id", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestImportFileErrors(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/file", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	dir := t.TempDir()
	payload := bytes.NewBufferString(`{"file_path":"` + dir + `"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/import/file", payload)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	txtFile := filepath.Join(t.TempDir(), "bad.txt")
	require.NoError(t, os.WriteFile(txtFile, []byte("data"), 0o644))
	payload = bytes.NewBufferString(`{"file_path":"` + txtFile + `"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/import/file", payload)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSystemLogsEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	folder := "/tmp"
	op, err := database.GlobalStore.CreateOperation("log-op", "scan", &folder)
	require.NoError(t, err)
	require.NoError(t, database.GlobalStore.AddOperationLog(op.ID, "info", "hello", nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/logs?level=info", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/system/logs?operation_id=log-op", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAIEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	origConfig := config.AppConfig
	defer func() {
		config.AppConfig = origConfig
	}()
	config.AppConfig.EnableAIParsing = false
	config.AppConfig.OpenAIAPIKey = ""

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/parse-filename", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/ai/parse-filename", bytes.NewBufferString(`{"filename":"book.mp3"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/ai/test-connection", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/missing/parse-with-ai", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	bookPath := filepath.Join(t.TempDir(), "ai.m4b")
	require.NoError(t, os.WriteFile(bookPath, []byte("audio"), 0o644))
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "AI Book",
		FilePath: bookPath,
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+book.ID+"/parse-with-ai", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVersionEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	book1Path := filepath.Join(t.TempDir(), "v1.m4b")
	book2Path := filepath.Join(t.TempDir(), "v2.m4b")
	require.NoError(t, os.WriteFile(book1Path, []byte("audio"), 0o644))
	require.NoError(t, os.WriteFile(book2Path, []byte("audio"), 0o644))

	book1, err := database.GlobalStore.CreateBook(&database.Book{Title: "Version One", FilePath: book1Path})
	require.NoError(t, err)
	book2, err := database.GlobalStore.CreateBook(&database.Book{Title: "Version Two", FilePath: book2Path})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+book1.ID+"/versions", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	linkPayload := bytes.NewBufferString(`{"other_id":"` + book2.ID + `"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/"+book1.ID+"/versions", linkPayload)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var linkResp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &linkResp))
	groupID := linkResp["version_group_id"]

	req = httptest.NewRequest(http.MethodGet, "/api/v1/version-groups/"+groupID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/"+book1.ID+"/set-primary", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	book3Path := filepath.Join(t.TempDir(), "v3.m4b")
	require.NoError(t, os.WriteFile(book3Path, []byte("audio"), 0o644))
	book3, err := database.GlobalStore.CreateBook(&database.Book{Title: "Version Three", FilePath: book3Path})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/"+book3.ID+"/set-primary", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestBlockedHashEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	require.NoError(t, database.RunMigrations(database.GlobalStore))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocked-hashes", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/blocked-hashes", bytes.NewBufferString(`{"hash":"short","reason":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	validHash := strings.Repeat("a", 64)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/blocked-hashes", bytes.NewBufferString(`{"hash":"`+validHash+`","reason":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/blocked-hashes/"+validHash, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleEventsUnavailable(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	origHub := realtime.GetGlobalHub()
	realtime.SetGlobalHub(nil)
	defer func() {
		realtime.SetGlobalHub(origHub)
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestBackupEndpointsErrors(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	origConfig := config.AppConfig
	origDir, err := os.Getwd()
	require.NoError(t, err)
	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	defer func() {
		_ = os.Chdir(origDir)
		config.AppConfig = origConfig
	}()

	config.AppConfig.DatabasePath = filepath.Join(tempDir, "missing.db")
	config.AppConfig.DatabaseType = "sqlite"

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/backup/restore", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/backup/missing.tar.gz", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
