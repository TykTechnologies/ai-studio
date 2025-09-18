# Midsommar AI Gateway Performance Testing Guide

## Quick Start

### Prerequisites
- Go 1.23+ installed
- Project built successfully (`go build .`)
- Performance framework fixed and working

### Running Performance Tests

#### Individual Component Benchmarks
```bash
# Analytics performance (data insertion, batch processing)
go test -bench=BenchmarkAnalytics.* ./analytics/ -benchmem -benchtime=2s

# Proxy performance (routing, authentication)
go test -bench=BenchmarkProxy.* ./proxy/ -benchmem -benchtime=2s

# Gateway performance (initialization, resource loading)
go test -bench=BenchmarkGateway.* ./pkg/aigateway/ -benchmem -benchtime=2s
```

#### All Benchmarks (Warning: Takes 10+ minutes)
```bash
go test -bench=. ./analytics/ ./proxy/ ./pkg/aigateway/ -benchmem -benchtime=3s
```

#### Quick Health Check Benchmarks
```bash
# Fast benchmarks for CI/regression testing
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=1s
go test -bench=BenchmarkProxyRequestRouting ./proxy/ -benchtime=1s
go test -bench=BenchmarkGatewayInitialization ./pkg/aigateway/ -benchtime=1s
```

### Understanding Benchmark Output

#### Example Output
```
BenchmarkAnalyticsDataInsertion/Chat_Records-16    1575956    760.5 ns/op    542.0 insert-ns    103668 insert-queries
```

**Breakdown:**
- `BenchmarkAnalyticsDataInsertion/Chat_Records-16`: Test name and CPU cores used
- `1575956`: Number of iterations run
- `760.5 ns/op`: Nanoseconds per operation (lower is better)
- `542.0 insert-ns`: Custom metric (database insert time)
- `103668 insert-queries`: Custom metric (query count)

#### Key Metrics
- **ns/op**: Latency per operation (nanoseconds)
- **B/op**: Bytes allocated per operation (memory usage)
- **allocs/op**: Memory allocations per operation
- **Custom metrics**: Component-specific measurements (e.g., routing-ns, init-ns)

## Performance Baselines

### Current Performance (Apple M4 Max, 2025-09-18)

| Component | Benchmark | Performance | Target Status |
|-----------|-----------|-------------|---------------|
| **Analytics** | Data Insertion | ~1.3M ops/sec (760ns/op) | ✅ Excellent |
| **Analytics** | Batch Processing | ~23K records/sec | ✅ 236x above target (>100/sec) |
| **Proxy** | Health Check | ~22.5K RPS (44μs/op) | ✅ Above target (>10K RPS) |
| **Gateway** | Initialization | ~1.8M ops/sec (550ns/op) | ✅ Excellent |

### Performance Targets (From Documentation)

| Component | Target | Current Status |
|-----------|--------|----------------|
| Health endpoint | >10,000 RPS | ✅ **22,546 RPS** |
| Management API | >1,000 RPS | 🔍 *Needs testing* |
| LLM proxy | 500-2,000 RPS | 🔍 *Needs end-to-end testing* |
| Gateway overhead | <10ms | ✅ **0.044ms health check** |
| Plugin overhead | <4ms per plugin | 🔍 *Needs testing* |
| Memory usage | 100-500MB | ✅ **Reasonable allocation patterns** |
| Analytics batch | >100 events/sec | ✅ **23,643 events/sec** |

## Regression Detection

### Establishing Baselines
```bash
# Generate new baseline (run on clean system)
mkdir -p performance/baselines
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchmem -benchtime=3s > performance/baselines/baseline_$(date +%Y%m%d).txt
go test -bench=BenchmarkProxyRequestRouting ./proxy/ -benchmem -benchtime=3s >> performance/baselines/baseline_$(date +%Y%m%d).txt
go test -bench=BenchmarkGatewayInitialization ./pkg/aigateway/ -benchmem -benchtime=3s >> performance/baselines/baseline_$(date +%Y%m%d).txt
```

### Comparing Performance
```bash
# After code changes, run same benchmarks
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchmem -benchtime=3s > performance/current_results.txt

# Compare manually or use benchcmp tool
go install golang.org/x/perf/cmd/benchcmp@latest
benchcmp performance/baselines/baseline_20250918.txt performance/current_results.txt
```

### Performance Regression Thresholds
- **❌ Critical Regression**: >30% performance decrease
- **⚠️  Warning**: >15% performance decrease
- **✅ Acceptable**: <15% variance
- **📈 Improvement**: Performance increase

## Troubleshooting

### Common Issues

#### "Build Failed" Errors
```bash
# Check for import issues
go build ./performance/framework/
go build ./analytics/
go build ./proxy/
go build ./pkg/aigateway/
```

#### Large Log Output in Benchmarks
- Benchmarks may produce verbose logging
- Use `2>/dev/null` to suppress logs: `go test -bench=. ./analytics/ 2>/dev/null`
- Or filter output: `go test -bench=. ./analytics/ | grep "^Benchmark"`

#### "Directory Not Found" Errors
- Ensure you're in the project root directory
- Check module path: `go list -m`

#### Memory/Performance Issues
```bash
# Run shorter benchmarks
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=500ms

# Profile memory usage
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -memprofile=mem.prof
go tool pprof mem.prof
```

### Performance Analysis

#### Memory Profiling
```bash
# Generate memory profile
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -memprofile=mem.prof -benchtime=3s

# Analyze with pprof
go tool pprof mem.prof
# Commands in pprof: top10, list FunctionName, web
```

#### CPU Profiling
```bash
# Generate CPU profile
go test -bench=BenchmarkProxyRequestRouting ./proxy/ -cpuprofile=cpu.prof -benchtime=3s

# Analyze with pprof
go tool pprof cpu.prof
```

## Integration with CI/CD

### Quick Performance Check (< 1 minute)
```bash
# Add to CI pipeline for regression detection
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=500ms | grep "^Benchmark"
go test -bench=BenchmarkProxyRequestRouting/Health_Check ./proxy/ -benchtime=500ms | grep "^Benchmark"
go test -bench=BenchmarkGatewayInitialization ./pkg/aigateway/ -benchtime=500ms | grep "^Benchmark"
```

### Full Performance Suite (Weekly/Release)
```bash
# Comprehensive testing for releases
go test -bench=. ./analytics/ ./proxy/ ./pkg/aigateway/ -benchmem -benchtime=3s > performance/release_$(date +%Y%m%d).txt
```

## Available Benchmarks

### Analytics (`./analytics/`)
- `BenchmarkAnalyticsDataInsertion` - Individual record insertion performance
- `BenchmarkAnalyticsBatchProcessing` - Batch processing performance (10, 50, 100, 500, 1000 records)
- `BenchmarkAnalyticsQuerying` - Query performance
- `BenchmarkAnalyticsAggregation` - Data aggregation performance
- `BenchmarkAnalyticsConcurrency` - Concurrent operations
- `BenchmarkAnalyticsMemoryUsage` - Memory usage patterns

### Proxy (`./proxy/`)
- `BenchmarkProxyRequestRouting` - Request routing overhead
- `BenchmarkAuthenticationOverhead` - Token validation performance
- `BenchmarkVendorTranslation` - LLM vendor translation
- `BenchmarkStreamingVsREST` - Streaming vs REST comparison
- `BenchmarkResponseCapture` - Response processing
- `BenchmarkErrorHandling` - Error scenario performance
- `BenchmarkConcurrentRequests` - Multi-threaded handling
- `BenchmarkMemoryAllocation` - Memory patterns
- `BenchmarkEndToEndLatency` - Complete request lifecycle

### AI Gateway (`./pkg/aigateway/`)
- `BenchmarkGatewayInitialization` - Gateway startup performance
- `BenchmarkResourceLoading` - LLM/filter/datasource loading
- `BenchmarkRequestProcessingPipeline` - End-to-end processing
- `BenchmarkAnalyticsRecording` - Analytics overhead
- `BenchmarkResponseHooks` - Hook execution overhead
- `BenchmarkConcurrentRequestHandling` - Concurrent throughput
- `BenchmarkGatewayStartStop` - Start/stop cycle performance
- `BenchmarkGatewayMemoryUsage` - Memory under load
- `BenchmarkDifferentRequestSizes` - Performance vs request size

## Advanced Usage

### Custom Benchtime
```bash
# Run for specific duration
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=5s

# Run specific number of iterations
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=1000x
```

### Parallel Execution Control
```bash
# Limit CPU cores used
GOMAXPROCS=4 go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/

# Control benchmark parallelism
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -cpu=1,2,4,8
```

### Memory and CPU Limits
```bash
# Limit memory
GOMEMLIMIT=500MiB go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/

# Monitor resource usage
time go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/
```

## Framework Architecture

The performance testing framework provides:

- **Mock Services**: Realistic LLM provider simulation
- **Database Testing**: SQLite test database with query monitoring
- **Concurrent Testing**: Multi-worker load testing utilities
- **Memory Monitoring**: Automated leak detection
- **Custom Metrics**: Domain-specific measurements (routing-ns, init-ns, etc.)
- **Test Data Generation**: Realistic test data with proper relationships

**Framework Location**: `./performance/framework/`
**Documentation**: `./performance/README.md`
**Implementation Details**: `./performance/IMPLEMENTATION_SUMMARY.md`

---

*Generated for Midsommar AI Gateway Performance Testing - 2025-09-18*