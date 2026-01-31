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

# Build enterprise edition
echo "Performing Go build (enterprise)..."
mkdir -p bin
CGO_ENABLED=1 go build -tags enterprise -o bin/midsommar-ent .
if [ $? -ne 0 ]; then
    echo "Go build failed!"
    exit 1
fi

# Start the server
echo "Starting server..."
./bin/midsommar-ent &
SERVER_PID=$!

# Wait for any process to exit
wait -n

# Kill remaining processes
kill $SERVER_PID 2>/dev/null

# Exit with the same code as the failed process
exit $?
