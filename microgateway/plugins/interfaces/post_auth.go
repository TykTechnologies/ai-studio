// plugins/interfaces/post_auth.go
package interfaces

import "context"

// EnrichedRequest represents a request that has been enriched with authentication context
type EnrichedRequest struct {
	*PluginRequest
	UserID        string            `json:"user_id,omitempty"`
	AppID         string            `json:"app_id,omitempty"`
	AuthClaims    map[string]string `json:"auth_claims,omitempty"`
	Authenticated bool              `json:"authenticated"`
}

// PostAuthPlugin defines the interface for post-authentication plugins
// These plugins execute after authentication and can be used for:
// - Authorization checks
// - Request transformation
// - Content filtering
// - Logging enrichment
type PostAuthPlugin interface {
	BasePlugin
	
	// ProcessRequest processes the request after successful authentication
	ProcessRequest(ctx context.Context, req *EnrichedRequest, pluginCtx *PluginContext) (*PluginResponse, error)
}