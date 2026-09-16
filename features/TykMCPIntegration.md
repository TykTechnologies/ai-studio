# Tyk APIM ↔ AI Studio MCP integration — discovery & design spec

Status: design 2026-09-16; M1–M5 implemented (see §18), M6–M7 pending. Reviewed once for codebase and Tyk API consistency. Enterprise Edition only. Target: AI Studio ≥ 2.3, Tyk Dashboard/Gateway ≥ 5.13 (remote MCP proxies), ≥ 5.15 for REST-to-MCP.

## 1. Context

Tyk Gateway now natively terminates MCP (Streamable HTTP, JSON-RPC 2.0, spec 2025-11-25) as **MCP proxies**: Tyk OAS definitions with `mcpTools` / `mcpResources` / `mcpPrompts` middleware, per-primitive policies, OAuth 2.1 protected-resource metadata, and MCP analytics. That is the supported way for a Tyk customer to run MCP in production.

AI Studio's own MCP surfaces are the wrong tool for that job:

- The embedded gateway serves MCP only as a shim *over Studio Tools* (`proxy/proxy.go` `handleMCPTool*`); it never consumes remote MCP servers.
- The internal `mcp-registry` plugin (`tyk-internal/plugins/mcp-registry`, KV-backed, proxies through the microgateway) exists for Tyk Technologies' own use only; it is not a customer feature and is not part of this integration.
- Yet the platform was designed for an "MCP servers" asset class: `mcp_servers` is the canonical example in `docs/site/docs/plugins-resource-types.md`, `docs/site/docs/architecture.md:18` already promises portal users can browse MCP servers, and nothing implements it.

The gap: AI Studio Enterprise is the governance, discovery and consumption layer for AI in the org, while the APIM stack owns the MCP data plane. This spec makes the two work as one system for four capabilities:

1. **Discovery** (Dashboard → Studio): MCP proxies are imported and published to the AI Portal as a first-class asset class.
2. **Registration** (Studio admin → Dashboard): an admin creates an MCP asset in Studio and pushes it to the Dashboard as a managed proxy.
3. **Community registration** (Portal user → admin approval → Dashboard): the same, initiated by a portal user through the existing submission workflow.
4. **Access credential brokering**: Studio mints, tracks, updates and revokes Tyk keys for Apps that use MCP assets, records who has access to what, and handles the auth modes a Tyk MCP proxy can enforce (including the OAuth ones where Studio mints nothing).

### Decisions already taken (with the user)

| Decision | Choice | Why |
|---|---|---|
| Packaging | **In-tree enterprise feature** (`services/tykmcp` contract + `enterprise/features/tykmcp` + core `models/`), not a marketplace plugin | Plugins are KV-only, cannot reach the secrets store, cannot page/filter in SQL, and their mutations audit as one opaque RPC record. Connection credentials, reviewer workflow, minted-key ledger and a leased sync worker are relational, audited state. Follows the webhooks pattern. |
| Data plane | **Tyk Gateway only.** Studio never proxies MCP traffic for Tyk-managed servers. Clients call the Tyk listen path with a Tyk-issued key. | Removes the double enforcement surface; the Tyk-internal plugin stays out of the customer product. |
| Chat / agents consuming Tyk MCP servers | **Design-only later phase** (§13) | Studio has no MCP client today; sizeable, separable. |
| Credential model | **Studio-minted Tyk keys per App per connection**, built from **policy bundles pinned per MCP asset**, never one policy per app | Tyk policies are templates: editing a pinned policy's limits changes every linked key at request time, no Studio work. Mirrors the Tyk Developer Portal (Products = ACL policies, Plans = rate/quota policies, merged into one key). |
| Connection trust levels | **Three modes**: `catalogue`, `broker`, `full` | Low-trust orgs (platform team owns APIM, AI team read-only), mid-trust (AI team may mint keys against pinned policies), same-team (AI team may create proxies and policies). Studio runs at the lower of the declared mode and what it can verify. |
| Consumption tiers | **One bundle per asset in v1**; tier picker is a follow-on (§14) | Keeps the App builder unchanged. |
| Key drift | **Automatic, audited; approval only when access widens** | Removals apply immediately; a policy that grants a *new* proxy re-enters approval unless the actor holds `mcp-credentials:execute`. |
| License gate | Enterprise availability only (`tykmcp.IsEnterpriseAvailable()`); **no new license claim** | Licensing has no capacity for new claims. If a claim is ever added it must follow the restrict-only form at `services/service.go:437-438` (`!ok \|\| ent.Bool()`), not the `handleFeatureSet` absent-means-false form. No config switch may relax the gate. |

### Non-goals (v1)

- Proxying MCP through the Studio microgateway or embedded gateway for Tyk-managed servers.
- Replacing Tyk's MCP Designer: Studio's registration forms cover the common shapes; anything exotic is edited in the Dashboard and synced back.
- Cataloguing MCP servers that are not behind Tyk (kept possible by a nullable connection id, not built).
- Tyk Developer Portal interplay beyond "another outlet".
- Tyk Classic API definitions: MCP proxies are Tyk OAS only.
- Deferred to §14 after review: discovery probe key, rotation grace window, CSV export of the access report.

## 2. Glossary

| Tyk term | Meaning | Studio term |
|---|---|---|
| MCP proxy / MCP OAS definition | Tyk OAS doc with `x-tyk-api-gateway` and MCP middleware, listed at `GET /api/mcps` (excluded from `/api/apis`) | **MCP server** (asset) |
| Remote MCP server | `upstream.url` is a remote MCP endpoint | kind `remote` |
| REST API to MCP (5.15+, Enterprise) | `upstream.url = tyk://<api-id>/mcp` + root `x-tyk-mcp-server.primitives[]` | kind `rest_to_mcp` |
| Primitives | tools / resources / prompts; each may carry `ignoreAuthentication`, `security`, `scopeCheck` | tools, resources, prompts (with per-primitive auth) |
| Security policy, partitions `acl`, `rate_limit`, `quota`, `complexity`, `per_api` | Template applied to many keys. MCP-only ACL fields (`mcp_access_rights`, `json_rpc_methods_access_rights`, `mcp_primitives`, `json_rpc_methods`) are **documented but absent from the nightly swagger and the 5.14 schema**; treated as unverified until M4 | **policy** (cached), **bundle** (pinned set: one access policy + 0..n consumption policies) |
| Key / session (`SessionState`, `apply_policies`, `alias`, `meta_data`, `expires` epoch, `key_hash`) | Consumer credential | **MCP credential** (minted key ledger row) |
| Dashboard user access key (`Authorization` header) | Per-user Dashboard API key | **connection token** |
| PRM (RFC 9728) | `securitySchemes[<name>].oauth2.protectedResourceMetadata` (new, wins) or the deprecated `authentication.protectedResourceMetadata` | surfaced on the asset detail page |

## 3. Architecture

```
 AI Studio (control plane, Enterprise)                       Tyk APIM
 ┌──────────────────────────────────────────────┐            ┌─────────────────────────┐
 │ Admin UI      Portal UI      Submission flow │            │ Tyk Dashboard :3000     │
 │   │              │               │           │  REST      │  /api/mcps              │
 │   ▼              ▼               ▼           │ (user key) │  /api/portal/policies   │
 │ api/tykmcp_handlers.go  api/portal_catalog…  │◀──────────▶│  /api/keys[/…]          │
 │   │                                          │            │  /api/apis (REST→MCP)   │
 │   ▼                                          │            └────────────┬────────────┘
 │ services/tykmcp (contract, CE stub)          │                         │ config push
 │ enterprise/features/tykmcp                   │            ┌────────────▼────────────┐
 │   client/   dashboard client + URL policy    │            │ Tyk Gateway(s) :8080    │
 │   sync/     leased poll: proxies, policies   │            │  /<listen-path>/mcp     │
 │   broker/   mint / drift / revoke keys       │  MCP over  │  auth, ACL, rate, quota │
 │   register/ create/update proxies, policies  │  HTTP      │  MCP analytics          │
 │ models/tyk_connection.go, mcp_server.go, …   │   ▲        └────────────┬────────────┘
 └──────────────────────────────────────────────┘   │                     │
                                                    │                     ▼
        MCP clients (Claude Desktop, IDEs, agents) ──┘            upstream MCP servers / REST APIs
        hold a Studio-minted Tyk key (or an OAuth token from the advertised AS)
```

Studio is the catalogue, the approval workflow, the credential broker and the audit ledger. Tyk enforces. Nothing on the MCP data path touches Studio.

Layering follows `features/Webhooks.md`: core owns the models (migrated in both editions), the service contract, the CE stub, the HTTP handlers and the UI; the enterprise submodule owns the implementation, registered by `enterprise/features/tykmcp/init.go` and one blank import in `main_enterprise.go`. **Every write handler goes through the enterprise service**, so in CE and in a disabled ENT state (`Status().Enabled == false`) no write path exists: `GET /api/v1/tyk-mcp/status` answers 200 `{available:false}`, every other route 403 `ErrEnterpriseFeature` (exactly the webhooks convention, `api/webhook_handlers.go:221-223`).

## 4. Trust model: connections and modes

A **Tyk connection** is one Dashboard URL + one Dashboard user access key + one organisation. Several connections are supported; every MCP asset, policy and credential belongs to exactly one.

| Mode | Studio may | Dashboard user needs | Typical org |
|---|---|---|---|
| `catalogue` | list/import MCP proxies, list policies, list Tyk OAS APIs (for REST-to-MCP source pickers), read details of keys it minted earlier (reconciliation after a downgrade) | apis read, policies read, mcp read (+ keys read if downgraded from broker) | platform team owns APIM; AI team discovers and publishes; registrations become **handoff packages** (§7.3) |
| `broker` | catalogue + mint/update/revoke keys against pinned policies | + keys write | AI team runs the AI portal on top of platform-managed proxies |
| `full` | broker + create/update MCP proxies, create policies (minimal creator), delete studio-origin proxies | + apis write, policies write, mcp write | same team, or high trust |

**Declared vs effective.** The admin picks the declared mode. On save, on activation and on every sync Studio runs a **capability probe** with side-effect-free calls. Because the Dashboard offers no permission-introspection endpoint usable with an API key (`/api/users/me` is cookie-only and 404s with a key on 5.14), write capabilities can only be proven by a real write:

| Capability | Probe | Result states |
|---|---|---|
| `mcp_read` | `GET /api/mcps?p=0` | ok / denied |
| `policies_read` | `GET /api/portal/policies?p=1` | ok / denied |
| `apis_read` | `GET /api/apis?p=1` | ok / denied |
| `mcp_supported`, `rest_to_mcp_supported` | `GET /api/schemas/apidefs/mcp` exists; schema mentions `x-tyk-mcp-server` | ok / no |
| `keys_write` | `POST /api/keys/preview` with a synthetic session (validates without creating; proves the endpoint exists, not write rights) | `unverified` until the first successful mint, then ok; denied on 403 |
| `keys_read_by_hash` | lazily, on first reconcile | unverified / ok / denied |
| `mcp_write` | `POST /api/mcps?dryRun=true` (schema validation only) | `unverified` until first successful create |
| `mdcb_read` (only when MDCB is configured) | `GET {mdcb_url}/dataplanes` | ok / denied / unreachable; never blocks the connection |
| `policies_write` | none exists | `unverified` until first successful create |
| `key_delete_by_hash` | none exists (gateway config) | learned on first revoke (§7.4) |

The effective mode is the lower of the declared mode and the probed capabilities; `unverified` capabilities let the mode stand but are shown as such in the UI, and the first 401/403 on a real call flips the connection to `degraded` with the failing capability named, notifies admins (`NotificationService.NotifyDirect`), and the Studio action returns `ErrCapabilityUnavailable` (typed, not a 500).

**Connection lifecycle**: `pending → active → disabled`, plus `degraded` as an overlay flag. Creating or editing URL/token/mode needs `tyk-connections:write`; activating needs `tyk-connections:execute` (the outward-data precedent from webhooks approve). `TYK_MCP_REQUIRE_DIFFERENT_ACTIVATOR=true` enforces four-eyes like `WEBHOOKS_REQUIRE_DIFFERENT_APPROVER`.

### 4a. Gateway segmentation and MDCB (optional per connection)

In sharded deployments (MDCB data planes, or Dashboard-connected gateways started with `node_is_segmented` and tags) a definition is loaded only by gateways whose tags match `x-tyk-api-gateway.server.gatewayTags`. A proxy created from Studio without tags would be loaded by every non-segmented gateway and by no segmented one, which is an operational trap for a submitter who cannot know the topology. Two sources tell Studio which tags exist:

- **MDCB**: when `mdcb_url` + `mdcb_access_token` are set, the probe and every sync call `GET {mdcb_url}/dataplanes` (`X-Tyk-Authorization`), extract `group_id`, `tags`, `node_id`, `node_version`, health and host details into `data_planes`, and discard the rest. The raw body is never logged or stored: each node object carries the gateway's own `api_key`. Failures degrade only the MDCB capability (`mdcb_read`), never the connection.
- **Manual**: `known_gateway_tags` for segmented-but-not-MDCB stacks, or to label tags MDCB reports (`{tag, label, description}`).

The **deployment target** control (registration wizard, submission form, edit form, handoff package) is rendered only when the union of discovered and known tags is non-empty. It is a multi-select of tags showing, per tag, the data planes that carry it and their node count and health; the choice writes `gatewayTags {enabled:true, tags:[…]}`. A connection whose data planes are all untagged and whose known list is empty shows nothing, and the definition is created without `gatewayTags` (today's behaviour). When tags exist but the user picks none, the form warns that only non-segmented gateways will load the proxy and requires an explicit confirmation. The admin review of an imported proxy shows its tags and flags a mismatch against the discovered set (a tag no data plane advertises) as a warning.

**Dashboard-side identity.** Recommended: a dedicated Dashboard user per Studio connection whose Dashboard permissions *are* the trust boundary; Studio's mode can only narrow it. Studio never uses the admin secret (the `admin-auth` header is never sent). The org id is admin-entered (pre-filled from `data.org_id` of the `keys/preview` response when available; an empty Dashboard has no listed object to read it from) and every synced object's `org_id` is checked against it.

## 5. Data model (core `models/`, AutoMigrate in both editions)

All new tables use `gorm.Model` unless noted; timestamps in UTC; JSON columns as `text` so the audit trail can snapshot rows.

**Secret column naming rule.** The audit diff redacts by column-name fragment (`enterprise/features/audit/diff.go:66-69`: `password, secret, api_key, apikey, token, auth_key, …, access_key, authorization, cookie`). Every encrypted column below therefore contains `token` or `secret` in its name, and a unit test asserts each is redacted in a snapshot diff.

**Encryption rule.** `secrets.EncryptValue` passes plaintext through when `TYK_AI_SECRET_KEY` is unset (`secrets/encrypt_fallback_test.go`). The GORM `BeforeSave` hooks on these models therefore **return an error** when `!secrets.EncryptionKeyConfigured()` instead of falling back, and the enterprise service reports `Enabled=false` in that state (webhooks precedent, `enterprise/features/webhooks/service.go:129-136`), so no write path exists without a key.

### `tyk_connections`
| Column | Notes |
|---|---|
| `name`, `description` | |
| `dashboard_url` | validated by the always-on URL policy (§10) |
| `gateway_base_url` | public base URL clients use to reach proxies (e.g. `https://gw.example.com`); the Dashboard API does not expose the gateway hostname. Used to render endpoint docs. |
| `dashboard_access_token` (`json:"-"`) | Dashboard user access key, encrypted by hooks. DTO carries `has_token` + last-4 hint. |
| `org_id` | admin-entered / pre-filled (see §4) |
| `declared_mode`, `effective_mode` | `catalogue \| broker \| full` |
| `capabilities` (text JSON) | per-capability `ok \| denied \| unverified \| no` + probe time; `dashboard_version` if detectable; `revoke_mode` (`delete \| inactive_only`) learned on first revoke |
| `status` | `pending \| active \| disabled`; `degraded bool`, `degraded_reason` |
| `sync_interval_seconds` | default 300; min `TYK_MCP_SYNC_MIN_INTERVAL` |
| `next_sync_at`, `sync_lease_owner`, `sync_lease_until` | lease columns (§7.1) |
| `auto_publish` | default false; when true requires `default_privacy_score` and publishes as `SYSTEM` (audited) |
| `default_privacy_score` (nullable) | administrator-chosen; required for `auto_publish`. Without `auto_publish` every imported server keeps a null score until an administrator sets one, and publish refuses meanwhile |
| `accept_handoffs` | default true; lets `catalogue`/`broker` connections receive community registrations as handoff packages |
| `key_defaults` (text JSON) | `expires_in_seconds` (0 = never), `alias_prefix`, `detailed_recording` |
| `allow_internal_host` | per-connection exemption for an internal Dashboard hostname (§10) |
| `mdcb_url` (nullable), `mdcb_access_token` (`json:"-"`, encrypted), `mdcb_allow_internal_host` | optional MDCB section (§4a); same URL policy and encryption rules as the Dashboard fields |
| `known_gateway_tags` (text JSON) | admin-maintained list of segmentation tags for Dashboard-connected segmented gateways without MDCB; merged with MDCB-discovered tags |
| `gateway_base_urls` (text JSON) | `{ "<tag>": "https://…" }` public base URL per deployment tag; `gateway_base_url` stays the default |
| `data_planes` (text JSON) | MDCB snapshot: `[{group_id, tags[], node_count, node_versions[], healthy, last_seen}]`; never the raw response (it carries each node's `api_key`) |
| `last_sync_at`, `last_sync_status`, `last_sync_error`, `last_probe_at`, `last_mdcb_probe_at` | |
| `created_by_user_id`, `activated_by_user_id`, `activated_at`, `lock_version` | |

### `mcp_servers` (the asset)
| Column | Notes |
|---|---|
| `connection_id` (FK, nullable for a future non-Tyk kind) | |
| `tyk_api_id` (unique with connection; nullable while `pending_platform`) | `x-tyk-api-gateway.info.id` |
| `name`, `name_overridden`, `slug` (unique), `description`, `long_description`, `logo_url`, `tags` (text JSON) | Studio-owned presentation; seeded from the definition on import; `name` follows the Dashboard unless overridden |
| `kind` | `remote \| rest_to_mcp` |
| `listen_path`, `transport_path` | `server.listenPath.value`; transport path taken from the OAS `paths` entry whose POST operation is the MCP transport (`mcpTransportPost`, else the first POST), default `/mcp` |
| `gateway_tags` (text JSON) | `{enabled, tags[]}` from `x-tyk-api-gateway.server.gatewayTags`; definition-derived, follows the Dashboard. Shown on the admin detail page always, and on the portal detail page as "Deployed to" (tag chips) |
| `endpoint_url`, `endpoint_urls` (text JSON) | default: `gateway_base_url + listen_path + transport_path` (respecting `strip`); when `gateway_tags.enabled`, one URL per tag that has an entry in the connection's `gateway_base_urls`, else the default with a "confirm the gateway host with your platform team" note. Recomputed when the connection's URLs change |
| `upstream_url` | admin-only, never in portal responses |
| `source_api_id` | REST-to-MCP only |
| `auth_mode` | proxy-level default derived from **the scheme objects**, not their names: OAS `components.securitySchemes[name].type/scheme/in` when present, else the vendor block's discriminator (`X-Tyk-Token`, `X-Tyk-JWT`, `X-Tyk-Basic`, `X-Tyk-OAuth`, `X-Tyk-ExternalOAuth`, `X-Tyk-OAuth2`, `certificateAuth`, `hmac`, `oidc`, `custom`). Values: `keyless \| auth_token \| basic \| jwt \| oauth_tyk \| oauth_external \| oauth21 \| mtls \| hmac \| custom \| mixed`; unresolvable ⇒ `custom` (not brokerable) |
| `auth_details` (text JSON) | header/param name and location for token schemes; PRM (`resource`, `authorizationServers[]`, `scopesSupported[]`, `autoDeriveScopes`, `wellKnownPath`) read from the scheme-level block first, deprecated top-level block second; never secrets |
| `primitives` (text JSON) | tools/resources/prompts known to Studio from `x-tyk-mcp-server` (REST-to-MCP) and the `mcpTools`/`mcpResources`/`mcpPrompts` map keys. Each: `type, name, description, annotations, auth {ignore_authentication, scopes}` |
| `definition` (text) | last synced OAS JSON with `upstream.authentication` values masked (`***`) |
| `definition_hash` | SHA-256 of the canonicalised **unmasked** doc (so a Dashboard-side upstream header change is still detected); the hash is computed before masking and the unmasked doc is discarded |
| `dashboard_state` | `active \| inactive \| missing \| pending_platform` |
| `origin` | `dashboard \| studio \| submission` |
| `submission_id` (nullable FK) | |
| `privacy_score` (nullable) | must be set before publish |
| `is_active` | published in the portal; `publish` refuses unless `dashboard_state=active` and `privacy_score` is set |
| `brokerable` | computed: `effective_mode ≥ broker` AND `auth_mode ∈ {auth_token, basic}` (or `mixed` including one of them) AND the bundle has a valid access pin |
| `owner_user_id`, `community_submitted` | |
| `last_seen_at`, `last_synced_at`, `lock_version` | |

Studio-owned fields (`description`, `long_description`, `logo_url`, `tags`, `name` when overridden, `privacy_score`, `is_active`, team grants, bundle) are never overwritten by sync. Definition-derived fields always follow the Dashboard.

The GORM relation on `models.App` is `MCPServers []MCPServer gorm:"many2many:app_mcp_servers" json:"-"`, excluded from the generic `App.Get` preload; App DTOs carry a slim `mcp_servers[] {id, name, slug, connection_id, auth_mode, endpoint_url}` projection.

### `mcp_server_groups`
`mcp_server_id`, `group_id` — direct team grants (same model as `group_plugin_resources`). A newly published server is visible to no team until granted, except under `auto_publish` (Default team).

### `tyk_policies` (cache)
| Column | Notes |
|---|---|
| `connection_id`, `tyk_policy_id` (unique together) | |
| `name`, `active`, `is_inactive`, `partitions` (text JSON), `tags` (text JSON), `key_expires_in` | |
| `mcp_api_ids` (text JSON) | MCP proxy ids found in `access_rights`; non-empty ⇒ MCP-related |
| `is_partitioned`, `has_acl`, `has_rate_limit`, `has_quota`, `has_complexity`, `has_per_api` | denormalised |
| `studio_managed` | created by the minimal creator (tagged `studio-managed`, `meta_data.studio_connection_id`) |
| `raw` (text) | full policy JSON, admin-only |
| `dashboard_state` | `present \| missing` |
| `last_seen_at` | |

### `mcp_server_policy_pins` (the bundle)
`mcp_server_id`, `tyk_policy_id` (FK), `role` (`access \| consumption`), `position`, `invalid_reason` (set by sync when the policy vanishes or no longer covers the server). Rules in §8.2.

### `app_mcp_servers`
`app_id`, `mcp_server_id` (unique together).

### `mcp_credentials` (minted-key ledger)
| Column | Notes |
|---|---|
| `id` (UUID string PK) | |
| `app_id`, `connection_id` | partial unique index where `status IN (minting, active, suspended)` — one live key per App per connection |
| `status` | `minting \| active \| suspended \| revoked \| failed` |
| `tyk_key_hash` | stable identifier; plaintext never stored |
| `tyk_key_plain_token` (`json:"-"`, encrypted) | only when the Dashboard runs unhashed keys (`key_id` equals the plaintext); needed for update/delete |
| `alias` | `<alias_prefix>app:<app_id>` |
| `applied_policy_ids` (text JSON) | what Studio last wrote/observed in `apply_policies`, Studio-owned ids only |
| `external_policy_ids` (text JSON) | ids present on the key that no pinned bundle owns; preserved, reported, never removed by Studio |
| `desired_policy_ids` (text JSON) | union of bundles of the App's servers on this connection |
| `drift` | `none \| pending_widen \| applying \| error` |
| `expires_at` | read back from the mint response `data.expires` (policies' `key_expires_in` override the request) |
| `minted_by_user_id`, `minted_at`, `revealed_to_user_id`, `revealed_at` | reveal happens exactly once, in the mint response |
| `revoked_by_user_id`, `revoked_at`, `revoke_reason`, `revoke_mode` (`deleted \| inactive_only`) | |
| `last_synced_at`, `last_error`, `lock_version` | |

### `mcp_access_grants`
Who has access to what, independent of whether a key exists. One row per (`app_id`, `mcp_server_id`) with `user_id` (App owner at grant time), `grant_kind` (`key \| oauth \| keyless \| external`), `credential_id` (nullable), `granted_at`, `granted_by_user_id`, `revoked_at`, `revoke_reason`. Written on App credential activation **and on every binding change while the credential is active** (`UpdateApp` / `UpdateAppWithResources` paths in `services/app.go`, not only `ActivateAppCredential`). This satisfies "logged who has access" for OAuth/mTLS/keyless servers.

### `mcp_sync_runs`
`connection_id`, `started_at`, `finished_at`, `node_id`, `status`, counters, `error`. Retention `TYK_MCP_SYNC_RUN_RETENTION` (default 30 days).

### `submissions` (existing)
New `resource_type = "mcp_server"`. Concrete edits: `CreateSubmission` type check (`services/submission_service.go:106-107`), `createResourceFromSubmissionTx` dispatch (`:561-566`), `payloadCredentialFields` (`models/submission.go:83`) gains `upstream_auth_token`, and the `[redacted]` response masking covers it; `SubmissionForm.js` type list (`:55-80`). The JSON schema carries no `x-secret` marker (nothing reads it).

### Governed metadata
`GovernedObjectTypeMCPServer = "mcp_server"` added to `BuiltinObjectTypes()` (`services/governed_metadata/factory.go:35-41`), to `isValidManifestAppliesTo` (`models/plugin_manifest.go:112-115`) and to the enterprise validator; the detail page renders portal-visible governed metadata like the other types.

### Audit
Add `tyk-connections`, `mcp-servers`, `mcp-credentials` (UUID → `column:"id"`) to `snapshotTargets`. Sync-worker and broker mutations outside HTTP call `audit.Record` as `SYSTEM` (`mcp_server.imported|missing|resumed`, `mcp_credential.drift_applied|external_change|revoked`, …).

## 6. Service layout

```
services/tykmcp/
  interface.go        Service, DTOs, errors (ErrEnterpriseFeature, ErrDisabled, ErrCapabilityUnavailable,
                      ErrInvalidBundle, ErrNotBrokerable, ErrDashboardConflict, ErrInvalidState, ErrNotVisible)
  factory.go          RegisterEnterpriseFactory, IsEnterpriseAvailable
  community.go        stub
enterprise/features/tykmcp/
  init.go, service.go Deps{DB, Bus, Notifier, Audit, Config, NodeID, Version}; Start/Stop; Status()
  client/             Dashboard REST client (typed calls), one http.Client per connection with its own URL policy
                      (extracted from enterprise/features/webhooks/policy.go into pkg/netguard/policy.go, since the
                      webhooks identifiers are package-private), timeouts, no redirects, Proxy=nil, response caps,
                      jittered retry on 429/5xx for idempotent calls only
  probe.go            capability probe (§4)
  sync/               connection-leased poll engine (§7.1), diff + hash, policy refresh, credential reconcile
  registry.go         import/publish/grant/pin logic, bundle validation
  register/           proxy builders (remote, rest_to_mcp), dry-run, create/update with conflict rule,
                      minimal policy creator, handoff packages, link
  broker/             mint / drift / revoke / rotate / suspend
  report.go           access report
  events.go           bus topics
api/tykmcp_handlers.go, api/tykmcp_portal_handlers.go
```

`services.Service.InitTykMCP(cfg, version)` follows `InitWebhooks` (`services/service.go:92-111`), constructed after the bus is attached in both control and standalone modes; stopped in `Cleanup` before the webhooks stop block (`:339-345`). Node id `hostname-pid`.

### Event topics
`system.tyk_connection.{activated,disabled,degraded}`, `system.mcp_server.{imported,updated,missing,resumed,published,unpublished,registered,registration_handoff,linked}`, `system.mcp_credential.{minted,drift_applied,drift_pending,external_change,suspended,revoked,rotated}`, `system.mcp_access_grant.{granted,revoked}`. Payloads never carry secrets. The webhooks ingestor forwards them unchanged, which is how a platform team in a `catalogue`-mode org receives handoff packages.

## 7. Flows

### 7.1 Discovery: Dashboard → Studio (all modes)

**Scheduling.** The unit of work is a connection, not a request. A node claims a connection by CAS on (`sync_lease_owner`, `sync_lease_until`, `lock_version`) when `next_sync_at ≤ now`; the lease is `max(sync_interval, estimated run)` and is **heartbeat-extended every 25 requests**; a failed CAS aborts the run before any write. "Sync now" (`POST /api/v1/tyk-connections/:id/sync`, `execute`) sets `next_sync_at = now`, so whichever node polls next runs it (a local `wakeUp` only helps the local node). Multi-node safe by construction; single-node dev needs nothing.

**Run.**
1. `GET /api/mcps?p=0,1,…` until `pages` is reached (`p=-1` is not documented for this endpoint). For each doc: canonicalise, hash (unmasked), upsert `mcp_servers` by (`connection_id`, `info.id`). New rows: `origin=dashboard`, `is_active=false` (unless `auto_publish`), derived fields filled, `definition` stored masked. Changed hash: update derived fields, keep Studio-owned fields, emit `updated`; if `auth_mode`/`brokerable` changed, re-evaluate every live credential on that server (§7.4 drift). Not returned: `dashboard_state=missing`, unpublish, **suspend** its credentials, notify. Same id seen again after `missing`: `resumed`, but only after re-running the brokerable check and drift evaluation; a proxy recreated under a new id is a new asset.
2. `GET /api/portal/policies?p=-1` (documented for this endpoint) → refresh `tyk_policies`; a pinned policy that vanished or no longer covers the server gets `invalid_reason`, the server drops `brokerable`, admins are notified, and affected keys are handled per §7.4 (dangling ids removed; key suspended if it would be left without an access policy).
3. `full` mode: cache `GET /api/apis?p=-1` names in memory for the REST-to-MCP picker, filtered to OAS definitions client-side (the list mixes Classic and OAS).
4. Credential reconciliation (§7.4).
5. Write `mcp_sync_runs`; on transport error mark failed and back off (jittered, capped 30 min) without touching asset state; on 401/403 → `degraded`.

**Admin review → publish**: edit presentation, set `privacy_score`, grant teams, pin a bundle (§8.2), flip `is_active` (`mcp-servers:publish`). Published servers appear in the unified portal catalogue (§9).

### 7.2 Registration: Studio admin → Dashboard (`full` mode only)

Wizard at Admin → MCP Servers → "Register MCP server", two shapes:

- **Remote MCP server**: name, listen path (validated `^/[a-z0-9-/]+/$` and against the synced `listen_path`s; the real create's 400 is the authority since `dryRun` validates schema only), deployment target (§4a, when tags are known), upstream MCP URL, upstream auth (none / static header). The header value is entered in the form, sent in the create call, and **never persisted in Studio**: not on the asset, not in the masked definition. Consumer auth: `authToken` (default), `oauth21` with PRM authorization servers, or keyless with an explicit confirmation. Optional primitive allow-list.
- **REST API to MCP** (only when `rest_to_mcp_supported`): pick a Tyk OAS API, choose operations (from `GET /api/apis/oas/{id}`), per-tool name/description/annotations. Studio builds `x-tyk-mcp-server.primitives[]` and `upstream.url = tyk://<id>/mcp`.

Sequence: build → `POST /api/mcps?dryRun=true&expand=true` (errors surfaced verbatim) → confirm → `POST /api/mcps` → read `ID` (fall back to `Meta`) → `GET /api/mcps/{id}` → upsert `origin=studio` → optional policy create + pin → publish.

Edits: Dashboard is the source of truth. The edit form is offered for `studio`/`submission` origin and, behind an explicit confirmation, for `dashboard` origin. Before `PUT /api/mcps/{id}` Studio re-fetches the live doc, compares its hash with the one the form loaded, refuses on mismatch (`ErrDashboardConflict`, 409, "reload"), and **splices the live `upstream.authentication` block back into the outgoing document** so the masked `***` is never pushed. Deleting a `studio`-origin server revokes its credentials, then `DELETE /api/mcps/{id}`; `dashboard`-origin servers can only be unpublished.

### 7.3 Community registration: Portal → approval → Dashboard

Reuses the submission pipeline with a platform-defined resource type `mcp_server` and schema:

```json
{"type":"object","required":["name","description","kind","connection_id"],
 "properties":{
   "name":{"type":"string"},"description":{"type":"string"},
   "kind":{"enum":["remote","rest_to_mcp"]},
   "connection_id":{"type":"integer"},
   "upstream_url":{"type":"string","format":"uri"},
   "upstream_auth_header_name":{"type":"string"},
   "upstream_auth_token":{"type":"string"},
   "source_api_id":{"type":"string"},
   "consumer_auth":{"enum":["auth_token","oauth21","keyless"]},
   "authorization_servers":{"type":"array","items":{"type":"string","format":"uri"}},
   "transport_notes":{"type":"string"},
   "suggested_listen_path":{"type":"string"},
   "gateway_tags":{"type":"array","items":{"type":"string"}}}}
```

The portal form lists connections whose mode is `full` or that have `accept_handoffs`; the `gateway_tags` control follows §4a (hidden when the connection knows no tags, validated server-side against the known set). The reviewer may change the tags before approval; the handoff package carries them as the requested deployment target. States unchanged. Reviewer actions:

- **Test** (`POST /api/v1/submissions/:id/test`): a Dashboard `dryRun` of the rendered definition only. Studio does **not** contact the submitter-supplied `upstream_url` itself (that would be a reviewer-triggered SSRF against a portal-user-controlled URL); reachability is Tyk's concern once the proxy exists.
- **Approve on a `full` connection** is two-phase because `POST /api/mcps` is not idempotent and runs outside any DB transaction:
  1. Create the proxy (§7.2) and write `tyk_api_id` onto the submission row (`plugin_instance_id`-style column reuse: `external_resource_id`) in its own short transaction.
  2. Run the existing approval transaction, creating the `mcp_servers` row (`origin=submission`, owner = submitter, `community_submitted`, `privacy_score = final_privacy_score`, unpublished unless the reviewer ticks publish).
  A retry after a failure between 1 and 2 finds `external_resource_id` set and skips the create; a retry after a failure inside 1 first looks for a proxy with the same `info.name` + listen path and adopts it before creating.
- **Approve on a `catalogue`/`broker` connection**: create the `mcp_servers` row with `dashboard_state=pending_platform` (no `tyk_api_id`; not publishable, not bindable), emit `registration_handoff`, and offer the **handoff package** download: the ready-to-`POST` OAS definition, the requested policy shape, submitter contact. When the platform team creates the proxy, sync surfaces "Link to imported server" (`POST /mcp-servers/:id/link`, `execute`); Studio suggests candidates by `info.name` equality but never links silently.

Update proposals reuse `is_update`; on approval Studio applies the diff via the §7.2 edit rules.

### 7.4 Credentials: minting, drift, revocation

**Pre-conditions.** Binding a server to an App requires: `is_active`, `dashboard_state=active`, visible to the caller's teams (a new `AccessibleMCPServerQuery` checked in `createUserApp`/update paths, like the handler-side `GetAccessibleLLMs` check in `api/common.go:376-412`; `ErrResourceNotAppGranted` is *not* the right error, it means a plugin type is not app-granted), and the privacy rule `mcp_servers.privacy_score ≤ max LLM privacy score` (extend `validatePrivacyScoresWithPluginResources`). App approval stays the existing credential activation, which also writes `mcp_access_grants` (and every later binding change does too).

**Minting** is per (App, connection), on demand by the App owner after activation (portal App page, one "Get access key" per connection) or by an admin (`mcp-credentials:execute`). Plaintext is never stored: it is returned once. Steps:

1. Insert the ledger row with `status=minting` under the partial unique index; a concurrent second mint fails here before any Dashboard call.
2. Compute `desired_policy_ids` (ordered union of bundles of the App's servers on this connection); validate composition (§8.2); non-brokerable servers are excluded and reported; if none remain, delete the row and return `ErrNotBrokerable`.
3. `POST /api/keys` with `{apply_policies, alias, meta_data:{studio_app_id, studio_app_name, studio_user_id, studio_user_email, studio_connection_id, studio_credential_id, studio_version}, tags:["ai-studio"], expires (epoch, from key_defaults; 0 = never), enable_detailed_recording}`. No inline `access_rights`, no inline limits.
4. On success: store `tyk_key_hash` (and `tyk_key_plain_token` when the response `key_id` is not a hash), `expires_at` from `data.expires`, `applied = desired`, `status=active`, audit, event; return `{key, endpoint docs}` once with `Cache-Control: no-store`. On failure after the Dashboard call returned a key: delete that key, mark the row `failed`.

**Endpoint docs**: per server `endpoint_url`, header to send, an `mcp-remote` snippet; for OAuth 2.1 servers the PRM URL and authorization servers instead of a key; for keyless servers "no credential needed".

**Drift** (`desired ≠ applied`) arises when a bundle is re-pinned, the App gains/loses a server on this connection, or a pinned policy vanishes. Evaluated synchronously on those actions and on every sync:
- **Narrowing**: removing policy ids, swapping a consumption policy, or swapping the *access* policy of a server the key already reaches (one atomic PUT so the key is never left without an ACL). Applied immediately via `GET /api/keys/{hash}?hashed=true` → replace Studio-owned ids, keep `external_policy_ids` → `PUT /api/keys/{hash}?hashed=true`; audit `drift_applied`; owner notified.
- **Widening**: an access policy covering a `tyk_api_id` the key did not reach before. `drift=pending_widen`, admins notified; applied on `POST /mcp-credentials/:id/apply-drift` (`execute`) or immediately when the actor who caused it holds `mcp-credentials:execute`.
- **Vanished pinned policy**: dropped from the PUT body immediately; if the key would then hold no access policy, `is_inactive:true` (`suspended`, reason `no_access_policy`) rather than a key with no ACL.
- **Expiry**: fixed at creation; only via rotate.

**Reconciliation on sync**: `GET /api/keys/{hash}?hashed=true` per live credential. 404 ⇒ `revoked`, reason `deleted_on_dashboard`, grants closed, owner notified. `apply_policies` differs from `applied ∪ external`: ids that belong to a pinned bundle are adopted into `applied`; ids that belong to no bundle are recorded as `external_policy_ids` and reported (`external_change` audit + event) but **never removed**, so Studio and a Dashboard operator do not ping-pong. Drift is then re-evaluated on Studio-owned ids only.

**Suspend / revoke.** App deactivated or credential deactivated → `PUT … is_inactive:true` (`suspended`, reversible; resume re-checks brokerable + drift). App deleted, server unbound from every App on the connection, owner disabled/deleted, admin revoke, connection disabled → `DELETE /api/keys/{hash}?hashed=true`, then **verify with `GET … ?hashed=true` expecting 404**; only then `revoked/deleted`. Any other outcome (400, 200 no-op) ⇒ `PUT is_inactive:true`, `revoke_mode=inactive_only`, and the connection learns `revoke_mode=inactive_only` so the UI says "suspended on Dashboard, delete manually".

**Rotate.** Mint new (steps 1–4, allowed because the old row moves to `status=rotating_out`, excluded from the unique index), then revoke old. No grace window in v1.

## 8. Policies and bundles

### 8.1 Classification on sync
`mcp_api_ids` = `access_rights` keys that are MCP proxy ids on this connection. Partition flags from `partitions`; a policy with no partitions is "all-in-one".

### 8.2 Bundle validation (`ErrInvalidBundle` with reasons)
- Exactly one access pin; it must reference this server's `tyk_api_id` and be non-partitioned or `acl=true`.
- Consumption pins: partitioned with `rate_limit` and/or `quota` (`complexity` allowed, `acl` not). Referencing another proxy is a warning.
- `per_api` cannot be combined with any other pin, and an App whose bundles mix `per_api` with anything else is refused at mint with the servers named. (Rule from the Tyk policy docs, "the Gateway explicitly rejects this combination"; verified against a live 5.13+ Gateway in M4 before the message quotes Tyk.)
- Composition warning when two all-in-one policies are unioned (Tyk takes the most permissive rate/quota); UI copy recommends partitioned bundles.
- `is_inactive` on any pinned policy inactivates every key using it (Tyk ORs them): warning on the pin.

### 8.3 Minimal policy creator (`full` mode, `mcp-servers:execute`)
Creates only partitioned policies:
- **Access policy**: name, this server's `tyk_api_id` in `access_rights` with `versions:["Default"]`, `partitions.acl=true`, `active=true`, `tags:["ai-studio","studio-managed"]`, `meta_data.studio_connection_id`. Per-primitive allow/block lists (`mcp_access_rights`, `json_rpc_methods_access_rights`, `mcp_primitives`) are offered **only after** M4 verifies the Dashboard accepts them (they are documented but absent from the nightly swagger and the 5.14 schema); until then the creator writes plain `access_rights` and links to the Dashboard for primitive-level rules.
- **Consumption policy**: name, `partitions.rate_limit`/`quota`, `rate`/`per`, `quota_max`/`quota_renewal_rate`, optional `key_expires_in`.
`POST /api/portal/policies`, refresh cache, pin in one action. Studio-managed policies may be edited (PUT with the same conflict rule); foreign policies link to the Dashboard.

## 9. Portal, App and admin surfaces

### Portal (`/common`)
- Unified catalogue type `mcp_server`: `GET /common/catalog` items `{type:"mcp_server", attributes:{…, kind, auth_mode, endpoint_url, primitives (names, per-primitive auth), brokerable, access_granted_via_app:true}}`. `api/portal_catalog_query.go`: add an `AccessibleMCPServerQuery` source (team-grant join) to the `UNION ALL`; make `catalogueNameCol`/`catalogueIDCol` optional in `scope()` (`:246-263`) so a source without catalogue membership skips those clauses and `catalog=` filters do not apply to it. Facets: `counts.mcp_server`, kind = `remote|rest_to_mcp`.
- Detail `GET /common/catalog/mcp-servers/:id` (404 when not visible): About, Primitives (with auth badges), How to connect (endpoint, auth mode, OAuth PRM info), Governance, Your apps.
- App builder: "MCP servers" picker; `?mcp_server=<id>` deep link; privacy feedback.
- App page: "MCP access" section grouped by connection: bound servers, credential status, "Get access key" / "Rotate" / "Revoke" per connection, endpoint docs, OAuth/keyless instructions.
- Submission form: "MCP server" type.
- `/common/system.features.feature_tyk_mcp`.

### Admin (`/api/v1`, annotated in `api/api.go`)
| Route | Permission |
|---|---|
| `GET/POST /tyk-connections`, `GET/PATCH/DELETE /tyk-connections/:id` | `tyk-connections` r/w/d |
| `POST /tyk-connections/:id/{activate,disable,probe,sync}` | `tyk-connections:execute` |
| `GET /tyk-connections/:id/policies`, `GET …/apis` | `tyk-connections:read` |
| `POST /tyk-connections/:id/policies`, `PATCH …/policies/:pid` (minimal creator, `full` only) | `mcp-servers:execute` |
| `GET/POST /mcp-servers`, `GET/PATCH/DELETE /mcp-servers/:id` | `mcp-servers` r/w/d |
| `POST /mcp-servers/:id/{activate,deactivate}` | `mcp-servers:publish` (refuses unless `dashboard_state=active` and `privacy_score` set) |
| `PUT /mcp-servers/:id/bundle`, `PUT /mcp-servers/:id/groups` | `mcp-servers:write` |
| `POST /mcp-servers/register` (`?dry_run=1`), `POST /mcp-servers/:id/push`, `POST /mcp-servers/:id/link` | `mcp-servers:execute` |
| `GET /mcp-servers/:id/handoff` | `mcp-servers:read` |
| `GET /mcp-credentials`, `GET /mcp-credentials/:id` | `mcp-credentials:read` |
| `POST /mcp-credentials {app_id, connection_id}`, `POST /mcp-credentials/:id/{rotate,suspend,resume,revoke,apply-drift}` | `mcp-credentials:execute` |
| `GET /mcp-access-report?connection=&server=&user=` (JSON) | `mcp-credentials:read` |
| `GET /tyk-mcp/status` | `authz.AnyAdmin`; 200 `{available:false}` in CE/disabled |

RBAC (`pkg/authz/catalogue.go`, `Actions[0]` must be `read`): `tyk-connections` (Settings, `crudx`, Sensitive, Privileged), `mcp-servers` (Context management, `crudxp`), `mcp-credentials` (AI Portal, `crudx`, Sensitive, Privileged). Frontend: constants in `admin/rbac/permissions.js`, nav in `Drawer.js` (Settings → Tyk Dashboard; Context management → MCP Servers, `exact` on the parent path; AI Portal → MCP Credentials), descriptors in `admin/routes.js`, soft-gated (mounted in CE, page shows the enterprise prompt). Audit action names in `enterprise/features/audit/actions.go` for `register`, `push`, `link`, `apply-drift`.

Admin pages: `TykConnections.js` (+ form with probe panel), `MCPServers.js`, `MCPServerDetail.js` (definition viewer, bundle editor, teams, primitives, credentials using it), `MCPServerRegister.js`, `MCPCredentials.js` (ledger with drift filter, access report). Portal: `AssetDetail.js` `mcp_server` branch + `DETAIL_PATHS`, `CATALOG_TYPES.MCP_SERVER` in `portal/utils/catalog.js`, App builder/detail changes.

## 10. Security

- **URL policy** (extracted from `enterprise/features/webhooks/policy.go` into `pkg/netguard/policy.go` so both features share it; identifiers there are package-private today): http/https only, no userinfo, no fragment, deny internal ranges and `localhost/.local/.internal`, resolved-IP re-check in `DialContext.Control`, `Proxy=nil`, no redirects, re-validated before every call. One `http.Client` per connection carrying that connection's `allow_internal_host` exemption (setting it needs `tyk-connections:execute`, shown as a warning chip). `TYK_MCP_ALLOWED_HOSTS` (exact or `.suffix`) and `TYK_MCP_DENIED_HOSTS` (always wins). Dev stack: `host.docker.internal:3000` via `allow_internal_host`.
- **No Studio-initiated calls to user-supplied MCP upstreams** in v1 (see §7.3 Test).
- **Secrets at rest**: `dashboard_access_token`, `tyk_key_plain_token`, submission `upstream_auth_token` encrypted; hooks refuse to save without `TYK_AI_SECRET_KEY`; column names match the audit redaction fragments and a test proves it. Minted key plaintext never persisted; upstream auth header values never persisted; `definition` stored masked.
- **Redaction in DTOs**: no `dashboard_access_token`; `upstream_url`, `raw` policy JSON and `definition` are admin-only.
- **Least privilege on the Dashboard**: documented user permission recipes per mode; admin secret never accepted.
- **Outbound hygiene**: per-connection rate limit (10 rps default), timeouts (10 s reads, 30 s writes), 8 MB response cap, retries only on idempotent calls, circuit breaker → `degraded`.
- **Portal authorisation**: ownership + team visibility checked server-side on every portal action (portal routes are not role-governed).
- **Approval boundaries**: connection activation, widening drift, registration pushes, policy creation and links are `execute` on Sensitive+Privileged resources; four-eyes optional.

## 11. Configuration

| Env | Default | Purpose |
|---|---|---|
| `TYK_MCP_ENABLED` | true (ENT) | master switch (can only disable) |
| `TYK_MCP_SYNC_MIN_INTERVAL` | 60s | floor for per-connection interval |
| `TYK_MCP_REQUEST_TIMEOUT` | 10s | Dashboard reads (writes 30s) |
| `TYK_MCP_ALLOWED_HOSTS`, `TYK_MCP_DENIED_HOSTS` | empty | URL policy |
| `TYK_MCP_REQUIRE_DIFFERENT_ACTIVATOR` | false | four-eyes on activation |
| `TYK_MCP_SYNC_RUN_RETENTION` | 720h | |

`/common/system` gains `feature_tyk_mcp` = `tykmcp.IsEnterpriseAvailable() && Status().Enabled`.

## 12. Edge cases and failure handling

| Case | Behaviour |
|---|---|
| Dashboard < 5.13 (no `/api/mcps`) | probe `mcp_supported=no`; connection saved, not activatable |
| Dashboard 5.13/5.14 (no `x-tyk-mcp-server`) | REST-to-MCP hidden in wizard and submission form |
| Unhashed keys on the Dashboard | plaintext key id stored encrypted; key ops without `hashed=true` |
| Delete-by-hash unavailable | learned on first revoke by the verify-GET; degrade to `inactive_only` and say so |
| Proxy deleted on Dashboard with live keys | `missing`, unpublished, keys suspended, owners + admins notified; same id reappearing resumes after re-check; new id = new asset |
| Pinned policy deleted on Dashboard | dropped from keys; key suspended if left with no access policy; server not brokerable; admins notified |
| Policy limits edited on Dashboard | nothing to do (Tyk applies dynamically); cache refresh |
| Policy added to a key on the Dashboard | preserved as `external_policy_ids`, reported, never removed |
| App bound to servers on two connections | one credential per connection; App page groups by connection |
| Org mismatch | `degraded`, no writes |
| Connection deleted | needs zero live credentials, or explicit "revoke all N keys"; assets soft-deleted; grants closed |
| App owner leaves (orphaned App) | existing orphan flow suspends; reassignment resumes |
| Two Studio nodes | connection lease + heartbeat + `lock_version` CAS |
| Dashboard 429 | backoff honours `Retry-After`; run marked partial |
| Keyless proxy | catalogued, grant recorded (`keyless`), no key |
| OAuth 2.1 / external OAuth / JWT / mTLS proxy | catalogued, grant recorded, no key; detail page shows PRM / authorization servers / configurable "contact platform" text |
| Mixed schemes | `auth_mode=mixed`; brokerable only if a token or basic scheme is among them |
| Per-primitive `ignoreAuthentication` / scopes | shown per primitive; proxy-level `auth_mode` unchanged |
| `per_api` in a bundle | refused at pin/mint |
| `TYK_AI_SECRET_KEY` unset | service disabled; no writes possible; status says why |
| Community Edition | tables migrate but are unused; status 200 `{available:false}`, other routes 403; nav hidden |
| Gateway config snapshot | MCP servers not shipped to Studio gateways; `app_mcp_servers` not in `AppConfig` |
| Segmented gateways, MDCB unreachable | tags fall back to `known_gateway_tags` plus the tags seen on synced proxies (union, marked "unverified"); the control still renders; `mdcb_read` degraded |
| Proxy tagged for a data plane that no longer exists | admin review warning; portal detail shows the tags unchanged |
| MDCB configured, all data planes untagged | control hidden; definitions created without `gatewayTags` |

## 13. Later phase (design sketch): Studio as MCP client

Goal: chat sessions and agents use a Tyk MCP server's tools under the calling App's identity.
- New Tool type `MCP` referencing an `mcp_servers` row; operations = discovered tools, refreshed by `tools/list`.
- MCP client via `mark3labs/mcp-go` `client` package (the module is a dependency for the server side; the client package is not imported anywhere yet), Streamable HTTP, `Mcp-Session-Id` per chat session.
- Needs a **Studio-held** key per App (`alias …:chat`, stored encrypted, a separate ledger row with `purpose=chat`), distinct from the user-held key v1 never stores. The v1 ledger's `purpose` column is reserved for this.
- Tool filters/guardrails run on arguments and results like REST tools (`scripting.NewFilterRunner`); privacy rule unchanged.

## 14. Follow-ups after v1

- Consumption tiers per asset (named alternative bundles; picker in the App builder; recorded on the binding).
- Discovery probe key (`tools/list` through the proxy to enrich `primitives` for remote servers) — deferred because it is itself a long-lived credential to protect.
- Rotation grace window; CSV export of the access report.
- Auto-link handoff rows by a Dashboard tag convention.
- Catalogue (Catalogs → Teams) membership for MCP servers, if admins prefer it over direct team grants.
- Dashboard MCP analytics (`/api/activity/mcp/*`) on the App usage page.
- Tyk Developer Portal awareness; governed-metadata schema pack for MCP servers.

## 15. Delivery plan

| # | Milestone | Deliverable | Tests |
|---|---|---|---|
| M1 | Contract + connection | `services/tykmcp` contract/stub, `tyk_connections` model + refusing hooks, `pkg/netguard/policy.go` extraction (webhooks switched to it), enterprise client + probe, optional MDCB client (`/dataplanes` extraction that drops `api_key`), admin CRUD/activate/probe routes + RBAC rows, `feature_tyk_mcp`, Connections page incl. MDCB section, known tags and per-tag base URLs | URL policy table; probe classification against `httptest` fake Dashboard and fake MDCB; a test asserting the stored `data_planes` never contains `api_key`; CE 403 vs ENT; encryption round-trip; **audit redaction test for every secret column**; hook refuses without key |
| M2 | Discovery sync | `mcp_servers`, `tyk_policies`, `mcp_sync_runs`, connection lease engine with heartbeat, paging, hashing/masking, missing/resume, MCP Servers admin pages, publish gate, teams | engine tests on the shared-cache SQLite DSN (`file:<test>?mode=memory&cache=shared`, `SetMaxOpenConns(1)`); fake Dashboard fixtures from the scratchpad docs examples; a Postgres run of the sync; two-node lease contention test |
| M3 | Portal asset class | catalogue source with optional catalogue columns, detail endpoint/page, App binding + visibility + privacy rule, `mcp_access_grants` on activate and on update, App page section, governed-metadata object type | `api/portal_catalog_*` tests, App service tests, Jest for `AssetDetail`/builder, Playwright `mcp-portal.spec.ts` |
| M4 | Bundles + broker | pins + validation, insert-first mint, reveal-once, drift engine (narrow/widen/external), suspend/revoke with verify-GET/rotate, reconciliation, ledger UI, access report; **live verification of `per_api` mixing and of the MCP policy ACL fields on a 5.13+ Gateway** | broker tests incl. hashed/unhashed, delete-by-hash unavailable, 404 on reconcile, external ids preserved, concurrent mint; audit records asserted |
| M5 | Registration (admin) | wizard (remote + REST-to-MCP) with the deployment-target control, dry-run, create/edit with hash conflict + upstream-auth splice, delete, minimal policy creator | fake Dashboard asserting request bodies (no `***` ever sent; `gatewayTags` present exactly when chosen); live smoke against `localhost:3000` behind `TYK_MCP_LIVE_TESTS=1` |
| M6 | Community registration | submission type + schema + encrypted field + redaction, review/test (dry-run only)/approve two-phase (full) and handoff (catalogue/broker), link, handoff package + event | `services/submission_*` tests; retry-after-partial-failure tests; webhook delivery of `registration_handoff` in the webhooks E2E harness |
| M7 | Docs + polish | `features/TykMCP.md`, `docs/site/docs/tyk-mcp-integration.md`, Dashboard permission recipes per mode, dev-stack recipe, changelog | docs build; VitePress angle-bracket rule |

Critical files (representative): `services/tykmcp/*`, `enterprise/features/tykmcp/*`, `pkg/netguard/policy.go`, `models/tyk_connection.go`, `models/mcp_server.go`, `models/mcp_credential.go`, `models/app.go`, `models/submission.go`, `services/app.go`, `services/submission_service.go`, `services/governed_metadata/factory.go`, `models/plugin_manifest.go`, `api/api.go`, `api/tykmcp_handlers.go`, `api/portal_catalog_query.go`, `api/portal_catalog_handlers.go`, `api/auth_handlers.go`, `pkg/authz/catalogue.go`, `enterprise/features/audit/{diff,actions}.go`, `main_enterprise.go`, `services/service.go`, `ui/admin-frontend/src/{admin/pages/*, admin/routes.js, admin/components/layout/Drawer.js, admin/rbac/permissions.js, portal/utils/catalog.js, portal/pages/AssetDetail.js, portal/pages/SubmissionForm.js, routes/PortalRoutes.js}`.

Reused: webhooks engine claim/lease shape and backoff (`enterprise/features/webhooks/{engine,backoff,redact}.go`), `secrets.EncryptValue/DecryptValue`, `NotificationService.NotifyDirect`, `eventbridge` bus, submission pipeline, `models.AccessibleXQuery` pattern, `PaginationControls`, `AssetCard`, `PublishSwitch`, `<Can>`/`<RequirePermission>`, `apitest` helpers, the webhooks enterprise API test harness (`api/webhook_handlers_enterprise_test.go`) as the fake-Dashboard template.

## 16. Verification (end-to-end, on the local stack)

1. Dev stack `FRONTEND_PORT=3100 STUDIO_PORT=8090 make dev-ent` (already remapped in this worktree); Tyk Dashboard 5.14 on `:3000`, Gateway EE on `:8080`. Create a connection to `http://host.docker.internal:3000` with the Dashboard user's access key, mode `full`, `allow_internal_host=true`, `gateway_base_url=http://localhost:8080`. Expect probe: `mcp_supported=ok`, `rest_to_mcp_supported=no` (5.14), `mcp_write`/`policies_write`/`keys_write` `unverified`.
2. Register a remote MCP proxy from Studio (a local `mcp-go` example server reachable from the Tyk gateway container); confirm it at `GET localhost:3000/api/mcps` and in Studio as `origin=studio`; capabilities flip to `ok`.
3. Create an access policy + a consumption policy with the creator; pin; set privacy score; publish; grant Default team.
4. As a portal user: find it in Browse, build an App with it and an LLM, get approved, "Get access key" for the connection; connect `npx mcp-remote http://localhost:8080/<listen>/mcp --header "Authorization: <key>"` and run `tools/list`. Confirm on the Dashboard the key carries both policies, `meta_data.studio_app_id`, `tags:["ai-studio"]`, and `expires` matches the ledger.
5. Edit the consumption policy's rate on the Dashboard: no Studio change, limit changes. Re-pin a different consumption policy: key updated automatically, audit record. Bind a second server: `pending_widen` until an admin applies. Add a foreign policy to the key on the Dashboard: preserved and reported.
6. Delete the proxy on the Dashboard: next sync marks `missing`, unpublishes, suspends the key, notifies. Recreate under the same id (PUT with the same `info.id`): resumes. Delete the key on the Dashboard: ledger shows `revoked/deleted_on_dashboard`.
7. Switch the connection to `catalogue`: minting refused (`ErrCapabilityUnavailable`); submit a community MCP server; approve → `pending_platform` row, handoff package, `registration_handoff` webhook delivered to a local receiver; create the proxy on the Dashboard by hand; link it.
8. Segmentation: the local stack has no MDCB, so set `known_gateway_tags=["edge-eu"]` on the connection, register a proxy with that tag, and confirm `gatewayTags {enabled:true, tags:["edge-eu"]}` in the stored definition on the Dashboard and the chip on the portal detail page; the MDCB path is covered by the fake in M1 and by a manual run against a Tyk MDCB stack when one is available.
9. Automated: `go test ./services/tykmcp/... ./api/... -run TykMCP`, `cd enterprise && go test -tags enterprise ./features/tykmcp/...`, Jest for touched components, Playwright `tests/ui/tests/mcp-portal.spec.ts`, live suite behind `TYK_MCP_LIVE_TESTS=1` in the manual comprehensive CI job.

## 17. Resolved questions

1. **Privacy scores are always admin-set.** Imported servers have no score until an administrator assigns one, and `publish` refuses without it. `auto_publish` works only with an administrator-chosen `default_privacy_score` on the connection; there is no platform default. (Decided 2026-09-16.)
2. **The internal `mcp-registry` plugin is for Tyk Technologies' own use only.** It is not a customer feature, needs no deprecation notice, and is not referenced anywhere in customer-facing docs or UI for this integration. (Decided 2026-09-16.)

## 18. Implementation status

| Milestone | State | Notes |
|---|---|---|
| M1 Contract + connection | done | `pkg/netguard` URL policy shared with webhooks; MDCB `/dataplanes` scrape drops `api_key`; audit redaction test per secret column |
| M2 Discovery sync | done | connection lease + heartbeat; masked definition, unmasked hash; missing/resume |
| M3 Portal asset class | done | catalogue source, detail page, App binding with visibility + privacy rule, `mcp_access_grants` on activation and every binding change |
| M4 Bundles + broker | done, one item deferred | insert-first mint, reveal-once (`Cache-Control: no-store`), narrowing applied at once, widening `pending_widen` unless the actor holds `mcp-credentials:execute`, external policy ids preserved and reported, verify-GET revoke with `inactive_only` fallback, rotate, reconciliation, admin ledger + access report (`/admin/mcp-credentials`), portal App page key controls (`POST /common/apps/:id/mcp/credentials`, `…/:cid/{rotate,revoke}`). **Deferred**: the live verification of `per_api` mixing and of the MCP-only policy ACL fields on a 5.13+ Gateway (§8.2, §8.3) has not been run; the minimal policy creator (M5) writes plain `access_rights` until it is. |
| M5 Registration (admin) | done | wizard (remote + REST-to-MCP) with the deployment-target control, `POST /mcp-servers/register?dry_run=1` then create, `POST /mcp-servers/:id/push` with live-hash conflict and generic mask splice (every `***` restored from the live document, refused when it has no counterpart), delete-on-Dashboard for Studio-origin proxies with `?force=true` revoking keys left without access, minimal policy creator (`POST/PATCH /tyk-connections/:id/policies`, partitioned only, tagged `studio-managed`, pin in the same call), source API and operation pickers, `GET /tyk-connections/:id/gateway-tags`. A Studio-registered proxy carries its static upstream token as a global request-header transform; header values whose name looks like a credential are masked like `upstream.authentication`. |
| M6 Community registration | pending | |
| M7 Docs + polish | pending | |

Broker rules that were settled during M4 and are not obvious from the code:

- A policy id Studio once wrote onto a key stays Studio-managed after the policy is un-pinned; only ids Studio never wrote count as external. Otherwise an un-pin would strand the old policy on the key forever.
- A server that comes back from `missing` re-runs drift with `forceApply`, so keys suspended for `no_access_policy` are resumed without an approval round-trip: the access was approved before the outage.
- Policy pins are hard-deleted (`Unscoped`) on re-pin; soft-deleted rows would keep the unique index and refuse the re-pin.
- The Dashboard answers a policy create with the new id in `Message` (not `ID`); the client accepts either.
- OAuth 2.1 proxies built by the wizard carry `components.securitySchemes.oauth21` (`type: oauth2`) plus the vendor scheme's `oauth2.protectedResourceMetadata`, which is what the discovery parser classifies as `oauth21`; the OAS flow URLs point at the first authorization server and are informational.

