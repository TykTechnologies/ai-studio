# Build stage
FROM golang:1.23.2-alpine AS builder

WORKDIR /app

# set up
RUN apk add git make npm

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN git clone https://github.com/lonelycode/langchaingo.git /langchaingo
RUN go mod download

# Copy the source code
COPY . .

# Build the application for multiple architectures
ARG TARGETARCH
ENV NODE_ENV notDevelopment

RUN mkdir -p docs/site/public && \
    touch docs/site/public/empty && \
    make clean && \
    make build-admin-frontend-clean && \
    GOOS=linux GOARCH=$TARGETARCH go build -o midsommar .

# Final stage
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache poppler-utils

# Copy the binary from builder
COPY --from=builder /app/templates .
COPY --from=builder /app/midsommar .

# Expose the required ports
EXPOSE 8080 9090

# Run the binary
ENTRYPOINT ["/app/midsommar"]
