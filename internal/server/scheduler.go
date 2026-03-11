// file: internal/server/scheduler.go
// version: 1.9.0
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
	IsEnabled   func() bool
	GetInterval func() time.Duration // 0 = manual only
	RunOnStart  func() bool
}

// TaskInfo is the API-facing view of a registered task.
type TaskInfo struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Category        string  `json:"category"`
	Enabled         bool    `json:"enabled"`
	IntervalMinutes int     `json:"interval_minutes"`
	RunOnStartup    bool    `json:"run_on_startup"`
	LastRun         *string `json:"last_run,omitempty"`
}

// TaskScheduler manages all registered tasks, their schedules, and manual triggers.
type TaskScheduler struct {
	server   *Server
	tasks    map[string]*TaskDefinition
	order    []string // insertion order for listing
	lastRun  map[string]time.Time
	mu       sync.RWMutex
	shutdown chan struct{}
}

// NewTaskScheduler creates a scheduler and registers all known tasks.
func NewTaskScheduler(s *Server) *TaskScheduler {
	ts := &TaskScheduler{
		server:  s,
		tasks:   make(map[string]*TaskDefinition),
		lastRun: make(map[string]time.Time),
	}
	ts.registerAllTasks()
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
				return s.scanService.PerformScan(ctx, &ScanRequest{}, progress)
			})
		},
		IsEnabled:   func() bool { return config.AppConfig.ScanOnStartup }, // reuse existing field
		GetInterval: func() time.Duration { return 0 },                     // manual only by default
		RunOnStart:  func() bool { return config.AppConfig.ScanOnStartup },
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
				return s.organizeService.PerformOrganize(ctx, &OrganizeRequest{}, progress)
			})
		},
		IsEnabled:   func() bool { return true },
		GetInterval: func() time.Duration { return 0 },
		RunOnStart:  func() bool { return false },
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
		IsEnabled:   func() bool { return true },
		GetInterval: func() time.Duration { return 0 },
		RunOnStart:  func() bool { return false },
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
		RunOnStart: func() bool { return false },
	})

	ts.registerTask(TaskDefinition{
		Name:        "itunes_import",
		Description: "Import from iTunes library",
		Category:    "sync",
		TriggerFn: func() (*database.Operation, error) {
			return nil, fmt.Errorf("iTunes import requires parameters — use the iTunes API directly")
		},
		IsEnabled:   func() bool { return true },
		GetInterval: func() time.Duration { return 0 },
		RunOnStart:  func() bool { return false },
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledDedupRefreshOnStartup },
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledSeriesPruneOnStartup },
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledAuthorSplitOnStartup },
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
				_ = progress.Log("info", "Starting database optimization", nil)
				_ = progress.UpdateProgress(0, 100, "Optimizing database...")
				if err := store.Optimize(); err != nil {
					_ = progress.Log("error", fmt.Sprintf("Optimization failed: %v", err), nil)
					return fmt.Errorf("database optimization failed: %w", err)
				}
				_ = progress.Log("info", "Database optimization completed successfully", nil)
				_ = progress.UpdateProgress(100, 100, "Database optimized")
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledDbOptimizeOnStartup },
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
		RunOnStart: func() bool { return config.AppConfig.PurgeSoftDeletedAfterDays > 0 },
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
		IsEnabled:   func() bool { return true },
		GetInterval: func() time.Duration { return 24 * time.Hour },
		RunOnStart:  func() bool { return false },
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
		RunOnStart: func() bool { return false },
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledMetadataRefreshOnStartup },
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
					result, scanErr := buildReconcilePreviewWithProgress(store, progress)
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledReconcileOnStartup },
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
		RunOnStart: func() bool { return config.AppConfig.ScheduledAIDedupBatchOnStartup },
	})

	// AI Pipeline Batch Polling — checks for completed batch phases in the pipeline
	ts.registerTask(TaskDefinition{
		Name:        "ai_pipeline_batch_poll",
		Description: "Poll OpenAI Batch API for completed pipeline phases",
		Category:    "maintenance",
		TriggerFn: func() (*database.Operation, error) {
			if s.pipelineManager == nil {
				return nil, fmt.Errorf("pipeline manager not initialized")
			}
			ctx := context.Background()
			s.pipelineManager.PollBatchPhases(ctx)
			return nil, nil // No operation created — polling is lightweight
		},
		IsEnabled: func() bool {
			return config.AppConfig.EnableAIParsing && s.pipelineManager != nil
		},
		GetInterval: func() time.Duration {
			return 5 * time.Minute // Poll every 5 minutes
		},
		RunOnStart: func() bool { return false },
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
