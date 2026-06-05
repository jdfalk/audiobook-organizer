// file: internal/plugins/dedup/llm_review.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901
// last-edited: 2026-05-06

package dedup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) llmReviewDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "dedup.llm-review",
		Plugin:          "dedup",
		DisplayName:     "LLM review of candidates",
		Description:     "Runs LLM review pass over ambiguous embedding-layer candidates.",
		ResumePolicy:    sdk.ResumeDrop,
		DefaultPriority: sdk.PriorityLow,
		Timeout:         120 * time.Minute,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapNetworkOpenAI,
		},
		Run: p.runLLMReview,
	}
}

func (p *Plugin) runLLMReview(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	if p.engine == nil {
		return fmt.Errorf("dedup engine not available")
	}

	prog := sdk.NewProgress(reporter, 0)
	prog.Start("Starting LLM review of ambiguous candidates...")
	if err := p.engine.RunLLMReview(ctx); err != nil {
		reporter.Logger().Error("LLM review error", "error", err)
		return fmt.Errorf("LLM review: %w", err)
	}
	prog.Finalize("writing results...")
	prog.Done("LLM review complete")
	return nil
}
