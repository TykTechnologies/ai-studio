package studio

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/edition"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ControlPlaneOptions configures a headless control plane. Config and DB are
// required.
type ControlPlaneOptions struct {
	// Config is Studio's configuration (config.Load or config.LoadFrom).
	// The control plane reads the gRPC settings (GRPCHost, GRPCPort,
	// GRPCAuthToken, GRPCNextAuthToken, the TLS files unless TLSConfig is
	// set, BudgetSyncInterval), MicrogatewayEncryptionKey, SecretKey,
	// LicenseKey, AnalyticsBufferSize and LogLevel. NewControlPlane installs
	// it as the process-wide configuration.
	Config *config.AppConf

	// DB is the database a full Studio (New) migrates and runs on: Postgres
	// only, since the replicas coordinate through it. The control plane
	// never changes its schema (see CheckSchema) and the caller closes it
	// after Stop.
	DB *gorm.DB

	// Version is recorded with this replica in the cluster registry.
	Version string

	// Logger receives the control plane's logging. Nil logs JSON to stderr
	// at Config.LogLevel (info when empty).
	Logger *zerolog.Logger

	// License supplies the AI Studio Enterprise licence, as for New; nil uses
	// Config.LicenseKey. OnLicenceInvalid is called when it fails its
	// periodic re-check; nil exits the process. The control plane sends no
	// licence telemetry: the full Studio does.
	License          func() string
	OnLicenceInvalid func(error)

	// TLSConfig, when set, is the TLS configuration edges are served with,
	// in place of Config's certificate and key files.
	TLSConfig *tls.Config

	// NodeID identifies this replica in the cluster registry (pkg/cluster)
	// and as the owner of the edge streams it holds. Empty means a fresh ID
	// per process.
	NodeID string

	// TracerProvider, Propagator and MeterProvider are the host's; when nil
	// the control plane records no spans or metrics of its own. It never
	// starts an exporter or serves a metrics endpoint.
	TracerProvider trace.TracerProvider
	Propagator     propagation.TextMapPropagator
	MeterProvider  metric.MeterProvider

	// AnalyticsSinks receive a copy of every analytics record edges send
	// (as for New), besides the shared database.
	AnalyticsSinks []analytics.AnalyticsHandler
}

// ErrControlPlaneNeedsPostgres is returned by NewControlPlane for a database
// other than Postgres: a headless control plane shares a full Studio's
// database, and replicas coordinate only through Postgres.
var ErrControlPlaneNeedsPostgres = errors.New("studio: a headless control plane needs a Postgres database shared with a full Studio")

// ControlPlane is a headless AI Studio control plane: a replica that holds
// edge (microgateway) gRPC streams next to a full Studio sharing its
// database. It serves edges their configuration snapshots, delivers pushes
// to the edges whose streams it holds, records the analytics they send, and
// relays events between its edges and the other replicas. It runs no API,
// UI, gateway, plugins, marketplace, scheduler or telemetry, never migrates
// the database, and never takes the leader lease, so singleton work (budget
// blocks and budget.sync, marketplace sync, telemetry) stays with the full
// Studio and reaches this replica's edges through the relay.
//
// Only one ControlPlane or Studio may run in a process at a time.
type ControlPlane struct {
	conf *config.AppConf
	db   *gorm.DB

	clusterParts

	licensing licensing.Service
	control   *grpc.ControlServer
	pushes    *pushes.Coordinator

	cancelBackground context.CancelFunc
	analyticsTee     *analytics.Tee
	analyticsPrimary analytics.AnalyticsHandler

	stopOnce sync.Once
	stopErr  error
}

// NewControlPlane starts a headless control plane on a database a full
// Studio has migrated. It refuses a schema this build does not understand
// (CheckSchema), joins the cluster as a replica that never leads, and builds
// the gRPC control server; nothing listens until Serve.
func NewControlPlane(opts ControlPlaneOptions) (_ *ControlPlane, err error) {
	if opts.Config == nil || opts.DB == nil {
		return nil, errors.New("studio: ControlPlaneOptions.Config and ControlPlaneOptions.DB are required")
	}
	if opts.DB.Dialector.Name() != "postgres" {
		return nil, ErrControlPlaneNeedsPostgres
	}
	if !running.CompareAndSwap(false, true) {
		return nil, ErrAlreadyRunning
	}

	c := &ControlPlane{conf: opts.Config, db: opts.DB}
	defer func() {
		if err != nil {
			c.stop(context.Background())
		}
	}()

	conf := opts.Config
	config.Set(conf)
	if opts.Logger != nil {
		logger.Use(*opts.Logger)
	} else {
		logger.Use(defaultControlPlaneLogger(conf.LogLevel))
	}

	if err := edition.CheckRegistered(); err != nil {
		return nil, err
	}
	secrets.SetEncryptionKey(conf.SecretKey)
	analytics.SetBufferSize(conf.AnalyticsBufferSize)
	metrics.SetLegacyNames(!conf.MetricsNoLegacyNames)
	secrets.WarnIfEncryptionUnconfigured()

	backgroundCtx, cancel := context.WithCancel(context.Background())
	c.cancelBackground = cancel

	// Before anything writes: the schema must be one this build reads.
	if err := CheckSchema(backgroundCtx, c.db); err != nil {
		return nil, err
	}

	c.licensing = licensing.NewService(licensing.Config{
		LicenseKey:          conf.LicenseKey,
		LicenseSource:       opts.License,
		TelemetryDisabled:   true,
		ValidityCheckPeriod: conf.LicenseValidityPeriod,
		OnInvalid:           opts.OnLicenceInvalid,
	}, c.db)
	if err := c.licensing.Start(); err != nil {
		return nil, fmt.Errorf("studio: start licensing: %w", err)
	}

	nodeID := opts.NodeID
	if nodeID == "" {
		nodeID = cluster.NewNodeID()
	}
	if err := c.joinCluster(backgroundCtx, c.db, nodeID, opts.Version); err != nil {
		return nil, err
	}
	// No leader lease: pkg/replicas answers "not the leader" for good, and
	// replica signals (budgets, governed metadata) still reach this
	// replica's caches.
	c.connectReplicas()

	if opts.MeterProvider != nil {
		metrics.InitWithProvider(opts.MeterProvider)
	}
	if opts.TracerProvider != nil {
		tracing.Use(opts.TracerProvider, opts.Propagator)
	}

	analytics.StartRecording(backgroundCtx, c.db)
	if len(opts.AnalyticsSinks) > 0 {
		c.analyticsPrimary = analytics.GetHandler()
		c.analyticsTee = analytics.NewTee(c.analyticsPrimary, opts.AnalyticsSinks...)
		analytics.SetHandler(c.analyticsTee)
	}

	c.control, err = grpc.NewControlServer(controlServerConfig(conf, nodeID, opts.TLSConfig), c.db)
	if err != nil {
		return nil, fmt.Errorf("studio: create gRPC control server: %w", err)
	}
	// Read-only: snapshots carry gateway-visible governed metadata
	// (Enterprise), with no hooks or events of its own.
	c.control.SetGovernedMetadataReader(governed_metadata.NewService(c.db, governed_metadata.Deps{}))

	c.pushes = pushes.New(c.db, nodeID, c.control, pushes.Options{})
	c.control.SetPushDelivery(c.pushes)
	if err := c.pushes.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("studio: start edge push delivery: %w", err)
	}

	// Edge-to-control traffic is for plugins, which run on the full
	// replicas: what edges publish is relayed to them, and plugin payloads
	// go to the leader through the log (edge_payloads.go).
	c.control.SetEdgePayloadForwarder(edgePayloadForwarder{log: c.clusterLog})
	c.startRelay(c.control.GetEventBus(), cluster.RelayOptions{Filter: headlessRelayFilter})
	logger.Infof("Headless control plane ready as replica %s", nodeID)
	return c, nil
}

// defaultControlPlaneLogger logs JSON to stderr at level (info when empty or
// unknown), without touching zerolog's global logger or level, which belong
// to the host.
func defaultControlPlaneLogger(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	return zerolog.New(os.Stderr).Level(lvl).With().Timestamp().Logger()
}

// NodeID is this replica's ID in the cluster registry.
func (c *ControlPlane) NodeID() string { return c.clusterNode.ID() }

// Serve serves the gRPC control server for edge gateways on listener, or on
// Config.GRPCHost:GRPCPort when listener is nil. It blocks until Stop is
// called or serving fails.
func (c *ControlPlane) Serve(listener net.Listener) error {
	if listener == nil {
		return c.control.Start()
	}
	return c.control.Serve(listener)
}

// Stop shuts the control plane down in reverse order: edge streams (their
// unanswered pushes go back to pending for the replica each edge reconnects
// to), push delivery, the cluster membership, analytics, licensing. It leaves
// the database open for its owner to close. Stop is safe to call more than
// once; after it returns, NewControlPlane or New may start another instance.
func (c *ControlPlane) Stop(ctx context.Context) error {
	c.stopOnce.Do(func() { c.stopErr = c.stop(ctx) })
	return c.stopErr
}

func (c *ControlPlane) stop(ctx context.Context) error {
	defer running.Store(false)
	if c.control != nil {
		c.control.Stop()
	}
	if c.pushes != nil {
		c.pushes.Stop()
	}
	c.stopCluster(ctx)
	if c.analyticsTee != nil {
		if analytics.GetHandler() == analytics.AnalyticsHandler(c.analyticsTee) {
			analytics.SetHandler(c.analyticsPrimary)
		}
		c.analyticsTee.Stop(5 * time.Second)
	}
	if c.cancelBackground != nil {
		c.cancelBackground()
		analytics.ResetHandler()
	}
	if c.licensing != nil {
		c.licensing.Stop()
	}
	return nil
}
