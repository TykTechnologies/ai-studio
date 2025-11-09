package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	studiomgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	gwmgmt "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
)

// Embed UI assets and manifest into the binary
//
//go:embed ui assets manifest.json config.schema.json
var embeddedAssets embed.FS

//go:embed manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

const (
	PluginName    = "llm-rate-limiter"
	PluginVersion = "2.0.0"

	// K/V key prefixes
	PolicyPrefix = "policy:"
	CachePrefix  = "cache:app:"
	RatePrefix   = "rate:"
)

// Request types for token estimation
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeMessagesRequest struct {
	Model       string          `json:"model"`
	Messages    []ClaudeMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

// ModelLimits defines rate limits for a specific model
type ModelLimits struct {
	TPM        int `json:"tpm"`        // Tokens per minute
	RPM        int `json:"rpm"`        // Requests per minute
	Concurrent int `json:"concurrent"` // Max concurrent requests
}

// RateLimitPolicy defines a reusable rate limit configuration
type RateLimitPolicy struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Models      map[string]ModelLimits `json:"models"` // model name -> limits, "*" for default
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// AppRateLimitConfig is stored in App.Metadata under "rate_limiter" key
type AppRateLimitConfig struct {
	PolicyName  string                 `json:"policy_name"`
	Enabled     bool                   `json:"enabled"`
	Models      map[string]ModelLimits `json:"models"`              // Full policy data embedded
	Description string                 `json:"description,omitempty"`
	Overrides   map[string]ModelLimits `json:"overrides,omitempty"` // Per-model overrides
}

// RateState tracks current usage for rate limiting
type RateState struct {
	MinuteKey       string `json:"minute_key"` // YYYY-MM-DDTHH:mm format
	TokenCount      int    `json:"token_count"`      // Actual tokens used (from responses)
	RequestCount    int    `json:"request_count"`    // Number of requests
	ConcurrentCount int    `json:"concurrent_count"` // Active concurrent requests
	EstimatedTokens int    `json:"estimated_tokens"` // For debugging/comparison
	UpdatedAt       int64  `json:"updated_at"`
}

// TokenUsage represents extracted token usage from LLM responses
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheReadTokens  int // Anthropic prompt caching
	CacheWriteTokens int
}

// LLMRateLimiterPlugin implements the unified plugin SDK interfaces
type LLMRateLimiterPlugin struct {
	plugin_sdk.BasePlugin
	mu        sync.RWMutex
	rateLocks map[string]*sync.Mutex
}

// NewLLMRateLimiterPlugin creates a new rate limiter plugin
func NewLLMRateLimiterPlugin() *LLMRateLimiterPlugin {
	return &LLMRateLimiterPlugin{
		BasePlugin: plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "LLM Rate Limiter with policy management"),
		rateLocks:  make(map[string]*sync.Mutex),
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *LLMRateLimiterPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized in %s runtime", PluginName, ctx.Runtime)

	// Extract broker ID from config and set it for service API access
	// Note: This is also done in serve.go, but we do it here too for explicit clarity
	brokerIDStr := ""
	if id, ok := config["_service_broker_id"]; ok {
		brokerIDStr = id
	} else if id, ok := config["service_broker_id"]; ok {
		brokerIDStr = id
	}

	if brokerIDStr != "" {
		var brokerID uint32
		if _, err := fmt.Sscanf(brokerIDStr, "%d", &brokerID); err == nil {
			ai_studio_sdk.SetServiceBrokerID(brokerID)
			log.Printf("%s: Set service broker ID: %d", PluginName, brokerID)
		}
	}

	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *LLMRateLimiterPlugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("%s: Shutting down", PluginName)
	return nil
}

// HandlePostAuth implements plugin_sdk.PostAuthHandler
// POST-AUTH PHASE: Soft checks with estimates, hard checks for RPM/Concurrent
func (p *LLMRateLimiterPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	pluginReq := req.Request
	pluginCtx := pluginReq.Context

	appID := pluginCtx.AppId
	model := pluginCtx.LlmSlug
	vendor := pluginCtx.Vendor

	if appID == 0 {
		log.Printf("⚠️ %s: No app ID in context - skipping rate limit", PluginName)
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Fetch app rate limit configuration
	rateLimitConfig, err := p.getAppRateLimitConfig(ctx, appID)
	if err != nil {
		log.Printf("⚠️ %s: Failed to fetch rate limit config for app %d: %v", PluginName, appID, err)
		return &pb.PluginResponse{Modified: false}, nil
	}

	// If no rate limit configured or disabled, allow request
	if rateLimitConfig == nil || !rateLimitConfig.Enabled {
		return &pb.PluginResponse{Modified: false}, nil
	}

	// Get limits for this model (with fallback to wildcard)
	limits, ok := rateLimitConfig.Models[model]
	if !ok {
		limits, ok = rateLimitConfig.Models["*"]
		if !ok {
			return &pb.PluginResponse{Modified: false}, nil
		}
	}

	// Apply per-model overrides if configured
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

	// Get current minute for rate limiting
	now := time.Now()
	minuteKey := now.Format("2006-01-02T15:04")
	resetTime := now.Truncate(time.Minute).Add(time.Minute)
	requestID := pluginCtx.RequestId

	// Build rate state key (minute-based for aggregation)
	rateKey := fmt.Sprintf("%s%d:%s:%s", RatePrefix, appID, model, minuteKey)

	// Build request-specific state key (links post-auth and response phases)
	reqStateKey := fmt.Sprintf("rate-req:%s", requestID)

	// Get or create lock for atomic updates
	lockKey := fmt.Sprintf("%d:%s", appID, model)
	p.mu.Lock()
	if _, exists := p.rateLocks[lockKey]; !exists {
		p.rateLocks[lockKey] = &sync.Mutex{}
	}
	rateLock := p.rateLocks[lockKey]
	p.mu.Unlock()

	// Acquire lock for atomic rate check and update
	rateLock.Lock()
	defer rateLock.Unlock()

	// Read current rate state
	var state RateState
	stateData, err := ctx.Services.KV().Read(ctx, rateKey)
	if err == nil && len(stateData) > 0 {
		json.Unmarshal(stateData, &state)
		log.Printf("📖 %s: [POST-AUTH] Read existing state from KV (key=%s): TPM=%d, RPM=%d, Concurrent=%d",
			PluginName, rateKey, state.TokenCount, state.RequestCount, state.ConcurrentCount)
	} else {
		// Initialize new state for this minute
		log.Printf("🆕 %s: [POST-AUTH] Creating new state (key=%s) - first request in this minute", PluginName, rateKey)
		state = RateState{
			MinuteKey:       minuteKey,
			TokenCount:      0,
			RequestCount:    0,
			ConcurrentCount: 0,
			EstimatedTokens: 0,
			UpdatedAt:       now.Unix(),
		}
	}

	// HARD LIMIT: Check concurrent requests
	if limits.Concurrent > 0 && state.ConcurrentCount >= limits.Concurrent {
		log.Printf("🚫 %s: Concurrent limit exceeded for app %d, model %s: %d/%d",
			PluginName, appID, model, state.ConcurrentCount, limits.Concurrent)
		return p.buildRateLimitResponse(rateLimitConfig.PolicyName, "concurrent",
			state.ConcurrentCount, limits.Concurrent, resetTime, appID, model), nil
	}

	// HARD LIMIT: Check RPM
	if limits.RPM > 0 && state.RequestCount >= limits.RPM {
		log.Printf("🚫 %s: RPM limit exceeded for app %d, model %s: %d/%d",
			PluginName, appID, model, state.RequestCount, limits.RPM)
		return p.buildRateLimitResponse(rateLimitConfig.PolicyName, "rpm",
			state.RequestCount, limits.RPM, resetTime, appID, model), nil
	}

	// HARD LIMIT: Check if current TPM (from previous requests) already exceeded
	// This prevents new requests when we've already used up the quota
	if limits.TPM > 0 && state.TokenCount >= limits.TPM {
		log.Printf("🚫 %s: TPM limit already exceeded for app %d, model %s: current=%d, limit=%d",
			PluginName, appID, model, state.TokenCount, limits.TPM)
		return p.buildRateLimitResponse(rateLimitConfig.PolicyName, "tpm",
			state.TokenCount, limits.TPM, resetTime, appID, model), nil
	}

	// SOFT CHECK: Estimate TPM for this request and warn if it would exceed
	// We allow it through because estimates are inaccurate, but log for visibility
	estimatedTokens := p.estimateTokensFromRequest(pluginReq.Body, vendor)
	if limits.TPM > 0 && estimatedTokens > 0 {
		projectedTPM := state.TokenCount + estimatedTokens
		if projectedTPM > limits.TPM {
			log.Printf("⚠️ %s: This request's estimate would exceed TPM limit for app %d, model %s: projected=%d, limit=%d (allowing request, will track actual usage)",
				PluginName, appID, model, projectedTPM, limits.TPM)
			// Don't block on estimates - actual enforcement happens after response
		}
	}

	// Increment counters for this request
	state.RequestCount++
	state.ConcurrentCount++
	state.EstimatedTokens += estimatedTokens
	state.UpdatedAt = now.Unix()

	// Save updated minute-based state with 2-minute TTL (survives 1 full minute + buffer)
	stateJSON, _ := json.Marshal(state)
	ctx.Services.KV().WriteWithTTL(ctx, rateKey, stateJSON, 2*time.Minute)

	// Save request-specific state for response phase to find with 5-minute TTL
	// This links post-auth and response phases across minute boundaries
	reqState := map[string]interface{}{
		"minute_key": minuteKey,
		"app_id":     appID,
		"model":      model,
		"rate_key":   rateKey, // The minute-based key to update
		"timestamp":  now.Unix(),
	}
	reqStateJSON, _ := json.Marshal(reqState)
	created, err := ctx.Services.KV().WriteWithTTL(ctx, reqStateKey, reqStateJSON, 5*time.Minute)
	if err != nil {
		log.Printf("❌ %s: [POST-AUTH] Failed to save request state to KV (key=%s): %v", PluginName, reqStateKey, err)
	} else {
		log.Printf("💾 %s: [POST-AUTH] Saved request state to KV (key=%s, created=%v, size=%d bytes)",
			PluginName, reqStateKey, created, len(reqStateJSON))
	}

	log.Printf("💾 %s: [POST-AUTH] Saved minute state to KV (minute_key=%s): TPM=%d, RPM=%d, Concurrent=%d",
		PluginName, rateKey, state.TokenCount, state.RequestCount, state.ConcurrentCount)

	log.Printf("✅ %s: Rate limit check passed for app %d, model %s (RPM: %d/%d, Concurrent: %d/%d, Est.TPM: %d/%d)",
		PluginName, appID, model,
		state.RequestCount, limits.RPM,
		state.ConcurrentCount, limits.Concurrent,
		state.TokenCount+estimatedTokens, limits.TPM)

	return &pb.PluginResponse{Modified: false}, nil
}

// GetAsset implements plugin_sdk.UIProvider
func (p *LLMRateLimiterPlugin) GetAsset(assetPath string) ([]byte, string, error) {
	if strings.HasPrefix(assetPath, "/") {
		assetPath = strings.TrimPrefix(assetPath, "/")
	}

	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		return nil, "", fmt.Errorf("asset not found: %s", assetPath)
	}

	mimeType := "application/octet-stream"
	if strings.HasSuffix(assetPath, ".js") {
		mimeType = "application/javascript"
	} else if strings.HasSuffix(assetPath, ".css") {
		mimeType = "text/css"
	} else if strings.HasSuffix(assetPath, ".svg") {
		mimeType = "image/svg+xml"
	} else if strings.HasSuffix(assetPath, ".json") {
		mimeType = "application/json"
	}

	return content, mimeType, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *LLMRateLimiterPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{}, nil
}

// GetManifest implements plugin_sdk.UIProvider
func (p *LLMRateLimiterPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// HandleRPC implements plugin_sdk.UIProvider
func (p *LLMRateLimiterPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	log.Printf("%s: RPC Call - method: %s", PluginName, method)

	// Extract broker ID from payload
	if brokerID := ai_studio_sdk.ExtractBrokerIDFromPayload(payload); brokerID != 0 {
		ai_studio_sdk.SetServiceBrokerID(brokerID)
	}

	var result interface{}
	var err error

	switch method {
	case "listPolicies":
		result, err = p.rpcListPolicies(payload)
	case "getPolicy":
		result, err = p.rpcGetPolicy(payload)
	case "createPolicy":
		result, err = p.rpcCreatePolicy(payload)
	case "updatePolicy":
		result, err = p.rpcUpdatePolicy(payload)
	case "deletePolicy":
		result, err = p.rpcDeletePolicy(payload)
	case "listAppsWithPolicies":
		result, err = p.rpcListAppsWithPolicies(payload)
	case "assignPolicy":
		result, err = p.rpcAssignPolicy(payload)
	case "removePolicy":
		result, err = p.rpcRemovePolicy(payload)
	default:
		return nil, fmt.Errorf("unknown RPC method: %s", method)
	}

	if err != nil {
		log.Printf("%s: RPC error - method: %s, error: %v", PluginName, method, err)
		return nil, err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %v", err)
	}

	return resultJSON, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *LLMRateLimiterPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// === RESPONSE PHASE: Actual Token Tracking ===

// OnBeforeWriteHeaders implements plugin_sdk.ResponseHandler
// RESPONSE PHASE: Extract actual tokens, update state, add tracking headers
func (p *LLMRateLimiterPlugin) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	// Response phase doesn't need to do anything with headers alone
	// We'll handle everything in OnBeforeWrite where we have the body
	return &pb.HeadersResponse{
		Modified: false,
		Headers:  req.Headers,
	}, nil
}

// OnBeforeWrite implements plugin_sdk.ResponseHandler
// RESPONSE PHASE: Extract actual tokens, update TPM, decrement concurrent, add headers
func (p *LLMRateLimiterPlugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	pluginCtx := req.Context
	appID := pluginCtx.AppId
	model := pluginCtx.LlmSlug
	vendor := pluginCtx.Vendor

	if appID == 0 {
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	// Only process non-streaming chunks or final streaming response
	// For streaming, we typically get usage in the final chunk
	if req.IsStreamChunk {
		// TODO: Handle streaming - for now, skip intermediate chunks
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	// Extract actual token usage from response
	actualUsage := p.extractTokenUsage(req.Body, vendor)
	if actualUsage.TotalTokens == 0 {
		// Couldn't parse usage - log and pass through
		log.Printf("⚠️ %s: Could not extract token usage from response for app %d, model %s, vendor %s",
			PluginName, appID, model, vendor)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	log.Printf("📊 %s: Extracted actual token usage for app %d: prompt=%d, completion=%d, total=%d",
		PluginName, appID, actualUsage.PromptTokens, actualUsage.CompletionTokens, actualUsage.TotalTokens)

	// Fetch rate limit config
	rateLimitConfig, err := p.getAppRateLimitConfig(ctx, appID)
	if err != nil {
		log.Printf("⚠️ %s: Failed to fetch rate limit config in response phase for app %d: %v", PluginName, appID, err)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}
	if rateLimitConfig == nil || !rateLimitConfig.Enabled {
		log.Printf("ℹ️ %s: No rate limiting configured for app %d - skipping headers", PluginName, appID)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	log.Printf("✅ %s: Rate limit config found for app %d, policy: %s", PluginName, appID, rateLimitConfig.PolicyName)

	// Get limits
	limits, ok := rateLimitConfig.Models[model]
	if !ok {
		limits, ok = rateLimitConfig.Models["*"]
		if !ok {
			return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
		}
	}

	// Apply overrides
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

	// Use request ID to find the exact minute key from post-auth phase
	// This handles minute boundary crossings robustly
	requestID := pluginCtx.RequestId
	reqStateKey := fmt.Sprintf("rate-req:%s", requestID)

	// Read request-specific state to get the correct minute key
	log.Printf("🔍 %s: [RESPONSE] Looking for request state with key=%s", PluginName, reqStateKey)
	reqStateData, err := ctx.Services.KV().Read(ctx, reqStateKey)
	if err != nil {
		log.Printf("❌ %s: [RESPONSE] KV Read failed for key=%s: %v", PluginName, reqStateKey, err)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}
	if len(reqStateData) == 0 {
		log.Printf("⚠️ %s: [RESPONSE] Request state empty (req_id=%s, key=%s) - post-auth may have skipped or KV not persisting", PluginName, requestID, reqStateKey)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}
	log.Printf("✅ %s: [RESPONSE] Found request state (size=%d bytes)", PluginName, len(reqStateData))

	var reqState map[string]interface{}
	if err := json.Unmarshal(reqStateData, &reqState); err != nil {
		log.Printf("⚠️ %s: [RESPONSE] Failed to parse request state", PluginName)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	// Extract the minute key and rate key from post-auth
	rateKey, ok := reqState["rate_key"].(string)
	if !ok {
		log.Printf("⚠️ %s: [RESPONSE] No rate_key in request state", PluginName)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	minuteKey, ok := reqState["minute_key"].(string)
	if !ok {
		log.Printf("⚠️ %s: [RESPONSE] No minute_key in request state", PluginName)
		return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}

	now := time.Now()
	resetTime := now.Truncate(time.Minute).Add(time.Minute)

	log.Printf("🔗 %s: [RESPONSE] Found request state for req_id=%s, using minute_key=%s", PluginName, requestID, minuteKey)

	// Get lock
	lockKey := fmt.Sprintf("%d:%s", appID, model)
	p.mu.Lock()
	if _, exists := p.rateLocks[lockKey]; !exists {
		p.rateLocks[lockKey] = &sync.Mutex{}
	}
	rateLock := p.rateLocks[lockKey]
	p.mu.Unlock()

	rateLock.Lock()
	defer rateLock.Unlock()

	// Read current state from the SAME minute as post-auth
	var state RateState
	stateData, err := ctx.Services.KV().Read(ctx, rateKey)
	if err == nil && len(stateData) > 0 {
		json.Unmarshal(stateData, &state)
		log.Printf("📖 %s: [RESPONSE] Read existing state from KV: TPM=%d, RPM=%d, Concurrent=%d",
			PluginName, state.TokenCount, state.RequestCount, state.ConcurrentCount)
	} else {
		// This shouldn't happen since post-auth created it
		log.Printf("⚠️ %s: [RESPONSE] State missing for key=%s (post-auth should have created it)", PluginName, rateKey)
		state = RateState{
			MinuteKey:       minuteKey,
			TokenCount:      0,
			RequestCount:    0,
			ConcurrentCount: 0,
			UpdatedAt:       now.Unix(),
		}
	}

	// Update with ACTUAL token usage
	state.TokenCount += actualUsage.TotalTokens
	state.ConcurrentCount-- // Request completed
	if state.ConcurrentCount < 0 {
		state.ConcurrentCount = 0 // Prevent negative
	}
	state.UpdatedAt = now.Unix()

	// Save updated state with 2-minute TTL
	stateJSON, _ := json.Marshal(state)
	ctx.Services.KV().WriteWithTTL(ctx, rateKey, stateJSON, 2*time.Minute)

	// Cleanup: Delete request-specific state (no longer needed)
	ctx.Services.KV().Delete(ctx, reqStateKey)
	log.Printf("🧹 %s: [RESPONSE] Cleaned up request state for req_id=%s", PluginName, requestID)

	// Build enhanced headers with rate limit info
	modifiedHeaders := make(map[string]string)
	for k, v := range req.Headers {
		modifiedHeaders[k] = v
	}

	// Add TPM tracking headers
	if limits.TPM > 0 {
		modifiedHeaders["X-Tyk-RateLimit-TPM-Used"] = fmt.Sprintf("%d", state.TokenCount)
		modifiedHeaders["X-Tyk-RateLimit-TPM-Limit"] = fmt.Sprintf("%d", limits.TPM)
		remaining := limits.TPM - state.TokenCount
		if remaining < 0 {
			remaining = 0
		}
		modifiedHeaders["X-Tyk-RateLimit-TPM-Remaining"] = fmt.Sprintf("%d", remaining)
	}

	// Add RPM tracking headers
	if limits.RPM > 0 {
		modifiedHeaders["X-Tyk-RateLimit-RPM-Used"] = fmt.Sprintf("%d", state.RequestCount)
		modifiedHeaders["X-Tyk-RateLimit-RPM-Limit"] = fmt.Sprintf("%d", limits.RPM)
		remaining := limits.RPM - state.RequestCount
		if remaining < 0 {
			remaining = 0
		}
		modifiedHeaders["X-Tyk-RateLimit-RPM-Remaining"] = fmt.Sprintf("%d", remaining)
	}

	// Add reset time and policy name
	modifiedHeaders["X-Tyk-RateLimit-Reset"] = resetTime.Format(time.RFC3339)
	modifiedHeaders["X-Tyk-RateLimit-Policy"] = rateLimitConfig.PolicyName

	log.Printf("✅ %s: Updated rate state with actual usage for app %d (TPM: %d/%d, RPM: %d/%d, Concurrent: %d)",
		PluginName, appID, state.TokenCount, limits.TPM, state.RequestCount, limits.RPM, state.ConcurrentCount)

	log.Printf("📋 %s: Adding %d rate limit headers to response", PluginName, len(modifiedHeaders)-len(req.Headers))

	return &pb.ResponseWriteResponse{
		Modified: true, // We added headers
		Body:     req.Body,
		Headers:  modifiedHeaders,
	}, nil
}

// === Helper Methods ===

// extractTokenUsage parses actual token usage from LLM response (vendor-agnostic)
func (p *LLMRateLimiterPlugin) extractTokenUsage(responseBody []byte, vendor string) TokenUsage {
	if len(responseBody) == 0 {
		return TokenUsage{}
	}

	var response map[string]interface{}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		log.Printf("⚠️ %s: Failed to parse response body for token extraction: %v", PluginName, err)
		return TokenUsage{}
	}

	// Try to extract usage information (works for both OpenAI and Anthropic)
	usage, ok := response["usage"].(map[string]interface{})
	if !ok {
		return TokenUsage{}
	}

	tokens := TokenUsage{}

	// Anthropic uses: input_tokens, output_tokens
	// OpenAI uses: prompt_tokens, completion_tokens, total_tokens
	if pt, ok := usage["input_tokens"].(float64); ok {
		tokens.PromptTokens = int(pt)
	} else if pt, ok := usage["prompt_tokens"].(float64); ok {
		tokens.PromptTokens = int(pt)
	}

	if ct, ok := usage["output_tokens"].(float64); ok {
		tokens.CompletionTokens = int(ct)
	} else if ct, ok := usage["completion_tokens"].(float64); ok {
		tokens.CompletionTokens = int(ct)
	}

	if tt, ok := usage["total_tokens"].(float64); ok {
		tokens.TotalTokens = int(tt)
	} else {
		tokens.TotalTokens = tokens.PromptTokens + tokens.CompletionTokens
	}

	// Anthropic prompt caching tokens
	if cwt, ok := usage["cache_creation_input_tokens"].(float64); ok {
		tokens.CacheWriteTokens = int(cwt)
	}
	if crt, ok := usage["cache_read_input_tokens"].(float64); ok {
		tokens.CacheReadTokens = int(crt)
	}

	return tokens
}

// estimateTokensFromRequest estimates tokens for soft limit checking in post-auth
func (p *LLMRateLimiterPlugin) estimateTokensFromRequest(requestBody []byte, vendor string) int {
	if len(requestBody) == 0 {
		return 100 // Default fallback
	}

	var claudeReq ClaudeMessagesRequest
	if err := json.Unmarshal(requestBody, &claudeReq); err != nil {
		return 100 // Fallback estimate
	}

	totalChars := 0

	// Count system prompt characters
	if claudeReq.System != "" {
		totalChars += len(claudeReq.System)
	}

	// Count all message characters
	for _, msg := range claudeReq.Messages {
		totalChars += len(msg.Content) + 10 // +10 for structure overhead
	}

	// Estimate: ~4 characters per token
	estimatedInputTokens := totalChars / 4

	// Add estimated output from max_tokens
	estimatedOutputTokens := claudeReq.MaxTokens
	if estimatedOutputTokens == 0 {
		estimatedOutputTokens = 1024 // Default
	}

	return estimatedInputTokens + estimatedOutputTokens
}

// buildRateLimitResponse creates a 429 rate limit exceeded response
func (p *LLMRateLimiterPlugin) buildRateLimitResponse(policyName, limitType string, currentUsage, limitValue int, resetTime time.Time, appID uint32, model string) *pb.PluginResponse {
	errorResponse := map[string]interface{}{
		"error":         "Rate limit exceeded",
		"limit_type":    limitType,
		"limit_value":   limitValue,
		"current_usage": currentUsage,
		"reset_at":      resetTime.Format(time.RFC3339),
		"policy":        policyName,
		"app_id":        appID,
		"model":         model,
	}

	body, _ := json.Marshal(errorResponse)
	return &pb.PluginResponse{
		Block:      true,
		StatusCode: 429,
		Headers: map[string]string{
			"Content-Type":       "application/json",
			"X-RateLimit-Policy": policyName,
			"X-RateLimit-Type":   limitType,
			"X-RateLimit-Reset":  resetTime.Format(time.RFC3339),
			"Retry-After":        fmt.Sprintf("%d", int(time.Until(resetTime).Seconds())),
		},
		Body: body,
	}
}

// getAppRateLimitConfig fetches rate limit configuration from app metadata
func (p *LLMRateLimiterPlugin) getAppRateLimitConfig(ctx plugin_sdk.Context, appID uint32) (*AppRateLimitConfig, error) {
	// Try cache first (5 minute TTL)
	cacheKey := fmt.Sprintf("%s%d", CachePrefix, appID)
	cached, err := ctx.Services.KV().Read(ctx, cacheKey)
	if err == nil && len(cached) > 0 {
		var config AppRateLimitConfig
		if err := json.Unmarshal(cached, &config); err == nil {
			return &config, nil
		}
	}

	// Fetch from app metadata - use runtime-specific service
	// Both proto types have App.Metadata field, we just need to extract it differently
	var metadataStr string

	if ctx.Runtime == plugin_sdk.RuntimeGateway {
		// Use Gateway services - returns *gwmgmt.GetAppResponse
		resp, err := ctx.Services.Gateway().GetApp(ctx, appID)
		if err != nil {
			return nil, fmt.Errorf("failed to get app from Gateway: %w", err)
		}

		// Type assert to concrete Gateway proto type
		if gwResp, ok := resp.(*gwmgmt.GetAppResponse); ok {
			if gwResp.App != nil {
				metadataStr = gwResp.App.Metadata
				log.Printf("🔍 %s: Retrieved app metadata from Gateway (len=%d)", PluginName, len(metadataStr))
			}
		} else {
			log.Printf("⚠️ %s: Failed to type assert Gateway response to *gwmgmt.GetAppResponse, got %T", PluginName, resp)
		}
	} else {
		// Use Studio services - returns *studiomgmt.GetAppResponse
		resp, err := ctx.Services.Studio().GetApp(ctx, appID)
		if err != nil {
			return nil, fmt.Errorf("failed to get app from Studio: %w", err)
		}

		// Type assert to concrete Studio proto type
		if studioResp, ok := resp.(*studiomgmt.GetAppResponse); ok {
			if studioResp.App != nil {
				metadataStr = studioResp.App.Metadata
				log.Printf("🔍 %s: Retrieved app metadata from Studio (len=%d)", PluginName, len(metadataStr))
			}
		} else {
			log.Printf("⚠️ %s: Failed to type assert Studio response to *studiomgmt.GetAppResponse, got %T", PluginName, resp)
		}
	}

	if metadataStr == "" {
		return nil, nil // No metadata
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	rateLimiterData, ok := metadata["rate_limiter"]
	if !ok {
		return nil, nil // No rate limiter config
	}

	rateLimiterJSON, _ := json.Marshal(rateLimiterData)
	var config AppRateLimitConfig
	if err := json.Unmarshal(rateLimiterJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to parse rate limiter config: %w", err)
	}

	// Cache for 5 minutes with TTL
	configJSON, _ := json.Marshal(config)
	ctx.Services.KV().WriteWithTTL(ctx, cacheKey, configJSON, 5*time.Minute)

	return &config, nil
}

func main() {
	plugin := NewLLMRateLimiterPlugin()
	plugin_sdk.Serve(plugin)
}

// === RPC Method Implementations ===

type ListPoliciesResponse struct {
	Policies []RateLimitPolicy `json:"policies"`
	Count    int               `json:"count"`
}

func (p *LLMRateLimiterPlugin) rpcListPolicies(payload []byte) (interface{}, error) {
	ctx := context.Background()
	policies := []RateLimitPolicy{}

	indexKey := "policy_index"
	indexData, err := ai_studio_sdk.ReadPluginKV(ctx, indexKey)
	if err == nil && len(indexData) > 0 {
		var policyNames []string
		if err := json.Unmarshal(indexData, &policyNames); err == nil {
			for _, name := range policyNames {
				policyData, err := ai_studio_sdk.ReadPluginKV(ctx, PolicyPrefix+name)
				if err == nil {
					var policy RateLimitPolicy
					if err := json.Unmarshal(policyData, &policy); err == nil {
						policies = append(policies, policy)
					}
				}
			}
		}
	}

	return ListPoliciesResponse{Policies: policies, Count: len(policies)}, nil
}

type GetPolicyRequest struct {
	Name string `json:"name"`
}

func (p *LLMRateLimiterPlugin) rpcGetPolicy(payload []byte) (interface{}, error) {
	var req GetPolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	ctx := context.Background()
	policyData, err := ai_studio_sdk.ReadPluginKV(ctx, PolicyPrefix+req.Name)
	if err != nil {
		return nil, fmt.Errorf("policy not found: %s", req.Name)
	}

	var policy RateLimitPolicy
	if err := json.Unmarshal(policyData, &policy); err != nil {
		return nil, fmt.Errorf("failed to parse policy: %v", err)
	}

	return policy, nil
}

type CreatePolicyRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Models      map[string]ModelLimits `json:"models"`
}

type CreatePolicyResponse struct {
	Success bool            `json:"success"`
	Policy  RateLimitPolicy `json:"policy"`
	Message string          `json:"message"`
}

func (p *LLMRateLimiterPlugin) rpcCreatePolicy(payload []byte) (interface{}, error) {
	var req CreatePolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("policy name is required")
	}
	if len(req.Models) == 0 {
		return nil, fmt.Errorf("at least one model configuration is required")
	}

	ctx := context.Background()
	key := PolicyPrefix + req.Name
	_, err := ai_studio_sdk.ReadPluginKV(ctx, key)
	if err == nil {
		return nil, fmt.Errorf("policy already exists: %s", req.Name)
	}

	now := time.Now()
	policy := RateLimitPolicy{
		Name:        req.Name,
		Description: req.Description,
		Models:      req.Models,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	policyData, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal policy: %v", err)
	}

	_, err = ai_studio_sdk.WritePluginKV(ctx, key, policyData, nil) // No expiration for policies
	if err != nil {
		return nil, fmt.Errorf("failed to write policy: %v", err)
	}

	p.addPolicyToIndex(ctx, req.Name)
	log.Printf("%s: Created policy '%s'", PluginName, req.Name)

	return CreatePolicyResponse{Success: true, Policy: policy, Message: "Policy created successfully"}, nil
}

type UpdatePolicyRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Models      map[string]ModelLimits `json:"models"`
}

type UpdatePolicyResponse struct {
	Success bool            `json:"success"`
	Policy  RateLimitPolicy `json:"policy"`
	Message string          `json:"message"`
}

func (p *LLMRateLimiterPlugin) rpcUpdatePolicy(payload []byte) (interface{}, error) {
	var req UpdatePolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	ctx := context.Background()
	key := PolicyPrefix + req.Name

	existingData, err := ai_studio_sdk.ReadPluginKV(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("policy not found: %s", req.Name)
	}

	var existing RateLimitPolicy
	if err := json.Unmarshal(existingData, &existing); err != nil {
		return nil, fmt.Errorf("failed to parse existing policy: %v", err)
	}

	existing.Description = req.Description
	existing.Models = req.Models
	existing.UpdatedAt = time.Now()

	policyData, err := json.Marshal(existing)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal policy: %v", err)
	}

	_, err = ai_studio_sdk.WritePluginKV(ctx, key, policyData, nil) // No expiration for policies
	if err != nil {
		return nil, fmt.Errorf("failed to write policy: %v", err)
	}

	log.Printf("%s: Updated policy '%s'", PluginName, req.Name)
	return UpdatePolicyResponse{Success: true, Policy: existing, Message: "Policy updated successfully"}, nil
}

type DeletePolicyRequest struct {
	Name string `json:"name"`
}

type DeletePolicyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p *LLMRateLimiterPlugin) rpcDeletePolicy(payload []byte) (interface{}, error) {
	var req DeletePolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	ctx := context.Background()
	key := PolicyPrefix + req.Name

	deleted, err := ai_studio_sdk.DeletePluginKV(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to delete policy: %v", err)
	}
	if !deleted {
		return nil, fmt.Errorf("policy not found: %s", req.Name)
	}

	p.removePolicyFromIndex(ctx, req.Name)
	log.Printf("%s: Deleted policy '%s'", PluginName, req.Name)

	return DeletePolicyResponse{Success: true, Message: "Policy deleted successfully"}, nil
}

type AppWithPolicy struct {
	ID          uint32              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	OwnerEmail  string              `json:"owner_email"`
	IsActive    bool                `json:"is_active"`
	RateLimit   *AppRateLimitConfig `json:"rate_limit,omitempty"`
}

type ListAppsWithPoliciesResponse struct {
	Apps  []AppWithPolicy `json:"apps"`
	Count int             `json:"count"`
}

func (p *LLMRateLimiterPlugin) rpcListAppsWithPolicies(payload []byte) (interface{}, error) {
	ctx := context.Background()

	appsResp, err := ai_studio_sdk.ListApps(ctx, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %v", err)
	}

	apps := []AppWithPolicy{}
	for _, app := range appsResp.Apps {
		appWithPolicy := AppWithPolicy{
			ID:          app.Id,
			Name:        app.Name,
			Description: app.Description,
			OwnerEmail:  app.OwnerEmail,
			IsActive:    app.IsActive,
		}

		if app.Metadata != "" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(app.Metadata), &metadata); err == nil {
				if rateLimiterData, ok := metadata["rate_limiter"]; ok {
					rateLimiterJSON, _ := json.Marshal(rateLimiterData)
					var rateLimitConfig AppRateLimitConfig
					if err := json.Unmarshal(rateLimiterJSON, &rateLimitConfig); err == nil {
						appWithPolicy.RateLimit = &rateLimitConfig
					}
				}
			}
		}

		apps = append(apps, appWithPolicy)
	}

	return ListAppsWithPoliciesResponse{Apps: apps, Count: len(apps)}, nil
}

type AssignPolicyRequest struct {
	AppID      uint32                 `json:"app_id"`
	PolicyName string                 `json:"policy_name"`
	Enabled    bool                   `json:"enabled"`
	Overrides  map[string]ModelLimits `json:"overrides,omitempty"`
}

type AssignPolicyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p *LLMRateLimiterPlugin) rpcAssignPolicy(payload []byte) (interface{}, error) {
	var req AssignPolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	ctx := context.Background()

	appResp, err := ai_studio_sdk.GetApp(ctx, req.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %v", err)
	}

	policyData, err := ai_studio_sdk.ReadPluginKV(ctx, PolicyPrefix+req.PolicyName)
	if err != nil {
		return nil, fmt.Errorf("policy not found: %s", req.PolicyName)
	}

	var policy RateLimitPolicy
	if err := json.Unmarshal(policyData, &policy); err != nil {
		return nil, fmt.Errorf("failed to parse policy: %v", err)
	}

	metadata := make(map[string]interface{})
	if appResp.App.Metadata != "" {
		if err := json.Unmarshal([]byte(appResp.App.Metadata), &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse app metadata: %v", err)
		}
	}

	rateLimitConfig := AppRateLimitConfig{
		PolicyName:  req.PolicyName,
		Enabled:     req.Enabled,
		Models:      policy.Models,
		Description: policy.Description,
		Overrides:   req.Overrides,
	}
	metadata["rate_limiter"] = rateLimitConfig

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %v", err)
	}

	_, err = ai_studio_sdk.UpdateAppWithMetadata(ctx, req.AppID, appResp.App.Name, appResp.App.Description,
		appResp.App.IsActive, appResp.App.LlmIds, appResp.App.ToolIds, appResp.App.DatasourceIds,
		appResp.App.MonthlyBudget, string(metadataJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to update app: %v", err)
	}

	log.Printf("%s: Assigned policy '%s' to app %d", PluginName, req.PolicyName, req.AppID)
	return AssignPolicyResponse{Success: true, Message: "Policy assigned successfully"}, nil
}

type RemovePolicyRequest struct {
	AppID uint32 `json:"app_id"`
}

type RemovePolicyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p *LLMRateLimiterPlugin) rpcRemovePolicy(payload []byte) (interface{}, error) {
	var req RemovePolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	ctx := context.Background()

	appResp, err := ai_studio_sdk.GetApp(ctx, req.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %v", err)
	}

	metadata := make(map[string]interface{})
	if appResp.App.Metadata != "" {
		if err := json.Unmarshal([]byte(appResp.App.Metadata), &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse app metadata: %v", err)
		}
	}

	delete(metadata, "rate_limiter")

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %v", err)
	}

	_, err = ai_studio_sdk.UpdateAppWithMetadata(ctx, req.AppID, appResp.App.Name, appResp.App.Description,
		appResp.App.IsActive, appResp.App.LlmIds, appResp.App.ToolIds, appResp.App.DatasourceIds,
		appResp.App.MonthlyBudget, string(metadataJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to update app: %v", err)
	}

	log.Printf("%s: Removed rate limit policy from app %d", PluginName, req.AppID)
	return RemovePolicyResponse{Success: true, Message: "Policy removed successfully"}, nil
}

func (p *LLMRateLimiterPlugin) addPolicyToIndex(ctx context.Context, policyName string) error {
	indexKey := "policy_index"
	var policyNames []string
	indexData, err := ai_studio_sdk.ReadPluginKV(ctx, indexKey)
	if err == nil && len(indexData) > 0 {
		json.Unmarshal(indexData, &policyNames)
	}

	exists := false
	for _, name := range policyNames {
		if name == policyName {
			exists = true
			break
		}
	}
	if !exists {
		policyNames = append(policyNames, policyName)
		indexJSON, _ := json.Marshal(policyNames)
		ai_studio_sdk.WritePluginKV(ctx, indexKey, indexJSON, nil) // No expiration for policy index
	}

	return nil
}

func (p *LLMRateLimiterPlugin) removePolicyFromIndex(ctx context.Context, policyName string) error {
	indexKey := "policy_index"
	var policyNames []string
	indexData, err := ai_studio_sdk.ReadPluginKV(ctx, indexKey)
	if err == nil && len(indexData) > 0 {
		json.Unmarshal(indexData, &policyNames)
	}

	newNames := []string{}
	for _, name := range policyNames {
		if name != policyName {
			newNames = append(newNames, name)
		}
	}

	indexJSON, _ := json.Marshal(newNames)
	ai_studio_sdk.WritePluginKV(ctx, indexKey, indexJSON, nil) // No expiration for policy index
	return nil
}
