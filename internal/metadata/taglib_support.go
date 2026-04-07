// file: internal/metadata/taglib_support.go
// version: 2.0.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f
//
// TagLib WASM writer (default, no CGO required).
// For native CGO performance, build with -tags native_taglib.

//go:build !native_taglib

package metadata

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	taglib "go.senan.xyz/taglib"
)

// taglibAvailable indicates native taglib path compiled in
var taglibAvailable = true

// writeMetadataWithTaglib performs metadata writing using TagLib via WASM.
func writeMetadataWithTaglib(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	backupPath := filePath + ".backup"
	if err := fileops.SafeCopy(filePath, backupPath, config); err != nil {
		return fmt.Errorf("taglib backup failed: %w", err)
	}
	defer func() {
		if !config.PreserveOriginal {
			_ = os.Remove(backupPath)
		}
	}()

	abs, _ := filepath.Abs(filePath)

	tags := buildWriteTagMap(metadata)
	if len(tags) == 0 {
		return fmt.Errorf("no writable metadata supplied")
	}

	if err := taglib.WriteTags(abs, tags, 0); err != nil {
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("taglib write failed and restore failed: write=%w restore=%v", err, restoreErr)
		}
		return fmt.Errorf("taglib write failed (restored): %w", err)
	}

	// Force fsync to ensure ZFS/COW filesystems flush all data.
	if f, err := os.OpenFile(abs, os.O_RDWR, 0); err == nil {
		_ = f.Sync()
		f.Close()
	}

	return nil
}
