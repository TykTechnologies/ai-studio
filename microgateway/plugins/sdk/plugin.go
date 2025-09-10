// plugins/sdk/plugin.go
package sdk

import (
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/hashicorp/go-plugin"
)

// HandshakeConfig is used for handshaking with the main microgateway process
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "MICROGATEWAY_PLUGIN",
	MagicCookieValue: "v1",
}

// ServePlugin serves a plugin implementation via gRPC
func ServePlugin(impl interface{}) {
	var pluginMap = map[string]plugin.Plugin{}
	
	// Determine plugin type and add to plugin map
	switch p := impl.(type) {
	case interfaces.PreAuthPlugin:
		pluginMap["plugin"] = &PreAuthPluginGRPC{Impl: p}
	case interfaces.AuthPlugin:
		pluginMap["plugin"] = &AuthPluginGRPC{Impl: p}
	case interfaces.PostAuthPlugin:
		pluginMap["plugin"] = &PostAuthPluginGRPC{Impl: p}
	case interfaces.ResponsePlugin:
		pluginMap["plugin"] = &ResponsePluginGRPC{Impl: p}
	default:
		panic("unsupported plugin type")
	}
	
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins:         pluginMap,
		GRPCServer:      plugin.DefaultGRPCServer,
	})
}

// Re-export interfaces for convenience
type (
	HookType = interfaces.HookType
	
	BasePlugin      = interfaces.BasePlugin
	PreAuthPlugin   = interfaces.PreAuthPlugin
	AuthPlugin      = interfaces.AuthPlugin
	PostAuthPlugin  = interfaces.PostAuthPlugin
	ResponsePlugin  = interfaces.ResponsePlugin
	
	PluginContext         = interfaces.PluginContext
	PluginRequest         = interfaces.PluginRequest
	PluginResponse        = interfaces.PluginResponse
	AuthRequest           = interfaces.AuthRequest
	AuthResponse          = interfaces.AuthResponse
	EnrichedRequest       = interfaces.EnrichedRequest
	ResponseData          = interfaces.ResponseData
	HeadersRequest        = interfaces.HeadersRequest
	HeadersResponse       = interfaces.HeadersResponse
	ResponseWriteRequest  = interfaces.ResponseWriteRequest
	ResponseWriteResponse = interfaces.ResponseWriteResponse
)

// Re-export constants for convenience
const (
	HookTypePreAuth    = interfaces.HookTypePreAuth
	HookTypeAuth       = interfaces.HookTypeAuth
	HookTypePostAuth   = interfaces.HookTypePostAuth
	HookTypeOnResponse = interfaces.HookTypeOnResponse
)