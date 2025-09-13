// internal/config/plugin_config.go
package config

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

// PluginConfigLoader defines how plugin configurations are loaded
// This interface enables different configuration sources: files, HTTP services, databases, etc.
type PluginConfigLoader interface {
	// LoadDataCollectionPlugins loads the data collection plugin configurations
	LoadDataCollectionPlugins(ctx context.Context) ([]plugins.DataCollectionPluginConfig, error)
	
	// Watch monitors for configuration changes and calls callback when changes occur
	// This enables hot reload of plugin configurations without restart
	Watch(ctx context.Context, callback func([]plugins.DataCollectionPluginConfig)) error
	
	// Close cleans up resources used by the loader
	Close() error
}

// PluginConfig holds all plugin-related configuration
type PluginConfig struct {
	// File-based configuration
	ConfigPath string `env:"PLUGINS_CONFIG_PATH" envDefault:""`

	// Service-based configuration
	ConfigServiceURL   string        `env:"PLUGINS_CONFIG_SERVICE_URL" envDefault:""`
	ConfigServiceToken string        `env:"PLUGINS_CONFIG_SERVICE_TOKEN" envDefault:""`
	ConfigPollInterval time.Duration `env:"PLUGINS_CONFIG_POLL_INTERVAL" envDefault:"30s"`

	// Runtime configuration (populated by loader)
	Loader                PluginConfigLoader                    `env:"-"`
	DataCollectionPlugins []plugins.DataCollectionPluginConfig `env:"-"`
}

// OCIPluginConfig holds OCI-specific plugin configuration for the microgateway
type OCIPluginConfig struct {
	// Cache settings
	CacheDir     string `env:"OCI_PLUGINS_CACHE_DIR" envDefault:"/var/lib/microgateway/plugins"`
	MaxCacheSize int64  `env:"OCI_PLUGINS_MAX_CACHE_SIZE" envDefault:"1073741824"` // 1GB

	// Default security settings
	DefaultPublicKeys []string `env:"OCI_PLUGINS_DEFAULT_PUBKEYS"`
	AllowedRegistries []string `env:"OCI_PLUGINS_ALLOWED_REGISTRIES"`

	// Authentication (registry name -> auth config)
	// Note: This would be better configured via environment variables or config files
	// For now, we'll support basic auth via environment variables

	// Network settings
	Timeout       time.Duration `env:"OCI_PLUGINS_TIMEOUT" envDefault:"30s"`
	RetryAttempts int          `env:"OCI_PLUGINS_RETRY_ATTEMPTS" envDefault:"3"`

	// Security settings
	RequireSignature bool `env:"OCI_PLUGINS_REQUIRE_SIGNATURE" envDefault:"true"`

	// Garbage collection
	GCInterval   time.Duration `env:"OCI_PLUGINS_GC_INTERVAL" envDefault:"24h"`
	KeepVersions int          `env:"OCI_PLUGINS_KEEP_VERSIONS" envDefault:"3"`
}

// ToOCIConfig converts microgateway OCI config to the library config format
func (c *OCIPluginConfig) ToOCIConfig() *ociplugins.OCIConfig {
	return &ociplugins.OCIConfig{
		CacheDir:          c.CacheDir,
		MaxCacheSize:      c.MaxCacheSize,
		DefaultPublicKeys: c.DefaultPublicKeys,
		AllowedRegistries: c.AllowedRegistries,
		RegistryAuth:      ociplugins.LoadRegistryAuthFromEnv(),
		Timeout:           c.Timeout,
		RetryAttempts:     c.RetryAttempts,
		RequireSignature:  c.RequireSignature,
		GCInterval:        c.GCInterval,
		KeepVersions:      c.KeepVersions,
	}
}

