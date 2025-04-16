#!/bin/bash

# Demo script for mockproxy utility

# Set variables
PROXY_PORT=8080
MOCK_LLM="mockgpt"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== MockProxy Demo ===${NC}"
echo -e "${BLUE}This script demonstrates how to use the mockproxy utility${NC}"

# Step 1: Build the mockproxy
echo -e "\n${GREEN}Step 1: Building the mockproxy utility...${NC}"
./build.sh

# Check if the build was successful
if [ ! -f "./mockproxy" ]; then
    echo -e "${YELLOW}Failed to build mockproxy. Please check for errors.${NC}"
    exit 1
fi

# Step 2: Start the proxy in the background
echo -e "\n${GREEN}Step 2: Starting the mockproxy...${NC}"
echo -e "Running on port ${PROXY_PORT}"
./mockproxy --conf ./conf.json &
PROXY_PID=$!

# Give the proxy time to start
sleep 2
echo -e "Proxy started with PID: ${PROXY_PID}"

# Step 3: Test the LLM endpoint
echo -e "\n${GREEN}Step 3: Testing LLM endpoint...${NC}"
echo -e "Sending a test request to the LLM endpoint:"
echo -e "${BLUE}curl -X POST http://localhost:${PROXY_PORT}/llm/rest/${MOCK_LLM}/completions -H \"Content-Type: application/json\" -d '{\"model\": \"gpt-3.5-turbo\", \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]}'${NC}"

curl -s -X POST http://localhost:${PROXY_PORT}/llm/rest/${MOCK_LLM}/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}' | jq . || echo "Response couldn't be parsed as JSON (jq may not be installed)"

# Step 4: Test the datasource endpoint
echo -e "\n${GREEN}Step 4: Testing datasource endpoint...${NC}"
echo -e "Sending a test request to the datasource endpoint:"
echo -e "${BLUE}curl -X POST http://localhost:${PROXY_PORT}/datasource/mockvectordb -H \"Content-Type: application/json\" -d '{\"query\": \"test query\", \"n\": 5}'${NC}"

curl -s -X POST http://localhost:${PROXY_PORT}/datasource/mockvectordb \
  -H "Content-Type: application/json" \
  -d '{"query": "test query", "n": 5}' | jq . || echo "Response couldn't be parsed as JSON (jq may not be installed)"

# Step 5: Clean up
echo -e "\n${GREEN}Step 5: Cleaning up...${NC}"
echo -e "Stopping the proxy (PID: ${PROXY_PID})"
kill $PROXY_PID

echo -e "\n${BLUE}Demo complete!${NC}"
echo -e "${BLUE}You can run the proxy manually with:${NC}"
echo -e "${BLUE}./mockproxy --conf ./conf.json${NC}"
