// file: internal/server/entity_tag_handlers_test.go
// version: 1.0.0

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func setupEntityTagServer(t *testing.T) (*Server, database.Store) {
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

func TestHandleGetAuthorTags_Empty(t *testing.T) {
	srv, store := setupEntityTagServer(t)

	author, _ := store.CreateAuthor("Test Author")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authors/"+strconv.Itoa(author.ID)+"/tags", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tags []database.BookTag `json:"tags"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(resp.Tags))
	}
}

func TestHandleAddAuthorTag(t *testing.T) {
	srv, store := setupEntityTagServer(t)

	author, _ := store.CreateAuthor("Tagged Author")
	url := "/api/v1/authors/" + strconv.Itoa(author.ID) + "/tags"

	body, _ := json.Marshal(map[string]string{"tag": "sci-fi"})
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Added string `json:"added"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Added != "sci-fi" {
		t.Errorf("expected added=sci-fi, got %s", resp.Added)
	}

	// Verify via GET.
	req = httptest.NewRequest(http.MethodGet, url, nil)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET tags: %d %s", w.Code, w.Body.String())
	}
}

func TestHandleAddAuthorTag_WithSource(t *testing.T) {
	srv, store := setupEntityTagServer(t)

	author, _ := store.CreateAuthor("Sourced Author")
	url := "/api/v1/authors/" + strconv.Itoa(author.ID) + "/tags"

	body, _ := json.Marshal(map[string]string{"tag": "fantasy", "source": "goodreads"})
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAddAuthorTag_InvalidID(t *testing.T) {
	srv, _ := setupEntityTagServer(t)

	body, _ := json.Marshal(map[string]string{"tag": "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/authors/notanumber/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetSeriesTags(t *testing.T) {
	srv, store := setupEntityTagServer(t)

	series, _ := store.CreateSeries("Test Series", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/series/"+strconv.Itoa(series.ID)+"/tags", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAddSeriesTag(t *testing.T) {
	srv, store := setupEntityTagServer(t)

	series, _ := store.CreateSeries("Tagged Series", nil)
	url := "/api/v1/series/" + strconv.Itoa(series.ID) + "/tags"

	body, _ := json.Marshal(map[string]string{"tag": "epic"})
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
