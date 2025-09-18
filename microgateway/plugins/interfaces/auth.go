// plugins/interfaces/auth.go
package interfaces

import "context"

// AuthRequest represents an authentication request
type AuthRequest struct {
	Credential string         `json:"credential"`
	AuthType   string         `json:"auth_type"` // "token", "bearer", "api-key", etc.
	Request    *PluginRequest `json:"request"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Authenticated bool              `json:"authenticated"`
	UserID        string            `json:"user_id,omitempty"`
	AppID         string            `json:"app_id,omitempty"`
	Claims        map[string]string `json:"claims,omitempty"`
	ErrorMessage  string            `json:"error_message,omitempty"`
}

// AuthPlugin defines the interface for authentication plugins
// These plugins can replace or augment the default authentication mechanism
type AuthPlugin interface {
	BasePlugin
	
	// Authenticate performs authentication based on the provided request
	Authenticate(ctx context.Context, req *AuthRequest, pluginCtx *PluginContext) (*AuthResponse, error)
	
	// ValidateToken validates a specific token (if the plugin supports token-based auth)
	ValidateToken(ctx context.Context, token string, pluginCtx *PluginContext) (*AuthResponse, error)
}