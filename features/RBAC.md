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

**Permission** — a string `<resource>:<action>`. Resources are the plural, kebab-case route collection segments of the management API (`llms`, `data-catalogues`, `sso-profiles`). Actions are exactly four:

| Action | Meaning | HTTP mapping |
|---|---|---|
| `read` | list, get, search, status, history, download an existing artefact | `GET` |
| `write` | create, update, link sub-resources, activate/deactivate, approve/reject, rollback | `POST`, `PUT`, `PATCH` |
| `delete` | remove a resource or a sub-resource link | `DELETE` |
| `execute` | side-effecting operations that do not persist configuration: test, call, reload, sync, re-process | `POST` on a verb route |

`write`, `delete` and `execute` each imply `read`. There are no deny rules. The single wildcard `*` is held only by the Owner and Administrator system roles.

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
| `editor` | Editor | Everything except: `users`/`groups` beyond read, `roles`, `sso-profiles`, `plugins`/`marketplace` write and delete, `metadata` write and delete, `audit`, `exports`, `proxy-logs`, `chat-history`. Includes `credentials:*` and `secrets:*`. |
| `viewer` | Viewer | `read` on every resource except the sensitive ones. |
| `auditor` | Auditor | Viewer plus `audit`, `compliance`, `proxy-logs`, `chat-history`, `exports` read. Never credentials or identity provider secrets. |

Portal/chat-only users hold no binding; `ShowPortal`/`ShowChat` keep governing those surfaces. Operator- or analyst-style roles are a clone of Viewer with a few `execute`/`read` permissions added.

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
- The API client turns `permission_denied`/`enterprise_required` responses into typed errors; denied mutations show one global toast. A code-less 403 keeps its historical "Community Edition" meaning during rollout.
- Roles pages under Access: list (system/custom badge, counts), detail (read-only matrix), form (matrix with implied-read behaviour, shield/key markers), clone dialog. Users and teams get a role selector; user details show roles and effective permissions.

---

## 5. Adding a permission for a new feature

1. Register the resource once in `pkg/authz/catalogue.go` (`Register(Resource{Key, Label, Group, Actions, Sensitive, Privileged})`) — the UI matrix and system roles pick it up automatically.
2. Annotate its routes in `api/api.go` with `authz.Read/Write/Delete/Execute("<key>")`; the completeness test fails otherwise.
3. Add a constant to `ui/admin-frontend/src/admin/rbac/permissions.js`, a `permission` on the nav item in `Drawer.js`, and on the route descriptor in `admin/routes.js`.
4. Add explicit audit action names in `enterprise/features/audit/actions.go` if the generic derivation reads badly.

Plugin-declared permissions (`plugin:<slug>:<action>`) and scoped bindings (namespace, catalogue) are reserved for a later phase; the binding table already carries `scope_type`/`scope_id`.

---

## 6. Tests

- `pkg/authz/authz_test.go` — catalogue shape, parsing, set semantics.
- `api/authz_routes_test.go` — every `/api/v1` route annotated (both editions, including the conditional marketplace block).
- `api/authz_middleware_test.go` — TestMode off: 401/403/200 with error codes; CE management endpoints answer 402.
- `api/rbac_enterprise_test.go` — end to end with the Enterprise evaluator: Viewer read-only, Editor cannot manage access, masked API keys, team roles flip `is_admin`, Owner rules, `role_ids` on the user payload.
- `enterprise/features/rbac/service_test.go` — seeding, system role shape, union across direct and team bindings, admin-flag sync and self-heal, lockout rules, unlicensed fallback.
- Frontend: `PermissionsContext`, `identityStore`, `apiErrors`, `rbac/gating`, drawer filtering, `PermissionMatrix`, `Roles` page, `UserForm.rbac`.

`TestMode` bypasses the enforcing middleware exactly as it bypassed `AdminOnly`; only the tests that set `TestMode = false` prove enforcement.
