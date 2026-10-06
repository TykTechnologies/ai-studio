# Tyk Gateway (open source) MCP Connections

AI Studio Enterprise can manage MCP proxies on **open-source Tyk Gateways**
directly, without a Tyk Dashboard. It is the same feature as the
[Tyk Dashboard MCP integration](./tyk-mcp-integration.md):

- discover MCP proxies and publish them in the AI Portal,
- register new MCP proxies from AI Studio or from community submissions,
- mint, rotate and revoke Tyk keys for Apps.

It talks to each gateway's Gateway API instead of a Dashboard.

It is meant for teams that want governed MCP access without running the full
Tyk API management stack.

::: warning MCP only
A Tyk Gateway connection manages **MCP proxies and keys and nothing else**.

- AI Studio never lists, creates, changes or deletes REST or GraphQL APIs,
  policies, certificates or OAuth clients on the gateways. Every call it can
  make is on a fixed allow-list in the code.
- AI Studio never changes or deletes an MCP proxy it did not create.
:::

## Requirements

- **Tyk Gateway 5.13 or later.** MCP proxies that front a remote MCP server
  are available on every Tyk Gateway licence.
- **The gateways' `secret`**, which AI Studio sends as `X-Tyk-Authorization`.
  - The secret grants full control of a gateway. AI Studio stores it
    encrypted (`TYK_AI_SECRET_KEY` must be set).
  - Expose the Gateway API on a `control_api_port` that is not reachable from
    the internet.
- **All nodes share one Redis.** That is where keys live, so a key AI Studio
  mints through one node works on every node. The probe proves this by
  writing a switched-off throwaway key through one node and reading it
  through another.
- **A writable `app_path` on every node.** The Gateway API writes proxy
  definitions there. The official image runs as a non-root user, so a fresh
  root-owned volume makes writes fail with `file object creation failed`.

Not available on a Tyk Gateway connection:

- REST-API-to-MCP proxies (Tyk Enterprise Edition)
- API templates
- policies
- MDCB data planes
- gateway segmentation tags
- the Tools "Import OpenAPI from Tyk" wizard

## How AI Studio keeps a cluster in step

An open-source gateway has no API that spans the cluster. A proxy written
through the Gateway API lands on the disk of the node that answered and is
served only after that node reloads. `/tyk/reload/group` reloads every node,
but each from its own disk.

AI Studio therefore:

1. **Keeps the desired state** of every proxy it creates, encrypted, in its
   own database.
2. **Finds the nodes** on every sync (see below) and records them under
   **Settings → Tyk Connections → (connection) → Gateway nodes**.
3. **Reconciles each node.** It writes the AI Studio proxies a node is
   missing or holds a stale copy of, removes AI Studio proxies that are no
   longer wanted, reloads the node, and records the node as *In sync*,
   *Pending*, *Unreachable* or *Gone*.
   - Proxies AI Studio did not create are never touched.
   - A definition edited on a node behind AI Studio's back is restored on the
     next sync.
4. **Writes through at once.** A registration or edit goes to every node
   immediately, so it is live without waiting for the next sync. Nodes that
   fail are retried by the sync.

AI Studio's proxies carry API ids of the form `studio-xxxxxxxxxx-...`. The
middle part is random per connection, so two AI Studio installations, or two
connections, sharing a cluster never remove each other's proxies.

### Finding the nodes

| Mode | Nodes |
|---|---|
| Single node | The connection URL. |
| List of nodes | The connection URL plus the URLs you list. |
| DNS | Every address the connection URL's hostname resolves to, re-resolved on every sync. Use a Kubernetes headless Service or a Compose service name. Each address is dialled directly while the URL, Host header and TLS name keep the hostname. |

**Shared storage.** If the nodes share `app_path` (a shared volume), tick
*The nodes share app_path*. AI Studio then writes each proxy through one node
and a group reload loads it everywhere. New nodes serve the proxies from
their first start.

**Autoscaling without shared storage.** A newly started node serves AI
Studio's proxies only after the next sync, so for up to one sync interval it
answers 404 on their listen paths. Use shared storage, or a short sync
interval, if your gateways autoscale.

### Proxies found on some nodes only

A proxy someone else created that is missing on some nodes is imported but
flagged *partial* (for example "On 1/2 nodes"):

- It can be published, but no keys are minted for it.
- Apps cannot be bound to it.

Clients would reach it on some nodes and get 404 on the others. AI Studio
does not copy proxies it did not create. Once every node serves it, the flag
clears on the next sync.

## Endpoint URLs in the portal

Clients call the gateways, not AI Studio. The portal URL of a proxy is built
from the connection's **public gateway base URL**: the address clients use,
usually the load balancer or ingress in front of the nodes, never a node's
Gateway API URL. A proxy with an enabled custom domain uses that host
instead. Per-tag URLs do not exist here; every node serves every proxy.

See [Endpoint URLs in the portal](./tyk-mcp-integration.md#endpoint-urls-in-the-portal)
for the rules.

## Keys carry their rights

A Dashboard connection mints keys from pinned policies. Gateway policies,
however, are files on each node's disk, so a key that names a policy some
node has not loaded fails on that node.

Keys AI Studio mints on a Tyk Gateway connection carry the rights
themselves instead: one `access_rights` entry per MCP proxy the App uses.
Keys live in the shared Redis, so they are valid on every node at once.

Each MCP server has a **Key access** section (replacing *Access policies*):

- **Allowed tools.** Keys may list and call only these tools, and `tools/list`
  shows only these. Empty means every tool the proxy exposes.
- **Rate limit and quota.** These count per App key and per server
  (`allowance_scope`). Empty means no limit.

Saving rewrites the live keys of every App that uses the server. Binding an
App to an additional server reaches a new proxy, so it waits for an
administrator's approval like any other widening change. Rights someone added
to a key on the gateway for other APIs are kept and reported as external.

Minted keys have no key-wide rate limit and no quota (`quota_max: -1`) beyond
what each server's key access sets.

## Setting up a connection

1. **Settings → Tyk Connections → Add connection.** Set *Connect to* to
   **Tyk Gateway (open source)**.
2. Enter the **Gateway API URL** of one node, the **gateway secret**, the
   **public gateway base URL** clients use, and how to find the nodes.
3. Pick a **trust mode**:
   - *Catalogue*: import the proxies.
   - *Broker*: also mint keys.
   - *Full*: also create proxies on every node.

   The trust mode caps what AI Studio does with the secret.
4. **Test connection**, save, and have a second administrator **activate**
   it.

The probe reports:

- every node's version,
- whether the secret is accepted,
- whether the nodes share Redis (broker mode needs it),
- whether a dry-run proxy validates (full mode).

## Registration

Registering an MCP server on a Tyk Gateway connection works as on a
Dashboard, with these differences:

- The **preview is validated by a gateway node** (the Gateway honours dry
  runs) and writes nothing.
- AI Studio sets `stripAuthorizationData` on the proxies it creates with
  access-key auth, so the client's Tyk key is not forwarded to the upstream
  MCP server.
- Upstream credentials go into the definition on every node's disk. Prefer a
  gateway secret reference as the value, for example
  `$secret_env.WEATHER_TOKEN` (read from the gateway's `TYK_SECRET_WEATHER_TOKEN`
  environment variable), over a literal token.

## API

| Endpoint | Purpose |
|---|---|
| `POST /api/v1/tyk-connections` with `"kind": "gateway"` | Create a Tyk Gateway connection (`gateway_discovery`, `gateway_node_urls`, `gateway_shared_storage`). |
| `GET /api/v1/tyk-connections/{id}/nodes` | The nodes and their state. |
| `PUT /api/v1/mcp-servers/{id}/key-access` | Set a server's key access (`allowed_tools`, `rate`, `per`, `quota_max`, `quota_renewal_rate`). |

## Local development

`tests/tykmcp/oss/docker-compose.yml` runs two open-source Gateway replicas
with separate `app_path` volumes and one Redis, plus a third replica that
joins on demand.

- `tests/tykmcp/oss/m0_verify.py` checks the Gateway behaviours this feature
  relies on.
- `tests/tykmcp/oss/live_e2e.py` drives AI Studio against the cluster end to
  end.
