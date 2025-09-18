// Package framework provides performance metrics collection and analysis utilities
package framework

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

// PerformanceMetrics captures comprehensive performance data
type PerformanceMetrics struct {
	// Latency metrics
	Latencies []time.Duration
	P50       time.Duration
	P95       time.Duration
	P99       time.Duration
	Average   time.Duration
	Min       time.Duration
	Max       time.Duration

	// Throughput metrics
	TotalRequests   int64
	SuccessRequests int64
	ErrorRequests   int64
	RequestsPerSec  float64
	TestDuration    time.Duration

	// Resource metrics
	MemAllocBefore   uint64
	MemAllocAfter    uint64
	MemAllocDelta    uint64
	GoroutinesBefore int
	GoroutinesAfter  int
	GoroutinesDelta  int

	// Custom metrics
	CustomMetrics map[string]float64

	mu sync.RWMutex
}

// NewPerformanceMetrics creates a new metrics collector
func NewPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		Latencies:     make([]time.Duration, 0, 1000),
		CustomMetrics: make(map[string]float64),
	}
}

// StartMeasurement records initial system state
func (pm *PerformanceMetrics) StartMeasurement() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	pm.MemAllocBefore = memStats.Alloc
	pm.GoroutinesBefore = runtime.NumGoroutine()
}

// EndMeasurement records final system state and calculates metrics
func (pm *PerformanceMetrics) EndMeasurement(duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	pm.MemAllocAfter = memStats.Alloc
	pm.GoroutinesAfter = runtime.NumGoroutine()

	pm.TestDuration = duration
	if pm.MemAllocAfter > pm.MemAllocBefore {
		pm.MemAllocDelta = pm.MemAllocAfter - pm.MemAllocBefore
	}
	if pm.GoroutinesAfter > pm.GoroutinesBefore {
		pm.GoroutinesDelta = pm.GoroutinesAfter - pm.GoroutinesBefore
	}

	pm.calculateLatencyMetrics()
	pm.calculateThroughputMetrics()
}

// RecordLatency adds a latency measurement
func (pm *PerformanceMetrics) RecordLatency(duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Latencies = append(pm.Latencies, duration)
}

// RecordSuccess increments successful request counter
func (pm *PerformanceMetrics) RecordSuccess() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.SuccessRequests++
	pm.TotalRequests++
}

// RecordError increments error request counter
func (pm *PerformanceMetrics) RecordError() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.ErrorRequests++
	pm.TotalRequests++
}

// RecordCustomMetric adds a custom metric value
func (pm *PerformanceMetrics) RecordCustomMetric(name string, value float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.CustomMetrics[name] = value
}

// calculateLatencyMetrics computes percentile and aggregate latency metrics
func (pm *PerformanceMetrics) calculateLatencyMetrics() {
	if len(pm.Latencies) == 0 {
		return
	}

	// Sort latencies for percentile calculation
	sorted := make([]time.Duration, len(pm.Latencies))
	copy(sorted, pm.Latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	pm.Min = sorted[0]
	pm.Max = sorted[len(sorted)-1]

	// Calculate percentiles
	pm.P50 = sorted[int(0.50*float64(len(sorted)))]
	pm.P95 = sorted[int(0.95*float64(len(sorted)))]
	pm.P99 = sorted[int(0.99*float64(len(sorted)))]

	// Calculate average
	var total time.Duration
	for _, latency := range pm.Latencies {
		total += latency
	}
	pm.Average = total / time.Duration(len(pm.Latencies))
}

// calculateThroughputMetrics computes request rate metrics
func (pm *PerformanceMetrics) calculateThroughputMetrics() {
	if pm.TestDuration > 0 {
		pm.RequestsPerSec = float64(pm.TotalRequests) / pm.TestDuration.Seconds()
	}
}

// ReportToB reports metrics to the testing framework
func (pm *PerformanceMetrics) ReportToB(b *testing.B) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Report standard benchmark metrics
	if pm.RequestsPerSec > 0 {
		b.ReportMetric(pm.RequestsPerSec, "req/sec")
	}

	if pm.Average > 0 {
		b.ReportMetric(float64(pm.Average.Nanoseconds()), "avg-latency-ns")
	}

	if pm.P95 > 0 {
		b.ReportMetric(float64(pm.P95.Nanoseconds()), "p95-latency-ns")
	}

	if pm.P99 > 0 {
		b.ReportMetric(float64(pm.P99.Nanoseconds()), "p99-latency-ns")
	}

	if pm.MemAllocDelta > 0 {
		b.ReportMetric(float64(pm.MemAllocDelta), "bytes-allocated")
	}

	if pm.GoroutinesDelta > 0 {
		b.ReportMetric(float64(pm.GoroutinesDelta), "goroutines-created")
	}

	// Report error rate
	if pm.TotalRequests > 0 {
		errorRate := float64(pm.ErrorRequests) / float64(pm.TotalRequests) * 100
		b.ReportMetric(errorRate, "error-rate-percent")
	}

	// Report custom metrics
	for name, value := range pm.CustomMetrics {
		b.ReportMetric(value, name)
	}
}

// String provides a human-readable summary of the metrics
func (pm *PerformanceMetrics) String() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return fmt.Sprintf(
		"Performance Metrics:\n"+
			"  Requests: %d total, %d success, %d errors\n"+
			"  Throughput: %.2f req/sec\n"+
			"  Latency: avg=%v, p50=%v, p95=%v, p99=%v, min=%v, max=%v\n"+
			"  Memory: %d bytes allocated\n"+
			"  Goroutines: %d created\n"+
			"  Error Rate: %.2f%%",
		pm.TotalRequests, pm.SuccessRequests, pm.ErrorRequests,
		pm.RequestsPerSec,
		pm.Average, pm.P50, pm.P95, pm.P99, pm.Min, pm.Max,
		pm.MemAllocDelta,
		pm.GoroutinesDelta,
		float64(pm.ErrorRequests)/float64(pm.TotalRequests)*100,
	)
}

// LatencyHistogram creates a histogram of latency distribution
type LatencyHistogram struct {
	Buckets []LatencyBucket
}

// LatencyBucket represents a range of latencies and their count
type LatencyBucket struct {
	Min   time.Duration
	Max   time.Duration
	Count int
}

// CreateLatencyHistogram generates a histogram from latency data
func (pm *PerformanceMetrics) CreateLatencyHistogram() LatencyHistogram {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.Latencies) == 0 {
		return LatencyHistogram{}
	}

	// Define bucket ranges (exponential scaling)
	bucketRanges := []time.Duration{
		time.Microsecond * 100,  // 0-100μs
		time.Millisecond,        // 100μs-1ms
		time.Millisecond * 5,    // 1ms-5ms
		time.Millisecond * 10,   // 5ms-10ms
		time.Millisecond * 50,   // 10ms-50ms
		time.Millisecond * 100,  // 50ms-100ms
		time.Millisecond * 500,  // 100ms-500ms
		time.Second,             // 500ms-1s
		time.Second * 5,         // 1s-5s
	}

	buckets := make([]LatencyBucket, len(bucketRanges))
	for i, maxDuration := range bucketRanges {
		var minDuration time.Duration
		if i > 0 {
			minDuration = bucketRanges[i-1]
		}
		buckets[i] = LatencyBucket{
			Min: minDuration,
			Max: maxDuration,
		}
	}

	// Count latencies in each bucket
	for _, latency := range pm.Latencies {
		for i := range buckets {
			if latency >= buckets[i].Min && latency < buckets[i].Max {
				buckets[i].Count++
				break
			}
		}
	}

	return LatencyHistogram{Buckets: buckets}
}

// ConcurrentMetrics collects metrics from concurrent operations
type ConcurrentMetrics struct {
	metrics []*PerformanceMetrics
	mu      sync.RWMutex
}

// NewConcurrentMetrics creates a new concurrent metrics collector
func NewConcurrentMetrics(workers int) *ConcurrentMetrics {
	metrics := make([]*PerformanceMetrics, workers)
	for i := 0; i < workers; i++ {
		metrics[i] = NewPerformanceMetrics()
	}
	return &ConcurrentMetrics{
		metrics: metrics,
	}
}

// GetWorkerMetrics returns metrics for a specific worker
func (cm *ConcurrentMetrics) GetWorkerMetrics(workerID int) *PerformanceMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if workerID < len(cm.metrics) {
		return cm.metrics[workerID]
	}
	return nil
}

// Aggregate combines metrics from all workers
func (cm *ConcurrentMetrics) Aggregate() *PerformanceMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	aggregated := NewPerformanceMetrics()

	for _, workerMetrics := range cm.metrics {
		workerMetrics.mu.RLock()

		// Aggregate counters
		aggregated.TotalRequests += workerMetrics.TotalRequests
		aggregated.SuccessRequests += workerMetrics.SuccessRequests
		aggregated.ErrorRequests += workerMetrics.ErrorRequests
		aggregated.MemAllocDelta += workerMetrics.MemAllocDelta
		aggregated.GoroutinesDelta += workerMetrics.GoroutinesDelta

		// Combine latencies
		aggregated.Latencies = append(aggregated.Latencies, workerMetrics.Latencies...)

		// Aggregate custom metrics (sum for now)
		for name, value := range workerMetrics.CustomMetrics {
			aggregated.CustomMetrics[name] += value
		}

		workerMetrics.mu.RUnlock()
	}

	// Calculate combined metrics
	if len(aggregated.Latencies) > 0 {
		aggregated.calculateLatencyMetrics()
	}

	// Use the longest test duration
	for _, workerMetrics := range cm.metrics {
		if workerMetrics.TestDuration > aggregated.TestDuration {
			aggregated.TestDuration = workerMetrics.TestDuration
		}
	}

	aggregated.calculateThroughputMetrics()

	return aggregated
}

// DatabaseMetrics tracks database-specific performance metrics
type DatabaseMetrics struct {
	QueryCount      int64
	QueryDuration   time.Duration
	ConnectionsUsed int64
	CacheHits       int64
	CacheMisses     int64
}

// RecordQuery adds a database query measurement
func (dm *DatabaseMetrics) RecordQuery(duration time.Duration) {
	dm.QueryCount++
	dm.QueryDuration += duration
}

// RecordConnection increments connection usage
func (dm *DatabaseMetrics) RecordConnection() {
	dm.ConnectionsUsed++
}

// RecordCacheHit increments cache hit counter
func (dm *DatabaseMetrics) RecordCacheHit() {
	dm.CacheHits++
}

// RecordCacheMiss increments cache miss counter
func (dm *DatabaseMetrics) RecordCacheMiss() {
	dm.CacheMisses++
}

// AvgQueryDuration returns average query duration
func (dm *DatabaseMetrics) AvgQueryDuration() time.Duration {
	if dm.QueryCount == 0 {
		return 0
	}
	return dm.QueryDuration / time.Duration(dm.QueryCount)
}

// CacheHitRate returns cache hit percentage
func (dm *DatabaseMetrics) CacheHitRate() float64 {
	total := dm.CacheHits + dm.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(dm.CacheHits) / float64(total) * 100
}

// MemoryProfiler helps track memory usage patterns during benchmarks
type MemoryProfiler struct {
	samples []MemorySample
	mu      sync.Mutex
}

// MemorySample represents a point-in-time memory measurement
type MemorySample struct {
	Timestamp  time.Time
	Alloc      uint64
	TotalAlloc uint64
	Sys        uint64
	NumGC      uint32
}

// NewMemoryProfiler creates a new memory profiler
func NewMemoryProfiler() *MemoryProfiler {
	return &MemoryProfiler{
		samples: make([]MemorySample, 0, 100),
	}
}

// Sample records current memory statistics
func (mp *MemoryProfiler) Sample() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	sample := MemorySample{
		Timestamp:  time.Now(),
		Alloc:      memStats.Alloc,
		TotalAlloc: memStats.TotalAlloc,
		Sys:        memStats.Sys,
		NumGC:      memStats.NumGC,
	}

	mp.samples = append(mp.samples, sample)
}

// GetMemoryTrend returns memory usage trend over time
func (mp *MemoryProfiler) GetMemoryTrend() []MemorySample {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]MemorySample, len(mp.samples))
	copy(result, mp.samples)
	return result
}

// DetectMemoryLeak analyzes samples for potential memory leaks
func (mp *MemoryProfiler) DetectMemoryLeak() (bool, string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if len(mp.samples) < 10 {
		return false, "Insufficient samples for leak detection"
	}

	// Look for consistently increasing memory usage
	increasing := 0
	for i := 1; i < len(mp.samples); i++ {
		if mp.samples[i].Alloc > mp.samples[i-1].Alloc {
			increasing++
		}
	}

	// If more than 70% of samples show increasing memory, potential leak
	leakThreshold := 0.7
	if float64(increasing)/float64(len(mp.samples)-1) > leakThreshold {
		initialMem := mp.samples[0].Alloc
		finalMem := mp.samples[len(mp.samples)-1].Alloc
		growth := finalMem - initialMem

		return true, fmt.Sprintf("Memory grew from %d to %d bytes (%d bytes increase, %.1f%% of samples increasing)",
			initialMem, finalMem, growth, float64(increasing)/float64(len(mp.samples)-1)*100)
	}

	return false, "No memory leak detected"
}