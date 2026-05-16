// file: internal/database/sqlite_store_mark_import.go
// version: 1.0.0
// guid: a9b8c7d6-e5f4-3210-abcd-9876543210fe
// last-edited: 2026-05-15

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// MarkFileImportedFromDeluge sets ImportedFromDelugeAt and DelugeOriginalPath on
// the matching BookFile. Matching is attempted by originalPath first and then
// by torrentHash (via BookVersion lookup). If no matching row is found, the
// call is a no-op and returns nil.
func (s *SQLiteStore) MarkFileImportedFromDeluge(ctx context.Context, originalPath, libraryPath, torrentHash string) error {
	if originalPath == "" && torrentHash == "" {
		return fmt.Errorf("originalPath and torrentHash both empty")
	}

	// Try match by original download path first.
	if originalPath != "" {
		bf, err := s.GetBookFileByPath(originalPath)
		if err != nil {
			return err
		}
		if bf != nil {
			now := time.Now()
			bf.DelugeOriginalPath = originalPath
			bf.FilePath = libraryPath
			bf.ImportedFromDelugeAt = &now
			if torrentHash != "" {
				bf.DelugeHash = torrentHash
			}
			return s.UpdateBookFile(bf.ID, bf)
		}
	}

	// Fallback: match by torrent hash to locate the BookVersion, then update
	// a file belonging to that book/version if we can find one.
	if torrentHash != "" {
		bv, err := s.GetBookVersionByTorrentHash(torrentHash)
		if err != nil {
			return err
		}
		if bv == nil {
			return nil
		}

		files, err := s.GetBookFiles(bv.BookID)
		if err != nil {
			return err
		}

		base := filepath.Base(libraryPath)
		for i := range files {
			f := &files[i]
			if f.DelugeHash == torrentHash || f.VersionID == bv.ID || filepath.Base(f.FilePath) == base {
				now := time.Now()
				f.DelugeOriginalPath = originalPath
				f.FilePath = libraryPath
				f.ImportedFromDelugeAt = &now
				if torrentHash != "" {
					f.DelugeHash = torrentHash
				}
				return s.UpdateBookFile(f.ID, f)
			}
		}
	}

	return nil
}
