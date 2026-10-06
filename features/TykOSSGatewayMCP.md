# Tyk OSS Gateway as an MCP backend (draft plan)

Status: **built 2026-10-06** (decisions in §9, as built in §11, verification
in §12). Enterprise only. User docs: `docs/site/docs/tyk-oss-gateway-mcp.md`. Builds on
`features/TykMCPIntegration.md` (the Tyk Dashboard integration, merged as core
#581 and enterprise #27).

## 1. Goal and scope

Let AI Studio use a **Tyk OSS Gateway** (no Dashboard, no MDCB) as the data
plane for MCP servers, the same way it uses a Tyk Dashboard today:

- register MCP proxies on the Gateway,
- show them in the AI Portal (tool catalogues),
- mint and broker per-App Tyk keys for them.

**This covers MCP only.** Studio never creates, edits, lists for management, or
deletes REST/GraphQL APIs on an OSS Gateway, and never mints a key that grants
anything other than Studio-known MCP proxies. Section 7 lists the guardrails
that enforce this.

Out of scope for v1:
- REST-to-MCP proxies (Tyk Enterprise Edition gateway feature).
- Gateway analytics (OSS writes analytics to Redis for Tyk Pump; there is no
  Dashboard to read them from).
- Upstream OAuth and token exchange (Gateway EE features).

## 2. What the OSS Gateway offers (verified in source)

Source read: `TykTechnologies/tyk` at `875d987d` (2026-10-05).

**MCP support is in the OSS build.** Docs: "Proxies fronting a remote MCP
server are available on all Tyk Gateway licenses", Gateway ≥ 5.13.

- None of the MCP middleware files carry an `ee` build tag:
  - `gateway/mcp_*.go`
  - `mw_jsonrpc*.go`
  - `mw_mcp_access_control.go`
  - `res_handler_mcp_list_filter.go`
- The `ee`-tagged pieces are:
  - upstream OAuth (`mw_oauth2_auth_ee.go`)
  - OAuth2 token exchange
  - upstream basic auth (`mw_upstream_basic_auth_ee.go`)
  - streaming
  - Vault/Consul KV (`kv_ce.go` returns no factories)

**Gateway API** (`gateway/server.go:921-980`). It is authenticated by
`X-Tyk-Authorization: <gateway secret>`. In RPC (MDCB) mode most of it is
switched off.

| Purpose | Endpoint | Notes |
|---|---|---|
| MCP proxies | `GET/POST /tyk/mcps`, `GET/PUT/DELETE /tyk/mcps/{id}` | Tyk OAS. `?dryRun=true` **is honoured**: it returns the document without writing, unlike Dashboard 5.14. `?expand=true` exists. |
| Policies | `GET/POST/PUT/DELETE /tyk/policies[/{id}]` | Only with `policies.policy_path` set. Refused when `policy_source=service`. |
| Keys | `/tyk/keys[/{key}]`, `?hashed=true` | Stored in **Redis**. |
| Reload | `GET /tyk/reload[?block=true]`, `GET /tyk/reload/group` | Group reload is a Redis pub/sub signal. |
| Health | `GET /hello` | `HealthCheckResponse.version` |

**The core constraint: definitions are per node.**
- `handleAddMCP` (`gateway/mcp_api.go:283`) writes the definition to the
  **local** `app_path` of the node that answered (`writeOASAndAPIDefToFile`)
  and does not reload.
- Policies are written to the local `policy_path` in the same way.
- `/tyk/reload/group` tells every node to reload, but each node reloads **from
  its own disk**.

Behind a load balancer, a POST to `/tyk/mcps` lands on one random pod. The
other replicas never see it.

**Keys are cluster-wide.**
- Sessions live in the shared Redis, so one key write through any node is
  visible to every node at once.
- `user.AccessDefinition` (`user/session.go:145-148`) carries the MCP fields
  per API: `json_rpc_methods`, `json_rpc_methods_access_rights`,
  `mcp_primitives`, `mcp_access_rights`.
- A key can therefore hold tool-level ACLs and per-tool limits **inline,
  without any policy**.

**There is no fleet API.** The OSS Gateway has no node registry and no
command-and-control API. The only "fleet" mechanisms are:
- Redis pub/sub (reload signals; DRL notifications when DRL is on),
- the Dashboard node protocol (`/register/node`, `/register/ping`,
  `/system/apis`, `/system/policies`), which is only used when
  `use_db_app_configs=true`.

## 3. The two hard problems

1. **Replication.** An MCP proxy definition must exist on every gateway
   replica. A policy must too, if we use policies.
2. **Cluster tracking.** Studio has to know which replicas exist, which of
   them hold the current Studio-owned definitions, and when a new replica
   appears.

Keys need neither, because Redis is shared.

## 4. Options for config distribution and fleet tracking

### A. Studio pushes and reconciles per node (recommended for v1)

Studio resolves the connection to a **set of node control URLs** and
reconciles each node to the desired set of Studio-owned MCP proxies: list,
diff, create/update/delete, then `/tyk/reload?block=true` on that node.

Node discovery modes, per connection:
- `single`: one URL. Docker, dev, and single-node installs.
- `static`: an admin-entered list of node URLs.
- `dns`: a hostname (a Kubernetes headless Service, or a Compose service name)
  resolved to **all** A/AAAA records on every reconcile tick, plus port and
  scheme. This tracks autoscaling without any agent.
- `shared_storage`: the replicas share `app_path` (an RWX volume). Studio writes
  through any one node and then calls `/tyk/reload/group`. The node list is
  still resolved for status reporting.

Fleet state goes in a new `tyk_gateway_nodes` table:
- connection id, address, discovery source
- first/last seen, reachable flag, gateway version
- checksum of the Studio-owned definitions on the node, last reconcile time,
  last error

A node that drops out of DNS is marked gone after a grace period. This is the
OSS equivalent of the edge "In sync / Pending / Stale" view.

**Pros**
- No new deployable.
- Only public, documented Gateway API endpoints.
- Coexists with the operator's own file-based APIs, because Studio touches only
  the definitions it owns.
- Reuses the existing sync scheduler and its lease (one Studio replica
  reconciles a given connection at a time).

**Cons**
- Studio needs network reach to every replica's control port.
- **Scale-up gap:** a new pod serves 404 on Studio's listen paths until the next
  reconcile tick (default 10 s, while DNS is re-resolved each tick). The same
  happens when a pod restarts with an `emptyDir` `app_path`.
  - For autoscaled fleets the docs recommend `shared_storage` (new pods load
    from disk at boot) or Option C.

### B. Studio impersonates the Dashboard node protocol (rejected)

Gateways run with `use_db_app_configs=true` pointing at Studio. Studio serves
`/register/node`, `/register/ping`, `/system/apis` and `/system/policies`, and
gets registration and heartbeats for free.

Rejected for four reasons:
1. **Exclusive config source.** In that mode `syncAPISpecs`
   (`gateway/server.go:724`) loads only from the "Dashboard", so every non-MCP
   API the operator has disappears. Studio would either break those APIs or
   have to serve them, which makes it a Dashboard.
2. **Unversioned internal protocol.** It is stateful and has no published
   contract: the nonce is chained across every call
   (`dashboard_register.go`).
3. **Shared nonce state.** Multi-replica Studio would need nonce state shared
   between replicas.
4. **Scope.** It amounts to shipping an open replacement for the Dashboard's
   gateway-management role. That is a product and commercial decision well
   beyond "MCP only".

### C. Pull-based sidecar agent (possible later addition, not planned)

A small `tyk-mcp-agent` runs as a sidecar or init container next to each
Gateway:
1. It enrols with a Studio-issued token.
2. It long-polls a Studio desired-state endpoint (ETag).
3. It writes through the **localhost** Gateway API and reloads locally.
4. It heartbeats with its checksum.

The init container can block pod readiness until the first sync, which
removes the scale-up gap. It also works when Studio cannot reach the
gateways (edge DCs, locked-down control ports).

It mirrors the microgateway hub-spoke model. It should use a small dedicated
HTTP protocol rather than `config_sync.proto`, whose snapshot carries
LLM/App/namespace data.

**Design v1 so that C is additive.** The reconciler's input (a "desired
Studio-owned MCP set for connection X", with a checksum) is the same document
the agent would pull. C then becomes a fifth discovery mode, `agent`, that
writes the same node table.

## 5. Entitlements: inline key access rights rather than policies (recommended)

The Dashboard integration brokers keys from **policy bundles pinned per MCP
asset**. On OSS, policies are per-node files, so a key referencing a policy
that a replica has not loaded yet fails on that replica. This is the
Dashboard's 403-lag trap, made permanent.

Recommendation for `gateway` connections: Studio does **not** write
`/tyk/policies`.
- An **access profile** (allowed tools/resources/prompts, JSON-RPC methods,
  rate, quota, expiry) is a Studio-side object pinned per MCP server. It is the
  OSS equivalent of a policy bundle and can live in `mcp_server_policy_pins`
  with a `source=studio` discriminator, or in a sibling table.
- `buildSession` compiles the App's bindings into `access_rights` with the MCP
  ACL fields inline (`mcp_access_rights`, `mcp_primitives`,
  `json_rpc_methods*`, per-API `limit`).
- Drift is applied by rewriting the affected keys. The broker already does a
  GET-and-PUT per key in `applyDesired`. The cost is O(keys per changed
  server), which is fine at Studio scale.
- **Benefit:** keys are consistent across the cluster the moment Redis has
  them. No policy files need replicating.

Decided (§9): inline only. Replicating policies was rejected because it brings
back the key-before-policy race on new or lagging nodes.

## 6. Where the code seam goes

Today the enterprise service (`enterprise/features/tykmcp/`) uses the concrete
Dashboard `*client.Client` directly, in about 12 signatures, and knows the
Dashboard's response shapes:
- `{mcps, pages}`, `{Data, Pages}`
- the create id in `ID`, `Meta` or `Message`
- `{key_id, key_hash}`

Plan:

1. **Backend interface** (`enterprise/features/tykmcp/backend.go`).
   - Normalised operations: `ListMCPs`, `GetMCP`, `CreateMCP` / `UpdateMCP` /
     `DeleteMCP`, `Probe`, `CreateKey` / `GetKey` / `UpdateKey` / `DeleteKey`
     (with a `KeyRef`), optional `Policies` (Dashboard only), optional `Templates`, optional
     `SourceAPIs`.
   - Two implementations:
     - `dashboardBackend`: wraps today's client with no behaviour change.
     - `gatewayBackend`: the `/tyk/...` calls plus the node fan-out.
   - The existing `client.Do` transport is reused, since it already supports
     `X-Tyk-Authorization`.
   - `dashboardClient(conn)` (`service.go:169`) becomes `backendFor(conn)`.
2. **Reused unchanged:**
   - `parseMCPDefinition` and `buildDefinition`. The latter already injects the
     upstream token with `transformRequestHeaders`, which is OSS-safe.
   - Masking and splicing.
   - Grants, events, sync lease/scheduler, the credential ledger, and portal
     endpoints.
3. **Connection kind.**
   - New `tyk_connections.kind` column: `dashboard` (default) | `gateway`.
   - Gateway-only fields:
     - `discovery_mode` (`single|static|dns|shared_storage`)
     - `node_seeds` (JSON)
     - `control_scheme_port`
     - `reconcile_interval`
   - Reuse `DashboardURL` (as the seed URL) and `DashboardAccessToken` (as the
     encrypted gateway secret) under neutral JSON names (`url`, `secret`),
     keeping the old names as aliases.
   - This is a schema change: bump `models.SchemaVersion` and run
     `make schema-golden`.
4. **Probe and capabilities for `gateway`:**
   - Reachability and version come from `/hello`; mark the connection degraded
     below 5.13.
   - `mcp_read` from `GET /tyk/mcps`.
   - `mcp_write` from `POST /tyk/mcps?dryRun=true`, which is a real dry run
     here.
   - `keys_write` from `POST /tyk/keys/preview`.
   - New `cluster_shared_redis`: create a throwaway key on node 1, read it on
     node 2, delete it. This proves the nodes are one cluster.
   - New `nodes_consistent`: every node holds the same Studio-owned checksum.
   - `template_read`, `mdcb_read`, `rest_to_mcp_supported` and
     `policies_write` (inline mode) are reported as not applicable.
   - `effectiveMode` gets a per-kind rule table.
5. **Trust modes still apply, as Studio-side self-restraint.**
   - The gateway secret is all-powerful (OSS has no RBAC), so `catalogue` and
     `broker` are enforced only by Studio not calling write endpoints.
   - The connection form must say so, and recommend a dedicated
     `control_api_port` that is never exposed publicly.
6. **Ownership marker.**
   - Studio-created proxies get API ids prefixed `studio-<conn>-`, plus an
     `x-tyk-api-gateway.info` tag/label.
   - The reconciler only ever creates, updates or deletes ids carrying the
     marker. Proxies it did not create are imported read-only (origin
     `gateway`) and are never replicated or deleted by Studio.
   - Discovery takes the **union** across nodes and flags a proxy that is
     present on only some nodes as `partial`.
7. **Dashboard-only features hidden for `gateway`:**
   - API templates. A connection-level "definition defaults" JSON fragment
     could replace them, merged by the existing `template.go` merge.
   - MDCB data planes, gateway segmentation tags, and REST-to-MCP.
   - The Tools "Import OpenAPI → Tyk" wizard (`/tyk/apis` is REST, so out of
     scope).
   - org id inference. Studio uses a connection-level org id, defaulting to
     the one seen on existing MCP proxies, else `default`.
8. **UI.**
   - `tykShared.js` gains a `type` toggle with conditional field groups:
     - Dashboard: today's form.
     - Gateway: seed URL / hostname, discovery mode, secret, org id, public
       base URL.
   - The connection detail page gets a **Nodes** panel listing address,
     version, reachable, in sync / pending / unreachable, last reconcile, and
     error.
   - "Tyk Dashboard" strings become "Tyk" where they are shared.

## 7. "MCP only" guardrails (enforced in code and tested)

- `gatewayBackend` has a hard allow-list of paths:
  - `/hello`
  - `/tyk/mcps[/...]`
  - `/tyk/keys[/...]`, `/tyk/keys/preview`
  - `/tyk/reload`, `/tyk/reload/group`
  - Anything else is a programming error that fails closed. A unit test
    asserts the allow-list.
- Writes and deletes only touch Studio-owned ids (the ownership marker).
- Minted sessions contain `access_rights` only for Studio-known MCP proxy ids
  on that connection. `buildSession` validates this before every write.
- `/tyk/apis` is never called. That rules out REST-API discovery, the Tools
  import, and REST-to-MCP sources on OSS connections.

## 8. Milestones

| # | Milestone | Exit criteria |
|---|---|---|
| M0 | **Spike** against Gateway 5.13+ OSS image ×2 replicas + Redis (Compose) | Confirm: per-node file writes; reload needed; inline `mcp_access_rights` enforce tool ACL and list filtering on the OSS build; `?hashed=true` key ops; `/tyk/mcps?dryRun`; header-injected upstream token; behaviour when a key names an API a node lacks. |
| M1 | Backend seam refactor | `dashboardBackend` behind the interface; every existing tykmcp test and the live suite are green with no behaviour change. |
| M2 | `gateway` kind | Model column and schema bump; connection CRUD and form; gateway probe + capabilities; `fakeGateway` test server modelled on `fake_dashboard_test.go`, with per-node state and shared key state. |
| M3 | Fleet + reconcile | `tyk_gateway_nodes`; `single` / `static` / `dns` / `shared_storage` discovery; per-node reconcile of Studio-owned proxies with checksum; Nodes panel; tests for node add/remove, a partial failure, and a node that restarts empty. |
| M4 | Discovery import | Union import of existing MCP proxies (read-only, `partial` flag); catalogue publish, as for the Dashboard. |
| M5 | Broker (inline entitlements) | Access profiles; `buildSession` compiles inline ACL; drift by key rewrite; mint / rotate / revoke / suspend; App binding and portal key reveal work unchanged. |
| M6 | E2E + docs | `tests/tykmcp` live variant against the Compose OSS cluster (scale 2→3 mid-run); docs page; spec §18-style live-run record. |
| later (not planned) | `tyk-mcp-agent` | Pull agent (Option C) as discovery mode `agent`, only if push-and-sync proves insufficient. |

## 9. Decisions (Martin, 2026-10-06)

1. **Edition: Enterprise only.** The target is teams that want governed MCP
   without buying the full Tyk Dashboard APIM. All code stays in
   `enterprise/features/tykmcp`; CE shows the existing upsell.
2. **Distribution: push and sync (Option A).** Option C stays a possible later
   addition and is not planned.
3. **Entitlements: on the keys (inline access rights).** Studio never writes
   `/tyk/policies` on Gateway connections, and there is no `policies` mode.
   §6 and §7 already reflect this.
4. **Partial operator-created proxies: flag only.** Studio never replicates
   proxies it did not create.

## 10. Risks and things to verify in M0

- Definitions written by Studio contain upstream credentials **in plaintext on
  every node's disk** and are readable through `GET /tyk/mcps`. OSS has no
  Vault/Consul KV. Check whether `$secret_env.` / `$secret_conf.` references
  work in MCP definitions on OSS, and prefer them.
- The 404 window on new or restarted pods in non-shared-storage mode (Option A).
- Gateway secret blast radius: it is root on the Gateway and its Redis keys.
- Validation differences between nodes on different Gateway versions, which
  is possible during a rolling upgrade. The reconciler must tolerate one node
  rejecting a definition and report it per node.
- `hash_keys=false` clusters: the existing ledger path for plaintext key ids
  already covers this.

## 11. As built (2026-10-06, branch `feat/tyk-oss-gateway-mcp`, core + enterprise)

M0–M6 are done. Where the implementation differs from §4–§7:

**Seam.**
- `mcpBackend` (`enterprise/features/tykmcp/backend.go`) has the MCP and key
  methods. `*client.Client` implements it unchanged, so there is no wrapper
  type.
- `gatewayBackend` (`gateway_backend.go`) implements it over the nodes.
- `dashboardClient(conn)` refuses gateway connections (`errDashboardOnly`).
  `asDashboard` narrows a backend for policies and templates.
- `fullConnection` and the broker take the interface.

**Client.**
- `client.Gateway` (`client/gateway.go`) reuses the transport with
  `X-Tyk-Authorization`.
- Every call is checked against `gatewayAllowed` before it is sent; the
  allow-list is unit-tested.
- Errors are labelled "tyk gateway".

**Model.**
- `tyk_connections.kind` (default `dashboard`), `gateway_discovery`
  (`single|static|dns`), `gateway_node_urls`, `gateway_shared_storage`,
  `gateway_owner_id`.
- Shared storage is a flag next to discovery, not a fourth mode.
- New tables:
  - `tyk_gateway_nodes`: per-node state, plus an `applied` map of the
    sent/observed hash per proxy.
  - `tyk_gateway_definitions`: desired state, encrypted like connection
    tokens.
- `mcp_servers.key_access` holds the access profile as a column (no pin
  table). Also `gateway_coverage` and `gateway_partial`.
- `MCPOriginGateway` marks proxies found on the nodes that Studio did not
  create.
- Schema version 7.

**Ownership.**
- Studio's API ids are `studio-<owner>-<16 hex>`, where `<owner>` is a random
  10-hex id per connection. Two Studios, or two connections, sharing a cluster
  cannot touch each other's proxies.
- The reconciler, `UpdateMCP` and `DeleteMCP` act only on owned ids.
  `PushServer` and `DeleteServer` refuse `gateway`-origin servers.

**Reconcile** (`gateway_sync.go`):
1. Rewrite a node's copy when the desired hash changed since it was sent, or
   when the node's copy no longer hashes to what it returned after the write
   (an edit behind Studio's back).
2. Remove owned ids that are no longer desired.
3. Reload the node with `?block=true`, or with a group reload when storage is
   shared.
4. Re-list and record the observed hashes.

Writes go through at once (registration, push and delete); the sync retries
the nodes that failed. In catalogue and broker mode the sync records node
health only.

**Entitlements** (`gateway_access.go`):
- An entitlement id is the API id, plus `@<12 hex>` when the server's
  `MCPKeyAccess` restricts keys. The existing ledger, drift (widening means a
  new API id), external-change detection and `applyDesired` work unchanged.
- `keyEntitlements`, `ownedEntitlements` and `writeEntitlements` switch on the
  connection kind.
- Each key entry gets `mcp_access_rights.tools.allowed` and, when limits are
  set, `limit` plus `allowance_scope = api id`.
- The session gets `rate 0` and `quota_max -1` (0 would mean no requests).
- Editing key access rewrites live keys at once (`SetServerKeyAccess` →
  `evaluateDriftForServer`). It is never a widening, because it is the same
  proxy.

**Brokerable** on a gateway connection means: active, key-backed auth,
broker mode or higher, and not partial. No pin is needed.

**Registration on gateway connections:**
- `stripAuthorizationData: true` on access-key proxies. M0 showed the Gateway
  forwards the client's Tyk key to the upstream otherwise.
- Gateway tags are refused.
- The preview is validated by a node (`dashboard_validated: true`).
- REST-to-MCP, templates, MDCB, gateway tags, policies and the Tools import
  are refused or hidden.

**API.**
- `GET /tyk-connections/:id/nodes`: read on `tyk-connections`.
- `PUT /mcp-servers/:id/key-access`: write on `mcp-servers`.
- The Tools import list skips gateway connections.

**UI.**
- The connection form has a *Connect to* toggle, a *Nodes* section
  (discovery, node list, shared storage), a *Gateway nodes* table on edit, and
  hides the Dashboard-only sections.
- The list has *Type* and *Deployment* columns.
- Server detail has a *Key access* editor instead of *Access policies*, a
  partial chip and alert, and Gateway wording.

**Endpoint URLs (both kinds):**
- `proxyEndpointURLs` prefers an enabled `server.customDomain` that is a
  plain host. It uses the base URL's scheme (default https) and drops
  per-tag URLs.
- Otherwise it falls back to the connection's per-tag and default public
  base URLs.
- Fixed a pre-existing bug: `upsertServer` now writes the endpoint URLs and
  the PRM URL even when the definition is unchanged. Before, a changed base
  URL never reached imported servers until their definition changed.
- Changing the base URLs sets `next_sync_at = now`.

**Not done / follow-ups:**
- The Dashboard integration does not set `stripAuthorizationData` either. Its
  Studio-minted keys reach third-party upstreams. Decide separately; it
  changes existing proxies.
- The Gateway's own MCP analytics are not read; OSS writes them to Redis for
  Tyk Pump.
- The scale-up 404 window remains when storage is not shared (see the docs).
- `TestHostConnection_HostMovesToAnUnreachableDashboard` is flaky on `main`
  too (5/8 failures on the base commit). The test's explicit `reconcileHost`
  races the one `newTestServiceOn` starts. Not fixed here.

## 12. Verification record

**M0** (`tests/tykmcp/oss/m0_verify.py`, Gateway OSS 5.15.1 ×2 + Redis):
- A write lands on the answering node's disk only, and is served (even by
  that node) only after `/tyk/reload`.
- `/tyk/reload/group` does not spread it.
- `POST /tyk/mcps` with an existing id replaces it ("added").
- `?dryRun=true` validates without writing.
- Keys are shared through Redis: create on one node, use and `?hashed=true`
  read/update/delete on another.
- Inline `mcp_access_rights` filter `tools/list` and refuse blocked calls on
  the OSS build.
- `is_inactive` suspends.
- `$secret_env.X` works in header values (reads `TYK_SECRET_X`).
- `stripAuthorizationData` stops the client key reaching the upstream.
- A node lacking a proxy answers 404 on its listen path.
- `/tyk/apis/oas` does not list MCP proxies.
- **Trap:** the image runs as a non-root user, so a fresh root-owned
  `app_path` volume makes writes fail with "file object creation failed".

**M6 live run 1** (`tests/tykmcp/oss/live_e2e.py`, enterprise Studio binary
against the Compose cluster, gw3 joining mid-run): **68/68 checks passed.**
**Run 2**, after adding custom-domain and base-URL-change checks: **72/72**.
It passed twice in a row, so the suite cleans up after itself.
Covered:
- probe and activation, including shared Redis
- union import and the partial flag
- registration on both nodes with stripping verified at the upstream
- App binding, refusal of partial proxies, inline keys used on every node
- key access filtering on another node at once
- gw3 filled by the sync, with existing keys working on it
- a tampered definition restored
- a node down (partial run, unreachable) and removed (gone)
- rotate and revoke
- guardrails, and delete from every node
- DNS discovery against `localhost` with pinned dials

**Tests.**
- Enterprise package unhappy paths, `gateway_failures_test.go` (each was
  checked to run):
  - a definition the Gateway rejects leaves nothing behind (no row, no
    desired state, no node copy);
  - no node takes a write: the desired state is withdrawn;
  - one node fails a write, then catches up on the sync;
  - a reload failure is reported;
  - with shared storage, the writer fails over;
  - the backend refuses foreign ids without calling the Gateway;
  - node URLs follow the URL policy;
  - a rotated secret degrades the connection (nodes stay reachable) and
    mints are refused;
  - a mint refused by the Gateway removes the ledger row, and a retry works;
  - a key deleted or switched off on the Gateway;
  - a proxy lost on one node narrows the key at once, re-granting waits for
    review, and gone from every node means missing;
  - unbinding everything suspends the key;
  - a push edits every node, restores masked secrets and refuses stale
    hashes;
  - custom domain and base-URL change, plus the Dashboard regression
    `TestSync_BaseURLChangeReachesUnchangedServers`.
- A mutation check (disabling the URL refresh) fails
  `TestGatewayEndpoints_CustomDomainAndBaseURLChange`.
- Enterprise package: `gateway_test.go` with `fake_gateway_test.go` (per-node
  disk and loaded state, shared or separate Redis, a down node, and recording
  of calls outside the allow-list). Green under `-race` together with the
  existing suite.
- API: `api/tykmcp_gateway_enterprise_test.go`, plus the CE route list.
- Frontend: `TykConnectionForm.gateway.test.js`. Full Jest suite green
  (292 suites).
- `pkg/netguard`: a pinned-dial test.
