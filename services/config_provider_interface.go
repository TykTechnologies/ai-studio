package services

import (
	"context"
)

// ConfigProvider is a minimal interface for loading plugins ONLY to get their configuration schema.
// This allows AI Studio to load plugins with minimal resource usage for schema extraction only.
// The interface is kept minimal to reduce the plugin loading footprint.
type ConfigProvider interface {
	// GetConfigSchema returns the JSON Schema for the plugin's configuration
	// The returned schema should be valid JSON Schema (jsonschema.org format)
	// Returns (schemaJSON, error) where schemaJSON is the schema as JSON string
	GetConfigSchema(ctx context.Context) ([]byte, error)
}

// ConfigProviderLoader manages loading plugins as ConfigProviders
type ConfigProviderLoader interface {
	// LoadPluginForConfigOnly loads a plugin with minimal resources for schema extraction
	// command: the plugin command (e.g., "./my-plugin", "oci://registry/plugin:v1.0.0")
	// Returns a ConfigProvider interface that can be used to get the schema
	LoadPluginForConfigOnly(ctx context.Context, command string) (ConfigProvider, error)

	// UnloadConfigProvider releases resources used by a ConfigProvider
	// This should be called after schema extraction is complete
	UnloadConfigProvider(provider ConfigProvider) error
}

// ConfigSchemaService provides high-level configuration schema operations
type ConfigSchemaService interface {
	// GetPluginConfigSchema retrieves the schema for a plugin, using cache when possible
	GetPluginConfigSchema(ctx context.Context, pluginID uint) (string, error)

	// GetPluginConfigSchemaByCommand retrieves schema by command, using cache when possible
	GetPluginConfigSchemaByCommand(ctx context.Context, command string) (string, error)

	// RefreshPluginConfigSchema forces a refresh of the schema from the plugin
	RefreshPluginConfigSchema(ctx context.Context, pluginID uint) (string, error)

	// ValidatePluginConfig validates a config map against the plugin's schema
	ValidatePluginConfig(ctx context.Context, pluginID uint, config map[string]interface{}) error

	// InvalidateSchemaCache removes a schema from the cache
	InvalidateSchemaCache(command string) error
}