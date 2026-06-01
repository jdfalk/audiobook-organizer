// file: internal/server/handlers/cache.go
// version: 1.1.0
// guid: c9d0e1f2-a3b4-5678-cdef-678901234567
// last-edited: 2026-06-01

package handlers

import "encoding/json"

// CacheStatsResponse represents the JSON response for GET /api/v1/cache/stats.
type CacheStatsResponse struct {
	Caches      []CacheStat `json:"caches"`
	GeneratedAt string      `json:"generated_at"`
}

// CacheStat represents metrics for a single cache.
type CacheStat struct {
	Name              string            `json:"name"`
	Hits              int64             `json:"hits"`
	Misses            map[string]int64  `json:"misses"`
	Sets              int64             `json:"sets"`
	Invalidations     map[string]int64  `json:"invalidations"`
	Evictions         map[string]int64  `json:"evictions"`
	Size              int64             `json:"size"`
	HitRate           *float64          `json:"hit_rate,omitempty"`
	GetDurationMetric GetDurationMetric `json:"get_duration_seconds"`
}

// GetDurationMetric represents count and sum of cache get durations.
type GetDurationMetric struct {
	Count int64   `json:"count"`
	Sum   float64 `json:"sum"`
}

// MarshalJSON ensures proper JSON marshaling of CacheStatsResponse.
func (resp CacheStatsResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Caches      []CacheStat `json:"caches"`
		GeneratedAt string      `json:"generated_at"`
	}{
		Caches:      resp.Caches,
		GeneratedAt: resp.GeneratedAt,
	})
}
