# deployments/Dockerfile

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /build

# Copy parent project go.mod first (for the replace directive)
COPY go.mod go.sum ./
COPY microgateway/go.mod microgateway/go.sum ./microgateway/

# Copy all source code needed for the replace directive
COPY . .

# Change to microgateway directory
WORKDIR /build/microgateway

# Download dependencies (now the replace directive can find the parent)
RUN go mod download

# Build arguments for version information
ARG VERSION=dev
ARG BUILD_HASH=unknown
ARG BUILD_TIME=unknown

# Build both microgateway server and CLI with optimized flags
# Note: Not setting GOOS/GOARCH explicitly - Docker Buildx will use native arch for each platform
RUN CGO_ENABLED=1 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" \
    -trimpath \
    -o microgateway \
    ./cmd/microgateway

RUN CGO_ENABLED=1 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" \
    -trimpath \
    -o mgw \
    ./cmd/mgw

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies including sqlite for database operations
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite \
    wget \
    curl

# Create non-root user
RUN addgroup -g 1000 gateway && \
    adduser -D -u 1000 -G gateway gateway

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/microgateway/microgateway ./
COPY --from=builder /build/microgateway/mgw ./

# Create necessary directories with proper permissions
RUN mkdir -p /app/data /app/logs /app/plugins && \
    chown -R gateway:gateway /app

# Switch to non-root user
USER gateway

# Expose ports - 8080 for HTTP, 9090 for gRPC control server
EXPOSE 8080 9090

# Health check using the built-in health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set default environment variables for microgateway
ENV SERVER_HOST=0.0.0.0
ENV SERVER_PORT=8080
ENV DATABASE_TYPE=sqlite
ENV DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json
ENV GATEWAY_MODE=standalone

# Labels for better image management
LABEL org.opencontainers.image.title="Tyk AI Microgateway"
LABEL org.opencontainers.image.description="AI/LLM Gateway for secure, governed, and scalable AI integration"
LABEL org.opencontainers.image.source="https://github.com/TykTechnologies/midsommar"
LABEL org.opencontainers.image.vendor="Tyk Technologies"

# Run the microgateway server by default
ENTRYPOINT ["./microgateway"]