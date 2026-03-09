package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// cfgLog is initialized early with a console writer for config loading
// This ensures consistent log formatting from the very start of the application
var cfgLog zerolog.Logger

func init() {
	// Initialize config logger with console writer before main logger is set up
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000-0700",
		NoColor:    false,
	}
	cfgLog = zerolog.New(output).With().Timestamp().Logger()
}

type AppConf struct {
	SMTPServer            string
	SMTPPort              int
	SMTPUser              string
	SMTPPass              string
	FromEmail             string
	AllowRegistrations    bool
	AdminEmail            string
	SiteURL              string
	ProxyURL             string
	ToolDisplayURL       string
	DataSourceDisplayURL string
	ServerPort           string
	ProxyPort            int
	CertFile              string
	KeyFile               string
	DisableCors           bool
	DatabaseURL           string
	DatabaseType          string
	FilterSignupDomains   []string
	EchoConversation      bool
	ProxyOnly             bool
	DocsURL               string
	DefaultSignupMode     string
	TIBEnabled            bool
	TIBAPISecret          string
	DocsLinks             DocsLinks
	DevMode               bool
	AuthServerURL         string
	ProxyOAuthMetadataURL string
	TelemetryEnabled      bool
	QueueConfig           QueueConfig
	LogLevel              string

	// Session Configuration
	SessionDuration       time.Duration

	// OCI Plugin Configuration
	OCIPlugins            OCIConfig

	// Marketplace Configuration
	MarketplaceEnabled      bool
	MarketplaceIndexURL     string
	MarketplaceSyncInterval time.Duration
	MarketplaceCacheDir     string

	// Hub-and-Spoke Configuration
	GatewayMode        string
	GRPCPort           int
	GRPCHost           string
	GRPCTLSEnabled     bool
	GRPCTLSCertPath    string
	GRPCTLSKeyPath     string
	GRPCAuthToken      string
	GRPCNextAuthToken  string

	// Licensing Configuration (Enterprise Edition)
	LicenseKey              string
	LicenseTelemetryPeriod  time.Duration
	LicenseDisableTelemetry bool
	LicenseTelemetryURL     string
	LicenseValidityPeriod   time.Duration
	LicenseTelemetryConcurrency int

	// Budget Configuration
	DefaultAppBudget *float64

	// Docs Server Configuration
	DocsPort     int
	DocsDisabled bool

	// Submission Configuration
	MaxResourcePayloadSize int // Max size in bytes for submission resource_payload JSON (default: 5MB)

	// Rate Limiting Configuration
	RateLimit RateLimitConfig
}

// RateLimitConfig holds configuration for auth endpoint rate limiting.
type RateLimitConfig struct {
	Enabled bool   // Enable rate limiting (default: true)
	Backend string // "memory" (default) or "redis"
	Redis   RateLimitRedisConfig
	Rules   RateLimitRules
}

// RateLimitRedisConfig holds Redis-specific rate limit configuration.
type RateLimitRedisConfig struct {
	URL       string // Redis URL (e.g. "redis://localhost:6379/0")
	KeyPrefix string // Key prefix for namespacing (default: "tyk:ratelimit:")
}

// RateLimitRule defines a rate limit: max requests per window.
type RateLimitRule struct {
	Limit  int
	Window time.Duration
}

// RateLimitRules holds per-endpoint rate limit configuration.
type RateLimitRules struct {
	LoginIP           RateLimitRule // Per-IP limit on /auth/login (default: 10/1m)
	LoginAccount      RateLimitRule // Per-IP+email limit on /auth/login (default: 5/1m)
	Register          RateLimitRule // Per-IP limit on /auth/register (default: 3/1m)
	ForgotPassword    RateLimitRule // Per-email limit on /auth/forgot-password (default: 2/5m)
	ResendVerify      RateLimitRule // Per-email limit on /auth/resend-verification (default: 3/5m)
	OAuthToken        RateLimitRule // Per-IP limit on /oauth/token (default: 10/1m)
}

// QueueConfig holds configuration for message queues
type QueueConfig struct {
	Type       string         `json:"type"`        // "inmemory" | "nats" | "postgres"
	BufferSize int            `json:"buffer_size"` // Local channel buffer (default: 100)
	NATS       NATSConfig     `json:"nats"`        // NATS-specific config
	PostgreSQL PostgreSQLQueueConfig `json:"postgresql"` // PostgreSQL-specific config
}

// NATSConfig holds NATS JetStream configuration
type NATSConfig struct {
	URL             string `json:"url"`
	StorageType     string `json:"storage_type"`     // "memory" | "file"
	RetentionPolicy string `json:"retention_policy"` // "limits" | "interest" | "workqueue"
	MaxAge          string `json:"max_age"`          // Duration string like "2h", "30m"
	MaxBytes        int64  `json:"max_bytes"`
	DurableConsumer bool   `json:"durable_consumer"`
	AckWait         string `json:"ack_wait"` // Duration string like "30s"
	MaxDeliver      int    `json:"max_deliver"`
	FetchTimeout    string `json:"fetch_timeout"`    // Duration string like "5s"
	RetryInterval   string `json:"retry_interval"`   // Duration string like "1s"
	MaxRetries      int    `json:"max_retries"`      // Max retries for failed operations
	
	// Authentication options
	CredentialsFile string `json:"credentials_file"` // Optional NATS credentials file
	Username        string `json:"username"`         // Optional username for basic auth
	Password        string `json:"password"`         // Optional password for basic auth
	Token           string `json:"token"`            // Optional token for token-based auth
	NKeyFile        string `json:"nkey_file"`        // Optional NKey file path
	
	// TLS options
	TLSEnabled      bool   `json:"tls_enabled"`      // Enable TLS connection
	TLSCertFile     string `json:"tls_cert_file"`    // Optional client certificate file
	TLSKeyFile      string `json:"tls_key_file"`     // Optional client key file
	TLSCAFile       string `json:"tls_ca_file"`      // Optional CA certificate file
	TLSSkipVerify   bool   `json:"tls_skip_verify"`  // Skip TLS certificate verification
}

// PostgreSQLQueueConfig holds PostgreSQL-specific queue configuration
type PostgreSQLQueueConfig struct {
	ReconnectInterval   string `json:"reconnect_interval"`   // Duration string like "2s"
	MaxReconnectRetries int    `json:"max_reconnect_retries"` // Maximum reconnection attempts (default: 10)
	NotifyTimeout       string `json:"notify_timeout"`       // Duration string like "5s"
}

type DocsLinks map[string]string

func (d DocsLinks) ReadFromFile(fileName string) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		cfgLog.Warn().Err(err).Msg("Failed to parse docs_links.json")
		return
	}

	err = json.Unmarshal(data, &d)
	if err != nil {
		cfgLog.Warn().Err(err).Msg("Could not read docs_links.json")
	}
}

var globalConfig *AppConf

func getConfigFromEnv(envFile string) *AppConf {
	conf := &AppConf{}

	// Determine which env file to load
	envFilePath := ".env" // Default
	if envFile != "" {
		envFilePath = envFile
	}

	// Try to load env file first
	if envMap, err := godotenv.Read(envFilePath); err == nil {
		cfgLog.Info().Msgf("Successfully loaded %s (environment variables will take precedence if set)", envFilePath)
		// Set environment variables from env file if they're not already set
		for key, value := range envMap {
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	} else {
		if envFile != "" {
			// User explicitly specified a file that doesn't exist - this is an error
			cfgLog.Warn().Msgf("Warning: Could not load specified environment file %s: %v", envFilePath, err)
		} else {
			// Default .env doesn't exist - this is expected in containers
			cfgLog.Info().Msg("No .env file found or error loading it - this is expected when running in containers. Will use environment variables.")
		}
	}

	conf.SMTPServer = os.Getenv("SMTP_SERVER")
	if conf.SMTPServer == "" {
		cfgLog.Warn().Msg("Warning: SMTP_SERVER environment variable is not set")
	}

	smtpPortStr := os.Getenv("SMTP_PORT")
	if smtpPortStr == "" {
		cfgLog.Warn().Msg("Warning: SMTP_PORT environment variable is not set")
	} else {
		port, err := strconv.Atoi(smtpPortStr)
		if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid SMTP_PORT value: %s", smtpPortStr)
		} else {
			conf.SMTPPort = port
		}
	}

	conf.SMTPUser = os.Getenv("SMTP_USER")
	if conf.SMTPUser == "" {
		cfgLog.Warn().Msg("Warning: SMTP_USER environment variable is not set")
	}

	conf.SMTPPass = os.Getenv("SMTP_PASS")
	if conf.SMTPPass == "" {
		cfgLog.Warn().Msg("Warning: SMTP_PASS environment variable is not set")
	}

	allowRegStr := os.Getenv("ALLOW_REGISTRATIONS")
	if allowRegStr == "" {
		cfgLog.Warn().Msg("Warning: ALLOW_REGISTRATIONS environment variable is not set")
	} else {
		allowReg, err := strconv.ParseBool(allowRegStr)
		if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid ALLOW_REGISTRATIONS value: %s", allowRegStr)
		} else {
			conf.AllowRegistrations = allowReg
		}
	}

	conf.AdminEmail = os.Getenv("ADMIN_EMAIL")
	if conf.AdminEmail != "" {
		cfgLog.Warn().Msg("Warning: ADMIN_EMAIL is deprecated")
	}

	conf.FromEmail = os.Getenv("FROM_EMAIL")
	if conf.FromEmail == "" {
		cfgLog.Warn().Msg("Warning: FROM_EMAIL environment variable is not set")
	}

	conf.SiteURL = os.Getenv("SITE_URL")
	if conf.SiteURL == "" {
		cfgLog.Warn().Msg("Warning: SITE_URL environment variable is not set")
	}

	conf.ServerPort = os.Getenv("SERVER_PORT")
	if conf.ServerPort == "" {
		cfgLog.Warn().Msg("Warning: SERVER_PORT environment variable is not set, defaulting to 8080")
		conf.ServerPort = "8080"
	}

	proxyPortStr := os.Getenv("PROXY_PORT")
	if proxyPortStr != "" {
		if port, err := strconv.Atoi(proxyPortStr); err == nil {
			conf.ProxyPort = port
		} else {
			cfgLog.Info().Msgf("Warning: Invalid PROXY_PORT value: %s. Using default: 9090", proxyPortStr)
			conf.ProxyPort = 9090
		}
	} else {
		conf.ProxyPort = 9090 // Default embedded gateway port
	}

	conf.CertFile = os.Getenv("CERT_FILE")
	conf.KeyFile = os.Getenv("KEY_FILE")
	if conf.KeyFile == "" || conf.CertFile == "" {
		cfgLog.Warn().Msg("Warning: KEY_FILE or CERT_FILE environment variable is not set, server will run in standard HTTP mode")
	}

	devMode := os.Getenv("DEVMODE")
	if devMode == "true" || devMode == "1" {
		conf.DevMode = true
		conf.DisableCors = true
	}

	conf.DatabaseURL = os.Getenv("DATABASE_URL")
	if conf.DatabaseURL == "" {
		cfgLog.Info().Msg("Warning: DATABASE_URL environment variable is not set, defaulting to SQLite")
		conf.DatabaseURL = "midsommar.db"
	}

	conf.DatabaseType = os.Getenv("DATABASE_TYPE")
	if conf.DatabaseType == "" {
		cfgLog.Info().Msg("Warning: DATABASE_TYPE environment variable is not set, defaulting to sqlite")
		conf.DatabaseType = "sqlite"
	}

	if conf.DatabaseType != "sqlite" && conf.DatabaseType != "postgres" {
		cfgLog.Info().Msgf("Warning: Unsupported DATABASE_TYPE: %s. Defaulting to sqlite", conf.DatabaseType)
		conf.DatabaseType = "sqlite"
	}

	filterDomains := os.Getenv("FILTER_SIGNUP_DOMAINS")
	if filterDomains != "" {
		conf.FilterSignupDomains = strings.Split(filterDomains, ",")
		cfgLog.Info().Msgf("Filtering signup domains to: %v", conf.FilterSignupDomains)
	}

	echoConvStr := os.Getenv("ECHO_CONVERSATION")
	if echoConvStr != "" {
		conf.EchoConversation = true
	}

	proxyOnlyStr := os.Getenv("PROXY_ONLY")
	if proxyOnlyStr == "true" || proxyOnlyStr == "1" {
		conf.ProxyOnly = true
	}

	// Docs server configuration - read port first so we can use it in default URL
	docsPortStr := os.Getenv("DOCS_PORT")
	if docsPortStr != "" {
		if port, err := strconv.Atoi(docsPortStr); err == nil {
			conf.DocsPort = port
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid DOCS_PORT value: %s. Using default: 8989", docsPortStr)
			conf.DocsPort = 8989
		}
	} else {
		conf.DocsPort = 8989
	}

	// Default DocsURL constructed from port, can be overridden for production/proxy setups
	conf.DocsURL = fmt.Sprintf("http://localhost:%d", conf.DocsPort)
	if override := os.Getenv("DOCS_URL_OVERRIDE"); override != "" {
		conf.DocsURL = override
	}

	docsDisabledStr := os.Getenv("DOCS_DISABLED")
	if docsDisabledStr == "true" || docsDisabledStr == "1" {
		conf.DocsDisabled = true
	}

	conf.DocsLinks = make(DocsLinks)
	conf.DocsLinks.ReadFromFile("config/docs_links.json")

	conf.ProxyURL = os.Getenv("PROXY_URL")
	if conf.ProxyURL == "" {
		cfgLog.Info().Msg("Warning: PROXY_URL environment variable is not set")
	}

	// Display URLs for Tools and Datasources (optional, fallback to ProxyURL in API handler)
	conf.ToolDisplayURL = os.Getenv("TOOL_DISPLAY_URL")
	conf.DataSourceDisplayURL = os.Getenv("DATASOURCE_DISPLAY_URL")

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
		cfgLog.Info().Msg("Warning: TYK_AI_SECRET_KEY environment variable is not set but TIB is enabled")
	}

	// Licensing configuration (Enterprise Edition)
	conf.LicenseKey = os.Getenv("TYK_AI_LICENSE")

	// License telemetry configuration
	conf.LicenseTelemetryURL = os.Getenv("LICENSE_TELEMETRY_URL")
	if conf.LicenseTelemetryURL == "" {
		conf.LicenseTelemetryURL = "https://telemetry.tyk.technology/api/track"
	}

	conf.LicenseTelemetryPeriod = parseDurationWithDefault("LICENSE_TELEMETRY_PERIOD", 1*time.Hour)
	conf.LicenseValidityPeriod = parseDurationWithDefault("LICENSE_VALIDITY_CHECK_PERIOD", 24*time.Hour)

	conf.LicenseDisableTelemetry = os.Getenv("LICENSE_DISABLE_TELEMETRY") == "true"

	telemetryConcurrency := os.Getenv("LICENSE_TELEMETRY_CONCURRENCY")
	if telemetryConcurrency != "" {
		if concurrency, err := strconv.Atoi(telemetryConcurrency); err == nil {
			conf.LicenseTelemetryConcurrency = concurrency
		}
	}
	if conf.LicenseTelemetryConcurrency == 0 {
		conf.LicenseTelemetryConcurrency = 20 // Default
	}

	// Telemetry configuration - enabled by default, can be disabled by setting TELEMETRY_ENABLED=false
	telemetryEnabledStr := os.Getenv("TELEMETRY_ENABLED")
	if telemetryEnabledStr == "false" || telemetryEnabledStr == "0" {
		conf.TelemetryEnabled = false
	} else {
		conf.TelemetryEnabled = true // Default to enabled
	}

	conf.AuthServerURL = os.Getenv("AUTH_SERVER_URL")
	if conf.AuthServerURL == "" {
		if conf.SiteURL != "" {
			conf.AuthServerURL = conf.SiteURL
			cfgLog.Info().Msgf("AUTH_SERVER_URL not set, using SITE_URL: %s", conf.AuthServerURL)
		} else {
			conf.AuthServerURL = "http://localhost:3000"
			cfgLog.Info().Msg("Warning: AUTH_SERVER_URL and SITE_URL not set, defaulting to http://localhost:3000")
		}
	}

	conf.ProxyOAuthMetadataURL = os.Getenv("PROXY_OAUTH_METADATA_URL")
	if conf.ProxyOAuthMetadataURL == "" {
		var baseURL string
		if conf.ProxyURL != "" {
			baseURL = conf.ProxyURL
			cfgLog.Info().Msgf("PROXY_OAUTH_METADATA_URL not set, using PROXY_URL: %s", baseURL)
		} else {
			baseURL = "http://localhost:9090"
			cfgLog.Info().Msg("Warning: PROXY_OAUTH_METADATA_URL and PROXY_URL not set, defaulting to http://localhost:9090")
		}
		conf.ProxyOAuthMetadataURL = baseURL + "/.well-known/oauth-protected-resource"
	}

	// Queue configuration
	conf.QueueConfig = getQueueConfig()

	// Hub-and-Spoke configuration
	conf.GatewayMode = os.Getenv("GATEWAY_MODE")
	if conf.GatewayMode == "" {
		conf.GatewayMode = "standalone" // Default to standalone mode
	}

	grpcPortStr := os.Getenv("GRPC_PORT")
	if grpcPortStr != "" {
		if port, err := strconv.Atoi(grpcPortStr); err == nil {
			conf.GRPCPort = port
		} else {
			cfgLog.Info().Msgf("Warning: Invalid GRPC_PORT value: %s. Using default: 50051", grpcPortStr)
			conf.GRPCPort = 50051
		}
	} else {
		conf.GRPCPort = 50051 // Default gRPC port
	}

	conf.GRPCHost = os.Getenv("GRPC_HOST")
	if conf.GRPCHost == "" {
		conf.GRPCHost = "0.0.0.0" // Default to listen on all interfaces
	}

	// gRPC TLS is enabled by default (secure by default)
	grpcTLSInsecureStr := os.Getenv("GRPC_TLS_INSECURE")
	if grpcTLSInsecureStr == "true" || grpcTLSInsecureStr == "1" {
		conf.GRPCTLSEnabled = false
		cfgLog.Info().Msg("⚠️  SECURITY WARNING: gRPC TLS is DISABLED. This should only be used for development!")
		cfgLog.Info().Msg("⚠️  To enable TLS for production, remove GRPC_TLS_INSECURE=true")
	} else {
		conf.GRPCTLSEnabled = true
		cfgLog.Info().Msg("✅ gRPC TLS enabled (secure by default)")
	}

	conf.GRPCTLSCertPath = os.Getenv("GRPC_TLS_CERT_PATH")
	conf.GRPCTLSKeyPath = os.Getenv("GRPC_TLS_KEY_PATH")
	conf.GRPCAuthToken = os.Getenv("GRPC_AUTH_TOKEN")
	conf.GRPCNextAuthToken = os.Getenv("GRPC_AUTH_TOKEN_NEXT")

	// OCI Plugin configuration
	conf.OCIPlugins = getOCIConfig()

	// Marketplace configuration
	conf.MarketplaceEnabled = true // Enabled by default
	if enabledStr := os.Getenv("MARKETPLACE_ENABLED"); enabledStr != "" {
		if enabled, err := strconv.ParseBool(enabledStr); err == nil {
			conf.MarketplaceEnabled = enabled
		}
	}

	conf.MarketplaceIndexURL = os.Getenv("MARKETPLACE_INDEX_URL")
	if conf.MarketplaceIndexURL == "" {
		conf.MarketplaceIndexURL = "https://raw.githubusercontent.com/TykTechnologies/tyk-ai-studio-plugins-ce/main/index.yaml"
	}

	conf.MarketplaceSyncInterval = 1 * time.Hour // Default: sync every hour
	if intervalStr := os.Getenv("MARKETPLACE_SYNC_INTERVAL"); intervalStr != "" {
		if interval, err := time.ParseDuration(intervalStr); err == nil {
			conf.MarketplaceSyncInterval = interval
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid MARKETPLACE_SYNC_INTERVAL value: %s. Using default: %s", intervalStr, conf.MarketplaceSyncInterval)
		}
	}

	conf.MarketplaceCacheDir = os.Getenv("MARKETPLACE_CACHE_DIR")
	if conf.MarketplaceCacheDir == "" {
		conf.MarketplaceCacheDir = "./.marketplace-cache"
	}

	// Log level configuration
	conf.LogLevel = os.Getenv("LOG_LEVEL")
	if conf.LogLevel == "" {
		conf.LogLevel = "info" // Default to info level
	}

	// Session duration configuration
	conf.SessionDuration = parseDurationWithDefault("SESSION_DURATION", 6*time.Hour)

	// Max resource payload size for submissions (default: 5MB)
	conf.MaxResourcePayloadSize = 5 * 1024 * 1024
	if maxPayloadStr := os.Getenv("MAX_RESOURCE_PAYLOAD_SIZE"); maxPayloadStr != "" {
		if maxPayload, err := strconv.Atoi(maxPayloadStr); err == nil && maxPayload > 0 {
			conf.MaxResourcePayloadSize = maxPayload
			cfgLog.Info().Msgf("Max resource payload size set to: %d bytes", maxPayload)
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid MAX_RESOURCE_PAYLOAD_SIZE value: %s, using default 5MB", maxPayloadStr)
		}
	}

	// Default app budget configuration
	if defaultBudgetStr := os.Getenv("DEFAULT_APP_BUDGET"); defaultBudgetStr != "" {
		if defaultBudget, err := strconv.ParseFloat(defaultBudgetStr, 64); err == nil && defaultBudget > 0 {
			conf.DefaultAppBudget = &defaultBudget
			cfgLog.Info().Msgf("Default app budget set to: %.2f", defaultBudget)
		} else if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid DEFAULT_APP_BUDGET value: %s", defaultBudgetStr)
		}
	}

	conf.RateLimit.Enabled = true
	if v := os.Getenv("TYK_AI_RATE_LIMIT_DISABLED"); v == "true" || v == "1" {
		conf.RateLimit.Enabled = false
	}
	conf.RateLimit.Backend = "memory"
	if v := os.Getenv("TYK_AI_RATE_LIMIT_BACKEND"); v == "redis" {
		conf.RateLimit.Backend = "redis"
	}
	if v := os.Getenv("TYK_AI_RATE_LIMIT_REDIS_URL"); v != "" {
		conf.RateLimit.Redis.URL = v
	}
	conf.RateLimit.Redis.KeyPrefix = "tyk:ratelimit:"
	if v := os.Getenv("TYK_AI_RATE_LIMIT_REDIS_PREFIX"); v != "" {
		conf.RateLimit.Redis.KeyPrefix = v
	}

	conf.RateLimit.Rules = RateLimitRules{
		LoginIP:        RateLimitRule{Limit: 10, Window: time.Minute},
		LoginAccount:   RateLimitRule{Limit: 5, Window: time.Minute},
		Register:       RateLimitRule{Limit: 3, Window: time.Minute},
		ForgotPassword: RateLimitRule{Limit: 2, Window: 5 * time.Minute},
		ResendVerify:   RateLimitRule{Limit: 3, Window: 5 * time.Minute},
		OAuthToken:     RateLimitRule{Limit: 10, Window: time.Minute},
	}
	parseRateLimitRule("TYK_AI_RATE_LIMIT_LOGIN_IP", &conf.RateLimit.Rules.LoginIP)
	parseRateLimitRule("TYK_AI_RATE_LIMIT_LOGIN_ACCOUNT", &conf.RateLimit.Rules.LoginAccount)
	parseRateLimitRule("TYK_AI_RATE_LIMIT_REGISTER", &conf.RateLimit.Rules.Register)
	parseRateLimitRule("TYK_AI_RATE_LIMIT_FORGOT_PASSWORD", &conf.RateLimit.Rules.ForgotPassword)
	parseRateLimitRule("TYK_AI_RATE_LIMIT_RESEND_VERIFY", &conf.RateLimit.Rules.ResendVerify)
	parseRateLimitRule("TYK_AI_RATE_LIMIT_OAUTH_TOKEN", &conf.RateLimit.Rules.OAuthToken)

	return conf
}

// getQueueConfig parses queue-related environment variables
func getQueueConfig() QueueConfig {
	config := QueueConfig{
		Type:       "inmemory", // Default to in-memory queue
		BufferSize: 100,        // Default buffer size
	}

	// Parse queue type
	queueType := os.Getenv("QUEUE_TYPE")
	if queueType == "nats" || queueType == "inmemory" || queueType == "postgres" {
		config.Type = queueType
	} else if queueType != "" {
		cfgLog.Info().Msgf("Warning: Invalid QUEUE_TYPE value: %s. Defaulting to inmemory", queueType)
	}

	// Parse buffer size
	if bufferSizeStr := os.Getenv("QUEUE_BUFFER_SIZE"); bufferSizeStr != "" {
		if bufferSize, err := strconv.Atoi(bufferSizeStr); err == nil && bufferSize > 0 {
			config.BufferSize = bufferSize
		} else {
			cfgLog.Info().Msgf("Warning: Invalid QUEUE_BUFFER_SIZE value: %s. Using default: %d", bufferSizeStr, config.BufferSize)
		}
	}

	// Parse NATS configuration
	config.NATS = getNATSConfig()
	
	// Parse PostgreSQL configuration
	config.PostgreSQL = getPostgreSQLQueueConfig()

	return config
}

// getNATSConfig parses NATS-specific environment variables
func getNATSConfig() NATSConfig {
	config := NATSConfig{
		URL:             "nats://localhost:4222", // Default NATS URL
		StorageType:     "file",                  // Default to persistent storage
		RetentionPolicy: "interest",              // Default to interest-based retention
		MaxAge:          "2h",                    // Default 2 hour retention
		MaxBytes:        100 * 1024 * 1024,       // Default 100MB
		DurableConsumer: true,                    // Default to durable consumers
		AckWait:         "30s",                   // Default 30 second ack wait
		MaxDeliver:      3,                       // Default max 3 delivery attempts
		FetchTimeout:    "5s",                    // Default 5 second fetch timeout
		RetryInterval:   "1s",                    // Default 1 second retry interval
		MaxRetries:      3,                       // Default max 3 retries
		TLSEnabled:      false,                   // Default TLS off
	}

	// NATS server URL
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		config.URL = natsURL
	}

	// Storage type
	if storageType := os.Getenv("NATS_STORAGE_TYPE"); storageType == "memory" || storageType == "file" {
		config.StorageType = storageType
	} else if storageType != "" {
		cfgLog.Info().Msgf("Warning: Invalid NATS_STORAGE_TYPE value: %s. Using default: %s", storageType, config.StorageType)
	}

	// Retention policy
	retentionPolicy := os.Getenv("NATS_RETENTION_POLICY")
	if retentionPolicy == "limits" || retentionPolicy == "interest" || retentionPolicy == "workqueue" {
		config.RetentionPolicy = retentionPolicy
	} else if retentionPolicy != "" {
		cfgLog.Info().Msgf("Warning: Invalid NATS_RETENTION_POLICY value: %s. Using default: %s", retentionPolicy, config.RetentionPolicy)
	}

	// Max age
	if maxAge := os.Getenv("NATS_MAX_AGE"); maxAge != "" {
		config.MaxAge = maxAge
	}

	// Max bytes
	if maxBytesStr := os.Getenv("NATS_MAX_BYTES"); maxBytesStr != "" {
		if maxBytes, err := strconv.ParseInt(maxBytesStr, 10, 64); err == nil && maxBytes > 0 {
			config.MaxBytes = maxBytes
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_MAX_BYTES value: %s. Using default: %d", maxBytesStr, config.MaxBytes)
		}
	}

	// Durable consumer
	if durableStr := os.Getenv("NATS_DURABLE_CONSUMER"); durableStr != "" {
		if durable, err := strconv.ParseBool(durableStr); err == nil {
			config.DurableConsumer = durable
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_DURABLE_CONSUMER value: %s. Using default: %t", durableStr, config.DurableConsumer)
		}
	}

	// Ack wait
	if ackWait := os.Getenv("NATS_ACK_WAIT"); ackWait != "" {
		config.AckWait = ackWait
	}

	// Max deliver
	if maxDeliverStr := os.Getenv("NATS_MAX_DELIVER"); maxDeliverStr != "" {
		if maxDeliver, err := strconv.Atoi(maxDeliverStr); err == nil && maxDeliver > 0 {
			config.MaxDeliver = maxDeliver
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_MAX_DELIVER value: %s. Using default: %d", maxDeliverStr, config.MaxDeliver)
		}
	}

	// Fetch timeout
	if fetchTimeout := os.Getenv("NATS_FETCH_TIMEOUT"); fetchTimeout != "" {
		config.FetchTimeout = fetchTimeout
	}

	// Retry interval
	if retryInterval := os.Getenv("NATS_RETRY_INTERVAL"); retryInterval != "" {
		config.RetryInterval = retryInterval
	}

	// Max retries
	if maxRetriesStr := os.Getenv("NATS_MAX_RETRIES"); maxRetriesStr != "" {
		if maxRetries, err := strconv.Atoi(maxRetriesStr); err == nil && maxRetries >= 0 {
			config.MaxRetries = maxRetries
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_MAX_RETRIES value: %s. Using default: %d", maxRetriesStr, config.MaxRetries)
		}
	}

	// Credentials file
	if credFile := os.Getenv("NATS_CREDENTIALS_FILE"); credFile != "" {
		config.CredentialsFile = credFile
	}

	// Authentication credentials
	if username := os.Getenv("NATS_USERNAME"); username != "" {
		config.Username = username
	}
	
	if password := os.Getenv("NATS_PASSWORD"); password != "" {
		config.Password = password
	}
	
	if token := os.Getenv("NATS_TOKEN"); token != "" {
		config.Token = token
	}
	
	if nkeyFile := os.Getenv("NATS_NKEY_FILE"); nkeyFile != "" {
		config.NKeyFile = nkeyFile
	}

	// TLS configuration
	if tlsStr := os.Getenv("NATS_TLS_ENABLED"); tlsStr != "" {
		if tls, err := strconv.ParseBool(tlsStr); err == nil {
			config.TLSEnabled = tls
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_TLS_ENABLED value: %s. Using default: %t", tlsStr, config.TLSEnabled)
		}
	}
	
	if certFile := os.Getenv("NATS_TLS_CERT_FILE"); certFile != "" {
		config.TLSCertFile = certFile
	}
	
	if keyFile := os.Getenv("NATS_TLS_KEY_FILE"); keyFile != "" {
		config.TLSKeyFile = keyFile
	}
	
	if caFile := os.Getenv("NATS_TLS_CA_FILE"); caFile != "" {
		config.TLSCAFile = caFile
	}
	
	if skipVerifyStr := os.Getenv("NATS_TLS_SKIP_VERIFY"); skipVerifyStr != "" {
		if skipVerify, err := strconv.ParseBool(skipVerifyStr); err == nil {
			config.TLSSkipVerify = skipVerify
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_TLS_SKIP_VERIFY value: %s. Using default: %t", skipVerifyStr, config.TLSSkipVerify)
		}
	}

	return config
}

// getPostgreSQLQueueConfig parses PostgreSQL-specific queue environment variables
func getPostgreSQLQueueConfig() PostgreSQLQueueConfig {
	config := PostgreSQLQueueConfig{
		ReconnectInterval:   "2s", // Default 2 second reconnection interval
		MaxReconnectRetries: 10,   // Default max 10 reconnection attempts
		NotifyTimeout:       "5s", // Default 5 second notify timeout
	}

	// Reconnect interval
	if reconnectInterval := os.Getenv("POSTGRES_QUEUE_RECONNECT_INTERVAL"); reconnectInterval != "" {
		config.ReconnectInterval = reconnectInterval
	}

	// Max reconnect retries
	if maxRetriesStr := os.Getenv("POSTGRES_QUEUE_MAX_RECONNECT_RETRIES"); maxRetriesStr != "" {
		if maxRetries, err := strconv.Atoi(maxRetriesStr); err == nil && maxRetries >= 0 {
			config.MaxReconnectRetries = maxRetries
		} else {
			cfgLog.Info().Msgf("Warning: Invalid POSTGRES_QUEUE_MAX_RECONNECT_RETRIES value: %s. Using default: %d", maxRetriesStr, config.MaxReconnectRetries)
		}
	}

	// Notify timeout
	if notifyTimeout := os.Getenv("POSTGRES_QUEUE_NOTIFY_TIMEOUT"); notifyTimeout != "" {
		config.NotifyTimeout = notifyTimeout
	}

	return config
}

// getOCIConfig parses OCI plugin-related environment variables
func getOCIConfig() OCIConfig {
	config := OCIConfig{}

	// Cache directory - if not set, OCI support is disabled
	config.CacheDir = os.Getenv("AI_STUDIO_OCI_CACHE_DIR")

	// Only parse other settings if OCI is enabled
	if config.CacheDir == "" {
		return config
	}

	// Max cache size
	if cacheSizeStr := os.Getenv("AI_STUDIO_OCI_MAX_CACHE_SIZE"); cacheSizeStr != "" {
		if cacheSize, err := strconv.ParseInt(cacheSizeStr, 10, 64); err == nil && cacheSize > 0 {
			config.MaxCacheSize = cacheSize
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_MAX_CACHE_SIZE value: %s. Using default: %d", cacheSizeStr, config.MaxCacheSize)
		}
	}

	// Allowed registries
	if allowedRegistries := os.Getenv("AI_STUDIO_OCI_ALLOWED_REGISTRIES"); allowedRegistries != "" {
		config.AllowedRegistries = strings.Split(allowedRegistries, ",")
		for i, registry := range config.AllowedRegistries {
			config.AllowedRegistries[i] = strings.TrimSpace(registry)
		}
	}

	// Require signature verification
	if requireSigStr := os.Getenv("AI_STUDIO_OCI_REQUIRE_SIGNATURE"); requireSigStr != "" {
		if requireSig, err := strconv.ParseBool(requireSigStr); err == nil {
			config.RequireSignature = requireSig
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_REQUIRE_SIGNATURE value: %s. Using default: %t", requireSigStr, config.RequireSignature)
		}
	}

	// Network timeout
	if timeoutStr := os.Getenv("AI_STUDIO_OCI_TIMEOUT"); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			config.Timeout = timeout
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_TIMEOUT value: %s. Using default: %s", timeoutStr, config.Timeout)
		}
	}

	// Retry attempts
	if retriesStr := os.Getenv("AI_STUDIO_OCI_RETRY_ATTEMPTS"); retriesStr != "" {
		if retries, err := strconv.Atoi(retriesStr); err == nil && retries >= 0 {
			config.RetryAttempts = retries
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_RETRY_ATTEMPTS value: %s. Using default: %d", retriesStr, config.RetryAttempts)
		}
	}

	// Garbage collection interval
	if gcIntervalStr := os.Getenv("AI_STUDIO_OCI_GC_INTERVAL"); gcIntervalStr != "" {
		if gcInterval, err := time.ParseDuration(gcIntervalStr); err == nil {
			config.GCInterval = gcInterval
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_GC_INTERVAL value: %s. Using default: %s", gcIntervalStr, config.GCInterval)
		}
	}

	// Keep versions
	if keepVersionsStr := os.Getenv("AI_STUDIO_OCI_KEEP_VERSIONS"); keepVersionsStr != "" {
		if keepVersions, err := strconv.Atoi(keepVersionsStr); err == nil && keepVersions > 0 {
			config.KeepVersions = keepVersions
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_KEEP_VERSIONS value: %s. Using default: %d", keepVersionsStr, config.KeepVersions)
		}
	}

	// Insecure registries
	if insecureRegistries := os.Getenv("AI_STUDIO_OCI_INSECURE_REGISTRIES"); insecureRegistries != "" {
		config.InsecureRegistries = strings.Split(insecureRegistries, ",")
		for i, registry := range config.InsecureRegistries {
			config.InsecureRegistries[i] = strings.TrimSpace(registry)
		}
	}

	// Apply defaults and validate
	config.SetDefaults()
	if err := config.Validate(); err != nil {
		cfgLog.Info().Msgf("Warning: Invalid OCI configuration: %v. OCI support will be disabled.", err)
		return OCIConfig{} // Return empty config to disable OCI
	}

	cfgLog.Info().Msgf("✅ AI Studio OCI configuration loaded successfully - cache dir: %s", config.CacheDir)
	return config
}

func Get(envFile string) *AppConf {
	if globalConfig == nil {
		globalConfig = getConfigFromEnv(envFile)
	}
	return globalConfig
}

// ResetGlobalConfig resets the global configuration cache, forcing a reload on next Get() call
// This is primarily for testing purposes to ensure test isolation
func ResetGlobalConfig() {
	globalConfig = nil
}

// parseRateLimitRule parses a "limit/window" env var (e.g. "10/1m", "5/30s") into a RateLimitRule.
// If the env var is empty, the rule is left unchanged. If malformed, a warning is logged.
func parseRateLimitRule(envVar string, rule *RateLimitRule) {
	value := os.Getenv(envVar)
	if value == "" {
		return
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		cfgLog.Warn().Msgf("Invalid rate limit rule for %s: %q (expected format: limit/window, e.g. 10/1m)", envVar, value)
		return
	}
	limit, err := strconv.Atoi(parts[0])
	if err != nil || limit <= 0 {
		cfgLog.Warn().Msgf("Invalid rate limit count for %s: %q", envVar, parts[0])
		return
	}
	window, err := time.ParseDuration(parts[1])
	if err != nil || window <= 0 {
		cfgLog.Warn().Msgf("Invalid rate limit window for %s: %q", envVar, parts[1])
		return
	}
	rule.Limit = limit
	rule.Window = window
}

// parseDurationWithDefault parses a duration from an environment variable with a default fallback
func parseDurationWithDefault(envVar string, defaultDuration time.Duration) time.Duration {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultDuration
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		cfgLog.Warn().Msgf("Invalid duration for %s: %s, using default %s", envVar, value, defaultDuration)
		return defaultDuration
	}

	return duration
}
