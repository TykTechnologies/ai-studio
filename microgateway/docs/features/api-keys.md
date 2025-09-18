# API Keys (Tokens)

The microgateway uses a two-tier authentication system with credentials and tokens to provide secure, granular access control for AI/LLM resources.

## Overview

Authentication features:
- **Two-Tier Authentication**: Credentials generate tokens for secure access
- **Scoped Tokens**: Granular permission control with scope validation
- **Token Expiration**: Configurable token lifetimes
- **Secure Storage**: AES-256 encryption for sensitive data
- **Token Rotation**: Seamless credential and token rotation
- **Audit Trail**: Complete authentication event logging

## Authentication Architecture

### Two Authentication Systems
The microgateway uses separate authentication for different purposes:

1. **Admin Authentication**: For management API endpoints (`/api/v1/*`)
2. **Application Authentication**: For gateway proxy endpoints (`/llm/*`, `/tools/*`, `/datasource/*`)

### Authentication Flow
```
Application Credentials → API Token → LLM Request
     (key_id + secret)   →  (Bearer token)  →  (Authorized request)
```

## Credential Management

### Creating Credentials
```bash
# Create credentials for an application
mgw credential create 1 --name="Production Key"

# Create credential with expiration
mgw credential create 1 \
  --name="Temporary Key" \
  --expires=2024-12-31T23:59:59Z

# List application credentials
mgw credential list 1
```

### Credential Response
```json
{
  "data": {
    "id": 1,
    "app_id": 1,
    "key_id": "key_abc123",
    "secret_hash": "secret_xyz789",  // Shown only once!
    "name": "Production Key",
    "is_active": true,
    "expires_at": null,
    "created_at": "2024-01-01T00:00:00Z"
  },
  "message": "Credential created successfully",
  "warning": "Save the secret - it won't be shown again"
}
```

### Managing Credentials
```bash
# List credentials
mgw credential list 1

# Delete credential
mgw credential delete 1 2

# Credential rotation
# 1. Create new credential
mgw credential create 1 --name="Rotated Key"
# 2. Update application to use new credential
# 3. Delete old credential
mgw credential delete 1 1
```

## Token Management

### Token Types
The microgateway supports different token types:

#### Admin Tokens
```bash
# Create admin token (for management API)
mgw token create \
  --app-id=1 \
  --name="Admin Token" \
  --scopes="admin" \
  --expires=24h
```

#### API Tokens
```bash
# Create API token (for gateway requests)
mgw token create \
  --app-id=1 \
  --name="Application Token" \
  --scopes="api" \
  --expires=720h
```

#### Read-Only Tokens
```bash
# Create read-only token
mgw token create \
  --app-id=1 \
  --name="Read Only Token" \
  --scopes="read" \
  --expires=168h
```

### Token Scopes
Available token scopes:
- **admin**: Full administrative access to management API
- **api**: Access to gateway proxy endpoints
- **read**: Read-only access to resources
- **write**: Write access to resources (future)

### Token Operations
```bash
# List tokens for application
mgw token list --app-id=1

# Get token information
mgw token info abc123def456

# Validate token
mgw token validate abc123def456

# Revoke token
mgw token revoke abc123def456
```

## Using Tokens

### Gateway Requests
```bash
# Use API token for LLM requests
curl -X POST http://localhost:8080/llm/rest/gpt-4/chat/completions \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### Management API
```bash
# Use admin token for management operations
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/llms"
```

## Token Configuration

### Token Generation Settings
```bash
# JWT configuration
JWT_SECRET=your-jwt-secret-key    # JWT signing secret (32+ chars)
TOKEN_LENGTH=32                   # Generated token length
SESSION_TIMEOUT=24h               # Default token lifetime

# Encryption settings
ENCRYPTION_KEY=your-32-char-key   # AES encryption key (exactly 32 chars)
BCRYPT_COST=10                    # bcrypt hashing cost
```

### Security Settings
```bash
# Token validation
ENABLE_TOKEN_VALIDATION=true
TOKEN_CACHE_ENABLED=true
TOKEN_CACHE_TTL=1h

# Rate limiting
ENABLE_RATE_LIMITING=true
DEFAULT_RATE_LIMIT=100           # Requests per minute
```

## Admin Token Bootstrap

### Initial Setup
```bash
# Generate admin token on first startup
./dist/microgateway -create-admin-token \
  -admin-name="Production Admin" \
  -admin-expires="720h"

# Save the generated token
export MGW_TOKEN="generated-admin-token"
```

### Admin Token Management
```bash
# Create additional admin tokens
./dist/microgateway -create-admin-token \
  -admin-name="Ops Team" \
  -admin-expires="168h"

# Admin tokens are stored in app_id=1 (system app)
mgw token list --app-id=1
```

## Token Security

### Best Practices
- **Rotate tokens regularly**: Implement token rotation schedules
- **Use minimal scopes**: Grant only necessary permissions
- **Set expiration times**: Avoid long-lived tokens
- **Monitor token usage**: Track authentication patterns
- **Secure storage**: Store tokens securely in applications

### Token Rotation
```bash
#!/bin/bash
# token-rotation.sh

APP_ID=1
OLD_TOKEN="current-app-token"

# Create new token
NEW_TOKEN=$(mgw token create \
  --app-id=$APP_ID \
  --name="Rotated Token $(date +%Y%m%d)" \
  --scopes="api" \
  --expires=720h \
  --format=json | jq -r '.data.token')

# Update application configuration with new token
# (application-specific process)

# Revoke old token
mgw token revoke $OLD_TOKEN

echo "Token rotated successfully: $NEW_TOKEN"
```

## API Reference

### Token Creation API
```bash
# Create token via API
curl -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": 1,
    "name": "API Generated Token",
    "scopes": ["api"],
    "expires_in": 86400000000000
  }'
```

### Token Validation API
```bash
# Validate token
curl -X POST http://localhost:8080/api/v1/auth/validate \
  -H "Content-Type: application/json" \
  -d '{"token": "token-to-validate"}'

# Response:
{
  "valid": true,
  "app_id": 1,
  "scopes": ["api"],
  "expires_at": "2024-12-31T23:59:59Z"
}
```

### Token Information API
```bash
# Get token details
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/tokens/abc123def456"
```

## Token Monitoring

### Usage Tracking
```bash
# Monitor token usage
mgw analytics events 1 --format=json | \
  jq '.data[] | .credential_id' | sort | uniq -c

# Find unused tokens
mgw token list --app-id=1 --format=json | \
  jq '.data[] | select(.last_used_at == null)'

# Monitor authentication failures
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code == 401)'
```

### Token Lifecycle
```bash
# Track token age
mgw token list --app-id=1 --format=json | \
  jq '.data[] | {name, created_at, expires_at, last_used_at}'

# Find expiring tokens
mgw token list --app-id=1 --format=json | \
  jq '.data[] | select(.expires_at and (.expires_at | fromdateiso8601) < (now + 86400*7))'
```

## Authentication Scenarios

### Development Setup
```bash
# Development tokens (shorter expiration)
mgw token create \
  --app-id=1 \
  --name="Dev Token" \
  --scopes="api" \
  --expires=24h
```

### Production Setup
```bash
# Production tokens (longer expiration)
mgw token create \
  --app-id=1 \
  --name="Production Token" \
  --scopes="api" \
  --expires=720h  # 30 days
```

### CI/CD Setup
```bash
# CI/CD tokens (specific expiration)
mgw token create \
  --app-id=1 \
  --name="CI Pipeline Token" \
  --scopes="api" \
  --expires=168h  # 7 days
```

### Read-Only Access
```bash
# Monitoring tokens (read-only)
mgw token create \
  --app-id=1 \
  --name="Monitoring Token" \
  --scopes="read" \
  --expires=8760h  # 1 year
```

## Integration Examples

### Application Integration
```javascript
// Node.js example
const axios = require('axios');

const client = axios.create({
  baseURL: 'http://localhost:8080',
  headers: {
    'Authorization': `Bearer ${process.env.MGW_TOKEN}`,
    'Content-Type': 'application/json'
  }
});

// Make LLM request
const response = await client.post('/llm/rest/gpt-4/chat/completions', {
  model: 'gpt-4',
  messages: [{role: 'user', content: 'Hello'}]
});
```

### Python Integration
```python
# Python example
import requests
import os

headers = {
    'Authorization': f'Bearer {os.environ["MGW_TOKEN"]}',
    'Content-Type': 'application/json'
}

response = requests.post(
    'http://localhost:8080/llm/rest/gpt-4/chat/completions',
    headers=headers,
    json={
        'model': 'gpt-4',
        'messages': [{'role': 'user', 'content': 'Hello'}]
    }
)
```

## Troubleshooting

### Authentication Failures
```bash
# Check token validity
mgw token validate $TOKEN

# Check token scopes
mgw token info $TOKEN

# Review authentication errors
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code == 401 or .status_code == 403)'
```

### Token Issues
```bash
# Check token expiration
mgw token list --app-id=1 --format=json | \
  jq '.data[] | select(.expires_at and (.expires_at | fromdateiso8601) < now)'

# Find inactive tokens
mgw token list --app-id=1 --format=json | \
  jq '.data[] | select(.is_active == false)'

# Test token manually
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/llms"
```

### Credential Issues
```bash
# Check credential status
mgw credential list 1

# Test credential creation
mgw credential create 1 --name="Test Credential"

# Review credential usage
mgw credential list 1 --format=json | \
  jq '.data[] | {name, last_used_at, is_active}'
```

---

API keys and tokens provide secure access control for AI/LLM resources. For application management, see [Apps](apps.md). For usage monitoring, see [Analytics](analytics.md).
