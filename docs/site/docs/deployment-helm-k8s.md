---
title: "Kubernetes / Helm Deployment"
weight: 2
---

# Kubernetes / Helm Deployment

This guide deploys Tyk AI Studio (control plane), a Microgateway (data plane), and PostgreSQL on Kubernetes using Helm. AI Studio manages configuration centrally and the Microgateway processes AI requests, receiving config via gRPC.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- kubectl configured with access to your cluster
- For production with TLS: cert-manager installed in your cluster

## Edition Selection

Tyk AI Studio is available in two editions:

| Component | Community Edition | Enterprise Edition |
|-----------|------------------|-------------------|
| AI Studio | `tykio/tyk-ai-studio:v2.0.0` | `tykio/tyk-ai-studio-ent:v2.0.0` |
| Microgateway | `tykio/tyk-microgateway:v2.0.0` | `tykio/tyk-microgateway-ent:v2.0.0` |

Images are tagged with semver versions (e.g. `v2.0.0`). There is no `latest` tag — always specify a version. Enterprise Edition includes SSO, edge gateways, model router, and plugin marketplace features. Enterprise Edition configuration variables (e.g. `tykAiLicense`) are safely ignored by Community Edition images, so you can use the same values file for both.

## Generate Secrets

Before installing, generate three secret keys:

```bash
# Secret key for encryption (used for secrets management and SSO)
openssl rand -hex 16
# Example output: a35b3f7b0fb4dd3a048ba4fc6e9fe0a8

# Encryption key for microgateway communication (must be exactly 32 hex chars)
openssl rand -hex 16
# Example output: 822d3d1e0e2d849263e45fc7bb842364

# gRPC auth token (for hub-spoke communication)
openssl rand -hex 16
# Example output: 9f2c4a6b8d0e1f3a5c7d9e1b3a5c7d9e
```

Save these values — you will substitute them into the values file below.

---

## Option 1: Testing / Quickstart

For local development or test clusters. Uses NodePort services, internal PostgreSQL, and no ingress.

### 1. Add the Helm Chart

```bash
cd /path/to/tyk-ai-studio/helm
helm dependency update .
```

### 2. Create `values-testing.yaml`

Replace the placeholder secrets with your generated values. The `grpcAuthToken` / `edgeAuthToken` and `microgatewayEncryptionKey` / `encryptionKey` pairs **must match**.

```yaml
midsommar:
  image:
    repository: tykio/tyk-ai-studio  # Enterprise: tykio/tyk-ai-studio-ent
    tag: v2.0.0
  service:
    type: NodePort
    ports:
      - name: http
        port: 8080
        targetPort: 8080
        nodePort: 32580
      - name: gateway
        port: 9090
        targetPort: 9090
        nodePort: 32590
      - name: grpc
        port: 50051
        targetPort: 50051

config:
  allowRegistrations: "true"
  siteUrl: "http://localhost:32580"                       # Update post-install if not localhost
  fromEmail: "noreply@localhost"
  devMode: "true"                                       # Required for login over plain HTTP
  databaseType: "postgres"
  tykAiSecretKey: "CHANGE-ME-first-secret"
  tykAiLicense: "your-license-key"                      # Enterprise only, ignored by CE
  ociCacheDir: "./data/cache/plugins"
  ociRequireSignature: "false"
  gatewayMode: "control"
  grpcPort: "50051"
  grpcHost: "0.0.0.0"
  grpcTlsInsecure: "true"
  grpcAuthToken: "CHANGE-ME-third-secret"
  microgatewayEncryptionKey: "CHANGE-ME-second-secret"
  # proxyUrl auto-resolves to the microgateway k8s service — no need to set it

database:
  internal: true
  user: "tyk"
  password: "your-db-password"
  name: "tyk_ai_studio"

postgres:
  persistence:
    enabled: true
    size: 1Gi

microgateway:
  enabled: true
  image:
    repository: tykio/tyk-microgateway  # Enterprise: tykio/tyk-microgateway-ent
    tag: v2.0.0
  service:
    type: NodePort
    port: 8080
    nodePort: 32591
  config:
    edgeId: "edge-1"
    edgeNamespace: "default"
  secrets:
    edgeAuthToken: "CHANGE-ME-third-secret"             # Must match config.grpcAuthToken
    encryptionKey: "CHANGE-ME-second-secret"             # Must match config.microgatewayEncryptionKey
    tykAiLicense: "your-license-key"                     # Enterprise only, ignored by CE
```

### 3. Install

```bash
helm install midsommar . -f values-testing.yaml
```

### 4. Set External Gateway URL

The Microgateway's internal service URL is used for routing by default, but the portal needs to display the correct external URL for tools and datasources. After install, patch the config with your cluster's node IP:

```bash
# Get the node IP and set the gateway URL
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
GATEWAY_URL="http://${NODE_IP}:32591"

# Patch the configmap with correct URLs and restart AI Studio
STUDIO_URL="http://${NODE_IP}:32580"
kubectl patch configmap midsommar-config -p \
  "{\"data\":{\"SITE_URL\":\"${STUDIO_URL}\",\"TOOL_DISPLAY_URL\":\"${GATEWAY_URL}\",\"DATASOURCE_DISPLAY_URL\":\"${GATEWAY_URL}\"}}"
kubectl rollout restart deployment midsommar
```

> **Tip:** If you know your cluster's external IP or hostname upfront, you can skip this step by setting `config.toolDisplayUrl` and `config.datasourceDisplayUrl` in your values file instead.

### 5. Verify

```bash
# Check all pods are running
kubectl get pods

# Check AI Studio health (via NodePort)
curl -s http://${NODE_IP}:32580/health

# Check Microgateway health (via NodePort)
curl -s http://${NODE_IP}:32591/health
```

### Access Points

| Port | URL | Purpose |
|------|-----|---------|
| 32580 | `http://<node-ip>:32580` | AI Studio UI + REST API |
| 32590 | `http://<node-ip>:32590` | Embedded AI Gateway |
| 32591 | `http://<node-ip>:32591` | Microgateway (Edge Gateway) |

---

## Option 2: Production with TLS

For production deployments with Ingress, TLS via cert-manager, and an external database.

### 1. Create `values-production.yaml`

Replace all placeholder values with your actual configuration.

```yaml
midsommar:
  image:
    repository: tykio/tyk-ai-studio  # Enterprise: tykio/tyk-ai-studio-ent
    tag: v2.0.0
  ingress:
    enabled: true
    certificateEnabled: true
    className: nginx
    certManager:
      issuer: letsencrypt-prod
    hosts:
      - host: studio.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 8080
    tls:
      - secretName: studio-tls-secret
        hosts:
          - studio.yourdomain.com
  service:
    ports:
      - name: http
        port: 8080
        targetPort: 8080
      - name: gateway
        port: 9090
        targetPort: 9090
      - name: grpc
        port: 50051
        targetPort: 50051

config:
  allowRegistrations: "true"
  siteUrl: "https://studio.yourdomain.com"
  fromEmail: "noreply@yourdomain.com"
  devMode: "false"
  databaseType: "postgres"
  tykAiSecretKey: "CHANGE-ME-first-secret"
  tykAiLicense: "your-license-key"
  ociCacheDir: "./data/cache/plugins"
  ociRequireSignature: "false"
  gatewayMode: "control"
  grpcPort: "50051"
  grpcHost: "0.0.0.0"
  grpcTlsInsecure: "true"                               # Set to "false" with TLS certs
  grpcAuthToken: "CHANGE-ME-third-secret"
  microgatewayEncryptionKey: "CHANGE-ME-second-secret"
  # proxyUrl, toolDisplayUrl, datasourceDisplayUrl auto-resolve from microgateway ingress config

database:
  internal: false
  url: "postgres://user:password@your-db-host:5432/tyk_ai_studio?sslmode=require"

microgateway:
  enabled: true
  image:
    repository: tykio/tyk-microgateway  # Enterprise: tykio/tyk-microgateway-ent
    tag: v2.0.0
  ingress:
    enabled: true
    certificateEnabled: true
    className: nginx
    certManager:
      issuer: letsencrypt-prod
    hosts:
      - host: gateway.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: gateway-tls-secret
        hosts:
          - gateway.yourdomain.com
  config:
    edgeId: "edge-1"
    edgeNamespace: "default"
    allowInsecure: "false"
    tlsEnabled: "false"                                  # gRPC client TLS to AI Studio
  secrets:
    edgeAuthToken: "CHANGE-ME-third-secret"
    encryptionKey: "CHANGE-ME-second-secret"
    tykAiLicense: "your-license-key"
```

### 2. Install

```bash
helm dependency update .
helm install midsommar . -f values-production.yaml
```

### 3. Verify

```bash
kubectl get pods
kubectl get ingress
curl -s https://studio.yourdomain.com/health
curl -s https://gateway.yourdomain.com/health
```

---

## After Deployment

AI Studio pre-populates OpenAI and Anthropic LLM configurations on first startup with placeholder secrets (`OPENAI_KEY` and `ANTHROPIC_KEY`). To start using them:

### 1. Add Your API Keys

1. Open AI Studio at the `siteUrl` you configured and register your first admin account
2. Navigate to **Governance → Secrets** in the sidebar
3. Click on **`OPENAI_KEY`** and edit it to add your OpenAI API key
4. Click on **`ANTHROPIC_KEY`** and edit it to add your Anthropic API key

### 2. Push Configuration to the Microgateway

1. Navigate to **AI Portal → Edge Gateways** in the sidebar
2. Verify your edge gateway (`edge-1`) shows as **Connected**
3. Click **Push Configuration** to sync the latest settings to the Microgateway

Once the sync status shows **Synced**, the Microgateway is ready to proxy LLM requests.

For further setup (additional LLMs, users, applications), see the **[Initial Configuration](./configuration.md)** guide.

---

## Shared Secrets Reference

These values **must match** between AI Studio and Microgateway configuration:

| AI Studio Config | Microgateway Config | Purpose |
|---|---|---|
| `config.grpcAuthToken` | `microgateway.secrets.edgeAuthToken` | Authenticates the gRPC connection |
| `config.microgatewayEncryptionKey` | `microgateway.secrets.encryptionKey` | Encrypts synced configuration data |
| `config.tykAiLicense` | `microgateway.secrets.tykAiLicense` | Enterprise license (if applicable) |

## Port Reference

| Port | Component | Purpose |
|------|-----------|---------|
| 8080 | AI Studio | Admin UI + REST API |
| 9090 | AI Studio | Embedded AI Gateway |
| 50051 | AI Studio | gRPC control server (internal) |
| 8080 | Microgateway | Edge AI Gateway |
| 5432 | PostgreSQL | Database |

---

## Advanced Configuration

### Message Queue (NATS)

For distributed deployments with message persistence, add NATS configuration to your values file:

```yaml
config:
  queueType: "nats"
  natsUrl: "nats://nats-server:4222"
  natsStorageType: "file"
  natsRetentionPolicy: "interest"
  natsMaxAge: "4h"
  natsTlsEnabled: true
  natsCredentialsFile: "/etc/nats/user.creds"
```

For detailed NATS setup, see the [NATS Configuration Guide](./nats-configuration.md).

### Optional Components

#### Reranker Service

Improves RAG result relevance:

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

#### Transformer Server

Handles embedding generation:

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

### Scaling Edge Gateways

To deploy multiple edge gateways for different regions, override `edgeId` and `edgeNamespace` per instance. You can either deploy separate Helm releases or create additional Kubernetes Deployments with unique values:

```yaml
microgateway:
  config:
    edgeId: "edge-eu-west-1"
    edgeNamespace: "eu-west"
```

Each edge instance registers independently with AI Studio and receives only the configuration assigned to its namespace.

### Database Options

**Internal PostgreSQL** (testing/small deployments):

```yaml
database:
  internal: true
  user: "tyk"
  password: "secure-password"
  name: "tyk_ai_studio"

postgres:
  persistence:
    enabled: true
    size: 10Gi
    storageClass: "standard"
```

**External Database** (production):

```yaml
database:
  internal: false
  url: "postgres://user:password@your-db-host:5432/tyk_ai_studio?sslmode=require"
```

---

## Maintenance

### Upgrading

```bash
helm upgrade midsommar . -f your-values.yaml
```

### Uninstalling

```bash
helm uninstall midsommar
```

### Viewing Logs

```bash
# AI Studio logs
kubectl logs -l app.kubernetes.io/name=midsommar

# Microgateway logs
kubectl logs -l app=microgateway

# Database logs (internal postgres)
kubectl logs -l app=postgres
```

## Troubleshooting

### Check pod and ingress status

```bash
kubectl get pods
kubectl get ingress
kubectl describe pod <pod-name>
```

### Common Issues

- **Database connection failures**: Check credentials and network access
- **Ingress not working**: Verify DNS records and TLS configuration
- **Login fails on HTTP**: Set `devMode: "true"` — session cookies require this when not using HTTPS
- **Marketplace page is empty**: Set `ociCacheDir: "./data/cache/plugins"` in your config values — the marketplace service will not start without it
- **Plugin signature verification**: Docker images use distroless bases without cosign. Set `ociRequireSignature: "false"` to disable signature verification

### Microgateway cannot connect to AI Studio

- Verify the microgateway pod logs: `kubectl logs -l app=microgateway`
- Check that `CONTROL_ENDPOINT` resolves to the AI Studio service (default: `midsommar:50051`)
- Verify `edgeAuthToken` matches `grpcAuthToken` exactly
- Verify `encryptionKey` matches `microgatewayEncryptionKey` exactly
- Check that `GATEWAY_MODE=control` is set in AI Studio config

## Next Steps

Once deployed, proceed to the [Initial Configuration](./configuration.md) guide to set up Tyk AI Studio.
