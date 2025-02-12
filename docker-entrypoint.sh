#!/bin/sh
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    exec /app/midsommar-amd64
elif [ "$ARCH" = "aarch64" ]; then
    exec /app/midsommar-arm64
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi
