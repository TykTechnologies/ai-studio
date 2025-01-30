package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type AppConf struct {
	SMTPServer          string
	SMTPPort            int
	SMTPUser            string
	SMTPPass            string
	FromEmail           string
	AllowRegistrations  bool
	AdminEmail          string
	SiteURL             string
	ProxyURL            string
	ServerPort          string
	CertFile            string
	KeyFile             string
	DisableCors         bool
	DatabaseURL         string
	DatabaseType        string
	FilterSignupDomains []string
	EchoConversation    bool
	ProxyOnly           bool
	DocsURL             string
}

var globalConfig *AppConf

func getConfigFromEnv() *AppConf {
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
		log.Fatalf("Unsupported DATABASE_TYPE: %s. Supported types are 'sqlite' and 'postgres'", conf.DatabaseType)
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

	conf.ProxyURL = os.Getenv("PROXY_URL")
	if conf.ProxyURL == "" {
		log.Println("Warning: PROXY_URL environment variable is not set")
	}

	return conf
}

func Get() *AppConf {
	if globalConfig == nil {
		globalConfig = getConfigFromEnv()
	}
	return globalConfig
}
