package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
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
	SMTPServer           string
	SMTPPort             int
	SMTPUser             string
	SMTPPass             string
	FromEmail            string
	AllowRegistrations   bool
	// AllowSSOUserAPIKeys lets administrators issue a user API key to an
	// account provisioned through SSO. Off by default: an IdP-managed user
	// should hold no credential the IdP cannot revoke.
	AllowSSOUserAPIKeys bool
	// SSOAPIKeyLiveness bounds how long an SSO-provisioned user's API key
	// keeps working without a fresh SSO login (0 disables the check).
	SSOAPIKeyLiveness time.Duration
	AdminEmail           string
	SiteURL              string
	ProxyURL             string
	ToolDisplayURL       string
	DataSourceDisplayURL string
	ServerPort           string
	ProxyPort            int
	// UnifiedRouterPath moves the gateway's OpenRouter-style single endpoint
	// ({base}/chat/completions, {base}/completions, {base}/models) off its default
	// "/v1". UnifiedRouterDisabled removes it altogether. Both exist because the
	// gateway is embeddable in hosts that own the "/v1" path space.
	UnifiedRouterPath     string
	UnifiedRouterDisabled bool
	// GatewayServerTiming adds Server-Timing headers/trailers to embedded
	// gateway LLM responses (GATEWAY_SERVER_TIMING), for benchmarking.
	GatewayServerTiming bool
	CertFile              string
	KeyFile               string
	DisableCors           bool
	DatabaseURL           string
	DatabaseType          string
	// DatabaseSchema (DATABASE_SCHEMA), for postgres only, puts Studio's
	// tables in this schema instead of the connection's default (public).
	// studio.OpenDatabase creates it when missing and pins search_path to it
	// alone. Use it to share a database with another application: Studio's
	// table names are unprefixed and include common ones such as users,
	// roles and audit_records.
	DatabaseSchema        string
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
	SessionDuration time.Duration

	// OCI Plugin Configuration
	OCIPlugins OCIConfig

	// Audit Trail Configuration (Enterprise)
	Audit AuditConfig

	// Webhooks Configuration (Enterprise)
	Webhooks WebhooksConfig

	// Tyk Dashboard MCP integration (Enterprise)
	TykMCP TykMCPConfig

	// Marketplace Configuration
	MarketplaceEnabled      bool
	MarketplaceIndexURL     string
	MarketplaceSyncInterval time.Duration
	MarketplaceCacheDir     string

	// Hub-and-Spoke Configuration
	GatewayMode       string
	GRPCPort          int
	GRPCHost          string
	GRPCTLSEnabled    bool
	GRPCTLSCertPath   string
	GRPCTLSKeyPath    string
	GRPCAuthToken     string
	GRPCNextAuthToken string

	// Licensing Configuration (Enterprise Edition)
	LicenseKey                  string
	LicenseTelemetryPeriod      time.Duration
	LicenseDisableTelemetry     bool
	LicenseTelemetryURL         string
	LicenseValidityPeriod       time.Duration
	LicenseTelemetryConcurrency int

	// Budget Configuration
	DefaultAppBudget *float64

	// Docs Server Configuration
	DocsPort     int
	DocsDisabled bool

	// Metrics Configuration
	MetricsEnabled bool
	MetricsPath    string
	// MetricsAuthToken, when set, requires "Authorization: Bearer <token>" on the
	// metrics endpoint. When empty, the endpoint is only served if
	// MetricsAllowUnauthenticated is explicitly enabled.
	MetricsAuthToken            string
	MetricsAllowUnauthenticated bool

	// Tracing Configuration. Names match the microgateway's equivalents so a
	// single set of env vars configures either side of the hub-spoke pair.
	TracingEnabled  bool
	TracingEndpoint string

	// Submission Configuration
	MaxResourcePayloadSize int // Max size in bytes for submission resource_payload JSON (default: 5MB)

	// BasePath is the path prefix Studio's HTTP interface is served under,
	// such as "/ai-studio" (BASE_PATH); empty serves it at the root. SiteURL
	// should include it.
	BasePath string

	// SecretKey encrypts secrets at rest (TYK_AI_SECRET_KEY).
	SecretKey string
	// MicrogatewayEncryptionKey is the 32-character key edges decrypt the
	// credentials in their configuration with (MICROGATEWAY_ENCRYPTION_KEY).
	MicrogatewayEncryptionKey string
	// CSRFTrustedOrigins lists extra origins, comma-separated, the CSRF check
	// accepts in DevMode (CSRF_TRUSTED_ORIGINS).
	CSRFTrustedOrigins string
	// CSRFKey is a secret the CSRF token key is derived from (CSRF_KEY).
	// Set it so tokens survive restarts and are shared by replicas; empty
	// uses a random key per process.
	CSRFKey string
	// CSRFCookieName names the CSRF cookie (CSRF_COOKIE_NAME, default
	// _gorilla_csrf); change it when a host on the same domain uses the
	// default name too.
	CSRFCookieName string
	// ExportStoragePath is where log exports are written (EXPORT_STORAGE_PATH,
	// default ./data/exports).
	ExportStoragePath string
	// BrandingStoragePath is where uploaded logos and favicons are kept
	// (BRANDING_STORAGE_PATH, default ./data/branding).
	BrandingStoragePath string
	// DebugHTTP logs admin API request and response bodies (DEBUG_HTTP).
	DebugHTTP bool
	// ChatUIV2Enabled turns on the v2 chat UI; it is on unless
	// CHAT_UI_V2_ENABLED is explicitly false.
	ChatUIV2Enabled bool
	// ChatSessionIdleTTL is how long an idle chat session keeps its queue
	// (CHAT_SESSION_IDLE_TTL, default 10m).
	ChatSessionIdleTTL time.Duration

	// Tuning and debug settings (also read by the microgateway under the
	// same environment variable names).

	// AnalyticsBufferSize is how many analytics records wait in memory to
	// be written (ANALYTICS_BUFFER_SIZE, default 1000).
	AnalyticsBufferSize int
	// BudgetSyncInterval is how often budget usage is synced to edges
	// (BUDGET_SYNC_INTERVAL, default 30s).
	BudgetSyncInterval time.Duration
	// DebugHTTPProxy logs the embedded gateway's requests
	// (DEBUG_HTTP_PROXY).
	DebugHTTPProxy bool
	// MetricsNoLegacyNames stops emitting the pre-conventions aistudio_*
	// metrics next to the gen_ai.* ones (METRICS_LEGACY_NAMES=false). The
	// zero value keeps them, so dashboards built on them keep working.
	MetricsNoLegacyNames bool
	// CORSAllowedOrigins is the comma-separated origin list for the
	// endpoints that allow cross-origin calls (CORS_ALLOWED_ORIGINS; empty
	// allows any origin).
	CORSAllowedOrigins string
	// SkipFilterDefaults skips creating the example filters in an
	// Enterprise database (SKIP_FILTER_DEFAULTS).
	SkipFilterDefaults bool
	// FilterLimits are the Enterprise filter-script limits, by their
	// environment variable names (see FilterLimitNames). A name missing
	// here falls back to the environment. The switch that lets scripts use
	// the operating system (FILTER_SCRIPT_ALLOW_OS) is a security setting
	// and is read from the environment only.
	FilterLimits map[string]string
}

// FilterLimitNames are the tuning limits of Enterprise filter scripts and
// the helpers they call, which AppConf.FilterLimits may set.
var FilterLimitNames = []string{
	"FILTER_SCRIPT_TIMEOUT",
	"FILTER_SCRIPT_MAX_ALLOCS",
	"FILTER_HTTP_TIMEOUT",
	"FILTER_HTTP_MAX_RESPONSE_BYTES",
	"FILTER_LLM_TIMEOUT",
}

// Installed returns the configuration installed with Set (or loaded by
// Get), or nil. Unlike Get it never loads one: code shared with the
// microgateway, which has no AppConf, uses it to prefer Studio's settings
// when there are any.
func Installed() *AppConf {
	return globalConfig.Load()
}

// QueueConfig holds configuration for message queues
type QueueConfig struct {
	Type       string                `json:"type"`        // "inmemory" | "nats" | "postgres"
	BufferSize int                   `json:"buffer_size"` // Local channel buffer (default: 100)
	NATS       NATSConfig            `json:"nats"`        // NATS-specific config
	PostgreSQL PostgreSQLQueueConfig `json:"postgresql"`  // PostgreSQL-specific config
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
	FetchTimeout    string `json:"fetch_timeout"`  // Duration string like "5s"
	RetryInterval   string `json:"retry_interval"` // Duration string like "1s"
	MaxRetries      int    `json:"max_retries"`    // Max retries for failed operations

	// Authentication options
	CredentialsFile string `json:"credentials_file"` // Optional NATS credentials file
	Username        string `json:"username"`         // Optional username for basic auth
	Password        string `json:"password"`         // Optional password for basic auth
	Token           string `json:"token"`            // Optional token for token-based auth
	NKeyFile        string `json:"nkey_file"`        // Optional NKey file path

	// TLS options
	TLSEnabled    bool   `json:"tls_enabled"`     // Enable TLS connection
	TLSCertFile   string `json:"tls_cert_file"`   // Optional client certificate file
	TLSKeyFile    string `json:"tls_key_file"`    // Optional client key file
	TLSCAFile     string `json:"tls_ca_file"`     // Optional CA certificate file
	TLSSkipVerify bool   `json:"tls_skip_verify"` // Skip TLS certificate verification
}

// PostgreSQLQueueConfig holds PostgreSQL-specific queue configuration
type PostgreSQLQueueConfig struct {
	ReconnectInterval   string `json:"reconnect_interval"`    // Duration string like "2s"
	MaxReconnectRetries int    `json:"max_reconnect_retries"` // Maximum reconnection attempts (default: 10)
	NotifyTimeout       string `json:"notify_timeout"`        // Duration string like "5s"
}

type DocsLinks map[string]string

// defaultDocsLinks is config/docs_links.json, embedded so the console's
// documentation links work wherever the process runs.
//
//go:embed docs_links.json
var defaultDocsLinks []byte

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

// globalConfig holds the configuration Get returns. It is set either lazily
// by Get or explicitly by Set, which is how an embedding host supplies its own.
var globalConfig atomic.Pointer[AppConf]

// Load reads the configuration from the process environment. Variables the
// environment leaves unset are taken from envFile (".env" when empty) if it
// exists. Unlike Get, Load neither caches the result nor writes to the
// process environment.
func Load(envFile string) *AppConf {
	fileVals := readEnvFile(envFile)
	return loadFrom(true, func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fileVals[key]
	})
}

// ExportEnvFile copies the variables in envFile (".env" when empty) into the
// process environment, leaving variables that are already set untouched. The
// standalone binary calls it so packages that still read the environment
// directly see values from the file.
func ExportEnvFile(envFile string) {
	for key, value := range readEnvFile(envFile) {
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

// NormalizeBasePath returns p as a path prefix with a leading slash and no
// trailing one ("ai-studio/" becomes "/ai-studio"), or "" for the root.
func NormalizeBasePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

// PublicPath returns path (which starts with "/") as seen by a browser: under
// BasePath.
func (c *AppConf) PublicPath(path string) string {
	return c.BasePath + path
}

// Set installs conf as the configuration Get returns.
func Set(conf *AppConf) {
	globalConfig.Store(conf)
}

func readEnvFile(envFile string) map[string]string {
	envFilePath := ".env" // Default
	if envFile != "" {
		envFilePath = envFile
	}

	envMap, err := godotenv.Read(envFilePath)
	if err == nil {
		cfgLog.Info().Msgf("Successfully loaded %s (environment variables will take precedence if set)", envFilePath)
		return envMap
	}
	if envFile != "" {
		// User explicitly specified a file that doesn't exist - this is an error
		cfgLog.Warn().Msgf("Warning: Could not load specified environment file %s: %v", envFilePath, err)
	} else {
		// Default .env doesn't exist - this is expected in containers
		cfgLog.Info().Msg("No .env file found or error loading it - this is expected when running in containers. Will use environment variables.")
	}
	return nil
}

// LoadFrom builds a configuration from getenv, which maps a variable name to
// its value ("" when unset), applying the same defaults and validation as
// Load. It lets a host build the configuration from its own settings rather
// than from the process environment. Unlike Load it does not log the
// "environment variable is not set" notices, since a host supplies only the
// settings it needs; and because only the standalone binary runs the
// documentation server, the docs link is left out (DocsURL empty,
// DocsDisabled set) unless DOCS_URL_OVERRIDE names one.
func LoadFrom(getenv func(string) string) *AppConf {
	return loadFrom(false, getenv)
}

// loadFrom is LoadFrom; fromEnv is set when getenv reads the standalone
// binary's environment (Load, Get).
func loadFrom(fromEnv bool, getenv func(string) string) *AppConf {
	conf := &AppConf{}

	// notSet logs the notices about unset variables, which only mean
	// something to an operator configuring the standalone binary.
	notSet := cfgLog
	if !fromEnv {
		notSet = zerolog.Nop()
	}

	conf.SMTPServer = getenv("SMTP_SERVER")
	if conf.SMTPServer == "" {
		notSet.Warn().Msg("Warning: SMTP_SERVER environment variable is not set")
	}

	smtpPortStr := getenv("SMTP_PORT")
	if smtpPortStr == "" {
		notSet.Warn().Msg("Warning: SMTP_PORT environment variable is not set")
	} else {
		port, err := strconv.Atoi(smtpPortStr)
		if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid SMTP_PORT value: %s", smtpPortStr)
		} else {
			conf.SMTPPort = port
		}
	}

	conf.SMTPUser = getenv("SMTP_USER")
	if conf.SMTPUser == "" {
		notSet.Warn().Msg("Warning: SMTP_USER environment variable is not set")
	}

	conf.SMTPPass = getenv("SMTP_PASS")
	if conf.SMTPPass == "" {
		notSet.Warn().Msg("Warning: SMTP_PASS environment variable is not set")
	}

	allowRegStr := getenv("ALLOW_REGISTRATIONS")
	if allowRegStr == "" {
		notSet.Warn().Msg("Warning: ALLOW_REGISTRATIONS environment variable is not set")
	} else {
		allowReg, err := strconv.ParseBool(allowRegStr)
		if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid ALLOW_REGISTRATIONS value: %s", allowRegStr)
		} else {
			conf.AllowRegistrations = allowReg
		}
	}

	conf.AdminEmail = getenv("ADMIN_EMAIL")
	if conf.AdminEmail != "" {
		cfgLog.Warn().Msg("Warning: ADMIN_EMAIL is deprecated")
	}

	conf.FromEmail = getenv("FROM_EMAIL")
	if conf.FromEmail == "" {
		notSet.Warn().Msg("Warning: FROM_EMAIL environment variable is not set")
	}

	conf.SiteURL = getenv("SITE_URL")
	if conf.SiteURL == "" {
		notSet.Warn().Msg("Warning: SITE_URL environment variable is not set")
	}

	conf.BasePath = NormalizeBasePath(getenv("BASE_PATH"))
	if conf.BasePath != "" && conf.SiteURL != "" {
		if u, err := url.Parse(conf.SiteURL); err == nil && strings.TrimRight(u.Path, "/") != conf.BasePath {
			cfgLog.Warn().Msgf("Warning: SITE_URL (%s) should end with BASE_PATH (%s); links in emails are built from SITE_URL", conf.SiteURL, conf.BasePath)
		}
	}

	conf.ServerPort = getenv("SERVER_PORT")
	if conf.ServerPort == "" {
		notSet.Warn().Msg("Warning: SERVER_PORT environment variable is not set, defaulting to 8080")
		conf.ServerPort = "8080"
	}

	proxyPortStr := getenv("PROXY_PORT")
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

	conf.CertFile = getenv("CERT_FILE")
	conf.KeyFile = getenv("KEY_FILE")
	if conf.KeyFile == "" || conf.CertFile == "" {
		notSet.Warn().Msg("Warning: KEY_FILE or CERT_FILE environment variable is not set, server will run in standard HTTP mode")
	}

	devMode := getenv("DEVMODE")
	if devMode == "true" || devMode == "1" {
		conf.DevMode = true
		conf.DisableCors = true
	}

	conf.DatabaseURL = getenv("DATABASE_URL")
	if conf.DatabaseURL == "" {
		notSet.Info().Msg("Warning: DATABASE_URL environment variable is not set, defaulting to SQLite")
		conf.DatabaseURL = "midsommar.db"
	}

	conf.DatabaseType = getenv("DATABASE_TYPE")
	if conf.DatabaseType == "" {
		notSet.Info().Msg("Warning: DATABASE_TYPE environment variable is not set, defaulting to sqlite")
		conf.DatabaseType = "sqlite"
	}

	if conf.DatabaseType != "sqlite" && conf.DatabaseType != "postgres" {
		cfgLog.Info().Msgf("Warning: Unsupported DATABASE_TYPE: %s. Defaulting to sqlite", conf.DatabaseType)
		conf.DatabaseType = "sqlite"
	}

	conf.DatabaseSchema = getenv("DATABASE_SCHEMA")

	filterDomains := getenv("FILTER_SIGNUP_DOMAINS")
	if filterDomains != "" {
		conf.FilterSignupDomains = strings.Split(filterDomains, ",")
		cfgLog.Info().Msgf("Filtering signup domains to: %v", conf.FilterSignupDomains)
	}

	echoConvStr := getenv("ECHO_CONVERSATION")
	if echoConvStr != "" {
		conf.EchoConversation = true
	}

	proxyOnlyStr := getenv("PROXY_ONLY")
	if proxyOnlyStr == "true" || proxyOnlyStr == "1" {
		conf.ProxyOnly = true
	}

	conf.UnifiedRouterPath = strings.TrimSpace(getenv("UNIFIED_ROUTER_PATH"))
	unifiedDisabledStr := getenv("UNIFIED_ROUTER_DISABLED")
	if unifiedDisabledStr == "true" || unifiedDisabledStr == "1" {
		conf.UnifiedRouterDisabled = true
		cfgLog.Info().Msg("Unified router endpoint disabled; only per-route LLM endpoints are served")
	} else if conf.UnifiedRouterPath != "" {
		cfgLog.Info().Msgf("Unified router endpoint mounted at %s", conf.UnifiedRouterPath)
	}

	if v := getenv("GATEWAY_SERVER_TIMING"); v == "true" || v == "1" {
		conf.GatewayServerTiming = true
		cfgLog.Info().Msg("Gateway Server-Timing headers enabled")
	}

	// Docs server configuration - read port first so we can use it in default URL
	docsPortStr := getenv("DOCS_PORT")
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
	if !fromEnv {
		// A host: nothing serves the documentation site (the docs server
		// runs only in the standalone binary), so there is no link to it.
		conf.DocsURL = ""
		conf.DocsDisabled = true
	}
	if override := getenv("DOCS_URL_OVERRIDE"); override != "" {
		conf.DocsURL = override
		conf.DocsDisabled = false
	}

	docsDisabledStr := getenv("DOCS_DISABLED")
	if docsDisabledStr == "true" || docsDisabledStr == "1" {
		conf.DocsDisabled = true
	}

	// Embedded defaults, overridden by config/docs_links.json in the working
	// directory when a deployment provides one.
	conf.DocsLinks = make(DocsLinks)
	if err := json.Unmarshal(defaultDocsLinks, &conf.DocsLinks); err != nil {
		cfgLog.Warn().Err(err).Msg("Could not read the embedded docs_links.json")
	}
	if _, err := os.Stat("config/docs_links.json"); err == nil {
		conf.DocsLinks.ReadFromFile("config/docs_links.json")
	}

	conf.ProxyURL = getenv("PROXY_URL")
	if conf.ProxyURL == "" {
		notSet.Info().Msg("Warning: PROXY_URL environment variable is not set")
	}

	// Display URLs for Tools and Datasources (optional, fallback to ProxyURL in API handler)
	conf.ToolDisplayURL = getenv("TOOL_DISPLAY_URL")
	conf.DataSourceDisplayURL = getenv("DATASOURCE_DISPLAY_URL")

	conf.DefaultSignupMode = getenv("DEFAULT_SIGNUP_MODE")
	if conf.DefaultSignupMode == "" {
		conf.DefaultSignupMode = "both"
	}

	tibEnabledStr := getenv("TIB_ENABLED")
	if tibEnabledStr == "true" || tibEnabledStr == "1" {
		conf.TIBEnabled = true
	}

	conf.TIBAPISecret = getenv("TYK_AI_SECRET_KEY")
	if conf.TIBAPISecret == "" && conf.TIBEnabled {
		cfgLog.Info().Msg("Warning: TYK_AI_SECRET_KEY environment variable is not set but TIB is enabled")
	}

	// Licensing configuration (Enterprise Edition)
	conf.LicenseKey = getenv("TYK_AI_LICENSE")

	// License telemetry configuration
	conf.LicenseTelemetryURL = getenv("LICENSE_TELEMETRY_URL")
	if conf.LicenseTelemetryURL == "" {
		conf.LicenseTelemetryURL = "https://telemetry.tyk.technology/api/track"
	}

	conf.LicenseTelemetryPeriod = parseDurationWithDefault(getenv, "LICENSE_TELEMETRY_PERIOD", 1*time.Hour)
	conf.LicenseValidityPeriod = parseDurationWithDefault(getenv, "LICENSE_VALIDITY_CHECK_PERIOD", 24*time.Hour)

	conf.LicenseDisableTelemetry = getenv("LICENSE_DISABLE_TELEMETRY") == "true"

	telemetryConcurrency := getenv("LICENSE_TELEMETRY_CONCURRENCY")
	if telemetryConcurrency != "" {
		if concurrency, err := strconv.Atoi(telemetryConcurrency); err == nil {
			conf.LicenseTelemetryConcurrency = concurrency
		}
	}
	if conf.LicenseTelemetryConcurrency == 0 {
		conf.LicenseTelemetryConcurrency = 20 // Default
	}

	// Telemetry configuration - enabled by default, can be disabled by setting TELEMETRY_ENABLED=false
	telemetryEnabledStr := getenv("TELEMETRY_ENABLED")
	if telemetryEnabledStr == "false" || telemetryEnabledStr == "0" {
		conf.TelemetryEnabled = false
	} else {
		conf.TelemetryEnabled = true // Default to enabled
	}

	// Metrics configuration - enabled by default
	metricsEnabledStr := getenv("METRICS_ENABLED")
	if metricsEnabledStr == "false" || metricsEnabledStr == "0" {
		conf.MetricsEnabled = false
	} else {
		conf.MetricsEnabled = true
	}
	conf.MetricsPath = getenv("METRICS_PATH")
	if conf.MetricsPath == "" {
		conf.MetricsPath = "/metrics"
	}
	conf.MetricsAuthToken = getenv("METRICS_AUTH_TOKEN")
	metricsAllowUnauthStr := getenv("METRICS_ALLOW_UNAUTHENTICATED")
	conf.MetricsAllowUnauthenticated = metricsAllowUnauthStr == "true" || metricsAllowUnauthStr == "1"

	// Tracing configuration - disabled by default
	tracingEnabledStr := getenv("ENABLE_TRACING")
	conf.TracingEnabled = tracingEnabledStr == "true" || tracingEnabledStr == "1"
	conf.TracingEndpoint = getenv("TRACING_ENDPOINT")

	conf.AuthServerURL = getenv("AUTH_SERVER_URL")
	if conf.AuthServerURL == "" {
		if conf.SiteURL != "" {
			conf.AuthServerURL = conf.SiteURL
			notSet.Info().Msgf("AUTH_SERVER_URL not set, using SITE_URL: %s", conf.AuthServerURL)
		} else {
			conf.AuthServerURL = "http://localhost:3000"
			notSet.Info().Msg("Warning: AUTH_SERVER_URL and SITE_URL not set, defaulting to http://localhost:3000")
		}
	}

	conf.ProxyOAuthMetadataURL = getenv("PROXY_OAUTH_METADATA_URL")
	if conf.ProxyOAuthMetadataURL == "" {
		var baseURL string
		if conf.ProxyURL != "" {
			baseURL = conf.ProxyURL
			notSet.Info().Msgf("PROXY_OAUTH_METADATA_URL not set, using PROXY_URL: %s", baseURL)
		} else {
			baseURL = "http://localhost:9090"
			notSet.Info().Msg("Warning: PROXY_OAUTH_METADATA_URL and PROXY_URL not set, defaulting to http://localhost:9090")
		}
		conf.ProxyOAuthMetadataURL = baseURL + "/.well-known/oauth-protected-resource"
	}

	// Queue configuration
	conf.QueueConfig = getQueueConfig(getenv)

	// Hub-and-Spoke configuration
	conf.GatewayMode = getenv("GATEWAY_MODE")
	if conf.GatewayMode == "" {
		conf.GatewayMode = "standalone" // Default to standalone mode
	}

	grpcPortStr := getenv("GRPC_PORT")
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

	conf.GRPCHost = getenv("GRPC_HOST")
	if conf.GRPCHost == "" {
		conf.GRPCHost = "0.0.0.0" // Default to listen on all interfaces
	}

	// gRPC TLS is enabled by default (secure by default)
	grpcTLSInsecureStr := getenv("GRPC_TLS_INSECURE")
	if grpcTLSInsecureStr == "true" || grpcTLSInsecureStr == "1" {
		conf.GRPCTLSEnabled = false
		cfgLog.Info().Msg("⚠️  SECURITY WARNING: gRPC TLS is DISABLED. This should only be used for development!")
		cfgLog.Info().Msg("⚠️  To enable TLS for production, remove GRPC_TLS_INSECURE=true")
	} else {
		conf.GRPCTLSEnabled = true
		cfgLog.Info().Msg("✅ gRPC TLS enabled (secure by default)")
	}

	conf.GRPCTLSCertPath = getenv("GRPC_TLS_CERT_PATH")
	conf.GRPCTLSKeyPath = getenv("GRPC_TLS_KEY_PATH")
	conf.GRPCAuthToken = getenv("GRPC_AUTH_TOKEN")
	conf.GRPCNextAuthToken = getenv("GRPC_AUTH_TOKEN_NEXT")

	// OCI Plugin configuration
	conf.OCIPlugins = getOCIConfig(getenv)

	// Audit trail configuration
	conf.Audit = getAuditConfig(getenv)

	// Webhooks configuration
	conf.Webhooks = getWebhooksConfig(getenv)
	conf.TykMCP = getTykMCPConfig(getenv)

	// Marketplace configuration
	conf.MarketplaceEnabled = true // Enabled by default
	if enabledStr := getenv("MARKETPLACE_ENABLED"); enabledStr != "" {
		if enabled, err := strconv.ParseBool(enabledStr); err == nil {
			conf.MarketplaceEnabled = enabled
		}
	}

	conf.MarketplaceIndexURL = getenv("MARKETPLACE_INDEX_URL")
	if conf.MarketplaceIndexURL == "" {
		conf.MarketplaceIndexURL = "https://raw.githubusercontent.com/TykTechnologies/tyk-ai-studio-plugins-ce/main/index.yaml"
	}

	conf.MarketplaceSyncInterval = 1 * time.Hour // Default: sync every hour
	if intervalStr := getenv("MARKETPLACE_SYNC_INTERVAL"); intervalStr != "" {
		if interval, err := time.ParseDuration(intervalStr); err == nil {
			conf.MarketplaceSyncInterval = interval
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid MARKETPLACE_SYNC_INTERVAL value: %s. Using default: %s", intervalStr, conf.MarketplaceSyncInterval)
		}
	}

	conf.MarketplaceCacheDir = getenv("MARKETPLACE_CACHE_DIR")
	if conf.MarketplaceCacheDir == "" {
		conf.MarketplaceCacheDir = "./.marketplace-cache"
	}

	// Log level configuration
	conf.LogLevel = getenv("LOG_LEVEL")
	if conf.LogLevel == "" {
		conf.LogLevel = "info" // Default to info level
	}

	// Session duration configuration
	conf.SessionDuration = parseDurationWithDefault(getenv, "SESSION_DURATION", 6*time.Hour)

	// User API keys for SSO-provisioned accounts: issuance is opt-in, and
	// an issued key only works while the user keeps signing in through the
	// identity provider (Go durations, so 30 days is "720h").
	if v := getenv("ALLOW_SSO_USER_API_KEYS"); v != "" {
		allow, err := strconv.ParseBool(v)
		if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid ALLOW_SSO_USER_API_KEYS value: %s", v)
		} else {
			conf.AllowSSOUserAPIKeys = allow
		}
	}
	conf.SSOAPIKeyLiveness = parseDurationWithDefault(getenv, "SSO_API_KEY_LIVENESS", 30*24*time.Hour)

	// Max resource payload size for submissions (default: 5MB)
	conf.MaxResourcePayloadSize = 5 * 1024 * 1024
	if maxPayloadStr := getenv("MAX_RESOURCE_PAYLOAD_SIZE"); maxPayloadStr != "" {
		if maxPayload, err := strconv.Atoi(maxPayloadStr); err == nil && maxPayload > 0 {
			conf.MaxResourcePayloadSize = maxPayload
			cfgLog.Info().Msgf("Max resource payload size set to: %d bytes", maxPayload)
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid MAX_RESOURCE_PAYLOAD_SIZE value: %s, using default 5MB", maxPayloadStr)
		}
	}

	// Default app budget configuration
	if defaultBudgetStr := getenv("DEFAULT_APP_BUDGET"); defaultBudgetStr != "" {
		if defaultBudget, err := strconv.ParseFloat(defaultBudgetStr, 64); err == nil && defaultBudget > 0 {
			conf.DefaultAppBudget = &defaultBudget
			cfgLog.Info().Msgf("Default app budget set to: %.2f", defaultBudget)
		} else if err != nil {
			cfgLog.Warn().Msgf("Warning: Invalid DEFAULT_APP_BUDGET value: %s", defaultBudgetStr)
		}
	}

	conf.SecretKey = getenv("TYK_AI_SECRET_KEY")
	conf.MicrogatewayEncryptionKey = getenv("MICROGATEWAY_ENCRYPTION_KEY")
	conf.CSRFTrustedOrigins = getenv("CSRF_TRUSTED_ORIGINS")
	conf.CSRFKey = getenv("CSRF_KEY")
	conf.CSRFCookieName = getenv("CSRF_COOKIE_NAME")
	conf.ExportStoragePath = getenv("EXPORT_STORAGE_PATH")
	if conf.ExportStoragePath == "" {
		conf.ExportStoragePath = "./data/exports"
	}
	conf.BrandingStoragePath = getenv("BRANDING_STORAGE_PATH")
	if conf.BrandingStoragePath == "" {
		conf.BrandingStoragePath = "./data/branding"
	}
	conf.DebugHTTP = getenv("DEBUG_HTTP") == "true"
	switch strings.ToLower(strings.TrimSpace(getenv("CHAT_UI_V2_ENABLED"))) {
	case "0", "false", "no", "off":
		conf.ChatUIV2Enabled = false
	default:
		conf.ChatUIV2Enabled = true
	}
	conf.ChatSessionIdleTTL = 10 * time.Minute
	if v := getenv("CHAT_SESSION_IDLE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			conf.ChatSessionIdleTTL = d
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid CHAT_SESSION_IDLE_TTL value: %s, using default 10m", v)
		}
	}

	conf.AnalyticsBufferSize = 1000
	if v := getenv("ANALYTICS_BUFFER_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			conf.AnalyticsBufferSize = n
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid ANALYTICS_BUFFER_SIZE value: %s, using default 1000", v)
		}
	}
	conf.BudgetSyncInterval = 30 * time.Second
	if v := getenv("BUDGET_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			conf.BudgetSyncInterval = d
		} else {
			cfgLog.Warn().Msgf("Warning: Invalid BUDGET_SYNC_INTERVAL value: %s, using default 30s", v)
		}
	}
	conf.DebugHTTPProxy = getenv("DEBUG_HTTP_PROXY") == "true"
	conf.MetricsNoLegacyNames = getenv("METRICS_LEGACY_NAMES") == "false"
	conf.CORSAllowedOrigins = getenv("CORS_ALLOWED_ORIGINS")
	conf.SkipFilterDefaults = getenv("SKIP_FILTER_DEFAULTS") == "true"
	conf.FilterLimits = map[string]string{}
	for _, name := range FilterLimitNames {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			conf.FilterLimits[name] = v
		}
	}
	return conf
}

// getQueueConfig parses queue-related environment variables
func getQueueConfig(getenv func(string) string) QueueConfig {
	config := QueueConfig{
		Type:       "inmemory", // Default to in-memory queue
		BufferSize: 100,        // Default buffer size
	}

	// Parse queue type
	queueType := getenv("QUEUE_TYPE")
	if queueType == "nats" || queueType == "inmemory" || queueType == "postgres" {
		config.Type = queueType
	} else if queueType != "" {
		cfgLog.Info().Msgf("Warning: Invalid QUEUE_TYPE value: %s. Defaulting to inmemory", queueType)
	}

	// Parse buffer size
	if bufferSizeStr := getenv("QUEUE_BUFFER_SIZE"); bufferSizeStr != "" {
		if bufferSize, err := strconv.Atoi(bufferSizeStr); err == nil && bufferSize > 0 {
			config.BufferSize = bufferSize
		} else {
			cfgLog.Info().Msgf("Warning: Invalid QUEUE_BUFFER_SIZE value: %s. Using default: %d", bufferSizeStr, config.BufferSize)
		}
	}

	// Parse NATS configuration
	config.NATS = getNATSConfig(getenv)

	// Parse PostgreSQL configuration
	config.PostgreSQL = getPostgreSQLQueueConfig(getenv)

	return config
}

// getNATSConfig parses NATS-specific environment variables
func getNATSConfig(getenv func(string) string) NATSConfig {
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
	if natsURL := getenv("NATS_URL"); natsURL != "" {
		config.URL = natsURL
	}

	// Storage type
	if storageType := getenv("NATS_STORAGE_TYPE"); storageType == "memory" || storageType == "file" {
		config.StorageType = storageType
	} else if storageType != "" {
		cfgLog.Info().Msgf("Warning: Invalid NATS_STORAGE_TYPE value: %s. Using default: %s", storageType, config.StorageType)
	}

	// Retention policy
	retentionPolicy := getenv("NATS_RETENTION_POLICY")
	if retentionPolicy == "limits" || retentionPolicy == "interest" || retentionPolicy == "workqueue" {
		config.RetentionPolicy = retentionPolicy
	} else if retentionPolicy != "" {
		cfgLog.Info().Msgf("Warning: Invalid NATS_RETENTION_POLICY value: %s. Using default: %s", retentionPolicy, config.RetentionPolicy)
	}

	// Max age
	if maxAge := getenv("NATS_MAX_AGE"); maxAge != "" {
		config.MaxAge = maxAge
	}

	// Max bytes
	if maxBytesStr := getenv("NATS_MAX_BYTES"); maxBytesStr != "" {
		if maxBytes, err := strconv.ParseInt(maxBytesStr, 10, 64); err == nil && maxBytes > 0 {
			config.MaxBytes = maxBytes
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_MAX_BYTES value: %s. Using default: %d", maxBytesStr, config.MaxBytes)
		}
	}

	// Durable consumer
	if durableStr := getenv("NATS_DURABLE_CONSUMER"); durableStr != "" {
		if durable, err := strconv.ParseBool(durableStr); err == nil {
			config.DurableConsumer = durable
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_DURABLE_CONSUMER value: %s. Using default: %t", durableStr, config.DurableConsumer)
		}
	}

	// Ack wait
	if ackWait := getenv("NATS_ACK_WAIT"); ackWait != "" {
		config.AckWait = ackWait
	}

	// Max deliver
	if maxDeliverStr := getenv("NATS_MAX_DELIVER"); maxDeliverStr != "" {
		if maxDeliver, err := strconv.Atoi(maxDeliverStr); err == nil && maxDeliver > 0 {
			config.MaxDeliver = maxDeliver
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_MAX_DELIVER value: %s. Using default: %d", maxDeliverStr, config.MaxDeliver)
		}
	}

	// Fetch timeout
	if fetchTimeout := getenv("NATS_FETCH_TIMEOUT"); fetchTimeout != "" {
		config.FetchTimeout = fetchTimeout
	}

	// Retry interval
	if retryInterval := getenv("NATS_RETRY_INTERVAL"); retryInterval != "" {
		config.RetryInterval = retryInterval
	}

	// Max retries
	if maxRetriesStr := getenv("NATS_MAX_RETRIES"); maxRetriesStr != "" {
		if maxRetries, err := strconv.Atoi(maxRetriesStr); err == nil && maxRetries >= 0 {
			config.MaxRetries = maxRetries
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_MAX_RETRIES value: %s. Using default: %d", maxRetriesStr, config.MaxRetries)
		}
	}

	// Credentials file
	if credFile := getenv("NATS_CREDENTIALS_FILE"); credFile != "" {
		config.CredentialsFile = credFile
	}

	// Authentication credentials
	if username := getenv("NATS_USERNAME"); username != "" {
		config.Username = username
	}

	if password := getenv("NATS_PASSWORD"); password != "" {
		config.Password = password
	}

	if token := getenv("NATS_TOKEN"); token != "" {
		config.Token = token
	}

	if nkeyFile := getenv("NATS_NKEY_FILE"); nkeyFile != "" {
		config.NKeyFile = nkeyFile
	}

	// TLS configuration
	if tlsStr := getenv("NATS_TLS_ENABLED"); tlsStr != "" {
		if tls, err := strconv.ParseBool(tlsStr); err == nil {
			config.TLSEnabled = tls
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_TLS_ENABLED value: %s. Using default: %t", tlsStr, config.TLSEnabled)
		}
	}

	if certFile := getenv("NATS_TLS_CERT_FILE"); certFile != "" {
		config.TLSCertFile = certFile
	}

	if keyFile := getenv("NATS_TLS_KEY_FILE"); keyFile != "" {
		config.TLSKeyFile = keyFile
	}

	if caFile := getenv("NATS_TLS_CA_FILE"); caFile != "" {
		config.TLSCAFile = caFile
	}

	if skipVerifyStr := getenv("NATS_TLS_SKIP_VERIFY"); skipVerifyStr != "" {
		if skipVerify, err := strconv.ParseBool(skipVerifyStr); err == nil {
			config.TLSSkipVerify = skipVerify
		} else {
			cfgLog.Info().Msgf("Warning: Invalid NATS_TLS_SKIP_VERIFY value: %s. Using default: %t", skipVerifyStr, config.TLSSkipVerify)
		}
	}

	return config
}

// getPostgreSQLQueueConfig parses PostgreSQL-specific queue environment variables
func getPostgreSQLQueueConfig(getenv func(string) string) PostgreSQLQueueConfig {
	config := PostgreSQLQueueConfig{
		ReconnectInterval:   "2s", // Default 2 second reconnection interval
		MaxReconnectRetries: 10,   // Default max 10 reconnection attempts
		NotifyTimeout:       "5s", // Default 5 second notify timeout
	}

	// Reconnect interval
	if reconnectInterval := getenv("POSTGRES_QUEUE_RECONNECT_INTERVAL"); reconnectInterval != "" {
		config.ReconnectInterval = reconnectInterval
	}

	// Max reconnect retries
	if maxRetriesStr := getenv("POSTGRES_QUEUE_MAX_RECONNECT_RETRIES"); maxRetriesStr != "" {
		if maxRetries, err := strconv.Atoi(maxRetriesStr); err == nil && maxRetries >= 0 {
			config.MaxReconnectRetries = maxRetries
		} else {
			cfgLog.Info().Msgf("Warning: Invalid POSTGRES_QUEUE_MAX_RECONNECT_RETRIES value: %s. Using default: %d", maxRetriesStr, config.MaxReconnectRetries)
		}
	}

	// Notify timeout
	if notifyTimeout := getenv("POSTGRES_QUEUE_NOTIFY_TIMEOUT"); notifyTimeout != "" {
		config.NotifyTimeout = notifyTimeout
	}

	return config
}

// getOCIConfig parses OCI plugin-related environment variables
// AuditConfig controls the platform audit trail (Enterprise feature). The
// keys mirror the Tyk Dashboard audit settings (enabled, store_type, path,
// format, detailed_recording) with retention and read-recording added.
type AuditConfig struct {
	// Enabled turns recording on. Enterprise builds default to true.
	Enabled bool
	// StoreType is "db" (queryable from the API/UI), "file" (append-only log
	// file), or "both".
	StoreType string
	// FilePath is the audit log file used when StoreType is "file" or "both".
	FilePath string
	// FileFormat is "json" (one JSON object per line) or "text".
	FileFormat string
	// DetailedRecording stores redacted request and response bodies.
	DetailedRecording bool
	// RecordReads also records GET requests. Off by default because the admin
	// UI polls several read endpoints and the volume is rarely useful.
	RecordReads bool
	// RetentionDays deletes database records older than this. 0 keeps forever.
	RetentionDays int
	// MaxBodyBytes caps each stored request/response dump and diff.
	MaxBodyBytes int
	// QueueSize bounds the in-memory write queue between the HTTP path and
	// the background writer. Overflow is dropped and logged, never blocking
	// the request.
	QueueSize int
	// RedactKeys adds to the built-in list of column / JSON key fragments
	// whose values are replaced with [REDACTED] in diffs and dumps. Matching
	// is case-insensitive substring, e.g. "ssn" matches "customer_ssn".
	RedactKeys []string
	// RedactHeaders adds to the built-in list of HTTP header names masked in
	// detailed request/response dumps.
	RedactHeaders []string
}

const (
	AuditStoreDB   = "db"
	AuditStoreFile = "file"
	AuditStoreBoth = "both"
)

// StoresToDB reports whether records are written to the database.
func (c AuditConfig) StoresToDB() bool {
	return c.StoreType == AuditStoreDB || c.StoreType == AuditStoreBoth
}

// StoresToFile reports whether records are appended to the log file.
func (c AuditConfig) StoresToFile() bool {
	return c.StoreType == AuditStoreFile || c.StoreType == AuditStoreBoth
}

func getAuditConfig(getenv func(string) string) AuditConfig {
	cfg := AuditConfig{
		Enabled:           true,
		StoreType:         AuditStoreDB,
		FilePath:          "./data/audit/audit.log",
		FileFormat:        "json",
		DetailedRecording: false,
		RecordReads:       false,
		RetentionDays:     90,
		MaxBodyBytes:      64 * 1024,
		QueueSize:         4096,
	}

	if v := getenv("AUDIT_ENABLED"); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = enabled
		} else {
			cfgLog.Warn().Msgf("Invalid AUDIT_ENABLED value: %s. Using default: %t", v, cfg.Enabled)
		}
	}

	switch v := strings.ToLower(getenv("AUDIT_STORE_TYPE")); v {
	case "":
	case AuditStoreDB, AuditStoreFile, AuditStoreBoth:
		cfg.StoreType = v
	default:
		cfgLog.Warn().Msgf("Invalid AUDIT_STORE_TYPE value: %s. Using default: %s", v, cfg.StoreType)
	}

	if v := getenv("AUDIT_FILE_PATH"); v != "" {
		cfg.FilePath = v
	}

	switch v := strings.ToLower(getenv("AUDIT_FILE_FORMAT")); v {
	case "":
	case "json", "text":
		cfg.FileFormat = v
	default:
		cfgLog.Warn().Msgf("Invalid AUDIT_FILE_FORMAT value: %s. Using default: %s", v, cfg.FileFormat)
	}

	if v := getenv("AUDIT_DETAILED_RECORDING"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.DetailedRecording = b
		}
	}

	if v := getenv("AUDIT_RECORD_READS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.RecordReads = b
		}
	}

	if v := getenv("AUDIT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.RetentionDays = n
		} else {
			cfgLog.Warn().Msgf("Invalid AUDIT_RETENTION_DAYS value: %s. Using default: %d", v, cfg.RetentionDays)
		}
	}

	if v := getenv("AUDIT_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxBodyBytes = n
		}
	}

	if v := getenv("AUDIT_QUEUE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.QueueSize = n
		}
	}

	cfg.RedactKeys = splitCSVList(getenv("AUDIT_REDACT_KEYS"))
	cfg.RedactHeaders = splitCSVList(getenv("AUDIT_REDACT_HEADERS"))

	return cfg
}

// WebhooksConfig controls outbound webhook delivery (Enterprise feature):
// event-bus events are fanned out to admin-approved HTTP targets with
// retries, dead-lettering and a searchable delivery log.
type WebhooksConfig struct {
	// Enabled turns the feature on. Enterprise builds default to true.
	Enabled bool
	// WorkerEnabled runs the delivery worker on this node. Set false on nodes
	// that should only manage targets (all nodes still ingest events).
	WorkerEnabled bool
	// WorkerCount is the number of concurrent delivery workers per node.
	WorkerCount int
	// MaxAttempts is the number of HTTP attempts before a delivery is dead-lettered.
	MaxAttempts int
	// BaseBackoff and MaxBackoff bound the exponential retry schedule.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	// RequestTimeout is the per-attempt HTTP timeout (capped at 60s).
	RequestTimeout time.Duration
	// AllowInternalTargets permits targets on loopback, RFC1918, link-local
	// and other internal ranges. Off by default: webhooks are an
	// exfiltration vector and internal services must not be reachable.
	AllowInternalTargets bool
	// AllowedHosts, when non-empty, restricts targets to these hosts (exact
	// or ".suffix"). DeniedHosts always rejects, and wins over AllowedHosts.
	AllowedHosts []string
	DeniedHosts  []string
	// RetentionDays deletes succeeded/cancelled deliveries older than this.
	// DeadLetterRetentionDays does the same for dead letters. 0 keeps forever.
	RetentionDays           int
	DeadLetterRetentionDays int
	// MaxResponseSnippetBytes caps the stored response body per attempt.
	MaxResponseSnippetBytes int
	// RequireDifferentApprover rejects approval by the admin who created the target.
	RequireDifferentApprover bool
	// SecretRotationGrace is how long the previous signing secret keeps
	// producing a second signature after a rotation.
	SecretRotationGrace time.Duration
	// RedactKeys adds to the built-in list of JSON key fragments whose values
	// are replaced with [REDACTED] before an event is stored or templated.
	RedactKeys []string
	// AuditDeliveries records every succeeded delivery in the audit trail
	// (dead letters are always recorded). Off by default because of volume.
	AuditDeliveries bool
	// ShutdownDrainTimeout bounds how long Stop waits for in-flight deliveries.
	ShutdownDrainTimeout time.Duration
}

const (
	webhooksMaxRequestTimeout = 60 * time.Second
	webhooksMaxWorkerCount    = 64
	webhooksMaxAttempts       = 50
)

func getWebhooksConfig(getenv func(string) string) WebhooksConfig {
	cfg := WebhooksConfig{
		Enabled:                 true,
		WorkerEnabled:           true,
		WorkerCount:             4,
		MaxAttempts:             10,
		BaseBackoff:             5 * time.Second,
		MaxBackoff:              time.Hour,
		RequestTimeout:          10 * time.Second,
		AllowInternalTargets:    false,
		RetentionDays:           14,
		DeadLetterRetentionDays: 30,
		MaxResponseSnippetBytes: 4096,
		SecretRotationGrace:     24 * time.Hour,
		ShutdownDrainTimeout:    15 * time.Second,
	}

	boolEnv := func(key string, dst *bool) {
		if v := getenv(key); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				*dst = b
			} else {
				cfgLog.Warn().Msgf("Invalid %s value: %s. Using default: %t", key, v, *dst)
			}
		}
	}
	intEnv := func(key string, dst *int, min, max int) {
		if v := getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= min && (max <= 0 || n <= max) {
				*dst = n
			} else {
				cfgLog.Warn().Msgf("Invalid %s value: %s. Using default: %d", key, v, *dst)
			}
		}
	}

	boolEnv("WEBHOOKS_ENABLED", &cfg.Enabled)
	boolEnv("WEBHOOKS_WORKER_ENABLED", &cfg.WorkerEnabled)
	intEnv("WEBHOOKS_WORKER_COUNT", &cfg.WorkerCount, 1, webhooksMaxWorkerCount)
	intEnv("WEBHOOKS_MAX_ATTEMPTS", &cfg.MaxAttempts, 1, webhooksMaxAttempts)
	cfg.BaseBackoff = parseDurationWithDefault(getenv, "WEBHOOKS_BASE_BACKOFF", cfg.BaseBackoff)
	cfg.MaxBackoff = parseDurationWithDefault(getenv, "WEBHOOKS_MAX_BACKOFF", cfg.MaxBackoff)
	if cfg.MaxBackoff < cfg.BaseBackoff {
		cfgLog.Warn().Msgf("WEBHOOKS_MAX_BACKOFF (%s) is below WEBHOOKS_BASE_BACKOFF (%s); using the base value", cfg.MaxBackoff, cfg.BaseBackoff)
		cfg.MaxBackoff = cfg.BaseBackoff
	}
	cfg.RequestTimeout = parseDurationWithDefault(getenv, "WEBHOOKS_REQUEST_TIMEOUT", cfg.RequestTimeout)
	if cfg.RequestTimeout <= 0 || cfg.RequestTimeout > webhooksMaxRequestTimeout {
		cfgLog.Warn().Msgf("WEBHOOKS_REQUEST_TIMEOUT (%s) must be between 1s and %s; using 10s", cfg.RequestTimeout, webhooksMaxRequestTimeout)
		cfg.RequestTimeout = 10 * time.Second
	}
	boolEnv("WEBHOOKS_ALLOW_INTERNAL_TARGETS", &cfg.AllowInternalTargets)
	cfg.AllowedHosts = splitCSVList(getenv("WEBHOOKS_ALLOWED_HOSTS"))
	cfg.DeniedHosts = splitCSVList(getenv("WEBHOOKS_DENIED_HOSTS"))
	intEnv("WEBHOOKS_RETENTION_DAYS", &cfg.RetentionDays, 0, 0)
	intEnv("WEBHOOKS_DEAD_LETTER_RETENTION_DAYS", &cfg.DeadLetterRetentionDays, 0, 0)
	intEnv("WEBHOOKS_MAX_RESPONSE_SNIPPET_BYTES", &cfg.MaxResponseSnippetBytes, 0, 64*1024)
	boolEnv("WEBHOOKS_REQUIRE_DIFFERENT_APPROVER", &cfg.RequireDifferentApprover)
	cfg.SecretRotationGrace = parseDurationWithDefault(getenv, "WEBHOOKS_SECRET_ROTATION_GRACE", cfg.SecretRotationGrace)
	cfg.RedactKeys = splitCSVList(getenv("WEBHOOKS_REDACT_KEYS"))
	boolEnv("WEBHOOKS_AUDIT_DELIVERIES", &cfg.AuditDeliveries)
	cfg.ShutdownDrainTimeout = parseDurationWithDefault(getenv, "WEBHOOKS_SHUTDOWN_DRAIN_TIMEOUT", cfg.ShutdownDrainTimeout)

	return cfg
}

// splitCSVList parses a comma-separated env value into trimmed, lower-cased,
// non-empty entries.
func splitCSVList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getOCIConfig(getenv func(string) string) OCIConfig {
	config := OCIConfig{}

	// Cache directory - if not set, OCI support is disabled
	config.CacheDir = getenv("AI_STUDIO_OCI_CACHE_DIR")

	// Only parse other settings if OCI is enabled
	if config.CacheDir == "" {
		return config
	}

	// Max cache size
	if cacheSizeStr := getenv("AI_STUDIO_OCI_MAX_CACHE_SIZE"); cacheSizeStr != "" {
		if cacheSize, err := strconv.ParseInt(cacheSizeStr, 10, 64); err == nil && cacheSize > 0 {
			config.MaxCacheSize = cacheSize
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_MAX_CACHE_SIZE value: %s. Using default: %d", cacheSizeStr, config.MaxCacheSize)
		}
	}

	// Allowed registries
	if allowedRegistries := getenv("AI_STUDIO_OCI_ALLOWED_REGISTRIES"); allowedRegistries != "" {
		config.AllowedRegistries = strings.Split(allowedRegistries, ",")
		for i, registry := range config.AllowedRegistries {
			config.AllowedRegistries[i] = strings.TrimSpace(registry)
		}
	}

	// Require signature verification
	if requireSigStr := getenv("AI_STUDIO_OCI_REQUIRE_SIGNATURE"); requireSigStr != "" {
		if requireSig, err := strconv.ParseBool(requireSigStr); err == nil {
			config.RequireSignature = requireSig
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_REQUIRE_SIGNATURE value: %s. Using default: %t", requireSigStr, config.RequireSignature)
		}
	}

	// Network timeout
	if timeoutStr := getenv("AI_STUDIO_OCI_TIMEOUT"); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			config.Timeout = timeout
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_TIMEOUT value: %s. Using default: %s", timeoutStr, config.Timeout)
		}
	}

	// Retry attempts
	if retriesStr := getenv("AI_STUDIO_OCI_RETRY_ATTEMPTS"); retriesStr != "" {
		if retries, err := strconv.Atoi(retriesStr); err == nil && retries >= 0 {
			config.RetryAttempts = retries
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_RETRY_ATTEMPTS value: %s. Using default: %d", retriesStr, config.RetryAttempts)
		}
	}

	// Garbage collection interval
	if gcIntervalStr := getenv("AI_STUDIO_OCI_GC_INTERVAL"); gcIntervalStr != "" {
		if gcInterval, err := time.ParseDuration(gcIntervalStr); err == nil {
			config.GCInterval = gcInterval
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_GC_INTERVAL value: %s. Using default: %s", gcIntervalStr, config.GCInterval)
		}
	}

	// Keep versions
	if keepVersionsStr := getenv("AI_STUDIO_OCI_KEEP_VERSIONS"); keepVersionsStr != "" {
		if keepVersions, err := strconv.Atoi(keepVersionsStr); err == nil && keepVersions > 0 {
			config.KeepVersions = keepVersions
		} else {
			cfgLog.Info().Msgf("Warning: Invalid AI_STUDIO_OCI_KEEP_VERSIONS value: %s. Using default: %d", keepVersionsStr, config.KeepVersions)
		}
	}

	// Insecure registries
	if insecureRegistries := getenv("AI_STUDIO_OCI_INSECURE_REGISTRIES"); insecureRegistries != "" {
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
	if conf := globalConfig.Load(); conf != nil {
		return conf
	}
	ExportEnvFile(envFile)
	globalConfig.CompareAndSwap(nil, loadFrom(true, os.Getenv))
	return globalConfig.Load()
}

// ResetGlobalConfig resets the global configuration cache, forcing a reload on next Get() call
// This is primarily for testing purposes to ensure test isolation
func ResetGlobalConfig() {
	globalConfig.Store(nil)
}

// parseDurationWithDefault parses a duration from an environment variable with a default fallback
func parseDurationWithDefault(getenv func(string) string, envVar string, defaultDuration time.Duration) time.Duration {
	value := getenv(envVar)
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
