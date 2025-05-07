package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type AppConf struct {
	SMTPServer              string
	SMTPPort                int
	SMTPUser                string
	SMTPPass                string
	FromEmail               string
	AllowRegistrations      bool
	AdminEmail              string
	SiteURL                 string
	ProxyURL                string
	ServerPort              string
	CertFile                string
	KeyFile                 string
	DisableCors             bool
	DatabaseURL             string
	DatabaseType            string
	FilterSignupDomains     []string
	EchoConversation        bool
	ProxyOnly               bool
	DocsURL                 string
	DefaultSignupMode       string
	TIBEnabled              bool
	TIBAPISecret            string
	DocsLinks               DocsLinks
	LicenseKey              string
	LicenseTelemetryPeriod  time.Duration
	LicenseDisableTelemetry bool
	LicenseTelemetryURL     string
}

type DocsLinks map[string]string

func (d DocsLinks) ReadFromFile(fileName string) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		log.Printf("Warning: Failed to parse docs_links.json: %v", err)
		return
	}

	err = json.Unmarshal(data, &d)
	if err != nil {
		log.Printf("Warning: Could not read docs_links.json: %v", err)
	}
}

var globalConfig *AppConf

func getConfigFromEnv() *AppConf {
	conf := &AppConf{}

	// Try to load .env file first
	if envMap, err := godotenv.Read(".env"); err == nil {
		log.Println("Successfully loaded .env file (environment variables will take precedence if set)")
		// Set environment variables from .env file if they're not already set
		for key, value := range envMap {
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	} else {
		log.Println("No .env file found or error loading it - this is expected when running in containers. Will use environment variables.")
	}

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
	if conf.AdminEmail != "" {
		log.Println("Warning: ADMIN_EMAIL is deprecated")
	}

	conf.FromEmail = os.Getenv("FROM_EMAIL")
	if conf.FromEmail == "" {
		log.Println("Warning: FROM_EMAIL environment variable is not set")
	}

	conf.SiteURL = os.Getenv("SITE_URL")
	if conf.SiteURL == "" {
		log.Println("Warning: SITE_URL environment variable is not set")
	}

	conf.ServerPort = os.Getenv("SERVER_PORT")
	if conf.ServerPort == "" {
		log.Println("Warning: SERVER_PORT environment variable is not set, defaulting to 8080")
		conf.ServerPort = "8080"
	}

	conf.CertFile = os.Getenv("CERT_FILE")
	conf.KeyFile = os.Getenv("KEY_FILE")
	if conf.KeyFile == "" || conf.CertFile == "" {
		log.Println("Warning: KEY_FILE or CERT_FILE environment variable is not set, server will run in standard HTTP mode")
	}

	if os.Getenv("DEVMODE") != "" {
		conf.DisableCors = true
	}

	conf.DatabaseURL = os.Getenv("DATABASE_URL")
	if conf.DatabaseURL == "" {
		log.Println("Warning: DATABASE_URL environment variable is not set, defaulting to SQLite")
		conf.DatabaseURL = "midsommar.db"
	}

	conf.DatabaseType = os.Getenv("DATABASE_TYPE")
	if conf.DatabaseType == "" {
		log.Println("Warning: DATABASE_TYPE environment variable is not set, defaulting to sqlite")
		conf.DatabaseType = "sqlite"
	}

	if conf.DatabaseType != "sqlite" && conf.DatabaseType != "postgres" {
		log.Printf("Warning: Unsupported DATABASE_TYPE: %s. Defaulting to sqlite", conf.DatabaseType)
		conf.DatabaseType = "sqlite"
	}

	filterDomains := os.Getenv("FILTER_SIGNUP_DOMAINS")
	if filterDomains != "" {
		conf.FilterSignupDomains = strings.Split(filterDomains, ",")
		log.Println("Filtering signup domains to:", conf.FilterSignupDomains)
	}

	echoConvStr := os.Getenv("ECHO_CONVERSATION")
	if echoConvStr != "" {
		conf.EchoConversation = true
	}

	proxyOnlyStr := os.Getenv("PROXY_ONLY")
	if proxyOnlyStr == "true" || proxyOnlyStr == "1" {
		conf.ProxyOnly = true
	}

	conf.DocsURL = os.Getenv("DOCS_URL")
	if conf.DocsURL == "" {
		conf.DocsURL = "http://localhost:8989"
	}

	conf.DocsLinks = make(DocsLinks)
	conf.DocsLinks.ReadFromFile("config/docs_links.json")

	conf.ProxyURL = os.Getenv("PROXY_URL")
	if conf.ProxyURL == "" {
		log.Println("Warning: PROXY_URL environment variable is not set")
	}

	conf.DefaultSignupMode = os.Getenv("DEFAULT_SIGNUP_MODE")
	if conf.DefaultSignupMode == "" {
		conf.DefaultSignupMode = "both"
	}

	tibEnabledStr := os.Getenv("TIB_ENABLED")
	if tibEnabledStr == "true" || tibEnabledStr == "1" {
		conf.TIBEnabled = true
	}

	conf.TIBAPISecret = os.Getenv("TYK_AI_SECRET_KEY")
	if conf.TIBAPISecret == "" && conf.TIBEnabled {
		log.Println("Warning: TYK_AI_SECRET_KEY environment variable is not set but TIB is enabled")
	}

	conf.LicenseKey = os.Getenv("TYK_AI_LICENSE")
	if conf.LicenseKey == "" {
		log.Println("Warning: TYK_AI_LICENSE environment variable is not set")
	}

	licenseDisableTelemetryStr := os.Getenv("LICENSE_DISABLE_TELEMETRY")
	if licenseDisableTelemetryStr == "true" || licenseDisableTelemetryStr == "1" {
		conf.LicenseDisableTelemetry = true
	}

	conf.LicenseTelemetryURL = os.Getenv("LICENSE_TELEMETRY_URL")

	licenseReportPeriodStr := os.Getenv("LICENSE_TELEMETRY_PERIOD")
	if licenseReportPeriodStr != "" {
		duration, err := time.ParseDuration(licenseReportPeriodStr)
		if err == nil {
			conf.LicenseTelemetryPeriod = duration
		}
	}

	return conf
}

func Get() *AppConf {
	if globalConfig == nil {
		globalConfig = getConfigFromEnv()
	}
	return globalConfig
}
