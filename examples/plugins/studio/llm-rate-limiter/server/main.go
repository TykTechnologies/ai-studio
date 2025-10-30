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
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
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
	PluginVersion = "1.0.0"

	// K/V key prefixes
	PolicyPrefix = "policy:"
	CachePrefix  = "cache:app:"
	RatePrefix   = "rate:"
)

// ModelLimits defines rate limits for a specific model
type ModelLimits struct {
	TPM        int `json:"tpm"`         // Tokens per minute
	RPM        int `json:"rpm"`         // Requests per minute
	Concurrent int `json:"concurrent"`  // Max concurrent requests
}

// RateLimitPolicy defines a reusable rate limit configuration
type RateLimitPolicy struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Models      map[string]ModelLimits  `json:"models"` // model name -> limits, "*" for default
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// AppRateLimitConfig is stored in App.Metadata under "rate_limiter" key
// Contains the full policy data so Gateway can access it without needing K/V store
type AppRateLimitConfig struct {
	PolicyName  string                  `json:"policy_name"`
	Enabled     bool                    `json:"enabled"`
	Models      map[string]ModelLimits  `json:"models"`    // Full policy data embedded
	Description string                  `json:"description,omitempty"`
	Overrides   map[string]ModelLimits  `json:"overrides,omitempty"` // Per-model overrides
}

// RateState tracks current usage for rate limiting
type RateState struct {
	MinuteKey   string `json:"minute_key"`   // YYYY-MM-DDTHH:mm format
	Tokens      int    `json:"tokens"`
	Requests    int    `json:"requests"`
	Concurrent  int    `json:"concurrent"`
	UpdatedAt   int64  `json:"updated_at"`
}

// LLMRateLimiterPlugin implements both studio_ui and post_auth hooks
// This single plugin works in both AI Studio and Microgateway contexts
type LLMRateLimiterPlugin struct {
	pb.UnimplementedPluginServiceServer
	serviceAPI mgmt.AIStudioManagementServiceClient
	pluginID   uint32
	mu         sync.RWMutex
	rateLocks  map[string]*sync.Mutex // Per-app/model locks for atomic rate updates (gateway mode)
}

// OnInitialize implements AIStudioPluginImplementation interface
func (p *LLMRateLimiterPlugin) OnInitialize(serviceAPI mgmt.AIStudioManagementServiceClient, pluginID uint32, config map[string]string) error {
	p.serviceAPI = serviceAPI
	p.pluginID = pluginID
	p.rateLocks = make(map[string]*sync.Mutex)

	log.Printf("%s: Initialized (Plugin ID: %d)", PluginName, pluginID)
	return nil
}

// OnShutdown implements AIStudioPluginImplementation interface
func (p *LLMRateLimiterPlugin) OnShutdown() error {
	log.Printf("%s: OnShutdown called", PluginName)
	return nil
}

// GetAsset implements AIStudioPluginImplementation interface
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

// GetManifest implements AIStudioPluginImplementation interface
func (p *LLMRateLimiterPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema implements AIStudioPluginImplementation interface
func (p *LLMRateLimiterPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// HandleCall implements AIStudioPluginImplementation interface for RPC methods
func (p *LLMRateLimiterPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
	log.Printf("%s: RPC Call - method: %s", PluginName, method)

	// Extract broker ID from payload and set it for service API access
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

// === RPC Method Implementations ===

type ListPoliciesResponse struct {
	Policies []RateLimitPolicy `json:"policies"`
	Count    int               `json:"count"`
}

func (p *LLMRateLimiterPlugin) rpcListPolicies(payload []byte) (interface{}, error) {
	ctx := context.Background()

	// List all policy keys from K/V store
	// Note: We'll iterate through known policies or maintain an index
	// For simplicity, we'll return policies we find by scanning

	// This is a simplified implementation - in production you'd maintain a policy index
	policies := []RateLimitPolicy{}

	// Try to read a policy index if it exists
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

	return ListPoliciesResponse{
		Policies: policies,
		Count:    len(policies),
	}, nil
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
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Models      map[string]ModelLimits  `json:"models"`
}

type CreatePolicyResponse struct {
	Success bool              `json:"success"`
	Policy  RateLimitPolicy   `json:"policy"`
	Message string            `json:"message"`
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

	// Check if policy already exists
	key := PolicyPrefix + req.Name
	_, err := ai_studio_sdk.ReadPluginKV(ctx, key)
	if err == nil {
		return nil, fmt.Errorf("policy already exists: %s", req.Name)
	}

	// Create policy
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

	// Write policy to K/V
	_, err = ai_studio_sdk.WritePluginKV(ctx, key, policyData)
	if err != nil {
		return nil, fmt.Errorf("failed to write policy: %v", err)
	}

	// Update policy index
	p.addPolicyToIndex(ctx, req.Name)

	log.Printf("%s: Created policy '%s'", PluginName, req.Name)

	return CreatePolicyResponse{
		Success: true,
		Policy:  policy,
		Message: "Policy created successfully",
	}, nil
}

type UpdatePolicyRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Models      map[string]ModelLimits  `json:"models"`
}

type UpdatePolicyResponse struct {
	Success bool              `json:"success"`
	Policy  RateLimitPolicy   `json:"policy"`
	Message string            `json:"message"`
}

func (p *LLMRateLimiterPlugin) rpcUpdatePolicy(payload []byte) (interface{}, error) {
	var req UpdatePolicyRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	ctx := context.Background()
	key := PolicyPrefix + req.Name

	// Check if policy exists
	existingData, err := ai_studio_sdk.ReadPluginKV(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("policy not found: %s", req.Name)
	}

	var existing RateLimitPolicy
	if err := json.Unmarshal(existingData, &existing); err != nil {
		return nil, fmt.Errorf("failed to parse existing policy: %v", err)
	}

	// Update policy
	existing.Description = req.Description
	existing.Models = req.Models
	existing.UpdatedAt = time.Now()

	policyData, err := json.Marshal(existing)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal policy: %v", err)
	}

	_, err = ai_studio_sdk.WritePluginKV(ctx, key, policyData)
	if err != nil {
		return nil, fmt.Errorf("failed to write policy: %v", err)
	}

	log.Printf("%s: Updated policy '%s'", PluginName, req.Name)

	return UpdatePolicyResponse{
		Success: true,
		Policy:  existing,
		Message: "Policy updated successfully",
	}, nil
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

	// Remove from index
	p.removePolicyFromIndex(ctx, req.Name)

	log.Printf("%s: Deleted policy '%s'", PluginName, req.Name)

	return DeletePolicyResponse{
		Success: true,
		Message: "Policy deleted successfully",
	}, nil
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

	// List all apps - now with metadata support
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

		// Parse rate limit config from app metadata
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

	return ListAppsWithPoliciesResponse{
		Apps:  apps,
		Count: len(apps),
	}, nil
}

type AssignPolicyRequest struct {
	AppID       uint32                  `json:"app_id"`
	PolicyName  string                  `json:"policy_name"`
	Enabled     bool                    `json:"enabled"`
	Overrides   map[string]ModelLimits  `json:"overrides,omitempty"`
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

	// Get app
	appResp, err := ai_studio_sdk.GetApp(ctx, req.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %v", err)
	}

	// Fetch the full policy data from K/V
	policyData, err := ai_studio_sdk.ReadPluginKV(ctx, PolicyPrefix+req.PolicyName)
	if err != nil {
		return nil, fmt.Errorf("policy not found: %s", req.PolicyName)
	}

	var policy RateLimitPolicy
	if err := json.Unmarshal(policyData, &policy); err != nil {
		return nil, fmt.Errorf("failed to parse policy: %v", err)
	}

	// Parse existing metadata
	metadata := make(map[string]interface{})
	if appResp.App.Metadata != "" {
		if err := json.Unmarshal([]byte(appResp.App.Metadata), &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse app metadata: %v", err)
		}
	}

	// Embed the full policy data in the app metadata so Gateway can access it
	rateLimitConfig := AppRateLimitConfig{
		PolicyName:  req.PolicyName,
		Enabled:     req.Enabled,
		Models:      policy.Models,      // Embed full policy limits
		Description: policy.Description,
		Overrides:   req.Overrides,
	}
	metadata["rate_limiter"] = rateLimitConfig

	// Marshal updated metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %v", err)
	}

	// Update app with new metadata using SDK helper function
	// IMPORTANT: Pass existing IDs to preserve all associations
	_, err = ai_studio_sdk.UpdateAppWithMetadata(
		ctx,
		req.AppID,
		appResp.App.Name,
		appResp.App.Description,
		appResp.App.IsActive,
		appResp.App.LlmIds,        // Preserve existing LLM associations
		appResp.App.ToolIds,       // Preserve existing Tool associations
		appResp.App.DatasourceIds, // Preserve existing Datasource associations
		appResp.App.MonthlyBudget,
		string(metadataJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update app: %v", err)
	}

	log.Printf("%s: Assigned policy '%s' to app %d", PluginName, req.PolicyName, req.AppID)

	return AssignPolicyResponse{
		Success: true,
		Message: "Policy assigned successfully",
	}, nil
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

	// Get app
	appResp, err := ai_studio_sdk.GetApp(ctx, req.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %v", err)
	}

	// Parse existing metadata
	metadata := make(map[string]interface{})
	if appResp.App.Metadata != "" {
		if err := json.Unmarshal([]byte(appResp.App.Metadata), &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse app metadata: %v", err)
		}
	}

	// Remove rate limiter config
	delete(metadata, "rate_limiter")

	// Marshal updated metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %v", err)
	}

	// Update app using SDK helper function
	// IMPORTANT: Pass existing IDs to preserve all associations
	_, err = ai_studio_sdk.UpdateAppWithMetadata(
		ctx,
		req.AppID,
		appResp.App.Name,
		appResp.App.Description,
		appResp.App.IsActive,
		appResp.App.LlmIds,        // Preserve existing LLM associations
		appResp.App.ToolIds,       // Preserve existing Tool associations
		appResp.App.DatasourceIds, // Preserve existing Datasource associations
		appResp.App.MonthlyBudget,
		string(metadataJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update app: %v", err)
	}

	log.Printf("%s: Removed rate limit policy from app %d", PluginName, req.AppID)

	return RemovePolicyResponse{
		Success: true,
		Message: "Policy removed successfully",
	}, nil
}

// === Helper Functions ===

func (p *LLMRateLimiterPlugin) addPolicyToIndex(ctx context.Context, policyName string) error {
	indexKey := "policy_index"

	var policyNames []string
	indexData, err := ai_studio_sdk.ReadPluginKV(ctx, indexKey)
	if err == nil && len(indexData) > 0 {
		json.Unmarshal(indexData, &policyNames)
	}

	// Add if not exists
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
		ai_studio_sdk.WritePluginKV(ctx, indexKey, indexJSON)
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

	// Remove from list
	newNames := []string{}
	for _, name := range policyNames {
		if name != policyName {
			newNames = append(newNames, name)
		}
	}

	indexJSON, _ := json.Marshal(newNames)
	ai_studio_sdk.WritePluginKV(ctx, indexKey, indexJSON)

	return nil
}


// === Main ===

func main() {
	log.Printf("Starting %s Plugin v%s", PluginName, PluginVersion)
	log.Printf("This is a STUDIO plugin with UI - not a gateway plugin")
	log.Printf("Gateway enforcement happens separately")

	plugin := &LLMRateLimiterPlugin{}
	ai_studio_sdk.ServePlugin(plugin)
}

