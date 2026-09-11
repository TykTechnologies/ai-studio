# Roles and Permissions (Enterprise)

Roles let you give people exactly the access they need to Tyk AI Studio's administration UI and API: a platform engineer who runs models and tools but cannot change who has access, an auditor who reads the audit trail but changes nothing, a reviewer who only sees the community submission queue.

Roles are an **Enterprise Edition** feature. In Community Edition a user is either an administrator (full access) or not.

## How it works

- A **permission** is `resource:action`, for example `llms:write` or `audit:read`. Actions are `read`, `write` (create and update), `delete`, `execute` (test, call, reload, sync) and `publish` (make live: set an LLM, tool, data source, app or agent active, enable a plugin, activate a metadata schema). Write, delete, execute and publish each include read. Publish does not include write, so you can build a *submitter* role that drafts providers without releasing them and a *reviewer* role that releases them: a submitter who flips the Active switch gets `403` with `"permission": "llms:publish"`, and the switch is disabled in the form.
- A **role** is a named set of permissions.
- Roles are **assigned to users and to teams**. Someone's access is the union of their own roles and the roles of every team they belong to, so mapping identity provider groups onto teams (see [Single Sign-On](./sso)) assigns roles automatically.
- Permissions apply everywhere: navigation, pages, buttons, and every management API call, including calls made with a user's API key.

The Portal and Chat surfaces are unchanged: whether a user sees them is still controlled by the *Show Portal* and *Show Chat* switches, and what they see inside them by their teams' catalogues.

## Built-in roles

| Role | Use it for | What it can do |
|---|---|---|
| **Owner** | The people ultimately responsible for the installation | Everything, including making other Owners. The last Owner can never be removed. |
| **Administrator** | Day-to-day platform administrators | Everything, except granting or revoking Owner. |
| **Editor** | Engineers who build and operate AI resources | Create, change, delete and test LLM providers, tools, data sources, filters, apps, credentials, secrets, chats, agents, catalogues and edge gateways. Cannot manage users, teams, roles or identity providers, install plugins, change governance schemas, or read conversation transcripts, proxy logs, exports or the audit trail. |
| **Viewer** | People who need to look but not touch | Read-only access to configuration and analytics. No sensitive data: no transcripts, proxy logs, audit records, exports, credentials or identity provider settings. |
| **Auditor** | Compliance and security reviewers | Viewer plus the audit trail, compliance reports, proxy logs, conversation transcripts and existing exports. Still no credentials or identity provider secrets. |

Built-in roles cannot be edited. To customise one, **clone** it and adjust the copy.

The first user of the installation is the Owner. On upgrade, every existing administrator becomes an Administrator; everyone else keeps their Portal and Chat access with no administrative role.

## Managing roles

Open **Access → Roles** in the administration UI (you need the `roles:read` permission to see it and `roles:write` to change anything).

- **Add role** opens the permission matrix: one row per resource, grouped like the navigation, with a column per action. Ticking write, delete or execute also ticks read. Rows marked with a shield expose sensitive data; rows marked with a key can grant access to other people.
- **Clone** a built-in role to start from a known set, then remove or add what you need.
- **Delete** removes the role from every user and team that had it.

### Assigning roles

- **Users → Edit user** has a *Roles* selector. Roles assigned here belong to the user directly.
- **Teams → Edit team** has a *Roles* section. Every member of the team inherits those roles.
- A user's page shows their roles and an *Effective permissions* panel that merges direct and team roles.

The Owner role can only be assigned to users (not teams) and only by another Owner.

## Recipes

- **Operator** who can push configuration to edge gateways and test filters but not change them: clone Viewer, add `edges:execute`, `filters:execute`, `tools:execute`.
- **Submission reviewer**: clone Viewer, add `submissions:write` and `submissions:execute`, remove everything outside Community if you prefer.
- **LLM submitter / LLM reviewer**: a new role with `llms:read` and `llms:write` can create and edit providers but not set them active; add `llms:publish` for the reviewer who releases them. A role holding only `llms:publish` can activate and deactivate providers through the Activate action but cannot edit them.
- **One plugin only**: every installed plugin with pages or resource types appears in the Plugins group of the matrix under its own name, with read (open its pages and configuration), write (call its actions, edit its configuration) and execute. Tick those instead of `Installed plugins: execute`, which unlocks every plugin at once. Grants on a plugin that is later uninstalled are kept on the role and listed as "Not installed" until it returns.
- **LLM cost analyst**: a custom role with only `analytics:read` and `model-prices:read`.

## API

Everything the UI does is available under `/api/v1/rbac/`:

```
GET    /api/v1/rbac/permissions            the catalogue (also tells you whether roles are enabled)
GET    /api/v1/rbac/me                     your effective permissions and roles
GET    /api/v1/rbac/roles                  list roles
POST   /api/v1/rbac/roles                  {"name": "...", "description": "...", "permissions": ["llms:read", ...]}
PATCH  /api/v1/rbac/roles/:id
DELETE /api/v1/rbac/roles/:id
POST   /api/v1/rbac/roles/:id/clone        {"name": "..."}
GET    /api/v1/rbac/bindings?subject_type=user&subject_id=42
POST   /api/v1/rbac/bindings               {"subject_type": "user"|"group", "subject_id": 42, "role_id": 3}
DELETE /api/v1/rbac/bindings/:id
GET    /api/v1/rbac/users/:id/effective
```

You can also pass `role_ids` when creating or updating a user through `/api/v1/users`.

A request your role does not allow returns `403` with `"code": "permission_denied"` and the missing permission:

```json
{"errors":[{"title":"Forbidden","detail":"missing permission llms:write","code":"permission_denied","permission":"llms:write"}]}
```

In Community Edition, or when the licence does not include roles, the role management endpoints return `402` with `"code": "enterprise_required"`.

## Things to know

- **API keys.** A user's API key has exactly the user's permissions. Another user's key is only shown to people who may manage users (`users:write`); everyone else sees the last four characters.
- **`is_admin`.** The legacy flag is still reported and still accepted by the API: it now means "holds Owner or Administrator", and setting it adds or removes the Administrator role. Omitting it leaves roles alone.
- **Identity provider settings** follow the `sso-profiles` permission. The old per-user "access to IdP configuration" switch is no longer used; administrators who lacked it gain that access through the Administrator role, and the server logs who they were at startup.
- **Audit trail.** Role and assignment changes are recorded like any other change, with diffs. Requests refused for lack of permission are recorded too, including reads.
- **Plugins.** Admin plugin pages require `plugins:execute` unless the plugin's manifest names another permission (`mount_config.required_permission`). Plugins receive the caller's permissions alongside the existing `is_admin` flag.
