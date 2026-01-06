# Dockerfile

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata gcc musl-dev sqlite-dev npm nodejs

# Set working directory
WORKDIR /build

# Clone the langchaingo fork first
RUN git clone --depth 1 https://github.com/lonelycode/langchaingo /build/langchaingo

# Copy go.mod files first for better caching
COPY go.mod go.sum ./
COPY microgateway/go.mod microgateway/go.sum ./microgateway/

# Copy all source code
COPY . .

# Fix the replace directives to use the cloned langchaingo
RUN sed -i 's|replace github.com/tmc/langchaingo => /Users/martinbuhr/apps/lonelycode/langchaingo|replace github.com/tmc/langchaingo => /build/langchaingo|g' go.mod && \
    sed -i 's|replace github.com/tmc/langchaingo => /Users/martinbuhr/apps/lonelycode/langchaingo|replace github.com/tmc/langchaingo => /build/langchaingo|g' microgateway/go.mod

# Build frontend
RUN cd ui/admin-frontend && npm ci && PUBLIC_URL="/" REACT_APP_API_URL="" CI=false npm run build

# Download dependencies
RUN go mod download

# Build arguments for version information and edition
ARG VERSION=dev
ARG BUILD_HASH=unknown
ARG BUILD_TIME=unknown
ARG EDITION=ce

# Build the binary
RUN if [ "$EDITION" = "ent" ]; then \
        BUILD_TAGS="-tags enterprise"; \
    else \
        BUILD_TAGS=""; \
    fi && \
    echo "Building tyk-ai-studio $EDITION edition with tags: $BUILD_TAGS" && \
    CGO_ENABLED=1 go build \
        $BUILD_TAGS \
        -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" \
        -trimpath \
        -o tyk-ai-studio \
        .

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    poppler-utils \
    wget

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/tyk-ai-studio ./tyk-ai-studio

# Copy templates directory
COPY templates ./templates

# Copy docs_links.json file
COPY config/docs_links.json ./config/docs_links.json

# Expose the required ports
EXPOSE 8080 9090

# Run the binary directly
ENTRYPOINT ["./tyk-ai-studio"]
