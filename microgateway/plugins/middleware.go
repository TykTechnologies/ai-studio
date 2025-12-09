// plugins/middleware.go
package plugins

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
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ServiceContainerInterface defines minimal interface needed to break circular dependency
type ServiceContainerInterface interface {
	GetGatewayService() GatewayServiceInterface
	GetEdgeID() string
	GetEdgeNamespace() string
}

// GatewayServiceInterface defines minimal interface for gateway service
type GatewayServiceInterface interface {
	GetLLMBySlug(slug string) (interface{}, error)
}

// PluginMiddlewareConfig holds configuration for plugin middleware
type PluginMiddlewareConfig struct {
	PluginManager *PluginManager
	Services      ServiceContainerInterface
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

		// Extract LLM slug from path
		llmSlug := extractLLMSlug(c.Request.URL.Path)
		if llmSlug == "" {
			c.Next()
			return
		}

		// Get LLM information
		llmInterface, err := config.Services.GetGatewayService().GetLLMBySlug(llmSlug)
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

		// Get canonical request ID from context (set by RequestIDMiddleware)
		requestID := ""
		if reqID := c.Request.Context().Value("request_id"); reqID != nil {
			requestID = reqID.(string)
		}
		if requestID == "" {
			log.Error().Msg("Request ID not found in context - RequestIDMiddleware not configured")
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		// Create plugin context
		// Include edge identity in metadata for plugin context
		middlewareMetadata := make(map[string]interface{})
		if edgeID := config.Services.GetEdgeID(); edgeID != "" {
			middlewareMetadata["edge_id"] = edgeID
		}
		if edgeNamespace := config.Services.GetEdgeNamespace(); edgeNamespace != "" {
			middlewareMetadata["edge_namespace"] = edgeNamespace
		}

		pluginCtx := &interfaces.PluginContext{
			RequestID:    requestID, // Use canonical request ID from context
			LLMID:        llmID,
			LLMSlug:      llmSlug,
			Vendor:       vendor,
			AppID:        appID,
			UserID:       userID,
			Metadata:     middlewareMetadata,
			TraceContext: make(map[string]string),
		}

		// Set request ID header for tracking
		c.Header("X-Request-ID", pluginCtx.RequestID)

		// Execute pre-auth plugins
		if pluginResp := executePreAuthPlugins(config.PluginManager, llmID, c, pluginCtx); pluginResp != nil {
			return // Request was blocked by plugin
		}

		// Store plugin context for post-processing
		c.Set("plugin_context", pluginCtx)

		// Wrap the response writer to capture response for post-processing
		responseWriter := &pluginResponseWriter{
			ResponseWriter: c.Writer,
			statusCode:     200,
			body:          &bytes.Buffer{},
			headers:       make(http.Header),
			pluginCtx:     pluginCtx,
			pluginManager: config.PluginManager,
			llmID:         llmID,
		}
		c.Writer = responseWriter

		// Continue to next middleware/handler
		c.Next()
	}
}

// executePreAuthPlugins executes pre-authentication plugins
func executePreAuthPlugins(manager *PluginManager, llmID uint, c *gin.Context, pluginCtx *interfaces.PluginContext) *interfaces.PluginResponse {
	// Read request body for processing
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request.Body)
		// Restore body for subsequent middleware
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Create plugin request
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0] // Take first value
		}
	}

	pluginReq := &interfaces.PluginRequest{
		Method:     c.Request.Method,
		Path:       c.Request.URL.Path,
		Headers:    headers,
		Body:       bodyBytes,
		RemoteAddr: c.ClientIP(),
		Context:    pluginCtx,
	}

	// Execute pre-auth plugin chain
	result, err := manager.ExecutePluginChain(llmID, interfaces.HookTypePreAuth, pluginReq, pluginCtx)
	if err != nil {
		log.Error().Err(err).Msg("Pre-auth plugin chain failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Plugin execution failed",
			"message": "Pre-authentication plugin error",
		})
		c.Abort()
		return &interfaces.PluginResponse{Block: true}
	}

	// Check if any plugin wants to block the request
	if pluginResp, ok := result.(*interfaces.PluginResponse); ok {
		if pluginResp.Block {
			// Plugin blocked the request
			c.Status(pluginResp.StatusCode)
			
			// Set headers from plugin
			for key, value := range pluginResp.Headers {
				c.Header(key, value)
			}
			
			if len(pluginResp.Body) > 0 {
				c.Data(pluginResp.StatusCode, "application/json", pluginResp.Body)
			}
			
			c.Abort()
			return pluginResp
		}
		
		// Apply plugin modifications to request
		if pluginResp.Modified {
			applyRequestModifications(c, pluginResp)
		}
	}

	return nil
}

// applyRequestModifications applies plugin modifications to the request
func applyRequestModifications(c *gin.Context, resp *interfaces.PluginResponse) {
	// Apply header modifications
	if resp.Headers != nil {
		for key, value := range resp.Headers {
			c.Request.Header.Set(key, value)
		}
	}
	
	// Apply body modifications
	if resp.Body != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(resp.Body))
		c.Request.ContentLength = int64(len(resp.Body))
	}
}

// pluginResponseWriter wraps the response writer to capture response for plugin processing
type pluginResponseWriter struct {
	gin.ResponseWriter
	statusCode    int
	body         *bytes.Buffer
	headers      http.Header
	pluginCtx    *interfaces.PluginContext
	pluginManager *PluginManager
	llmID        uint
}

func (w *pluginResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *pluginResponseWriter) Write(data []byte) (int, error) {
	// Write to both the actual response and our buffer
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *pluginResponseWriter) Header() http.Header {
	// Capture headers in our buffer
	for key, values := range w.ResponseWriter.Header() {
		w.headers[key] = values
	}
	return w.ResponseWriter.Header()
}

// CloseNotify implements http.CloseNotifier for compatibility
func (w *pluginResponseWriter) CloseNotify() <-chan bool {
	if cn, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	// Fallback: return a closed channel
	closed := make(chan bool)
	close(closed)
	return closed
}

// executeResponsePlugins executes response plugins after the request completes
func (w *pluginResponseWriter) executeResponsePlugins() {
	if w.pluginManager == nil {
		return
	}

	// Create response data
	respHeaders := make(map[string]string)
	for key, values := range w.headers {
		if len(values) > 0 {
			respHeaders[key] = values[0]
		}
	}

	respData := &interfaces.ResponseData{
		RequestID:  w.pluginCtx.RequestID,
		StatusCode: w.statusCode,
		Headers:    respHeaders,
		Body:       w.body.Bytes(),
		Context:    w.pluginCtx,
		LatencyMs:  0, // Would need to be calculated from request start time
	}

	// Execute response plugin chain
	result, err := w.pluginManager.ExecutePluginChain(w.llmID, interfaces.HookTypeOnResponse, respData, w.pluginCtx)
	if err != nil {
		log.Error().Err(err).Msg("Response plugin chain failed")
		return
	}

	// Apply plugin modifications to response (if any)
	if modifiedResp, ok := result.(*interfaces.ResponseData); ok {
		// Note: At this point, the response has already been written
		// For future enhancement, we'd need to buffer the entire response
		// and apply modifications before writing to the client
		log.Debug().
			Str("request_id", w.pluginCtx.RequestID).
			Int("original_status", w.statusCode).
			Int("modified_status", modifiedResp.StatusCode).
			Msg("Response plugin processing completed")
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

func generateRequestID() string {
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}