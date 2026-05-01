// file: internal/maintenance/jobs/scan_composer_tags.go
// version: 1.0.0
// guid: d9e5f3c4-6a7b-8c9d-0e1f-2a3b4c5d6e7f
// last-edited: 2026-04-28

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

func init() { maintenance.Register(&scanComposerTagsJob{}) }

type scanComposerTagsJob struct{}

type sct_params struct {
	DryRun  bool   `json:"dry_run"`
	FixMode string `json:"fix_mode"` // "set_narrator" or "clear"
}

func (j *scanComposerTagsJob) ID() string          { return "scan-composer-tags" }
func (j *scanComposerTagsJob) Name() string        { return "Scan Composer Tags" }
func (j *scanComposerTagsJob) Category() string    { return "Scanning" }
func (j *scanComposerTagsJob) Description() string { return "Bulk-scans COMPOSER tags on all audio files and optionally fixes them to match the narrator field" }
func (j *scanComposerTagsJob) DefaultParams() any  { return &sct_params{DryRun: true, FixMode: "set_narrator"} }
func (j *scanComposerTagsJob) CanResume() bool     { return true }

func (j *scanComposerTagsJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	opID := maintenance.OperationIDFromCtx(ctx)

	// Load fix_mode from persisted params when resuming.
	fixMode := "set_narrator"
	if opID != "" {
		if raw, err := store.GetOperationParams(opID); err == nil && len(raw) > 0 {
			var p sct_params
			if jerr := json.Unmarshal(raw, &p); jerr == nil && p.FixMode != "" {
				fixMode = p.FixMode
				dryRun = p.DryRun
			}
		}
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("GetAllBooks: %w", err)
	}
	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("GetAllAuthors: %w", err)
	}
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}
	allFiles, err := store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("GetAllBookFiles: %w", err)
	}
	filesByBook := make(map[string][]database.BookFile, len(allFiles))
	for i := range allFiles {
		f := &allFiles[i]
		filesByBook[f.BookID] = append(filesByBook[f.BookID], *f)
	}

	// Skip already-processed files from a prior interrupted run.
	var existingResults []database.OperationResult
	if opID != "" {
		existingResults, _ = store.GetOperationResults(opID)
	}
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true // BookID stores file path for this op
	}

	audioExts := map[string]bool{".m4b": true, ".m4a": true, ".mp3": true, ".flac": true, ".ogg": true}
	var workItems []sct_work
	for i := range allBooks {
		b := &allBooks[i]
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		narrator := ""
		if b.Narrator != nil {
			narrator = *b.Narrator
		}
		for _, f := range filesByBook[b.ID] {
			if f.FilePath == "" || f.Missing {
				continue
			}
			if !audioExts[strings.ToLower(filepath.Ext(f.FilePath))] {
				continue
			}
			if done[f.FilePath] {
				continue
			}
			workItems = append(workItems, sct_work{
				bookID:    b.ID,
				bookTitle: b.Title,
				filePath:  f.FilePath,
				author:    author,
				narrator:  narrator,
			})
		}
	}

	totalFiles := len(existingResults) + len(workItems)
	alreadyDone := len(existingResults)
	log.Printf("[INFO] scan-composer-tags %s: %d files total, %d already done, %d to process",
		opID, totalFiles, alreadyDone, len(workItems))

	reporter.SetTotal(totalFiles)
	for i := 0; i < alreadyDone; i++ {
		reporter.Increment()
	}

	if len(workItems) == 0 {
		reporter.Log("info", "all files already processed", nil)
		return nil
	}

	const workers = 8
	workCh := make(chan sct_work, len(workItems))
	for _, w := range workItems {
		workCh <- w
	}
	close(workCh)

	var completed int64 = int64(alreadyDone)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				if ctx.Err() != nil {
					return
				}
				if _, statErr := os.Stat(w.filePath); statErr != nil {
					if opID != "" {
						_ = store.CreateOperationResult(&database.OperationResult{
							OperationID: opID,
							BookID:      w.filePath,
							ResultJSON:  `{"category":"missing"}`,
							Status:      "missing",
						})
					}
					atomic.AddInt64(&completed, 1)
					mu.Lock()
					reporter.Increment()
					mu.Unlock()
					continue
				}

				tags, readErr := metadata.ReadRawTags(w.filePath)
				var r sct_result
				if readErr != nil {
					r = sct_result{
						BookID: w.bookID, BookTitle: w.bookTitle, FilePath: w.filePath,
						Category: "read_error", Error: readErr.Error(),
					}
				} else {
					composer := ""
					if vs, ok := tags["COMPOSER"]; ok && len(vs) > 0 {
						composer = strings.TrimSpace(vs[0])
					}
					category, willWrite := sct_categorize(composer, w.author, w.narrator, fixMode)
					r = sct_result{
						BookID: w.bookID, BookTitle: w.bookTitle, FilePath: w.filePath,
						Category: category, Composer: composer,
						Author: w.author, Narrator: w.narrator, WillWrite: willWrite,
					}
					if !dryRun && category != "ok" && willWrite != composer {
						if writeErr := metadata.WriteSingleTag(w.filePath, "COMPOSER", willWrite); writeErr != nil {
							r.Error = writeErr.Error()
							log.Printf("[WARN] scan-composer-tags %s: write failed %s: %v", opID, w.filePath, writeErr)
						} else {
							r.Applied = true
							log.Printf("[INFO] scan-composer-tags %s: COMPOSER %q→%q %s", opID, composer, willWrite, w.filePath)
						}
					}
				}

				if opID != "" {
					resultJSON, _ := json.Marshal(r)
					_ = store.CreateOperationResult(&database.OperationResult{
						OperationID: opID,
						BookID:      w.filePath,
						ResultJSON:  string(resultJSON),
						Status:      r.Category,
					})
				}

				atomic.AddInt64(&completed, 1)
				mu.Lock()
				reporter.Increment()
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	finalCount := atomic.LoadInt64(&completed)
	log.Printf("[INFO] scan-composer-tags %s: finished %d/%d files", opID, finalCount, totalFiles)
	reporter.Log("info", fmt.Sprintf("scan complete: processed %d/%d files", finalCount, totalFiles), nil)
	return nil
}

// sct_result describes the COMPOSER field state for one audio file.
type sct_result struct {
	BookID    string `json:"book_id"`
	BookTitle string `json:"book_title"`
	FilePath  string `json:"file_path"`
	Category  string `json:"category"`
	Composer  string `json:"composer_on_disk"`
	Author    string `json:"author,omitempty"`
	Narrator  string `json:"narrator,omitempty"`
	WillWrite string `json:"will_write,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
	Error     string `json:"error,omitempty"`
}

// sct_work is one unit of work dispatched to the parallel reader pool.
type sct_work struct {
	bookID    string
	bookTitle string
	filePath  string
	author    string
	narrator  string
}

// sct_categorize returns the problem category and the value that should
// be written in the given fix_mode ("set_narrator" or "clear").
func sct_categorize(composer, author, narrator, fixMode string) (category, willWrite string) {
	composerLower := strings.ToLower(strings.TrimSpace(composer))
	authorLower := strings.ToLower(strings.TrimSpace(author))
	narratorLower := strings.ToLower(strings.TrimSpace(narrator))

	if fixMode == "set_narrator" {
		willWrite = strings.TrimSpace(narrator)
	} else {
		willWrite = ""
	}

	if strings.TrimSpace(composer) == "" {
		if fixMode == "set_narrator" && strings.TrimSpace(narrator) != "" {
			return "missing_narrator", strings.TrimSpace(narrator)
		}
		return "ok", ""
	}

	if author != "" && composerLower == authorLower {
		return "composer_equals_author", willWrite
	}
	if narrator != "" && composerLower == narratorLower {
		if fixMode == "set_narrator" {
			return "ok", strings.TrimSpace(narrator)
		}
		return "composer_equals_narrator", ""
	}
	return "composer_mismatch", willWrite
}
