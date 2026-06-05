// file: internal/itunes/service/status.go
// version: 1.0.0
// guid: 7a4f1e3c-9b2d-4e8f-a0c1-5d7e9f3b6a2c

package itunesservice

import (
	"fmt"
	"sync"

	"github.com/falkcorp/audiobook-organizer/internal/logger"
)

const importErrorLimit = 50
const importProgressBatch = 100

// itunesImportStatus holds in-flight progress for one import operation.
type itunesImportStatus struct {
	mu        sync.Mutex
	Total     int
	Processed int
	Imported  int
	Skipped   int
	Linked    int
	Failed    int
	Errors    []string
}

// importStatusMap is a concurrent map from opID → *itunesImportStatus,
// scoped to Importer so each instance has isolated state (testable).
type importStatusMap struct {
	m sync.Map
}

func (sm *importStatusMap) load(opID string) *itunesImportStatus {
	if v, ok := sm.m.Load(opID); ok {
		if s, ok := v.(*itunesImportStatus); ok {
			return s
		}
	}
	s := &itunesImportStatus{}
	sm.m.Store(opID, s)
	return s
}

func (sm *importStatusMap) snapshot(opID string) *itunesImportStatus {
	s := sm.load(opID)
	s.mu.Lock()
	defer s.mu.Unlock()
	return &itunesImportStatus{
		Total:     s.Total,
		Processed: s.Processed,
		Imported:  s.Imported,
		Skipped:   s.Skipped,
		Linked:    s.Linked,
		Failed:    s.Failed,
		Errors:    append([]string(nil), s.Errors...),
	}
}

func setImportTotal(s *itunesImportStatus, total int) {
	s.mu.Lock()
	s.Total = total
	s.mu.Unlock()
}

func incImportProcessed(s *itunesImportStatus, n int) {
	s.mu.Lock()
	s.Processed = n
	s.mu.Unlock()
}

func incImportImported(s *itunesImportStatus) {
	s.mu.Lock()
	s.Imported++
	s.mu.Unlock()
}

func incImportSkipped(s *itunesImportStatus) {
	s.mu.Lock()
	s.Skipped++
	s.mu.Unlock()
}

func incImportLinked(s *itunesImportStatus) {
	s.mu.Lock()
	s.Linked++
	s.mu.Unlock()
}

func recordImportFailure(s *itunesImportStatus, msg string) {
	s.mu.Lock()
	s.Failed++
	if len(s.Errors) < importErrorLimit {
		s.Errors = append(s.Errors, msg)
	}
	s.mu.Unlock()
}

func recordImportError(s *itunesImportStatus, msg string) {
	s.mu.Lock()
	if len(s.Errors) < importErrorLimit {
		s.Errors = append(s.Errors, msg)
	}
	s.mu.Unlock()
}

func updateImportProgress(log logger.Logger, s *itunesImportStatus, processed, total int, currentTitle ...string) {
	s.mu.Lock()
	current := s.Processed
	imported := s.Imported
	linked := s.Linked
	skipped := s.Skipped
	failed := s.Failed
	s.mu.Unlock()

	if processed%importProgressBatch != 0 && processed != total {
		return
	}

	msg := fmt.Sprintf(
		"Book %d/%d — %d new, %d linked, %d skipped, %d failed",
		current, total, imported, linked, skipped, failed,
	)
	if len(currentTitle) > 0 && currentTitle[0] != "" {
		msg += " — " + currentTitle[0]
	}
	log.UpdateProgress(processed, total, msg)
}

func buildImportSummary(s *itunesImportStatus) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf(
		"Import completed: %d new, %d linked, %d skipped, %d failed",
		s.Imported, s.Linked, s.Skipped, s.Failed,
	)
}
