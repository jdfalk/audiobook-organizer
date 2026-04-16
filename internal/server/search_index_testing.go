// file: internal/server/search_index_testing.go
// version: 1.0.0
// guid: 6f2a4d3e-8b5a-4a70-b8c5-3d7e0f1b9a99
//
// Test-only helper for injecting a pre-built Bleve index into a
// Server without spinning up the real Start() lifecycle. Production
// code opens the index inside Start; tests that need the server
// with search wired up can pass their own index via setSearchIndex.

package server

import "github.com/jdfalk/audiobook-organizer/internal/search"

// setSearchIndex replaces the server's search index. Intended for
// test setup only — production code opens the index inside Start().
func (s *Server) setSearchIndex(idx *search.BleveIndex) {
	s.searchIndex = idx
}
