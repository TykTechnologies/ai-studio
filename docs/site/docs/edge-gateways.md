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

### AI Studio is the control plane

Edge gateways register with AI Studio. The microgateway's own control mode (`GATEWAY_MODE=control`) is deprecated and will be removed in a future release. A microgateway started in that mode logs a deprecation warning, and in Enterprise builds it serves only the default namespace (`""`, `global` or `default`) and refuses edges in any other namespace with an error telling the operator to register against AI Studio. Namespaces are licensed by AI Studio, so they can only be served from there.

### No management API on edges

An edge gateway (`GATEWAY_MODE=edge`) does not serve the microgateway management API: every `/api/v1/...` path answers `404`. Its LLMs, Apps, credentials, filters and plugins come only from the control plane, over gRPC. The `mgw` CLI, which uses that API, works only against a standalone microgateway. An edge still serves the AI gateway endpoints (`/llm/`, `/ai/`, `/v1/`, `/tools/`, `/datasource/`, `/anthropic/`, `/router/`), custom plugin endpoints under `/plugins/`, `/health`, `/ready` and, when enabled, metrics.

Before 2.2.2 an edge served the full management API. One of its routes, `POST /api/v1/auth/token`, needed no credentials and issued a token with any scopes, including `admin`, which an attacker could use to load a malicious plugin onto the edge. Upgrade every edge, and keep edge ports reachable only by the clients that need the gateway. An upgraded edge applies AI Studio's configuration in full when it starts, replacing anything written through the old API.

### Edges enforce their own plugin source policy

An edge accepts only the plugin deployment types its own `PLUGINS_ALLOWED_SOURCES` allows (`oci`, `file`, `grpc`; all by default), whatever AI Studio allows. A plugin AI Studio pushes with a refused type is not loaded on that edge: the refusal is logged with the plugin ID and shows in the edge's plugin health, and the edge's other plugins load as usual. AI Studio does not know an edge's policy in advance. See [Hardening: restrict plugin sources](./plugins-security.md#hardening-restrict-plugin-sources).

## Edge Gateway Management UI

The Edge Gateways page in the Admin UI provides visibility into all connected edge instances. Access it via **Admin > Edge Gateways** in the sidebar.

### Edge Gateway List

The list view displays all registered edge gateways with key status information:

| Column | Description |
|--------|-------------|
| **Edge ID** | Unique identifier for the edge gateway |
| **Namespace** | The namespace the edge belongs to (Enterprise). **Unlicensed namespace** marks an edge whose namespace has no licensed slot yet (see [Licensed Namespaces](#licensed-namespaces-enterprise)) |
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

## Licensed Namespaces (Enterprise)

An Enterprise licence sets how many edge namespaces a deployment may use. The default namespace (`default`, `global` or an empty `EDGE_NAMESPACE`) is always available and does not count. Every other namespace holds one **slot** while it is in use.

- **Registering always works.** An edge in a namespace without a slot registers and stays connected, but gets no configuration: no snapshot, no pushes, and its API tokens are not validated. The Edge Gateways list marks it **Unlicensed namespace**, and the edge keeps any configuration it already had.
- **Waiting namespaces are admitted automatically.** Namespaces without a slot wait in the order they first had to wait. As soon as a slot is free (the licence allows more namespaces, an Owner releases a slot, or an idle slot frees itself), the longest-waiting namespace with an edge connected gets it, and AI Studio pushes the configuration to its edges straight away. No edge needs a restart or reconnect.
- **A slot follows a renamed edge.** When an edge is restarted with a new `EDGE_NAMESPACE`, and no other edge is connected in its old namespace, the old namespace's slot moves to the new one. If other edges are still connected in the old namespace, the new namespace needs a free slot or waits for one.
- **Idle slots free themselves.** A namespace with no connected edge for 24 hours loses its slot, which then goes to the next waiting namespace. An edge counts as connected while it is streaming to AI Studio and has sent a heartbeat in the last 90 seconds, so a crashed edge does not hold a slot.
- **An AI Studio outage costs no slot.** Idle time counts only while AI Studio is running: time while every AI Studio instance is down does not count, and after a restart no slot is freed and the queue is not reordered for the first few minutes, so edges that reconnect keep their own namespaces and waiting namespaces keep their place. A namespace that stops waiting (no edge connected for 24 hours) leaves the queue.
- **Owners manage slots on the License page.** It lists the namespaces holding a slot (with when an idle one frees), the waiting namespaces in order, and lets an Owner release a slot that has no connected edge (see [License and Quotas](./license.md)).

Upgrading an existing deployment: when the licence is first loaded, the namespaces that already have edges take the available slots in the order their first edge registered; the rest wait in that order.

A licence with fewer namespaces than are in use takes nothing away: the namespaces keep their slots until they go idle.

Community Edition uses the default namespace only and has no namespace limit.

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
- **Trusted prompt templates** - the approved [prompt templates](./guardrails.md#trusted-prompt-templates) of the Apps in the snapshot (see below)
- **Guardrail exceptions** - the approved [guardrail exceptions](./guardrails.md#exceptions-softening-a-verdict-for-one-app) of the Apps in the snapshot, each with its App's guardrail capability (see below)

Any create, update, or delete operation on these objects triggers a checksum recalculation. This includes changes a plugin saves through the Plugin SDK, such as its own configuration (for example a load balancer's upstream pools edited on the plugin's page) or the plugin chain of an LLM.

### Trusted prompt templates

The approved prompt templates of the Apps an edge serves (the snapshot's Apps: active, global or in the edge's namespace) travel in its snapshot, and, unlike the Apps themselves, they count in the checksum. Approving, retiring, editing or deleting a template marks the namespace's edges **Pending**. The edge registers them with its request guardrails, so a message is matched exactly as on AI Studio: the same matcher, the same residual, backstop and exemption cap.

- **Until the next push an edge keeps its old templates.** A template retired or deleted on AI Studio still exempts its constants on an edge that has not applied the new snapshot; one approved since is not yet used there (its messages are scored whole). Push after approving or retiring a template that matters.
- **Lapsing.** A template that has gone `expires_after_days` without a match is left out of the next snapshot. Edges report the templates that matched in their heartbeats (every 30 seconds), and AI Studio records those matches, so a template used only through edges does not lapse.
- **Apps learnt by pull-on-miss** (created after the edge's snapshot) have no templates on that edge until the next push: their messages are scored whole.
- Templates are held in memory on the edge; an edge always starts from a snapshot, so they are never stale across a restart. With no templates the edge screens requests exactly as before.

### Guardrail exceptions

The approved, unexpired [guardrail exceptions](./guardrails.md#exceptions-softening-a-verdict-for-one-app) of the Apps an edge serves travel in its snapshot, each with its App's guardrail capability, and they count in the checksum (the Apps themselves do not, which is why the capability travels with each exception). An exception outside its App's floor is left out. The edge applies them with the same rules as AI Studio and checks the floor again itself.

- **Every change reaches an edge only at the next push.** Approving, changing or retiring an exception, attesting or resetting an App's capability, and an exception lapsing all mark the namespace's edges **Pending**; until an administrator pushes, an edge keeps applying its old set.
- **Expiry.** AI Studio checks for exceptions that have gone `expires_after_days` without a hit every hour, so on an edge a lapsed exception stops applying after that check plus the next push.
- **Hits.** Edges report the exceptions that applied in their heartbeats, and AI Studio counts only exceptions the edge's own namespace snapshot carries. A reported exception counts once per heartbeat, so an exception's hit count is the number of heartbeats in which it applied, not of requests; its last hit keeps it from lapsing. Hits are attributed by the namespace the edge declares.
- An edge from before this release ignores exceptions and applies the filters' own actions.

Two properties keep the checksum meaningful:

- **Deterministic encryption.** Secrets in the snapshot (LLM API keys and metadata credentials, tool auth keys, datasource credentials) are encrypted for the edge with AES-GCM using a nonce derived from the secret itself, so regenerating an unchanged configuration produces byte-identical ciphertext and the same checksum. A random nonce would make every regeneration look like a change.
- **Change detection.** When a recalculation yields the same checksum the control plane leaves the namespace status and edge sync states untouched and writes no audit entry. Only a change that alters the snapshot (including governed metadata fields marked *Sent to gateways*) marks edges as pending.

### Apps and Credentials

**Apps** are synced as part of the configuration snapshot but are **not** included in the checksum calculation. This is because Apps change frequently (users create and update them regularly), and including them in the checksum would cause unnecessary sync churn. App associations (LLM access, tool access, datasource access) are included in the snapshot to enable access control on edges.

**Credentials** (access tokens) are **not** pulled during the initial configuration snapshot. Instead, Microgateways use a **pull-on-miss** caching strategy:

1. When a gateway receives a request with an unknown access token, it contacts AI Studio to validate and fetch the credential.
2. The credential is then cached locally for subsequent requests.
3. This ensures the admin retains ongoing control. From 2.3, when a key is created, regenerated, activated, deactivated or deleted, or its App is activated or deactivated, AI Studio tells every connected edge at once (the `credential.changed` event, carried to the edges of every replica), and the edge drops what it cached for that key: a revoked key is refused within about a second on a connected edge. An edge that is not connected when the change is made learns of it when its cache entry expires (`EDGE_TOKEN_CACHE_TTL`) or at the next pull-on-miss. The event names the key only by the hash edges hold for it, and needs `MICROGATEWAY_ENCRYPTION_KEY` set on AI Studio.

This approach balances performance (no need to sync every credential change) with security (admin can revoke access without waiting for a full config push).

#### Full credential sync (Enterprise)

An edge set to `EDGE_CREDENTIAL_SYNC=full` asks AI Studio for the App keys of its namespace in advance and validates them locally, so a key it has never seen is accepted while no control plane is reachable. AI Studio never sends a key: it sends a keyed hash of each one (HMAC-SHA-256 under a key derived from `MICROGATEWAY_ENCRYPTION_KEY`), with the key's id, whether it is active, and its App. An edge cannot turn a hash back into a key.

What AI Studio does:

- **First, after a reconnect to another control plane, and at least every 15 minutes:** a full list, in pages of 500 keys. The edge drops whatever the list does not contain.
- **In between:** only what changed since the last answer (a new key, a deactivated or re-activated one, a regenerated secret, a deleted key or App, a deactivated App).
- **Only for the edge's registered namespace**, plus global Apps, and only if the namespace is licensed. An edge that has not registered gets nothing until it does.

How long a revoked or deactivated key can still be accepted by an edge in this mode:

| Situation | At most |
|---|---|
| Edge connected | About a second: Studio's `credential.changed` notice makes the edge sync (after a random delay of up to half a second; at most about one such sync per second per edge) |
| Edge connected, the change notification was lost | Until the edge's next sync (its heartbeat interval) |
| Edge connected, and a sync missed the change (clocks more than 30 seconds apart between control planes or the database) | 15 minutes plus one heartbeat interval |
| Edge cannot reach any control plane | `EDGE_CREDENTIAL_SYNC_MAX_AGE` (24 hours by default) |

Keep the clocks of both control planes and the database within 30 seconds of each other.

Cost on AI Studio, measured with 50,000 keys: a full list is about 8 MB and takes about 4 seconds of one CPU core per edge, every 15 minutes, at a moment that differs from edge to edge (ten edges taking theirs at the same moment took about 12 seconds); memory use does not grow with the number of keys. For 100 edges that averages about half a core and 1 MB/s. A sync with nothing changed takes a few tens of milliseconds.

A Community AI Studio does not offer this; an edge that asks for it stays on pull-on-miss.

On the edge:

- **Settings.** `EDGE_CREDENTIAL_SYNC=full` (default `on_demand`) and `EDGE_CREDENTIAL_SYNC_MAX_AGE` (default `24h`, `0` for no limit). A Community edge given `full` logs that it is an Enterprise option and validates on demand.
- **When it syncs.** After every registration (at start, after a reconnect, after a move to another control plane), on every acknowledged heartbeat, and, within about a second, when AI Studio says a key the edge holds changed (the edge takes nothing from that message but the prompt to sync; a key it does not hold, such as another namespace's, starts no sync): the whole list the first time, the changes afterwards. A sync runs one at a time; one that fails is retried on the next heartbeat, and the edge keeps the list it has. An AI Studio that answers that it does not know the edge gets a new registration, then the sync again.
- **What it stores.** In its own database: each key's hash, id, App, and whether it is active (`synced_credentials`), and the Apps of those keys. Never a key. A page of the list is written in one transaction; an App that has not changed since the edge last stored it is not written again.
- **How it validates a key.**
  1. On the list, active, App active: accepted, without asking AI Studio.
  2. On the list, inactive or deleted, or its App inactive: refused (`401`, or `403` "app is inactive").
  3. Not on the list (created a moment ago): AI Studio is asked, as on demand, and the answer is added to the list. An explicit refusal from AI Studio always wins and updates the list.
  4. Not on the list, and no control plane answers: `503 credential_check_unavailable`, as on demand.
- **Past `EDGE_CREDENTIAL_SYNC_MAX_AGE`** without a successful sync, the edge stops trusting the list and validates as on demand until a sync succeeds.
- **Across a restart.** The list is in the edge's database, so an edge restarted with no control plane (with `EDGE_START_FROM_LAST_KNOWN_GOOD`) validates from it, within the max age. This needs a database that survives the restart: the StatefulSet form of the Helm chart with persistence.
- **Older or Community AI Studio.** It answers that it does not offer the sync: the edge logs that once and validates on demand.
- **Metrics.** `microgateway_credential_sync_last_success_timestamp` (when the last sync completed), `microgateway_credentials_synced` (usable keys on the list), and `microgateway_credential_validations_total{source}`: `local` (the list answered), `control` (AI Studio or its cached answer did) and `stale` (the list was past its max age). `microgateway_credential_sync_calls_total{trigger}` counts the syncs the edge asked for: `registration` (AI Studio accepted it), `heartbeat` (one acknowledged: one sync per heartbeat interval) and `event` (a change to a key on its list). A key change in another namespace adds nothing to `event`.

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

### Push reminder after saving

When you save something the edges use (an LLM provider, filter, tool, data source, app, catalogue, plugin configuration or a gateway-visible metadata field) and some edge is now out of date, a toast appears at the bottom right: **Saved, not live on edge gateways yet**. Its **Push Configuration** button opens the same push modal. The toast does not appear when no edge gateway is registered or every edge is already in sync. Running a filter test, or a plugin's own calls, doesn't trigger it.

## Pushing Configuration

Configuration changes are pushed to edge gateways on-demand (not automatically) to ensure administrators maintain control over when changes are deployed.

### Push Configuration Modal

Click the **Push Configuration** button to open the push modal. You can choose to:

1. **Push to All Namespaces:** Sends configuration to all connected edge gateways
2. **Push to Specific Namespace:** Sends configuration only to edges in a selected namespace (Enterprise)

Before you confirm, the modal lists **what will be pushed** for the chosen scope: every object that was created, updated or deleted since the namespace's last push, grouped by type (LLM providers, Apps, Tools, Data sources, Filters, Model prices, Model routers, Plugins, OAuth clients, Access tokens), each with a link to its admin page where one exists. A summary line reads "12 changes since the last push at 13:12"; when nothing has changed the button becomes **Push anyway**, since a push can still be useful after an edge has been re-registered. With "All Namespaces" selected, each namespace gets its own collapsible block.

The Edge Gateways list and each gateway's detail page show **Last pushed HH:MM** for the namespace.

### Push Process

> **Several replicas from 2.3.** The notes on replicas below apply to 2.3, where several AI Studio replicas can serve one database, in one site or in two (see [Several replicas](./reference-architecture.md#several-replicas-from-2-3) and [Active-Active](./deployment-active-active.md)). AI Studio 2.2 runs as a single instance (see [One instance, with a cold standby](./reference-architecture.md#one-instance-with-a-cold-standby)).

When you push configuration:

1. AI Studio records the push: one entry per target edge gateway, with a deadline (5 minutes). The modal shows which edges are connected now and warns about the ones that are not.
2. The AI Studio instance that holds each edge's connection sends it a reload request. From 2.3, with several AI Studio replicas behind a load balancer, it does not matter which replica you pushed from.
3. Each edge pulls the current configuration, applies it and answers **ready** (or **failed**, with the reason).
4. The modal follows the push until every edge has answered and shows each edge's outcome:
   - **Updated**: the edge loaded the configuration.
   - **Updated, with a warning**: the edge loaded a configuration that has since changed again; push once more.
   - **Failed**: the edge could not apply it (its error is shown), or three delivery attempts in a row were cut short.
   - **Timed out**: the edge did not connect, or did not finish, before the deadline.

You can close the modal at any time; the push carries on. Edges that are offline when you push receive it as soon as they reconnect, until the deadline (an edge that is restarting gets it as soon as it is up; edges from 2.2 and before, which drop a push that arrives while they are still starting, get it 10 seconds after they connect). The "not connected" warning stays only while an edge is still waiting to connect. When no edge can receive the push (every edge offline for more than 5 minutes, or none registered), the modal says so at its top and disables **Push** until you choose another target. If an edge's connection drops while it is reloading, or the AI Studio instance it was connected to stops, the push is sent again when the edge reconnects (from 2.3, to any replica). A namespace or "all" push leaves out edges that have been offline for more than 5 minutes, and (Enterprise) edges in a namespace without a licensed slot, and lists them with the reason. The "All Namespaces" choice in the modal counts the licensed namespaces only, and a namespace without a slot cannot be chosen. Community Edition shows the push's overall outcome; the per-edge breakdown is part of Enterprise Edition.

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

Secrets in a snapshot (LLM API keys, signing secrets, plugin and guardrail credentials) are encrypted with a fresh random nonce each time a snapshot is built. The checksum covers a keyed digest of each secret rather than its ciphertext, so it changes when configuration changes, including a rotated secret, and stays the same otherwise.

## FIPS 140-3 Status

AI Studio and every microgateway report their FIPS 140-3 status: whether FIPS mode is on, the Go Cryptographic Module version, and whether non-approved algorithms are refused (`GODEBUG=fips140=only`).

- **At startup**, each logs a `FIPS 140-3` line with `enabled`, `version` and `enforced`.
- **`GET /health`** on AI Studio and on a microgateway includes the same fields:

  ```json
  {"status": "ok", "fips": {"enabled": true, "version": "v1.0.0", "enforced": false}}
  ```

  A standard build reports `"enabled": false` and `"version": "latest"`. A FIPS build started with `GODEBUG=fips140=off` reports `"enabled": false` with its module version, so check this field rather than the package name.
- **Edge gateways** send their status with registration and heartbeats. The edge list shows it under the version, and the detail view in **FIPS 140-3 (reported by edge)**: `On (v1.0.0)` (`, enforced` in strict mode), `Off`, or `Unknown` for an edge released before this field. The edge API returns it as `attributes.fips` (`null` when unknown). The status is what the edge reports, not something the hub verifies; a reported version longer than 64 characters or outside `[A-Za-z0-9.+_-]` is treated as unknown.

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

### Several control planes

An edge can be given more than one control plane, for example one per site when AI Studio runs in two sites over one replicated database (see [Active-Active (Two Sites)](./deployment-active-active.md)):

```bash
# Ordered: the edge's own site first
CONTROL_ENDPOINTS=cp-a.example.com:50051,cp-b.example.com:50051
```

| Edge setting | Default | Purpose |
|---|---|---|
| `CONTROL_ENDPOINTS` | unset | Ordered, comma-separated list of control planes (`host:port`). `CONTROL_ENDPOINT` still works and is a list of one; setting both stops the edge at start-up with an error. |
| `EDGE_CONTROL_FAILBACK_INTERVAL` | off | How often an edge that is not on its first endpoint tries to return to it. |

How an edge uses the list:

- **At start** it tries each endpoint once, in order, and takes the first that accepts its registration. If none does, it exits, as an edge with one endpoint does.
- **Failover.** When the connection to its control plane fails (the dial, the registration or the stream), the edge tries the other endpoints straight away, in list order after the one it was on. Only when every endpoint has failed does it back off (`EDGE_RECONNECT_INTERVAL`, doubling to at most 30 seconds), and it then tries the whole list, from the first endpoint, once per backoff. An endpoint that does not answer at all is given 5 seconds before the edge moves on.
- **While it fails over** the edge keeps serving, as in any control-plane outage. Token validation, the analytics pulse and plugin payloads use the new connection as soon as it is up. A configuration push in flight is delivered again by the control plane the edge arrives at.
- **Failback.** An edge stays on the control plane it moved to until that connection ends. To have edges return to their first endpoint on their own, set `EDGE_CONTROL_FAILBACK_INTERVAL` (for example `5m`): an edge that is not on its first endpoint then tries it at that interval and moves when it accepts a registration. Requests are not dropped by the move: calls in flight finish on the old connection, which stays open for 30 seconds. Alternatively, AI Studio's `GRPC_MAX_CONNECTION_AGE` ends every edge connection on a schedule, and an edge whose connection ends tries the endpoints after the one it was on first.
- **A lost network is noticed by the keepalive** (see below): an edge whose control plane disappears without closing the connection moves on after about 35 seconds. Token validation does not wait for that; each call has its own 5 second limit.

All endpoints must be replicas of one AI Studio deployment (one database), with the same `GRPC_AUTH_TOKEN` and `MICROGATEWAY_ENCRYPTION_KEY`. A single DNS name or load balancer in front of several control planes keeps working as before, with `CONTROL_ENDPOINT`; the list is for when the edge itself should choose.

The endpoint an edge is on is shown in its log (`Connected to control plane`, with `control_endpoint`), in the body of its `/ready` endpoint (`control_endpoint`, `control_connected`; the probe stays 200 while the edge has no control plane) and in the metric `microgateway_control_connected{endpoint="..."}`, which is 1 for the endpoint in use and 0 for the others.

### Starting without a control plane

By default an edge exits if no control plane accepts it at start-up. With `EDGE_START_FROM_LAST_KNOWN_GOOD=true` it can start from the configuration it last applied instead:

| Edge setting | Default | Purpose |
|---|---|---|
| `EDGE_START_FROM_LAST_KNOWN_GOOD` | `false` | Start from the stored configuration snapshot when no control plane answers at start-up. |
| `EDGE_STARTUP_CONTROL_TIMEOUT` | `30s` | How long the edge keeps trying its control endpoints first. |
| `EDGE_LAST_KNOWN_GOOD_MAX_AGE` | `72h` | The longest the edge may have been without a control plane and still start this way. `0` means no limit. |

- **What is stored.** Every edge keeps the snapshot it last applied in its own database (table `applied_snapshots`), encrypted with a key derived from its `ENCRYPTION_KEY`, whether or not this option is on. So the option can be turned on later, and it works at the next restart.
- **What happens.** The edge waits `EDGE_STARTUP_CONTROL_TIMEOUT` for a control plane, then applies the stored snapshot exactly as it would one from AI Studio (LLMs, Apps, filters, plugins, and what an edge holds only in memory, such as trusted prompt templates and guardrail exceptions), starts serving, and keeps trying to connect. It logs a warning with the age of the snapshot. `/ready` answers 200 with `config_source: last_known_good`, `microgateway_config_source{source="last_known_good"}` is 1, and `microgateway_config_age_seconds` shows how long the edge has been without a control plane. On the first registration a control plane accepts, the edge applies the live configuration and reports `control` again; AI Studio shows it In Sync or Pending as usual.
- **When it refuses.** The edge exits with the reason, as it does without the option, when no snapshot is stored (a new or lost database), when the snapshot is for another `EDGE_NAMESPACE`, when it does not decrypt with this `ENCRYPTION_KEY`, when the edge has been without a control plane for longer than `EDGE_LAST_KNOWN_GOOD_MAX_AGE`, or when the stored snapshot's time is in the future by the edge's clock (the clock was set back, so no age can be worked out: correct the clock, or let the edge connect to a control plane once). That age is counted from the edge's last contact with a control plane (a registration accepted or a heartbeat acknowledged), not from when the configuration last changed: an edge with a stable configuration that was connected an hour ago is not stale.
- **What it means.** An edge started this way serves the configuration it last knew: an LLM, App or filter removed in AI Studio since then keeps working on that edge until it reconnects. It is the same trade as `EDGE_TOKEN_CACHE_STALE_GRACE`, which is why both are off by default.
- **Keys.** With credential validation on demand (the default), the edge **cannot validate any key** until a control plane answers: it remembers AI Studio's answers only in memory, so after the restart every request gets `503` with `Retry-After` (`credential_check_unavailable`), which SDKs retry. `EDGE_TOKEN_CACHE_STALE_GRACE` does not change that: the cache is empty. With [full credential sync](#full-credential-sync-enterprise) (`EDGE_CREDENTIAL_SYNC=full`, Enterprise) the edge validates keys from the hashed list in its database, so it serves them; a key revoked in the meantime then keeps working until the list is older than `EDGE_CREDENTIAL_SYNC_MAX_AGE` (24 hours).
- **The rest.** Budgets use the edge's own counters and stored budget blocks. Heartbeats and plugin payloads queue as in any outage. Analytics wait in the edge's in-memory backlog, up to `max_backlog_bytes` (see [Analytics during a control-plane outage](#analytics-during-a-control-plane-outage)).
- **What it needs.** A database that survives the restart. A container restart keeps an `emptyDir`; a rescheduled pod does not, so on Kubernetes use the StatefulSet form of the chart with persistence.

### Analytics during a control-plane outage

An edge keeps serving while no control plane answers, and keeps what it records about that traffic for AI Studio in a backlog, up to a bound you set.

- **What is kept.** The records for AI Studio (one usage record, one budget record and one proxy summary per request, plus compliance events and tool calls) wait in the edge's memory. Each pulse is tried `max_retries` + 1 times, `retry_interval_secs` apart (4 attempts over 15 seconds with the packaged settings). When that fails, the pulse and everything buffered since go into the **backlog**: serialized pulses, oldest first, up to `max_backlog_bytes` (256 MiB by default). Without stored request and response bodies a request takes about 280 bytes there, so the default keeps about 900,000 requests: 5 minutes at 3,000 requests per second.
- **What AI Studio gets afterwards.** Everything in the backlog, then what was buffered, one pulse at a time, oldest first, each pulse at most `max_buffer_size` records and 8 MB. Each pulse keeps the sequence number it was first sent with, and AI Studio stores a pulse once, so one sent again after a lost acknowledgement is not counted twice.
- **It is the same when AI Studio answers but cannot store** (its database is failing over): the pulses wait in the backlog until it can.
- **What is lost.** Records beyond `max_backlog_bytes`, the oldest first, and the whole backlog if the edge restarts or its pod is rescheduled during the outage: the backlog is in memory, and the edge does not resend from the copy of the analytics in its own database. Records older than `edge_retention_hours` (24) are dropped too.
- **Memory.** During an outage, and while the backlog is sent afterwards, an edge holds the backlog, up to `max_backlog_bytes`, plus at most about two batches of records in their in-memory form (about 2 KB per request): the batch being sent and the one filling up, each at most `max_buffer_size` records (about 7 MB at the packaged 10,000). From a pulse's first failed attempt, and for as long as the backlog is not empty, a full batch goes into the backlog serialized instead of waiting in memory, so neither the retries nor the length of the drain add to this. Measured at 3,000 requests per second through a 3.5-minute outage and the drain, with the packaged settings: the analytics' live memory peaked at the backlog plus 17 MB (81 MB with a full 64 MiB backlog). On top of the live memory the Go runtime keeps room for garbage: up to about 256 MB with the gateway's own tuning, held under 90% of the memory limit when there is one. Size the edge's memory (`resources.limits.memory` in the Helm chart) for its working set plus `max_backlog_bytes` plus about 50 MB, or lower the bound. With stored request and response bodies (`ANALYTICS_STORE_REQUESTS`, `ANALYTICS_STORE_RESPONSES`, forwarded with `include_request_response_data`), the backlog holds those bodies too: more bytes per request, and prompts and completions kept in the edge's memory for as long as the outage lasts.
- **How you see it.** `microgateway_analytics_pulse_backlog_bytes` and `microgateway_analytics_pulse_backlog_batches` on `/metrics` show what is waiting. What is discarded is logged (`Analytics records discarded`, with `dropped_over_backlog` and the reason pulses were failing) and counted in `microgateway_analytics_pulse_dropped_records_total{reason="over_backlog"}`. It counts records, not requests: a proxied request makes three (its usage record, budget record and proxy summary), so 3,000 dropped records are about 1,000 requests. Alert on that counter increasing, and on the backlog approaching its bound.
- **Budgets.** Each edge enforces an App's or LLM's budget on its own count of what it served, which is complete, or on AI Studio's total when that is higher. AI Studio's total catches up when the backlog is delivered. Only what was discarded is missing from it for good, and an App can overrun its budget by what other edges served and could not keep.
- **Settings.** `max_backlog_bytes` in the pulse file (`analyticsPulse.maxBacklogBytes` in the Helm chart). **Two defaults:** the microgateway keeps up to 256 MiB when the pulse file does not set it, but the Helm chart sets 64 MiB (about 230,000 requests: 75 seconds at 3,000 requests per second), because the chart's default memory limit is 512Mi and a full 256 MiB backlog on top of an edge's working set would not fit. On Kubernetes, raise `analyticsPulse.maxBacklogBytes` and `resources.limits.memory` together. When the backlog first discards records in an outage, the edge's log says how to raise the bound. `0` turns the backlog off: the edge then keeps only the newest `max_buffer_size` records through an outage, as before 2.3, and counts the rest as `reason="over_cap"`. `max_buffer_size` keeps its meaning: the size at which a filling buffer is sent early and the most records one pulse carries.

### The edge token is a hub-wide secret

Every edge authenticates to AI Studio with the same token: `EDGE_AUTH_TOKEN` on the edge, matching `GRPC_AUTH_TOKEN` (or `GRPC_AUTH_TOKEN_NEXT` during a rotation) on AI Studio. The token is not bound to an edge or a namespace, so whoever holds it can:

- register under any edge ID, including one that already exists;
- claim any namespace and receive that namespace's configuration snapshot;
- report analytics, budget usage and agent-trace captures for that namespace's Apps;
- on Enterprise, receive the hashes of that namespace's App keys ([full credential sync](#full-credential-sync-enterprise)). These are not keys and cannot be turned into keys.

The snapshot carries the namespace's secrets. Most are encrypted with `MICROGATEWAY_ENCRYPTION_KEY`, which every edge also holds, so the token plus that key decrypts them:

- LLM provider API keys, each LLM's resource-signing secrets, and from 2.3.0 the credentials in an LLM's metadata (fields whose name contains `key`, `secret` or `token`, such as a Bedrock LLM's AWS keys). Edges store them encrypted too. In the metadata itself those fields read `refused-credential-reference`, so an edge older than 2.3.0, which reads only the metadata, fails Bedrock calls instead of using another credential: upgrade edges with AI Studio;
- tool authentication keys;
- datasource connection strings, vector store keys and embedder API keys, including semantic router embedders;
- from 2.3.0, guardrail provider credentials (the connection fields a provider marks secret, such as `api_key`; unknown fields count as secret). In the guardrail's configuration those fields read `refused-credential-reference` and the fail mode reads `closed`, so an edge older than 2.3.0 blocks the traffic such a guardrail screens rather than passing it unscreened. Upgrade edges together with the hub: pre-2.3 edges block guardrailed traffic until upgraded;
- top-level `$SECRET/` references in the configuration of a plugin that runs on edges, resolved;
- OAuth access tokens issued to MCP clients;
- while agent tracing runs, the tracing fingerprint (HMAC) key.

**Plugin configuration is not treated as secret.** Apart from the resolved `$SECRET/` references above, every active plugin's configuration travels in the snapshot without that encryption, protected only by the connection, so the token alone is enough to read it. It is also stored as is in AI Studio's database and each edge's database, shown to administrators with plugin access in the UI and the API, and passed to the plugin process. Never put a literal credential in plugin configuration: store it in the secret store and reference it with a top-level `$SECRET/NAME` value (see [Plugin Security](./plugins-security.md#plugin-configuration-and-secrets)). OAuth client secrets (stored as hashes) and App API tokens are not in the snapshot.

Treat the token and `MICROGATEWAY_ENCRYPTION_KEY` as secrets for the whole hub, not for one edge: give them only to edges you control, and store them like a database password.

If an edge host is compromised:

- rotate the edge token, with `GRPC_AUTH_TOKEN_NEXT`, and `MICROGATEWAY_ENCRYPTION_KEY`;
- App keys need no rotation for this: the edge holds only their hashes ([full credential sync](#full-credential-sync-enterprise)), which cannot be turned into keys;
- in every namespace the edge served, rotate the LLM provider keys and each LLM's resource-signing secrets, tool, datasource, embedder and guardrail provider credentials, and any secrets in plugin configuration;
- revoke the OAuth access tokens issued to MCP clients in those namespaces;
- optionally, rotate `AGENT_TRACING_KEY` (keeping the old value as `AGENT_TRACING_KEY_PREVIOUS` until payload retention has passed) if forged links into genuine agent sessions matter to you. Fingerprints and exported `user.id` pseudonyms change at the rotation, so sessions that span it lose cross-gateway correlation.

Per-edge credentials, bound to an edge ID and an admin-assigned namespace, are planned separately.

### TLS

AI Studio serves the control connection with TLS unless `GRPC_TLS_INSECURE=true`, using `GRPC_TLS_CERT_PATH` and `GRPC_TLS_KEY_PATH`. The edge checks that certificate as follows:

| Edge setting | Purpose |
|---|---|
| `EDGE_TLS_CA_PATH` | PEM file of the CA(s) that issued the control plane's certificate, for a private CA. Unset uses the system roots. |
| `EDGE_TLS_SERVER_NAME` | The name the certificate is checked against, when it differs from the host of the control endpoint (an IP address, or a load balancer's internal name). With `CONTROL_ENDPOINTS`, each endpoint's certificate is checked against that endpoint's own host; setting this name applies it to every endpoint. |
| `EDGE_TLS_CERT_PATH`, `EDGE_TLS_KEY_PATH` | Client certificate for mutual TLS. |
| `EDGE_SKIP_TLS_VERIFY` | Skips verification entirely. Development only. |

The CA and the client certificate are the same for every endpoint of `CONTROL_ENDPOINTS`. The edge requires TLS 1.2 or later. An unreadable `EDGE_TLS_CA_PATH`, or one with no PEM certificates, stops the connection with an error naming the setting. Edges before 2026-10-01 checked that `EDGE_TLS_CA_PATH` existed but did not use it, so a certificate from a private CA only worked with `EDGE_SKIP_TLS_VERIFY`.

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
- An edge with several control planes (`CONTROL_ENDPOINTS`) shows as connected on whichever replica holds its stream; see [Several control planes](#several-control-planes).
- After a control plane restart or outage, edges reconnect on their own, retrying with exponential backoff: `EDGE_RECONNECT_INTERVAL` (default 5 seconds) doubling to at most 30 seconds (up to 2.2, and until the fix of 2026-09-30, at most 5 minutes, so an edge could come back minutes after the control plane). Edges older than the fix of 2026-09-24 stopped retrying after the first failed attempt and needed a restart once the outage lasted longer than about 5 seconds.

### Edge Analytics Missing in AI Studio

Edges send analytics and spend to the control plane in the analytics pulse, which is loaded from `PLUGINS_CONFIG_PATH`. If an edge serves traffic but AI Studio shows none of it:

- Check that `PLUGINS_CONFIG_PATH` points at an existing pulse config; the startup path report flags it when it doesn't.
- Check the edge logs for `Failed to send analytics pulse`.
- From 2.3, AI Studio acknowledges a pulse only once it is stored, and stores each pulse once. An edge whose pulse is not acknowledged (the control plane went away, or could not write to its database) keeps that batch and sends it again, unchanged, with the next pulse, for up to `edge_retention_hours` (24 by default). If the first attempt was in fact stored, AI Studio recognises the repeat and does not store it again, so a control-plane failover does not double-count analytics or spend.
- Edges built before 2026-09-27 kept sending the pulse over their first connection to the control plane. After any AI Studio restart that connection was closed, so no further analytics arrived until the edge was restarted, and a graceful edge shutdown dropped what the pulse had buffered. Upgrade the edge, or restart it after each AI Studio restart.

### Configuration Entries an Edge Cannot Store

Edges keep some keys unique: LLM and tool slugs, model price model and vendor pairs, router slugs per namespace, OAuth client IDs and access token hashes. When a snapshot repeats one of those keys, the edge leaves out each entry that repeats it, keeping the one with the lowest ID. It also drops the references other entries hold to what it left out. For example, an App's grant to a left-out tool is dropped instead of reaching the tool that kept the slug. The rest of the configuration is applied. Filters are never left out, and an LLM that is kept keeps all of its filters and plugins.

Attachments that an edge does not receive are handled the same way. An edge receives the filters and plugins of its own namespace and the global ones, and nothing filed under another namespace. So:

- An LLM or tool attached to a filter or plugin that the edge does not receive is left out, and never serves without it. For example, a global LLM using a filter or plugin filed under another namespace is left out on that namespace's edges.
- An endpoint (tool, datasource, router or plugin endpoint) whose auth plugin the edge does not receive is left out, instead of falling back to App keys.
- A plugin left out because of this takes the LLMs that run it along.
- Deactivating or deleting a filter or plugin detaches it, as before. It does not leave anything out.

To fix such an entry, file the filter or plugin under the global namespace or the LLM's namespace, or detach it.

From 2.3, AI Studio refuses a tool or LLM name whose slug another tool or LLM already has (see [Tools](./tools.md) and [LLM Management](./llm-management.md)). Duplicates stored by earlier versions are not changed. Instead:

- AI Studio leaves them out of every snapshot and logs `Configuration snapshot for edges: tool 3 left out: slug "page-fetcher" is already used by tool 2`, at most once an hour per entry.
- The edge logs `Configuration entry not applied: …` for each entry left out, whether AI Studio or the edge left it out.
- The edge reports a checksum of its own for a configuration applied without entries, so it shows as **Pending** until the duplicate is renamed.
- A push to it ends `succeeded_with_warning`, and its `warning` lists the entries.

Rename the duplicate, then push again.

An edge also reports, in its heartbeats, the configuration it applied rather than the last one it received, so a snapshot it failed to apply no longer shows as in sync.

### Edge Shows "Pending" After Push

- Wait a few seconds for the heartbeat cycle to complete
- Check the push's `warning` and the edge log for entries it could not store (see above)
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
- [Active-Active (Two Sites)](./deployment-active-active.md) — edges with a control plane in each of two sites
- [Kubernetes / Helm Deployment](./deployment-helm-k8s.md) — running edges on Kubernetes, including scale-out
- [Running Alongside the Kubernetes Inference Gateway](./deployment-kubernetes-inference-gateway.md)
