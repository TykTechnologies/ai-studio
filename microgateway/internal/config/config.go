// internal/config/config.go
package config

import (
	"context"
	"fmt"
	"time"

	"github.com/caarlos0/env/v9"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// Config holds the complete application configuration
type Config struct {
	// Server Configuration
	Server ServerConfig

	// Database Configuration
	Database DatabaseConfig

	// Cache Configuration
	Cache CacheConfig

	// Gateway Configuration
	Gateway GatewayConfig

	// Analytics Configuration
	Analytics AnalyticsConfig

	// Security Configuration
	Security SecurityConfig

	// Observability Configuration
	Observability ObservabilityConfig

	// Plugin Configuration
	Plugins PluginConfig

	// OCI Plugin Configuration
	OCIPlugins OCIPluginConfig

	// Hub-and-Spoke Configuration
	HubSpoke HubSpokeConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            int           `env:"PORT" envDefault:"8080"`
	Host            string        `env:"HOST" envDefault:"0.0.0.0"`
	TLSEnabled      bool          `env:"TLS_ENABLED" envDefault:"false"`
	TLSCertPath     string        `env:"TLS_CERT_PATH"`
	TLSKeyPath      string        `env:"TLS_KEY_PATH"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout     time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type            string        `env:"DATABASE_TYPE" envDefault:"sqlite"` // sqlite or postgres
	DSN             string        `env:"DATABASE_DSN" envDefault:"file:./data/microgateway.db?cache=shared&mode=rwc"`
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"25"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
	AutoMigrate     bool          `env:"DB_AUTO_MIGRATE" envDefault:"true"`
	LogLevel        string        `env:"DB_LOG_LEVEL" envDefault:"warn"`
}

// CacheConfig holds caching configuration
type CacheConfig struct {
	Enabled         bool          `env:"CACHE_ENABLED" envDefault:"true"`
	MaxSize         int           `env:"CACHE_MAX_SIZE" envDefault:"1000"`
	TTL             time.Duration `env:"CACHE_TTL" envDefault:"1h"`
	CleanupInterval time.Duration `env:"CACHE_CLEANUP_INTERVAL" envDefault:"10m"`
	PersistToDB     bool          `env:"CACHE_PERSIST_TO_DB" envDefault:"false"`
}

// GatewayConfig holds gateway-specific configuration
type GatewayConfig struct {
	Timeout         time.Duration `env:"GATEWAY_TIMEOUT" envDefault:"30s"`
	MaxRequestSize  int64         `env:"GATEWAY_MAX_REQUEST_SIZE" envDefault:"10485760"`   // 10MB
	MaxResponseSize int64         `env:"GATEWAY_MAX_RESPONSE_SIZE" envDefault:"52428800"` // 50MB
	RateLimitRPM    int           `env:"GATEWAY_DEFAULT_RATE_LIMIT" envDefault:"100"`
	EnableFilters   bool          `env:"GATEWAY_ENABLE_FILTERS" envDefault:"true"`
	EnableAnalytics bool          `env:"GATEWAY_ENABLE_ANALYTICS" envDefault:"true"`
}

// HubSpokeConfig holds hub-and-spoke architecture configuration
type HubSpokeConfig struct {
	// Gateway Mode: "standalone" (default), "control", or "edge"
	Mode string `env:"GATEWAY_MODE" envDefault:"standalone"`
	
	// Control Instance Configuration (for control mode)
	GRPCPort       int           `env:"GRPC_PORT" envDefault:"9090"`
	GRPCHost       string        `env:"GRPC_HOST" envDefault:"0.0.0.0"`
	TLSEnabled     bool          `env:"GRPC_TLS_ENABLED" envDefault:"false"`
	TLSCertPath    string        `env:"GRPC_TLS_CERT_PATH"`
	TLSKeyPath     string        `env:"GRPC_TLS_KEY_PATH"`
	AuthToken      string        `env:"GRPC_AUTH_TOKEN" envDefault:""`
	
	// Edge Instance Configuration (for edge mode)
	ControlEndpoint    string        `env:"CONTROL_ENDPOINT" envDefault:""`
	EdgeID            string        `env:"EDGE_ID" envDefault:""`
	EdgeNamespace     string        `env:"EDGE_NAMESPACE" envDefault:""`
	ReconnectInterval time.Duration `env:"EDGE_RECONNECT_INTERVAL" envDefault:"5s"`
	HeartbeatInterval time.Duration `env:"EDGE_HEARTBEAT_INTERVAL" envDefault:"30s"`
	SyncTimeout       time.Duration `env:"EDGE_SYNC_TIMEOUT" envDefault:"10s"`
	
	// Authentication
	ClientToken       string        `env:"EDGE_AUTH_TOKEN" envDefault:""`
	ClientTLSEnabled  bool          `env:"EDGE_TLS_ENABLED" envDefault:"false"`
	ClientTLSCertPath string        `env:"EDGE_TLS_CERT_PATH"`
	ClientTLSKeyPath  string        `env:"EDGE_TLS_KEY_PATH"`
	ClientTLSCAPath   string        `env:"EDGE_TLS_CA_PATH"`
	SkipTLSVerify     bool          `env:"EDGE_SKIP_TLS_VERIFY" envDefault:"false"`
	
	// Token Validation Cache Configuration
	TokenCacheEnabled    bool          `env:"EDGE_TOKEN_CACHE_ENABLED" envDefault:"true"`
	TokenCacheTTL        time.Duration `env:"EDGE_TOKEN_CACHE_TTL" envDefault:"5m"`
	TokenCacheMaxSize    int           `env:"EDGE_TOKEN_CACHE_MAX_SIZE" envDefault:"1000"`
	TokenCacheCleanupInt time.Duration `env:"EDGE_TOKEN_CACHE_CLEANUP_INTERVAL" envDefault:"1m"`
}

// AnalyticsConfig holds analytics configuration
type AnalyticsConfig struct {
	Enabled             bool          `env:"ANALYTICS_ENABLED" envDefault:"true"`
	BufferSize          int           `env:"ANALYTICS_BUFFER_SIZE" envDefault:"1000"`
	FlushInterval       time.Duration `env:"ANALYTICS_FLUSH_INTERVAL" envDefault:"10s"`
	RetentionDays       int           `env:"ANALYTICS_RETENTION_DAYS" envDefault:"90"`
	EnableRealtime      bool          `env:"ANALYTICS_REALTIME" envDefault:"false"`
	
	// Detailed payload storage (disabled by default for privacy/storage)
	StoreRequestBodies  bool          `env:"ANALYTICS_STORE_REQUESTS" envDefault:"false"`
	StoreResponseBodies bool          `env:"ANALYTICS_STORE_RESPONSES" envDefault:"false"`
	MaxBodySize         int           `env:"ANALYTICS_MAX_BODY_SIZE" envDefault:"4096"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	JWTSecret         string        `env:"JWT_SECRET" envDefault:"change-me-in-production"`
	EncryptionKey     string        `env:"ENCRYPTION_KEY" envDefault:"change-me-in-production"`
	BCryptCost        int           `env:"BCRYPT_COST" envDefault:"10"`
	TokenLength       int           `env:"TOKEN_LENGTH" envDefault:"32"`
	SessionTimeout    time.Duration `env:"SESSION_TIMEOUT" envDefault:"24h"`
	EnableRateLimiting bool          `env:"ENABLE_RATE_LIMITING" envDefault:"true"`
	EnableIPWhitelist bool          `env:"ENABLE_IP_WHITELIST" envDefault:"false"`
}

// ObservabilityConfig holds logging, metrics, and monitoring configuration
type ObservabilityConfig struct {
	LogLevel        string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat       string `env:"LOG_FORMAT" envDefault:"json"` // json or text
	EnableMetrics   bool   `env:"ENABLE_METRICS" envDefault:"true"`
	MetricsPath     string `env:"METRICS_PATH" envDefault:"/metrics"`
	EnableTracing   bool   `env:"ENABLE_TRACING" envDefault:"false"`
	TracingEndpoint string `env:"TRACING_ENDPOINT"`
	EnableProfiling bool   `env:"ENABLE_PROFILING" envDefault:"false"`
}

// Load reads configuration from environment variables and .env file
func Load(envFile string) (*Config, error) {
	// Load .env file if it exists
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			// Not fatal if .env doesn't exist
			fmt.Printf("Warning: Could not load %s: %v\n", envFile, err)
		}
	}

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	// Post-process OCI plugin configuration
	cfg.postProcessOCIConfig()

	// Debug: Log OCI plugin configuration
	log.Debug().
		Str("cache_dir", cfg.OCIPlugins.CacheDir).
		Bool("require_signature", cfg.OCIPlugins.RequireSignature).
		Strs("allowed_registries", cfg.OCIPlugins.AllowedRegistries).
		Int("public_keys", len(cfg.OCIPlugins.DefaultPublicKeys)).
		Msg("OCI plugin configuration loaded")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate database configuration
	if c.Database.Type != "sqlite" && c.Database.Type != "postgres" {
		return fmt.Errorf("unsupported database type: %s", c.Database.Type)
	}

	// Validate server configuration
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// Validate TLS configuration
	if c.Server.TLSEnabled {
		if c.Server.TLSCertPath == "" || c.Server.TLSKeyPath == "" {
			return fmt.Errorf("TLS enabled but cert/key paths not provided")
		}
	}

	// Validate cache configuration
	if c.Cache.MaxSize <= 0 {
		return fmt.Errorf("cache max size must be positive: %d", c.Cache.MaxSize)
	}

	// Validate analytics configuration
	if c.Analytics.BufferSize <= 0 {
		return fmt.Errorf("analytics buffer size must be positive: %d", c.Analytics.BufferSize)
	}

	if c.Analytics.RetentionDays < 1 {
		return fmt.Errorf("analytics retention days must be at least 1: %d", c.Analytics.RetentionDays)
	}

	// Validate security configuration
	if c.Security.JWTSecret == "change-me-in-production" {
		fmt.Println("Warning: Using default JWT secret. Change this in production!")
	}

	if c.Security.EncryptionKey == "change-me-in-production" {
		fmt.Println("Warning: Using default encryption key. Change this in production!")
	}

	if len(c.Security.EncryptionKey) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 characters")
	}

	// Validate BCrypt cost
	if c.Security.BCryptCost < 4 || c.Security.BCryptCost > 31 {
		return fmt.Errorf("bcrypt cost must be between 4 and 31: %d", c.Security.BCryptCost)
	}

	// Validate observability configuration
	validLogLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true, "panic": true,
	}
	if !validLogLevels[c.Observability.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.Observability.LogLevel)
	}

	if c.Observability.LogFormat != "json" && c.Observability.LogFormat != "text" {
		return fmt.Errorf("invalid log format: %s (must be 'json' or 'text')", c.Observability.LogFormat)
	}

	// Validate hub-and-spoke configuration
	if err := c.validateHubSpokeConfig(); err != nil {
		return fmt.Errorf("hub-spoke configuration error: %w", err)
	}

	return nil
}

// GetDatabaseConfig returns database configuration for the database package
func (c *Config) GetDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Type:            c.Database.Type,
		DSN:             c.Database.DSN,
		MaxOpenConns:    c.Database.MaxOpenConns,
		MaxIdleConns:    c.Database.MaxIdleConns,
		ConnMaxLifetime: c.Database.ConnMaxLifetime,
		AutoMigrate:     c.Database.AutoMigrate,
		LogLevel:        c.Database.LogLevel,
	}
}

// IsDevelopment returns true if the application is running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Observability.LogLevel == "debug" || c.Observability.LogLevel == "trace"
}

// IsProduction returns true if the application is running in production mode
func (c *Config) IsProduction() bool {
	return !c.IsDevelopment() && c.Server.TLSEnabled
}

// LoadPluginConfig initializes and loads plugin configuration using appropriate loader
func (c *Config) LoadPluginConfig(ctx context.Context) error {
	// Create appropriate plugin config loader based on configuration
	loader, err := NewPluginConfigLoader(c)
	if err != nil {
		return fmt.Errorf("failed to create plugin config loader: %w", err)
	}
	
	c.Plugins.Loader = loader
	
	// Load initial configuration
	plugins, err := loader.LoadDataCollectionPlugins(ctx)
	if err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}
	
	c.Plugins.DataCollectionPlugins = plugins
	
	log.Info().Int("count", len(plugins)).Msg("Loaded plugin configurations")
	return nil
}

// validateHubSpokeConfig validates hub-and-spoke specific configuration
func (c *Config) validateHubSpokeConfig() error {
	// Validate gateway mode
	validModes := []string{"standalone", "control", "edge"}
	validMode := false
	for _, mode := range validModes {
		if c.HubSpoke.Mode == mode {
			validMode = true
			break
		}
	}
	if !validMode {
		return fmt.Errorf("invalid gateway mode: %s (must be one of: %v)", c.HubSpoke.Mode, validModes)
	}
	
	// Validate control mode specific settings
	if c.HubSpoke.Mode == "control" {
		if c.HubSpoke.GRPCPort < 1 || c.HubSpoke.GRPCPort > 65535 {
			return fmt.Errorf("invalid gRPC port: %d", c.HubSpoke.GRPCPort)
		}
		if c.HubSpoke.GRPCPort == c.Server.Port {
			return fmt.Errorf("gRPC port cannot be the same as HTTP server port")
		}
	}
	
	// Validate edge mode specific settings
	if c.HubSpoke.Mode == "edge" {
		if c.HubSpoke.ControlEndpoint == "" {
			return fmt.Errorf("control endpoint is required for edge mode")
		}
		if c.HubSpoke.EdgeID == "" {
			return fmt.Errorf("edge ID is required for edge mode")
		}
	}
	
	return nil
}

// IsStandalone returns true if the gateway is in standalone mode
func (c *Config) IsStandalone() bool {
	return c.HubSpoke.Mode == "standalone"
}

// IsControl returns true if the gateway is in control mode
func (c *Config) IsControl() bool {
	return c.HubSpoke.Mode == "control"
}

// IsEdge returns true if the gateway is in edge mode
func (c *Config) IsEdge() bool {
	return c.HubSpoke.Mode == "edge"
}

// postProcessOCIConfig loads OCI plugin configuration that can't be handled by env tags
func (c *Config) postProcessOCIConfig() {
	log.Debug().Msg("Post-processing OCI plugin configuration")

	// The DefaultPublicKeys and RegistryAuth will be loaded by ToOCIConfig()
	// when the OCI client is created, so we don't need to do it here

	// Just ensure the basic fields are populated correctly
	if c.OCIPlugins.CacheDir == "" {
		c.OCIPlugins.CacheDir = "/var/lib/microgateway/plugins"
	}
}