#!/bin/bash
# Test script for file-based data collection plugins

set -e

echo "🔧 Setting up file-based data collection plugin test..."

# Set up test environment
export DATA_OUTPUT_DIR="./test_data"
export PLUGINS_CONFIG_PATH="./examples/plugins-file-collectors.yaml"

# Create test directories
mkdir -p ./test_data/collected/{proxy_logs,analytics,budget}

echo "📂 Test directories created:"
echo "   - ./test_data/collected/proxy_logs"  
echo "   - ./test_data/collected/analytics"
echo "   - ./test_data/collected/budget"

echo ""
echo "⚙️ Plugin Configuration:"
echo "   Config file: $PLUGINS_CONFIG_PATH"
echo "   Output directory: $DATA_OUTPUT_DIR"

# Check if plugin binaries exist
echo ""
echo "🔍 Checking plugin binaries..."

PROXY_PLUGIN="./plugins/examples/file_proxy_collector/file_proxy_collector"
ANALYTICS_PLUGIN="./plugins/examples/file_analytics_collector/file_analytics_collector" 
BUDGET_PLUGIN="./plugins/examples/file_budget_collector/file_budget_collector"

if [ -f "$PROXY_PLUGIN" ]; then
    echo "   ✅ Proxy collector: $PROXY_PLUGIN"
else
    echo "   ❌ Proxy collector binary not found: $PROXY_PLUGIN"
    echo "      Run: cd plugins/examples/file_proxy_collector && go build -o file_proxy_collector main.go"
fi

if [ -f "$ANALYTICS_PLUGIN" ]; then
    echo "   ✅ Analytics collector: $ANALYTICS_PLUGIN"
else
    echo "   ❌ Analytics collector binary not found: $ANALYTICS_PLUGIN"
    echo "      Run: cd plugins/examples/file_analytics_collector && go build -o file_analytics_collector main.go"
fi

if [ -f "$BUDGET_PLUGIN" ]; then
    echo "   ✅ Budget collector: $BUDGET_PLUGIN"
else
    echo "   ❌ Budget collector binary not found: $BUDGET_PLUGIN"
    echo "      Run: cd plugins/examples/file_budget_collector && go build -o file_budget_collector main.go"
fi

echo ""
echo "🚀 To test the plugins:"
echo "1. Start microgateway with the plugin configuration:"
echo "   export PLUGINS_CONFIG_PATH=$PLUGINS_CONFIG_PATH"
echo "   export DATA_OUTPUT_DIR=$DATA_OUTPUT_DIR"
echo "   ./microgateway"
echo ""
echo "2. Make some API calls to generate data:"
echo "   # Create an app and get credentials first"
echo "   ./mgw app create --name=\"Test App\" --owner-email=\"test@example.com\""
echo "   TOKEN=\$(./mgw app list --format=json | jq -r '.[0].credentials[0].secret')"
echo ""
echo "   # Make LLM API call (replace with your LLM endpoint)"
echo "   curl -X POST http://localhost:8080/llm/rest/gpt-4/chat/completions \\"
echo "     -H \"Authorization: Bearer \$TOKEN\" \\"
echo "     -H \"Content-Type: application/json\" \\"
echo "     -d '{\"model\":\"gpt-4\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}'"
echo ""
echo "3. Check the generated files:"
echo "   ls -la $DATA_OUTPUT_DIR/collected/*/"
echo "   tail $DATA_OUTPUT_DIR/collected/proxy_logs/proxy_logs_\$(date +%Y-%m-%d).jsonl"
echo "   tail $DATA_OUTPUT_DIR/collected/analytics/analytics_\$(date +%Y-%m-%d).jsonl" 
echo "   tail $DATA_OUTPUT_DIR/collected/budget/budget_usage_\$(date +%Y-%m-%d).csv"
echo ""
echo "📊 Expected output files:"
echo "   - Proxy logs: Daily JSONL files with request/response data"
echo "   - Analytics: Daily JSONL files with token and cost data"
echo "   - Budget: Daily CSV files + aggregate JSON summary"
echo ""
echo "✨ Plugin behaviors:"
echo "   - Proxy logs: Truncated request/response previews (200 chars)"
echo "   - Analytics: Full token usage, cost, and metadata"
echo "   - Budget: Individual usage + running aggregates per app/LLM"
echo ""
echo "🔧 Plugin Configuration Files:"
echo "   - Supplement DB: ./examples/plugins-file-collectors.yaml"
echo "   - Replace DB: ./examples/plugins-file-collectors-replace.yaml" 
echo "   - Mixed mode: ./examples/plugins-mixed-example.yaml"