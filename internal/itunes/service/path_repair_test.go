// file: internal/itunes/service/path_repair_test.go
// version: 1.0.0
// guid: 6b7e3d51-c0a3-4ab2-8d6c-7e9c1d4a8f01

package itunesservice

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	queuemocks "github.com/jdfalk/audiobook-organizer/internal/operations/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// newPathRepairer constructor
// ---------------------------------------------------------------------------

func TestNewPathRepairer(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	cfg := PathRepairConfig{XMLPath: "/tmp/iTunes Library.xml", AudiobookRoot: "/tmp/books"}
	r := newPathRepairer(m, nil, nil, cfg)
	require.NotNil(t, r)
	assert.Equal(t, m, r.store)
	assert.Nil(t, r.enqueuer)
	assert.Nil(t, r.queue)
	assert.Equal(t, cfg.XMLPath, r.cfg.XMLPath)
	assert.Equal(t, cfg.AudiobookRoot, r.cfg.AudiobookRoot)
}

// ---------------------------------------------------------------------------
// Start — nil store returns 500
// ---------------------------------------------------------------------------

func TestPathRepairerStart_NilStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newPathRepairer(nil, nil, nil, PathRepairConfig{})

	router := gin.New()
	router.POST("/repair", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repair", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "database not initialized")
}

// ---------------------------------------------------------------------------
// Start — nil queue returns 500
// ---------------------------------------------------------------------------

func TestPathRepairerStart_NilQueue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := dbmocks.NewMockStore(t)
	r := newPathRepairer(m, nil, nil, PathRepairConfig{}) // queue is nil

	router := gin.New()
	router.POST("/repair", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repair", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "operation queue not initialized")
}

// ---------------------------------------------------------------------------
// Start — CreateOperation error returns 500
// ---------------------------------------------------------------------------

func TestPathRepairerStart_CreateOperationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := dbmocks.NewMockStore(t)
	q := queuemocks.NewMockQueue(t)
	m.EXPECT().CreateOperation(mock.Anything, "itunes_path_repair", mock.Anything).
		Return(nil, assert.AnError).Once()

	r := newPathRepairer(m, nil, q, PathRepairConfig{})
	router := gin.New()
	router.POST("/repair", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repair", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------------------------------------------------------------------------
// Start — happy path returns 202
// ---------------------------------------------------------------------------

func TestPathRepairerStart_HappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := dbmocks.NewMockStore(t)
	q := queuemocks.NewMockQueue(t)

	op := &database.Operation{ID: "test-op-id", Type: "itunes_path_repair", Status: "queued"}
	m.EXPECT().CreateOperation(mock.Anything, "itunes_path_repair", mock.Anything).
		Return(op, nil).Once()
	q.EXPECT().Enqueue(op.ID, "itunes_path_repair", mock.Anything, mock.Anything).
		Return(nil).Once()

	r := newPathRepairer(m, nil, q, PathRepairConfig{})
	router := gin.New()
	router.POST("/repair", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repair", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Contains(t, w.Body.String(), "test-op-id")
}

// ---------------------------------------------------------------------------
// parseDryRun — query param parsing helper
// ---------------------------------------------------------------------------

func TestParseDryRun(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"", true},                  // default
		{"apply=true", false},       // explicit apply
		{"apply=1", false},          // truthy
		{"apply=false", true},       // explicit dry
		{"apply=0", true},           // falsy
		{"apply=anything-else", true}, // unknown values stay safe
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/repair?"+tc.query, nil)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req
			assert.Equal(t, tc.want, parseDryRun(c))
		})
	}
}
