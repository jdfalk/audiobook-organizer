// file: internal/server/file_pipeline.go
// version: 2.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890
//
// Thin forwarding layer — the real implementation now lives in
// internal/organizer/pipeline.go. This file provides type aliases so
// the rest of the server package can keep using the old names.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// Type aliases for backward compatibility.
type FileRenameEntry = organizer.FileRenameEntry
type FilePipelineResult = organizer.FilePipelineResult
type RenameResult = organizer.RenameFilesResult
type RelocateRequest = organizer.RelocateRequest
type RelocateResult = organizer.RelocateResult

// ComputeTargetPaths forwards to organizer.ComputeTargetPaths.
var ComputeTargetPaths = organizer.ComputeTargetPaths

// ComputeTargetPathsFromSegments forwards to organizer.ComputeTargetPathsFromSegments.
var ComputeTargetPathsFromSegments = organizer.ComputeTargetPathsFromSegments

// RenameFiles forwards to organizer.RenameFiles.
var RenameFiles = organizer.RenameFiles
