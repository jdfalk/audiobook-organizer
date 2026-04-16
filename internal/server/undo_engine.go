// file: internal/server/undo_engine.go
// version: 1.0.0
// guid: 0b8c9d6e-1f7a-4a70-b8c5-3d7e0f1b9a99
//
// Undo engine (spec 3.2 task 3). Reverses the destructive changes
// recorded by a prior operation by walking its operation_changes
// rows in reverse order and applying the inverse transform.
//
// Supported change_type reversals:
//   - file_move / organize_rename: os.Rename(new_value → old_value)
//   - metadata_update / db_update: restore field from old_value
//   - dir_create: remove directory if empty
//   - tag_write: no-op (tags are idempotently re-derived)
//
// Each change row is marked reverted_at on success; already-reverted
// rows are skipped (idempotent for resume after crash).

package server

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// UndoResult summarizes the outcome of an undo operation.
type UndoResult struct {
	Reverted        int      `json:"reverted"`
	SkippedConflict int      `json:"skipped_conflict"`
	SkippedReverted int      `json:"skipped_reverted"`
	Failed          int      `json:"failed"`
	Errors          []string `json:"errors,omitempty"`
}

// RunUndoOperation loads the changes for targetOpID, walks them in
// reverse order, and applies the inverse of each change. Progress
// is reported via the callback (step description + percentage).
func RunUndoOperation(
	store database.Store,
	targetOpID string,
	progress func(step string, pct int),
) (*UndoResult, error) {
	if progress == nil {
		progress = func(string, int) {}
	}

	changes, err := store.GetOperationChanges(targetOpID)
	if err != nil {
		return nil, fmt.Errorf("load operation changes: %w", err)
	}
	if len(changes) == 0 {
		return &UndoResult{}, nil
	}

	// Reverse order: last change undone first.
	reversed := make([]*database.OperationChange, len(changes))
	for i, c := range changes {
		reversed[len(changes)-1-i] = c
	}

	result := &UndoResult{}
	total := len(reversed)

	for i, change := range reversed {
		pct := int(float64(i+1) / float64(total) * 100)
		progress(fmt.Sprintf("undo %s on %s", change.ChangeType, change.BookID), pct)

		if change.RevertedAt != nil {
			result.SkippedReverted++
			continue
		}

		err := revertChange(store, change)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", change.BookID, change.ChangeType, err))
			continue
		}

		now := time.Now()
		change.RevertedAt = &now
		if updateErr := store.CreateOperationChange(change); updateErr != nil {
			// Best effort — the FS/DB change already succeeded.
			result.Errors = append(result.Errors, fmt.Sprintf("mark reverted %s: %v", change.ID, updateErr))
		}
		result.Reverted++
	}

	return result, nil
}

// revertChange applies the inverse of a single operation change.
func revertChange(store database.Store, change *database.OperationChange) error {
	switch change.ChangeType {
	case "file_move", "organize_rename":
		return revertFileMove(change)
	case "metadata_update", "db_update":
		return revertMetadataUpdate(store, change)
	case "dir_create":
		return revertDirCreate(change)
	case "tag_write":
		return nil // tags are re-derived on next metadata apply
	default:
		return fmt.Errorf("unknown change_type: %s", change.ChangeType)
	}
}

// revertFileMove renames new_value back to old_value. If old_value
// already exists (conflict), returns an error rather than clobbering.
func revertFileMove(change *database.OperationChange) error {
	if change.OldValue == "" || change.NewValue == "" {
		return fmt.Errorf("missing path in change %s", change.ID)
	}

	// Check that new_value (the current location) exists.
	if _, err := os.Stat(change.NewValue); os.IsNotExist(err) {
		// Already moved back or deleted — idempotent no-op.
		return nil
	}

	// Check that old_value (restore target) doesn't already exist.
	if _, err := os.Stat(change.OldValue); err == nil {
		return fmt.Errorf("conflict: restore target already exists: %s", change.OldValue)
	}

	// Ensure parent directory of old_value exists.
	if err := os.MkdirAll(parentDir(change.OldValue), 0o775); err != nil {
		return fmt.Errorf("mkdir for restore: %w", err)
	}

	return os.Rename(change.NewValue, change.OldValue)
}

// revertMetadataUpdate restores a book field from the change's
// OldValue. OldValue is either a plain string (for single-field
// changes) or a JSON object (for multi-field snapshots).
func revertMetadataUpdate(store database.Store, change *database.OperationChange) error {
	if change.BookID == "" {
		return fmt.Errorf("no book_id on metadata change %s", change.ID)
	}

	book, err := store.GetBookByID(change.BookID)
	if err != nil || book == nil {
		return fmt.Errorf("book %s not found", change.BookID)
	}

	if change.FieldName != "" && change.OldValue != "" {
		applyFieldRestore(book, change.FieldName, change.OldValue)
	} else if change.OldValue != "" {
		// Multi-field JSON snapshot.
		var snapshot map[string]interface{}
		if err := json.Unmarshal([]byte(change.OldValue), &snapshot); err == nil {
			for field, val := range snapshot {
				if s, ok := val.(string); ok {
					applyFieldRestore(book, field, s)
				}
			}
		}
	}

	_, err = store.UpdateBook(book.ID, book)
	return err
}

// applyFieldRestore sets a single field on a Book from a string value.
func applyFieldRestore(book *database.Book, field, value string) {
	switch field {
	case "title":
		book.Title = value
	case "file_path":
		book.FilePath = value
	case "format":
		book.Format = value
	case "narrator":
		book.Narrator = &value
	case "edition":
		book.Edition = &value
	case "description":
		book.Description = &value
	case "language":
		book.Language = &value
	case "publisher":
		book.Publisher = &value
	case "genre":
		book.Genre = &value
	case "library_state":
		book.LibraryState = &value
	}
}

// revertDirCreate removes a directory if it's empty. Non-empty
// directories are left alone (the files inside may still be needed).
func revertDirCreate(change *database.OperationChange) error {
	if change.NewValue == "" {
		return nil
	}
	entries, err := os.ReadDir(change.NewValue)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // already gone
		}
		return err
	}
	if len(entries) > 0 {
		return nil // not empty, leave it
	}
	return os.Remove(change.NewValue)
}

func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
