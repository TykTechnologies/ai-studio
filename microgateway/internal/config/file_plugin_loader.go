// internal/config/file_plugin_loader.go
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"gopkg.in/yaml.v3"
)

// FilePluginConfigLoader loads plugin configuration from local files
type FilePluginConfigLoader struct {
	configPath   string
	lastModified time.Time
	watchers     []func([]plugins.DataCollectionPluginConfig)
}

// NewFilePluginConfigLoader creates a new file-based plugin config loader
func NewFilePluginConfigLoader(configPath string) *FilePluginConfigLoader {
	return &FilePluginConfigLoader{
		configPath: configPath,
		watchers:   make([]func([]plugins.DataCollectionPluginConfig), 0),
	}
}

// LoadDataCollectionPlugins loads plugin configurations from the specified file
func (f *FilePluginConfigLoader) LoadDataCollectionPlugins(ctx context.Context) ([]plugins.DataCollectionPluginConfig, error) {
	if f.configPath == "" {
		return []plugins.DataCollectionPluginConfig{}, nil // No config file specified
	}
	
	data, err := os.ReadFile(f.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []plugins.DataCollectionPluginConfig{}, nil // File doesn't exist, return empty
		}
		return nil, fmt.Errorf("failed to read plugin config file %s: %w", f.configPath, err)
	}
	
	var config struct {
		Version               string                                  `yaml:"version" json:"version"`
		DataCollectionPlugins []plugins.DataCollectionPluginConfig  `yaml:"data_collection_plugins" json:"data_collection_plugins"`
	}
	
	// Support both YAML and JSON based on file extension
	if strings.HasSuffix(strings.ToLower(f.configPath), ".json") {
		err = json.Unmarshal(data, &config)
	} else {
		// Default to YAML for .yaml, .yml, or no extension
		err = yaml.Unmarshal(data, &config)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to parse plugin config: %w", err)
	}
	
	// Expand environment variables in configurations
	for i := range config.DataCollectionPlugins {
		if err := f.expandEnvVars(&config.DataCollectionPlugins[i]); err != nil {
			return nil, fmt.Errorf("failed to expand env vars for plugin %s: %w", 
				config.DataCollectionPlugins[i].Name, err)
		}
	}
	
	return config.DataCollectionPlugins, nil
}

// Watch monitors the config file for changes and calls callback on updates
func (f *FilePluginConfigLoader) Watch(ctx context.Context, callback func([]plugins.DataCollectionPluginConfig)) error {
	if f.configPath == "" {
		return nil // No file to watch
	}
	
	f.watchers = append(f.watchers, callback)
	
	// Get initial file modification time
	if info, err := os.Stat(f.configPath); err == nil {
		f.lastModified = info.ModTime()
	}
	
	// Start file watcher goroutine
	go f.watchFile(ctx)
	
	return nil
}

// Close cleans up resources
func (f *FilePluginConfigLoader) Close() error {
	// Clear watchers
	f.watchers = nil
	return nil
}

// watchFile monitors the config file for changes
func (f *FilePluginConfigLoader) watchFile(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if info, err := os.Stat(f.configPath); err == nil {
				if info.ModTime().After(f.lastModified) {
					f.lastModified = info.ModTime()
					
					// File was modified, reload configuration
					if plugins, err := f.LoadDataCollectionPlugins(ctx); err == nil {
						// Notify all watchers of the configuration change
						for _, watcher := range f.watchers {
							watcher(plugins)
						}
					}
				}
			}
		}
	}
}

// expandEnvVars expands environment variables in plugin configuration
func (f *FilePluginConfigLoader) expandEnvVars(plugin *plugins.DataCollectionPluginConfig) error {
	// Expand environment variables in Path
	plugin.Path = os.ExpandEnv(plugin.Path)
	
	// Recursively expand environment variables in Config map
	f.expandEnvVarsInMap(plugin.Config)
	
	return nil
}

// expandEnvVarsInMap recursively expands environment variables in a map
func (f *FilePluginConfigLoader) expandEnvVarsInMap(m map[string]interface{}) {
	for key, value := range m {
		switch v := value.(type) {
		case string:
			// Expand environment variables in string values
			m[key] = os.ExpandEnv(v)
		case map[string]interface{}:
			// Recursively expand in nested maps
			f.expandEnvVarsInMap(v)
		case []interface{}:
			// Handle arrays
			for i, item := range v {
				if strVal, ok := item.(string); ok {
					v[i] = os.ExpandEnv(strVal)
				} else if mapVal, ok := item.(map[string]interface{}); ok {
					f.expandEnvVarsInMap(mapVal)
				}
			}
		}
	}
}