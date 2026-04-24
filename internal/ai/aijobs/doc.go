// file: internal/ai/aijobs/doc.go
// version: 1.0.0
// guid: 238dd64d-bf97-4fdb-a092-079dce106fac

// Package aijobs routes bulk-scale LLM chat-completion work through the
// OpenAI Batch API. It sits on top of internal/server.BatchPoller and the
// internal/ai.OpenAIParser batch helpers.
//
// Usage for a feature:
//
//	aijobs.Register("my_feature", func(ctx context.Context, deps aijobs.Deps, itemsJSON []byte, results []aijobs.RowResult) (int, int, []database.AIJobRowError, error) {
//	    // 1. Deserialize items from itemsJSON (feature-specific type)
//	    // 2. For each row in results, match by CustomID to an item
//	    // 3. Apply the result (feature-specific DB mutation), capturing per-row errors
//	    // 4. Return (successCount, errorCount, rowErrors, nil)
//	})
//
//	jobID, err := aijobs.Submit(ctx, deps, aijobs.SubmitRequest{
//	    Type:  "my_feature",
//	    Items: myItems,
//	    Build: func(i int, item MyItem) (aijobs.BatchRequest, error) { ... },
//	})
//
// Synchronous callers stay on internal/ai directly; bulk callers go through Submit.
// The split is enforced by internal/ai.priority_marker_test.go.
package aijobs
