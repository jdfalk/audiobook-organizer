// file: internal/itunes/fingerprint.go
// version: 1.0.0
// guid: d8e9f0a1-b2c3-4d5e-6f7a-8b9c0d1e2f3a

package itunes

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"time"
)

// LibraryFingerprint captures the state of an iTunes Library.xml file
// for change detection. Uses CRC32 for speed (50ms vs 500ms for SHA256
// on large 100MB+ library files).
type LibraryFingerprint struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	CRC32   uint32    `json:"crc32"`
}

// ErrLibraryModified is returned when a write-back is attempted but the
// library file has been modified since last import.
type ErrLibraryModified struct {
	Stored  *LibraryFingerprint
	Current *LibraryFingerprint
}

func (e *ErrLibraryModified) Error() string {
	return fmt.Sprintf(
		"iTunes library has been modified externally (size: %d→%d, mtime: %s→%s)",
		e.Stored.Size, e.Current.Size,
		e.Stored.ModTime.Format(time.RFC3339),
		e.Current.ModTime.Format(time.RFC3339),
	)
}

// ComputeFingerprint reads a file and computes its fingerprint (size, mtime, CRC32).
func ComputeFingerprint(path string) (*LibraryFingerprint, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat library file: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open library file: %w", err)
	}
	defer f.Close()

	hasher := crc32.NewIEEE()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, fmt.Errorf("failed to compute CRC32: %w", err)
	}

	return &LibraryFingerprint{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		CRC32:   hasher.Sum32(),
	}, nil
}

// Matches returns true if two fingerprints represent the same file state.
// Compares size and CRC32 (mtime can drift on some filesystems).
func (fp *LibraryFingerprint) Matches(other *LibraryFingerprint) bool {
	if fp == nil || other == nil {
		return false
	}
	return fp.Size == other.Size && fp.CRC32 == other.CRC32
}
