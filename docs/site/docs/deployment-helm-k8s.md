---
title: "Installation (Helm/Kubernetes)"
weight: 2
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Installation (Helm/Kubernetes)

This guide explains how to deploy Tyk AI Studio (Tyk AI Studio), a secure and extensible AI gateway, using Helm on Kubernetes.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- kubectl configured with access to your cluster
- A securely generated `TYK_AI_SECRET_KEY` string for secrets encryption (generate with `openssl rand -hex 16`)
- A `tykAiLicense` string from Tyk Technologies (Enterprise Edition)
- If using SSL/TLS: cert-manager installed in your cluster

## Edition Selection

Tyk AI Studio images are available in two editions:

| Component | Community Edition | Enterprise Edition |
|-----------|------------------|--------------------|
| AI Studio | `tykio/tyk-ai-studio:latest` | `tykio/tyk-ai-studio:latest-ent` |
| Microgateway | `tykio/microgateway:latest` | `tykio/microgateway:latest-ent` |

Update the `image` values in your Helm values file according to your edition.

*Note: The following examples use placeholder values (e.g., `your-domain.com`, `your-secret-key`). Remember to replace these with your actual configuration values.*

## Installation Options

Tyk AI Studio can be deployed in several configurations:

1. Local Development  
2. Production without TLS
3. Production with TLS
4. Production with NATS Distributed Queue

### Option 1: Local Development Setup

1. Create a `values-local.yaml` file:

```yaml
midsommar:
  ingress:
    enabled: false
  service:
    type: NodePort
    ports:
      - name: http
        port: 8080
        nodePort: 32580
      - name: gateway
        port: 9090
        nodePort: 32590

config:
  allowRegistrations: "true"
  adminEmail: "admin@localhost"
  siteUrl: "http://localhost:32580"
  fromEmail: "noreply@localhost"
  devMode: "true"
  databaseType: "postgres"
  tykAiSecretKey: "your-secret-key"
  tykAiLicense: "your-license"

database:
  internal: true
  user: "postgres"
  password: "localdev123"
  name: "midsommar"

# Optional AI components
reranker:
  enabled: true
  image:
    repository: tykio/reranker_cpu
    tag: latest

transformer-server:
  enabled: true
  image:
    repository: tykio/transformer_server_cpu
    tag: latest
```

2. Install the chart:

```bash
helm install midsommar . -f values-local.yaml
```

3. Access the application:
- Web Interface: http://localhost:32580
- Gateway: http://localhost:32590

### Option 2: Production without TLS

For a production deployment without TLS certificates:

1. Create `values-prod-no-tls.yaml`:

```yaml
midsommar:
  ingress:
    enabled: true
    certificateEnabled: false
    className: nginx
    hosts:
      - host: app.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 8080
      - host: gateway.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 9090

config:
  allowRegistrations: "true"
  adminEmail: "admin@yourdomain.com"
  siteUrl: "http://app.yourdomain.com"
  fromEmail: "noreply@yourdomain.com"
  devMode: "false"
  databaseType: "postgres"
  tykAiSecretKey: "your-production-key"
  tykAiLicense: "your-production-license"

database:
  internal: false
  host: "your-db-host"
  port: 5432
  name: "midsommar"
  user: "your-db-user"
  password: "your-db-password"
```

2. Install:

```bash
helm install midsommar . -f values-prod-no-tls.yaml
```

### Option 3: Production with TLS

For a secure production deployment with TLS:

1. Create `values-prod-tls.yaml`:

```yaml
midsommar:
  ingress:
    enabled: true
    certificateEnabled: true
    className: nginx
    certManager:
      issuer: letsencrypt-prod
    hosts:
      - host: app.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 8080
      - host: gateway.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 9090
    tls:
      - secretName: app-tls-secret
        hosts:
          - app.yourdomain.com
      - secretName: gateway-tls-secret
        hosts:
          - gateway.yourdomain.com

config:
  allowRegistrations: "true"
  adminEmail: "admin@yourdomain.com"
  siteUrl: "https://app.yourdomain.com"
  fromEmail: "noreply@yourdomain.com"
  devMode: "false"
  databaseType: "postgres"
  tykAiSecretKey: "your-production-key"
  tykAiLicense: "your-production-license"

database:
  internal: false
  url: "postgres://user:password@your-production-db:5432/midsommar"
```

2. Install:

```bash
helm install midsommar . -f values-prod-tls.yaml
```

### Option 4: Production with NATS Distributed Queue

For high-availability production deployment with distributed message queuing:

1. Create `values-prod-nats.yaml`:

```yaml
midsommar:
  ingress:
    enabled: true
    certificateEnabled: true
    className: nginx
    certManager:
      issuer: letsencrypt-prod
    hosts:
      - host: app.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 8080
      - host: gateway.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 9090
    tls:
      - secretName: app-tls-secret
        hosts:
          - app.yourdomain.com
      - secretName: gateway-tls-secret
        hosts:
          - gateway.yourdomain.com

config:
  allowRegistrations: "true"
  adminEmail: "admin@yourdomain.com"
  siteUrl: "https://app.yourdomain.com"
  fromEmail: "noreply@yourdomain.com"
  devMode: "false"
  databaseType: "postgres"
  tykAiSecretKey: "your-production-key"
  tykAiLicense: "your-production-license"
  
  # NATS Queue Configuration
  queueType: "nats"
  natsUrl: "nats://nats-cluster:4222"
  natsStorageType: "file"
  natsRetentionPolicy: "interest"
  natsMaxAge: "4h"
  natsMaxBytes: 536870912  # 512MB
  natsDurableConsumer: true
  natsCredentialsFile: "/etc/nats/user.creds"
  natsTlsEnabled: true
  natsTlsCaFile: "/etc/ssl/certs/nats-ca.pem"

database:
  internal: false
  url: "postgres://user:password@your-production-db:5432/midsommar"

# NATS Cluster Configuration
nats:
  enabled: true
  cluster:
    enabled: true
    replicas: 3
  jetstream:
    enabled: true
    fileStore:
      enabled: true
      size: 50Gi
      storageClass: "fast-ssd"
  auth:
    enabled: true
    resolver:
      type: "jwt"
      configMap:
        name: "nats-accounts"
        key: "resolver.conf"
  tls:
    enabled: true
    ca: "nats-ca-secret"
    cert: "nats-server-cert"
```

2. Create NATS authentication secrets:

```bash
# Create JWT resolver configuration
kubectl create configmap nats-accounts --from-file=resolver.conf

# Create user credentials secret
kubectl create secret generic nats-user-creds --from-file=user.creds

# Create TLS certificates
kubectl create secret tls nats-server-cert --cert=server.crt --key=server.key
kubectl create secret generic nats-ca-secret --from-file=ca.crt
```

3. Install:

```bash
helm install midsommar . -f values-prod-nats.yaml
```

## Option 5: Adding Edge Gateways (Microgateway)

> **Note:** Edge gateway deployments are an **Enterprise Edition** feature.

To add a Microgateway as a data plane (spoke) to your AI Studio deployment (hub), you need to:

1. Enable control plane mode on AI Studio
2. Deploy the Microgateway with edge configuration
3. Configure shared secrets between the two components

### Step 1: Update AI Studio Values for Control Mode

Add these settings to your existing AI Studio values file:

```yaml
config:
  # ... your existing config ...
  gatewayMode: "control"
  grpcPort: "50051"
  grpcHost: "0.0.0.0"
  grpcTlsInsecure: "true"  # Set to "false" in production with TLS
  grpcAuthToken: "your-grpc-auth-token"  # Generate with: openssl rand -hex 16

midsommar:
  service:
    ports:
      - name: http
        port: 8080
      - name: gateway
        port: 9090
      - name: grpc
        port: 50051
```

### Step 2: Create Microgateway Resources

Create a `microgateway-values.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: analytics-pulse-config
data:
  analytics-pulse.yaml: |
    version: "1.0"
    data_collection_plugins:
      - name: "analytics_pulse"
        enabled: true
        hook_types: ["analytics", "budget", "proxy_log"]
        replace_database: false
        priority: 100
        config:
          interval_seconds: 10
          max_batch_size: 1000
          max_buffer_size: 10000
          compression_enabled: true
          include_proxy_summaries: true
          include_request_response_data: true
          edge_retention_hours: 24
          excluded_vendors: ["mock", "test"]
          timeout_seconds: 30
          max_retries: 3
          retry_interval_secs: 5
---
apiVersion: v1
kind: Secret
metadata:
  name: microgateway-secrets
type: Opaque
stringData:
  EDGE_AUTH_TOKEN: "your-grpc-auth-token"          # Must match AI Studio grpcAuthToken
  ENCRYPTION_KEY: "your-microgateway-encryption-key" # Must match AI Studio microgatewayEncryptionKey
  # TYK_AI_LICENSE: "your-license-key"             # Enterprise only
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway
spec:
  replicas: 1
  selector:
    matchLabels:
      app: microgateway
  template:
    metadata:
      labels:
        app: microgateway
    spec:
      containers:
        - name: microgateway
          image: tykio/microgateway:latest-ent
          ports:
            - containerPort: 8080
          envFrom:
            - secretRef:
                name: microgateway-secrets
          env:
            - name: PORT
              value: "8080"
            - name: GATEWAY_MODE
              value: "edge"
            - name: CONTROL_ENDPOINT
              value: "midsommar:50051"  # AI Studio service name and gRPC port
            - name: EDGE_ID
              value: "edge-1"
            - name: EDGE_NAMESPACE
              value: "default"
            - name: EDGE_HEARTBEAT_INTERVAL
              value: "30s"
            - name: EDGE_ALLOW_INSECURE
              value: "true"
            - name: EDGE_TLS_ENABLED
              value: "false"
            - name: DATABASE_TYPE
              value: "sqlite"
            - name: DATABASE_DSN
              value: "file:./data/edge.db?cache=shared&mode=rwc"
            - name: DB_AUTO_MIGRATE
              value: "true"
            - name: GATEWAY_ENABLE_ANALYTICS
              value: "true"
            - name: ANALYTICS_ENABLED
              value: "true"
            - name: PLUGINS_CONFIG_PATH
              value: "/app/config/analytics-pulse.yaml"
            - name: LOG_LEVEL
              value: "info"
          volumeMounts:
            - name: analytics-config
              mountPath: /app/config
            - name: data
              mountPath: /app/data
      volumes:
        - name: analytics-config
          configMap:
            name: analytics-pulse-config
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: microgateway
spec:
  selector:
    app: microgateway
  ports:
    - name: http
      port: 8080
      targetPort: 8080
```

### Step 3: Deploy

```bash
# Update AI Studio with control mode
helm upgrade midsommar . -f your-values.yaml

# Deploy Microgateway resources
kubectl apply -f microgateway-values.yaml
```

### Step 4: Add Ingress for Microgateway (Optional)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: microgateway-ingress
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  rules:
    - host: gateway.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: microgateway
                port:
                  number: 8080
```

### Shared Secrets Reference

These values **must match** between AI Studio and Microgateway:

| AI Studio Config | Microgateway Secret/Env | Purpose |
|---|---|---|
| `config.grpcAuthToken` | `EDGE_AUTH_TOKEN` | gRPC authentication |
| `config.microgatewayEncryptionKey` | `ENCRYPTION_KEY` | Config encryption |
| `config.tykAiLicense` | `TYK_AI_LICENSE` | Enterprise license |

### Scaling Edge Gateways

To deploy multiple edge gateways (e.g., for different regions), create separate Deployments with unique `EDGE_ID` and `EDGE_NAMESPACE` values:

```yaml
env:
  - name: EDGE_ID
    value: "edge-eu-west-1"
  - name: EDGE_NAMESPACE
    value: "eu-west"
```

Each edge instance registers independently with the AI Studio control plane and receives only the configuration assigned to its namespace.

## Message Queue Configuration

Tyk AI Studio supports two message queue implementations:

### In-Memory Queue (Default)

For single-instance deployments:

```yaml
config:
  queueType: "inmemory"
  queueBufferSize: 100
```

### NATS JetStream Queue

For distributed deployments with message persistence:

```yaml
config:
  queueType: "nats"
  queueBufferSize: 100
  
  # NATS Connection
  natsUrl: "nats://nats-server:4222"
  natsStorageType: "file"
  natsRetentionPolicy: "interest"
  natsMaxAge: "2h"
  natsMaxBytes: 104857600
  
  # NATS Authentication (choose one method)
  natsUsername: "chat_service"           # Basic auth
  natsPassword: "secure_password"        # Basic auth
  # OR
  natsToken: "your-secret-token"         # Token auth
  # OR  
  natsCredentialsFile: "/etc/nats/user.creds"  # JWT auth (recommended)
  
  # NATS TLS (optional)
  natsTlsEnabled: true
  natsTlsCertFile: "/etc/ssl/client-cert.pem"
  natsTlsKeyFile: "/etc/ssl/client-key.pem"
  natsTlsCaFile: "/etc/ssl/ca-cert.pem"
```

### NATS Server Deployment

To deploy NATS with your Helm chart:

```yaml
# Add to your values.yaml
nats:
  enabled: true
  image:
    repository: nats
    tag: "latest"
  jetstream:
    enabled: true
    storage: file
    storageSize: 10Gi
  auth:
    enabled: true
    # Configure authentication method
```

For detailed NATS configuration, see the [NATS Configuration Guide](./nats-configuration.md).

## Optional Components

### Reranker Service

The Reranker service improves RAG result relevance. Enable it with:

```yaml
reranker:
  enabled: true
  image:
    repository: tykio/reranker_cpu
    tag: latest
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
```

### Transformer Server

The Transformer Server handles embedding generation and model inference. Enable it with:

```yaml
transformer-server:
  enabled: true
  image:
    repository: tykio/transformer_server_cpu
    tag: latest
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
```

## Database Options

### Using Internal PostgreSQL

For development or small deployments:

```yaml
database:
  internal: true
  user: "postgres"
  password: "secure-password"
  name: "midsommar"

postgres:
  persistence:
    enabled: true
    size: 10Gi
    storageClass: "standard"
```

### Using External Database

For production environments:

```yaml
database:
  internal: false
  url: "postgres://user:password@your-db-host:5432/midsommar"
```

## Maintenance

### Upgrading

To upgrade an existing installation:

```bash
helm upgrade midsommar . -f your-values.yaml
```

### Uninstalling

To remove the deployment:

```bash
helm uninstall midsommar
```

### Viewing Logs

```bash
# Main application logs
kubectl logs -l app.kubernetes.io/name=midsommar

# Database logs (if using internal database)
kubectl logs -l app=postgres

# Optional component logs
kubectl logs -l app=reranker
kubectl logs -l app=transformer
```

## Troubleshooting

1. Check pod status:
```bash
kubectl get pods
```

2. Check ingress configuration:
```bash
kubectl get ingress
```

3. View pod details:
```bash
kubectl describe pod <pod-name>
```

4. Common issues:
- Database connection failures: Check credentials and network access
- Ingress not working: Verify DNS records and TLS configuration
- Resource constraints: Check pod resource limits and node capacity

## Next Steps

Once deployed, proceed to the [Initial Configuration](./configuration.md) guide to set up Tyk AI Studio.
