# microgateway/Makefile

# Variables
BINARY_NAME=microgateway
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Directories
CMD_DIR=./cmd/microgateway
DIST_DIR=./dist

.PHONY: all build clean test coverage lint help deps tidy docker-build docker-compose-up

all: test build ## Run tests and build

build: ## Build the binary
	@mkdir -p $(DIST_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(CMD_DIR)

build-all: ## Build for all platforms
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(DIST_DIR)

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

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

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help