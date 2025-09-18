// plugins/config.go
package plugins

// DataCollectionPluginConfig represents a single data collection plugin configuration
type DataCollectionPluginConfig struct {
	// Name is a unique identifier for this plugin instance
	Name string `json:"name" yaml:"name"`
	
	// Path to the plugin binary or shared library
	Path string `json:"path" yaml:"path"`
	
	// Enabled controls whether this plugin is active
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// HookTypes specifies which data types this plugin should handle
	// Valid values: "proxy_log", "analytics", "budget"
	HookTypes []string `json:"hook_types" yaml:"hook_types"`
	
	// Config contains plugin-specific configuration parameters
	Config map[string]interface{} `json:"config" yaml:"config"`
	
	// Priority determines execution order (higher priority runs first)
	Priority int `json:"priority" yaml:"priority"`
	
	// ReplaceDatabase indicates if plugin should replace database storage
	// If false, plugin supplements database storage
	ReplaceDatabase bool `json:"replace_database" yaml:"replace_database"`
}