# Performance Testing Framework

This directory contains comprehensive performance testing tools and benchmarks for the Midsommar AI Gateway system.

## Overview

The performance testing framework is designed to validate the system's claims and identify optimization opportunities across all critical components:

- **Core Proxy**: Request routing, vendor translation, streaming
- **AI Gateway Library**: Gateway wrapper and simplified API
- **Microgateway**: Full server with management API and plugins
- **Analytics**: Batch processing and data handling
- **Services**: Cache operations and database queries

## Quick Start

### Running Individual Benchmarks

```bash
# Core proxy benchmarks
go test -bench=BenchmarkProxy* ./proxy/ -benchmem

# AI Gateway benchmarks
go test -bench=BenchmarkGateway* ./pkg/aigateway/ -benchmem

# Analytics benchmarks
go test -bench=BenchmarkAnalytics* ./analytics/ -benchmem

# Services benchmarks
go test -bench=BenchmarkService* ./services/ -benchmem
```

### Running Performance Test Suite

```bash
# Run all benchmarks with detailed output
make perf-test

# Generate performance reports
make perf-report

# Performance profiling with CPU and memory analysis
make perf-profile
```

### Load Testing

```bash
# Sustained load testing (various RPS targets)
go test -bench=BenchmarkLoad* ./tests/performance/ -benchtime=30s

# Stress testing with connection pool exhaustion
go test -bench=BenchmarkStress* ./tests/performance/ -benchtime=60s
```

## Performance Targets

Based on documentation and system requirements:

| Component | Target | Measurement |
|-----------|--------|-------------|
| Health endpoint | >10,000 RPS | Requests per second |
| Management API | >1,000 RPS | Requests per second |
| LLM proxy | 500-2,000 RPS | Requests per second |
| Gateway overhead | <10ms | Added latency |
| Plugin overhead | <4ms per plugin | Added latency |
| Memory usage | 100-500MB | Under normal load |
| Analytics batch | >100 events/sec | Processing rate |

## Benchmark Structure

All benchmarks follow this standard pattern:

```go
func BenchmarkFeatureName(b *testing.B) {
    // Setup (runs once)

    b.ResetTimer() // Don't count setup time

    for i := 0; i < b.N; i++ {
        // Code being benchmarked
    }

    // Optional: Record custom metrics
    b.ReportMetric(float64(customMetric), "custom-unit")
}
```

### Sub-benchmarks for Different Scenarios

```go
func BenchmarkProxyRequest(b *testing.B) {
    scenarios := []struct {
        name     string
        vendor   string
        streaming bool
    }{
        {"OpenAI_REST", "openai", false},
        {"OpenAI_Stream", "openai", true},
        {"Anthropic_REST", "anthropic", false},
        {"Anthropic_Stream", "anthropic", true},
    }

    for _, scenario := range scenarios {
        b.Run(scenario.name, func(b *testing.B) {
            // Benchmark specific scenario
        })
    }
}
```

## Framework Components

### Test Utilities (`performance/framework/`)

- **`testutil.go`**: Database setup, mock servers, request builders
- **`metrics.go`**: Custom metric collection and reporting
- **`concurrent.go`**: Concurrent load testing helpers
- **`profiling.go`**: CPU and memory profiling integration

### Mock Services (`performance/mocks/`)

- **`mock_llm.go`**: Simulated LLM provider responses
- **`mock_auth.go`**: Authentication service mocks
- **`mock_analytics.go`**: Analytics service mocks

### Test Data (`performance/testdata/`)

- **`requests/`**: Sample LLM requests for different vendors
- **`responses/`**: Expected responses for testing
- **`configs/`**: Test configurations for different scenarios

## Key Metrics Tracked

### Latency Metrics
- **p50**: Median response time
- **p95**: 95th percentile (most users)
- **p99**: 99th percentile (worst case)

### Throughput Metrics
- **RPS**: Requests per second
- **Events/sec**: Analytics processing rate
- **Queries/sec**: Database operations

### Resource Metrics
- **Memory allocations**: Per operation
- **CPU usage**: Under load
- **Goroutines**: Concurrency overhead
- **File descriptors**: Connection pooling

### Database Metrics
- **Query count**: N+1 prevention
- **Query duration**: Database performance
- **Connection pool**: Utilization rates

## Memory Profiling

Enable memory profiling in benchmarks:

```bash
# Generate memory profile
go test -bench=BenchmarkProxyRequest -memprofile=mem.prof ./proxy/

# Analyze memory usage
go tool pprof mem.prof
```

Common pprof commands:
- `top10`: Show top memory allocators
- `list FunctionName`: Show line-by-line allocation
- `web`: Generate visual call graph

## CPU Profiling

Enable CPU profiling in benchmarks:

```bash
# Generate CPU profile
go test -bench=BenchmarkProxyRequest -cpuprofile=cpu.prof ./proxy/

# Analyze CPU usage
go tool pprof cpu.prof
```

## Continuous Integration

### Performance Regression Detection

```bash
# Establish performance baseline
make perf-baseline

# Compare current performance to baseline
make perf-compare
```

### Automated Performance Monitoring

The CI pipeline runs performance tests on every commit and alerts if:
- Latency increases by >20%
- Throughput decreases by >15%
- Memory usage increases by >30%
- Critical path performance degrades

## Optimization Guidelines

### When Benchmarks Fail Targets

1. **High Latency (>10ms overhead)**
   - Profile CPU usage to find bottlenecks
   - Check for inefficient database queries
   - Review goroutine usage patterns

2. **Low Throughput (<target RPS)**
   - Increase connection pool sizes
   - Optimize JSON serialization/deserialization
   - Review context switching overhead

3. **High Memory Usage (>500MB)**
   - Check for memory leaks with profiling
   - Optimize object allocation patterns
   - Review caching strategies

4. **Database Performance Issues**
   - Analyze query patterns for N+1 problems
   - Review index usage
   - Consider query optimization

## Common Performance Patterns

### N+1 Query Prevention

```go
// Bad: Triggers N+1 queries
llms, err := service.GetAllLLMs()
for _, llm := range llms {
    plugins := llm.Plugins // Lazy loading triggers query
}

// Good: Preload relationships
llms, err := service.GetAllLLMsWithPlugins() // Single query with JOIN
```

### Efficient JSON Processing

```go
// Use json.NewDecoder for streaming
decoder := json.NewDecoder(request.Body)
var req LLMRequest
err := decoder.Decode(&req)

// Reuse byte buffers
var buf bytes.Buffer
encoder := json.NewEncoder(&buf)
```

### Connection Pool Optimization

```go
// Configure appropriate pool sizes
db.SetMaxOpenConns(25)  // Limit concurrent connections
db.SetMaxIdleConns(5)   // Reuse connections
db.SetConnMaxLifetime(time.Hour) // Prevent stale connections
```

## Troubleshooting

### Benchmark Variability

If benchmarks show high variability:
- Increase `-benchtime` duration
- Run on dedicated hardware
- Disable CPU frequency scaling
- Use `-count=10` for multiple runs

### Memory Leak Detection

For long-running tests:
```bash
# Run extended benchmark to detect leaks
go test -bench=BenchmarkLongRunning -benchtime=5m -memprofile=leak.prof

# Compare memory usage over time
go tool pprof -base=baseline.prof leak.prof
```

### Database Lock Contention

If database benchmarks are slow:
- Use separate test databases per benchmark
- Implement proper transaction isolation
- Consider read-only replicas for analytics

## Best Practices

1. **Always use `b.ResetTimer()`** after setup code
2. **Report custom metrics** for domain-specific measurements
3. **Use sub-benchmarks** for different scenarios
4. **Profile regularly** to understand resource usage
5. **Test realistic data sizes** - avoid trivial test cases
6. **Measure end-to-end** and individual components
7. **Document performance assumptions** in benchmark comments
8. **Run benchmarks in CI** to prevent regressions

## Contributing

When adding new benchmarks:

1. Follow the standard benchmark naming pattern: `BenchmarkComponentFeature`
2. Include relevant sub-benchmarks for different scenarios
3. Document the performance expectations in comments
4. Add the benchmark to the appropriate test suite
5. Update this README with any new framework components

## Reference

- [Go Benchmarking Guide](https://golang.org/pkg/testing/#hdr-Benchmarks)
- [Performance Profiling](https://golang.org/blog/pprof)
- [Memory Profiling](https://golang.org/blog/profiling-go-programs)