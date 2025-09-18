# Makefile

# Variables
ADMIN_FRONTEND_DIR := ui/admin-frontend
FORCE_BUILD := false

# Default target
all: build

# Build target
build: build-frontend build-binaries

# Build frontend
build-frontend:
	cd $(ADMIN_FRONTEND_DIR) && npm run build

# Build Go binaries for all architectures
build-binaries:
	GOOS=linux GOARCH=amd64 go build -o midsommar-amd64
	GOOS=linux GOARCH=arm64 go build -o midsommar-arm64
	chmod +x midsommar-*

# Build for local development (single architecture)
build-local:
	cd $(ADMIN_FRONTEND_DIR) && npm run build
	go build -o midsommar

# Test target
test:
	go test ./...

# Performance testing targets
perf-test:
	@echo "Running performance test suite..."
	go test -bench=BenchmarkProxy* ./proxy/ -benchmem
	go test -bench=BenchmarkGateway* ./pkg/aigateway/ -benchmem
	go test -bench=BenchmarkAnalytics* ./analytics/ -benchmem
	go test -bench=BenchmarkService* ./services/ -benchmem
	go test -bench=BenchmarkLoad* ./tests/performance/ -benchtime=30s

perf-profile:
	@echo "Running performance tests with profiling..."
	go test -bench=BenchmarkProxyRequest ./proxy/ -cpuprofile=cpu.prof -memprofile=mem.prof -benchmem
	@echo "Profile files generated: cpu.prof, mem.prof"
	@echo "Analyze with: go tool pprof cpu.prof or go tool pprof mem.prof"

perf-baseline:
	@echo "Establishing performance baseline..."
	@mkdir -p performance/baselines
	go test -bench=. ./... -benchmem -count=5 > performance/baselines/baseline-$(shell date +%Y%m%d-%H%M%S).txt
	@echo "Baseline saved to performance/baselines/"

perf-compare:
	@echo "Comparing current performance to baseline..."
	@if [ ! -d performance/baselines ]; then \
		echo "No baselines found. Run 'make perf-baseline' first."; \
		exit 1; \
	fi
	@LATEST_BASELINE=$$(ls -t performance/baselines/baseline-*.txt | head -n 1); \
	go test -bench=. ./... -benchmem -count=5 > performance/current-run.txt; \
	echo "Comparing against: $$LATEST_BASELINE"; \
	echo "Current results saved to: performance/current-run.txt"; \
	echo "Use benchstat to compare: benchstat $$LATEST_BASELINE performance/current-run.txt"

perf-report:
	@echo "Generating performance report..."
	@mkdir -p performance/reports
	@REPORT_FILE=performance/reports/report-$(shell date +%Y%m%d-%H%M%S).html; \
	echo "<!DOCTYPE html><html><head><title>Performance Report</title></head><body>" > $$REPORT_FILE; \
	echo "<h1>Midsommar Performance Report - $(shell date)</h1>" >> $$REPORT_FILE; \
	echo "<h2>System Info</h2><pre>" >> $$REPORT_FILE; \
	go version >> $$REPORT_FILE; \
	echo "CPU: $(shell nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'unknown')" >> $$REPORT_FILE; \
	echo "Memory: $(shell free -h 2>/dev/null | grep '^Mem:' | awk '{print $$2}' || echo 'unknown')" >> $$REPORT_FILE; \
	echo "</pre><h2>Benchmark Results</h2><pre>" >> $$REPORT_FILE; \
	go test -bench=. ./... -benchmem >> $$REPORT_FILE 2>&1; \
	echo "</pre></body></html>" >> $$REPORT_FILE; \
	echo "Report generated: $$REPORT_FILE"

perf-clean:
	@echo "Cleaning performance test artifacts..."
	rm -f *.prof
	rm -f performance/current-run.txt
	rm -rf performance/reports/*
	@echo "Performance artifacts cleaned"

# Clean target
clean:
	rm -f midsommar*
	rm -rf $(ADMIN_FRONTEND_DIR)/build

# Development targets
start-dev: stop-dev
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ".env file created from .env.example"; \
	fi
	@if [ ! -d "../langchaingo" ]; then \
		git clone https://github.com/lonelycode/langchaingo ../langchaingo; \
		echo "langchaingo repository cloned"; \
	fi
	@screen -dmS midsommar -t frontend bash -c 'cd $(ADMIN_FRONTEND_DIR) && SITE_URL=http://localhost:8080 npm start; read -n 1'
	@screen -S midsommar -X screen -t backend bash -c 'go build && ./midsommar; read -n 1'
	@screen -r midsommar

stop-dev:
	@pkill -f "npm start" || true
	@pkill -f "./midsommar" || true
	@screen -S midsommar -X quit || true

# Start pre-defined test env in docker
start-test-env:
	echo "Make sure the frontend is already built"
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ".env file created from .env.example"; \
	fi
	echo "Creating copy of the postgres test data"
	mkdir -p ./tests/postgres_data_temp
	cp -r tests/postgres_data ./tests/postgres_data_temp
	echo "Starting the test environment"
	docker compose --env-file .env -f tests/compose.yml up

# Stop pre-defined test env in docker
stop-test-env:
	echo "Stopping the test environment"
	docker compose --env-file .env -f tests/compose.yml down
	echo "Removing the copy of the postgres test data"
	rm -rf /tests/postgres_temp

# Build extras only (transformer and reranker)
build-docker-extras:
	cd extra/transformer_server && \
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/transformer_server_cpu:latest --push -f dockerfile.cpu .
	cd extra/reranker && \
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/reranker_cpu:latest --push -f dockerfile.cpu .

.PHONY: all build build-frontend build-binaries build-local test clean start-dev stop-dev perf-test perf-profile perf-baseline perf-compare perf-report perf-clean
