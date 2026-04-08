// file: internal/metadata/taglib_cgo.go
// version: 1.0.0
// guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d
//
// Native CGO bindings to TagLib C API for high-performance tag writing.
// Build with: -tags native_taglib
// Requires static libtag.a, libtag_c.a, libz.a in third_party/taglib/lib/

//go:build native_taglib

package metadata

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/taglib/include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/taglib/lib -ltag_c -ltag -lz -lstdc++ -lm -lgcc -lgcc_eh
#include <stdlib.h>
#include "tag_c.h"
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/jdfalk/audiobook-organizer/internal/fileops"
)

var taglibAvailable = true

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

	// Build tag map identical to the WASM version
	tags := buildWriteTagMap(metadata)
	if len(tags) == 0 {
		return fmt.Errorf("no writable metadata supplied")
	}

	// Open file with TagLib
	cPath := C.CString(abs)
	defer C.free(unsafe.Pointer(cPath))

	file := C.taglib_file_new(cPath)
	if file == nil {
		return fmt.Errorf("taglib: failed to open %s", abs)
	}
	defer C.taglib_file_free(file)

	// taglib_file_new can return non-nil for unrecognised formats but
	// the internal File* is null, causing a SIGSEGV in property calls.
	if C.taglib_file_is_valid(file) == 0 {
		return fmt.Errorf("taglib: file not valid/supported: %s", abs)
	}

	// Write each property via the C property API
	for key, values := range tags {
		cKey := C.CString(key)
		if len(values) == 0 || (len(values) == 1 && values[0] == "") {
			// Clear this property
			C.taglib_property_set(file, cKey, nil)
		} else {
			// Set first value (replaces existing)
			cVal := C.CString(values[0])
			C.taglib_property_set(file, cKey, cVal)
			C.free(unsafe.Pointer(cVal))

			// Append additional values
			for _, v := range values[1:] {
				cVal = C.CString(v)
				C.taglib_property_set_append(file, cKey, cVal)
				C.free(unsafe.Pointer(cVal))
			}
		}
		C.free(unsafe.Pointer(cKey))
	}

	if C.taglib_file_save(file) == 0 {
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("taglib save failed and restore failed: restore=%v", restoreErr)
		}
		return fmt.Errorf("taglib: save failed for %s (restored)", abs)
	}

	// Force fsync for ZFS/COW filesystems
	if f, err := os.OpenFile(abs, os.O_RDWR, 0); err == nil {
		_ = f.Sync()
		f.Close()
	}

	return nil
}
