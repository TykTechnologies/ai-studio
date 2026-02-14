// Package plugin_sdk provides a unified SDK for building plugins that work in both
// AI Studio and Microgateway contexts. This SDK wraps the underlying proto definitions
// and provides a clean, simple interface for plugin developers.
package plugin_sdk

import (
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// Plugin is the base interface ALL plugins must implement.
// This provides the minimum lifecycle management required for any plugin.
type Plugin interface {
	// GetInfo returns metadata about the plugin
	GetInfo() PluginInfo

	// Initialize is called when plugin starts.
	// config contains runtime-specific configuration from the host.
	// The Context parameter provides runtime information (Studio vs Gateway)
	// and access to host services (KV storage, logging, etc.).
	Initialize(ctx Context, config map[string]string) error

	// Shutdown is called before plugin stops.
	// Plugins should clean up resources and complete in-flight operations.
	Shutdown(ctx Context) error
}

// PluginInfo provides plugin metadata returned by GetInfo()
type PluginInfo struct {
	Name        string // Plugin name (e.g., "llm-rate-limiter")
	Version     string // Semantic version (e.g., "1.0.0")
	Description string // Human-readable description
}

// BasePlugin is a convenience struct that provides default implementations
// of the Plugin interface. Plugin developers can embed this to reduce boilerplate.
type BasePlugin struct {
	info PluginInfo
}

// NewBasePlugin creates a new BasePlugin with the given metadata
func NewBasePlugin(name, version, description string) BasePlugin {
	return BasePlugin{
		info: PluginInfo{
			Name:        name,
			Version:     version,
			Description: description,
		},
	}
}

// GetInfo returns the plugin metadata
func (b *BasePlugin) GetInfo() PluginInfo {
	return b.info
}

// Initialize provides a default no-op implementation
func (b *BasePlugin) Initialize(ctx Context, config map[string]string) error {
	return nil
}

// Shutdown provides a default no-op implementation
func (b *BasePlugin) Shutdown(ctx Context) error {
	return nil
}

// Re-export commonly used proto types for convenience
// This allows plugin developers to import only plugin_sdk without importing proto directly
type (
	// Request/Response types
	PluginRequest  = pb.PluginRequest
	PluginResponse = pb.PluginResponse
	PluginContext  = pb.PluginContext
	EnrichedRequest = pb.EnrichedRequest

	// Auth types
	AuthRequest  = pb.AuthRequest
	AuthResponse = pb.AuthResponse

	// Response modification types
	HeadersRequest        = pb.HeadersRequest
	HeadersResponse       = pb.HeadersResponse
	ResponseWriteRequest  = pb.ResponseWriteRequest
	ResponseWriteResponse = pb.ResponseWriteResponse

	// Data collection types
	ProxyLogRequest     = pb.ProxyLogRequest
	AnalyticsRequest    = pb.AnalyticsRequest
	BudgetUsageRequest  = pb.BudgetUsageRequest
	DataCollectionResponse = pb.DataCollectionResponse

	// Auth context types
	App  = pb.App
	User = pb.User

	// Agent types
	AgentMessageRequest = pb.AgentMessageRequest
	AgentMessageChunk   = pb.AgentMessageChunk

	// Custom endpoint types
	EndpointRegistration  = pb.EndpointRegistration
	EndpointRequest       = pb.EndpointRequest
	EndpointResponse      = pb.EndpointResponse
	EndpointResponseChunk = pb.EndpointResponseChunk
)
