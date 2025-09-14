# Security Configuration

The microgateway provides comprehensive security features including encryption, authentication, access controls, and audit logging.

## Overview

Security configuration features:
- **Encryption**: AES-256 encryption for sensitive data storage
- **Authentication**: JWT-based token authentication with scoping
- **Access Control**: IP whitelisting and rate limiting
- **TLS Support**: HTTPS and gRPC TLS encryption
- **Audit Logging**: Complete audit trail for security events
- **Secret Management**: Secure handling of API keys and credentials

## Encryption Configuration

### Data Encryption
```bash
# AES encryption for sensitive data
ENCRYPTION_KEY=your-32-character-encryption-key!!

# Key requirements:
# - Exactly 32 characters (256-bit AES)
# - Cryptographically secure random generation
# - Stored securely (environment variables, secret management)

# Generate secure encryption key
ENCRYPTION_KEY=$(openssl rand -hex 16)  # 32 hex chars = 16 bytes
```

### JWT Configuration
```bash
# JWT token signing
JWT_SECRET=your-production-jwt-secret-key-here

# Key requirements:
# - Minimum 32 characters for security
# - Cryptographically secure random generation
# - Different from encryption key

# Generate secure JWT secret
JWT_SECRET=$(openssl rand -hex 32)  # 64 hex chars
```

### Password Hashing
```bash
# bcrypt configuration for credential hashing
BCRYPT_COST=10              # Default cost (good balance)
BCRYPT_COST=12              # Higher security (slower)
BCRYPT_COST=8               # Lower security (faster)

# Higher cost = more secure but slower authentication
# Recommended: 10-12 for production
```

## TLS Configuration

### HTTPS Configuration
```bash
# Enable HTTPS
TLS_ENABLED=true
TLS_CERT_PATH=/etc/ssl/certs/microgateway.crt
TLS_KEY_PATH=/etc/ssl/private/microgateway.key
TLS_MIN_VERSION=1.2

# Optional: Intermediate certificates
TLS_CA_CERT_PATH=/etc/ssl/certs/ca-chain.crt
```

### Generate TLS Certificates
```bash
# Self-signed certificate (development)
openssl req -x509 -newkey rsa:4096 \
  -keyout microgateway.key -out microgateway.crt \
  -days 365 -nodes \
  -subj "/CN=microgateway.local"

# Production certificate (Let's Encrypt)
certbot certonly --standalone \
  -d microgateway.company.com \
  --cert-path /etc/ssl/certs/microgateway.crt \
  --key-path /etc/ssl/private/microgateway.key
```

### gRPC TLS (Hub-and-Spoke)
```bash
# Control instance TLS
GRPC_TLS_ENABLED=true
GRPC_TLS_CERT_PATH=/etc/ssl/certs/control.crt
GRPC_TLS_KEY_PATH=/etc/ssl/private/control.key

# Edge instance TLS
EDGE_TLS_ENABLED=true
EDGE_TLS_CA_PATH=/etc/ssl/certs/ca.crt
EDGE_TLS_CERT_PATH=/etc/ssl/certs/edge.crt
EDGE_TLS_KEY_PATH=/etc/ssl/private/edge.key
EDGE_SKIP_TLS_VERIFY=false
```

## Authentication Configuration

### Token Authentication
```bash
# Token generation settings
TOKEN_LENGTH=32             # Generated token length
SESSION_TIMEOUT=24h         # Default token lifetime
ENABLE_TOKEN_VALIDATION=true

# Token caching for performance
TOKEN_CACHE_ENABLED=true
TOKEN_CACHE_TTL=1h
TOKEN_CACHE_MAX_SIZE=10000
```

### API Key Management
```bash
# API key security
REDACT_API_KEYS=true        # Redact API keys in logs
API_KEY_ROTATION_ENABLED=true
API_KEY_ROTATION_INTERVAL=90d

# Key storage encryption
ENCRYPT_API_KEYS=true       # Encrypt stored API keys
```

### Authentication Scopes
```bash
# Available scopes for token-based access
# - admin: Full administrative access
# - api: Gateway proxy access
# - read: Read-only access
# - write: Write access (future)

# Scope validation
ENABLE_SCOPE_VALIDATION=true
STRICT_SCOPE_ENFORCEMENT=true
```

## Access Control

### IP Whitelisting
```bash
# Enable IP address restrictions
ENABLE_IP_WHITELIST=false   # Disabled by default

# Configure per application
mgw app create \
  --name="Restricted App" \
  --allowed-ips="203.0.113.1,203.0.113.2" \
  --email=user@company.com

# IP range support (CIDR notation)
mgw app update 1 --allowed-ips="10.0.0.0/8,192.168.1.0/24"
```

### Rate Limiting
```bash
# Global rate limiting
ENABLE_RATE_LIMITING=true
DEFAULT_RATE_LIMIT=100      # Requests per minute

# Rate limiting algorithms
RATE_LIMIT_ALGORITHM=token_bucket  # token_bucket, sliding_window
RATE_LIMIT_BURST_SIZE=200   # Burst capacity

# Rate limiting storage
RATE_LIMIT_STORAGE=memory   # memory, redis
REDIS_URL=redis://localhost:6379  # If using Redis
```

### CORS Configuration
```bash
# Cross-Origin Resource Sharing
CORS_ENABLED=true
CORS_ALLOWED_ORIGINS=https://dashboard.company.com,https://app.company.com
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE
CORS_ALLOWED_HEADERS=Authorization,Content-Type
CORS_MAX_AGE=86400          # Preflight cache duration
```

## Security Hardening

### Security Headers
```bash
# HTTP security headers
SECURITY_HEADERS_ENABLED=true

# Automatically adds:
# - X-Content-Type-Options: nosniff
# - X-Frame-Options: DENY
# - X-XSS-Protection: 1; mode=block
# - Strict-Transport-Security: max-age=31536000
# - Content-Security-Policy: default-src 'self'
```

### Request Validation
```bash
# Input validation
ENABLE_REQUEST_VALIDATION=true
MAX_REQUEST_SIZE=10MB
MAX_RESPONSE_SIZE=50MB

# Content validation
VALIDATE_JSON_PAYLOADS=true
SANITIZE_USER_INPUT=true
REJECT_MALFORMED_REQUESTS=true
```

### Security Logging
```bash
# Audit logging
AUDIT_LOG_ENABLED=true
AUDIT_LOG_PATH=/var/log/microgateway/audit.log

# Security event logging
LOG_SECURITY_EVENTS=true
LOG_AUTH_FAILURES=true
LOG_RATE_LIMIT_VIOLATIONS=true
LOG_IP_WHITELIST_VIOLATIONS=true
```

## Secrets Management

### Environment Variables
```bash
# Use environment variables for secrets
JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
DATABASE_DSN=postgres://user:${DB_PASSWORD}@host:port/db

# Never hardcode secrets in configuration files
```

### External Secret Management
```bash
# HashiCorp Vault integration
export JWT_SECRET=$(vault kv get -field=jwt_secret secret/microgateway)
export ENCRYPTION_KEY=$(vault kv get -field=encryption_key secret/microgateway)

# AWS Secrets Manager
export JWT_SECRET=$(aws secretsmanager get-secret-value \
  --secret-id microgateway/jwt-secret \
  --query SecretString --output text)

# Kubernetes secrets
export JWT_SECRET=$(kubectl get secret microgateway-secrets \
  -o jsonpath='{.data.jwt-secret}' | base64 -d)
```

### Key Rotation
```bash
# Regular key rotation schedule
# JWT secrets: Every 90 days
# Encryption keys: Every 365 days
# API keys: Every 30-90 days

# Rotation procedure:
# 1. Generate new key
# 2. Deploy new key alongside old key
# 3. Update applications to use new key
# 4. Remove old key after transition period
```

## Security Monitoring

### Security Metrics
```bash
# Monitor security events
curl http://localhost:8080/metrics | grep security

# Key security metrics:
# - auth_failures_total
# - rate_limit_violations_total
# - ip_whitelist_violations_total
# - token_validation_failures_total
# - tls_handshake_failures_total
```

### Audit Logging
```bash
# Security audit events
tail -f /var/log/microgateway/audit.log

# Example audit log entry:
{
  "timestamp": "2024-01-01T12:00:00Z",
  "event_type": "authentication_failure",
  "client_ip": "203.0.113.1",
  "user_agent": "curl/7.68.0",
  "app_id": 1,
  "reason": "invalid_token",
  "severity": "warning"
}
```

### Intrusion Detection
```bash
# Monitor for suspicious activity
# Multiple failed authentication attempts
grep "authentication_failure" /var/log/microgateway/audit.log | \
  awk '{print $5}' | sort | uniq -c | sort -nr

# Rate limiting violations
grep "rate_limit_violation" /var/log/microgateway/audit.log

# Unusual access patterns
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code == 401 or .status_code == 403)'
```

## Compliance Configuration

### Data Protection
```bash
# Data redaction for compliance
REDACT_SENSITIVE_HEADERS=true
REDACT_API_KEYS=true
REDACT_USER_CONTENT=false   # Set to true for sensitive applications

# Custom redaction patterns
REDACTION_PATTERNS=password,secret,key,token,ssn,email

# PII handling
ENABLE_PII_DETECTION=false  # Future feature
PII_REDACTION_ENABLED=false
```

### Audit Requirements
```bash
# Comprehensive audit logging
AUDIT_ALL_REQUESTS=true
AUDIT_CONFIGURATION_CHANGES=true
AUDIT_AUTHENTICATION_EVENTS=true
AUDIT_AUTHORIZATION_EVENTS=true

# Audit log retention
AUDIT_LOG_RETENTION_DAYS=2555  # 7 years for compliance
AUDIT_LOG_COMPRESSION=true
AUDIT_LOG_ENCRYPTION=true
```

### Data Residency
```bash
# Geographic data handling
DATA_RESIDENCY_ENFORCEMENT=true
ALLOWED_REGIONS=us,eu,ca
DEFAULT_REGION=us

# Cross-border data transfer controls
CROSS_BORDER_TRANSFER_ALLOWED=false
TRANSFER_APPROVAL_REQUIRED=true
```

## Security Deployment Examples

### Development Security
```bash
# Minimal security for development
TLS_ENABLED=false
ENABLE_IP_WHITELIST=false
ENABLE_RATE_LIMITING=false
JWT_SECRET=development-secret-key
ENCRYPTION_KEY=development-key-32-characters!
LOG_LEVEL=debug
AUDIT_LOG_ENABLED=false
```

### Production Security
```bash
# Full security for production
TLS_ENABLED=true
TLS_CERT_PATH=/etc/ssl/certs/microgateway.crt
TLS_KEY_PATH=/etc/ssl/private/microgateway.key
TLS_MIN_VERSION=1.2

JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
BCRYPT_COST=12

ENABLE_IP_WHITELIST=true
ENABLE_RATE_LIMITING=true
SECURITY_HEADERS_ENABLED=true

AUDIT_LOG_ENABLED=true
LOG_SECURITY_EVENTS=true
REDACT_SENSITIVE_HEADERS=true

PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_TRUSTED_KEYS_PATH=/etc/microgateway/trusted
```

### Enterprise Security
```bash
# Enterprise-grade security
TLS_ENABLED=true
TLS_MIN_VERSION=1.3
SECURITY_HEADERS_ENABLED=true
CORS_ENABLED=true

# Advanced authentication
ENABLE_SCOPE_VALIDATION=true
STRICT_SCOPE_ENFORCEMENT=true
TOKEN_ROTATION_ENABLED=true

# Comprehensive audit
AUDIT_ALL_REQUESTS=true
AUDIT_LOG_ENCRYPTION=true
COMPLIANCE_MODE=true

# Plugin security
PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_ALLOW_UNSIGNED=false
PLUGINS_SANDBOX_ENABLED=true
```

## Security Best Practices

### Key Management
```bash
# Generate cryptographically secure keys
JWT_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 16)

# Store keys securely
# - Use environment variables
# - Use external secret management
# - Never commit keys to version control
# - Rotate keys regularly

# Key rotation checklist:
# 1. Generate new key
# 2. Test new key in staging
# 3. Deploy new key to production
# 4. Monitor for authentication failures
# 5. Remove old key after grace period
```

### Network Security
```bash
# TLS best practices
# - Use TLS 1.2 minimum, prefer TLS 1.3
# - Use strong cipher suites
# - Enable certificate validation
# - Use proper certificate chain

# Network isolation
# - Deploy in private subnets
# - Use security groups/firewalls
# - Limit network access to necessary ports
# - Monitor network traffic patterns
```

### Authentication Security
```bash
# Token security
# - Use strong JWT secrets (32+ characters)
# - Set appropriate token expiration times
# - Implement token rotation
# - Monitor token usage patterns

# Credential security
# - Use strong bcrypt cost factors
# - Implement credential rotation
# - Monitor authentication failures
# - Lock accounts after failed attempts
```

## Compliance Features

### SOC 2 Compliance
```bash
# Audit logging for SOC 2
AUDIT_LOG_ENABLED=true
AUDIT_ALL_REQUESTS=true
AUDIT_CONFIGURATION_CHANGES=true
AUDIT_LOG_RETENTION_DAYS=2555  # 7 years

# Access controls
ENABLE_IP_WHITELIST=true
ENABLE_RATE_LIMITING=true
STRICT_SCOPE_ENFORCEMENT=true

# Data protection
REDACT_SENSITIVE_HEADERS=true
REDACT_USER_CONTENT=true
ENCRYPT_STORED_DATA=true
```

### GDPR Compliance
```bash
# Data protection for GDPR
GDPR_COMPLIANCE_MODE=true
DATA_MINIMIZATION=true
CONSENT_REQUIRED=true

# Data subject rights
ENABLE_DATA_EXPORT=true
ENABLE_DATA_DELETION=true
DATA_PORTABILITY_ENABLED=true

# Privacy controls
REDACT_PII=true
ANONYMIZE_ANALYTICS=true
GEOGRAPHIC_DATA_RESIDENCY=true
```

### HIPAA Compliance
```bash
# Healthcare data protection
HIPAA_COMPLIANCE_MODE=true
ENCRYPT_ALL_DATA=true
AUDIT_ALL_ACCESS=true

# Access controls
MINIMUM_NECESSARY_ACCESS=true
ROLE_BASED_ACCESS_CONTROL=true
SESSION_TIMEOUT=15m

# Data handling
PHI_REDACTION_ENABLED=true
BUSINESS_ASSOCIATE_AGREEMENT=required
```

## Security Monitoring

### Threat Detection
```bash
# Monitor for security threats
# Unusual authentication patterns
grep "auth_failure" /var/log/microgateway/audit.log | \
  awk '{print $4}' | sort | uniq -c | sort -nr

# Potential brute force attacks
grep "rate_limit_violation" /var/log/microgateway/audit.log | \
  awk '{print $4}' | sort | uniq -c | awk '$1 > 10'

# Unusual access patterns
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code == 401)' | \
  jq -s 'group_by(.client_ip) | map({ip: .[0].client_ip, failures: length}) | sort_by(.failures) | reverse'
```

### Security Alerting
```bash
# Set up security alerts
# Failed authentication rate > 10/minute
# Rate limiting violations > 100/hour
# IP whitelist violations > 5/hour
# Unusual geographic access patterns
# Large number of requests from single IP

# Example alert script
#!/bin/bash
FAILURES=$(grep "auth_failure" /var/log/microgateway/audit.log | \
  tail -n 100 | wc -l)

if [ $FAILURES -gt 50 ]; then
  echo "Security Alert: High authentication failure rate"
  # Send alert via webhook, email, etc.
fi
```

## Plugin Security

### Plugin Verification
```bash
# Plugin signature verification
PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_TRUSTED_KEYS_PATH=/etc/microgateway/trusted
PLUGINS_ALLOW_UNSIGNED=false

# Plugin sandboxing
PLUGINS_SANDBOX_ENABLED=true
PLUGINS_MAX_MEMORY=256MB
PLUGINS_MAX_CPU=50
PLUGINS_NETWORK_ACCESS=restricted
```

### Plugin Distribution Security
```bash
# OCI registry security
PLUGINS_REGISTRY_TLS_VERIFY=true
PLUGINS_ALLOWED_REGISTRIES=registry.company.com,ghcr.io

# Plugin integrity
PLUGINS_CHECKSUM_VERIFICATION=true
PLUGINS_MALWARE_SCANNING=false  # Future feature
```

## Security Testing

### Vulnerability Assessment
```bash
# Security scanning tools
# Install gosec for Go security analysis
go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest

# Run security scan
gosec ./...

# Example results:
# - Hardcoded credentials
# - Weak cryptographic practices
# - SQL injection vulnerabilities
# - Path traversal issues
```

### Penetration Testing
```bash
# Basic security testing
# Test authentication bypass
curl -X GET http://localhost:8080/api/v1/llms
# Should return 401 Unauthorized

# Test rate limiting
for i in {1..150}; do
  curl -H "Authorization: Bearer $TOKEN" \
    http://localhost:8080/api/v1/llms &
done
# Should trigger rate limiting at configured threshold

# Test input validation
curl -X POST http://localhost:8080/api/v1/llms \
  -H "Content-Type: application/json" \
  -d '{"name": "<script>alert(1)</script>"}'
# Should reject malicious input
```

## Incident Response

### Security Incident Handling
```bash
# Incident response procedures
# 1. Identify security event
# 2. Isolate affected systems
# 3. Analyze attack vectors
# 4. Implement containment measures
# 5. Eradicate threats
# 6. Recover normal operations
# 7. Document lessons learned

# Emergency procedures
# Revoke compromised tokens
mgw token revoke compromised-token

# Disable compromised applications
mgw app update 1 --active=false

# Block malicious IPs
mgw app update 1 --allowed-ips="trusted-ip-only"
```

### Forensic Analysis
```bash
# Collect forensic data
# Authentication logs
grep "auth_failure\|auth_success" /var/log/microgateway/audit.log

# Request patterns
mgw analytics events 1 --format=json | \
  jq '.data[] | {timestamp, client_ip, endpoint, status_code}'

# Configuration changes
grep "config_change" /var/log/microgateway/audit.log
```

## Security Automation

### Automated Security Monitoring
```bash
#!/bin/bash
# security-monitor.sh

# Monitor authentication failures
AUTH_FAILURES=$(grep "auth_failure" /var/log/microgateway/audit.log | \
  tail -n 1000 | wc -l)

if [ $AUTH_FAILURES -gt 100 ]; then
  # Alert security team
  curl -X POST https://alerts.company.com/webhook \
    -d '{"alert": "High authentication failure rate", "count": '$AUTH_FAILURES'}'
fi

# Monitor unusual access patterns
UNIQUE_IPS=$(mgw analytics events 1 --format=json | \
  jq -r '.data[].client_ip' | sort | uniq | wc -l)

if [ $UNIQUE_IPS -gt 1000 ]; then
  # Potential DDoS or scanning
  echo "Unusual IP diversity detected: $UNIQUE_IPS unique IPs"
fi
```

### Automated Response
```bash
# Automated threat response
# Block IPs with high failure rates
AUTH_FAILURES_BY_IP=$(grep "auth_failure" /var/log/microgateway/audit.log | \
  awk '{print $4}' | sort | uniq -c | awk '$1 > 50 {print $2}')

for ip in $AUTH_FAILURES_BY_IP; do
  # Add to firewall block list
  iptables -A INPUT -s $ip -j DROP
  echo "Blocked IP: $ip"
done
```

## Compliance Reporting

### Security Reports
```bash
# Generate security compliance report
#!/bin/bash
# security-report.sh

echo "Microgateway Security Report - $(date)"
echo "====================================="

# Authentication metrics
echo "Authentication Events:"
grep "auth_" /var/log/microgateway/audit.log | \
  awk '{print $3}' | sort | uniq -c

# Access control violations
echo "Access Control Violations:"
grep "violation" /var/log/microgateway/audit.log | wc -l

# Configuration changes
echo "Configuration Changes:"
grep "config_change" /var/log/microgateway/audit.log | wc -l

# TLS usage
echo "TLS Status:"
echo "TLS_ENABLED: $TLS_ENABLED"
echo "TLS_MIN_VERSION: $TLS_MIN_VERSION"
```

### Compliance Validation
```bash
# Validate compliance configuration
./microgateway --validate-compliance

# Check security configuration
./microgateway --security-check

# Generate compliance report
mgw compliance report --format=json > compliance-report.json
```

## Troubleshooting Security

### Authentication Issues
```bash
# Check JWT configuration
echo $JWT_SECRET | wc -c  # Should be 32+ characters

# Test token validation
mgw token validate $TOKEN

# Review authentication logs
grep "auth" /var/log/microgateway/microgateway.log
```

### TLS Issues
```bash
# Check certificate validity
openssl x509 -in /etc/ssl/certs/microgateway.crt -text -noout

# Test TLS connection
openssl s_client -connect localhost:8080 -servername microgateway.local

# Check certificate expiration
openssl x509 -in /etc/ssl/certs/microgateway.crt -noout -dates
```

### Access Control Issues
```bash
# Test IP whitelisting
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/llms
# Should work from allowed IPs, fail from others

# Test rate limiting
# Make rapid requests to trigger rate limiting
# Should return 429 Too Many Requests
```

---

Security configuration ensures the microgateway operates safely in production environments. For performance optimization, see [Performance Tuning](performance.md). For monitoring setup, see [Monitoring Configuration](monitoring.md).
