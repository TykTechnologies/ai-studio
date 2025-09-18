// internal/api/plugin_adapter.go
package api

import (
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/rs/zerolog/log"
)

// PluginManagerAdapter adapts the plugins.PluginManager to work with the API middleware
// This avoids circular imports between api and plugins packages
type PluginManagerAdapter struct {
	manager interface {
		ExecutePluginChain(llmID uint, hookType interfaces.HookType, input interface{}, pluginCtx *interfaces.PluginContext) (interface{}, error)
		GetPluginsForLLM(llmID uint, hookType interfaces.HookType) ([]*plugins.LoadedPlugin, error)
		IsPluginLoaded(pluginID uint) bool
		RefreshLLMPluginMapping(llmID uint) error
	}
}

// NewPluginManagerAdapter creates a new plugin manager adapter
func NewPluginManagerAdapter(manager interface{}) PluginManagerInterface {
	if pm, ok := manager.(interface {
		ExecutePluginChain(llmID uint, hookType interfaces.HookType, input interface{}, pluginCtx *interfaces.PluginContext) (interface{}, error)
		GetPluginsForLLM(llmID uint, hookType interfaces.HookType) ([]*plugins.LoadedPlugin, error)
		IsPluginLoaded(pluginID uint) bool
		RefreshLLMPluginMapping(llmID uint) error
	}); ok {
		return &PluginManagerAdapter{manager: pm}
	}
	log.Fatal().Interface("manager_type", manager).Msg("FATAL: Plugin manager type assertion failed - interface mismatch")
	return nil // Never reached
}

// ExecutePluginChain adapts the method signature
func (a *PluginManagerAdapter) ExecutePluginChain(llmID uint, hookType string, input interface{}, pluginCtx interface{}) (interface{}, error) {
	// Convert string hookType to interfaces.HookType
	var ht interfaces.HookType
	switch hookType {
	case "pre_auth":
		ht = interfaces.HookTypePreAuth
	case "auth":
		ht = interfaces.HookTypeAuth
	case "post_auth":
		ht = interfaces.HookTypePostAuth
	case "on_response":
		ht = interfaces.HookTypeOnResponse
	default:
		return nil, nil // Unknown hook type, skip
	}

	// Convert pluginCtx map to *interfaces.PluginContext
	ctx := convertMapToPluginContext(pluginCtx)

	// Convert input map to appropriate interface type
	convertedInput := convertMapToPluginRequest(input, hookType)

	result, err := a.manager.ExecutePluginChain(llmID, ht, convertedInput, ctx)
	if err != nil {
		return nil, err
	}

	// Convert the result back to map format expected by middleware
	return convertPluginResultToMap(result, hookType), nil
}

// GetPluginsForLLM adapts the method signature
func (a *PluginManagerAdapter) GetPluginsForLLM(llmID uint, hookType string) (interface{}, error) {
	// Convert string hookType to interfaces.HookType
	var ht interfaces.HookType
	switch hookType {
	case "pre_auth":
		ht = interfaces.HookTypePreAuth
	case "auth":
		ht = interfaces.HookTypeAuth
	case "post_auth":
		ht = interfaces.HookTypePostAuth
	case "on_response":
		ht = interfaces.HookTypeOnResponse
	default:
		return nil, nil // Unknown hook type
	}

	return a.manager.GetPluginsForLLM(llmID, ht)
}

// IsPluginLoaded passes through
func (a *PluginManagerAdapter) IsPluginLoaded(pluginID uint) bool {
	return a.manager.IsPluginLoaded(pluginID)
}

// RefreshLLMPluginMapping passes through
func (a *PluginManagerAdapter) RefreshLLMPluginMapping(llmID uint) error {
	return a.manager.RefreshLLMPluginMapping(llmID)
}

// Helper conversion functions

func convertMapToPluginContext(ctx interface{}) *interfaces.PluginContext {
	if ctxMap, ok := ctx.(map[string]interface{}); ok {
		pluginCtx := &interfaces.PluginContext{}

		if requestID, ok := ctxMap["request_id"].(string); ok {
			pluginCtx.RequestID = requestID
		}
		if llmID, ok := ctxMap["llm_id"].(uint); ok {
			pluginCtx.LLMID = llmID
		}
		if llmSlug, ok := ctxMap["llm_slug"].(string); ok {
			pluginCtx.LLMSlug = llmSlug
		}
		if vendor, ok := ctxMap["vendor"].(string); ok {
			pluginCtx.Vendor = vendor
		}
		if appID, ok := ctxMap["app_id"].(uint); ok {
			pluginCtx.AppID = appID
		}
		if userID, ok := ctxMap["user_id"].(uint); ok {
			pluginCtx.UserID = userID
		}
		if metadata, ok := ctxMap["metadata"].(map[string]interface{}); ok {
			pluginCtx.Metadata = metadata
		}
		if traceContext, ok := ctxMap["trace_context"].(map[string]string); ok {
			pluginCtx.TraceContext = traceContext
		}

		return pluginCtx
	}
	return &interfaces.PluginContext{}
}

// convertPluginResultToMap converts plugin manager results back to map format expected by middleware
func convertPluginResultToMap(result interface{}, hookType string) map[string]interface{} {
	switch hookType {
	case "pre_auth":
		// For pre-auth plugins, check if a plugin response was returned (indicating modification)
		if pluginResp, ok := result.(*interfaces.PluginResponse); ok && pluginResp != nil {
			return map[string]interface{}{
				"modified":     pluginResp.Modified,
				"block":        pluginResp.Block,
				"status_code":  pluginResp.StatusCode,
				"headers":      pluginResp.Headers,
				"body":         pluginResp.Body,
				"error":        pluginResp.ErrorMessage,
			}
		}
		// Also check for modified plugin request (if plugin returns modified request instead of response)
		if pluginReq, ok := result.(*interfaces.PluginRequest); ok && pluginReq != nil {
			return map[string]interface{}{
				"modified":    true,
				"method":      pluginReq.Method,
				"path":        pluginReq.Path,
				"headers":     pluginReq.Headers,
				"body":        pluginReq.Body,
				"remote_addr": pluginReq.RemoteAddr,
				"block":       false,
			}
		}
		
	case "post_auth":
		// For post-auth plugins, check if a plugin response was returned (indicating modification)
		if pluginResp, ok := result.(*interfaces.PluginResponse); ok && pluginResp != nil {
			return map[string]interface{}{
				"modified":     pluginResp.Modified,
				"block":        pluginResp.Block,
				"status_code":  pluginResp.StatusCode,
				"headers":      pluginResp.Headers,
				"body":         pluginResp.Body,
				"error":        pluginResp.ErrorMessage,
			}
		}
		// Also check for enriched request (if plugin returns modified request instead of response)
		if enrichedReq, ok := result.(*interfaces.EnrichedRequest); ok && enrichedReq != nil && enrichedReq.PluginRequest != nil {
			return map[string]interface{}{
				"modified":    true,
				"method":      enrichedReq.PluginRequest.Method,
				"path":        enrichedReq.PluginRequest.Path,
				"headers":     enrichedReq.PluginRequest.Headers,
				"body":        enrichedReq.PluginRequest.Body,
				"remote_addr": enrichedReq.PluginRequest.RemoteAddr,
				"block":       false,
			}
		}
		// If input is returned unchanged, no modification occurred
		return map[string]interface{}{
			"modified": false,
			"block":    false,
		}
		
	case "auth":
		// For auth plugins, return authentication result
		if authResp, ok := result.(*interfaces.AuthResponse); ok {
			return map[string]interface{}{
				"authenticated": authResp.Authenticated,
				"user_id":       authResp.UserID,
				"app_id":        authResp.AppID,
				"claims":        authResp.Claims,
				"error":         authResp.ErrorMessage,
			}
		}
		
	case "on_response":
		// For response plugins, check if response was modified
		if respData, ok := result.(*interfaces.ResponseData); ok {
			return map[string]interface{}{
				"modified":     true,
				"request_id":   respData.RequestID,
				"status_code":  respData.StatusCode,
				"headers":      respData.Headers,
				"body":         respData.Body,
				"latency_ms":   respData.LatencyMs,
			}
		}
	}
	
	// Default case - no modification
	return map[string]interface{}{
		"modified": false,
		"block":    false,
	}
}

func convertMapToPluginRequest(input interface{}, hookType string) interface{} {
	log.Debug().Str("hook_type", hookType).Interface("input", input).Msg("convertMapToPluginRequest called")
	
	if inputMap, ok := input.(map[string]interface{}); ok {
		log.Debug().Str("hook_type", hookType).Msg("Input is a map, proceeding with conversion")
		switch hookType {
		case "pre_auth":
			// For pre-auth plugins, extract fields directly
			req := &interfaces.PluginRequest{}
			if method, ok := inputMap["method"].(string); ok {
				req.Method = method
			}
			if path, ok := inputMap["path"].(string); ok {
				req.Path = path
			}
			if headers, ok := inputMap["headers"].(map[string]string); ok {
				req.Headers = headers
			}
			if body, ok := inputMap["body"].([]byte); ok {
				req.Body = body
				log.Debug().Int("body_len", len(body)).Msg("Pre-auth: Successfully extracted body from input")
			}
			if remoteAddr, ok := inputMap["remote_addr"].(string); ok {
				req.RemoteAddr = remoteAddr
			}
			if ctx, ok := inputMap["context"].(map[string]interface{}); ok {
				req.Context = convertMapToPluginContext(ctx)
			}
			return req
			
		case "post_auth":
			// For post-auth plugins, extract the nested request data and preserve auth context
			enriched := &interfaces.EnrichedRequest{}
			
			// Extract auth context from top level
			if userID, ok := inputMap["user_id"].(string); ok {
				enriched.UserID = userID
			}
			if appID, ok := inputMap["app_id"].(string); ok {
				enriched.AppID = appID
			}
			if authenticated, ok := inputMap["authenticated"].(bool); ok {
				enriched.Authenticated = authenticated
			}
			if claims, ok := inputMap["auth_claims"].(map[string]string); ok {
				enriched.AuthClaims = claims
			}
			
			// Extract the nested request data
			if requestData, ok := inputMap["request"].(map[string]interface{}); ok {
				log.Debug().Interface("nested_request", requestData).Msg("Post-auth: Extracting nested request data")
				req := &interfaces.PluginRequest{}
				if method, ok := requestData["method"].(string); ok {
					req.Method = method
				}
				if path, ok := requestData["path"].(string); ok {
					req.Path = path
				}
				if headers, ok := requestData["headers"].(map[string]string); ok {
					req.Headers = headers
				}
				if body, ok := requestData["body"].([]byte); ok {
					req.Body = body
					log.Debug().Int("body_len", len(body)).Msg("Post-auth: Successfully extracted body from nested request")
				} else {
					log.Error().Interface("body_value", requestData["body"]).Msg("Post-auth: Failed to extract body from nested request")
				}
				if remoteAddr, ok := requestData["remote_addr"].(string); ok {
					req.RemoteAddr = remoteAddr
				}
				if ctx, ok := requestData["context"].(map[string]interface{}); ok {
					req.Context = convertMapToPluginContext(ctx)
				}
				
				enriched.PluginRequest = req
			} else {
				log.Fatal().Interface("request_value", inputMap["request"]).Msg("FATAL: Post-auth plugin missing nested request data")
			}
			
			log.Debug().Interface("enriched_request", enriched).Int("body_len", len(enriched.PluginRequest.Body)).Msg("Post-auth: EnrichedRequest created with full context")
			return enriched

		case "auth":
			// For auth plugins, create AuthRequest structure
			authReq := &interfaces.AuthRequest{}
			if credential, ok := inputMap["credential"].(string); ok {
				authReq.Credential = credential
			}
			if authType, ok := inputMap["auth_type"].(string); ok {
				authReq.AuthType = authType
			}
			if requestData, ok := inputMap["request"].(map[string]interface{}); ok {
				// Convert nested request to PluginRequest
				pluginReq := &interfaces.PluginRequest{}
				if method, ok := requestData["method"].(string); ok {
					pluginReq.Method = method
				}
				if path, ok := requestData["path"].(string); ok {
					pluginReq.Path = path
				}
				if headers, ok := requestData["headers"].(map[string]string); ok {
					pluginReq.Headers = headers
				}
				if body, ok := requestData["body"].([]byte); ok {
					pluginReq.Body = body
					log.Debug().Int("body_len", len(body)).Msg("Auth: Successfully extracted body from nested request")
				}
				if remoteAddr, ok := requestData["remote_addr"].(string); ok {
					pluginReq.RemoteAddr = remoteAddr
				}
				if ctx, ok := requestData["context"].(map[string]interface{}); ok {
					pluginReq.Context = convertMapToPluginContext(ctx)
				}
				authReq.Request = pluginReq
			}
			log.Debug().Interface("auth_request", authReq).Str("credential", authReq.Credential).Msg("Auth: AuthRequest created with credential")
			return authReq

		case "on_response":
			resp := &interfaces.ResponseData{}
			if requestID, ok := inputMap["request_id"].(string); ok {
				resp.RequestID = requestID
			}
			if statusCode, ok := inputMap["status_code"].(int); ok {
				resp.StatusCode = statusCode
			}
			if headers, ok := inputMap["headers"].(map[string]string); ok {
				resp.Headers = headers
			}
			if body, ok := inputMap["body"].([]byte); ok {
				resp.Body = body
			}
			if latency, ok := inputMap["latency_ms"].(int64); ok {
				resp.LatencyMs = latency
			}
			if ctx, ok := inputMap["context"].(map[string]interface{}); ok {
				resp.Context = convertMapToPluginContext(ctx)
			}
			return resp

		default:
			return input
		}
	}
	return input
}