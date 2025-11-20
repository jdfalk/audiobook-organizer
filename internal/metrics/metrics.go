// file: internal/metrics/metrics.go
// version: 1.0.1
// guid: 9f8e7d6c-5b4a-3210-9fed-cba876543210

package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

	operationStarted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "audiobook_organizer",
		Name:      "operations_started_total",
		Help:      "Total number of operations started by type",
	}, []string{"type"})
	operationCompleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "audiobook_organizer",
		Name:      "operations_completed_total",
		Help:      "Total number of operations successfully completed by type",
	}, []string{"type"})
	operationFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "audiobook_organizer",
		Name:      "operations_failed_total",
		Help:      "Total number of operations failed by type",
	}, []string{"type"})
	operationCanceled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "audiobook_organizer",
		Name:      "operations_canceled_total",
		Help:      "Total number of operations canceled by type",
	}, []string{"type"})
	operationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "audiobook_organizer",
		Name:      "operation_duration_seconds",
		Help:      "Histogram of operation durations in seconds by type",
		Buckets:   prometheus.ExponentialBuckets(0.05, 1.6, 10), // ~50ms up to several seconds/minutes
	}, []string{"type"})

	booksGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "audiobook_organizer",
		Name:      "books_total",
		Help:      "Current total number of books in library",
	})
	foldersGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "audiobook_organizer",
		Name:      "library_folders_total",
		Help:      "Current total number of enabled library folders",
	})
	memoryAllocGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "audiobook_organizer",
		Name:      "process_memory_alloc_bytes",
		Help:      "Current process memory allocation (runtime.Alloc)",
	})
	goroutinesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "audiobook_organizer",
		Name:      "process_goroutines",
		Help:      "Number of currently running goroutines",
	})
)

// Register initializes metrics with the global Prometheus registry (idempotent)
func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(operationStarted, operationCompleted, operationFailed, operationCanceled, operationDuration,
			booksGauge, foldersGauge, memoryAllocGauge, goroutinesGauge)
	})
}

// Operation lifecycle helpers
func IncOperationStarted(opType string)   { operationStarted.WithLabelValues(opType).Inc() }
func IncOperationCompleted(opType string) { operationCompleted.WithLabelValues(opType).Inc() }
func IncOperationFailed(opType string)    { operationFailed.WithLabelValues(opType).Inc() }
func IncOperationCanceled(opType string)  { operationCanceled.WithLabelValues(opType).Inc() }
func ObserveOperationDuration(opType string, d time.Duration) {
	operationDuration.WithLabelValues(opType).Observe(d.Seconds())
}

// Gauges
func SetBooks(n int)          { booksGauge.Set(float64(n)) }
func SetFolders(n int)        { foldersGauge.Set(float64(n)) }
func SetMemoryAlloc(b uint64) { memoryAllocGauge.Set(float64(b)) }
func SetGoroutines(n int)     { goroutinesGauge.Set(float64(n)) }
