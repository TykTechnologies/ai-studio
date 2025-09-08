# Microgateway API Reference

This document provides a comprehensive reference for all microgateway API endpoints, request/response formats, and authentication methods.

## Base Information

- **Base URL**: `http://localhost:8080` (configurable)
- **Content-Type**: `application/json`
- **Authentication**: Bearer token in Authorization header
- **API Version**: v1 (all endpoints prefixed with `/api/v1`)

## Authentication

### Admin Authentication
Most management endpoints require admin authentication:
```
Authorization: Bearer <admin-token>
```

### App Authentication  
Gateway proxy endpoints require app-specific credentials:
```
Authorization: Bearer <app-token>
```

## Health Endpoints

### Health Check
**GET** `/health`

Basic health check endpoint (no authentication required).

**Response:**
```json
{
  "status": "ok",
  "service": "microgateway"
}
```

### Readiness Check
**GET** `/ready`

Readiness check with dependency validation (no authentication required).

**Response:**
```json
{
  "status": "ready", 
  "service": "microgateway"
}
```

## LLM Management

All LLM endpoints require admin authentication.

### List LLMs
**GET** `/api/v1/llms`

**Query Parameters:**
- `page` (int): Page number (default: 1)
- `limit` (int): Items per page (default: 20, max: 100)
- `vendor` (string): Filter by vendor (openai, anthropic, google, vertex, ollama)
- `active` (bool): Filter by active status (default: true)

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "name": "GPT-4 Production",
      "slug": "gpt-4-production",
      "vendor": "openai",
      "endpoint": "",
      "default_model": "gpt-4",
      "max_tokens": 4096,
      "timeout_seconds": 30,
      "retry_count": 3,
      "is_active": true,
      "monthly_budget": 1000.0,
      "rate_limit_rpm": 100,
      "metadata": {},
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 5,
    "total_pages": 1
  }
}
```

### Create LLM
**POST** `/api/v1/llms`

**Request Body:**
```json
{
  "name": "GPT-4 Production",
  "vendor": "openai",
  "endpoint": "",
  "api_key": "sk-...",
  "default_model": "gpt-4",
  "max_tokens": 4096,
  "timeout_seconds": 30,
  "retry_count": 3,
  "is_active": true,
  "monthly_budget": 1000.0,
  "rate_limit_rpm": 100,
  "metadata": {}
}
```

**Required Fields:**
- `name`: LLM display name
- `vendor`: One of: openai, anthropic, google, vertex, ollama  
- `default_model`: Default model identifier
- `api_key`: Required for openai, anthropic
- `endpoint`: Required for ollama

**Response:** (201 Created)
```json
{
  "data": { /* LLM object */ },
  "message": "LLM created successfully"
}
```

### Get LLM
**GET** `/api/v1/llms/{id}`

**Response:**
```json
{
  "data": { /* LLM object */ }
}
```

### Update LLM
**PUT** `/api/v1/llms/{id}`

**Request Body:** (all fields optional)
```json
{
  "name": "Updated LLM Name",
  "endpoint": "https://new-endpoint.com",
  "api_key": "new-api-key",
  "default_model": "gpt-4-turbo",
  "max_tokens": 8192,
  "timeout_seconds": 60,
  "retry_count": 5,
  "is_active": false,
  "monthly_budget": 2000.0,
  "rate_limit_rpm": 200,
  "metadata": {"updated": true}
}
```

### Delete LLM
**DELETE** `/api/v1/llms/{id}`

Soft deletes the LLM (sets deleted_at timestamp).

**Response:**
```json
{
  "message": "LLM deleted successfully"
}
```

### Get LLM Statistics
**GET** `/api/v1/llms/{id}/stats`

**Response:**
```json
{
  "data": {
    "request_count": 1250,
    "total_tokens": 125000,
    "total_cost": 25.50
  }
}
```

## Application Management

### List Applications
**GET** `/api/v1/apps`

**Query Parameters:**
- `page` (int): Page number (default: 1)
- `limit` (int): Items per page (default: 20, max: 100)
- `active` (bool): Filter by active status (default: true)

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "name": "Production App",
      "description": "Main production application",
      "owner_email": "ops@company.com",
      "is_active": true,
      "monthly_budget": 500.0,
      "budget_reset_day": 1,
      "rate_limit_rpm": 1000,
      "allowed_ips": ["203.0.113.1", "203.0.113.2"],
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "pagination": { /* pagination info */ }
}
```

### Create Application
**POST** `/api/v1/apps`

**Request Body:**
```json
{
  "name": "My Application",
  "description": "Application description", 
  "owner_email": "user@company.com",
  "monthly_budget": 100.0,
  "budget_reset_day": 1,
  "rate_limit_rpm": 100,
  "allowed_ips": ["203.0.113.1"],
  "llm_ids": [1, 2, 3]
}
```

**Required Fields:**
- `name`: Application name
- `owner_email`: Valid email address

### Get Application
**GET** `/api/v1/apps/{id}`

### Update Application  
**PUT** `/api/v1/apps/{id}`

**Request Body:** (all fields optional)
```json
{
  "name": "Updated App Name",
  "description": "New description",
  "owner_email": "newowner@company.com",
  "is_active": true,
  "monthly_budget": 200.0,
  "budget_reset_day": 15,
  "rate_limit_rpm": 500,
  "allowed_ips": ["203.0.113.1", "203.0.113.2"]
}
```

### Delete Application
**DELETE** `/api/v1/apps/{id}`

### Get App LLM Associations
**GET** `/api/v1/apps/{id}/llms`

**Response:**
```json
{
  "data": [
    { /* LLM objects associated with this app */ }
  ]
}
```

### Update App LLM Associations
**PUT** `/api/v1/apps/{id}/llms`

**Request Body:**
```json
{
  "llm_ids": [1, 3, 5]
}
```

## Credential Management

### List Credentials
**GET** `/api/v1/apps/{id}/credentials`

**Response:**
```json
{
  "data": [
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
  ]
}
```

### Create Credential
**POST** `/api/v1/apps/{id}/credentials`

**Request Body:**
```json
{
  "name": "Production Credentials",
  "expires_at": "2024-12-31T23:59:59Z"
}
```

**Response:** (201 Created)
```json
{
  "data": {
    "id": 1,
    "app_id": 1,
    "key_id": "key_abc123",
    "secret_hash": "secret_xyz789",  // Plain secret shown only once!
    "name": "Production Credentials",
    "is_active": true,
    "expires_at": "2024-12-31T23:59:59Z",
    "created_at": "2024-01-01T00:00:00Z"
  },
  "message": "Credential created successfully",
  "warning": "Save the secret - it won't be shown again"
}
```

### Delete Credential
**DELETE** `/api/v1/apps/{id}/credentials/{credId}`

## Token Management

### List Tokens
**GET** `/api/v1/tokens`

**Query Parameters:**
- `app_id` (uint): Filter by application ID

### Create Token (Admin)
**POST** `/api/v1/tokens`

**Request Body:**
```json
{
  "app_id": 1,
  "name": "Admin Token", 
  "scopes": ["admin", "read"],
  "expires_in": 86400000000000  // Duration in nanoseconds (24h)
}
```

### Generate Token (Public)
**POST** `/api/v1/auth/token`

Same request format as create token, but available without admin authentication.

### Revoke Token
**DELETE** `/api/v1/tokens/{token}`

### Get Token Info
**GET** `/api/v1/tokens/{token}`

### Validate Token
**POST** `/api/v1/auth/validate`

**Request Body:**
```json
{
  "token": "token-to-validate"
}
```

**Response:**
```json
{
  "valid": true,
  "app_id": 1,
  "scopes": ["admin"],
  "expires_at": "2024-12-31T23:59:59Z"
}
```

## Budget Management

### List Budgets (Admin)
**GET** `/api/v1/budgets`

Returns budget summary for all applications.

### Get Budget Usage
**GET** `/api/v1/budgets/{appId}/usage`

**Query Parameters:**
- `llm_id` (uint): Filter by specific LLM

**Response:**
```json
{
  "data": {
    "app_id": 1,
    "llm_id": 2,
    "monthly_budget": 100.0,
    "current_usage": 45.75,
    "remaining_budget": 54.25,
    "tokens_used": 45750,
    "requests_count": 123,
    "period_start": "2024-01-01T00:00:00Z",
    "period_end": "2024-01-31T23:59:59Z",
    "is_over_budget": false,
    "percentage_used": 45.75
  }
}
```

### Update Budget
**PUT** `/api/v1/budgets/{appId}`

**Request Body:**
```json
{
  "monthly_budget": 200.0,
  "budget_reset_day": 15
}
```

### Get Budget History
**GET** `/api/v1/budgets/{appId}/history`

**Query Parameters:**
- `start_time` (RFC3339): Start time (default: 30 days ago)
- `end_time` (RFC3339): End time (default: now)  
- `llm_id` (uint): Filter by specific LLM

## Analytics

### Get Analytics Events
**GET** `/api/v1/analytics/events`

**Query Parameters:**
- `app_id` (uint): Application ID (required)
- `page` (int): Page number (default: 1)
- `limit` (int): Items per page (default: 50, max: 1000)

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "request_id": "req_abc123",
      "app_id": 1,
      "llm_id": 2,
      "credential_id": 1,
      "endpoint": "/llm/rest/gpt-4/chat/completions",
      "method": "POST",
      "status_code": 200,
      "request_tokens": 50,
      "response_tokens": 100,
      "total_tokens": 150,
      "cost": 0.045,
      "latency_ms": 1250,
      "error_message": "",
      "created_at": "2024-01-01T12:00:00Z"
    }
  ],
  "pagination": { /* pagination info */ }
}
```

### Get Analytics Summary
**GET** `/api/v1/analytics/summary`

**Query Parameters:**
- `app_id` (uint): Application ID (required)
- `start_time` (RFC3339): Start time (default: 7 days ago)
- `end_time` (RFC3339): End time (default: now)

**Response:**
```json
{
  "data": {
    "total_requests": 1500,
    "successful_requests": 1450,
    "failed_requests": 50,
    "total_tokens": 150000,
    "total_cost": 45.75,
    "average_latency": 856.5,
    "requests_per_hour": 62.5
  },
  "time_range": {
    "start_time": "2024-01-01T00:00:00Z",
    "end_time": "2024-01-07T23:59:59Z"
  }
}
```

### Get Cost Analysis
**GET** `/api/v1/analytics/costs`

**Query Parameters:**
- `app_id` (uint): Application ID (required)  
- `start_time` (RFC3339): Start time (default: 30 days ago)
- `end_time` (RFC3339): End time (default: now)

**Response:**
```json
{
  "data": {
    "total_cost": 125.75,
    "average_cost_per_request": 0.084,
    "cost_by_llm": {
      "GPT-4 Production": 85.25,
      "Claude 3.5 Sonnet": 40.50
    }
  },
  "time_range": { /* time range info */ }
}
```

## Gateway Proxy Endpoints

These endpoints proxy requests to configured LLM providers and require app authentication.

### LLM REST API
**ANY** `/llm/rest/{llmSlug}/*path`

Proxies REST API requests to the specified LLM provider.

**Example:**
```bash
POST /llm/rest/gpt-4-production/chat/completions
Content-Type: application/json
Authorization: Bearer <app-token>

{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ]
}
```

### LLM Streaming API
**ANY** `/llm/stream/{llmSlug}/*path`

Proxies streaming requests to the specified LLM provider.

### Tool Operations
**ANY** `/tools/{toolSlug}/*path`

Proxies tool operation requests (placeholder for future implementation).

### Datasource Operations  
**ANY** `/datasource/{dsSlug}/*path`

Proxies datasource requests (placeholder for future implementation).

## Metrics and Monitoring

### Prometheus Metrics
**GET** `/metrics`

Returns Prometheus-format metrics for monitoring.

**Response:** (text/plain)
```
# HELP microgateway_info Microgateway service info
# TYPE microgateway_info gauge
microgateway_info{version="dev"} 1

# HELP microgateway_requests_total Total number of requests
# TYPE microgateway_requests_total counter
microgateway_requests_total 1250

# HELP microgateway_build_info Build information
# TYPE microgateway_build_info gauge
microgateway_build_info{version="dev",build_hash="abc123"} 1
```

### Swagger Documentation
**GET** `/swagger/*any`

Returns API documentation in Swagger format.

## Error Responses

All endpoints return standardized error responses:

### Client Errors (4xx)
```json
{
  "error": "Invalid request format",
  "message": "name field is required"
}
```

### Server Errors (5xx)
```json
{
  "error": "Failed to create LLM", 
  "message": "database connection failed"
}
```

### Common Error Codes
- **400 Bad Request**: Invalid request format or missing required fields
- **401 Unauthorized**: Invalid or missing authentication token
- **403 Forbidden**: Insufficient permissions for the operation
- **404 Not Found**: Requested resource does not exist
- **409 Conflict**: Resource already exists (e.g., duplicate LLM name)
- **500 Internal Server Error**: Server-side error occurred

## Authentication Flow

### 1. Create Application
```bash
POST /api/v1/apps
{
  "name": "My App",
  "owner_email": "user@company.com"
}
```

### 2. Create Credentials
```bash
POST /api/v1/apps/1/credentials
{
  "name": "Production Key"
}
# Returns key_id and secret
```

### 3. Generate App Token  
```bash
POST /api/v1/auth/token
{
  "app_id": 1,
  "name": "App Token",
  "scopes": ["api"]
}
# Returns token for gateway requests
```

### 4. Use Gateway
```bash
POST /llm/rest/gpt-4/chat/completions
Authorization: Bearer <app-token>
{
  "model": "gpt-4",
  "messages": [{"role": "user", "content": "Hello"}]
}
```

## Rate Limiting

### App-Level Rate Limiting
- Configured via `rate_limit_rpm` in app configuration
- Applied across all LLM requests for the application
- Returns 429 status when exceeded

### LLM-Level Rate Limiting
- Configured via `rate_limit_rpm` in LLM configuration
- Applied to specific LLM provider requests
- Takes precedence over app-level limits

## Budget Enforcement

### Pre-Request Budget Check
1. Budget checked before each LLM request
2. Request blocked if estimated cost exceeds remaining budget
3. Returns 402 Payment Required if over budget

### Real-Time Usage Tracking
1. Token usage and costs recorded for each request
2. Budget status updated in real-time
3. Monthly reset based on `budget_reset_day`

## Webhook Integration (Future)

Placeholder for future webhook endpoints:
- Budget threshold notifications
- Usage alerts
- System health alerts

## SDK Support (Future)

Planned SDK support for:
- Python SDK
- Node.js SDK  
- Go SDK
- REST client libraries

## API Versioning

- Current version: v1
- Versioned via URL path: `/api/v1/`
- Backwards compatibility maintained within major versions
- Breaking changes require new major version