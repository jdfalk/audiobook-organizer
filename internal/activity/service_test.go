// file: internal/activity/service_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package activity

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_RecordAndQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "activity_test.db")

	store, err := database.NewActivityStore(dbPath)
	require.NoError(t, err)
	defer func() {
		store.Close()
		os.Remove(dbPath)
	}()

	svc := NewService(store)
	require.NotNil(t, svc)
	assert.Equal(t, store, svc.Store())

	// Record two entries with different tiers.
	err = svc.Record(database.ActivityEntry{
		Tier:    "realtime",
		Type:    "tag_write",
		Level:   "info",
		Source:  "test",
		Summary: "wrote tags for book A",
	})
	require.NoError(t, err)

	err = svc.Record(database.ActivityEntry{
		Tier:    "background",
		Type:    "isbn_lookup",
		Level:   "info",
		Source:  "test",
		Summary: "ISBN lookup for book B",
	})
	require.NoError(t, err)

	// Query all entries.
	entries, total, err := svc.Query(database.ActivityFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2)

	// Query by tier=realtime — should return only 1.
	entries, total, err = svc.Query(database.ActivityFilter{Tier: "realtime", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "tag_write", entries[0].Type)

	// Summarize realtime entries older than 1 hour in the future (captures all).
	future := time.Now().UTC().Add(time.Hour)
	deleted, err := svc.Summarize(future, "realtime")
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	// Prune background entries.
	deleted, err = svc.Prune(future, "background")
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)
}
