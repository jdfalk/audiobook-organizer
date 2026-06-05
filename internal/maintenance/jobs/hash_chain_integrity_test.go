// file: internal/maintenance/jobs/hash_chain_integrity_test.go
// version: 1.0.0
// guid: f1a2b3c4-d5e6-7890-abcd-ef0123456789
// last-edited: 2026-06-07

package jobs_test

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashChainIntegrityJob_Registered(t *testing.T) {
	assertJobRegistered(t, "hash-chain-integrity")
}

func TestHashChainIntegrityJob_Metadata(t *testing.T) {
	j, err := maintenance.Get("hash-chain-integrity")
	require.NoError(t, err)
	assert.Equal(t, "hash-chain-integrity", j.ID())
	assert.NotEmpty(t, j.Name())
	assert.NotEmpty(t, j.Description())
	assert.Equal(t, "files", j.Category())
	assert.False(t, j.CanResume())
	require.NotNil(t, j.DefaultParams())
}

func TestHashChainIntegrityJob_FlagsIntegrityIssue(t *testing.T) {
	store := newHashChainTestStore()
	store.GetAllBookFilesFunc = func() ([]database.BookFile, error) {
		return []database.BookFile{{
			ID:               "file-1",
			BookID:           "book-1",
			FilePath:         "/tmp/audio.m4b",
			FileHash:         "sha256:current",
			OriginalFileHash: "sha256:original",
		}}, nil
	}

	j, err := maintenance.Get("hash-chain-integrity")
	require.NoError(t, err)
	require.NoError(t, j.Run(context.Background(), store, &noopReporter{}, false))
	require.Len(t, store.recorded, 1)
	rec := store.recorded[0]
	assert.Equal(t, "hash-chain-integrity", rec.errClass)
	assert.Equal(t, "/tmp/audio.m4b", rec.filePath)
	assert.Equal(t, "book-1", rec.bookID)
	assert.Contains(t, rec.message, "file_hash=")
}

func TestHashChainIntegrityJob_DryRunSkipsPersistence(t *testing.T) {
	store := newHashChainTestStore()
	store.GetAllBookFilesFunc = func() ([]database.BookFile, error) {
		return []database.BookFile{{
			ID:               "file-2",
			BookID:           "book-2",
			FilePath:         "/tmp/audio2.m4b",
			FileHash:         "current",
			OriginalFileHash: "original",
		}}, nil
	}

	j, err := maintenance.Get("hash-chain-integrity")
	require.NoError(t, err)
	require.NoError(t, j.Run(context.Background(), store, &noopReporter{}, true))
	assert.Len(t, store.recorded, 0)
}

func TestHashChainIntegrityJob_SkipsPostMetadataHash(t *testing.T) {
	store := newHashChainTestStore()
	store.GetAllBookFilesFunc = func() ([]database.BookFile, error) {
		return []database.BookFile{{
			ID:               "file-3",
			BookID:           "book-3",
			FilePath:         "/tmp/audio3.m4b",
			FileHash:         "current",
			OriginalFileHash: "original",
			PostMetadataHash: "sha256:post",
		}}, nil
	}

	j, err := maintenance.Get("hash-chain-integrity")
	require.NoError(t, err)
	require.NoError(t, j.Run(context.Background(), store, &noopReporter{}, false))
	assert.Len(t, store.recorded, 0)
}

func TestHashChainIntegrityJob_SkipsWhenHashesMatch(t *testing.T) {
	store := newHashChainTestStore()
	store.GetAllBookFilesFunc = func() ([]database.BookFile, error) {
		return []database.BookFile{{
			ID:               "file-4",
			BookID:           "book-4",
			FilePath:         "/tmp/audio4.m4b",
			FileHash:         "sha256:match",
			OriginalFileHash: "sha256:match",
		}}, nil
	}

	j, err := maintenance.Get("hash-chain-integrity")
	require.NoError(t, err)
	require.NoError(t, j.Run(context.Background(), store, &noopReporter{}, false))
	assert.Len(t, store.recorded, 0)
}

type hashChainTestStore struct {
	*database.MockStore
	recorded []integrityAlert
}

type integrityAlert struct {
	filePath string
	bookID   string
	errClass string
	message string
}

func newHashChainTestStore() *hashChainTestStore {
	return &hashChainTestStore{MockStore: &database.MockStore{}}
}

func (s *hashChainTestStore) RecordFileError(filePath, bookID, errClass, message string) error {
	s.recorded = append(s.recorded, integrityAlert{
		filePath: filePath,
		bookID:   bookID,
		errClass: errClass,
		message: message,
	})
	return nil
}
