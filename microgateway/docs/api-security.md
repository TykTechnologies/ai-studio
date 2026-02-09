# API Security and Input Validation

This document describes the security measures and input validation requirements for the microgateway API endpoints.

## Path Parameter Validation

### Edge ID Format

**Usage:** Edge instance identification in hub-and-spoke deployments

**Endpoints:**
- `GET /api/v1/edges/{edge_id}`
- `POST /api/v1/edges/{edge_id}/reload`
- `DELETE /api/v1/edges/{edge_id}`

**Format Requirements:**
- **Character Set**: Alphanumeric characters, hyphens (`-`), underscores (`_`), dots (`.`)
- **Length Limit**: Maximum 64 characters
- **Pattern**: Must start and end with alphanumeric character
- **Regex**: `^[a-zA-Z0-9][a-zA-Z0-9\-_.]*[a-zA-Z0-9]$`

**Valid Examples:**
```
edge-1
edge-production-us-west-1
tenant-a-edge-01
mgw.prod.region1
gateway_instance_001
```

**Invalid Examples:**
```
-edge-1               # Cannot start with hyphen
edge-1-               # Cannot end with hyphen
edge@1                # Invalid character (@)
edge;rm -rf /         # Shell injection attempt
```

### Namespace Format

**Usage:** Multi-tenant namespace identification

**Endpoints:**
- `POST /api/v1/namespaces/{namespace}/reload`
- `GET /api/v1/namespaces/{namespace}/edges`

**Format Requirements:**
- **Character Set**: Alphanumeric characters, hyphens (`-`), underscores (`_`)
- **Length Limit**: Maximum 64 characters
- **Pattern**: Must start and end with alphanumeric character
- **Special Cases**: `"global"` is accepted as an alias for the global namespace
- **Regex**: `^[a-zA-Z0-9][a-zA-Z0-9\-_]*[a-zA-Z0-9]$`

**Valid Examples:**
```
global                # Special alias
production
tenant-1
dev-environment
staging_cluster
team-alpha
```

**Invalid Examples:**
```
-production           # Cannot start with hyphen
production-           # Cannot end with hyphen
prod.env              # Dots not allowed
tenant#1              # Invalid character (#)
```

### Operation ID Format

**Usage:** Reload operation tracking

**Endpoints:**
- `GET /api/v1/reload-operations/{operation_id}/status`

**Format Requirements:**
- **Character Set**: Alphanumeric characters, hyphens (`-`)
- **Length Limit**: Maximum 64 characters
- **Pattern**: Must start and end with alphanumeric character
- **Regex**: `^[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9]$`

**Valid Examples:**
```
reload-20240101-123456-001
operation-abc123
mgw-reload-1704067200
```

## Security Measures

### Input Validation

All path parameters undergo strict validation:

1. **Length Validation**: Prevents resource exhaustion attacks
2. **Character Set Validation**: Blocks shell metacharacters and injection attempts
3. **Pattern Validation**: Ensures proper format using regex patterns
4. **Boundary Validation**: Start/end character requirements prevent edge cases

### Security Benefits

- **Log Injection Prevention**: Invalid characters that could break log parsing are blocked
- **Command Injection Prevention**: Shell metacharacters are rejected
- **Resource Exhaustion Prevention**: Length limits prevent memory/storage abuse
- **Directory Traversal Prevention**: Path-like constructs are restricted

### Error Responses

Invalid input returns `400 Bad Request` with security-focused error messages:

```json
{
  "errors": [{
    "title": "Bad Request",
    "detail": "🔒 SECURITY: edge_id contains invalid characters. Must be alphanumeric with hyphens, underscores, or dots"
  }]
}
```

## Configuration Security

### gRPC Client Security

**Default Behavior:**
- **TLS Enabled by Default**: `EDGE_TLS_ENABLED=true` (secure-by-default)
- **Explicit Insecure Opt-in**: Requires `EDGE_ALLOW_INSECURE=true` for development
- **Certificate Validation**: `EDGE_SKIP_TLS_VERIFY=false` by default

**Environment Variables:**
```bash
# Secure defaults (production)
EDGE_TLS_ENABLED=true                    # Default: true
EDGE_ALLOW_INSECURE=false               # Default: false
EDGE_SKIP_TLS_VERIFY=false              # Default: false

# Development override (explicit opt-in required)
EDGE_ALLOW_INSECURE=true                # Required for insecure connections
EDGE_TLS_ENABLED=false                  # Only works if ALLOW_INSECURE=true
```

### Plugin Security

**Path Validation:**
- **Allowed Directories**: Plugins must be in approved directories
- **Shell Metacharacter Detection**: Blocks dangerous characters (`;&|$(){}[]<>?*!~\`\\`)
- **Symlink Protection**: Symbolic links are rejected for security
- **File Verification**: Validates file existence and executability

**Default Allowed Directories:**
```
/opt/microgateway/plugins
./plugins
plugins/
```

**Secure Plugin Usage:**
```bash
# Recommended: Use OCI distribution
mgw plugin create --name="rate-limiter" --command="oci://registry.company.com/plugins/rate-limiter:v1.0"

# Local plugins: Must be in allowed directories
mgw plugin create --name="local-plugin" --command="./plugins/my-plugin"

# External gRPC: Use grpc:// scheme
mgw plugin create --name="remote-plugin" --command="grpc://plugin-service:9090"
```

## Database Security

### Error Logging

**Sensitive Data Redaction:**
- **Automatic Detection**: Regex patterns identify tokens, keys, secrets, passwords
- **Safe Logging**: Sensitive values replaced with `***REDACTED***`
- **Comprehensive Coverage**: All database error logging locations secured

**Redacted Patterns:**
- API keys and tokens
- Passwords and secrets
- Authorization headers
- Database connection strings containing credentials

## Best Practices

### Secure Configuration

1. **Use strong identifiers**: Choose descriptive, unique edge IDs and namespaces
2. **Enable TLS**: Always use TLS in production environments
3. **Regular rotation**: Rotate authentication tokens regularly
4. **Monitor validation errors**: Review security validation failures in logs
5. **Principle of least privilege**: Use specific namespaces instead of global when possible

### Example Secure Configuration

**Production Edge Instance:**
```bash
GATEWAY_MODE=edge
EDGE_ID=prod-api-gateway-us-west-1
EDGE_NAMESPACE=production
CONTROL_ENDPOINT=control.internal.company.com:50051
EDGE_TLS_ENABLED=true
EDGE_AUTH_TOKEN=${SECURE_AUTH_TOKEN}
LOG_LEVEL=info
LOG_FORMAT=json
```

This configuration provides:
- ✅ Descriptive, unique edge ID
- ✅ Environment-specific namespace
- ✅ TLS encryption enabled
- ✅ Secure token authentication
- ✅ Production-appropriate logging