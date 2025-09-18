# OCI Plugin Configuration Examples

This document provides comprehensive examples for configuring OCI plugins with embedded public keys for containerized deployments.

## Environment Variable-Based Public Key Configuration

### Basic Setup (Docker/Container Friendly)

```bash
# Embedded PEM public keys (recommended for containers)
export OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4qiw8PWe4N5yKnXNAneu
TGGw6Gi6zp0SUHmQPIeP3w+2aV5PpnpNf8QzVwXFyLHb8gj9pkpUlzALVVLLSU/i
...
-----END PUBLIC KEY-----"

export OCI_PLUGINS_PUBKEY_2="-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyV8l2Z7X3f+EQ8/QJ4qK
...
-----END PUBLIC KEY-----"

# Named keys for different environments
export OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----..."
export OCI_PLUGINS_PUBKEY_PROD="-----BEGIN PUBLIC KEY-----..."
export OCI_PLUGINS_PUBKEY_DEV="-----BEGIN PUBLIC KEY-----..."
```

### Registry Authentication

```bash
# Registry-specific authentication
export OCI_PLUGINS_REGISTRY_NEXUS_USERNAME=plugin-reader
export OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV=NEXUS_PASSWORD
export OCI_PLUGINS_REGISTRY_HARBOR_TOKENENV=HARBOR_TOKEN

# Registry authentication credentials
export NEXUS_PASSWORD=supersecretpassword
export HARBOR_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Complete OCI Plugin Configuration

```bash
# Cache and security settings
export OCI_PLUGINS_CACHE_DIR=/var/lib/microgateway/plugins
export OCI_PLUGINS_MAX_CACHE_SIZE=1073741824  # 1GB
export OCI_PLUGINS_REQUIRE_SIGNATURE=true
export OCI_PLUGINS_ALLOWED_REGISTRIES=nexus.internal.com,harbor.company.com

# Network and retry settings
export OCI_PLUGINS_TIMEOUT=30s
export OCI_PLUGINS_RETRY_ATTEMPTS=3

# Garbage collection
export OCI_PLUGINS_GC_INTERVAL=24h
export OCI_PLUGINS_KEEP_VERSIONS=3
```

## OCI Plugin Command Examples

### Using Numbered Keys

```bash
# Use first key (OCI_PLUGINS_PUBKEY_1)
oci://nexus.company.com/plugins/auth@sha256:abc123?pubkey=1

# Use second key (OCI_PLUGINS_PUBKEY_2)
oci://harbor.company.com/plugins/rate-limiter@sha256:def456?pubkey=2
```

### Using Named Keys

```bash
# Use CI key (OCI_PLUGINS_PUBKEY_CI)
oci://registry.com/plugins/auth@sha256:abc123?pubkey=CI

# Use production key (OCI_PLUGINS_PUBKEY_PROD)
oci://registry.com/plugins/analytics@sha256:def456?pubkey=PROD

# Use development key (OCI_PLUGINS_PUBKEY_DEV)
oci://localhost:5000/plugins/test@sha256:ghi789?pubkey=DEV
```

### Using Default Keys

```bash
# Use first available key (automatic selection)
oci://registry.com/plugins/collector@sha256:jkl012

# Architecture-specific
oci://registry.com/plugins/auth@sha256:abc123?arch=linux/arm64

# Complete example with all parameters
oci://nexus.company.com/plugins/auth@sha256:abc123?arch=linux/amd64&pubkey=PROD&auth=nexus
```

## Docker Compose Example

```yaml
version: '3.8'
services:
  microgateway:
    image: microgateway:latest
    environment:
      # OCI Plugin Configuration
      OCI_PLUGINS_CACHE_DIR: /var/lib/microgateway/plugins
      OCI_PLUGINS_REQUIRE_SIGNATURE: "true"
      OCI_PLUGINS_ALLOWED_REGISTRIES: "nexus.company.com,harbor.company.com"

      # Embedded Public Keys
      OCI_PLUGINS_PUBKEY_CI: |
        -----BEGIN PUBLIC KEY-----
        MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4qiw8PWe4N5yKnXNAneu
        TGGw6Gi6zp0SUHmQPIeP3w+2aV5PpnpNf8QzVwXFyLHb8gj9pkpUlzALVVLLSU/i
        ...
        -----END PUBLIC KEY-----

      OCI_PLUGINS_PUBKEY_PROD: |
        -----BEGIN PUBLIC KEY-----
        MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyV8l2Z7X3f+EQ8/QJ4qK
        ...
        -----END PUBLIC KEY-----

      # Registry Authentication
      OCI_PLUGINS_REGISTRY_NEXUS_USERNAME: plugin-reader
      OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV: NEXUS_PASSWORD
      OCI_PLUGINS_REGISTRY_HARBOR_TOKENENV: HARBOR_TOKEN

      # Authentication Credentials
      NEXUS_PASSWORD: supersecretpassword
      HARBOR_TOKEN: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

    volumes:
      - plugin-cache:/var/lib/microgateway/plugins

volumes:
  plugin-cache:
```

## Kubernetes Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: oci-plugin-keys
type: Opaque
stringData:
  pubkey-ci.pem: |
    -----BEGIN PUBLIC KEY-----
    MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...
    -----END PUBLIC KEY-----
  pubkey-prod.pem: |
    -----BEGIN PUBLIC KEY-----
    MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...
    -----END PUBLIC KEY-----

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: oci-plugin-config
data:
  OCI_PLUGINS_CACHE_DIR: "/var/lib/microgateway/plugins"
  OCI_PLUGINS_REQUIRE_SIGNATURE: "true"
  OCI_PLUGINS_ALLOWED_REGISTRIES: "nexus.company.com,harbor.company.com"

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway
spec:
  template:
    spec:
      containers:
      - name: microgateway
        image: microgateway:latest
        env:
        # Load config from ConfigMap
        - name: OCI_PLUGINS_CACHE_DIR
          valueFrom:
            configMapKeyRef:
              name: oci-plugin-config
              key: OCI_PLUGINS_CACHE_DIR
        - name: OCI_PLUGINS_REQUIRE_SIGNATURE
          valueFrom:
            configMapKeyRef:
              name: oci-plugin-config
              key: OCI_PLUGINS_REQUIRE_SIGNATURE
        - name: OCI_PLUGINS_ALLOWED_REGISTRIES
          valueFrom:
            configMapKeyRef:
              name: oci-plugin-config
              key: OCI_PLUGINS_ALLOWED_REGISTRIES

        # Load keys from Secret
        - name: OCI_PLUGINS_PUBKEY_CI
          valueFrom:
            secretKeyRef:
              name: oci-plugin-keys
              key: pubkey-ci.pem
        - name: OCI_PLUGINS_PUBKEY_PROD
          valueFrom:
            secretKeyRef:
              name: oci-plugin-keys
              key: pubkey-prod.pem

        volumeMounts:
        - name: plugin-cache
          mountPath: /var/lib/microgateway/plugins

      volumes:
      - name: plugin-cache
        persistentVolumeClaim:
          claimName: plugin-cache-pvc
```

## Plugin Configuration in Database

```sql
-- Example plugin entries using OCI references
INSERT INTO plugins (name, slug, description, command, hook_type, is_active, namespace) VALUES
  ('OCI Auth Plugin', 'oci-auth', 'Authentication plugin from OCI registry',
   'oci://nexus.company.com/plugins/auth@sha256:abc123?pubkey=CI', 'auth', true, 'production'),

  ('OCI Rate Limiter', 'oci-rate-limiter', 'Rate limiting plugin with embedded key',
   'oci://harbor.company.com/plugins/rate-limiter@sha256:def456?pubkey=PROD&arch=linux/amd64', 'pre_auth', true, 'production'),

  ('OCI Analytics Collector', 'oci-analytics', 'Analytics collector using numbered key',
   'oci://registry.company.com/plugins/analytics@sha256:ghi789?pubkey=1', 'data_collection', true, 'production');
```

## Environment Variable Patterns

### Numbered Keys (Simple)
```bash
OCI_PLUGINS_PUBKEY_1="<PEM content>"
OCI_PLUGINS_PUBKEY_2="<PEM content>"
OCI_PLUGINS_PUBKEY_3="<PEM content>"
```

### Named Keys (Descriptive)
```bash
OCI_PLUGINS_PUBKEY_CI="<PEM content>"        # For CI-signed plugins
OCI_PLUGINS_PUBKEY_PROD="<PEM content>"      # For production plugins
OCI_PLUGINS_PUBKEY_DEV="<PEM content>"       # For development plugins
OCI_PLUGINS_PUBKEY_VENDOR="<PEM content>"    # For vendor-signed plugins
```

### File-Based Keys (Traditional)
```bash
OCI_PLUGINS_PUBKEY_FILE_CI=/etc/microgateway/keys/ci.pub
OCI_PLUGINS_PUBKEY_FILE_PROD=/etc/microgateway/keys/prod.pub
```

## Key Resolution Examples

### In Plugin Commands

```bash
# Numeric reference → OCI_PLUGINS_PUBKEY_1
oci://registry.com/plugin@sha256:abc?pubkey=1

# Named reference → OCI_PLUGINS_PUBKEY_CI
oci://registry.com/plugin@sha256:abc?pubkey=CI

# Environment variable → Direct lookup
oci://registry.com/plugin@sha256:abc?pubkey=env:OCI_PLUGINS_PUBKEY_CI

# File path → Direct file access
oci://registry.com/plugin@sha256:abc?pubkey=/path/to/key.pub

# Default → First available key
oci://registry.com/plugin@sha256:abc
```

## Development vs Production

### Development Setup
```bash
# Disable signature verification for development
export OCI_PLUGINS_REQUIRE_SIGNATURE=false

# Allow localhost registry
export OCI_PLUGINS_ALLOWED_REGISTRIES=localhost:5000,registry-1.docker.io

# Development public key
export OCI_PLUGINS_PUBKEY_DEV="$(cat testdata/keys/dev-plugin.pub)"
```

### Production Setup
```bash
# Strict security for production
export OCI_PLUGINS_REQUIRE_SIGNATURE=true
export OCI_PLUGINS_ALLOWED_REGISTRIES=nexus.company.com,harbor.company.com

# Production public keys (managed via secret management)
export OCI_PLUGINS_PUBKEY_PROD="$(cat /run/secrets/plugin-pubkey-prod)"
export OCI_PLUGINS_PUBKEY_CI="$(cat /run/secrets/plugin-pubkey-ci)"
```

## Benefits of Embedded Key Approach

1. **Container-Native**: No filesystem dependencies or volume mounts required
2. **Secure**: Keys managed via container orchestration secret systems
3. **Flexible**: Support multiple key sources and formats
4. **Operational**: Easy key rotation via environment variable updates
5. **Debuggable**: Clear key resolution with descriptive error messages

## Migration from File-Based Keys

```bash
# Old approach (file-based)
oci://registry.com/plugin@sha256:abc?pubkey=/etc/keys/plugin.pub

# New approach (embedded)
export OCI_PLUGINS_PUBKEY_PLUGIN="$(cat /etc/keys/plugin.pub)"
# Then use: oci://registry.com/plugin@sha256:abc?pubkey=PLUGIN
```

This approach eliminates filesystem dependencies while maintaining security and flexibility for all deployment scenarios.