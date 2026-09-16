# Tyk Dashboard MCP Integration

:::note Enterprise Edition
The Tyk Dashboard MCP integration is an Enterprise Edition feature. In
Community Edition the Settings → Tyk Connections page shows what the feature
offers and every integration route answers `403`.
:::

Tyk Gateway terminates the Model Context Protocol natively: an **MCP proxy**
is a Tyk OAS API definition with MCP middleware, per-primitive policies,
OAuth 2.1 protected-resource metadata and MCP analytics. This integration
makes those proxies part of AI Studio without moving a single MCP request
through AI Studio:

- **Discovery.** MCP proxies on a Tyk Dashboard are imported, reviewed,
  scored for privacy and published to the AI Portal as an asset class, next
  to LLMs, tools and data sources.
- **Access.** Portal users bind MCP servers to their Apps. For servers that
  authenticate with an API key, AI Studio mints a Tyk key for the App from
  policies an administrator pinned, shows it once, and keeps the key in step
  with those policies afterwards. For OAuth, mTLS and keyless servers it
  records who has access and tells the user how to connect.
- **Registration.** An administrator creates a new MCP proxy on the
  Dashboard from AI Studio, or a portal user asks for one through the
  submission queue.
- **Audit.** Every key, grant, push and download is in the ledger and the
  [audit trail](audit-trail.md), and the access report answers "who reaches
  what".

The Tyk Gateway stays the only data plane. Clients call the proxy's listen
path on the gateway with the key (or token) they hold; AI Studio is never on
that path.

## Connections and trust modes

A **connection** is one Tyk Dashboard URL and one Dashboard user access
key. Several connections are supported; every imported server, cached policy
and minted key belongs to exactly one. Connections live under Settings →
Tyk Connections.

The Dashboard user's permissions are the trust boundary. The connection's
**mode** can only narrow what AI Studio does with them:

| Mode | AI Studio may | Dashboard user needs | Typical organisation |
|---|---|---|---|
| `catalogue` | import MCP proxies and policies; read back keys it minted earlier | APIs read, policies read | The platform team owns APIM; the AI team discovers and publishes. Registrations become handoff packages. |
| `broker` | catalogue, plus mint, update and revoke keys against pinned policies | plus keys write | The AI team runs the portal on top of platform-managed proxies. |
| `full` | broker, plus create and edit MCP proxies and create policies | plus APIs write, policies write | One team owns both, or high trust. |

On activation, and on every sync, AI Studio **probes** the Dashboard with
side-effect-free calls and records what it can prove. The Dashboard offers
no permission introspection for API keys, so write capabilities stay
`unverified` until the first real write succeeds, and the first `401`/`403`
marks the connection **degraded** with the failing capability named. The
effective mode is the lower of the declared mode and what the probe found.

Activating a connection is an `execute` action on the `tyk-connections`
resource. Set `TYK_MCP_REQUIRE_DIFFERENT_ACTIVATOR=true` to require that the
activator is not the person who created or last edited the connection.

### Dashboard user recipes

Create a dedicated Dashboard user per connection and give it a user group
with exactly these permissions:

- **catalogue**: `apis: read`, `policy: read`, `mcps: read` (Dashboard 5.13
  and later expose MCP proxies under their own permission).
- **broker**: the above plus `keys: write`. Enable
  `enable_delete_key_by_hash` on the Gateway if you want revoked keys
  deleted; without it AI Studio switches revoked keys off instead and says
  so.
- **full**: the above plus `apis: write`, `policy: write`, `mcps: write`.

The admin secret is never accepted. The organisation id is entered on the
connection (pre-filled from a key preview when the Dashboard returns it) and
every imported object is checked against it.

### API template

A connection can name a Dashboard **API template** (Assets → API templates;
the template's custom id or database id). Its defaults are merged into every
MCP proxy AI Studio creates on that connection, whether an administrator
registers it or a community submission is approved, and again on every push
from AI Studio, so an edit never strips them. This is how a platform team
says "publish MCP proxies as you like, but every one of them carries our
traffic logging, caching, middleware and tags". The merge follows the
Dashboard's own rule for `POST /api/apis/oas?templateID=`: the template is
the base, the registration's own values win on conflicts, arrays are
replaced rather than combined, and the template's identity fields are
dropped. A Dashboard 5.14 ignores `templateID` on the MCP endpoint, which is
why AI Studio fetches the asset (`GET /api/assets/{id}`) and merges it
itself. The probe records `template_read`; a template that cannot be fetched
fails registration closed rather than creating an ungoverned proxy.

### Segmented gateways and MDCB

Where gateways are segmented, a proxy is only loaded by gateways whose tags
match its `gatewayTags`. A connection can learn the tags in two ways:

- **MDCB.** Set the MDCB URL and access token and AI Studio calls
  `/dataplanes` on every probe and sync, keeping the group id, tags, node
  count, versions and health of each data plane. The raw response, which
  carries each node's own API key, is never stored or logged.
- **Known tags.** Maintain a list of tags with labels for stacks without
  MDCB.

The **deployment target** control (registration wizard, submission form,
reviewer page) only renders when a connection knows at least one tag, and
warns when none is chosen. A per-tag public gateway URL on the connection
lets the portal show the right endpoint for each target.

## Discovery and publishing

Each active connection is synced on its interval (`sync_interval_seconds`,
never below `TYK_MCP_SYNC_MIN_INTERVAL`) by whichever AI Studio node claims
it; "Sync now" on the connection or the servers page runs it at once. A sync
lists every MCP proxy, derives the catalogue fields (listen path, transport
path, kind, consumer authentication, primitives, gateway tags), stores the
definition with upstream credentials masked, refreshes the policy cache and
reconciles minted keys.

Imported servers appear under Context management → MCP servers,
unpublished. Before publishing, an administrator:

1. sets a **privacy score** (there is no platform default; an imported server
   has none until someone decides),
2. adds it to the **tool catalogs** whose teams may see it in the portal
   (Catalogs → Teams, exactly as for tools),
3. optionally pins a **policy bundle** so keys can be minted for it.

A server can only be published while its proxy is active on the Dashboard. A
proxy that disappears from the Dashboard is marked missing, unpublished, and
its keys are suspended; if the same proxy id comes back, the server resumes
and the keys are restored.

Presentation (name override, descriptions, logo, tags), privacy score,
catalogs and bundle are AI Studio's and survive every sync; everything derived from
the definition follows the Dashboard.

## Policy bundles and keys

Tyk policies are templates: a key carries policy ids, and Tyk applies the
policies' access rights and limits at request time. AI Studio never creates
a policy per App. Instead an administrator pins a **bundle** on each MCP
server, in the spirit of the Tyk Developer Portal's products and plans:

- exactly one **access policy** whose access rights name the proxy
  (unpartitioned, or carrying the `acl` partition), and
- any number of **consumption policies** carrying the `rate_limit` and/or
  `quota` partitions.

A key minted for an App carries the union of the bundles of every MCP
server the App uses on that connection. Editing a pinned policy on the
Dashboard changes every key at once, with no AI Studio involvement. A policy
with the `per_api` partition cannot be pinned next to anything else, which
is Tyk's own rule.

On a full-mode connection the **Create policy** button on the server page
writes a partitioned access or consumption policy to the Dashboard, tagged
`studio-managed`, and pins it in the same call. Those policies can be edited
from AI Studio; policies created elsewhere are linked to the Dashboard.

### Minting

Once an App that uses an MCP server is approved, its owner sees **Get access
key** on the App page, one per connection. Minting:

1. inserts the ledger row first, so a second mint for the same App and
   connection fails before any Dashboard call,
2. creates the key on the Dashboard with the desired policy ids, an alias,
   metadata naming the App and user, the `ai-studio` tag and the expiry from
   the connection's key defaults,
3. returns the key **once**, with the endpoint of every server it reaches
   and an `mcp-remote` snippet, in a response sent with
   `Cache-Control: no-store`.

The plaintext is never stored. AI Studio keeps the key hash (and, on a
Dashboard that runs unhashed keys, the plaintext id encrypted at rest,
because that is the only way to update or delete such a key). Administrators
can mint on a user's behalf, rotate, suspend, resume and revoke from AI
Portal → MCP credentials; every one of those is an `execute` action on the
`mcp-credentials` resource.

### Drift

A key's desired policies change when a bundle is re-pinned, the App gains or
loses a server on the connection, or a pinned policy vanishes. AI Studio
evaluates that on the spot and on every sync:

- **Narrowing** (a policy removed, a consumption policy swapped) is applied
  immediately and audited.
- **Widening** (an access policy that grants a proxy the key did not reach)
  waits as `pending_widen` for an administrator to apply, unless the person
  who caused it may already execute on credentials.
- A **vanished policy** is dropped; a key left without an access policy is
  switched off rather than left open.
- Policies someone added to the key on the Dashboard are preserved,
  reported as external, and never removed.

Revoking deletes the key and verifies the deletion with a read; where the
Gateway does not allow delete-by-hash, the key is switched off and the
connection remembers that for next time.

### Servers without keys

OAuth 2.1, external OAuth, JWT, mTLS and keyless proxies are catalogued and
published like any other, but AI Studio does not broker access to them: they
cannot be attached to an App (the App builder does not offer them and the
API refuses the binding), the portal shows no **Build app** button for them,
and the detail page tells the user how to connect directly, with the
protected-resource metadata, authorization servers and scopes, or that no
credential is needed. Consequently the access report lists key-backed access
only.

## Registering a proxy from AI Studio

On a full-mode connection, Context management → MCP servers → **Register
MCP server** walks through:

1. the connection and the kind: a **remote MCP server** (the gateway proxies
   an existing MCP endpoint) or **REST API to MCP** (Tyk 5.15 and later turn
   the operations of a Tyk OAS API into MCP tools),
2. the proxy: name, listen path, the upstream server's base URL (the
   gateway strips the listen path and appends `/mcp` itself, so a pasted
   `/mcp` suffix is removed) and an optional static upstream header, or the source API and the operations to expose as tools
   with their names and descriptions,
3. consumer authentication (API key, OAuth 2.1 with its authorization
   servers, or keyless after an explicit confirmation), the deployment
   target, and the portal presentation, privacy score and whether to
   publish,
4. a review: AI Studio renders the definition, checks it against the
   connection and shows the masked document, the endpoint and any warnings
   before creating it. The Dashboard validates it on create (Dashboard 5.14
   persists `dryRun` requests instead of validating them, so AI Studio never
   sends any).

The upstream header value travels to the Dashboard in the create request
and is stored nowhere in AI Studio. The definition AI Studio keeps masks it,
as it masks `upstream.authentication`, and the server page's editor works
on that masked copy: when a definition is pushed back, every masked value is
restored from the live document, the push is refused if the proxy changed
on the Dashboard since it was loaded, and a Dashboard-origin proxy needs an
explicit confirmation before AI Studio overwrites it.

Deleting a Studio-registered proxy removes it from the Dashboard; keys that
would be left without access are revoked when the administrator says so.
Proxies created on the Dashboard can only be unpublished here.

## Community submissions

Portal users submit an **MCP server** through Contribute like a tool or a
data source, naming the connection, the upstream (or the source API), the
consumer authentication, a suggested listen path and, where the connection
knows tags, a deployment target. The upstream credential is encrypted at
rest and redacted in every response, including the reviewer's.

The reviewer can **validate on the Dashboard** (a dry run; AI Studio never
contacts the submitter's upstream), change the deployment target, and
approve. Approval is two-phase because creating a proxy is neither
transactional nor idempotent:

- On a **full** connection the proxy is created (or, on a retry, a proxy
  already tied to the submission or already on the listen path is adopted),
  the submission records the Tyk api id, and only then is it marked
  approved. The server belongs to the submitter, with the reviewer's privacy
  score, and can be published in the same step.
- On a **catalogue** or **broker** connection that accepts handoffs, a
  server is recorded as *awaiting the platform team*, administrators are
  notified and a `system.mcp_server.registration_handoff` event is
  published (deliverable through [webhooks](webhooks.md)). The server page
  offers the **handoff package**: the ready-to-`POST` definition, the policy
  shape, the submitter's contact and the steps. The credential is masked
  unless someone with `mcp-servers: execute` downloads the package with it,
  which is audited. When the platform team has created the proxy and a
  sync has imported it, the page lists candidates on the same name or listen
  path and **Link** moves ownership, privacy score, catalogs and the
  submission onto the imported server.

## Access report

AI Portal → MCP credentials has two tabs: the **minted keys** ledger (App,
connection, key hint, status, applied and external policies, drift, expiry,
with rotate, suspend, resume, revoke and apply-change actions) and the
**access report**, one row per App and MCP server the App holds a key for,
with the App's owner and the key behind it, filterable by connection,
server, user and App, with closed grants on request. Servers AI Studio does
not broker (OAuth, mTLS, keyless) never appear: nobody can attach them to an
App.

## Permissions

| Resource | Group | Actions | Notes |
|---|---|---|---|
| `tyk-connections` | Settings | read, write, delete, execute | execute: activate, disable, probe, sync now. Sensitive and privileged. |
| `mcp-servers` | Context management | read, write, delete, execute, publish | write: presentation, catalogs, bundle; publish: portal visibility; execute: register, push, link, create policies, download a handoff with the credential. |
| `mcp-credentials` | AI Portal | read, write, delete, execute | execute: mint, rotate, suspend, resume, revoke, apply a widening change. Sensitive and privileged. |

Portal actions (binding servers, minting a key for one's own App) are
checked by ownership and team visibility, not by roles.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `TYK_MCP_ENABLED` | `true` | Master switch (can only disable the feature). |
| `TYK_MCP_SYNC_MIN_INTERVAL` | `60s` | Floor for every connection's sync interval. |
| `TYK_MCP_REQUEST_TIMEOUT` | `10s` | Dashboard reads; writes get three times this. |
| `TYK_MCP_RATE_LIMIT_PER_SECOND` | `10` | Outbound requests per connection. |
| `TYK_MCP_ALLOWED_HOSTS` | empty | Dashboard and MDCB hosts allowed (exact or `.suffix`); empty means any public host. |
| `TYK_MCP_DENIED_HOSTS` | empty | Hosts always refused. |
| `TYK_MCP_REQUIRE_DIFFERENT_ACTIVATOR` | `false` | Four-eyes on connection activation. |
| `TYK_MCP_SYNC_RUN_RETENTION` | `720h` | How long sync run records are kept. |

`TYK_AI_SECRET_KEY` must be set: connection tokens, MDCB secrets, unhashed
key ids and submitted upstream credentials are encrypted with it, and the
integration refuses to run without it.

Dashboard and MDCB URLs go through the same URL policy as webhook targets:
`http`/`https` only, no credentials in the URL, internal ranges and
`localhost`/`.local`/`.internal` refused unless the connection's
**allow internal host** flag is set (an `execute` action, shown as a
warning), re-checked against the resolved address on every connection.

## Local development

With the dev stack remapped so it can coexist with a Tyk Dashboard on port
3000 (`FRONTEND_PORT=3100 STUDIO_PORT=8090 make dev-ent`), create a
connection to `http://host.docker.internal:3000` with the Dashboard user's
access key, mode `full`, **allow internal host** on, and a gateway base URL
of `http://localhost:8080`. The probe should report MCP support and, on a
5.14 Dashboard, no REST-to-MCP support. Register a remote proxy that points
at an MCP server reachable from the gateway container, pin a bundle, publish
it, add it to a catalog a team is granted, and as a portal user build an App
with it and take a key:

```bash
npx mcp-remote http://localhost:8080/<listen-path>/mcp \
  --header "Authorization: <key>"
```

## Limitations

- AI Studio's chat and agents do not consume Tyk-managed MCP servers yet;
  the integration covers discovery, access and registration.
- One bundle per server; consumption tiers per server are a follow-on.
- The minimal policy creator writes plain access rights. Tyk's per-primitive
  policy fields for MCP are documented, but a Dashboard 5.14 rejects or drops
  them, so primitive-level rules are made on the Dashboard.
- Key rotation has no grace window: the old key stops when the new one is
  shown.
