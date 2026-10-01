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
- **Post-2.2 sweep follow-ups (M2, M3, L3, L7, L8):** pushes reach an edge
  that is still starting up at once (the edge holds them until its reload
  handler is set; control waits for the stream's first heartbeat or a
  10 s grace); the edge's reconnect backoff is capped at 30 s; v2.2 fields
  on Enterprise reload-all; the API contract change is documented below.

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

Each row also carries a `label` for operators (`Options.NodeLabel`, default
`studio`; `ControlPlaneOptions.NodeLabel`, default `control-plane`, e.g.
`mdcb-eu-1`; at most 64 letters, digits, spaces and `. _ - : / ( )`) and
`leader_eligible` (false for a headless control plane, which never takes the
leader lease). Both are nullable: a row written before they existed counts
as unlabelled and eligible. Eligibility is recorded for operators; a
replica's own code decides whether it contends for the lease.

### Edge stream ownership

`edge_instances` gains `owner_node_id` and `stream_session_id`. Opening a
stream sets both (a new session UUID) and status `connected`. Every write
that ends a stream (close, stale sweep) is conditional on the session it
belongs to, so a stale replica never overwrites a newer connection. An edge
is *reachable* when its owner node is live and its last heartbeat is fresh.

Edges heartbeat on their stream. The deprecated unary `SendHeartbeat` RPC
(no current edge calls it) used to look the edge up in the replica's own
stream table and answer NotFound on any other replica; it now reads and
updates `edge_instances`, so it works on every replica, and leaves ownership
alone.

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
- Every replica runs a dispatcher, woken by NOTIFY, by an edge's first
  heartbeat on a stream, and by a 1 s poll: for each `pending` command
  whose edge has a stream on this replica that is ready for pushes, it
  claims it (`claimed`, `claimed_by`, the
  stream session, `attempts + 1`), sends the reload request on exactly
  that stream and marks it `sent`. Only one replica wins a claim. A send
  that finds the stream already gone (the dispatcher's view is a moment
  old) hands the claim back without using an attempt; a transport error on
  a live stream uses one.
- A stream is ready for pushes after its first heartbeat, or once it has
  been open for 10 s. An edge opens its stream while it is still starting
  up; the microgateway used to set its reload handler only once its
  services were up, drop a push that arrived before, and leave it for the
  answer timeout (a minute, and an attempt). Edges now send a heartbeat as
  soon as the stream is open and hold a push that arrives before the
  handler is set (up to 16, a resend of the same operation replaces the
  held one), handing it over when the handler is set; the grace covers
  older edges, whose first heartbeat comes a full interval (30 s) later.
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

#### API changes from v2.2 (upgrade note)

Paths and the response envelope (`data.type`, `data.id`,
`data.attributes.operation_id|target_namespace|status|message`) are
unchanged, and every v2.2 attribute is still returned. What changed:

| | v2.2 | now |
|---|---|---|
| final `status` | `completed`, `failed`, `timed_out` | `succeeded`, `succeeded_with_warnings`, `partially_failed`, `failed`, `expired` (`in_progress` until then; no `initiated`) |
| operation IDs | `reload-...`, `edge-reload-...` | `push-<uuid>` |
| ENT `reload-all` | one operation per namespace: `data.{message, operations[], operations_count}` | one operation (`data.attributes`); `data.message`, `data.operations` (one entry per namespace pushed to, all with the same `operation_id`) and `data.operations_count` are kept for v2.2 clients |
| no edge at all | ENT `reload-all` 202 with no operations | 409 |
| namespace with no edge to push to | 404 | 409 |
| offline edge (`/edges/:id/reload`) | 500 | 202, and the push waits for the edge until its deadline |
| `GET /edges/reload-operations` | operations in memory on this replica, with `target_edges` | the last day's operations from the database, with outcome `counts` (no `target_edges`; the status endpoint lists the edges) |

A script that waits for `completed` never finishes: it must treat
`succeeded` (and `succeeded_with_warnings`) as done and `failed`,
`partially_failed` and `expired` as failed. A single `status` field cannot
carry both spellings, so no alias is returned.

### Fan-out events: `cluster_events` and the bus relay

A replica publishing a cluster-wide event inserts a row (topic, payload,
origin node) and sends `NOTIFY` with its id. Each replica keeps a cursor and
reads new rows on NOTIFY, after every listener reconnect, and every 5 s (the
poll only covers a listener connection that died unnoticed; at 1 s it was
two queries a second on every replica); it also re-reads a 30 s window and
skips ids it has seen, so a row whose transaction committed after a later id
is not missed. Rows older than 15 minutes are pruned. The listener is the
chat queue's shared listener, moved to `pkg/pglisten`.

The listener connects with pgx from the pool's own DSN, so it accepts
exactly what the pool accepts (no `sslmode`, `sslmode=prefer`,
`default_query_exec_mode`...). Before the post-2.2 fix it used lib/pq,
which refused those DSNs; and the log's failed start left `Stop` waiting on
a reader that never ran, so Studio hung at startup without opening a port.
Now:

- The notifications are only an accelerator, for the event log and for
  push delivery alike: when the listener cannot connect or subscribe,
  `pglisten.Follow` logs a warning, the service polls alone (every second)
  and the listener is retried every 30 s; once it connects, a catch-up read
  runs. Startup never fails, and never waits, on the listener.
  `LogStats.Listening` and a cluster-status warning show a replica running
  without it. Other start errors (a failing query) still fail `studio.New`.
- `Log.Stop` and `pushes.Coordinator.Stop` return at once whether `Start`
  ran, failed or succeeded; a `Start` after `Stop` does nothing.
- One goroutine owns the listener's connection: it waits for notifications,
  issues LISTEN/UNLISTEN (bringing the connection in line with the
  subscribed channels whenever they change), pings after 90 s of quiet and
  reconnects with backoff. Subscribing wakes it by cancelling its wait,
  which pgx's deadline-based cancellation does without closing the
  connection.
- Behind PgBouncer in transaction mode, LISTEN does not work (the server
  connection changes between statements); the log and pushes then run on
  their poll.

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
their label, `leader_eligible`, edge counts and the leader, this replica's
event log and relay counters, the push backlog (pending, in flight, held by
stopped replicas), and warnings.

The edges list, detail and namespace listings name each edge's owning
replica: `owner_node_id`, `owner_label` and `owner_live` (the owner's
registration is fresh), looked up with one query per page
(`cluster.Owners`). The Edge Gateways page shows a "Held by" column once the
listed edges are held by more than one replica, and the edge's detail page
shows its owner.

### Replicas that never lead

A headless control plane (`studio.NewControlPlane`, embedded in MDCB) is a
replica like the others (registry row, event log, relay, replica signals,
edge streams and pushes) except that it never contends for the leader
lease: it creates no `Leadership`, and its `pkg/replicas` backend answers
`IsLeader` false for good. (With no backend at all, `pkg/replicas` treats
the process as the only replica, which leads; a replica without a lease
must say so.) So leader-only work, including the jobs only a full Studio
has (marketplace sync, telemetry), always runs on a full Studio, and the
relay carries its `budget.sync` to the edges a control plane holds. Its
budget sync service still computes spend for its own snapshots, as on any
non-leader. While no full Studio is up, nothing leads: its edges keep the
last budget blocks they were sent.

**Edge-to-control traffic.** Edges send two things for plugins, which a
control plane does not run:

- *Events published `DirUp`.* A control node's bridge republishes an edge's
  events on its bus as `DirLocal` (so they are never forwarded again) and
  marks them `FromEdge`, a local field like `RelayedFrom`. A control plane's
  relay also relays `FromEdge` events (`headlessRelayFilter`); a full replica
  keeps `RelayedByDefault`, since its own plugins already had its edges'
  events. On the receiving replicas the event is published with
  `RelayedFrom` set, which stops it being relayed again, and as `DirLocal`
  it never goes down to their edges.
- *Plugin payloads (`SendPluginControlBatch`).* A replica without a plugin
  manager forwards the edge's batch (`grpc.EdgePayloadForwarder`) as one log
  row per payload on topic `plugin.control`, its proto encoding (edges cap a
  payload at 1 MB; a row is `bytea`), written in one statement with one
  `NOTIFY` (`cluster.Log.PublishBatch`), and answers the edge "queued for the
  plugin host". A failed write fails every payload of the batch in the
  response, not with a gRPC error, as the edge would otherwise keep and
  resend the whole batch. Every full replica with a plugin manager subscribes
  (`edgePayloadHost`, `pkg/studio/edge_payloads.go`); only the one leading at
  delivery routes the payload to `RouteEdgePayload`, one row once (ids are
  remembered for the log's retention, and forgotten by a minutely sweep).

  The log gives a replica no history: rows it reads while it does not lead
  are gone for it. So a replica that becomes the leader reads the
  `plugin.control` rows of the last 45 s (the default lease TTL and a
  margin, `cluster.DefaultLeaseTTL`) from the table, a page at a time, and
  routes those it has not routed. That covers a leader that
  crashed with payloads unhandled and the gap while no replica led, at the
  price of handing plugins again what the previous leader handled in that
  window (on any leader change, a clean handover included). Payloads that
  arrive while no full Studio runs for longer than 45 s are lost, and a
  payload a plugin fails on is not retried, as on a single Studio.

`Relay.Stop` stops taking the log's events and waits for one being
published on the bus before it leaves the bus: the bus library's
`Unsubscribe` is not synchronised with a `Publish` in flight on another
goroutine (the log's reader).

### Schema version: instances that do not migrate

Every full replica migrates under the migration lock and then records the
schema version (`studio_schema`: `version`, `min_reader_version`, writer,
time; never lowered). A replica that must not migrate a database it shares,
such as a headless control plane embedded in another product, checks it
first with `studio.CheckSchema`, which only reads: it refuses a schema that
is missing or older than it needs, or one whose `min_reader_version` is
newer than its own `models.SchemaVersion`. A newer schema that still lists
it as a reader is accepted, so such a replica can be upgraded after the full
ones. See `features/Embedding.md` ("Schema version and `studio.CheckSchema`")
for the bump rules and the golden guard.

### Panics in the cluster machinery

The replica's long-lived loops (node heartbeat, lease renewal, event log
reader, relay, Postgres listener and its follower retry, push dispatcher and
janitor, the control server's budget sync and connection cleanup, the replica
signal sender) run under `safe.Loop`: a panic is logged with its stack,
counted (`aistudio_goroutine_panics_total`), and the loop starts again after
a backoff (1 s, doubling to 30 s) instead of ending the process. Each loop
still closes its done channel once, when it stops, so `Stop` behaves as
before. Restarts start the loop from the top; a few need more:

- **Lease.** A panic (in a renewal, or in a lease listener) drops the
  leader belief at once and tells the listeners, rather than letting it run
  out: the replica cannot vouch for renewals it did not make. The restarted
  loop renews, and the replica leads again if it still holds the row.
- **Postgres listener.** A panic costs the connection: it is closed and the
  owner reconnects, and the reconnect hooks run (notifications may be lost),
  exactly as after a disconnect. A panicking subscriber handler or
  reconnect hook is recovered on its own, so it neither loses the
  connection nor leaves the listener's lock held.
- **Edge streams.** A panic handling an edge's messages ends that edge's
  stream with `Internal`; the edge reconnects, to this replica or another,
  and pushes in flight on the stream are requeued, as when a stream drops.

Event bus subscribers run on the publisher's goroutine (an API request, an
edge stream, the event log reader); each is recovered on its own, so one
subscriber's panic neither ends the publisher nor keeps the event from the
others.

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
  nothing to push to. The dialog shows that refusal at its top, stops
  saying it will push to every namespace, and disables Push until the
  target changes.
- The "not connected; the push waits up to 5 minutes" warning stands only
  while an edge is still waiting to connect; an edge that has its push
  (reloading) no longer counts.
- The change preview measures from the last push or, when none was
  recorded, from the latest time an edge confirmed it was in sync
  (connected or not, whatever changed since); it says "never pushed" only
  when no edge ever confirmed.
- Community Edition shows the push's outcome counts and message (the
  per-edge report is an Enterprise endpoint).
- `GET /api/v1/cluster/status` (admin): live nodes, their edge counts,
  event-log lag, and pending or stuck commands.

## Test plan

Unit tests use fakes; the rest run on Postgres (`DATABASE_URL`) with two or
three in-process replicas sharing a schema, and real microgateway edge
clients over gRPC (`microgateway/tests/cluster`). Every Go test here also
runs on SQLite, and on Postgres when `DATABASE_URL` is set.

CI runs them on Postgres in the "Go Postgres Tests" job
(`.github/workflows/ci-test.yml`): `pkg/cluster`, `pkg/pglisten`,
`services/pushes` and `services/scheduler` whole, `grpc` filtered to
`TestPushCluster_|TestRelay_|_Postgres`, and `microgateway/tests/cluster`,
all with `-race`. The unit jobs cover the SQLite cases.

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
- an edge that restarts while a push waits for it, and sets its reload
  handler a moment after connecting (as cmd/microgateway does): the push
  succeeds in seconds with one attempt (`TestE2E_PushToEdgeStillStartingUp`);
  an older edge that drops pushes while starting up gets it after the grace
  (`TestPushCluster_OldEdgeStartingUpGetsThePushAfterTheGrace`);
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
