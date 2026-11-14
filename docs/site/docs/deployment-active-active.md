---
title: "Active-Active (Two Sites)"
weight: 4
---

# Active-Active Deployment (Two Sites)

From 2.3, AI Studio can run in two sites at once. Both sites serve the admin UI, the API, the portal, chat and edge gateways, and either site can be lost, PostgreSQL node included, while the other carries on. Edges keep serving through a control-plane outage, move to the other site's control plane on their own, and can start again while no control plane answers.

This page is the topology: what it needs from the database and the network, what must be the same in both sites, what happens when something fails, and the runbooks for operating it. The two-site development stack (`make dev-aa`) is the worked example throughout: it is the topology in miniature, and every runbook below can be tried on it (see [Try it on the development stack](#try-it-on-the-development-stack)).

Everything here is in both editions except **full credential sync**, which is Enterprise.

## What "active-active" means here

```
              Site A                                   Site B
  ┌──────────────────────────────┐        ┌──────────────────────────────┐
  │ Studio replica(s)  label: a  │        │ Studio replica(s)  label: b  │
  │  UI/API :8080 · gRPC :50051  │        │  UI/API :8080 · gRPC :50051  │
  └───────┬──────────────────────┘        └──────────────────────┬───────┘
          │   multi-host DSN, target_session_attrs=read-write    │
          ▼                                                      ▼
  ┌───────────────┐   synchronous streaming replication   ┌───────────────┐
  │ PostgreSQL    │◄─────────────────────────────────────►│ PostgreSQL    │
  │ primary       │        automatic failover             │ hot standby   │
  └───────────────┘     (needs a witness, see below)      └───────────────┘

  Edges: CONTROL_ENDPOINTS=cp-a:50051,cp-b:50051  (own site first)
         own database on a persistent volume
```

- **Two active control planes over one logical database.** Every Studio replica, in either site, writes to whichever PostgreSQL node is the primary at the time. The replicas coordinate through that database exactly as several replicas in one site do (see [Several replicas](./reference-architecture.md#several-replicas-from-2-3)): configuration pushes any replica delivers, a leader for the work that must happen once, edge connections held by whichever replica an edge reached.
- **One primary, one hot standby.** The standby is not used for reads. It is there to be promoted.
- **Multi-primary PostgreSQL is not supported.** BDR, bidirectional logical replication and other setups that accept writes on both nodes break Studio: it relies on sequences for ids, on advisory locks for migrations and singleton work, and on one database for the leader lease and the cluster event log. Two primaries would hand out the same ids and let two replicas each believe they lead.
- **Failover is the database layer's job.** Studio follows a failover without a restart; it does not decide one.

## Database requirements

- **PostgreSQL with physical streaming replication**: a primary in one site and a hot standby in the other (`hot_standby = on`). The failover behaviour on this page is tested with PostgreSQL 16 (the failover test rig and CI run `postgres:16-alpine`); AI Studio's general requirement is PostgreSQL 14 or later (see [Reference Architecture](./reference-architecture.md#database)), not separately tested for failover.
- **Synchronous replication between the sites** is the recommended setting: on the primary, `synchronous_commit = on` and `synchronous_standby_names` naming the other site's standby. A commit is then acknowledged only once the standby has it, so nothing acknowledged is lost when the primary's site is.
  - **With asynchronous replication** a failover can lose the last transactions the primary acknowledged. An App key created and shown to a user just before the failover can be gone afterwards, and an edge that validated it may have cached it. Studio copes with the database going back in time (its event log and budget sync recover), but the lost writes are lost.
  - **Losing the standby's site.** With synchronous replication every commit on the primary waits for the standby, so when the standby's site is lost the failover manager must drop the synchronous requirement (or hold writes until the standby is back). Managed services and the operators below do this; in a hand-built setup it is a runbook step.
- **Automatic failover** from one of:
  - a **managed PostgreSQL** with cross-zone or cross-region high availability;
  - **CloudNativePG** on Kubernetes, across two zones or clusters;
  - **Patroni**, with its distributed configuration store spread over three locations.
- **Clocks.** Keep the database nodes and the Studio replicas on NTP. A failover to a node whose clock differs by up to 30 seconds keeps one leader at a time (the leader lease's clock allowance; tested with the new primary's clock 30 seconds ahead and 30 seconds behind). Full credential sync assumes the replicas' and the database's clocks are within 30 seconds of each other. Larger differences are not covered.

### Two sites need a third location to fail over automatically

Two sites cannot safely decide a failover on their own. When site A loses contact with site B, it cannot tell "B is down" from "the link is down": if each side promoted its own node, both would accept writes (split brain), and the writes of one side would be lost when they are joined again. A safe automatic failover needs a **witness in a third location** that breaks the tie:

- a managed service's own control plane;
- a Kubernetes API server, or a CloudNativePG or Patroni quorum, spread over three zones;
- a witness node (etcd, Consul or ZooKeeper member, a Patroni DCS member) in a third site.

**Without a third location, failover is manual.** An operator decides that a site is lost and promotes the standby ([Manual promotion](#manual-promotion)). Until then there is no primary: a write is held for at most `DATABASE_WRITE_RETRY_WINDOW` (15 seconds), then answered with a retryable `503`, and it is not queued for later (see [Requests while there is no primary](#requests-while-there-is-no-primary)). Studio never promotes anything, and it refuses to treat a read-only node as a primary: it neither writes to nor migrates a standby.

### Connection string

Give every replica, in both sites, the same multi-host connection string with `target_session_attrs=read-write`, so the driver connects to whichever node takes writes:

```
DATABASE_URL=postgres://studio:secret@pg-a.example.internal:5432,pg-b.example.internal:5432/studio?target_session_attrs=read-write&sslmode=require&connect_timeout=3
```

- **List your own site's node first.** Hosts are tried in order, and a node that does not answer at all costs the full connect timeout before the next is tried.
- **Set a short connect timeout** (`connect_timeout=3` in the URL, or `DATABASE_CONNECT_TIMEOUT=3s`) and an explicit `sslmode` (`require` or `verify-full`; the default, `prefer`, tries every host twice).
- **One name in front of both nodes** (a virtual IP, a DNS record or a service your failover manager moves, such as CloudNativePG's `-rw` service) works too: Studio notices a demoted primary behind an unchanged address and reconnects.
- The pool settings (`DATABASE_MAX_OPEN_CONNS`, `DATABASE_MAX_IDLE_CONNS`, `DATABASE_CONN_MAX_LIFETIME`, `DATABASE_CONNECT_TIMEOUT`), how Studio leaves a demoted primary, and how long an unreachable first host costs are in [Postgres with a standby (failover)](./configuration.md#postgres-with-a-standby-failover).

**PgBouncer.** Studio works behind PgBouncer in transaction mode with `default_query_exec_mode=simple_protocol` in the URL, but LISTEN does not pass through transaction pooling: the cluster event log and push delivery then poll (at most a second late), and the PostgreSQL chat queue does not work, so use NATS for chat. Put PgBouncer in front of the database role your failover manager moves, not in front of one node. See [Behind PgBouncer](./configuration.md#behind-pgbouncer).

## What must be the same in both sites

| What | Why | Checked by |
|---|---|---|
| `TYK_AI_SECRET_KEY` | Encrypts secrets at rest, and App keys are found by a hash keyed with it: with another value no App key validates | Cluster status warns when replicas differ |
| `MICROGATEWAY_ENCRYPTION_KEY` (32 characters) | Encrypts configuration snapshots for edges, and keys the credential hashes edges receive | Cluster status |
| `GRPC_AUTH_TOKEN` (and `GRPC_AUTH_TOKEN_NEXT` during a rotation) | Edges authenticate with one token to whichever site they reach | Cluster status |
| `CSRF_KEY` | Signs the admin UI's and portal's forms; required with more than one replica | Cluster status, and a warning when more than one replica runs without it |
| TLS certificates for gRPC `:50051` | Valid for both sites' endpoints (both names, or a wildcard), from a CA your edges trust | — |
| `TYK_AI_LICENSE` (Enterprise) | One license covers both sites: licenses meter namespaces, not control planes | — |
| The AI Studio version | Both sites run the same release; during an upgrade, see [Upgrading](#upgrading) | Cluster status lists each replica's version |
| `DATABASE_URL` | The same database (same hosts, both listed), the same `DATABASE_SCHEMA` if set | — |
| `QUEUE_TYPE=postgres` (or NATS reachable from both sites) | Chat sessions work whichever replica a request lands on; the default in-memory queue is per replica | — |
| Plugins from the marketplace or an OCI registry | A plugin installed from a file (`file://`) must exist at the same path on every replica, in both sites | — |

Set a different `CLUSTER_NODE_LABEL` per site (`site-a`, `site-b`). The label is shown for each replica in the cluster status and in the **Held by** column of the Edge Gateways page, so you can see which site holds which edge. See [Secrets that must match](./reference-architecture.md#secrets-that-must-match) for the whole list, including what edges need.

**Each site's own.** Log exports (`EXPORT_STORAGE_PATH`) stay on the replica that made them; download them through that replica. Branding (logo and favicon) is in the database and needs nothing shared.

## Load balancers and DNS

- **UI and API.** Put both sites behind one name with **session affinity** (cookie or client address): chat and agent conversations and MCP connections over SSE are held by the replica that started them. Health-check `GET /ready` (or `/ready?require=write` to send traffic only where the database takes writes) and use `GET /health` for liveness. `/ready` stays 200 through a database failover shorter than `DATABASE_READY_GRACE` (30 seconds), so a failover does not empty the load balancer; see [Readiness during a failover](./configuration.md#postgres-with-a-standby-failover). With a global load balancer, or DNS failover between two site addresses, a user moves to the other site when theirs stops answering. Sign-in sessions are in the database and carry over; a chat or agent conversation that was open on the lost replica ends and is started again.
- **gRPC for edges** (`:50051`). Give each site's control plane its own address and list both on the edges (below). A load balancer in front of the port must support HTTP/2 and keep idle streams open for at least a minute.
- **Name resolution inside each site.** This is a requirement of the cluster, not something AI Studio or the gateway provides. Clients, edges and Studio find each other through the cluster's DNS. If a site's lookups can reach a DNS server only across the link between the sites, losing that link stops name resolution in that site. Every request from its clients then fails for as long as the link is down, although its edges and Studio keep running. Keep DNS answering inside each site: on Kubernetes, run a DNS replica in each zone and route lookups to the replica in their own zone (zone-local routing on the DNS Service), or use NodeLocal DNSCache. NodeLocal DNSCache sends the names it does not have cached on to the cluster's DNS Service, so pair it with zone-local routing. In the partition test on one EKS cluster (a three-minute cut between two zones), there was no split brain and each zone's data plane kept serving. But clients whose lookups went to the DNS replica on the other side of the cut failed every request for the whole cut.

## Edges

### Two control endpoints, own site first

```bash
# Edge in site A
CONTROL_ENDPOINTS=cp-a.example.com:50051,cp-b.example.com:50051
# Edge in site B
CONTROL_ENDPOINTS=cp-b.example.com:50051,cp-a.example.com:50051
```

An edge connects to the first endpoint that accepts it. When that connection fails it tries the others at once, in list order, and backs off only when all have failed (`EDGE_RECONNECT_INTERVAL`, doubling to at most 30 seconds). It keeps serving throughout; a configuration push in flight is completed by the control plane it arrives at. An edge stays where it moved until that connection ends; set `EDGE_CONTROL_FAILBACK_INTERVAL` (for example `5m`) to have it return to its own site's control plane when that is back. Details: [Several control planes](./edge-gateways.md#several-control-planes).

**The alternative: one name.** A single DNS name or global load balancer in front of both sites' gRPC ports, set as `CONTROL_ENDPOINT`, works too. The edge then reconnects to whatever the name resolves to. The list is better when edges should prefer their own site and move without waiting for DNS or health checks.

### Starting during an outage

By default an edge that starts while no control plane answers exits. With `EDGE_START_FROM_LAST_KNOWN_GOOD=true` it waits `EDGE_STARTUP_CONTROL_TIMEOUT` (30 seconds), then starts from the configuration it last applied, which every edge stores encrypted in its own database, as long as it has been without a control plane for less than `EDGE_LAST_KNOWN_GOOD_MAX_AGE` (72 hours).

- **It needs a database that survives the pod.** On Kubernetes use the microgateway chart's StatefulSet form with persistence; an `emptyDir` covers a container restart but not a rescheduled pod. The chart prints a warning when the option is set without it.
- **The trade-off.** An edge started this way serves the configuration it last knew: an LLM, App or filter removed in Studio since then keeps working on it until it reconnects. That is why it is off by default.
- **Keys need full credential sync.** Without it, an edge validates a key only by asking a control plane, and remembers the answers in memory, so a restarted edge **cannot validate any key** until a control plane answers: every request gets `503` with `Retry-After` (`credential_check_unavailable`), which SDKs retry, and `EDGE_TOKEN_CACHE_STALE_GRACE` does not help because the cache is empty after the restart. With full credential sync (Enterprise, below) the edge validates keys from the hashed list in its database, so it serves keys too; a key revoked in the meantime then keeps working until the list is older than `EDGE_CREDENTIAL_SYNC_MAX_AGE`.
- Details: [Starting without a control plane](./edge-gateways.md#starting-without-a-control-plane).

### Edge volumes and capacity

An edge in either site may carry the other site's load after a site is lost, so size each site's edges for all of it, in memory too: an edge's working set grows with its load, and through a control-plane outage it also holds its analytics backlog (see [Analytics during a control-plane outage](./edge-gateways.md#analytics-during-a-control-plane-outage)). The microgateway chart's default memory limit, 1Gi, covers about 3,000 requests per second with its 64 MiB backlog. Each edge keeps a copy of its analytics in its own database for `ANALYTICS_RETENTION_DAYS`, which fills a small volume quickly: size the volume with the table in [Scaling Edge Gateways](./deployment-helm-k8s.md#scaling-edge-gateways) (about 35 MB per day per request per second), or set a short retention, since Studio has the same records. When the volume is full the edge keeps serving and says so (`"database_full"` on `/ready`, `microgateway_database_full`), but it can no longer apply a configuration push or store what last-known-good starts from.

On the test cluster (see [Measured on one test cluster](#measured-on-one-test-cluster)), one 4-vCPU edge carried 2,700 short requests per second on its own at an added p99 of 49.5 ms (median 1.1 ms), using 3.0 to 3.3 of its 4 cores, against 3.7 ms when two edges shared the load. Size each site's edges so that, after a site is lost, they carry both sites' load at a latency you accept.

**A full volume, measured** (Community build, see the set-up below). One edge's volume was held full for 271 s at 3,000 short and 188 streaming requests per second. The edge kept serving: no errors, an added p99 of 3.1 ms, no restarts. `/ready` stayed 200 with `database_full` and what it could not store. A configuration pushed meanwhile left that edge with its old configuration, shown in Studio as not in sync. Once the volume had room, the next push applied within 5 s, without a restart. How fast the volume fills was not re-measured: size it with the table in [Scaling Edge Gateways](./deployment-helm-k8s.md#scaling-edge-gateways), or set a short retention.

**Past capacity.** Overload was measured only on an earlier 2.3 build. Two edges driven at their combined benchmark capacity were restarted by their liveness probes instead of refusing requests with `503`. It was not repeated on the final build. Size each site's edges so that, after a site is lost, the remaining edges carry both sites' load below the benchmarked capacity in the [Reference Architecture](./reference-architecture.md).

### Keys an edge has never seen (full credential sync, Enterprise)

Without it, an edge validates a key it has not seen by asking a control plane, and caches the answer for 5 minutes (`EDGE_TOKEN_CACHE_TTL`). While no control plane answers, a key the edge has not cached is answered `503` with `Retry-After` (`credential_check_unavailable`).

With full credential sync, Studio sends each edge a keyed hash of every App key of its namespace in advance (never a key), so the edge validates keys locally whether or not a control plane answers:

| Edge setting | Default | Purpose |
|---|---|---|
| `EDGE_CREDENTIAL_SYNC` | `on_demand` | `full` (Enterprise): keep the hashed list of every App key of the edge's namespace in its database and validate from it. A Community edge given `full` validates on demand. Any other value stops start-up. |
| `EDGE_CREDENTIAL_SYNC_MAX_AGE` | `24h` | How long the list is trusted after the last successful sync. Past it the edge validates on demand until a sync succeeds. `0` means no limit: an edge cut off from every control plane then accepts keys revoked since its last sync for as long as the outage lasts. Keep a limit in production. |

- **How it stays current.** The edge syncs on every registration a control plane accepts, on every heartbeat it acknowledges (`EDGE_HEARTBEAT_INTERVAL`, 30 seconds), and within about a second when Studio says a key on its list changed: the whole list the first time, after a move to the other site's control plane and at least every 15 minutes, only the changes in between.
- **How a key is checked.** On the list and active, with its App active: accepted without asking a control plane. On the list but deactivated, deleted, or its App inactive: refused. Not on the list (created in the last seconds): asked as on demand, and the answer recorded. An explicit refusal from a control plane always wins.
- **What it needs.** The list is in the edge's database, so on Kubernetes use the microgateway chart's StatefulSet form with persistence (`config.credentialSync: full`, `config.credentialSyncMaxAge`); without it a rescheduled pod validates on demand until its first sync. An older or Community control plane cannot send the list: the edge logs it and stays on demand.
- Details: [Full credential sync](./edge-gateways.md#full-credential-sync-enterprise) and [Scaling Edge Gateways](./deployment-helm-k8s.md#scaling-edge-gateways).

**The revocation trade-off.** A key revoked in Studio (deactivated, deleted, its secret regenerated, or its App deactivated) stops working on a connected edge on demand within about a second, whichever site holds the edge: Studio tells every edge (`credential.changed`), and the edge drops what it cached for that key. With full credential sync the same notice makes the edge sync when the key is on its list, after a random delay of up to half a second, and at most about one such sync per second per edge, so it stops within about a second too. A key the edge does not hold (another namespace's, or one created since its last sync) starts no sync. If the notice is lost, the edge's next sync (one heartbeat interval, 30 seconds) repairs it. If Studio cannot read the key's secret, or has no `MICROGATEWAY_ENCRYPTION_KEY`, it cannot name the key to the edges: an on-demand edge then refuses it when its cache entry expires (`EDGE_TOKEN_CACHE_TTL`, 5 minutes), and a full-sync edge at its next sync. An edge that reaches no control plane cannot learn of a revocation at all: with `EDGE_TOKEN_CACHE_STALE_GRACE` it keeps accepting a cached key until the grace runs out, and with full credential sync until its list is older than `EDGE_CREDENTIAL_SYNC_MAX_AGE` (24 hours). Choose those maximums for how long you are willing to let a revoked key work on an isolated edge.

## Requests while there is no primary

While the database has no writable node (between the loss of the primary and the promotion of the standby), Studio **holds** a request's database statement and tries it again until a writable node answers, for up to `DATABASE_WRITE_RETRY_WINDOW` (15 seconds by default):

- **A promotion inside the window**: the request waits a few seconds and succeeds, once. An App or a key created during an automatic failover is created exactly once.
- **No promotion inside the window**: the request is answered `503` with `Retry-After` and `errors[0].code` `database_unavailable`, and nothing it began is kept. Signing in and requests with a session or an API key answer the same `503`, never `401`, so clients retry rather than drop their credentials.
- **No queue beyond the window.** A write that was answered `503` is not applied later: the client makes it again. This is the line between a failover (seconds, covered) and an outage (a manual promotion, a lost database): during an outage writes fail.
- **What is not held**: a statement inside a transaction that was already open when the primary was lost fails at once (and leaves no partial rows), as does a statement that may have reached the server. At most 512 statements are held at once per replica; beyond that a request is answered `503` straight away. Edges' calls to the control plane and Studio's background work are not held: they fail at once, so an edge moves to the other site's control plane without waiting.

Size the window for your failover manager: longer than its detection time plus the promotion. Keep `DATABASE_READY_GRACE` longer than the window, so the load balancer still sends traffic to a replica while its requests are held (Studio warns at start when it is not). See [Requests during a failover](./configuration.md#postgres-with-a-standby-failover).

Edges are not affected by a database failover beyond the control plane being briefly unable to answer: they keep serving from their loaded configuration, and validation and analytics resume when the control plane can reach the database again.

## When something fails

| Situation | What happens | What to plan for |
|---|---|---|
| **A whole site is lost** (Studio and its PostgreSQL node) | Edges of that site move to the other site's control plane and keep serving. If the lost node was the primary, the failover manager promotes the other site's standby (automatically with a witness, by hand without one) and the surviving replicas resume writing, without a restart. The leader lease moves to a surviving replica within about 30 seconds. Pushes in flight are completed by the other site. | A witness in a third location for automatic failover. Capacity in each site to carry all edges and all admin traffic. |
| **The PostgreSQL primary is lost**, both Studios up | No primary until the standby is promoted (see the row above for who promotes). Requests are held for up to `DATABASE_WRITE_RETRY_WINDOW` (15 seconds) and complete once the new primary is up, or are answered `503 database_unavailable`. Replicas follow the promotion on their own: broken connections are dialled again and reach the new primary. `/ready` stays 200 for `DATABASE_READY_GRACE` (30 seconds), so the load balancers keep both sites. | A promotion time shorter than the window; `DATABASE_READY_GRACE` longer than both. |
| **A primary is demoted but keeps running** (a switchover, or fencing) | Its connections would fail every write with SQLSTATE 25006. Studio notices from the first refused write, or within 5 seconds when idle, and reconnects to the new primary. | Nothing: this is the planned switchover's path. |
| **One site's Studio is lost**, its database node fine | Edges of that site move to the other site. The UI and API load balancer stops sending traffic there; users on that site's replicas may lose an open chat session. | Session affinity that fails over; `EDGE_CONTROL_FAILBACK_INTERVAL` if edges should return when it is back. |
| **The link between the sites is lost** | Each site's replicas keep the database they can reach. The site that loses the primary sees its database as unreachable: after `DATABASE_READY_GRACE` its `/ready` fails and the load balancer stops sending traffic there; its edges keep serving and keep their connections. The failover manager must not promote the cut-off standby unless a witness says the primary's site is gone (this is the split-brain case). | A witness; alerts on `aistudio_db_writable == 0` per replica. |
| **DNS reachable only across the link** (a site's lookups go to a DNS server in the other site, and the link is lost) | Name resolution fails in that site. Its clients cannot reach its edges or Studio, and every request from them fails for as long as the link is down, although the gateway and Studio keep running. This is a property of the cluster's DNS, not of AI Studio. | DNS that answers inside each site ([Load balancers and DNS](#load-balancers-and-dns)). |
| **An edge restarts during a control-plane outage** | By default it exits until a control plane answers. With `EDGE_START_FROM_LAST_KNOWN_GOOD=true` and a persistent volume it starts from its stored configuration, but it validates keys only with full credential sync (Enterprise): on demand, every key gets `503` until a control plane answers. | Decide on last-known-good; persistent volumes for edges. |
| **An unknown key during a control-plane outage** | `503` with `Retry-After` (`credential_check_unavailable`), which SDKs retry. With `EDGE_CREDENTIAL_SYNC=full` (Enterprise), validated locally from the synced list. | Decide on full credential sync and on `EDGE_TOKEN_CACHE_STALE_GRACE` against the revocation trade-off above. |
| **Asynchronous replication and a failover** | The last transactions the old primary acknowledged may be lost. Studio's event log, leader lease and budget sync recover on their own; lost writes stay lost. | Synchronous replication between the sites. |

### Measured on one test cluster

These figures come from one test cluster. They show what the mechanisms above do under load, not what your deployment will measure: your failover manager, network and load decide that.

**Set-up.**
- One Amazon EKS cluster in one region, with its two availability zones as the two sites.
- In each site, one 4-vCPU node (m7i.xlarge) for Studio and that site's PostgreSQL node, and one 4-vCPU node (c7i.xlarge) for one edge.
- PostgreSQL with a synchronous standby in the other site. Failover was done by a script standing in for a failover manager, with no witness.
- Community edition images, built from commit `fe145cc2` of the 2.3 development branch.
- Load: 3,000 short requests per second plus 188 streaming requests per second in total over the two edges, against a mock LLM, unless a row says otherwise. Each Studio also ran an admin loop of writes and sign-ins, plus App creates in the database-loss runs.
- Baseline without faults: no errors, added p99 latency 3.7 ms, and analytics complete (every answered request has its record in Studio).

**Database faults.** Analytics were complete in every run.

| Fault | Without a primary | Data plane | Admin API |
|---|---|---|---|
| Planned switchover | 13 to 14 s | 0 errors of 2.2 million requests; added p99 6.0 ms | All answered 200. Writes waited up to 10.6 s, sign-ins up to 8.7 s. |
| Switchover held without a primary for longer than the window | 33 s | 0 errors of 2.07 million short and 130,000 streaming requests; added p99 6.8 ms | Per Studio, 8 sign-ins and 16 writes were answered `503` after 15 s (`DATABASE_WRITE_RETRY_WINDOW`); the rest 200. No `401`, no `500`. |
| Primary's PostgreSQL killed | 14 s | 0 errors of 2.07 million short and 130,000 streaming requests; added p99 3.2 ms | All answered 200 or 201, longest wait 13.7 s. 1,578 Apps created, each exactly once. |
| Primary's PostgreSQL crashed (`pg_ctl stop -m immediate`) | 14 s | 0 errors of 1.35 million short and 85,000 streaming requests; added p99 5.8 ms | All answered 200 or 201, longest wait 13.9 s. 1,047 Apps created, each exactly once. |
| Primary lost at saturation (ramp from 6,000 to 14,000 short requests per second, primary lost at about 12,000) | 14 s | 0 errors of 5.2 million requests; added p99 38.4 ms over the ramp | All answered 200, longest wait 12.7 s. |

**Site and control-plane faults.**
- **A site lost for five minutes, then returned.**
  - 13 short and 38 streaming requests failed, of 2.79 million and 175,000, all in the second the lost site's edge was killed.
  - The surviving edge carried the whole load at an added p99 of 47.7 ms. One edge carrying that load on its own, with no fault, measured 49.5 ms.
  - Analytics were complete. The returning PostgreSQL node was a synchronous standby again 63 s after the site came back.
- **Every control plane cut for 3.5 minutes, and one edge restarted during the cut** (Community, so no full credential sync).
  - 632 of 1.35 million short requests (0.05%) and 406 of 85,000 streaming requests failed, nearly all of them requests that reached the restarted edge, which cannot validate a key until a control plane answers.
  - All App creates on the Studios succeeded (1,070, each once).
- **The link between the sites cut for three minutes.**
  - Load: each zone's clients sent half the load to their own zone's edge.
  - The stand-in failover script dropped the synchronous requirement 60 s into the cut.
  - No split brain, and analytics were complete.
  - **The zone with the primary:** its clients had 0 errors of 675,968 short requests, at an added p99 of 3.3 ms. Its Studio took writes again 108 s before the link returned, once the synchronous requirement was dropped.
  - **The cut-off zone's Studio:** it stayed ready for `DATABASE_READY_GRACE` (30 s), then answered `503`, and took writes again within a second of the link returning.
  - **The cut-off zone's clients:** they failed every request for the whole cut, because their DNS lookups went to a DNS server on the other side (see [Load balancers and DNS](#load-balancers-and-dns)).

**Enterprise edition, with full credential sync.** The same cluster and load, on Enterprise images built from commits `859c9bbb` and `1897ef3a` of the 2.3 development branch.
- **Cost per request.** On one edge at 2,700 short requests per second, full credential sync and on-demand validation measured level:
  - added p50 / p90 / p99 of 1.29 / 1.64 / 7.56 ms with full sync, against 1.27 / 1.57 / 6.87 ms on demand;
  - edge CPU 1.10 against 1.11 cores.

  An earlier 2.3 build added about 3 ms at p99 with full sync; the final build does not. Absolute latency varied between runs on different nodes (an on-demand p99 of 2.6 ms one day, 6.9 ms the next), so compare the two modes, not the figures.
- **One site's control plane lost for 3 minutes.** 0 errors of 1.35 million short and 85,000 streaming requests; added p99 6.4 ms; analytics complete. The edges moved to the other site's control plane, so nothing waited in a backlog.
- **Every control plane cut for 3.5 minutes, one edge restarted during the cut.**
  - The restarted edge was serving from last-known-good about 75 s after the restart. With every control plane still cut, it answered a key it had never served with 200 (from its synced list) and refused an unknown key.
  - 45 of 1.35 million short requests failed, 43 of them refused at the moment the pod was killed. The 355 streams open on the killed pod timed out. No request was answered `503` (632 failed in the Community run above).
  - Analytics were 10,016 rows short: what the killed pod held in memory.
- **The analytics backlog through an outage.** Every control plane was cut for 3.5 minutes, no edge restarted, with the chart's 64 MiB backlog.
  - Both edges kept serving: 0 errors of 2,249,599 short and 141,213 streaming requests, added p99 3.67 ms, no restarts, no shedding.
  - Peak memory was 520 MB on a 1Gi limit and 517 MB on 512Mi.
  - Each edge's backlog filled about 3 minutes into the cut, at 1,500 requests per second per edge. Past that, the oldest records were dropped: 829,898 records (276,633 requests at three records each), exactly the requests missing from Studio's analytics.
  - The backlog drained 58 s after the control planes returned.
  - **With the chart's 64 MiB, an outage longer than about 3 minutes at 3,000 requests per second over two edges loses analytics.** For longer outages or higher rates, raise `analyticsPulse.maxBacklogBytes`, and the memory limit with it ([Analytics during a control-plane outage](./edge-gateways.md#analytics-during-a-control-plane-outage)).
- **Revocation.** A key deactivated in Studio was refused by a full-sync edge 0.9 s after the API call returned.
- **Namespaces.** A key changed in one namespace started a sync only on that namespace's edge. A key of a global App started one on every edge.
- **Primary's PostgreSQL killed.**
  - 14 s without a primary.
  - Data plane: 0 errors, added p99 5.5 ms, analytics complete.
  - Admin API: all answered 200 or 201, with waits up to 14.2 s. 60 Apps were created, each with exactly one audit entry.
- **Budgets.** After all the faults, every App's usage total in Studio equalled the sum of its 5,536,849 records.
- **Both databases down for 91 s.** `/health` answered 200 throughout. `/ready` answered 200 (`"database": "unreachable"`) for `DATABASE_READY_GRACE` (30 s), then `503`, then 200 again 16 s after the primary returned.

**Not measured:**
- How long edges take to move to the other site's control plane under load. No request failed while they moved, and an end-to-end test measures 2.0 s.
- How fast an edge's volume fills (see [Edge volumes and capacity](#edge-volumes-and-capacity)).

## Runbooks

The commands in brackets are the development stack's (`dev/aa/aa.sh`), which plays the failover manager by hand. In production the same steps are your failover manager's or your managed service's.

### Planned switchover

Moving the primary role to the other site with both sites up, for maintenance:

1. Check the standby is streaming and synchronous (`pg_stat_replication` on the primary shows it with `sync_state = sync`; `aa.sh status`).
2. Ask the failover manager for a switchover (`patronictl switchover`, `kubectl cnpg promote`, your managed service's failover action; `aa.sh switchover`). It stops writes on the old primary, lets the standby catch up, promotes it, and turns the old primary into the new standby.
3. Studio follows on its own: replicas still connected to the old primary notice it is read-only at the first refused write, or within 5 seconds, and reconnect to the new one. Connections also drift over within `DATABASE_CONN_MAX_LIFETIME` (30 minutes).
4. Check: `aa.sh status` or the cluster status shows every replica live, `aistudio_db_writable` is 1 on every replica, and `aistudio_db_failovers_total` went up by one.

### Unplanned failover

The primary's site, or its node, is lost:

1. With a witness, the failover manager promotes the standby. Without one, see [Manual promotion](#manual-promotion).
2. Studio replicas in the surviving site reach the new primary on their own; `/ready` has stayed 200 if the promotion took less than `DATABASE_READY_GRACE`.
3. Edges of the lost site are already on the surviving site's control plane.
4. If replication was synchronous, drop the synchronous requirement on the new primary until a standby is back (the failover manager usually does), or every commit waits for a standby that is not there.
5. Check as for a switchover. Then bring the lost site back as a standby ([below](#bringing-the-old-primary-back-as-a-standby)).

### Manual promotion

Without a witness, a person decides the primary's site is gone:

1. **Make sure the old primary cannot accept writes**: stop it, or cut it off (fence it). Promoting while it may still be running and reachable is how split brain happens.
2. Promote the standby (`pg_ctl promote` or `SELECT pg_promote()` on it; `aa.sh promote`). Clear `synchronous_standby_names` on it.
3. Studio replicas reach it on their own. Until this step, writes held longer than `DATABASE_WRITE_RETRY_WINDOW` have been answered `503` and were not queued: anything users tried to save during the outage must be done again.

### Bringing the old primary back as a standby

A node that was the primary must not come back as one next to the promoted node. Rebuild it as a standby of the new primary (`pg_rewind` or a fresh `pg_basebackup`; your failover manager's reinit; `aa.sh rejoin a`), let it stream, then make it synchronous again. Then start that site's Studio replicas: they wait for a writable database at start-up (`DATABASE_STARTUP_TIMEOUT`, 60 seconds) and join the cluster.

### Failback

Failing back is a planned switchover in the other direction, once the rejoined standby is streaming and synchronous. Edges return to their own site's control plane when their current connection ends, or within `EDGE_CONTROL_FAILBACK_INTERVAL` when it is set.

### Rotating the shared secrets across both sites

Replicas are never refused for disagreeing, so a rotation can roll; the cluster status names the setting and the replicas until every live replica has the new value.

- **Edge token.** Set the new value as `GRPC_AUTH_TOKEN_NEXT` on every replica in both sites, roll the edges onto it (`EDGE_AUTH_TOKEN`), then make it `GRPC_AUTH_TOKEN` everywhere and remove `GRPC_AUTH_TOKEN_NEXT`.
- **`CSRF_KEY`.** Roll it through the replicas of both sites. Forms loaded before the change on a replica that has not yet changed fail once and must be reloaded.
- **`TYK_AI_SECRET_KEY` and `MICROGATEWAY_ENCRYPTION_KEY`** cannot be rotated by rolling: App keys are found by a hash keyed with `TYK_AI_SECRET_KEY`, and edges must decrypt what `MICROGATEWAY_ENCRYPTION_KEY` encrypted. Do not change them on a running deployment. See [Secrets](./secrets.md).

### Upgrading

- **To 2.3 (once).** Enterprise needs a **license issued for 2.3**: a 2.2 license is refused, so get one from your account manager before you start. **Take a database backup** before the first 2.3 replica starts, and **keep your 2.2 license**: the 2.3 migration is one way (App keys are hashed and encrypted at rest), and returning to 2.2 means restoring that backup and running 2.2 with its 2.2 license. Stop every older replica, in both sites, before the first 2.3 replica starts: an older AI Studio on a 2.3 database refuses every App key. The upgrade to 2.3 is therefore not rolling. See [Upgrading from 2.2](./license.md#upgrading-from-2-2) and the 2.3.0 release notes (`CHANGELOG-v2.3.0.md`, "Before you upgrade" and "Upgrade notes").
- **Later upgrades roll**, one replica at a time, across both sites. Migrations run under a database lock (a replica that is waiting logs `Another Studio instance is migrating this database`, and gives up after `MIGRATION_LOCK_TIMEOUT`, 15 minutes), and a replica that stops hands its edges and pushes to the others. An older replica refuses to start on a database a newer version changed in a way it cannot read (the schema's minimum reader version), so a rollback fails safely instead of corrupting data.
- **Upgrade the hub before the edges.** Every new edge option is off by default and behaves as before against an older hub.

## Monitoring

| Signal | Where | Alert when |
|---|---|---|
| Live replicas, their sites, versions, the leader, edges per replica, shared-secret disagreements, a read-only database | `GET /api/v1/cluster/status` (administrators) | A warning is listed; a replica is missing; no leader |
| `aistudio_db_writable` per replica | Studio `/metrics` | 0 for longer than your failover manager's promotion time |
| `aistudio_db_failovers_total` | Studio `/metrics` | It increases (a failover happened: check the runbook steps) |
| `/ready` body: `"database"` and `"database_since"` | Studio | — (what the load balancer sees) |
| `aistudio_db_writes_held`, `aistudio_db_write_retries_total{outcome}`, `aistudio_db_write_hold_seconds` | Studio `/metrics` | `outcome="gave_up"` or `"rejected"` increases: requests answered `503` during a failover |
| `aistudio_db_unavailable_responses_total` | Studio `/metrics` | It increases: `500`s answered as `503 database_unavailable` (linked to the outage by time, so it can include `500`s of other causes during one) |
| `microgateway_control_connected{endpoint}` | edge `/metrics` | An edge's first endpoint is 0 for long (it is on the other site) |
| `microgateway_config_source{source}` and `microgateway_config_age_seconds` | edge `/metrics` | `last_known_good` is 1; the age approaches `EDGE_LAST_KNOWN_GOOD_MAX_AGE` |
| `microgateway_analytics_pulse_backlog_bytes` | edge `/metrics` | It approaches `max_backlog_bytes`: analytics held for a control plane that is not taking them |
| `microgateway_analytics_pulse_dropped_records_total{reason}` | edge `/metrics` | It increases: analytics lost during an outage |
| `microgateway_credential_sync_last_success_timestamp` (full credential sync) | edge `/metrics` | `time() - ` it approaches `EDGE_CREDENTIAL_SYNC_MAX_AGE`: past it the edge stops trusting its list |
| `microgateway_credentials_synced`, `microgateway_credential_validations_total{source}` | edge `/metrics` | `source="stale"` increases (validations made on demand because the list was too old); the synced count drops to 0 |
| Edge sync status (In Sync, Pending, Stale) and **Held by** | Studio → Edge Gateways | Edges stale; all edges held by one site for long |
| Replication lag and `sync_state` | PostgreSQL (`pg_stat_replication`) or your managed service | The standby is not synchronous, or lags |


See [Observability](./observability.md#database-metrics) for the metric definitions.

## What is guaranteed

1. With automatic database failover, losing either site (Studio, PostgreSQL or both) needs no operator action. Studio replicas in the surviving site resume writing within seconds of the promotion, without a restart.
2. API writes in flight during a failover are held and retried up to `DATABASE_WRITE_RETRY_WINDOW`, then fail with a retryable `503 database_unavailable`. They are never applied twice.
3. A running edge keeps serving through any control-plane outage, moves to the other control plane on its own, and loses no configuration push.
4. With `EDGE_START_FROM_LAST_KNOWN_GOOD=true`, an edge with an intact database starts and serves with no control plane reachable.
5. With `EDGE_CREDENTIAL_SYNC=full` (Enterprise), an edge validates every active key of its namespace locally, control plane or not, from hashes it cannot reverse, for up to `EDGE_CREDENTIAL_SYNC_MAX_AGE` after its last sync.
6. With synchronous replication, a key whose creation was committed is valid on every edge after a failover.

**Not guaranteed:**

- Revocation reaching an edge that cannot reach any control plane. It is bounded by the maximum ages you configure (`EDGE_LAST_KNOWN_GOOD_MAX_AGE`, `EDGE_CREDENTIAL_SYNC_MAX_AGE`, `EDGE_TOKEN_CACHE_STALE_GRACE`).
- A key being usable on edges before its creation has committed.
- Writes surviving an outage longer than the retry window: they fail with a retryable `503` and must be made again. There is no write queue beyond the window.
- Analytics beyond an edge's backlog (`max_backlog_bytes`) through a long control-plane outage, or through an edge restart during one (see [Analytics during a control-plane outage](./edge-gateways.md#analytics-during-a-control-plane-outage)).
- Anything with multi-primary PostgreSQL, or with two nodes promoted at once.

## Try it on the development stack

The repository's two-site stack runs this topology on one machine: two Studio replicas labelled `site-a` and `site-b` with a sticky front proxy, a PostgreSQL primary and a synchronous hot standby, a cuttable gRPC endpoint per site, and two edges, each listing its own site first, with databases on volumes.

```bash
make dev-aa            # Community Edition (make dev-aa-ent: Enterprise, license from dev/.env.secrets)
make dev-aa-status     # roles, replication, replicas, edges
```

| Port | What |
|---|---|
| 3100 | UI, through the sticky front proxy |
| 8100 | API through the front proxy (either site) |
| 8180, 8280 | site-a and site-b Studio API |
| 50151, 50251 | site-a and site-b control gRPC (cuttable) |
| 8181, 8281 | edge-a and edge-b gateway |

Its configuration (`dev/docker-compose.active-active.yml`) is a reference for what each piece needs: the multi-host `DATABASE_URL` with `target_session_attrs=read-write&connect_timeout=3`, the shared secrets, `CLUSTER_NODE_LABEL`, `QUEUE_TYPE=postgres`, `CONTROL_ENDPOINTS` with the own site first, and the edges' databases on volumes, with `EDGE_START_FROM_LAST_KNOWN_GOOD=true` so an edge restarted while every control plane is cut can be tried (see [Starting during an outage](#starting-during-an-outage)). Nothing in it fails over by itself; `dev/aa/aa.sh` does each step of a failover manager by hand:

```bash
dev/aa/aa.sh stop-site a     # lose site A: its Studio, its control endpoint, its PostgreSQL node
dev/aa/aa.sh promote         # the database failover: promote the standby
dev/aa/aa.sh rejoin a        # bring site A back: its node rebuilt as the standby, then Studio
dev/aa/aa.sh switchover      # planned move of the primary role, both sites up
dev/aa/aa.sh cut-cp b        # cut edges off from site B's control plane (Studio keeps running)
dev/aa/aa.sh restore-cp b
```

Between steps, watch `make dev-aa-status`, the cluster status (`curl` `localhost:8180/api/v1/cluster/status` with an administrator's key), the edges' `/ready` (`localhost:8181/ready`, `localhost:8281/ready`) and the Edge Gateways page.

## See also

- [Reference Architecture](./reference-architecture.md): the single-site model this extends, failure behaviour and sizing
- [Kubernetes / Helm](./deployment-helm-k8s.md#two-clusters-one-replicated-postgresql): one release per site on one replicated PostgreSQL
- [Edge Gateways](./edge-gateways.md#connection-settings): control endpoints, last-known-good, full credential sync
- [Configuration](./configuration.md#postgres-with-a-standby-failover): the database settings and readiness
- [Observability](./observability.md): metrics
