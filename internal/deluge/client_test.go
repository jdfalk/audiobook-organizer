// file: internal/deluge/client_test.go
// version: 1.0.0
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
