// file: internal/server/malformed_m4b_wrappers.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/remux"
)

// remuxMalformedM4BFiles is a thin wrapper that delegates to the remux package.
func (s *Server) remuxMalformedM4BFiles() {
	remuxer := remux.New(s.store)
	remuxer.RemuxMalformedFiles()
}

// transcodeMalformedM4BFiles is a thin wrapper that delegates to the remux package.
func (s *Server) transcodeMalformedM4BFiles() {
	transcoder := remux.NewTranscoder(s.store)
	transcoder.TranscodeMalformedFiles()
}
