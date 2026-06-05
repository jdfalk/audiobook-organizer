// file: internal/server/batch_save_op.go
// version: 1.0.0
// guid: 3f2a1b4c-5d6e-7f8a-9b0c-1d2e3f4a5b6c
// last-edited: 2026-05-10
//
// batch_save_op registers the "metadata.batch-save" v2 OperationDef.
// The HTTP handler batchWriteBackAudiobooks creates a v1 op record for
// backward-compatible polling, then enqueues the run here.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/auth"
	"github.com/falkcorp/audiobook-organizer/internal/config"
	"github.com/falkcorp/audiobook-organizer/internal/logger"
	opsregistry "github.com/falkcorp/audiobook-organizer/internal/operations/registry"
	"github.com/falkcorp/audiobook-organizer/internal/organizer"
)

// batchSaveOpParams is the JSON params for the metadata.batch-save op.
type batchSaveOpParams struct {
	LegacyOpID string   `json:"legacy_op_id"`
	BookIDs    []string `json:"book_ids"`
	Organize   bool     `json:"organize"`
	Force      bool     `json:"force"`
}

// RegisterBatchSaveToFilesOp registers the "metadata.batch-save" v2 OperationDef.
// The HTTP handler batchWriteBackAudiobooks pre-creates a v1 op record for
// backwards-compatible progress polling, then enqueues here via opRegistry.
func (s *Server) RegisterBatchSaveToFilesOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "metadata.batch-save",
		Plugin:          "metadata",
		DisplayName:     "Batch Save to Files",
		Description:     "Write metadata from database back to audio file tags for a set of books, with optional re-organize.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "metadata.batch-save",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapFilesWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p batchSaveOpParams
			if len(rawParams) > 0 {
				if err := json.Unmarshal(rawParams, &p); err != nil {
					return fmt.Errorf("batch-save: decode params: %w", err)
				}
			}

			store := s.Store()
			progress := registryProgressAdapter{r: reporter}
			opID := p.LegacyOpID
			bookIDs := p.BookIDs
			totalBooks := len(bookIDs)

			_ = progress.UpdateProgress(0, totalBooks, "starting save to files")

			written, organized, failed, skipped := 0, 0, 0, 0
			org := organizer.NewOrganizer(&config.AppConfig)
			log2 := logger.NewWithActivityLog("batch-write-back", store)

			for i, id := range bookIDs {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				book, err := store.GetBookByID(id)
				if err != nil || book == nil {
					failed++
					_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("book %s not found", id), nil)
					continue
				}

				// Skip if already written and metadata hasn't changed since last write
				if !p.Force && book.LastWrittenAt != nil && !book.UpdatedAt.After(*book.LastWrittenAt) {
					skipped++
					_ = progress.UpdateProgress(i+1, totalBooks,
						fmt.Sprintf("processed %d/%d (skipped: %d — already up to date)", i+1, totalBooks, skipped))
					continue
				}

				// Write tags
				_, wbErr := s.metadataFetchService.WriteBackMetadataForBook(id)
				if wbErr != nil {
					failed++
					detail := wbErr.Error()
					_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("write-back failed for %s", book.Title), &detail)
					continue
				}
				written++
				// Stamp last_written_at on the book the user sees (may differ from library copy)
				_ = store.SetLastWrittenAt(id, time.Now())

				// Organize
				if p.Organize {
					book, _ = store.GetBookByID(id)
					if book != nil {
						oldPath := book.FilePath
						alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)
						var newPath string
						var orgErr error
						if alreadyInRoot {
							newPath, orgErr = s.organizeService.ReOrganizeInPlace(book, log2)
						} else {
							bookFiles, _ := store.GetBookFiles(id)
							isDir := len(bookFiles) > 1
							if !isDir {
								if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
									isDir = true
								}
							}
							if isDir {
								newPath, orgErr = s.organizeService.OrganizeDirectoryBook(org, book, log2)
							} else {
								newPath, _, orgErr = org.OrganizeBook(book)
							}
						}
						if orgErr != nil {
							detail := orgErr.Error()
							_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("organize failed for %s", book.Title), &detail)
						} else if newPath != "" && newPath != oldPath {
							organized++
						}
					}
				}

				// Enqueue ITL write-back
				if s.writeBackBatcher != nil {
					s.writeBackBatcher.Enqueue(id)
				}

				_ = progress.UpdateProgress(i+1, totalBooks,
					fmt.Sprintf("processed %d/%d (written: %d, organized: %d, failed: %d)",
						i+1, totalBooks, written, organized, failed))
			}

			_ = progress.UpdateProgress(totalBooks, totalBooks,
				fmt.Sprintf("complete: written %d, organized %d, skipped %d, failed %d", written, organized, skipped, failed))
			return nil
		},
	})
}

func init() {
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterBatchSaveToFilesOp(reg) })
}
