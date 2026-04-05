// file: cmd/dedup_bench_summary.go
// version: 1.0.1
// guid: e5f6a7b8-c9d0-1234-efab-567890123456

//go:build bench

package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// generateSummary creates a cross-run comparison from all results.
func generateSummary(results []BenchRunResult, authorCount, groupCount int) *BenchSummary {
	return &BenchSummary{
		Timestamp:   time.Now().Format(time.RFC3339),
		AuthorCount: authorCount,
		GroupCount:  groupCount,
		Runs:        results,
	}
}

// writeSummaryMarkdown generates a human-readable markdown report.
func writeSummaryMarkdown(path string, summary *BenchSummary) error {
	var sb strings.Builder

	sb.WriteString("# AI Author Dedup Benchmark Results\n\n")
	sb.WriteString(fmt.Sprintf("**Run:** %s\n", summary.Timestamp))
	sb.WriteString(fmt.Sprintf("**Authors:** %d | **Heuristic Groups:** %d\n\n", summary.AuthorCount, summary.GroupCount))

	// Group by mode
	groupsRuns := []BenchRunResult{}
	fullRuns := []BenchRunResult{}
	for _, r := range summary.Runs {
		if r.Error != "" {
			continue
		}
		if r.Mode == "groups" {
			groupsRuns = append(groupsRuns, r)
		} else {
			fullRuns = append(fullRuns, r)
		}
	}

	for _, mode := range []string{"groups", "full"} {
		runs := groupsRuns
		if mode == "full" {
			runs = fullRuns
		}
		if len(runs) == 0 {
			continue
		}

		modeTitle := strings.ToUpper(mode[:1]) + mode[1:]
		sb.WriteString(fmt.Sprintf("## %s Mode\n\n", modeTitle))

		// Sort by number of suggestions (more = more thorough)
		sort.Slice(runs, func(i, j int) bool {
			return runs[i].NumSuggestions > runs[j].NumSuggestions
		})

		// Main results table
		sb.WriteString("| Model | Prompt | Temp | Suggestions | Merge | Split | Rename | Skip | Alias | Reclass | Chunks | Duration | Tokens (in/out) | Cost |\n")
		sb.WriteString("|-------|--------|------|-------------|-------|-------|--------|------|-------|---------|--------|----------|-----------------|------|\n")

		for _, r := range runs {
			dur := formatDuration(r.DurationMs)
			sb.WriteString(fmt.Sprintf("| %s | %s | %.1f | %d | %d | %d | %d | %d | %d | %d | %d | %s | %d/%d | $%.4f |\n",
				r.Config.Model,
				r.Config.PromptVariant,
				r.Config.Temperature,
				r.NumSuggestions,
				r.ActionCounts["merge"],
				r.ActionCounts["split"],
				r.ActionCounts["rename"],
				r.ActionCounts["skip"],
				r.ActionCounts["alias"],
				r.ActionCounts["reclassify"],
				r.NumChunks,
				dur,
				r.InputTokens,
				r.OutputTokens,
				r.CostEstimate,
			))
		}

		sb.WriteString("\n")

		// Confidence breakdown
		sb.WriteString("### Confidence Distribution\n\n")
		sb.WriteString("| Model | Prompt | High | Medium | Low |\n")
		sb.WriteString("|-------|--------|------|--------|-----|\n")
		for _, r := range runs {
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d |\n",
				r.Config.Model,
				r.Config.PromptVariant,
				r.ConfidenceCounts["high"],
				r.ConfidenceCounts["medium"],
				r.ConfidenceCounts["low"],
			))
		}
		sb.WriteString("\n")
	}

	// Error summary
	errorRuns := []BenchRunResult{}
	for _, r := range summary.Runs {
		if r.Error != "" {
			errorRuns = append(errorRuns, r)
		}
	}
	if len(errorRuns) > 0 {
		sb.WriteString("## Errors\n\n")
		for _, r := range errorRuns {
			sb.WriteString(fmt.Sprintf("- **%s** (%s, %s): %s\n",
				r.Config.Model, r.Config.PromptVariant, r.Mode, r.Error))
		}
		sb.WriteString("\n")
	}

	// Cost summary
	sb.WriteString("## Cost Summary\n\n")
	var totalCost float64
	for _, r := range summary.Runs {
		totalCost += r.CostEstimate
	}
	sb.WriteString(fmt.Sprintf("**Total estimated cost:** $%.4f\n\n", totalCost))

	return os.WriteFile(path, []byte(sb.String()), 0664)
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%.1fm", float64(ms)/60000)
}
