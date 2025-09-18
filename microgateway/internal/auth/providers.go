// internal/auth/providers.go
package auth

import (
	"net/http"
)

// AuthenticationProvider defines a pluggable authentication mechanism
type AuthenticationProvider interface {
	// Name returns the provider name (e.g., "token", "oauth", "api-key")
	Name() string
	
	// ValidateRequest validates an HTTP request and returns authentication result
	ValidateRequest(req *http.Request) (*AuthResult, error)
	
	// Configure sets up the provider with LLM-specific configuration
	Configure(config map[string]interface{}) error
	
	// Description returns a human-readable description of the auth mechanism
	Description() string
}

// AuthProviderRegistry manages available authentication providers
type AuthProviderRegistry struct {
	providers map[string]AuthenticationProvider
}

// NewAuthProviderRegistry creates a new provider registry
func NewAuthProviderRegistry() *AuthProviderRegistry {
	return &AuthProviderRegistry{
		providers: make(map[string]AuthenticationProvider),
	}
}

// Register adds an authentication provider to the registry
func (r *AuthProviderRegistry) Register(provider AuthenticationProvider) {
	r.providers[provider.Name()] = provider
}

// Get retrieves an authentication provider by name
func (r *AuthProviderRegistry) Get(name string) (AuthenticationProvider, bool) {
	provider, exists := r.providers[name]
	return provider, exists
}

// List returns all registered provider names
func (r *AuthProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// TokenAuthenticationProvider implements token-based authentication
type TokenAuthenticationProvider struct {
	authProvider AuthProvider // Existing token auth system
}

// NewTokenAuthenticationProvider creates a new token authentication provider
func NewTokenAuthenticationProvider(authProvider AuthProvider) *TokenAuthenticationProvider {
	return &TokenAuthenticationProvider{
		authProvider: authProvider,
	}
}

// Name returns the provider identifier
func (p *TokenAuthenticationProvider) Name() string {
	return "token"
}

// Description returns a description of the provider
func (p *TokenAuthenticationProvider) Description() string {
	return "Bearer token authentication using microgateway API tokens"
}

// ValidateRequest validates a Bearer token from the Authorization header
func (p *TokenAuthenticationProvider) ValidateRequest(req *http.Request) (*AuthResult, error) {
	// Extract Bearer token from Authorization header
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return &AuthResult{
			Valid: false,
			Error: "Missing Authorization header",
		}, nil
	}

	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
		return &AuthResult{
			Valid: false,
			Error: "Invalid Authorization format (expected: Bearer <token>)",
		}, nil
	}

	token := authHeader[len(bearerPrefix):]
	return p.authProvider.ValidateToken(token)
}

// Configure sets up provider-specific configuration
func (p *TokenAuthenticationProvider) Configure(config map[string]interface{}) error {
	// Token provider doesn't need LLM-specific configuration
	return nil
}

// Future authentication providers can be added here:

// OAuthAuthenticationProvider (future implementation)
type OAuthAuthenticationProvider struct {
	clientID     string
	clientSecret string
	tokenURL     string
	introspectURL string
}

// APIKeyAuthenticationProvider (future implementation)  
type APIKeyAuthenticationProvider struct {
	headerName string
	prefix     string
}

// CustomAuthenticationProvider (future implementation)
type CustomAuthenticationProvider struct {
	validationURL string
	headers       map[string]string
}