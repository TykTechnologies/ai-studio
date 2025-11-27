// internal/config/plugin_config_factory.go
package config

import (
	"context"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/rs/zerolog/log"
)

// NewPluginConfigLoader creates appropriate plugin config loader based on configuration
func NewPluginConfigLoader(cfg *Config) (PluginConfigLoader, error) {
	switch {
	case cfg.Plugins.ConfigServiceURL != "":
		// HTTP service-based loader
		log.Debug().Str("url", cfg.Plugins.ConfigServiceURL).Msg("Creating HTTP plugin config loader")
		return NewHTTPPluginConfigLoader(
			cfg.Plugins.ConfigServiceURL,
			WithAuthToken(cfg.Plugins.ConfigServiceToken),
			WithPollInterval(cfg.Plugins.ConfigPollInterval),
		), nil
		
	case cfg.Plugins.ConfigPath != "":
		// File-based loader
		log.Debug().Str("path", cfg.Plugins.ConfigPath).Msg("Creating file-based plugin config loader")
		return NewFilePluginConfigLoader(cfg.Plugins.ConfigPath), nil
		
	default:
		// No plugin configuration specified, return empty loader
		log.Debug().Msg("No plugin configuration specified - using empty loader")
		return NewEmptyPluginConfigLoader(), nil
	}
}

// EmptyPluginConfigLoader returns empty configuration (no plugins)
// Used when no plugin configuration is specified
type EmptyPluginConfigLoader struct{}

// NewEmptyPluginConfigLoader creates a loader that returns no plugins
func NewEmptyPluginConfigLoader() *EmptyPluginConfigLoader {
	return &EmptyPluginConfigLoader{}
}

// LoadDataCollectionPlugins returns an empty plugin list
func (e *EmptyPluginConfigLoader) LoadDataCollectionPlugins(ctx context.Context) ([]plugins.DataCollectionPluginConfig, error) {
	return []plugins.DataCollectionPluginConfig{}, nil
}

// Watch is a no-op for empty loader
func (e *EmptyPluginConfigLoader) Watch(ctx context.Context, callback func([]plugins.DataCollectionPluginConfig)) error {
	return nil // No-op
}

// Close is a no-op for empty loader
func (e *EmptyPluginConfigLoader) Close() error {
	return nil // No-op
}