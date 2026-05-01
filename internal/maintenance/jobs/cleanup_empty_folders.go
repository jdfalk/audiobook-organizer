// file: internal/maintenance/jobs/cleanup_empty_folders.go
// version: 1.0.0
// guid: a1000006-0000-0000-0000-000000000006
// last-edited: 2026-05-03

package jobs

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&cleanupEmptyFoldersJob{}) }

type cleanupEmptyFoldersJob struct{}

func (j *cleanupEmptyFoldersJob) ID() string          { return "cleanup-empty-folders" }
func (j *cleanupEmptyFoldersJob) Description() string { return "Remove empty directories from the library root" }
func (j *cleanupEmptyFoldersJob) CanResume() bool     { return false }
func (j *cleanupEmptyFoldersJob) Run(ctx context.Context, _ database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	root := config.AppConfig.RootDir
	if root == "" {
		reporter.Log("warn", "cleanup-empty-folders: RootDir not configured", nil)
		return nil
	}
	removed := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		entries, rerr := os.ReadDir(path)
		if rerr != nil || len(entries) != 0 {
			return nil
		}
		if !dryRun {
			if rerr2 := os.Remove(path); rerr2 == nil {
				removed++
			}
		} else {
			removed++
		}
		return nil
	})
	_ = removed
	reporter.Log("info", "cleanup-empty-folders complete", nil)
	return err
}
