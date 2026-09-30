---
title: "Edge Gateways"
weight: 7
---

# Edge Gateways

> **Note:** Edge Gateway management is an **Enterprise Edition** feature for hub-and-spoke deployments.

Edge Gateways are distributed Microgateway instances that connect to the Tyk AI Studio control plane. They process AI requests locally while receiving configuration from the central hub.

## Overview

In enterprise deployments, Tyk AI Studio supports a hub-and-spoke architecture:

- **Hub (Control Plane):** Central Tyk AI Studio instance managing configuration, policies, and analytics
- **Spoke (Edge Gateways):** Distributed Microgateway instances processing requests locally

This architecture enables:

- **Regional Compliance:** Keep data processing within specific geographic regions
- **Reduced Latency:** Route requests to the nearest edge instance
- **High Availability:** Continue processing even if the hub is temporarily unreachable
- **Multi-Tenancy:** Isolate resources between different teams or customers using namespaces

## Edge Gateway Management UI

The Edge Gateways page in the Admin UI provides visibility into all connected edge instances. Access it via **Admin > Edge Gateways** in the sidebar.

### Edge Gateway List

The list view displays all registered edge gateways with key status information:

| Column | Description |
|--------|-------------|
| **Edge ID** | Unique identifier for the edge gateway |
| **Namespace** | The namespace the edge belongs to (Enterprise) |
| **Connection** | Connection status based on heartbeat: Connected, Disconnected, or Stale |
| **Config Sync** | Whether the edge has the latest configuration |
| **Held by** | The AI Studio replica holding the edge's connection, by its label (for example `dashboard` or `mdcb-eu-1`), or its node ID when it has none. Shown only when the listed edges are spread over more than one replica; "(not responding)" marks a replica that has stopped refreshing its registration, and the edge reconnects elsewhere shortly |
| **Version** | Software version and build hash of the edge gateway |
| **Last Heartbeat** | Time since the last heartbeat was received |

### Edge Gateway Detail View

Click on any edge gateway to view detailed information:

- **Basic Information:** Edge ID, namespace, status, session ID, and the replica holding the edge's connection ("Held by")
- **Version Information:** Software version, build hash, and last heartbeat timestamp
- **Configuration Sync Status:** Detailed sync status including checksums and last sync acknowledgment
- **Timestamps:** When the edge was registered and last updated
- **Metadata:** Any custom metadata reported by the edge

## Configuration Synchronization

Tyk AI Studio uses a checksum-based system to track configuration synchronization between the control plane and edge gateways.

### How It Works

1. **Checksum Generation:** When configuration changes occur on the control plane, a SHA-256 checksum is computed from the serialized configuration snapshot
2. **Heartbeat Reporting:** Edge gateways report their loaded configuration checksum in each heartbeat
3. **Status Comparison:** The control plane compares reported checksums to determine sync status
4. **UI Notifications:** The admin UI displays sync status and notifies administrators when edges are out of sync

### Configuration Objects in Checksum

The configuration checksum includes objects that need to be synchronized to edge gateways for request processing:

- **LLM Configurations** - AI provider settings and credentials
- **Tools** - REST API tool configurations with OpenAPI specs and encrypted auth keys
- **Datasources** - Vector store configurations with encrypted connection strings and embedder credentials
- **Filters** - Request/response processing rules
- **Plugins** - Gateway plugin configurations
- **Model Prices** - Cost tracking configurations
- **Model Routers** - Request routing rules (Enterprise)
- **OAuth Clients & Access Tokens** - MCP authentication state for tool access via OAuth

Any create, update, or delete operation on these objects triggers a checksum recalculation.

Two properties keep the checksum meaningful:

- **Deterministic encryption.** Secrets in the snapshot (LLM API keys, tool auth keys, datasource credentials) are encrypted for the edge with AES-GCM using a nonce derived from the secret itself, so regenerating an unchanged configuration produces byte-identical ciphertext and the same checksum. A random nonce would make every regeneration look like a change.
- **Change detection.** When a recalculation yields the same checksum the control plane leaves the namespace status and edge sync states untouched and writes no audit entry. Only a change that alters the snapshot (including governed metadata fields marked *Sent to gateways*) marks edges as pending.

### Apps and Credentials

**Apps** are synced as part of the configuration snapshot but are **not** included in the checksum calculation. This is because Apps change frequently (users create and update them regularly), and including them in the checksum would cause unnecessary sync churn. App associations (LLM access, tool access, datasource access) are included in the snapshot to enable access control on edges.

**Credentials** (access tokens) are **not** pulled during the initial configuration snapshot. Instead, Microgateways use a **pull-on-miss** caching strategy:

1. When a gateway receives a request with an unknown access token, it contacts AI Studio to validate and fetch the credential.
2. The credential is then cached locally for subsequent requests.
3. This ensures the admin retains ongoing control — disabling a credential in AI Studio takes effect as soon as the gateway's cache expires or the next pull-on-miss occurs.

This approach balances performance (no need to sync every credential change) with security (admin can revoke access without waiting for a full config push).

### Sync Status Values

| Status | Description | UI Indicator |
|--------|-------------|--------------|
| **In Sync** | Edge has the current configuration | Green chip |
| **Pending** | Edge needs a configuration update | Yellow chip |
| **Stale** | Edge has been out of sync for >15 minutes | Orange chip |
| **Unknown** | Edge hasn't reported a checksum yet | Gray chip |

### Sync Status Banner

When any edge gateways are out of sync, a warning banner appears at the top of the admin UI. The banner:

- Says what is waiting: "3 changes not yet pushed to 1 edge gateway in namespace default" (the change count comes from the pending-changes lookup below; an older Studio falls back to the edge count alone)
- Opens the Push Configuration modal directly, and links to the Edge Gateways page
- Automatically disappears when all edges are synchronized
- After a push, refreshes at once and then every 3 seconds (for up to 30 seconds) until the edges have acknowledged

## Pushing Configuration

Configuration changes are pushed to edge gateways on-demand (not automatically) to ensure administrators maintain control over when changes are deployed.

### Push Configuration Modal

Click the **Push Configuration** button to open the push modal. You can choose to:

1. **Push to All Namespaces:** Sends configuration to all connected edge gateways
2. **Push to Specific Namespace:** Sends configuration only to edges in a selected namespace (Enterprise)

Before you confirm, the modal lists **what will be pushed** for the chosen scope: every object that was created, updated or deleted since the namespace's last push, grouped by type (LLM providers, Apps, Tools, Data sources, Filters, Model prices, Model routers, Plugins, OAuth clients, Access tokens), each with a link to its admin page where one exists. A summary line reads "12 changes since the last push at 13:12"; when nothing has changed the button becomes **Push anyway**, since a push can still be useful after an edge has been re-registered. With "All Namespaces" selected, each namespace gets its own collapsible block.

The Edge Gateways list and each gateway's detail page show **Last pushed HH:MM** for the namespace.

### Push Process

> **AI Studio 2.2 runs as a single instance.** Replicated AI Studio instances are not supported in 2.2: run one hot instance with an optional cold standby (see [Reference Architecture](./reference-architecture.md#run-studio-as-a-hot-cold-singleton)). Support for several active replicas is planned for 2.3; the notes on replicas below apply from then.

When you push configuration:

1. AI Studio records the push: one entry per target edge gateway, with a deadline (5 minutes). The modal shows which edges are connected now and warns about the ones that are not.
2. The AI Studio instance that holds each edge's connection sends it a reload request. From 2.3, with several AI Studio replicas behind a load balancer, it does not matter which replica you pushed from.
3. Each edge pulls the current configuration, applies it and answers **ready** (or **failed**, with the reason).
4. The modal follows the push until every edge has answered and shows each edge's outcome:
   - **Updated**: the edge loaded the configuration.
   - **Updated, with a warning**: the edge loaded a configuration that has since changed again; push once more.
   - **Failed**: the edge could not apply it (its error is shown), or three delivery attempts in a row were cut short.
   - **Timed out**: the edge did not connect, or did not finish, before the deadline.

You can close the modal at any time; the push carries on. Edges that are offline when you push receive it as soon as they reconnect, until the deadline (an edge that is restarting gets it as soon as it is up; edges from 2.2 and before, which drop a push that arrives while they are still starting, get it 10 seconds after they connect). The "not connected" warning stays only while an edge is still waiting to connect. When no edge can receive the push (every edge offline for more than 5 minutes, or none registered), the modal says so at its top and disables **Push** until you choose another target. If an edge's connection drops while it is reloading, or the AI Studio instance it was connected to stops, the push is sent again when the edge reconnects (from 2.3, to any replica). A namespace or "all" push leaves out edges that have been offline for more than 5 minutes and lists them. Community Edition shows the push's overall outcome; the per-edge breakdown is part of Enterprise Edition.

How an edge applies a snapshot:

- **In one transaction.** A snapshot that fails to apply leaves the previous configuration in place.
- **Apps and LLMs are updated in place.** An App or LLM the snapshot no longer has is retired: the edge stops serving it, but keeps the row, so its analytics and budget usage stay valid (Postgres edges enforce those references). An App that comes back is served again.
- **Apps newer than the snapshot are kept.** An edge can learn of an App from token validation after the snapshot was taken; the snapshot does not remove it.
- **Spend is never lowered.** The snapshot carries each App's spend as Studio knows it, which can lag behind the edge's own. The edge takes the higher of the two figures.

### Monitoring Push Results

After pushing configuration:

- The sync status banner updates within a few seconds
- Individual edge sync status is visible in the list and detail views
- Hover over the Config Sync chip to see checksum details

## Checksum Details

For debugging sync issues, the UI displays checksum information:

- **Loaded Config Checksum:** The checksum reported by the edge gateway
- **Expected Config Checksum:** The checksum expected by the control plane (shown when out of sync)
- **Loaded Config Version:** Version string of the edge's current configuration
- **Last Sync Acknowledgment:** Timestamp when the edge last confirmed receiving a configuration

Hover over the Config Sync status chip in the list view to see a truncated checksum comparison.

## Removing Edge Gateways

To remove an edge gateway entry from the control plane:

1. Click the three-dot menu (⋮) on the edge row, or go to the detail view
2. Select **Remove Entry**
3. Confirm the removal

> **Note:** This removes the entry from the control plane database. If the edge gateway is still running, it will re-register on its next connection attempt.

## API Endpoints

The Edge Gateway sync status can also be queried via the REST API:

### Get Sync Status Summary

```
GET /api/v1/sync/status
```

Returns sync status for all namespaces:

```json
{
  "data": [
    {
      "namespace": "default",
      "expected_checksum": "abc123...",
      "last_config_change": "2024-01-15T10:30:00Z",
      "synced_count": 3,
      "pending_count": 1,
      "stale_count": 0,
      "total_edges": 4
    }
  ],
  "has_pending": true
}
```

Each namespace summary also carries `last_push_at` (null until the first push).

The global/default namespace has one row whatever it is called: `""`, `global` and `default` are the same namespace to every sync endpoint (edges register under `default`; an unset `EDGE_NAMESPACE` on the microgateway means the same thing). A database from before this used `""`; its row is folded into `default` on startup.

### Get Namespace Sync Status

```
GET /api/v1/sync/status/:namespace
```

Returns detailed sync status for a specific namespace, including per-edge status.

### Get Pending Changes

```
GET /api/v1/sync/pending-changes?namespace=default
```

Returns what has changed in a namespace since its last push, as shown in the push modal's preview:

```json
{
  "data": {
    "namespace": "default",
    "since": "2024-01-15T10:30:00Z",
    "last_push_at": "2024-01-15T10:30:00Z",
    "baseline": "push",
    "total": 2,
    "changes": [
      { "type": "llm", "id": 3, "name": "OpenAI", "change": "updated", "at": "2024-01-15T11:02:00Z" },
      { "type": "app", "id": 9, "name": "Support bot", "change": "created", "at": "2024-01-15T10:45:00Z" }
    ]
  }
}
```

`changes` is capped at 200 entries; `total` is the real count. `baseline` says what `since` was taken from: `push` (the recorded last push), `edge_ack` (no push was ever recorded, for example on a database upgraded from before pushes were stamped, or when the edges only got their configuration when they registered: the latest time an edge confirmed it was in sync is used, whether or not that edge is connected now, and the UI says "since the last sync") or `none` (no edge ever confirmed; every object is listed as created).

A reload makes each edge pull the snapshot through `GetFullConfiguration`, which marks that edge in sync immediately rather than waiting for its next heartbeat; the other edges in the namespace stay pending until they pull.

### Push Configuration

```
POST /api/v1/edges/{edge_id}/reload          # one edge
POST /api/v1/namespaces/{namespace}/reload   # one namespace (Enterprise)
POST /api/v1/edges/reload-all                # every edge gateway
```

Each answers `202 Accepted` with the push operation: `operation_id`, `target_edges`, `targets` (each with `reachable` and, if not, the reason), `skipped` (edges left out because they have been offline for more than 5 minutes), `warnings` and `deadline_at`. It answers `404` for an unknown edge, `409` when there is nothing to push to, and `503` when this AI Studio runs no control server. On Enterprise, `reload-all` also returns `data.message`, `data.operations` and `data.operations_count` as 2.2 did (see the upgrade note below).

```
GET /api/v1/reload-operations/{operation_id}/status   # Enterprise
GET /api/v1/edges/reload-operations                  # the last day's pushes
```

The status reports the operation's `status` (`in_progress`, `succeeded`, `succeeded_with_warnings`, `partially_failed`, `failed`, `expired`), `progress`, `counts` per outcome, and `edges`: for each edge its `status`, the phase it last reported, `message`, `warning`, `attempts` and the history of every delivery attempt. From 2.3, any AI Studio replica answers it. The listing gives each push's status, counts and message.

#### Upgrading scripts from 2.2

The paths and the response envelope are unchanged, and every attribute 2.2 returned is still there. What scripts may need to change:

- **Final status.** 2.2 ended an operation with `completed`, `failed` or `timed_out`. Now it is `succeeded` or `succeeded_with_warnings` when every edge loaded the configuration, and `partially_failed`, `failed` or `expired` otherwise; `in_progress` until then. A script that waits for `completed` never finishes.
- **Operation IDs** start with `push-` (2.2: `reload-` or `edge-reload-`).
- **Reload-all (Enterprise)** starts one operation for every edge instead of one per namespace. `data.operations` has one entry per namespace pushed to, all with the same `operation_id`, so polling each entry still works.
- **Status codes.** A namespace with no edge to push to answers `409` (2.2: `404`); reload-all with no edge at all answers `409` (2.2: `202` with no operations); an edge that is offline answers `202` and the push waits for it (2.2: `500`).
- **The listing** (`GET /edges/reload-operations`) returns the last day's pushes from the database, with `counts` per outcome instead of `target_edges`; the status endpoint lists each push's edges.

## Connection Settings

### TLS

AI Studio serves the control connection with TLS unless `GRPC_TLS_INSECURE=true`, using `GRPC_TLS_CERT_PATH` and `GRPC_TLS_KEY_PATH`. The edge checks that certificate as follows:

| Edge setting | Purpose |
|---|---|
| `EDGE_TLS_CA_PATH` | PEM file of the CA(s) that issued the control plane's certificate, for a private CA. Unset uses the system roots. |
| `EDGE_TLS_SERVER_NAME` | The name the certificate is checked against, when it differs from the host in `EDGE_CONTROL_ENDPOINT` (an IP address, or a load balancer's internal name). |
| `EDGE_TLS_CERT_PATH`, `EDGE_TLS_KEY_PATH` | Client certificate for mutual TLS. |
| `EDGE_SKIP_TLS_VERIFY` | Skips verification entirely. Development only. |

The edge requires TLS 1.2 or later. An unreadable `EDGE_TLS_CA_PATH`, or one with no PEM certificates, stops the connection with an error naming the setting. Edges before 2026-10-01 checked that `EDGE_TLS_CA_PATH` existed but did not use it, so a certificate from a private CA only worked with `EDGE_SKIP_TLS_VERIFY`.

### Keepalive and message sizes

Edges ping the control plane every 30 seconds, also while no stream is open, and drop a connection whose ping is not answered within 5 seconds. AI Studio accepts pings as often as every 10 seconds and pings idle edges on the same schedule, so half-open connections behind a load balancer or NAT are noticed. Before 2026-10-01 AI Studio kept gRPC's default policy (one ping per 5 minutes, none without a stream) and could answer an edge's pings with `GOAWAY` (`too_many_pings`), which the edge sees as a disconnect.

| AI Studio setting | Default | Purpose |
|---|---|---|
| `GRPC_MAX_MESSAGE_SIZE` | 16 MB | Largest message in either direction: configuration snapshots out, analytics pulses and plugin payloads in. Keep it equal to the edges' `GRPC_MAX_MESSAGE_SIZE`. It used to be gRPC's 4 MB for messages from edges. |
| `GRPC_MAX_CONNECTION_AGE` | off | Closes each edge connection after about this long, so edges spread again over replicas behind a load balancer. Edges reconnect at once, and pushes in flight on a closed stream go back to pending. |
| `GRPC_MAX_CONNECTION_AGE_GRACE` | none | How long a connection past its age gets to finish before it is closed. |

## Troubleshooting

### Edge Shows "Disconnected"

- Check network connectivity between the edge and control plane
- Verify the edge gateway is running and healthy
- Check edge gateway logs for connection errors
- Ensure firewall rules allow gRPC traffic (default port 50051)
- After a control plane restart or outage, edges reconnect on their own, retrying with exponential backoff: `EDGE_RECONNECT_INTERVAL` (default 5 seconds) doubling to at most 30 seconds (up to 2.2, and until the fix of 2026-09-30, at most 5 minutes, so an edge could come back minutes after the control plane). Edges older than the fix of 2026-09-24 stopped retrying after the first failed attempt and needed a restart once the outage lasted longer than about 5 seconds.

### Edge Analytics Missing in AI Studio

Edges send analytics and spend to the control plane in the analytics pulse, which is loaded from `PLUGINS_CONFIG_PATH`. If an edge serves traffic but AI Studio shows none of it:

- Check that `PLUGINS_CONFIG_PATH` points at an existing pulse config; the startup path report flags it when it doesn't.
- Check the edge logs for `Failed to send analytics pulse`.
- Edges built before 2026-09-27 kept sending the pulse over their first connection to the control plane. After any AI Studio restart that connection was closed, so no further analytics arrived until the edge was restarted, and a graceful edge shutdown dropped what the pulse had buffered. Upgrade the edge, or restart it after each AI Studio restart.

### Edge Shows "Pending" After Push

- Wait a few seconds for the heartbeat cycle to complete
- Check if the edge is connected (not disconnected)
- Verify the edge gateway logs for configuration load errors
- Check if the edge has sufficient permissions to fetch configuration

### Checksum Mismatch Persists

- Try pushing configuration again
- Check for configuration validation errors in edge logs
- Verify the edge and control plane are running compatible versions
- Check for database replication lag if using PostgreSQL replication

### Sync Status Banner Doesn't Disappear

- Verify all edges have successfully loaded the new configuration
- Check for any disconnected edges that can't receive updates
- Refresh the page to ensure the latest status is displayed

## See Also

- [Observability](./observability.md) — metrics and traces emitted by each edge
- [Kubernetes / Helm Deployment](./deployment-helm-k8s.md) — running edges on Kubernetes, including scale-out
- [Running Alongside the Kubernetes Inference Gateway](./deployment-kubernetes-inference-gateway.md)
