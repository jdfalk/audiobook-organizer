// file: internal/server/version_swap.go
// version: 1.1.0
// guid: 6c3d5a2e-8b4c-4a70-b8c5-3d7e0f1b9a99
//
// Thin wrappers delegating to internal/versions package.

package server

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/versions"
)

// VersionSwapParams is an alias for versions.VersionSwapParams.
type VersionSwapParams = versions.VersionSwapParams

// RunVersionSwap delegates to versions.RunVersionSwap, wiring up the
// WriteBackBatcher and NotifyDelugeAfterVersionSwap callbacks.
func RunVersionSwap(
	ctx context.Context,
	store database.Store,
	params VersionSwapParams,
	progress func(step string, pct int),
	batcher Enqueuer,
) error {
	var onWriteBack func(bookID string)
	if batcher != nil {
		onWriteBack = func(id string) { batcher.Enqueue(id) }
	}

	return versions.RunVersionSwap(ctx, store, params, progress, onWriteBack,
		func(st database.Store, from, to *database.BookVersion, bookFilePath string) {
			NotifyDelugeAfterVersionSwap(st, from, to, bookFilePath)
		},
	)
}

// ResumeVersionSwaps delegates to versions.ResumeVersionSwaps.
func ResumeVersionSwaps(ctx context.Context, store database.Store) {
	versions.ResumeVersionSwaps(ctx, store)
}
