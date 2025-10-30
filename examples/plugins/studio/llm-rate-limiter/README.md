# LLM Rate Limiter Plugin

A comprehensive rate limiting plugin for Tyk AI Studio (Midsommar) that provides policy-based rate limiting for LLM requests with support for:

- **Tokens Per Minute (TPM)** - Limit the number of tokens processed per minute
- **Requests Per Minute (RPM)** - Limit the number of requests per minute
- **Concurrent Requests** - Limit simultaneous requests to prevent resource exhaustion

## Features

### Policy-Based Configuration
- Define reusable rate limit policies (e.g., Bronze, Silver, Gold tiers)
- Configure limits per model with wildcard support for defaults
- Update policies centrally without modifying apps

### Flexible Assignment
- Assign policies to Apps via intuitive UI
- Enable/disable rate limiting per app without removing the policy
- Per-app overrides for specific models (future enhancement)

### Real-Time Enforcement
- Gateway plugin enforces limits in real-time during LLM requests
- Tracks usage per minute windows with automatic reset
- Returns detailed 429 responses when limits are exceeded

## Architecture

### Data Flow

```
┌─────────────────┐
│  Studio UI      │
│  - Policies     │  1. Define rate limit policies
│  - Assignments  │  2. Assign policies to Apps
└────────┬────────┘
         │
         ├──► Plugin K/V Store (Policies)
         │
         └──► App Metadata (Policy Assignment)
                    │
                    │
         ┌──────────▼────────┐
         │  Microgateway     │
         │  - Read metadata  │  3. Read App config with policy
         │  - Check limits   │  4. Enforce rate limits
         │  - Track usage    │  5. Track current usage
         └───────────────────┘
```

### Storage Strategy

**Policy Definitions**: Stored in Plugin K/V store
- Key: `policy:{policy_name}`
- Contains model-specific limits (TPM, RPM, concurrent)

**App Policy Assignments**: Stored in App.Metadata field
- Key: `rate_limiter`
- Contains policy name, enabled flag, and optional overrides

**Gateway Runtime State**: Tracked in Gateway Plugin K/V
- Key: `rate:{app_id}:{model}:{minute}`
- Contains current usage counters for TPM, RPM, concurrent requests

## Installation

This plugin works in **two contexts**:
1. **AI Studio** - For policy management and app configuration via UI
2. **Microgateway** - For enforcing rate limits on LLM traffic

### 1. Build the Plugin

```bash
cd examples/plugins/studio/llm-rate-limiter/server
go build -o llm-rate-limiter
```

### 2. Install in AI Studio (UI Management)

```bash
# Copy plugin binary to AI Studio plugins directory
cp llm-rate-limiter ~/.tyk-ai-studio/plugins/

# Restart AI Studio
systemctl restart tyk-ai-studio  # or your preferred method
```

### 3. Enable Plugin in AI Studio

Navigate to AI Studio Admin UI → Plugins → Enable "LLM Rate Limiter"

This enables the management UI for creating policies and assigning them to apps.

### 4. Install in Microgateway (Rate Limit Enforcement)

```bash
# Copy the same binary to microgateway plugins directory
cp llm-rate-limiter ~/.tyk-microgateway/plugins/

# Update microgateway configuration to load the plugin
# Add to your microgateway.yaml or via CLI:
./mgw plugin add llm-rate-limiter

# Restart microgateway
systemctl restart tyk-microgateway  # or your preferred method
```

**Important**: The same binary works in both environments. The plugin automatically detects which context it's running in and serves the appropriate interface.

## Usage

### Step 1: Create Rate Limit Policies

1. Navigate to **Rate Limiting** → **Rate Limit Policies** in the sidebar
2. Click **Add Policy**
3. Configure the policy:

**Example Bronze Tier:**
```json
{
  "name": "bronze",
  "description": "Basic tier for testing and development",
  "models": {
    "*": {
      "tpm": 10000,
      "rpm": 10,
      "concurrent": 2
    },
    "gpt-4": {
      "tpm": 5000,
      "rpm": 5,
      "concurrent": 1
    }
  }
}
```

**Example Silver Tier:**
```json
{
  "name": "silver",
  "description": "Production tier for standard users",
  "models": {
    "*": {
      "tpm": 50000,
      "rpm": 50,
      "concurrent": 5
    },
    "gpt-4": {
      "tpm": 25000,
      "rpm": 25,
      "concurrent": 3
    }
  }
}
```

**Example Gold Tier:**
```json
{
  "name": "gold",
  "description": "Premium tier for power users",
  "models": {
    "*": {
      "tpm": 200000,
      "rpm": 200,
      "concurrent": 10
    },
    "gpt-4": {
      "tpm": 100000,
      "rpm": 100,
      "concurrent": 5
    }
  }
}
```

### Step 2: Assign Policies to Apps

1. Navigate to **Rate Limiting** → **App Assignments** in the sidebar
2. Find the app you want to configure
3. Click **Assign** (or **Edit** if already assigned)
4. Select a policy from the dropdown
5. Toggle "Enable rate limiting" on/off
6. Click **Assign Policy**

### Step 3: Gateway Enforcement

Once policies are assigned to apps, the microgateway automatically enforces rate limits on incoming requests.

**How it works:**
1. Request arrives at microgateway with authentication credentials
2. Authentication identifies the App ID
3. Rate limiter plugin checks:
   - Does this app have a rate limit policy assigned?
   - Is rate limiting enabled for this app?
   - What are the limits for the requested model?
4. Plugin fetches current usage from K/V store
5. If within limits:
   - Request proceeds to LLM
   - Usage counters incremented
6. If limit exceeded:
   - Request blocked with 429 response
   - Client receives detailed error message

**Monitoring Enforcement:**

Watch gateway logs for rate limit events:
```bash
# View rate limit decisions
tail -f /var/log/microgateway.log | grep "llm-rate-limiter"

# Success:
✅ llm-rate-limiter: Rate limit check passed for app 1, model gpt-4

# Blocked:
🚫 llm-rate-limiter: Rate limit exceeded for app 1, model gpt-4: rpm (usage: 11, limit: 10)
```

### Step 4: Test Rate Limiting

Make LLM requests through the gateway and observe rate limiting:

```bash
# Example request
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Successful Response (within limits):**
```json
{
  "id": "chatcmpl-...",
  "model": "gpt-4",
  "choices": [...]
}
```

**Rate Limit Exceeded Response (429):**
```json
{
  "error": "Rate limit exceeded",
  "limit_type": "rpm",
  "limit_value": 10,
  "current_usage": 11,
  "reset_at": "2025-10-30T15:24:00Z",
  "policy": "bronze",
  "app_id": 1,
  "model": "gpt-4"
}
```

**Response Headers:**
```
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
X-RateLimit-Policy: bronze
X-RateLimit-Type: rpm
X-RateLimit-Reset: 2025-10-30T15:24:00Z
```

## Configuration Details

### Policy Structure

```go
type RateLimitPolicy struct {
    Name        string                  // Unique policy identifier
    Description string                  // Human-readable description
    Models      map[string]ModelLimits  // Model-specific limits
    CreatedAt   time.Time               // Creation timestamp
    UpdatedAt   time.Time               // Last update timestamp
}

type ModelLimits struct {
    TPM        int  // Tokens per minute
    RPM        int  // Requests per minute
    Concurrent int  // Max concurrent requests
}
```

### Model Matching

- Use `*` as a wildcard for default limits applied to all models
- Specific model names override the wildcard
- Model names are case-sensitive and should match LLM slugs exactly

### App Metadata Structure

When a policy is assigned to an app, the following is stored in `App.Metadata`:

```json
{
  "rate_limiter": {
    "policy_name": "bronze",
    "enabled": true,
    "overrides": {}
  }
}
```

## Rate Limiting Algorithm

### Minute Windows
- Usage is tracked in per-minute windows (e.g., `2025-10-30T15:23`)
- Windows automatically reset at the start of each new minute
- No sliding windows - hard resets provide predictable behavior

### Token Counting
- Tokens are extracted from LLM response metadata when available
- If not available, requests are counted but TPM may not be enforced accurately
- Consider implementing token estimation for requests

### Concurrent Request Tracking
- Incremented at request start (pre-request hook)
- Decremented at request completion (post-request hook)
- Prevents burst traffic and resource exhaustion

### Caching
- Gateway caches app configuration for 5 minutes
- Reduces service API calls for high-traffic apps
- Cache is invalidated on explicit policy updates

## Troubleshooting

### Policy Not Enforced

**Issue**: Rate limits are not being applied

**Solutions**:
1. Check that rate limiting is enabled for the app in **App Assignments**
2. Verify the gateway plugin is loaded (check gateway logs)
3. Confirm the app has the correct policy assigned
4. Check for errors in gateway logs: `grep "rate-limiter" /var/log/microgateway.log`

### Limits Too Strict/Lenient

**Issue**: Rate limits don't match expected behavior

**Solutions**:
1. Review the policy configuration in **Rate Limit Policies**
2. Check if model-specific limits override wildcard defaults
3. Verify minute windows are resetting correctly
4. Increase cache TTL if updates aren't reflected quickly

### High Memory Usage

**Issue**: Gateway memory usage increases over time

**Solutions**:
1. Implement TTL for rate state K/V entries
2. Clean up expired minute window data
3. Limit the number of tracked apps/models

## Future Enhancements

- [ ] Per-app model overrides in UI
- [ ] Sliding window rate limiting
- [ ] Burst allowance configuration
- [ ] Rate limit analytics and reporting
- [ ] Multi-gateway coordination (Redis backend)
- [ ] Automatic token estimation for requests
- [ ] Rate limit exemption for specific users
- [ ] Webhook notifications on limit exceeded

## API Reference

### RPC Methods

The plugin exposes the following RPC methods for UI integration:

#### `listPolicies`
Lists all configured rate limit policies.

**Request**: `{}`

**Response**:
```json
{
  "policies": [/* array of RateLimitPolicy */],
  "count": 3
}
```

#### `getPolicy`
Retrieves a specific policy by name.

**Request**: `{ "name": "bronze" }`

**Response**: `RateLimitPolicy object`

#### `createPolicy`
Creates a new rate limit policy.

**Request**:
```json
{
  "name": "custom",
  "description": "Custom tier",
  "models": {
    "*": { "tpm": 10000, "rpm": 10, "concurrent": 2 }
  }
}
```

**Response**:
```json
{
  "success": true,
  "policy": {/* RateLimitPolicy object */},
  "message": "Policy created successfully"
}
```

#### `updatePolicy`
Updates an existing policy.

**Request**: Same as `createPolicy` (name cannot be changed)

**Response**: Same as `createPolicy`

#### `deletePolicy`
Deletes a policy.

**Request**: `{ "name": "custom" }`

**Response**:
```json
{
  "success": true,
  "message": "Policy deleted successfully"
}
```

#### `listAppsWithPolicies`
Lists all apps with their assigned rate limit policies.

**Request**: `{}`

**Response**:
```json
{
  "apps": [/* array of AppWithPolicy */],
  "count": 5
}
```

#### `assignPolicy`
Assigns a policy to an app.

**Request**:
```json
{
  "app_id": 1,
  "policy_name": "bronze",
  "enabled": true,
  "overrides": {}
}
```

**Response**:
```json
{
  "success": true,
  "message": "Policy assigned successfully"
}
```

#### `removePolicy`
Removes a policy assignment from an app.

**Request**: `{ "app_id": 1 }`

**Response**:
```json
{
  "success": true,
  "message": "Policy removed successfully"
}
```

## License

This plugin is part of the Tyk AI Studio project and follows the same license.

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/TykTechnologies/midsommar/issues
- Documentation: https://docs.tyk.io/ai-studio
- Community: https://community.tyk.io
