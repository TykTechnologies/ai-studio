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

- **Own database.** Studio keeps its own database or schema; the host passes a
  `*gorm.DB` for it. Studio's table names are not prefixed.
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

Deliberately still environment-only: the network and plugin security knobs
(`ALLOW_INTERNAL_NETWORK_ACCESS`, `PLUGIN_COMMAND_ALLOWLIST`,
`PLUGIN_BLOCK_INTERNAL_URLS`, the plugin allowed directories, `pkg/netguard`),
OCI registry credentials (`OCI_PLUGINS_REGISTRY_*`, which reference other
variables by name), `$ENV/` secret references, and tuning and debug switches
shared with the microgateway (`ANALYTICS_BUFFER_SIZE`, `BUDGET_SYNC_INTERVAL`,
`DEBUG_HTTP_PROXY`, the metrics legacy-names switch).

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
- Email templates are embedded (package `templates`). A `templates/` directory
  in the working directory still takes precedence, so deployments can
  customise them, but a process started elsewhere renders the defaults
  instead of failing.

## The studio package (Phase 2)

`pkg/studio` holds the wiring that used to live in `main.go`; `main.go` is
now a thin wrapper (flags, `config.Get`, logger, connectivity checks, opening
the database, the docs server, signal handling). See `pkg/studio/README.md`
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
- Package `ui` embeds the built frontend (`ui.FS`, rooted at the build
  directory); `api.New` takes it as an `fs.FS`, and `Options.UIAssets`
  overrides it.
- `enterprise/all` imports every enterprise feature; `main_enterprise.go` and
  enterprise hosts import it.
- `grpc.ControlServer.Serve(listener)` serves on a host-supplied listener;
  `API.Shutdown` stops the audit writer even when the host served the router.

## Base path (Phase 3)

`BASE_PATH` (`AppConf.BasePath`, normalised to `/prefix` or `""`) serves the
API and UI under a path prefix. `SITE_URL` (and `AUTH_SERVER_URL`, which
defaults to it) should include the prefix: emails, the OAuth consent
redirect and the OAuth metadata are built from it, and a warning is logged
when it does not end with the base path.

- Routes stay registered at the root. `API.Handler()` (what
  `studio.HTTPHandler` and the standalone server serve) strips the prefix;
  a request without it passes through unchanged, so a reverse proxy may
  strip it first. The bare prefix redirects to `prefix/`.
- The session cookie (`auth.Config.CookiePath`) and the CSRF cookie are
  scoped to the base path.
- Logout expires Studio's session cookie and the identity broker's
  `_gothic_session`, and nothing else (it used to expire every cookie on the
  request, which would sign a user out of the host too).
- Post-SSO redirects and the email-verified page's redirect go to the base
  path instead of `/`.
- OAuth: the consent redirect and the authorization server metadata keep the
  path of `SITE_URL`/`AUTH_SERVER_URL` (`url.JoinPath` instead of resolving
  absolute paths, which dropped it). RFC 8414 discovery for a pathed issuer
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
groups), nil when the request has no host identity, or an error to reject
it.

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
- Fixed on the way: the admin plugin iframe loaded `/plugins/assets/...`,
  which no route serves (now `/api/v1/plugins/assets/...`), and "mark plugin
  UI loaded" posted to a doubled `/api/v1/api/v1/...`.
- `config/docs_links.json` is embedded (an on-disk copy still overrides it),
  so documentation links work from any working directory.

`examples/embed-host` is a runnable host: its own login page and cookie, an
`Authenticator` over that cookie, Studio at `/ai-studio`, and the OAuth
discovery document at the host root.

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
  both to the tag's GitHub release, creating a draft release when there is
  none. It is the only job with `contents: write`.
- A host builds with `-tags studio_noui`, unpacks the tarball for the Studio
  version it imports, and passes `os.DirFS(dir)` (or its own embed of the
  directory) as `Options.UIAssets`. `examples/embed-host -ui <dir>` does
  this.

Verified: with `ui/admin-frontend/build` moved away, the default build of
`pkg/studio` fails on the embed pattern and the `studio_noui` build of
`pkg/studio` and `examples/embed-host` succeeds; that `studio_noui` host,
given a tarball made as the release job makes it, serves the full console
under `/ai-studio`.

