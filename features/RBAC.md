# Role-Based Access Control (ENTERPRISE EDITION ONLY)

**⚠️ ENTERPRISE EDITION ONLY**

Fine-grained roles are an Enterprise Edition feature. Community Edition keeps the historical model: a user is either an administrator (full access to the management UI and API) or not. The role management endpoints return `402 Payment Required` in Community Edition and the Roles page shows an upgrade prompt.

## Edition Comparison

| Capability | Community | Enterprise |
|---|---|---|
| Admin-or-not access to the management UI and API | ✅ | ✅ (as the Owner / Administrator roles) |
| Built-in roles: Owner, Administrator, Editor, Viewer, Auditor | ❌ | ✅ |
| Custom roles built from the permission catalogue | ❌ | ✅ |
| Roles assigned to users and to teams (groups) | ❌ | ✅ |
| Navigation, pages and buttons follow the caller's permissions | ❌ (admin sees everything) | ✅ |
| Permission denials recorded in the audit trail | ❌ | ✅ |
| Permission catalogue endpoint (`GET /api/v1/rbac/permissions`) | ✅ (read-only, `enabled: false`) | ✅ |

User documentation: [`docs/site/docs/rbac.md`](../docs/site/docs/rbac.md). Framework notes: `ENTERPRISE_FRAMEWORK.md` → "Role-Based Access Control Feature Specifics".

---

## 1. Model

**Permission** — a string `<resource>:<action>`. Resources are the plural, kebab-case route collection segments of the management API (`llms`, `data-catalogues`, `sso-profiles`). Actions are exactly five:

| Action | Meaning | HTTP mapping |
|---|---|---|
| `read` | list, get, search, status, history, download an existing artefact | `GET` |
| `write` | create, update, link sub-resources, approve/reject, rollback | `POST`, `PUT`, `PATCH` |
| `delete` | remove a resource or a sub-resource link | `DELETE` |
| `execute` | side-effecting operations that do not persist configuration: test, call, reload, sync, re-process | `POST` on a verb route |
| `publish` | make an object live: set an LLM, tool, data source, app or agent active, enable a plugin, activate a metadata schema | `POST /<resource>/:id/activate|deactivate` (`enable|disable` for plugins, `PATCH /model-routers/:id/toggle`), plus the live switch inside `PATCH`/`PUT` |

`write`, `delete`, `execute` and `publish` each imply `read`. `publish` does **not** imply `write` and `write` does not imply `publish`: the two are orthogonal so that a *submitter* role (`llms:write`) can draft and edit providers without releasing them, a *reviewer* role (`llms:write` + `llms:publish`) can do both, and an *approver* role (`llms:publish` only) can release without editing. There are no deny rules. The single wildcard `*` is held only by the Owner and Administrator system roles.

### The publish rule

Only resources with a live switch offer `publish`: `llms` (`active`), `tools` (`active`), `datasources` (`active`), `apps` (`is_active`), `agents` (`is_active`), `model-routers` (`active`), `plugins` (`is_active`) and `metadata` (schema `active`). `authz.Publishable()` lists them. Enforcement (`api/authz_publish.go`):

- `PATCH`/`PUT` routes keep their `write` annotation. The handler loads the stored object and calls `requirePublishIfChanged(c, resource, before, after)`: a request that flips the switch without `publish` is answered `403 permission_denied` with `"permission": "<resource>:publish"`; a request that leaves the switch alone (or omits it) is plain write, so editing an already-live object never needs `publish`.
- Create routes call `requirePublishToCreateLive`: asking for an active object explicitly needs `publish`. Where the column defaults to live (tools, apps, agents, metadata schemas) a caller **without** `publish` who does not mention the switch gets a draft (inactive) object rather than an error.
- Every publishable resource has a dedicated route annotated `publish` only (`POST /llms/:id/activate`, `/deactivate`, … ; `POST /plugins/:id/enable|disable`; `PATCH /model-routers/:id/toggle`; `POST /metadata/schemas/:id/activate|deactivate`; the existing `POST /agents/:id/activate|deactivate`), backed by `services.Set*Active`, which flips only that column and fires object hooks and system events like an update. `POST /apps/:id/activate-credential` is about the credential and stays `write`.
- `ToolInput.active` and `AppInput.is_active` are new optional (`*bool`) attributes; until this change neither flag had an admin write path.
- Submission approval creates the object with the submitted `active` value under `submissions:write`; reviewing submissions is its own workflow.

Editor holds `publish` on everything it can write except `plugins` (enabling a plugin stays with administrators, like installing one) and `metadata` (read-only for Editor). Viewer and Auditor never publish.

**Role** — a named bundle of permissions (`models.Role`). System roles are immutable and recomputed from the catalogue on every boot; custom roles hold an explicit list and never gain permissions automatically.

**Binding** — a role assigned to a subject (`models.RoleBinding`): `subject_type` is `user` or `group`, `scope_type`/`scope_id` are reserved (empty string = global). A user's effective permissions are the union of their direct bindings and the bindings of every team they belong to. SSO group mapping therefore drives roles without extra configuration.

### The catalogue

The catalogue lives in core (`pkg/authz/catalogue.go`) so both editions share one source of truth, routes can be annotated with typed permissions, and the UI renders the role editor straight from `GET /api/v1/rbac/permissions`. Resources are grouped to match the admin navigation:

| Group | Resources (S = sensitive data class) |
|---|---|
| Analytics | `analytics`, `proxy-logs` (S) |
| Plugins | `plugins`, `marketplace` |
| LLM management | `llms`, `model-prices`, `model-routers` |
| Context management | `datasources`, `tools`, `filters`, `filestores`, `tags` |
| Community | `submissions`, `attestation-templates` |
| Access | `users`, `groups`, `roles`, `sso-profiles` (S) |
| Governance | `audit` (S), `compliance`, `metadata`, `exports` (S) |
| Settings | `secrets`, `branding` |
| AI Portal | `apps`, `credentials` (S), `edges` |
| Chat | `chats`, `agents`, `llm-settings`, `chat-history` (S) |
| Catalogs | `catalogues`, `data-catalogues`, `tool-catalogues` |

Sensitive resources are split out so `read` on them can be withheld independently (a Viewer never reads transcripts, proxy logs, audit records, exports, credentials or identity provider secrets). Privileged resources (`users`, `groups`, `roles`, `sso-profiles`, `plugins`) are flagged because write access to them can grant access to other people; the role editor warns on them.

Governed metadata on an object is authorised by that object's permission: `PUT /metadata/objects/llm/:id` needs `llms:write`, not `metadata:write` (`api/authz_routes.go` → `metadataObjectPermission`). The one GET that writes, `GET /model-prices/by-name` (get-or-create), is annotated `model-prices:write`.

### System roles

| Slug | Name | Permissions |
|---|---|---|
| `owner` | Owner | `*`. Seeded to user ID 1. User-only binding. Only an Owner may grant or revoke Owner. The last Owner cannot be removed or demoted. |
| `administrator` | Administrator | `*`. Bindable to users and teams. Cannot bind/unbind Owner. |
| `editor` | Editor | Everything except: `users`/`groups` beyond read, `roles`, `sso-profiles`, `plugins`/`marketplace` write, delete and publish, `metadata` write, delete and publish, `audit`, `exports`, `proxy-logs`, `chat-history`. Includes `credentials:*`, `secrets:*` and `publish` on LLMs, tools, data sources, apps, agents and model routers. |
| `viewer` | Viewer | `read` on every resource except the sensitive ones. |
| `auditor` | Auditor | Viewer plus `audit`, `compliance`, `proxy-logs`, `chat-history`, `exports` read. Never credentials or identity provider secrets. |

Portal/chat-only users hold no binding; `ShowPortal`/`ShowChat` keep governing those surfaces. Operator- or analyst-style roles are a clone of Viewer with a few `execute`/`read` permissions added. Workflow roles are built from `publish`: *LLM Submitter* = `llms:read, llms:write`; *LLM Reviewer* = `llms:read, llms:write, llms:publish`; *Release approver* = `llms:publish` (and the other `*:publish` grants) with no write.

### Rules

1. `roles:write` gates all role and binding changes (Azure-style: no "you cannot grant what you do not hold" check). System roles answer `409 system_role` to edits; clone them to customise.
2. Only an Owner may bind or unbind `owner` (`403 owner_required`); Owner bindings are user-only (`409 owner_user_only`).
3. The last Owner cannot lose the binding, be deleted, or have `is_admin` cleared (`409 last_owner` / `403`).
4. Deleting a role deletes its bindings; deleting a user or team deletes theirs.
5. There is no self-lockout rule; the UI asks for confirmation when you remove your own administrator-bearing role.
6. Custom roles never hold `*`; cloning Owner or Administrator expands the wildcard to every concrete permission.

### The legacy `is_admin` flag

`users.is_admin` stays as a derived cache meaning "holds a wildcard role" so that plugins, portal-side owner checks and a Community downgrade keep working. It is recomputed on binding changes, team membership changes (`AddUserToGroup`, `RemoveUserFromGroup`, `UpdateGroupUsers`, team deletion) and, as a safety net, inside `Resolve` whenever the computed value differs from the stored one. `PATCH /users/:id` with `is_admin` is translated into an Administrator binding add/remove (`SetFullAdmin`) so API automation keeps working; the Enterprise UI omits the field and sends `role_ids` instead (omitted `is_admin` means unchanged).

`AccessToSSOConfig` is no longer consulted for authorisation in Enterprise: identity provider configuration follows `sso-profiles:*`. Administrators who previously lacked the flag gain it through the Administrator role; the seeder logs their emails at WARN.

Other users' API keys are returned only to callers holding `users:write` (or for the caller's own record); everyone else receives `api_key_hint` (last four characters) and `has_api_key`.

---

## 2. Architecture

| Layer | Core (public) | Enterprise submodule |
|---|---|---|
| Catalogue, `Permission`, `Set`, request `Context` | `pkg/authz/` | — |
| Route annotations + registry | `api/authz_routes.go` (`permRouter`), every `/api/v1` route in `api/api.go` | — |
| Enforcement middleware | `api/authz_middleware.go` (`rbacContext`, `requireRoutePermission`, `ssoConfigGuard`) | — |
| Models | `models/rbac.go` (`Role`, `RoleBinding`), migrated by `models.InitModels` in both editions | — |
| Service contract + CE stub | `services/rbac/{interface,factory,community}.go` | `enterprise/features/rbac/{init,service,evaluator,seed,rules}.go` |
| API | `api/rbac_handlers.go`, payload helpers in `api/rbac_payloads.go` | — |
| Audit | — | `enterprise/features/audit/actions.go` entries, denial recording in `middleware.go` |

**Request flow (`/api/v1`)**: `AuthMiddleware` sets the user → `rbacContext` attaches a lazy `authz.Context` → `requireRoutePermission` looks up `METHOD + FullPath` in the registry and evaluates `Set.Has(permission)`. In Community Edition the stub resolves admins to the wildcard and everyone else to nothing, so behaviour is the historical admin-or-not check. Unannotated routes fail closed to full-administrator access and log once; `api/authz_routes_test.go` fails if any `/api/v1` route lacks an annotation.

`authz.AnyAdmin` marks the handful of routes any role holder needs (logout, plugin UI registry and assets, feature probes, `/rbac/permissions`, `/rbac/me`). Everything under `/common/*` (portal and chat) keeps its ownership rules and is not governed by roles in this version; `rbacContext` is attached there so handlers can call `authz.Can` for admin-or-owner decisions.

**Enterprise evaluator** (`Resolve`): one query joins `role_bindings` to `roles` for the user directly or via `user_groups`, unions the permission lists, and short-circuits to the wildcard. Per request only, no cache. When the licence lacks `feature_rbac` the evaluator falls back to the admin-or-not rule and management endpoints answer 402, so an unlicensed Enterprise build behaves like Community Edition while keeping any bindings for later.

**Seeding** (`Seed`, run from `main.go` after the services are built and lazily on first use): upserts the five system roles (refreshing their permissions from the catalogue), binds every legacy admin lacking bindings to Administrator, binds user ID 1 to Owner, and promotes the first administrator when no Owner exists.

**Error envelope**: authorization failures use `authz.ErrorBody` — `{"errors":[{"title","detail","code","permission"}]}` with codes `unauthenticated`, `permission_denied`, `enterprise_required`, `system_role`, `last_owner`, `owner_user_only`, `owner_required`. The UI keys off `code`, never the status alone.

---

## 3. API

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/api/v1/rbac/permissions` | any admin-surface user | `{enabled, groups, actions, resources}`; works in CE |
| GET | `/api/v1/rbac/me` | any admin-surface user | caller's effective permissions and roles |
| GET | `/api/v1/rbac/roles` | `roles:read` | includes `users_count`, `groups_count` |
| POST | `/api/v1/rbac/roles` | `roles:write` | `{name, description, permissions[]}` |
| GET / PATCH / DELETE | `/api/v1/rbac/roles/:id` | `roles:read` / `roles:write` / `roles:delete` | |
| POST | `/api/v1/rbac/roles/:id/clone` | `roles:write` | `{name}` |
| GET | `/api/v1/rbac/bindings?subject_type=&subject_id=&role_id=` | `roles:read` | |
| POST | `/api/v1/rbac/bindings` | `roles:write` | `{subject_type, subject_id, role_id}` |
| DELETE | `/api/v1/rbac/bindings/:id` | `roles:delete` | |
| GET | `/api/v1/rbac/users/:id/effective` | `users:read` | |

Related payload changes: `/common/me` carries `permissions`, `roles`, `has_admin_access`, `rbac_enabled`; user and team payloads carry `roles` (Enterprise) and users carry `api_key_hint`/`has_api_key` when the key is masked; `POST/PATCH /users` accept `role_ids` (reconciled against direct bindings, requires `roles:write` when it changes anything); the `/plugins/sidebar-menu` items carry `required_permission` and are filtered server-side (manifest `mount_config.required_permission`, default `plugins:execute`); plugins receive `permissions` on `PortalUserContext`.

---

## 4. UI

- One `/common/me` fetch feeds an identity store (`admin/utils/identityStore.js`) and `PermissionsProvider` (`admin/context/PermissionsContext.js`); `usePermissions()` exposes `can`, `canAny`, `canAll`, `isFullAdmin`, `hasAdminAccess`, `rbacEnabled`.
- `<Can>` hides controls; `<RequirePermission>` wraps every admin route (`admin/routes.js` descriptors) and renders a denial panel instead of a blank page; nav items carry `permission` and `BaseDrawer` drops what the user cannot see (empty groups included).
- `<PublishSwitch permission={P.LLMS_PUBLISH} …>` (`components/rbac/PublishSwitch.js`) renders the Active/Enabled switch of a form disabled, with a tooltip naming the missing permission, when the user lacks publish. Used by the LLM, data source, plugin, model router, agent and metadata schema forms; the Activate/Deactivate actions on the agent and model router pages are wrapped in `<Can>`. The matrix shows a fifth "Publish" column only on rows that offer it.
- The API client turns `permission_denied`/`enterprise_required` responses into typed errors; denied mutations show one global toast. A code-less 403 keeps its historical "Community Edition" meaning during rollout.
- Roles pages under Access: list (system/custom badge, counts), detail (read-only matrix), form (matrix with implied-read behaviour, shield/key markers), clone dialog. Users and teams get a role selector; user details show roles and effective permissions.

---

## 5. Adding a permission for a new feature

1. Register the resource once in `pkg/authz/catalogue.go` (`Register(Resource{Key, Label, Group, Actions, Sensitive, Privileged})`) — the UI matrix and system roles pick it up automatically. Use `crudp`/`crudxp` when the object has a live switch, and guard that switch in the handlers with `requirePublishIfChanged` / `requirePublishToCreateLive` plus a dedicated `activate`/`deactivate` route annotated `authz.Publish`.
2. Annotate its routes in `api/api.go` with `authz.Read/Write/Delete/Execute/Publish("<key>")`; the completeness test fails otherwise.
3. Add a constant to `ui/admin-frontend/src/admin/rbac/permissions.js`, a `permission` on the nav item in `Drawer.js`, and on the route descriptor in `admin/routes.js`.
4. Add explicit audit action names in `enterprise/features/audit/actions.go` if the generic derivation reads badly.

Scoped bindings (namespace, catalogue) are reserved for a later phase; the binding table already carries `scope_type`/`scope_id`.

---

## 7. Plugin permissions

Every installed plugin with an administrator-facing surface (`studio_ui`, `portal_ui` or `resource_provider` hook) contributes one resource to the catalogue, in the Plugins group, keyed `plugin:<manifest id>` (`models.Plugin.PermissionKey`; `plugin:id-<database id>` until the manifest is known). It offers `read`, `write` and `execute`:

| Permission | Opens |
|---|---|
| `plugin:<key>:read` | the plugin's admin pages (sidebar section, routes served by `GET /plugins/ui-registry` and `/plugins/sidebar-menu`), its detail/configuration page (`GET /plugins/:id`, `/status`, `/config-schema`) and RPC methods the manifest declares read-only |
| `plugin:<key>:write` | any other admin RPC method (`POST /plugins/:id/rpc/:method`) and `PATCH /plugins/:id` limited to `config`, `name` and `description` |
| `plugin:<key>:execute` | reserved for plugin-declared side-effecting methods |

**Umbrella rule.** `plugins:execute` ("call plugins") implies every `plugin:*` permission (`authz.Set.Has`), so Editor and every existing custom role keep using every plugin; per-plugin grants exist to *narrow* access to one plugin. `plugins:read`/`write`/`delete`/`publish` stay the platform-level lifecycle permissions (list, install, scopes, enable, remove) and do not imply anything per plugin. Where a route accepts either, the annotation is any-of (`permRouter.HandleAny`/`HandleAnyFn`): the platform permission or the per-plugin one.

**Lifecycle.** The catalogue is in memory. `services.RebuildPermissionCatalogue` registers every installed plugin at boot, before `Authz().Seed`, so the computed system roles see the full catalogue; the plugin and manifest services then call `SyncPluginPermissions` on create, update, manifest registration (which also stores the manifest on the plugin row, fixing the key) and delete, and `rbac.Service.RefreshSystemRoles` recomputes Editor/Viewer/Auditor after each change. `GET /rbac/permissions` carries `version` (and an `ETag`) so the UI refreshes its cached catalogue; the frontend also drops the cache on the `plugin-loader-refreshed` event.

Viewer therefore reads (opens the pages of) every plugin; a plugin that must not be browsed by read-only roles can be marked `sensitive` in its manifest (Part 3).

**Orphans.** A role may hold `plugin:*` permissions whose plugin is uninstalled or not loaded: `authz.ParseStored` accepts any well-formed plugin permission when a role is saved, `NewSetFromStrings` drops them from evaluation while the resource is absent, and the role payload lists them as `orphaned_permissions` (shown as "Not installed" chips in the role editor, removable there). Reinstalling the plugin makes them effective again.

**Plugin-declared resources.** A manifest may carry an `rbac` block (`models.ManifestRBAC`, validated by `ValidateManifest`):

```json
"rbac": {
  "sensitive": false,
  "resources": [
    {"key": "asset-types", "label": "Asset types", "actions": ["read", "write", "delete"]},
    {"key": "assets", "label": "Assets", "actions": ["read", "write", "delete", "publish"]}
  ],
  "rpc_methods": {"admin_list_types": "asset-types:read", "admin_upsert_type": "asset-types:write", "admin_stats": "read"}
}
```

Each resource becomes `plugin:<manifest id>:<key>` in the catalogue, listed beneath the plugin's row in the role editor. `rpc_methods` values are plugin-relative (`read` = the base resource, `assets:write` = a sub-resource, `plugins:execute` = the platform permission) and are enforced by the RPC route resolver (`api/authz_routes.go` → `pluginRPCPermission`) before the call reaches the plugin; a method that is not listed needs the base `write`, and a declared permission whose resource is not registered falls back to base `write` rather than opening the method. `sensitive: true` withholds the plugin's read from Viewer and Auditor. Declared resources are stored in `plugin_permission_resources` (`source = manifest`, replaced on every manifest registration) so the catalogue can be rebuilt at boot without the plugin process.

Resources that only exist at runtime (asset classes an administrator defines) are registered through the management API: `rpc RegisterPermissionResources` (`services/grpc/plugin_permissions_server.go`, scope `rbac.register`), exposed in the SDK as `ctx.Services.Studio().RegisterPermissionResources(ctx, []plugin_sdk.PermissionResource{...}, removeMissing)`; rows are stored with `source = runtime` and `removeMissing` only prunes runtime rows. Every change refreshes the catalogue and the computed system roles.

**Checking inside a plugin.** The RPC user context carries the caller's permissions with the plugin's own grants spelled out (`pluginCallerPermissions`) plus `Metadata["plugin_permission_key"]`; `userCtx.Can("write")`, `userCtx.Can("assets:publish")` and `userCtx.Can("llms:read")` resolve plugin-relative and platform permissions without the plugin knowing the umbrella rule. On an older host without the key, `Can` falls back to `IsAdmin`. The enterprise asset-catalog plugin is the reference adoption: its manifest declares the three static resources and the method map, and it registers one `assets-<type>` resource (read/write/delete/publish) per active asset type at runtime.

**Plugin pages.** `mount_config.required_permission` (manifest `mount.required_permission`) still overrides the page permission; it is resolved against the plugin (`services.ResolvePluginPermission`): `"read"` means the base resource, `"assets:write"` a declared sub-resource, `"plugins:execute"` the platform permission. `GET /plugins/ui-registry` and `/plugins/sidebar-menu` filter entries by the caller's permissions server-side and carry `required_permission` and `plugin_permission_key`; each plugin's sidebar section gains a "Configuration" link to `/admin/plugins/:id` for holders of the plugin's read. Plugin web components receive `pluginAPI.permissions` and `pluginAPI.can(perm)` (a full permission or a plugin-relative one such as `"write"`), and the RPC call hands the plugin the caller's permissions with the plugin's own grants expanded (`pluginCallerPermissions`), so plugin code never needs the umbrella rule: `userCtx.HasPermission("plugin:<key>:write")` is enough.

---

## 6. Tests

- `pkg/authz/authz_test.go` — catalogue shape, parsing, set semantics.
- `api/authz_routes_test.go` — every `/api/v1` route annotated (both editions, including the conditional marketplace block).
- `api/authz_middleware_test.go` — TestMode off: 401/403/200 with error codes; CE management endpoints answer 402.
- `api/rbac_enterprise_test.go` — end to end with the Enterprise evaluator: Viewer read-only, Editor cannot manage access, masked API keys, team roles flip `is_admin`, Owner rules, `role_ids` on the user payload, and the submitter / reviewer / approver publish workflow on LLMs.
- `enterprise/features/rbac/service_test.go` — seeding, system role shape, union across direct and team bindings, admin-flag sync and self-heal, lockout rules, unlicensed fallback.
- Frontend: `PermissionsContext`, `identityStore`, `apiErrors`, `rbac/gating`, drawer filtering, `PermissionMatrix`, `Roles` page, `UserForm.rbac`.

`TestMode` bypasses the enforcing middleware exactly as it bypassed `AdminOnly`; only the tests that set `TestMode = false` prove enforcement.
