# Microgateway CLI Examples

This document provides practical examples of using the `mgw` CLI tool to manage the microgateway.

## Initial Setup

First, set your environment variables or use command-line flags:

```bash
# Environment variables (recommended)
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="your-admin-token-here"

# Or use command-line flags
mgw --url=http://localhost:8080 --token=your-admin-token-here llm list
```

## LLM Management

### List LLMs
```bash
# List all active LLMs
mgw llm list

# List LLMs by vendor
mgw llm list --vendor=openai

# Include inactive LLMs
mgw llm list --active=false

# Custom pagination
mgw llm list --page=2 --limit=10

# Output as JSON
mgw llm list --format=json
```

### Create LLMs
```bash
# Create OpenAI LLM
mgw llm create \
  --name="GPT-4 Production" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000.0 \
  --rate-limit=100

# Create Anthropic LLM
mgw llm create \
  --name="Claude 3.5 Sonnet" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500.0

# Create Ollama LLM
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

# Get LLM usage statistics
mgw llm stats 1

# Delete (disable) LLM
mgw llm delete 1
```

## Application Management

### Create Applications
```bash
# Create a basic app
mgw app create \
  --name="My AI App" \
  --email=developer@company.com \
  --budget=100.0

# Create app with specific settings
mgw app create \
  --name="Production App" \
  --email=ops@company.com \
  --description="Production AI application" \
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

# Get app details
mgw app get 1

# Update app settings
mgw app update 1 --budget=2000.0 --description="Updated description"

# Manage LLM associations
mgw app llms 1              # View current associations
mgw app llms 1 --set="1,3"  # Associate with LLMs 1 and 3

# Delete app
mgw app delete 1
```

## Credential Management

```bash
# List credentials for an app
mgw credential list 1

# Create a new credential
mgw credential create 1 --name="Production Key"

# Create credential with expiration
mgw credential create 1 \
  --name="Temporary Key" \
  --expires=2024-12-31T23:59:59Z

# Delete credential
mgw credential delete 1 2
```

## Token Management

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

# Validate a token
mgw token validate abc123def456

# Revoke token
mgw token revoke abc123def456
```

## Budget Management

```bash
# List all budget information (admin)
mgw budget list

# Check app budget usage
mgw budget usage 1

# Check budget for specific LLM
mgw budget usage 1 --llm-id=2

# Update budget limits
mgw budget update 1 --budget=1500.0 --reset-day=15

# Get budget history
mgw budget history 1

# Get budget history for specific time range
mgw budget history 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z \
  --llm-id=1
```

## Analytics and Reporting

```bash
# Get recent analytics events
mgw analytics events 1

# Get analytics events with pagination
mgw analytics events 1 --page=2 --limit=100

# Get analytics summary (last 7 days)
mgw analytics summary 1

# Get analytics summary for specific period
mgw analytics summary 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z

# Get cost analysis (last 30 days)
mgw analytics costs 1

# Get cost analysis for specific period
mgw analytics costs 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z
```

## System Monitoring

```bash
# Check service health
mgw system health

# Check service readiness
mgw system ready

# Get Prometheus metrics
mgw system metrics

# View CLI configuration
mgw system config

# Get service version info
mgw system version
```

## Output Formats

The CLI supports multiple output formats:

```bash
# Table format (default) - human-readable
mgw llm list --format=table

# JSON format - machine-readable
mgw llm list --format=json

# YAML format - configuration-friendly
mgw llm list --format=yaml
```

## Complete Workflow Example

Here's a complete example of setting up a microgateway configuration:

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
  --description="Production AI app" \
  --budget=800.0 \
  --llm-ids="1,2"

# 3. Create credentials for the app
mgw credential create 1 --name="Production Credentials"

# 4. Create API token for the app
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

## Configuration Management

You can create a configuration file to avoid repeating common flags:

```bash
# Create ~/.mgw.yaml
cat > ~/.mgw.yaml << EOF
url: http://localhost:8080
token: your-admin-token-here
format: table
verbose: false
EOF

# Now you can use the CLI without specifying URL and token each time
mgw llm list
mgw app create --name="Test App" --email=test@example.com
```

## Error Handling

The CLI provides clear error messages for common issues:

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