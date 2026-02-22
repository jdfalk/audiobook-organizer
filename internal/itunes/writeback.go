package itunes

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteBackUpdate represents a single audiobook file path update
type WriteBackUpdate struct {
	ITunesPersistentID string // iTunes unique identifier
	OldPath            string // Original iTunes file path
	NewPath            string // New organized file path
}

// WriteBackOptions configures the write-back operation
type WriteBackOptions struct {
	LibraryPath      string             // Path to iTunes Library.xml to update
	Updates          []*WriteBackUpdate // List of path updates to apply
	CreateBackup      bool               // Create backup before modifying (recommended)
	BackupPath        string             // Optional custom backup path
	ForceOverwrite    bool               // Skip fingerprint check (user confirmed override)
	StoredFingerprint *LibraryFingerprint // Fingerprint from last import (nil = skip check)
}

// WriteBackResult contains the results of a write-back operation
type WriteBackResult struct {
	Success      bool   // Overall success
	UpdatedCount int    // Number of tracks updated
	BackupPath   string // Path to backup file (if created)
	Message      string // Success or error message
}

// WriteBack updates an iTunes Library.xml file with new file paths after organization
// This allows iTunes to find audiobooks after they've been moved/organized
func WriteBack(opts WriteBackOptions) (*WriteBackResult, error) {
	// Validate inputs
	if opts.LibraryPath == "" {
		return nil, fmt.Errorf("library path is required")
	}
	if len(opts.Updates) == 0 {
		return nil, fmt.Errorf("no updates provided")
	}
	if _, err := os.Stat(opts.LibraryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("iTunes library file not found: %s", opts.LibraryPath)
	}

	// Check for external modifications (unless force override)
	if !opts.ForceOverwrite && opts.StoredFingerprint != nil {
		current, err := ComputeFingerprint(opts.LibraryPath)
		if err != nil {
			return nil, fmt.Errorf("failed to check library state: %w", err)
		}
		if !opts.StoredFingerprint.Matches(current) {
			return nil, &ErrLibraryModified{
				Stored:  opts.StoredFingerprint,
				Current: current,
			}
		}
	}

	// Create backup if requested
	var backupPath string
	if opts.CreateBackup {
		if opts.BackupPath != "" {
			backupPath = opts.BackupPath
		} else {
			// Generate backup filename with timestamp
			timestamp := time.Now().Format("20060102-150405")
			backupPath = opts.LibraryPath + ".backup." + timestamp
		}

		// Copy original file to backup
		if err := copyFile(opts.LibraryPath, backupPath); err != nil {
			return nil, fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Parse the library
	library, err := ParseLibrary(opts.LibraryPath)
	if err != nil {
		// Restore backup if parsing fails
		if opts.CreateBackup {
			os.Rename(backupPath, opts.LibraryPath)
		}
		return nil, fmt.Errorf("failed to parse iTunes library: %w", err)
	}

	// Create a map of persistent ID -> new path for fast lookup
	updateMap := make(map[string]string)
	for _, update := range opts.Updates {
		updateMap[update.ITunesPersistentID] = update.NewPath
	}

	// Update track locations
	updatedCount := 0
	for trackID, track := range library.Tracks {
		if newPath, ok := updateMap[track.PersistentID]; ok {
			// Encode the new path as a file:// URL
			encodedPath := EncodeLocation(newPath)
			library.Tracks[trackID].Location = encodedPath
			updatedCount++
		}
	}

	// Write the updated library back to disk
	if err := writeLibrary(library, opts.LibraryPath); err != nil {
		// Restore backup on error
		if opts.CreateBackup {
			os.Rename(backupPath, opts.LibraryPath)
		}
		return nil, fmt.Errorf("failed to write updated library: %w", err)
	}

	return &WriteBackResult{
		Success:      true,
		UpdatedCount: updatedCount,
		BackupPath:   backupPath,
		Message:      fmt.Sprintf("Successfully updated %d audiobook locations", updatedCount),
	}, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// Read source file
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Write to destination
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	return nil
}

// writeLibrary writes a Library structure back to an iTunes Library.xml file
func writeLibrary(library *Library, path string) error {
	// Actual implementation is in plist_parser.go
	return writePlist(library, path)

	// The implementation uses:

	// When implemented, the flow should be:
	// 1. Open temp file for writing
	// 2. Encode library to plist format
	// 3. Close temp file
	// 4. Atomically rename temp file to final path
	// 5. Sync to ensure data is written to disk

	// Example:
	// file, err := os.Create(tempFile)
	// if err != nil {
	//     return err
	// }
	// defer os.Remove(tempFile) // Clean up temp file on error
	//
	// encoder := plist.NewEncoder(file)
	// if err := encoder.Encode(library); err != nil {
	//     file.Close()
	//     return err
	// }
	// file.Close()
	//
	// // Atomic rename
	// if err := os.Rename(tempFile, path); err != nil {
	//     return err
	// }
	//
	// return nil
}

// ValidateWriteBack performs a dry-run validation of write-back updates
// Returns a list of warnings/errors without modifying the iTunes library
func ValidateWriteBack(opts WriteBackOptions) ([]string, error) {
	warnings := make([]string, 0)

	// Check if library file exists
	if _, err := os.Stat(opts.LibraryPath); os.IsNotExist(err) {
		return warnings, fmt.Errorf("iTunes library file not found: %s", opts.LibraryPath)
	}

	// Parse the library
	library, err := ParseLibrary(opts.LibraryPath)
	if err != nil {
		return warnings, fmt.Errorf("failed to parse iTunes library: %w", err)
	}

	// Check each update
	persistentIDs := make(map[string]bool)
	for _, track := range library.Tracks {
		persistentIDs[track.PersistentID] = true
	}

	for _, update := range opts.Updates {
		// Check if persistent ID exists in library
		if !persistentIDs[update.ITunesPersistentID] {
			warnings = append(warnings, fmt.Sprintf("Persistent ID not found in iTunes library: %s", update.ITunesPersistentID))
			continue
		}

		// Check if new path exists
		if _, err := os.Stat(update.NewPath); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("New file path does not exist: %s", update.NewPath))
		}
	}

	return warnings, nil
}
