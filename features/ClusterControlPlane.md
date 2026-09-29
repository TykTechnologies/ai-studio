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
- **8b, done:** durable pushes (`push_operations`, `edge_push_commands`,
  `services/pushes`), the push dialog's per-edge results, and end-to-end
  tests with real edges (`microgateway/tests/cluster`). It also fixed
  delivery hazards found on the way: concurrent `Send` on one gRPC stream
  (both sides), a replica shutdown that waited for its edges to leave, the
  edge's loaded-checksum race, and `EDGE_RECONNECT_INTERVAL` being ignored.
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
   edge (`edge_push_commands`), written before the API returns. Any replica that
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
   - `failed`: the edge reported FAILED (its message is kept, and it is
     not retried), or the last allowed attempt (3) ended without an answer:
     the stream broke, the replica died, the send failed, or the edge was
     silent past the answer timeout (60 s, reset by every progress report);
   - `expired`: the deadline passed first: "not connected to any
     control-plane replica", "disconnected and did not reconnect", or "did
     not finish reloading".
4. **Status is visible from every replica** (`push_operations` +
   `edge_push_commands`), per edge and aggregated, while in progress and
   after.
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

### Pushes: `push_operations` and `edge_push_commands`

- The API resolves the targets from the database (not local memory),
  writes one operation and one `pending` command per edge with the
  deadline (default 5 min), and answers at once with the targets
  classified: reachable now, or waiting to reconnect (a warning). A
  namespace or all-edges push leaves out edges offline for more than
  5 minutes and lists them as skipped; an explicit edge push always
  includes the edge. Reload-all is one operation over every namespace.
- Every change to a command is version-checked: a replica reads the
  command, checks the change still applies to it as it is now, and writes
  it only if `version` is unchanged, otherwise it reads it again. So no
  transition and no history entry is lost to a concurrent one (a stream
  closing while the send is being recorded; a READY racing a requeue).
- Every replica runs a dispatcher, woken by NOTIFY, by an edge opening a
  stream, and by a 1 s poll: for each `pending` command whose edge has a
  stream on this replica, it claims it (`claimed`, `claimed_by`, the
  stream session, `attempts + 1`), sends the reload request on exactly
  that stream and marks it `sent`. Only one replica wins a claim. A send
  that finds the stream already gone (the dispatcher's view is a moment
  old) hands the claim back without using an attempt; a transport error on
  a live stream uses one.
- Reload responses arrive on the stream of the replica that sent the
  command, and are attributed to the edge the stream registered as (not
  the edge the message names). Each phase is recorded and resets the answer
  timeout. READY is verified against the expected checksum; FAILED is
  terminal (an edge-side failure such as a rejected snapshot is not retried
  automatically; the user sees the edge's message). Late, duplicate and
  unknown reports change nothing.
- When a stream closes, the commands in flight on that session go back to
  `pending` at once, for whichever replica the edge reconnects to. A
  replica that stops ends its edge streams itself (so the edges reconnect
  elsewhere and this happens), without the `ShutdownRequested` notice: the
  edge client stops for good on it.
- A janitor on every replica returns to `pending` the commands claimed by
  dead replicas, lapsed claims and commands the edge has been silent on
  past the answer timeout (up to 3 attempts, then `failed` with every
  attempt in the history), and expires commands past their deadline with
  the specific reason.
- The operation's status is derived from its commands: `in_progress`,
  `succeeded`, `succeeded_with_warnings`, `partially_failed`, `failed`,
  `expired`.
- API: `POST /edges/:id/reload`, `/namespaces/:ns/reload` (ENT) and
  `/edges/reload-all` answer 202 with the operation, its targets,
  warnings and skipped edges; 404 for an unknown edge, 409 when there is
  nothing to push to, 503 when this Studio runs no control server.
  `GET /reload-operations/:id/status` (ENT; 402 in CE) reports every edge;
  `GET /edges/reload-operations` lists the last day's pushes with their
  outcome counts, which is what the CE dialog shows.

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
- Reload-all is one operation over every edge; it lists the edges it
  left out (offline too long) and refuses (with the reason) when there is
  nothing to push to.
- Community Edition shows the push's outcome counts and message (the
  per-edge report is an Enterprise endpoint).
- `GET /api/v1/cluster/status` (admin): live nodes, their edge counts,
  event-log lag, and pending or stuck commands.

## Test plan

Unit tests use fakes; the rest run on Postgres (`DATABASE_URL`) with two or
three in-process replicas sharing a schema, and real microgateway edge
clients over gRPC (`microgateway/tests/cluster`). Every Go test here also
runs on SQLite, and on Postgres when `DATABASE_URL` is set.

Where they are (8b):
- `services/pushes`: the coordinator's rules with fake streams (targets,
  claims, requeues, timeouts, deadlines, attempts, checksum verification,
  version-checked concurrency);
- `grpc/push_delivery_test.go`: the control server's side (stream
  lifecycle hooks, session-bound sends, report attribution, serialised
  sends, stop ending streams);
- `grpc/push_cluster_test.go`: two or three real control servers and
  coordinators with fake edges;
- `microgateway/tests/cluster`: real edges (client, reload handler, own
  SQLite) through a TCP forwarder that can cut and reroute connections;
  each test checks the configuration arrived in the edge's database;
- `api/edge_push_handlers_enterprise_test.go`, `api/edge_handlers_community_test.go`:
  the API contract per edition; `PushConfigurationModal.test.js`,
  `edgeGatewayService.test.js`: the dialog and its CE fallback.

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
- the edge never answers: retried after each answer timeout, then
  `failed` after three attempts (or `expired` if the deadline comes first);
- the stream closes between the send and its record, or the dispatcher
  sends on a stream that has just closed: requeued, no history lost, no
  attempt used when nothing was sent;
- a replica is stopped (rolling restart) while an edge is reloading: its
  streams end, the push is finished by the edge's new replica;
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
