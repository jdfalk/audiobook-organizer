// file: internal/server/file_move.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// MoveBookFileResult captures the outcome of an atomic file move.
type MoveBookFileResult struct {
	OldPath string
	NewPath string
	BookID  string
}

// MoveBookFile atomically moves a book's file and updates the database.
// The sequence is designed to be rollback-safe:
//  1. Verify source file exists
//  2. Physically move/rename the file
//  3. Update the database with new path
//  4. If DB update fails, move the file back and return error
//
// This prevents orphaned files (file moved but DB not updated).
func MoveBookFile(store database.Store, bookID, oldPath, newPath string, extraUpdates *database.Book) error {
	if oldPath == newPath {
		return nil // Nothing to do
	}

	// Step 1: Verify source exists
	if _, err := os.Stat(oldPath); err != nil {
		return fmt.Errorf("source file does not exist: %s: %w", oldPath, err)
	}

	// Step 2: Check destination doesn't already exist
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("destination already exists: %s", newPath)
	}

	// Step 3: Ensure destination directory exists
	if dir := filepath.Dir(newPath); dir != "" {
		if err := os.MkdirAll(dir, 0o775); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}
	}

	// Step 4: Move the file
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to move file %s → %s: %w", oldPath, newPath, err)
	}

	// Step 5: Update database
	update := &database.Book{
		FilePath: newPath,
	}
	if extraUpdates != nil {
		// Merge extra updates
		update = extraUpdates
		update.FilePath = newPath
	}

	if _, err := store.UpdateBook(bookID, update); err != nil {
		// ROLLBACK: Move file back to original location
		log.Printf("[ERROR] file_move: DB update failed for %s, rolling back file move: %v", bookID, err)
		if rbErr := os.Rename(newPath, oldPath); rbErr != nil {
			// Critical: file is at new location but DB points to old location
			log.Printf("[CRITICAL] file_move: rollback failed! File at %s, DB expects %s: %v", newPath, oldPath, rbErr)
			return fmt.Errorf("DB update failed and rollback failed: file at %s, DB expects %s: %w", newPath, oldPath, err)
		}
		return fmt.Errorf("DB update failed (file rolled back): %w", err)
	}

	log.Printf("[INFO] file_move: moved %s → %s for book %s", oldPath, newPath, bookID)
	return nil
}
