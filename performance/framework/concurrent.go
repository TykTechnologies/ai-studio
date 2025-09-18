// Package framework provides concurrent load testing utilities
package framework

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ConcurrentTester manages concurrent load testing scenarios
type ConcurrentTester struct {
	workerCount    int
	requestCount   int64
	duration       time.Duration
	rampUpDuration time.Duration
	metrics        *ConcurrentMetrics
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewConcurrentTester creates a new concurrent tester
func NewConcurrentTester(workers int) *ConcurrentTester {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConcurrentTester{
		workerCount: workers,
		metrics:     NewConcurrentMetrics(workers),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// WithDuration sets the test duration
func (ct *ConcurrentTester) WithDuration(duration time.Duration) *ConcurrentTester {
	ct.duration = duration
	return ct
}

// WithRequestCount sets the total number of requests to send
func (ct *ConcurrentTester) WithRequestCount(count int64) *ConcurrentTester {
	ct.requestCount = count
	return ct
}

// WithRampUp sets the ramp-up duration for gradual load increase
func (ct *ConcurrentTester) WithRampUp(duration time.Duration) *ConcurrentTester {
	ct.rampUpDuration = duration
	return ct
}

// WorkerFunc defines the function each worker will execute
type WorkerFunc func(ctx context.Context, workerID int, metrics *PerformanceMetrics) error

// Run executes the concurrent test with the given worker function
func (ct *ConcurrentTester) Run(b *testing.B, workerFunc WorkerFunc) *PerformanceMetrics {
	var wg sync.WaitGroup
	var requestCounter int64
	startTime := time.Now()

	// Create context with timeout if duration is set
	workerCtx := ct.ctx
	if ct.duration > 0 {
		var cancel context.CancelFunc
		workerCtx, cancel = context.WithTimeout(ct.ctx, ct.duration)
		defer cancel()
	}

	// Start workers with optional ramp-up
	for i := 0; i < ct.workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Ramp-up delay
			if ct.rampUpDuration > 0 {
				delay := time.Duration(workerID) * ct.rampUpDuration / time.Duration(ct.workerCount)
				time.Sleep(delay)
			}

			workerMetrics := ct.metrics.GetWorkerMetrics(workerID)
			workerMetrics.StartMeasurement()

			// Worker loop
			for {
				select {
				case <-workerCtx.Done():
					return
				default:
					// Check request count limit
					if ct.requestCount > 0 {
						if atomic.LoadInt64(&requestCounter) >= ct.requestCount {
							return
						}
						atomic.AddInt64(&requestCounter, 1)
					}

					// Execute the work
					requestStart := time.Now()
					err := workerFunc(workerCtx, workerID, workerMetrics)
					requestDuration := time.Since(requestStart)

					// Record metrics
					workerMetrics.RecordLatency(requestDuration)
					if err != nil {
						workerMetrics.RecordError()
					} else {
						workerMetrics.RecordSuccess()
					}
				}
			}
		}(i)
	}

	// Wait for all workers to complete
	wg.Wait()
	totalDuration := time.Since(startTime)

	// Finalize metrics for each worker
	for i := 0; i < ct.workerCount; i++ {
		workerMetrics := ct.metrics.GetWorkerMetrics(i)
		workerMetrics.EndMeasurement(totalDuration)
	}

	// Aggregate and report metrics
	aggregatedMetrics := ct.metrics.Aggregate()
	aggregatedMetrics.ReportToB(b)

	return aggregatedMetrics
}

// Stop cancels the concurrent test
func (ct *ConcurrentTester) Stop() {
	ct.cancel()
}

// LoadPattern defines different load testing patterns
type LoadPattern int

const (
	ConstantLoad LoadPattern = iota
	SpikeLoad
	StepLoad
	SineWaveLoad
)

// LoadTester provides advanced load testing patterns
type LoadTester struct {
	pattern    LoadPattern
	baseRPS    float64
	maxRPS     float64
	duration   time.Duration
	stepSize   float64
	stepDuration time.Duration
}

// NewLoadTester creates a new load tester with pattern
func NewLoadTester(pattern LoadPattern) *LoadTester {
	return &LoadTester{
		pattern: pattern,
		baseRPS: 100,
		maxRPS:  1000,
		duration: time.Minute,
		stepSize: 100,
		stepDuration: time.Second * 10,
	}
}

// WithBaseRPS sets the baseline requests per second
func (lt *LoadTester) WithBaseRPS(rps float64) *LoadTester {
	lt.baseRPS = rps
	return lt
}

// WithMaxRPS sets the maximum requests per second
func (lt *LoadTester) WithMaxRPS(rps float64) *LoadTester {
	lt.maxRPS = rps
	return lt
}

// WithDuration sets the test duration
func (lt *LoadTester) WithDuration(duration time.Duration) *LoadTester {
	lt.duration = duration
	return lt
}

// WithStepConfig sets step load configuration
func (lt *LoadTester) WithStepConfig(stepSize float64, stepDuration time.Duration) *LoadTester {
	lt.stepSize = stepSize
	lt.stepDuration = stepDuration
	return lt
}

// Execute runs the load test with the specified pattern
func (lt *LoadTester) Execute(b *testing.B, workerFunc WorkerFunc) *PerformanceMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), lt.duration)
	defer cancel()

	metrics := NewPerformanceMetrics()
	metrics.StartMeasurement()

	var wg sync.WaitGroup
	requestChan := make(chan struct{}, 1000) // Buffer for request pacing

	// Start rate controller goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(requestChan)
		lt.controlRate(ctx, requestChan)
	}()

	// Start worker pool
	workerCount := int(lt.maxRPS/10) // Rough estimate
	if workerCount < 10 {
		workerCount = 10
	}
	if workerCount > 100 {
		workerCount = 100
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for range requestChan {
				requestStart := time.Now()
				err := workerFunc(ctx, workerID, metrics)
				requestDuration := time.Since(requestStart)

				metrics.RecordLatency(requestDuration)
				if err != nil {
					metrics.RecordError()
				} else {
					metrics.RecordSuccess()
				}
			}
		}(i)
	}

	wg.Wait()
	metrics.EndMeasurement(lt.duration)
	metrics.ReportToB(b)

	return metrics
}

// controlRate manages request rate according to the load pattern
func (lt *LoadTester) controlRate(ctx context.Context, requestChan chan<- struct{}) {
	startTime := time.Now()
	ticker := time.NewTicker(time.Millisecond * 10) // 100 Hz control loop
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(startTime)
			targetRPS := lt.calculateTargetRPS(elapsed)

			// Calculate how many requests we should send in this interval
			intervalDuration := time.Millisecond * 10
			requestsThisInterval := int(targetRPS * intervalDuration.Seconds())

			// Send requests (non-blocking)
			for i := 0; i < requestsThisInterval; i++ {
				select {
				case requestChan <- struct{}{}:
				default:
					// Channel full, skip this request
				}
			}
		}
	}
}

// calculateTargetRPS determines the target RPS based on elapsed time and pattern
func (lt *LoadTester) calculateTargetRPS(elapsed time.Duration) float64 {
	progress := elapsed.Seconds() / lt.duration.Seconds()
	if progress > 1 {
		progress = 1
	}

	switch lt.pattern {
	case ConstantLoad:
		return lt.baseRPS

	case SpikeLoad:
		// Spike at 50% through the test
		spikeStart := 0.45
		spikeEnd := 0.55
		if progress >= spikeStart && progress <= spikeEnd {
			return lt.maxRPS
		}
		return lt.baseRPS

	case StepLoad:
		// Increase RPS in steps
		steps := int(lt.duration / lt.stepDuration)
		if steps <= 0 {
			steps = 1
		}
		currentStep := int(progress * float64(steps))
		rps := lt.baseRPS + float64(currentStep)*lt.stepSize
		if rps > lt.maxRPS {
			rps = lt.maxRPS
		}
		return rps

	case SineWaveLoad:
		// Sine wave pattern (using simple approximation to avoid math import)
		amplitude := (lt.maxRPS - lt.baseRPS) / 2
		offset := lt.baseRPS + amplitude
		// Simple sine approximation: sin(x) ≈ x - x³/6 for small x
		angle := progress * 2 * 3.14159
		if angle > 3.14159 {
			angle = 2*3.14159 - angle
		}
		sineApprox := angle - (angle*angle*angle)/6
		return offset + amplitude*sineApprox

	default:
		return lt.baseRPS
	}
}

// RateLimiter provides request rate limiting for load tests
type RateLimiter struct {
	rps       float64
	interval  time.Duration
	tokens    chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rps float64) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	interval := time.Duration(1e9 / rps) // nanoseconds per request

	rl := &RateLimiter{
		rps:      rps,
		interval: interval,
		tokens:   make(chan struct{}, int(rps)), // Buffer size = RPS
		ctx:      ctx,
		cancel:   cancel,
	}

	// Fill initial tokens
	for i := 0; i < int(rps); i++ {
		rl.tokens <- struct{}{}
	}

	// Start token refill goroutine
	go rl.refillTokens()

	return rl
}

// refillTokens continuously adds tokens at the specified rate
func (rl *RateLimiter) refillTokens() {
	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.ctx.Done():
			return
		case <-ticker.C:
			select {
			case rl.tokens <- struct{}{}:
			default:
				// Token buffer full, skip
			}
		}
	}
}

// Wait blocks until a token is available
func (rl *RateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops the rate limiter
func (rl *RateLimiter) Close() {
	rl.cancel()
	close(rl.tokens)
}

// ConnectionPoolTester tests connection pool behavior under load
type ConnectionPoolTester struct {
	maxConnections  int
	connectionDelay time.Duration
	holdDuration    time.Duration
}

// NewConnectionPoolTester creates a connection pool tester
func NewConnectionPoolTester(maxConnections int) *ConnectionPoolTester {
	return &ConnectionPoolTester{
		maxConnections:  maxConnections,
		connectionDelay: time.Millisecond * 10,
		holdDuration:    time.Millisecond * 100,
	}
}

// WithConnectionDelay sets the simulated connection establishment delay
func (cpt *ConnectionPoolTester) WithConnectionDelay(delay time.Duration) *ConnectionPoolTester {
	cpt.connectionDelay = delay
	return cpt
}

// WithHoldDuration sets how long each connection is held
func (cpt *ConnectionPoolTester) WithHoldDuration(duration time.Duration) *ConnectionPoolTester {
	cpt.holdDuration = duration
	return cpt
}

// TestExhaustion tests what happens when the connection pool is exhausted
func (cpt *ConnectionPoolTester) TestExhaustion(b *testing.B, acquireFunc func() error, releaseFunc func()) *PerformanceMetrics {
	metrics := NewPerformanceMetrics()
	metrics.StartMeasurement()

	// Create more workers than available connections
	workerCount := cpt.maxConnections * 2
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for ctx.Err() == nil {
				requestStart := time.Now()

				// Simulate acquiring connection
				err := acquireFunc()
				if err != nil {
					metrics.RecordError()
					continue
				}

				// Hold the connection
				time.Sleep(cpt.holdDuration)

				// Release connection
				releaseFunc()

				requestDuration := time.Since(requestStart)
				metrics.RecordLatency(requestDuration)
				metrics.RecordSuccess()

				// Brief pause between requests
				time.Sleep(time.Millisecond * 10)
			}
		}(i)
	}

	wg.Wait()
	metrics.EndMeasurement(time.Second * 30)
	metrics.ReportToB(b)

	return metrics
}

// MemoryLeakTester helps detect memory leaks during load testing
type MemoryLeakTester struct {
	profiler       *MemoryProfiler
	sampleInterval time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewMemoryLeakTester creates a memory leak tester
func NewMemoryLeakTester() *MemoryLeakTester {
	ctx, cancel := context.WithCancel(context.Background())
	return &MemoryLeakTester{
		profiler:       NewMemoryProfiler(),
		sampleInterval: time.Second,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// StartMonitoring begins memory monitoring
func (mlt *MemoryLeakTester) StartMonitoring() {
	go func() {
		ticker := time.NewTicker(mlt.sampleInterval)
		defer ticker.Stop()

		for {
			select {
			case <-mlt.ctx.Done():
				return
			case <-ticker.C:
				mlt.profiler.Sample()
			}
		}
	}()
}

// StopMonitoring stops memory monitoring and returns results
func (mlt *MemoryLeakTester) StopMonitoring() (bool, string) {
	mlt.cancel()
	return mlt.profiler.DetectMemoryLeak()
}

// GetMemoryTrend returns the memory usage trend
func (mlt *MemoryLeakTester) GetMemoryTrend() []MemorySample {
	return mlt.profiler.GetMemoryTrend()
}