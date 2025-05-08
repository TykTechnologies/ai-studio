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
	"github.com/TykTechnologies/midsommar/v2/licensing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/go-mail/mail"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed ui/admin-frontend/build templates docs/site/public
var staticFiles embed.FS

func printWelcome() {
	fmt.Printf("Starting Tyk AI Portal %v\n", VERSION)
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

	licenseConfig := licensing.LicenseConfig{
		LicenseKey:          appConf.LicenseKey,
		ValidityCheckPeriod: 10 * time.Minute,
		TelemetryPeriod:     appConf.LicenseTelemetryPeriod,
		DisableTelemetry:    appConf.LicenseDisableTelemetry,
		TelemetryURL:        appConf.LicenseTelemetryURL,
		Version:             VERSION,
		Component:           "tyk-ai-studio",
		TelemetryService:    services.NewTelemetryService(db),
	}

	licenser := licensing.NewLicenser(licenseConfig)

	licenser.Start()
	defer licenser.Stop()

	// Create a new service instance
	service := services.NewService(db)

	// Initialize mail service and notification service
	mailer := mail.NewDialer(appConf.SMTPServer, appConf.SMTPPort, appConf.SMTPUser, appConf.SMTPPass)
	mailService := notifications.NewMailService(
		appConf.FromEmail,
		appConf.SMTPServer,
		appConf.SMTPPort,
		appConf.SMTPUser,
		appConf.SMTPPass,
		mailer,
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
		CookieSecure:           os.Getenv("DEVMODE") == "", // false in dev mode
		CookieHTTPOnly:         true,
		CookieSameSite:         http.SameSiteLaxMode, // less restrictive
		CookieDomain:           "",                   // empty for development to work with localhost
		ResetTokenExpiry:       time.Hour,
		FrontendURL:            appConf.SiteURL,
		RegistrationAllowed:    appConf.AllowRegistrations,
		AdminEmail:             appConf.AdminEmail,
		TestMode:               os.Getenv("DEVMODE") == "true" || os.Getenv("DEVMODE") == "1",
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

	// start the Proxy
	pConfig := &proxy.Config{
		Port: 9090,
	}
	p := proxy.NewProxy(service, pConfig, budgetService)

	gatewayEnabled, gatewayOk := licenser.Entitlement(licensing.FEATUREGateway)
	if gatewayOk && gatewayEnabled.Bool() {
		go p.Start()
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
		api := api.NewAPI(service, appConf.DisableCors, authService, config, p, staticFiles, licenser) // true to disable CORS for development

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
