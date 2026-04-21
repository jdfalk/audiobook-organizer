// file: internal/server/deluge_discovery_test.go
// version: 1.1.0
// guid: f7a8b9c0-d1e2-3f4a-5b6c-7d8e9f0a1b2c
//
// isTracked receives content_path = filepath.Join(save_path, torrent_name),
// NOT save_path alone. Tests reflect that contract.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The shared download dir — every torrent has this as save_path.
const dlDir = "/mnt/bigdata/books/deluge"

func TestIsTracked_EmptyContentPath(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, isTracked("", known))
}

func TestIsTracked_ExactContentDir(t *testing.T) {
	// save_path/name exactly equals a book's FilePath (single-file torrent)
	known := map[string]struct{}{dlDir + "/Dune.m4b": {}}
	assert.True(t, isTracked(dlDir+"/Dune.m4b", known))
}

func TestIsTracked_ContentDirPrefixMatch(t *testing.T) {
	// Multi-file torrent: content_path = /mnt/…/deluge/Dune
	// DB has:            /mnt/…/deluge/Dune/Dune.m4b
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.True(t, isTracked(dlDir+"/Dune", known))
}

func TestIsTracked_UnimportedTorrent(t *testing.T) {
	// Foundation torrent: content_path not in DB
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, isTracked(dlDir+"/Foundation", known))
}

func TestIsTracked_EmptyKnown(t *testing.T) {
	assert.False(t, isTracked(dlDir+"/Dune", map[string]struct{}{}))
}

func TestIsTracked_PartialNameNotMatched(t *testing.T) {
	// /mnt/…/Du must NOT match /mnt/…/Dune/…
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, isTracked(dlDir+"/Du", known))
}

func TestIsTracked_TrailingSlashNormalized(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.True(t, isTracked(dlDir+"/Dune/", known))
}
