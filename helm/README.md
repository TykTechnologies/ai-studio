# Midsommar Helm Chart

This Helm chart deploys Midsommar along with its optional components. It supports both development and production configurations with flexible database options.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- kubectl configured to communicate with your cluster
- If using SSL/TLS: cert-manager installed in your cluster

## Configuration Options

### Core Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `midsommar.image.repository` | Docker image repository | `tykio/midsommar` |
| `midsommar.image.tag` | Docker image tag | `latest` |
| `midsommar.service.type` | Kubernetes service type | `ClusterIP` |
| `config.allowRegistrations` | Enable user registrations | `"true"` |
| `config.adminEmail` | Admin email address | `"you@tyk.io"` |
| `config.siteUrl` | Site URL | `"http://localhost:3000"` |
| `config.fromEmail` | From email address | `"noreply@tyk.io"` |
| `config.devMode` | Enable development mode | `"false"` |
| `config.filterSignupDomains` | Restrict signup to specific domains | `""` |

### Database Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `database.internal` | Use internal PostgreSQL | `true` |
| `database.url` | External database URL | `""` |
| `database.host` | Database host | `""` |
| `database.port` | Database port | `5432` |
| `database.name` | Database name | `"midsommar"` |
| `database.user` | Database user | `"postgres"` |
| `database.password` | Database password | `"postgres"` |

### SMTP Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.smtpServer` | SMTP server address | `""` |
| `config.smtpPort` | SMTP server port | `"587"` |
| `config.smtpUser` | SMTP username | `"apikey"` |
| `config.smtpPass` | SMTP password | `""` |

## Installation

### Local Development Setup

1. Create a values file for local development:

```yaml
# local-values.yaml
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
  devMode: "true"
  allowRegistrations: "true"
  adminEmail: "admin@localhost"
  siteUrl: "http://localhost:32580"

database:
  internal: true
  password: "localdev123"
```

2. Install the chart:

```bash
helm install midsommar . -f local-values.yaml
```

### Production Setup with External Database

1. Create a production values file:

```yaml
# production-values.yaml
midsommar:
  ingress:
    enabled: true
    certificateEnabled: true
    hosts:
      - host: app.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
            port: 8080

config:
  devMode: "false"
  smtpServer: "smtp.sendgrid.net"
  smtpPort: "587"
  smtpUser: "apikey"
  smtpPass: "your-smtp-password"
  siteUrl: "https://app.yourdomain.com"

database:
  internal: false
  url: "postgres://user:password@your-db-host:5432/midsommar"
```

2. Install the chart:

```bash
helm install midsommar . -f production-values.yaml
```

## Using Internal PostgreSQL with Persistence

To use the built-in PostgreSQL with persistence:

```yaml
database:
  internal: true
  user: "postgres"
  password: "your-secure-password"

postgres:
  persistence:
    enabled: true
    size: 10Gi
    storageClass: "standard" # Specify your storage class
```

## Optional Components

### Reranker

The Reranker service provides re-ranking capabilities for RAG (Retrieval-Augmented Generation) results. It helps improve the relevance of retrieved documents before they're used in the generation process.

To enable the Reranker:

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

The Transformer Server hosts Hugging Face transformer models for:
- Generating embeddings for RAG queries
- Creating embeddings for vector database storage
- Running inference on transformer models

To enable the Transformer Server:

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

## Upgrading

To upgrade an existing installation:

```bash
helm upgrade midsommar . -f your-values.yaml
```

## Uninstalling

To uninstall/delete the deployment:

```bash
helm uninstall midsommar
```

## Note on Persistence

If using the internal PostgreSQL database, make sure to back up your data before upgrading or uninstalling the chart. When the chart is uninstalled, the PVC and its data will remain unless manually deleted.
