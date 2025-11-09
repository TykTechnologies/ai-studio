// plugins/examples/custom_auth/main.go
package main

import (
	"context"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// CustomAuthPlugin replaces default token authentication with custom logic
type CustomAuthPlugin struct {
	validToken string
}

// Initialize implements BasePlugin
func (p *CustomAuthPlugin) Initialize(config map[string]interface{}) error {
	if token, ok := config["valid_token"]; ok {
		p.validToken = token.(string)
	} else {
		p.validToken = "moocow"
	}
	return nil
}

// GetHookType implements BasePlugin
func (p *CustomAuthPlugin) GetHookType() sdk.HookType {
	return sdk.HookTypeAuth
}

// GetName implements BasePlugin
func (p *CustomAuthPlugin) GetName() string {
	return "custom-auth"
}

// GetVersion implements BasePlugin
func (p *CustomAuthPlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *CustomAuthPlugin) Shutdown() error {
	return nil
}

// Authenticate implements AuthPlugin
func (p *CustomAuthPlugin) Authenticate(ctx context.Context, req *sdk.AuthRequest, pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {
	// Extract token from credential
	token := req.Credential

	// Handle Bearer token format
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	// Check if token matches our custom token
	if token == p.validToken {
		return &sdk.AuthResponse{
			Authenticated: true,
			UserID:        "plugin-user",
			AppID:         "plugin-app",
			Claims: map[string]string{
				"source": "custom-auth-plugin",
				"token":  "moocow",
			},
		}, nil
	}

	// All other tokens are rejected (including real tokens)
	return &sdk.AuthResponse{
		Authenticated: false,
		ErrorMessage:  "Invalid token. Only 'moocow' is accepted by custom auth plugin.",
	}, nil
}

// ValidateToken implements AuthPlugin
func (p *CustomAuthPlugin) ValidateToken(ctx context.Context, token string, pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {
	// Handle Bearer token format
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	// Check if token matches our custom token
	if token == p.validToken {
		return &sdk.AuthResponse{
			Authenticated: true,
			UserID:        "plugin-user",
			AppID:         "plugin-app",
			Claims: map[string]string{
				"source": "custom-auth-plugin",
				"token":  "moocow",
			},
		}, nil
	}

	return &sdk.AuthResponse{
		Authenticated: false,
		ErrorMessage:  "Invalid token. Only 'moocow' is accepted by custom auth plugin.",
	}, nil
}

func main() {
	plugin := &CustomAuthPlugin{}
	sdk.ServePlugin(plugin)
}