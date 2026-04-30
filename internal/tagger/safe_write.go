// file: internal/tagger/safe_write.go
// version: 1.0.0
// guid: 4a7e1c3b-9f02-4d85-b8e6-2f5a0d3c7b91
//
// WriteTagsSafe / WriteImageSafe — pre-flight guard for all taglib writes.
//
// If the target file lives under a Deluge-managed (protected) path, it is
// first copied into the library via the LibraryImporter before the tag write
// proceeds. This ensures we never modify a file that is still being seeded by
// Deluge, preserving seeding integrity while allowing the organizer to enrich
// metadata.
//
// Falls back to a plain taglib call when no deps are provided or when the
// path is not protected.

package tagger

import (
	"context"
	"fmt"
	"log"

	taglib "go.senan.xyz/taglib"
)

// PathChecker reports whether a filesystem path is Deluge-protected.
// Satisfied by *deluge.ProtectedPathCache.
type PathChecker interface {
	IsProtected(filePath string) bool
}

// LibraryImporter copies a protected file into the library root and returns
// the new path. Implementations are expected to be idempotent.
//
// The context is passed through for cancellation; a nil context is treated as
// context.Background() by well-behaved implementations.
type LibraryImporter interface {
	// ImportPath resolves src to a library path, copying if necessary.
	// Returns the effective (possibly new) path to write to.
	ImportPath(ctx context.Context, srcPath string) (libraryPath string, err error)
}

// SafeWriteDeps bundles the optional dependencies needed for the pre-flight
// guard. All fields are optional; nil values disable the guard for that
// concern (the write proceeds directly against the original path).
type SafeWriteDeps struct {
	// ProtectedCache checks whether a path is under a Deluge save_path prefix.
	// If nil, no protection check is performed.
	ProtectedCache PathChecker

	// Importer copies a file from a protected path into the library.
	// If nil (and ProtectedCache is set), a protected-path hit is logged but
	// the write still proceeds in-place — callers should always wire both.
	Importer LibraryImporter
}

// WriteTagsSafe writes tags to path, importing first if the path is protected.
//
// opts is the taglib write option (0 = merge, taglib.Clear = replace-all).
// deps may be zero-value; in that case this is equivalent to a plain
// taglib.WriteTags call.
func WriteTagsSafe(ctx context.Context, path string, tags map[string][]string, opts taglib.WriteOption, deps SafeWriteDeps) error {
	if ctx == nil {
		ctx = context.Background()
	}

	effectivePath, err := resolvePath(ctx, path, deps)
	if err != nil {
		return fmt.Errorf("WriteTagsSafe: resolve path: %w", err)
	}

	if err := taglib.WriteTags(effectivePath, tags, opts); err != nil {
		return fmt.Errorf("WriteTagsSafe: taglib.WriteTags %s: %w", effectivePath, err)
	}
	return nil
}

// WriteImageSafe embeds cover art into the audio file at path, importing first
// if the path is protected.
//
// data is the raw image bytes (JPEG, PNG, etc.).
// deps may be zero-value; in that case this is equivalent to a plain
// taglib.WriteImage call.
func WriteImageSafe(ctx context.Context, path string, data []byte, deps SafeWriteDeps) error {
	if ctx == nil {
		ctx = context.Background()
	}

	effectivePath, err := resolvePath(ctx, path, deps)
	if err != nil {
		return fmt.Errorf("WriteImageSafe: resolve path: %w", err)
	}

	if err := taglib.WriteImage(effectivePath, data); err != nil {
		return fmt.Errorf("WriteImageSafe: taglib.WriteImage %s: %w", effectivePath, err)
	}
	return nil
}

// ResolvePathForWrite returns the path to actually write to.
// If the path is protected and an Importer is configured, it imports the file
// and returns the new library path. Otherwise it returns path unchanged.
//
// Exported so that build-variant packages (e.g. metadata/taglib_cgo.go) can
// call the same resolution logic without duplicating it.
func ResolvePathForWrite(ctx context.Context, path string, deps SafeWriteDeps) (string, error) {
	return resolvePath(ctx, path, deps)
}

// resolvePath is the internal implementation used by WriteTagsSafe, WriteImageSafe,
// and ResolvePathForWrite.
func resolvePath(ctx context.Context, path string, deps SafeWriteDeps) (string, error) {
	if deps.ProtectedCache == nil {
		return path, nil
	}

	if !deps.ProtectedCache.IsProtected(path) {
		return path, nil
	}

	// Path is protected.
	log.Printf("[INFO] safe_write: path %s is in a protected Deluge directory; importing to library before tag write", path)

	if deps.Importer == nil {
		// Guard is incomplete — log a warning and proceed in-place rather than
		// failing silently or corrupting a torrent file. This should not happen
		// in production (both fields must be wired together).
		log.Printf("[WARN] safe_write: LibraryImporter is nil for protected path %s; writing in-place (may affect seeding)", path)
		return path, nil
	}

	libraryPath, err := deps.Importer.ImportPath(ctx, path)
	if err != nil {
		return "", fmt.Errorf("import protected path %s: %w", path, err)
	}

	log.Printf("[INFO] safe_write: imported %s → %s before tag write", path, libraryPath)
	return libraryPath, nil
}
