// Package studio runs Tyk AI Studio inside another Go process. A host builds
// a Studio with New, serves its HTTP handler, optionally starts the embedded
// AI gateway and the gRPC control server, and calls Stop on shutdown. The
// standalone binary is a thin wrapper over the same package.
//
// Only one Studio may run in a process at a time: several packages keep
// process-wide state (the configuration, the analytics recorder, the secrets
// key). New refuses a second instance until the first is stopped.
package studio

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/go-mail/mail"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/corsutil"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/edition"
	_ "github.com/TykTechnologies/midsommar/v2/services/grpc" // Registers the AIStudioManagementServer factory
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/services/log_export"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/services/scheduler"
	"github.com/TykTechnologies/midsommar/v2/ui"
)

// Options configures a Studio. Config and DB are required.
type Options struct {
	// Config is Studio's configuration, typically from config.Load or
	// config.LoadFrom. New installs it as the process-wide configuration.
	Config *config.AppConf

	// DB is Studio's own database or schema. New migrates it; the caller
	// owns it and closes it after Stop.
	DB *gorm.DB

	// Version, BuildHash and BuildTime are reported by /common/system and in
	// telemetry and webhooks.
	Version, BuildHash, BuildTime string

	// Logger receives Studio's own logging. Nil keeps the logger package's
	// current logger (logger.Init sets it up for the standalone binary).
	// Packages that log through zerolog's global logger keep doing so.
	Logger *zerolog.Logger

	// TracerProvider and Propagator are used for Studio's spans and for
	// carrying trace context to upstream providers. When TracerProvider is
	// nil, Studio configures tracing from Config and installs it as the
	// OpenTelemetry global, as the standalone binary does.
	TracerProvider trace.TracerProvider
	Propagator     propagation.TextMapPropagator

	// MeterProvider receives Studio's metrics, which the host then exports;
	// Studio serves no metrics endpoint of its own. When nil and
	// Config.MetricsEnabled is set, Studio serves Prometheus metrics at
	// Config.MetricsPath.
	MeterProvider metric.MeterProvider

	// AnalyticsSinks receive a copy of every analytics record (chat
	// records, proxy logs, tool calls, compliance events, batches from
	// edges) besides Studio's own database, which budgets and spend are
	// computed from. Each sink has its own bounded queue: a slow sink loses
	// records (counted and logged) rather than slowing requests.
	AnalyticsSinks []analytics.AnalyticsHandler

	// OnLicenceInvalid is called when an Enterprise licence fails its
	// periodic re-check. Nil exits the process.
	OnLicenceInvalid func(error)

	// License supplies the AI Studio Enterprise licence (the same JWT as
	// TYK_AI_LICENSE, validated the same way) from the host's own
	// settings. It is read at start and at every validity check, so a
	// renewed licence takes effect without a restart; call ReloadLicense to
	// apply one at once. Nil uses Config.LicenseKey.
	License func() string

	// UIAssets is the built admin frontend, rooted at its build directory
	// (for example os.DirFS over the unpacked tyk-ai-studio-ui release
	// tarball). Nil uses the frontend embedded in package ui, which a build
	// with the studio_noui tag leaves out.
	UIAssets fs.FS

	// NodeID identifies this replica among the Studio replicas sharing the
	// database (pkg/cluster): it is recorded as the owner of the edge streams
	// this replica holds and of the work it claims. Empty means a fresh ID
	// (hostname, pid and a random suffix) per process, which is what a
	// replica restart needs: it must not inherit its predecessor's claims.
	NodeID string

	// NodeLabel names this replica on the cluster status and the Edge
	// Gateways page (at most 64 letters, digits, spaces and . _ - : / ( ),
	// e.g. "dashboard"). Empty means DefaultNodeLabel.
	NodeLabel string

	// SkipLLMDefaults skips seeding the default LLM configurations and
	// their secrets.
	SkipLLMDefaults bool

	// Auth, when set, authenticates every request on the host's behalf:
	// Studio provisions a user for each identity it returns (see Identity)
	// and switches off its own password login, registration and SSO. API
	// keys still authenticate requests Auth has no identity for.
	Auth Authenticator
	// LoginURL and LogoutURL are where the console sends a user to sign in
	// or out when Auth is set. The console replaces "{return_to}" anywhere
	// in LoginURL (e.g. "/login?next={return_to}") with the page the user
	// asked for, URL-encoded: a path on Studio's origin under the base path,
	// with its query and hash. The host validates it and returns the user
	// there after signing in. A LoginURL without it is used as it is.
	LoginURL, LogoutURL string

	// CSRF, when set, replaces Studio's CSRF protection for
	// cookie-authenticated requests with the host's. It must call the
	// handler it wraps only for requests that pass. CSRFTokenHeader and
	// CSRFTokenURL tell the console how to obtain and present the token.
	CSRF            func(http.Handler) http.Handler
	CSRFTokenHeader string
	CSRFTokenURL    string

	// GRPCTLSConfig, when set, is the TLS configuration the gRPC control
	// server serves edges with, in place of Config's certificate and key
	// files (Config.GRPCTLSCertPath, GRPCTLSKeyPath): a host with its own
	// certificate store, cipher suites or minimum version passes its own.
	GRPCTLSConfig *tls.Config

	// Chromeless makes the console render pages only: no top bar (the
	// Admin / Portal / Chat switch and the user menu) and no navigation
	// drawers, because the host draws its own. Sticky page headers then sit
	// at the top of the page instead of below Studio's 64px bar.
	Chromeless bool
}

// Identity is a user as the host has authenticated them. Subject and Email
// are required; Studio keeps the user's name, email, administrator status
// and, when Groups is not nil, group memberships in step with it.
type Identity = services.HostIdentity

// Authenticator authenticates a request on the host's behalf. It returns
// nil and no error when the request carries no host identity, and an error
// to reject the request.
type Authenticator = auth.Authenticator

// ErrAlreadyRunning is returned by New while another Studio is running in
// the process.
var ErrAlreadyRunning = errors.New("studio: another instance is already running in this process")

// ErrGatewayNotLicensed is returned by StartProxy when the licence does not
// include the gateway.
var ErrGatewayNotLicensed = errors.New("studio: feature_gateway is not in the licence entitlements")

// ErrNotControlPlane is returned by StartGRPC unless Config.GatewayMode is
// "control".
var ErrNotControlPlane = errors.New(`studio: the gRPC control server runs only when GatewayMode is "control"`)

var running atomic.Bool

// Studio is a running AI Studio instance.
type Studio struct {
	conf      *config.AppConf
	db        *gorm.DB
	service   *services.Service
	licensing licensing.Service
	api       *api.API
	proxy     *proxy.Proxy
	control   *grpc.ControlServer

	// clusterParts is this replica's cluster membership: node, event log,
	// relay, leader lease and replica signals.
	clusterParts
	// relayed applies other replicas' changes to this replica's caches.
	relayed *relayedChanges
	// edgePayloads hands this replica's plugins the edge payloads headless
	// control planes forward, while this replica leads.
	edgePayloads *edgePayloadHost
	// pushes delivers configuration pushes to the edges whose streams this
	// replica holds (control mode only).
	pushes *pushes.Coordinator

	scheduler        *scheduler.SchedulerService
	telemetry        *services.TelemetryManager
	tracingShutdown  tracing.Shutdown
	cancelBackground context.CancelFunc
	// analyticsTee copies analytics records to Options.AnalyticsSinks;
	// analyticsPrimary is the handler it wraps, put back on Stop.
	analyticsTee     *analytics.Tee
	analyticsPrimary analytics.AnalyticsHandler

	stopOnce sync.Once
	stopErr  error
}

// New builds a Studio: it migrates the database, seeds defaults, starts the
// background services and builds the HTTP API, the gateway and (in control
// mode) the gRPC control server. Nothing listens until the caller serves
// HTTPHandler or calls ListenAndServe, StartProxy or StartGRPC.
func New(opts Options) (_ *Studio, err error) {
	if opts.Config == nil || opts.DB == nil {
		return nil, errors.New("studio: Options.Config and Options.DB are required")
	}
	if !running.CompareAndSwap(false, true) {
		return nil, ErrAlreadyRunning
	}

	s := &Studio{conf: opts.Config, db: opts.DB}
	// Undo whatever was started if a later step fails.
	defer func() {
		if err != nil {
			s.stop(context.Background())
		}
	}()

	conf := opts.Config
	config.Set(conf)
	if opts.Logger != nil {
		logger.Use(*opts.Logger)
	}
	api.SetBuildInfo(opts.Version, opts.BuildHash, opts.BuildTime)

	if err := edition.CheckRegistered(); err != nil {
		return nil, err
	}

	secrets.SetEncryptionKey(conf.SecretKey)
	services.SetBrandingStoragePath(conf.BrandingStoragePath)
	// Tuning settings shared with the microgateway: the packages read them
	// from the environment unless Studio sets them.
	analytics.SetBufferSize(conf.AnalyticsBufferSize)
	metrics.SetLegacyNames(!conf.MetricsNoLegacyNames)
	corsutil.SetAllowedOrigins(conf.CORSAllowedOrigins)
	secrets.WarnIfEncryptionUnconfigured()

	backgroundCtx, cancel := context.WithCancel(context.Background())
	s.cancelBackground = cancel

	// Migrations and seeding (through the RBAC seed below) run under a
	// Postgres advisory lock, so replicas sharing a database do not race
	// through AutoMigrate and the get-or-create seeds. A no-op on SQLite.
	// Bounded: an instance that never finishes (or a lock left behind)
	// must not keep this one waiting for ever.
	lockWait := conf.MigrationLockTimeout
	if lockWait <= 0 {
		lockWait = models.DefaultMigrationLockWait
	}
	lockCtx, cancelLockWait := context.WithTimeout(backgroundCtx, lockWait)
	releaseMigrationLock, err := models.AcquireMigrationLock(lockCtx, s.db)
	cancelLockWait()
	if err != nil {
		return nil, fmt.Errorf("studio: %w", err)
	}
	defer func() {
		if releaseMigrationLock != nil {
			releaseMigrationLock()
		}
	}()

	if err := models.InitModels(s.db); err != nil {
		return nil, fmt.Errorf("studio: migrate database: %w", err)
	}
	if err := analytics.Migrate(s.db); err != nil {
		return nil, fmt.Errorf("studio: migrate analytics tables: %w", err)
	}
	if err := models.MigrateTIBStores(s.db); err != nil {
		return nil, fmt.Errorf("studio: migrate identity broker tables: %w", err)
	}
	if err := ensureDefaults(s.db, opts.SkipLLMDefaults, conf.SkipFilterDefaults); err != nil {
		return nil, fmt.Errorf("studio: seed defaults: %w", err)
	}

	// Licensing (Enterprise: validates the licence and starts periodic checks).
	s.licensing = licensing.NewService(licensing.Config{
		LicenseKey:           conf.LicenseKey,
		LicenseSource:        opts.License,
		TelemetryURL:         conf.LicenseTelemetryURL,
		TelemetryPeriod:      conf.LicenseTelemetryPeriod,
		TelemetryDisabled:    conf.LicenseDisableTelemetry,
		ValidityCheckPeriod:  conf.LicenseValidityPeriod,
		TelemetryConcurrency: conf.LicenseTelemetryConcurrency,
		OnInvalid:            opts.OnLicenceInvalid,
	}, s.db)
	if err := s.licensing.Start(); err != nil {
		return nil, fmt.Errorf("studio: start licensing: %w", err)
	}
	logger.Info("Licensing service initialized")

	if _, err := services.NewBrandingFileStorage(conf.BrandingStoragePath); err != nil {
		logger.Warnf("Failed to initialize branding storage: %v", err)
	} else {
		logger.Infof("Branding storage initialized at: %s", conf.BrandingStoragePath)
	}

	var ociConfig *ociplugins.OCIConfig
	if conf.OCIPlugins.IsEnabled() {
		ociConfig = conf.OCIPlugins.ToOCILibConfig()
		logger.Debugf("OCI plugin support enabled - cache dir: %s", conf.OCIPlugins.CacheDir)
	} else {
		logger.Debug("OCI plugin support disabled - set AI_STUDIO_OCI_CACHE_DIR to enable")
	}

	service := services.NewServiceWithOCI(s.db, ociConfig)
	s.service = service
	service.SetLicensingService(s.licensing)

	// Register the per-plugin permission resources of every installed plugin
	// before the system roles are seeded, so Viewer/Editor/Auditor are
	// computed against the full catalogue.
	if err := service.RebuildPermissionCatalogue(); err != nil {
		logger.Warnf("Failed to register plugin permission resources: %v", err)
	}

	// A build without cgo has no Chroma client: say so up front for the
	// Chroma datasources it cannot search, rather than only at query time.
	if unavailable, err := service.UnavailableVectorStoreDatasources(); err != nil {
		logger.Warnf("Failed to check datasources for unavailable vector stores: %v", err)
	} else if len(unavailable) > 0 {
		names := make([]string, 0, len(unavailable))
		for _, d := range unavailable {
			names = append(names, fmt.Sprintf("%q (id %d)", d.Name, d.ID))
		}
		logger.Warnf("This build has no cgo, so Chroma is unavailable: searches of these Chroma datasources fail until they are moved to another vector store: %s", strings.Join(names, ", "))
	}

	// Seed RBAC system roles and migrate legacy admin flags into bindings
	// (Enterprise; no-op in Community Edition). Idempotent on every boot.
	if err := service.Authz().Seed(backgroundCtx); err != nil {
		return nil, fmt.Errorf("studio: seed RBAC roles: %w", err)
	}
	// Last under the lock: the schema is now this build's (CheckSchema
	// reads it back for instances that do not migrate).
	if err := models.RecordSchemaVersion(s.db, schemaWriter(opts.Version)); err != nil {
		return nil, fmt.Errorf("studio: %w", err)
	}
	releaseMigrationLock()
	releaseMigrationLock = nil

	// Join the cluster: register this replica and start reading the event
	// log. With one replica (or SQLite) this is one row and an idle reader.
	nodeID := opts.NodeID
	if nodeID == "" {
		nodeID = cluster.NewNodeID()
	}
	if err := s.joinCluster(backgroundCtx, s.db, nodeID, opts.Version, cluster.NodeOptions{
		Label: nodeLabel(opts.NodeLabel, DefaultNodeLabel),
	}); err != nil {
		return nil, err
	}
	// Singleton jobs (aggregations, alerts, syncs, cleanups) run only on
	// the replica holding the leader lease.
	s.leadership = cluster.NewLeadership(s.db, cluster.LeaderLease, nodeID, cluster.LeadershipOptions{})
	// Leader-only work that was skipped while another replica held the
	// lease catches up as soon as this one gains it (pkg/replicas.OnLeading).
	s.leadership.OnChange(func(leading bool) {
		if leading {
			replicas.BecameLeader()
		}
	})
	s.leadership.Start()
	s.connectReplicas()

	// Plugin loading waits until the event bus is wired (below), so plugins
	// can subscribe to events during initialization.

	if conf.MarketplaceEnabled && ociConfig != nil {
		var ociClient *ociplugins.OCIPluginClient
		if service.PluginService != nil {
			ociClient, _ = ociplugins.NewOCIPluginClient(ociConfig)
		}
		service.MarketplaceService = services.NewMarketplaceService(
			s.db,
			ociClient,
			service.PluginService,
			service.AIStudioPluginManager,
			conf.MarketplaceCacheDir,
			conf.MarketplaceIndexURL,
			conf.MarketplaceSyncInterval,
		)
		go service.MarketplaceService.Start(backgroundCtx)
		logger.Debugf("Marketplace service started - index URL: %s, sync interval: %v",
			conf.MarketplaceIndexURL, conf.MarketplaceSyncInterval)
	} else if !conf.MarketplaceEnabled {
		logger.Info("Marketplace is disabled via MARKETPLACE_ENABLED=false")
	} else {
		logger.Warn("Marketplace requires OCI support - set AI_STUDIO_OCI_CACHE_DIR to enable")
	}

	if service.AIStudioPluginManager != nil {
		s.scheduler = scheduler.NewSchedulerService(s.db, service.AIStudioPluginManager)
		if err := s.scheduler.Start(); err != nil {
			logger.Errorf("Failed to start scheduler service: %v", err)
			s.scheduler = nil
		} else {
			logger.Info("Scheduler service started successfully")
		}
	}

	mailer := mail.NewDialer(conf.SMTPServer, conf.SMTPPort, conf.SMTPUser, conf.SMTPPass)
	mailService := notifications.NewMailService(
		conf.FromEmail,
		conf.SMTPServer,
		conf.SMTPPort,
		conf.SMTPUser,
		conf.SMTPPass,
		mailer,
		conf.DevMode,
	)
	notificationService := services.NewNotificationService(
		s.db,
		conf.FromEmail,
		conf.SMTPServer,
		conf.SMTPPort,
		conf.SMTPUser,
		conf.SMTPPass,
		mailer,
	)
	// Rows recorded before email framing was stripped at write time still
	// read "Subject: ... Dear Administrator ..." in the bell. Rewrite them
	// once, off the startup path; a second run finds nothing to change.
	safe.Go("notification body backfill", func() {
		changed, err := notificationService.BackfillLegacyBodies()
		if err != nil {
			logger.Warnf("Notification body backfill stopped after %d rows: %v", changed, err)
		} else if changed > 0 {
			logger.Infof("Rewrote %d legacy notification bodies", changed)
		}
	})

	authConfig := &auth.Config{
		DB:                     s.db,
		Service:                service,
		CookieName:             "session",
		CookieSecure:           !conf.DevMode,
		CookieHTTPOnly:         true,
		CookieSameSite:         http.SameSiteLaxMode,
		CookieDomain:           "",
		CookiePath:             cookiePath(conf.BasePath),
		ResetTokenExpiry:       time.Hour,
		SessionDuration:        conf.SessionDuration,
		FrontendURL:            conf.SiteURL,
		RegistrationAllowed:    conf.AllowRegistrations,
		AdminEmail:             conf.AdminEmail,
		AllowedRegisterDomains: conf.FilterSignupDomains,
		TIBEnabled:             conf.TIBEnabled,
		TIBAPISecret:           conf.TIBAPISecret,
		OCIConfig:              conf.OCIPlugins.ToOCILibConfig(),
		AllowSSOUserAPIKeys:    conf.AllowSSOUserAPIKeys,
		SSOAPIKeyLiveness:      conf.SSOAPIKeyLiveness,
		HostAuth:               opts.Auth,
		ProvisionHostUser:      service.ProvisionHostUser,
		HostLoginURL:           opts.LoginURL,
		HostLogoutURL:          opts.LogoutURL,
		CSRF:                   opts.CSRF,
		CSRFTokenHeader:        opts.CSRFTokenHeader,
		CSRFTokenURL:           opts.CSRFTokenURL,
		Chromeless:             opts.Chromeless,
	}
	authService := auth.NewAuthService(authConfig, mailService, service, notificationService)

	if opts.MeterProvider != nil {
		metrics.InitWithProvider(opts.MeterProvider)
	} else if conf.MetricsEnabled {
		metrics.Init()
		logger.Infof("Prometheus metrics enabled at %s", conf.MetricsPath)
	}

	if opts.TracerProvider != nil {
		tracing.Use(opts.TracerProvider, opts.Propagator)
		s.tracingShutdown = func(context.Context) error { return nil }
	} else {
		// Init is called even with export off, so the W3C propagator is
		// installed and an inbound traceparent flows through to the upstream
		// provider.
		shutdown, err := tracing.Init(backgroundCtx, tracing.Config{
			Enabled:        conf.TracingEnabled,
			Endpoint:       conf.TracingEndpoint,
			ServiceName:    "tyk-ai-studio",
			ServiceVersion: opts.Version,
		})
		if err != nil {
			logger.Errorf("Tracing disabled: %v", err)
		} else if conf.TracingEnabled {
			logger.Infof("OpenTelemetry tracing enabled, exporting to %s", conf.TracingEndpoint)
		}
		s.tracingShutdown = shutdown
	}

	analytics.StartRecording(backgroundCtx, s.db)
	if len(opts.AnalyticsSinks) > 0 {
		s.analyticsPrimary = analytics.GetHandler()
		s.analyticsTee = analytics.NewTee(s.analyticsPrimary, opts.AnalyticsSinks...)
		analytics.SetHandler(s.analyticsTee)
	}
	// One budget service (and team budget service) for the API and the
	// proxy, so resets and allocation changes clear the cache the proxy reads.
	service.InitBudgets(notificationService)

	// Replace the log export service NewServiceWithOCI built without SMTP,
	// stopping its cleanup goroutine first.
	if service.LogExportService != nil {
		service.LogExportService.Stop()
	}
	service.LogExportService = log_export.NewService(s.db, notificationService, conf.ExportStoragePath, conf.SiteURL)

	s.telemetry = services.NewTelemetryManager(s.db, conf.TelemetryEnabled, opts.Version)
	s.telemetry.Start()

	s.proxy = proxy.NewProxy(service, &proxy.Config{
		Port:                  conf.ProxyPort,
		UnifiedRouterBasePath: conf.UnifiedRouterPath,
		DisableUnifiedRouter:  conf.UnifiedRouterDisabled,
		ServerTiming:          conf.GatewayServerTiming,
		DebugHTTPProxy:        conf.DebugHTTPProxy,
	}, service.Budget)

	if conf.GatewayMode == "control" {
		if err := s.wireControlPlane(opts.Version, opts.GRPCTLSConfig); err != nil {
			return nil, err
		}
	} else {
		// Standalone: there is no gRPC control server, so use a node-local
		// event bus. System CRUD events, plugin pub/sub and webhooks work the
		// same as in control mode; nothing is forwarded because there are no
		// edges.
		s.wireEventBus(eventbridge.NewBus())
		service.InitWebhooks(conf.Webhooks, opts.Version)
		service.InitTykMCP(conf.TykMCP, opts.Version)
	}
	// Headless control planes (MDCB) hold edges but run no plugins: their
	// edges' plugin payloads reach the plugins here, on the leader. In
	// either mode: the edges may all be held by headless replicas.
	if service.AIStudioPluginManager != nil {
		s.edgePayloads = newEdgePayloadHost(s.db, s.clusterLog, service.AIStudioPluginManager)
	}

	frontend := opts.UIAssets
	if frontend == nil {
		if !ui.Embedded {
			logger.Warn("Built with studio_noui and no Options.UIAssets: the web interface is a placeholder page")
		}
		frontend = ui.FS
	}
	s.api, err = api.New(service, conf.DisableCors, authService, authConfig, s.proxy, frontend, s.licensing)
	if err != nil {
		return nil, fmt.Errorf("studio: build API: %w", err)
	}
	s.api.SetClusterStatus(func(ctx context.Context) (interface{}, error) {
		return cluster.Snapshot(ctx, s.db, nodeID, s.clusterLog, s.relay)
	})

	return s, nil
}

// wireControlPlane builds the gRPC control server and connects the push
// coordinator, plugin manager and event bus to it.
func (s *Studio) wireControlPlane(version string, tlsConfig *tls.Config) error {
	conf, service := s.conf, s.service
	control, err := grpc.NewControlServer(controlServerConfig(conf, s.clusterNode.ID(), tlsConfig), s.db)
	if err != nil {
		return fmt.Errorf("studio: create gRPC control server: %w", err)
	}
	s.control = control

	control.SetGovernedMetadataReader(service.GovernedMetadataService)
	// Enterprise: edges learn which Apps to refuse (budget 0, team over a
	// hard-blocking budget) and edge spend raises budget alerts.
	if src, ok := service.Budget.(grpc.EdgeBudgetSource); ok {
		control.SetEdgeBudgetSource(src)
	}

	// Pushes are recorded in the database; this replica delivers those for
	// edges whose streams it holds and reports on any of them.
	s.pushes = pushes.New(s.db, s.clusterNode.ID(), control, pushes.Options{})
	control.SetPushDelivery(s.pushes)
	service.NamespaceService.SetPushes(s.pushes)
	if err := s.pushes.Start(context.Background()); err != nil {
		return fmt.Errorf("studio: start edge push delivery: %w", err)
	}

	if service.AIStudioPluginManager != nil {
		// Route edge-to-control payloads to plugins.
		control.SetPluginManager(service.AIStudioPluginManager)
	}
	s.wireEventBus(control.GetEventBus())
	service.InitWebhooks(conf.Webhooks, version)
	service.InitTykMCP(conf.TykMCP, version)
	logger.Info("Edge push delivery started on this replica")
	return nil
}

// wireEventBus connects the plugin manager and the service to bus, then loads
// plugins so they can subscribe to events while they initialize.
func (s *Studio) wireEventBus(bus eventbridge.Bus) {
	service := s.service
	if service.AIStudioPluginManager != nil {
		service.AIStudioPluginManager.SetEventBus(bus, "control")
		logger.Debug("Loading AI Studio plugins (UI, Agent, Object Hooks)...")
		if err := service.AIStudioPluginManager.LoadAllUIAndAgentPlugins(); err != nil {
			logger.Warnf("Failed to load some AI Studio plugins: %v", err)
		} else {
			logger.Debug("AI Studio plugins loaded successfully")
		}
	}
	service.SetEventBus(bus)

	// Other replicas' edges and caches must hear about this replica's
	// events, and this replica about theirs.
	s.watchRelayedChanges(bus)
	s.startRelay(bus, cluster.RelayOptions{})
}

// controlServerConfig is the gRPC control server's configuration, for a full
// Studio and a headless control plane alike. tlsConfig, when set, replaces
// the certificate and key files in conf.
func controlServerConfig(conf *config.AppConf, nodeID string, tlsConfig *tls.Config) *grpc.Config {
	return &grpc.Config{
		GRPCPort:      conf.GRPCPort,
		GRPCHost:      conf.GRPCHost,
		TLSEnabled:    conf.GRPCTLSEnabled,
		TLSCertPath:   conf.GRPCTLSCertPath,
		TLSKeyPath:    conf.GRPCTLSKeyPath,
		TLSConfig:     tlsConfig,
		AuthToken:     conf.GRPCAuthToken,
		NextAuthToken: conf.GRPCNextAuthToken,
		EncryptionKey: conf.MicrogatewayEncryptionKey,
		NodeID:        nodeID,

		BudgetSyncInterval: conf.BudgetSyncInterval,

		MaxMessageSize:        conf.GRPCMaxMessageSize,
		MaxConnectionAge:      conf.GRPCMaxConnectionAge,
		MaxConnectionAgeGrace: conf.GRPCMaxConnectionAgeGrace,
	}
}

// HTTPHandler returns the admin API and UI handler: the portal, chat,
// management API and admin interface. Mount it at Config.BasePath (or the
// root when that is empty); it strips the base path itself, so the host
// passes requests through unchanged.
func (s *Studio) HTTPHandler() http.Handler {
	return s.api.Handler()
}

// OAuthMetadataHandler serves Studio's OAuth authorization server metadata
// for MCP clients. With a base path, RFC 8414 discovery happens outside it,
// at /.well-known/oauth-authorization-server followed by the base path, so
// the host mounts this handler there.
func (s *Studio) OAuthMetadataHandler() http.Handler {
	return s.api.OAuthMetadataHandler()
}

// ListenAndServe serves HTTPHandler on addr, with TLS when certFile and
// keyFile are set. It blocks until Stop is called, returning
// http.ErrServerClosed, or until serving fails.
func (s *Studio) ListenAndServe(addr, certFile, keyFile string) error {
	return s.api.Run(addr, certFile, keyFile)
}

// ProxyHandler returns the embedded AI gateway's handler for a host that
// serves it itself. Call StartProxy as well: the gateway's /ai/ routes hop to
// its own listener on Config.ProxyPort.
func (s *Studio) ProxyHandler() http.Handler {
	return s.proxy.Handler()
}

// StartProxy serves the embedded AI gateway on Config.ProxyPort. It blocks
// until Stop is called, returning http.ErrServerClosed, or until serving
// fails. It returns ErrGatewayNotLicensed at once when the licence does not
// include the gateway.
func (s *Studio) StartProxy() error {
	if ent, ok := s.licensing.Entitlement(licensing.FeatureGateway); !ok || !ent.Bool() {
		return ErrGatewayNotLicensed
	}
	return s.proxy.Start()
}

// StartGRPC serves the gRPC control server for edge gateways on listener, or
// on Config.GRPCHost:GRPCPort when listener is nil. It blocks until Stop is
// called or serving fails, and returns ErrNotControlPlane unless
// Config.GatewayMode is "control".
func (s *Studio) StartGRPC(listener net.Listener) error {
	if s.control == nil {
		return ErrNotControlPlane
	}
	if listener == nil {
		return s.control.Start()
	}
	return s.control.Serve(listener)
}

// Stop shuts Studio down: the HTTP API, the gateway, the gRPC control server,
// plugins and background workers, then licensing. It leaves the database
// open for its owner to close. ctx bounds the HTTP servers' graceful
// shutdown. Stop is safe to call more than once; after it returns, New may
// build another Studio.
func (s *Studio) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() { s.stopErr = s.stop(ctx) })
	return s.stopErr
}

func (s *Studio) stop(ctx context.Context) error {
	defer running.Store(false)
	var errs []error

	if s.api != nil {
		if err := s.api.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if s.proxy != nil {
		if err := s.proxy.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop gateway: %w", err))
		}
	}
	if s.control != nil {
		// Streams close here; pushes sent on them and not yet answered go
		// back to pending for the replica each edge reconnects to.
		s.control.Stop()
	}
	if s.pushes != nil {
		s.pushes.Stop()
	}
	s.edgePayloads.stop()
	s.relayed.stop()
	s.stopCluster(ctx)
	if s.analyticsTee != nil {
		// The handler is process-wide: unwrap it so a later New in this
		// process does not tee into this instance's stopped sinks.
		if analytics.GetHandler() == analytics.AnalyticsHandler(s.analyticsTee) {
			analytics.SetHandler(s.analyticsPrimary)
		}
		s.analyticsTee.Stop(5 * time.Second)
	}
	if s.service != nil {
		if err := s.service.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.scheduler != nil {
		if err := s.scheduler.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop scheduler: %w", err))
		}
	}
	if s.telemetry != nil {
		s.telemetry.Stop()
	}
	// Stops the marketplace sync and the analytics recorder, among others.
	if s.cancelBackground != nil {
		s.cancelBackground()
		// Let the next Studio in this process start a fresh recorder.
		analytics.ResetHandler()
	}
	if s.tracingShutdown != nil {
		if err := s.tracingShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("flush traces: %w", err))
		}
	}
	if s.licensing != nil {
		s.licensing.Stop()
	}
	return errors.Join(errs...)
}

// cookiePath scopes Studio's cookies to its base path.
func cookiePath(basePath string) string {
	if basePath = config.NormalizeBasePath(basePath); basePath != "" {
		return basePath
	}
	return "/"
}

// ReloadLicense re-reads the licence (Options.License, or Config.LicenseKey)
// and validates it now, for a host that has just stored a renewed one.
// Community Edition has no licence and returns nil. An error leaves the
// licence Studio already holds in place; the host can show it.
func (s *Studio) ReloadLicense() error {
	if r, ok := s.licensing.(licensing.Revalidator); ok {
		return r.Revalidate()
	}
	return nil
}

// LicenseStatus is the Enterprise licence as Studio holds it, for a host to
// show next to its own licence.
type LicenseStatus struct {
	Enterprise bool      // an Enterprise build (Community Edition has no licence)
	Valid      bool      // the licence is valid now
	ExpiresAt  time.Time // zero without a licence
	DaysLeft   int       // -1 without a licence
	// Entitlements are the licence's feature names.
	Entitlements []string
}

// LicenseStatus reports the licence Studio holds.
func (s *Studio) LicenseStatus() LicenseStatus {
	st := LicenseStatus{Enterprise: config.IsEnterprise(), Valid: s.licensing.IsValid(), DaysLeft: s.licensing.DaysLeft()}
	if info := s.licensing.GetLicenseInfo(); info != nil {
		st.ExpiresAt = info.ExpiresAt
		for name := range info.Features {
			st.Entitlements = append(st.Entitlements, name)
		}
		sort.Strings(st.Entitlements)
	}
	return st
}
