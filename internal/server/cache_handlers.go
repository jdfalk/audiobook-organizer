// file: internal/server/cache_handlers.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/cache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_model/go"
)

// CacheStatsResponse represents the JSON response for GET /api/v1/cache/stats
type CacheStatsResponse struct {
	Caches    []CacheStat `json:"caches"`
	GeneratedAt string     `json:"generated_at"`
}

// CacheStat represents metrics for a single cache
type CacheStat struct {
	Name              string             `json:"name"`
	Hits              int64              `json:"hits"`
	Misses            map[string]int64   `json:"misses"`
	Sets              int64              `json:"sets"`
	Invalidations     map[string]int64   `json:"invalidations"`
	Evictions         map[string]int64   `json:"evictions"`
	Size              int64              `json:"size"`
	HitRate           *float64           `json:"hit_rate,omitempty"`
	GetDurationMetric GetDurationMetric  `json:"get_duration_seconds"`
}

// GetDurationMetric represents count and sum of cache get durations
type GetDurationMetric struct {
	Count int64   `json:"count"`
	Sum   float64 `json:"sum"`
}

// handleCacheStats returns aggregated cache metrics from Prometheus default registry
// GET /api/v1/cache/stats (public, no auth)
func (s *Server) handleCacheStats(c *gin.Context) {
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to gather metrics"})
		return
	}

	stats := aggregateCacheMetrics(metrics)
	resp := CacheStatsResponse{
		Caches:      stats,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, resp)
}

// handleCacheKeysIntrospection returns key names for a specific cache
// GET /api/v1/cache/stats/keys?cache=<name> (admin-gated)
func (s *Server) handleCacheKeysIntrospection(c *gin.Context) {
	cacheName := c.Query("cache")
	if cacheName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cache parameter required"})
		return
	}

	// Check if it's a cache we can introspect
	cacheInst, ok := cache.Lookup(cacheName)
	if !ok {
		// Cache not found — check if it's a known non-introspectable name
		if isNonIntrospectableCache(cacheName) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "not introspectable",
				"cache": cacheName,
			})
			return
		}
		// Unknown cache
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cache not found",
			"cache": cacheName,
		})
		return
	}

	keys := cacheInst.Keys()
	c.JSON(http.StatusOK, gin.H{
		"cache": cacheName,
		"keys":  keys,
		"count": len(keys),
	})
}

// aggregateCacheMetrics extracts all audiobook_organizer_cache_* metrics from Prometheus
// and builds a CacheStat for each unique cache name.
func aggregateCacheMetrics(mfs []*io_prometheus_client.MetricFamily) []CacheStat {
	// Map from cache name to its aggregated stats
	statMap := make(map[string]*CacheStat)

	for _, mf := range mfs {
		// Only process cache metrics
		if mf.Name == nil || *mf.Name == "" {
			continue
		}
		metricName := *mf.Name

		switch metricName {
		case "audiobook_organizer_cache_hits_total":
			processCounterMetric(mf, statMap, func(stat *CacheStat, value int64) {
				stat.Hits = value
			})

		case "audiobook_organizer_cache_misses_total":
			processCounterMetricWithReason(mf, statMap, func(stat *CacheStat, reason string, value int64) {
				if stat.Misses == nil {
					stat.Misses = make(map[string]int64)
				}
				stat.Misses[reason] = value
			})

		case "audiobook_organizer_cache_sets_total":
			processCounterMetric(mf, statMap, func(stat *CacheStat, value int64) {
				stat.Sets = value
			})

		case "audiobook_organizer_cache_invalidations_total":
			processCounterMetricWithReason(mf, statMap, func(stat *CacheStat, scope string, value int64) {
				if stat.Invalidations == nil {
					stat.Invalidations = make(map[string]int64)
				}
				stat.Invalidations[scope] = value
			})

		case "audiobook_organizer_cache_evictions_total":
			processCounterMetricWithReason(mf, statMap, func(stat *CacheStat, reason string, value int64) {
				if stat.Evictions == nil {
					stat.Evictions = make(map[string]int64)
				}
				stat.Evictions[reason] = value
			})

		case "audiobook_organizer_cache_size":
			processGaugeMetric(mf, statMap, func(stat *CacheStat, value float64) {
				stat.Size = int64(value)
			})

		case "audiobook_organizer_cache_get_duration_seconds":
			processHistogramMetric(mf, statMap, func(stat *CacheStat, count int64, sum float64) {
				stat.GetDurationMetric = GetDurationMetric{Count: count, Sum: sum}
			})
		}
	}

	// Convert to sorted slice and compute hit rates
	result := make([]CacheStat, 0, len(statMap))
	for _, stat := range statMap {
		// Compute hit_rate = hits / (hits + sum(misses))
		totalMisses := int64(0)
		if stat.Misses != nil {
			for _, count := range stat.Misses {
				totalMisses += count
			}
		}
		denominator := stat.Hits + totalMisses
		if denominator > 0 {
			rate := float64(stat.Hits) / float64(denominator)
			stat.HitRate = &rate
		}

		result = append(result, *stat)
	}

	// Sort by cache name for stable output
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Name > result[j].Name {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// processCounterMetric extracts counter metrics with a single {cache} label
func processCounterMetric(mf *io_prometheus_client.MetricFamily,
	statMap map[string]*CacheStat,
	fn func(*CacheStat, int64)) {
	if mf.Metric == nil {
		return
	}
	for _, m := range mf.Metric {
		cacheName := getLabelValue(m.Label, "cache")
		if cacheName == "" {
			continue
		}
		if _, ok := statMap[cacheName]; !ok {
			statMap[cacheName] = &CacheStat{Name: cacheName}
		}
		if m.Counter != nil && m.Counter.Value != nil {
			fn(statMap[cacheName], int64(*m.Counter.Value))
		}
	}
}

// processCounterMetricWithReason extracts counter metrics with {cache} and {reason/scope} labels
func processCounterMetricWithReason(mf *io_prometheus_client.MetricFamily,
	statMap map[string]*CacheStat,
	fn func(*CacheStat, string, int64)) {
	if mf.Metric == nil {
		return
	}
	for _, m := range mf.Metric {
		cacheName := getLabelValue(m.Label, "cache")
		if cacheName == "" {
			continue
		}
		// Try both "reason" and "scope" label names
		reason := getLabelValue(m.Label, "reason")
		if reason == "" {
			reason = getLabelValue(m.Label, "scope")
		}
		if reason == "" {
			continue
		}
		if _, ok := statMap[cacheName]; !ok {
			statMap[cacheName] = &CacheStat{Name: cacheName}
		}
		if m.Counter != nil && m.Counter.Value != nil {
			fn(statMap[cacheName], reason, int64(*m.Counter.Value))
		}
	}
}

// processGaugeMetric extracts gauge metrics with a single {cache} label
func processGaugeMetric(mf *io_prometheus_client.MetricFamily,
	statMap map[string]*CacheStat,
	fn func(*CacheStat, float64)) {
	if mf.Metric == nil {
		return
	}
	for _, m := range mf.Metric {
		cacheName := getLabelValue(m.Label, "cache")
		if cacheName == "" {
			continue
		}
		if _, ok := statMap[cacheName]; !ok {
			statMap[cacheName] = &CacheStat{Name: cacheName}
		}
		if m.Gauge != nil && m.Gauge.Value != nil {
			fn(statMap[cacheName], *m.Gauge.Value)
		}
	}
}

// processHistogramMetric extracts histogram bucket data, returning count and sum
func processHistogramMetric(mf *io_prometheus_client.MetricFamily,
	statMap map[string]*CacheStat,
	fn func(*CacheStat, int64, float64)) {
	if mf.Metric == nil {
		return
	}
	for _, m := range mf.Metric {
		cacheName := getLabelValue(m.Label, "cache")
		if cacheName == "" {
			continue
		}
		if _, ok := statMap[cacheName]; !ok {
			statMap[cacheName] = &CacheStat{Name: cacheName}
		}
		if m.Histogram != nil {
			var count int64
			var sum float64
			if m.Histogram.SampleCount != nil {
				count = int64(*m.Histogram.SampleCount)
			}
			if m.Histogram.SampleSum != nil {
				sum = *m.Histogram.SampleSum
			}
			fn(statMap[cacheName], count, sum)
		}
	}
}

// getLabelValue finds a label value by name in a list of label pairs
func getLabelValue(labels []*io_prometheus_client.LabelPair, name string) string {
	for _, lp := range labels {
		if lp.Name != nil && *lp.Name == name && lp.Value != nil {
			return *lp.Value
		}
	}
	return ""
}

// isNonIntrospectableCache returns true if the cache name is a known non-introspectable cache
// (i.e., backed by a database or external service, not an in-memory *Cache[T])
func isNonIntrospectableCache(name string) bool {
	nonIntrospectable := map[string]bool{
		"metadata_fetch": true,
		"embedding":      true,
		// Add other DB-backed or external caches here
	}
	return nonIntrospectable[name]
}

// MarshalJSON ensures proper JSON marshaling of CacheStatsResponse
func (resp CacheStatsResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Caches      []CacheStat `json:"caches"`
		GeneratedAt string      `json:"generated_at"`
	}{
		Caches:      resp.Caches,
		GeneratedAt: resp.GeneratedAt,
	})
}
