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

# Default target
all: build

# Build target
build: build-frontend build-binaries

# Build frontend
build-frontend:
	cd $(ADMIN_FRONTEND_DIR) && npm run build

# Build Go binaries for all architectures (linux amd64 and darwin amd64)
build-binaries:
	@echo "Building Midsommar $(EDITION) edition..."
	GOOS=linux GOARCH=amd64 go build $(BUILD_TAGS) -o bin/midsommar-$(EDITION)-linux-amd64
	GOOS=darwin GOARCH=amd64 go build $(BUILD_TAGS) -o bin/midsommar-$(EDITION)-darwin-amd64
	@echo "Building Microgateway $(EDITION) edition..."
	@mkdir -p bin
	cd microgateway && $(MAKE) build
	cp microgateway/dist/microgateway-$(EDITION) bin/mgw-$(EDITION)-linux-amd64
	# Build darwin version of microgateway
	cd microgateway && GOOS=darwin GOARCH=amd64 $(MAKE) GOOS=darwin GOARCH=amd64 build
	cp microgateway/dist/microgateway-$(EDITION) bin/mgw-$(EDITION)-darwin-amd64
	chmod +x bin/*
	@echo "✅ Built: bin/midsommar-$(EDITION)-* and bin/mgw-$(EDITION)-*"

# Build for local development (single architecture)
build-local:
	@echo "Building $(EDITION) edition for local development..."
	cd $(ADMIN_FRONTEND_DIR) && npm run build
	go build $(BUILD_TAGS) -o bin/midsommar-$(EDITION)
	@mkdir -p bin
	cd microgateway && $(MAKE) build
	cp microgateway/dist/microgateway-$(EDITION) bin/mgw-$(EDITION)
	@echo "✅ Built: bin/midsommar-$(EDITION) and bin/mgw-$(EDITION)"

# Force build Community Edition (ignore enterprise submodule)
build-community:
	@echo "🌍 Force building Community Edition..."
	cd $(ADMIN_FRONTEND_DIR) && npm run build
	@echo "Building Midsommar CE..."
	GOOS=linux GOARCH=amd64 go build -o bin/midsommar-ce-linux-amd64
	GOOS=darwin GOARCH=amd64 go build -o bin/midsommar-ce-darwin-amd64
	@echo "Building Microgateway CE..."
	@mkdir -p bin
	cd microgateway && $(MAKE) build-community
	cp microgateway/dist/microgateway-ce bin/mgw-ce-linux-amd64
	cd microgateway && GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.BuildHash=$$(git rev-parse HEAD 2>/dev/null || echo 'unknown') -X main.BuildTime=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o dist/microgateway-ce-darwin ./cmd/microgateway
	cp microgateway/dist/microgateway-ce-darwin bin/mgw-ce-darwin-amd64
	chmod +x bin/*
	@echo "✅ Built Community Edition: bin/midsommar-ce-* and bin/mgw-ce-*"

# Force build Enterprise Edition (require enterprise submodule)
build-enterprise:
	@if [ ! -f enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo ""; \
		echo "To build Enterprise Edition:"; \
		echo "  1. make init-enterprise"; \
		echo ""; \
		echo "For enterprise access: contact enterprise@tyk.io"; \
		exit 1; \
	fi
	@echo "🏢 Force building Enterprise Edition..."
	cd $(ADMIN_FRONTEND_DIR) && npm run build
	@echo "Building Midsommar ENT..."
	GOOS=linux GOARCH=amd64 go build -tags enterprise -o bin/midsommar-ent-linux-amd64
	GOOS=darwin GOARCH=amd64 go build -tags enterprise -o bin/midsommar-ent-darwin-amd64
	@echo "Building Microgateway ENT..."
	@mkdir -p bin
	cd microgateway && $(MAKE) build-enterprise
	cp microgateway/dist/microgateway-ent bin/mgw-ent-linux-amd64
	cd microgateway && GOOS=darwin GOARCH=amd64 go build -tags enterprise -ldflags "-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.BuildHash=$$(git rev-parse HEAD 2>/dev/null || echo 'unknown') -X main.BuildTime=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o dist/microgateway-ent-darwin ./cmd/microgateway
	cp microgateway/dist/microgateway-ent-darwin bin/mgw-ent-darwin-amd64
	chmod +x bin/*
	@echo "✅ Built Enterprise Edition: bin/midsommar-ent-* and bin/mgw-ent-*"

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

# Enterprise Edition specific targets
.PHONY: build-enterprise
build-enterprise:
	@if [ ! -f enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo ""; \
		echo "To build Enterprise Edition:"; \
		echo "  1. Ensure you have access to the private repository"; \
		echo "  2. Run: make init-enterprise"; \
		echo ""; \
		echo "For enterprise access: contact enterprise@tyk.io"; \
		exit 1; \
	fi
	$(MAKE) build BUILD_TAGS="-tags enterprise" EDITION=ent

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

.PHONY: all build build-frontend build-binaries build-local build-community build-enterprise plugins test clean start-dev stop-dev perf-test perf-profile perf-baseline perf-compare perf-report perf-clean init-enterprise update-enterprise show-edition
