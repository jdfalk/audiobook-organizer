// file: internal/itunes/service/path_reconcile_test.go
// version: 1.1.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

package itunesservice

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
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
	r := newPathReconciler(m, nil)
	require.NotNil(t, r)
	assert.Equal(t, m, r.store)
	assert.Nil(t, r.enqueuer)
}

// ---------------------------------------------------------------------------
// Reconcile — nil store returns error
// ---------------------------------------------------------------------------

func TestPathReconcilerReconcile_NilStore(t *testing.T) {
	r := newPathReconciler(nil, nil)
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

	r := newPathReconciler(m, nil)
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

	r := newPathReconciler(m, nil)
	err := r.Reconcile(context.Background(), "op-3", noopProgress{})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Reconcile — GetAllBooks error propagates
// ---------------------------------------------------------------------------

func TestPathReconcilerReconcile_LoadBooksError(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetAllBooks(100000, 0).Return(nil, assert.AnError).Once()

	r := newPathReconciler(m, nil)
	err := r.Reconcile(context.Background(), "op-4", noopProgress{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load books")
}
