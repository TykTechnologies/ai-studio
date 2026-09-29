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
| 3 | Configurable base path for the backend | Planned |
| 4 | Pluggable authentication and CSRF | Planned |
| 5 | UI served under a base path; host-authentication UI mode | Planned |
| 6 | Shipping the built UI to importers | Planned |

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

