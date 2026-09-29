// plugins/interfaces/pre_auth.go
package interfaces

import "context"

// PreAuthPlugin defines the interface for pre-authentication plugins
// These plugins execute before authentication and can be used for:
// - Rate limiting
// - Request enrichment
// - Request validation
// - IP filtering
type PreAuthPlugin interface {
	BasePlugin
	
	// ProcessRequest processes the incoming request before authentication
	ProcessRequest(ctx context.Context, req *PluginRequest, pluginCtx *PluginContext) (*PluginResponse, error)
}