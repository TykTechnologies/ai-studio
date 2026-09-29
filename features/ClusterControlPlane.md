# Multi-replica control plane

Several AI Studio replicas can serve one database (for example a host
embedding Studio behind a load balancer). Microgateways (edges) hold a
long-lived gRPC stream to whichever replica they reached. This document is
the contract for how configuration changes and configuration pushes reach
every edge in that setup, what is guaranteed, and how every failure is
reported. It is the design for PR 8 of the embedding work.

## Delivery status

Built in three steps:

- **8a (foundations), done:** `pkg/pglisten` (the chat queue's shared
  LISTEN connection, now shared by any subscriber, with a reconnect hook);
  `pkg/cluster` (the node registry and the event log); edge stream
  ownership with session-conditional writes; the checksum recompute over
  every namespace in the database. Studio joins the cluster in
  `studio.New` (`Options.NodeID`). It is always on with Postgres, whether
  one replica runs or ten, so a multi-replica deployment cannot forget to
  enable it; with SQLite (one process) the registry has one row and the
  event log is inert.
- **8b:** durable pushes (`reload_operations`, `edge_commands`), the push
  dialog's per-edge results, and end-to-end tests with real edges.
- **8c:** `DirDown` fan-out, cross-replica gateway reload and budget cache
  invalidation, leader-only background jobs, `GET /cluster/status`.

## What goes wrong today with more than one replica

- **Stream-bound state is in memory.** A push (`POST /edges/:id/reload`,
  `/namespaces/:ns/reload`, `/edges/reload-all`) only reaches edges whose
  stream is on the replica that received the API call; others get
  "no connected edges". The operation's status lives in that replica's
  memory, so polling another replica says "operation not found".
- **Config-change detection is per replica.** A change recomputes the
  namespace checksum only for namespaces with edges on the replica that
  made the change, so edges elsewhere are not marked pending.
- **Edge status is last-writer-wins.** When an edge moves from replica A to
  B, A's stream-close handler (and its stale-connection sweep) marks the
  edge disconnected after B marked it connected.
- **Events stop at the replica.** `DirDown` events (budget sync, plugin
  events) reach only that replica's edges; the embedded gateway and budget
  caches of other replicas never hear about changes.
- **The user is told "successfully pushed" when the push was only
  initiated**, reload-all drops failing namespaces silently, and nothing
  shows per-edge outcomes.

## Guarantees

1. **A push is durable and replica-independent.** It is a row per target
   edge (`edge_commands`), written before the API returns. Any replica that
   holds the edge's stream delivers it; no replica has to be "the right
   one", and a replica dying loses nothing.
2. **Delivery is at least once, until the push's deadline.** A command is
   (re)sent whenever the edge is connected to any live replica before the
   deadline: at once if it is connected, on reconnect if it is not, again
   if the stream breaks before the edge answers or the replica that sent it
   dies. The edge's reload is idempotent (it pulls the full snapshot), so a
   repeat is harmless.
3. **Every command ends in exactly one terminal state, with a reason:**
   - `succeeded`: the edge reported READY and its loaded checksum matches
     the namespace's expected checksum;
   - `succeeded_with_warning`: READY, but the checksum it loaded differs
     (the configuration changed during the push; the edge will show as
     Pending and needs another push);
   - `failed`: the edge reported FAILED (its message is kept), or the
     stream broke mid-reload on the last allowed attempt;
   - `expired`: the deadline passed first: "not connected to any
     control-plane replica", or "connected but did not answer within N s".
4. **Status is visible from every replica** (`reload_operations` +
   `edge_commands`), per edge and aggregated, while in progress and after.
5. **Independently of pushes, drift is always detected.** Every heartbeat
   compares the edge's loaded checksum with the namespace's expected
   checksum in the database, on whichever replica receives it. Every config
   change recomputes that checksum for every namespace known to the
   database, on the replica that made the change.
6. **Fan-out events are at least once per live replica.** `DirDown` events
   and cache-invalidation events go through a database-backed log
   (`cluster_events`) that every replica reads; LISTEN/NOTIFY only wakes
   readers early. Handlers are idempotent. A replica that starts later
   builds its state from the database, so it does not need old events.

What is **not** guaranteed: delivery to an edge that stays disconnected
past the deadline (the push expires with that reason, and heartbeat drift
detection keeps showing the edge as Pending once it returns); and exactly
once (a reload may run twice on an edge).

## Design

### Replica identity and liveness: `cluster_nodes`

Each replica has a node ID (`Options.NodeID`, default `hostname-pid`) and
upserts `cluster_nodes(node_id, hostname, version, started_at, last_seen)`
every 5 s. A node is live while `last_seen` is under 20 s old. Single
replica and SQLite: the same code runs with one node.

### Edge stream ownership

`edge_instances` gains `owner_node_id` and `stream_session_id`. Opening a
stream sets both (a new session UUID) and status `connected`. Every write
that ends a stream (close, stale sweep) is conditional on the session it
belongs to, so a stale replica never overwrites a newer connection. An edge
is *reachable* when its owner node is live and its last heartbeat is fresh.

### Pushes: `reload_operations` and `edge_commands`

- The API resolves the targets from the database (not local memory),
  writes one operation and one `pending` command per edge with the
  deadline (default 5 min), and answers at once with the targets
  classified: reachable now, or waiting to reconnect (a warning).
- Every replica runs a dispatcher, woken by NOTIFY and a 1 s poll: for each
  `pending` command whose edge has a stream on this replica, it claims it
  with a conditional update (`status = 'pending'` → `claimed`,
  `claimed_by`, `claim_expires_at`, `attempts + 1`), sends the reload
  request on the stream and marks it `sent`. Only one replica can win a
  claim; a failed send puts the command back to `pending`.
- Reload responses arrive on the stream of the replica that sent the
  command; each phase is recorded. READY is verified against the expected
  checksum; FAILED is terminal (an edge-side failure such as a rejected
  snapshot is not retried automatically; the user sees the edge's message).
- A janitor on every replica (idempotent conditional updates) returns to
  `pending` any command whose claim expired on a dead node or whose stream
  closed before a terminal phase (up to 3 attempts), expires commands past
  their deadline with the specific reason, and fails `sent` commands the
  edge has not answered within the answer timeout.
- The operation's status is derived from its commands: `in_progress`,
  `succeeded`, `partially_failed`, `failed`, `expired`.

### Fan-out events: `cluster_events`

A replica publishing a cluster-wide event inserts a row (topic, direction,
payload, origin node) and sends `NOTIFY` with its id. Each replica keeps a
cursor and reads new rows on NOTIFY and every second; it also re-reads a
30 s window and skips ids it has seen, so a row whose transaction committed
after a later id is not missed. Rows older than 15 minutes are pruned. The
listener is the chat queue's shared `pq` listener, moved to `pkg/pglisten`.
Uses:

- `DirDown` events: each replica forwards them to its own edges.
- `system.llm.*` / datasource / filter changes: other replicas reload their
  embedded gateway.
- Budget and team-budget cache invalidation.

The checksum recompute is not an event consumer: the replica that made the
change recomputes every namespace known to the database, which is enough.

### Singleton jobs

The scheduler lease becomes a conditional update. Budget-sync aggregation
and alerts, marketplace sync, telemetry and retention jobs run only on the
lease holder; each replica still pushes budget state to its own edges.

## User feedback

- The push dialog tracks the operation to its end: per-edge state
  (waiting for connection / sent / reloading, with the edge's phase /
  succeeded / warning / failed / expired, each with its reason), then a
  summary. It never says "successfully pushed" before the edges confirm.
- Initiation warns when targets are not reachable ("2 of 5 edges are not
  connected to the control plane; the push will wait up to 5 minutes for
  them").
- Reload-all reports every namespace, including those that failed to
  start, and refuses (with the reason) when there is nothing to push to.
- `GET /api/v1/cluster/status` (admin): live nodes, their edge counts,
  event-log lag, and pending or stuck commands.

## Test plan

Unit tests use fakes; the rest run on Postgres (`DATABASE_URL`) with two or
three in-process replicas sharing a schema, and real microgateway edge
clients over gRPC (`microgateway/tests/cluster`).

Delivery and routing:
- push issued on replica A for an edge whose stream is on B: B delivers,
  the edge reloads, the status reads `succeeded` on A and on B;
- namespace push with edges spread over three replicas: all succeed;
- an edge not connected at push time connects within the deadline: it is
  delivered; one that never connects: `expired`, "not connected";
- an edge connected at push time that disconnects before answering and
  reconnects to another replica: redelivered there, `succeeded`, attempts 2;
- the replica holding a claim is killed before sending, and after sending:
  the command returns to `pending` and another replica delivers it;
- two replicas both hold a stream for the edge during a reconnect: exactly
  one claims the command;
- the edge answers FAILED: `failed` with its message, not retried;
- the edge answers READY with a checksum that no longer matches:
  `succeeded_with_warning`;
- the edge never answers: `expired` after the answer timeout;
- duplicate and late responses are ignored; responses for unknown
  operations are logged, not applied;
- three attempts exhausted: `failed` with the attempt history.

Ownership:
- reconnect to B, then A's stream close and A's stale sweep: the edge stays
  connected, owned by B;
- the owner node dies: the edge reads as unreachable until it reconnects.

Change detection:
- a change made on a replica with no edges marks the edges on other
  replicas' namespaces pending; heartbeats on any replica compute drift.

Event log:
- every event reaches every replica exactly once in handlers (despite
  NOTIFY and poll overlap); with the listener's backend terminated
  mid-stream, events still arrive by polling and the reconnect is logged;
  a row committed late (lower id, later commit) is still delivered;
  payloads over NOTIFY's 8000-byte limit work; origin echo is suppressed;
  pruning never removes rows inside the re-read window.

End to end (dev stack): two Studio replicas behind nginx on one Postgres,
three edges; push from each replica; kill a replica mid-push; restart
edges; check the UI's per-edge results against the edges' logs.
