// file: internal/server/server_accessors.go
// version: 1.0.0
// guid: 503fedeb-56f2-4a8e-812d-484a6413d88b
// last-edited: 2026-05-10

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// OrganizeService returns the server's organize service instance.
func (s *Server) OrganizeService() *OrganizeService { return s.organizeService }

// FilesystemService returns the server's filesystem browsing and exclusion service.
func (s *Server) FilesystemService() *fileops.FilesystemService { return s.filesystemService }

// ImportPathService returns the service managing import-path CRUD.
func (s *Server) ImportPathService() *importer.ImportPathService { return s.importPathService }

// ImportService returns the service handling single-file imports.
func (s *Server) ImportService() *importer.ImportService { return s.importService }

// Queue returns the server's operation queue (may be nil before server start).
func (s *Server) Queue() operations.Queue { return s.queue }

// DedupEngine returns the deduplication engine (may be nil if dedup is disabled).
func (s *Server) DedupEngine() *dedup.Engine { return s.dedupEngine }

// ActivityService returns the activity log service (may be nil if not configured).
func (s *Server) ActivityService() *activity.Service { return s.activityService }
