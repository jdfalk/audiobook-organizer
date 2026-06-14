// file: internal/operations/registry/deps.go
// version: 1.0.0
// guid: f2a3b4c5-d6e7-8f9a-0b1c-2d3e4f5a6b7c
// last-edited: 2026-06-13

// deps.go implements the UOS M1 requirement evaluator, AllSatisfied aggregator,
// and cycle-detection guard for OperationDef.Requires graphs.
//
// Design constraints:
//   - Pure evaluator: no I/O side-effects, no goroutines, no locks.
//   - DepStore is a narrow interface (4 methods) so the evaluator is testable
//     with a fake without importing PebbleDB.
//   - database.OpSubject is the persisted shape; registry.Subject is the
//     in-process shape. Conversion happens at the call boundary in this file.
//   - ReqFieldSet always returns (false, "not implemented in M1", nil).
//     No error — callers can distinguish "not satisfied" from "broken".

package registry

import (
	"fmt"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// DepStore is the narrow persistence interface the evaluator needs.
// It is satisfied by *database.PebbleStore in production and by fakeDepStore
// in tests. The full database.OpsV2Store interface is not required here.
type DepStore interface {
	// GetDepRev returns the current freshness counter for sub.
	// Returns 0 and no error when no counter exists yet.
	GetDepRev(sub database.OpSubject) (uint64, error)

	// GetOpCompletion returns the dep_rev at which opType last completed for
	// sub at the book level. ok=false when no record exists.
	GetOpCompletion(sub database.OpSubject, opType string) (rev uint64, ok bool, err error)

	// ListFileCompletions returns a map of fileID→depRev for all per-file
	// completion records of opType on sub.
	ListFileCompletions(sub database.OpSubject, opType string) (map[string]uint64, error)

	// BookFiles returns the list of file IDs belonging to bookID.
	// Used when evaluating AllFiles=true requirements.
	BookFiles(bookID string) ([]string, error)
}

// subjectToOpSubject converts the registry's Subject to the database wire type.
func subjectToOpSubject(sub Subject) database.OpSubject {
	return database.OpSubject{Type: sub.Type, ID: sub.ID}
}

// Satisfied evaluates a single Requirement against the current state in store.
//
// Returns:
//   - (true, "", nil) when the requirement is met
//   - (false, reason, nil) when unmet — reason is human-readable for logs/UI
//   - (false, "", err) when a store error prevents evaluation
func Satisfied(store DepStore, req Requirement, sub Subject) (bool, string, error) {
	switch req.Kind {
	case ReqOpCompleted:
		return evalOpCompleted(store, req, sub)
	case ReqFieldSet:
		// M1 stub: field_set requirements are defined in the type system but
		// not yet evaluated. Return false with a clear reason, no error.
		return false, fmt.Sprintf("field_set requirement on %q not implemented in M1", req.Field), nil
	default:
		return false, fmt.Sprintf("unknown requirement kind %q", req.Kind), nil
	}
}

// AllSatisfied evaluates every requirement in reqs. Returns true only when all
// are met. On first unmet requirement, returns (false, firstUnmet, nil) where
// firstUnmet is the human-readable reason from Satisfied. Returns an error only
// when a store error is encountered.
func AllSatisfied(store DepStore, reqs []Requirement, sub Subject) (bool, string, error) {
	for _, req := range reqs {
		ok, reason, err := Satisfied(store, req, sub)
		if err != nil {
			return false, "", err
		}
		if !ok {
			return false, reason, nil
		}
	}
	return true, "", nil
}

// evalOpCompleted evaluates a ReqOpCompleted requirement.
// A completion is fresh when its stored dep_rev equals the subject's current dep_rev.
// If AllFiles is true, every file of the book must have its own fresh completion record.
func evalOpCompleted(store DepStore, req Requirement, sub Subject) (bool, string, error) {
	dbSub := subjectToOpSubject(sub)
	curRev, err := store.GetDepRev(dbSub)
	if err != nil {
		return false, "", fmt.Errorf("GetDepRev(%v): %w", sub, err)
	}

	if req.AllFiles {
		return evalAllFilesCompleted(store, req, sub, dbSub, curRev)
	}

	// Book-level completion check.
	completionRev, ok, err := store.GetOpCompletion(dbSub, req.OpType)
	if err != nil {
		return false, "", fmt.Errorf("GetOpCompletion(%v, %q): %w", sub, req.OpType, err)
	}
	if !ok {
		return false, fmt.Sprintf("op %q has never completed for %s/%s", req.OpType, sub.Type, sub.ID), nil
	}
	if completionRev < curRev {
		return false, fmt.Sprintf("op %q completion at rev %d is stale (current dep_rev=%d) for %s/%s",
			req.OpType, completionRev, curRev, sub.Type, sub.ID), nil
	}
	return true, "", nil
}

// evalAllFilesCompleted checks that every file of the book has a per-file
// completion record at the current dep_rev.
func evalAllFilesCompleted(store DepStore, req Requirement, sub Subject, dbSub database.OpSubject, curRev uint64) (bool, string, error) {
	if sub.Type != "book" {
		return false, fmt.Sprintf("AllFiles=true is only supported for book subjects, got %q", sub.Type), nil
	}

	files, err := store.BookFiles(sub.ID)
	if err != nil {
		return false, "", fmt.Errorf("BookFiles(%q): %w", sub.ID, err)
	}
	if len(files) == 0 {
		// No files on record → treat as unmet: the book may not be scanned yet.
		return false, fmt.Sprintf("op %q AllFiles=true: book %q has no files on record", req.OpType, sub.ID), nil
	}

	fileRevs, err := store.ListFileCompletions(dbSub, req.OpType)
	if err != nil {
		return false, "", fmt.Errorf("ListFileCompletions(%v, %q): %w", sub, req.OpType, err)
	}

	for _, fileID := range files {
		rev, ok := fileRevs[fileID]
		if !ok {
			return false, fmt.Sprintf("op %q AllFiles=true: file %q has no completion record for %s/%s",
				req.OpType, fileID, sub.Type, sub.ID), nil
		}
		if rev < curRev {
			return false, fmt.Sprintf("op %q AllFiles=true: file %q completion at rev %d is stale (current dep_rev=%d)",
				req.OpType, fileID, rev, curRev), nil
		}
	}
	return true, "", nil
}

// CheckRequirementCycle checks whether the requirement graph defined by
// defReqsByOpType contains any cycle. The map keys are op-type IDs and the
// values are their Requires slices (only ReqOpCompleted edges are traversed;
// ReqFieldSet has no graph edge).
//
// Returns a non-nil error naming the cycle when one exists.
// This is intended to run once at registry startup, not per-enqueue.
func CheckRequirementCycle(defReqsByOpType map[string][]Requirement) error {
	// Standard DFS with three-color marking:
	//   white (0) = unvisited
	//   grey  (1) = in current DFS stack (back-edge = cycle)
	//   black (2) = fully processed (cross/forward edges are fine)
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := make(map[string]int, len(defReqsByOpType))

	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = grey
		for _, req := range defReqsByOpType[node] {
			if req.Kind != ReqOpCompleted {
				continue // only op-completed edges form graph edges
			}
			neighbor := req.OpType
			switch color[neighbor] {
			case grey:
				return fmt.Errorf("requirement cycle detected: %q → %q forms a cycle", node, neighbor)
			case white:
				if err := dfs(neighbor); err != nil {
					return err
				}
			}
			// black = already fully processed, skip
		}
		color[node] = black
		return nil
	}

	for node := range defReqsByOpType {
		if color[node] == white {
			if err := dfs(node); err != nil {
				return err
			}
		}
	}
	return nil
}
