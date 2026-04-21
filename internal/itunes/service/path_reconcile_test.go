// file: internal/itunes/service/path_reconcile_test.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

package itunesservice

import (
	"context"
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

// noopProgress is a zero-dependency ProgressReporter for tests.
type noopProgress struct{}

func (noopProgress) UpdateProgress(_, _ int, _ string) error { return nil }
func (noopProgress) Log(_, _ string, _ *string) error        { return nil }
func (noopProgress) IsCanceled() bool                        { return false }

// ---------------------------------------------------------------------------
// newPathReconciler constructor
// ---------------------------------------------------------------------------

func TestNewPathReconciler(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	r := newPathReconciler(m, nil, nil)
	require.NotNil(t, r)
	assert.Equal(t, m, r.store)
	assert.Nil(t, r.enqueuer)
	assert.Nil(t, r.queue)
}

// ---------------------------------------------------------------------------
// Start — nil store returns 500
// ---------------------------------------------------------------------------

func TestPathReconcilerStart_NilStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newPathReconciler(nil, nil, nil)

	router := gin.New()
	router.POST("/reconcile", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reconcile", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "database not initialized")
}

// ---------------------------------------------------------------------------
// Start — nil queue returns 500
// ---------------------------------------------------------------------------

func TestPathReconcilerStart_NilQueue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := dbmocks.NewMockStore(t)
	r := newPathReconciler(m, nil, nil) // queue is nil

	router := gin.New()
	router.POST("/reconcile", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reconcile", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "operation queue not initialized")
}

// ---------------------------------------------------------------------------
// Start — CreateOperation error returns 500
// ---------------------------------------------------------------------------

func TestPathReconcilerStart_CreateOperationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := dbmocks.NewMockStore(t)
	q := queuemocks.NewMockQueue(t)
	m.EXPECT().CreateOperation(mock.Anything, "itunes_path_reconcile", mock.Anything).
		Return(nil, assert.AnError).Once()

	r := newPathReconciler(m, nil, q)
	router := gin.New()
	router.POST("/reconcile", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reconcile", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------------------------------------------------------------------------
// Start — happy path returns 202
// ---------------------------------------------------------------------------

func TestPathReconcilerStart_HappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := dbmocks.NewMockStore(t)
	q := queuemocks.NewMockQueue(t)

	op := &database.Operation{ID: "test-op-id", Type: "itunes_path_reconcile", Status: "queued"}
	m.EXPECT().CreateOperation(mock.Anything, "itunes_path_reconcile", mock.Anything).
		Return(op, nil).Once()
	q.EXPECT().Enqueue(op.ID, "itunes_path_reconcile", mock.Anything, mock.Anything).
		Return(nil).Once()

	r := newPathReconciler(m, nil, q)
	router := gin.New()
	router.POST("/reconcile", r.Start)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reconcile", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Contains(t, w.Body.String(), "test-op-id")
}

// ---------------------------------------------------------------------------
// Reconcile — nil store returns error
// ---------------------------------------------------------------------------

func TestPathReconcilerReconcile_NilStore(t *testing.T) {
	r := newPathReconciler(nil, nil, nil)
	err := r.Reconcile(context.Background(), "op-1", noopProgress{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

// ---------------------------------------------------------------------------
// Reconcile — empty book list (no iTunes books) → noop success
// ---------------------------------------------------------------------------

func TestPathReconcilerReconcile_EmptyLibrary(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetAllBooks(100000, 0).Return([]database.Book{}, nil).Once()
	m.EXPECT().DeleteOperationState("op-2").Return(nil).Once()

	r := newPathReconciler(m, nil, nil)
	err := r.Reconcile(context.Background(), "op-2", noopProgress{})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Reconcile — book without iTunes PID is skipped
// ---------------------------------------------------------------------------

func TestPathReconcilerReconcile_SkipsNonITunesBooks(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	books := []database.Book{
		{ID: "b1", Title: "No iTunes", FilePath: "/mnt/books/b1.m4b"},
	}
	m.EXPECT().GetAllBooks(100000, 0).Return(books, nil).Once()
	m.EXPECT().GetBookFiles("b1").Return([]database.BookFile{}, nil).Once()
	m.EXPECT().DeleteOperationState("op-3").Return(nil).Once()

	r := newPathReconciler(m, nil, nil)
	err := r.Reconcile(context.Background(), "op-3", noopProgress{})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Reconcile — GetAllBooks error propagates
// ---------------------------------------------------------------------------

func TestPathReconcilerReconcile_LoadBooksError(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetAllBooks(100000, 0).Return(nil, assert.AnError).Once()

	r := newPathReconciler(m, nil, nil)
	err := r.Reconcile(context.Background(), "op-4", noopProgress{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load books")
}
