// file: internal/activity/legacy_tag_enrichment.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-1234-abcd-ef0123456789

package activity

import (
	"strings"
)

// EnrichLegacyLogTags derives intelligent tags from legacy system_activity_log entries.
// Analyzes message content and level to infer action, outcome, source, and tier tags.
// Returns a de-duplicated list of tags that describe what the log entry represents.
func EnrichLegacyLogTags(message, source, level string) []string {
	seen := make(map[string]bool)
	var tags []string

	addTag := func(tag string) {
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}

	// Always include legacy marker
	addTag("legacy")

	// Detect action/log type from message patterns
	msgLower := strings.ToLower(message)

	// Compaction and maintenance operations
	if strings.Contains(msgLower, "compacted") || strings.Contains(msgLower, "compaction") {
		addTag("action:compact")
		addTag("tier:maintenance")
	}

	// Purge operations
	if strings.Contains(msgLower, "purged") || strings.Contains(msgLower, "purge") {
		addTag("action:purge")
	}

	// Scan operations
	if strings.Contains(msgLower, "scanned") || strings.Contains(msgLower, "scan") {
		addTag("action:scan")
	}

	// Metadata operations
	if strings.Contains(msgLower, "metadata") || strings.Contains(msgLower, "isbn") ||
		strings.Contains(msgLower, "enriched") {
		addTag("action:metadata-apply")
	}

	// Dedup operations
	if strings.Contains(msgLower, "dedup") || strings.Contains(msgLower, "duplicate") ||
		strings.Contains(msgLower, "merged") {
		addTag("action:dedup")
	}

	// Cover operations
	if strings.Contains(msgLower, "cover") {
		addTag("action:cover-update")
	}

	// Tag write operations
	if strings.Contains(msgLower, "tag") && strings.Contains(msgLower, "write") {
		addTag("action:tag-write")
	}

	// Organize/rename operations
	if strings.Contains(msgLower, "rename") || strings.Contains(msgLower, "organize") {
		addTag("action:organizer")
	}

	// Derive outcome from level
	switch strings.ToLower(level) {
	case "error":
		addTag("outcome:error")
	case "warning":
		addTag("outcome:warn")
	case "info", "debug":
		addTag("outcome:ok")
	}

	// Add source tag if present
	if source != "" {
		addTag("source:" + strings.ToLower(source))
	}

	return tags
}
