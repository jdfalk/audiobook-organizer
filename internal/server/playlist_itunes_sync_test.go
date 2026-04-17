// file: internal/server/playlist_itunes_sync_test.go
// version: 1.0.0

package server

import (
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func TestMigrateITunesSmartPlaylists_NilLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	imported, skipped := MigrateITunesSmartPlaylists(nil, nil)
	if imported != 0 || skipped != 0 {
		t.Errorf("nil library: imported=%d skipped=%d, want 0/0", imported, skipped)
	}
}

func TestMigrateITunesSmartPlaylists_SkipsNonSmart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pebblePath := filepath.Join(t.TempDir(), "pebble")
	store, err := database.NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	lib := &itunes.ITLLibrary{
		Playlists: []itunes.ITLPlaylist{
			{
				Title:   "Not Smart",
				IsSmart: false,
			},
		},
	}

	imported, skipped := MigrateITunesSmartPlaylists(store, lib)
	if imported != 0 || skipped != 0 {
		t.Errorf("non-smart: imported=%d skipped=%d, want 0/0", imported, skipped)
	}
}

func TestMigrateITunesSmartPlaylists_SkipsAlreadyImported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pebblePath := filepath.Join(t.TempDir(), "pebble")
	store, err := database.NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	var pid [8]byte
	pid[0] = 0xAA
	pidHex := "aa00000000000000"

	// Pre-create a playlist with this iTunes PID.
	_, _ = store.CreateUserPlaylist(&database.UserPlaylist{
		Name:               "Already Imported",
		Type:               database.UserPlaylistTypeSmart,
		ITunesPersistentID: pidHex,
	})

	lib := &itunes.ITLLibrary{
		Playlists: []itunes.ITLPlaylist{
			{
				Title:         "Already Imported",
				IsSmart:       true,
				PersistentID:  pid,
				SmartCriteria: []byte{0x01, 0x02, 0x03, 0x04}, // non-empty
			},
		},
	}

	imported, skipped := MigrateITunesSmartPlaylists(store, lib)
	if imported != 0 {
		t.Errorf("expected 0 imported (already exists), got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", skipped)
	}
}

func TestPushDirtyPlaylistsToITunes_NoDirty(t *testing.T) {
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

	pushed := PushDirtyPlaylistsToITunes(store)
	if pushed != 0 {
		t.Errorf("expected 0 pushed with no dirty playlists, got %d", pushed)
	}
}
