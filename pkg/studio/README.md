# pkg/studio

Runs Tyk AI Studio inside another Go process. The standalone binary
(`main.go`) is a thin wrapper over this package.

```go
import (
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
)

conf := config.LoadFrom(func(key string) string { return hostSettings[key] })
conf.SiteURL = "https://control.example.com"

s, err := studio.New(studio.Options{
	Config:  conf,
	DB:      studioDB, // Studio's own database or schema; the host closes it
	Version: hostVersion,
	Logger:  &hostLogger,
	TracerProvider: hostTracerProvider,
	MeterProvider:  hostMeterProvider,
	OnLicenceInvalid: func(err error) { /* alert, degrade, or stop Studio */ },
})
if err != nil {
	return err
}
defer s.Stop(ctx)

mux.Handle("/", s.HTTPHandler())      // admin API, portal, chat and UI
go s.StartProxy()                      // AI gateway on Config.ProxyPort
go s.StartGRPC(edgeListener)           // edge control plane, when GatewayMode is "control"
```

## Lifecycle

- `New` migrates the database, seeds defaults, starts background services
  and builds the API, gateway and (in control mode) gRPC control server. It
  returns an error rather than exiting; nothing listens yet.
- `HTTPHandler` is the admin API and UI. It still expects to be served at the
  root of its host; serving it under a path prefix is the next phase of the
  embedding work (see `features/Embedding.md`).
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
  is not committed; build it (`npm run build`) before compiling, or pass
  `Options.UIAssets`.
- **Telemetry globals.** Pass `TracerProvider` and `MeterProvider` to keep
  Studio off the OpenTelemetry globals. Without them Studio configures
  tracing and metrics from `Config` the way the standalone binary does,
  which installs a global tracer provider and propagator.
- **Replace directives.** Go ignores `replace` directives in dependencies,
  so an importing module must copy the ones in this repository's `go.mod`
  (the langchaingo fork, the OpenTelemetry SDK pin, `./microgateway` and
  `./enterprise`).
- **Enterprise edition.** Build with `-tags enterprise` and import
  `github.com/TykTechnologies/midsommar/v2/enterprise/all` for its side
  effects. `New` fails if an enterprise feature is missing.
