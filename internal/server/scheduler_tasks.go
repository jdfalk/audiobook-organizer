// file: internal/server/scheduler_tasks.go
// version: 1.2.0
// guid: 4ed1afbd-7c63-487a-9a53-3b1b05eb06ee
// last-edited: 2026-05-07

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
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/reconcile"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
)

func (ts *TaskScheduler) registerAllTasks() {
	s := ts.server

	// --- Library tasks ---

	ts.registerTask(TaskDefinition{
		Name:        "library_scan",
		Description: "Scan library for new/changed audiobooks (incremental by default, use force_update for full rescan)",
		Category:    "library",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("scan", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				if s.scanService == nil {
					return fmt.Errorf("scan service not initialized")
				}
				return s.scanService.PerformScan(ctx, &scanner.ScanRequest{}, operations.LoggerFromReporter(progress))
			})
		},
		IsEnabled:              func() bool { return config.AppConfig.ScanOnStartup }, // reuse existing field
		GetInterval:            func() time.Duration { return 0 },                     // manual only by default
		RunOnStart:             func() bool { return config.AppConfig.ScanOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowLibraryScan },
	})

	ts.registerTask(TaskDefinition{
		Name:        "library_organize",
		Description: "Organize audiobooks into folder structure",
		Category:    "library",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("organize", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				if s.organizeService == nil {
					return fmt.Errorf("organize service not initialized")
				}
				return s.organizeService.PerformOrganize(ctx, &OrganizeRequest{}, operations.LoggerFromReporter(progress))
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowLibraryOrganize },
	})

	ts.registerTask(TaskDefinition{
		Name:        "transcode",
		Description: "Transcode audiobooks to target format",
		Category:    "library",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("transcode", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				return fmt.Errorf("transcode requires parameters — use the operations API directly")
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return false },
	})

	// --- Sync tasks ---
	// iTunes sync and import are now registered via UOS plugin (UOS-10)

	// --- Maintenance tasks ---

	ts.registerTask(TaskDefinition{
		Name:        "dedup_refresh",
		Description: "Refresh author & series dedup cache",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperationWithID("author-dedup-scan", source, func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
				store := ts.server.Store()
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				startMsg := fmt.Sprintf("Starting author dedup scan")
				_ = progress.Log("info", startMsg, nil)
				_ = progress.UpdateProgress(0, 100, "Fetching authors...")
				if operations.IsManual(ctx) {
					activity.EmitInfo(ts.server.activityWriter, opID, "author-dedup-scan", "dedup-refresh", startMsg, activity.AlwaysShow)
				}
				authors, err := store.GetAllAuthors()
				if err != nil {
					return fmt.Errorf("failed to get authors: %w", err)
				}

				bookCounts, _ := store.GetAllAuthorBookCounts()
				booksWithCounts := 0
				for _, cnt := range bookCounts {
					if cnt > 0 {
						booksWithCounts++
					}
				}
				msg := fmt.Sprintf("Fetched %d authors (%d with book counts), running duplicate comparison...", len(authors), booksWithCounts)
				_ = progress.Log("info", msg, nil)
				_ = progress.UpdateProgress(25, 100, msg)
				if operations.IsManual(ctx) {
					activity.EmitInfo(ts.server.activityWriter, opID, "author-dedup-scan", "dedup-refresh", msg, activity.AlwaysShow)
				}

				total := len(authors)
				groups := dedup.FindDuplicateAuthors(authors, 0.85, func(id int) int { return bookCounts[id] },
					func(current, t int, message string) {
						pct := 25 + (70 * current / total)
						_ = progress.UpdateProgress(pct, 100, message)
						if current%500 == 0 {
							_ = progress.Log("debug", message, nil)
						}
					},
				)

				resultMsg := fmt.Sprintf("Dedup scan complete: %d duplicate groups found across %d authors", len(groups), total)
				_ = progress.Log("info", resultMsg, nil)
				_ = progress.UpdateProgress(100, 100, resultMsg)
				tags := activity.TagsIf(len(groups) == 0, activity.NoOpTag)
				if operations.IsManual(ctx) {
					tags = append(tags, activity.AlwaysShow)
				}
				activity.EmitInfo(ts.server.activityWriter, opID, "author-dedup-scan", "dedup-refresh", resultMsg, tags...)
				return nil
			})
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledDedupRefreshEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledDedupRefreshInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledDedupRefreshOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowDedupRefresh },
	})

	ts.registerTask(TaskDefinition{
		Name:        "dedup_llm_review",
		Description: "Run LLM review on ambiguous dedup candidates",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("dedup-llm-review", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				if ts.server.dedupEngine == nil {
					_ = progress.Log("info", "Dedup engine not initialized, skipping LLM review", nil)
					return nil
				}
				_ = progress.Log("info", "Starting LLM review of ambiguous dedup candidates", nil)
				return ts.server.dedupEngine.RunLLMReview(ctx)
			})
		},
		IsEnabled:              func() bool { return ts.server.dedupEngine != nil },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return true },
	})

	ts.registerTask(TaskDefinition{
		Name:        "series_prune",
		Description: "Merge duplicate series and delete orphans",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperationWithID("series-prune", source, func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
				store := ts.server.Store()
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				return s.executeSeriesPrune(ctx, store, progress, opID)
			})
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledSeriesPruneEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledSeriesPruneInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledSeriesPruneOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowSeriesPrune },
	})

	ts.registerTask(TaskDefinition{
		Name:        "series_normalize",
		Description: "Strip title/position contamination from series names and run write-back + organize for affected books",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperationWithID("series-normalize", source, func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
				store := ts.server.Store()
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				enqueueWB := func(bookID string) {
					if ts.server.writeBackBatcher != nil {
						ts.server.writeBackBatcher.Enqueue(bookID)
					}
				}
				affected, err := executeSeriesNormalizeCore(ctx, store, enqueueWB)
				msg := fmt.Sprintf("Series normalize complete: %d series affected, %d books enqueued for write-back",
					len(affected), len(affected))
				_ = progress.Log("info", msg, nil)
				tags := activity.TagsIf(len(affected) == 0, activity.NoOpTag)
				if operations.IsManual(ctx) {
					tags = append(tags, activity.AlwaysShow)
				}
				activity.EmitInfo(ts.server.activityWriter, opID, "series-normalize", "series-normalize", msg, tags...)
				return err
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return false },
	})

	ts.registerTask(TaskDefinition{
		Name:        "isbn_enrichment",
		Description: "Enrich missing ISBN identifiers from external metadata sources",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperationWithID("isbn-enrichment", source, s.runIsbnEnrichment)
		},
		IsEnabled:              func() bool { return s.metadataFetchService != nil && s.metadataFetchService.ISBNEnrichment() != nil },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return true },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowMetadataRefresh },
	})

	// iTunes position sync is now registered via UOS plugin (UOS-10)

	ts.registerTask(TaskDefinition{
		Name:        "temp_file_cleanup",
		Description: "Remove orphaned *.tmp.m4b / *.tmp.m4a files left by crashed ffmpeg operations",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperationWithID("temp-file-cleanup", source, func(_ context.Context, progress operations.ProgressReporter, opID string) error {
				removed := cleanupOrphanedTempFiles(config.AppConfig.RootDir, ts.server.activityWriter, opID)
				activity.FlushOperation(ts.server.activityWriter, opID)
				msg := fmt.Sprintf("Removed %d orphaned temp files", removed)
				_ = progress.Log("info", msg, nil)
				activity.EmitInfo(ts.server.activityWriter, opID, "temp-file-cleanup", "temp-file-cleanup", msg,
					activity.TagsIf(removed == 0, activity.NoOpTag)...)
				return nil
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return true },
		RunInMaintenanceWindow: func() bool { return true },
	})

	ts.registerTask(TaskDefinition{
		Name:        "trash_cleanup",
		Description: "Purge trashed book versions past their 14-day TTL",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("trash-cleanup", source, func(_ context.Context, progress operations.ProgressReporter) error {
				purged := CleanupTrashedVersions(s.Store())
				_ = progress.Log("info", fmt.Sprintf("Trash cleanup: purged %d versions", purged), nil)
				return nil
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return true },
	})

	ts.registerTask(TaskDefinition{
		Name:        "archive_sweep",
		Description: "Remove soft-deleted books past the 30-day retention window",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("archive-sweep", source, func(_ context.Context, progress operations.ProgressReporter) error {
				cleaned := SweepArchivedBooks(s.Store())
				_ = progress.Log("info", fmt.Sprintf("Archive sweep: cleaned %d books", cleaned), nil)
				return nil
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return true },
	})

	ts.registerTask(TaskDefinition{
		Name:        "metadata_upgrade",
		Description: "Upgrade metadata from lower-quality sources (Google Books, Wikipedia) to richer ones (Hardcover, Audible) when a high-confidence match is available",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("metadata-upgrade", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				if s.metadataFetchService == nil {
					return fmt.Errorf("metadata fetch service not initialized")
				}
				svc := NewMetadataUpgradeService(ts.server.Store(), s.metadataFetchService)
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
			})
		},
		IsEnabled:              func() bool { return s.metadataFetchService != nil },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowMetadataRefresh },
	})

	ts.registerTask(TaskDefinition{
		Name:        "author_split_scan",
		Description: "Find & split composite author names",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("author-split-scan", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				store := ts.server.Store()
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
			})
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledAuthorSplitEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledAuthorSplitInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledAuthorSplitOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowAuthorSplit },
	})

	ts.registerTask(TaskDefinition{
		Name:        "db_optimize",
		Description: "Optimize database (VACUUM/compact)",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("db-optimize", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				store := ts.server.Store()
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

				_ = progress.UpdateProgress(storesTotal, storesTotal, fmt.Sprintf("Database optimization complete: %d/%d stores in %s", storesOptimized, storesTotal, time.Since(startTotal).Round(time.Millisecond)))
				return nil
			})
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledDbOptimizeEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledDbOptimizeInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledDbOptimizeOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowDbOptimize },
	})

	ts.registerTask(TaskDefinition{
		Name:        "cleanup_old_backups",
		Description: "Remove old .bak-* backup files past retention",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("cleanup-old-backups", source, func(ctx context.Context, progress operations.ProgressReporter) error {
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
			})
		},
		IsEnabled:              func() bool { return config.AppConfig.MaintenanceWindowDbOptimize }, // enabled when maintenance window backup cleanup is on
		GetInterval:            func() time.Duration { return 0 },                                  // manual or maintenance window only
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowDbOptimize },
	})

	ts.registerTask(TaskDefinition{
		Name:        "purge_deleted",
		Description: "Purge soft-deleted books past retention",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperationWithID("purge-deleted", source, func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
				_ = progress.Log("info", "Starting purge of soft-deleted books", nil)
				_ = progress.UpdateProgress(0, 100, "Purging soft-deleted books...")
				s.runAutoPurgeSoftDeleted(opID)
				activity.FlushOperation(ts.server.activityWriter, opID)
				_ = progress.Log("info", "Purge complete", nil)
				_ = progress.UpdateProgress(100, 100, "Purge complete")
				return nil
			})
		},
		IsEnabled: func() bool { return config.AppConfig.PurgeSoftDeletedAfterDays > 0 },
		GetInterval: func() time.Duration {
			if config.AppConfig.PurgeSoftDeletedAfterDays > 0 {
				return 6 * time.Hour
			}
			return 0
		},
		RunOnStart:             func() bool { return config.AppConfig.PurgeSoftDeletedAfterDays > 0 },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowPurgeDeleted },
	})

	ts.registerTask(TaskDefinition{
		Name:        "tombstone_cleanup",
		Description: "Resolve author tombstone chains (A→B→C becomes A→C)",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("tombstone-cleanup", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				store := ts.server.Store()
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
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 24 * time.Hour },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowTombstoneCleanup },
	})

	ts.registerTask(TaskDefinition{
		Name:        "resolve_production_authors",
		Description: "Resolve real authors for production company entries",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("resolve-production-authors", source, func(ctx context.Context, progress operations.ProgressReporter) error {
				store := ts.server.Store()
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
			})
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledResolveProductionAuthorsEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledResolveProductionAuthorsInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return false },
	})

	ts.registerTask(TaskDefinition{
		Name:        "metadata_refresh",
		Description: "Re-fetch metadata for incomplete books",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			return ts.triggerOperation("metadata-refresh", source, s.runMetadataRefreshScan)
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledMetadataRefreshEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledMetadataRefreshInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledMetadataRefreshOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowMetadataRefresh },
	})

	// Reconcile — find broken file paths and match to untracked files on disk
	ts.registerTask(TaskDefinition{
		Name:        "reconcile_scan",
		Description: "Find books with missing files and match to untracked files on disk",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			store := ts.server.Store()
			if store == nil {
				return nil, fmt.Errorf("database not initialized")
			}
			id := ulid.Make().String()
			op, err := store.CreateOperation(id, "reconcile_scan", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create operation: %w", err)
			}
			if err := ts.server.queue.Enqueue(op.ID, "reconcile_scan", operations.PriorityNormal,
				func(ctx context.Context, progress operations.ProgressReporter) error {
					reconcileLog := operations.LoggerFromReporter(progress)
					result, scanErr := reconcile.BuildReconcilePreviewWithProgress(store, reconcileLog)
					if scanErr != nil {
						return fmt.Errorf("reconcile scan failed: %w", scanErr)
					}
					resultJSON, marshalErr := json.Marshal(result)
					if marshalErr != nil {
						return fmt.Errorf("failed to marshal scan results: %w", marshalErr)
					}
					if err := store.UpdateOperationResultData(id, string(resultJSON)); err != nil {
						return fmt.Errorf("failed to store scan results: %w", err)
					}
					summary := fmt.Sprintf("Found %d broken records, %d matches, %d unmatched",
						len(result.BrokenRecords), len(result.Matches), len(result.UnmatchedBooks))
					_ = progress.Log("info", summary, nil)
					return nil
				},
			); err != nil {
				return nil, fmt.Errorf("failed to enqueue reconcile scan: %w", err)
			}
			return op, nil
		},
		IsEnabled: func() bool { return config.AppConfig.ScheduledReconcileEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledReconcileInterval
			if mins <= 0 {
				return 0
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledReconcileOnStartup },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowReconcile },
	})

	// AI Dedup Batch — uses OpenAI Batch API at 50% cost
	ts.registerTask(TaskDefinition{
		Name:        "ai_dedup_batch",
		Description: "Run AI author dedup via Batch API (50% cheaper, up to 24h)",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			store := ts.server.Store()
			if store == nil {
				return nil, fmt.Errorf("database not initialized")
			}
			if ts.server.queue == nil {
				return nil, fmt.Errorf("operation queue not initialized")
			}
			opID := ulid.Make().String()
			op, err := store.CreateOperation(opID, "ai-dedup-batch", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create operation: %w", err)
			}
			if err := ts.server.queue.Enqueue(op.ID, "ai-dedup-batch", operations.PriorityLow, func(ctx context.Context, progress operations.ProgressReporter) error {
				parser := ai.NewOpenAIParser(&config.AppConfig, config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
				if !parser.IsEnabled() {
					return fmt.Errorf("AI parsing is not enabled")
				}

				_ = progress.Log("info", "Building author list for batch AI dedup", nil)
				_ = progress.UpdateProgress(0, 100, "Loading authors...")

				allAuthors, err := store.GetAllAuthors()
				if err != nil {
					return fmt.Errorf("failed to get authors: %w", err)
				}

				var inputs []ai.AuthorDiscoveryInput
				for _, author := range allAuthors {
					var sampleTitles []string
					books, bErr := store.GetBooksByAuthorIDWithRole(author.ID)
					if bErr == nil {
						for j, b := range books {
							if j >= 3 {
								break
							}
							sampleTitles = append(sampleTitles, b.Title)
						}
					}
					inputs = append(inputs, ai.AuthorDiscoveryInput{
						ID: author.ID, Name: author.Name,
						BookCount: len(books), SampleTitles: sampleTitles,
					})
				}

				if len(inputs) == 0 {
					_ = progress.Log("info", "No authors to process", nil)
					return nil
				}

				_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Submitting %d authors to OpenAI Batch API...", len(inputs)))

				batchID, err := parser.CreateBatchAuthorDedup(ctx, inputs)
				if err != nil {
					return fmt.Errorf("failed to create batch: %w", err)
				}

				_ = progress.Log("info", fmt.Sprintf("Batch created: %s — polling for completion", batchID), nil)

				// Poll for completion (up to 24h, check every 5 min)
				pollInterval := 5 * time.Minute
				maxPolls := 288 // 24h / 5min
				for i := 0; i < maxPolls; i++ {
					if progress.IsCanceled() {
						return fmt.Errorf("cancelled")
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(pollInterval):
					}

					status, outputFileID, sErr := parser.CheckBatchStatus(ctx, batchID)
					if sErr != nil {
						_ = progress.Log("warn", fmt.Sprintf("Poll error: %v", sErr), nil)
						continue
					}

					_ = progress.UpdateProgress(10+i, maxPolls, fmt.Sprintf("Batch status: %s", status))

					switch status {
					case "completed":
						_ = progress.Log("info", "Batch completed, downloading results", nil)
						discoveries, dErr := parser.DownloadBatchResults(ctx, outputFileID)
						if dErr != nil {
							return fmt.Errorf("failed to download results: %w", dErr)
						}
						resultPayload := map[string]any{
							"mode":        "batch-full",
							"suggestions": discoveries,
							"batch_id":    batchID,
						}
						resultJSON, jErr := json.Marshal(resultPayload)
						if jErr != nil {
							return fmt.Errorf("failed to marshal results: %w", jErr)
						}
						if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
							return fmt.Errorf("failed to store results: %w", err)
						}
						_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Batch complete: %d suggestions", len(discoveries)))
						return nil

					case "failed", "expired", "cancelled":
						return fmt.Errorf("batch %s: %s", batchID, status)
					}
				}
				return fmt.Errorf("batch timed out after 24h")
			}); err != nil {
				return nil, err
			}
			return op, nil
		},
		IsEnabled: func() bool {
			return config.AppConfig.ScheduledAIDedupBatchEnabled && config.AppConfig.EnableAIParsing
		},
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ScheduledAIDedupBatchInterval
			if mins <= 0 {
				return 24 * time.Hour
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return config.AppConfig.ScheduledAIDedupBatchOnStartup },
		RunInMaintenanceWindow: func() bool { return false },
	})

	// Unified Batch Poller — discovers all project-tagged OpenAI batches and routes
	// completed ones to the appropriate handler (author_dedup, author_review,
	// diagnostics, pipeline, etc.)
	ts.registerTask(TaskDefinition{
		Name:        "batch_poller",
		Description: "Poll OpenAI for completed batch jobs",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			if s.batchPoller == nil {
				return nil, nil
			}
			processed, err := s.batchPoller.Poll(context.Background())
			if err != nil {
				log.Printf("[WARN] batch_poller: %v", err)
			}
			if processed > 0 {
				log.Printf("[INFO] batch_poller: processed %d completed batches", processed)
			}
			return nil, nil
		},
		IsEnabled: func() bool {
			return config.AppConfig.OpenAIAPIKey != "" && s.batchPoller != nil
		},
		GetInterval: func() time.Duration {
			return 5 * time.Minute
		},
		RunOnStart:             func() bool { return true },
		RunInMaintenanceWindow: func() bool { return false },
	})

	// Log Retention Pruning — prune old operation logs and system activity logs
	ts.registerTask(TaskDefinition{
		Name:        "purge_old_logs",
		Description: "Prune operation logs and system activity logs older than retention period",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			opID := ulid.Make().String()
			op, err := ts.server.Store().CreateOperation(opID, "purge_old_logs", nil)
			if err != nil {
				return nil, err
			}
			_ = ts.server.queue.Enqueue(opID, "purge_old_logs", operations.PriorityLow,
				func(ctx context.Context, progress operations.ProgressReporter) error {
					retLog := logger.New("purge_old_logs")
					_, err := logger.PruneOldLogs(ts.server.Store(), config.AppConfig.LogRetentionDays, retLog)
					return err
				},
			)
			return op, nil
		},
		IsEnabled:              func() bool { return config.AppConfig.LogRetentionDays > 0 },
		GetInterval:            func() time.Duration { return 7 * 24 * time.Hour },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowPurgeOldLogs },
	})

	// Activity Log Cleanup — summarize old change entries and prune old debug entries
	ts.registerTask(TaskDefinition{
		Name:        "cleanup_activity_log",
		Description: "Summarize old change entries and prune old debug entries from activity log",
		Category:    "maintenance",
		TriggerFn: func(source string) (*database.Operation, error) {
			opID := ulid.Make().String()
			op, err := ts.server.Store().CreateOperation(opID, "cleanup_activity_log", nil)
			if err != nil {
				return nil, err
			}
			_ = ts.server.queue.Enqueue(opID, "cleanup_activity_log", operations.PriorityLow,
				func(ctx context.Context, progress operations.ProgressReporter) error {
					if ts.server.activityService == nil {
						return nil
					}

					// Step 1: Compact old entries into daily digests
					compactionDays := config.AppConfig.ActivityLogCompactionDays
					if compactionDays <= 0 {
						compactionDays = 14
					}
					compactionCutoff := time.Now().AddDate(0, 0, -compactionDays)
					compacted, err := ts.server.activityService.CompactByDay(ctx, compactionCutoff)
					if err != nil {
						return fmt.Errorf("compact activity: %w", err)
					}

					// Step 2: Summarize remaining old change entries
					changeDays := config.AppConfig.ActivityLogRetentionChangeDays
					if changeDays <= 0 {
						changeDays = 90
					}
					changeCutoff := time.Now().AddDate(0, 0, -changeDays)
					summarized, err := ts.server.activityService.Summarize(ctx, changeCutoff, "change")
					if err != nil {
						return fmt.Errorf("summarize activity: %w", err)
					}

					// Step 3: Prune old debug entries
					debugDays := config.AppConfig.ActivityLogRetentionDebugDays
					if debugDays <= 0 {
						debugDays = 30
					}
					debugCutoff := time.Now().AddDate(0, 0, -debugDays)
					pruned, err := ts.server.activityService.Prune(debugCutoff, "debug")
					if err != nil {
						return fmt.Errorf("prune activity: %w", err)
					}

					log.Printf("Activity log cleanup: compacted %d days (%d entries), summarized %d, pruned %d",
						compacted.DaysCompacted, compacted.EntriesDeleted, summarized, pruned)
					return nil
				},
			)
			return op, nil
		},
		IsEnabled:              func() bool { return ts.server.activityService != nil },
		GetInterval:            func() time.Duration { return 24 * time.Hour },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return true },
	})
}

