// file: internal/server/activity_handlers_test.go
// version: 1.1.0
// guid: d4e5f6a7-b8c9-0123-defa-234567890123

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupActivityTestRouter creates a temporary ActivityStore, wraps it in an
// ActivityService, mounts the listActivity handler on a minimal gin router,
// and returns the router plus a cleanup function.
func setupActivityTestRouter(t *testing.T) (*gin.Engine, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "activity_handler_test.db")

	store, err := database.NewActivityStore(dbPath)
	require.NoError(t, err)

	svc := NewActivityService(store)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	srv := &Server{activityService: svc}
	router.GET("/api/v1/activity", srv.listActivity)

	cleanup := func() {
		store.Close()
	}
	return router, cleanup
}

// TestListActivity_Empty verifies that an empty store returns HTTP 200 with
// an entries array (not null) and a total of 0.
func TestListActivity_Empty(t *testing.T) {
	router, cleanup := setupActivityTestRouter(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Total)
	// entries must be an array, not null.
	assert.NotNil(t, resp.Entries)
	assert.Empty(t, resp.Entries)
}

// TestListActivity_WithFilters inserts two entries (tiers: change, debug) and
// verifies that filtering by tier=change returns only the one matching entry.
func TestListActivity_WithFilters(t *testing.T) {
	// Use a fresh store so we can seed specific data.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "filter_test.db")
	store, err := database.NewActivityStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	svc := NewActivityService(store)
	gin.SetMode(gin.TestMode)
	filterRouter := gin.New()
	srv := &Server{activityService: svc}
	filterRouter.GET("/api/v1/activity", srv.listActivity)

	now := time.Now().UTC()

	err = svc.Record(database.ActivityEntry{
		Tier:      "change",
		Type:      "metadata_apply",
		Level:     "info",
		Source:    "test",
		Summary:   "metadata applied",
		Timestamp: now,
	})
	require.NoError(t, err)

	err = svc.Record(database.ActivityEntry{
		Tier:      "debug",
		Type:      "isbn_lookup",
		Level:     "debug",
		Source:    "test",
		Summary:   "ISBN lookup",
		Timestamp: now,
	})
	require.NoError(t, err)

	// Filter by tier=change.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/activity?tier=change", nil)
	filterRouter.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "change", resp.Entries[0].Tier)
	assert.Equal(t, "metadata_apply", resp.Entries[0].Type)
}

// TestListActivity_SearchParam verifies that the search query param filters
// entries by substring match on summary.
func TestListActivity_SearchParam(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "search_test.db")
	store, err := database.NewActivityStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	svc := NewActivityService(store)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{activityService: svc}
	r.GET("/api/v1/activity", srv.listActivity)

	now := time.Now().UTC()

	require.NoError(t, svc.Record(database.ActivityEntry{
		Tier:      "realtime",
		Type:      "scanner",
		Level:     "info",
		Source:    "scanner",
		Summary:   "Found: Project Hail Mary",
		Timestamp: now,
	}))
	require.NoError(t, svc.Record(database.ActivityEntry{
		Tier:      "realtime",
		Type:      "scanner",
		Level:     "info",
		Source:    "scanner",
		Summary:   "Found: The Martian",
		Timestamp: now,
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/activity?search=Hail+Mary", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Entries, 1)
	assert.Contains(t, resp.Entries[0].Summary, "Hail Mary")
}

// TestListActivitySources verifies that the sources endpoint returns distinct
// source names with counts, ordered by count descending.
func TestListActivitySources(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources_test.db")
	store, err := database.NewActivityStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	svc := NewActivityService(store)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{activityService: svc}
	r.GET("/api/v1/activity/sources", srv.listActivitySources)

	now := time.Now().UTC()

	// Record 2 gin entries and 1 scanner entry.
	for i := 0; i < 2; i++ {
		require.NoError(t, svc.Record(database.ActivityEntry{
			Tier:      "realtime",
			Type:      "request",
			Level:     "info",
			Source:    "gin",
			Summary:   "HTTP request",
			Timestamp: now,
		}))
	}
	require.NoError(t, svc.Record(database.ActivityEntry{
		Tier:      "background",
		Type:      "scan",
		Level:     "info",
		Source:    "scanner",
		Summary:   "scan complete",
		Timestamp: now,
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/activity/sources", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Sources []database.SourceCount `json:"sources"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Sources, 2)
	// Ordered by count DESC: gin (2) first, scanner (1) second.
	assert.Equal(t, "gin", resp.Sources[0].Source)
	assert.Equal(t, 2, resp.Sources[0].Count)
	assert.Equal(t, "scanner", resp.Sources[1].Source)
	assert.Equal(t, 1, resp.Sources[1].Count)
}
