#!/bin/bash

# Build the mockproxy utility
echo "Building mockproxy..."
go build -o mockproxy main.go dependencies.go

# Check if the build was successful
if [ $? -eq 0 ]; then
    echo "Build successful. You can run the utility with:"
    echo "./mockproxy --conf ./conf.json"
else
    echo "Build failed."
fi
