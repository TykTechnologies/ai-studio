// internal/config/http_plugin_loader.go
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
)

// HTTPPluginConfigLoader loads plugin configuration from an HTTP service
type HTTPPluginConfigLoader struct {
	endpoint     string
	client       *http.Client
	headers      map[string]string
	pollInterval time.Duration
	watchers     []func([]plugins.DataCollectionPluginConfig)
}

// HTTPLoaderOption configures HTTPPluginConfigLoader
type HTTPLoaderOption func(*HTTPPluginConfigLoader)

// WithAuthToken sets the authorization token for HTTP requests
func WithAuthToken(token string) HTTPLoaderOption {
	return func(h *HTTPPluginConfigLoader) {
		if token != "" {
			h.headers["Authorization"] = "Bearer " + token
		}
	}
}

// WithPollInterval sets the polling interval for configuration updates
func WithPollInterval(interval time.Duration) HTTPLoaderOption {
	return func(h *HTTPPluginConfigLoader) {
		if interval > 0 {
			h.pollInterval = interval
		}
	}
}

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) HTTPLoaderOption {
	return func(h *HTTPPluginConfigLoader) {
		h.client.Timeout = timeout
	}
}

// NewHTTPPluginConfigLoader creates a new HTTP-based plugin config loader
func NewHTTPPluginConfigLoader(endpoint string, options ...HTTPLoaderOption) *HTTPPluginConfigLoader {
	loader := &HTTPPluginConfigLoader{
		endpoint:     endpoint,
		client:       &http.Client{Timeout: 10 * time.Second},
		headers:      make(map[string]string),
		pollInterval: 30 * time.Second, // Default poll every 30s
		watchers:     make([]func([]plugins.DataCollectionPluginConfig), 0),
	}
	
	// Set default headers
	loader.headers["Content-Type"] = "application/json"
	loader.headers["Accept"] = "application/json"
	
	// Apply options
	for _, opt := range options {
		opt(loader)
	}
	
	return loader
}

// LoadDataCollectionPlugins fetches plugin configurations from the HTTP service
func (h *HTTPPluginConfigLoader) LoadDataCollectionPlugins(ctx context.Context) ([]plugins.DataCollectionPluginConfig, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.endpoint+"/plugins/data-collection", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Add headers (auth, content-type, etc.)
	for key, value := range h.headers {
		req.Header.Set(key, value)
	}
	
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugin config from %s: %w", h.endpoint, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin config service returned status %d", resp.StatusCode)
	}
	
	var config struct {
		Version               string                                  `json:"version"`
		DataCollectionPlugins []plugins.DataCollectionPluginConfig  `json:"data_collection_plugins"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode plugin config response: %w", err)
	}
	
	return config.DataCollectionPlugins, nil
}

// Watch polls the HTTP service for configuration changes
func (h *HTTPPluginConfigLoader) Watch(ctx context.Context, callback func([]plugins.DataCollectionPluginConfig)) error {
	h.watchers = append(h.watchers, callback)
	
	// Start polling goroutine
	go h.pollForChanges(ctx)
	
	return nil
}

// Close cleans up resources
func (h *HTTPPluginConfigLoader) Close() error {
	// Clear watchers
	h.watchers = nil
	return nil
}

// pollForChanges continuously polls the HTTP service for configuration updates
func (h *HTTPPluginConfigLoader) pollForChanges(ctx context.Context) {
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()
	
	var lastConfig []plugins.DataCollectionPluginConfig
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Fetch current configuration
			currentConfig, err := h.LoadDataCollectionPlugins(ctx)
			if err != nil {
				// Log error but continue polling
				continue
			}
			
			// Check if configuration has changed
			if h.configChanged(lastConfig, currentConfig) {
				lastConfig = currentConfig
				
				// Notify all watchers of the configuration change
				for _, watcher := range h.watchers {
					watcher(currentConfig)
				}
			}
		}
	}
}

// configChanged compares two plugin configurations to detect changes
func (h *HTTPPluginConfigLoader) configChanged(old, new []plugins.DataCollectionPluginConfig) bool {
	if len(old) != len(new) {
		return true
	}
	
	// Create maps for easier comparison
	oldMap := make(map[string]plugins.DataCollectionPluginConfig)
	newMap := make(map[string]plugins.DataCollectionPluginConfig)
	
	for _, plugin := range old {
		oldMap[plugin.Name] = plugin
	}
	
	for _, plugin := range new {
		newMap[plugin.Name] = plugin
	}
	
	// Check if any plugin configurations changed
	for name, newPlugin := range newMap {
		if oldPlugin, exists := oldMap[name]; !exists || !h.pluginConfigEqual(oldPlugin, newPlugin) {
			return true
		}
	}
	
	return false
}

// pluginConfigEqual compares two plugin configurations for equality
func (h *HTTPPluginConfigLoader) pluginConfigEqual(a, b plugins.DataCollectionPluginConfig) bool {
	return a.Name == b.Name &&
		   a.Path == b.Path &&
		   a.Enabled == b.Enabled &&
		   a.Priority == b.Priority &&
		   a.ReplaceDatabase == b.ReplaceDatabase &&
		   h.stringSliceEqual(a.HookTypes, b.HookTypes)
		   // Note: Config comparison is complex due to interface{} types
		   // For now, we'll consider configs equal. In production, you might
		   // want to implement deep comparison or use checksums
}

// stringSliceEqual compares two string slices for equality
func (h *HTTPPluginConfigLoader) stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	
	for i, v := range a {
		if b[i] != v {
			return false
		}
	}
	
	return true
}