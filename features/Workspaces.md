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
| Pending changes report each change's Workspace; pushes limited to one Workspace (409 `push_stained`) | ✅ (every change is in the Default) | ✅ |

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

**Inserts lock their Workspace.** Every stamped model's `BeforeCreate` takes the Workspace row `FOR SHARE` (Postgres) inside the create's transaction (`wsstamp`), so an object never lands in a Workspace a concurrent delete is removing (422 `invalid_workspace` then). Never create stamped rows on a `SkipDefaultTransaction` session.

### Credential material

Read-global reads are metadata. A route that returns raw credential material of an object needs **write** in the object's Workspace: an LLM's `config_override` and resource-signing key, an agent's configuration (`agents:write`), and so on. The upstream credential of an MCP server (`GET /mcp-servers/:id/handoff?include_secrets=true`) needs `mcp-servers:execute` in the server's Workspace; the handoff without secrets is read-global. The configuration of an embedded plugin is shown only to holders of global `plugins:read` (the same rule as `GET /plugins`). Secret values are never returned.

### Secrets

- A `$SECRET/<name>` reference in an object must name a secret of the object's own Workspace (no Default fallback). An unknown name answers 403, so names in other Workspaces cannot be probed.
- Secret names are unique among live secrets across Workspaces: exact and case-sensitive, under a per-name lock.
- A secret that objects reference cannot be deleted or renamed (409) and does not move (422).
- Creating or renaming a secret to a name that objects outside the new secret's Workspace reference is refused (409), so a dangling reference never picks up another Workspace's secret.
- Grandfathered duplicate names (from before uniqueness) resolve to the lowest id within one Workspace, are refused (unresolved) across Workspaces, and a duplicate cannot move (409).
- Moving an object re-checks its references (422 if any would point outside the destination).
- Cloning a datasource needs write in the source's Workspace; the copy lands by D5 (a global writer's clone lands in the Default), and its secret references must be allowed in the landing Workspace.

---

## 4. Moves

A move is a `PATCH` (or `PUT`) of the object with a different `workspace_id`:

- The caller's write must cover **both** Workspaces (`requireMove`); the destination must exist (422 `invalid_workspace`, the same body when it is missing or not covered).
- The move writes `workspace_id` and `updated_at` together, guarded on the stored value (a concurrent move answers 409), so it shows as a **pending change** and stains pushes (section 7).
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

Every `/api/v1` route on a Workspace resource follows the pattern of section 3: the route is registered Workspace-checked, the handler (or route middleware such as `onObject` / `onVisibleObject`) checks each object it touches, creates land by D5, and lists show only what the caller may read. A hidden object and a missing one answer with byte-identical 404s on every route (no existence oracle).

| Resources | Kind | Notes |
|---|---|---|
| LLMs, tools, datasources, filters, embedders, secrets (.7) | `workspace_write` | Reads are read-global (D4) with credentials redacted. Write, delete, publish and execute need the permission in the object's Workspace. Raw credential material (an LLM's `config_override`, its resource-signing key) needs write in the object's Workspace. `$SECRET/` references, secret moves and datasource clones follow section 3 "Secrets". |
| File stores, MCP servers (.8) | `workspace_write` | As above. Push and link (and the per-server `CanExecute`) are `mcp-servers:execute` in the server's Workspace. Register creates a server, so it is execute at the route plus write in the landing Workspace (D5). Discover has no object: execute in any scope. The upstream credential in the handoff needs execute in the server's Workspace (see "Credential material"). The Tyk connection these actions use is global (accepted risk, section 10). |
| Plugins, webhook targets (.8) | `global` | Stamped for pushes only; the stamp is neither enforced nor shown. |
| Apps and their credentials (.9) | `workspace_full`, `workspace_parent` | Every `/apps/:id` route checks the App (`onObject`, the handlers' own "App not found"). Publishing and binding MCP servers are checked in the App's Workspace. Router grants (.49): every router named must be visible to the caller (404 like a missing one); the "may grant any active router" path is for administrators and global `apps:write` only, so a writer scoped to the App's Workspace goes through the team-catalogue rule; stored grants to hidden routers are kept on update. Linking LLMs, tools and datasources needs read on them in some scope. API and MCP credentials are checked in their App's Workspace; a credential held by no live App, or by more than one, is reachable only with the permission held globally (404 otherwise). Minting an MCP credential needs `mcp-credentials:execute` in the App's Workspace. Credential lists and the MCP access report are limited to visible Apps. Portal-created Apps land in the Default. |
| Agents (.10) | `workspace_full` | On the admin API, agent lists and reads are scoped by Workspace, not by team. The agent's App must be visible (422 otherwise). The agent configuration is shown only with `agents:write` in its Workspace, an embedded plugin configuration only with global `plugins:read`. Changing an agent's teams (`group_ids`) needs the global `groups:write` (403, .48). The plugin must be enabled and agent-capable (422). The run routes check visibility, then `agents:execute` in the agent's Workspace. `/common` agent routes are unchanged (portal, team access). |
| Model routers, semantic routers (.11) | `workspace_full` | Router ↔ catalogue saves keep links to catalogues the caller cannot see; router responses leave out the names of hidden catalogues. Pool and target LLMs are read-global. |
| LLM, data and tool catalogues (.12) | `workspace_full` | Team ↔ catalogue saves keep links to hidden catalogues. Team responses list only visible catalogues; the team catalogue lists filter in the query, so totals match the page. Catalogue members (LLMs, datasources, tools) are read-global. |

**Link-replacing saves.** A save that replaces a set of links (team ↔ catalogues, catalogue ↔ routers, router ↔ catalogues, App ↔ routers) validates the submitted ids the caller can see (404 for one they cannot, the same as a missing one) and keeps the stored links the caller cannot see (`mergeVisibleLinks`). Linking needs write on the object being edited and read on the linked object. Links to read-global objects are never hidden.

### Cross-cutting paths (B4g, .13)

- **Bulk actions** (`POST /{llms,tools,datasources,apps,filters,secrets,model-routers}/bulk`): before anything changes, every id is checked with the action's permission (publish for activate/deactivate, delete for delete) in its Workspace, in one query. The first **visible** object the caller cannot act on refuses the whole request with 403, and nothing changes. An object the caller may not see answers exactly like a missing id: a per-id "not found" in the 200, left untouched.
- **Dependents** (`GET /{resource}/:id/dependents`): Apps, catalogues, agents and routers the caller may not see are left out of the lists and counted in `hidden`; `total` counts every dependent. No ids or names of hidden dependents are returned.
- **Governed metadata on objects** (`/metadata/objects/{llm,tool,datasource}/:id`): writes need the object's write permission in its Workspace (403); a non-numeric id is a 400. Reads stay read-global. Other object types (`mcp_server`, `plugin_resource:*`) need the global `metadata` permission.
- **Submission approval** (UGC): a new tool, datasource or MCP server lands by the reviewer's landing rule, or in the `workspace_id` they name (422 when their write does not cover it). Approving is the global `submissions:write`, so a reviewer who holds no write on the resource anywhere approves into the Default. Update submissions change the object where it is. Assigning catalogues needs write in each catalogue's Workspace: a missing or hidden catalogue answers the same 404, a visible one they cannot write 403; nothing is created. See `features/UGC.md`.

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

Configuration reaches edges by **push** (`POST /edges/:edge_id/reload`, `/namespaces/:namespace/reload`, `/edges/reload-all`, global `edges:execute`). A push delivers a whole namespace: Workspaces do not split what an edge loads. What Slice C adds is visibility of whose changes a push carries, and a way for an automated caller (the Tyk Operator) to refuse a push that would carry someone else's.

### Pending changes (C1, .14)

`GET /api/v1/sync/pending-changes?namespace=` (global `edges:read`) lists what changed since the namespace's last push.
- **Per change:** `workspace_id` and `workspace_name`, for stamped types (LLM, App, filter, tool, datasource, model and semantic router, embedder; a stored 0 reads as the Default). Plugins, model prices, OAuth clients and access tokens report none.
- **Per response:** `by_workspace` (`{workspace_id, name, total}` over every pending change, not only the listed ones) and `unattributed_total`.
- **With `?workspace_id=W`:** `outside_total` counts the changes foreign to W and `stained` says whether there are any, i.e. whether a push limited to W would carry changes from outside it.

**Foreign** (`services.PendingChangeIsForeign`):
- A change on a stamped type is foreign to W when another Workspace owns it.
- Unstamped types (plugins, model prices) are foreign to every W, since they're global configuration.
- OAuth clients and access tokens never stain (runtime artifacts with no owner).
- An unknown type stains (fail closed).

**Moves** bump `updated_at` and are attributed to their **destination**. So a move stains every Workspace-limited push except the destination's, and no record of the source is kept.

**Redaction** (.44, `api/pending_changes_redaction.go`), for a Workspace-scoped caller:
- The id and name of a change on a read-scoped type (Apps, routers) in a Workspace their read does not cover are left out (`hidden: true`); its type, kind, time and Workspace id remain.
- Workspace names show only where the caller holds a binding, or for a caller whose access covers every Workspace (global `workspaces:read`, full admin, or a global read on a Workspace object type: `mayNameWorkspace`).
- Read-global and unstamped types keep their names (D4).
- Counts are always shown, an accepted exception (§10).

The push dialog (E6) shows each change's Workspace and the per-Workspace summary.

### Push precondition (C2, .15)

The three push routes take an optional body `{"only_workspace_id": W}`.
- **A malformed body** answers 400. **A Workspace that does not exist** answers 422 `invalid_workspace`.
- **The check:** once the targets are resolved and before anything is recorded, the push is refused if any target namespace has a change pending from outside W. The answer is 409 `push_stained`, carrying:
  - `workspace_id`, `outside_total`, `by_workspace` (W excluded) and `unattributed_total`;
  - up to 20 foreign changes, newest first, each with its namespace, redacted as in the preview.
- **A refused push records nothing:** no operation, command or `last_push_at`.
- **Without the body** a push behaves as before.

Studio only refuses when asked. The Operator decides what to do with a stained push (wait, or push anyway without the precondition). The check runs on the replica that accepts the request. A change that lands after it still ships with the snapshot the edges pull, and a push still delivers the whole namespace. See `features/ClusterControlPlane.md` (push model).

**Operator recipe:** a service account with a global role holding `edges:read` and `edges:execute`, plus a Workspace-scoped role on its own Workspace. It reads `pending-changes?workspace_id=W` and pushes with `only_workspace_id: W`, so it never ships another team's half-finished change.

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
- **Names across Workspaces (D6).** Object names stay unique across Workspaces, so a 409 on create can reveal that a hidden object has that name.
- **Counts of hidden objects.** Dependents' `total` and the per-Workspace counts in pending changes include objects the caller cannot see (counts only, never ids or names).
- **Linked building blocks.** An App can link another Workspace's read-global building blocks and use the credentials they hold at runtime (until the `use` action below).

**Deferred**, in intended order: Workspace Owners (a scoped `workspaces:write`); escalation and protection rules (no self-escalation, a role in use cannot be deleted); SSO membership provenance; the vocabulary pass (Team → User Group); a `use` action and ID-and-name link pickers (read scoping for building blocks); Workspace scoping of analytics, logs, traces and chat history; Tyk connections owned by Workspaces (so MCP actions in one Workspace use only its connections).

## 11. Tests

- `pkg/authz/grants_test.go` — scope kinds of every resource, `Grants` semantics, D2, the umbrella, the scoped wildcard, parent-scoped credentials.
- `services/rbac/scope_test.go` — canonical scope ids, the global predicate.
- `enterprise/features/rbac/scoped_test.go` — one query, team bindings, D2/D3, validation, malformed rows, effective permissions per Workspace, unlicensed, `SetFullAdmin(off)`.
- `api/authz_workspace_test.go`, `api/authz_workspace_routes_test.go` — helpers, the gate, the classification registry.
- `enterprise/features/workspaces/service_test.go` — CRUD, delete (moves, `updated_at`), counts, and the Postgres delete-vs-binding race.
- `enterprise/_coretests/api/workspace_api_entlink_test.go` — the API, visibility, per-Workspace access, audit; Community: `api/workspace_handlers_community_test.go` (402).
- B4 per-resource (Enterprise, real middleware chain): `api/workspace_b4_enterprise_test.go`, `api/llm_workspace_test.go` (.7: write-scoped building blocks, secrets, clone), `api/workspace_b4b_enterprise_test.go` (.8: file stores, MCP servers), `api/workspace_b4c_enterprise_test.go` (.9: Apps and credentials), and in `enterprise/_coretests/api/`: `workspace_agents_entlink_test.go` (.10), `workspace_routers_entlink_test.go` (.11), `workspace_catalogues_entlink_test.go` (.12), `workspace_crosscutting_entlink_test.go` (.13: bulk, dependents, metadata objects, submission approval, hidden/missing parity).
- Slice C:
  - `services/pending_changes_workspace_test.go`: attribution, the stain rule, moves, the totals.
  - `api/sync_pending_changes_test.go`, `api/pending_changes_redaction_test.go` and `api/pending_changes_redaction_enterprise_test.go`: `?workspace_id=`, redaction, Workspace names.
  - `api/push_precondition_enterprise_test.go` and `services/pushes/pushes_test.go`: 409 `push_stained`, its body, nothing recorded, 400/422.
  - UI: `components/edge-gateways/PendingChangesWorkspaces.test.js`.
