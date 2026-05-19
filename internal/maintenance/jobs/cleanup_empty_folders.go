// file: internal/maintenance/jobs/cleanup_empty_folders.go
// version: 1.2.1
// guid: a1000006-0000-0000-0000-000000000006
// last-edited: 2026-05-05

package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"log/slog")

func init() { maintenance.Register(&cleanupEmptyFoldersJob{}) }

type cleanupEmptyFoldersJob struct{}

func (j *cleanupEmptyFoldersJob) ID() string       { return "cleanup-empty-folders" }
func (j *cleanupEmptyFoldersJob) Name() string     { return "Cleanup Empty Folders" }
func (j *cleanupEmptyFoldersJob) Category() string { return "cleanup" }
func (j *cleanupEmptyFoldersJob) DefaultParams() any {
	return struct {
		DryRun bool `json:"dry_run"`
	}{DryRun: true}
}
func (j *cleanupEmptyFoldersJob) Description() string {
	return "Remove empty directories from the library root (bottom-up walk, deepest first)"
}
func (j *cleanupEmptyFoldersJob) CanResume() bool { return true }

func (j *cleanupEmptyFoldersJob) Run(ctx context.Context, _ database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	root := config.AppConfig.RootDir
	if root == "" {
		slog.Warn("cleanup-empty-folders: RootDir not configured")
		return nil
	}

	// Collect all directories with a top-down walk, then sort deepest first
	// so children are processed before their parents.
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	}); err != nil {
		return fmt.Errorf("cleanup-empty-folders: walk error: %w", err)
	}

	// Sort by descending path length so deepest directories come first.
	sort.Slice(dirs, func(i, k int) bool { return len(dirs[i]) > len(dirs[k]) })

	reporter.SetTotal(len(dirs))
	slog.Info(fmt.Sprintf("cleanup-empty-folders: found %d directories to check (dry_run=%v)", len(dirs), dryRun))

	removed := 0
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			slog.Error(fmt.Sprintf("cleanup-empty-folders: failed to read %s: %v", dir, err))
			reporter.Increment()
			continue
		}

		if len(entries) > 0 {
			reporter.Increment()
			continue
		}

		if dryRun {
			slog.Info(fmt.Sprintf("[dry] would remove empty dir: %s", dir))
		} else {
			if err := os.Remove(dir); err != nil {
				slog.Error(fmt.Sprintf("cleanup-empty-folders: failed to remove %s: %v", dir, err))
			} else {
				slog.Info(fmt.Sprintf("removed empty dir: %s", dir))
				removed++
			}
		}
		reporter.Increment()
	}

	slog.Info(fmt.Sprintf("cleanup-empty-folders: complete — checked %d dirs, removed %d (dry_run=%v)", len(dirs), removed, dryRun))
	return nil
}
