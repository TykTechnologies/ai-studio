# deployments/Dockerfile

# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with version info
ARG VERSION=dev
ARG BUILD_HASH=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" \
    -o microgateway \
    ./cmd/microgateway

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 gateway && \
    adduser -D -u 1000 -G gateway gateway

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/microgateway .

# Copy default configuration
COPY --from=builder /build/configs/.env.example .env.example

# Create necessary directories
RUN mkdir -p /app/data /app/logs && \
    chown -R gateway:gateway /app

# Switch to non-root user
USER gateway

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set default environment variables
ENV PORT=8080
ENV DATABASE_TYPE=sqlite
ENV DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Run the application
ENTRYPOINT ["./microgateway"]