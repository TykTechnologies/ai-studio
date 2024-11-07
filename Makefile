# Makefile

# Variables
ADMIN_FRONTEND_DIR := ui/admin-frontend
FORCE_BUILD := false

# Default target
all: build

# Build target
build: build-admin-frontend build-golang

# Build admin frontend
build-admin-frontend:
	@if [ ! -d "$(ADMIN_FRONTEND_DIR)/build" ] || [ "$(FORCE_BUILD)" = "true" ]; then \
		cd $(ADMIN_FRONTEND_DIR) && \
		npm run build && \
		cd ../..; \
	else \
		echo "Admin frontend build already exists. Use FORCE_BUILD=true to rebuild."; \
	fi

# Build admin frontend
build-admin-frontend-clean:
	cd ui/admin-frontend && \
	npm install && \
	npm run build

# Build Golang
build-golang:
	go build

# Test target
test: build-admin-frontend
	go test ./...

# Clean target
clean:
	rm -rf $(ADMIN_FRONTEND_DIR)/build
	rm -f midsommar

# Start frontend development mode
start-frontend:
	cd $(ADMIN_FRONTEND_DIR) && npm start

# Stop frontend development mode
stop-frontend:
	@pkill -f "npm start" || true

# Start backend
start-backend: build-golang
	./midsommar

# Stop backend
stop-backend:
	@pkill -f "./midsommar" || true

# Start both frontend and backend in screen
start-dev: stop-dev
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ".env file created from .env.example"; \
	fi
	@if [ ! -d "../langchaingo" ]; then \
		git clone https://github.com/lonelycode/langchaingo ../langchaingo; \
		echo "langchaingo repository cloned"; \
	fi
	@screen -dmS midsommar -t frontend bash -c 'make start-frontend'
	@screen -S midsommar -X screen -t backend bash -c 'make start-backend'
	@screen -r midsommar

build-docker-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/midsommar:latest --push .

build-docker-extras:
	cd extra/transformer_server && \
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/transformer_server_cpu:latest --push -f dockerfile.cpu .
	cd extra/reranker && \
	docker buildx build --platform linux/amd64,linux/arm64 -t tykio/reranker_cpu:latest --push -f dockerfile.cpu .
# Stop both frontend and backend
stop-dev:
	make stop-frontend
	make stop-backend
	@screen -S midsommar -X quit || true

.PHONY: all build build-admin-frontend build-golang test clean start-frontend stop-frontend start-backend stop-backend start-all stop-all
