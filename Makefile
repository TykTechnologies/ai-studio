# Makefile

# Variables
ADMIN_FRONTEND_DIR := ui/admin-frontend
FORCE_BUILD := false

# Detect if enterprise submodule exists and is initialized
ENTERPRISE_EXISTS := $(shell test -f enterprise/.git && echo "yes" || echo "no")

ifeq ($(ENTERPRISE_EXISTS),yes)
    BUILD_TAGS := -tags enterprise
    EDITION := ent
    $(info 🏢 Building Enterprise Edition)
else
    BUILD_TAGS :=
    EDITION := ce
    $(info 🌍 Building Community Edition)
endif

# Build flags for production (optimized, stripped)
PROD_LDFLAGS := -ldflags="-w -s" -trimpath

# Default target
.DEFAULT_GOAL := help
all: build

# Help target
help:
	@echo "Tyk AI Studio Build Targets"
	@echo ""
	@echo "Current edition: $(EDITION)"
	@echo ""
	@echo "Development (single platform, fast):"
	@echo "  make build-native       - Build for current platform with CGO (auto-detect edition)"
	@echo "  make build-native-ce    - Build CE for current platform with CGO"
	@echo "  make build-native-ent   - Build ENT for current platform with CGO"
	@echo "  make build-local        - Alias for build-native (backward compatible)"
	@echo ""
	@echo "Production (multi-platform, optimized, CGO-enabled):"
	@echo "  make build-prod         - Build all platforms with CGO (auto-detect edition)"
	@echo "  make build-prod-ce      - Build CE for all platforms with CGO"
	@echo "  make build-prod-ent     - Build ENT for all platforms with CGO"
	@echo ""
	@echo "Platform-specific (CGO-enabled):"
	@echo "  make build-linux-amd64  - Linux AMD64 with CGO"
	@echo "  make build-linux-arm64  - Linux ARM64 with CGO (requires cross-compiler)"
	@echo "  make build-darwin-amd64 - Darwin AMD64 with CGO"
	@echo "  make build-darwin-arm64 - Darwin ARM64 with CGO"
	@echo ""
	@echo "Other targets:"
	@echo "  make plugins            - Build all plugins"
	@echo "  make test               - Run tests"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make show-edition       - Show current edition info"
	@echo ""

# Build target (default to production multi-platform builds)
build: build-prod

# Build frontend
build-frontend:
	cd $(ADMIN_FRONTEND_DIR) && npm run build

# ============================================================================
# Development Builds (single platform, fast, CGO enabled)
# ============================================================================

# Build for native platform (auto-detect edition)
build-native: build-frontend
	@echo "🔨 Building $(EDITION) for native platform with CGO..."
	@mkdir -p bin
	CGO_ENABLED=1 go build $(BUILD_TAGS) -o bin/midsommar-$(EDITION)
	cd microgateway && $(MAKE) build
	cp microgateway/dist/microgateway-$(EDITION) bin/mgw-$(EDITION)
	chmod +x bin/*
	@echo "✅ Native build complete: bin/midsommar-$(EDITION) and bin/mgw-$(EDITION)"

# Build CE for native platform
build-native-ce: build-frontend
	@echo "🔨 Building CE for native platform with CGO..."
	@mkdir -p bin
	CGO_ENABLED=1 go build -o bin/midsommar-ce
	cd microgateway && $(MAKE) build-community
	cp microgateway/dist/microgateway-ce bin/mgw-ce
	chmod +x bin/*
	@echo "✅ Native CE build complete"

# Build ENT for native platform
build-native-ent: build-frontend
	@if [ ! -f enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo "Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "🔨 Building ENT for native platform with CGO..."
	@mkdir -p bin
	CGO_ENABLED=1 go build -tags enterprise -o bin/midsommar-ent
	cd microgateway && $(MAKE) build-enterprise
	cp microgateway/dist/microgateway-ent bin/mgw-ent
	chmod +x bin/*
	@echo "✅ Native ENT build complete"

# ============================================================================
# Production Builds (multi-platform, optimized, CGO enabled)
# ============================================================================

# Build production artifacts for all platforms (auto-detect edition)
build-prod: build-frontend
	@echo "🏗️  Building production $(EDITION) artifacts..."
	@mkdir -p bin
	$(MAKE) build-linux-amd64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	$(MAKE) build-linux-arm64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	$(MAKE) build-darwin-amd64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	$(MAKE) build-darwin-arm64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	cd microgateway && $(MAKE) build-prod EDITION=$(EDITION)
	cp microgateway/dist/microgateway-$(EDITION)-linux-amd64 bin/mgw-$(EDITION)-linux-amd64 2>/dev/null || true
	cp microgateway/dist/microgateway-$(EDITION)-linux-arm64 bin/mgw-$(EDITION)-linux-arm64 2>/dev/null || true
	cp microgateway/dist/microgateway-$(EDITION)-darwin-amd64 bin/mgw-$(EDITION)-darwin-amd64 2>/dev/null || true
	cp microgateway/dist/microgateway-$(EDITION)-darwin-arm64 bin/mgw-$(EDITION)-darwin-arm64 2>/dev/null || true
	chmod +x bin/* 2>/dev/null || true
	@echo "✅ Production build complete"

# Build production CE for all platforms
build-prod-ce: build-frontend
	@echo "🏗️  Building production CE artifacts..."
	@mkdir -p bin
	$(MAKE) build-linux-amd64 EDITION=ce BUILD_TAGS=""
	$(MAKE) build-linux-arm64 EDITION=ce BUILD_TAGS=""
	$(MAKE) build-darwin-amd64 EDITION=ce BUILD_TAGS=""
	$(MAKE) build-darwin-arm64 EDITION=ce BUILD_TAGS=""
	cd microgateway && $(MAKE) build-prod-ce
	cp microgateway/dist/microgateway-ce-linux-amd64 bin/mgw-ce-linux-amd64 2>/dev/null || true
	cp microgateway/dist/microgateway-ce-linux-arm64 bin/mgw-ce-linux-arm64 2>/dev/null || true
	cp microgateway/dist/microgateway-ce-darwin-amd64 bin/mgw-ce-darwin-amd64 2>/dev/null || true
	cp microgateway/dist/microgateway-ce-darwin-arm64 bin/mgw-ce-darwin-arm64 2>/dev/null || true
	chmod +x bin/* 2>/dev/null || true
	@echo "✅ Production CE build complete"

# Build production ENT for all platforms
build-prod-ent: build-frontend
	@if [ ! -f enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo "Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "🏗️  Building production ENT artifacts..."
	@mkdir -p bin
	$(MAKE) build-linux-amd64 EDITION=ent BUILD_TAGS="-tags enterprise"
	$(MAKE) build-linux-arm64 EDITION=ent BUILD_TAGS="-tags enterprise"
	$(MAKE) build-darwin-amd64 EDITION=ent BUILD_TAGS="-tags enterprise"
	$(MAKE) build-darwin-arm64 EDITION=ent BUILD_TAGS="-tags enterprise"
	cd microgateway && $(MAKE) build-prod-ent
	cp microgateway/dist/microgateway-ent-linux-amd64 bin/mgw-ent-linux-amd64 2>/dev/null || true
	cp microgateway/dist/microgateway-ent-linux-arm64 bin/mgw-ent-linux-arm64 2>/dev/null || true
	cp microgateway/dist/microgateway-ent-darwin-amd64 bin/mgw-ent-darwin-amd64 2>/dev/null || true
	cp microgateway/dist/microgateway-ent-darwin-arm64 bin/mgw-ent-darwin-arm64 2>/dev/null || true
	chmod +x bin/* 2>/dev/null || true
	@echo "✅ Production ENT build complete"

# ============================================================================
# Platform-Specific Builds (CGO enabled)
# ============================================================================

# Linux AMD64 with CGO
build-linux-amd64:
	@echo "Building midsommar $(EDITION) for linux/amd64 with CGO..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	go build $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o bin/midsommar-$(EDITION)-linux-amd64

# Linux ARM64 with CGO (requires cross-compiler)
build-linux-arm64:
	@echo "Building midsommar $(EDITION) for linux/arm64 with CGO..."
	@if ! which aarch64-linux-gnu-gcc > /dev/null 2>&1; then \
		echo "⚠️  Warning: aarch64-linux-gnu-gcc not found"; \
		echo "   Install: sudo apt-get install gcc-aarch64-linux-gnu (Ubuntu/Debian)"; \
		echo "   Or: brew install FiloSottile/musl-cross/musl-cross (macOS)"; \
		echo "   Attempting build anyway..."; \
	fi
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
	CC=aarch64-linux-gnu-gcc \
	go build $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o bin/midsommar-$(EDITION)-linux-arm64

# Darwin AMD64 with CGO
build-darwin-amd64:
	@echo "Building midsommar $(EDITION) for darwin/amd64 with CGO..."
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
	go build $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o bin/midsommar-$(EDITION)-darwin-amd64

# Darwin ARM64 with CGO
build-darwin-arm64:
	@echo "Building midsommar $(EDITION) for darwin/arm64 with CGO..."
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
	go build $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o bin/midsommar-$(EDITION)-darwin-arm64

# ============================================================================
# Backward Compatibility Targets
# ============================================================================

# Build for local development (alias for build-native)
build-local: build-native

# Legacy build-community (now uses explicit CGO builds)
build-community: build-prod-ce
	@echo "Note: build-community now builds production artifacts with CGO"
	@echo "For quick local builds, use: make build-native-ce"

# Legacy build-enterprise (now uses explicit CGO builds)
build-enterprise: build-prod-ent
	@echo "Note: build-enterprise now builds production artifacts with CGO"
	@echo "For quick local builds, use: make build-native-ent"

# Build all plugins
plugins:
	@echo "Building studio plugins..."
	@failed_plugins=""; \
	for plugin in examples/plugins/studio/*/; do \
		if [ -d "$$plugin/server" ]; then \
			plugin_name=$$(basename "$$plugin"); \
			echo "Building studio plugin: $$plugin_name"; \
			if (cd "$$plugin/server" && go build -o ../$$plugin_name); then \
				echo "  ✓ Successfully built $$plugin_name"; \
			else \
				echo "  ✗ Failed to build $$plugin_name"; \
				failed_plugins="$$failed_plugins $$plugin_name"; \
			fi; \
		fi; \
	done; \
	echo "Building gateway plugins..."; \
	for plugin in examples/plugins/gateway/*/; do \
		if [ -f "$$plugin/main.go" ]; then \
			plugin_name=$$(basename "$$plugin"); \
			echo "Building gateway plugin: $$plugin_name"; \
			if (cd microgateway && go build -o "../$$plugin/$$plugin_name" "../$$plugin"); then \
				echo "  ✓ Successfully built $$plugin_name"; \
			else \
				echo "  ✗ Failed to build $$plugin_name"; \
				failed_plugins="$$failed_plugins $$plugin_name"; \
			fi; \
		fi; \
	done; \
	if [ -n "$$failed_plugins" ]; then \
		echo ""; \
		echo "⚠️  Some plugins failed to build:$$failed_plugins"; \
		echo "The following plugins built successfully can be used."; \
	else \
		echo ""; \
		echo "✅ All plugins built successfully!"; \
	fi

# Test target
test:
	go test $(BUILD_TAGS) ./...
	cd microgateway && go test $(BUILD_TAGS) ./...

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
	rm -rf bin/
	rm -rf $(ADMIN_FRONTEND_DIR)/build
	cd microgateway && rm -rf bin/ dist/

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

# ============================================================================
# Enterprise Submodule Management
# ============================================================================

.PHONY: init-enterprise
init-enterprise:
	@echo "🔐 Initializing enterprise submodule..."
	@git submodule init
	@git submodule update --remote
	@if [ -f enterprise/.git ]; then \
		echo "✅ Enterprise edition initialized successfully"; \
		echo "Run 'make build' to build enterprise edition"; \
	else \
		echo "❌ Failed to initialize enterprise submodule"; \
		echo "You may not have access to the private repository"; \
		echo "For enterprise access: contact enterprise@tyk.io"; \
		exit 1; \
	fi

.PHONY: update-enterprise
update-enterprise:
	@if [ ! -f enterprise/.git ]; then \
		echo "❌ Enterprise submodule not initialized"; \
		echo "Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "Updating enterprise submodule..."
	@git submodule update --remote enterprise
	@git add enterprise
	@echo "✅ Enterprise submodule updated"
	@echo "Commit the change: git commit -m 'Update enterprise submodule'"

.PHONY: show-edition
show-edition:
	@echo "Current edition: $(EDITION)"
	@if [ "$(ENTERPRISE_EXISTS)" = "yes" ]; then \
		echo "Enterprise commit:"; \
		cd enterprise && git log -1 --oneline; \
	else \
		echo "Enterprise submodule: not initialized"; \
	fi

.PHONY: all build help build-frontend \
	build-native build-native-ce build-native-ent \
	build-prod build-prod-ce build-prod-ent \
	build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 \
	build-local build-community build-enterprise \
	plugins test clean start-dev stop-dev \
	perf-test perf-profile perf-baseline perf-compare perf-report perf-clean \
	init-enterprise update-enterprise show-edition
