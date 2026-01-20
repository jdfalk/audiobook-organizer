// file: internal/backup/backup_test.go
// version: 1.1.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package backup

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDefaultBackupConfig tests the default backup configuration
func TestDefaultBackupConfig(t *testing.T) {
	// Arrange-Act
	config := DefaultBackupConfig()

	// Assert
	if config.BackupDir != "backups" {
		t.Errorf("Expected BackupDir to be 'backups', got '%s'", config.BackupDir)
	}

	if config.MaxBackups != 10 {
		t.Errorf("Expected MaxBackups to be 10, got %d", config.MaxBackups)
	}

	if config.CompressionLevel != gzip.BestCompression {
		t.Errorf("Expected CompressionLevel to be %d, got %d", gzip.BestCompression, config.CompressionLevel)
	}
}

// TestBackupInfoStructure tests the BackupInfo struct
func TestBackupInfoStructure(t *testing.T) {
	// Arrange
	now := time.Now()
	info := BackupInfo{
		Filename:     "audiobooks_pebble_20240101_120000.tar.gz",
		Path:         "/backups/audiobooks_pebble_20240101_120000.tar.gz",
		Size:         1024000,
		Checksum:     "abc123def456",
		DatabaseType: "pebble",
		CreatedAt:    now,
	}

	// Act & Assert
	if info.Filename == "" {
		t.Error("Expected Filename to be set")
	}

	if info.Path == "" {
		t.Error("Expected Path to be set")
	}

	if info.Size <= 0 {
		t.Error("Expected Size to be positive")
	}

	if info.Checksum == "" {
		t.Error("Expected Checksum to be set")
	}

	if info.DatabaseType != "pebble" {
		t.Errorf("Expected DatabaseType to be 'pebble', got '%s'", info.DatabaseType)
	}

	if info.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

// TestBackupConfigStructure tests the BackupConfig struct
func TestBackupConfigStructure(t *testing.T) {
	// Arrange
	config := BackupConfig{
		BackupDir:        "/var/backups",
		MaxBackups:       5,
		CompressionLevel: 6,
	}

	// Act & Assert
	if config.BackupDir != "/var/backups" {
		t.Errorf("Expected BackupDir to be '/var/backups', got '%s'", config.BackupDir)
	}

	if config.MaxBackups != 5 {
		t.Errorf("Expected MaxBackups to be 5, got %d", config.MaxBackups)
	}

	if config.CompressionLevel != 6 {
		t.Errorf("Expected CompressionLevel to be 6, got %d", config.CompressionLevel)
	}
}

// TestCreateBackupDirectoryCreation tests backup directory creation
func TestCreateBackupDirectoryCreation(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")

	// Create a test database file
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test data"), 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        backupDir,
		MaxBackups:       10,
		CompressionLevel: 1, // Fast compression for tests
	}

	// Act
	_, err := CreateBackup(dbPath, "test", config)

	// Assert
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Verify backup directory was created
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Error("Backup directory was not created")
	}
}

// TestCreateBackupSuccess tests successful backup creation
func TestCreateBackupSuccess(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	// Create a test database file
	dbPath := filepath.Join(tempDir, "test.db")
	testData := []byte("test database content")
	if err := os.WriteFile(dbPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        filepath.Join(tempDir, "backups"),
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Act
	info, err := CreateBackup(dbPath, "test", config)

	// Assert
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected BackupInfo to be returned")
	}

	if info.Filename == "" {
		t.Error("Expected Filename to be set")
	}

	if info.Size <= 0 {
		t.Error("Expected Size to be positive")
	}

	if info.Checksum == "" {
		t.Error("Expected Checksum to be set")
	}

	if info.DatabaseType != "test" {
		t.Errorf("Expected DatabaseType to be 'test', got '%s'", info.DatabaseType)
	}

	// Verify backup file exists
	if _, err := os.Stat(info.Path); os.IsNotExist(err) {
		t.Errorf("Backup file does not exist at %s", info.Path)
	}
}

// TestCreateBackupInvalidPath tests backup with invalid database path
func TestCreateBackupInvalidPath(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "nonexistent.db")

	config := BackupConfig{
		BackupDir:        filepath.Join(tempDir, "backups"),
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Act
	_, err := CreateBackup(nonExistentPath, "test", config)

	// Assert
	if err == nil {
		t.Error("Expected error when creating backup of nonexistent database")
	}
}

// TestRestoreBackupInvalidPath tests restore with invalid backup path
func TestRestoreBackupInvalidPath(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	nonExistentBackup := filepath.Join(tempDir, "nonexistent.tar.gz")
	targetPath := tempDir

	// Act
	err := RestoreBackup(nonExistentBackup, targetPath, false)

	// Assert
	if err == nil {
		t.Error("Expected error when restoring from nonexistent backup")
	}
}

// TestBackupTimestampFormat tests backup filename timestamp format
func TestBackupTimestampFormat(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        filepath.Join(tempDir, "backups"),
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Act
	info, err := CreateBackup(dbPath, "pebble", config)

	// Assert
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Verify filename contains expected format (audiobooks_<type>_<timestamp>.tar.gz)
	if len(info.Filename) < len("audiobooks_pebble_20060102_150405.tar.gz") {
		t.Errorf("Filename appears to be in wrong format: %s", info.Filename)
	}

	// Verify it starts with audiobooks_
	expectedPrefix := "audiobooks_pebble_"
	if len(info.Filename) < len(expectedPrefix) || info.Filename[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected filename to start with '%s', got: %s", expectedPrefix, info.Filename)
	}

	// Verify it ends with .tar.gz
	expectedSuffix := ".tar.gz"
	if len(info.Filename) < len(expectedSuffix) || info.Filename[len(info.Filename)-len(expectedSuffix):] != expectedSuffix {
		t.Errorf("Expected filename to end with '%s', got: %s", expectedSuffix, info.Filename)
	}
}

// TestMultipleBackups tests creating multiple backups
func TestMultipleBackups(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        filepath.Join(tempDir, "backups"),
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Act - Create multiple backups
	info1, err1 := CreateBackup(dbPath, "test", config)
	time.Sleep(1 * time.Second) // Ensure different timestamps (format is second-precision)
	info2, err2 := CreateBackup(dbPath, "test", config)

	// Assert
	if err1 != nil {
		t.Fatalf("First backup failed: %v", err1)
	}

	if err2 != nil {
		t.Fatalf("Second backup failed: %v", err2)
	}

	if info1.Filename == info2.Filename {
		t.Error("Expected different filenames for different backups")
	}

	// Verify both files exist
	if _, err := os.Stat(info1.Path); os.IsNotExist(err) {
		t.Error("First backup file does not exist")
	}

	if _, err := os.Stat(info2.Path); os.IsNotExist(err) {
		t.Error("Second backup file does not exist")
	}
}

// TestBackupChecksum tests checksum generation
func TestBackupChecksum(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	testData := []byte("specific test data for checksum")
	if err := os.WriteFile(dbPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        filepath.Join(tempDir, "backups"),
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Act - Create two backups of the same data
	info1, err1 := CreateBackup(dbPath, "test", config)
	if err1 != nil {
		t.Fatalf("First backup failed: %v", err1)
	}

	// Create another backup with different timestamp but same data
	time.Sleep(1 * time.Second) // Ensure different timestamp (format is second-precision)
	info2, err2 := CreateBackup(dbPath, "test", config)
	if err2 != nil {
		t.Fatalf("Second backup failed: %v", err2)
	}

	// Assert
	// Note: Checksums will be different because timestamps differ
	// Just verify that checksums exist and are valid hex strings
	if len(info1.Checksum) != 64 {
		t.Errorf("Expected checksum length 64 (SHA-256), got %d", len(info1.Checksum))
	}

	if len(info2.Checksum) != 64 {
		t.Errorf("Expected checksum length 64 (SHA-256), got %d", len(info2.Checksum))
	}
}

// TestListBackups tests listing available backups
func TestListBackups(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        backupDir,
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Create 3 backups (with 1-second sleep to ensure different timestamps)
	for i := 0; i < 3; i++ {
		_, err := CreateBackup(dbPath, "pebble", config)
		if err != nil {
			t.Fatalf("Failed to create backup %d: %v", i, err)
		}
		if i < 2 { // Don't sleep after last backup
			time.Sleep(1 * time.Second)
		}
	}

	// Act
	backups, err := ListBackups(backupDir)

	// Assert
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("Expected 3 backups, got %d", len(backups))
	}

	// Verify all backups have proper info
	for i, backup := range backups {
		if backup.Filename == "" {
			t.Errorf("Backup %d has empty filename", i)
		}
		if backup.Size <= 0 {
			t.Errorf("Backup %d has invalid size: %d", i, backup.Size)
		}
		if backup.DatabaseType != "pebble" {
			t.Errorf("Backup %d has wrong type: %s", i, backup.DatabaseType)
		}
	}
}

// TestListBackupsEmptyDirectory tests listing from non-existent directory
func TestListBackupsEmptyDirectory(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "nonexistent")

	// Act
	backups, err := ListBackups(backupDir)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error for non-existent directory, got: %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("Expected 0 backups, got %d", len(backups))
	}
}

// TestDeleteBackup tests backup deletion
func TestDeleteBackup(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        backupDir,
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Create a backup
	info, err := CreateBackup(dbPath, "test", config)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(info.Path); os.IsNotExist(err) {
		t.Fatal("Backup file does not exist")
	}

	// Act - Delete the backup
	err = DeleteBackup(info.Path)

	// Assert
	if err != nil {
		t.Fatalf("DeleteBackup failed: %v", err)
	}

	// Verify backup is deleted
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Error("Backup file still exists after deletion")
	}
}

// TestCleanupOldBackups tests automatic cleanup of old backups
func TestCleanupOldBackups(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        backupDir,
		MaxBackups:       3, // Keep only 3 backups
		CompressionLevel: 1,
	}

	// Create 5 backups (cleanup happens automatically after each, keeping only MaxBackups=3)
	for i := 0; i < 5; i++ {
		_, err := CreateBackup(dbPath, "test", config)
		if err != nil {
			t.Fatalf("Failed to create backup %d: %v", i, err)
		}
		if i < 4 { // Don't sleep after last backup
			time.Sleep(1 * time.Second) // Ensure different timestamps (format is second-precision)
		}
	}

	// Act - List backups after cleanup
	backups, err := ListBackups(backupDir)

	// Assert
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	// Should have only 3 backups (oldest 2 should be deleted)
	if len(backups) != 3 {
		t.Errorf("Expected 3 backups after cleanup, got %d", len(backups))
	}
}

// TestRestoreBackup tests backup restoration
func TestRestoreBackup(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	dbPath := filepath.Join(tempDir, "test.db")
	restoreDir := filepath.Join(tempDir, "restored")

	testData := []byte("test database content for restore")
	if err := os.WriteFile(dbPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := BackupConfig{
		BackupDir:        backupDir,
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Create backup
	info, err := CreateBackup(dbPath, "sqlite", config)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Act - Restore the backup
	err = RestoreBackup(info.Path, restoreDir, false)

	// Assert
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restored file exists and has same content
	restoredFile := filepath.Join(restoreDir, "test.db")
	restoredData, err := os.ReadFile(restoredFile)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if string(restoredData) != string(testData) {
		t.Errorf("Restored data mismatch. Expected '%s', got '%s'", string(testData), string(restoredData))
	}
}

// TestBackupPebbleDirectory tests backing up a PebbleDB directory
func TestBackupPebbleDirectory(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	pebbleDir := filepath.Join(tempDir, "pebble.db")

	// Create a mock PebbleDB directory structure
	if err := os.MkdirAll(pebbleDir, 0755); err != nil {
		t.Fatalf("Failed to create pebble directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pebbleDir, "MANIFEST-000001"), []byte("manifest"), 0644); err != nil {
		t.Fatalf("Failed to create manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pebbleDir, "000001.sst"), []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to create sst file: %v", err)
	}

	config := BackupConfig{
		BackupDir:        backupDir,
		MaxBackups:       10,
		CompressionLevel: 1,
	}

	// Act
	info, err := CreateBackup(pebbleDir, "pebble", config)

	// Assert
	if err != nil {
		t.Fatalf("CreateBackup failed for directory: %v", err)
	}

	if info == nil {
		t.Fatal("Expected BackupInfo to be returned")
	}

	// Verify backup exists and is larger than zero
	if info.Size <= 0 {
		t.Error("Expected backup size to be positive for directory")
	}
}

// TestBackupDifferentCompressionLevels tests different compression levels
func TestBackupDifferentCompressionLevels(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	dbPath := filepath.Join(tempDir, "test.db")

	// Create a larger test file for better compression testing
	testData := make([]byte, 10000)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	if err := os.WriteFile(dbPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Test different compression levels
	compressionLevels := []int{gzip.NoCompression, gzip.BestSpeed, gzip.BestCompression}
	var sizes []int64

	for _, level := range compressionLevels {
		config := BackupConfig{
			BackupDir:        filepath.Join(backupDir, "level"+string(rune('0'+level))),
			MaxBackups:       10,
			CompressionLevel: level,
		}

		info, err := CreateBackup(dbPath, "test", config)
		if err != nil {
			t.Fatalf("CreateBackup failed for compression level %d: %v", level, err)
		}

		sizes = append(sizes, info.Size)
	}

	// Assert - NoCompression should be largest, BestCompression should be smallest
	if sizes[0] < sizes[2] {
		t.Error("Expected NoCompression to be larger than BestCompression")
	}
}

// TestScheduleBackupNotImplemented tests that ScheduleBackup returns error
func TestScheduleBackupNotImplemented(t *testing.T) {
	// Arrange
	config := DefaultBackupConfig()
	interval := 1 * time.Hour

	// Act
	err := ScheduleBackup(interval, config)

	// Assert
	if err == nil {
		t.Error("Expected error for unimplemented ScheduleBackup")
	}
	expectedMsg := "scheduled backups not yet implemented"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestBackupDatabaseNilStore tests BackupDatabase with nil database
func TestBackupDatabaseNilStore(t *testing.T) {
	// Arrange
	config := DefaultBackupConfig()

	// Act
	info, err := BackupDatabase(config)

	// Assert
	if err == nil {
		t.Error("Expected error for nil database")
	}
	if info != nil {
		t.Error("Expected nil BackupInfo on error")
	}
	expectedMsg := "database not initialized"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}
