package api

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// EndpointRouteInfo is a minimal representation of an endpoint route
// used in the API layer to avoid importing the plugins package directly.
type EndpointRouteInfo struct {
	PluginID       uint
	PluginName     string
	Path           string
	Methods        []string
	RequireAuth    bool
	StreamResponse bool
	Description    string
	Metadata       map[string]string
}

// PluginManagerInterface defines the interface we need from the plugin manager
// This avoids circular imports between api and plugins packages
type PluginManagerInterface interface {
	ExecutePluginChain(llmID uint, hookType string, input interface{}, pluginCtx interface{}) (interface{}, error)
	GetPluginsForLLM(llmID uint, hookType string) (interface{}, error)
	IsPluginLoaded(pluginID uint) bool
	RefreshLLMPluginMapping(llmID uint) error

	// Custom endpoint support
	GetEndpointRoute(method, pluginName, subPath string) *EndpointRouteInfo
	HandleEndpointRequest(ctx context.Context, pluginID uint, req *pb.EndpointRequest) (*pb.EndpointResponse, error)
	HandleEndpointRequestStream(ctx context.Context, pluginID uint, req *pb.EndpointRequest) (interface{ Recv() (*pb.EndpointResponseChunk, error) }, error)
}
