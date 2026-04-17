// file: internal/server/user_handlers_test.go
// version: 1.0.0

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func setupUserHandlerServer(t *testing.T) (*Server, database.Store) {
	t.Helper()
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

	srv := NewServer(nil)
	return srv, store
}

func TestHandleListUsers_Empty(t *testing.T) {
	srv, _ := setupUserHandlerServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Users []map[string]interface{} `json:"users"`
		Count int                      `json:"count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 0 {
		t.Errorf("expected 0 users, got %d", resp.Count)
	}
}

func TestHandleListUsers_WithUsers(t *testing.T) {
	srv, store := setupUserHandlerServer(t)

	_, _ = store.CreateUser("alice", "alice@example.com", "bcrypt", "hash123", []string{"admin"}, "active")
	_, _ = store.CreateUser("bob", "bob@example.com", "bcrypt", "hash456", []string{"viewer"}, "active")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Users []map[string]interface{} `json:"users"`
		Count int                      `json:"count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Errorf("expected 2 users, got %d", resp.Count)
	}

	// Verify password hash is NOT exposed.
	for _, u := range resp.Users {
		if _, ok := u["password_hash"]; ok {
			t.Error("password_hash should not be exposed in list response")
		}
	}
}

func TestHandleCreateInvite(t *testing.T) {
	srv, store := setupUserHandlerServer(t)

	// Create a role first since the invite references a role_id.
	_, _ = store.CreateRole(&database.Role{
		ID: "editor", Name: "editor", Permissions: []string{"library.view"},
	})

	body, _ := json.Marshal(map[string]interface{}{
		"username": "newuser",
		"role_id":  "editor",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token  string           `json:"token"`
		Invite *database.Invite `json:"invite"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.Invite == nil {
		t.Fatal("expected invite in response")
	}
	if resp.Invite.Username != "newuser" {
		t.Errorf("expected username=newuser, got %s", resp.Invite.Username)
	}
}

func TestHandleCreateInvite_MissingFields(t *testing.T) {
	srv, _ := setupUserHandlerServer(t)

	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		// missing role_id
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing role_id, got %d", w.Code)
	}
}

func TestHandleListInvites(t *testing.T) {
	srv, _ := setupUserHandlerServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/invites", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Invites []database.Invite `json:"invites"`
		Count   int               `json:"count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 0 {
		t.Errorf("expected 0 invites, got %d", resp.Count)
	}
}

func TestGenerateToken(t *testing.T) {
	t1 := generateToken()
	t2 := generateToken()

	if t1 == "" || len(t1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("unexpected token length: %d", len(t1))
	}
	if t1 == t2 {
		t.Error("tokens should be unique")
	}
}
