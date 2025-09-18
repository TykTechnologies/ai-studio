# Performance Testing Quick Reference

## Essential Commands

### Quick Health Check (30 seconds)
```bash
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=1s
go test -bench=BenchmarkProxyRequestRouting ./proxy/ -benchtime=1s
go test -bench=BenchmarkGatewayInitialization ./pkg/aigateway/ -benchtime=1s
```

### Individual Components
```bash
# Analytics benchmarks
go test -bench=BenchmarkAnalytics.* ./analytics/ -benchmem

# Proxy benchmarks
go test -bench=BenchmarkProxy.* ./proxy/ -benchmem

# Gateway benchmarks
go test -bench=BenchmarkGateway.* ./pkg/aigateway/ -benchmem
```

### Generate Baseline
```bash
mkdir -p performance/baselines
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -benchtime=2s > performance/baselines/$(date +%Y%m%d).txt
```

## Current Baselines (2025-09-18)

| Metric | Performance | Status |
|--------|-------------|---------|
| Analytics insertion | 1.3M ops/sec | ✅ Excellent |
| Analytics batch | 23K records/sec | ✅ 236x target |
| Health endpoint | 22.5K RPS | ✅ 2.2x target |
| Gateway init | 1.8M ops/sec | ✅ Excellent |

## Key Performance Targets

- **Health endpoint**: >10,000 RPS ✅ **22,546 RPS**
- **Analytics batch**: >100 events/sec ✅ **23,643 events/sec**
- **Gateway overhead**: <10ms ✅ **0.044ms**
- **LLM proxy**: 500-2,000 RPS 🔍 *End-to-end testing needed*

## Regression Detection

**Critical**: >30% decrease ❌
**Warning**: >15% decrease ⚠️
**Acceptable**: <15% variance ✅

## Memory Profiling
```bash
go test -bench=BenchmarkAnalyticsDataInsertion ./analytics/ -memprofile=mem.prof
go tool pprof mem.prof
```

## Troubleshooting

- **Build errors**: Check `go build ./performance/framework/`
- **Large output**: Add `2>/dev/null` to suppress logs
- **Directory errors**: Ensure you're in project root

## Framework Status

✅ **Performance framework**: Fixed and working
✅ **Analytics benchmarks**: All working
✅ **Proxy benchmarks**: Core tests working
✅ **Gateway benchmarks**: Core tests working
🔍 **End-to-end testing**: Needs LLM integration tests
🔍 **Load testing**: Needs concurrent request testing