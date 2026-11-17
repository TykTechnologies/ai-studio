# Workspaces (Enterprise)

A **Workspace** groups configuration objects (model providers, Apps, tools, data sources, routers, catalogues and the rest) so that people and automation can be given a role **within** it. A role given in a Workspace applies only to that Workspace's objects; outside it, the same person may see some objects read-only and others not at all.

Workspaces are an **Enterprise Edition** feature and need role-based access control licensed. In Community Edition every object is in the Default workspace and the Workspaces pages show an upgrade prompt.

Typical uses:

- Give each team a Workspace for its Apps and agents, and let its members manage only those.
- Give automation, such as the [Tyk Operator](#operator-recipe), its own Workspace, so it manages only its own objects and can tell when a push to the edges would carry someone else's changes.

## Roles, teams and Workspaces

- A **role** says *what* someone may do (see [Roles and Permissions](./rbac)).
- A **team** or a user says *who*.
- A Workspace says *where*.

Every role assignment (binding) is either **global** or **scoped to a Workspace**:

- A **global** role applies everywhere, in every Workspace. All role assignments made before Workspaces existed are global, so upgrading changes nothing for anyone.
- A **scoped** role applies only to the objects of its Workspace. Only the role's permissions on Workspace objects count there: permissions on global resources in the same role (users, teams, roles, plugins, webhooks, edges, settings and so on) are ignored inside a Workspace. Give those with a global role.

A role assigned in a Workspace can be any role, built-in or custom, except **Owner**, which is always global. *Administrator* given in a Workspace means "everything inside that Workspace"; it does not make the person an administrator of the installation.

::: warning A global role on a team everyone is in
A global role covers every Workspace. If you give a global role (Editor, say) to a team that everyone belongs to, such as Default, everyone in that team can work in every Workspace. Give such teams a scoped role instead.
:::

## What is visible across Workspaces

Objects fall into three kinds:

| Kind | Objects | Outside the caller's Workspaces |
|---|---|---|
| **Shared building blocks** | Model providers, tools, data sources, filters, embedders, secrets, file stores, MCP servers | **Visible, read-only.** Anyone who may read that kind of object in any Workspace sees all of them, with credentials redacted, and can link them (to an App, a catalogue and so on). Changing, deleting, publishing or testing one needs the permission in its own Workspace. |
| **Owned objects** | Apps, agents, model routers, semantic routers, model provider catalogs, data source catalogs, tool catalogs | **Hidden.** They are missing from lists and answer "not found", exactly as if they did not exist. |
| **Credentials** | App credentials and minted MCP keys | **Follow their App**: hidden when the App is hidden, managed with the permission in the App's Workspace. |

Plugins, webhooks, edges, users, teams, roles, analytics, logs, traces and other settings are **global**: they belong to no Workspace and are governed by global roles only.

::: warning Linking is using
An App linked to another Workspace's model provider, tool or data source uses it, credentials included, when it serves requests. Workspaces protect who may **change** an object, not who may **use** it: anyone who may write an App can link any shared building block they can read. Keep credentials you need to restrict to some teams on objects only those teams' Apps link to, and review App links. A separate permission for use is planned.
:::

A few things to know:

- **Secret references stay inside a Workspace.** A `$SECRET/NAME` reference must name a secret in the same Workspace as the object that uses it, and a secret's name is unique across all Workspaces. A referenced secret cannot be deleted, renamed or moved to another Workspace.
- **Do not put secrets in file stores.** File store content is a shared building block: anyone who may read file stores in any Workspace can read every file.
- **Tyk connections are global.** Someone who may run MCP server actions in one Workspace (register, push, link, discover) can use any Tyk Dashboard or Gateway connection. Restricting connections by Workspace is planned.
- **Unique names are unique across Workspaces.** Where a name must be unique (secret names, agent slugs and the like), it is unique across all Workspaces. A create that takes such a name fails with a conflict, even when the other object is hidden from you, so the conflict can reveal a hidden object's name.
- **Counts include what you cannot see.** Dependency totals and pending-change counts include hidden objects; their names and ids are never shown.

## What Workspaces do not cover

Workspaces govern the administration UI and the management API. These parts of AI Studio do not look at Workspaces, on purpose:

- **The Portal and Chat.** What a Portal or Chat user sees is still decided by their teams' catalogues, never by Workspaces. Apps created in the Portal land in the Default workspace.
- **Serving requests.** The gateway, edge gateways, chat and agent sessions serve every object they are configured with, whatever its Workspace.
- **Edges and namespaces.** They belong to no Workspace (see [Pushes limited to one Workspace](./edge-gateways.md#pushes-limited-to-one-workspace) for how the two meet).
- **Analytics, logs, traces and the audit trail**, including compliance reports, MCP analytics, guardrail block reports and proxy log exports: global, governed by their global permissions.
- **Plugins and webhooks**: global, even though each records the Workspace it was created in.
- **Access management**: users, teams, roles, App quotas and the license are global.
- **Governed metadata** on MCP servers and plugin resources needs the global `metadata` permission.

Management API routes on Workspace objects that are not Workspace-aware require their permission **globally**, so a person with only Workspace roles is refused (403) rather than given wider access.

## Managing Workspaces

Open **Access → Workspaces**. You need `workspaces:read` to see the page, `workspaces:write` to create or rename a Workspace, `workspaces:delete` to delete one, and `roles:write` (global) to give people roles in it.

- **Add Workspace** asks for a name (unique, ignoring case) and a description.
- The **Default** workspace always exists, comes first, and cannot be deleted. Objects created by someone with a global role land there.
- A Workspace's page has two tabs:
  - **Access**: the users, service accounts and teams that hold a role in this Workspace. *Add user or team*, change their roles, or remove them. The Owner role is not offered.
  - **Objects**: how many objects of each kind the Workspace owns, each linking to that kind's list filtered to the Workspace.
- **Delete** is refused while anyone still holds a role in the Workspace (remove them on the Access tab first). Deleting moves every object the Workspace owns to the Default workspace.

## Creating objects in a Workspace

Forms for Workspace objects show a **Workspace** selector listing the Workspaces where you may create that kind of object. With one choice it is preselected.

Through the API, send `workspace_id` with the object (in `data.attributes` for JSON:API payloads). When you leave it out:

- with a global role, the object lands in the Default workspace;
- with the permission in exactly one Workspace, it lands there;
- with the permission in several Workspaces, the request fails with `422 workspace_required`: name one.

Naming a Workspace that does not exist, or one where you lack the permission, fails with `422 invalid_workspace` (the same answer in both cases).

Apps created in the Portal always land in the Default workspace.

## Moving objects

To move an object, change its Workspace in its edit form, or send a different `workspace_id` in its update. You need write permission in **both** Workspaces. A moved object:

- keeps its links and its credentials (an App's credentials move with it);
- shows as a pending change for the edges, attributed to its new Workspace;
- cannot move if it references a secret in a Workspace other than its destination, and a secret cannot move while anything references it.

## Lists and the API

Every list of Workspace objects accepts `?workspace_id=<id>` to show one Workspace's objects. Responses carry each object's `workspace_id`.

```
GET    /api/v1/workspaces                  the Workspaces you can work in
POST   /api/v1/workspaces                  {"name": "...", "description": "..."}
GET    /api/v1/workspaces/:id
PATCH  /api/v1/workspaces/:id              name and description only
DELETE /api/v1/workspaces/:id              409 workspace_in_use while roles are assigned in it
GET    /api/v1/workspaces/:id/objects      object counts per kind
POST   /api/v1/rbac/bindings               {"subject_type": "user"|"group", "subject_id": 42, "role_id": 3,
                                            "scope_type": "workspace", "scope_id": "7"}
GET    /api/v1/rbac/bindings?scope_type=workspace&scope_id=7
```

| Status and code | When |
|---|---|
| `404` | An owned object (or its credential) outside your Workspaces, exactly as for a missing one |
| `403 permission_denied` | You can see the object but lack the permission in its Workspace |
| `422 workspace_required` | A create with no `workspace_id` when you could create in several Workspaces |
| `422 invalid_workspace` | The named Workspace does not exist or is not one you may use |
| `409 default_workspace_locked` | Deleting the Default workspace |
| `409 workspace_in_use` | Deleting a Workspace someone still holds a role in |
| `409 owner_scope_forbidden` | Assigning the Owner role within a Workspace |

`GET /common/me`, `GET /api/v1/rbac/me` and `GET /api/v1/rbac/users/:id/effective` list, under `workspaces`, the permissions held in each Workspace. A user's page in the administration UI shows them per Workspace too.

## Operator recipe

The Tyk Operator manages AI Studio objects from Kubernetes and pushes configuration to edge gateways. Give it:

1. **A service account** (see [Users](./users#service-accounts)): **Users → Add user**, account type *Service account*. Copy its API key when it is shown; it is shown once.
2. **Its own Workspace**, for example *Operator - payments*: **Access → Workspaces → Add Workspace**.
3. **A role in that Workspace**: on the Workspace's Access tab, add the service account with *Editor* (or a custom role with just what it manages).
4. **A small global role for pushes**: a custom role with `edges:read` and `edges:execute` only, assigned to the service account globally (**Users → Edit user → Roles**). Pushing and reading pending changes are global actions; this role grants nothing else.
5. **One namespace per team** for the edges the Operator pushes to (see [Edge Gateways](./edge-gateways)). Objects in the global namespace reach every edge, so a change to one of them shows up in every team's pushes.

The Operator then:

- lists and changes only its own Workspace's objects (`?workspace_id=<id>` on lists, `workspace_id` on creates);
- reads `GET /api/v1/sync/pending-changes?namespace=<ns>&workspace_id=<id>` before a push: `outside_total` and `stained` say whether changes from outside its Workspace are waiting;
- pushes with `POST /api/v1/namespaces/<ns>/reload`, choosing in its own configuration how to treat a *stained* push:
  - **allow**: push without a body, and report the foreign changes it read from pending changes as a warning;
  - **refuse**: send `{"only_workspace_id": <id>}`; AI Studio refuses the push with `409 push_stained` while foreign changes are waiting, and records nothing.

See [Stained pushes](./edge-gateways#pushes-limited-to-one-workspace) for the body of a `409 push_stained` and the limits of the check.

## Upgrading and rolling back

- **Upgrade:** every existing object lands in the Default workspace and every role assignment stays global. Nobody sees or loses anything, and nothing shows as a pending change because of the upgrade.
- **Rollback** to a release without Workspaces: roles assigned within a Workspace grant nothing there (the older release ignores them, which fails safe), and Workspaces are ignored. A service account keeps working through its API key on the older release, as an ordinary user without the service-account restrictions.

## See also

- [Roles and Permissions](./rbac)
- [Users](./users) and [User Management](./user-management)
- [Edge Gateways](./edge-gateways)
- [License and Quotas](./license)
