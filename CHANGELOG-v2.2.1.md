# Tyk AI Studio 2.2.1

**Status:** Released — `v2.2.1`
**Release date:** 2026-10-02
**Diff:** [`v2.2.0...v2.2.1`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.0...v2.2.1)

83 commits, 1,529 files, +162,172 / −70,833. Most of that is generated or vendored: without the regenerated Swagger, the in-repo copies of gorm and langchaingo, protobuf output and schema goldens, it is 1,137 files, +41,955 / −41,522.

2.2.1 is a patch release with three purposes.

- **The governance plugins.** **Gateway auth plugins now run on every gateway endpoint**, not only `/llm/`: the OpenAI-compatible `/ai` and unified `/v1` endpoints, routers, data sources, tools and their MCP transports, and custom plugin endpoints, each with its own explicit list. Plugins gain governance RPCs: team grants for their own resources, audit and component reads, and App suspension. The proxy log and chat records now show **who a call was for and which agent made it**. Together these are what the OAuth2 client-credentials plugin (1.3.1) and Asset Catalog (1.2.0) need, and both require 2.2.1.
- **Embedding.** AI Studio can now be **embedded in another Go program**, for example the Tyk Dashboard or MDCB, as a module (`pkg/studio`). The host can supply the configuration, authentication, roles, licence, analytics sinks, navigation and a Tyk Dashboard connection. It can share the host's Postgres database under its own schema, and run as a headless control plane for edges. The groundwork for several AI Studio replicas ships too, but the supported 2.2.x topology is still a hot/cold singleton.
- **Fixes.** Edges no longer drop idle control connections or lose analytics batches over 4 MB. `EDGE_TLS_CA_PATH` is honoured. AI Studio shuts down cleanly. An edge can no longer inject object-change events. A Vertex LLM on `/ai` or `/v1` no longer sends the caller's App secret to Google, and OpenAI prompt-cache tokens are recorded and billed.

Read **Breaking changes and upgrade notes** before upgrading. A few behaviours change in ways a 2.2.0 script or deployment can notice: App keys on LLMs that have an auth plugin, the push API's final states, and the Community Edition edge detail response.

---

## Highlights

- **Auth plugins on every gateway endpoint** ([#710](https://github.com/TykTechnologies/ai-studio/pull/710)). LLMs already had a list of auth plugins. Model and Semantic Routers, data sources, tools (REST and every MCP transport) and custom plugin endpoints now get one too, and an LLM's list now applies on `/ai/{slug}`, the unified `/v1`, `/anthropic/{slug}` and `/router/{slug}` as well as `/llm/`. There is no implicit fallback to "any loaded auth plugin", and the edge fails closed: an unloadable or erroring plugin answers `503`, and a plugin rejection answers `401 invalid credential` without the plugin's reason. The ACL, the inactive-App check and the budget apply to the App the plugin resolved. Edit the lists on each resource's detail page (**Authentication plugins**).
- **Who a call was for** ([#714](https://github.com/TykTechnologies/ai-studio/pull/714)). When an auth plugin authenticates a delegated token, the proxy log and the chat record keep the user it was for (`on_behalf_of`) and the acting agent (`acting_agent`, from the plugin's `auth_actor` claim). They travel from edges to AI Studio in the analytics pulse, and the App and LLM proxy-log tables show them as **For** and **Agent**. They are for audit only and are never used to authorise. `user_id` stays the App owner.
- **Governance RPCs for plugins** ([#712](https://github.com/TykTechnologies/ai-studio/pull/712)). A plugin can control who sees its resources: plugin resource types declare `default_access: auto | explicit`, explicit types are not granted to the Default team, and the plugin manages team grants itself, writing the same rows as the Teams page. It can read what AI Studio objects are made of and what happened to them, and it can suspend or flag an App without being able to change it. Five new plugin scopes cover this, and each RPC checks its scope.
- **Embeddable AI Studio** (`pkg/studio`). `studio.New` runs AI Studio from a configuration struct and returns errors instead of exiting ([#645](https://github.com/TykTechnologies/ai-studio/pull/645)). A host can:
  - serve it under a base path, authenticate users itself and serve the console under that path ([#650](https://github.com/TykTechnologies/ai-studio/pull/650));
  - supply roles ([#672](https://github.com/TykTechnologies/ai-studio/pull/672)), the licence ([#674](https://github.com/TykTechnologies/ai-studio/pull/674)) and analytics sinks ([#675](https://github.com/TykTechnologies/ai-studio/pull/675));
  - draw the navigation itself ([#676](https://github.com/TykTechnologies/ai-studio/pull/676), [#677](https://github.com/TykTechnologies/ai-studio/pull/677), [#680](https://github.com/TykTechnologies/ai-studio/pull/680));
  - hand over a Tyk Dashboard connection ([#709](https://github.com/TykTechnologies/ai-studio/pull/709)).

  AI Studio can be fetched with `go get` for the first time ([#657](https://github.com/TykTechnologies/ai-studio/pull/657)–[#662](https://github.com/TykTechnologies/ai-studio/pull/662)). It builds without cgo, and its gorm and langchaingo are private copies, so a host's `replace` directives cannot change them. See `features/Embedding.md`.
- **Headless control plane** ([#703](https://github.com/TykTechnologies/ai-studio/pull/703), [#704](https://github.com/TykTechnologies/ai-studio/pull/704)). `studio.NewControlPlane` runs only the edge-facing control server, for example inside MDCB, next to a full AI Studio sharing its Postgres database. It never changes the schema: it refuses a database that is missing or older than it understands (`studio.CheckSchema`, [#699](https://github.com/TykTechnologies/ai-studio/pull/699)). Its edges get configuration, pushes and budgets, and their analytics and plugin traffic reach the full AI Studio.
- **A sturdier control connection** ([#700](https://github.com/TykTechnologies/ai-studio/pull/700), [#701](https://github.com/TykTechnologies/ai-studio/pull/701), [#702](https://github.com/TykTechnologies/ai-studio/pull/702)):
  - The control server has a keepalive policy, so a quiet edge no longer gets `GOAWAY too_many_pings` and reconnects about every 90 seconds.
  - Messages up to 16 MB are accepted, where an analytics pulse over 4 MB used to be refused and retried forever.
  - `EDGE_TLS_CA_PATH` is actually loaded.
  - Panics in background loops and gRPC handlers are recovered, logged and counted (`aistudio_goroutine_panics_total`) instead of ending the process.
  - Nothing is written to the database after the analytics handler stops.
- **Durable configuration pushes** ([#666](https://github.com/TykTechnologies/ai-studio/pull/666), [#689](https://github.com/TykTechnologies/ai-studio/pull/689)). A push is recorded in the database, reaches an edge that was offline as soon as it reconnects (until the deadline), and is sent again if an edge drops mid-reload. Its status reports what each edge actually did. The edge's reconnect backoff is capped at about 30 seconds (it was 5 minutes).

## New features

### Gateway and plugins
- **Endpoint auth-plugin lists** for data sources, tools, Model Routers, Semantic Routers and plugin custom endpoints, with `GET`/`PUT /api/v1/{datasources,tools,model-routers,semantic-routers,plugins}/{id}/auth-plugins` ([#710](https://github.com/TykTechnologies/ai-studio/pull/710)). The lists reach edges in the configuration snapshot, in order. Edges apply them after the next push.
- **`on_behalf_of` and `acting_agent`** on `proxy_logs` and `llm_chat_records`, on the edge's `analytics_events`, in the plugin `AnalyticsData`, and in the pulse ([#714](https://github.com/TykTechnologies/ai-studio/pull/714)). The shared identity lives in `pkg/authidentity`, and `proxy.AuthIdentity` is an alias for it.
- **Plugin governance RPCs** ([#712](https://github.com/TykTechnologies/ai-studio/pull/712)):
  - `ListGroups`;
  - `Get`/`SetResourceInstanceGroups`;
  - `ListAccessibleResourceInstances`;
  - audit and component reads;
  - App governance state (suspend, resume with `apps:publish`, flag).

  The Teams page lists each plugin resource type and its published instances.
- **OpenAI prompt-cache tokens** (`prompt_tokens_details.cached_tokens` and `cache_write_tokens`) are parsed on the chat-completions path. They are subtracted from the prompt count and billed at the model's cache prices ([#679](https://github.com/TykTechnologies/ai-studio/pull/679)). For OpenAI and Google, a cache price left at 0 means "not set", and the tokens are billed at the input price ([#716](https://github.com/TykTechnologies/ai-studio/pull/716)).

### Embedding (`pkg/studio`)
- **Configuration and process control.** Configuration comes from a struct (`config.Load`/`LoadFrom`, `AppConf` fields for settings that used to be read from the environment where they were used), and errors are returned instead of exits ([#645](https://github.com/TykTechnologies/ai-studio/pull/645), [#671](https://github.com/TykTechnologies/ai-studio/pull/671)).
- **Serving under a host.**
  - `BASE_PATH`: the API, OAuth and the console are served under a prefix, and `SITE_URL` keeps its path.
  - Host authentication with CSRF: `Options.HostAuth`, `CSRF_KEY` and `CSRF_COOKIE_NAME`.
  - A chromeless console, with the navigation manifest `GET /common/nav` for hosts that draw their own menus.
  - The UI as a release tarball, for `studio_noui` builds.

  See [#650](https://github.com/TykTechnologies/ai-studio/pull/650), [#676](https://github.com/TykTechnologies/ai-studio/pull/676), [#677](https://github.com/TykTechnologies/ai-studio/pull/677), [#680](https://github.com/TykTechnologies/ai-studio/pull/680) and [#687](https://github.com/TykTechnologies/ai-studio/pull/687).
- **Host-supplied state.**
  - Host-managed roles: `HostIdentity.Roles`, Enterprise ([#672](https://github.com/TykTechnologies/ai-studio/pull/672)).
  - The licence from the host, read at every validity check so a renewal needs no restart ([#674](https://github.com/TykTechnologies/ai-studio/pull/674)).
  - Analytics sinks that receive every record next to AI Studio's own database ([#675](https://github.com/TykTechnologies/ai-studio/pull/675)).
  - `Options.HostTykConnection`, a Tyk Dashboard connection the host manages, Enterprise ([#709](https://github.com/TykTechnologies/ai-studio/pull/709)).
- **A shared Postgres database.** `DATABASE_SCHEMA` puts every AI Studio table in its own schema, and a migration lock lets several instances boot at once ([#664](https://github.com/TykTechnologies/ai-studio/pull/664), [#691](https://github.com/TykTechnologies/ai-studio/pull/691)).
- **A schema version record.** `studio_schema` holds the version and the oldest reader version. It is never lowered by an older build, and `studio.CheckSchema` reads it without DDL ([#699](https://github.com/TykTechnologies/ai-studio/pull/699)).
- **A fetchable Go module.**
  - The root module zip is valid ([#657](https://github.com/TykTechnologies/ai-studio/pull/657)).
  - `pkg/studio` builds with `CGO_ENABLED=0`: Chroma sits behind a cgo tag and SQLite is opt-in ([#658](https://github.com/TykTechnologies/ai-studio/pull/658)).
  - The gateway plugin SDK moved into the root module (`pkg/gatewayplugin`), ending the root ↔ microgateway module cycle ([#659](https://github.com/TykTechnologies/ai-studio/pull/659)).
  - langchaingo is an in-tree copy of Tyk's fork ([#660](https://github.com/TykTechnologies/ai-studio/pull/660)).
  - The enterprise module is `github.com/TykTechnologies/ai-studio-enterprise/v2` ([#661](https://github.com/TykTechnologies/ai-studio/pull/661)).
  - gorm is a pinned, verified in-repo copy ([#651](https://github.com/TykTechnologies/ai-studio/pull/651), [#652](https://github.com/TykTechnologies/ai-studio/pull/652)).
  - Every release tags the enterprise module and proves both editions can be imported from a clean module cache ([#662](https://github.com/TykTechnologies/ai-studio/pull/662), [#685](https://github.com/TykTechnologies/ai-studio/pull/685)).
- **Dependency versions follow the Tyk Dashboard's `go.mod`** where both require a module, so embedding does not raise the Dashboard's versions ([#663](https://github.com/TykTechnologies/ai-studio/pull/663), [#688](https://github.com/TykTechnologies/ai-studio/pull/688)). `make host-compat` checks this against the Dashboard and MDCB in CI ([#698](https://github.com/TykTechnologies/ai-studio/pull/698)).

### Control plane and replicas
- **Groundwork for several replicas on one Postgres database** ([#665](https://github.com/TykTechnologies/ai-studio/pull/665), [#666](https://github.com/TykTechnologies/ai-studio/pull/666), [#670](https://github.com/TykTechnologies/ai-studio/pull/670), [#692](https://github.com/TykTechnologies/ai-studio/pull/692)):
  - a node registry;
  - a cluster event log;
  - edge stream ownership;
  - a leader lease, with fast takeover after a crash restart;
  - cross-replica cache invalidation and plugin changes;
  - a bus relay.

  `GET /api/v1/cluster/status` reports the nodes. **The supported topology for 2.2.x remains a hot/cold singleton** ([#668](https://github.com/TykTechnologies/ai-studio/pull/668)); active replicas are supported from 2.3.
- **Edge ownership.** Edges record which replica holds their stream (`owner_node_id`, `owner_label`, `owner_live`). The Edge Gateways page shows a **Held by** column once edges are spread over more than one replica. Replicas carry a label and a may-lead flag (`6f1df93e`, `ce462c84`, `286c381a`).
- **Control server settings.** `GRPC_MAX_MESSAGE_SIZE` and the optional `GRPC_MAX_CONNECTION_AGE` and `GRPC_MAX_CONNECTION_AGE_GRACE`, which let edges spread out again across replicas behind a load balancer. On the edge, `EDGE_TLS_SERVER_NAME` and a TLS 1.2 minimum ([#700](https://github.com/TykTechnologies/ai-studio/pull/700)).

### Tooling and docs
- `make swagger` regenerates `docs/swagger` with a pinned swag. The API reference now documents 423 paths, up from 314 ([#715](https://github.com/TykTechnologies/ai-studio/pull/715)).
- `make schema-golden` and schema snapshot tests pin the schema that Studio's and the gateway's migrations produce, on SQLite and Postgres ([#651](https://github.com/TykTechnologies/ai-studio/pull/651)). A schema change must bump `models.SchemaVersion`.
- The README quickstart port and commands are fixed ([#708](https://github.com/TykTechnologies/ai-studio/pull/708)).
- The Helm chart and docs pin v2.2.0 images ([#695](https://github.com/TykTechnologies/ai-studio/pull/695)). They move to v2.2.1 after the release.

## Bug fixes

### Gateway
- **Vertex and Hugging Face LLMs on `/ai` and `/v1`** now answer `400 unsupported_vendor`, naming the `/llm/rest` and `/llm/stream` routes that serve them, and send nothing upstream ([#717](https://github.com/TykTechnologies/ai-studio/pull/717)).
  - Vertex sent each request to `generativelanguage.googleapis.com` with the caller's App secret as its bearer token, and failed there.
  - Hugging Face answered `500`.
  - In a failover waterfall the next LLM is still tried.
- **OpenAI prompt-cache tokens were discarded** on `/v1/chat/completions`, `/ai` and `/llm`, so a mostly cached prompt was billed as fresh ([#679](https://github.com/TykTechnologies/ai-studio/pull/679), [#716](https://github.com/TykTechnologies/ai-studio/pull/716)).
- **An auth-plugin rejection answered `500` with the plugin's reason.** It now answers `401 invalid credential` and logs the reason. A plugin "authentication" without an App ID is refused; it no longer falls back to App 1 ([#710](https://github.com/TykTechnologies/ai-studio/pull/710)).
- **A plugin attached to two LLMs broke edge sync** (`UNIQUE constraint failed: plugins.id`), and the order of an LLM's plugins was lost on the edge ([#710](https://github.com/TykTechnologies/ai-studio/pull/710)).
- **A blocked plugin response's framing headers** (`Content-Length`, `Transfer-Encoding` and the rest) were copied as they were, so a stale `Content-Length` gave the client an empty body. llm-cache did this on every streamed hit. The edge now frames the body itself ([#693](https://github.com/TykTechnologies/ai-studio/pull/693)).

### Control plane and edges
- **An edge on a quiet stream was disconnected about every 90 seconds** with `GOAWAY too_many_pings`, because the control server had no keepalive policy ([#700](https://github.com/TykTechnologies/ai-studio/pull/700)).
- **An analytics pulse or plugin batch over 4 MB was refused.** The edge retried it forever, and no later analytics from that edge arrived ([#700](https://github.com/TykTechnologies/ai-studio/pull/700)).
- **`EDGE_TLS_CA_PATH` was checked for existence but never loaded**, so an edge could not trust a hub certificate from a private CA ([#700](https://github.com/TykTechnologies/ai-studio/pull/700)).
- **An edge could publish `system.*` object-change events** that triggered the gateway reload, cache clears, webhooks and plugins as if AI Studio had changed an object. A control node now drops them ([#704](https://github.com/TykTechnologies/ai-studio/pull/704), [#707](https://github.com/TykTechnologies/ai-studio/pull/707)).
- **A push that arrived while an edge was still starting was dropped**, and waited for the one-minute timeout ([#689](https://github.com/TykTechnologies/ai-studio/pull/689)).
- **AI Studio did not exit on SIGTERM while an edge was connected.** It closed its database but kept serving: the embedded gateway answered `401` to everything, and buffered analytics were lost. It now stops in under a second, and nothing is written after the analytics handler stops ([#702](https://github.com/TykTechnologies/ai-studio/pull/702)). A record sent while the handler stopped could also panic with `send on closed channel` ([#653](https://github.com/TykTechnologies/ai-studio/pull/653)).
- **Server start-up failures exit with code 1.** These include the HTTP port, the gRPC port and TLS. Some of them exited 0 ([#691](https://github.com/TykTechnologies/ai-studio/pull/691)).

### Admin console and portal
- **The Community Edition edge detail page showed "Edge Gateway Not Found"**, because `GET /api/v1/edges/{id}` returned a bare object ([#706](https://github.com/TykTechnologies/ai-studio/pull/706)).
- **New `auto` plugin resource instances stayed invisible to non-admins** until the plugin was next loaded. They are now granted to the Default team at once ([#712](https://github.com/TykTechnologies/ai-studio/pull/712)).
- **The chat view dropped the base path** when it rewrote its URL ([#690](https://github.com/TykTechnologies/ai-studio/pull/690)).
- **Plugin pages wrote to the server on every view.** A viewer without `plugins:write` got a `403` toast. Host-authenticated consoles hide the local-account pages ([#684](https://github.com/TykTechnologies/ai-studio/pull/684)).

## Security

- **Vertex on `/ai` and `/v1` sent the caller's App secret to Google**, on AI Studio's embedded gateway and on edges ([#717](https://github.com/TykTechnologies/ai-studio/pull/717)). It was present in 2.2.0 and earlier. If Vertex LLMs were called through `/ai` or `/v1`, consider rotating the App secrets that were used.
- **Edges can no longer inject `system.*` object-change events** into the control plane ([#704](https://github.com/TykTechnologies/ai-studio/pull/704), [#707](https://github.com/TykTechnologies/ai-studio/pull/707)).
- **Auth-plugin rejections no longer leak the plugin's reason** to the caller, and a plugin answer without an App ID is refused ([#710](https://github.com/TykTechnologies/ai-studio/pull/710)).
- **The OAuth2 client-credentials plugin (1.3.1)** creates an auto-provisioned App inactive, and approval fails closed: an RPC failure or a narrower token no longer approves a gated App (ai-studio-enterprise#69).
- **Studio's dependency versions are aligned with the Tyk Dashboard's** ([#663](https://github.com/TykTechnologies/ai-studio/pull/663), [#688](https://github.com/TykTechnologies/ai-studio/pull/688)). Session cookies keep `HttpOnly` and `SameSite=Lax`, and `Secure` outside `DEVMODE`.

## Breaking changes and upgrade notes

- **An LLM that has an auth plugin now refuses App keys on `/ai`, `/v1`, `/anthropic` and `/router` on edges.** In 2.2.0 an LLM's auth plugins ran only on `/llm/`, so App keys still worked on the other endpoints ([#710](https://github.com/TykTechnologies/ai-studio/pull/710)). Clients using App keys on those endpoints need the plugin's tokens, or the plugin removed from that LLM.
  - With a plugin that lets unknown tokens through (for example the example custom-auth plugin with `reject_unknown_tokens=false`), App-key traffic on those paths now goes through the plugin and is attributed to whichever App it resolves.
  - New lists on data sources, tools and routers start empty, so nothing changes until you attach a plugin.
- **Auth-plugin lists apply on edges only.** AI Studio's embedded gateway does not run gateway auth plugins: it accepts App keys and refuses plugin tokens on every endpoint, as in 2.2.0. Serve plugin-authenticated traffic through a microgateway.
- **Deactivating an attached auth plugin opens its endpoints to App keys.** This is by design, but an endpoint is only protected while its plugin is active.
- **A valid App key sent as `Authorization: <key>` (without `Bearer`)** is now accepted on `/ai`, `/v1` and `/router`. It still is not on `/llm/`.
- **Push API (Enterprise and Community).** The paths and the response envelope are unchanged, and every 2.2 attribute is still returned ([#666](https://github.com/TykTechnologies/ai-studio/pull/666), [#689](https://github.com/TykTechnologies/ai-studio/pull/689)). Scripts may need to change:
  - **Final states:** `succeeded`, `succeeded_with_warnings`, `partially_failed`, `failed` or `expired`. `completed` and `timed_out` are gone, so a script that waits for `completed` never finishes.
  - **Operation IDs** start with `push-`.
  - **Enterprise `reload-all`** starts one operation for all namespaces.
  - **Status codes:** no edge to push to answers `409` (was `404`, or `202` with no operations); an offline edge answers `202` and the push waits for it (was `500`).
  - **The listing** comes from the database, with `counts`.

  See "Upgrading scripts from 2.2" in `docs/site/docs/edge-gateways.md`.
- **Community Edition `GET /api/v1/edges/{edge_id}` is wrapped in `{"data": …}`**, as Enterprise always was ([#706](https://github.com/TykTechnologies/ai-studio/pull/706)). CE API clients reading that one endpoint need to read `.data`.
- **Control connection:**
  - AI Studio now caps gRPC messages at 16 MB in both directions. 2.2.0 had no send limit. If you raised an edge's `GRPC_MAX_MESSAGE_SIZE` to receive larger snapshots, set it on AI Studio too.
  - `EDGE_TLS_CA_PATH` is now honoured and replaces the system roots, so a path that used to be ignored must now point at the hub certificate's issuer.
  - Edges require TLS 1.2 or later.
  - `GRPC_MAX_CONNECTION_AGE` needs a non-zero `GRPC_MAX_CONNECTION_AGE_GRACE`, or streams never close. Each age cycle disconnects an edge for about 5 seconds, so use ages of minutes or hours ([#700](https://github.com/TykTechnologies/ai-studio/pull/700)).
- **OpenAI cost figures change** ([#679](https://github.com/TykTechnologies/ai-studio/pull/679), [#716](https://github.com/TykTechnologies/ai-studio/pull/716)):
  - With cache prices set, cached tokens are billed at those prices.
  - With none set, the total is what 2.2.0 billed, but the record now shows the cached share separately.
- **Schema version 6.** New tables: `studio_schema`, `endpoint_auth_plugins`, `cluster_*` and `push_*`. New nullable columns:
  - `proxy_logs`/`llm_chat_records.on_behalf_of` and `acting_agent`;
  - `plugin_resource_types.default_access`;
  - `tyk_connections.host_key`;
  - `role_bindings.source`;
  - edge ownership on `edge_instances`.

  The edge's `analytics_events` gains the two identity columns. Every change is additive, and a 2.2.0 binary still runs on a 2.2.1 database, so a rollback works. On a rollback, endpoints with auth-plugin lists go back to accepting App keys.
- **Docker `v2.2` tag.** The floating `v2.2` image tag moves to 2.2.1 with this release, carrying the changes above.
- **Exit codes.** A server that fails to start now exits with code 1 ([#691](https://github.com/TykTechnologies/ai-studio/pull/691)).
- **Go embedders and plugin authors:**
  - The enterprise module path is `github.com/TykTechnologies/ai-studio-enterprise/v2`.
  - Gateway plugin interfaces moved to `pkg/gatewayplugin`. The old `microgateway/plugins` paths forward to it.
  - Thirteen methods were added to the Go `StudioServices` interface, so custom fakes must add them. Built plugins keep working.
  - The `go` directive is 1.26.5, with `toolchain go1.26.6`.
  - About 69 module versions moved to match the Tyk Dashboard, among them grpc 1.84, TIB 1.8, libopenapi 0.36 and gorilla/sessions 1.4.
- **Helm.** The Studio Deployment uses the `Recreate` strategy ([#692](https://github.com/TykTechnologies/ai-studio/pull/692)), so an upgrade briefly has no Studio pod. Edges keep serving.

### New configuration

| Variable | Default | Purpose |
|---|---|---|
| `BASE_PATH` | unset | Serve the API, OAuth and the console under a path prefix |
| `DATABASE_SCHEMA` | unset | Postgres only: keep every AI Studio table in this schema. Up to 63 characters are accepted, but keep it to 41 or fewer: a longer name stops cross-replica events |
| `MIGRATION_LOCK_TIMEOUT` | `15m` | How long a booting instance waits for another's migrations |
| `CSRF_KEY` | random per process | Secret the CSRF key is derived from; set it so tokens survive restarts and several instances can serve one console |
| `CSRF_COOKIE_NAME` | `_gorilla_csrf` | CSRF cookie name, for hosts that set their own |
| `GRPC_MAX_MESSAGE_SIZE` (AI Studio) | 16 MB | Control-server send and receive limit; the edge already had this setting |
| `GRPC_MAX_CONNECTION_AGE` | off | Close edge streams after this age so they rebalance across replicas |
| `GRPC_MAX_CONNECTION_AGE_GRACE` | off | Grace period for the above; required for it to take effect |
| `EDGE_TLS_SERVER_NAME` (edge) | unset | Server name to verify on the hub certificate |

No new setting is required, and none changes behaviour while unset.

## Known issues

- **Re-activating a plugin does not restart its AI Studio half.** After switching a plugin off and on, use **Reload** on the plugin (`POST /api/v1/plugins/{id}/reload`). For the OAuth2 client-credentials plugin this matters: until the reload, edges keep their last binding table, so unbinding a client or deactivating its App does not take effect.
- **An in-place plugin upgrade through a `file://` command is not reloaded by running edges.** Restart the edges afterwards. Marketplace (OCI) upgrades change the checksum and reload as usual.
- **An edge's analytics pulse is split by event count, not size.** With request and response bodies stored and very large prompts, a pulse can exceed 16 MB and stall that edge's analytics.
- **The Held by column labels every replica "studio".** The node label can only be set when embedding.

## Submodules

- **enterprise** `1360a1f` → `118ad19`:
  - the embedding halves ([ai-studio-enterprise#47](https://github.com/TykTechnologies/ai-studio-enterprise/pull/47)–[#57](https://github.com/TykTechnologies/ai-studio-enterprise/pull/57)): gorm isolation, in-tree langchaingo, the module rename, cluster replicas, host roles, host licence and configuration limits;
  - dependency-alignment fallout ([#58](https://github.com/TykTechnologies/ai-studio-enterprise/pull/58), [#60](https://github.com/TykTechnologies/ai-studio-enterprise/pull/60));
  - Postgres startup safety and the webhooks `pg_trgm` schema ([#59](https://github.com/TykTechnologies/ai-studio-enterprise/pull/59));
  - the host-managed Tyk Dashboard connection ([#61](https://github.com/TykTechnologies/ai-studio-enterprise/pull/61));
  - a date-independent audit Postgres test ([#62](https://github.com/TykTechnologies/ai-studio-enterprise/pull/62));
  - **Asset Catalog 1.2.0** ([#63](https://github.com/TykTechnologies/ai-studio-enterprise/pull/63), [#65](https://github.com/TykTechnologies/ai-studio-enterprise/pull/65)): governance inventory, dependency graphs, risk rating, composer, CycloneDX export, and the App governance chain;
  - **OAuth2 client-credentials 1.2.0 → 1.3.1** ([#64](https://github.com/TykTechnologies/ai-studio-enterprise/pull/64), [#66](https://github.com/TykTechnologies/ai-studio-enterprise/pull/66)–[#69](https://github.com/TykTechnologies/ai-studio-enterprise/pull/69)): revocation, validation, audit identity, and approval for auto-provisioned Apps, off by default and overridable per mapping, failing closed.

  Both plugins require AI Studio 2.2.1. `min_studio_version` is shown but not enforced, so do not install them on 2.2.0.
- **community** `42f1c50` → `2e23cdf`: llm-cache 1.2.0 serves the right response shape to every endpoint and vendor; plugin `go.mod` files tidied after the dependency alignment.
- **tyk-internal** `ac84594` → `e5db6ed`: plugin `go.mod` files tidied after the dependency alignment.
