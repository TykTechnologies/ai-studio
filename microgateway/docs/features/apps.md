# Application Management

The microgateway provides multi-tenant application management, allowing isolated environments with separate credentials, budgets, and access controls.

## Overview

Application management features:
- **Multi-Tenancy**: Complete isolation between applications
- **LLM Access Control**: Flexible LLM association per application
- **Independent Budgets**: Separate budget management per application
- **Credential Management**: Secure key generation and rotation
- **Rate Limiting**: Per-application request rate controls
- **Usage Isolation**: Separate analytics and billing per application

## Application Concepts

### What is an Application?
An application represents a logical grouping of:
- **LLM Access Rights**: Which LLM providers the app can use
- **Budget Allocation**: Monthly spending limits
- **Credentials**: Authentication keys for the app
- **Rate Limits**: Request frequency controls
- **Usage Analytics**: Isolated usage tracking

### Application Isolation
Each application has:
- Separate authentication tokens
- Independent budget tracking
- Isolated analytics data
- Configurable access controls
- Individual rate limiting

## Creating Applications

### Basic Application
```bash
# Create basic application
mgw app create \
  --name="My AI App" \
  --email=developer@company.com \
  --budget=100.0
```

### Application with Full Configuration
```bash
# Create application with all options
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

### Required vs Optional Parameters
```bash
# Required parameters
--name="Application Name"     # Human-readable name
--email=user@company.com     # Owner email address

# Optional parameters
--description="Description"   # Application description
--budget=1000.0              # Monthly budget limit (0 = unlimited)
--reset-day=1                # Budget reset day (1-28)
--rate-limit=100             # Requests per minute (0 = unlimited)
--allowed-ips="ip1,ip2"      # Comma-separated IP whitelist
--llm-ids="1,2,3"            # Comma-separated LLM IDs
```

## Managing Applications

### List Applications
```bash
# List all applications
mgw app list

# Filter by active status
mgw app list --active=true

# Paginated results
mgw app list --page=2 --limit=10

# JSON output
mgw app list --format=json
```

### Get Application Details
```bash
# Get specific application
mgw app get 1

# JSON output for scripting
mgw app get 1 --format=json
```

### Update Applications
```bash
# Update budget
mgw app update 1 --budget=2000.0

# Update multiple settings
mgw app update 1 \
  --budget=1500.0 \
  --description="Updated description" \
  --rate-limit=500

# Update IP whitelist
mgw app update 1 --allowed-ips="203.0.113.1,203.0.113.5"
```

### Delete Applications
```bash
# Soft delete application
mgw app delete 1

# Application becomes inactive but data is preserved
```

## LLM Associations

### Managing LLM Access
```bash
# View current LLM associations
mgw app llms 1

# Associate with specific LLMs
mgw app llms 1 --set="1,2,3"

# Add LLMs to existing associations
mgw app llms 1 --add="4,5"

# Remove LLMs from associations
mgw app llms 1 --remove="3"
```

### Access Control
Applications can only access associated LLMs:
- Requests to non-associated LLMs are rejected
- Association changes take effect immediately
- No restart required for access control updates

## Credential Management

### Application Credentials
Each application uses key-secret pairs for authentication:

```bash
# Create credentials for an application
mgw credential create 1 --name="Production Key"

# Create credential with expiration
mgw credential create 1 \
  --name="Temporary Key" \
  --expires=2024-12-31T23:59:59Z

# List credentials
mgw credential list 1

# Delete credential
mgw credential delete 1 2
```

### Credential Structure
```json
{
  "id": 1,
  "app_id": 1,
  "key_id": "key_abc123",
  "name": "Production Key",
  "is_active": true,
  "expires_at": null,
  "last_used_at": "2024-01-01T12:00:00Z",
  "created_at": "2024-01-01T00:00:00Z"
}
```

## Authentication Flow

### Token Generation
Applications use credentials to generate API tokens:

```bash
# Generate token for application
mgw token create \
  --app-id=1 \
  --name="Application Token" \
  --scopes="api" \
  --expires=720h
```

### Using Tokens
```bash
# Use token for LLM requests
curl -X POST http://localhost:8080/llm/rest/gpt-4/chat/completions \
  -H "Authorization: Bearer $APP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}'
```

## Application Configuration

### Budget Settings
```bash
# Set monthly budget
--budget=1000.0              # $1000 monthly limit

# Unlimited budget
--budget=0                   # No budget enforcement

# Budget reset day
--reset-day=15               # Reset on 15th of each month (1-28)
```

### Rate Limiting
```bash
# Set rate limit
--rate-limit=100             # 100 requests per minute

# Unlimited rate
--rate-limit=0               # No rate limiting

# Rate limiting is enforced per application across all LLMs
```

### IP Whitelisting
```bash
# Single IP
--allowed-ips="203.0.113.1"

# Multiple IPs
--allowed-ips="203.0.113.1,203.0.113.2,203.0.113.3"

# IP ranges (CIDR notation)
--allowed-ips="203.0.113.0/24"

# Disable IP restrictions (default)
# Omit --allowed-ips parameter
```

## Application Scenarios

### Development Environment
```bash
# Development application
mgw app create \
  --name="Development Team" \
  --email=dev@company.com \
  --budget=200.0 \
  --rate-limit=50 \
  --llm-ids="3"  # Local Ollama model
```

### Production Environment
```bash
# Production application
mgw app create \
  --name="Production API" \
  --email=ops@company.com \
  --budget=10000.0 \
  --rate-limit=1000 \
  --allowed-ips="prod-server-1,prod-server-2" \
  --llm-ids="1,2"  # OpenAI and Anthropic
```

### Customer Application (SaaS)
```bash
# Customer-specific application
mgw app create \
  --name="Customer ABC Corp" \
  --email=abc-corp@customer.com \
  --budget=500.0 \
  --rate-limit=100 \
  --allowed-ips="customer-network" \
  --llm-ids="1,2,3"
```

### Testing Environment
```bash
# Testing application
mgw app create \
  --name="QA Testing" \
  --email=qa@company.com \
  --budget=50.0 \
  --rate-limit=25 \
  --llm-ids="3"  # Local models for testing
```

## Application Monitoring

### Usage Analytics
```bash
# Monitor application usage
mgw analytics summary 1

# View cost breakdown
mgw analytics costs 1

# Check budget status
mgw budget usage 1
```

### Performance Monitoring
```bash
# Monitor request patterns
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.llm_id) | map({llm_id: .[0].llm_id, count: length})'

# Track error rates
mgw analytics events 1 --format=json | \
  jq '.data | map(select(.status_code >= 400)) | length'

# Monitor latency
mgw analytics summary 1 --format=json | \
  jq '.data.average_latency'
```

## Multi-Tenant Deployment

### Tenant Isolation
```bash
# Each tenant gets their own application
mgw app create --name="Tenant A" --email=tenant-a@company.com --budget=1000.0
mgw app create --name="Tenant B" --email=tenant-b@company.com --budget=2000.0
mgw app create --name="Tenant C" --email=tenant-c@company.com --budget=500.0
```

### Tenant-Specific LLMs
```bash
# Premium tenant with access to all LLMs
mgw app llms 1 --set="1,2,3,4"

# Standard tenant with limited access
mgw app llms 2 --set="1,3"

# Basic tenant with minimal access
mgw app llms 3 --set="3"  # Local models only
```

### Tenant Management
```bash
# Monitor all tenants
mgw app list --format=json | \
  jq '.data[] | {id, name, budget: .monthly_budget, active: .is_active}'

# Tenant usage summary
for app_id in $(mgw app list --format=json | jq -r '.data[].id'); do
  echo "App $app_id:"
  mgw budget usage $app_id
  echo
done
```

## API Integration

### Application API
```bash
# Create application via API
curl -X POST http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "API Created App",
    "description": "Created via API",
    "owner_email": "api@company.com",
    "monthly_budget": 500.0,
    "rate_limit_rpm": 100,
    "llm_ids": [1, 2]
  }'

# Update application
curl -X PUT http://localhost:8080/api/v1/apps/1 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"monthly_budget": 1000.0}'
```

### Credential API
```bash
# Create credentials via API
curl -X POST http://localhost:8080/api/v1/apps/1/credentials \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "API Generated Key"}'
```

## Best Practices

### Application Design
- Use separate applications for different environments (dev, staging, prod)
- Create applications per team or project for cost attribution
- Use descriptive names and maintain owner email addresses
- Set appropriate budgets based on expected usage

### Security
- Rotate credentials regularly
- Use IP whitelisting for sensitive applications
- Monitor authentication failures and unusual access patterns
- Implement least-privilege access with minimal LLM associations

### Cost Management
- Set conservative initial budgets and adjust based on usage
- Monitor budget utilization regularly
- Use application-level budgets for overall cost control
- Implement approval workflows for budget increases

### Monitoring
- Track application usage trends and patterns
- Monitor error rates and performance metrics
- Set up alerts for budget thresholds and unusual activity
- Regular review of application configurations

## Troubleshooting

### Application Access Issues
```bash
# Check application status
mgw app get 1

# Verify LLM associations
mgw app llms 1

# Check credential status
mgw credential list 1

# Test authentication
mgw token validate $APP_TOKEN
```

### Budget Issues
```bash
# Check budget status
mgw budget usage 1

# Verify budget configuration
mgw app get 1 | grep budget

# Review recent costs
mgw analytics costs 1
```

### Rate Limiting Issues
```bash
# Check rate limit configuration
mgw app get 1 | grep rate_limit

# Monitor request rates
mgw analytics summary 1 --format=json | \
  jq '.data.requests_per_hour'

# Review rate limit errors
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code == 429)'
```

---

Application management provides the foundation for multi-tenant AI/LLM access control. For authentication details, see [API Keys](api-keys.md). For cost control, see [Budgets](budgets.md).
