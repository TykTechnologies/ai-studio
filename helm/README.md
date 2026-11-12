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
| `midsommar.strategy` | Deployment strategy. Empty: `Recreate` for one replica (the old pod stops before the new one starts, so two Studio instances never run side by side during an upgrade), a rolling update (`maxUnavailable: 0`, `maxSurge: 1`) for several | `{}` |
| `replicaCount` | Studio replicas. Above 1 needs an external PostgreSQL and `config.csrfKey` (see [Several Studio replicas](#several-studio-replicas)); Studio has no autoscaler | `1` |
| `podDisruptionBudget.minAvailable` / `.maxUnavailable` | PodDisruptionBudget, rendered when `replicaCount` is above 1 (or `podDisruptionBudget.enabled`) | `1` / unset |
| `topologySpreadConstraints`, `affinity` | Scheduling of the Studio pods. Empty spread with several replicas: over nodes and zones, `ScheduleAnyway` | `[]`, `{}` |
| `global.nodeSelector`, `global.tolerations`, `global.affinity` | Scheduling of the AI Studio pods and the bundled PostgreSQL pod (for example one site per node pool); `affinity` replaces `global.affinity` for AI Studio. The microgateway subchart has its own `microgateway.nodeSelector`, `.tolerations` and `.affinity` | `{}`, `[]`, `{}` |
| `midsommar.ingress.sessionAffinity` | Cookie affinity on the nginx Ingress with several replicas (`enabled`, `cookieName`, `maxAge`) | on |
| `config.clusterNodeLabel` | `CLUSTER_NODE_LABEL`: names this release's replicas on the cluster status (for example `site-a`) | `""` (`studio`) |
| `config.csrfKey` | `CSRF_KEY`, stored in the Secret; required with several replicas | `""` |
| `config.databaseMaxOpenConns`, `databaseMaxIdleConns`, `databaseConnMaxLifetime`, `databaseConnectTimeout`, `databaseReadyGrace`, `databaseStartupTimeout` | The `DATABASE_*` pool, readiness and start-up settings; empty keeps Studio's default, `0` is passed on | `""` |
| `probes.startup` | Startup probe on `/health`: `periodSeconds` × `failureThreshold` must cover `DATABASE_STARTUP_TIMEOUT` plus the migrations | `5` × `36` (180 s) |
| `shutdown.preStopDelaySeconds` | Seconds an AI Studio pod keeps serving after Kubernetes starts removing it from the Service, before it gets SIGTERM, so requests routed to it meanwhile are answered (`0`: none). Rendered on Kubernetes 1.30 and later (the `sleep` action); on an older cluster no hook is rendered and the install notes say so | `10` |
| `shutdown.terminationGracePeriodSeconds` | Pod grace period. Empty: the delay plus Studio's 30-second drain plus 5 s; a smaller value fails the render | `""` (45) |
| `probes.readiness.requireWrite` | Probe `/ready?require=write` instead of `/ready` | `false` |
| `config.allowRegistrations` | Enable user registrations | `"true"` |
| `config.adminEmail` | Admin email address | `"you@tyk.io"` |
| `config.siteUrl` | Site URL | `"http://localhost:3000"` |
| `config.fromEmail` | From email address | `"noreply@tyk.io"` |
| `config.devMode` | Enable development mode | `"false"` |
| `config.logLevel` | AI Studio log level (`LOG_LEVEL`): `trace`, `debug`, `info`, `warn`, `error` | `"info"` |
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
| `database.existingSecret` / `database.existingSecretKey` | A Secret holding the whole `DATABASE_URL` (for example the multi-host form); the other `database.*` connection values are then not used | `""` / `DATABASE_URL` |

### SMTP Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.smtpServer` | SMTP server address | `""` |
| `config.smtpPort` | SMTP server port | `"587"` |
| `config.smtpUser` | SMTP username | `"apikey"` |
| `config.smtpPass` | SMTP password | `""` |

## Several Studio replicas

With an external PostgreSQL, Studio can run several replicas (from 2.3):

```yaml
replicaCount: 2
database:
  internal: false
  existingSecret: studio-database   # a Secret with the key DATABASE_URL
config:
  csrfKey: "<openssl rand -hex 32>"
```

The chart then renders a rolling update, a PodDisruptionBudget, spread over nodes and zones, and cookie affinity on the nginx Ingress, and refuses to render with the bundled PostgreSQL, with SQLite, or without `config.csrfKey`. For two sites (one release per cluster on one replicated PostgreSQL, labelled with `config.clusterNodeLabel: site-a` and `site-b`), the probes, gRPC and the upgrade to 2.3, see "Several Studio replicas" in `docs/site/docs/deployment-helm-k8s.md`.

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
