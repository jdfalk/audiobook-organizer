// file: internal/maintenance/jobs/merge_chapter_groups.go
// version: 1.1.0
// guid: a1000020-0000-0000-0000-000000000020
// last-edited: 2026-05-01

package jobs

import (
	"context"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
)

func init() { maintenance.Register(&mergeChapterGroupsJob{}) }

type mergeChapterGroupsJob struct{}

func (j *mergeChapterGroupsJob) ID() string          { return "merge-chapter-groups" }
func (j *mergeChapterGroupsJob) Name() string     { return "Merge Chapter Groups" }
func (j *mergeChapterGroupsJob) Category() string { return "files" }
func (j *mergeChapterGroupsJob) DefaultParams() any { return struct{ DryRun bool `json:"dry_run"` }{DryRun: true} }
func (j *mergeChapterGroupsJob) Description() string { return "Merge multi-chapter book files into consolidated book records" }
func (j *mergeChapterGroupsJob) CanResume() bool     { return false }
func (j *mergeChapterGroupsJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	books, err := store.GetAllBooks(0, 0)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	groups := scanner.DetectChapterGroups(books, 2, 600)
	reporter.SetTotal(len(groups))
	merged := 0
	for _, g := range groups {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reporter.Increment()
		if len(g.BookIDs) < 2 {
			continue
		}
		primaryID := g.BookIDs[0]
		srcIDs := g.BookIDs[1:]
		if !dryRun {
			if merr := store.MergeChapterBooks(primaryID, srcIDs, g.CommonTitle, g.TotalDuration); merr != nil {
				msg := merr.Error()
				reporter.Log("error", "merge-chapter-groups: MergeChapterBooks failed", &msg)
				continue
			}
		}
		merged++
		detail := fmt.Sprintf("primary=%s srcs=%v title=%q", primaryID, srcIDs, g.CommonTitle)
		reporter.Log("info", "merged chapter group", &detail)
	}
	reporter.Log("info", fmt.Sprintf("merge-chapter-groups complete: %d merged", merged), nil)
	return nil
}
