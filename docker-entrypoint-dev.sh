#!/bin/bash

# Start frontend in background
cd /app/ui/admin-frontend
npm start &

# Start backend with Air for hot reloading
cd /app
air -c .air.toml

# Keep container running
wait
