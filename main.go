package main

import (
	"os"
	"strconv"

	"gorm.io/driver/postgres"

	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	logrus "github.com/sirupsen/logrus"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/docs"
	"github.com/TykTechnologies/midsommar/v2/grpc"
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

	// Set up debug logging
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	appConf := config.Get()

	// Perform connectivity tests before proceeding with initialization
	if err := startup.TestConnectivity(appConf); err != nil {
		log.Fatalf("Connectivity tests failed: %v", err)
	}

	var dialector gorm.Dialector
	switch appConf.DatabaseType {
	case "sqlite":
		dialector = sqlite.Open(appConf.DatabaseURL)
	case "postgres":
		dialector = postgres.Open(appConf.DatabaseURL)
	default:
		log.Fatalf("Unsupported database type: %s", appConf.DatabaseType)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Test the database connection
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get database instance: %v", err)
	}
	err = sqlDB.Ping()
	if err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Successfully connected to the database")

	// Auto Migrate the schemas
	err = models.InitModels(db)
	if err != nil {
		log.Fatalf("Failed to initialize models: %v", err)
	}

	// Create a new service instance with OCI support if configured
	var ociConfig *ociplugins.OCIConfig
	if appConf.OCIPlugins.IsEnabled() {
		ociConfig = appConf.OCIPlugins.ToOCILibConfig()
		log.Printf("🔧 OCI plugin support enabled - cache dir: %s", appConf.OCIPlugins.CacheDir)
	} else {
		log.Println("ℹ️  OCI plugin support disabled - set AI_STUDIO_OCI_CACHE_DIR to enable")
	}

	service := services.NewServiceWithOCI(db, ociConfig)

	// Load AI Studio plugins at startup
	if service.AIStudioPluginManager != nil {
		log.Println("🔌 Loading AI Studio plugins...")
		if err := service.AIStudioPluginManager.LoadAllAIStudioPlugins(); err != nil {
			log.Printf("⚠️  Failed to load some AI Studio plugins: %v", err)
		} else {
			log.Println("✅ AI Studio plugins loaded successfully")
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
		
		log.Printf("✅ Reload coordinator created and connected to control server and namespace service")
		
		go func() {
			log.Printf("Starting AI Studio gRPC control server on port %d", appConf.GRPCPort)
			if err := controlServer.Start(); err != nil {
				log.Fatalf("Failed to start gRPC control server: %v", err)
			}
		}()
		
		// Graceful shutdown of gRPC server
		defer func() {
			if controlServer != nil {
				log.Printf("Shutting down gRPC control server...")
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
		log.Println("server listening on", listenOn)
		if err := api.Run(listenOn, appConf.CertFile, appConf.KeyFile); err != nil {
			log.Fatalf("Failed to run server: %v", err)
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
