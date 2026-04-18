// file: internal/server/path_format.go
// version: 2.0.0
// guid: a7b3c1d2-e4f5-6789-abcd-ef0123456789
//
// Thin forwarding layer — the real implementation now lives in
// internal/organizer/path_format.go.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// Type alias for backward compatibility.
type FormatVars = organizer.FormatVars

// Constants forwarded from organizer.
const (
	DefaultPathFormat         = organizer.DefaultPathFormat
	DefaultSegmentTitleFormat = organizer.DefaultSegmentTitleFormat
)

// Function forwards.
var FormatSegmentTitle = organizer.FormatSegmentTitle
var FormatPath = organizer.FormatPath

// Unexported forwarding functions for backward-compatible test usage.
func sanitizePathComponent(s string) string { return organizer.SanitizePathComponent(s) }
func collapseEmptySegments(s string) string { return organizer.CollapseEmptySegments(s) }
