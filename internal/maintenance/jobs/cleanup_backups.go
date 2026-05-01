// file: internal/maintenance/jobs/cleanup_backups.go
// version: 1.0.0
// guid: a1000021-0000-0000-0000-000000000021
// last-edited: 2026-05-03

package jobs

import (
	"context"
	"os"
	"path/filepath"
	"regexp"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&cleanupBackupsJob{}) }

var backupFileRe = regexp.MustCompile(`(?i)\.(backup|bak)$|\.bak-\d{8}-\d{6}$`)

type cleanupBackupsJob struct{}

func (j *cleanupBackupsJob) ID() string          { return "cleanup-backups" }
func (j *cleanupBackupsJob) Description() string { return "Delete .backup and .bak files from the library root" }
func (j *cleanupBackupsJob) CanResume() bool     { return false }
func (j *cleanupBackupsJob) Run(ctx context.Context, _ database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	root := config.AppConfig.RootDir
	if root == "" {
		reporter.Log("warn", "cleanup-backups: RootDir not configured", nil)
		return nil
	}
	removed := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !backupFileRe.MatchString(filepath.Base(path)) {
			return nil
		}
		if !dryRun {
			if rerr := os.Remove(path); rerr == nil {
				removed++
			}
		} else {
			removed++
			reporter.Log("info", "would remove: "+path, nil)
		}
		return nil
	})
	_ = removed
	reporter.Log("info", "cleanup-backups complete", nil)
	return err
}
