// file: internal/server/deluge_discovery_test.go
// version: 2.0.0
// guid: f7a8b9c0-d1e2-3f4a-5b6c-7d8e9f0a1b2c

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shared download dir — every torrent has this as save_path.
const dlDir = "/mnt/bigdata/books/deluge"

// ---------------------------------------------------------------------------
// Tier 2: isPathTracked
// ---------------------------------------------------------------------------

func TestIsPathTracked_EmptyContentPath(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, isPathTracked("", known))
}

func TestIsPathTracked_ExactMatch(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune.m4b": {}}
	assert.True(t, isPathTracked(dlDir+"/Dune.m4b", known))
}

func TestIsPathTracked_ContentDirPrefixMatch(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.True(t, isPathTracked(dlDir+"/Dune", known))
}

func TestIsPathTracked_UnimportedTorrent(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, isPathTracked(dlDir+"/Foundation", known))
}

func TestIsPathTracked_EmptyKnown(t *testing.T) {
	assert.False(t, isPathTracked(dlDir+"/Dune", map[string]struct{}{}))
}

func TestIsPathTracked_PartialNameNotMatched(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.False(t, isPathTracked(dlDir+"/Du", known))
}

func TestIsPathTracked_TrailingSlashNormalized(t *testing.T) {
	known := map[string]struct{}{dlDir + "/Dune/Dune.m4b": {}}
	assert.True(t, isPathTracked(dlDir+"/Dune/", known))
}

// ---------------------------------------------------------------------------
// Tier 3: isTitleTracked / parseTorrentNameCandidates / normalizeTitle
// ---------------------------------------------------------------------------

func TestNormalizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Way of Kings", "the way of kings"},
		{"The Way of Kings!", "the way of kings"},
		{"Dune (2023)", "dune 2023"},
		{"Foundation - Isaac Asimov", "foundation isaac asimov"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, normalizeTitle(c.in), c.in)
	}
}

func TestParseTorrentNameCandidates_DashSeparated(t *testing.T) {
	candidates := parseTorrentNameCandidates("Brandon Sanderson - The Way of Kings")
	assert.Contains(t, candidates, normalizeTitle("Brandon Sanderson"))
	assert.Contains(t, candidates, normalizeTitle("The Way of Kings"))
}

func TestParseTorrentNameCandidates_ByKeyword(t *testing.T) {
	candidates := parseTorrentNameCandidates("The Way of Kings by Brandon Sanderson [M4B]")
	assert.Contains(t, candidates, normalizeTitle("The Way of Kings"))
}

func TestParseTorrentNameCandidates_DotSeparated(t *testing.T) {
	candidates := parseTorrentNameCandidates("Dune.Frank.Herbert.2023.M4B")
	assert.Contains(t, candidates, "dune frank herbert")
}

func TestIsTitleTracked_Hit(t *testing.T) {
	titles := map[string]struct{}{
		normalizeTitle("The Way of Kings"): {},
	}
	assert.True(t, isTitleTracked("Brandon Sanderson - The Way of Kings [M4B]", titles))
}

func TestIsTitleTracked_Miss(t *testing.T) {
	titles := map[string]struct{}{
		normalizeTitle("Dune"): {},
	}
	assert.False(t, isTitleTracked("Brandon Sanderson - The Way of Kings", titles))
}

// ---------------------------------------------------------------------------
// Tier 4: isContentHashTracked / sha256File
// ---------------------------------------------------------------------------

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.m4b")
	require.NoError(t, os.WriteFile(f, []byte("audiodata"), 0o644))

	hash1, err := sha256File(f)
	require.NoError(t, err)
	assert.Len(t, hash1, 64) // hex SHA256

	// Same content → same hash.
	hash2, _ := sha256File(f)
	assert.Equal(t, hash1, hash2)
}

func TestSha256File_Missing(t *testing.T) {
	_, err := sha256File("/nonexistent/file.m4b")
	assert.Error(t, err)
}

func TestIsContentHashTracked_MatchFound(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "book.m4b")
	require.NoError(t, os.WriteFile(audio, []byte("audiodata"), 0o644))

	expected, _ := sha256File(audio)
	lookup := func(h string) bool { return h == expected }

	assert.True(t, isContentHashTracked(dir, lookup))
}

func TestIsContentHashTracked_NoMatch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "book.m4b"), []byte("audiodata"), 0o644))

	lookup := func(h string) bool { return false }
	assert.False(t, isContentHashTracked(dir, lookup))
}

func TestIsContentHashTracked_SkipsNonAudioFiles(t *testing.T) {
	dir := t.TempDir()
	// Only a .txt file — no audio files to hash.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0o644))

	called := false
	lookup := func(h string) bool { called = true; return true }
	assert.False(t, isContentHashTracked(dir, lookup))
	assert.False(t, called, "lookup should not be called for non-audio files")
}

func TestIsContentHashTracked_MissingDir(t *testing.T) {
	// Walk on a nonexistent dir returns false without panicking.
	lookup := func(h string) bool { return true }
	assert.False(t, isContentHashTracked("/nonexistent/path", lookup))
}
