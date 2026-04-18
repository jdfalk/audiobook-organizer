// file: internal/server/file_move.go
// version: 2.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
//
// Thin forwarding layer — the real implementation now lives in
// internal/organizer/move.go.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// Type alias for backward compatibility.
type MoveBookFileResult = organizer.MoveBookFileResult

// MoveBookFile forwards to organizer.MoveBookFile.
var MoveBookFile = organizer.MoveBookFile
