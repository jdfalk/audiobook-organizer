// file: internal/backup/backup.go
// version: 1.2.1
// guid: 8f9e0a1b-2c3d-4e5f-6a7b-8c9d0e1f2a3b

package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// BackupInfo contains information about a backup
type BackupInfo struct {
	Filename     string    `json:"filename"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	Checksum     string    `json:"checksum"`
	DatabaseType string    `json:"database_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// BackupConfig holds backup configuration
type BackupConfig struct {
	BackupDir        string
	MaxBackups       int
	CompressionLevel int
}

// DefaultBackupConfig returns default backup configuration
func DefaultBackupConfig() BackupConfig {
	return BackupConfig{
		BackupDir:        "backups",
		MaxBackups:       10,
		CompressionLevel: gzip.BestCompression,
	}
}

// CreateBackup creates a compressed backup of the database
func CreateBackup(databasePath, databaseType string, config BackupConfig) (*BackupInfo, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(config.BackupDir, 0775); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupFilename := fmt.Sprintf("audiobooks_%s_%s.tar.gz", databaseType, timestamp)
	backupPath := filepath.Join(config.BackupDir, backupFilename)

	// Create backup file
	backupFile, err := os.Create(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backupFile.Close()

	// Create gzip writer
	gzipWriter, err := gzip.NewWriterLevel(backupFile, config.CompressionLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Add database files to archive
	if err := addToArchive(tarWriter, databasePath, databaseType); err != nil {
		os.Remove(backupPath) // Clean up on failure
		return nil, fmt.Errorf("failed to add files to archive: %w", err)
	}

	// Close writers to ensure all data is flushed
	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	if err := backupFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close backup file: %w", err)
	}

	// Get backup file info
	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat backup file: %w", err)
	}

	// Calculate checksum
	checksum, err := calculateFileChecksum(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	info := &BackupInfo{
		Filename:     backupFilename,
		Path:         backupPath,
		Size:         fileInfo.Size(),
		Checksum:     checksum,
		DatabaseType: databaseType,
		CreatedAt:    time.Now(),
	}

	// Clean up old backups
	if err := cleanupOldBackups(config.BackupDir, config.MaxBackups); err != nil {
		// Log error but don't fail the backup
		log.Printf("[WARN] backup: failed to clean up old backups: %v", err)
	}

	return info, nil
}

// isPathWithinTarget validates that a path resolves within the target directory.
// This prevents zipslip attacks where archive entries use traversal sequences.
func isPathWithinTarget(targetPath, entryPath string) (bool, error) {
	// Ensure target path is absolute
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false, err
	}

	// Join entry path with target (this handles "." entries)
	candidate := filepath.Join(absTarget, entryPath)

	// Clean the path to remove . and .. sequences
	candidate = filepath.Clean(candidate)

	// Verify the cleaned path is still within target
	// Use filepath.Rel to compute relative path; if it escapes, Rel will return ".." 
	rel, err := filepath.Rel(absTarget, candidate)
	if err != nil {
		return false, err
	}

	// If rel is absolute, it escaped the target
	if filepath.IsAbs(rel) {
		return false, nil
	}

	// If rel contains "..", it tried to escape
	if strings.Contains(rel, "..") {
		return false, nil
	}

	return true, nil
}

// RestoreBackup restores a database from a backup file
func RestoreBackup(backupPath, targetPath string, verify bool) error {
	// Verify checksum if requested
	if verify {
		// TODO: Store checksums in metadata file and verify
		log.Printf("[INFO] backup: Checksum verification not yet implemented")
	}

	// Open backup file
	backupFile, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer backupFile.Close()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(backupFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Validate entry path to prevent zipslip attacks
		within, err := isPathWithinTarget(targetPath, header.Name)
		if err != nil {
			return fmt.Errorf("failed to validate archive entry path %q: %w", header.Name, err)
		}
		if !within {
			return fmt.Errorf("archive entry %q escapes target directory", header.Name)
		}

		// Construct target path
		target := filepath.Join(targetPath, header.Name)

		// Handle directories and files
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0775); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0775); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", target, err)
			}

			// Create file
			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}

			// Copy data
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}

			outFile.Close()

			// Set file permissions
			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to set permissions on %s: %w", target, err)
			}
		default:
			log.Printf("[WARN] backup: unsupported file type %d for %s", header.Typeflag, header.Name)
		}
	}

	return nil
}

// ListBackups lists all available backups
func ListBackups(backupDir string) ([]BackupInfo, error) {
	var backups []BackupInfo

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return backups, nil // No backups directory yet
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backupPath := filepath.Join(backupDir, entry.Name())
		checksum, _ := calculateFileChecksum(backupPath)

		// Parse database type from filename
		dbType := "unknown"
		if strings.Contains(entry.Name(), "_pebble_") {
			dbType = "pebble"
		} else if strings.Contains(entry.Name(), "_sqlite_") {
			dbType = "sqlite"
		}

		backups = append(backups, BackupInfo{
			Filename:     entry.Name(),
			Path:         backupPath,
			Size:         info.Size(),
			Checksum:     checksum,
			DatabaseType: dbType,
			CreatedAt:    info.ModTime(),
		})
	}

	return backups, nil
}

// DeleteBackup deletes a specific backup file
func DeleteBackup(backupPath string) error {
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	return nil
}

// addToArchive adds a database path to a tar archive
func addToArchive(tarWriter *tar.Writer, path, dbType string) error {
	// Check if path is a directory (PebbleDB) or file (SQLite)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat database path: %w", err)
	}

	if info.IsDir() {
		// PebbleDB - archive entire directory
		return filepath.Walk(path, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Create tar header
			header, err := tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}

			// Use relative path in archive
			relPath, err := filepath.Rel(filepath.Dir(path), file)
			if err != nil {
				return err
			}
			header.Name = relPath

			// Write header
			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}

			// Write file content if not a directory
			if !fi.IsDir() {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()

				if _, err := io.Copy(tarWriter, f); err != nil {
					return err
				}
			}

			return nil
		})
	} else {
		// SQLite - archive single file
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.Base(path)

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	}
}

// calculateFileChecksum calculates SHA256 checksum of a file
func calculateFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// cleanupOldBackups removes old backups exceeding the maximum count
func cleanupOldBackups(backupDir string, maxBackups int) error {
	backups, err := ListBackups(backupDir)
	if err != nil {
		return err
	}

	if len(backups) <= maxBackups {
		return nil
	}

	// Sort backups by creation time (oldest first)
	// Simple bubble sort since list is typically small
	for i := 0; i < len(backups)-1; i++ {
		for j := i + 1; j < len(backups); j++ {
			if backups[i].CreatedAt.After(backups[j].CreatedAt) {
				backups[i], backups[j] = backups[j], backups[i]
			}
		}
	}

	// Delete oldest backups
	deleteCount := len(backups) - maxBackups
	for i := 0; i < deleteCount; i++ {
		if err := os.Remove(backups[i].Path); err != nil {
			log.Printf("[WARN] backup: failed to delete old backup %s: %v", backups[i].Filename, err)
		}
	}

	return nil
}

// ScheduleBackup schedules automatic backups (placeholder for future implementation)
func ScheduleBackup(interval time.Duration, config BackupConfig) error {
	// TODO: Implement scheduled backups using a ticker
	// This would run in a goroutine and create backups at regular intervals
	return fmt.Errorf("scheduled backups not yet implemented")
}

// BackupDatabase is a convenience function that backs up the global database
func BackupDatabase(config BackupConfig) (*BackupInfo, error) {
	if database.GetGlobalStore() == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get database path and type from config
	// TODO: Add methods to Store interface to get database path and type
	// For now, we'll need to pass these as parameters

	return nil, fmt.Errorf("backup requires database path and type information")
}
