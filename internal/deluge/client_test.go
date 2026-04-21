// file: internal/deluge/client_test.go
// version: 1.1.0
// guid: 0b8c9d7e-1f2a-4a70-b8c5-3d7e0f1b9a99

package deluge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "auth.login" {
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	if err := c.Login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !c.authed {
		t.Error("should be authed after login")
	}
	// Idempotent.
	if err := c.Login(); err != nil {
		t.Fatalf("second login: %v", err)
	}
}

func TestLogin_BadPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rpcResponse{Result: json.RawMessage(`false`)})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "wrong")
	if err := c.Login(); err == nil {
		t.Error("expected error on bad password")
	}
}

func TestListTorrents(t *testing.T) {
	reqCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		reqCount++
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
		case "core.get_torrents_status":
			result := map[string]TorrentStatus{
				"abc123": {Hash: "abc123", Name: "Test Book", SavePath: "/downloads/books", State: "Seeding", Progress: 100},
			}
			data, _ := json.Marshal(result)
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: data})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	torrents, err := c.ListTorrents()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(torrents) != 1 {
		t.Errorf("got %d torrents, want 1", len(torrents))
	}
	if torrents["abc123"].Name != "Test Book" {
		t.Errorf("name = %q", torrents["abc123"].Name)
	}
}

func TestListTorrentsByLabel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
		case "core.get_torrents_status":
			result := map[string]TorrentStatus{
				"a": {Hash: "a", Name: "Dune", SavePath: "/dl/dune", Label: "audiobooks"},
				"b": {Hash: "b", Name: "Linux", SavePath: "/dl/linux", Label: "linux"},
				"c": {Hash: "c", Name: "Foundation", SavePath: "/dl/foundation", Label: "Audiobooks"},
			}
			data, _ := json.Marshal(result)
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: data})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	got, err := c.ListTorrentsByLabel("audiobooks")
	if err != nil {
		t.Fatalf("list by label: %v", err)
	}
	// case-insensitive: "audiobooks" and "Audiobooks" both match
	if len(got) != 2 {
		t.Errorf("want 2 audiobook torrents, got %d", len(got))
	}
	for _, t2 := range got {
		if t2.Hash == "b" {
			t.Errorf("linux torrent should not be in results")
		}
	}
}

func TestListTorrentsByLabel_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
		case "core.get_torrents_status":
			data, _ := json.Marshal(map[string]TorrentStatus{
				"a": {Hash: "a", Name: "Dune", Label: "audiobooks"},
			})
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: data})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	// empty label = return all
	got, err := c.ListTorrentsByLabel("")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1, got %d", len(got))
	}
}

func TestListLabels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
		case "label.get_labels":
			data, _ := json.Marshal([]string{"audiobooks", "linux", "movies"})
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: data})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	labels, err := c.ListLabels()
	if err != nil {
		t.Fatalf("list labels: %v", err)
	}
	if len(labels) != 3 {
		t.Errorf("want 3 labels, got %d", len(labels))
	}
}

func TestListLabels_PluginNotInstalled(t *testing.T) {
	// Simulate Deluge returning an RPC error when Label plugin is absent.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
		case "label.get_labels":
			json.NewEncoder(w).Encode(rpcResponse{
				ID:    req.ID,
				Error: &rpcError{Code: -1, Message: "unknown method"},
			})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	labels, err := c.ListLabels()
	if err != nil {
		t.Fatalf("should not error when label plugin absent: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("want empty slice, got %v", labels)
	}
}

func TestMoveStorage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "auth.login":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`true`)})
		case "core.move_storage":
			json.NewEncoder(w).Encode(rpcResponse{ID: req.ID, Result: json.RawMessage(`null`)})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "deluge")
	err := c.MoveStorage([]string{"abc123"}, "/new/path")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
}
