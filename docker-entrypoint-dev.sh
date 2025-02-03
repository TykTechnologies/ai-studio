#!/bin/bash

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

# Initial build and run
echo "Performing initial Go build..."
go build -o ./tmp/main .
if [ $? -ne 0 ]; then
    echo "Initial Go build failed!"
    exit 1
fi

# Start the initial binary in the background
echo "Starting initial server..."
./tmp/main &
FIRST_RUN_PID=$!

# Start Air for hot reloading
echo "Starting Air for hot reloading..."
air -c .air.toml &
AIR_PID=$!

# Wait for any process to exit
wait -n

# Kill remaining processes
kill $FIRST_RUN_PID 2>/dev/null
kill $AIR_PID 2>/dev/null

# Exit with the same code as the failed process
exit $?
