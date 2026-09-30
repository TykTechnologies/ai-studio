# Embedding AI Studio

## Overview

AI Studio is being made importable as a Go library so another control plane
can run it in-process: the host constructs Studio, mounts its HTTP handler
under a path prefix, supplies identity and CSRF, and owns process-wide
concerns (configuration, logging, tracing, lifecycle). The standalone binary
becomes a thin wrapper over the same library.

The work lands in phases, each shippable on its own with standalone
behaviour unchanged:

| Phase | Scope | Status |
|-------|-------|--------|
| 1 | Configuration from a struct; injectable keys and paths; errors instead of process exits | Done |
| 2 | `pkg/studio` (`New`, `HTTPHandler`, `StartGRPC`, `StartProxy`, `Stop`); `ui` embed package; thin `main.go` | Done |
| 3 | Configurable base path for the backend | Done |
| 4 | Pluggable authentication and CSRF | Done |
| 5 | UI served under a base path; host-authentication UI mode | Done |
| 6 | Shipping the built UI to importers | Done |

Decisions that shape the design:

- **Own database.** Studio keeps its own database or schema (`DATABASE_SCHEMA`,
  see "Sharing the host's Postgres"); the host opens it
  with `studio.OpenDatabase(conf)` and passes the result as `Options.DB`, so
  the host never names Studio's gorm. Studio builds with its own copy of
  gorm (`third_party/gorm.io`), which a host's `replace gorm.io/gorm` cannot
  reach; see "gorm isolation" below. Studio's table names are not prefixed.
- **Host-authoritative identity.** The host authenticates the user and Studio
  provisions a matching user on first sight, keeping its own RBAC and groups.
- **One instance per process.** Package-level state that clashes with a host
  is removed; internal singletons stay, and a second instance is refused.

## Configuration (Phase 1)

`config.AppConf` holds everything Studio reads from the environment.

| Function | Behaviour |
|----------|-----------|
| `config.Load(envFile)` | Builds an `AppConf` from the environment, with `envFile` (default `.env`) filling unset variables. Neither caches nor writes to the environment. |
| `config.LoadFrom(getenv)` | Builds an `AppConf` from any lookup function, applying the same defaults. A host uses it to build configuration from its own settings. |
| `config.Set(conf)` | Installs `conf` as the configuration `config.Get` returns. |
| `config.ExportEnvFile(envFile)` | Copies `.env` values into the environment. Only the standalone binary calls it, so packages that still read the environment directly see file values. |
| `config.Get(envFile)` | Returns the installed configuration, loading it from the environment (after `ExportEnvFile`) on first use. |

Settings that were read from the environment at the point of use and now live
on `AppConf`: `SecretKey` (`TYK_AI_SECRET_KEY`), `MicrogatewayEncryptionKey`,
`CSRFTrustedOrigins`, `ExportStoragePath`, `BrandingStoragePath`,
`DebugHTTP`, `ChatUIV2Enabled` and `ChatSessionIdleTTL`. The chat queue's
Postgres listener falls back to `AppConf.DatabaseURL` when the database it was
given carries no DSN (a host that opened it from a `*sql.DB`).

Process-wide values are installed with setters that fall back to the
environment when unset:

- `secrets.SetEncryptionKey(key)` for the key that encrypts secrets at rest.
- `services.SetBrandingStoragePath(path)` for branding assets.
- `grpc.Config.EncryptionKey` for the key edges decrypt credentials with.

Tuning and debug settings shared with the microgateway (same variable names)
are on `AppConf` too: `AnalyticsBufferSize`, `BudgetSyncInterval`,
`DebugHTTPProxy`, `MetricsNoLegacyNames`, `CORSAllowedOrigins`,
`SkipFilterDefaults` and `FilterLimits` (the Enterprise filter-script limits,
by variable name: `config.FilterLimitNames`). `pkg/studio` hands them to their
packages (`analytics.SetBufferSize`, `metrics.SetLegacyNames`,
`corsutil.SetAllowedOrigins`, `grpc.Config.BudgetSyncInterval`,
`proxy.Config.DebugHTTPProxy`); the packages read the environment only when
Studio has not set them (the microgateway). `config.Installed` returns the
installed configuration without loading one, for code shared with the
microgateway. The deferred Postgres chat queue uses `AppConf.DatabaseURL`
before `DATABASE_URL`.

Deliberately still environment-only: the network and plugin security knobs
(`ALLOW_INTERNAL_NETWORK_ACCESS`, `PLUGIN_COMMAND_ALLOWLIST`,
`PLUGIN_BLOCK_INTERNAL_URLS`, the plugin allowed directories, `pkg/netguard`),
the filter-script switch `FILTER_SCRIPT_ALLOW_OS`, OCI registry credentials
(`OCI_PLUGINS_REGISTRY_*`, which reference other variables by name), and
`$ENV/` secret references.

## Errors instead of exits (Phase 1)

- `api.New` returns an error where the API constructor used to exit (SSO
  initialisation, CSRF key generation, frontend file systems). `api.NewAPI`
  remains as a panicking wrapper for tests. `api.New` leaves `gin.SetMode` to
  the caller.
- `grpc.NewControlServer` returns an error for a missing, short or default
  microgateway encryption key.
- `edition.CheckRegistered` (package `services/edition`) returns an error
  naming any enterprise feature an enterprise build did not register. Call it
  at startup; the per-feature factories would otherwise panic on first use.
- Enterprise licensing: `Start` returns an error when the licence fails
  validation at boot. A failed periodic re-check calls
  `licensing.Config.OnInvalid`; with no callback set the process exits, which
  is what the standalone binaries rely on.
- The licence from the host (S1): a host embedding Studio passes the
  customer's **AI Studio Enterprise licence** (the same JWT as
  `TYK_AI_LICENSE`, validated unchanged: two licences, the host's own and
  Studio's; no licence generator changes and no host licence claims).
  `studio.Options.License` (`licensing.Config.LicenseSource`) supplies it
  from the host's settings and is read at start and at every validity
  check, so a renewal takes effect without a restart;
  `(*studio.Studio).ReloadLicense()` validates a newly stored licence at
  once (an error leaves the held licence in place, for the host to show).
  `(*studio.Studio).LicenseStatus()` reports validity, expiry, days left
  and entitlements for the host to show next to its own licence. Nil
  `License` uses `Config.LicenseKey`.
- Email templates are embedded (package `templates`). A `templates/` directory
  in the working directory still takes precedence, so deployments can
  customise them, but a process started elsewhere renders the defaults
  instead of failing.

## The studio package (Phase 2)

`pkg/studio` holds the wiring that used to live in `main.go`; `main.go` is
now a thin wrapper (flags, `config.Get`, logger, connectivity checks, opening
the database with `studio.OpenDatabase`, the docs server, signal handling). See `pkg/studio/README.md`
for the host-facing API.

- `studio.New(Options)` installs the configuration, checks the edition,
  migrates and seeds the database, starts licensing, the service layer,
  marketplace sync, scheduler, analytics, telemetry and plugins, and builds
  the API, gateway and (in control mode) the gRPC control server. Every
  failure is a returned error, and a failed `New` stops whatever it started.
- `HTTPHandler`, `ListenAndServe`, `ProxyHandler`, `StartProxy` and
  `StartGRPC(listener)` serve it. `Stop(ctx)` shuts down the API, gateway,
  gRPC server, plugins and workers, analytics, tracing and licensing in that
  order, and leaves the database open. Previously `Service.Cleanup` closed
  the database before the deferred stops in `main` ran; `Service.Stop` now
  does everything but close it, and `Cleanup` is `Stop` plus the close.
- One Studio runs per process (`ErrAlreadyRunning`); after `Stop`, `New` may
  build another.
- Host-owned telemetry: `Options.Logger` (`logger.Use`),
  `Options.TracerProvider`/`Propagator` (`tracing.Use`) and
  `Options.MeterProvider` (`metrics.InitWithProvider`) keep Studio off
  zerolog's and OpenTelemetry's globals.
- `Options.AnalyticsSinks` ships analytics into the host's own pipeline.
  `analytics.Tee` wraps Studio's database handler, which always stays
  (budgets and spend read `llm_chat_records`), and gives each sink a copy of
  every record (chat records, proxy logs, tool calls, compliance events and
  edge batches; a request's proxy log and chat record reach an
  `ExchangeRecorder` sink together). Each sink has a bounded queue
  (`analytics.TeeQueueSize`, 10000) drained by its own goroutine, so a slow
  sink loses records, counted in `Tee.Dropped` and logged, instead of
  slowing requests, and a panicking sink is recovered. `Stop` gives the
  sinks up to 5 seconds to drain and puts the database handler back as the
  process-wide handler.
- Package `ui` embeds the built frontend (`ui.FS`, rooted at the build
  directory); `api.New` takes it as an `fs.FS`, and `Options.UIAssets`
  overrides it.
- `enterprise/all` (`github.com/TykTechnologies/ai-studio-enterprise/v2/all`)
  imports every enterprise feature; `main_enterprise.go` and enterprise hosts
  import it. The enterprise module is named after its own private repository,
  so a host with read access fetches it like any module; under the old path
  (`midsommar/v2/enterprise`, a directory of this repository that is really a
  submodule) Go could never resolve it.
- Enterprise features register themselves with core through hooks (feature
  factories, `scripting/engine.Register`, the guardrails registry), and
  `services/edition.CheckRegistered` fails `New` when one is missing. No
  public package imports the enterprise module, not even behind the
  `enterprise` build tag, because `go mod tidy` in a host considers every tag
  and would try to fetch the private module; `make enterprise-import-guard`
  (CI) enforces it. Tests may not import it either, because tidy also reads
  the tests of every package it imports: enterprise tests of core packages
  live in the enterprise repository (`enterprise/_coretests`) and
  `make ent-link` links them in. Only the `main_enterprise.go` files, the
  microgateway module and `tests/`, none of which a host imports, may import
  it.
- `grpc.ControlServer.Serve(listener)` serves on a host-supplied listener;
  `API.Shutdown` stops the audit writer even when the host served the router.

## Base path (Phase 3)

`BASE_PATH` (`AppConf.BasePath`, normalised to `/prefix` or `""`) serves the
API and UI under a path prefix. `SITE_URL` (and `AUTH_SERVER_URL`, which
defaults to it) should include the prefix: email links are built from it,
and a warning is logged when it does not end with the base path.

- Routes stay registered at the root. `API.Handler()` (what
  `studio.HTTPHandler` and the standalone server serve) strips the prefix;
  a request without it passes through unchanged, so a reverse proxy may
  strip it first. The bare prefix redirects to `prefix/`.
- The session cookie (`auth.Config.CookiePath`) and the CSRF cookie are
  scoped to the base path.
- Logout expires Studio's session cookie and the identity broker's
  `_gothic_session`, and nothing else (it used to expire every cookie on the
  request, which would sign a user out of the host too). Behaviour change
  for standalone Studio too: logout no longer expires the CSRF cookie
  (`_gorilla_csrf`, or `CSRF_COOKIE_NAME`). The cookie carries no
  session, so this is intended and not exploitable.
- Post-SSO redirects and the email-verified page's redirect go to the base
  path instead of `/`.
- OAuth: the consent redirect and the authorization server metadata
  endpoints are the origin of `SITE_URL`/`AUTH_SERVER_URL` plus `BASE_PATH`
  (`API.publicURL`). A path in `SITE_URL` without `BASE_PATH` is ignored, as
  in v2.2.0, so a standalone Studio served from the root keeps root URLs;
  the issuer is still `AUTH_SERVER_URL` verbatim. RFC 8414 discovery for a pathed issuer
  happens at the host root (`/.well-known/oauth-authorization-server/<base>`);
  `studio.OAuthMetadataHandler()` serves it there. The gateway's protected
  resource metadata is unchanged: the gateway keeps its own port.
- The SPA fallback injects a `<base>` element and the frontend bootstrap
  into `index.html` (see Phase 5). `/auth/config` returns `basePath`.
- The resend-verification email linked to `/verify-email`, which nothing
  serves; it now links to `/auth/verify-email` like the registration email.


## Host authentication and CSRF (Phase 4)

`studio.Options.Auth` (an `auth.Authenticator`) lets the host authenticate
every request: `Authenticate(r)` returns the signed-in user as a
`studio.Identity` (`services.HostIdentity`: subject, email, name, admin,
groups, roles), nil when the request has no host identity, or an error to
reject it.

- It runs first in `auth.GetAuthenticatedUser`, so `AuthMiddleware`, RBAC,
  the audit trail (`auth_method = host`) and every handler reading `"user"`
  work unchanged. With no host identity, Studio's API keys still
  authenticate the request; a host error or an identity that cannot be
  provisioned ends it as unauthenticated.
- `Service.ProvisionHostUser` finds the user by `users.external_subject`
  (unique among live users: a partial index that soft-deleted rows do not
  hold). On first sight it links an existing account with the same email
  and no subject, or creates one (`auth_source = host`, verified, portal and
  chat on, a random unusable password). Name, email, administrator status
  and, when `Groups` is not nil, group memberships (by name, always keeping
  Default, unknown names skipped) follow the host through `UpdateUser`, so
  plugin hooks, Enterprise role bindings (admin is an Administrator binding)
  and group rules apply. Unchanged identities write nothing but a login
  stamp at most every 15 minutes. Disabled users are refused, and an email
  linked to another subject is a conflict.
- **Roles (Enterprise).** `Roles` names Studio roles by slug (`editor`,
  `viewer`, a custom role's slug). They become host-managed role bindings
  (`role_bindings.source = 'host'`, direct and global): added and removed
  as the host says on each sign-in (nil leaves them, an empty list removes
  them; unknown slugs are logged and skipped). Roles an administrator
  assigns in Studio are never touched, and a role the user already holds
  that way stays with that binding. Studio's administration shows host
  roles locked (`via: "host"` on the user's roles), keeps them when an
  administrator saves the user's roles, and refuses to delete one
  (`409`). `Admin` still decides the Administrator role, compared with the
  user's direct Administrator (or Owner) binding: a team granting
  Administrator is not a difference to write on every request. The host
  cannot remove the last Owner, by `Admin` or `Roles`: the Owner role is
  kept and logged, and the user is still signed in.
- Host users are externally managed like SSO users
  (`User.IsExternallyManaged`): no API key unless
  `ALLOW_SSO_USER_API_KEYS`, and an issued key lapses once the user stops
  signing in through the host (`SSO_API_KEY_LIVENESS`).
- With `Auth` set, Studio's own sign-in is off: password login,
  registration, password reset, email verification and every SSO route
  answer 404, and the identity broker (whose library keeps process-wide
  state a host running its own broker would clash with) is not started.
  `/auth/config` reports `authMode: "host"` with `loginURL`/`logoutURL`
  from `Options.LoginURL`/`LogoutURL`.
- `Options.CSRF` (`func(http.Handler) http.Handler`, calling the wrapped
  handler only for requests that pass) replaces Studio's gorilla/csrf
  protection; requests with an `Authorization` header and `/oauth/` stay
  exempt as before. `/auth/config` reports `csrfTokenHeader` and
  `csrfTokenURL` (defaults `X-CSRF-Token` and `<base>/csrf-token`, or
  `Options.CSRFTokenHeader`/`CSRFTokenURL`).
- Studio's own CSRF key can now be stable across restarts and replicas:
  `CSRF_KEY` (the token key is derived from it). `CSRF_COOKIE_NAME`
  renames the cookie when a host on the same domain also uses gorilla's
  default.
- The console labels host-provisioned users ("Host application") and can
  filter by that origin. Using `authMode` (redirecting to the host's login,
  hiding login and SSO pages) is Phase 5.

`pkg/studio` tests run on Postgres, one schema per test, when
`STUDIO_TEST_POSTGRES_DSN` is set.

## Frontend under a base path, and host sign-in (Phase 5)

One frontend build serves any base path:

- It is built with a relative public path (`"homepage": "."`, and
  `PUBLIC_URL="."` in the Dockerfile and the release, prod and benchmark
  builds), so `index.html` loads `./static/...` and the CSS refers to
  `../../static/media/...`.
- The server injects `<base href="<base>/">` (`<base href="/">` at the
  root) so those relative URLs resolve from any client-side route, and
  `window.__TYK_AI_STUDIO__`: `basePath`, `authMode`, `loginURL`,
  `logoutURL`, `csrfTokenHeader`, `csrfTokenURL` (`api.frontendBootstrap`,
  the same values `/auth/config` reports). It is read synchronously, so it
  applies before the first request.
- `src/runtimeConfig.js` reads it. `withBase(path)` puts an absolute path
  under the base; `stripBase(pathname)` turns a browser path into a route.
  The router gets `basename`; `apiClient` (`/api/v1`) and `pubClient` (via
  `getBaseUrl`) carry the base, so the hundreds of client calls and router
  links need no change. Everything the browser loads directly goes through
  `withBase`: logos, branding, `window.location`/`window.open`,
  `fetch`/`EventSource`, the OAuth consent page, plugin iframes and remote
  entries. Comparisons against `window.location.pathname` use `stripBase`.
- `src/basePathGuard.test.js` fails on new root-absolute URLs in those
  places.
- CSRF tokens are fetched from `csrfTokenURL` and sent in `csrfTokenHeader`.
- Host sign-in: with `authMode: "host"` a signed-out visitor goes to
  `loginURL` (the login route shows a pointer to it), and logout goes to
  `logoutURL` after Studio's own sign-out.
  - Deep links: the console replaces `{return_to}` anywhere in `loginURL`
    (`Options.LoginURL: "/login?next={return_to}"`) with the page the
    visitor asked for, URL-encoded: the browser path under the base path,
    with query and hash (`%2Fai-studio%2Fadmin%2Fllms%3Ftab%3Dkeys`). From
    the login or password pages it is the console's home. The host must
    check it is a local path under the base path before redirecting to it
    (`examples/embed-host` `returnTo`). A `loginURL` without the
    placeholder is used unchanged, so existing hosts see no difference.
    This applies to the first visit and to a session that expires later
    (`authRedirect.hostSignInURL`). Standalone Studio is unchanged: it goes
    to `/login` and, as in v2.2.0, lands on the user's home after sign-in.
  - Local-account pages: `/register`, `/forgot-password`,
    `/reset-password` (and `/auth/reset-password`) lead to the console's
    home, and from there to sign-in, instead of rendering forms whose APIs
    answer 404. `/common/me` reports `show_sso_config: false`, so the
    `/admin/sso-profiles` routes are not registered, and the navigation
    manifest leaves out Identity providers (both through
    `api.showSSOConfig`).
- Fixed on the way: the admin plugin iframe loaded `/plugins/assets/...`,
  which no route serves (now `/api/v1/plugins/assets/...`). "Mark plugin
  UI loaded" posted to a doubled `/api/v1/api/v1/...`, which the SPA
  fallback answered; with the path fixed it reached
  `POST /api/v1/plugins/:id/ui/load` (plugins:write), so read-only users
  saw a permission-denied toast on every plugin page and an administrator's
  page view wrote `registered_plugins`, an audit entry and a config-sync
  refresh. Nothing reads the "loaded" state, so the console no longer
  posts it: viewing a plugin page writes nothing, as in v2.2.0. The
  endpoint stays for API clients.
- `config/docs_links.json` is embedded (an on-disk copy still overrides it),
  so documentation links work from any working directory.

`examples/embed-host` is a runnable host: its own login page and cookie, an
`Authenticator` over that cookie, Studio at `/ai-studio`, and the OAuth
discovery document at the host root. `-tags enterprise` builds the
Enterprise Edition (`examples/embed-host/main_enterprise.go`, allowed by
`make enterprise-import-guard` like the root `main_enterprise.go`, since a
main package is never imported); `-proxy-port` sets the gateway port.

`config.LoadFrom` (a host's settings) does not log the standalone binary's
"environment variable is not set" notices, and leaves the docs link out
(`DocsURL` empty, `DocsDisabled` set) unless `DOCS_URL_OVERRIDE` is given:
the documentation site server runs only in `main`. `config.Load` and
`config.Get` (the environment) behave as before.

Verified by hand in a browser: the standalone binary with
`BASE_PATH=/ai-studio` (registration, login, admin, portal and chat,
deep-link reloads, logout; every request stayed under the prefix), the same
build at the root, and `embed-host` (host login, provisioning as an
administrator, logout through the host).

## UI assets for importers (Phase 6)

`go:embed` needs the built frontend at compile time, and
`ui/admin-frontend/build` is not committed, so a host that imports Studio
as a module cannot compile the default `ui` package.

- `ui` embeds `admin-frontend/build` by default (`ui/embed.go`). With the
  `studio_noui` build tag (`ui/noui.go`) it embeds only `ui/noui/index.html`,
  a tracked placeholder page, and `ui.Embedded` is false. `pkg/studio` then
  expects `Options.UIAssets` and logs a warning without it. `pkg/studio`
  imports nothing else that embeds uncommitted files (the docs site server
  is only in `main`).
- The `ui-assets` job in `release.yml` builds the frontend once per tag,
  packs it as `tyk-ai-studio-ui-<tag>.tar.gz` with a `.sha256`, and uploads
  both to the tag's GitHub release. When there is none it creates it,
  published (not a draft; a prerelease for a `-rc` tag, never marked latest)
  with placeholder notes, and it publishes a draft left by an earlier run. It
  is the only job with `contents: write`. The release notes are then added
  by hand with `gh release edit <tag> --notes-file notes.md` (plus
  `--latest` for a final release); `gh release create <tag>` fails with
  "already exists" once the job has run.
- A host builds with `-tags studio_noui`, unpacks the tarball for the Studio
  version it imports, and passes `os.DirFS(dir)` (or its own embed of the
  directory) as `Options.UIAssets`. `examples/embed-host -ui <dir>` does
  this.

Verified: with `ui/admin-frontend/build` moved away, the default build of
`pkg/studio` fails on the embed pattern and the `studio_noui` build of
`pkg/studio` and `examples/embed-host` succeeds; that `studio_noui` host,
given a tarball made as the release job makes it, serves the full console
under `/ai-studio`.


## Pages without Studio's chrome

A host that draws its own navigation (the Tyk Dashboard's top bar and
sidebar) sets `Options.Chromeless`:

- The bootstrap (`window.__TYK_AI_STUDIO__`) and `/auth/config` carry
  `chrome: "none"` (`"full"` otherwise).
- `layouts/MainLayout.js` then leaves out `TopNavigation` (the Admin /
  Portal / Chat switch and the user menu) and the admin, portal and chat
  drawers; pages take the full width.
- Sticky page headers sit below `--studio-header-height`, a CSS variable
  that defaults to the 64px top bar (`index.css`) and that
  `runtimeConfig.applyChrome` sets to 0 when chromeless. Pages used to
  hard-code `top="64px"`.
- The host links straight to Studio's routes under the base path
  (`/admin/llms`, `/portal/dashboard`, `/chat/...`), and builds its menu
  from `GET /common/nav` (below).

### Navigation manifest

`GET /common/nav` (`api/nav.go`) returns what the signed-in user may
navigate to:

- `surfaces`: Admin (`/admin`, when the user holds any permission), AI
  Portal (`/portal/dashboard`) and Chat (`/chat/dashboard`), gated as the
  console's top bar gates them (the user's show-portal / show-chat options
  and the licensed features).
- `admin`: the admin menu, as groups (`items`) of pages, each with `id`,
  `text`, `path` (a console route without the base path), `icon` (a Font
  Awesome name), `exact`, and the `permission` that unlocks it. Plugin
  sections carry `pluginId`. Entries the user may not open are already
  left out, with the drawer's rule: a group stays while one of its pages
  does. Empty for a user with no admin permissions.
- `portal`: Overview, Apps, Browse (the catalog, one entry per asset type
  with its feature, and one per plugin resource type the user has
  instances of), Community, then the portal plugin sections the user's
  teams may see (a one-page section links straight to its page). Empty
  unless the user may use the portal and the portal or gateway is
  licensed.
- `chat`: Overview, the chat rooms the user is entitled to, their five
  most recent conversations (with "View all conversations"), and the
  active agents they may talk to (public ones and those shared with one of
  their teams), sorted by name. Empty groups are left out. Empty unless
  the user may use chat and chat is licensed.

Each lookup behind a menu (plugin sections, resource types, chats, history,
agents) fails on its own: it is logged and costs its entries, never the
manifest.

The manifest is the one source of truth: the console's admin, portal and
chat drawers render from it (`useNavManifest`), reloading when the user's
permissions change or a plugin UI is installed. Group order, feature gates (portal, chat,
gateway-only, Enterprise-only groups) and plugin placement are tested in
`api/nav_test.go`. `TestNavGolden` writes the full menus to
`ui/admin-frontend/src/admin/nav.golden.json` (`UPDATE_NAV_GOLDEN=1` to
regenerate), and `nav.golden.test.js` checks every page in it against the
console's routes (`admin/routes.js`, `routes/PortalRoutes.js`,
`routes/ChatRoutes.js`), and for admin pages that the menu and the route
need the same permission.
- `examples/embed-host -chromeless` shows it.

## Module layout

A host imports the root module only. The root module does not require the
microgateway module: the gateway plugin interfaces and SDK that `pkg/plugin_sdk`
uses live in `pkg/gatewayplugin/{interfaces,sdk}`, and the gateway management
gRPC API is generated from `proto/microgateway_management.proto` into
`proto/microgateway_management`. The old paths under `microgateway/plugins/`
and `microgateway/proto/microgateway_management` are deprecated forwarding
packages (type aliases and wrappers, generated when the code moved) so that
existing plugins keep compiling. The proto package name is unchanged, so the
wire format and gRPC method names are the same.

`microgateway/go.mod` requires `midsommar/v2` at a pseudo-version of a main
commit that has `pkg/gatewayplugin` (the local `replace ../` still applies
to in-repo builds). It used to require `v2.0.0`, whose module zip the proxy
cannot build (v2.0.0 and v2.2.0 both committed files with `:` in their
names), so a plugin importing the old paths could not be fetched through the
proxy unless it also pinned `midsommar/v2` itself. Raise the requirement to
the release tag when the next one is cut.

## Releases a host can import

Every `v*` tag is a version of `github.com/TykTechnologies/midsommar/v2` a
host can `go get` (the module proxy builds its zip from the tagged tree;
`make module-check` keeps that tree valid). `release.yml` then:

- builds the enterprise edition from the commit the `enterprise` submodule
  pins, not from the enterprise repository's current main;
- tags `github.com/TykTechnologies/ai-studio-enterprise` with the same
  version on that commit (`scripts/release/tag-enterprise.sh`), refusing a
  pin that is not on enterprise main, so an enterprise host requires both
  modules at one version;
- runs `scripts/release/consume-module.sh` for both editions: a throwaway
  host with a clean module cache imports the tag through the proxy, runs
  `go mod tidy` and builds with `CGO_ENABLED=0`. The Community Edition run
  has no credentials at all, and runs even when `enterprise-tag` fails (the
  enterprise run then fails at once, naming it). The proxy can take a while
  to see a new tag, so `go get` retries with a doubling backoff (15 s up to
  5 min) for up to 30 minutes (`CONSUME_MODULE_DEADLINE`, in seconds).

The same script checks any commit by hand, e.g.
`scripts/release/consume-module.sh ce <commit>`.

## Host version floor

When a host imports Studio, Go's version selection picks, for every module,
the highest version any `go.mod` in the build requires. So each Studio
requirement above the host's own upgrades the host silently, and a host
dependency newer than Studio's is what Studio actually runs with there.
Studio's `go.mod` (and the enterprise module's) therefore follows the Tyk
Dashboard's, and is checked against Tyk MDCB's (`tyk-sink`) as well, which
embeds Studio as a headless control plane:

- Where both require a module, Studio uses the Dashboard's version, up or
  down (2026-09-30: TIB 1.8, libopenapi 0.36, gorilla/sessions 1.4,
  go-redis 9.18, nats 1.49, the AWS and Google SDKs and more up;
  gosimple/slug and mergo down). The `go` directive matches the Dashboard's
  (`go 1.26.5`, `toolchain go1.26.6` for Studio's own builds).
- Where a Studio dependency needs a newer version, the module is in the
  host's allowlist, `scripts/host-compat-allow.<repo>.txt`
  (`tyk-analytics`, `tyk-sink`), with the dependency that needs it (the
  OpenTelemetry 1.46 exporters, the Prometheus client behind the otel
  Prometheus exporter, go-openapi v0.25+, weaviate). Each was checked by
  lowering it alone: every one drags others down with it. One raise is a
  choice rather than a need: pgx stays at 5.10 (v2.2.0's version, for its
  hardening against hostile servers) above the Dashboard's 5.9.2.
- MDCB (checked 2026-10-01) lags on a subset of the same modules (the otel
  exporters, the Prometheus client 1.21.1, pgx 5.9.2, go-openapi,
  jsonparser), so its allowlist holds that subset with the same reasons.
  Where MDCB is lower than the Dashboard and nothing in Studio's graph
  needs more, Studio follows MDCB (`golang.org/x/exp`).
- `mattn/go-sqlite3` is deliberately not aligned: the Dashboard carries the
  retracted `v2.0.3+incompatible`, and `pkg/studio` does not link SQLite.
- The in-repo plugin modules (`examples/`, `enterprise/plugins/`, and the
  `community/` and `tyk-internal/` submodules) replace Studio's module with
  the checkout, so any version change here needs `go mod tidy` in each of
  them too, or their `go build` stops at "updates to go.mod needed".
  `make plugins-mod-check` checks them (CI covers `examples` and
  `enterprise/plugins`; the submodules are separate repositories).

`make host-compat` (`scripts/host-compat.sh --build`, a CI job per host on
this repository's branches: "Host Compatibility (Dashboard)" and "(MDCB)")
fetches the `go.mod` of each repository in `HOST_REPOS` (default
`tyk-analytics tyk-sink`) at run time (they are private; never commit a
copy), checks each in turn, fails if any fails, and for each host:

1. fails if Studio or the enterprise module requires anything above the
   host's version that the host's allowlist does not name
   (`tools/hostcompat`, no network);
2. builds `pkg/studio` for both editions inside the host's module
   graph, its requirements and replaces included, with `CGO_ENABLED=0`, and
   lists every module that ends up above the host's `go.mod`. That list
   also shows raises from the `go.mod` files of Studio's dependencies, which
   the first check cannot see; most come from
   `github.com/weaviate/weaviate`, the server module, of which Studio only
   uses `entities/models`.

`make host-compat HOST_REPOS=tyk-sink` checks one host;
`scripts/host-compat.sh [--build] path/to/go.mod` checks a local copy
against `HOST_ALLOW` (default the Dashboard's allowlist).

A host that never builds the enterprise edition can build, tidy and verify
Studio without access to the private enterprise module, but `go list -m
all` fails there: it resolves every module in the graph, including the
enterprise module Studio's `go.mod` names at a placeholder version. The
Dashboard always builds the enterprise edition, requiring a real version.

## Sharing the host's Postgres

A host can give Studio a schema in its own database instead of a database of
its own: `Config.DatabaseSchema` (`DATABASE_SCHEMA`) makes
`studio.OpenDatabase` create the schema when missing and pin `search_path` to
it alone (not `schema,public`: Postgres skips missing entries and gorm's
migrator creates tables in `current_schema()`, so a fallback would put
tables in `public`). Studio's unprefixed table names (`users`, `roles`,
`audit_records`, `tyk_policies`, ...) then cannot collide with the host's.
Raw SQL in Studio never qualifies a schema, and the schema snapshot goldens
are schema-independent, so nothing else changes.

`studio.New` runs every migration and seed, from `models.InitModels` through
the RBAC seed (plus the analytics and identity broker tables, which used to
migrate later), under `models.AcquireMigrationLock`: a Postgres
transaction-level advisory lock (`pg_try_advisory_xact_lock`, polled every
500 ms) held by an open transaction on a connection of its own, keyed by
the current schema, so replicas sharing one schema take turns and different
schemas do not wait for each other. On SQLite, or with a pool of one
connection, it is a no-op. Tests: `pkg/studio/database_schema_postgres_test.go`,
`models/migration_lock_postgres_test.go`.

Nothing else migrates. The analytics recorder used to run
`analytics.Migrate` when it started, outside the lock, and
`grpc.NewControlServer` started a recorder of its own (on a context that was
never cancelled); both are gone, so starting the recorder or the control
server runs no DDL. A host that records analytics without `studio.New`
calls `analytics.Migrate` itself, under its own lock.

It started as a session-level lock. Behind PgBouncer in transaction mode
that leaked: the lock stayed on whichever pooled server connection took it,
the unlock ran on another one, and every later instance waited for ever.
A transaction keeps one server connection until it ends, and ending it (or
the server dropping the session) releases the lock. The holder runs
`SELECT 1` in its transaction every 5 s, so a server's
`idle_in_transaction_session_timeout` does not end it mid-migration; if the
lock is lost anyway, a warning is logged and the migration carries on. The
wait is bounded: `Config.MigrationLockTimeout` (`MIGRATION_LOCK_TIMEOUT`,
default 15 min), after which `New` fails with an error naming the lock. Concurrent unlocked
boots of a fresh schema did not fail in tests (the seeds are protected by
unique constraints), so the lock is a guard for upgrades, where replicas
starting together would run the same ALTERs and backfills, rather than for
an observed race.

### Schema version and `studio.CheckSchema`

The last step under the migration lock records the schema in `studio_schema`
(one row, `models.RecordSchemaVersion`): `version` (`models.SchemaVersion`),
`min_reader_version` (`models.MinReaderSchemaVersion`, the oldest schema
version whose code can still read this one), the Studio version that wrote
it and when. It never lowers the record: an older Studio started against a
database a newer one migrated keeps the newer version (its own migrations
only add), and logs a warning.

An instance that must not migrate the database, such as a headless control
plane sharing it with a full Studio, calls `studio.CheckSchema(ctx, db)`
first. It only reads (no DDL) and fails with:

- `studio.ErrSchemaMissing`: no record; no Studio of this generation has
  migrated the database yet.
- `studio.ErrSchemaTooOld`: the record's `version` is below this build's
  `SchemaVersion`; upgrade the full Studio first.
- `studio.ErrSchemaTooNew`: the record's `min_reader_version` is above this
  build's `SchemaVersion`; a newer Studio made a change this build cannot
  read, so upgrade it.

A newer schema that still lists this build as a reader is accepted, so the
headless instance may lag the full one across additive migrations. Every
schema change bumps `SchemaVersion`; `MinReaderSchemaVersion` rises only for
a change that breaks older readers (the rules are next to the constants in
`models/schema_version.go`). `models/testdata/schema/VERSION` records the
version and a hash of the schema goldens: `TestSchemaVersionMatchesGoldens`
and `make schema-golden` fail when the goldens change without a bump. The
goldens cover `models.InitModels` (with the profile and KV tables), which
holds the Enterprise tables too, and the analytics tables
(`models.AnalyticsModels`, which `analytics.Migrate` creates): a control
plane that does not migrate still writes those.

## Several replicas

A host may run several Studio replicas against one database. Each joins
the cluster in `studio.New` (`Options.NodeID`, default a fresh per-process
ID): a registry row other replicas use to tell live replicas from dead ones,
an event log and bus relay for what every replica must hear, and a claim on
the leader lease for work that must happen once. Code that is not handed
the cluster (Enterprise features, say) uses `pkg/replicas`: `IsLeader`,
`OnLeading` (catch up on leader-only work skipped before the lease was
held), `Signal` and `OnSignal`. See `features/ClusterControlPlane.md` for the
guarantees, and the reference architecture for what a deployment must
provide (session affinity, shared files).

## The host's logger, and panics

`Options.Logger` (`logger.Use`) makes the host's `zerolog.Logger` the one
Studio's own logging goes through; Studio leaves zerolog's global logger and
level alone. Every line the edge control plane writes reaches it: the gRPC
control server and budget sync, `pkg/cluster`, `pkg/pglisten`, edge pushes,
analytics recording, `secrets`, `pkg/safe` and `pkg/studio`.
`make logging-guard` (CI) keeps zerolog's global logger, `log/slog` and the
standard library's `log` out of those packages. Other packages (the proxy,
the API, `services/grpc`, ...) still partly log through zerolog's global
logger, which is the host's to configure.

A host that logs with logrus (MDCB) passes a zerolog logger over a writer
into logrus. zerolog hands a `zerolog.LevelWriter` each line, one JSON
object, with its level:

```go
type logrusWriter struct{ l *logrus.Logger }

func (w logrusWriter) Write(p []byte) (int, error) { return w.WriteLevel(zerolog.InfoLevel, p) }

func (w logrusWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	var fields map[string]interface{}
	if err := json.Unmarshal(p, &fields); err != nil {
		w.l.Info(strings.TrimSpace(string(p)))
		return len(p), nil
	}
	msg, _ := fields[zerolog.MessageFieldName].(string)
	delete(fields, zerolog.MessageFieldName)
	delete(fields, zerolog.LevelFieldName)
	entry := w.l.WithFields(logrus.Fields(fields))
	switch {
	case level <= zerolog.DebugLevel:
		entry.Debug(msg)
	case level == zerolog.InfoLevel:
		entry.Info(msg)
	case level == zerolog.WarnLevel:
		entry.Warn(msg)
	default:
		entry.Error(msg)
	}
	return len(p), nil
}

zl := zerolog.New(logrusWriter{l: log}).Level(zerolog.DebugLevel).With().Timestamp().Logger()
studio.New(studio.Options{Logger: &zl, ...})
```

A panic in Studio's background work must not take the host down. `pkg/safe`
recovers them: the gRPC control server's interceptors (a panicking call
fails with `Internal`; a panic handling an edge's stream messages ends that
edge's stream, which reconnects), supervised restarts with backoff for the
long-lived loops (budget sync, cluster heartbeat, lease, event log, relay,
Postgres listener, edge pushes, analytics writer, replica signals), and
per-item recovery for event bus subscribers, listener handlers and replica
change handlers. Each is logged with its stack and counted in
`aistudio_goroutine_panics_total{goroutine}`. The panic log goes through
`logger.Current()`: the host's logger once `logger.Use` or `logger.Init`
ran, zerolog's global logger before that (the microgateway uses Studio's
event bus without setting Studio's logger up).

## Headless control plane (`studio.NewControlPlane`)

A product that holds edge (microgateway) connections for a region, such as
MDCB, runs Studio as a pure control plane next to the full Studio embedded
in the Dashboard, on the same Postgres database:

```go
cp, err := studio.NewControlPlane(studio.ControlPlaneOptions{
	Config:    conf,      // gRPC, encryption and licence settings
	DB:        sharedDB,  // the database the full Studio migrates (Postgres)
	Version:   hostVersion,
	Logger:    &hostLogger,
	TLSConfig: hostTLSConfig, // optional: the host's certificates and ciphers
	License:   func() string { return hostSettings.AIStudioLicence() },
})
if err != nil {
	return err // studio.ErrSchemaMissing / ErrSchemaTooOld / ErrSchemaTooNew, ...
}
defer cp.Stop(ctx)
go cp.Serve(edgeListener) // nil: Config.GRPCHost:GRPCPort
```

What it runs:

- the gRPC control server: edge registration, configuration snapshots,
  analytics pulses (recorded in the shared database, and copied to
  `AnalyticsSinks`), token validation;
- edge push delivery for the edges whose streams it holds (a push made on
  the full Studio is delivered by whichever replica holds the edge);
- cluster membership: a `cluster_nodes` row, the event log, replica signals
  (budget and governed-metadata caches), and the bus relay, which carries the
  full Studio's edge-bound events (`budget.sync`, configuration changes) to
  its edges, and its edges' events and plugin payloads to the full Studio
  (below);
- licence validity checks (Enterprise), without telemetry.

What it leaves out: migrations and seeds, the API and UI, the AI gateway,
authentication, plugins, the marketplace, the plugin scheduler, usage and
licence telemetry, and metrics or trace exporters (it uses the host's
`TracerProvider` and `MeterProvider` when given, and records nothing of its
own otherwise).

- **Postgres only.** SQLite serves one process; `NewControlPlane` returns
  `studio.ErrControlPlaneNeedsPostgres`.
- **No DDL.** It calls `studio.CheckSchema` first and never changes the
  schema: upgrade the full Studio before the control plane, which can lag it
  across additive migrations.
- **Never the leader.** It joins the cluster without contending for the
  leader lease, and `pkg/replicas.IsLeader` is false on it for good. So
  singleton work (budget blocks, alerts and `budget.sync`, marketplace sync,
  telemetry) stays with the full Studio, even while it is down: nothing
  takes that work over in a control plane. Its edges keep the last budget
  blocks they received until the full Studio is back.
- **Logger.** `Logger` nil logs JSON to stderr at `Config.LogLevel`, without
  touching zerolog's global logger.
- **One instance per process**, shared with `New`: a Studio and a control
  plane cannot run in one process. After `Stop`, either may start again.
- **Edge-to-control plugin traffic reaches the full Studio's plugins.** A
  control plane runs no plugins, so what its edges send for them goes on:
  - events edges publish `DirUp` are relayed through the cluster event log
    to every full replica, whose plugins get each once (as `DirLocal`, like
    their own edges' events; never sent down to any edge);
  - plugin payloads (`SendPluginControlBatch`, `SendToControl` in the SDK)
    are written to the log, one row per payload, and the edge is told they
    are queued. The leader, which is always a full Studio, hands each to its
    plugin. Delivery is at least once: when a replica becomes the leader it
    takes the payloads of the last 45 s again (a crashed leader may not have
    handled them, and none was handled while no replica led), so plugins see
    those twice and should use the correlation ID. A payload that arrives
    while no full Studio is up for longer than that is lost, and so is one a
    plugin fails to handle (as on a single Studio). See
    `docs/site/docs/plugins-edge-to-control.md`.

## langchaingo in tree

Studio's langchaingo fork (Anthropic temperature, OpenAI reasoning_effort and
other fixes) lives in `third_party/langchaingo` and is imported by its
in-tree path. It used to be a `replace` in `go.mod`, which Go ignores in a
module that imports Studio, so a host would have built with upstream
langchaingo instead. `make langchaingo-verify` keeps upstream imports out.
See `third_party/README.md`.

## gorm isolation

The Tyk Dashboard `replace`s `gorm.io/gorm` with a fork, and a `replace`
applies to the whole build. So Studio imports gorm and its postgres and
sqlite drivers from its own copy,
`github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/...`, and the
Dashboard's replace cannot reach it. `third_party/README.md` has the details.

- The copy is the published modules at the versions pinned in
  `third_party/gorm-pin/go.mod`, with their `gorm.io/...` imports rewritten.
  `scripts/gorm-vendor.sh` (`make gorm-vendor`) is the only thing that
  writes it. No automation changes the pins, so an upgrade is always a
  deliberate PR whose diff is the upstream change.
- `make gorm-verify`, a CI job, checks two things. The tree must be exactly
  what the pins produce. And no package of the root, microgateway or
  enterprise module may import `gorm.io/...`. gorm finds model hooks, column
  types and sentinel errors by type assertion at runtime, so a stray import
  of another gorm compiles and then fails silently. Third-party libraries may use
  upstream gorm internally (TIB 1.8's `TykTechnologies/storage` does); the
  check allows those importers by prefix. Also, `go mod tidy`
  would resolve `gorm.io/gorm` to v1.21.16, the version the Tyk gateway
  module requires.
- The schema snapshot tests (`models/`, `microgateway/internal/database/`)
  pin what the migrations produce on SQLite and Postgres. The hook canary
  (`models/hooks_canary_test.go`) ties every model hook to the copy's
  callback interfaces.
- The drivers underneath (pgx, go-sqlite3) are not copied. They stay
  ordinary requirements, because a second copy would register the same
  `database/sql` driver name again and panic. In a host's build they may
  move up to the host's versions.
- A host uses `studio.OpenDatabase` for `Options.DB` and never imports
  gorm. Its binary carries both gorms, about 2–3 MB.

## Building without cgo

A host may build with `CGO_ENABLED=0` (the Tyk Dashboard's dev builds do), so
`pkg/studio` links nothing that needs cgo. `TestHostBuildHasNoCgoOnlyDependencies`
(`pkg/studio/deps_test.go`) and a CI build step keep it that way.

- **SQLite** (go-sqlite3 needs cgo) lives in `pkg/studio/sqlitedb`, which
  registers itself with `studio.RegisterDatabaseDriver`. The standalone
  binary and `examples/embed-host` import it, so standalone Studio still
  runs on SQLite. A host that does not import it opens Postgres only, and
  asking for `sqlite` gives an error naming the package.
- **Chroma**: chroma-go's v2 client loads a tokenizer and the ONNX runtime
  through cgo. `data_session/chroma.go` is `//go:build cgo`, and
  `chroma_nocgo.go` stands in for it: Chroma datasources return
  `ErrChromaUnavailable`, and Chroma is left out of the vector store lists.
  Creating a Chroma datasource, or switching one to Chroma, fails with
  `services.ErrVectorStoreUnavailable` (400); an existing one can still be
  edited or moved to another store. `studio.New` logs a warning naming the
  existing Chroma datasources. `DataSession.Search` skips (and logs) a
  datasource that fails, so the others still answer, and errors only when
  every datasource fails (it used to abort on the first failure, as v2.2.0
  did).
  Studio always supplies vectors, so every collection it opens directly
  gets an explicit embedding function that refuses to embed
  (`precomputedEmbeddings`). Without one, chroma-go builds its default ONNX
  function, which downloads ORT 1.21 while the Dashboard-aligned
  `onnxruntime_go` v1.26 asks for API 24, and every store and search fails.
  chroma-go v0.4 was not an option: it adds an embedded runtime, and
  `chroma-go-local@v0.3.4`, which it requires, failed checksum-database
  verification (2026-09-29).
- The microgateway is not embedded and keeps its cgo build and local SQLite.
