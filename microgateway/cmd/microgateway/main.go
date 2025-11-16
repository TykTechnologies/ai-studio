// cmd/microgateway/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/licensing"
	"github.com/TykTechnologies/midsommar/microgateway/internal/providers"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Version information (set by build)
var (
	Version   = "dev"
	BuildHash = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Parse command line flags
	var (
		envFile          = flag.String("env", ".env", "Path to environment file")
		migrate          = flag.Bool("migrate", false, "Run database migrations and exit")
		version          = flag.Bool("version", false, "Show version and exit")
		createAdminToken = flag.Bool("create-admin-token", false, "Create admin token and exit")
		adminName        = flag.String("admin-name", "Admin User", "Name for admin token")
		adminExpires     = flag.String("admin-expires", "720h", "Admin token expiration (e.g., 24h, 720h)")
		_                = flag.String("config", "", "Path to config file (optional)")
	)
	flag.Parse()

	// Show version if requested
	if *version {
		fmt.Printf("Microgateway v%s\n", Version)
		fmt.Printf("Build Hash: %s\n", BuildHash)
		fmt.Printf("Build Time: %s\n", BuildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*envFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logging
	setupLogging(cfg.Observability)

	log.Info().
		Str("version", Version).
		Str("build", BuildHash).
		Str("build_time", BuildTime).
		Str("gateway_mode", cfg.HubSpoke.Mode).
		Msg("Starting Microgateway")

	// Connect to database
	dbConfig := database.DatabaseConfig{
		Type:            cfg.Database.Type,
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		AutoMigrate:     cfg.Database.AutoMigrate,
		LogLevel:        cfg.Database.LogLevel,
	}

	db, err := database.Connect(dbConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer func() {
		if err := database.Close(db); err != nil {
			log.Error().Err(err).Msg("Failed to close database connection")
		}
	}()

	// Run migrations if requested or auto-migrate is enabled
	if *migrate || cfg.Database.AutoMigrate {
		log.Info().Msg("Running database migrations...")
		if err := database.Migrate(db); err != nil {
			log.Fatal().Err(err).Msg("Failed to run migrations")
		}
		if *migrate {
			log.Info().Msg("Migrations completed successfully")
			os.Exit(0)
		}
	}

	// Check database health
	if err := database.IsHealthy(db); err != nil {
		log.Fatal().Err(err).Msg("Database health check failed")
	}

	// Initialize and start licensing service (ENT: validates license, CE: no-op)
	licensingConfig := licensing.Config{
		LicenseKey:          os.Getenv("TYK_AI_LICENSE"),
		ValidityCheckPeriod: 24 * time.Hour, // Re-validate every 24 hours
	}
	licensingService := licensing.NewService(licensingConfig)
	if err := licensingService.Start(); err != nil {
		log.Fatal().Err(err).Msg("License validation failed")
	}
	defer licensingService.Stop()
	log.Info().Msg("Licensing service initialized")

	// Initialize edge client FIRST if in edge mode (before creating services)
	var edgeClient *grpc.SimpleEdgeClient
	if cfg.IsEdge() {
		log.Info().
			Str("control_endpoint", cfg.HubSpoke.ControlEndpoint).
			Str("edge_id", cfg.HubSpoke.EdgeID).
			Msg("Connecting to control server before initializing services")
		
		edgeClient = grpc.NewSimpleEdgeClient(cfg, Version, BuildHash, BuildTime)
		
		if err := edgeClient.Start(); err != nil {
			log.Fatal().Err(err).Msg("Failed to connect to control server - edge cannot start without configuration")
		}
		
		log.Info().Msg("Successfully connected to control server")
	}

	// Initialize hub-spoke service container based on gateway mode
	var serviceContainer *services.ServiceContainer
	var hubSpokeContainer *services.HubSpokeServiceContainer
	
	// Always use hub-spoke container for control and edge modes
	if cfg.IsControl() || cfg.IsEdge() {
		log.Info().
			Str("gateway_mode", cfg.HubSpoke.Mode).
			Msg("Initializing hub-spoke service container")
		hubSpokeContainer, err = services.NewHubSpokeServiceContainer(db, cfg)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize hub-spoke services")
		}
		serviceContainer = hubSpokeContainer.ServiceContainer
		
		// Connect edge client to provider if in edge mode
		if cfg.IsEdge() && edgeClient != nil {
			if grpcProvider, ok := hubSpokeContainer.ConfigProvider.(*providers.GRPCProvider); ok {
				log.Info().Msg("Connecting edge client to gRPC provider")
				
				// Set up callback for configuration updates
				edgeClient.SetOnConfigChange(func(config *pb.ConfigurationSnapshot) {
					log.Info().
						Str("version", config.Version).
						Int("llm_count", len(config.Llms)).
						Int("app_count", len(config.Apps)).
						Msg("Received configuration update from control, syncing to local SQLite")
					
					// Create sync service and sync to local SQLite with join tables
					syncService := services.NewEdgeSyncService(db, cfg.HubSpoke.EdgeNamespace)
					if err := syncService.SyncConfiguration(config); err != nil {
						log.Error().Err(err).Msg("Failed to sync configuration to local SQLite")
					} else {
						log.Info().Msg("Configuration synced to local SQLite successfully")
					}
					
					// Also update gRPC provider cache for compatibility
					grpcProvider.SetConfigurationCache(config)
				})
				
				grpcProvider.SetEdgeClient(edgeClient)
				
				// If edge client already has configuration, sync it to SQLite
				if initialConfig := edgeClient.GetCurrentConfiguration(); initialConfig != nil {
					log.Info().
						Str("version", initialConfig.Version).
						Msg("Setting initial configuration from edge client, syncing to local SQLite")
					
					// Sync initial configuration to SQLite
					syncService := services.NewEdgeSyncService(db, cfg.HubSpoke.EdgeNamespace)
					if err := syncService.SyncConfiguration(initialConfig); err != nil {
						log.Error().Err(err).Msg("Failed to sync initial configuration to local SQLite")
					} else {
						log.Info().Msg("Initial configuration synced to local SQLite successfully")
					}
					
					grpcProvider.SetConfigurationCache(initialConfig)
				}
				
				// Also connect edge client to hybrid gateway service for on-demand token validation
				if hybridGateway, ok := hubSpokeContainer.GatewayService.(*services.HybridGatewayService); ok {
					hybridGateway.SetEdgeClient(edgeClient)
					log.Info().Msg("Edge client connected to hybrid gateway service for token validation")
				}
				
				// Create and connect edge reload handler for distributed reload operations
				syncService := services.NewEdgeSyncService(db, cfg.HubSpoke.EdgeNamespace)
				reloadHandler := services.NewEdgeReloadHandler(
					edgeClient,
					syncService,
					db,
					cfg.HubSpoke.EdgeID,
					func(response *pb.ConfigurationReloadResponse) {
						// Send reload status back to control via edge client
						if err := edgeClient.SendReloadStatus(response); err != nil {
							log.Error().Err(err).Msg("Failed to send reload status to control")
						}
					},
				)
				edgeClient.SetReloadHandler(reloadHandler)
				log.Info().Msg("Edge reload handler created and connected to edge client")

				// Connect edge client to plugin manager for built-in plugins
				serviceContainer.PluginManager.SetEdgeClient(edgeClient)
				log.Info().Msg("Edge client connected to plugin manager for built-in plugin support")

				// Load any deferred built-in plugins (like analytics_pulse)
				if err := serviceContainer.PluginManager.LoadDeferredBuiltinPlugins(cfg.Plugins.DataCollectionPlugins); err != nil {
					log.Error().Err(err).Msg("Failed to load deferred built-in plugins")
				}

				log.Info().Msg("Edge client connected to gRPC provider")
			}
		}
	} else {
		// Standalone instances use traditional service container
		log.Info().Msg("Initializing traditional service container for standalone mode")
		serviceContainer, err = services.NewServiceContainer(db, cfg)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize services")
		}
	}

	// Create admin token if requested
	if *createAdminToken {
		token, err := createAdminTokenCommand(serviceContainer, *adminName, *adminExpires)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create admin token")
		}
		
		fmt.Printf("✅ Admin token created successfully!\n")
		fmt.Printf("Token: %s\n", token)
		fmt.Printf("Name: %s\n", *adminName)
		fmt.Printf("Expires: %s\n", *adminExpires)
		fmt.Printf("\nSave this token - it won't be shown again!\n")
		fmt.Printf("Use it with the CLI: export MGW_TOKEN=\"%s\"\n", token)
		os.Exit(0)
	}

	// Create and configure server
	srv, err := server.New(cfg, serviceContainer, Version, BuildHash, BuildTime)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Info().
			Int("port", cfg.Server.Port).
			Str("host", cfg.Server.Host).
			Bool("tls", cfg.Server.TLSEnabled).
			Msg("Starting HTTP server")

		if err := srv.Start(); err != nil {
			serverErrors <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Start gRPC control server if in control mode
	var controlServer *grpc.ControlServer
	var reloadCoordinator *services.ReloadCoordinator
	
	if cfg.IsControl() {
		log.Info().
			Int("grpc_port", cfg.HubSpoke.GRPCPort).
			Msg("Starting gRPC control server for configuration synchronization")
		
		controlServer = grpc.NewControlServer(cfg, db)
		
		// Create reload coordinator and connect it to control server
		reloadCoordinator = services.NewReloadCoordinator(controlServer)
		controlServer.SetReloadCoordinator(reloadCoordinator)
		
		// Update server with reload coordinator for API endpoints
		srv.SetReloadCoordinator(reloadCoordinator)
		
		log.Info().Msg("Reload coordinator created and connected to control server")
		
		go func() {
			if err := controlServer.Start(); err != nil {
				log.Error().Err(err).Msg("gRPC control server failed to start")
			}
		}()
	}
	
	// Start background tasks and hub-spoke specific services
	go func() {
		log.Info().Msg("Starting background tasks")
		serviceContainer.StartBackgroundTasks(ctx)
		
		// Start hub-spoke specific services if available
		if hubSpokeContainer != nil {
			if err := hubSpokeContainer.StartHubSpokeServices(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to start hub-spoke services")
			}
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		log.Error().Err(err).Msg("Server error occurred")
		stop()
	case <-ctx.Done():
		log.Info().Msg("Shutdown signal received")
	}

	// Graceful shutdown
	log.Info().Msg("Starting graceful shutdown...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Stop background tasks and hub-spoke services
	log.Info().Msg("Stopping background tasks")
	serviceContainer.StopBackgroundTasks()
	
	// Stop gRPC components
	if controlServer != nil {
		log.Info().Msg("Stopping gRPC control server")
		controlServer.Stop()
	}
	
	if edgeClient != nil {
		log.Info().Msg("Stopping gRPC edge client")
		edgeClient.Stop()
	}
	
	// Stop hub-spoke specific services
	if hubSpokeContainer != nil {
		log.Info().Msg("Stopping hub-spoke services")
		hubSpokeContainer.StopHubSpokeServices()
	}

	// Shutdown server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	// Final cleanup
	serviceContainer.Cleanup()

	log.Info().Msg("Microgateway stopped gracefully")
}

// setupLogging configures the global logger based on configuration
func setupLogging(cfg config.ObservabilityConfig) {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output format
	if cfg.LogFormat == "text" {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	} else {
		log.Logger = log.With().Timestamp().Logger()
	}

	// Set global timestamp format
	zerolog.TimeFieldFormat = time.RFC3339Nano
}

// createAdminTokenCommand creates an admin token for management API access
func createAdminTokenCommand(serviceContainer *services.ServiceContainer, name, expiresStr string) (string, error) {
	// Parse expiration duration
	expires, err := time.ParseDuration(expiresStr)
	if err != nil {
		return "", fmt.Errorf("invalid expiration format '%s' (use format like '24h', '720h'): %w", expiresStr, err)
	}

	// Ensure admin app exists (ID = 1, reserved for admin operations)
	adminApp, err := ensureAdminAppExists(serviceContainer)
	if err != nil {
		return "", fmt.Errorf("failed to create admin app: %w", err)
	}

	// Generate admin token with admin scope
	scopes := []string{"admin"}
	token, err := serviceContainer.AuthProvider.GenerateToken(adminApp.ID, name, scopes, expires)
	if err != nil {
		return "", fmt.Errorf("failed to generate admin token: %w", err)
	}

	return token, nil
}

// ensureAdminAppExists creates the admin app if it doesn't exist
func ensureAdminAppExists(serviceContainer *services.ServiceContainer) (*database.App, error) {
	// Try to get existing admin app (ID = 1)
	adminApp, err := serviceContainer.Management.GetApp(1)
	if err == nil {
		// Admin app already exists
		return adminApp, nil
	}

	// Create admin app
	adminAppReq := &services.CreateAppRequest{
		Name:           "Admin System",
		Description:    "Administrative access for microgateway management",
		OwnerEmail:     "admin@microgateway.local",
		MonthlyBudget:  0, // No budget limit for admin
		BudgetResetDay: 1,
		RateLimitRPM:   0, // No rate limit for admin
	}

	adminApp, err = serviceContainer.Management.CreateApp(adminAppReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin app: %w", err)
	}

	log.Info().Uint("app_id", adminApp.ID).Msg("Created admin app for token management")
	return adminApp, nil
}