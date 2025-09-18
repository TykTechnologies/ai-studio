// internal/auth/middleware.go
package auth

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ContextKey represents the type for context keys
type ContextKey string

const (
	// AuthResultKey is the context key for auth result
	AuthResultKey ContextKey = "auth_result"

	// AppIDKey is the context key for app ID
	AppIDKey ContextKey = "app_id"

	// ScopesKey is the context key for scopes
	ScopesKey ContextKey = "scopes"
)

// PluginAwareAuthProvider extends AuthProvider with plugin awareness
type PluginAwareAuthProvider interface {
	AuthProvider
	HasAuthPluginsForLLM(llmSlug string) (bool, error)
}

// llmSlugRegex extracts LLM slug from path
var llmSlugRegex = regexp.MustCompile(`^/llm/(rest|stream)/([^/]+)/`)

// RequireAuth middleware validates API tokens and sets auth context
func RequireAuth(provider AuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this is an LLM request and if auth plugins are present
		if isLLMRequest(c) {
			llmSlug := extractLLMSlug(c)
			if llmSlug != "" {
				if pluginProvider, ok := provider.(PluginAwareAuthProvider); ok {
					hasAuthPlugins, err := pluginProvider.HasAuthPluginsForLLM(llmSlug)
					if err != nil {
						log.Debug().Err(err).Str("llm_slug", llmSlug).Msg("Failed to check for auth plugins")
					} else if hasAuthPlugins {
						log.Debug().Str("llm_slug", llmSlug).Msg("Auth plugins detected for LLM, skipping regular auth")
						c.Next()
						return
					}
				}
			}
		}

		token := extractToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Missing or invalid authorization token",
			})
			c.Abort()
			return
		}

		authResult, err := provider.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal Server Error",
				"message": "Failed to validate token",
			})
			c.Abort()
			return
		}

		if !authResult.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": authResult.Error,
			})
			c.Abort()
			return
		}

		// Set auth context
		c.Set(string(AuthResultKey), authResult)
		c.Set(string(AppIDKey), authResult.AppID)
		c.Set(string(ScopesKey), authResult.Scopes)

		c.Next()
	}
}

// RequireScopes middleware checks if the authenticated token has required scopes
func RequireScopes(requiredScopes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authResult := GetAuthResult(c)
		if authResult == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authentication required",
			})
			c.Abort()
			return
		}

		if !hasRequiredScopes(authResult.Scopes, requiredScopes) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth middleware validates tokens if present but doesn't require them
func OptionalAuth(provider AuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.Next()
			return
		}

		authResult, err := provider.ValidateToken(token)
		if err != nil {
			// Log error but don't abort
			c.Next()
			return
		}

		if authResult.Valid {
			// Set auth context
			c.Set(string(AuthResultKey), authResult)
			c.Set(string(AppIDKey), authResult.AppID)
			c.Set(string(ScopesKey), authResult.Scopes)
		}

		c.Next()
	}
}

// extractToken extracts the bearer token from the Authorization header
func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		// Check query parameter as fallback
		return c.Query("token")
	}

	// Check for Bearer token
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Check for simple token format
	return authHeader
}

// hasRequiredScopes checks if the user scopes include all required scopes
func hasRequiredScopes(userScopes, requiredScopes []string) bool {
	if len(requiredScopes) == 0 {
		return true
	}

	userScopeSet := make(map[string]bool)
	for _, scope := range userScopes {
		userScopeSet[scope] = true
	}

	// Check for wildcard scope
	if userScopeSet["*"] || userScopeSet["admin"] {
		return true
	}

	// Check each required scope
	for _, required := range requiredScopes {
		if !userScopeSet[required] {
			return false
		}
	}

	return true
}

// Helper functions to get auth data from context

// GetAuthResult retrieves the auth result from context
func GetAuthResult(c *gin.Context) *AuthResult {
	if value, exists := c.Get(string(AuthResultKey)); exists {
		if authResult, ok := value.(*AuthResult); ok {
			return authResult
		}
	}
	return nil
}

// GetAppID retrieves the app ID from context
func GetAppID(c *gin.Context) uint {
	if value, exists := c.Get(string(AppIDKey)); exists {
		if appID, ok := value.(uint); ok {
			return appID
		}
	}
	return 0
}

// GetScopes retrieves the scopes from context
func GetScopes(c *gin.Context) []string {
	if value, exists := c.Get(string(ScopesKey)); exists {
		if scopes, ok := value.([]string); ok {
			return scopes
		}
	}
	return nil
}

// HasScope checks if the current user has a specific scope
func HasScope(c *gin.Context, scope string) bool {
	scopes := GetScopes(c)
	return hasRequiredScopes(scopes, []string{scope})
}

// IsAdmin checks if the current user has admin privileges
func IsAdmin(c *gin.Context) bool {
	return HasScope(c, "admin") || HasScope(c, "*")
}

// isLLMRequest checks if the current request is an LLM request
func isLLMRequest(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/llm/")
}

// extractLLMSlug extracts the LLM slug from Gin path parameter
func extractLLMSlug(c *gin.Context) string {
	path := c.Param("path")
	if path == "" {
		return ""
	}
	// path would be something like "rest/claude-sonnet-4/v1/messages"
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[1] // Return "claude-sonnet-4"
	}
	return ""
}
