// file: internal/server/cache_snapshotter.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890
// last-edited: 2026-06-02

// Rescued from cache_handlers.go during Phase 2 handler extraction.
// These helpers support the background cache-stats snapshotter goroutine
// which is a server lifecycle concern, not an HTTP handler concern.

package server

import (
	"log/slog"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
	"github.com/prometheus/client_golang/prometheus"
)

// gatherCacheSnapshots reads the current Prometheus default registry and
// returns one CacheStatsSnapshot per cache name.
func gatherCacheSnapshots(now time.Time) []database.CacheStatsSnapshot {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil
	}
	stats := handlers.AggregateCacheMetrics(mfs)
	out := make([]database.CacheStatsSnapshot, 0, len(stats))
	for _, st := range stats {
		out = append(out, database.CacheStatsSnapshot{
			CacheName:        st.Name,
			Timestamp:        now,
			Hits:             st.Hits,
			Misses:           sumMap(st.Misses),
			Sets:             st.Sets,
			Invalidations:    sumMap(st.Invalidations),
			Evictions:        sumMap(st.Evictions),
			Size:             st.Size,
			GetDurationCount: st.GetDurationMetric.Count,
			GetDurationSum:   st.GetDurationMetric.Sum,
		})
	}
	return out
}

func sumMap(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

// runCacheStatsSnapshotter periodically captures the live Prometheus cache
// counters into the metrics sidecar store so trends survive restart.
func (s *Server) runCacheStatsSnapshotter(shutdown <-chan struct{}) {
	const (
		snapshotInterval = 5 * time.Minute
		retention        = 30 * 24 * time.Hour
	)
	if s.metricsStore == nil {
		return
	}
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-shutdown:
			return
		case now := <-ticker.C:
			snaps := gatherCacheSnapshots(now)
			if len(snaps) == 0 {
				continue
			}
			if err := s.metricsStore.RecordCacheStatsSnapshots(snaps); err != nil {
				slog.Warn("cache snapshotter record failed", "err", err)
			}
			if _, err := s.metricsStore.PruneCacheStatsHistory(now.Add(-retention)); err != nil {
				slog.Warn("cache snapshotter prune failed", "err", err)
			}
		}
	}
}
