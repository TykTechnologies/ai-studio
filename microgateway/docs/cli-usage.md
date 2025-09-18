# CLI Usage Guide

Complete reference for using the `mgw` CLI tool to manage the microgateway.

## Initial Setup

### Configuration
```bash
# Environment variables (recommended)
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="your-admin-token-here"

# Or use command-line flags
mgw --url=http://localhost:8080 --token=your-admin-token-here llm list
```

### Configuration File
```bash
# Create ~/.mgw/config.yaml
mkdir -p ~/.mgw
cat > ~/.mgw/config.yaml << EOF
url: http://localhost:8080
token: your-admin-token-here
format: table
verbose: false
timeout: 30s
EOF
```

## System Management

### Health Checks
```bash
# Check service health
mgw system health

# Check service readiness
mgw system ready

# Get service version
mgw system version

# View Prometheus metrics
mgw system metrics
```

### Configuration
```bash
# View CLI configuration
mgw system config

# Test connectivity
mgw --verbose system health
```

## LLM Management

### List LLMs
```bash
# List all active LLMs
mgw llm list

# List by vendor
mgw llm list --vendor=openai

# Include inactive LLMs
mgw llm list --active=false

# Custom pagination
mgw llm list --page=2 --limit=10

# JSON output
mgw llm list --format=json
```

### Create LLMs
```bash
# OpenAI LLM
mgw llm create \
  --name="GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000.0 \
  --rate-limit=100

# Anthropic LLM
mgw llm create \
  --name="Claude 3.5 Sonnet" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500.0

# Ollama LLM (local)
mgw llm create \
  --name="Local Llama" \
  --vendor=ollama \
  --model=llama3.1:8b \
  --endpoint=http://localhost:11434 \
  --budget=0
```

### Manage LLMs
```bash
# Get LLM details
mgw llm get 1

# Update LLM configuration
mgw llm update 1 --budget=2000.0 --active=true

# Get usage statistics
mgw llm stats 1

# Delete (soft delete)
mgw llm delete 1
```

## Application Management

### Create Applications
```bash
# Basic application
mgw app create \
  --name="My AI App" \
  --email=developer@company.com \
  --budget=100.0

# Application with specific settings
mgw app create \
  --name="Production App" \
  --email=ops@company.com \
  --description="Main application" \
  --budget=5000.0 \
  --reset-day=1 \
  --rate-limit=1000 \
  --allowed-ips="203.0.113.1,203.0.113.2" \
  --llm-ids="1,2,3"
```

### Manage Applications
```bash
# List applications
mgw app list

# Get application details
mgw app get 1

# Update application settings
mgw app update 1 --budget=2000.0 --description="Updated description"

# Manage LLM associations
mgw app llms 1              # View current associations
mgw app llms 1 --set="1,3"  # Associate with specific LLMs

# Delete application
mgw app delete 1
```

## Credential Management

### Manage Credentials
```bash
# List credentials for an app
mgw credential list 1

# Create new credential
mgw credential create 1 --name="Production Key"

# Create credential with expiration
mgw credential create 1 \
  --name="Temporary Key" \
  --expires=2024-12-31T23:59:59Z

# Delete credential
mgw credential delete 1 2
```

## Token Management

### Token Operations
```bash
# List tokens for an app
mgw token list --app-id=1

# Create admin token
mgw token create \
  --app-id=1 \
  --name="Admin Token" \
  --scopes="admin" \
  --expires=24h

# Create read-only token
mgw token create \
  --app-id=1 \
  --name="Read Only Token" \
  --scopes="read"

# Get token information
mgw token info abc123def456

# Validate token
mgw token validate abc123def456

# Revoke token
mgw token revoke abc123def456
```

## Budget Management

### Budget Operations
```bash
# List all budgets (admin)
mgw budget list

# Check application budget usage
mgw budget usage 1

# Check budget for specific LLM
mgw budget usage 1 --llm-id=2

# Update budget limits
mgw budget update 1 --budget=1500.0 --reset-day=15

# Get budget history
mgw budget history 1

# Budget history for specific time range
mgw budget history 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z \
  --llm-id=1
```

## Analytics and Reporting

### Analytics Operations
```bash
# Get recent analytics events
mgw analytics events 1

# Analytics events with pagination
mgw analytics events 1 --page=2 --limit=100

# Analytics summary (last 7 days)
mgw analytics summary 1

# Analytics summary for specific period
mgw analytics summary 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z

# Cost analysis (last 30 days)
mgw analytics costs 1

# Cost analysis for specific period
mgw analytics costs 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z
```

## Output Formats

### Format Options
```bash
# Table format (default) - human-readable
mgw llm list --format=table

# JSON format - machine-readable
mgw llm list --format=json

# YAML format - configuration-friendly
mgw llm list --format=yaml
```

### Example Outputs

#### Table Format
```
ID  NAME              VENDOR     MODEL                      ACTIVE  BUDGET
1   GPT-4             openai     gpt-4                      ✅      $1000
2   Claude Sonnet     anthropic  claude-sonnet-4-20250514  ✅      unlimited
3   Local Llama       ollama     llama3.1:8b               ❌      unlimited
```

#### JSON Format
```json
{
  "data": [
    {
      "id": 1,
      "name": "GPT-4",
      "vendor": "openai",
      "default_model": "gpt-4",
      "is_active": true,
      "monthly_budget": 1000.0
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 3
  }
}
```

## Complete Workflow Example

### Setup Workflow
```bash
# 1. Create LLMs
mgw llm create \
  --name="GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000.0

mgw llm create \
  --name="Claude 3.5 Sonnet" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500.0

# 2. Create application
mgw app create \
  --name="My AI Application" \
  --email=developer@company.com \
  --description="Main app" \
  --budget=800.0 \
  --llm-ids="1,2"

# 3. Create credentials
mgw credential create 1 --name="Production Credentials"

# 4. Create API token
mgw token create \
  --app-id=1 \
  --name="Application Token" \
  --scopes="api" \
  --expires=720h

# 5. Monitor usage
mgw analytics summary 1
mgw budget usage 1

# 6. Check system health
mgw system health
```

## Automation and Scripting

### Bulk Operations
```bash
# Export configurations
mkdir -p backup/$(date +%Y%m%d)
mgw llm list --format=yaml > backup/$(date +%Y%m%d)/llms.yaml
mgw app list --format=yaml > backup/$(date +%Y%m%d)/apps.yaml

# Create multiple LLMs
for model in gpt-3.5-turbo gpt-4 gpt-4-turbo; do
  mgw llm create \
    --name="OpenAI $model" \
    --vendor=openai \
    --model=$model \
    --api-key=$OPENAI_API_KEY \
    --budget=100.0
done
```

### Scripting Example
```bash
#!/bin/bash
# setup-environment.sh

set -e

# Set CLI configuration
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="$ADMIN_TOKEN"

# Create LLMs and capture IDs
LLM_GPT4=$(mgw llm create \
  --name="GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000 \
  --format=json | jq -r '.data.id')

LLM_CLAUDE=$(mgw llm create \
  --name="Claude" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500 \
  --format=json | jq -r '.data.id')

# Create application
APP_ID=$(mgw app create \
  --name="Production App" \
  --email=ops@company.com \
  --budget=1200 \
  --llm-ids="$LLM_GPT4,$LLM_CLAUDE" \
  --format=json | jq -r '.data.id')

echo "Setup complete. App ID: $APP_ID"
```

## Error Handling

### Common Errors
```bash
# Missing required fields
mgw llm create --name="Test"
# Error: vendor is required

# Invalid values
mgw app create --name="Test" --email=invalid-email
# Error: invalid email format

# Resource not found
mgw llm get 999
# Error: LLM not found

# Authentication issues
mgw llm list --token=invalid
# Error: authentication failed
```

### Debug Mode
```bash
# Enable verbose output for debugging
mgw --verbose llm list

# Show request/response details
mgw --debug system health
```

## Monitoring and Maintenance

### Daily Operations
```bash
# Check overall status
mgw system health

# Monitor budget usage
mgw budget usage 1

# Review recent activity
mgw analytics summary 1 --start=$(date -d "24 hours ago" -Iseconds)

# Check for errors
mgw analytics events 1 --format=json | jq '.data[] | select(.status_code >= 400)'
```

### Maintenance Tasks
```bash
# Token rotation
OLD_TOKEN="old-token-value"
NEW_TOKEN=$(mgw token create --app-id=1 --name="New Token" --expires=30d --format=json | jq -r '.data.token')
mgw token revoke $OLD_TOKEN

# Configuration backup
mgw llm list --format=yaml > backups/llms-$(date +%Y%m%d).yaml
mgw app list --format=yaml > backups/apps-$(date +%Y%m%d).yaml
```

## Global Options

### Available for All Commands
- `--url`: Microgateway URL (default: $MGW_URL)
- `--token`: Authentication token (default: $MGW_TOKEN)
- `--format`: Output format - table, json, yaml (default: table)
- `--verbose`: Enable verbose output
- `--timeout`: Request timeout (default: 30s)
- `--help`: Show command help

### Examples
```bash
# Override default settings
mgw --url=https://gateway.company.com \
    --token=prod-token \
    --format=json \
    --timeout=60s \
    llm list

# Debug a failing command
mgw --verbose token validate invalid-token
```

---

This CLI provides comprehensive management capabilities for the microgateway platform. For API-level integration, see the [API Reference](../API_REFERENCE.md).
