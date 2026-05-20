# Operation Context Logging Integration Example

## Before (Current)
```go
func (s *Server) runBulkMetadataFetchAll(
	ctx context.Context,
	opID string,
	params operations.BulkMetadataFetchParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	// ... setup code ...
	
	totalBooks := len(existingResults) + len(work)
	alreadyDone := len(existingResults)
	slog.Info("bulk-metadata-fetch books total, already cached, to fetch", 
		"opID", opID, "totalBooks", totalBooks, "alreadyDone", alreadyDone, "work_count", len(work))
	
	// ... more code with scattered opID logging ...
	
	for _, bw := range work {
		slog.Info("fetching metadata", "opID", opID, "bookID", bw.book.ID)
		// ... fetch logic ...
	}
	
	slog.Info("bulk-metadata-fetch done books — cached not_found", 
		"opID", opID, "finalCount", finalCount, "found", found, "notFound", notFound)
}
```

## After (With Operation Context)
```go
func (s *Server) runBulkMetadataFetchAll(
	ctx context.Context,
	opID string,
	params operations.BulkMetadataFetchParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	// Create operation context at entry point
	op := &logging.OpContext{
		ID:     opID,
		Type:   "metadata-fetch",
		Status: "pending",
	}
	ctx = logging.WithOp(ctx, op)
	
	// ... setup code ...
	
	totalBooks := len(existingResults) + len(work)
	alreadyDone := len(existingResults)
	
	// Log without repeating opID - it's automatically added from context
	logging.Info(ctx, "bulk-metadata-fetch starting", 
		"totalBooks", totalBooks, "alreadyDone", alreadyDone, "work_count", len(work))
	
	// Add books to operation context
	for _, bw := range work {
		op.AddEntity("books", bw.book.ID)
	}
	
	// ... more code - all logs automatically tagged with opID/opType/opStatus ...
	
	for _, bw := range work {
		logging.Info(ctx, "fetching metadata", "bookID", bw.book.ID)
		// ... fetch logic ...
	}
	
	// Update status at completion
	op.SetStatus("success")
	logging.Info(ctx, "bulk-metadata-fetch complete", 
		"finalCount", finalCount, "found", found, "notFound", notFound)
}
```

## Benefits

1. **No repetition** — opID/opType/opStatus automatically on every log
2. **Entity tracking** — track which books/genres/playlists were affected
3. **Propagation** — context flows through all called functions automatically
4. **UI integration** — UI can group all logs by opID

## Rollout Steps

1. Apply to metadata-fetch (largest operation)
2. Apply to dedup operations
3. Apply to organize operations
4. Expand to remaining operations as needed

