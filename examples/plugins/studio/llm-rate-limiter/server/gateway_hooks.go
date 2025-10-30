package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	gatewaySDK "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// === Gateway Post-Auth Hook Implementation ===
// The following methods enable LLMRateLimiterPlugin to work as a post_auth hook in the Microgateway

// ClaudeMessagesRequest represents the Claude v1/messages API request format
type ClaudeMessagesRequest struct {
	Model       string          `json:"model"`
	Messages    []ClaudeMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// estimateTokensFromRequest estimates token count from Claude API request
// Uses rule of thumb: ~4 characters per token for English text
func (p *LLMRateLimiterPlugin) estimateTokensFromRequest(requestBody []byte) int {
	if len(requestBody) == 0 {
		return 100 // Default fallback
	}

	var claudeReq ClaudeMessagesRequest
	if err := json.Unmarshal(requestBody, &claudeReq); err != nil {
		log.Printf("⚠️ %s: Failed to parse request body for token estimation: %v", PluginName, err)
		return 100 // Fallback estimate
	}

	totalChars := 0

	// Count characters in system prompt
	if claudeReq.System != "" {
		totalChars += len(claudeReq.System)
	}

	// Count characters in all messages
	for _, msg := range claudeReq.Messages {
		totalChars += len(msg.Content)
		// Add small overhead for role and structure (~10 chars per message)
		totalChars += 10
	}

	// Estimate tokens: ~4 characters per token (rule of thumb for English)
	estimatedInputTokens := totalChars / 4

	// Add estimated output tokens from max_tokens parameter
	estimatedOutputTokens := claudeReq.MaxTokens
	if estimatedOutputTokens == 0 {
		estimatedOutputTokens = 1024 // Default if not specified
	}

	totalEstimatedTokens := estimatedInputTokens + estimatedOutputTokens

	log.Printf("📊 %s: Token estimate - input: %d, output: %d, total: %d (from %d chars)",
		PluginName, estimatedInputTokens, estimatedOutputTokens, totalEstimatedTokens, totalChars)

	return totalEstimatedTokens
}

// ProcessPostAuth implements the post_auth hook - this is where rate limiting happens
func (p *LLMRateLimiterPlugin) ProcessPostAuth(ctx context.Context, enrichedReq *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	req := enrichedReq.Request
	pluginCtx := req.Context

	// Extract app ID and model from context
	appID := pluginCtx.AppId
	model := pluginCtx.LlmSlug

	if appID == 0 {
		log.Printf("⚠️ %s: No app ID in context - skipping rate limit", PluginName)
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Fetch app rate limit configuration from App metadata
	rateLimitConfig, err := p.getAppRateLimitConfigGateway(ctx, appID)
	if err != nil {
		log.Printf("⚠️ %s: Failed to fetch rate limit config for app %d: %v", PluginName, appID, err)
		return &pb.PluginResponse{Modified: false}, nil
	}

	// If no rate limit configured or disabled, allow request
	if rateLimitConfig == nil || !rateLimitConfig.Enabled {
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Policy data is now embedded in the rate limit config (no K/V lookup needed)
	// This works because Studio and Gateway have separate K/V stores
	if len(rateLimitConfig.Models) == 0 {
		log.Printf("⚠️ %s: No model limits in policy '%s'", PluginName, rateLimitConfig.PolicyName)
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Get limits for this model (with fallback to wildcard)
	limits, ok := rateLimitConfig.Models[model]
	if !ok {
		// Try wildcard
		limits, ok = rateLimitConfig.Models["*"]
		if !ok {
			log.Printf("⚠️ %s: No limits defined for model '%s' in policy '%s'", PluginName, model, rateLimitConfig.PolicyName)
			return &pb.PluginResponse{Modified: false}, nil
		}
	}

	// Apply overrides if any
	if override, ok := rateLimitConfig.Overrides[model]; ok {
		if override.TPM > 0 {
			limits.TPM = override.TPM
		}
		if override.RPM > 0 {
			limits.RPM = override.RPM
		}
		if override.Concurrent > 0 {
			limits.Concurrent = override.Concurrent
		}
	}

	// Estimate tokens from request body
	estimatedTokens := p.estimateTokensFromRequest(req.Body)

	// Check rate limits with actual token estimate
	allowed, limitType, currentUsage, resetTime := p.checkRateLimits(ctx, appID, model, limits, estimatedTokens)

	if !allowed {
		// Rate limit exceeded - return 429
		log.Printf("🚫 %s: Rate limit exceeded for app %d, model %s: %s (usage: %d, limit: %d)",
			PluginName, appID, model, limitType, currentUsage, p.getLimitValue(limits, limitType))

		errorResponse := map[string]interface{}{
			"error":         "Rate limit exceeded",
			"limit_type":    limitType,
			"limit_value":   p.getLimitValue(limits, limitType),
			"current_usage": currentUsage,
			"reset_at":      resetTime.Format(time.RFC3339),
			"policy":        rateLimitConfig.PolicyName,
			"app_id":        appID,
			"model":         model,
		}

		body, _ := json.Marshal(errorResponse)
		return &pb.PluginResponse{
			Block:      true,
			StatusCode: 429,
			Headers: map[string]string{
				"Content-Type":       "application/json",
				"X-RateLimit-Policy": rateLimitConfig.PolicyName,
				"X-RateLimit-Type":   limitType,
				"X-RateLimit-Reset":  resetTime.Format(time.RFC3339),
			},
			Body: body,
		}, nil
	}

	// Rate limit OK - allow request
	log.Printf("✅ %s: Rate limit check passed for app %d, model %s", PluginName, appID, model)
	return &pb.PluginResponse{Modified: false}, nil
}

// getAppRateLimitConfigGateway fetches rate limit config from App metadata (gateway context)
func (p *LLMRateLimiterPlugin) getAppRateLimitConfigGateway(ctx context.Context, appID uint32) (*AppRateLimitConfig, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("%s%d", CachePrefix, appID)
	cached, err := ai_studio_sdk.ReadPluginKV(ctx, cacheKey)
	if err == nil && len(cached) > 0 {
		var config AppRateLimitConfig
		if err := json.Unmarshal(cached, &config); err == nil {
			return &config, nil
		}
	}

	// Fetch app from service API to get metadata
	appResp, err := ai_studio_sdk.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	// Parse metadata
	if appResp.App.Metadata == "" {
		return nil, nil // No metadata
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(appResp.App.Metadata), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Extract rate_limiter config
	rateLimiterData, ok := metadata["rate_limiter"]
	if !ok {
		return nil, nil // No rate limiter config
	}

	rateLimiterJSON, _ := json.Marshal(rateLimiterData)
	var config AppRateLimitConfig
	if err := json.Unmarshal(rateLimiterJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to parse rate limiter config: %w", err)
	}

	// Cache for 5 minutes
	configJSON, _ := json.Marshal(config)
	ai_studio_sdk.WritePluginKV(ctx, cacheKey, configJSON)

	return &config, nil
}

// checkRateLimits checks if the request is within rate limits
func (p *LLMRateLimiterPlugin) checkRateLimits(ctx context.Context, appID uint32, model string, limits ModelLimits, estimatedTokens int) (allowed bool, limitType string, currentUsage int, resetTime time.Time) {
	// Get current minute key
	now := time.Now()
	minuteKey := now.Format("2006-01-02T15:04")
	resetTime = now.Truncate(time.Minute).Add(time.Minute)

	// Build rate state key
	rateKey := fmt.Sprintf("%s%d:%s:%s", RatePrefix, appID, model, minuteKey)

	// Get or create lock for this rate key
	lockKey := fmt.Sprintf("%d:%s", appID, model)
	p.mu.Lock()
	if _, exists := p.rateLocks[lockKey]; !exists {
		p.rateLocks[lockKey] = &sync.Mutex{}
	}
	rateLock := p.rateLocks[lockKey]
	p.mu.Unlock()

	// Lock for atomic rate update
	rateLock.Lock()
	defer rateLock.Unlock()

	// Read current rate state
	var state RateState
	stateData, err := ai_studio_sdk.ReadPluginKV(ctx, rateKey)
	if err == nil && len(stateData) > 0 {
		json.Unmarshal(stateData, &state)
	} else {
		// Initialize new state
		state = RateState{
			MinuteKey:  minuteKey,
			Tokens:     0,
			Requests:   0,
			Concurrent: 0,
			UpdatedAt:  now.Unix(),
		}
	}

	// Check concurrent limit
	if limits.Concurrent > 0 && state.Concurrent >= limits.Concurrent {
		return false, "concurrent", state.Concurrent, resetTime
	}

	// Check RPM limit
	if limits.RPM > 0 && state.Requests >= limits.RPM {
		return false, "rpm", state.Requests, resetTime
	}

	// Check TPM limit
	if limits.TPM > 0 && estimatedTokens > 0 {
		if state.Tokens+estimatedTokens > limits.TPM {
			return false, "tpm", state.Tokens, resetTime
		}
	}

	// All checks passed - increment counters
	state.Requests++
	state.Concurrent++
	if estimatedTokens > 0 {
		state.Tokens += estimatedTokens
	}
	state.UpdatedAt = now.Unix()

	// Save updated state
	stateJSON, _ := json.Marshal(state)
	ai_studio_sdk.WritePluginKV(ctx, rateKey, stateJSON)

	// Note: Concurrent should be decremented after request completes
	// This would ideally happen in a response hook

	return true, "", 0, resetTime
}

// getLimitValue gets the limit value for a given limit type
func (p *LLMRateLimiterPlugin) getLimitValue(limits ModelLimits, limitType string) int {
	switch limitType {
	case "tpm":
		return limits.TPM
	case "rpm":
		return limits.RPM
	case "concurrent":
		return limits.Concurrent
	default:
		return 0
	}
}

// === Gateway SDK Interface Methods ===
// These methods allow the plugin to satisfy the gateway SDK's PostAuthPlugin interface

// Initialize implements gateway SDK BasePlugin interface
func (p *LLMRateLimiterPlugin) Initialize(config map[string]interface{}) error {
	// Gateway initialization
	if p.rateLocks == nil {
		p.rateLocks = make(map[string]*sync.Mutex)
	}
	log.Printf("%s: Gateway Initialize called", PluginName)
	return nil
}

// GetHookType implements gateway SDK BasePlugin interface
func (p *LLMRateLimiterPlugin) GetHookType() gatewaySDK.HookType {
	return gatewaySDK.HookTypePostAuth
}

// GetName implements gateway SDK BasePlugin interface
func (p *LLMRateLimiterPlugin) GetName() string {
	return PluginName
}

// GetVersion implements gateway SDK BasePlugin interface
func (p *LLMRateLimiterPlugin) GetVersion() string {
	return PluginVersion
}

// Shutdown implements gateway SDK BasePlugin interface
func (p *LLMRateLimiterPlugin) Shutdown() error {
	log.Printf("%s: Gateway Shutdown called", PluginName)
	return nil
}

// ProcessRequest implements gateway SDK PostAuthPlugin interface
// This is the method the gateway actually calls (not ProcessPostAuth)
func (p *LLMRateLimiterPlugin) ProcessRequest(ctx context.Context, req *gatewaySDK.EnrichedRequest, pluginCtx *gatewaySDK.PluginContext) (*gatewaySDK.PluginResponse, error) {
	// Convert gateway SDK types to unified proto types and call ProcessPostAuth
	pbReq := &pb.EnrichedRequest{
		Request: &pb.PluginRequest{
			Method:  req.Method,
			Path:    req.Path,
			Headers: req.Headers,
			Body:    req.Body,
			Context: &pb.PluginContext{
				RequestId:    pluginCtx.RequestID,
				Vendor:       pluginCtx.Vendor,
				LlmId:        uint32(pluginCtx.LLMID),
				LlmSlug:      pluginCtx.LLMSlug,
				AppId:        uint32(pluginCtx.AppID),
				UserId:       uint32(pluginCtx.UserID),
				Metadata:     convertMetadata(pluginCtx.Metadata),
				TraceContext: pluginCtx.TraceContext,
			},
			RemoteAddr: req.RemoteAddr,
		},
		UserId:        req.UserID,
		AppId:         req.AppID,
		AuthClaims:    req.AuthClaims,
		Authenticated: req.Authenticated,
	}

	// Call the unified proto implementation
	pbResp, err := p.ProcessPostAuth(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	// Convert response back to gateway SDK types
	return &gatewaySDK.PluginResponse{
		Modified:     pbResp.Modified,
		StatusCode:   int(pbResp.StatusCode),
		Headers:      pbResp.Headers,
		Body:         pbResp.Body,
		Block:        pbResp.Block,
		ErrorMessage: pbResp.ErrorMessage,
	}, nil
}

// convertMetadata converts map[string]interface{} to map[string]string
func convertMetadata(metadata map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range metadata {
		if str, ok := v.(string); ok {
			result[k] = str
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}
