# Makefile

# Variables
ADMIN_FRONTEND_DIR := ui/admin-frontend
FORCE_BUILD := false
SKIP_FRONTEND := false
SKIP_DOCS := false

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
	@echo "🐳 Docker Development (RECOMMENDED - with hot reloading):"
	@echo "  make dev                - Start minimal env (Studio + Frontend + Postgres)"
	@echo "  make dev-full           - Start full stack (+ Gateway + Plugins)"
	@echo "  make dev-ent            - Start enterprise minimal env"
	@echo "  make dev-full-ent       - Start enterprise full stack"
	@echo "  make dev-down           - Stop development environment"
	@echo "  make dev-help           - Show all development commands"
	@echo ""
	@echo "Development (single platform, fast):"
	@echo "  make build-native       - Build for current platform with CGO (auto-detect edition)"
	@echo "  make build-native-ce    - Build CE for current platform with CGO"
	@echo "  make build-native-ent   - Build ENT for current platform with CGO"
	@echo "  make build-local        - Alias for build-native (backward compatible)"
	@echo ""
	@echo "Development flags:"
	@echo "  SKIP_FRONTEND=true      - Skip frontend build (for faster iteration)"
	@echo "  SKIP_DOCS=true          - Skip docs build (for faster iteration)"
	@echo "  Example: make build-native-ce SKIP_FRONTEND=true SKIP_DOCS=true"
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
	@echo "Plugin Development:"
	@echo "  make tools              - Build development tools (plugin-scaffold)"
	@echo "  make plugin-new         - Scaffold a new plugin (NAME=x TYPE=y [CAPABILITIES=a,b])"
	@echo "  make plugin-help        - Show plugin scaffolding help"
	@echo "  make plugins            - Build all example plugins"
	@echo ""
	@echo "Packaging (RPM/DEB via GoReleaser + nfpm):"
	@echo "  make package            - Build all packages (auto-detect edition)"
	@echo "  make package-ce         - Build CE packages (both components)"
	@echo "  make package-ent        - Build ENT packages (both components)"
	@echo "  make package-help       - Show all packaging targets"
	@echo "  make test-package-smoke - Build CE packages and run smoke tests"
	@echo ""
	@echo "Other targets:"
	@echo "  make test               - Run tests"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make show-edition       - Show current edition info"
	@echo ""
	@echo "Integration Tests (requires Docker):"
	@echo "  make test-integration                      - Run all integration tests"
	@echo "  make test-integration-plugin-cache         - Run cache plugin integration tests"
	@echo "  make test-integration-plugin-cache-full    - Run all cache plugin tests (with cluster+syslog)"
	@echo "  make test-integration-plugin-cache-coverage - Run with coverage report"
	@echo "  make test-integration-plugin-github-rag    - Run github-rag-ingest Vault integration tests"
	@echo ""

# Build target (default to production multi-platform builds)
build: build-prod

# Build frontend (can be skipped with SKIP_FRONTEND=true)
build-frontend:
ifeq ($(SKIP_FRONTEND),true)
	@echo "⏭️  Skipping frontend build (SKIP_FRONTEND=true)"
else
	@echo "🔨 Building frontend..."
	cd $(ADMIN_FRONTEND_DIR) && npm run build
endif

# Build documentation site (can be skipped with SKIP_DOCS=true)
build-docs:
ifeq ($(SKIP_DOCS),true)
	@echo "⏭️  Skipping docs build (SKIP_DOCS=true)"
else
	@echo "📚 Building documentation..."
	cd docs/site && npm ci && npm run docs:build
endif

# ============================================================================
# Development Builds (single platform, fast, CGO enabled)
# ============================================================================

# Build for native platform (auto-detect edition)
build-native: build-frontend build-docs
	@echo "🔨 Building $(EDITION) for native platform with CGO..."
	@mkdir -p bin
	CGO_ENABLED=1 go build $(BUILD_TAGS) -o bin/midsommar-$(EDITION)
	cd microgateway && $(MAKE) build
	cp microgateway/dist/microgateway-$(EDITION) bin/mgw-$(EDITION)
	chmod +x bin/*
	@echo "✅ Native build complete: bin/midsommar-$(EDITION) and bin/mgw-$(EDITION)"

# Build CE for native platform
build-native-ce: build-frontend build-docs
	@echo "🔨 Building CE for native platform with CGO..."
	@mkdir -p bin
	CGO_ENABLED=1 go build -o bin/midsommar-ce
	cd microgateway && $(MAKE) build-community
	cp microgateway/dist/microgateway-ce bin/mgw-ce
	chmod +x bin/*
	@echo "✅ Native CE build complete"

# Build ENT for native platform
build-native-ent: build-frontend build-docs
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

# ============================================================================
# Package Building (RPM/DEB using GoReleaser + nfpm)
# ============================================================================
# These targets use the SAME GoReleaser configs and Docker image as CI,
# ensuring local packages are identical to release packages.
#
# Builds run inside the tykio/golang-cross Docker container (same as CI)
# because CGO cross-compilation requires Linux cross-compiler toolchains.
#
# Prerequisites:
#   Docker must be running
#
# Usage:
#   make package                    Build all packages (auto-detect edition)
#   make package-studio             Build AI Studio packages only
#   make package-microgateway       Build Microgateway packages only
#   make package-ce                 Build CE packages for both components
#   make package-ent                Build ENT packages for both components
# ============================================================================

GOLANG_CROSS_IMAGE ?= tykio/golang-cross:1.25-bullseye
GORELEASER_FLAGS ?= --snapshot --skip=sign --skip=publish --clean

# Internal helper: run goreleaser inside the cross-compilation Docker container.
# Usage: $(call run_goreleaser,<goreleaser-config>,<goflags>)
# This matches CI exactly: same image, same env vars, same goreleaser invocation.
# NOTE: does NOT use --clean to allow building both components into the same dist/.
# Use `make clean-dist` to reset before a fresh build.
define run_goreleaser
	docker run --rm --platform linux/amd64 \
		-e CGO_ENABLED=1 \
		-e GOFLAGS='$(2)' \
		-e PACKAGECLOUD_REPO=local/dev \
		-e DEBVERS='unused' \
		-e RPMVERS='unused' \
		-e GOCACHE=/cache/go-build \
		-e GOMODCACHE=/go/pkg/mod \
		-v $(CURDIR):/go/src/github.com/TykTechnologies/midsommar \
		-v $(HOME)/go/pkg/mod:/go/pkg/mod \
		-v $(HOME)/.cache/go-build:/cache/go-build \
		-w /go/src/github.com/TykTechnologies/midsommar \
		$(GOLANG_CROSS_IMAGE) \
		goreleaser release -f $(1) $(GORELEASER_FLAGS)
endef

clean-dist:
	rm -rf dist/

.PHONY: package package-studio package-microgateway package-ce package-ent
.PHONY: package-studio-ce package-studio-ent package-microgateway-ce package-microgateway-ent
.PHONY: package-help test-package-smoke test-package-smoke-ent

# Build all packages for current edition
package: clean-dist package-studio package-microgateway
	@echo "✅ All packages built in dist/"
	@ls -la dist/*.deb dist/*.rpm 2>/dev/null || echo "No packages found"

# Build AI Studio packages (current edition)
package-studio: build-frontend build-docs
	@echo "📦 Building AI Studio $(EDITION) packages..."
	$(call run_goreleaser,ci/goreleaser/goreleaser.yml,$(BUILD_TAGS))
	@echo "✅ AI Studio packages:"
	@ls -la dist/tyk-ai-studio*.deb dist/tyk-ai-studio*.rpm 2>/dev/null

# Build Microgateway packages (current edition)
package-microgateway:
	@echo "📦 Building Microgateway $(EDITION) packages..."
	$(call run_goreleaser,ci/goreleaser/goreleaser-microgateway.yml,$(BUILD_TAGS))
	@echo "✅ Microgateway packages:"
	@ls -la dist/tyk-microgateway*.deb dist/tyk-microgateway*.rpm 2>/dev/null

# CE variants
# GoReleaser uses --clean which wipes dist/ each run. For combined builds,
# we save studio packages to .dist-staging/ (outside dist/) then merge after mgw build.
package-ce: build-frontend build-docs
	@echo "📦 Building all CE packages..."
	$(call run_goreleaser,ci/goreleaser/goreleaser.yml,)
	@mkdir -p .dist-staging && cp dist/*.deb dist/*.rpm .dist-staging/ 2>/dev/null || true
	$(call run_goreleaser,ci/goreleaser/goreleaser-microgateway.yml,)
	@cp .dist-staging/* dist/ 2>/dev/null || true
	@rm -rf .dist-staging
	@echo "✅ CE packages:"
	@ls dist/*.deb dist/*.rpm 2>/dev/null

# ENT variants
package-ent: build-frontend build-docs
	@if [ ! -f enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized. Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "📦 Building all ENT packages..."
	$(call run_goreleaser,ci/goreleaser/goreleaser.yml,-tags=enterprise)
	@mkdir -p .dist-staging && cp dist/*.deb dist/*.rpm .dist-staging/ 2>/dev/null || true
	$(call run_goreleaser,ci/goreleaser/goreleaser-microgateway.yml,-tags=enterprise)
	@cp .dist-staging/* dist/ 2>/dev/null || true
	@rm -rf .dist-staging
	@echo "✅ ENT packages:"
	@ls dist/*.deb dist/*.rpm 2>/dev/null

# Package smoke tests
test-package-smoke: package-ce
	@echo "🧪 Running package smoke tests (CE)..."
	docker compose -f tests/packaging/compose.yml up -d --build --wait
	cd tests/packaging && npx playwright install --with-deps chromium && npx playwright test --reporter=list || \
		(docker compose -f ../../tests/packaging/compose.yml logs && exit 1)
	docker compose -f tests/packaging/compose.yml down -v
	@echo "✅ Package smoke tests passed"

test-package-smoke-ent: package-ent
	@echo "🧪 Running package smoke tests (ENT)..."
	@# Source license from dev/.env.secrets if available
	$(eval export TYK_AI_LICENSE=$(shell grep -s '^TYK_AI_LICENSE=' dev/.env.secrets | cut -d= -f2-))
	@if [ -z "$$TYK_AI_LICENSE" ]; then \
		echo "⚠️  Warning: TYK_AI_LICENSE not set. ENT smoke tests may fail."; \
		echo "   Set it in dev/.env.secrets or export TYK_AI_LICENSE=..."; \
	fi
	docker compose -f tests/packaging/compose.yml up -d --build --wait
	cd tests/packaging && npx playwright install --with-deps chromium && npx playwright test --reporter=list || \
		(docker compose -f ../../tests/packaging/compose.yml logs && exit 1)
	docker compose -f tests/packaging/compose.yml down -v
	@echo "✅ Package smoke tests passed"

# Package help
package-help:
	@echo "Package Building Targets (all run inside Docker for cross-compilation)"
	@echo ""
	@echo "  make package                    Build all packages (auto-detect edition)"
	@echo "  make package-studio             Build AI Studio packages"
	@echo "  make package-microgateway       Build Microgateway packages"
	@echo "  make package-ce                 Build all CE packages"
	@echo "  make package-ent                Build all ENT packages"
	@echo "  make package-studio-ce          Build AI Studio CE packages"
	@echo "  make package-studio-ent         Build AI Studio ENT packages"
	@echo "  make package-microgateway-ce    Build Microgateway CE packages"
	@echo "  make package-microgateway-ent   Build Microgateway ENT packages"
	@echo "  make test-package-smoke         Build CE packages and run smoke tests"
	@echo "  make test-package-smoke-ent     Build ENT packages and run smoke tests"
	@echo ""
	@echo "Packages output to dist/ directory"
	@echo "Override Docker image: GOLANG_CROSS_IMAGE=... make package"

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

# ============================================================================
# Plugin Scaffolding
# ============================================================================
# Usage:
#   make plugin-new NAME=my-plugin TYPE=studio
#   make plugin-new NAME=my-plugin TYPE=studio CAPABILITIES=studio_ui,post_auth,on_response
#   make plugin-new NAME=my-enricher TYPE=gateway
#   make plugin-new NAME=my-assistant TYPE=agent
#   make plugin-help

.PHONY: plugin-new plugin-help tools

# Build all development tools
tools: bin/plugin-scaffold
	@echo "✅ Development tools built successfully"
	@echo "   - bin/plugin-scaffold (plugin scaffolding)"

# Build the scaffolding tool (rebuilds only if source changes)
bin/plugin-scaffold: $(wildcard cmd/plugin-scaffold/*.go)
	@echo "Building plugin-scaffold tool..."
	@mkdir -p bin
	@go build -o bin/plugin-scaffold ./cmd/plugin-scaffold

# Scaffold a new plugin
plugin-new: bin/plugin-scaffold
	@if [ -z "$(NAME)" ] || [ -z "$(TYPE)" ]; then \
		echo "Usage: make plugin-new NAME=my-plugin TYPE=studio|gateway|agent|data-collector [CAPABILITIES=cap1,cap2]"; \
		echo ""; \
		echo "Run 'make plugin-help' for more information."; \
		exit 1; \
	fi
	@./bin/plugin-scaffold -name="$(NAME)" -type="$(TYPE)" -capabilities="$(CAPABILITIES)"

# Show plugin scaffolding help
plugin-help: bin/plugin-scaffold
	@./bin/plugin-scaffold -help

# Test target (legacy - use test-all for more control)
test:
	go test $(BUILD_TAGS) ./...
	cd microgateway && go test $(BUILD_TAGS) ./...

# ============================================================================
# Unified Testing System
# ============================================================================
# Usage:
#   make test-all                           # Run unit + integration tests (auto-detect edition)
#   make test-all EDITION=ent               # Run with enterprise edition
#   make test-quick                         # Unit tests only (fast feedback)
#   make test-ci                            # CI tests with coverage
#   make test-all TEST_COMPONENTS="studio"  # Test only studio
#   make test-all TEST_TYPES="unit"         # Unit tests only
#
# Components: studio, microgateway, frontend, plugins
# Types: unit, integration, e2e
# Editions: ce, ent

# Test configuration variables (can be overridden)
TEST_COMPONENTS ?= studio microgateway frontend plugins
TEST_TYPES ?= unit integration
TEST_EDITION ?= $(EDITION)
TEST_VERBOSE ?= false
TEST_COVERAGE ?= false
TEST_TIMEOUT ?= 30m

# Determine build tags based on edition
ifeq ($(TEST_EDITION),ent)
    TEST_BUILD_TAGS := -tags enterprise
else
    TEST_BUILD_TAGS :=
endif

# Verbose flag
ifeq ($(TEST_VERBOSE),true)
    TEST_VERBOSE_FLAG := -v
else
    TEST_VERBOSE_FLAG :=
endif

# Coverage flag
ifeq ($(TEST_COVERAGE),true)
    TEST_COVERAGE_FLAG := -coverprofile=coverage.out -covermode=atomic
else
    TEST_COVERAGE_FLAG :=
endif

# Primary unified test target
.PHONY: test-all
test-all: ## Run all tests with configurable options
	@echo "=========================================="
	@echo "Tyk AI Studio Unified Test Suite"
	@echo "=========================================="
	@echo "Edition:    $(TEST_EDITION)"
	@echo "Components: $(TEST_COMPONENTS)"
	@echo "Types:      $(TEST_TYPES)"
	@echo "Timeout:    $(TEST_TIMEOUT)"
	@echo "=========================================="
	@failed=0; \
	for component in $(TEST_COMPONENTS); do \
		for type in $(TEST_TYPES); do \
			target="test-$${component}-$${type}"; \
			if $(MAKE) -n $$target > /dev/null 2>&1; then \
				echo ""; \
				echo ">>> Running: $$target"; \
				if ! $(MAKE) $$target TEST_EDITION=$(TEST_EDITION) TEST_VERBOSE=$(TEST_VERBOSE) TEST_COVERAGE=$(TEST_COVERAGE) TEST_TIMEOUT=$(TEST_TIMEOUT); then \
					echo "FAILED: $$target"; \
					failed=1; \
				fi; \
			fi; \
		done; \
	done; \
	if [ $$failed -eq 1 ]; then \
		echo ""; \
		echo "=========================================="; \
		echo "SOME TESTS FAILED"; \
		echo "=========================================="; \
		exit 1; \
	fi; \
	echo ""; \
	echo "=========================================="; \
	echo "ALL TESTS PASSED"; \
	echo "=========================================="

.PHONY: test-quick
test-quick: ## Run only unit tests (fast feedback)
	$(MAKE) test-all TEST_TYPES="unit" TEST_COMPONENTS="studio microgateway frontend"

.PHONY: test-ci
test-ci: ## Run CI-appropriate tests (unit + integration, with coverage)
	$(MAKE) test-all TEST_TYPES="unit integration" TEST_COVERAGE=true TEST_VERBOSE=true

# ============================================================================
# AI Studio Tests
# ============================================================================

.PHONY: test-studio-unit
test-studio-unit: ## Run AI Studio unit tests
	@echo "Running AI Studio unit tests ($(TEST_EDITION))..."
	go test $(TEST_BUILD_TAGS) $(TEST_VERBOSE_FLAG) $(TEST_COVERAGE_FLAG) \
		-timeout $(TEST_TIMEOUT) -race -short ./...

.PHONY: test-studio-integration
test-studio-integration: ## Run AI Studio integration tests
	@echo "Running AI Studio integration tests ($(TEST_EDITION))..."
	go test $(TEST_BUILD_TAGS) $(TEST_VERBOSE_FLAG) \
		-timeout $(TEST_TIMEOUT) -count=1 -run "Integration|TestOAuth" ./tests/... || true

# ============================================================================
# Microgateway Tests
# ============================================================================

.PHONY: test-microgateway-unit
test-microgateway-unit: ## Run Microgateway unit tests
	@echo "Running Microgateway unit tests ($(TEST_EDITION))..."
	cd microgateway && go test $(TEST_BUILD_TAGS) $(TEST_VERBOSE_FLAG) \
		-timeout $(TEST_TIMEOUT) -race -short ./...

.PHONY: test-microgateway-integration
test-microgateway-integration: ## Run Microgateway integration tests
	@echo "Running Microgateway integration tests ($(TEST_EDITION))..."
	cd microgateway && go test $(TEST_BUILD_TAGS) $(TEST_VERBOSE_FLAG) \
		-timeout $(TEST_TIMEOUT) -count=1 -run Integration ./tests/integration/... || true

# ============================================================================
# Frontend Tests
# ============================================================================

.PHONY: test-frontend-unit
test-frontend-unit: ## Run frontend Jest tests
	@echo "Running frontend unit tests..."
	cd ui/admin-frontend && npm test

# ============================================================================
# Plugin Tests
# ============================================================================

.PHONY: test-plugins-unit
test-plugins-unit: ## Run plugin unit tests
	@echo "Running plugin unit tests..."
	@# Community plugins
	@if [ -d "community/plugins/llm-cache" ]; then \
		echo ">>> community/plugins/llm-cache"; \
		cd community/plugins/llm-cache && go test $(TEST_VERBOSE_FLAG) ./... || true; \
	fi

.PHONY: test-plugins-integration
test-plugins-integration: ## Run plugin integration tests (requires Docker)
	@if [ "$(TEST_EDITION)" != "ent" ]; then \
		echo "Skipping enterprise plugin integration tests (set EDITION=ent to enable)"; \
		exit 0; \
	fi
	@echo "Running enterprise plugin integration tests..."
	@echo ">>> advanced-llm-cache integration tests"
	cd enterprise/plugins/advanced-llm-cache && \
		go test -v -count=1 -tags="integration,enterprise" \
		-timeout $(TEST_TIMEOUT) ./tests/integration/...
	@echo ">>> llm-load-balancer integration tests"
	cd enterprise/plugins/llm-load-balancer && \
		go test -v -count=1 -tags="integration,enterprise" \
		-timeout $(TEST_TIMEOUT) ./tests/integration/...
	@echo ">>> github-rag-ingest Vault integration tests"
	@if [ -d "community/plugins/github-rag-ingest/server" ]; then \
		cd community/plugins/github-rag-ingest/server && \
		go test -v -count=1 -tags=integration -timeout $(TEST_TIMEOUT) ./secrets/... || true; \
	fi

.PHONY: test-plugins-e2e
test-plugins-e2e: ## Run plugin E2E tests (requires Docker)
	@if [ "$(TEST_EDITION)" != "ent" ]; then \
		echo "Skipping enterprise plugin e2e tests (set EDITION=ent to enable)"; \
		exit 0; \
	fi
	@echo "Running enterprise plugin E2E tests..."
	cd enterprise/plugins/llm-load-balancer && \
		go test -v -count=1 -tags="e2e,enterprise" \
		-timeout $(TEST_TIMEOUT) ./tests/e2e/...

# ============================================================================
# UI E2E Tests (Playwright)
# ============================================================================

.PHONY: test-ui-e2e
test-ui-e2e: ## Run UI E2E tests (requires Docker Compose environment)
	@echo "Running UI E2E tests..."
	@echo "Note: Requires Docker Compose test environment to be running"
	@echo "Start with: make start-test-env"
	@# Uses --workers=1 because tests share database state and will fail with race conditions if run in parallel
	cd tests/ui && npm ci && npx playwright install --with-deps chromium && npm run test:serial

.PHONY: test-ui-e2e-with-env
test-ui-e2e-with-env: ## Start test env and run UI E2E tests
	@echo "Starting test environment..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@mkdir -p ./tests/postgres_data_temp
	@cp -r tests/postgres_data ./tests/postgres_data_temp 2>/dev/null || true
	docker compose --env-file .env -f tests/compose.yml up -d
	@echo "Waiting for test environment to be ready..."
	@attempts=0; \
	max_attempts=60; \
	while [ "$$(curl -s -o /dev/null -w '%{http_code}' http://localhost:3000/common/api/v1/notifications/unread/count 2>/dev/null)" != "401" ]; do \
		attempts=$$((attempts+1)); \
		echo "Waiting for AI Studio... ($$attempts/$$max_attempts)"; \
		if [ $$attempts -ge $$max_attempts ]; then \
			echo "Timed out waiting for AI Studio"; \
			docker compose --env-file .env -f tests/compose.yml down; \
			exit 1; \
		fi; \
		sleep 5; \
	done
	@echo "AI Studio is ready, running tests..."
	$(MAKE) test-ui-e2e || (docker compose --env-file .env -f tests/compose.yml down && exit 1)
	docker compose --env-file .env -f tests/compose.yml down

# ============================================================================
# Test Help and Discovery
# ============================================================================

.PHONY: test-help
test-help: ## Show testing system help
	@echo "Tyk AI Studio Unified Testing System"
	@echo ""
	@echo "Primary Targets:"
	@echo "  make test-all              Run all tests (default: unit + integration)"
	@echo "  make test-quick            Run only unit tests (fast feedback)"
	@echo "  make test-ci               Run CI tests with coverage"
	@echo ""
	@echo "Component Targets:"
	@echo "  make test-studio-unit            AI Studio Go unit tests"
	@echo "  make test-studio-integration     AI Studio integration tests"
	@echo "  make test-microgateway-unit      Microgateway unit tests"
	@echo "  make test-microgateway-integration  Microgateway integration tests"
	@echo "  make test-frontend-unit          Frontend Jest tests"
	@echo "  make test-plugins-unit           Plugin unit tests"
	@echo "  make test-plugins-integration    Plugin integration tests (Docker required)"
	@echo "  make test-plugins-e2e            Plugin E2E tests (Docker required)"
	@echo "  make test-ui-e2e                 UI Playwright E2E tests"
	@echo "  make test-ui-e2e-with-env        UI E2E with auto-started environment"
	@echo ""
	@echo "Configuration Variables:"
	@echo "  TEST_EDITION=ce|ent        Edition to test (default: auto-detect)"
	@echo "  TEST_COMPONENTS=\"...\"      Components to test (space-separated)"
	@echo "  TEST_TYPES=\"...\"           Test types to run (space-separated)"
	@echo "  TEST_VERBOSE=true          Enable verbose output"
	@echo "  TEST_COVERAGE=true         Generate coverage report"
	@echo "  TEST_TIMEOUT=30m           Test timeout duration"
	@echo ""
	@echo "Components: studio, microgateway, frontend, plugins"
	@echo "Types: unit, integration, e2e"
	@echo ""
	@echo "Examples:"
	@echo "  make test-all EDITION=ent"
	@echo "  make test-all TEST_COMPONENTS=\"studio frontend\" TEST_TYPES=\"unit\""
	@echo "  make test-all TEST_VERBOSE=true TEST_COVERAGE=true"

.PHONY: test-list
test-list: ## List all available test targets
	@echo "Available test targets:"
	@grep -E '^test-[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-35s %s\n", $$1, $$2}'

# ============================================================================
# Integration Tests (Enterprise) - Legacy targets preserved
# ============================================================================
# Note: The advanced-llm-cache plugin has its own go.mod, so tests run from
# within the plugin directory

# Run all enterprise integration tests (requires Docker)
# Note: -count=1 disables test caching so env vars are respected on each run
test-integration:
	@echo "Running all enterprise integration tests..."
	cd enterprise/plugins/advanced-llm-cache && go test -v -count=1 -tags="integration,enterprise" ./tests/integration/...

# Run integration tests for advanced-llm-cache plugin
test-integration-plugin-cache:
	@echo "Running advanced-llm-cache integration tests..."
	cd enterprise/plugins/advanced-llm-cache && go test -v -count=1 -tags="integration,enterprise" ./tests/integration/...

# Run full integration tests with all optional features enabled
test-integration-plugin-cache-full:
	@echo "Running full advanced-llm-cache integration tests (all features)..."
	cd enterprise/plugins/advanced-llm-cache && INTEGRATION_REDIS_CLUSTER=1 INTEGRATION_SYSLOG=1 \
		go test -v -count=1 -tags="integration,enterprise" ./tests/integration/...

# Run integration tests with coverage
test-integration-plugin-cache-coverage:
	@echo "Running advanced-llm-cache integration tests with coverage..."
	cd enterprise/plugins/advanced-llm-cache && go test -v -count=1 -tags="integration,enterprise" -coverprofile=integration-coverage.out \
		./tests/integration/...
	cd enterprise/plugins/advanced-llm-cache && go tool cover -func=integration-coverage.out | tail -1

# Run integration tests with Redis cluster (slower startup)
test-integration-plugin-cache-cluster:
	@echo "Running Redis cluster integration tests..."
	cd enterprise/plugins/advanced-llm-cache && INTEGRATION_REDIS_CLUSTER=1 \
		go test -v -count=1 -tags="integration,enterprise" -run ".*Cluster.*" \
		./tests/integration/...

# Run integration tests with Syslog
test-integration-plugin-cache-syslog:
	@echo "Running Syslog audit integration tests..."
	cd enterprise/plugins/advanced-llm-cache && INTEGRATION_SYSLOG=1 \
		go test -v -count=1 -tags="integration,enterprise" -run ".*Syslog.*" \
		./tests/integration/...

# Run Vault integration tests for github-rag-ingest plugin
test-integration-plugin-github-rag:
	@echo "Running github-rag-ingest Vault integration tests..."
	cd community/plugins/github-rag-ingest/server && \
		go test -v -count=1 -tags=integration ./secrets/...

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

# Development targets (LEGACY - consider using 'make dev' instead)
# The new Docker Compose-based environment provides hot reloading and multi-component support.
# See 'make dev-help' for more information.
start-dev: stop-dev
	@echo "⚠️  Note: 'make start-dev' is the legacy approach."
	@echo "   Consider using 'make dev' for hot reloading and better DX."
	@echo ""
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

# ============================================================================
# Docker Compose Development Environment (NEW - Recommended)
# ============================================================================
# Modern Docker Compose-based development with hot reloading.
# See dev/README.md for full documentation.
#
# Quick Start:
#   make dev          - Start minimal dev env (Studio + Frontend + Postgres)
#   make dev-full     - Start full stack (+ Gateway + Plugins watcher)
#   make dev-ent      - Start enterprise dev env
#   make dev-full-ent - Start full enterprise stack
#
# All services have hot reloading enabled via Air (Go) and React HMR (frontend).

.PHONY: dev dev-full dev-ent dev-full-ent dev-down dev-logs dev-clean dev-status dev-rebuild

# Start minimal development environment (Studio + Frontend + PostgreSQL)
dev:
	@echo "🚀 Starting minimal development environment..."
	@echo "   Services: postgres, studio (with hot reload), frontend (with HMR)"
	@echo ""
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
		echo "   Edit dev/.env to add your API keys"; \
		echo ""; \
	fi
	cd dev && docker compose up --build

# Start full development environment (+ Gateway + Plugin watcher)
dev-full:
	@echo "🚀 Starting full development environment..."
	@echo "   Services: postgres, studio, frontend, gateway, plugins"
	@echo ""
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
		echo "   Edit dev/.env to add your API keys"; \
		echo ""; \
	fi
	@if [ ! -f dev/.env.gateway ]; then \
		cp dev/.env.gateway.dev dev/.env.gateway; \
		echo "📝 Created dev/.env.gateway from template"; \
	fi
	cd dev && docker compose -f docker-compose.yml -f docker-compose.full.yml up --build

# Start enterprise development environment (minimal)
dev-ent:
	@if [ ! -d "enterprise/.git" ] && [ ! -f "enterprise/.git" ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo "   Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "🏢 Starting enterprise development environment..."
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
	fi
	@if [ -f dev/.env.secrets ]; then \
		echo "🔐 Merging dev/.env.secrets into dev/.env"; \
		while IFS= read -r line || [ -n "$$line" ]; do \
			key=$$(echo "$$line" | cut -d= -f1); \
			if [ -n "$$key" ] && [ "$${key#\#}" = "$$key" ]; then \
				sed -i.bak "/^$$key=/d" dev/.env 2>/dev/null || sed -i '' "/^$$key=/d" dev/.env; \
				echo "$$line" >> dev/.env; \
			fi; \
		done < dev/.env.secrets; \
		rm -f dev/.env.bak; \
	fi
	@if ! grep -q "^TYK_AI_LICENSE=" dev/.env || [ "$$(grep "^TYK_AI_LICENSE=" dev/.env | cut -d= -f2)" = "" ]; then \
		echo "⚠️  Warning: TYK_AI_LICENSE not set"; \
		echo "   Create dev/.env.secrets with:"; \
		echo "     TYK_AI_LICENSE=your-license-key"; \
		echo ""; \
	fi
	cd dev && docker compose -f docker-compose.yml -f docker-compose.ent.yml up --build

# Start full enterprise development environment
dev-full-ent:
	@if [ ! -d "enterprise/.git" ] && [ ! -f "enterprise/.git" ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo "   Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "🏢 Starting full enterprise development environment..."
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
	fi
	@if [ -f dev/.env.secrets ]; then \
		echo "🔐 Merging dev/.env.secrets into dev/.env"; \
		while IFS= read -r line || [ -n "$$line" ]; do \
			key=$$(echo "$$line" | cut -d= -f1); \
			if [ -n "$$key" ] && [ "$${key#\#}" = "$$key" ]; then \
				sed -i.bak "/^$$key=/d" dev/.env 2>/dev/null || sed -i '' "/^$$key=/d" dev/.env; \
				echo "$$line" >> dev/.env; \
			fi; \
		done < dev/.env.secrets; \
		rm -f dev/.env.bak; \
	fi
	@if [ ! -f dev/.env.gateway ]; then \
		cp dev/.env.gateway.dev dev/.env.gateway; \
		echo "📝 Created dev/.env.gateway from template"; \
	fi
	cd dev && docker compose -f docker-compose.yml -f docker-compose.full.yml -f docker-compose.ent.yml -f docker-compose.full-ent.yml up --build

# Stop development environment (handles all compose file combinations)
dev-down:
	@echo "🛑 Stopping development environment..."
	@# Stop containers by name to avoid env file dependency issues
	docker stop midsommar-studio midsommar-frontend midsommar-postgres midsommar-gateway midsommar-plugins 2>/dev/null || true
	docker rm midsommar-studio midsommar-frontend midsommar-postgres midsommar-gateway midsommar-plugins 2>/dev/null || true
	@# Also try compose down with just the base file as fallback
	cd dev && docker compose down 2>/dev/null || true

# View all logs
dev-logs:
	cd dev && docker compose logs -f

# View logs for specific service (usage: make dev-logs-studio, make dev-logs-gateway, etc.)
dev-logs-%:
	cd dev && docker compose logs -f $*

# Shell into a service (usage: make dev-shell-studio, make dev-shell-gateway, etc.)
dev-shell-%:
	cd dev && docker compose exec $* sh

# Rebuild a specific service (usage: make dev-rebuild-studio)
dev-rebuild-%:
	cd dev && docker compose up --build -d $*

# Clean development environment (stops containers and removes volumes including postgres data)
dev-clean: dev-down
	@echo "🧹 Cleaning development environment..."
	@echo "   Removing Docker volumes..."
	docker volume rm midsommar-postgres-data 2>/dev/null || true
	docker volume rm midsommar-studio-tmp 2>/dev/null || true
	docker volume rm midsommar-studio-data 2>/dev/null || true
	docker volume rm midsommar-go-cache 2>/dev/null || true
	rm -f dev/.env dev/.env.gateway
	@echo "✅ Development environment cleaned (including postgres data)"
	@echo "   Run 'make dev' to start fresh"

# Show development environment status
dev-status:
	@echo "Development Environment Status:"
	@echo ""
	cd dev && docker compose ps 2>/dev/null || echo "No containers running"

# Development help
dev-help:
	@echo "Docker Compose Development Environment"
	@echo ""
	@echo "Quick Start:"
	@echo "  make dev              Start minimal env (Studio + Frontend + Postgres)"
	@echo "  make dev-full         Start full stack (+ Gateway + Plugins watcher)"
	@echo ""
	@echo "Enterprise Edition:"
	@echo "  make dev-ent          Start enterprise minimal env"
	@echo "  make dev-full-ent     Start enterprise full stack"
	@echo ""
	@echo "Detached Mode (for automation/Claude):"
	@echo "  make dev-start        Start minimal env (detached)"
	@echo "  make dev-start-full   Start full stack (detached)"
	@echo "  make dev-start-ent    Start enterprise minimal (detached)"
	@echo "  make dev-start-full-ent Start enterprise full (detached)"
	@echo ""
	@echo "Management:"
	@echo "  make dev-down         Stop all containers"
	@echo "  make dev-logs         View all logs"
	@echo "  make dev-logs-studio  View studio logs only"
	@echo "  make dev-logs-gateway View gateway logs only"
	@echo "  make dev-shell-studio Shell into studio container"
	@echo "  make dev-status       Show container status"
	@echo "  make dev-clean        Stop and remove all data"
	@echo ""
	@echo "Non-blocking Logs (for automation/Claude):"
	@echo "  make dev-tail-studio  Last 100 lines of studio logs"
	@echo "  make dev-tail-gateway Last 100 lines of gateway logs"
	@echo "  make dev-tail LINES=50 Custom line count"
	@echo ""
	@echo "Ports:"
	@echo "  3000  Frontend (React dev server)"
	@echo "  8080  Studio REST API"
	@echo "  50051 Studio gRPC (control server)"
	@echo "  8081  Gateway REST API"
	@echo "  5432  PostgreSQL"
	@echo ""
	@echo "Hot Reloading:"
	@echo "  - Edit Go files → Air rebuilds in ~2-3 seconds"
	@echo "  - Edit React files → HMR updates instantly"
	@echo "  - Edit plugins → Auto-rebuilt by watcher (full mode)"
	@echo ""
	@echo "See dev/README.md for full documentation"

# ============================================================================
# Detached Mode Development (for automation/Claude Code)
# ============================================================================
# These targets run the dev environment in detached mode (-d flag) so that
# the command returns immediately. Useful for automation and Claude Code skills.

# Start minimal dev env in detached mode
dev-start:
	@echo "🚀 Starting minimal development environment (detached)..."
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
	fi
	cd dev && docker compose up --build -d
	@echo "✅ Environment started. Use 'make dev-status' to check."

# Start full dev env in detached mode
dev-start-full:
	@echo "🚀 Starting full development environment (detached)..."
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
	fi
	@if [ ! -f dev/.env.gateway ]; then \
		cp dev/.env.gateway.dev dev/.env.gateway; \
		echo "📝 Created dev/.env.gateway from template"; \
	fi
	cd dev && docker compose -f docker-compose.yml -f docker-compose.full.yml up --build -d
	@echo "✅ Environment started. Use 'make dev-status' to check."

# Start enterprise dev env in detached mode
dev-start-ent:
	@if [ ! -d "enterprise/.git" ] && [ ! -f "enterprise/.git" ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized. Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "🏢 Starting enterprise development environment (detached)..."
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
	fi
	@if [ -f dev/.env.secrets ]; then \
		echo "🔐 Merging dev/.env.secrets into dev/.env"; \
		while IFS= read -r line || [ -n "$$line" ]; do \
			key=$$(echo "$$line" | cut -d= -f1); \
			if [ -n "$$key" ] && [ "$${key#\#}" = "$$key" ]; then \
				sed -i.bak "/^$$key=/d" dev/.env 2>/dev/null || sed -i '' "/^$$key=/d" dev/.env; \
				echo "$$line" >> dev/.env; \
			fi; \
		done < dev/.env.secrets; \
		rm -f dev/.env.bak; \
	fi
	cd dev && docker compose -f docker-compose.yml -f docker-compose.ent.yml up --build -d
	@echo "✅ Environment started. Use 'make dev-status' to check."

# Start full enterprise dev env in detached mode
dev-start-full-ent:
	@if [ ! -d "enterprise/.git" ] && [ ! -f "enterprise/.git" ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized. Run: make init-enterprise"; \
		exit 1; \
	fi
	@echo "🏢 Starting full enterprise development environment (detached)..."
	@if [ ! -f dev/.env ]; then \
		cp dev/.env.dev dev/.env; \
		echo "📝 Created dev/.env from template"; \
	fi
	@if [ -f dev/.env.secrets ]; then \
		echo "🔐 Merging dev/.env.secrets into dev/.env"; \
		while IFS= read -r line || [ -n "$$line" ]; do \
			key=$$(echo "$$line" | cut -d= -f1); \
			if [ -n "$$key" ] && [ "$${key#\#}" = "$$key" ]; then \
				sed -i.bak "/^$$key=/d" dev/.env 2>/dev/null || sed -i '' "/^$$key=/d" dev/.env; \
				echo "$$line" >> dev/.env; \
			fi; \
		done < dev/.env.secrets; \
		rm -f dev/.env.bak; \
	fi
	@if [ ! -f dev/.env.gateway ]; then \
		cp dev/.env.gateway.dev dev/.env.gateway; \
		echo "📝 Created dev/.env.gateway from template"; \
	fi
	cd dev && docker compose -f docker-compose.yml -f docker-compose.full.yml -f docker-compose.ent.yml -f docker-compose.full-ent.yml up --build -d
	@echo "✅ Environment started. Use 'make dev-status' to check."

# ============================================================================
# Non-blocking Log Commands (for automation/Claude Code)
# ============================================================================
# These targets fetch logs without following (-f), returning immediately.
# Use LINES=N to control how many lines to fetch (default: 100).

# View last N lines of all logs (default 100, non-blocking)
dev-tail:
	cd dev && docker compose logs --tail=$(or $(LINES),100)

# View last N lines of specific service logs (usage: make dev-tail-studio LINES=50)
dev-tail-%:
	cd dev && docker compose logs --tail=$(or $(LINES),100) $*

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
	dev dev-full dev-ent dev-full-ent dev-down dev-logs dev-clean dev-status dev-help \
	dev-start dev-start-full dev-start-ent dev-start-full-ent dev-tail \
	perf-test perf-profile perf-baseline perf-compare perf-report perf-clean \
	init-enterprise update-enterprise show-edition \
	test-integration test-integration-plugin-cache test-integration-plugin-cache-full \
	test-integration-plugin-cache-coverage test-integration-plugin-cache-cluster \
	test-integration-plugin-cache-syslog test-integration-plugin-github-rag \
	test-all test-quick test-ci test-help test-list \
	test-studio-unit test-studio-integration \
	test-microgateway-unit test-microgateway-integration \
	test-frontend-unit \
	test-plugins-unit test-plugins-integration test-plugins-e2e \
	test-ui-e2e test-ui-e2e-with-env \
	plugin-new plugin-help tools
