# pkg/studio

Runs Tyk AI Studio inside another Go process. The standalone binary
(`main.go`) is a thin wrapper over this package.

```go
import (
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
)

conf := config.LoadFrom(func(key string) string { return hostSettings[key] })
conf.BasePath = "/ai-studio"
conf.SiteURL = "https://control.example.com/ai-studio" // includes the base path

// Studio's own database or schema (DatabaseType/DatabaseURL in conf).
studioDB, err := studio.OpenDatabase(conf)
if err != nil {
	return err
}

s, err := studio.New(studio.Options{
	Config:  conf,
	DB:      studioDB, // the host closes it after Stop
	Version: hostVersion,
	Logger:  &hostLogger,
	TracerProvider: hostTracerProvider,
	MeterProvider:  hostMeterProvider,
	OnLicenceInvalid: func(err error) { /* alert, degrade, or stop Studio */ },

	// The host signs users in; Studio provisions and authorises them.
	Auth:      hostAuthenticator, // Authenticate(*http.Request) (*studio.Identity, error)
	LoginURL:  "/login",
	LogoutURL: "/logout",
	CSRF:      hostCSRFMiddleware, // optional; Studio's own otherwise
})
if err != nil {
	return err
}
defer s.Stop(ctx)

mux.Handle("/ai-studio/", s.HTTPHandler()) // with conf.BasePath = "/ai-studio"
mux.Handle("/.well-known/oauth-authorization-server/ai-studio", s.OAuthMetadataHandler())
go s.StartProxy()                      // AI gateway on Config.ProxyPort
go s.StartGRPC(edgeListener)           // edge control plane, when GatewayMode is "control"
```

## Lifecycle

- `New` migrates the database, seeds defaults, starts background services
  and builds the API, gateway and (in control mode) gRPC control server. It
  returns an error rather than exiting; nothing listens yet.
- `HTTPHandler` is the admin API and UI, served under `Config.BasePath`; it
  strips the prefix itself. Session and CSRF cookies are scoped to the base
  path, and logout leaves the host's cookies alone. The console follows the
  base path and, with `Auth`, sends signed-out users to `LoginURL`.
- With `Auth`, every request is offered to the host first. The identity it
  returns (subject, email, name, admin, optional group names) becomes a
  Studio user on first sight and is kept in step after that; Studio's own
  RBAC then decides what the user may do. Studio's password login,
  registration and SSO are switched off; its API keys still work.
- `CSRF` replaces Studio's CSRF check for cookie-authenticated writes with
  the host's.
- `OAuthMetadataHandler` serves the OAuth authorization server metadata for
  MCP clients; with a base path, mount it at
  `/.well-known/oauth-authorization-server<base path>` on the host root.
- `ListenAndServe`, `StartProxy` and `StartGRPC` block until `Stop` or a
  serving error. `StartProxy` returns `ErrGatewayNotLicensed` at once without
  the gateway entitlement; `StartGRPC` returns `ErrNotControlPlane` outside
  control mode. `ProxyHandler` exposes the gateway for a host that serves it
  itself, but `StartProxy` is still needed: the gateway's `/ai/` routes hop
  to its own listener.
- `Stop` shuts everything down in dependency order, leaves the database
  open, and may be called more than once.

## Constraints

- **One instance per process.** Configuration, the analytics recorder and
  the secrets key are process-wide; `New` returns `ErrAlreadyRunning` until
  the running Studio is stopped.
- **Frontend assets.** Package `ui` embeds `ui/admin-frontend/build`, which
  is not committed, so a module download of Studio has no frontend to embed.
  Build with `-tags studio_noui` (package `ui` then embeds only a placeholder
  page) and pass the release's assets: every release tag carries
  `tyk-ai-studio-ui-<tag>.tar.gz` (and a `.sha256`) on its GitHub release.
  Unpack it and set `Options.UIAssets` to `os.DirFS(dir)`, or embed the
  directory in the host's own binary. In this repository, `npm run build` in
  `ui/admin-frontend` and the default build tags embed it as before.
- **Telemetry globals.** Pass `TracerProvider` and `MeterProvider` to keep
  Studio off the OpenTelemetry globals. Without them Studio configures
  tracing and metrics from `Config` the way the standalone binary does,
  which installs a global tracer provider and propagator.
- **Analytics.** Studio always writes analytics to its own database, since
  budgets and spend are computed from it. To also ship records into the
  host's pipeline, pass `AnalyticsSinks`: each sink gets a copy of every
  record through its own bounded queue, and records a slow sink cannot take
  are dropped (and logged) rather than slowing requests.
- **Replace directives.** Go ignores `replace` directives in dependencies,
  and a host needs to copy none of this repository's. The only one left, `./enterprise`, points
  this repository's own builds at the submodule; an enterprise host requires
  the enterprise module itself (below). langchaingo is in tree, under
  `third_party/langchaingo`, and the gateway plugin SDK Studio uses lives in
  `pkg/gatewayplugin` and `proto/microgateway_management`, so the
  microgateway module is not needed.
- **Sharing a Postgres database.** Set `conf.DatabaseSchema`
  (`DATABASE_SCHEMA`) to keep Studio's tables in a schema of their own inside
  the host's database; `OpenDatabase` creates it. Replicas may share it:
  `New` migrates and seeds under a Postgres advisory lock.
- **Databases and cgo.** `OpenDatabase` opens Postgres. SQLite needs cgo,
  so it is in `pkg/studio/sqlitedb`: import that package for its side effect
  to use `DatabaseType` `sqlite`. Studio builds with `CGO_ENABLED=0`, but
  Chroma datasources are then unavailable. See "Building without cgo" in
  `features/Embedding.md`.
- **Enterprise edition.** The enterprise module is the private repository
  `github.com/TykTechnologies/ai-studio-enterprise` (module path
  `github.com/TykTechnologies/ai-studio-enterprise/v2`, tagged with the same
  versions as Studio). Set
  `GOPRIVATE=github.com/TykTechnologies/ai-studio-enterprise` with read access
  to it, require both modules at the same version, build with
  `-tags enterprise`, and import
  `github.com/TykTechnologies/ai-studio-enterprise/v2/all` for its side
  effects. `New` fails if an enterprise feature is missing. A Community
  Edition host needs none of this: no public package imports the enterprise
  module (`make enterprise-import-guard` checks it), so it is never fetched.

## Example

`examples/embed-host` is a small runnable host: `go run ./examples/embed-host`
(after building the frontend), then open http://localhost:8090/.

