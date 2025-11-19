# microgateway/Makefile

# Variables
BINARY_NAME=microgateway
CLI_BINARY_NAME=mgw
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}"

# Detect if enterprise submodule exists (check parent directory)
ENTERPRISE_EXISTS := $(shell test -f ../enterprise/.git && echo "yes" || echo "no")

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
PROD_LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" -trimpath

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Directories
CMD_DIR=./cmd/microgateway
CLI_CMD_DIR=./cmd/mgw
DIST_DIR=./dist

# Default goal
.DEFAULT_GOAL := help

all: test build ## Run tests and build

# ============================================================================
# Help Target
# ============================================================================

help: ## Display help information
	@echo "Microgateway Build Targets"
	@echo ""
	@echo "Current edition: $(EDITION)"
	@echo ""
	@echo "Development (single platform, fast, CGO enabled):"
	@echo "  make build              - Build for current platform with CGO (auto-detect edition)"
	@echo "  make build-community    - Build CE for current platform with CGO"
	@echo "  make build-enterprise   - Build ENT for current platform with CGO"
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
	@echo "  make test               - Run tests"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make show-edition       - Show current edition info"
	@echo ""

# ============================================================================
# Development Builds (single platform, fast, CGO enabled)
# ============================================================================

build: ## Build the microgateway binary (auto-detects edition, CGO enabled)
	@mkdir -p $(DIST_DIR)
	@echo "🔨 Building microgateway $(EDITION) for native platform with CGO..."
	CGO_ENABLED=1 $(GOBUILD) $(BUILD_TAGS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-$(EDITION) $(CMD_DIR)
	@echo "✅ Built: $(DIST_DIR)/$(BINARY_NAME)-$(EDITION) (CGO enabled)"

build-community: ## Force build Community Edition (native, CGO enabled)
	@mkdir -p $(DIST_DIR)
	@echo "🔨 Building microgateway CE for native platform with CGO..."
	CGO_ENABLED=1 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-ce $(CMD_DIR)
	@echo "✅ Built: $(DIST_DIR)/$(BINARY_NAME)-ce (CGO enabled)"

build-enterprise: ## Force build Enterprise Edition (native, CGO enabled)
	@if [ ! -f ../enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo "Run: cd .. && make init-enterprise"; \
		exit 1; \
	fi
	@mkdir -p $(DIST_DIR)
	@echo "🔨 Building microgateway ENT for native platform with CGO..."
	CGO_ENABLED=1 $(GOBUILD) -tags enterprise $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-ent $(CMD_DIR)
	@echo "✅ Built: $(DIST_DIR)/$(BINARY_NAME)-ent (CGO enabled)"

build-cli: ## Build the CLI binary (auto-detects edition, CGO enabled)
	@mkdir -p $(DIST_DIR)
	@echo "Building mgw CLI $(EDITION)..."
	CGO_ENABLED=1 $(GOBUILD) $(BUILD_TAGS) $(LDFLAGS) -o $(DIST_DIR)/$(CLI_BINARY_NAME)-$(EDITION) $(CLI_CMD_DIR)
	@echo "✅ Built: $(DIST_DIR)/$(CLI_BINARY_NAME)-$(EDITION)"

build-both: build build-cli ## Build both server and CLI binaries

# ============================================================================
# Production Builds (multi-platform, optimized, CGO enabled)
# ============================================================================

build-prod: ## Build production artifacts for all platforms (auto-detect edition)
	@echo "🏗️  Building microgateway $(EDITION) production artifacts..."
	@mkdir -p $(DIST_DIR)
	$(MAKE) build-linux-amd64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	$(MAKE) build-linux-arm64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	$(MAKE) build-darwin-amd64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	$(MAKE) build-darwin-arm64 EDITION=$(EDITION) BUILD_TAGS="$(BUILD_TAGS)"
	@echo "✅ Production build complete"

build-prod-ce: ## Build production CE for all platforms
	@echo "🏗️  Building microgateway CE production artifacts..."
	@mkdir -p $(DIST_DIR)
	$(MAKE) build-linux-amd64 EDITION=ce BUILD_TAGS=""
	$(MAKE) build-linux-arm64 EDITION=ce BUILD_TAGS=""
	$(MAKE) build-darwin-amd64 EDITION=ce BUILD_TAGS=""
	$(MAKE) build-darwin-arm64 EDITION=ce BUILD_TAGS=""
	@echo "✅ Production CE build complete"

build-prod-ent: ## Build production ENT for all platforms
	@if [ ! -f ../enterprise/.git ]; then \
		echo "❌ ERROR: Enterprise submodule not initialized."; \
		echo "Run: cd .. && make init-enterprise"; \
		exit 1; \
	fi
	@echo "🏗️  Building microgateway ENT production artifacts..."
	@mkdir -p $(DIST_DIR)
	$(MAKE) build-linux-amd64 EDITION=ent BUILD_TAGS="-tags enterprise"
	$(MAKE) build-linux-arm64 EDITION=ent BUILD_TAGS="-tags enterprise"
	$(MAKE) build-darwin-amd64 EDITION=ent BUILD_TAGS="-tags enterprise"
	$(MAKE) build-darwin-arm64 EDITION=ent BUILD_TAGS="-tags enterprise"
	@echo "✅ Production ENT build complete"

# ============================================================================
# Platform-Specific Builds (CGO enabled)
# ============================================================================

build-linux-amd64: ## Linux AMD64 with CGO
	@echo "Building microgateway $(EDITION) for linux/amd64 with CGO..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	$(GOBUILD) $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o $(DIST_DIR)/$(BINARY_NAME)-$(EDITION)-linux-amd64 $(CMD_DIR)

build-linux-arm64: ## Linux ARM64 with CGO (requires cross-compiler)
	@echo "Building microgateway $(EDITION) for linux/arm64 with CGO..."
	@if ! which aarch64-linux-gnu-gcc > /dev/null 2>&1; then \
		echo "⚠️  Warning: aarch64-linux-gnu-gcc not found"; \
		echo "   Install: sudo apt-get install gcc-aarch64-linux-gnu (Ubuntu/Debian)"; \
		echo "   Or: brew install FiloSottile/musl-cross/musl-cross (macOS)"; \
		echo "   Attempting build anyway..."; \
	fi
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
	CC=aarch64-linux-gnu-gcc \
	$(GOBUILD) $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o $(DIST_DIR)/$(BINARY_NAME)-$(EDITION)-linux-arm64 $(CMD_DIR)

build-darwin-amd64: ## Darwin AMD64 with CGO
	@echo "Building microgateway $(EDITION) for darwin/amd64 with CGO..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
	$(GOBUILD) $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o $(DIST_DIR)/$(BINARY_NAME)-$(EDITION)-darwin-amd64 $(CMD_DIR)

build-darwin-arm64: ## Darwin ARM64 with CGO
	@echo "Building microgateway $(EDITION) for darwin/arm64 with CGO..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
	$(GOBUILD) $(BUILD_TAGS) $(PROD_LDFLAGS) \
	-o $(DIST_DIR)/$(BINARY_NAME)-$(EDITION)-darwin-arm64 $(CMD_DIR)

# ============================================================================
# Legacy Targets (for backward compatibility)
# ============================================================================

build-all: build-prod ## Alias for build-prod (backward compatible)
	@echo "Note: build-all is now an alias for build-prod"

build-cli-all: ## Build CLI for all platforms
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(CLI_BINARY_NAME)-linux-amd64 $(CLI_CMD_DIR)
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(CLI_BINARY_NAME)-darwin-amd64 $(CLI_CMD_DIR)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(CLI_BINARY_NAME)-darwin-arm64 $(CLI_CMD_DIR)
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(CLI_BINARY_NAME)-windows-amd64.exe $(CLI_CMD_DIR)

show-edition: ## Show current edition
	@echo "Current edition: $(EDITION)"
	@if [ "$(ENTERPRISE_EXISTS)" = "yes" ]; then \
		echo "Enterprise submodule: initialized"; \
		cd ../enterprise && git log -1 --oneline; \
	else \
		echo "Enterprise submodule: not initialized"; \
	fi

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(DIST_DIR)

test: ## Run tests (auto-detects edition)
	$(GOTEST) $(BUILD_TAGS) -v -race -coverprofile=coverage.out ./...

test-unit: ## Run unit tests
	$(GOTEST) -v -short ./...

test-integration: ## Run integration tests
	$(GOTEST) -v -run Integration ./tests/integration

test-e2e: ## Run end-to-end tests
	$(GOTEST) -v -run E2E ./tests/e2e

coverage: test ## Generate coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

deps: ## Download dependencies
	$(GOMOD) download

tidy: ## Tidy dependencies
	$(GOMOD) tidy

docker-build: ## Build Docker image
	docker build -f deployments/Dockerfile -t microgateway:$(VERSION) .

docker-push: ## Push Docker image
	docker tag microgateway:$(VERSION) your-registry/microgateway:$(VERSION)
	docker push your-registry/microgateway:$(VERSION)

docker-compose-up: ## Start with Docker Compose
	docker-compose -f deployments/docker-compose.yml up -d

docker-compose-down: ## Stop Docker Compose
	docker-compose -f deployments/docker-compose.yml down

migrate: build ## Run database migrations
	./$(DIST_DIR)/$(BINARY_NAME) -migrate

run: build ## Build and run
	./$(DIST_DIR)/$(BINARY_NAME)

dev: ## Run with hot reload (requires air)
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air

fmt: ## Format code
	$(GOCMD) fmt ./...

vet: ## Vet code
	$(GOCMD) vet ./...

security: ## Run security scanner
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest)
	gosec ./...

install: build ## Install binary to GOPATH
	$(GOCMD) install $(CMD_DIR)

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Build Hash: $(BUILD_HASH)"
	@echo "Build Time: $(BUILD_TIME)"

.PHONY: all help build build-community build-enterprise build-cli build-both \
	build-prod build-prod-ce build-prod-ent \
	build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 \
	build-all build-cli-all show-edition \
	clean test coverage lint deps tidy \
	docker-build docker-push docker-compose-up docker-compose-down \
	migrate run dev fmt vet security install version