# Workspaces (Enterprise)

A **Workspace** groups configuration objects (model providers, Apps, tools, data sources, routers, catalogues and the rest) so that people and automation can be given a role **within** it. A role given in a Workspace applies only to that Workspace's objects; outside it, the same person may see some objects read-only and others not at all.

Workspaces are an **Enterprise Edition** feature and need role-based access control licensed. In Community Edition every object is in the Default workspace and the Workspaces pages show an upgrade prompt.

Typical uses:

- Give each team a Workspace for its Apps and agents, and let its members manage only those.
- Give automation, such as the [Tyk Operator](#operator-recipe), its own Workspace, so it manages only its own objects and can tell when a push to the edges would carry someone else's changes.

New to Workspaces? Read [Roles, teams and Workspaces](#roles-teams-and-workspaces), then follow [Getting started](#getting-started-a-workspace-for-a-team). If your installation already has users and objects, see [Adopting Workspaces in an existing installation](#adopting-workspaces-in-an-existing-installation).

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

### Words used on this page

| Term | Meaning |
|---|---|
| **Team** | A set of users. The API calls teams *groups* (`/api/v1/groups`, `"subject_type": "group"`, the `groups` permission). |
| **Role assignment** | A role given to a user, service account or team, globally or within one Workspace. The API calls it a *binding* (`/api/v1/rbac/bindings`). |
| **Global** / **scoped** | A global role assignment applies everywhere; a scoped one applies only within its Workspace. |
| **Workspace** | Who manages an object. Not to be confused with a **namespace**, which is where an object is deployed (which edge gateways receive it). |
| **Model provider** | An LLM configuration. The API and permissions call it `llms`. |
| **Catalogue** | Also spelled *catalog* in places. A set of model providers, data sources or tools offered to teams in the Portal. |

## Getting started: a Workspace for a team

This walkthrough gives a team, *Payments*, a Workspace where its members manage their own Apps, agents and model providers, and nothing else. You need a global Administrator (or Owner) role.

1. **Create the Workspace.** **Access → Workspaces → Add Workspace**, name it *Payments*.
2. **Create the team.** **Access → Teams → Add team**, name it *Payments engineers*, and add its members. If your identity provider already has a group for them, map it onto the team instead (see [Single Sign-On](./sso)).
3. **Give the team a role within the Workspace.** Open the *Payments* Workspace, **Access** tab, **Add user or team**, choose *Payments engineers* and the role *Editor* (or a [custom role](./rbac) with less).
4. **Take away wider access.** A global role still covers every Workspace, so check that the members hold no global role that gives them more than you intend: on each member's page (**Users → Edit user**), the *Roles* selector and the teams listed there, including *Default*. The *Effective permissions* panel shows what they hold globally and, separately, in each Workspace.
5. **Move the team's existing objects in.** Open each App, agent, router or model provider the team owns, change its **Workspace** to *Payments* and save. Objects move one at a time, and you need write permission in both Workspaces. Deleting a Workspace is the only bulk move (its objects go to Default).
6. **Decide who pushes to the edges.** Pushing configuration to edge gateways is a global action that a Workspace role never grants (see [When a push is stained](#when-a-push-is-stained)). Either your administrators push for the team, or you give someone a global role with `edges:read` and `edges:execute`. Either way, a push delivers the whole namespace, with every team's waiting changes.

The team's members can now sign in to the administration console. What they see there is described in the next section.

## What someone with only a Workspace role sees

Take Alice, a member of *Payments engineers* from the walkthrough above. Her only role is *Editor* within *Payments*. She has no global role.

- **She can sign in to the administration console.** The menu shows only the sections where she holds a permission, in any Workspace: model management, context management, Apps, agents, catalogues. Users, teams, roles, edges, analytics, plugins and settings are missing, because a Workspace role never grants global resources, even though Editor includes some of them (reading users, for example).
- **Forms work.** The forms for her Workspace's objects offer what they need to be filled in: as owner of an App or a data source, herself and the other members of *Payments*; as an App's team, the teams that hold a role in *Payments*; namespace names; and the governed-metadata fields of a model provider, tool or data source. See [Lookups for the forms](#lookups-for-the-forms). Plugin sections, usage charts and proxy logs are not shown: they are global.
- **Model providers, tools, data sources and the other shared building blocks:** she sees all of them, with credentials redacted. She can change, delete, publish and test only those in *Payments*. She can open one from another Workspace, but not change, delete, publish or test it.
- **Apps, agents, routers and catalogues:** she sees only those in *Payments*. The others are missing from her lists and answer "not found", as if they did not exist.
- **Creating:** everything she creates lands in *Payments*: the form's Workspace selector offers only *Payments*, already selected.
- **Linking:** she can link a *Payments* App to any model provider, tool or data source she can see, including one in the Default workspace. Her App then uses its credentials when serving requests (see [Linking is using](#what-is-visible-across-workspaces)).
- **Pushing:** she cannot push to edges, read edge status, or open analytics.
- **Portal and Chat:** unchanged by Workspaces. What she sees there still depends on her teams' catalogues and on the *Show Portal* and *Show Chat* switches on her account.

If Alice were also given *Viewer* globally, she would see everything read-only, and could still change only *Payments* objects.

## Adopting Workspaces in an existing installation

Upgrading changes nothing: every object lands in the Default workspace and every role assignment stays global. Creating a Workspace changes nothing either. Access is restricted only once you replace global role assignments with scoped ones. A suggested order:

1. **List the global role assignments.** Each user's and team's page shows them, or use `GET /api/v1/rbac/bindings?scope_type=global`. After an upgrade from 2.2, administrators hold *Administrator* globally and everyone else holds no role; any role assigned since then is global too.
2. **Choose your Workspaces.** Usually one per team that owns Apps and agents. Shared building blocks (model providers, tools, data sources) can stay in the Default workspace: everyone can read and link them, and only people with a role in Default (or a global one) can change them.
3. **Create the Workspaces and move objects into them**, as in steps 1 and 5 of [Getting started](#getting-started-a-workspace-for-a-team).
4. **Assign roles within each Workspace** to the team that owns it.
5. **Remove the global assignments those people no longer need.** Pay particular attention to roles assigned to the *Default* team, which every user belongs to (see the warning above). Keep global roles for the people who administer the whole installation and for those who push to edges.
6. **Check a member's view.** Their *Effective permissions* panel should show little or nothing globally, and the expected roles under each Workspace.
7. **Plan namespaces with Workspaces in mind.** A push delivers a whole namespace, so a team that pushes for itself is best given its own namespace (see [Edge Gateways](./edge-gateways)).

To undo, assign the global roles again: objects can stay where they are.

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
- **Plugin resources** (what a plugin offers to Apps, such as the MCP servers of a registry plugin) are global too. Attaching one to an App that the App does not have yet needs the global `plugins:read` permission as well as write on the App: without it the admin UI does not show the pickers and the API answers `403`. Someone who can edit the App can still save it with the plugin resources it has, and remove them.
- **Access management**: users, teams, roles, App quotas and the license are global.
- **Governed metadata** on MCP servers and plugin resources needs the global `metadata` permission.

Management API routes on Workspace objects that are not Workspace-aware require their permission **globally**, so a person with only Workspace roles is refused (403) rather than given wider access.

### Lookups for the forms

A few lists that belong to no Workspace are needed to fill in the form of a Workspace object. Someone who holds the listed write permission **in any Workspace**, and not the global permission, gets a reduced answer from these routes and from no other route of the same resource:

| Route | Also open to | What a Workspace role gets |
|---|---|---|
| `GET /api/v1/users` | `apps:write` or `datasources:write` | The caller and the **members** of the Workspaces where they hold that permission: `id`, `name` and `email` only. `search` and `sort` work on name and email; other filters are ignored. |
| `GET /api/v1/users/:id` | the same | The same fields for the same people; anyone else answers `404`, as a user that does not exist. |
| `GET /api/v1/groups` | `apps:write` | The teams that hold a role in those Workspaces: `id` and `name` only. |
| `GET /api/v1/namespaces` | write on model providers, data sources, Apps, tools, filters, model routers or semantic routers | Namespace names only (no edge, object or token counts). |
| `GET /api/v1/metadata/schemas/resolve?object_type=` | write on that object type (`llm`, `tool`, `datasource`) | The resolved schema, as for `metadata:read`. |
| `GET /api/v1/metadata/vocabularies` | write on model providers, tools or data sources | The vocabularies, as for `metadata:read`. |
| `GET /api/v1/vendors/llm-drivers`, `/vendors/embedders`, `/vendors/vector-stores` | the same read permission, held in any Workspace | The list of supported vendors. |

A **member** of a Workspace is a user who holds a role assigned within it, directly or through a team. So giving a team that everyone belongs to (such as *Default*) a role within a Workspace makes every user visible, by name and email, to that Workspace's editors. People who hold only a global role are not members: an App they own shows its owner as `User #<id>` to a Workspace editor.

A holder of the global permission (`users:read`, `groups:read`, `edges:read`, `metadata:read`) gets the full answer, unchanged. The lookups only read: the write routes and the sub-routes (`/users/:id/...`, `/groups/:id/...`) still need the global permission, and the API does not yet refuse an owner or team from outside your Workspaces when an App or data source is saved.

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

## When a push is stained

A Workspace is who manages an object; a namespace is which edge gateways receive it. The two are independent, and **every push delivers the whole namespace**, with every change waiting in it, whichever Workspace it belongs to. The push dialog shows each waiting change's Workspace and a count per Workspace, so check it before you push.

Pushing is a **global** action (`edges:read` and `edges:execute`). A role within a Workspace never lets someone push, so in practice people with a global role push for everyone.

A push is **stained** when it would carry changes from outside a given Workspace. This only matters to API clients that ask AI Studio to refuse such pushes (`only_workspace_id`), such as the [Tyk Operator](#operator-recipe) set to refuse. A push from the administration UI is never refused for this.

- **What blocks a client:** any change waiting in its namespace from another Workspace, or from no Workspace at all (plugins, model prices and other global objects). It stays blocked until that change has been pushed.
- **Two clients can block each other.** If client A manages Workspace *Payments* and client B manages *Data*, both refuse stained pushes, and both push to the same namespace, then A's waiting change stains B's push and B's stains A's. Neither can push.
- **How to unblock:** someone with a global push role opens the push dialog for that namespace, checks the waiting changes and their Workspaces, and pushes. That delivers everything waiting, so every client's next push is clean. A client set to *allow* stained pushes unblocks the namespace the same way.
- **How to avoid it:** give each team, and each automated client, its own namespace; keep objects in the global namespace to a minimum, since they reach, and stain, every namespace; and push foreign changes promptly rather than leaving them waiting.

See [Pushes limited to one Workspace](./edge-gateways#pushes-limited-to-one-workspace) for the API details.

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

Any foreign change already waiting when the Operator starts stains its first push with `only_workspace_id`. Push the namespace once beforehand (as an administrator, after checking what is pending) if you want the Operator to start clean.

See [Stained pushes](./edge-gateways#pushes-limited-to-one-workspace) for the body of a `409 push_stained` and the limits of the check.

### The recipe through the API

The same setup and the Operator's calls, as an administrator (`$ADMIN_KEY`) and then as the service account (`$SA_KEY`). Ids in the examples are illustrative.

```bash
# 1. Admin: the Operator's Workspace (answers 201 with its id, here 4)
curl -X POST "$STUDIO/api/v1/workspaces" -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"name": "operator-team-a", "description": "Objects managed by the Tyk Operator for team A"}'

# 2. Admin: the service account (201; data.attributes.api_key is the key, in this response only)
curl -X POST "$STUDIO/api/v1/users" -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"data": {"type": "users", "attributes": {"account_type": "service", "name": "tyk-operator-team-a"}}}'

# 3. Admin: a small global role for pushes, assigned globally (no scope)
curl -X POST "$STUDIO/api/v1/rbac/roles" -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"name": "Operator push", "description": "Read edge sync state and push configuration", "permissions": ["edges:read", "edges:execute"]}'
curl -X POST "$STUDIO/api/v1/rbac/bindings" -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"subject_type": "user", "subject_id": 44, "role_id": 9}'

# 4. Admin: Editor within the Workspace (find Editor's id with GET /api/v1/rbac/roles)
curl -X POST "$STUDIO/api/v1/rbac/bindings" -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"subject_type": "user", "subject_id": 44, "role_id": 3, "scope_type": "workspace", "scope_id": "4"}'

# 5. Operator: where may I work? (/rbac/me lists the Workspace under "workspaces";
#    /workspaces returns only Workspace 4)
curl "$STUDIO/api/v1/rbac/me" -H "Authorization: Bearer $SA_KEY"
curl "$STUDIO/api/v1/workspaces" -H "Authorization: Bearer $SA_KEY"

# 6. Operator: objects it creates land in its Workspace, no workspace_id needed
curl -X POST "$STUDIO/api/v1/llms" -H "Authorization: Bearer $SA_KEY" \
  -H 'Content-Type: application/json' -d '{"data": {"type": "llms", "attributes": {...}}}'

# 7. Operator: would a push carry anyone else's changes? ("stained": true or false)
curl "$STUDIO/api/v1/sync/pending-changes?namespace=default&workspace_id=4" \
  -H "Authorization: Bearer $SA_KEY"

# 8. Operator: push only if it carries nothing foreign (202, or 409 push_stained)
curl -X POST "$STUDIO/api/v1/namespaces/default/reload" -H "Authorization: Bearer $SA_KEY" \
  -H 'Content-Type: application/json' -d '{"only_workspace_id": 4}'

# 8b. Operator in "allow" mode: push without the body (202) and report the stain itself
curl -X POST "$STUDIO/api/v1/namespaces/default/reload" -H "Authorization: Bearer $SA_KEY"
```

The service account cannot change objects outside its Workspace (a Default-workspace model provider answers `403 permission_denied` with `llms:write`) or reach global resources it has no role for (`PATCH /api/v1/users/44` answers `403` with `users:write`, and `GET /api/v1/edges` answers `403` with `edges:read`). `GET /api/v1/users` answers it with the [lookup](#lookups-for-the-forms) form only: the members of its Workspace, by id, name and email.

## Upgrading and rolling back

- **Upgrade:** every existing object lands in the Default workspace and every role assignment stays global. Nobody sees or loses anything, and nothing shows as a pending change because of the upgrade.
- **Rollback** to a release without Workspaces: roles assigned within a Workspace grant nothing there (the older release ignores them, which fails safe), and Workspaces are ignored. A service account keeps working through its API key on the older release, as an ordinary user without the service-account restrictions.

## See also

- [Roles and Permissions](./rbac)
- [Users](./users) and [User Management](./user-management)
- [Edge Gateways](./edge-gateways)
- [License and Quotas](./license)
