// internal/api/plugin_middleware.go
package api

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// PluginManagerInterface defines the interface we need from the plugin manager
// This avoids circular imports between api and plugins packages
type PluginManagerInterface interface {
	ExecutePluginChain(llmID uint, hookType string, input interface{}, pluginCtx interface{}) (interface{}, error)
	GetPluginsForLLM(llmID uint, hookType string) (interface{}, error)
	IsPluginLoaded(pluginID uint) bool
	RefreshLLMPluginMapping(llmID uint) error
}

// PluginMiddlewareConfig holds configuration for plugin middleware
type PluginMiddlewareConfig struct {
	PluginManager PluginManagerInterface
	Services      *services.ServiceContainer
}

// llmSlugRegex extracts LLM slug from path
var llmSlugRegex = regexp.MustCompile(`^/llm/(rest|stream)/([^/]+)/`)

// CreatePluginMiddleware creates middleware that integrates plugins with the gateway
func CreatePluginMiddleware(config *PluginMiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only process LLM proxy requests
		if !strings.HasPrefix(c.Request.URL.Path, "/llm/") {
			c.Next()
			return
		}

		// Extract LLM slug from Gin path parameter
		path := c.Param("path")
		if path == "" {
			c.Next()
			return
		}
		
		// path would be something like "rest/claude-sonnet-4/v1/messages"
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) < 2 {
			c.Next()
			return
		}
		llmSlug := parts[1] // "claude-sonnet-4"
		
		log.Debug().Str("llm_slug", llmSlug).Str("full_path", path).Msg("Plugin middleware processing LLM request")

		// Get LLM information
		llmInterface, err := config.Services.GatewayService.GetLLMBySlug(llmSlug)
		if err != nil {
			log.Error().Err(err).Str("llm_slug", llmSlug).Msg("Failed to get LLM by slug")
			c.Next()
			return
		}

		var llmID uint
		var vendor string
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
			vendor = llm.Vendor
		} else {
			log.Error().Str("llm_slug", llmSlug).Msg("Invalid LLM type from service")
			c.Next()
			return
		}

		// Get authentication context
		authResult := auth.GetAuthResult(c)
		var appID, userID uint
		if authResult != nil {
			appID = authResult.AppID
			// userID would be available if we had user authentication
		}

		// Create plugin context (using basic map[string]interface{} to avoid import cycle)
		pluginCtx := map[string]interface{}{
			"request_id":    generateRequestID(),
			"llm_id":        llmID,
			"llm_slug":      llmSlug,
			"vendor":        vendor,
			"app_id":        appID,
			"user_id":       userID,
			"metadata":      make(map[string]interface{}),
			"trace_context": make(map[string]string),
		}

		// Set request ID header for tracking
		c.Header("X-Request-ID", pluginCtx["request_id"].(string))

		// Execute pre-auth plugins
		if blocked := executePreAuthPlugins(config.PluginManager, llmID, c, pluginCtx); blocked {
			return // Request was blocked by plugin
		}

		// Store plugin context for post-processing
		c.Set("plugin_context", pluginCtx)
		c.Set("llm_id", llmID)

		// Continue to next middleware/handler
		c.Next()

		// Execute response plugins after the request completes
		// Note: This middleware is unused - response handling is in CreatePluginAwareLLMHandler
		// executeResponsePlugins(config.PluginManager, llmID, c, pluginCtx)
	}
}

// executePreAuthPlugins executes pre-authentication plugins
func executePreAuthPlugins(manager PluginManagerInterface, llmID uint, c *gin.Context, pluginCtx map[string]interface{}) bool {
	log.Debug().Uint("llm_id", llmID).Msg("Starting executePreAuthPlugins")
	
	// Read request body for processing
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request.Body)
		// Restore body for subsequent middleware
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
	
	log.Debug().Int("body_size", len(bodyBytes)).Msg("Read request body for plugin processing")

	// Create plugin request as a map to avoid type dependencies
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0] // Take first value
		}
	}

	pluginReq := map[string]interface{}{
		"method":      c.Request.Method,
		"path":        c.Request.URL.Path,
		"headers":     headers,
		"body":        bodyBytes,
		"remote_addr": c.ClientIP(),
		"context":     pluginCtx,
	}

	// Execute pre-auth plugin chain (using string constants to avoid import)
	log.Debug().Msg("Calling plugin manager ExecutePluginChain for pre_auth")
	result, err := manager.ExecutePluginChain(llmID, "pre_auth", pluginReq, pluginCtx)
	if err != nil {
		log.Error().Err(err).Msg("Pre-auth plugin chain failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Plugin execution failed",
			"message": "Pre-authentication plugin error",
		})
		c.Abort()
		return true
	}
	log.Debug().Interface("result_type", result).Msg("Pre-auth plugin chain completed, checking result type")

	// Check if any plugin wants to block the request
	if pluginResp, ok := result.(map[string]interface{}); ok {
		log.Debug().Interface("plugin_response", pluginResp).Msg("Plugin returned response, checking for modifications")
		if block, hasBlock := pluginResp["block"].(bool); hasBlock && block {
			// Plugin blocked the request
			statusCode := http.StatusForbidden
			if code, ok := pluginResp["status_code"].(int); ok {
				statusCode = code
			}
			
			c.Status(statusCode)
			
			// Set headers from plugin
			if headers, ok := pluginResp["headers"].(map[string]string); ok {
				for key, value := range headers {
					c.Header(key, value)
				}
			}
			
			if body, ok := pluginResp["body"].([]byte); ok && len(body) > 0 {
				c.Data(statusCode, "application/json", body)
			}
			
			c.Abort()
			return true
		}
		
		// Apply plugin modifications to request if any
		if modified, ok := pluginResp["modified"].(bool); ok && modified {
			log.Debug().Msg("Plugin returned Modified: true, applying request modifications")
			applyRequestModifications(c, pluginResp)
		} else {
			log.Debug().Bool("has_modified", ok).Interface("modified_value", pluginResp["modified"]).Msg("Plugin did not modify request")
		}
	} else {
		log.Error().Interface("result", result).Msg("CRITICAL: Plugin result is not a map[string]interface{} - data conversion failed")
	}

	return false
}

// executeResponsePlugins executes response plugins before sending response to client
func executeResponsePlugins(manager PluginManagerInterface, llmID uint, c *gin.Context, pluginCtx map[string]interface{}, responseBuffer *bufferingResponseWriter) {
	// Create response data from buffered response
	respData := map[string]interface{}{
		"request_id":   pluginCtx["request_id"],
		"status_code":  responseBuffer.statusCode,
		"headers":      responseBuffer.headers,
		"body":         responseBuffer.body.Bytes(),
		"context":      pluginCtx,
		"latency_ms":   int64(0), // Would need to be calculated from request start time
	}

	// Execute response plugin chain
	log.Debug().Msg("Calling plugin manager ExecutePluginChain for on_response")
	result, err := manager.ExecutePluginChain(llmID, "on_response", respData, pluginCtx)
	if err != nil {
		log.Error().Err(err).Msg("Response plugin chain failed")
		return
	}
	log.Debug().Interface("result_type", result).Msg("Response plugin chain completed, checking result type")

	// Check if response was modified
	if pluginResp, ok := result.(map[string]interface{}); ok {
		log.Debug().Interface("plugin_response", pluginResp).Msg("Response plugin returned response, checking for modifications")
		if modified, ok := pluginResp["modified"].(bool); ok && modified {
			log.Debug().Msg("Response plugin returned Modified: true, applying response modifications")
			// Apply modified response
			if modifiedBody, ok := pluginResp["body"].([]byte); ok {
				responseBuffer.body = bytes.NewBuffer(modifiedBody)
			}
			if modifiedHeaders, ok := pluginResp["headers"].(map[string]string); ok {
				responseBuffer.headers = modifiedHeaders
			}
			if modifiedStatus, ok := pluginResp["status_code"].(int); ok {
				responseBuffer.statusCode = modifiedStatus
			}
		} else {
			log.Debug().Bool("has_modified", ok).Interface("modified_value", pluginResp["modified"]).Msg("Response plugin did not modify response")
		}
	} else {
		log.Error().Interface("result", result).Msg("CRITICAL: Response plugin result is not a map[string]interface{} - data conversion failed")
	}

	// Send the final response to the client
	c.Writer.WriteHeader(responseBuffer.statusCode)
	for key, value := range responseBuffer.headers {
		c.Writer.Header().Set(key, value)
	}
	c.Writer.Write(responseBuffer.body.Bytes())

	log.Debug().
		Str("request_id", pluginCtx["request_id"].(string)).
		Int("status_code", responseBuffer.statusCode).
		Msg("Response plugin processing completed and response sent to client")
}

// applyRequestModifications applies plugin modifications to the request
func applyRequestModifications(c *gin.Context, resp map[string]interface{}) {
	// Apply header modifications
	if headers, ok := resp["headers"].(map[string]string); ok {
		for key, value := range headers {
			c.Request.Header.Set(key, value)
		}
	}
	
	// Apply body modifications
	if body, ok := resp["body"].([]byte); ok && len(body) > 0 {
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
	}
}

// Helper functions

func extractLLMSlug(path string) string {
	matches := llmSlugRegex.FindStringSubmatch(path)
	if len(matches) >= 3 {
		return matches[2]
	}
	return ""
}

// CreatePluginAwareLLMHandler creates a handler that processes plugins before calling the AI Gateway
func CreatePluginAwareLLMHandler(aiGatewayHandler http.Handler, config *PluginMiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Debug().Str("path", c.Request.URL.Path).Msg("Plugin-aware LLM handler processing request")
		
		// Extract LLM slug from Gin path parameter
		path := c.Param("path")
		if path == "" {
			// No path parameter, skip plugin processing
			aiGatewayHandler.ServeHTTP(c.Writer, c.Request)
			return
		}
		
		// path would be something like "rest/claude-sonnet-4/v1/messages"
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) < 2 {
			// Invalid path format, skip plugin processing
			aiGatewayHandler.ServeHTTP(c.Writer, c.Request)
			return
		}
		llmSlug := parts[1] // "claude-sonnet-4"
		
		log.Debug().Str("llm_slug", llmSlug).Str("full_path", path).Msg("Plugin-aware handler processing LLM request")

		// Get LLM information
		llmInterface, err := config.Services.GatewayService.GetLLMBySlug(llmSlug)
		if err != nil {
			log.Error().Err(err).Str("llm_slug", llmSlug).Msg("Failed to get LLM by slug")
			aiGatewayHandler.ServeHTTP(c.Writer, c.Request)
			return
		}

		var llmID uint
		var vendor string
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
			vendor = llm.Vendor
		} else {
			log.Error().Str("llm_slug", llmSlug).Msg("Invalid LLM type from service")
			aiGatewayHandler.ServeHTTP(c.Writer, c.Request)
			return
		}

		// Create plugin context
		pluginCtx := map[string]interface{}{
			"request_id":    generateRequestID(),
			"llm_id":        llmID,
			"llm_slug":      llmSlug,
			"vendor":        vendor,
			"app_id":        uint(1), // Default app ID for plugin auth
			"user_id":       uint(0),
			"metadata":      make(map[string]interface{}),
			"trace_context": make(map[string]string),
		}

		// Set request ID header for tracking
		c.Header("X-Request-ID", pluginCtx["request_id"].(string))

		// Execute pre-auth plugins
		log.Debug().Uint("llm_id", llmID).Str("method", c.Request.Method).Msg("About to execute pre-auth plugins")
		if blocked := executePreAuthPlugins(config.PluginManager, llmID, c, pluginCtx); blocked {
			return // Request was blocked by plugin
		}
		log.Debug().Msg("Pre-auth plugins completed")

		// Note: Auth plugins are handled by the AI Gateway's authentication layer, not here
		
		// Execute post-auth plugins
		log.Debug().Msg("About to execute post-auth plugins")
		if blocked := executePostAuthPlugins(config.PluginManager, llmID, c, pluginCtx); blocked {
			return // Request was blocked by post-auth plugin
		}
		log.Debug().Msg("Post-auth plugins completed")

		// Store plugin context for post-processing
		c.Set("plugin_context", pluginCtx)
		c.Set("llm_id", llmID)

		// Create a buffering response writer to capture response for plugin processing
		responseBuffer := &bufferingResponseWriter{
			ResponseWriter: c.Writer,
			statusCode:     200,
			headers:       make(map[string]string),
			body:          &bytes.Buffer{},
		}
		
		// Replace the response writer temporarily
		originalWriter := c.Writer
		c.Writer = responseBuffer

		// Call AI Gateway handler
		aiGatewayHandler.ServeHTTP(c.Writer, c.Request)

		// Restore original writer
		c.Writer = originalWriter
		
		// Execute response plugins before sending to client
		executeResponsePlugins(config.PluginManager, llmID, c, pluginCtx, responseBuffer)
	}
}

// executeAuthPlugins executes authentication plugins
func executeAuthPlugins(manager PluginManagerInterface, llmID uint, c *gin.Context, pluginCtx map[string]interface{}) bool {
	log.Debug().Uint("llm_id", llmID).Msg("Starting executeAuthPlugins")
	
	// Create auth request
	authReq := map[string]interface{}{
		"credential": extractToken(c),
		"auth_type":  "bearer",
		"request":    createBasicPluginRequest(c, pluginCtx),
		"context":    pluginCtx,
	}
	
	// Execute auth plugin chain
	_, err := manager.ExecutePluginChain(llmID, "auth", authReq, pluginCtx)
	if err != nil {
		log.Error().Err(err).Msg("Auth plugin chain failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Plugin execution failed",
			"message": "Authentication plugin error",
		})
		c.Abort()
		return true
	}
	
	log.Debug().Msg("Auth plugin chain completed successfully")
	return false
}

// executePostAuthPlugins executes post-authentication plugins
func executePostAuthPlugins(manager PluginManagerInterface, llmID uint, c *gin.Context, pluginCtx map[string]interface{}) bool {
	log.Debug().Uint("llm_id", llmID).Msg("Starting executePostAuthPlugins")
	
	// Create enriched request  
	basicReq := createBasicPluginRequest(c, pluginCtx)
	log.Debug().Interface("basic_request", basicReq).Msg("Created basic plugin request for post-auth")
	
	enrichedReq := map[string]interface{}{
		"request":       basicReq,
		"user_id":       "plugin-user",
		"app_id":        "plugin-app", 
		"authenticated": true,
		"auth_claims":   make(map[string]string),
	}
	log.Debug().Interface("enriched_request", enrichedReq).Msg("Created enriched request for post-auth plugin")
	
	// Execute post-auth plugin chain
	log.Debug().Msg("Calling plugin manager ExecutePluginChain for post_auth")
	result, err := manager.ExecutePluginChain(llmID, "post_auth", enrichedReq, pluginCtx)
	if err != nil {
		log.Error().Err(err).Msg("Post-auth plugin chain failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Plugin execution failed", 
			"message": "Post-authentication plugin error",
		})
		c.Abort()
		return true
	}
	log.Debug().Interface("result_type", result).Msg("Post-auth plugin chain completed, checking result type")
	
	// Check if any plugin wants to block the request
	if pluginResp, ok := result.(map[string]interface{}); ok {
		log.Debug().Interface("plugin_response", pluginResp).Msg("Post-auth plugin returned response, checking for modifications")
		if block, hasBlock := pluginResp["block"].(bool); hasBlock && block {
			log.Debug().Msg("Post-auth plugin blocked the request")
			c.Status(http.StatusForbidden)
			c.Abort()
			return true
		}
		
		if modified, ok := pluginResp["modified"].(bool); ok && modified {
			log.Debug().Msg("Post-auth plugin returned Modified: true, applying request modifications")
			applyRequestModifications(c, pluginResp)
		} else {
			log.Debug().Bool("has_modified", ok).Interface("modified_value", pluginResp["modified"]).Msg("Post-auth plugin did not modify request")
		}
	} else {
		log.Error().Interface("result", result).Msg("CRITICAL: Post-auth plugin result is not a map[string]interface{} - data conversion failed")
	}
	
	log.Debug().Msg("Post-auth plugin chain completed successfully")
	return false
}

// extractToken extracts the bearer token from the Authorization header
func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return authHeader
}

// createBasicPluginRequest creates a basic plugin request structure
func createBasicPluginRequest(c *gin.Context, pluginCtx map[string]interface{}) map[string]interface{} {
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
	
	return map[string]interface{}{
		"method":      c.Request.Method,
		"path":        c.Request.URL.Path,
		"headers":     headers,
		"body":        bodyBytes,
		"remote_addr": c.ClientIP(),
		"context":     pluginCtx,
	}
}

// bufferingResponseWriter captures response data for plugin processing
type bufferingResponseWriter struct {
	gin.ResponseWriter
	statusCode int
	headers    map[string]string
	body      *bytes.Buffer
}

func (w *bufferingResponseWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *bufferingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	// Capture headers
	for key, values := range w.Header() {
		if len(values) > 0 {
			w.headers[key] = values[0]
		}
	}
}

func (w *bufferingResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func generateRequestID() string {
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}