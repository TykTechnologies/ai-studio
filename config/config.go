package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type AppConf struct {
	SMTPServer            string
	SMTPPort              int
	SMTPUser              string
	SMTPPass              string
	FromEmail             string
	AllowRegistrations    bool
	AdminEmail            string
	SiteURL               string
	ProxyURL              string
	ServerPort            string
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

	devMode := os.Getenv("DEVMODE")
	if devMode == "true" || devMode == "1" {
		conf.DevMode = true
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

	// Licensing has been removed - TYK_AI_LICENSE no longer required

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
			log.Printf("AUTH_SERVER_URL not set, using SITE_URL: %s", conf.AuthServerURL)
		} else {
			conf.AuthServerURL = "http://localhost:3000"
			log.Println("Warning: AUTH_SERVER_URL and SITE_URL not set, defaulting to http://localhost:3000")
		}
	}

	conf.ProxyOAuthMetadataURL = os.Getenv("PROXY_OAUTH_METADATA_URL")
	if conf.ProxyOAuthMetadataURL == "" {
		var baseURL string
		if conf.ProxyURL != "" {
			baseURL = conf.ProxyURL
			log.Printf("PROXY_OAUTH_METADATA_URL not set, using PROXY_URL: %s", baseURL)
		} else {
			baseURL = "http://localhost:9090"
			log.Println("Warning: PROXY_OAUTH_METADATA_URL and PROXY_URL not set, defaulting to http://localhost:9090")
		}
		conf.ProxyOAuthMetadataURL = baseURL + "/.well-known/oauth-protected-resource"
	}

	// Queue configuration
	conf.QueueConfig = getQueueConfig()

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
		log.Printf("Warning: Invalid QUEUE_TYPE value: %s. Defaulting to inmemory", queueType)
	}

	// Parse buffer size
	if bufferSizeStr := os.Getenv("QUEUE_BUFFER_SIZE"); bufferSizeStr != "" {
		if bufferSize, err := strconv.Atoi(bufferSizeStr); err == nil && bufferSize > 0 {
			config.BufferSize = bufferSize
		} else {
			log.Printf("Warning: Invalid QUEUE_BUFFER_SIZE value: %s. Using default: %d", bufferSizeStr, config.BufferSize)
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
		log.Printf("Warning: Invalid NATS_STORAGE_TYPE value: %s. Using default: %s", storageType, config.StorageType)
	}

	// Retention policy
	retentionPolicy := os.Getenv("NATS_RETENTION_POLICY")
	if retentionPolicy == "limits" || retentionPolicy == "interest" || retentionPolicy == "workqueue" {
		config.RetentionPolicy = retentionPolicy
	} else if retentionPolicy != "" {
		log.Printf("Warning: Invalid NATS_RETENTION_POLICY value: %s. Using default: %s", retentionPolicy, config.RetentionPolicy)
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
			log.Printf("Warning: Invalid NATS_MAX_BYTES value: %s. Using default: %d", maxBytesStr, config.MaxBytes)
		}
	}

	// Durable consumer
	if durableStr := os.Getenv("NATS_DURABLE_CONSUMER"); durableStr != "" {
		if durable, err := strconv.ParseBool(durableStr); err == nil {
			config.DurableConsumer = durable
		} else {
			log.Printf("Warning: Invalid NATS_DURABLE_CONSUMER value: %s. Using default: %t", durableStr, config.DurableConsumer)
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
			log.Printf("Warning: Invalid NATS_MAX_DELIVER value: %s. Using default: %d", maxDeliverStr, config.MaxDeliver)
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
			log.Printf("Warning: Invalid NATS_MAX_RETRIES value: %s. Using default: %d", maxRetriesStr, config.MaxRetries)
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
			log.Printf("Warning: Invalid NATS_TLS_ENABLED value: %s. Using default: %t", tlsStr, config.TLSEnabled)
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
			log.Printf("Warning: Invalid NATS_TLS_SKIP_VERIFY value: %s. Using default: %t", skipVerifyStr, config.TLSSkipVerify)
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
			log.Printf("Warning: Invalid POSTGRES_QUEUE_MAX_RECONNECT_RETRIES value: %s. Using default: %d", maxRetriesStr, config.MaxReconnectRetries)
		}
	}

	// Notify timeout
	if notifyTimeout := os.Getenv("POSTGRES_QUEUE_NOTIFY_TIMEOUT"); notifyTimeout != "" {
		config.NotifyTimeout = notifyTimeout
	}

	return config
}

func Get() *AppConf {
	if globalConfig == nil {
		globalConfig = getConfigFromEnv()
	}
	return globalConfig
}

// ResetGlobalConfig resets the global configuration cache, forcing a reload on next Get() call
// This is primarily for testing purposes to ensure test isolation
func ResetGlobalConfig() {
	globalConfig = nil
}
