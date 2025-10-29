package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// CreatePostAuthPluginCallback creates a callback for executing post-auth plugins after AI Gateway authentication
func CreatePostAuthPluginCallback(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) proxy.PostAuthCallback {
	return func(w http.ResponseWriter, r *http.Request, appID uint) bool {
		// Create a gin.Context wrapper for the plugin middleware
		ginCtx, _ := gin.CreateTestContext(w)
		ginCtx.Request = r

		// Extract LLM info from request path
		llmSlug := extractLLMSlugFromPath(r.URL.Path)
		if llmSlug == "" {
			log.Debug().Str("path", r.URL.Path).Msg("No LLM slug found in path, skipping post-auth plugins")
			return false
		}

		// Get LLM ID
		llmInterface, err := serviceContainer.GatewayService.GetLLMBySlug(llmSlug)
		if err != nil {
			log.Debug().Err(err).Str("llm_slug", llmSlug).Msg("Failed to get LLM for post-auth plugins")
			return false
		}

		var llmID uint
		var vendor string
		if llm, ok := llmInterface.(interface{ GetID() uint; GetVendor() string }); ok {
			llmID = llm.GetID()
			vendor = llm.GetVendor()
		} else {
			log.Debug().Str("llm_slug", llmSlug).Msg("Invalid LLM type for post-auth plugins")
			return false
		}

		// Create plugin context with authenticated app_id
		pluginCtx := map[string]interface{}{
			"request_id":    r.Header.Get("X-Request-ID"),
			"llm_id":        llmID,
			"llm_slug":      llmSlug,
			"vendor":        vendor,
			"app_id":        appID, // Authenticated app ID from AI Gateway
			"user_id":       uint(0),
			"metadata":      make(map[string]interface{}),
			"trace_context": make(map[string]string),
		}

		// Create plugin manager adapter
		pluginManagerAdapter := NewPluginManagerAdapter(pluginManager)

		// Execute post-auth plugins
		log.Debug().Uint("llm_id", llmID).Uint("app_id", appID).Msg("Executing post-auth plugins after AI Gateway authentication")
		if blocked := executePostAuthPlugins(pluginManagerAdapter, llmID, ginCtx, pluginCtx); blocked {
			log.Debug().Msg("Post-auth plugin blocked the request")
			return true // Request was blocked
		}

		log.Debug().Msg("Post-auth plugins completed successfully")
		return false // Continue processing
	}
}

// extractLLMSlugFromPath extracts the LLM slug from the request path
// Handles paths like: /llm/rest/claude/v1/messages or /llm/stream/openai/v1/completions
func extractLLMSlugFromPath(path string) string {
	// Path format: /llm/{mode}/{slug}/{...}
	// where mode is "rest" or "stream"
	parts := splitPath(path)
	if len(parts) >= 3 && parts[0] == "llm" {
		return parts[2] // The slug is the third part
	}
	return ""
}

// splitPath splits a URL path into parts, skipping empty strings
func splitPath(path string) []string {
	var parts []string
	for _, part := range splitBySlash(path) {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// splitBySlash splits a string by "/"
func splitBySlash(s string) []string {
	var result []string
	current := ""
	for _, char := range s {
		if char == '/' {
			result = append(result, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	result = append(result, current)
	return result
}
