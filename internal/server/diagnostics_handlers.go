// file: internal/server/diagnostics_handlers.go
// version: 1.4.0
// guid: a2b3c4d5-e6f7-4890-ab12-cd34ef56gh78

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// diagnosticsSuggestion represents a single AI suggestion from diagnostics analysis.
type diagnosticsSuggestion struct {
	ID        string   `json:"id"`
	Action    string   `json:"action"`
	BookIDs   []string `json:"book_ids"`
	PrimaryID string   `json:"primary_id,omitempty"`
	Reason    string   `json:"reason"`
	Fix       string   `json:"fix,omitempty"`
}

// startDiagnosticsExport creates a diagnostic ZIP export asynchronously.
func (s *Server) startDiagnosticsExport(c *gin.Context) {
	var req struct {
		Category    string `json:"category"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Category == "" {
		req.Category = "general"
	}

	validCategories := map[string]bool{
		"deduplication":    true,
		"error_analysis":   true,
		"metadata_quality": true,
		"general":          true,
	}
	if !validCategories[req.Category] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category; must be one of: deduplication, error_analysis, metadata_quality, general"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	opID := ulid.Make().String()
	_, err := store.CreateOperation(opID, "diagnostics_export", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	ds := s.diagnosticsService
	if ds == nil {
		ds = NewDiagnosticsService(store, nil, config.AppConfig.ITunesLibraryReadPath)
	}

	category := req.Category
	description := req.Description

	if s.queue != nil {
		_ = s.queue.Enqueue(opID, "diagnostics_export", 5, func(_ context.Context, _ operations.ProgressReporter) error {
			zipPath, genErr := ds.GenerateExport(category, description)
			if genErr != nil {
				_ = store.UpdateOperationError(opID, genErr.Error())
				return genErr
			}
			resultJSON, _ := json.Marshal(map[string]string{"zip_path": zipPath})
			_ = store.UpdateOperationResultData(opID, string(resultJSON))
			_ = store.UpdateOperationStatus(opID, "completed", 100, 100, "Export complete")
			return nil
		})
	} else {
		// Synchronous fallback if no queue
		go func() {
			zipPath, genErr := ds.GenerateExport(category, description)
			if genErr != nil {
				_ = store.UpdateOperationError(opID, genErr.Error())
				return
			}
			resultJSON, _ := json.Marshal(map[string]string{"zip_path": zipPath})
			_ = store.UpdateOperationResultData(opID, string(resultJSON))
			_ = store.UpdateOperationStatus(opID, "completed", 100, 100, "Export complete")
		}()
	}

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"status":       "generating",
	})
}

// downloadDiagnosticsExport serves the ZIP file for a completed diagnostics export.
func (s *Server) downloadDiagnosticsExport(c *gin.Context) {
	opID := c.Param("operationId")

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	if op.Status != "completed" {
		c.JSON(http.StatusAccepted, gin.H{
			"operation_id": opID,
			"status":       op.Status,
			"message":      op.Message,
		})
		return
	}

	if op.ResultData == nil || *op.ResultData == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no result data available"})
		return
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(*op.ResultData), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse result data"})
		return
	}

	zipPath := result["zip_path"]
	if zipPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "zip path not found in result"})
		return
	}

	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export file no longer available"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=diagnostics-%s.zip", opID))
	c.File(zipPath)
}

// submitDiagnosticsAI generates a diagnostics export and submits it to OpenAI batch API.
func (s *Server) submitDiagnosticsAI(c *gin.Context) {
	if config.AppConfig.OpenAIAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenAI API key not configured"})
		return
	}

	var req struct {
		Category    string `json:"category"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Category == "" {
		req.Category = "general"
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	opID := ulid.Make().String()
	_, err := store.CreateOperation(opID, "diagnostics_ai", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	ds := s.diagnosticsService
	if ds == nil {
		ds = NewDiagnosticsService(store, nil, config.AppConfig.ITunesLibraryReadPath)
	}

	category := req.Category
	description := req.Description

	// Get the AI parser for file upload and batch creation
	var parser *ai.OpenAIParser
	if s.batchPoller != nil {
		parser = s.batchPoller.parser
	}

	// Run async: generate export, build JSONL, submit to OpenAI
	go func() {
		_ = store.UpdateOperationStatus(opID, "running", 10, 100, "Generating export data")

		// Collect books for JSONL
		allBooks, collectErr := ds.collectAllBooks()
		if collectErr != nil {
			_ = store.UpdateOperationError(opID, collectErr.Error())
			return
		}

		slimBooks := make([]slimBook, len(allBooks))
		for i, b := range allBooks {
			slimBooks[i] = toSlimBook(b)
		}

		_ = store.UpdateOperationStatus(opID, "running", 50, 100, "Building batch JSONL")

		jsonlData, buildErr := buildBatchJSONL(category, description, slimBooks, nil, nil, nil)
		if buildErr != nil {
			_ = store.UpdateOperationError(opID, buildErr.Error())
			return
		}

		// Count request lines
		requestCount := 0
		for _, b := range jsonlData {
			if b == '\n' {
				requestCount++
			}
		}
		if len(jsonlData) > 0 && jsonlData[len(jsonlData)-1] != '\n' {
			requestCount++
		}

		_ = store.UpdateOperationStatus(opID, "running", 70, 100, "Submitting to OpenAI batch API")

		// Upload JSONL and create batch with metadata tagging
		if parser != nil {
			ctx := context.Background()
			buf := bytes.NewBuffer(jsonlData)
			file, uploadErr := parser.UploadBatchFile(ctx, buf)
			if uploadErr != nil {
				_ = store.UpdateOperationError(opID, fmt.Sprintf("upload batch file: %v", uploadErr))
				return
			}

			batchID, createErr := parser.CreateBatchWithMetadata(ctx, file, "diagnostics")
			if createErr != nil {
				_ = store.UpdateOperationError(opID, fmt.Sprintf("create batch: %v", createErr))
				return
			}

			resultJSON, _ := json.Marshal(map[string]interface{}{
				"status":        "submitted",
				"request_count": requestCount,
				"batch_id":      batchID,
			})
			_ = store.UpdateOperationResultData(opID, string(resultJSON))
			_ = store.UpdateOperationStatus(opID, "running", 80, 100, fmt.Sprintf("Batch %s submitted, awaiting completion", batchID))
			log.Printf("[INFO] diagnostics_ai: batch %s submitted with %d requests", batchID, requestCount)
		} else {
			// Fallback: store JSONL metadata without actual submission
			resultJSON, _ := json.Marshal(map[string]interface{}{
				"status":        "submitted",
				"request_count": requestCount,
				"batch_id":      "pending-" + opID,
			})
			_ = store.UpdateOperationResultData(opID, string(resultJSON))
			_ = store.UpdateOperationStatus(opID, "completed", 100, 100, "Batch data prepared (no AI parser available)")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"status":       "submitted",
	})
}

// getDiagnosticsAIResults returns the AI analysis results for a diagnostics operation.
func (s *Server) getDiagnosticsAIResults(c *gin.Context) {
	opID := c.Param("operationId")

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	if op.Status != "completed" {
		c.JSON(http.StatusOK, gin.H{
			"operation_id": opID,
			"status":       op.Status,
			"message":      op.Message,
		})
		return
	}

	if op.ResultData == nil || *op.ResultData == "" {
		c.JSON(http.StatusOK, gin.H{
			"status":      "completed",
			"suggestions": []interface{}{},
		})
		return
	}

	var resultData map[string]interface{}
	if err := json.Unmarshal([]byte(*op.ResultData), &resultData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse result data"})
		return
	}

	suggestions, _ := resultData["suggestions"]
	if suggestions == nil {
		suggestions = []interface{}{}
	}
	rawResponses, _ := resultData["raw_responses"]
	if rawResponses == nil {
		rawResponses = []interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        op.Status,
		"suggestions":   suggestions,
		"raw_responses": rawResponses,
	})
}

// applyDiagnosticsSuggestions applies approved AI suggestions from diagnostics analysis.
func (s *Server) applyDiagnosticsSuggestions(c *gin.Context) {
	var req struct {
		OperationID           string   `json:"operation_id" binding:"required"`
		ApprovedSuggestionIDs []string `json:"approved_suggestion_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	op, err := store.GetOperationByID(req.OperationID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	if op.ResultData == nil || *op.ResultData == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no suggestions available"})
		return
	}

	var resultData map[string]interface{}
	if err := json.Unmarshal([]byte(*op.ResultData), &resultData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse result data"})
		return
	}

	suggestionsRaw, ok := resultData["suggestions"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no suggestions in result data"})
		return
	}

	suggestionsJSON, _ := json.Marshal(suggestionsRaw)
	var suggestions []diagnosticsSuggestion
	if err := json.Unmarshal(suggestionsJSON, &suggestions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse suggestions"})
		return
	}

	// Build lookup of approved IDs
	approvedSet := make(map[string]bool)
	for _, id := range req.ApprovedSuggestionIDs {
		approvedSet[id] = true
	}

	ms := s.mergeService
	if ms == nil {
		ms = merge.NewService(store)
	}

	applied := 0
	failed := 0
	var errors []string

	for _, suggestion := range suggestions {
		if !approvedSet[suggestion.ID] {
			continue
		}

		var applyErr error
		switch suggestion.Action {
		case "merge_versions":
			if len(suggestion.BookIDs) >= 2 {
				_, applyErr = ms.MergeBooks(suggestion.BookIDs, suggestion.PrimaryID)
			}

		case "delete_orphan":
			for _, bookID := range suggestion.BookIDs {
				book, getErr := store.GetBookByID(bookID)
				if getErr != nil || book == nil {
					applyErr = fmt.Errorf("book %s not found", bookID)
					break
				}
				marked := true
				book.MarkedForDeletion = &marked
				if _, updateErr := store.UpdateBook(book.ID, book); updateErr != nil {
					applyErr = updateErr
					break
				}
			}

		case "fix_metadata":
			// Fix field is a JSON string with field updates
			if suggestion.Fix != "" && len(suggestion.BookIDs) > 0 {
				var fixes map[string]interface{}
				if parseErr := json.Unmarshal([]byte(suggestion.Fix), &fixes); parseErr != nil {
					applyErr = fmt.Errorf("invalid fix data: %w", parseErr)
				} else {
					for _, bookID := range suggestion.BookIDs {
						book, getErr := store.GetBookByID(bookID)
						if getErr != nil || book == nil {
							applyErr = fmt.Errorf("book %s not found", bookID)
							break
						}
						if title, ok := fixes["title"].(string); ok {
							book.Title = title
						}
						if _, updateErr := store.UpdateBook(book.ID, book); updateErr != nil {
							applyErr = updateErr
							break
						}
					}
				}
			}

		case "reassign_series":
			if suggestion.Fix != "" && len(suggestion.BookIDs) > 0 {
				var fixes map[string]interface{}
				if parseErr := json.Unmarshal([]byte(suggestion.Fix), &fixes); parseErr != nil {
					applyErr = fmt.Errorf("invalid fix data: %w", parseErr)
				} else {
					if seriesIDFloat, ok := fixes["series_id"].(float64); ok {
						seriesID := int(seriesIDFloat)
						for _, bookID := range suggestion.BookIDs {
							book, getErr := store.GetBookByID(bookID)
							if getErr != nil || book == nil {
								applyErr = fmt.Errorf("book %s not found", bookID)
								break
							}
							book.SeriesID = &seriesID
							if _, updateErr := store.UpdateBook(book.ID, book); updateErr != nil {
								applyErr = updateErr
								break
							}
						}
					}
				}
			}

		default:
			applyErr = fmt.Errorf("unknown action: %s", suggestion.Action)
		}

		if applyErr != nil {
			failed++
			errors = append(errors, fmt.Sprintf("suggestion %s: %v", suggestion.ID, applyErr))
			log.Printf("[WARN] Failed to apply diagnostics suggestion %s: %v", suggestion.ID, applyErr)
		} else {
			applied++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"applied": applied,
		"failed":  failed,
		"errors":  errors,
	})
}
