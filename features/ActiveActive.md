# Active-active control planes and edge resilience

**Status:** built in 2.3.0 (as of 2026-11-12; the last pieces, a
full-sync edge syncing on `credential.changed`, are #991 and #998). The
operator documentation is `docs/site/docs/deployment-active-active.md`.
Target: **2.3.0** (a priority addition during the release-candidate
phase). This is the feature specification and
the build plan; the work is tracked in bd under the epic "Active-active
control planes and edge resilience". **Keep this file current:** whoever
changes the design while building it updates this spec in the same PR.

Two AI Studio control planes (CPs) run in two sites, both serving the UI,
the API and edge gateways. Either site can be lost, including its
PostgreSQL node, and the other carries on. Edges survive a control-plane
outage, restart during one, and can validate credentials they have never
seen.

Out of scope: Studio's embedded gateway (not part of this topology), and any
new data store. Everything here runs on what a deployment already has:
PostgreSQL for the control planes, each edge's own database (SQLite or
Postgres), and local files.

## Requirements and where they stand today

| # | Requirement | Today | After |
|---|---|---|---|
| R1 | A Postgres failure in one site is absorbed by the other, quickly | Studio handles only one writable database. Multi-host DSNs pass through to pgx untested; nothing detects a connection left on a demoted primary; startup exits if the first ping fails | Supported topology on Postgres streaming replication, with Studio following a failover without a restart (Phase 1) |
| R2 | Key creation during a failure is queued | A database error is a 500 (`api/credential_handlers.go:29`). The only outbox in the code is for webhooks | Writes are held in the request across a failover and applied when the new primary is up (Phase 1). A durable journal for longer outages was considered and dropped (see Decisions) |
| R3 | Keys created during a switchover are available to edges | Edges learn keys only by asking the CP they are connected to; one `CONTROL_ENDPOINT`, no failover between endpoints | Edges fail over between CPs; committed keys survive the switchover (synchronous replication) and reach edges by delta sync (Phases 2 and 4) |
| R4 | An edge restarted during a CP outage starts from its last-known-good configuration | **It exits** (`microgateway/cmd/microgateway/main.go:185`), even with a full configuration in its database. Templates and guardrail exceptions are held in memory only | Opt-in start from the last applied snapshot (Phase 3) |
| R5 | Option to sync the full credential list at start | Credentials are never synced. An unknown key during an outage is answered 503; a cached one lasts 5 minutes (`EDGE_TOKEN_CACHE_TTL`), plus `EDGE_TOKEN_CACHE_STALE_GRACE`, which is off by default | `EDGE_CREDENTIAL_SYNC=full` (Phase 4) |
| R6 | Credentials hashed at rest on the edge | See below | Edges receive and store only keyed hashes (Phase 4); the hub's own copy is hardened (Phase 6) |

### The at-rest finding, precisely

- **Edge mode today stores no App credentials on disk.** Validated keys live
  in an in-memory map keyed by the raw token
  (`microgateway/internal/services/hybrid_gateway_service.go:44`). The
  `token_cache` table exists but this path does not use it.
- **The plaintext columns are elsewhere.** On the hub, `credentials.secret`
  is plaintext and looked up by value (`grpc/control_server.go:1165`,
  `models/credentials.go:15`). On a standalone microgateway,
  `api_tokens.token` is plaintext
  (`microgateway/internal/database/models.go:14`).
- **So the concern becomes real the moment R5 is built naively.** The plan
  makes R5 hash-only from the start. It also matters more for the hub once
  its database is replicated to a second site.

### What already exists and is reused

`features/ClusterControlPlane.md` (2.3) already lets several Studio replicas
share one Postgres: durable pushes that any replica delivers, the
`cluster_events` log and bus relay, the leader lease, edge stream ownership,
`GET /api/v1/cluster/status`. Active-active is that design stretched over
two sites, plus a database that can move between them. No second
coordination mechanism is added.

## Topology

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

- **One logical database.** Both sites' Studio replicas write to whichever
  Postgres node is primary. Multi-primary Postgres (BDR, bidirectional
  logical replication) is **not supported**: Studio relies on sequences,
  the database clock for leases, and advisory locks.
- **Failover is the database layer's job**: a managed service with
  cross-zone or cross-region HA, CloudNativePG, or Patroni. Studio's part is
  to follow it.
- **Two sites cannot fail over automatically and safely on their own.** A
  third location must break ties (a managed service's control plane, a
  Kubernetes API in a third zone, or a witness). Without one, the choice is
  manual promotion or the risk of split brain. The documentation must say
  this plainly: until an operator promotes the standby, writes fail with a
  retryable 503 and are not queued.
- **Synchronous replication between the sites** (`synchronous_commit=on`
  with the other site as a synchronous standby) is the recommended setting.
  With asynchronous replication, a key acknowledged just before a failover
  can be lost: R3 cannot be met.
- **Per-site labels.** `CLUSTER_NODE_LABEL` (built) sets
  `studio.Options.NodeLabel` in the Studio binary, so the cluster status and
  the Edge Gateways page show which site holds each edge. The binary checks
  it with `cluster.ValidateLabel` before anything else starts and exits
  naming the setting when it is invalid (at most 64 characters: letters,
  digits, spaces and `. _ - : / ( )`); unset keeps the label `studio`.

## Guarantees after this work

1. With automatic database failover, losing either site (Studio, Postgres or
   both) needs no operator action. Studio replicas in the surviving site
   resume writing within seconds of the promotion, without a restart.
2. API writes in flight during a failover are held and retried up to
   `DATABASE_WRITE_RETRY_WINDOW`, then fail with a retryable 503. They are
   never applied twice.
3. A running edge keeps serving through any control-plane outage, moves to
   the other CP on its own, and loses no configuration push (pushes are
   already durable).
4. With `EDGE_START_FROM_LAST_KNOWN_GOOD=true`, an edge with an intact
   database starts and serves with no CP reachable.
5. With `EDGE_CREDENTIAL_SYNC=full`, an edge validates every active key of
   its namespace locally, CP or no CP, from hashes it cannot reverse.
6. A key whose creation was committed is valid on every edge after a
   switchover. A key creation held during a failover commits when the new
   primary is up, and is valid on edges from then.

Not guaranteed: revocation reaching an edge that cannot reach any CP (the
price of 4 and 5, bounded by configurable maximum ages); a key being usable
on edges before its creation has committed; writes surviving an outage
longer than the retry window (they fail with a retryable 503).

## Build plan

Phases 0 to 4 and 6 (Phase 5 was dropped; the numbering is kept so
references stay stable). 1 to 4 are independent enough to run in parallel
once Phase 0's test rig exists. Each phase ends with its tests green on
SQLite and Postgres and its documentation and changelog entries written.

### Phase 0: test rig

Nothing below can be verified without a database that actually fails over.

- `pkg/testinfra/containers`: a two-node Postgres 16 pair (primary and
  streaming hot standby) with helpers to stop the primary, promote the
  standby, and bring the old primary back as a standby.
- `dev/docker-compose.active-active.yml` and `make dev-aa`: two Postgres
  nodes, two Studio replicas with site labels, a TCP proxy per CP that can
  be cut, two edges with persistent volumes.
- Extend the forwarder in `microgateway/tests/cluster` to give an edge two
  CP endpoints.
- CI: add the failover package to `scripts/ci-postgres-tests.sh` (the
  "Go Postgres Tests" job).

### Phase 1: Studio follows a database failover (R1, first half of R2)

Files: `pkg/studio/database.go`, `startup/connectivity.go`,
`config/config.go`, `pkg/pglisten`, `pkg/cluster/events.go`,
`api/health_handlers.go`.

1. **Multi-host DSN as a supported configuration** (built).
   `DATABASE_URL=postgres://u:p@pg-a,pg-b/db?target_session_attrs=read-write`
   for the pool and for the `pglisten` connection (which parses the same
   DSN). Tested on the Phase 0 pair (`pkg/studio/failover_pair_test.go`).
2. **Leave a demoted primary** (built). A pooled connection to a node that
   was demoted stays open and fails every write with SQLSTATE 25006.
   Studio now opens the Postgres pool itself (`pkg/studio/database_postgres.go`,
   `stdlib.OpenDB`; the gorm dialector is given the pool) so it can retire
   connections: a `dbstate.Tracker` (`pkg/dbstate`) probes the server every
   5 s. A probe may have to dial, so it is bounded by the time a dial can
   legitimately take, the connect timeout for every attempt pgx makes (each
   host, twice with `sslmode=prefer`) plus 3 s for the query, not by a
   short fixed time: a first host that does not answer at all must not
   make a database that is writable on its second host look unreachable
   (it reads `pg_is_in_recovery() OR transaction_read_only`, the latter
   because a demoted primary that keeps running is not in recovery) and is
   told of
   every statement refused with 25006 (a gorm callback). While the server
   is not writable it raises a generation number; each connection is
   stamped with the generation it was dialled under and the driver's
   reset-session hook discards one with an older stamp when it would next
   be reused, so it is dialled again, to the writable node. database/sql
   has no "close every connection", and this also catches connections that
   were in use at the time. The first refusal while the database counted
   as writable acts at once; after that the probe does, once per period,
   so a server that stays read-only is not redialled for every statement.
   Known behaviour, for items 5 and 7: a 25006 from one connection that
   was held across a failover (in a long transaction, say) and is used
   after everything else has moved flips the state to `read_only` and
   retires the good connections too, until the probe it brings forward
   answers, milliseconds later. It errs on the safe side. And the state
   can lag the truth by up to one period in the other direction: an idle
   replica on a demoted primary still says `writable` until the next probe.
   The tracker's `Status()` (`writable` / `read_only` / `unreachable`, time
   of the last change, failover count) is the accessor items 5 and 7 use;
   `dbstate.For(db)` finds it, and is nil (writable) for SQLite and for a
   pool a host opened itself. A failover is counted when the writable
   server's address changes, or its timeline after an outage (one address
   in front of both nodes).
   The listener checks its server every `pglisten.RoleCheckInterval` (5 s,
   also its keep-alive now) and reconnects when it is read-only, running
   its reconnect hooks as after any reconnect; a hot standby refusing
   `LISTEN` (25006) is a reason to reconnect, no longer a channel failure.
   Reconnects that keep reaching a read-only server back off to the
   reconnect ceiling.
3. **Pool settings** (built). `DATABASE_MAX_OPEN_CONNS`,
   `DATABASE_MAX_IDLE_CONNS` (unset: database/sql's defaults),
   `DATABASE_CONN_MAX_LIFETIME` (default 30 minutes, so connections also
   drift back after a planned switchover) and `DATABASE_CONNECT_TIMEOUT`
   (default 10 s, applied when the DSN sets no `connect_timeout`). Logged
   at debug at start. All inert for SQLite.
4. **Start-up waits for the database** up to `DATABASE_STARTUP_TIMEOUT`
   (default 60 s) instead of exiting on the first failed ping, so a replica
   that starts mid-failover joins when the primary appears.
   **Built** (midsommar-clean-4x5y.5). `studio.OpenDatabase` opens the pool
   without gorm's ping and then, on Postgres, tries until a ping succeeds
   and the server is writable (`pg_is_in_recovery() OR
   transaction_read_only`), then creates `DATABASE_SCHEMA` if set; retries
   after 0.5 s, doubling to every 5 s; each kind of failure (cannot
   connect, no read-write server, read-only server, a SQLSTATE, timeout)
   is logged once, with progress every 10 s. A read-only server is never
   returned, so `New` never migrates a standby; at the timeout the error
   says `no writable database within DATABASE_STARTUP_TIMEOUT`, and `0`
   means one attempt (a read-only server then fails at once too). SQLite is
   opened once. The wait is inside start-up, before anything listens, so
   neither `/health` nor `/ready` answers during it (Martin, 2026-11-12):
   a deployment needs a startup probe of at least the timeout plus the
   migrations. `main.go` now opens the database before the one-shot
   connectivity tests, which would otherwise fail first. Embedders get the
   wait through `OpenDatabase`, which the README already has them use;
   an `AppConf` built by hand has a timeout of 0.
5. **Hold writes across the failover.** A wrapper at the gorm `ConnPool`
   retries statements that fail before reaching a writable server
   (connection refused, 25006, 57P01 to 57P03) until
   `DATABASE_WRITE_RETRY_WINDOW` (default 15 s). Only statements outside an
   explicit transaction are retried; a transaction that loses its
   connection fails as now and the handler returns 503 with `Retry-After`
   and code `database_unavailable` instead of 500. This covers key creation
   during an automatic failover with no API change.
   **Built** (midsommar-clean-4x5y.6). `pkg/dbhold.Pool` wraps Studio's own
   Postgres pool as gorm's `ConnPool` (installed by `OpenDatabase` when the
   window is above 0). Retried, because the statement cannot have
   executed: a pgconn `ConnectError` (never sent), 25006 (refused before
   executing; the tracker is told, so the connection is redialled), 57P03
   (only at connection start), `ErrBadConn` (database/sql's contract: not
   returned if the server might have performed the operation) and pgconn's
   `SafeToRetry` (failed before sending anything); backoff 100 ms doubling
   to 1 s. 57P01 and 57P02 are not in that list: 57P02 comes from the
   quickdie handler, possibly after an autocommit statement's commit record
   was flushed, so the write may exist (57P01 is treated the same). After
   them, as after a connection lost once the statement was sent (a pooled
   connection to a primary that has gone answers EOF), only a repeatable
   statement is retried: a read through `QueryContext` (a plain `SELECT`),
   a prepare, `BEGIN`. `ExecContext` is never repeatable (`SELECT
   pg_notify(...)` runs through it for its side effect). On a lost
   connection the pool's idle connections are closed so the next statement
   dials afresh.
   **Bounds.** At most `dbhold.MaxHeld` (512, a constant) statements are
   held at once; beyond it a statement fails at once as
   `UnavailableError`. Once a hold has run out its window, statements that
   fail within the next window are not held: a request runs several
   statements in turn, and this keeps it to about one window in an outage
   longer than the window (after a window the next one is held again, as a
   probe). Measured on the pair rig with no primary, window 10 s, pool of
   8 connections: 3,000 concurrent `POST /api/v1/credentials`, at most 512
   statements held, every request answered 503 `database_unavailable`,
   all within 27 s; goroutines 40 before and 12,042 at most, heap 14 MB
   before and 208 MB at most (both include the test's own 3,000 clients).
   With 1,500 requests: all within 18 s, 121 MB.
   `QueryRowContext` (its error only shows at `Scan`) waits instead while
   the tracker says the database is not writable. The request context ends
   any wait. A wait that runs out is a `dbhold.UnavailableError`;
   `dbhold.IsUnavailable` also matches a transaction that lost its
   connection (class 08, 57P0x, EOF, `ErrConnDone`, `*net.OpError`).
   Metrics `aistudio_db_write_retries_total` and
   `aistudio_db_write_hold_seconds` (label `outcome`: `succeeded`,
   `gave_up`, `cancelled`, `failed`, `rejected`), and
   `aistudio_db_writes_held` (statements held now).
   **Who is held and who fails fast.** Held: the management API, sign-in
   and session checks, and the embedded gateway's request path (it reaches
   the database through the same services as the API; no second Service).
   Fail fast, through `dbhold.Unheld(db)` applied at construction (a
   Session with its own Statement whose pool is the unwrapped one, which
   survives `Session`, `WithContext` and transactions): the control
   server's RPCs (`grpc.NewControlServer`: token validation, pulses,
   heartbeats, configuration pulls, credential sync, its cleanup ticker),
   the push coordinator (`services/pushes.New`: deliveries and the
   janitor) and the cluster loops (`pkg/cluster`: `NewLeadership`,
   `StartNode`, `NewLog`; the relay reads through the log). Why: an edge
   answered "unavailable" at once moves to its other control plane, which
   is the point of the epic; held, a token validation would outlive the
   edge's 5 s `tokenValidationTimeout`, and a pulse (edge timeout 30 s by
   default) would be answered up to the window late. A lease renewal must
   not wait past the lease's term, and a ticker retries on its next tick.
   There is no per-call marker; none is needed today. Tests: a token
   validation and a lease acquisition on a held database with no primary
   fail at once (`grpc`, `pkg/cluster` pair tests).
   **503 mapping.** There is no common API error path (handlers write their
   own 500s, and services mostly call gorm without the request context), so
   a gorm callback and the hold record when a statement found the database
   unavailable, and a middleware answers a 500 written by a request that
   was running since then as 503 with `Retry-After: 5` and
   `errors[0].code` `database_unavailable`, dropping the handler's body.
   Only an exact 500 not yet written changes; nothing is buffered. The
   link between the error and the request is by time, not by request:
   during an outage another cause's genuine 500 reads as 503, which errs
   toward a retryable answer; `aistudio_db_unavailable_responses_total`
   counts the conversions. It is mounted on the whole admin router
   (management API, portal, chat); the LLM proxy has its own server. Sign-in (`auth.Login`) and the session and API-key lookups
   (`AuthMiddleware`) answer the same 503 instead of 401 when the lookup
   failed for that reason; `helpers.SendErrorResponse` maps such an error
   too. gRPC handlers are unchanged (Internal, which edges treat as "hub
   unavailable").
   **Requests that hung for 38 to 60 s** in the load test's failovers were
   statements in flight on pooled connections to the killed primary:
   neither pgx nor database/sql has a read timeout, so a statement waits
   for as long as TCP retransmits to an address that drops packets. The
   pool now dials with TCP keep-alives (5 s idle, 3 probes 2 s apart) and,
   on Linux, `TCP_USER_TIMEOUT` 10 s, so such a connection fails in about
   10 s and the statement is retried or answered 503. A server that is
   alive but stalled (its kernel still acknowledges) is not covered; that
   is a slow database, not a lost one.
6. **Event log after a failover.** If asynchronous replication lost recent
   transactions, `cluster_events` ids can fall behind a replica's cursor
   and new events would be skipped. Detect an id below the cursor (or a
   changed system identifier/timeline) and reset the cursor, running the
   same catch-up as a listener reconnect. Verify the leader lease and push
   janitor against a new primary whose clock differs by a few seconds; the
   lease is timed by the database clock.
   As built (details and the reasoning in `features/ClusterControlPlane.md`):
   events are told apart by content, so one that takes a lost event's id is
   delivered; a cursor ahead of the table is moved back and counted; a
   replica away from the log for longer than its retention reloads from
   the database. The lease is no longer timed by the database clock:
   another replica takes it after watching one renewal go unrenewed, on its
   own clock, for as long as the holder believes in it, so there are never
   two leaders whatever the new primary's clock says (within 30 s), and a
   dead holder's lease is taken at most one TTL plus a clock-rate margin
   after its last renewal (30.5 s with the defaults). Open pushes are
   moved forward by the step when the new primary's clock is ahead (once
   per failover, between 1 s and 5 minutes), and replicas that only look
   dead by the new clock keep their commands. Known behaviour: for up to
   5 s after a failover to a server whose clock is ahead, live replicas can
   show as stopped in the cluster status.
7. **Readiness says what works** (built). A database outage shorter than
   the write hold must not take every replica out of the load balancer, or
   held writes and 503 `database_unavailable` never reach the client
   (found by the load test of bead .29: both replicas went unready at once).
   So: liveness (`/health`, `/healthz`) never looks at the database.
   `/ready` and `/readyz` answer from the `pkg/dbstate` tracker's state,
   never by pinging in the request, with `"database": "writable" |
   "read_only" | "unreachable"` and `"database_since"` in the body.
   Writable and read-only are 200; unreachable is 200 until the database
   has not taken writes for `DATABASE_READY_GRACE` (default 30 s, longer
   than `DATABASE_WRITE_RETRY_WINDOW`), measured from the tracker's
   `Status.NotWritableSince` (kept across read_only <-> unreachable), then
   503. `/ready?require=write` is 200 only when writable, with the same
   grace. The grace starts up to one check period (5 s) plus one probe's
   bound after the loss. A grace of 0 is the behaviour before 2.3 (a ping
   in the request decides). SQLite (no tracker): as before. Studio does not
   listen before it has reached the database at start-up, so `/ready` is
   never 200 before that. Metrics `aistudio_db_writable` (gauge) and
   `aistudio_db_failovers_total` (counter); the cluster status warns, naming
   the replica (label and node ID; in a partition one site may see the
   database and the other not), while its database is not writable. This
   replaces the earlier "unreachable stays a failing probe" on purpose.
8. **Budget sync sequence** (built). The leader's `budget.sync` sequence
   was nanosecond wall-clock time, and edges drop anything not above the
   last one they stored, so a leader in the other site with a slower clock
   was ignored until it caught up. The sequence is now a counter in the
   database with the leader's clock as its floor: `max(stored + 1, now)`,
   one statement on the `budget.sync` row of the new table
   `cluster_counters` (`models.NextClusterCounter`). The stored value makes
   it increase from one leader to the next whatever their clocks; the floor
   keeps values in the clock's range, so edges and Studios of older
   versions mix with it (rolling upgrade, rollback). This replaces the
   earlier wording here ("a sequence or the database clock"): a row works
   the same on SQLite and Postgres, and a pure counter would have needed a
   base above every wall-clock value, which older versions could never
   follow. An edge that receives a lower sequence logs it at WARN once per
   stream. Not covered: a leader older than 2.3 with a slower clock, and
   the first sync after an upgrade when the last older leader's clock ran
   ahead (ignored once, for as long as the difference).
   Lost commits (asynchronous replication only): when a failover loses the
   counter's last draws, the new leader's next value is above the lost ones
   at once if its clock has passed them, which it has unless it is behind
   the old leader's clock by more than the time since the last lost draw.
   If it is that far behind, it continues from the last replicated value
   and edges ignore budget syncs until its clock passes the highest lost
   value: at most the difference between the two clocks. Synchronous
   replication, the recommended setting, loses no draw.

Tests: writes during promotion succeed within the window; a replica left on
the demoted primary recovers without a restart; listener and event log
resume; a replica started with no primary joins later; lease moves between
sites; an edge accepts budget syncs from a new leader with a skewed clock.

### Phase 2: edges fail over between control planes (R3)

Files: `microgateway/internal/config/config.go`,
`microgateway/internal/grpc/simple_client.go`.

1. **`CONTROL_ENDPOINTS`**, an ordered comma-separated list;
   `CONTROL_ENDPOINT` keeps working as a list of one, and setting both is a
   start-up error (as are an empty or repeated entry). Per-endpoint TLS
   server name follows the host unless `EDGE_TLS_SERVER_NAME` is set; one
   CA and client certificate serve every endpoint.
2. **Failover.** On a failed dial, registration or stream, the reconnect
   loop moves to the next endpoint before backing off; the backoff applies
   per full pass over the list. `ValidateToken` and the analytics pulse use
   the current connection as now. **Built** (`reconnectLoop`,
   `tryEndpoints`):
   - At start the edge tries each endpoint once, in order, and exits when
     none accepts it (Phase 3 changes that).
   - When a connection ends, that endpoint has failed once: the others are
     tried at once, in list order after it (after a random wait of up to a
     tenth of `EDGE_RECONNECT_INTERVAL`, so the edges of a lost control
     plane do not arrive at the next in one instant). Only when all have
     failed does the backoff start; each backoff is then followed by one
     pass over the whole list from its first entry. An edge with one
     endpoint behaves exactly as before.
   - With several endpoints, one that does not answer at all is given 5 s
     (`controlConnectTimeout`) instead of gRPC's 20 s connect timeout. A
     connection that goes silent is noticed by the existing keepalive
     (30 s, plus 5 s), not sooner.
3. **Failback.** The edge stays where it is until that connection ends.
   `EDGE_CONTROL_FAILBACK_INTERVAL` (default off) makes an edge that is not
   on its first endpoint try it periodically; Studio's existing
   `GRPC_MAX_CONNECTION_AGE` rebalances without it. **Built**
   (`tryFailback`): the probe is a registration on a new connection to the
   first endpoint. When it is accepted the edge closes its stream, makes
   the new connection current and opens the stream there; the old
   connection stays open for 30 s so calls in flight on it finish. A
   failed probe changes nothing.
   The endpoint in use is reported in the edge's log, in `/ready`
   (`control_endpoint`, `control_connected`) and as
   `microgateway_control_connected{endpoint}`.
4. **Idempotent analytics pulses.** A pulse whose acknowledgement is lost
   when a CP dies is re-sent to the other CP, and the hub has no
   de-duplication (`control_server.go:1250` only logs the sequence), so
   spend is counted twice. Give each batch a stable ID (edge ID, process
   boot ID, sequence) and have the hub record applied batch IDs with a
   short retention, in the transaction that stores the batch.
   **Built:**
   - *Proto:* `AnalyticsPulse.boot_id` (field 17), a random value the edge
     draws once per process. The batch is identified by
     `(edge_id, boot_id, sequence_number)`.
   - *Edge:* a pulse that is not acknowledged is no longer merged back into
     the buffers. It is held as it is and sent again first, under the same
     sequence number; what arrived meanwhile follows as the next batch. A
     held batch can lose records (retention, the buffer cap) but never
     gains any, so a repeat is always a subset of what was first sent.
   - *Hub:* the analytics handler writes in the background, so "the
     transaction that stores the batch" did not exist. A pulse with a boot
     ID now takes a synchronous path (`analytics.RecordEdgeBatch`,
     implemented by the database handler and passed through by `Tee`): one
     transaction inserts the batch key into `edge_analytics_batches`
     (`ON CONFLICT DO NOTHING`; no row inserted means a repeat, which is
     acknowledged without storing anything) and writes the pulse's proxy
     logs, chat records, compliance events and tool calls. The edge is
     answered after the commit, so an acknowledged batch is stored, and a
     batch that could not be stored is answered `Unavailable` and sent
     again. A row the database refuses on its own is left out and logged,
     as the background writer does, so one bad record cannot block an
     edge's analytics.
   - Keys are pruned after 48 hours (twice the edge's default
     `edge_retention_hours`), by whichever replica stores a batch, at most
     hourly.
   - A pulse without a boot ID (an older edge) and a host's own analytics
     handler that cannot store a batch once keep the previous path.
     Capture-only pulses carry no boot ID: capture events have their own
     idempotency keys.
   - *Pulse size* (found by the load test, bead .37). A batch used to be
     everything buffered, so the backlog after failed pulses (a database
     switchover on the control plane's side is enough) went as one message:
     19 MB in the test, above the control plane's 16 MB gRPC limit, refused
     four times and then discarded as if the control plane were unreachable
     (4% of the run's analytics). A batch is now at most `max_buffer_size`
     records and 8 MB; a backlog goes as several batches, each with its own
     sequence number. A batch whose message is still too large (stored
     bodies, or a control plane with a lower `GRPC_MAX_MESSAGE_SIZE`: it
     answers `ResourceExhausted`) is split: the older half keeps the
     sequence number, the newer half goes back to the front of the buffer,
     and nothing is discarded, because the control plane is reachable. One
     record too large by itself is dropped with an error. What the edge
     drops while no control plane answers is unchanged here (bead .36); it
     is counted (`microgateway_analytics_pulse_dropped_records_total`,
     reasons `over_cap`, `expired`, `oversized`) and its log line names
     why pulses were failing: no connection, a control plane that answered
     but could not store, or one that did not answer in time.
   - *What is not kept through an outage* (established in bead .36; not
     changed by this epic unless a fix is chosen there). The records for
     the control plane wait in the edge's memory. A pulse is tried
     `max_retries` + 1 times (4 attempts over 15 s as packaged); when that
     fails, for whatever reason (no connection, a control plane that
     answers but cannot store, a timeout), the batch and everything
     buffered since are trimmed to `max_buffer_size` records (10,000,
     about 3,300 requests), oldest first, at every failed pulse. After the
     outage the control plane receives those newest records and nothing
     older: the edge does not resend from its own database, whose
     `analytics_events` rows lack what a pulse carries (the budget record,
     the proxy summary, model and vendor, stored bodies). So an outage of
     T seconds at R requests/s loses about R x (T - 15) requests' records
     (the load test: 698,000 rows in 3 min 28 s at 3,200 requests/s). The
     loss is logged with the reason pulses failed and counted
     (`microgateway_analytics_pulse_dropped_records_total{reason}`).
     Budgets: an edge enforces on the higher of its own count (complete)
     and the control plane's total; the control plane's total, its blocks
     and its reports stay short by what other edges could not report, so
     an App can overrun by that. Measured cost of keeping more in memory:
     about 1.8 KB per request (three records), against 282 bytes per
     request as a pulse message. "Pulses queue as in any outage", as an
     earlier draft of this spec and the docs said, was true only up to
     the buffer.
   - *The backlog* (bead .36, option B, Martin's choice; built in
     `microgateway/internal/plugins/analytics_pulse_backlog.go`). A pulse
     that is given up is no longer trimmed to `max_buffer_size`: it and
     everything buffered behind it go into a backlog of serialized pulse
     messages (`proto.Marshal`, each under its own sequence number, split
     until each is within the 8 MB pulse bound), bounded by the pulse
     file's `max_backlog_bytes` (default 256 MiB when unset; `0` turns it
     off and restores the trim). Over the bound the oldest batches go,
     counted as `reason="over_backlog"`; a batch whose newest record is
     older than `edge_retention_hours` expires whole. The sender sends the
     backlog's head first, as it was serialized, and on success the next,
     then the buffers, so recovery drains in bounded pulses (at most
     `max_buffer_size` records and 8 MB each, under the hub's 16 MiB). A
     failed head stays at the head under its number, so a repeat after a
     lost acknowledgement is stored once (item 4). Records are taken
     under the buffer lock and serialized outside it. Gauges
     `microgateway_analytics_pulse_backlog_bytes` and `_batches`. In
     memory: lost if the edge restarts during the outage (option C, a
     disk-backed outbox, is the later item for that). Live Go-struct memory
     stays bounded by what arrives between two pulse attempts.
5. A single DNS name or global load balancer in front of both CPs keeps
   working and stays documented as the alternative.

Tests (`microgateway/tests/cluster`): CP A cut mid-push, the edge finishes
on B; a key created on A before the cut validates through B; a re-sent
pulse is stored once.

### Phase 3: start from last-known-good (R4)

Files: `microgateway/cmd/microgateway/main.go`,
`microgateway/internal/services/edge_sync_service.go`,
`microgateway/internal/database/models.go`, the microgateway Helm chart.

1. **Persist the applied snapshot.** After a snapshot is applied, store its
   protobuf in a new edge table `applied_snapshots` (one row: checksum,
   version, namespace, applied_at, blob), encrypted with the edge's
   `ENCRYPTION_KEY`, in the same transaction as the apply. Storing the
   snapshot itself, not relying on the tables it produced, is what makes
   the in-memory state (prompt templates, guardrail exceptions and
   capabilities, tracing keys) recoverable.
   **Built** (`microgateway/internal/services/applied_snapshot.go`): an
   edge always stores it, whether or not it will start from it, so the
   option of item 2 can be turned on later without a control plane. The
   snapshot is stored as received (before entries the edge cannot store
   are left out), with the checksum the edge reports in heartbeats, so
   applying it again leaves out the same entries and reports the same
   checksum. Encryption is AES-256-GCM with a random nonce under a key
   derived by HKDF from `ENCRYPTION_KEY` (label `applied-snapshot-v1`),
   and the row's other columns are authenticated with the snapshot: a row
   whose namespace or time was edited does not open. The column is a BLOB
   on SQLite and `bytea` on Postgres, tested at 16 MiB. An unchanged
   snapshot (apply skipped) does not rewrite the row.
2. **Boot path.** `edgeClient.Start()` is fatal today and runs before the
   services exist. Change it so that when
   `EDGE_START_FROM_LAST_KNOWN_GOOD=true` and no CP in the list answers
   within `EDGE_STARTUP_CONTROL_TIMEOUT` (default 30 s), the edge loads the
   stored snapshot through the normal apply path (`onConfigChange`), starts
   serving, and leaves the reconnect loop running. The first successful
   registration applies the live snapshot as any reconnect does.
   **Built** (`SimpleEdgeClient.StartWithLastKnownGood`,
   `microgateway/internal/grpc/last_known_good.go`): with the option off
   the start is `Start()` as before, one attempt. With it on, the edge
   repeats the start with the reconnect backoff until the timeout (the
   last attempt is made when it runs out; a control plane that accepts the
   connection and does not answer is bounded too), then loads the stored
   snapshot. Applying it goes through the same sync as a snapshot from a
   control plane; the stored row is not rewritten by that
   (`services.KeepStoredSnapshot`), so restoring does not make the
   snapshot look newly received. The reconnect loop starts once the edge
   can apply what a control plane sends.
3. **Refusals.** No stored snapshot, a snapshot for another namespace, one
   that fails to decrypt, or one older than `EDGE_LAST_KNOWN_GOOD_MAX_AGE`
   (default 72 h, `0` for no limit): exit as today, with the reason. Added
   while building: a snapshot whose `applied_at` is more than five minutes
   in the future by the edge's clock (the clock was set back) is refused
   too, since no age can be trusted; the remedy is to correct the clock or
   let the edge connect to a control plane once.
   **What "older" is measured from** (changed while building, see the
   epic's notes): the edge's last contact with a control plane, not the
   time the snapshot was applied. A snapshot is only re-applied, and its
   row rewritten, when the configuration changes, so by `applied_at` an
   edge whose configuration had been stable for more than three days
   would refuse to start although it was in sync a minute before the
   outage. What the limit is there to bound is how long nobody could
   change or revoke anything on the edge. So the edge records each
   contact (a registration accepted, a heartbeat acknowledged) in
   `sync_states` (topic `control.contact`, written at most once a minute;
   no schema change), and the age is `now - max(applied_at, last
   contact)`. `applied_at` stays in the row, authenticated, and in the
   warning the edge logs.
   The contact time is authenticated as `applied_at` is, because it
   decides whether a configuration that may have been revoked is served:
   the row's `sequence_number` holds 63 bits of
   `HMAC-SHA256(k, topic || 0x00 || unix seconds)` with
   `k = HKDF-SHA256(ENCRYPTION_KEY, "control-contact-v1")`. A row without a
   valid tag (edited in the database, or written under another key) counts
   as no contact, and the age falls back to `applied_at`. A contact more
   than five minutes ahead of the edge's clock (the clock was set back) is
   ignored the same way and logged; within that tolerance the age is zero,
   never negative; a snapshot whose `applied_at` is itself in the future is
   refused, since no age can be worked out.
4. **Default off.** Starting from stored configuration means a restarted
   edge can serve something an administrator has since removed until it
   reconnects. That is an operator's decision, as the stale grace is.
5. **Visibility.** Log line and `/ready` body field `config_source:
   control | last_known_good`, metrics `microgateway_config_source` and
   `microgateway_config_age_seconds`. On reconnect the heartbeat reports
   the checksum as usual, so the edge shows Pending or In Sync correctly.
   **Built:** `microgateway_config_source{source}` is 1 for the source in
   use and 0 for the other; `microgateway_config_age_seconds` is the time
   since the last contact with a control plane (a few seconds while
   connected, growing through an outage): the quantity the max age is
   compared with.
6. **Deployment.** The chart only persists the database when
   `kind: StatefulSet` and `persistence.enabled`; a Deployment's `emptyDir`
   survives a container restart but not a rescheduled pod. Document that
   R4 needs the StatefulSet form, and have the chart warn when the flag is
   set without it.
   As built (microgateway chart): `config.startFromLastKnownGood`,
   `startupControlTimeout` and `lastKnownGoodMaxAge`, with
   `config.controlEndpoints` and `controlFailbackInterval` for Phase 2; the
   warning is printed after install or upgrade (`templates/NOTES.txt`) and
   the chart still installs, since an `emptyDir` does cover a container
   restart. Checked on EKS: an edge pod deleted with no control plane
   reachable came back Ready from the snapshot on its volume claim. The
   credential sync values follow with Phase 4.
   The per-database state behind this (the snapshot sealer, the control
   contact's write time, the configuration the edge reports, a snapshot
   being restored) is held in registries keyed by the database's
   connection pool (`dbKey`), which every session and transaction of a
   handle shares; they were keyed by the handle's `*gorm.Config`, which
   gorm copies per session, so a lookup through a session-derived handle
   silently found nothing (bead .38; every caller passed the base handle,
   so nothing was lost). A database without a snapshot store is reported
   once, as an error.

Tests: restart with all CPs cut serves traffic, including a templated
prompt and a guardrail exception; each refusal case; reconnect replaces the
configuration; a snapshot applied partially (entries left out) restores to
the same reported checksum.

**An edge whose volume is full** (bead .35, found by the load test: the
chart's 2Gi filled in 30 to 45 minutes at 3,000 to 6,000 requests/s with
the edge's own analytics copy). Established on a filesystem that is really
full (`TestFullDisk_WhatAnEdgeCanStillStore`, run in a container with a
16 MB tmpfs; a SQLite page limit is not the same thing, since writes that
replace rows in place still succeed there):

- A configuration cannot be applied, not even one of the same size:
  SQLite cannot extend its write-ahead log and the commit fails. The
  configuration and its stored snapshot are one transaction, so the edge
  keeps the configuration it had, in its tables, in memory and as its
  stored snapshot, and its heartbeat reports that one. Last-known-good
  therefore never holds a configuration the edge does not run, and never
  silently an older one than it reports.
- The control contact, the budget blocks and the budget sync's position are
  not stored; blocks are still enforced from memory. A volume full for
  longer than `EDGE_LAST_KNOWN_GOOD_MAX_AGE` means a later start without a
  control plane is refused, by age.
- Made visible, not fixed (no size cap or pruning here):
  `database.NoteWrite` tracks writes refused for lack of room by kind; one
  error in the log when a kind begins to fail and one line when it is
  stored again; `/ready` stays 200 with `database_full`,
  `database_full_since` and `database_full_not_stored`; metrics
  `microgateway_database_full` and
  `microgateway_database_full_writes_total{write}`.
- Sizing: about 400 bytes per request times rate times
  `ANALYTICS_RETENTION_DAYS` (the chart sets 90; an edge without the
  setting keeps 7). Documented with the persistence notes of the chart.

### Phase 4: full credential sync, hashed (R5, R6, rest of R3)

Files: `proto/config_sync.proto`, `proto/common.proto`,
`grpc/control_server.go`, `microgateway/internal/grpc/simple_client.go`,
`microgateway/internal/services/hybrid_gateway_service.go`,
`microgateway/internal/database/models.go`.

1. **The hash.** `HMAC-SHA-256(key, token)` with a key derived by HKDF from
   `MICROGATEWAY_ENCRYPTION_KEY` and a fixed label, the construction
   `grpc/snapshot_secret_digests.go` already uses. Keys are 256-bit random
   values, so a fast keyed hash is sufficient, FIPS-approved, and allows an
   indexed lookup; a captured edge database yields nothing usable without
   the key, and cannot be reversed with it.
   **Built** as `pkg/credhash` (usable by the hub and by the microgateway
   module): `credhash.New(encryptionKey)` then `Hash(credential)`, which
   returns `"v1:"` and 64 hexadecimal characters (67 in all,
   `credhash.Len`). The HKDF label is `credential-hash-v1`; the prefix
   names the construction, so a later one can be told apart. The key must
   be at least 14 bytes (112 bits, the least FIPS mode accepts). Test
   vectors are fixed in `pkg/credhash/credhash_test.go` and asserted again
   from the microgateway module.
2. **New RPC `SyncCredentials`** (server stream, paged): for the edge's
   namespace, every App credential as `{credential_id, key_id, secret_hash,
   active, deleted, updated_at}` with its App (`AppConfig`, as
   `ValidateToken` already returns for pull-on-miss), ending with a
   watermark. A `since` watermark turns it into a delta, including
   deactivations and soft-deleted rows. The hub re-reads a short overlap
   window, as the event log does, so a row committed late is not missed;
   upserts on the edge are idempotent. The plaintext secret never leaves
   the hub. Unlicensed namespaces get nothing, as with `ValidateToken`.
   **Built** (hub side; midsommar-clean-4x5y.16). The proto, the
   registration hook, what a handler reads from the control server and the
   row reader are in core (`proto/config_sync.proto`, `pkg/credsync`,
   `grpc/credential_sync.go`); the handler is in the Enterprise module
   (`features/credential_sync`, which imports `pkg/credsync` and not the
   control server), so a Community hub answers `Unimplemented`. It runs behind the control server's stream
   authentication (the edge token) and reads only the database, so every
   replica serves it. As built, and where it differs from the sketch above:
   - **Pages** of 500 credentials (`PageSize`), each with the Apps its
     credentials belong to, read and sent one page at a time. The request
     names the hash construction the edge expects (`hash_version`, `v1`);
     another is refused with `FailedPrecondition`.
   - **Namespace**: the registered edge's, not the one the request names.
     A full stream carries every credential of an App that namespace may
     serve (global or its own), inactive credentials and inactive Apps
     included, and nothing else; an unlicensed namespace gets one empty
     full page, so the edge drops what it held. An edge that is not
     registered (no edge ID, or one with no registration) gets
     `FailedPrecondition` ("register first") and nothing else: it
     registers, then asks again.
   - **Watermark**: `(unix seconds << 28) | tag`, opaque to the edge. The
     seconds are the earlier of the database's clock and the replica's
     when the stream started; the tag is 28 bits of a hash of the
     replica's node ID and the number of database failovers it has seen.
     A delta reads what changed (the credential's or its App's
     `updated_at` or `deleted_at`) since the watermark's time minus an
     overlap of 60 s, which assumes replica and database clocks within
     30 s of each other and a transaction that commits within 30 s of
     stamping its rows.
   - **A delta is only continued by the replica that issued the
     watermark, on the database it issued it for.** A watermark with
     another tag (another replica, a restart, a database failover since)
     gets a full stream. So an edge that fails over to the other control
     plane starts there with a full stream.
   - **Periodic full stream**: a chain of deltas becomes a full stream at
     least every 15 minutes (`FullInterval`, at a moment that differs per
     edge), which removes whatever a delta could not see (a change that
     moved no timestamp, a row removed outright, a clock or commit outside
     the allowance).
   - **Tombstones**: in a delta, a changed credential the edge must not
     hold (deleted, its App deleted or moved to another namespace) is sent
     as its id and `deleted`, with no key id, hash, App or time. Deltas are
     read without regard to namespace for that, so an edge also receives
     tombstones for credentials of other namespaces.
   - **How long a revoked key can still be accepted by an edge in `full`
     mode**: about a second while connected (the `credential.changed`
     event, item 4); otherwise until its next delta (one heartbeat tick);
     at most 15 minutes plus one delta interval if a delta missed the
     change; and up to `EDGE_CREDENTIAL_SYNC_MAX_AGE` (24 h by default) on
     an edge that reaches no control plane.
   - **Cost**, measured with 50,000 credentials, each the key of its own
     App with one LLM (development machine, Postgres 16): a full stream is
     101 pages and 8 MB on the wire and takes about 4 s (3 to 4 s on
     SQLite), holding under 10 MB of heap whatever the number of
     credentials; an unchanged delta is one 16-byte page in 20 to 50 ms. As a steady cost
     the periodic full stream is about 5 s of one hub core and 8 MB per
     edge every 15 minutes: for 100 edges, about half a core and 1 MB/s on
     average, spread over the interval. Ten edges taking theirs at the
     same moment took 12 s.
   - **Index**: each page's Apps are read by credential, so
     `apps.credential_id` is indexed (`idx_apps_credential_id`, schema
     43, additive; `MinReaderSchemaVersion` stays 42).
   - Every path that changes `credentials.active` or `apps.is_active`
     moves `updated_at` (a whole-struct save, or gorm `Update`); none uses
     `UpdateColumn`.
3. **Edge store.** New table `synced_credentials` (hash unique-indexed, App
   ID, active, updated_at) and the watermark in `sync_states`. No plaintext
   column. **Built** (bead .17). Each page is written in one
   transaction: its Apps (batched upserts, `is_active` written explicitly,
   false included), their LLM grants, and its credentials. An App is
   skipped when it has not changed since the sync last stored it: the
   table `synced_app_fingerprints` holds, per App, a SHA-256 of the
   `AppConfig` as the hub sent it (its deterministic protobuf encoding, so
   a field added later counts) and the `updated_at` the sync wrote on the
   App's row; any other write of the row changes that, and the App is
   stored again. A full sync then removes the fingerprints of Apps it did
   not bring and, on PostgreSQL only and only after a full sync, runs
   `ANALYZE` on `synced_credentials`, `synced_app_fingerprints`, `apps` and
   `app_llms`: a full sync can rewrite every row, and with statistics from
   before it the next sync of 50,000 credentials took 10 s instead of
   3.4 s. ANALYZE samples a bounded number of rows (about 0.3 s measured at
   50,000). Measured with 50,000 credentials, each the key of its own App:
   first full sync 2.8 s SQLite / 5.0 s Postgres, a repeat while
   validating 2.2 s / 3.4 s, a repeat with 1% of the Apps changed
   1.2 s / 2.7 s.
4. **Keeping it current.** Full sync at start and after every reconnect
   (delta when a watermark exists); a delta on each heartbeat tick; and a
   `DirDown` event `credential.changed` on create, activate, deactivate and
   delete, which the cluster relay already carries to every replica's
   edges, so a revocation lands in about a second while connected.
   The event's name and payload are fixed in `pkg/eventbridge`
   (`CredentialChangedTopic`, `CredentialChanged`: credential_id, hash,
   app_id, namespace, active, deleted; no key). The hash is the
   edge-facing one (`pkg/credhash` under `MICROGATEWAY_ENCRYPTION_KEY`, as
   `SyncCredentials` sends it), computed by the hub from the decrypted
   secret before a deactivation or delete takes effect; nothing is sent
   when the secret cannot be read. **Edge side built** (bead .18): on the
   event an edge drops what it cached for that hash, fresh or within the
   stale grace, forgets a validation in flight for it, and does not cache
   the result of a validation that started before the event. It only ever
   drops, whatever App or namespace the event names. An edge from before
   the event takes it without error (nothing subscribes to the topic).
   **Hub side built** (bead .18, `services/credential_changed.go`): the
   credential service raises the event on create, update (a regenerated
   secret raises the old hash, then the new one), delete (the hash is taken
   in the deleting transaction), activate and deactivate, and on an
   App's activation or deactivation and its key's activation through the
   App. The hash is made with `MICROGATEWAY_ENCRYPTION_KEY`
   (`services.SetEdgeCredentialHasher`, set by `pkg/studio`); without a
   usable key, or for a secret the replica cannot read, nothing is raised.
   The hub sends the hash only: the event reaches every edge of every
   namespace, and an edge drops by hash, so the credential ID, App ID and
   namespace of the payload type stay zero and empty.
   The topic is reserved (`eventbridge.IsCredentialTopic`): plugins cannot
   publish it, on Studio or on an edge, and a control node drops one an
   edge sends. Measured on the cluster rig: a key deactivated through
   another replica was refused by an on-demand edge 46 ms later (Postgres,
   through the relay). **Full mode built** (beads .18, .45): on the event
   an edge with full credential sync also asks for a sync, but only when
   its synced list holds the event's hash (one indexed read). The event
   carries no namespace and reaches every edge: a hash the list does not
   hold is another namespace's key, or one created since the last sync,
   and needs no sync (a new key's first use asks the control plane, and
   the heartbeat's sync brings it). So a key change makes only the edges
   that hold the key sync, not the fleet. Event-driven syncs are spaced:
   a random delay of up to 0.5 s before each, so the edges of one
   namespace do not call at the same instant, and at least 1 s between two
   on one edge (a stream of 100 events/s gave 11 syncs in 10 s); heartbeat
   syncs are not. The event is a hint only: nothing is added to or changed
   on the list from the payload; the delta brings the change with the
   credential's ID. On the rig a key deactivated through another replica
   was refused from the list within 2 s, and with no event raised the
   heartbeat's sync repaired it in 0.25 s (300 ms heartbeat).
5. **Validation order with `EDGE_CREDENTIAL_SYNC=full`.**
   - Local hit, active: accept, no CP call.
   - Local hit, inactive or deleted: reject.
   - Miss, CP reachable: ask the CP (`ValidateToken`, as today), which
     covers a key created in the last second.
   - Miss, no CP: 503 `credential_check_unavailable`, as today. The edge
     cannot know.
   - `EDGE_CREDENTIAL_SYNC_MAX_AGE` (default 24 h, `0` for no limit): once
     the last successful delta is older than this, the edge stops trusting
     the local list and behaves as `on_demand`.
   **Built** (bead .17, `microgateway/internal/services/credential_sync.go`,
   the RPC client in `microgateway/internal/grpc/credential_sync.go`): an
   explicit refusal from the hub always wins and updates the local row; a
   "valid" answer for an App the edge's namespace may not serve is refused
   on the synced path and nothing is added from it. A key recorded from a
   validation has no credential ID, so a delta cannot remove or replace it:
   every completed sync drops such rows recorded before it started (a key
   that still exists came with its ID; otherwise the next request asks
   again). The sync runs on every
   accepted registration (which also forgets an earlier `Unimplemented`)
   and every acknowledged heartbeat, one at a time, with a 5-minute bound.
   A `FailedPrecondition` "register first" answer makes the edge register
   again and ask again, at most once a minute; it is not the on-demand
   fallback. Metrics `microgateway_credential_sync_last_success_timestamp`,
   `microgateway_credentials_synced`,
   `microgateway_credential_sync_calls_total{trigger}` (registration,
   heartbeat, event; bead .50) and
   `microgateway_credential_validations_total{source}` (local, control,
   stale; a list that never synced counts as stale).
6. **Default stays `on_demand`.** In that mode nothing changes, except that
   the in-memory cache is keyed by the hash rather than the raw token.
   `full` is an **Enterprise** option: a Community edge given
   `EDGE_CREDENTIAL_SYNC=full` logs that and stays on demand, and a
   Community hub answers the RPC as unavailable. The hashing itself
   (items 1, 6 and 8) is in both editions.
7. **Compatibility.** An old hub answers `Unimplemented`: the edge logs it
   and stays on demand. An old edge never calls the RPC. Combined with
   Phase 3, the synced list is what a restarted edge validates against.
8. **Standalone microgateway.** `api_tokens.token` is plaintext there. Add
   a hash column, look up by hash, and stop storing the token (migrating
   existing rows on start). Small, and it closes the finding for every
   mode.
   **Built** (`microgateway/internal/database/api_token_hash.go`): new
   column `api_tokens.token_hash` (unique index), the credential hash made
   with the microgateway's `ENCRYPTION_KEY`. Every lookup (the gateway
   service, the auth provider, the database provider, the repository and
   the deprecated control mode's `ValidateToken`) goes by the hash; a
   gateway that cannot hash a token checks none (no fall back to comparing
   what is stored). The `token` column keeps its name, `NOT NULL` and
   unique index, and holds a redacted reference (the start of the token
   and part of its hash). A `BeforeCreate` hook hashes the token of any row
   built with the token itself, so no path stores one. At start,
   `HashStoredAPITokens` converts the rows an earlier version wrote; it is
   idempotent and safe for two gateways on one database. The conversion is
   one-way. Token listings (`mgw token list`, the management API) never
   returned token values, so nothing changes there. The `token_cache`
   table (never in the migrated schema, created only by early versions) is
   dropped where present and its model removed. `Credential.SecretHash`
   (bcrypt or PBKDF2, `HashSecret`) is a different mechanism and is left as
   it is.
   Because the conversion is one-way, the microgateway validates its
   license before it opens its database (`licenseThenDatabase` in
   `microgateway/cmd/microgateway/main.go`; it used to migrate first): an
   Enterprise microgateway that will not run has not migrated the database
   or converted the tokens, and `-migrate` alone needs the license too.

Tests: unknown-to-cache key accepted with every CP cut; revocation reaches
an edge on another replica; a key created while the edge was offline
arrives by delta on reconnect; max-age fallback; nothing reversible in the
edge database (assert no column holds the token); old hub and old edge
pairings.

### Phase 5: dropped

Read-only operation from the hot standby and a durable key-creation journal
were planned for outages longer than the retry window (manual promotion).
Dropped on 2026-11-10: with automatic failover the window is seconds and
Phase 1 covers it. Not built: a read pool on the standby, 202 responses for
key creation, an encrypted append-only journal file per replica under
`data/`, and replay that is idempotent on the key ID. Its known limits were
that the journal is lost with its replica, and that with no database node
reachable a control plane cannot authenticate a caller at all.

### Phase 6: hub hardening and non-database state

1. **`credentials.secret` on the hub** (built). The hub must keep the
   secret recoverable: the UI shows it again and Studio itself uses it as a
   bearer token (`services/grpc/decision_call.go`). So the secret is
   encrypted and found by `secret_hash` (unique; `ValidateToken` and
   `Credential.GetBySecret` look up by it, without reading the secret).
   Deviation from the first plan, which was to encrypt it "as LLM
   credentials are" (`secrets.EncryptValue`): that derives a key per
   value, about 100 ms each, and does not scale to App keys (80 minutes to
   migrate 50,000; 0.1 s of CPU per uncached key a hub validates). Instead
   the secret is sealed under ONE data key per process, derived from
   `TYK_AI_SECRET_KEY` and a random per-database salt (new table
   `data_key_salts`, one row, not secret, created once under the migration
   lock, never regenerated: Studio refuses to start if it is missing while
   sealed credentials exist), with AES-256-GCM, a random nonce and the
   credential's `key_id` as additional data; stored as `$DK1/...`
   (`secrets/datakey.go`). The hash is the hub's own, keyed with
   `TYK_AI_SECRET_KEY` (`secrets.LookupHash`: `pkg/credhash`'s
   construction under that key, or a plain SHA-256 when no key of 14 bytes
   is set), not the edge-facing one: that is computed from the decrypted
   secret when credentials are sent (Phase 4), so rotating
   `MICROGATEWAY_ENCRYPTION_KEY` needs no hub migration. Existing rows are
   migrated at start under the migration lock, in batches, idempotently
   (`models.MigrateCredentialsAtRest`; 50,000 rows in 7 s on SQLite and
   34 s on Postgres). Schema 42; `MinReaderSchemaVersion` rises to 42,
   because older code looks keys up by the plaintext secret (every App key
   would be refused) and would show and send the ciphertext. Changing
   `TYK_AI_SECRET_KEY` afterwards, or losing the salt, stops every App key
   validating until it is back. Details in `features/Secrets.md` ("App
   credentials").
2. **State that is per replica and cannot be a shared volume across
   sites.** Move branding assets into Postgres (two small files); keep log
   exports per replica and say so. Require `CSRF_KEY` when more than one
   replica is live (a random per-process key breaks forms across
   replicas), and warn in cluster status when replicas disagree on it or
   on the hash of `MICROGATEWAY_ENCRYPTION_KEY`.

   Shared secrets (built): each replica records in its `cluster_nodes` row
   a fingerprint of `CSRF_KEY`, `TYK_AI_SECRET_KEY`,
   `MICROGATEWAY_ENCRYPTION_KEY` and `GRPC_AUTH_TOKEN` (HMAC-SHA-256 with a
   fixed label, truncated to 32 bits; never the value, never logged or
   returned) and whether `CSRF_KEY` is set. The cluster status, and a WARN
   on the replica that notices at start, name the setting and the replicas
   when live replicas disagree, and when more than one replica runs without
   `CSRF_KEY`. `CSRF_KEY` is required in that sense only: nothing refuses
   to start, because a rolling rotation has replicas disagreeing for a
   while. Details in `features/ClusterControlPlane.md` ("Shared secrets").

   Branding assets (built): the logo and favicon are rows in the new table
   `branding_assets`, on SQLite as on Postgres, served by every replica with
   an ETag from a copy in memory that the replica signal `branding` (and a
   30 s hash check) keeps current. Files from earlier versions under
   `BRANDING_STORAGE_PATH` are imported once at start and left in place.
   Log exports stay files on the replica that made them. Details in
   `features/Branding.md` ("Asset Storage").
3. **Studio Helm chart** (built). The Deployment honours `replicaCount`.
   Above 1: a rolling update (maxUnavailable 0) unless `midsommar.strategy`
   says otherwise (Recreate stays the default for one replica), a
   PodDisruptionBudget, spread over nodes and zones (ScheduleAnyway; a
   `topologySpreadConstraints` list replaces it), cookie affinity on the
   nginx Ingress, and the render fails with the reason on the bundled
   single-pod PostgreSQL, on SQLite, or without `config.csrfKey`.
   `database.existingSecret` gives the whole (multi-host) `DATABASE_URL`
   from a Secret; `config.clusterNodeLabel`, `config.csrfKey` and the
   `DATABASE_*` pool, readiness and start-up settings are values. A startup
   probe on `/health` (180 s by default) covers the start-up wait for the
   database (`DATABASE_STARTUP_TIMEOUT`) and the migrations; readiness stays
   on `/ready`, `probes.readiness.requireWrite` switches it to
   `/ready?require=write`. The unused `autoscaling` values are removed:
   Studio is scaled by `replicaCount`.
   Microgateway chart: `controlEndpoints`, the new edge settings, and the
   persistence warning from Phase 3.

## New configuration

| Variable | Where | Default | Purpose |
|---|---|---|---|
| `CLUSTER_NODE_LABEL` | Studio | `studio` | Site label on cluster status and the Edge Gateways page |
| `DATABASE_MAX_OPEN_CONNS`, `DATABASE_MAX_IDLE_CONNS`, `DATABASE_CONN_MAX_LIFETIME` | Studio | unset, unset, `30m` | Pool limits |
| `DATABASE_CONNECT_TIMEOUT` | Studio | `10s` | One connection attempt, unless the DSN sets `connect_timeout` |
| `DATABASE_STARTUP_TIMEOUT` | Studio | `60s` | How long start-up waits for a database |
| `DATABASE_WRITE_RETRY_WINDOW` | Studio | `15s` | How long a write is held for a writable primary |
| `CONTROL_ENDPOINTS` | edge | unset | Ordered list of control planes (`CONTROL_ENDPOINT` is a list of one; not both) |
| `EDGE_CONTROL_FAILBACK_INTERVAL` | edge | off | How often an edge not on its first endpoint tries to return to it |
| `EDGE_START_FROM_LAST_KNOWN_GOOD` | edge | `false` | Start without a control plane |
| `EDGE_STARTUP_CONTROL_TIMEOUT` | edge | `30s` | Wait before using last-known-good |
| `EDGE_LAST_KNOWN_GOOD_MAX_AGE` | edge | `72h` | Longest an edge may have been without a control plane and still start from its stored snapshot |
| `EDGE_CREDENTIAL_SYNC` | edge | `on_demand` | `full` syncs hashed credentials (Enterprise) |
| `EDGE_CREDENTIAL_SYNC_MAX_AGE` | edge | `24h` | How long a synced list is trusted without a delta |

Studio settings go in `config/config.go` (`loadFrom`), `.env.example` and
`docs/site/docs/configuration.md`; edge settings in
`microgateway/internal/config/config.go` (struct tags),
`microgateway/docs/config/environment-variables.md` and
`microgateway/docs/configuration-reference.md`.

## Schema and protocol changes

- **Studio:** `credentials.secret_hash`, encrypted `credentials.secret`,
  applied analytics batch IDs, branding assets table. `SchemaVersion` bump
  and `make schema-golden` for each.
- **Edge:** `applied_snapshots`, `synced_credentials`, `api_tokens` hash
  column. Regenerate the microgateway goldens.
- **Proto (additive only):** `SyncCredentials` RPC and messages; a batch ID
  on the analytics pulse. `protoc` 25.3 is installed locally.
- **Upgrade order:** hub first, then edges, as for 2.3. Every edge feature
  is off by default and degrades to today's behaviour against an older hub.

## Documentation

| File | Change |
|---|---|
| `docs/site/docs/deployment-active-active.md` (new; register in `docs/site/.vitepress/config.mjs`; written, bead .26, with marked gaps for .17, .23 and the cluster pass's numbers) | The topology, database requirements (synchronous replication, the witness), DSN examples, what must match on both sites, load balancer and DNS options, failover and failback runbooks, what is and is not guaranteed, monitoring |
| `docs/site/docs/reference-architecture.md` | Failure table: the "edge starts while Studio is down" and "unknown token" rows gain the new options; the "PostgreSQL fails over" row describes Phase 1; add the topology under Topologies; checklist items. Fix the reconnect backoff, stated as 5 minutes at line 123 (the code caps at 30 s) |
| `docs/site/docs/edge-gateways.md` | "Apps and Credentials": the `full` sync mode and its revocation trade-off; Connection Settings: `CONTROL_ENDPOINTS`; the compromise section: what an edge database now holds (hashes only) |
| `docs/site/docs/configuration.md`, `microgateway/docs/config/environment-variables.md`, `microgateway/docs/configuration-reference.md`, `.env.example` | The variables above |
| `docs/site/docs/deployment-helm-k8s.md`, `helm/README.md` | Studio replicas, StatefulSet edges for last-known-good |
| `docs/site/docs/architecture.md` (line 94) | Says edges continue from their last-known snapshot; true only while running. Correct it and point at the new option |
| `microgateway/docs/scaling/controller-edge.md` (lines 575-588) | Describes `tokens.json`/`apps.json` caches that do not exist; replace with the real mechanism |
| `docs/site/docs/secrets.md`, `features/Secrets.md` | Credential hashing and hub encryption |
| `features/ClusterControlPlane.md`, `features/Proxy.md` (line 190), `features/README.md` (index) | Cross-references; this spec added to the index |
| `docs/site/docs/observability.md` | New metrics |

## Changelog

Entries go in `CHANGELOG-v2.3.0.md` (still marked Unreleased). Each PR adds
its own entries, and the feature gets a line in "At a glance" and, for the
credential migration, in "Before you upgrade":

- **What's new:** "Active-active control planes", linking to the new page
  (both editions, with full credential sync marked Enterprise): database failover without a restart;
  edges with several control endpoints; start from last-known-good; full
  credential sync.
- **Security:** credentials reach edges as keyed hashes only; App
  credentials encrypted at rest on the hub; standalone API tokens hashed.
- **Fixes → Edges and control plane:** duplicate analytics after a lost
  acknowledgement; budget sync ignored after a leader change with clock
  skew; a replica left on a demoted primary; start-up exiting when the
  database is briefly away.
- **Upgrade notes:** Database (the credential migration is one-way: take a
  backup; older Studio versions cannot read the migrated column. An older
  headless control plane refuses the database with the schema-version
  message. An older full Studio, 2.2.x or a 2.3 release candidate, does
  not check: it starts, refuses every App key and shows the ciphertext as
  the key, so every older replica is stopped first. Rolling back means
  restoring the backup taken before the upgrade. From this release on a
  full Studio makes the same check as a headless one before its first
  write, `models.CheckSchemaReadable`, and refuses a database whose
  minimum reader is above its own schema version, so later rollbacks fail
  safely; and an Enterprise Studio validates its license before that first
  write too, after the reader check, so a refused license leaves the
  database untouched); Edges
  (upgrade the hub first; new options are off by default); API (a write
  that cannot reach a writable database answers 503 `database_unavailable`
  with `Retry-After`, where it used to answer 500).
- **New configuration:** the table above.
- **Known issues:** revocation while an edge is cut off; no write queue
  beyond the retry window.

## Acceptance scenarios

Run on the Phase 0 rig, automated where marked (A) and in the dev stack
otherwise.

1. (A) Stop the primary Postgres under API load: writes resume within the
   retry window, no 500s, no replica restarted.
2. (A) Stop site A entirely (Studio and Postgres): edges move to B, a push
   issued before the loss completes, the leader lease moves.
3. (A) Create a key on A, cut A at once: the key validates on an edge
   through B.
4. (A) Cut every CP, restart an edge: it serves known keys from
   last-known-good; with `full` sync, a key it had never served.
5. (A) Revoke a key on B: an edge held by A rejects it within seconds.
6. (A) Create a key while the primary is being replaced: the request waits,
   answers 201 once, the key validates on an edge, and the audit trail
   records it once. With no promotion inside the window: 503
   `database_unavailable`, nothing written.
7. Inspect an edge database after scenario 4: no token recoverable.
8. Bring site A back as standby, then switch back: no duplicate analytics,
   budgets consistent.

## Risks

- **Split brain is a deployment risk Studio cannot remove.** Mitigated by
  documentation and by Studio refusing to treat a read-only node as a
  primary.
- **Landing in a release-candidate phase.** Every edge option is off by
  default and Phase 1 changes behaviour only when the database fails, but
  the credential migration (Phase 6.1) is one-way and changes the 2.3
  upgrade notes. It ships in its own PR and needs a fresh upgrade test from
  2.2.2 and from the current RC.
- **Longer offline life for revoked keys** with Phases 3 and 4. Both are
  opt-in and bounded by maximum ages.
- **What `SyncCredentials` gives the holder of the edge token.** The edge
  token is already a hub-wide secret (it can claim any namespace's
  configuration). With credential sync it also yields, for a licensed
  namespace, the hash of every App key (`pkg/credhash` under
  `MICROGATEWAY_ENCRYPTION_KEY`: not a key, and not reversible) with its
  key id and App. A tombstone carries nothing but a credential's numeric
  id, so the credentials of other namespaces are visible only as ids that
  changed. Only a registered edge's namespace is streamed: an unregistered
  edge ID is refused rather than served the namespace its request names.

## Decisions (Martin, 2026-11-10)

1. **A held or queued key does not need to work on edges before the
   database is back.** No ID reservation, no distribution of uncommitted
   Apps.
2. **Phase 5 is dropped** (read-only operation and the journal).
3. **Edition.** Resilience (Phases 1 to 3) and credential hashing are in
   both editions. Full credential sync (`EDGE_CREDENTIAL_SYNC=full`) is
   Enterprise.
4. **Licensing.** One licence covers both control planes of an
   active-active deployment; licences meter the data plane (namespaces),
   not topology. No licence change. Both sites run the same
   `TYK_AI_LICENSE`.
5. **Release.** 2.3.0.
6. **The embedded gateway is out of scope**, and no new data store is
   added.

## Not in this plan

- **Analytics during a long outage.** Edges buffer 10,000 records in
  memory. Shipping from the edge's own analytics table with a cursor would
  make this durable with no new store.
- **Per-edge credentials** (bd `midsommar-clean-jyx.86`): the edge token
  and `MICROGATEWAY_ENCRYPTION_KEY` remain hub-wide secrets, and the
  credential hash key is derived from the latter.
