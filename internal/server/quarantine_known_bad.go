// file: internal/server/quarantine_known_bad.go
// version: 1.1.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

package server

import (
	"log"

	"github.com/jdfalk/audiobook-organizer/internal/remux"
)

const quarantineKnownBadKey = "quarantine_known_bad_v1_done"

// quarantineKnownBadFiles runs once at startup: finds any book whose file path
// is marked as permanently taglib-unreadable (transcode_skip_* setting = "true")
// and quarantines it. These are the ~29 files the transcode pass could not fix.
func (s *Server) quarantineKnownBadFiles() {
	store := s.Store()
	if store == nil {
		return
	}

	if setting, err := store.GetSetting(quarantineKnownBadKey); err == nil && setting != nil && setting.Value == "true" {
		return
	}

	books, err := store.GetAllBooks(20000, 0)
	if err != nil {
		log.Printf("[WARN] quarantineKnownBadFiles: GetAllBooks: %v", err)
		return
	}

	quarantined := 0
	for _, b := range books {
		if b.QuarantinedAt != nil {
			continue
		}
		key := remux.TranscodeSkipKey(b.FilePath)
		setting, err := store.GetSetting(key)
		if err != nil || setting == nil || setting.Value != "true" {
			continue
		}
		if err := s.quarantineSvc.QuarantineBook(b.ID, "taglib permanently unreadable after transcode attempt"); err != nil {
			log.Printf("[WARN] quarantineKnownBadFiles: quarantine %s: %v", b.FilePath, err)
			continue
		}
		log.Printf("[INFO] quarantineKnownBadFiles: quarantined %s", b.FilePath)
		quarantined++
	}

	log.Printf("[INFO] quarantineKnownBadFiles: quarantined %d books", quarantined)
	_ = store.SetSetting(quarantineKnownBadKey, "true", "bool", false)
}
