// file: internal/server/deluge_discovery_test.go
// version: 3.0.0
// guid: f7a8b9c0-d1e2-3f4a-5b6c-7d8e9f0a1b2c
// last-edited: 2026-05-11
//
// Tests for the discovery helpers — now delegates to internal/deluge/discovery.go.

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falkcorp/audiobook-organizer/internal/deluge"
)

// The shared download dir — every torrent has this as save_path.
const dlDir = "/mnt/bigdata/books/deluge"

// ---------------------------------------------------------------------------
// Tier 2: IsPathTracked
// ---------------------------------------------------------------------------

func TestIsPathTracked_EmptyContentPath(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, deluge.IsPathTracked("", known))
}

func TestIsPathTracked_ExactMatch(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune.m4b": {}}
	assert.True(t, deluge.IsPathTracked(dlDir+"/Dune.m4b", known))
}

func TestIsPathTracked_ContentDirPrefixMatch(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.True(t, deluge.IsPathTracked(dlDir+"/Dune", known))
}

func TestIsPathTracked_UnimportedTorrent(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, deluge.IsPathTracked(dlDir+"/Foundation", known))
}

func TestIsPathTracked_EmptyKnown(t *testing.T) {
	assert.False(t, deluge.IsPathTracked(dlDir+"/Dune", map[string]struct{}{}))
}

func TestIsPathTracked_PartialNameNotMatched(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, deluge.IsPathTracked(dlDir+"/Du", known))
}

func TestIsPathTracked_TrailingSlashNormalized(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.True(t, deluge.IsPathTracked(dlDir+"/Dune/", known))
}

// ---------------------------------------------------------------------------
// Tier 3: IsTitleTracked / ParseTorrentNameCandidates / NormalizeTitle
// ---------------------------------------------------------------------------

func TestNormalizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Way of Kings", "the way of kings"},
		{"The Way of Kings!", "the way of kings"},
		{"Dune (2023)", "dune 2023"},
		{"Foundation - Isaac Asimov", "foundation isaac asimov"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, deluge.NormalizeTitle(c.in), c.in)
	}
}

func TestParseTorrentNameCandidates_DashSeparated(t *testing.T) {
	candidates := deluge.ParseTorrentNameCandidates("Brandon Sanderson - The Way of Kings")
	assert.Contains(t, candidates, deluge.NormalizeTitle("Brandon Sanderson"))
	assert.Contains(t, candidates, deluge.NormalizeTitle("The Way of Kings"))
}

func TestParseTorrentNameCandidates_ByKeyword(t *testing.T) {
	candidates := deluge.ParseTorrentNameCandidates("The Way of Kings by Brandon Sanderson [M4B]")
	assert.Contains(t, candidates, deluge.NormalizeTitle("The Way of Kings"))
}

func TestParseTorrentNameCandidates_DotSeparated(t *testing.T) {
	candidates := deluge.ParseTorrentNameCandidates("Dune.Frank.Herbert.2023.M4B")
	assert.Contains(t, candidates, "dune frank herbert")
}

func TestIsTitleTracked_Hit(t *testing.T) {
	titles := map[string]struct{}{
		deluge.NormalizeTitle("The Way of Kings"): {},
	}
	assert.True(t, deluge.IsTitleTracked("Brandon Sanderson - The Way of Kings [M4B]", titles))
}

func TestIsTitleTracked_Miss(t *testing.T) {
	titles := map[string]struct{}{
		deluge.NormalizeTitle("Dune"): {},
	}
	assert.False(t, deluge.IsTitleTracked("Brandon Sanderson - The Way of Kings", titles))
}

// ---------------------------------------------------------------------------
// Tier 4: IsContentHashTracked / SHA256File
// ---------------------------------------------------------------------------

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.m4b")
	require.NoError(t, os.WriteFile(f, []byte("audiodata"), 0o644))

	hash1, err := deluge.SHA256File(f)
	require.NoError(t, err)
	assert.Len(t, hash1, 64) // hex SHA256

	// Same content → same hash.
	hash2, _ := deluge.SHA256File(f)
	assert.Equal(t, hash1, hash2)
}

func TestSha256File_Missing(t *testing.T) {
	_, err := deluge.SHA256File("/nonexistent/file.m4b")
	assert.Error(t, err)
}

func TestIsContentHashTracked_MatchFound(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "book.m4b")
	require.NoError(t, os.WriteFile(audio, []byte("audiodata"), 0o644))

	expected, _ := deluge.SHA256File(audio)
	lookup := func(h string) bool { return h == expected }

	assert.True(t, deluge.IsContentHashTracked(dir, lookup))
}

func TestIsContentHashTracked_NoMatch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "book.m4b"), []byte("audiodata"), 0o644))

	lookup := func(h string) bool { return false }
	assert.False(t, deluge.IsContentHashTracked(dir, lookup))
}

func TestIsContentHashTracked_SkipsNonAudioFiles(t *testing.T) {
	dir := t.TempDir()
	// Only a .txt file — no audio files to hash.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0o644))

	called := false
	lookup := func(h string) bool { called = true; return true }
	assert.False(t, deluge.IsContentHashTracked(dir, lookup))
	assert.False(t, called, "lookup should not be called for non-audio files")
}

func TestIsContentHashTracked_MissingDir(t *testing.T) {
	// Walk on a nonexistent dir returns false without panicking.
	lookup := func(h string) bool { return true }
	assert.False(t, deluge.IsContentHashTracked("/nonexistent/path", lookup))
}
