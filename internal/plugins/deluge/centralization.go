// file: internal/plugins/deluge/centralization.go
// version: 1.0.1
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f
// last-edited: 2026-05-07

package deluge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/config"
	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) centralizationDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "deluge.centralize",
		Plugin:          "deluge",
		DisplayName:     "Centralize Deluge books",
		Description:     "Moves Deluge-sourced audiobooks from protected paths into the main library.",
		ResumePolicy:    sdk.ResumeRestart,
		DefaultPriority: sdk.PriorityNormal,
		ConcurrencyKey:  "deluge.centralize",
		Cancellable:     true,
		Isolate:         false,
		Timeout:         24 * time.Hour,
		Run:             p.runCentralization,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
			sdk.CapFilesWrite,
		},
		MinCheckpointInterval: 30 * time.Second, // checkpoint after each file
	}
}

// centralizationCheckpoint tracks state across restarts.
type centralizationCheckpoint struct {
	ProcessedFiles int    `json:"processed_files"`
	TotalFiles     int    `json:"total_files"`
	LastBookFileID string `json:"last_book_file_id"`
	LastError      string `json:"last_error,omitempty"`
}

func (p *Plugin) runCentralization(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	cfg := &config.AppConfig

	// Load checkpoint if resuming from a restart.
	var checkpoint centralizationCheckpoint
	if err := reporter.Checkpoint(nil); err == nil && params != nil {
		// Try to unmarshal checkpoint from last run
		_ = json.Unmarshal(params, &checkpoint)
	}

	sdk.NewProgress(reporter, 0).Start("Loading Deluge-imported files...")

	// Pull only BookFiles with non-empty DelugeHash AND not yet imported.
	// Centralized store method routes through the memdb sparse deluge_hash
	// index when published, avoiding the full 308K BookFile scan.
	pending, err := p.store.GetBookFilesNeedingDelugeImport()
	if err != nil {
		return fmt.Errorf("load deluge-pending book files: %w", err)
	}

	toImport := make([]*database.BookFile, 0, len(pending))
	for i := range pending {
		toImport = append(toImport, &pending[i])
	}

	total := len(toImport)
	prog := sdk.NewProgress(reporter, total)
	prog.Start(fmt.Sprintf("Centralizing %d files...", total))
	if total == 0 {
		prog.Finalize("Writing results...")
		prog.Done("No files to centralize")
		return nil
	}

	if checkpoint.ProcessedFiles == 0 {
		checkpoint.TotalFiles = total
	}

	var successCount, skipCount, errCount int
	lastProcessedIdx := checkpoint.ProcessedFiles

	for i, bf := range toImport {
		if reporter.IsCanceled() {
			return context.Canceled
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip already processed files on restart.
		if i < lastProcessedIdx {
			continue
		}

		srcPath := bf.FilePath
		if srcPath == "" {
			skipCount++
			prog.StepN(i+1, fmt.Sprintf("Skipped %d/%d files with no path", skipCount, total))
			continue
		}

		// Determine destination directory.
		var destDir string
		rel, relErr := filepath.Rel(cfg.RootDir, filepath.Dir(srcPath))
		if relErr == nil && !filepath.IsAbs(rel) && !isParentTraversal(rel) {
			// Source is under RootDir — preserve structure.
			destDir = filepath.Join(cfg.RootDir, rel)
		} else {
			// Source is outside RootDir — place directly under RootDir.
			destDir = cfg.RootDir
		}

		dest := filepath.Join(destDir, filepath.Base(srcPath))

		// Skip if already at destination.
		if srcPath == dest {
			skipCount++
			prog.StepN(i+1, fmt.Sprintf("Skipped %d/%d files already in library", skipCount, total))
			continue
		}

		// Copy the file.
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			errCount++
			checkpoint.LastError = fmt.Sprintf("mkdir %s: %v", destDir, err)
			reporter.Logger().Error("mkdir failed", "path", destDir, "error", err)
			prog.StepN(i+1, fmt.Sprintf("Error (%d/%d): %s", i+1, total, checkpoint.LastError))
			continue
		}

		// Use reflink if available, fall back to copy.
		if err := reflinkCopy(srcPath, dest); err != nil {
			if err := ioCopy(srcPath, dest); err != nil {
				errCount++
				checkpoint.LastError = fmt.Sprintf("copy %s: %v", srcPath, err)
				reporter.Logger().Error("copy failed", "src", srcPath, "dest", dest, "error", err)
				prog.StepN(i+1, fmt.Sprintf("Error (%d/%d): %s", i+1, total, checkpoint.LastError))
				continue
			}
		}

		// Update the BookFile record.
		now := time.Now()
		bf.DelugeOriginalPath = srcPath
		bf.FilePath = dest
		bf.ImportedFromDelugeAt = &now

		if err := p.store.UpdateBookFile(bf.ID, bf); err != nil {
			errCount++
			checkpoint.LastError = fmt.Sprintf("update book file: %v", err)
			reporter.Logger().Error("update book file failed", "id", bf.ID, "error", err)
			prog.StepN(i+1, fmt.Sprintf("Error (%d/%d): %s", i+1, total, checkpoint.LastError))
			continue
		}

		// Update Deluge storage path.
		if cfg.DelugeMoveEnabled && bf.DelugeHash != "" && p.client != nil {
			moveErr := p.client.MoveStorage([]string{bf.DelugeHash}, filepath.Dir(dest))
			if moveErr != nil {
				reporter.Logger().Warn("deluge move_storage failed", "hash", bf.DelugeHash, "error", moveErr)
				// Non-fatal: continue processing.
			} else {
				slog.Info("deluge move_storage succeeded", "hash", bf.DelugeHash, "dir", filepath.Dir(dest))
			}
		}

		successCount++
		checkpoint.ProcessedFiles = i + 1

		// Checkpoint after each file to support fine-grained restart.
		_ = reporter.Checkpoint(checkpoint)

		prog.StepN(i+1, fmt.Sprintf("Centralized %d/%d files", successCount, total))
	}

	prog.Finalize("Writing results...")
	prog.Done(fmt.Sprintf("Done: %d succeeded, %d skipped, %d errors", successCount, skipCount, errCount))
	return nil
}

// Helper functions copied from deluge_import.go
func isParentTraversal(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == "../"
}

// reflinkCopy attempts a reflink (copy-on-write) copy.
// Falls back to normal copy on error.
func reflinkCopy(src, dest string) error {
	// This would use platform-specific system calls.
	// For now, this is a placeholder that returns an error to force fallback.
	return fmt.Errorf("reflink not available")
}

// ioCopy copies a file using standard I/O.
func ioCopy(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer destFile.Close()

	_, err = ioCopyWithBuffer(destFile, srcFile)
	return err
}

// ioCopyWithBuffer copies from src to dst with a buffer.
func ioCopyWithBuffer(dst, src *os.File) (written int64, err error) {
	// Simple buffer copy.
	buf := make([]byte, 32*1024)
	return ioCopyBuffer(dst, src, buf)
}

// ioCopyBuffer copies with a provided buffer.
func ioCopyBuffer(dst, src *os.File, buf []byte) (written int64, err error) {
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			nw, err := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
			}
			written += int64(nw)
			if err != nil {
				return written, err
			}
			if nr != nw {
				return written, fmt.Errorf("short write")
			}
		}
		if err != nil {
			// io.EOF is expected at EOF
			if err.Error() == "EOF" {
				return written, nil
			}
			return written, err
		}
	}
}
