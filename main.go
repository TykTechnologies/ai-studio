package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/go-mail/mail"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type AppConf struct {
	SMTPServer         string
	SMTPPort           int
	SMTPUser           string
	SMTPPass           string
	FromEmail          string
	AllowRegistrations bool
	AdminEmail         string
	SiteURL            string
}

func GetConfigFromEnv() *AppConf {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	conf := &AppConf{}

	conf.SMTPServer = os.Getenv("SMTP_SERVER")
	if conf.SMTPServer == "" {
		log.Println("Warning: SMTP_SERVER environment variable is not set")
	}

	smtpPortStr := os.Getenv("SMTP_PORT")
	if smtpPortStr == "" {
		log.Println("Warning: SMTP_PORT environment variable is not set")
	} else {
		port, err := strconv.Atoi(smtpPortStr)
		if err != nil {
			log.Printf("Warning: Invalid SMTP_PORT value: %s", smtpPortStr)
		} else {
			conf.SMTPPort = port
		}
	}

	conf.SMTPUser = os.Getenv("SMTP_USER")
	if conf.SMTPUser == "" {
		log.Println("Warning: SMTP_USER environment variable is not set")
	}

	conf.SMTPPass = os.Getenv("SMTP_PASS")
	if conf.SMTPPass == "" {
		log.Println("Warning: SMTP_PASS environment variable is not set")
	}

	allowRegStr := os.Getenv("ALLOW_REGISTRATIONS")
	if allowRegStr == "" {
		log.Println("Warning: ALLOW_REGISTRATIONS environment variable is not set")
	} else {
		allowReg, err := strconv.ParseBool(allowRegStr)
		if err != nil {
			log.Printf("Warning: Invalid ALLOW_REGISTRATIONS value: %s", allowRegStr)
		} else {
			conf.AllowRegistrations = allowReg
		}
	}

	conf.AdminEmail = os.Getenv("ADMIN_EMAIL")
	if conf.AdminEmail == "" {
		log.Println("Warning: ADMIN_EMAIL environment variable is not set")
	}

	conf.FromEmail = os.Getenv("FROM_EMAIL")
	if conf.FromEmail == "" {
		log.Println("Warning: FROM_EMAIL environment variable is not set")
	}

	conf.SiteURL = os.Getenv("SITE_URL")
	if conf.SiteURL == "" {
		log.Println("Warning: SITE_URL environment variable is not set")
	}

	return conf
}

func main() {
	// Open a connection to the SQLite database
	// If the file doesn't exist, it will be created
	db, err := gorm.Open(sqlite.Open("midsommar.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto Migrate the schemas
	err = models.InitModels(db)
	if err != nil {
		log.Fatalf("Failed to initialize models: %v", err)
	}

	// Create a new service instance
	service := services.NewService(db)

	appConf := GetConfigFromEnv()

	config := &auth.Config{
		DB:                  db,
		Service:             service,
		CookieName:          "session",
		CookieSecure:        true,
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteStrictMode,
		ResetTokenExpiry:    time.Hour,
		FrontendURL:         appConf.SiteURL,
		RegistrationAllowed: appConf.AllowRegistrations,
		AdminEmail:          appConf.AdminEmail,
		FromEmail:           appConf.FromEmail,
		TestMode:            false,
		SMTPPort:            appConf.SMTPPort,
		SMTPHost:            appConf.SMTPServer,
		SMTPUsername:        appConf.SMTPUser,
		SMTPPassword:        appConf.SMTPPass,
	}

	mailer := mail.NewDialer(appConf.SMTPServer, appConf.SMTPPort, appConf.SMTPUser, appConf.SMTPPass)
	authService := auth.NewAuthService(config, mailer)

	// analytics
	ctx, stopRec := context.WithCancel(context.Background())
	defer stopRec()
	analytics.StartRecording(ctx, db)

	// Create a new API instance
	api := api.NewAPI(service, true, authService, config) // true to disable CORS for development

	// Run the API
	if err := api.Run(":8080"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
