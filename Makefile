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

# Build extras only (transformer and reranker)
build-docker-extras:
	cd extra/transformer_server && \
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/transformer_server_cpu:latest --push -f dockerfile.cpu .
	cd extra/reranker && \
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/reranker_cpu:latest --push -f dockerfile.cpu .

.PHONY: all build build-frontend build-binaries build-local test clean start-dev stop-dev
