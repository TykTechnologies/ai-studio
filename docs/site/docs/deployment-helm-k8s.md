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
| AI Studio | `tykio/tyk-ai-studio:v2.3.0` | `tykio/tyk-ai-studio-ent:v2.3.0` |
| Microgateway | `tykio/tyk-microgateway:v2.3.0` | `tykio/tyk-microgateway-ent:v2.3.0` |

Images are tagged with semver versions (e.g. `v2.0.0`). There is no `latest` tag — always specify a version. Enterprise Edition includes SSO, edge gateways, model router, and plugin marketplace features. Enterprise Edition configuration variables (e.g. `tykAiLicense`) are safely ignored by Community Edition images, so you can use the same values file for both.

## Generate Secrets

Before installing, generate three secret keys:

```bash
# Secret key for encryption (used for secrets management and SSO)
openssl rand -hex 16
# Example output (SAMPLE ONLY — generate your own, never copy this): xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Encryption key for microgateway communication (must be exactly 32 hex chars)
openssl rand -hex 16
# Example output (SAMPLE ONLY — generate your own, never copy this): yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy

# gRPC auth token (for hub-spoke communication)
openssl rand -hex 16
# Example output (SAMPLE ONLY — generate your own, never copy this): zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz
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
    tag: v2.3.0
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
  ociRequireSignature: "false"                          # Development only: loads unsigned plugins (default "true")
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
    tag: v2.3.0
  service:
    type: NodePort
    port: 8080
    nodePort: 32591
  config:
    # EDGE_ID is set per-pod from the pod name; do not set edgeId here.
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
    tag: v2.3.0
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
  ociRequireSignature: "true"
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
    tag: v2.3.0
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
    # EDGE_ID is set per-pod from the pod name; do not set edgeId here.
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
2. Navigate to **Settings → Secrets** in the sidebar
3. Click on **`OPENAI_KEY`** and edit it to add your OpenAI API key
4. Click on **`ANTHROPIC_KEY`** and edit it to add your Anthropic API key

### 2. Push Configuration to the Microgateway

1. Navigate to **Portal administration → Edge Gateways** in the sidebar
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

Two different things are often meant by "scaling", and they are configured differently.

**More replicas of one edge.** Just raise `replicaCount`. Each pod derives its
`EDGE_ID` from its own pod name via the downward API, so every replica registers
with AI Studio under a distinct identity. Do **not** set `edgeId` yourself — the
value is no longer read from the chart, because a single shared value made every
replica collide on one registration.

```yaml
microgateway:
  replicaCount: 3
  podDisruptionBudget:
    enabled: true
    maxUnavailable: 1
```

Pod names change when a Deployment rolls, which leaves behind stale edge
registrations; AI Studio reaps these automatically. Use `kind: StatefulSet` if
you would rather have stable per-replica identities.

**When to use the StatefulSet form.** A Deployment keeps the edge's database on
an `emptyDir`: it survives a container restart, but a rescheduled pod (a node
drain, an eviction, a rollout) starts with an empty one. That is fine while the
database is only a cache of what AI Studio sends. Use `kind: StatefulSet` with
`persistence.enabled: true` when the edge must keep something across a
rescheduling:

- **Starting without a control plane** (`config.startFromLastKnownGood`). The
  configuration snapshot the edge starts from is in its database. Without a
  persistent volume a rescheduled pod has none and crash-loops until a control
  plane answers, exactly as without the setting.
- **Full credential sync** (`config.credentialSync: full`, Enterprise). The
  hashed list of keys the edge validates from is in its database. Without a
  persistent volume a rescheduled pod starts with none, and validates on
  demand until its first sync completes.
- **A stable edge identity.** The edge ID is the pod name. A StatefulSet pod
  keeps its name (`<release>-microgateway-0`, `-1`, ...) across restarts and
  rescheduling, so the edge keeps its row, its history and its sync state in
  AI Studio; a Deployment pod gets a new name, and so a new edge, every time.
- **Buffered analytics and budget counters** that should survive a restart.

The chart prints a warning after `helm install` or `helm upgrade` when
`startFromLastKnownGood` or `credentialSync: full` is set without StatefulSet
persistence. It still installs: an `emptyDir` does cover container restarts.

**Sizing the edge's volume.** What fills it is the edge's own copy of the
analytics: one row per request, kept for `config.analyticsRetentionDays`
(`ANALYTICS_RETENTION_DAYS`; the chart sets 90 days, an edge without the
setting keeps 7). Configuration, the stored snapshot and budget state are
small beside it. Measured under load: about 2 GB per 5 to 6 million
requests, so roughly 400 bytes per request.

    volume size ≈ 400 bytes × requests per second × 86,400 × retention days

| Requests per second | 1 day | 7 days | 90 days |
|---|---|---|---|
| 1 | 35 MB | 240 MB | 3 GB |
| 10 | 350 MB | 2.4 GB | 31 GB |
| 100 | 3.5 GB | 24 GB | 310 GB |
| 1,000 | 35 GB | 240 GB | 3 TB |

The default `persistence.size` of 2Gi with the chart's 90 days holds about
0.7 requests per second sustained; in a load test at 3,000 to 6,000
requests per second it was full in 30 to 45 minutes. AI Studio has these
records too (the analytics pulse sends them), so on an edge that reports to
AI Studio a short retention loses nothing there: set
`config.analyticsRetentionDays` to what the edge's own use needs, often `"1"`,
and size the volume from the table.

**When the volume is full** the edge keeps serving, and says so: `/ready`
stays 200 with `"database_full": true` and what is no longer stored,
`microgateway_database_full` is 1, and the log has one error per kind of
write. What stops: its own analytics rows are dropped, **a configuration
push cannot be applied** (the edge keeps the configuration it had and
reports that one, so AI Studio shows it as not in sync and the push as
failed), and the last control contact and budget blocks are no longer
stored for a restart. See
[the microgateway's database guide](https://github.com/TykTechnologies/ai-studio/blob/main/microgateway/docs/config/database.md#when-the-database-is-full).

**Several control planes.** For an active-active AI Studio, give the edge both
sites, its own first, and decide whether it may start without either:

```yaml
microgateway:
  kind: StatefulSet
  persistence:
    enabled: true
  config:
    controlEndpoints:
      - studio.site-a.example.com:50051
      - studio.site-b.example.com:50051
    controlFailbackInterval: "5m"     # optional: return to the first when it is back
    startFromLastKnownGood: true      # optional: start from the stored configuration
    # startupControlTimeout: "30s"
    # lastKnownGoodMaxAge: "72h"
    # credentialSync: "full"          # Enterprise: validate keys from the synced, hashed list
    # credentialSyncMaxAge: "24h"     # how long that list is trusted without a sync ("0": no limit)
```

`controlEndpoints` replaces `controlEndpoint` when it is not empty. Keep
`probes.startup` (`periodSeconds` x `failureThreshold`, 150 s by default) above
`startupControlTimeout`: with `startFromLastKnownGood` the edge spends that
long looking for a control plane before it starts from its stored
configuration. See [Several control planes](./edge-gateways.md#several-control-planes)
and [Starting without a control plane](./edge-gateways.md#starting-without-a-control-plane).

**Separate edges per region.** Deploy separate Helm releases and override
`edgeNamespace` per instance. Each edge receives only the configuration assigned
to its namespace:

```yaml
microgateway:
  config:
    edgeNamespace: "eu-west"
```

### Production Hardening

The microgateway subchart supports the usual production controls. Defaults are
safe but conservative:

```yaml
microgateway:
  # Supply credentials from External Secrets, Sealed Secrets or your own tooling
  # instead of putting them in values. The Secret must carry EDGE_AUTH_TOKEN,
  # ENCRYPTION_KEY and TYK_AI_LICENSE.
  secrets:
    existingSecret: "microgateway-credentials"

  podDisruptionBudget:
    enabled: true
    maxUnavailable: 1

  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 75
    # LLM requests are long-lived, so scale in slowly rather than cutting
    # streams short.
    scaleDownStabilizationSeconds: 300

  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: ScheduleAnyway
      labelSelector:
        matchLabels:
          app: microgateway

  # Keeps analytics buffered between pulses across a restart. Optional while
  # the edge rebuilds its configuration from the hub on every start; required
  # for config.startFromLastKnownGood and config.credentialSync: full to cover
  # a rescheduled pod (see "When to use the StatefulSet form").
  kind: StatefulSet
  persistence:
    enabled: true
    size: 2Gi
```

Notes on the defaults:

- The container runs as non-root with a read-only root filesystem. A writable
  `/tmp` is mounted for it, since SQLite needs somewhere to put temporary files.
- `terminationGracePeriodSeconds` is 40s, comfortably above the 30s
  `SHUTDOWN_TIMEOUT`, so in-flight requests can drain before the pod is killed.
- Readiness probes `/ready`, which checks the database and plugin health, rather
  than `/health`, which only reports that the process is alive.

### Plugin Sources and Signatures

Plugin signatures are required by default, and both AI Studio and the
gateways can refuse whole plugin deployment types. Each gateway enforces its
own setting, whatever AI Studio pushes:

```yaml
config:
  ociRequireSignature: "true"         # AI_STUDIO_OCI_REQUIRE_SIGNATURE (default "true")
  pluginAllowedSources: "oci"         # AI_STUDIO_PLUGIN_ALLOWED_SOURCES: oci, file, grpc; "" allows all

microgateway:
  config:
    ociPluginsRequireSignature: "true"  # OCI_PLUGINS_REQUIRE_SIGNATURE (default "true")
    pluginsAllowedSources: "oci"        # PLUGINS_ALLOWED_SOURCES: oci, file, grpc; "" allows all
```

| Value | Environment variable | Default |
|---|---|---|
| `config.ociRequireSignature` | `AI_STUDIO_OCI_REQUIRE_SIGNATURE` | `"true"` |
| `config.pluginAllowedSources` | `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` | `""` (all sources) |
| `microgateway.config.ociPluginsRequireSignature` | `OCI_PLUGINS_REQUIRE_SIGNATURE` | `"true"` |
| `microgateway.config.pluginsAllowedSources` | `PLUGINS_ALLOWED_SOURCES` | `""` (all sources) |

An unknown source stops the pod at startup. See
[Hardening: restrict plugin sources](./plugins-security.md#hardening-restrict-plugin-sources)
and [Signature Verification](./plugins-deployment.md#signature-verification).

### Egress Policy

By default the gateway will connect to any upstream an LLM configuration names,
including cluster-internal addresses. To constrain that while still allowing
specific in-cluster model servers:

```yaml
microgateway:
  config:
    blockInternalUpstreams: true
    allowedInternalHosts: ".svc.cluster.local"
```

`allowedInternalHosts` is an additive exemption, not a global allowlist — naming
a cluster host does not restrict the external providers you are already using.
See [Running Alongside the Kubernetes Inference Gateway](./deployment-kubernetes-inference-gateway.md).

### Monitoring and Tracing

```yaml
microgateway:
  metrics:
    enabled: true
    allowUnauthenticated: true   # required for in-cluster scraping
    serviceMonitor:
      enabled: true
      interval: 30s
  tracing:
    enabled: true
    endpoint: "otel-collector.observability:4317"
```

The `/metrics` endpoint is secure by default and is not served at all unless
`allowUnauthenticated` is true or a `METRICS_AUTH_TOKEN` is set; the chart fails
the render rather than deploying a ServiceMonitor that would scrape nothing. For
the full metric reference and example queries, see [Observability](./observability.md).

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

To keep the credentials out of values, put the whole URL in a Secret of your own and name it with `database.existingSecret` (key `DATABASE_URL`, or `database.existingSecretKey`). It is then passed to Studio from the Secret only, not through the ConfigMap.

### Several Studio replicas

From 2.3 several AI Studio replicas can serve one PostgreSQL database (see [Several replicas](./reference-architecture.md#several-replicas-from-2-3)). Set `replicaCount` above 1. The chart then needs:

- **An external PostgreSQL** (`database.internal: false`). The bundled PostgreSQL is a single pod, and SQLite serves one process, so the render fails with either.
- **One `CSRF_KEY`** for every replica (`config.csrfKey`, generate it with `openssl rand -hex 32`). Without it each replica generates its own and a form loaded from one replica is refused by another, so the render fails without it. The other shared secrets (`config.tykAiSecretKey`, `config.grpcAuthToken`, `config.microgatewayEncryptionKey`) are the same for every replica of a release already.

And it renders, with `replicaCount` above 1:

- **A rolling update** that starts a new replica before stopping an old one (`maxUnavailable: 0`, `maxSurge: 1`), unless `midsommar.strategy` is set. With one replica the default stays `Recreate`.
- **A PodDisruptionBudget** keeping `podDisruptionBudget.minAvailable` (1) replicas through node drains, or allowing `podDisruptionBudget.maxUnavailable` when that is set.
- **Spread** of the replicas over nodes and zones (`ScheduleAnyway`). A `topologySpreadConstraints` list replaces it; `affinity` is passed through as given.
- **Cookie affinity on the Ingress** for the nginx class (`midsommar.ingress.sessionAffinity`), because chat, agent and MCP SSE sessions are held by the replica that started them. Annotations you set in `midsommar.ingress.annotations` win. For another ingress controller, set its own affinity annotations.

**Probes.** Studio waits for its database at start-up (`DATABASE_STARTUP_TIMEOUT`, 60 seconds by default) and runs its migrations before it listens, so neither `/health` nor `/ready` answers during that time. A startup probe on `/health` allows `probes.startup.periodSeconds` × `probes.startup.failureThreshold`, 180 seconds by default; keep it longer than `DATABASE_STARTUP_TIMEOUT` plus the migration time plus one more connection attempt (`DATABASE_CONNECT_TIMEOUT` per host, twice with `sslmode=prefer`), and raise it for the first start of a new version on a large database. Readiness is `/ready`, which stays 200 through a database failover shorter than `DATABASE_READY_GRACE` (30 seconds; see [Readiness during a failover](./configuration.md#postgres-with-a-standby-failover)). `probes.readiness.requireWrite: true` probes `/ready?require=write` instead, which takes a replica whose database is read-only out of the Service after the grace.

**Stopping a replica.** On SIGTERM AI Studio stops accepting connections and lets the requests in flight finish, for up to 30 seconds. Kubernetes takes the pod out of the Service at the same moment, which takes a few seconds to reach every node, so the chart has the pod sleep first (`shutdown.preStopDelaySeconds`, 10 seconds, with the `sleep` preStop action; on a cluster older than Kubernetes 1.30, which lacks that action, no hook is rendered and the install notes say a rolling update may refuse a few requests there) and sets `terminationGracePeriodSeconds` to cover the sleep and the drain (45 seconds by default). Without the sleep, a rolling restart refused 2 of 600 requests at the moment an old pod began terminating.

**gRPC.** Each edge holds one long-lived stream to port 50051. The Service spreads new connections over the replicas, and a replica that stops hands its edges to the others, which reconnect within seconds. A load balancer in front of port 50051 must support HTTP/2 and keep idle streams open for at least a minute (edges ping every 30 seconds). Set `appProtocol: grpc` on the `grpc` port in `midsommar.service.ports` when a service mesh or a Gateway API route needs to know the port speaks gRPC.

**Upgrading to 2.3.** Stop every older replica before the first 2.3 replica starts (see the 2.3 upgrade notes): for that one upgrade, set `midsommar.strategy.type: Recreate`, or scale to one replica first. Later upgrades can roll.

```yaml
replicaCount: 3

database:
  internal: false
  existingSecret: studio-database   # key DATABASE_URL

config:
  csrfKey: "<openssl rand -hex 32>"
  databaseMaxOpenConns: "30"        # all replicas together under the server's max_connections
  databaseConnectTimeout: "3s"
```

#### Two clusters, one replicated PostgreSQL

The active-active layout (see [Active-Active (Two Sites)](./deployment-active-active.md) for the database, the witness and the runbooks): one release in each site's cluster, both on the same replicated PostgreSQL, each site's replicas labelled with its name so the cluster status (`GET /api/v1/cluster/status`) and the Edge Gateways page tell them apart. The two releases must share every secret: put them in values both sites use, or in Secrets with the same contents.

```yaml
# values-site-a.yaml. values-site-b.yaml is the same with site-b as the label
# and site-b's control endpoint first.
replicaCount: 2

database:
  internal: false
  # postgres://studio:...@pg-a.example.internal,pg-b.example.internal/tyk_ai_studio?target_session_attrs=read-write&sslmode=require&connect_timeout=3
  existingSecret: studio-database

config:
  clusterNodeLabel: site-a
  csrfKey: "<the same in both sites>"
  tykAiSecretKey: "<the same in both sites>"
  grpcAuthToken: "<the same in both sites>"
  microgatewayEncryptionKey: "<the same in both sites>"
  databaseConnectTimeout: "3s"

microgateway:
  config:
    # Each site's edges try their own site's Studio first.
    controlEndpoints:
      - studio-site-a.example.internal:50051
      - studio-site-b.example.internal:50051
```

```bash
helm upgrade --install studio ./helm -f values-site-a.yaml --kube-context site-a
helm upgrade --install studio ./helm -f values-site-b.yaml --kube-context site-b
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

For more detail, raise the log level. AI Studio reads `LOG_LEVEL` (`trace`, `debug`, `info`, `warn`, `error`; default `info`), which the chart sets from `config.logLevel`; the microgateway subchart has its own `microgateway.config.logLevel`:

```bash
helm upgrade midsommar ./helm --reuse-values --set config.logLevel=debug
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
- **Plugin refused because its signature does not verify**: signatures are verified in-process (Enterprise Edition) and required by default (`ociRequireSignature: "true"` for AI Studio, `microgateway.config.ociPluginsRequireSignature: "true"` for the Microgateway). Configure the plugin's signing key, or set these to `"false"` to load unsigned plugins. See [Signature Verification](./plugins-deployment.md#signature-verification)

### Microgateway cannot connect to AI Studio

- Verify the microgateway pod logs: `kubectl logs -l app=microgateway`
- Check that `CONTROL_ENDPOINT` resolves to the AI Studio service (default: `midsommar:50051`)
- Verify `edgeAuthToken` matches `grpcAuthToken` exactly
- Verify `encryptionKey` matches `microgatewayEncryptionKey` exactly
- Check that `GATEWAY_MODE=control` is set in AI Studio config

## Next Steps

Once deployed, proceed to the [Initial Configuration](./configuration.md) guide to set up Tyk AI Studio.
