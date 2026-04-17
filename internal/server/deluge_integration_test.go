// file: internal/server/deluge_integration_test.go
// version: 1.0.0

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
)

func TestNotifyDelugeMoveStorage_EmptyHash(t *testing.T) {
	// Should silently no-op with empty hash.
	NotifyDelugeMoveStorage("", "/some/path")
}

func TestNotifyDelugeMoveStorage_NoClient(t *testing.T) {
	// Save and clear the global client.
	orig := globalDelugeClient
	globalDelugeClient = nil
	origURL := config.AppConfig.DelugeWebURL
	config.AppConfig.DelugeWebURL = ""
	origHost := config.AppConfig.DownloadClient.Torrent.Deluge.Host
	config.AppConfig.DownloadClient.Torrent.Deluge.Host = ""
	defer func() {
		globalDelugeClient = orig
		config.AppConfig.DelugeWebURL = origURL
		config.AppConfig.DownloadClient.Torrent.Deluge.Host = origHost
	}()

	// Should silently no-op when Deluge is not configured.
	NotifyDelugeMoveStorage("abc123", "/new/path")
}

func TestNotifyDelugeMoveStorage_WithMockServer(t *testing.T) {
	var calledMoveStorage bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
			ID     int64         `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(map[string]interface{}{"id": req.ID, "result": true})
		case "core.move_storage":
			calledMoveStorage = true
			json.NewEncoder(w).Encode(map[string]interface{}{"id": req.ID, "result": nil})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	// Create a real deluge client pointing to our mock.
	client, err := deluge.New(srv.URL, "deluge")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	orig := globalDelugeClient
	globalDelugeClient = client
	defer func() { globalDelugeClient = orig }()

	NotifyDelugeMoveStorage("abc123", "/new/path/to/book.m4b")

	if !calledMoveStorage {
		t.Error("expected MoveStorage to be called")
	}
}

func TestHandleDelugeStatus_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pebblePath := filepath.Join(t.TempDir(), "pebble")
	store, err := database.NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	origStore := database.GetGlobalStore()
	database.SetGlobalStore(store)
	t.Cleanup(func() {
		database.SetGlobalStore(origStore)
		store.Close()
	})

	origURL := config.AppConfig.DelugeWebURL
	origHost := config.AppConfig.DownloadClient.Torrent.Deluge.Host
	config.AppConfig.DelugeWebURL = ""
	config.AppConfig.DownloadClient.Torrent.Deluge.Host = ""
	defer func() {
		config.AppConfig.DelugeWebURL = origURL
		config.AppConfig.DownloadClient.Torrent.Deluge.Host = origHost
	}()

	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deluge/status", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Configured bool   `json:"configured"`
		URL        string `json:"url"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Configured {
		t.Error("expected configured=false when not set")
	}
}

func TestHandleDelugeStatus_Configured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pebblePath := filepath.Join(t.TempDir(), "pebble")
	store, err := database.NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	origStore := database.GetGlobalStore()
	database.SetGlobalStore(store)
	t.Cleanup(func() {
		database.SetGlobalStore(origStore)
		store.Close()
	})

	origURL := config.AppConfig.DelugeWebURL
	config.AppConfig.DelugeWebURL = "http://localhost:8112"
	defer func() { config.AppConfig.DelugeWebURL = origURL }()

	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deluge/status", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Configured bool   `json:"configured"`
		URL        string `json:"url"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Configured {
		t.Error("expected configured=true")
	}
	if resp.URL != "http://localhost:8112" {
		t.Errorf("expected url=http://localhost:8112, got %s", resp.URL)
	}
}

func TestNotifyDelugeAfterVersionSwap(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pebblePath := filepath.Join(t.TempDir(), "pebble")
	store, err := database.NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	_, _ = store.CreateBook(&database.Book{
		ID: "b1", Title: "Test Book", FilePath: "/lib/books/b1/book.m4b",
	})

	// With no Deluge client configured, should not panic.
	orig := globalDelugeClient
	globalDelugeClient = nil
	origURL := config.AppConfig.DelugeWebURL
	config.AppConfig.DelugeWebURL = ""
	origHost := config.AppConfig.DownloadClient.Torrent.Deluge.Host
	config.AppConfig.DownloadClient.Torrent.Deluge.Host = ""
	defer func() {
		globalDelugeClient = orig
		config.AppConfig.DelugeWebURL = origURL
		config.AppConfig.DownloadClient.Torrent.Deluge.Host = origHost
	}()

	fromVer := &database.BookVersion{ID: "v1", BookID: "b1", TorrentHash: "abc123"}
	toVer := &database.BookVersion{ID: "v2", BookID: "b1", TorrentHash: "def456"}

	// Should not panic even without Deluge configured.
	NotifyDelugeAfterVersionSwap(store, fromVer, toVer, "/lib/books/b1/book.m4b")
}
