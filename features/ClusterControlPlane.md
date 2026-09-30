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
- **8c, done:** the bus relay (`pkg/cluster.Relay`), the leader lease
  (`pkg/cluster.Leadership`) and `pkg/replicas` (leadership and signals for
  core and Enterprise code), the scheduler lease as a conditional update,
  cross-replica gateway reload, budget cache clearing and plugin lifecycle,
  leader-only jobs, and `GET /api/v1/cluster/status`.

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

Each replica has a node ID (`Options.NodeID`, default
`hostname-pid-random`, fresh for every process so a restarted replica never
inherits its predecessor's claims) and upserts `cluster_nodes(node_id,
hostname, version, started_at, last_seen, pid, boot_id, pid_namespace)`
every 5 s (one statement). A node is live while `last_seen` is under 20 s
old. `pid`, `boot_id` (the running kernel's boot ID: different on every
machine and after every reboot, shared by the containers of one machine)
and `pid_namespace` locate the process, so a replica restarted after a
crash can recognise its dead predecessor (see the leader lease). A node
that stops cleanly deletes its row; the rows of nodes that crashed are
deleted by any node once they are an hour old (`NodeRetention`), at start
and every minute. Single replica and SQLite: the same code runs with one
node.

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
  the specific reason. It first asks whether any command or operation is
  open at all (one query); when none is, it only prunes, and looks again
  after 10 s rather than 2 s unless a push is announced first (a
  notification from any replica, or a push or stream on this one). An idle
  hub no longer runs its seven queries every 2 s; a push whose notification
  was lost is picked up by an idle janitor within 10 s.
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

### Fan-out events: `cluster_events` and the bus relay

A replica publishing a cluster-wide event inserts a row (topic, payload,
origin node) and sends `NOTIFY` with its id. Each replica keeps a cursor and
reads new rows on NOTIFY, after every listener reconnect, and every 5 s (the
poll only covers a listener connection that died unnoticed; at 1 s it was
two queries a second on every replica); it also re-reads a 30 s window and
skips ids it has seen, so a row whose transaction committed after a later id
is not missed. Rows older than 15 minutes are pruned. The listener is the
chat queue's shared `pq` listener, moved to `pkg/pglisten`.

`pkg/cluster.Relay` joins every replica's event bus to the log. It relays
two kinds of bus events, from a queue (the bus is synchronous, so a
publisher never waits for the database; a full queue drops and counts):

- **Events for edges (`DirDown`)**: budget sync, plugins' events. Every
  replica republishes them on its bus, and its edge bridges forward them to
  the edges whose streams it holds, so each edge gets each event once.
- **Object changes (`system.*`)**: every replica's plugins and caches hear
  about changes made on any replica.

Relayed events keep their ID and carry `RelayedFrom` (local metadata, never
serialized), which also stops them being relayed again. Consumers choose:

| Consumer | Relayed events |
|---|---|
| Edge bridges | forward (that is the point) |
| Plugins (plugin event service) | receive: each replica runs its own plugin processes |
| Webhooks (Enterprise) | skip: the origin persists them (idempotent on ID anyway) |
| Checksum recompute | skip: the origin recomputes every namespace |
| Resource-instance refresh | skip: the origin made the RPC and wrote the rows |
| `pkg/studio` watcher | apply: reload the gateway, clear budgets, update plugins |

Changes without an object event travel as **replica signals**
(`pkg/replicas.Signal`, cluster log topic `replica.signal`): `budgets` (App
budget reset, team-budget switch) and `governed_metadata` (schema or
vocabulary change, Enterprise). The replica that made the change refreshes
itself; the others run their `OnSignal` handlers.

What each replica keeps current, and how:

- **Embedded gateway** (LLMs with filters and plugins, datasources):
  reloaded, coalesced, on any `system.llm|datasource|filter|plugin.*`
  event, local or relayed (not every local write path reloaded it before).
- **Budget caches**: cleared on relayed `system.app.*` and the `budgets`
  signal. Team budget settings also have a 30 s TTL.
- **Studio plugins**: a relayed `system.plugin.*` event re-reads the
  plugin's permission entries into the catalogue (no writes: the origin
  refreshed the system roles), stops a deleted or deactivated plugin,
  restarts one that was running, and starts one Studio loads at start.
  Scheduled tasks start their plugin if the scheduler's replica has not.
- **Governed metadata schemas** (Enterprise): dropped on the signal.
- **Webhook targets** (Enterprise): 30 s refresh, as before.

What stays per replica and needs deployment support: chat and agent
sessions and MCP SSE sessions (session affinity), branding uploads and log
export files (a shared volume), `file://` plugin commands (present on every
replica).

### Singleton jobs: the leader lease

`pkg/cluster.Leadership` holds the `leader` row of `cluster_leases`: taken
and renewed (every 10 s, TTL 30 s) with one conditional write against the
database clock, released on shutdown. A replica believes it leads only until
its last renewal plus the TTL minus one renewal period, by its monotonic
clock, so it stops acting before another replica can take over, even
without the database. `pkg/replicas.IsLeader` is the check for code that
does not hold the lease itself.

**After a crash.** A process that is killed never releases the lease, and
its restart has a new node ID, so without more it would wait up to the TTL
(30 s) while leader-only work (start-up marketplace sync, first telemetry
report, budget blocks to edges) is skipped. v2.2.0 had no lease, so this
was a regression for single-node installs. Now:

- **SQLite** serves one process by design (the event log is inert there),
  so that process leads from `Start` to `Stop`, whatever the row says. It
  still writes itself into the row, for the status page. v2.2.0 behaved the
  same: every process ran every job.
- **Postgres**: when another node holds the lease, the replica reads the
  holder's registration (one query per renewal period on non-leaders) and
  takes the lease over at once, with a write conditional on the holder
  being unchanged, only when the holder is provably its dead predecessor.
  It must never take a live replica's lease: that replica would keep
  acting as leader for up to 20 s more. So the evidence is local to the
  host:
  - same `boot_id` and hostname (a hostname alone is not enough: machines
    cloned from one image share one), otherwise wait for the TTL; a holder
    without a registration or registered before these columns existed
    also waits;
  - same `pid_namespace` (bare metal, VMs, systemd, one container): the
    holder is gone when its pid does not exist (`kill(pid, 0)`), or when it
    is this process's own pid (a container's pid 1 in a reused namespace)
    and no node of this process has that ID (tests run several replicas in
    one process). A live pid, including a reused one, keeps the lease: two
    Studio processes on one host sharing a database both keep working;
  - another pid namespace on the same machine and hostname (a container
    restarted in its pod: pid 1 again, new namespace) cannot be checked by
    pid, and looks like two live containers given one hostname (host
    networking, an explicit `hostname:`). Silence separates them: the
    holder is gone once it has not refreshed its registration for
    `PredecessorSilence` (two heartbeats, 10 s), re-checked exactly when
    that is due and again in the write. So a crashed container's successor
    leads 5 to 10 s after the crash instead of up to 30 s.
  - Other platforms (no boot ID): wait for the TTL.
- **Catching up.** Leader-only work skipped because the lease was not held
  yet runs when it is gained: `pkg/replicas.OnLeading` (fired by
  `pkg/studio` on the lease's change listener) wakes the marketplace sync,
  the first usage-telemetry report (once per process) and a budget sync to
  the edges at once. Enterprise licence telemetry starts a minute after
  start and is not hooked.

The plugin scheduler's own lease (`scheduler_leases`, 2 min TTL) is
unchanged: its instance ID is `hostname-pid-counter`, as in v2.2.0, so a
container restart (pid 1 again) keeps it, and elsewhere a crash waits out
the TTL.

Leader-only:

- budget blocks, alerts and the `budget.sync` to edges (every replica still
  computes spend for its own snapshots; the relay carries the leader's
  sync to every replica's edges);
- marketplace background sync (a manual refresh runs where it is asked);
- usage telemetry (CE) and licence telemetry (Enterprise).

The plugin scheduler keeps its own lease (`scheduler_leases`), now also a
conditional update on the database clock (it was read-then-save, so two
replicas could both take it). Idempotent cleanups (event log, webhook,
audit, sync-run retention) run on every replica.

`GET /api/v1/cluster/status` (edges:read) reports the live replicas with
their edge counts and the leader, this replica's event log and relay
counters, the push backlog (pending, in flight, held by stopped replicas),
and warnings.

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

Leader lease (`pkg/cluster/lease_test.go`, `services/replicas_leader_test.go`,
`grpc/budget_sync_leader_test.go`):
- replicas starting at once: exactly one leads; stopping it hands over at
  once; a holder that loses the database stops believing before anyone
  else can take over;
- SQLite: a restarted process leads at once although the row still names
  its killed predecessor;
- Postgres: a holder on this machine and hostname whose process exited, or
  whose pid is this process's own, is taken over at once; a live pid on this
  host, a replica of this process, another machine with the same hostname,
  another hostname and a pre-upgrade registration are not; a holder in
  another pid namespace is taken over once silent for `PredecessorSilence`,
  and never while it keeps refreshing;
- marketplace sync, first telemetry report and budget sync run when the
  replica becomes the leader after its start-up run was skipped;
- node rows an hour old are pruned, recent ones kept.

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
