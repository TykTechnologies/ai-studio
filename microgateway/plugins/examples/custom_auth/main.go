// plugins/examples/custom_auth/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// TokenConfig represents configuration for a single token
type TokenConfig struct {
	Token       string `json:"token"`
	AppID       uint   `json:"app_id"`
	UserID      string `json:"user_id"`
	Description string `json:"description"`
}

// CustomAuthPlugin replaces default token authentication with custom logic
type CustomAuthPlugin struct {
	tokenMap            map[string]*TokenConfig // token -> config
	defaultAppID        uint
	rejectUnknownTokens bool
}

// Initialize implements BasePlugin
func (p *CustomAuthPlugin) Initialize(config map[string]interface{}) error {
	// Initialize token map
	p.tokenMap = make(map[string]*TokenConfig)

	log.Printf("Custom auth: Initialize called with %d config keys", len(config))

	// Parse tokens array from config - REQUIRED, no fallback
	// NOTE: Config comes as map[string]interface{} but gRPC passes it as map[string]string
	// So complex values are stringified and need to be parsed from JSON
	tokensValue, hasTokens := config["tokens"]
	if !hasTokens {
		return fmt.Errorf("custom_auth plugin requires 'tokens' array in configuration")
	}

	log.Printf("Custom auth: Found 'tokens' in config, type: %T, value: %v", tokensValue, tokensValue)

	// Parse tokens - handle both string (from gRPC) and interface (from JSON)
	var tokensData interface{} = tokensValue
	if tokensStr, isString := tokensValue.(string); isString {
		log.Printf("Custom auth: Tokens is a string (from gRPC), parsing JSON: %s", tokensStr)
		if err := json.Unmarshal([]byte(tokensStr), &tokensData); err != nil {
			return fmt.Errorf("failed to parse 'tokens' as JSON: %w", err)
		}
	}

	// Parse the tokens array
	tokensJSON, err := json.Marshal(tokensData)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens data: %w", err)
	}

	var tokens []TokenConfig
	if err := json.Unmarshal(tokensJSON, &tokens); err != nil {
		return fmt.Errorf("failed to unmarshal tokens array: %w", err)
	}

	if len(tokens) == 0 {
		return fmt.Errorf("custom_auth plugin requires at least one token in configuration")
	}

	log.Printf("Custom auth: Successfully parsed %d tokens", len(tokens))
	for i := range tokens {
		if tokens[i].Token == "" {
			return fmt.Errorf("token at index %d has empty token value", i)
		}
		if tokens[i].AppID == 0 {
			return fmt.Errorf("token '%s' has invalid app_id (must be > 0)", tokens[i].Token)
		}
		p.tokenMap[tokens[i].Token] = &tokens[i]
		log.Printf("Custom auth: Loaded token '%s' with AppID %d, UserID '%s'",
			tokens[i].Token, tokens[i].AppID, tokens[i].UserID)
	}

	// Parse reject_unknown_tokens (default: true)
	p.rejectUnknownTokens = true // Default to secure behavior
	if rejectInterface, ok := config["reject_unknown_tokens"]; ok {
		if rejectStr, isString := rejectInterface.(string); isString {
			// Handle string "true"/"false" from gRPC
			p.rejectUnknownTokens = (rejectStr == "true")
		} else if rejectBool, isBool := rejectInterface.(bool); isBool {
			p.rejectUnknownTokens = rejectBool
		}
	}

	log.Printf("Custom auth: Plugin initialized successfully with %d tokens, rejectUnknown=%v",
		len(p.tokenMap), p.rejectUnknownTokens)

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

// GetConfigSchema implements ConfigSchemaProvider
func (p *CustomAuthPlugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"title":   "Custom Authentication Plugin Configuration",
		"description": "Configure custom token-based authentication with app and user mappings",
		"properties": map[string]interface{}{
			"tokens": map[string]interface{}{
				"type":  "array",
				"title": "Authentication Tokens",
				"description": "List of valid tokens with their app and user mappings",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"token": map[string]interface{}{
							"type":        "string",
							"title":       "Token Value",
							"description": "The authentication token (without 'Bearer ' prefix)",
							"minLength":   1,
							"maxLength":   500,
						},
						"app_id": map[string]interface{}{
							"type":        "integer",
							"title":       "App ID",
							"description": "Numeric App ID from the database that this token authenticates",
							"minimum":     1,
						},
						"user_id": map[string]interface{}{
							"type":        "string",
							"title":       "User ID",
							"description": "User identifier associated with this token",
							"default":     "",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"title":       "Description",
							"description": "Optional description for this token (e.g., 'John's dev token')",
							"default":     "",
						},
					},
					"required": []string{"token", "app_id"},
					"additionalProperties": false,
				},
				"default": []interface{}{},
			},
			"default_app_id": map[string]interface{}{
				"type":        "integer",
				"title":       "Default App ID",
				"description": "Fallback App ID if a token doesn't specify one (used for backward compatibility)",
				"default":     1,
				"minimum":     1,
			},
			"reject_unknown_tokens": map[string]interface{}{
				"type":        "boolean",
				"title":       "Reject Unknown Tokens",
				"description": "If true, reject tokens not in the configured list. If false, accept with default app ID.",
				"default":     true,
			},
		},
		"additionalProperties": false,
	}

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config schema: %w", err)
	}

	return schemaBytes, nil
}

// Authenticate implements AuthPlugin
func (p *CustomAuthPlugin) Authenticate(ctx context.Context, req *sdk.AuthRequest, pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {
	// Extract token from credential
	token := req.Credential

	// Handle Bearer token format
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	// Lookup token in configured token map
	if tokenConfig, ok := p.tokenMap[token]; ok {
		// Token found - return configured app and user
		return &sdk.AuthResponse{
			Authenticated: true,
			UserID:        tokenConfig.UserID,
			AppID:         fmt.Sprintf("%d", tokenConfig.AppID), // Convert uint to string for interface
			Claims: map[string]string{
				"source":      "custom-auth-plugin",
				"token":       token,
				"description": tokenConfig.Description,
			},
		}, nil
	}

	// Token not in map - handle based on policy
	if p.rejectUnknownTokens {
		return &sdk.AuthResponse{
			Authenticated: false,
			ErrorMessage:  "Invalid token. Token not found in custom auth plugin configuration.",
		}, nil
	}

	// Accept unknown tokens with default app ID
	return &sdk.AuthResponse{
		Authenticated: true,
		UserID:        "unknown-user",
		AppID:         fmt.Sprintf("%d", p.defaultAppID),
		Claims: map[string]string{
			"source": "custom-auth-plugin",
			"token":  "unknown",
		},
	}, nil
}

// ValidateToken implements AuthPlugin
func (p *CustomAuthPlugin) ValidateToken(ctx context.Context, token string, pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {
	// Handle Bearer token format
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	// Lookup token in configured token map
	if tokenConfig, ok := p.tokenMap[token]; ok {
		// Token found - return configured app and user
		return &sdk.AuthResponse{
			Authenticated: true,
			UserID:        tokenConfig.UserID,
			AppID:         fmt.Sprintf("%d", tokenConfig.AppID), // Convert uint to string for interface
			Claims: map[string]string{
				"source":      "custom-auth-plugin",
				"token":       token,
				"description": tokenConfig.Description,
			},
		}, nil
	}

	// Token not in map - handle based on policy
	if p.rejectUnknownTokens {
		return &sdk.AuthResponse{
			Authenticated: false,
			ErrorMessage:  "Invalid token. Token not found in custom auth plugin configuration.",
		}, nil
	}

	// Accept unknown tokens with default app ID
	return &sdk.AuthResponse{
		Authenticated: true,
		UserID:        "unknown-user",
		AppID:         fmt.Sprintf("%d", p.defaultAppID),
		Claims: map[string]string{
			"source": "custom-auth-plugin",
			"token":  "unknown",
		},
	}, nil
}

func main() {
	plugin := &CustomAuthPlugin{}
	sdk.ServePlugin(plugin)
}