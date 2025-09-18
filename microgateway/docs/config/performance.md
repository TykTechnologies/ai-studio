# Performance Tuning

The microgateway provides extensive performance tuning options for optimal throughput, latency, and resource utilization in production environments.

## Overview

Performance configuration areas:
- **Connection Pooling**: Database and HTTP connection optimization
- **Caching Strategy**: In-memory caching for frequently accessed data
- **Request Processing**: Gateway timeout and concurrency settings
- **Analytics Performance**: Buffer and batch processing optimization
- **Resource Limits**: Memory and CPU utilization controls
- **Network Optimization**: Keep-alive and compression settings

## Database Performance

### Connection Pool Tuning
```bash
# High-concurrency settings
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=50
DB_CONN_MAX_LIFETIME=10m
DB_CONN_MAX_IDLE_TIME=30m

# Low-concurrency settings
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME=1h

# Monitor connection usage
mgw system metrics | grep db_connections_in_use
```

### Query Optimization
```bash
# Enable query performance monitoring
DB_LOG_LEVEL=info
DB_SLOW_QUERY_THRESHOLD=1s

# Database-specific optimizations
# PostgreSQL settings in postgresql.conf:
shared_buffers = 256MB      # 25% of RAM
effective_cache_size = 1GB  # 75% of RAM
checkpoint_completion_target = 0.9
wal_buffers = 16MB
random_page_cost = 1.1      # For SSD storage
```

## Cache Performance

### In-Memory Cache Tuning
```bash
# Cache size optimization
CACHE_ENABLED=true
CACHE_MAX_SIZE=10000        # Number of cache entries
CACHE_TTL=30m               # Cache time-to-live
CACHE_CLEANUP_INTERVAL=5m   # Cleanup frequency

# High-performance caching
CACHE_MAX_SIZE=50000
CACHE_TTL=15m
CACHE_CLEANUP_INTERVAL=2m

# Memory-conscious caching
CACHE_MAX_SIZE=1000
CACHE_TTL=1h
CACHE_CLEANUP_INTERVAL=15m
```

### Cache Strategy
```bash
# Cache what to optimize
# - Authentication tokens (high hit rate)
# - LLM configurations (frequently accessed)
# - Application settings (read-heavy)
# - Model pricing (rarely changes)

# Cache monitoring
mgw system metrics | grep cache_hit_ratio
mgw system metrics | grep cache_evictions_total
```

## Gateway Performance

### Request Processing
```bash
# Gateway timeout settings
GATEWAY_TIMEOUT=30s         # Standard timeout
GATEWAY_TIMEOUT=15s         # Low-latency applications
GATEWAY_TIMEOUT=60s         # Large model operations

# Request size limits
GATEWAY_MAX_REQUEST_SIZE=10MB
GATEWAY_MAX_RESPONSE_SIZE=50MB

# Concurrency settings
GATEWAY_MAX_CONCURRENT_REQUESTS=1000
GATEWAY_WORKER_POOL_SIZE=100
```

### HTTP Server Tuning
```bash
# HTTP server performance
READ_TIMEOUT=15s
WRITE_TIMEOUT=15s
IDLE_TIMEOUT=60s

# Keep-alive settings
HTTP_KEEP_ALIVE_ENABLED=true
HTTP_KEEP_ALIVE_TIMEOUT=60s
HTTP_MAX_IDLE_CONNS=100
HTTP_MAX_CONNS_PER_HOST=50
```

### Compression
```bash
# Response compression
COMPRESSION_ENABLED=true
COMPRESSION_LEVEL=6         # Balance between speed and compression
COMPRESSION_MIN_SIZE=1024   # Only compress responses > 1KB

# Compression algorithms
COMPRESSION_ALGORITHM=gzip  # gzip, deflate, br (brotli)
```

## Analytics Performance

### Buffer Optimization
```bash
# High-throughput settings
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=5s
ANALYTICS_WORKERS=10
ANALYTICS_BATCH_SIZE=500

# Balanced settings
ANALYTICS_BUFFER_SIZE=2000
ANALYTICS_FLUSH_INTERVAL=10s
ANALYTICS_WORKERS=5
ANALYTICS_BATCH_SIZE=200

# Low-resource settings
ANALYTICS_BUFFER_SIZE=500
ANALYTICS_FLUSH_INTERVAL=30s
ANALYTICS_WORKERS=2
ANALYTICS_BATCH_SIZE=50
```

### Real-Time vs Batch Processing
```bash
# Real-time analytics (higher resource usage)
ANALYTICS_REALTIME=true
ANALYTICS_IMMEDIATE_PROCESSING=true

# Batch analytics (better performance)
ANALYTICS_REALTIME=false
ANALYTICS_BATCH_PROCESSING=true
ANALYTICS_BATCH_INTERVAL=60s
```

## Memory Optimization

### Memory Allocation
```bash
# Go memory settings
GOGC=100                    # Default garbage collection target
GOGC=50                     # More aggressive GC (lower memory)
GOGC=200                    # Less aggressive GC (higher performance)

# Memory limits
GOMEMLIMIT=512MB            # Soft memory limit
GOMAXPROCS=4                # CPU cores to use
```

### Buffer Management
```bash
# Analytics buffers
ANALYTICS_MAX_MEMORY=256MB
ANALYTICS_BUFFER_SIZE=2000

# Cache memory
CACHE_MAX_MEMORY=128MB
CACHE_MAX_SIZE=10000

# Plugin memory
PLUGINS_MAX_MEMORY=256MB
PLUGINS_MAX_INSTANCES=5
```

## CPU Optimization

### Concurrency Settings
```bash
# Worker pool optimization
GATEWAY_WORKER_POOL_SIZE=100
ANALYTICS_WORKERS=5
BACKGROUND_WORKERS=3

# CPU-bound operations
CPU_INTENSIVE_OPERATIONS_WORKERS=2
ENCRYPTION_WORKERS=4
COMPRESSION_WORKERS=4
```

### Processing Optimization
```bash
# Async processing
ASYNC_ANALYTICS_PROCESSING=true
ASYNC_AUDIT_LOGGING=true
ASYNC_PLUGIN_EXECUTION=false  # Plugins run synchronously

# Batch processing
BATCH_DATABASE_OPERATIONS=true
BATCH_ANALYTICS_WRITES=true
BATCH_SIZE=100
```

## Network Performance

### Connection Optimization
```bash
# HTTP client settings
HTTP_CLIENT_TIMEOUT=30s
HTTP_CLIENT_KEEP_ALIVE=true
HTTP_CLIENT_MAX_IDLE_CONNS=100
HTTP_CLIENT_MAX_CONNS_PER_HOST=50

# gRPC settings (hub-and-spoke)
GRPC_MAX_CONCURRENT_STREAMS=1000
GRPC_KEEPALIVE_TIME=30s
GRPC_KEEPALIVE_TIMEOUT=5s
GRPC_MAX_RECEIVE_MESSAGE_SIZE=4MB
```

### LLM Provider Optimization
```bash
# Provider-specific timeouts
OPENAI_TIMEOUT=30s
ANTHROPIC_TIMEOUT=45s
OLLAMA_TIMEOUT=120s         # Local models may be slower

# Connection pooling per provider
LLM_CONNECTION_POOL_SIZE=10
LLM_MAX_IDLE_CONNS=5
LLM_CONN_TIMEOUT=10s
```

## Performance Monitoring

### Key Performance Metrics
```bash
# Monitor performance metrics
curl http://localhost:8080/metrics | grep -E "(request_duration|throughput|latency)"

# Key metrics to monitor:
# - request_duration_seconds (histogram)
# - requests_per_second (gauge)
# - response_size_bytes (histogram)
# - concurrent_requests (gauge)
# - cache_hit_ratio (gauge)
# - database_query_duration_seconds (histogram)
```

### Performance Benchmarking
```bash
# Load testing with hey
go install github.com/rakyll/hey@latest

# Test health endpoint
hey -n 10000 -c 100 http://localhost:8080/health

# Test API endpoint (with auth)
hey -n 1000 -c 10 \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/llms

# Test LLM proxy (with auth)
hey -n 100 -c 5 \
  -H "Authorization: Bearer $APP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"test"}]}' \
  http://localhost:8080/llm/rest/gpt-3.5-turbo/chat/completions
```

### Resource Monitoring
```bash
# Monitor system resources
# CPU usage
top -p $(pgrep microgateway)

# Memory usage
ps aux | grep microgateway | awk '{print $6}'  # RSS memory

# File descriptors
lsof -p $(pgrep microgateway) | wc -l

# Network connections
netstat -an | grep :8080 | wc -l
```

## Performance Profiles

### High-Throughput Profile
```bash
# Optimized for maximum throughput
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=50
CACHE_MAX_SIZE=50000
CACHE_TTL=15m

ANALYTICS_BUFFER_SIZE=10000
ANALYTICS_FLUSH_INTERVAL=5s
ANALYTICS_WORKERS=20

GATEWAY_WORKER_POOL_SIZE=200
GATEWAY_MAX_CONCURRENT_REQUESTS=2000
COMPRESSION_ENABLED=false   # Disable for maximum speed

GOGC=200                    # Less frequent GC
```

### Low-Latency Profile
```bash
# Optimized for minimum latency
GATEWAY_TIMEOUT=10s
READ_TIMEOUT=5s
WRITE_TIMEOUT=5s

CACHE_TTL=5m                # Shorter TTL for fresher data
ANALYTICS_REALTIME=true     # Immediate analytics processing

DB_MAX_OPEN_CONNS=25        # Fewer connections, faster access
COMPRESSION_ENABLED=false   # No compression overhead

GOGC=50                     # More frequent GC for lower latency
```

### Balanced Profile
```bash
# Balanced performance and resource usage
DB_MAX_OPEN_CONNS=25
CACHE_MAX_SIZE=10000
CACHE_TTL=30m

ANALYTICS_BUFFER_SIZE=2000
ANALYTICS_FLUSH_INTERVAL=10s
ANALYTICS_WORKERS=5

GATEWAY_TIMEOUT=30s
COMPRESSION_ENABLED=true
COMPRESSION_LEVEL=6

GOGC=100                    # Default GC settings
```

### Resource-Constrained Profile
```bash
# Optimized for low resource usage
DB_MAX_OPEN_CONNS=5
DB_MAX_IDLE_CONNS=5
CACHE_MAX_SIZE=500
CACHE_TTL=2h

ANALYTICS_BUFFER_SIZE=100
ANALYTICS_FLUSH_INTERVAL=60s
ANALYTICS_WORKERS=1

GATEWAY_WORKER_POOL_SIZE=10
COMPRESSION_ENABLED=true

GOGC=50                     # Aggressive GC for memory efficiency
GOMEMLIMIT=256MB
```

## Performance Troubleshooting

### High Latency Issues
```bash
# Identify latency sources
mgw analytics summary 1 --format=json | jq '.data.average_latency'

# Check database query performance
mgw system metrics | grep db_query_duration

# Monitor LLM provider latency
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.llm_id) | map({llm_id: .[0].llm_id, avg_latency: (map(.latency_ms) | add / length)})'

# Analyze slow requests
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.latency_ms > 5000) | {endpoint, latency_ms, total_tokens}'
```

### High Memory Usage
```bash
# Monitor memory usage
ps aux | grep microgateway

# Check cache usage
mgw system metrics | grep cache_entries

# Monitor analytics buffer
mgw system metrics | grep analytics_buffer_size

# Garbage collection metrics
mgw system metrics | grep go_gc
```

### High CPU Usage
```bash
# Monitor CPU usage
top -p $(pgrep microgateway)

# Check worker utilization
mgw system metrics | grep worker_pool

# Monitor analytics processing
mgw system metrics | grep analytics_processing_time

# Profile CPU usage
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
```

### Database Performance Issues
```bash
# Monitor database connections
mgw system metrics | grep db_connections

# Check for connection pool exhaustion
mgw system metrics | grep db_connections_wait_count

# Analyze slow queries
tail -f /var/log/postgresql/postgresql.log | grep "slow query"

# Database connection debugging
DB_LOG_LEVEL=debug ./microgateway
```

## Performance Testing

### Load Testing Strategy
```bash
# Progressive load testing
# 1. Baseline test (low load)
hey -n 1000 -c 1 http://localhost:8080/health

# 2. Normal load test
hey -n 10000 -c 50 -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/llms

# 3. Stress test (high load)
hey -n 50000 -c 200 -H "Authorization: Bearer $APP_TOKEN" \
  http://localhost:8080/llm/rest/gpt-3.5-turbo/chat/completions

# 4. Endurance test (sustained load)
hey -n 100000 -c 10 -q 100 \
  http://localhost:8080/health
```

### Performance Benchmarks
```bash
# Typical performance expectations:
# - Health endpoint: >10,000 RPS
# - Management API: >1,000 RPS  
# - LLM proxy: 500-2,000 RPS (depends on LLM latency)
# - Memory usage: 100-500MB (depends on cache and buffer sizes)
# - CPU usage: 10-30% under normal load

# Benchmark script
#!/bin/bash
echo "Microgateway Performance Benchmark"
echo "=================================="

# Health endpoint
echo "Health endpoint:"
hey -n 10000 -c 100 http://localhost:8080/health | grep "Requests/sec"

# Management API
echo "Management API:"
hey -n 1000 -c 10 -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/llms | grep "Requests/sec"

# Resource usage
echo "Resource usage:"
ps aux | grep microgateway | awk '{print "CPU: " $3 "%, Memory: " $4 "%"}'
```

## Optimization Strategies

### CPU Optimization
```bash
# CPU-bound optimizations
GOMAXPROCS=4                # Match available CPU cores
GATEWAY_WORKER_POOL_SIZE=200 # 50x CPU cores
ANALYTICS_WORKERS=8         # 2x CPU cores

# Reduce CPU overhead
COMPRESSION_ENABLED=false   # Disable if CPU-bound
ANALYTICS_REALTIME=false    # Use batch processing
CACHE_CLEANUP_INTERVAL=30m  # Less frequent cleanup
```

### Memory Optimization
```bash
# Memory-constrained optimizations
CACHE_MAX_SIZE=1000
ANALYTICS_BUFFER_SIZE=500
GOGC=50                     # Aggressive garbage collection
GOMEMLIMIT=512MB

# Monitor memory allocation
mgw system metrics | grep go_memstats
```

### I/O Optimization
```bash
# Database I/O optimization
DB_MAX_OPEN_CONNS=25
DB_CONN_MAX_LIFETIME=5m
DB_BATCH_SIZE=100

# File I/O optimization
LOG_BUFFER_SIZE=64KB
LOG_SYNC_INTERVAL=1s
ASYNC_LOG_WRITING=true
```

### Network Optimization
```bash
# HTTP optimization
HTTP_KEEP_ALIVE_ENABLED=true
HTTP_KEEP_ALIVE_TIMEOUT=60s
HTTP_MAX_HEADER_SIZE=1MB

# Compression for bandwidth
COMPRESSION_ENABLED=true
COMPRESSION_LEVEL=6         # Balance speed vs compression ratio

# Connection reuse
HTTP_MAX_IDLE_CONNS=100
HTTP_IDLE_CONN_TIMEOUT=90s
```

## Scaling Configuration

### Horizontal Scaling
```bash
# Stateless configuration for horizontal scaling
# No session affinity required
# Database handles concurrent access
# Cache invalidation handled automatically

# Load balancer configuration
# - Health check: GET /health
# - Readiness check: GET /ready
# - Session affinity: None required
# - Load balancing algorithm: Round-robin or least-connections
```

### Vertical Scaling
```bash
# CPU scaling
GOMAXPROCS=auto             # Use all available cores
GATEWAY_WORKER_POOL_SIZE=auto # Scale with CPU cores

# Memory scaling
CACHE_MAX_SIZE=auto         # Scale with available memory
ANALYTICS_BUFFER_SIZE=auto  # Scale with memory

# Auto-scaling formulas
# GATEWAY_WORKER_POOL_SIZE = CPU_CORES * 50
# CACHE_MAX_SIZE = (AVAILABLE_MEMORY_MB - 200) * 10
# ANALYTICS_BUFFER_SIZE = AVAILABLE_MEMORY_MB * 5
```

## Performance Monitoring

### Real-Time Monitoring
```bash
# Monitor key performance indicators
watch -n 1 'curl -s http://localhost:8080/metrics | grep -E "(request_duration|cache_hit|db_connections)"'

# Resource utilization
watch -n 1 'ps aux | grep microgateway | awk "{print \"CPU: \" \$3 \"%, Memory: \" \$6 \" KB\"}"'

# Request rates
watch -n 1 'mgw analytics summary 1 --format=json | jq ".data.requests_per_hour"'
```

### Performance Alerting
```bash
# Set up performance alerts
# Response time > 5 seconds
# Error rate > 5%
# Memory usage > 80%
# CPU usage > 90%
# Cache hit rate < 80%

# Example monitoring script
#!/bin/bash
LATENCY=$(mgw analytics summary 1 --format=json | jq '.data.average_latency')
if (( $(echo "$LATENCY > 5000" | bc -l) )); then
  echo "Performance Alert: High latency - ${LATENCY}ms"
fi

ERROR_RATE=$(mgw analytics summary 1 --format=json | jq '.data.failed_requests / .data.total_requests')
if (( $(echo "$ERROR_RATE > 0.05" | bc -l) )); then
  echo "Performance Alert: High error rate - ${ERROR_RATE}"
fi
```

## Environment-Specific Performance

### Development Performance
```bash
# Development optimizations
LOG_LEVEL=debug             # More verbose logging
ANALYTICS_REALTIME=true     # Immediate feedback
CACHE_TTL=5m               # Shorter cache for rapid development
DB_LOG_LEVEL=info          # Database query logging
```

### Production Performance
```bash
# Production optimizations
LOG_LEVEL=info
LOG_FORMAT=json
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=30s
CACHE_MAX_SIZE=20000
CACHE_TTL=1h
DB_MAX_OPEN_CONNS=50
COMPRESSION_ENABLED=true
```

### High-Load Production
```bash
# High-load optimizations
DB_MAX_OPEN_CONNS=100
CACHE_MAX_SIZE=100000
ANALYTICS_BUFFER_SIZE=20000
ANALYTICS_WORKERS=20
GATEWAY_WORKER_POOL_SIZE=500
GOGC=200
COMPRESSION_LEVEL=1         # Fastest compression
```

## Performance Best Practices

### Configuration Tuning
- Monitor key metrics during load testing
- Tune one parameter at a time
- Test configuration changes in staging first
- Document performance impact of changes
- Use appropriate profiles for your workload

### Resource Planning
- Plan for 2-3x expected peak load
- Monitor resource utilization trends
- Plan scaling before reaching limits
- Consider burst capacity requirements
- Account for background processing overhead

### Optimization Process
1. **Baseline**: Establish current performance metrics
2. **Identify Bottlenecks**: Use monitoring to find constraints
3. **Optimize**: Tune configuration for identified bottlenecks
4. **Test**: Validate performance improvements
5. **Monitor**: Continue monitoring after changes
6. **Iterate**: Repeat process for continuous optimization

---

Performance tuning ensures optimal microgateway operation under load. For monitoring setup, see [Monitoring Configuration](monitoring.md). For database optimization, see [Database Configuration](database.md).
