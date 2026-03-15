// file: internal/server/scheduler.go
// version: 1.10.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// TaskDefinition defines a registered task in the unified task system.
type TaskDefinition struct {
	Name        string // unique key: "library_scan", "itunes_sync", etc.
	Description string // human-readable
	Category    string // "maintenance", "library", "sync"
	// TriggerFn creates and enqueues an operation, returning it.
	TriggerFn func() (*database.Operation, error)
	// Config accessors (read from AppConfig at runtime)
	IsEnabled              func() bool
	GetInterval            func() time.Duration // 0 = manual only
	RunOnStart             func() bool
	RunInMaintenanceWindow func() bool // whether this task runs during the maintenance window
}

// TaskInfo is the API-facing view of a registered task.
type TaskInfo struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Category        string  `json:"category"`
	Enabled         bool    `json:"enabled"`
	IntervalMinutes int     `json:"interval_minutes"`
	RunOnStartup           bool    `json:"run_on_startup"`
	RunInMaintenanceWindow bool    `json:"run_in_maintenance_window"`
	LastRun                *string `json:"last_run,omitempty"`
}

// TaskScheduler manages all registered tasks, their schedules, and manual triggers.
type TaskScheduler struct {
	server             *Server
	tasks              map[string]*TaskDefinition
	order              []string // insertion order for listing
	lastRun            map[string]time.Time
	mu                 sync.RWMutex
	shutdown           chan struct{}
	maintenanceOrder   []string
	lastMaintenanceRun time.Time
}

// NewTaskScheduler creates a scheduler and registers all known tasks.
func NewTaskScheduler(s *Server) *TaskScheduler {
	ts := &TaskScheduler{
		server:  s,
		tasks:   make(map[string]*TaskDefinition),
		lastRun: make(map[string]time.Time),
	}
	ts.registerAllTasks()
	ts.maintenanceOrder = []string{
		"reconcile_scan",
		"dedup_refresh",
		"author_split_scan",
		"series_prune",
		"tombstone_cleanup",
		"purge_deleted",
		"purge_old_logs",
		"db_optimize",
	}
	return ts
}

func (ts *TaskScheduler) registerTask(def TaskDefinition) {
	ts.tasks[def.Name] = &def
	ts.order = append(ts.order, def.Name)
}

func (ts *TaskScheduler) registerAllTasks() {
	s := ts.server

	// --- Library tasks ---

	ts.registerTask(TaskDefinition{
		Name:        "library_scan",
		Description: "Scan library for new/changed audiobooks (incremental by default, use force_update for full rescan)",
		Category:    "library",
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("scan", func(ctx context.Context, progress operations.ProgressReporter) error {
				if s.scanService == nil {
					return fmt.Errorf("scan service not initialized")
				}
				return s.scanService.PerformScan(ctx, &ScanRequest{}, operations.LoggerFromReporter(progress))
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
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("organize", func(ctx context.Context, progress operations.ProgressReporter) error {
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
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("transcode", func(ctx context.Context, progress operations.ProgressReporter) error {
				return fmt.Errorf("transcode requires parameters — use the operations API directly")
			})
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return false },
	})

	// --- Sync tasks ---

	ts.registerTask(TaskDefinition{
		Name:        "itunes_sync",
		Description: "Sync with iTunes/Music library",
		Category:    "sync",
		TriggerFn: func() (*database.Operation, error) {
			s.triggerITunesSync()
			return nil, nil // iTunes sync creates its own operation internally
		},
		IsEnabled: func() bool { return config.AppConfig.ITunesSyncEnabled },
		GetInterval: func() time.Duration {
			mins := config.AppConfig.ITunesSyncInterval
			if mins < 1 {
				mins = 30
			}
			return time.Duration(mins) * time.Minute
		},
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return false },
	})

	ts.registerTask(TaskDefinition{
		Name:        "itunes_import",
		Description: "Import from iTunes library",
		Category:    "sync",
		TriggerFn: func() (*database.Operation, error) {
			return nil, fmt.Errorf("iTunes import requires parameters — use the iTunes API directly")
		},
		IsEnabled:              func() bool { return true },
		GetInterval:            func() time.Duration { return 0 },
		RunOnStart:             func() bool { return false },
		RunInMaintenanceWindow: func() bool { return false },
	})

	// --- Maintenance tasks ---

	ts.registerTask(TaskDefinition{
		Name:        "dedup_refresh",
		Description: "Refresh author & series dedup cache",
		Category:    "maintenance",
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("author-dedup-scan", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				_ = progress.Log("info", "Starting author dedup scan", nil)
				_ = progress.UpdateProgress(0, 100, "Fetching authors...")
				authors, err := store.GetAllAuthors()
				if err != nil {
					return fmt.Errorf("failed to get authors: %w", err)
				}
				msg := fmt.Sprintf("Fetched %d authors, running duplicate comparison...", len(authors))
				_ = progress.Log("info", msg, nil)
				_ = progress.UpdateProgress(25, 100, msg)

				bookCounts, _ := store.GetAllAuthorBookCounts()

				total := len(authors)
				groups := FindDuplicateAuthors(authors, 0.85, func(id int) int { return bookCounts[id] },
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
		Name:        "series_prune",
		Description: "Merge duplicate series and delete orphans",
		Category:    "maintenance",
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperationWithID("series-prune", func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
				store := database.GlobalStore
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
		Name:        "author_split_scan",
		Description: "Find & split composite author names",
		Category:    "maintenance",
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("author-split-scan", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
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

					parts := SplitCompositeAuthorName(author.Name)
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
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("db-optimize", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
				if store == nil {
					return fmt.Errorf("database not initialized")
				}

				storesOptimized := 0
				storesTotal := 3

				// 1. Main store
				_ = progress.Log("info", "Optimizing main database", nil)
				_ = progress.UpdateProgress(0, storesTotal, "Optimizing main database...")
				if err := store.Optimize(); err != nil {
					_ = progress.Log("error", fmt.Sprintf("Main DB optimization failed: %v", err), nil)
				} else {
					storesOptimized++
					_ = progress.Log("info", "Main database optimized", nil)
				}

				// 2. AI scan store
				_ = progress.UpdateProgress(1, storesTotal, "Optimizing AI scan database...")
				if s.aiScanStore != nil {
					if err := s.aiScanStore.Optimize(); err != nil {
						_ = progress.Log("error", fmt.Sprintf("AI scan DB optimization failed: %v", err), nil)
					} else {
						storesOptimized++
						_ = progress.Log("info", "AI scan database optimized", nil)
					}
				} else {
					_ = progress.Log("info", "AI scan store not initialized, skipping", nil)
				}

				// 3. OpenLibrary store (accessed via olService)
				_ = progress.UpdateProgress(2, storesTotal, "Optimizing OpenLibrary cache...")
				if s.olService != nil && s.olService.Store() != nil {
					if err := s.olService.Store().Optimize(); err != nil {
						_ = progress.Log("error", fmt.Sprintf("OL cache optimization failed: %v", err), nil)
					} else {
						storesOptimized++
						_ = progress.Log("info", "OpenLibrary cache optimized", nil)
					}
				} else {
					_ = progress.Log("info", "OpenLibrary store not initialized, skipping", nil)
				}

				_ = progress.UpdateProgress(storesTotal, storesTotal, fmt.Sprintf("Database optimization complete: %d/%d stores", storesOptimized, storesTotal))
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
		Name:        "purge_deleted",
		Description: "Purge soft-deleted books past retention",
		Category:    "maintenance",
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("purge-deleted", func(ctx context.Context, progress operations.ProgressReporter) error {
				_ = progress.Log("info", "Starting purge of soft-deleted books", nil)
				_ = progress.UpdateProgress(0, 100, "Purging soft-deleted books...")
				s.runAutoPurgeSoftDeleted()
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
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("tombstone-cleanup", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
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
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("resolve-production-authors", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
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
					if isProductionCompany(a.Name) {
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
							if newAuthor != nil && !isProductionCompany(newAuthor.Name) {
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
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("metadata-refresh", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				_ = progress.Log("info", "Starting metadata refresh scan", nil)
				_ = progress.UpdateProgress(0, 100, "Scanning books for incomplete metadata...")
				books, err := store.GetAllBooks(10000, 0)
				if err != nil {
					return fmt.Errorf("failed to get books: %w", err)
				}
				_ = progress.Log("info", fmt.Sprintf("Checking %d books for incomplete metadata", len(books)), nil)
				incomplete := 0
				for i, book := range books {
					if book.AuthorID == nil || book.Title == "" {
						incomplete++
						_ = progress.Log("debug", fmt.Sprintf("Incomplete: %q (id=%s)", book.Title, book.ID), nil)
					}
					if (i+1)%200 == 0 {
						_ = progress.UpdateProgress(i+1, len(books), fmt.Sprintf("Checked %d/%d books", i+1, len(books)))
					}
				}
				resultMsg := fmt.Sprintf("Found %d books with incomplete metadata out of %d total", incomplete, len(books))
				_ = progress.Log("info", resultMsg, nil)
				_ = progress.UpdateProgress(len(books), len(books), resultMsg)
				return nil
			})
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
		TriggerFn: func() (*database.Operation, error) {
			store := database.GlobalStore
			if store == nil {
				return nil, fmt.Errorf("database not initialized")
			}
			id := ulid.Make().String()
			op, err := store.CreateOperation(id, "reconcile_scan", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create operation: %w", err)
			}
			if err := operations.GlobalQueue.Enqueue(op.ID, "reconcile_scan", operations.PriorityNormal,
				func(ctx context.Context, progress operations.ProgressReporter) error {
					reconcileLog := operations.LoggerFromReporter(progress)
					result, scanErr := buildReconcilePreviewWithProgress(store, reconcileLog)
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
		TriggerFn: func() (*database.Operation, error) {
			store := database.GlobalStore
			if store == nil {
				return nil, fmt.Errorf("database not initialized")
			}
			if operations.GlobalQueue == nil {
				return nil, fmt.Errorf("operation queue not initialized")
			}
			opID := ulid.Make().String()
			op, err := store.CreateOperation(opID, "ai-dedup-batch", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create operation: %w", err)
			}
			if err := operations.GlobalQueue.Enqueue(op.ID, "ai-dedup-batch", operations.PriorityLow, func(ctx context.Context, progress operations.ProgressReporter) error {
				parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
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
		TriggerFn: func() (*database.Operation, error) {
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
		TriggerFn: func() (*database.Operation, error) {
			opID := ulid.Make().String()
			op, err := database.GlobalStore.CreateOperation(opID, "purge_old_logs", nil)
			if err != nil {
				return nil, err
			}
			_ = operations.GlobalQueue.Enqueue(opID, "purge_old_logs", operations.PriorityLow,
				func(ctx context.Context, progress operations.ProgressReporter) error {
					retLog := logger.New("purge_old_logs")
					_, err := logger.PruneOldLogs(database.GlobalStore, config.AppConfig.LogRetentionDays, retLog)
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
}

// triggerOperation is a helper that creates a DB operation and enqueues it.
func (ts *TaskScheduler) triggerOperation(opType string, fn func(context.Context, operations.ProgressReporter) error) (*database.Operation, error) {
	store := database.GlobalStore
	if store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if operations.GlobalQueue == nil {
		return nil, fmt.Errorf("operation queue not initialized")
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, opType, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create operation: %w", err)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, opType, operations.PriorityNormal, fn); err != nil {
		return nil, fmt.Errorf("failed to enqueue operation: %w", err)
	}

	return op, nil
}

// triggerOperationWithID is like triggerOperation but passes the operation ID to the function.
func (ts *TaskScheduler) triggerOperationWithID(opType string, fn func(context.Context, operations.ProgressReporter, string) error) (*database.Operation, error) {
	store := database.GlobalStore
	if store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if operations.GlobalQueue == nil {
		return nil, fmt.Errorf("operation queue not initialized")
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, opType, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create operation: %w", err)
	}

	wrappedFn := func(ctx context.Context, progress operations.ProgressReporter) error {
		return fn(ctx, progress, op.ID)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, opType, operations.PriorityNormal, wrappedFn); err != nil {
		return nil, fmt.Errorf("failed to enqueue operation: %w", err)
	}

	return op, nil
}

// Start launches background goroutines for all scheduled and startup tasks.
func (ts *TaskScheduler) Start(shutdown chan struct{}, wg *sync.WaitGroup) {
	ts.shutdown = shutdown
	ts.loadLastMaintenanceRun()

	for _, name := range ts.order {
		task := ts.tasks[name]

		// Run on startup if configured
		if task.RunOnStart != nil && task.RunOnStart() && task.IsEnabled() {
			taskName := name
			go func() {
				log.Printf("[INFO] Running startup task: %s", taskName)
				if op, err := ts.RunTask(taskName); err != nil {
					log.Printf("[WARN] Startup task %s failed: %v", taskName, err)
				} else if op != nil {
					log.Printf("[INFO] Startup task %s started: operation %s", taskName, op.ID)
				}
			}()
		}

		// Start scheduled ticker if interval > 0 and enabled
		if task.IsEnabled() && task.GetInterval() > 0 {
			interval := task.GetInterval()
			taskName := name
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						if op, err := ts.RunTask(taskName); err != nil {
							log.Printf("[WARN] Scheduled task %s failed: %v", taskName, err)
						} else if op != nil {
							log.Printf("[INFO] Scheduled task %s started: operation %s", taskName, op.ID)
						}
					case <-shutdown:
						return
					}
				}
			}()
			log.Printf("[INFO] Scheduled task %s: interval=%v", taskName, interval)
		}
	}

	// Maintenance window checker — runs every 60 seconds
	if config.AppConfig.MaintenanceWindowEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			log.Printf("[INFO] Maintenance window enabled: %d:00 - %d:00",
				config.AppConfig.MaintenanceWindowStart, config.AppConfig.MaintenanceWindowEnd)
			for {
				select {
				case <-ticker.C:
					if isInMaintenanceWindow() && !ts.hasRunToday() {
						log.Printf("[INFO] Maintenance window open — starting maintenance run")
						if err := ts.RunMaintenanceWindow(context.Background()); err != nil {
							log.Printf("[WARN] Maintenance window failed: %v", err)
						}
					}
				case <-shutdown:
					return
				}
			}
		}()
	}
}

// RunTask triggers a task by name, returning the created operation.
func (ts *TaskScheduler) RunTask(name string) (*database.Operation, error) {
	ts.mu.RLock()
	task, ok := ts.tasks[name]
	ts.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown task: %s", name)
	}

	op, err := task.TriggerFn()
	if err != nil {
		return nil, err
	}

	ts.mu.Lock()
	ts.lastRun[name] = time.Now()
	ts.mu.Unlock()

	return op, nil
}

// ListTasks returns info about all registered tasks.
func (ts *TaskScheduler) ListTasks() []TaskInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []TaskInfo
	for _, name := range ts.order {
		task := ts.tasks[name]
		info := TaskInfo{
			Name:            task.Name,
			Description:     task.Description,
			Category:        task.Category,
			Enabled:         task.IsEnabled(),
			IntervalMinutes: int(task.GetInterval() / time.Minute),
			RunOnStartup:    task.RunOnStart(),
		}
		if task.RunInMaintenanceWindow != nil {
			info.RunInMaintenanceWindow = task.RunInMaintenanceWindow()
		}
		if t, ok := ts.lastRun[name]; ok {
			s := t.Format(time.RFC3339)
			info.LastRun = &s
		}
		result = append(result, info)
	}
	return result
}

// GetTask returns the definition for a named task.
func (ts *TaskScheduler) GetTask(name string) (*TaskDefinition, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	task, ok := ts.tasks[name]
	return task, ok
}

// --- Maintenance Window ---

// maintenanceCtxKey is a typed context key to avoid string-key collisions.
type maintenanceCtxKey string

const ignoreWindowKey maintenanceCtxKey = "ignore_window"

// isInMaintenanceWindowAt checks if a given hour falls within the configured window.
// Supports midnight-spanning windows (e.g., start=23, end=2).
func isInMaintenanceWindowAt(hour int) bool {
	if !config.AppConfig.MaintenanceWindowEnabled {
		return false
	}
	start := config.AppConfig.MaintenanceWindowStart
	end := config.AppConfig.MaintenanceWindowEnd

	if start < end {
		return hour >= start && hour < end
	}
	// Midnight spanning: e.g., start=23, end=2 → 23,0,1 are in window
	return hour >= start || hour < end
}

// isInMaintenanceWindow checks if the current time falls within the configured window.
func isInMaintenanceWindow() bool {
	return isInMaintenanceWindowAt(time.Now().Hour())
}

// loadLastMaintenanceRun reads the persisted last-run date from the database.
func (ts *TaskScheduler) loadLastMaintenanceRun() {
	store := database.GlobalStore
	if store == nil {
		return
	}
	setting, err := store.GetSetting("maintenance_window_last_run")
	if err != nil || setting == nil {
		return
	}
	t, err := time.Parse("2006-01-02", setting.Value)
	if err != nil {
		return
	}
	ts.lastMaintenanceRun = t
}

// saveLastMaintenanceRun persists today's date as the last-run date.
func (ts *TaskScheduler) saveLastMaintenanceRun() {
	store := database.GlobalStore
	if store == nil {
		return
	}
	today := time.Now().Format("2006-01-02")
	_ = store.SetSetting("maintenance_window_last_run", today, "string", false)
	ts.lastMaintenanceRun = time.Now()
}

// hasRunToday checks if the maintenance window has already run today.
func (ts *TaskScheduler) hasRunToday() bool {
	today := time.Now().Format("2006-01-02")
	return ts.lastMaintenanceRun.Format("2006-01-02") == today
}

// isTaskRunning checks if a task's operation is currently in progress.
func (ts *TaskScheduler) isTaskRunning(name string) bool {
	store := database.GlobalStore
	if store == nil {
		return false
	}
	ops, _, err := store.ListOperations(100, 0)
	if err != nil {
		return false
	}
	opTypeMap := map[string]string{
		"library_scan": "scan", "library_organize": "organize",
		"dedup_refresh": "author-dedup-scan", "series_prune": "series-prune",
		"author_split_scan": "author-split-scan", "db_optimize": "db-optimize",
		"purge_deleted": "purge-deleted", "tombstone_cleanup": "tombstone-cleanup",
		"reconcile_scan": "reconcile_scan", "purge_old_logs": "purge_old_logs",
		"metadata_refresh": "metadata-refresh",
	}
	opType, ok := opTypeMap[name]
	if !ok {
		return false
	}
	for _, op := range ops {
		if op.Type == opType && (op.Status == "running" || op.Status == "pending") {
			return true
		}
	}
	return false
}

// waitForOperation polls until an operation completes or the context is canceled.
func (ts *TaskScheduler) waitForOperation(ctx context.Context, opID string) {
	store := database.GlobalStore
	if store == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			op, err := store.GetOperationByID(opID)
			if err != nil {
				return
			}
			if op.Status == "completed" || op.Status == "failed" || op.Status == "canceled" {
				return
			}
		}
	}
}

// RunMaintenanceWindow runs all maintenance-window-eligible tasks in order.
// Step 1: auto-update (if enabled). Step 2+: maintenance tasks in fixed order.
func (ts *TaskScheduler) RunMaintenanceWindow(ctx context.Context) error {
	store := database.GlobalStore
	if store == nil {
		return fmt.Errorf("database not initialized")
	}
	if operations.GlobalQueue == nil {
		return fmt.Errorf("operation queue not initialized")
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "maintenance-window", nil)
	if err != nil {
		return fmt.Errorf("failed to create maintenance-window operation: %w", err)
	}

	_ = operations.GlobalQueue.Enqueue(op.ID, "maintenance-window", operations.PriorityNormal,
		func(innerCtx context.Context, progress operations.ProgressReporter) error {
			ignoreWindow := ctx.Value(ignoreWindowKey) != nil

			// Step 1: Auto-update (if enabled and not already completed post-restart)
			if config.AppConfig.AutoUpdateEnabled {
				updateDone, _ := store.GetSetting("maintenance_window_update_completed")
				today := time.Now().Format("2006-01-02")
				if updateDone == nil || updateDone.Value != today {
					_ = progress.Log("info", "Running auto-update (step 1)", nil)
					_ = progress.UpdateProgress(0, 100, "Running auto-update...")
					_ = store.SetSetting("maintenance_window_update_completed", today, "string", false)
					if ts.server.updater != nil {
						channel := config.AppConfig.AutoUpdateChannel
						info, checkErr := ts.server.updater.CheckForUpdate(channel)
						if checkErr != nil {
							_ = progress.Log("warning", fmt.Sprintf("Auto-update check failed: %v", checkErr), nil)
						} else if info != nil && info.UpdateAvailable {
							_ = progress.Log("info", fmt.Sprintf("Update available: %s, applying...", info.LatestVersion), nil)
							if applyErr := ts.server.updater.DownloadAndReplace(info); applyErr != nil {
								_ = progress.Log("error", fmt.Sprintf("Auto-update apply failed: %v", applyErr), nil)
							} else {
								_ = progress.Log("info", "Update applied, server will restart", nil)
								go ts.server.updater.RestartSelf()
								return nil // Exit — server restarting
							}
						} else {
							_ = progress.Log("info", "No update available", nil)
						}
					}
					_ = progress.Log("info", "Auto-update step complete", nil)
				} else {
					_ = progress.Log("info", "Auto-update already completed today, skipping", nil)
				}
			}

			// Step 2+: Maintenance tasks in order
			var eligible []string
			for _, name := range ts.maintenanceOrder {
				task, ok := ts.tasks[name]
				if !ok {
					continue
				}
				if task.IsEnabled() && task.RunInMaintenanceWindow != nil && task.RunInMaintenanceWindow() {
					eligible = append(eligible, name)
				}
			}

			_ = progress.Log("info", fmt.Sprintf("Maintenance window starting: %d tasks eligible", len(eligible)), nil)

			hadErrors := false
			for i, name := range eligible {
				// Check if window is still open (skip for manual "Run Now" triggers)
				if !ignoreWindow && !isInMaintenanceWindow() {
					_ = progress.Log("warning", fmt.Sprintf("Maintenance window closed after task %d/%d, skipping remaining", i, len(eligible)), nil)
					break
				}

				// Duplicate prevention: skip if already running from interval ticker
				if ts.isTaskRunning(name) {
					_ = progress.Log("info", fmt.Sprintf("Task %s already running (interval), skipping", name), nil)
					continue
				}

				_ = progress.UpdateProgress(i, len(eligible), fmt.Sprintf("Running task %d/%d: %s", i+1, len(eligible), name))
				_ = progress.Log("info", fmt.Sprintf("Starting maintenance task: %s", name), nil)

				taskOp, taskErr := ts.RunTask(name)
				if taskErr != nil {
					hadErrors = true
					_ = progress.Log("error", fmt.Sprintf("Task %s failed: %v", name, taskErr), nil)
				} else if taskOp != nil {
					// Wait for the task operation to complete before starting next
					ts.waitForOperation(innerCtx, taskOp.ID)
					completedOp, _ := store.GetOperationByID(taskOp.ID)
					if completedOp != nil && completedOp.Status == "failed" {
						hadErrors = true
						_ = progress.Log("warning", fmt.Sprintf("Task %s operation failed", name), nil)
					} else {
						_ = progress.Log("info", fmt.Sprintf("Task %s completed (op: %s)", name, taskOp.ID), nil)
					}
				} else {
					_ = progress.Log("info", fmt.Sprintf("Task %s triggered (no operation)", name), nil)
				}
			}

			ts.saveLastMaintenanceRun()

			if hadErrors {
				_ = progress.UpdateProgress(len(eligible), len(eligible), "Maintenance window completed with errors")
				return fmt.Errorf("maintenance window completed with errors")
			}
			_ = progress.UpdateProgress(len(eligible), len(eligible), "Maintenance window completed successfully")
			return nil
		},
	)
	return nil
}
