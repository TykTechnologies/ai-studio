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

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/docs"
	"github.com/TykTechnologies/midsommar/v2/licensing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/go-mail/mail"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed ui/admin-frontend/build
var staticFiles embed.FS

func printWelcome() {
	fmt.Printf("Starting Tyk AI Portal %v\n", VERSION)
	fmt.Println("Copyright Tyk Technologies, 2024")
}

func main() {
	printWelcome()
	appConf := config.Get()

	err := licensing.IsLicensed()
	if err != nil {
		log.Fatalf("License is not valid: %v", err)
	}

	// start ongoing check
	go licensing.LicenseService()

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

	// Create a new service instance
	service := services.NewService(db)

	config := &auth.Config{
		DB:                     db,
		Service:                service,
		CookieName:             "session",
		AdminAPIKey:            appConf.AdminAPIKey,
		CookieSecure:           true,
		CookieHTTPOnly:         true,
		CookieSameSite:         http.SameSiteStrictMode,
		ResetTokenExpiry:       time.Hour,
		FrontendURL:            appConf.SiteURL,
		RegistrationAllowed:    appConf.AllowRegistrations,
		AdminEmail:             appConf.AdminEmail,
		FromEmail:              appConf.FromEmail,
		TestMode:               false,
		SMTPPort:               appConf.SMTPPort,
		SMTPHost:               appConf.SMTPServer,
		SMTPUsername:           appConf.SMTPUser,
		SMTPPassword:           appConf.SMTPPass,
		AllowedRegisterDomains: appConf.FilterSignupDomains,
	}

	mailer := mail.NewDialer(appConf.SMTPServer, appConf.SMTPPort, appConf.SMTPUser, appConf.SMTPPass)
	authService := auth.NewAuthService(config, mailer, service)

	// analytics
	ctx, stopRec := context.WithCancel(context.Background())
	defer stopRec()
	analytics.StartRecording(ctx, db)

	// start the Proxy

	pConfig := &proxy.Config{
		Port: 9090,
	}
	p := proxy.NewProxy(service, pConfig)

	gatewayEnabled, gatewayOk := licensing.Entitlement(licensing.FEATUREGateway)
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

	if appConf.ProxyOnly == false {
		// Create a new API instance
		api := api.NewAPI(service, appConf.DisableCors, authService, config, p, staticFiles) // true to disable CORS for development

		//listEmbeddedFiles(staticFiles)
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
