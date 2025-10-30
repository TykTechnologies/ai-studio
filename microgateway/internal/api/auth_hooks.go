package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/rs/zerolog/log"
)

// CreateAuthHooks creates authentication lifecycle hooks for microgateway plugins
func CreateAuthHooks(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) *proxy.AuthHooks {
	return &proxy.AuthHooks{
		PreAuth:    createPreAuthHook(serviceContainer, pluginManager),
		CustomAuth: createCustomAuthHook(serviceContainer, pluginManager),
		PostAuth:   createPostAuthHook(serviceContainer, pluginManager),
	}
}

// ===================================
// PRE-AUTH HOOK
// ===================================
// Executes BEFORE authentication, NO access to user/app data

func createPreAuthHook(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		// Only process LLM requests
		if !strings.HasPrefix(r.URL.Path, "/llm/") {
			return false
		}

		// Extract LLM slug: /llm/rest/{slug}/... or /llm/stream/{slug}/...
		llmSlug := extractLLMSlugFromPath(r.URL.Path)
		if llmSlug == "" {
			return false
		}

		// Get LLM by slug
		llmInterface, err := serviceContainer.GatewayService.GetLLMBySlug(llmSlug)
		if err != nil {
			return false // Let normal flow handle 404
		}

		var llmID uint
		var vendor string
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
			vendor = llm.Vendor
		} else {
			return false
		}

		// Check for pre-auth plugins
		pluginList, err := pluginManager.GetPluginsForLLM(llmID, "pre_auth")
		if err != nil || isEmptySlice(pluginList) {
			return false // No plugins, continue
		}

		// Create plugin context (NO app_id yet - not authenticated)
		pluginCtx := &interfaces.PluginContext{
			RequestID:    generateRequestID(),
			LLMID:        llmID,
			LLMSlug:      llmSlug,
			Vendor:       vendor,
			AppID:        uint(0), // NOT authenticated yet
			UserID:       uint(0),
			Metadata:     make(map[string]interface{}),
			TraceContext: make(map[string]string),
		}

		// Create plugin request
		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}

		bodyBytes, _ := readBodyWithoutConsuming(r)

		// Create plugin request matching interfaces.PluginRequest structure
		pluginReq := &interfaces.PluginRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			Headers:    headers,
			Body:       bodyBytes,
			RemoteAddr: r.RemoteAddr,
			Context:    pluginCtx,
		}

		// Execute pre-auth plugin chain
		result, err := pluginManager.ExecutePluginChain(llmID, "pre_auth", pluginReq, pluginCtx)
		if err != nil {
			log.Error().Err(err).Msg("Pre-auth plugin chain failed")
			respondWithError(w, http.StatusInternalServerError, "Plugin execution failed", nil)
			return true
		}

		// Check if plugin blocked the request
		if pluginResp, ok := result.(*interfaces.PluginResponse); ok {
			if pluginResp.Block {
				statusCode := pluginResp.StatusCode
				if statusCode == 0 {
					statusCode = http.StatusForbidden
				}

				// Set headers from plugin
				for key, value := range pluginResp.Headers {
					w.Header().Set(key, value)
				}

				w.WriteHeader(statusCode)

				if len(pluginResp.Body) > 0 {
					w.Write(pluginResp.Body)
				}

				return true // Block request
			}

			// Apply modifications if the plugin modified the request
			if pluginResp.Modified {
				// Apply header modifications
				for key, value := range pluginResp.Headers {
					r.Header.Set(key, value)
				}
				// Apply body modifications
				if len(pluginResp.Body) > 0 {
					r.Body = io.NopCloser(bytes.NewReader(pluginResp.Body))
					r.ContentLength = int64(len(pluginResp.Body))
				}
			}
		}

		return false // Continue to auth
	}
}

// ===================================
// CUSTOM AUTH HOOK (Auth Plugins)
// ===================================
// Allows plugins to REPLACE the validation step (extraction still happens in CredentialValidator)

func createCustomAuthHook(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) func(string, *http.Request) (uint, bool, error) {
	return func(credential string, r *http.Request) (uint, bool, error) {
		// Only for LLM requests
		if !strings.HasPrefix(r.URL.Path, "/llm/") {
			return 0, false, nil // Use standard auth
		}

		llmSlug := extractLLMSlugFromPath(r.URL.Path)
		if llmSlug == "" {
			return 0, false, nil
		}

		llmInterface, err := serviceContainer.GatewayService.GetLLMBySlug(llmSlug)
		if err != nil {
			return 0, false, nil
		}

		var llmID uint
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
		} else {
			return 0, false, nil
		}

		// Check if this LLM has auth plugins
		authPlugins, err := pluginManager.GetPluginsForLLM(llmID, "auth")
		if err != nil || isEmptySlice(authPlugins) {
			return 0, false, nil // No auth plugins, use standard validation
		}

		// Create plugin context for auth
		pluginCtx := &interfaces.PluginContext{
			RequestID:    generateRequestID(),
			LLMID:        llmID,
			LLMSlug:      llmSlug,
			Metadata:     make(map[string]interface{}),
			TraceContext: make(map[string]string),
		}

		// Create auth request
		authReq := map[string]interface{}{
			"credential": credential,
			"auth_type":  "bearer",
			"context":    pluginCtx,
		}

		// Execute auth plugin chain
		result, err := pluginManager.ExecutePluginChain(llmID, "auth", authReq, pluginCtx)
		if err != nil {
			log.Error().Err(err).Msg("Auth plugin chain failed")
			return 0, false, err
		}

		// Parse plugin response
		if authResult, ok := result.(map[string]interface{}); ok {
			if authenticated, hasAuth := authResult["authenticated"].(bool); hasAuth {
				if !authenticated {
					log.Debug().Msg("Auth plugin rejected authentication")
					return 0, false, nil // Auth plugin rejected
				}

				// Extract app_id from plugin response
				var appID uint
				if appIDVal, ok := authResult["app_id"].(float64); ok {
					appID = uint(appIDVal)
				} else if appIDStr, ok := authResult["app_id"].(string); ok {
					if id, err := strconv.ParseUint(appIDStr, 10, 32); err == nil {
						appID = uint(id)
					}
				} else if appIDUint, ok := authResult["app_id"].(uint); ok {
					appID = appIDUint
				}

				if appID == 0 {
					appID = 1 // Default fallback
				}

				log.Debug().Uint("app_id", appID).Msg("Auth plugin authenticated request")
				return appID, true, nil
			}
		}

		return 0, false, nil // Invalid plugin response format
	}
}

// ===================================
// POST-AUTH HOOK
// ===================================
// Executes AFTER successful authentication, HAS access to authenticated user/app data

func createPostAuthHook(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) func(http.ResponseWriter, *http.Request, uint) bool {
	return func(w http.ResponseWriter, r *http.Request, appID uint) bool {
		// Only for LLM requests
		if !strings.HasPrefix(r.URL.Path, "/llm/") {
			return false
		}

		llmSlug := extractLLMSlugFromPath(r.URL.Path)
		if llmSlug == "" {
			return false
		}

		llmInterface, err := serviceContainer.GatewayService.GetLLMBySlug(llmSlug)
		if err != nil {
			return false
		}

		var llmID uint
		var vendor string
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
			vendor = llm.Vendor
		} else {
			return false
		}

		// Check for post-auth plugins
		pluginList, err := pluginManager.GetPluginsForLLM(llmID, "post_auth")
		if err != nil || isEmptySlice(pluginList) {
			return false // No plugins, continue
		}

		// Create plugin context (NOW with authenticated app_id)
		pluginCtx := &interfaces.PluginContext{
			RequestID:    generateRequestID(),
			LLMID:        llmID,
			LLMSlug:      llmSlug,
			Vendor:       vendor,
			AppID:        appID, // AUTHENTICATED app_id available!
			UserID:       uint(0),
			Metadata:     make(map[string]interface{}),
			TraceContext: make(map[string]string),
		}

		// Create enriched request
		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}

		bodyBytes, _ := readBodyWithoutConsuming(r)

		// Create enriched request matching interfaces.EnrichedRequest structure
		enrichedReq := &interfaces.EnrichedRequest{
			PluginRequest: &interfaces.PluginRequest{
				Method:     r.Method,
				Path:       r.URL.Path,
				Headers:    headers,
				Body:       bodyBytes,
				RemoteAddr: r.RemoteAddr,
				Context:    pluginCtx,
			},
			UserID:        "plugin-user",                          // String as per interface
			AppID:         strconv.FormatUint(uint64(appID), 10), // String as per interface
			AuthClaims:    make(map[string]string),
			Authenticated: true,
		}

		// Execute post-auth plugin chain
		result, err := pluginManager.ExecutePluginChain(llmID, "post_auth", enrichedReq, pluginCtx)
		if err != nil {
			log.Error().Err(err).Msg("Post-auth plugin chain failed")
			respondWithError(w, http.StatusInternalServerError, "Plugin execution failed", nil)
			return true
		}

		// Check if plugin blocked
		if pluginResp, ok := result.(*interfaces.PluginResponse); ok {
			if pluginResp.Block {
				statusCode := pluginResp.StatusCode
				if statusCode == 0 {
					statusCode = http.StatusForbidden
				}

				// Set headers from plugin
				for key, value := range pluginResp.Headers {
					w.Header().Set(key, value)
				}

				w.WriteHeader(statusCode)

				if len(pluginResp.Body) > 0 {
					w.Write(pluginResp.Body)
				}

				return true // Block request
			}

			// Apply modifications if the plugin modified the request
			if pluginResp.Modified {
				// Apply header modifications
				for key, value := range pluginResp.Headers {
					r.Header.Set(key, value)
				}
				// Apply body modifications
				if len(pluginResp.Body) > 0 {
					r.Body = io.NopCloser(bytes.NewReader(pluginResp.Body))
					r.ContentLength = int64(len(pluginResp.Body))
				}
			}
		}
		return false // Continue to proxy
	}
}

// ===================================
// HELPER FUNCTIONS
// ===================================

func extractLLMSlugFromPath(path string) string {
	// /llm/{mode}/{slug}/...
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "llm" {
		return parts[2]
	}
	return ""
}

func isEmptySlice(v interface{}) bool {
	if v == nil {
		return true
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice {
		return rv.Len() == 0
	}
	return false
}

func generateRequestID() string {
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func readBodyWithoutConsuming(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func respondWithError(w http.ResponseWriter, statusCode int, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := map[string]string{"error": message}
	if err != nil {
		resp["details"] = err.Error()
	}
	json.NewEncoder(w).Encode(resp)
}
