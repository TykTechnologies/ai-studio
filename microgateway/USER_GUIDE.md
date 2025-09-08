# Microgateway User Guide

Complete guide to using the Microgateway AI/LLM management platform.

## Quick Start

### 1. Installation
```bash
# Build from source
cd microgateway
make build-both

# Or download pre-built binaries
# wget https://releases.../microgateway-linux-amd64
# wget https://releases.../mgw-linux-amd64
```

### 2. Initial Setup
```bash
# Create configuration
cp configs/.env.example .env
nano .env  # Edit with your settings

# Set security keys
JWT_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 16)
echo "JWT_SECRET=$JWT_SECRET" >> .env
echo "ENCRYPTION_KEY=$ENCRYPTION_KEY" >> .env
```

### 3. Start Microgateway
```bash
# Run database migrations
./dist/microgateway -migrate

# Start the server
./dist/microgateway
```

### 4. Configure CLI
```bash
# Set CLI environment
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="admin-token-here"  # You'll generate this in step 5

# Test CLI connection
./dist/mgw system health
```

## Complete Setup Walkthrough

### Step 1: Create Your First LLM
```bash
# Create OpenAI LLM
./dist/mgw llm create \
  --name="GPT-4 Production" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000.0 \
  --rate-limit=100 \
  --active=true

# Create Anthropic LLM
./dist/mgw llm create \
  --name="Claude 3.5 Sonnet" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500.0

# Verify LLMs created
./dist/mgw llm list
```

### Step 2: Create Application
```bash
# Create your application
./dist/mgw app create \
  --name="My AI Application" \
  --email=developer@company.com \
  --description="Production AI application" \
  --budget=800.0 \
  --reset-day=1 \
  --llm-ids="1,2"

# Verify app created
./dist/mgw app get 1
```

### Step 3: Generate Credentials
```bash
# Create credentials for the app
./dist/mgw credential create 1 \
  --name="Production Credentials"

# IMPORTANT: Save the secret from the response!
# Example response:
# {
#   "key_id": "key_abc123",
#   "secret_hash": "secret_xyz789"  # Save this secret!
# }
```

### Step 4: Create API Token
```bash
# Generate API token for your application
./dist/mgw token create \
  --app-id=1 \
  --name="Application API Token" \
  --scopes="api" \
  --expires=720h

# Save the token from the response for gateway requests
```

### Step 5: Test Gateway Functionality
```bash
# Use the app token to make LLM requests
curl -X POST http://localhost:8080/llm/rest/gpt-4-production/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <app-token>" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'
```

## Daily Operations

### Monitoring Usage
```bash
# Check overall budget status
./dist/mgw budget usage 1

# View recent analytics
./dist/mgw analytics summary 1 --start=$(date -d "7 days ago" -Iseconds)

# Check specific LLM usage
./dist/mgw llm stats 1

# Monitor costs
./dist/mgw analytics costs 1 --start=$(date -d "30 days ago" -Iseconds)
```

### Managing Applications
```bash
# List all applications
./dist/mgw app list

# Update app budget
./dist/mgw app update 1 --budget=1500.0

# Add LLM association
./dist/mgw app llms 1 --set="1,2,3"

# View app details
./dist/mgw app get 1
```

### Token Management
```bash
# List active tokens
./dist/mgw token list --app-id=1

# Create temporary token
./dist/mgw token create \
  --app-id=1 \
  --name="Temporary Access" \
  --expires=24h

# Revoke compromised token
./dist/mgw token revoke <token-value>

# Validate token
./dist/mgw token validate <token-value>
```

### LLM Configuration Updates
```bash
# Update LLM settings
./dist/mgw llm update 1 \
  --budget=2000.0 \
  --rate-limit=200

# Temporarily disable LLM
./dist/mgw llm update 1 --active=false

# Update API key
./dist/mgw llm update 1 --api-key=$NEW_OPENAI_KEY
```

## Advanced Usage

### Bulk Operations
```bash
# Export all configurations
mkdir -p backup/$(date +%Y%m%d)
./dist/mgw llm list --format=yaml > backup/$(date +%Y%m%d)/llms.yaml
./dist/mgw app list --format=yaml > backup/$(date +%Y%m%d)/apps.yaml

# Create multiple LLMs from script
for model in gpt-3.5-turbo gpt-4 gpt-4-turbo; do
  ./dist/mgw llm create \
    --name="OpenAI $model" \
    --vendor=openai \
    --model=$model \
    --api-key=$OPENAI_API_KEY \
    --budget=100.0
done
```

### Scripting with CLI
```bash
#!/bin/bash
# setup-environment.sh

# Set CLI configuration
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="$ADMIN_TOKEN"

# Create LLMs
LLM_GPT4=$(./dist/mgw llm create \
  --name="GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000 \
  --format=json | jq -r '.data.id')

LLM_CLAUDE=$(./dist/mgw llm create \
  --name="Claude" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500 \
  --format=json | jq -r '.data.id')

# Create application with LLMs
APP_ID=$(./dist/mgw app create \
  --name="Production App" \
  --email=ops@company.com \
  --budget=1200 \
  --llm-ids="$LLM_GPT4,$LLM_CLAUDE" \
  --format=json | jq -r '.data.id')

echo "Setup complete. App ID: $APP_ID"
```

### Integration with CI/CD
```bash
#!/bin/bash
# deploy-config.sh

# Deploy LLM configurations
./dist/mgw llm create \
  --name="Production GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY_PROD \
  --budget=${GPT4_BUDGET:-1000} \
  --rate-limit=${GPT4_RATE_LIMIT:-100}

# Verify deployment
./dist/mgw system health
./dist/mgw llm list --format=json | jq '.data | length'
```

## Monitoring and Maintenance

### Health Monitoring
```bash
# Basic health check script
#!/bin/bash
if ./dist/mgw system health > /dev/null 2>&1; then
  echo "✅ Microgateway is healthy"
else
  echo "❌ Microgateway health check failed"
  exit 1
fi

# Advanced monitoring
./dist/mgw system metrics | grep -E "(requests_total|errors_total)"
```

### Usage Monitoring
```bash
# Daily usage report
#!/bin/bash
echo "=== Daily Usage Report ===" 
echo "Date: $(date)"
echo

for app_id in $(./dist/mgw app list --format=json | jq -r '.data[].id'); do
  app_name=$(./dist/mgw app get $app_id --format=json | jq -r '.data.name')
  echo "App: $app_name (ID: $app_id)"
  
  ./dist/mgw budget usage $app_id --format=json | \
    jq -r '.data | "  Budget: \(.current_usage)/\(.monthly_budget) (\(.percentage_used)%)"'
  
  ./dist/mgw analytics summary $app_id --format=json | \
    jq -r '.data | "  Requests: \(.total_requests) (\(.successful_requests) success)"'
  echo
done
```

### Maintenance Tasks
```bash
# Clean up old analytics data (if retention not automated)
# This would typically be handled by the microgateway automatically

# Backup current state
./dist/mgw llm list --format=yaml > backups/llms-$(date +%Y%m%d).yaml
./dist/mgw app list --format=yaml > backups/apps-$(date +%Y%m%d).yaml

# Check system resources
./dist/mgw system metrics | grep memory
df -h  # Check disk space for analytics data
```

## Security Best Practices

### Token Management
```bash
# Rotate tokens regularly
OLD_TOKEN="old-token-value"
NEW_TOKEN=$(./dist/mgw token create --app-id=1 --name="New Token" --expires=30d)
./dist/mgw token revoke $OLD_TOKEN

# Use scoped tokens
./dist/mgw token create --app-id=1 --name="Read Only" --scopes="read"
```

### Credential Security
```bash
# Regular credential rotation
./dist/mgw credential create 1 --name="New Production Key" --expires=90d
# Update applications with new credentials
./dist/mgw credential delete 1 <old-credential-id>
```

### Access Control
```bash
# Use IP restrictions for sensitive apps
./dist/mgw app update 1 --allowed-ips="203.0.113.1,203.0.113.2"

# Monitor authentication failures
./dist/mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code >= 400 and .status_code < 500)'
```

## Troubleshooting Guide

### Performance Issues
```bash
# Check cache performance
./dist/mgw system metrics | grep cache

# Monitor database connections
./dist/mgw system metrics | grep db_connections

# Check for slow queries
# Review database logs for slow query patterns
```

### Authentication Issues
```bash
# Verify token validity
./dist/mgw token validate <your-token>

# Check token scopes
./dist/mgw token info <your-token>

# Test authentication flow
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/v1/llms
```

### Budget and Cost Issues
```bash
# Check budget status
./dist/mgw budget usage <app-id>

# Analyze cost patterns
./dist/mgw analytics costs <app-id> --start=$(date -d "30 days ago" -Iseconds)

# Review high-cost requests
./dist/mgw analytics events <app-id> --format=json | \
  jq '.data[] | select(.cost > 1.0) | {cost, tokens: .total_tokens, endpoint}'
```

## Migration Guide

### From Direct LLM Usage
1. **Inventory Current Usage**: Document existing LLM integrations
2. **Create LLM Configurations**: Add all LLM providers to microgateway
3. **Update Application Code**: Change LLM endpoints to microgateway proxy URLs
4. **Implement Authentication**: Add token-based authentication to requests
5. **Monitor Migration**: Use analytics to verify request routing

### From Other API Gateways
1. **Export Configurations**: Extract existing API configurations
2. **Map to Microgateway**: Convert configurations to microgateway format
3. **Set Up Budget Controls**: Implement cost management not available in generic gateways
4. **Enable Analytics**: Configure comprehensive AI-specific monitoring
5. **Update Clients**: Point clients to microgateway endpoints

## Best Practices

### Configuration Management
- Use environment-specific configuration files
- Version control configuration templates (not secrets)
- Use secrets management for sensitive values
- Document configuration changes and rationale

### Monitoring and Alerting
- Set up alerts for budget thresholds (80%, 90%, 100%)
- Monitor API error rates and response times
- Track unusual usage patterns
- Set up health check monitoring

### Security
- Rotate secrets regularly (quarterly recommended)
- Use minimal scopes for tokens
- Monitor authentication failures
- Regular security audits of configurations

### Performance
- Monitor cache hit rates and adjust TTL as needed
- Scale database connections based on load
- Use analytics data to optimize LLM selection
- Regular performance benchmarking

## Support and Resources

### Documentation
- **API Reference**: `API_REFERENCE.md` - Complete API documentation
- **Configuration**: `CONFIGURATION.md` - All configuration options
- **Features**: `FEATURES.md` - Comprehensive feature overview
- **CLI Examples**: `CLI_EXAMPLES.md` - CLI usage examples
- **Build & Deploy**: `BUILD_DEPLOY.md` - Compilation and deployment
- **README**: `README.md` - Quick start and overview

### Getting Help
- **CLI Help**: Use `./dist/mgw <command> --help` for command-specific help
- **Health Checks**: Use `./dist/mgw system health` and `./dist/mgw system ready`
- **Configuration Check**: Use `./dist/mgw system config` to verify setup
- **API Documentation**: Access `/swagger/*any` endpoint for interactive docs

### Troubleshooting Checklist
1. ✅ **Service Health**: `./dist/mgw system health`
2. ✅ **Database Connection**: Check logs for database errors
3. ✅ **Authentication**: Verify tokens with `./dist/mgw token validate`
4. ✅ **LLM Configuration**: Check LLM settings and API keys
5. ✅ **Budget Status**: Verify budget limits aren't exceeded
6. ✅ **Network Connectivity**: Test LLM provider API access

This user guide provides everything needed to successfully deploy, configure, and operate the microgateway in any environment.