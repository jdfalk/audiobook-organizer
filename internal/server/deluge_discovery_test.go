// file: internal/server/deluge_discovery_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-3f4a-5b6c-7d8e9f0a1b2c

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTracked_EmptySavePath(t *testing.T) {
	known := map[string]struct{}{"/mnt/books/foo.m4b": {}}
	assert.False(t, isTracked("", known))
}

func TestIsTracked_ExactMatch(t *testing.T) {
	known := map[string]struct{}{"/mnt/books/foo": {}}
	assert.True(t, isTracked("/mnt/books/foo", known))
}

func TestIsTracked_PrefixMatch(t *testing.T) {
	known := map[string]struct{}{"/mnt/books/Dune/Dune.m4b": {}}
	assert.True(t, isTracked("/mnt/books/Dune", known))
}

func TestIsTracked_NoMatch(t *testing.T) {
	known := map[string]struct{}{"/mnt/books/Dune/Dune.m4b": {}}
	assert.False(t, isTracked("/mnt/audiobooks/Foundation", known))
}

func TestIsTracked_EmptyKnown(t *testing.T) {
	assert.False(t, isTracked("/mnt/books/Dune", map[string]struct{}{}))
}

func TestIsTracked_PartialNameNotMatched(t *testing.T) {
	// /mnt/books/Du should NOT match /mnt/books/Dune/…
	known := map[string]struct{}{"/mnt/books/Dune/Dune.m4b": {}}
	assert.False(t, isTracked("/mnt/books/Du", known))
}

func TestIsTracked_TrailingSlashNormalized(t *testing.T) {
	known := map[string]struct{}{"/mnt/books/Dune/Dune.m4b": {}}
	assert.True(t, isTracked("/mnt/books/Dune/", known))
}
