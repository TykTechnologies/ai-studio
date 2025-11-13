package main

import (
	"os"
	"strconv"

	"gorm.io/driver/postgres"

	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/docs"
	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	_ "github.com/TykTechnologies/midsommar/v2/services/grpc" // Initialize AIStudioManagementServer factory
	"github.com/TykTechnologies/midsommar/v2/startup"
	"github.com/go-mail/mail"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed ui/admin-frontend/build templates docs/site/public
var staticFiles embed.FS

func printWelcome() {
	fmt.Printf("Starting Tyk AI Portal %v\n", "v2.0-hub-spoke")
	fmt.Println("Copyright Tyk Technologies, 2024")
}

func main() {
	printWelcome()

	// Get configuration first to initialize logger with correct level
	appConf := config.Get()

	// Initialize logger with configured level
	logger.Init(appConf.LogLevel)
	logger.Infof("Log level set to: %s", appConf.LogLevel)

	// Perform connectivity tests before proceeding with initialization
	if err := startup.TestConnectivity(appConf); err != nil {
		logger.FatalErr("Connectivity tests failed", err)
	}

	var dialector gorm.Dialector
	switch appConf.DatabaseType {
	case "sqlite":
		dialector = sqlite.Open(appConf.DatabaseURL)
	case "postgres":
		dialector = postgres.Open(appConf.DatabaseURL)
	default:
		logger.Fatalf("Unsupported database type: %s", appConf.DatabaseType)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		logger.FatalErr("Failed to connect to database", err)
	}

	// Test the database connection
	sqlDB, err := db.DB()
	if err != nil {
		logger.FatalErr("Failed to get database instance", err)
	}
	err = sqlDB.Ping()
	if err != nil {
		logger.FatalErr("Failed to ping database", err)
	}
	logger.Info("Successfully connected to the database")

	// Auto Migrate the schemas
	err = models.InitModels(db)
	if err != nil {
		logger.FatalErr("Failed to initialize models", err)
	}

	// Initialize branding storage directory
	brandingStoragePath := services.GetBrandingStoragePath()
	_, err = services.NewBrandingFileStorage(brandingStoragePath)
	if err != nil {
		logger.Warnf("Failed to initialize branding storage: %v", err)
	} else {
		logger.Infof("Branding storage initialized at: %s", brandingStoragePath)
	}

	// Create a new service instance with OCI support if configured
	var ociConfig *ociplugins.OCIConfig
	if appConf.OCIPlugins.IsEnabled() {
		ociConfig = appConf.OCIPlugins.ToOCILibConfig()
		logger.Infof("OCI plugin support enabled - cache dir: %s", appConf.OCIPlugins.CacheDir)
	} else {
		logger.Info("OCI plugin support disabled - set AI_STUDIO_OCI_CACHE_DIR to enable")
	}

	service := services.NewServiceWithOCI(db, ociConfig)

	// Load AI Studio plugins at startup (UI, Agent, and Object Hooks)
	if service.AIStudioPluginManager != nil {
		logger.Info("Loading AI Studio plugins (UI, Agent, Object Hooks)...")
		if err := service.AIStudioPluginManager.LoadAllUIAndAgentPlugins(); err != nil {
			logger.Warnf("Failed to load some AI Studio plugins: %v", err)
		} else {
			logger.Info("AI Studio plugins loaded successfully")
		}
	}

	// Initialize and start marketplace service if enabled
	if appConf.MarketplaceEnabled && ociConfig != nil {
		logger.Info("Initializing marketplace service...")

		// Get OCI client from plugin service
		var ociClient *ociplugins.OCIPluginClient
		if service.PluginService != nil {
			ociClient, _ = ociplugins.NewOCIPluginClient(ociConfig)
		}

		// Create marketplace service
		service.MarketplaceService = services.NewMarketplaceService(
			db,
			ociClient,
			service.PluginService,
			service.AIStudioPluginManager,
			appConf.MarketplaceCacheDir,
			appConf.MarketplaceIndexURL,
			appConf.MarketplaceSyncInterval,
		)

		// Start background sync in a goroutine
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go service.MarketplaceService.Start(ctx)

		logger.Infof("Marketplace service started - index URL: %s, sync interval: %v",
			appConf.MarketplaceIndexURL, appConf.MarketplaceSyncInterval)
	} else {
		if !appConf.MarketplaceEnabled {
			logger.Info("Marketplace is disabled via MARKETPLACE_ENABLED=false")
		} else if ociConfig == nil {
			logger.Warn("Marketplace requires OCI support - set AI_STUDIO_OCI_CACHE_DIR to enable")
		}
	}

	// Initialize mail service and notification service
	mailer := mail.NewDialer(appConf.SMTPServer, appConf.SMTPPort, appConf.SMTPUser, appConf.SMTPPass)
	mailService := notifications.NewMailService(
		appConf.FromEmail,
		appConf.SMTPServer,
		appConf.SMTPPort,
		appConf.SMTPUser,
		appConf.SMTPPass,
		mailer,
		appConf.DevMode,
	)

	// Create notification service that will handle all notifications
	notificationService := services.NewNotificationService(
		db,
		appConf.FromEmail,
		appConf.SMTPServer,
		appConf.SMTPPort,
		appConf.SMTPUser,
		appConf.SMTPPass,
		mailer,
	)

	// Initialize auth config and service
	config := &auth.Config{
		DB:                     db,
		Service:                service,
		CookieName:             "session",
		CookieSecure:           !appConf.DevMode,
		CookieHTTPOnly:         true,
		CookieSameSite:         http.SameSiteLaxMode, // less restrictive
		CookieDomain:           "",                   // empty for development to work with localhost
		ResetTokenExpiry:       time.Hour,
		FrontendURL:            appConf.SiteURL,
		RegistrationAllowed:    appConf.AllowRegistrations,
		AdminEmail:             appConf.AdminEmail,
		TestMode:               false, // Always false in production - tests set this directly
		AllowedRegisterDomains: appConf.FilterSignupDomains,
		TIBEnabled:             appConf.TIBEnabled,
		TIBAPISecret:           appConf.TIBAPISecret,
	}

	authService := auth.NewAuthService(config, mailService, service, notificationService)

	// analytics
	ctx, stopRec := context.WithCancel(context.Background())
	defer stopRec()
	analytics.StartRecording(ctx, db)
	budgetService := services.NewBudgetService(db, notificationService)

	// Initialize and start telemetry
	telemetryManager := services.NewTelemetryManager(db, appConf.TelemetryEnabled, "v2.0-hub-spoke")
	telemetryManager.Start()
	defer telemetryManager.Stop()

	// start the Proxy
	pConfig := &proxy.Config{
		Port: 9090,
	}
	p := proxy.NewProxy(service, pConfig, budgetService)

	// Always enable gateway
	go p.Start()

	// Initialize gRPC control server and reload coordinator if in control mode
	var controlServer *grpc.ControlServer
	var reloadCoordinator *services.ReloadCoordinator
	if appConf.GatewayMode == "control" {
		grpcConfig := &grpc.Config{
			GRPCPort:      appConf.GRPCPort,
			GRPCHost:      appConf.GRPCHost,
			TLSEnabled:    appConf.GRPCTLSEnabled,
			TLSCertPath:   appConf.GRPCTLSCertPath,
			TLSKeyPath:    appConf.GRPCTLSKeyPath,
			AuthToken:     appConf.GRPCAuthToken,
			NextAuthToken: appConf.GRPCNextAuthToken,
		}
		
		controlServer = grpc.NewControlServer(grpcConfig, db)
		
		// Create reload coordinator and connect it to control server
		reloadCoordinator = services.NewReloadCoordinator(controlServer)
		controlServer.SetReloadCoordinator(reloadCoordinator)
		
		// Connect reload coordinator to namespace service
		service.NamespaceService.SetReloadCoordinator(reloadCoordinator)

		logger.Info("Reload coordinator created and connected to control server and namespace service")

		go func() {
			logger.Infof("Starting AI Studio gRPC control server on port %d", appConf.GRPCPort)
			if err := controlServer.Start(); err != nil {
				logger.FatalErr("Failed to start gRPC control server", err)
			}
		}()

		// Graceful shutdown of gRPC server
		defer func() {
			if controlServer != nil {
				logger.Info("Shutting down gRPC control server...")
				controlServer.Stop()
			}
		}()
	}

	noDocsArg := false
	docsPortArg := 8989
	for i, arg := range os.Args {
		if arg == "--no-docs" {
			noDocsArg = true
		}
		if arg == "--docs-port" && i+1 < len(os.Args) {
			if port, err := strconv.Atoi(os.Args[i+1]); err == nil {
				docsPortArg = port
			}
		}
	}

	if !noDocsArg {
		docsServer := docs.NewServer(docsPortArg)
		go docsServer.Start()
	}

	if !appConf.ProxyOnly {
		// Create a new API instance
		api := api.NewAPI(service, appConf.DisableCors, authService, config, p, staticFiles, nil) // true to disable CORS for development

		// listEmbeddedFiles(staticFiles)
		// Run the API
		listenOn := fmt.Sprintf(":%s", appConf.ServerPort)
		logger.Infof("Server listening on %s", listenOn)
		if err := api.Run(listenOn, appConf.CertFile, appConf.KeyFile); err != nil {
			logger.FatalErr("Failed to run server", err)
		}
	} else {
		// wait for Ctrl+C
		select {}
	}
}

func listEmbeddedFiles(fsys embed.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fmt.Printf("Path: %s, IsDir: %t\n", path, d.IsDir())
		return nil
	})
}
