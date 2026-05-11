// file: internal/server/scheduler_extra_ops.go
// version: 1.0.0
// guid: f1e2d3c4-b5a6-7890-fedc-ba9876543210

// scheduler_extra_ops registers OperationDefs for the 13 remaining scheduler
// tasks that previously used the legacy triggerOperation / triggerOperationWithID
// helpers. Each def extracts the closure logic from scheduler_tasks.go into a
// typed Run func so the UOS v2 dispatcher can manage lifecycle, cancellation,
// and concurrency without the old BridgeQueue.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
)

// schedulerExtraOpParams carries the v1 operation ID from the TriggerFn into
// the Run func, enabling the Run func to write results back to the v1 record.
type schedulerExtraOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

// --- dedup-llm-review ---

// RegisterDedupLLMReviewOp registers the scheduler.dedup-llm-review OperationDef.
func (s *Server) RegisterDedupLLMReviewOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.dedup-llm-review",
		Plugin:          "scheduler",
		DisplayName:     "Dedup LLM Review",
		Description:     "Run LLM review on ambiguous dedup candidates.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.dedup-llm-review",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			if s.dedupEngine == nil {
				_ = progress.Log("info", "Dedup engine not initialized, skipping LLM review", nil)
				return nil
			}
			_ = progress.Log("info", "Starting LLM review of ambiguous dedup candidates", nil)
			return s.dedupEngine.RunLLMReview(ctx)
		},
	})
}

// --- trash-cleanup ---

// RegisterTrashCleanupOp registers the scheduler.trash-cleanup OperationDef.
func (s *Server) RegisterTrashCleanupOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.trash-cleanup",
		Plugin:          "scheduler",
		DisplayName:     "Trash Cleanup",
		Description:     "Purge trashed book versions past their 14-day TTL.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.trash-cleanup",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			purged := CleanupTrashedVersions(s.Store())
			_ = progress.Log("info", fmt.Sprintf("Trash cleanup: purged %d versions", purged), nil)
			return nil
		},
	})
}

// --- archive-sweep ---

// RegisterArchiveSweepOp registers the scheduler.archive-sweep OperationDef.
func (s *Server) RegisterArchiveSweepOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.archive-sweep",
		Plugin:          "scheduler",
		DisplayName:     "Archive Sweep",
		Description:     "Remove soft-deleted books past the 30-day retention window.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.archive-sweep",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			cleaned := SweepArchivedBooks(s.Store())
			_ = progress.Log("info", fmt.Sprintf("Archive sweep: cleaned %d books", cleaned), nil)
			return nil
		},
	})
}

// --- metadata-upgrade ---

// RegisterMetadataUpgradeOp registers the scheduler.metadata-upgrade OperationDef.
func (s *Server) RegisterMetadataUpgradeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.metadata-upgrade",
		Plugin:          "scheduler",
		DisplayName:     "Metadata Upgrade",
		Description:     "Upgrade metadata from lower-quality sources to richer ones when a high-confidence match is available.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.metadata-upgrade",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapNetworkOpenAI},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			if s.metadataFetchService == nil {
				return fmt.Errorf("metadata fetch service not initialized")
			}
			svc := NewMetadataUpgradeService(s.Store(), s.metadataFetchService)
			_ = progress.Log("info", "Scanning for books with upgradeable metadata sources...", nil)
			result, err := svc.RunUpgrade(ctx, 200)
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Metadata upgrade complete: checked %d, upgraded %d, skipped %d, errors %d",
				result.Checked, result.Upgraded, result.Skipped, result.Errors)
			_ = progress.Log("info", msg, nil)
			_ = progress.UpdateProgress(100, 100, msg)
			return nil
		},
	})
}

// --- author-split-scan ---

// RegisterAuthorSplitScanOp registers the scheduler.author-split-scan OperationDef.
func (s *Server) RegisterAuthorSplitScanOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.author-split-scan",
		Plugin:          "scheduler",
		DisplayName:     "Author Split Scan",
		Description:     "Find and split composite author names into individual author records.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.author-split-scan",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("database not initialized")
			}
			_ = progress.Log("info", "Starting author split scan", nil)
			authors, err := store.GetAllAuthors()
			if err != nil {
				return fmt.Errorf("failed to get authors: %w", err)
			}
			_ = progress.Log("info", fmt.Sprintf("Scanning %d authors for composite names...", len(authors)), nil)

			splitCount := 0
			booksUpdated := 0
			errCount := 0
			total := len(authors)

			for i, author := range authors {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				parts := dedup.SplitCompositeAuthorName(author.Name)
				if len(parts) <= 1 {
					if (i+1)%200 == 0 {
						_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("Checked %d/%d authors", i+1, total))
					}
					continue
				}

				// Actually split: create/find individual authors
				var newAuthors []database.Author
				for _, name := range parts {
					name = strings.TrimSpace(name)
					if name == "" {
						continue
					}
					existing, err := store.GetAuthorByName(name)
					if err == nil && existing != nil {
						newAuthors = append(newAuthors, *existing)
						continue
					}
					created, err := store.CreateAuthor(name)
					if err != nil {
						errCount++
						_ = progress.Log("warning", fmt.Sprintf("Failed to create author %q: %v", name, err), nil)
						continue
					}
					newAuthors = append(newAuthors, *created)
				}
				if len(newAuthors) == 0 {
					continue
				}

				// Re-link all books from composite author to individual authors
				books, err := store.GetBooksByAuthorIDWithRole(author.ID)
				if err != nil {
					errCount++
					_ = progress.Log("warning", fmt.Sprintf("Failed to get books for author %q: %v", author.Name, err), nil)
					continue
				}

				for _, book := range books {
					bookAuthors, err := store.GetBookAuthors(book.ID)
					if err != nil {
						continue
					}
					role := "author"
					for _, ba := range bookAuthors {
						if ba.AuthorID == author.ID {
							role = ba.Role
							break
						}
					}
					var updated []database.BookAuthor
					for _, ba := range bookAuthors {
						if ba.AuthorID != author.ID {
							updated = append(updated, ba)
						}
					}
					for _, na := range newAuthors {
						alreadyLinked := false
						for _, ba := range updated {
							if ba.AuthorID == na.ID {
								alreadyLinked = true
								break
							}
						}
						if !alreadyLinked {
							updated = append(updated, database.BookAuthor{
								BookID:   book.ID,
								AuthorID: na.ID,
								Role:     role,
								Position: len(updated),
							})
						}
					}
					if err := store.SetBookAuthors(book.ID, updated); err != nil {
						errCount++
						continue
					}
					// Update primary AuthorID to first individual author
					if book.AuthorID != nil && *book.AuthorID == author.ID && len(newAuthors) > 0 {
						firstID := newAuthors[0].ID
						book.AuthorID = &firstID
						_, _ = store.UpdateBook(book.ID, &book)
					}
					booksUpdated++
				}

				// Delete the composite author record
				if err := store.DeleteAuthor(author.ID); err != nil {
					_ = progress.Log("warning", fmt.Sprintf("Failed to delete composite author %q: %v", author.Name, err), nil)
					errCount++
				} else {
					splitCount++
					partNames := fmt.Sprintf("%v", parts)
					_ = progress.Log("info", fmt.Sprintf("Split %q → %s (%d books updated)", author.Name, partNames, len(books)), nil)
				}

				if (i+1)%200 == 0 || splitCount%50 == 0 {
					_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("Checked %d/%d authors, split %d so far", i+1, total, splitCount))
				}
			}

			// Invalidate dedup cache since authors changed
			if s.dedupCache != nil {
				s.dedupCache.Invalidate("author-duplicates")
			}

			resultMsg := fmt.Sprintf("Split %d composite authors, updated %d books (%d errors)", splitCount, booksUpdated, errCount)
			_ = progress.Log("info", resultMsg, nil)
			_ = progress.UpdateProgress(total, total, resultMsg)
			return nil
		},
	})
}

// --- db-optimize ---

// RegisterDBOptimizeOp registers the scheduler.db-optimize OperationDef.
func (s *Server) RegisterDBOptimizeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.db-optimize",
		Plugin:          "scheduler",
		DisplayName:     "Database Optimize",
		Description:     "Optimize database (VACUUM/compact) for the main, AI scan, and OpenLibrary stores.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.db-optimize",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("database not initialized")
			}

			storesOptimized := 0
			storesTotal := 3
			startTotal := time.Now()

			// 1. Main store
			_ = progress.Log("info", "Optimizing main database (VACUUM, ANALYZE, WAL checkpoint)...", nil)
			_ = progress.UpdateProgress(0, storesTotal, "Optimizing main database...")
			t1 := time.Now()
			if err := store.Optimize(); err != nil {
				_ = progress.Log("error", fmt.Sprintf("Main DB optimization failed: %v", err), nil)
			} else {
				storesOptimized++
				_ = progress.Log("info", fmt.Sprintf("Main database optimized in %s", time.Since(t1).Round(time.Millisecond)), nil)
			}

			// 2. AI scan store
			_ = progress.UpdateProgress(1, storesTotal, "Optimizing AI scan database...")
			if s.aiScanStore != nil {
				t2 := time.Now()
				if err := s.aiScanStore.Optimize(); err != nil {
					_ = progress.Log("error", fmt.Sprintf("AI scan DB optimization failed: %v", err), nil)
				} else {
					storesOptimized++
					_ = progress.Log("info", fmt.Sprintf("AI scan database optimized in %s", time.Since(t2).Round(time.Millisecond)), nil)
				}
			} else {
				_ = progress.Log("info", "AI scan store not initialized, skipping", nil)
			}

			// 3. OpenLibrary store (accessed via olService)
			_ = progress.UpdateProgress(2, storesTotal, "Optimizing OpenLibrary cache...")
			if s.olService != nil && s.olService.Store() != nil {
				t3 := time.Now()
				if err := s.olService.Store().Optimize(); err != nil {
					_ = progress.Log("error", fmt.Sprintf("OL cache optimization failed: %v", err), nil)
				} else {
					storesOptimized++
					_ = progress.Log("info", fmt.Sprintf("OpenLibrary cache optimized in %s", time.Since(t3).Round(time.Millisecond)), nil)
				}
			} else {
				_ = progress.Log("info", "OpenLibrary store not initialized, skipping", nil)
			}

			_ = progress.UpdateProgress(storesTotal, storesTotal,
				fmt.Sprintf("Database optimization complete: %d/%d stores in %s",
					storesOptimized, storesTotal, time.Since(startTotal).Round(time.Millisecond)))
			return nil
		},
	})
}

// --- cleanup-old-backups ---

// RegisterCleanupOldBackupsOp registers the scheduler.cleanup-old-backups OperationDef.
func (s *Server) RegisterCleanupOldBackupsOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.cleanup-old-backups",
		Plugin:          "scheduler",
		DisplayName:     "Cleanup Old Backups",
		Description:     "Remove old .bak-* backup files past the configured retention period.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.cleanup-old-backups",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapFilesWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			rootDir := config.AppConfig.RootDir
			if rootDir == "" {
				_ = progress.Log("info", "No root directory configured, skipping backup cleanup", nil)
				return nil
			}
			retentionDays := config.AppConfig.PurgeSoftDeletedAfterDays
			if retentionDays <= 0 {
				retentionDays = 30
			}
			maxAge := time.Duration(retentionDays) * 24 * time.Hour
			removed := 0
			_ = progress.Log("info", fmt.Sprintf("Scanning %s for .bak-* files older than %d days", rootDir, retentionDays), nil)

			err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil || info.IsDir() {
					return nil
				}
				if strings.Contains(info.Name(), ".bak-") {
					age := time.Since(info.ModTime())
					if age > maxAge {
						if rmErr := os.Remove(path); rmErr != nil {
							log.Printf("[WARN] failed to remove old backup: %s: %v", path, rmErr)
						} else {
							removed++
							log.Printf("[INFO] cleaned up old backup: %s (age: %s)", path, age.Round(time.Hour))
						}
					}
				}
				return nil
			})
			_ = progress.Log("info", fmt.Sprintf("Backup cleanup complete: removed %d file(s)", removed), nil)
			return err
		},
	})
}

// --- isbn-enrichment ---

// RegisterISBNEnrichmentOp registers the scheduler.isbn-enrichment OperationDef.
func (s *Server) RegisterISBNEnrichmentOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.isbn-enrichment",
		Plugin:          "scheduler",
		DisplayName:     "ISBN Enrichment",
		Description:     "Enrich missing ISBN identifiers from external metadata sources.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.isbn-enrichment",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapNetworkOpenAI},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p schedulerExtraOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("isbn-enrichment: decode params: %w", err)
			}
			progress := registryProgressAdapter{r: reporter}
			return s.runIsbnEnrichment(ctx, progress, p.LegacyOpID)
		},
	})
}

// --- temp-file-cleanup ---

// RegisterTempFileCleanupOp registers the scheduler.temp-file-cleanup OperationDef.
func (s *Server) RegisterTempFileCleanupOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.temp-file-cleanup",
		Plugin:          "scheduler",
		DisplayName:     "Temp File Cleanup",
		Description:     "Remove orphaned *.tmp.m4b / *.tmp.m4a files left by crashed ffmpeg operations.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.temp-file-cleanup",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapFilesWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p schedulerExtraOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("temp-file-cleanup: decode params: %w", err)
			}
			progress := registryProgressAdapter{r: reporter}
			removed := cleanupOrphanedTempFiles(config.AppConfig.RootDir, s.activityWriter, p.LegacyOpID)
			activity.FlushOperation(s.activityWriter, p.LegacyOpID)
			msg := fmt.Sprintf("Removed %d orphaned temp files", removed)
			_ = progress.Log("info", msg, nil)
			activity.EmitInfo(s.activityWriter, p.LegacyOpID, "temp-file-cleanup", "temp-file-cleanup", msg,
				activity.TagsIf(removed == 0, activity.NoOpTag)...)
			return nil
		},
	})
}

// --- purge-deleted ---

// RegisterPurgeDeletedOp registers the scheduler.purge-deleted OperationDef.
func (s *Server) RegisterPurgeDeletedOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.purge-deleted",
		Plugin:          "scheduler",
		DisplayName:     "Purge Soft-Deleted",
		Description:     "Purge soft-deleted books past their retention period.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.purge-deleted",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p schedulerExtraOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("purge-deleted: decode params: %w", err)
			}
			progress := registryProgressAdapter{r: reporter}
			_ = progress.Log("info", "Starting purge of soft-deleted books", nil)
			_ = progress.UpdateProgress(0, 100, "Purging soft-deleted books...")
			s.runAutoPurgeSoftDeleted(p.LegacyOpID)
			activity.FlushOperation(s.activityWriter, p.LegacyOpID)
			_ = progress.Log("info", "Purge complete", nil)
			_ = progress.UpdateProgress(100, 100, "Purge complete")
			return nil
		},
	})
}

// --- tombstone-cleanup ---

// RegisterTombstoneCleanupOp registers the scheduler.tombstone-cleanup OperationDef.
func (s *Server) RegisterTombstoneCleanupOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.tombstone-cleanup",
		Plugin:          "scheduler",
		DisplayName:     "Tombstone Cleanup",
		Description:     "Resolve author tombstone chains (A→B→C becomes A→C).",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.tombstone-cleanup",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("database not initialized")
			}
			_ = progress.Log("info", "Starting author tombstone chain resolution", nil)
			_ = progress.UpdateProgress(0, 100, "Resolving tombstone chains...")
			updated, err := store.ResolveTombstoneChains()
			if err != nil {
				return fmt.Errorf("tombstone chain resolution failed: %w", err)
			}
			resultMsg := fmt.Sprintf("Resolved %d tombstone chains", updated)
			_ = progress.Log("info", resultMsg, nil)
			_ = progress.UpdateProgress(100, 100, resultMsg)
			return nil
		},
	})
}

// --- resolve-production-authors ---

// RegisterResolveProductionAuthorsOp registers the scheduler.resolve-production-authors OperationDef.
func (s *Server) RegisterResolveProductionAuthorsOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.resolve-production-authors",
		Plugin:          "scheduler",
		DisplayName:     "Resolve Production Authors",
		Description:     "Resolve real authors for production company entries.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.resolve-production-authors",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapNetworkOpenAI},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("database not initialized")
			}
			_ = progress.Log("info", "Starting production author resolution", nil)
			authors, err := store.GetAllAuthors()
			if err != nil {
				return fmt.Errorf("failed to get authors: %w", err)
			}

			var prodAuthors []database.Author
			for _, a := range authors {
				if dedup.IsProductionCompany(a.Name) {
					prodAuthors = append(prodAuthors, a)
				}
			}

			_ = progress.Log("info", fmt.Sprintf("Found %d production company authors", len(prodAuthors)), nil)
			total := len(prodAuthors)
			resolved := 0
			for i, author := range prodAuthors {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				books, err := store.GetBooksByAuthorIDWithRole(author.ID)
				if err != nil {
					continue
				}
				for _, book := range books {
					if s.metadataFetchService == nil {
						continue
					}
					resp, fetchErr := s.metadataFetchService.FetchMetadataForBookByTitle(book.ID)
					if fetchErr == nil && resp != nil && resp.Book != nil && resp.Book.AuthorID != nil {
						newAuthor, _ := store.GetAuthorByID(*resp.Book.AuthorID)
						if newAuthor != nil && !dedup.IsProductionCompany(newAuthor.Name) {
							resolved++
						}
					}
				}
				_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("Processed %d/%d production companies (%d books resolved)", i+1, total, resolved))
			}

			if s.dedupCache != nil {
				s.dedupCache.Invalidate("author-duplicates")
			}

			resultMsg := fmt.Sprintf("Resolved %d books across %d production companies", resolved, total)
			_ = progress.Log("info", resultMsg, nil)
			_ = progress.UpdateProgress(total, total, resultMsg)
			return nil
		},
	})
}

// --- metadata-refresh ---

// RegisterMetadataRefreshOp registers the scheduler.metadata-refresh OperationDef.
func (s *Server) RegisterMetadataRefreshOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "scheduler.metadata-refresh",
		Plugin:          "scheduler",
		DisplayName:     "Metadata Refresh",
		Description:     "Re-fetch metadata for incomplete books.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "scheduler.metadata-refresh",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapNetworkOpenAI},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			progress := registryProgressAdapter{r: reporter}
			return s.runMetadataRefreshScan(ctx, progress)
		},
	})
}

func init() {
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterDedupLLMReviewOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterTrashCleanupOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterArchiveSweepOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterMetadataUpgradeOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterAuthorSplitScanOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterDBOptimizeOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterCleanupOldBackupsOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterISBNEnrichmentOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterTempFileCleanupOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterPurgeDeletedOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterTombstoneCleanupOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterResolveProductionAuthorsOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterMetadataRefreshOp(reg) })
}
