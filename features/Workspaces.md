# Workspaces (ENTERPRISE EDITION ONLY)

A **Workspace** is the ownership boundary of objects: every LLM (model provider), App, tool, data source, router, catalogue and other configurable object belongs to exactly one Workspace, and a role can be assigned **within** a Workspace so that it applies only to that Workspace's objects. Together with roles (what) and teams (who), Workspaces (where, and which objects) align AI Studio's access model with the cross-Tyk RBAC model, conceptually rather than literally (see "Relation to the Developer Portal RBAC model" below).

The motivating case is automation: the Tyk Operator's service account gets its own Workspace, which bounds what it can manage and lets it tell when a push to the edges would carry changes it did not make.

Community Edition keeps the tables with every object in the Default workspace; the Workspaces API answers `402` with code `enterprise_required`. An Enterprise build without role-based access control licensed behaves the same way.

Spec of record history: epic `midsommar-clean-oztk` (design field and notes). This file is the long-term spec; keep it current when the model changes.

| Capability | Community | Enterprise |
|---|---|---|
| `workspaces` table, `workspace_id` on 17 object tables, Default workspace | ✅ (everything in Default) | ✅ |
| Create, rename and delete Workspaces (`/api/v1/workspaces`) | ❌ (402) | ✅ |
| Roles assigned within a Workspace (scoped bindings) | ❌ | ✅ |
| Objects hidden or write-protected outside the caller's Workspaces | ❌ | ✅ |
| Pending changes per Workspace, pushes limited to one Workspace | TODO (Slice C) | TODO (Slice C) |

---

## 1. Model

**Workspace** (`models.Workspace`, table `workspaces`): `id`, `name` (unique without regard to case: `name_key` is the trimmed, lower-cased name with a unique index), `description`, `is_default`. Exactly one row is the Default (a partial unique index); it cannot be deleted, and `is_default` is read-only on every endpoint. Ids are never reused (`AUTOINCREMENT` on SQLite, a sequence on Postgres; asserted by a test).

**Ownership stamp.** Seventeen tables carry `workspace_id` (`models.WorkspaceStampedModels`): `llms`, `apps`, `tools`, `datasources`, `filters`, `embedders`, `model_routers`, `semantic_routers`, `agent_configs`, `catalogues`, `data_catalogues`, `tool_catalogues`, `secrets`, `file_stores`, `mcp_servers`, `plugins`, `webhook_targets`. The column is a plain `uint` (`not null default 0`, indexed), with no foreign key. Global configuration is not stamped: users, teams, roles, SSO profiles, branding, tags, model prices, LLM settings, chats, attestation templates, metadata schemas, Tyk connections, edges, marketplace, audit, analytics, logs, traces.

**`workspace_id = 0` is the Default.** Rows written by a build from before Workspaces (a rollback window, or an older replica during an active-active rolling upgrade) carry 0. `models.EffectiveWorkspaceID` reads 0 as the Default; every permission check and list filter applies that rule, in Go and in SQL. `models.BackfillWorkspaceIDs` stamps 0 rows with the Default on every boot (`UpdateColumn`, so `updated_at` is untouched and nothing shows as a pending change).

**The stamp is protected.** An ordinary update never writes `workspace_id` (a `BeforeUpdate` guard on every stamped model drops it unless the statement selects it by name), and object hooks cannot change it (`models.KeepWorkspaceID`). A move writes it explicitly (section 4).

**Naming.** The column is `workspace_id` although LLMs already have `vendor_workspace_id` (an Anthropic workspace); the cross-product vocabulary wins (decision D11). Forms label the vendor field "Vendor workspace (Anthropic)".

### Scope kinds

Every resource of the permission catalogue has a scope kind (`authz.Resource.Scope`, served by `GET /api/v1/rbac/permissions` as `scope`):

| Kind | Resources | Read | Write, delete, execute, publish |
|---|---|---|---|
| `global` (`ScopeGlobal`) | everything not stamped, plus **plugins** and **webhooks** (stamped, but they act on or receive events from every Workspace), plus every plugin-contributed `plugin:*` resource | global binding | global binding |
| `workspace_write` | llms, tools, datasources, filters, embedders, secrets, filestores, mcp-servers | **any** binding granting read, anywhere (D4: read-global) | a binding covering the object's Workspace |
| `workspace_full` | apps, agents, model-routers, semantic-routers, catalogues, data-catalogues, tool-catalogues | a binding covering the object's Workspace, else the object is hidden (404, absent from lists) | a binding covering the object's Workspace |
| `workspace_parent` | credentials, mcp-credentials (`scope_parent: "apps"`) | as the parent App: hidden with it | in the parent App's Workspace; credentials follow their App when it moves |

A binding **covers** a Workspace when it is global, or scoped to that Workspace. `authz.WorkspaceResources()` lists the 17 non-global resources; `authz.StampedResources()` the 15 with their own stamp.

---

## 2. Decisions

Confirmed by Martin on 2026-11-16, with the security review's changes (.36-.41):

| # | Decision |
|---|---|
| D1 | An unscoped (global) binding keeps meaning "every Workspace". Nothing changes for existing deployments. |
| D2 | A Workspace-scoped binding grants only permissions on Workspace resources, inside that Workspace. Global permissions in it (users, roles, edges, plugins, webhooks, plugin:*, ...) are ignored, and a scoped `*` expands to every action of the Workspace resources, never to full admin. The `plugins:execute` umbrella is evaluated on global grants only. |
| D3 | The Owner role can never be bound with a scope (409 `owner_scope_forbidden`). Administrator can; it then means "everything inside that Workspace" and never sets `users.is_admin`. |
| D4 | For read-global (`workspace_write`) resources, read in any scope reads all of them. Reads are metadata: credentials are redacted. |
| D5 | Create with no `workspace_id`: a caller with global write lands in the Default; with scoped write in exactly one Workspace, there; in several, 422 `workspace_required`. Naming a Workspace that does not exist or that the caller's write does not cover gives 422 `invalid_workspace` with the same body in both cases. |
| D6 | Object names stay unique across all Workspaces (a duplicate-name 409 can reveal a hidden object's name; accepted). |
| D7-D9 | Service accounts: see `features/UserManagement.md`. |
| D10 | Managing Workspaces and scoped bindings needs the **global** `workspaces:write` / `roles:write`. Workspace Owners come later. |
| D11 | The column is named `workspace_id` (see above). |

Later rulings (2026-11-16): a move is a pending change attributed to its **destination** Workspace and stains every Workspace-limited push except the destination's; moves write `updated_at`; App credentials are scoped by their App; plugin and webhook stamps are kept but neither enforced nor shown.

---

## 3. Authorization

### Resolving grants

`rbac.Service.ResolveGrants` returns `authz.Grants{Global Set; Workspaces map[uint]Set}`, once per request (`authz.Context`), with no cache: a binding change applies at the next request on every replica.

- Community Edition and an unlicensed Enterprise build: the admin-or-not rule as `Global`, no Workspace grants.
- Enterprise (`enterprise/features/rbac/evaluator.go`): **one query** loads the user's direct and team bindings, global and scoped, and joins `workspaces` on the canonical text id. A scoped row grants something only when `scope_type = 'workspace'`, its `scope_id` is the canonical decimal id (`rbac.WorkspaceScopeID`: no sign, no leading zeros or spaces, never 0) and that Workspace exists. Anything else grants nothing.
- **Global binding** means both scope fields empty (`rbac.IsGlobalBinding` / `GlobalBindingSQL`). Every reader of `role_bindings` uses this predicate: the evaluator, the `is_admin` sync, host roles, the role selectors, the Owner rules and the Owner counts. A row with an empty `scope_type` but a stray `scope_id` is malformed and grants nothing.
- `Resolve`, the `is_admin` self-heal and the legacy admin migration read global bindings only (D3). `SetFullAdmin(off)` removes global Administrator and Owner bindings only.

`authz.Grants` methods: `Can(p)` (global only), `CanIn(ws, p)`, `CanAnywhere(p)`, `WorkspacesWith(p)` (all, or the sorted ids). A Workspace set never satisfies a global permission, whatever it holds. Package-level `authz.Can` stays global-only; use `CanIn` with the object's Workspace for an action on a Workspace object.

### The route gate (fail closed)

`requireRoutePermission` (`api/authz_middleware.go`): a permission passes when held globally. On a route registered **Workspace-checked** (`permRouter.InWorkspace()`), a permission on a Workspace resource also passes when held in any Workspace; that is only the capability gate, and the handler checks each object. Every other route needs the permission globally, so an unconverted route refuses a scoped-only caller. `AnyAdmin` routes pass for anyone holding anything in any scope.

`TestAuthzRoutes_WorkspaceClassification` requires every unmarked route on a Workspace resource to be listed in `api/testdata/workspace_global_only_routes.txt` (the union over both editions); a new route is a conscious decision. Routes on a Workspace resource whose object is global stay unmarked (the Tyk connection policy routes, `mcp-servers:execute` on a global connection).

### Object helpers (`api/authz_workspace.go`, `api/workspace_writes.go`)

Every helper reads the object's stored `workspace_id` from the database (never a per-replica cache) and applies the 0-is-Default rule:

| Helper | Use |
|---|---|
| `requireVisible(c, resource, ws, notFound)` | read-scoped kinds: 404 with the handler's own not-found body, so a hidden object looks missing |
| `requireInWorkspace(c, perm, ws)` / `holdsIn` | an action on the object: 403 `permission_denied` |
| `workspaceListScope` / `workspaceFilterParam` | lists: only the caller's Workspaces (read-scoped kinds); `?workspace_id=` narrows any stamped list, never widens it |
| `resolveCreateWorkspace` / `resolveCreateFromBody` | the landing rule (D5) |
| `requireMove`, `checkObjectUpdate`, `moveObject` | moves (section 4) |
| `requirePublishIfChangedIn` / `requirePublishToCreateLiveIn` | the live switch, in the object's Workspace (publish in W1 never releases a W2 object) |

The helpers allow everything in TestMode and for a legacy caller outside `/api/v1` with no authorization context. A request that passed the gate, or any `/api/v1` request, without a context fails closed. In Community Edition grants are global only, so every check reduces to admin-or-not.

The rules below marked **(B4)** land with the per-resource work (.7-.13) and are described as designed.

**Inserts lock their Workspace (B4).** Every stamped model's `BeforeCreate` takes the Workspace row `FOR SHARE` (Postgres) inside the create's transaction (`wsstamp`), so an object never lands in a Workspace a concurrent delete is removing (422 `invalid_workspace` then). Never create stamped rows on a `SkipDefaultTransaction` session.

### Credential material (B4)

Read-global reads are metadata. A route that returns raw credential material of an object needs **write** in the object's Workspace: an LLM's `config_override`, an agent's plugin configuration (`agents:write`), and so on. The configuration of an embedded plugin is shown only to holders of global `plugins:read` (the same rule as `GET /plugins`). Secret values are never returned.

### Secrets (B4)

- A `$SECRET/<name>` reference in an object must name a secret of the object's own Workspace (no Default fallback). An unknown name answers 403, so names in other Workspaces cannot be probed.
- Secret names are unique across Workspaces.
- A secret cannot be deleted or moved while an object references it.
- Moving an object re-checks its references (422 if any would point outside the destination).
- Cloning an object across Workspaces is refused (403).

---

## 4. Moves (B4)

A move is a `PATCH` (or `PUT`) of the object with a different `workspace_id`:

- The caller's write must cover **both** Workspaces (`requireMove`); the destination must exist (422 `invalid_workspace`, the same body when it is missing or not covered).
- The move writes `workspace_id` and `updated_at` together, guarded on the stored value (a concurrent move answers 409), so it shows as a **pending change** and stains pushes (section 6).
- A move is attributed to its **destination** Workspace; no record of the previous Workspace is kept.
- App credentials follow their App; nothing else moves with an object.

---

## 5. Workspaces API

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/api/v1/workspaces` | any admin-surface user | Default first, then by name. Every Workspace for a caller whose access covers them all (global `workspaces:read`, or any global permission on a Workspace resource); otherwise the Workspaces the caller holds a scoped role in (filtered in the query) |
| POST | `/api/v1/workspaces` | `workspaces:write` | `{name, description}`; 409 on a duplicate name |
| GET / PATCH | `/api/v1/workspaces/:id` | `workspaces:read` / `workspaces:write` | `is_default` is read-only |
| DELETE | `/api/v1/workspaces/:id` | `workspaces:delete` | 409 `default_workspace_locked`; 409 `workspace_in_use` while a binding is scoped to it; its objects move to the Default |
| GET | `/api/v1/workspaces/:id/objects` | `workspaces:read` | counts per resource (a 0 row counts for the Default; plugins and webhooks not counted) |
| POST | `/api/v1/rbac/bindings` | `roles:write` (global) | `scope_type` (`""` or `"workspace"`) and `scope_id` (canonical id); 422 `invalid_workspace` for anything else, 409 `owner_scope_forbidden` for the Owner |
| GET | `/api/v1/rbac/bindings` | `roles:read` | filters `scope_type=global|workspace`, `scope_id` |
| GET | `/api/v1/rbac/me`, `/rbac/users/:id/effective`, `/common/me` | unchanged | add `workspaces: [{id, name, permissions, roles?, sources?}]` |

**Delete** (`enterprise/features/workspaces`) runs in one transaction that locks the Workspace row `FOR UPDATE`, re-checks bindings, moves every object (soft-deleted ones too) to the Default with `workspace_id` and `updated_at` written together (moves stain), then deletes the row. `CreateBinding` and the insert hook read the row `FOR SHARE`, so across replicas exactly one of a delete and a racing binding or object create succeeds (Postgres race test, run in the enterprise CI Postgres job).

**`has_admin_access` vs `admin_nav_hint`.** `has_admin_access` (in `/rbac/me`, the effective views and `/common/me`) keeps its meaning: the user holds some permission **globally**. `admin_nav_hint` is true when the user holds anything, globally or within a Workspace, so the UI shows a scoped-only user the admin navigation. It is a UI hint only: it is not `users.is_admin`, and nothing on the server authorizes on it.

The audit trail records Workspace create, update and delete with field diffs (`workspaces` in `snapshotTargets`).

---

## 6. Per-resource enforcement

TODO (Slice B4: .7-.13). Per resource: create (landing rule), get (visibility), update/delete/publish/execute (in the object's Workspace), lists, moves, link-replacing saves that keep links the caller cannot see (team ↔ catalogues, catalogue ↔ routers, agent ↔ App), bulk actions, dependents, governed-metadata object routes, submission approval, and the list of exempt runtime, portal and edge paths.

### Exempt paths (B4g)

Every `/api/v1` route on a Workspace resource is either Workspace-checked (`permRouter.InWorkspace()` plus the object helpers) or listed in `api/testdata/workspace_global_only_routes.txt`, where the route gate requires the permission globally (fails closed for scoped callers). These paths read stamped tables without a Workspace check, on purpose:

| Path | Where | Why |
|------|-------|-----|
| Portal (`/common`) | `api/common.go` (catalogue LLMs, data catalogue datasources, tool catalogue tools, accessible LLMs and datasources, user Apps, chat history, App usage), `api/portal_catalog_handlers.go`, `api/nav.go` (agents in the nav), the `/common` agent list, get, stream and message (`api/agent_handler.go` off the admin API), `api/agent_v2_handlers.go` | Portal access is team membership (team ↔ catalogue, team ↔ agent); portal users hold no bindings. Portal-created Apps land in the Default. |
| Runtime (data plane) | `proxy/`, the gateway and microgateway config sync (gRPC), chat sessions (`api/chat_session_handler.go`), agent sessions | Workspaces govern the admin API, not request serving. |
| Edge | `api/edge_owner.go`, edge and namespace handlers | Edges and namespaces are not Workspace resources. |
| Analytics | `api/analytics_handlers.go`, `mcp_analytics_handlers.go`, service-tier and decision analytics, guardrail block reports, `compliance_handlers.go` | Analytics stays a global resource in this release (§10). |
| Other global resources | webhooks and webhook deliveries; the audit trail; agent traces; proxy logs and their exports; pending changes (redacted per §7, .44) | Global resources, gated by their global permission at the route. |
| Plugins | `plugin_handlers.go`, `plugin_workflow_handlers.go`, `plugin_upgrade_handlers.go`, `plugin_marketplace_info.go` | Plugins are global; the `plugins` row is stamped only for pushes. |
| Access management | `rbac_payloads.go`, `group_handlers_community.go`, `app_quota_handlers.go`, `user_handlers.go`, `license_handlers.go` | Global; not Workspace resources. |
| Read-global links (D4) | e.g. `toolCatalogueMCPServers` in `api/tool_catalogue_handlers.go` | LLMs, tools, datasources, filters, embedders and MCP servers are read-global, so they never count as hidden in link merges or dependents. The catalogue itself is checked (`onVisibleObject`). |
| Governed metadata on other object types | `mcp_server`, `plugin_resource:*` | They resolve to the global `metadata` permission at the route gate (fails closed for scoped callers). |

## 7. Pending changes and stained pushes

TODO (Slice C: .14, .15). Each pending change reports its Workspace; `GET /api/v1/sync/pending-changes?workspace_id=` adds `outside_total` and `stained`; pushes accept `{"only_workspace_id": W}` and answer 409 `push_stained`. Redaction (.44): for read-scoped types outside the caller's read, ids and names are left out; Workspace names show only where the caller has a binding or global `workspaces:read`; counts are always shown (the same in the `push_stained` body). Moves are attributed to their destination; plugin changes count as foreign; OAuth and access-token artifacts never do; an unflagged source counts as foreign (fail closed).

---

## 8. Upgrade, rollback and replicas

- **Upgrade:** every object lands in the Default, every binding is global (D1); no user sees or loses anything, and no object shows as pending because of the upgrade.
- **Rollback:** the old binary ignores `workspace_id` and its evaluator reads `scope_type = ''` only, so scoped bindings grant nothing (fails safe). A service account keeps working there through its API key, without the session and portal restrictions; note it in the release notes.
- **Roll forward / rolling upgrade:** rows an older replica writes carry 0 and read as the Default until the next boot's backfill.
- **Replicas:** no permission cache; Workspace delete serializes against binding and object creation; the Default workspace id is cached per process (it never changes).

## 9. Relation to the Developer Portal RBAC model

Studio follows the Portal's Phase 3 RBAC model (Role = what, Workspace = where, User Group = who) conceptually. Deliberate differences:

- A global binding covers every Workspace (D1); the Portal reaches every Workspace only as Super Admin.
- Scope is a property of the **binding**, not the role; any role can be bound globally or on a Workspace, and only its Workspace permissions count there (D2), where the Portal's scoped roles carry global keys portal-wide.
- Building blocks are read-global (D4); every Portal scoped key is read-scoped.
- Ambiguous creates answer 422 `workspace_required` instead of landing in the Default (D5).
- Webhooks and plugins are global.
- Studio has five actions instead of ordered levels, several roles per person per Workspace (union), service accounts, and stained pushes.

An internal comparison with interop notes is kept by the security role for the Operator and future shared-model work.

## 10. Deferred and accepted risks

**Accepted residual risks** (security review, 2026-11-16):

- **File store content is read-global (D4).** A caller with `filestores:read` in any Workspace can read the content of every file store. Do not put secrets or confidential data in file stores; the user docs say so.
- **Tyk connections are global.** A caller who may execute MCP-server actions in one Workspace (register, push, link, discover) can use any Tyk connection, and the Dashboard credentials it holds. Tying connections to Workspaces is a later design item.
- **Linked building blocks.** An App can link another Workspace's read-global building blocks and use the credentials they hold at runtime (until the `use` action below).

**Deferred**, in intended order: Workspace Owners (a scoped `workspaces:write`); escalation and protection rules (no self-escalation, a role in use cannot be deleted); SSO membership provenance; the vocabulary pass (Team → User Group); a `use` action and ID-and-name link pickers (read scoping for building blocks); Workspace scoping of analytics, logs, traces and chat history; Tyk connections owned by Workspaces (so MCP actions in one Workspace use only its connections).

## 11. Tests

- `pkg/authz/grants_test.go` — scope kinds of every resource, `Grants` semantics, D2, the umbrella, the scoped wildcard, parent-scoped credentials.
- `services/rbac/scope_test.go` — canonical scope ids, the global predicate.
- `enterprise/features/rbac/scoped_test.go` — one query, team bindings, D2/D3, validation, malformed rows, effective permissions per Workspace, unlicensed, `SetFullAdmin(off)`.
- `api/authz_workspace_test.go`, `api/authz_workspace_routes_test.go` — helpers, the gate, the classification registry.
- `enterprise/features/workspaces/service_test.go` — CRUD, delete (moves, `updated_at`), counts, and the Postgres delete-vs-binding race.
- `enterprise/_coretests/api/workspace_api_entlink_test.go` — the API, visibility, per-Workspace access, audit; Community: `api/workspace_handlers_community_test.go` (402).
- TODO: the B4 per-resource matrices and the Slice C tests.
