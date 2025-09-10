// internal/config/plugin_config.go
package config

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
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

