// file: internal/itunes/service/location_normalize.go
// version: 1.0.0
// guid: 7b2c5e91-3a48-4d6f-9c10-2e8a6b3f47d1

// Writeback-side location normalization (fable5 TASK-006, CRIT-2).
//
// WHY this helper exists: every writer call site that builds an
// itunes.ITLLocationUpdate from a BookFile's f.ITunesPath (or a deferred
// update's NewPath) must funnel that raw value through ONE normalizer. The DB
// column has historically held BOTH native Windows paths and file:// URLs (the
// CRIT-2 root cause — URL-shaped values copied verbatim into 0x0D, 83,783 blocks
// in damaged-1/3). Normalizing here means:
//
//   - the canonical WINPATH is stored in ITLLocationUpdate.NewLocation (the LE
//     writer derives the 0x0B URL from it — single source of truth, SPEC §1b);
//   - unmappable values (relative paths, .itunes-writeback/ staging leaks,
//     podcast http(s) URLs that have no 0x0D) are SKIPPED with a per-item WARN
//     and a Prometheus counter — never written raw;
//   - the DB is NOT mutated (TASK-006 explicitly defers backfill).

package itunesservice

import (
	"log/slog"
	"strings"

	"github.com/falkcorp/audiobook-organizer/internal/itunes"
	"github.com/falkcorp/audiobook-organizer/internal/metrics"
)

// normalizeITunesLocation converts a raw f.ITunesPath / NewPath value (path OR
// URL) into the canonical Windows path for an ITLLocationUpdate. On success it
// returns (winPath, true). On an unmappable value it logs a WARN, increments the
// itunes_location_unmappable_total{reason} counter, and returns ("", false) so the
// caller drops the update rather than writing a corrupt value.
func normalizeITunesLocation(pid, raw string) (string, bool) {
	pair, err := itunes.NewLocationPair(raw)
	if err != nil {
		reason := "invalid_path"
		if low := strings.ToLower(strings.TrimSpace(raw)); strings.HasPrefix(low, "http://") ||
			strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "file://") {
			reason = "url_unmappable"
		}
		metrics.RecordITunesLocationUnmappable(reason)
		slog.Warn("iTunes writeback: skipping unmappable location (never written raw — CRIT-2)",
			"pid", pid, "raw", raw, "reason", reason, "error", err.Error())
		return "", false
	}
	return pair.WinPath, true
}
