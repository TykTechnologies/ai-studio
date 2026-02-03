#!/bin/bash

# =============================================================================
# DEPRECATED: This entrypoint script is deprecated.
#
# Please use the new Docker Compose-based development environment instead:
#   make dev          - Start minimal development environment
#   make dev-full     - Start full stack with Gateway and Plugins
#
# See dev/README.md for full documentation.
# =============================================================================

# Create .env from example if it doesn't exist
if [ ! -f /app/.env ]; then
    cp /app/.env.example /app/.env
    echo ".env file created from .env.example"
fi

# Start frontend in background
echo "Starting frontend..."
cd /app/ui/admin-frontend
npm start &

# Build and start backend
echo "Building and starting backend..."
cd /app

# Build edition based on BUILD_EDITION env var (default: enterprise)
BUILD_EDITION="${BUILD_EDITION:-enterprise}"
echo "Performing Go build (${BUILD_EDITION})..."
mkdir -p bin

if [ "$BUILD_EDITION" = "community" ] || [ "$BUILD_EDITION" = "ce" ]; then
    CGO_ENABLED=1 go build -o bin/midsommar .
else
    CGO_ENABLED=1 go build -tags enterprise -o bin/midsommar-ent .
fi

if [ $? -ne 0 ]; then
    echo "Go build failed!"
    exit 1
fi

# Start the server
echo "Starting server..."
if [ "$BUILD_EDITION" = "community" ] || [ "$BUILD_EDITION" = "ce" ]; then
    ./bin/midsommar &
else
    ./bin/midsommar-ent &
fi
SERVER_PID=$!

# Wait for any process to exit
wait -n

# Kill remaining processes
kill $SERVER_PID 2>/dev/null

# Exit with the same code as the failed process
exit $?
