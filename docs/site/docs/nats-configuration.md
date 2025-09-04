---
title: "NATS Configuration"
weight: 15
---

# NATS Configuration

This guide covers configuring Tyk AI Studio to use NATS JetStream as the message queue backend for distributed deployments and high availability scenarios.

## Overview

Tyk AI Studio supports two queue implementations:
- **In-Memory Queue** (default): For single-instance deployments
- **NATS JetStream Queue**: For distributed, persistent message handling

NATS JetStream provides:
- **Persistent Message Storage**: Messages survive server restarts
- **Distributed Architecture**: Scale across multiple instances
- **Automatic Reconnection**: Handle network disruptions gracefully
- **Message Deduplication**: Ensure exactly-once delivery
- **Authentication & Security**: Multiple authentication methods including JWT and TLS

## Basic Configuration

### Enable NATS Queue

Set the queue type to NATS in your environment configuration:

```bash
QUEUE_TYPE=nats
NATS_URL=nats://your-nats-server:4222
```

### NATS Server Configuration

```bash
# NATS Connection
NATS_URL=nats://localhost:4222              # NATS server URL

# JetStream Settings  
NATS_STORAGE_TYPE=file                      # Use persistent storage (file|memory)
NATS_RETENTION_POLICY=interest              # Delete messages when consumed (interest|limits|workqueue)
NATS_MAX_AGE=2h                             # Maximum message age
NATS_MAX_BYTES=104857600                    # Max stream size (100MB)
NATS_DURABLE_CONSUMER=true                  # Use durable consumers for restart recovery

# Performance Tuning
NATS_ACK_WAIT=30s                           # Acknowledgment timeout
NATS_MAX_DELIVER=3                          # Maximum delivery attempts
NATS_FETCH_TIMEOUT=5s                       # Message fetch timeout
NATS_RETRY_INTERVAL=1s                      # Retry interval for failed operations
NATS_MAX_RETRIES=3                          # Maximum retry attempts
```

## Authentication Methods

### 1. No Authentication (Development Only)

```bash
NATS_URL=nats://localhost:4222
```

**Use Case**: Local development, testing environments
**Security**: None - suitable only for development

### 2. Username/Password Authentication

```bash
NATS_URL=nats://localhost:4222
NATS_USERNAME=chat_service
NATS_PASSWORD=secure_password_123
```

**Use Case**: Simple deployments with basic security
**Security**: Basic authentication with shared credentials

### 3. Token-Based Authentication

```bash
NATS_URL=nats://localhost:4222
NATS_TOKEN=your-secret-token-here
```

**Use Case**: Simple token-based access control
**Security**: Shared secret token authentication

### 4. JWT/User Credentials Authentication (Recommended)

```bash
NATS_URL=nats://localhost:4222
NATS_CREDENTIALS_FILE=/etc/nats/user.creds
```

**Use Case**: Production deployments requiring fine-grained access control
**Security**: JWT-based authentication with user credentials
**Benefits**:
- Decentralized authentication
- Subject-level permissions
- Automatic token renewal
- Audit trails

#### Creating User Credentials

1. Generate user credentials with NATS CLI:
```bash
# Create account
nsc add account TykAIStudio

# Create user for chat service
nsc add user chat_service --account TykAIStudio

# Generate credentials file
nsc generate creds --account TykAIStudio --name chat_service > /etc/nats/user.creds
```

2. Configure permissions in account settings:
```bash
nsc edit user chat_service --allow-pub "chat.>" --allow-sub "chat.>"
```

### 5. NKey Authentication

```bash
NATS_URL=nats://localhost:4222
NATS_NKEY_FILE=/etc/nats/user.nkey
```

**Use Case**: Cryptographic authentication without JWT overhead
**Security**: Ed25519 key-based authentication

#### Creating NKey

```bash
# Generate NKey
nsc add user chat_service_nkey --account TykAIStudio --allow-pub "chat.>" --allow-sub "chat.>"
nk -gen user > /etc/nats/user.nkey
```

### 6. TLS Configuration

#### Basic TLS (Server Authentication)

```bash
NATS_URL=nats://your-secure-nats:4222
NATS_TLS_ENABLED=true
```

#### Mutual TLS (Client + Server Authentication)

```bash
NATS_URL=nats://your-secure-nats:4222
NATS_TLS_ENABLED=true
NATS_TLS_CERT_FILE=/etc/ssl/certs/client-cert.pem
NATS_TLS_KEY_FILE=/etc/ssl/private/client-key.pem
NATS_TLS_CA_FILE=/etc/ssl/certs/ca-cert.pem
```

#### TLS Development Mode (Skip Verification)

```bash
NATS_URL=nats://localhost:4222
NATS_TLS_ENABLED=true
NATS_TLS_SKIP_VERIFY=true  # Only for development!
```

### 7. Combined Authentication (Production Recommended)

```bash
# TLS + JWT Authentication
NATS_URL=nats://secure-nats.production:4222
NATS_TLS_ENABLED=true
NATS_CREDENTIALS_FILE=/etc/nats/production-user.creds
NATS_TLS_CA_FILE=/etc/ssl/certs/nats-ca.pem

# Additional security settings
NATS_TLS_SKIP_VERIFY=false
NATS_MAX_DELIVER=3
NATS_ACK_WAIT=30s
```

## Docker/Kubernetes Configuration

### Docker Compose

```yaml
version: '3.8'
services:
  nats:
    image: nats:latest
    command: 
      - "--jetstream"
      - "--store_dir=/data"
    ports:
      - "4222:4222"
    volumes:
      - nats_data:/data
      
  midsommar:
    image: tyk/midsommar:latest
    environment:
      QUEUE_TYPE: "nats"
      NATS_URL: "nats://nats:4222"
      NATS_STORAGE_TYPE: "file"
      NATS_RETENTION_POLICY: "interest"
    depends_on:
      - nats
      
volumes:
  nats_data:
```

### Kubernetes ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nats-config
data:
  QUEUE_TYPE: "nats"
  NATS_URL: "nats://nats-cluster:4222"
  NATS_STORAGE_TYPE: "file"
  NATS_RETENTION_POLICY: "interest"
  NATS_MAX_AGE: "2h"
  NATS_DURABLE_CONSUMER: "true"
```

### Kubernetes Secrets (for Authentication)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nats-auth
type: Opaque
data:
  credentials: <base64-encoded-creds-file>
  username: <base64-encoded-username>
  password: <base64-encoded-password>
```

## Monitoring and Troubleshooting

### Connection Status

Monitor NATS connection status in application logs:

```bash
kubectl logs -f deployment/midsommar | grep NATS
```

Expected log messages:
```
INFO NATS authentication configured with credentials file file=/etc/nats/user.creds
INFO NATS reconnected session_id=abc123 url=nats://nats:4222
```

### Common Issues

#### 1. Connection Failures
```bash
ERROR failed to connect to NATS: nats: no servers available for connection
```
**Solution**: Check NATS server is running and URL is correct

#### 2. Authentication Failures  
```bash
ERROR failed to connect to NATS: nats: Authorization Violation
```
**Solution**: Verify credentials file exists and has correct permissions

#### 3. TLS Certificate Issues
```bash
ERROR failed to configure NATS authentication: failed to load TLS client certificate
```
**Solution**: Check certificate files exist and have correct permissions

#### 4. Permission Errors
```bash
ERROR failed to create stream: insufficient permissions
```
**Solution**: Ensure user has pub/sub permissions for `chat.>` subjects

### Performance Tuning

For high-throughput deployments:

```bash
# Increase buffer sizes
QUEUE_BUFFER_SIZE=1000

# Optimize NATS settings  
NATS_MAX_BYTES=1048576000          # 1GB max stream size
NATS_MAX_AGE=24h                   # Longer retention
NATS_FETCH_TIMEOUT=10s             # Longer fetch timeout
NATS_ACK_WAIT=60s                  # Longer ack timeout
```

## Security Best Practices

1. **Use JWT Authentication**: Preferred for production deployments
2. **Enable TLS**: Always use TLS in production
3. **Least Privilege**: Grant minimal required permissions
4. **Secure Credentials**: Store credentials in secure secret management
5. **Network Isolation**: Use private networks for NATS communication
6. **Regular Rotation**: Rotate credentials regularly
7. **Monitor Access**: Enable NATS logging and monitoring

## Migration from In-Memory

To migrate from in-memory to NATS queue:

1. **Deploy NATS Server**: Set up NATS with JetStream enabled
2. **Configure Authentication**: Set up appropriate auth method
3. **Update Configuration**: Change `QUEUE_TYPE=nats`
4. **Restart Application**: Deploy updated configuration
5. **Verify Operation**: Check logs for successful NATS connection

The migration is seamless as the queue interface abstracts the implementation details.

## Advanced Configuration

### Custom Stream Configuration

For advanced use cases, configure streams directly in NATS:

```bash
# Create custom stream for chat messages
nats stream add CHAT_CUSTOM \
  --subjects "chat.sessions.*.chat_response" \
  --storage file \
  --retention interest \
  --max-age 4h \
  --max-bytes 500MB
```

### High Availability Setup

For production HA deployments:

```bash
# Connect to NATS cluster
NATS_URL=nats://nats-1:4222,nats-2:4222,nats-3:4222

# Enable clustering support
NATS_DURABLE_CONSUMER=true
NATS_MAX_DELIVER=5
```

This configuration ensures message delivery even if individual NATS servers fail.