package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// Embed UI assets and manifest into the binary
//
//go:embed ui assets manifest.json config.schema.json
var embeddedAssets embed.FS

//go:embed manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

// TokenConfig represents configuration for a single token
type TokenConfig struct {
	ID          string `json:"id"`
	Token       string `json:"token"`
	AppID       uint   `json:"app_id"`
	UserID      string `json:"user_id"`
	Description string `json:"description"`
}

const (
	PluginName    = "custom-auth-ui"
	PluginVersion = "1.0.0"
)

// CustomAuthUIPlugin implements both auth and UI capabilities
// using the unified plugin SDK
type CustomAuthUIPlugin struct {
	plugin_sdk.BasePlugin
	tokenMap            map[string]*TokenConfig // token -> config
	tokenByID           map[string]*TokenConfig // id -> config
	defaultAppID        uint
	rejectUnknownTokens bool
	mu                  sync.RWMutex // Protects token maps
	nextID              int
}

// NewCustomAuthUIPlugin creates a new custom auth UI plugin
func NewCustomAuthUIPlugin() *CustomAuthUIPlugin {
	return &CustomAuthUIPlugin{
		BasePlugin:          plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Custom Auth with UI"),
		tokenMap:            make(map[string]*TokenConfig),
		tokenByID:           make(map[string]*TokenConfig),
		rejectUnknownTokens: true,
		defaultAppID:        1,
		nextID:              1,
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *CustomAuthUIPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized in %s runtime", PluginName, ctx.Runtime)

	// Extract and set broker ID for service API access
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

	// Parse tokens from config
	if tokensValue, hasTokens := config["tokens"]; hasTokens {
		// Parse JSON string to tokens array
		var tokens []TokenConfig
		if err := json.Unmarshal([]byte(tokensValue), &tokens); err != nil {
			return fmt.Errorf("failed to parse tokens array: %w", err)
		}

		// Load tokens into maps
		for _, token := range tokens {
			if token.Token == "" {
				continue
			}
			if token.ID == "" {
				token.ID = fmt.Sprintf("token-%d", p.nextID)
				p.nextID++
			}
			p.tokenMap[token.Token] = &token
			p.tokenByID[token.ID] = &token
		}
	}

	// Parse reject_unknown_tokens
	if rejectStr, ok := config["reject_unknown_tokens"]; ok {
		p.rejectUnknownTokens = (rejectStr == "true" || rejectStr == "1")
	}

	// Parse default_app_id
	if appIDStr, ok := config["default_app_id"]; ok {
		var appID uint64
		if _, err := fmt.Sscanf(appIDStr, "%d", &appID); err == nil {
			p.defaultAppID = uint(appID)
		}
	}

	log.Printf("%s: Initialized with %d tokens", PluginName, len(p.tokenMap))
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *CustomAuthUIPlugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("%s: Shutdown called", PluginName)
	return nil
}

// GetAsset implements plugin_sdk.UIProvider
func (p *CustomAuthUIPlugin) GetAsset(assetPath string) ([]byte, string, error) {
	// Remove leading slash
	if strings.HasPrefix(assetPath, "/") {
		assetPath = strings.TrimPrefix(assetPath, "/")
	}

	log.Printf("%s: GetAsset called for path: %s", PluginName, assetPath)

	// Read from embedded filesystem
	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		log.Printf("%s: Asset not found: %s - error: %v", PluginName, assetPath, err)
		return nil, "", fmt.Errorf("asset not found: %s", assetPath)
	}

	// Determine content type based on file extension
	mimeType := "application/octet-stream"
	if strings.HasSuffix(assetPath, ".js") {
		mimeType = "application/javascript"
	} else if strings.HasSuffix(assetPath, ".css") {
		mimeType = "text/css"
	} else if strings.HasSuffix(assetPath, ".html") {
		mimeType = "text/html"
	} else if strings.HasSuffix(assetPath, ".json") {
		mimeType = "application/json"
	} else if strings.HasSuffix(assetPath, ".svg") {
		mimeType = "image/svg+xml"
	} else if strings.HasSuffix(assetPath, ".png") {
		mimeType = "image/png"
	} else if strings.HasSuffix(assetPath, ".jpg") || strings.HasSuffix(assetPath, ".jpeg") {
		mimeType = "image/jpeg"
	}

	log.Printf("%s: Serving asset %s (%d bytes, type: %s)", PluginName, assetPath, len(content), mimeType)
	return content, mimeType, nil
}

// GetManifest implements plugin_sdk.UIProvider
func (p *CustomAuthUIPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *CustomAuthUIPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{}, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *CustomAuthUIPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// HandleRPC implements plugin_sdk.UIProvider
func (p *CustomAuthUIPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	// Extract broker ID from payload
	if brokerID := ai_studio_sdk.ExtractBrokerIDFromPayload(payload); brokerID != 0 {
		ai_studio_sdk.SetServiceBrokerID(brokerID)
	}
	log.Printf("%s: RPC Call - method: %s, payload size: %d bytes", PluginName, method, len(payload))

	var result interface{}
	var err error

	switch method {
	case "listTokens":
		result, err = p.rpcListTokens(payload)
	case "getToken":
		result, err = p.rpcGetToken(payload)
	case "addToken":
		result, err = p.rpcAddToken(payload)
	case "updateToken":
		result, err = p.rpcUpdateToken(payload)
	case "deleteToken":
		result, err = p.rpcDeleteToken(payload)
	default:
		return nil, fmt.Errorf("unknown RPC method: %s", method)
	}

	if err != nil {
		log.Printf("%s: RPC error - method: %s, error: %v", PluginName, method, err)
		return nil, err
	}

	// Marshal result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %v", err)
	}

	log.Printf("%s: RPC success - method: %s, result size: %d bytes", PluginName, method, len(resultJSON))
	return resultJSON, nil
}

// HandleAuth implements plugin_sdk.AuthHandler
func (p *CustomAuthUIPlugin) HandleAuth(ctx plugin_sdk.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	// Extract token from credential
	token := req.Credential

	// Handle Bearer token format
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Lookup token in configured token map
	if tokenConfig, ok := p.tokenMap[token]; ok {
		// Token found - return configured app and user
		return &pb.AuthResponse{
			Authenticated: true,
			UserId:        tokenConfig.UserID,
			AppId:         fmt.Sprintf("%d", tokenConfig.AppID),
			Claims: map[string]string{
				"source":      PluginName,
				"token_id":    tokenConfig.ID,
				"description": tokenConfig.Description,
			},
		}, nil
	}

	// Token not in map - handle based on policy
	if p.rejectUnknownTokens {
		return &pb.AuthResponse{
			Authenticated: false,
			ErrorMessage:  "Invalid token. Token not found in custom auth plugin configuration.",
		}, nil
	}

	// Accept unknown tokens with default app ID
	return &pb.AuthResponse{
		Authenticated: true,
		UserId:        "unknown-user",
		AppId:         fmt.Sprintf("%d", p.defaultAppID),
		Claims: map[string]string{
			"source": PluginName,
			"token":  "unknown",
		},
	}, nil
}

// GetAppByCredential implements plugin_sdk.AuthHandler
func (p *CustomAuthUIPlugin) GetAppByCredential(ctx plugin_sdk.Context, credential string) (*pb.App, error) {
	// Extract token from credential
	token := credential
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	p.mu.RLock()
	tokenConfig, tokenFound := p.tokenMap[token]
	p.mu.RUnlock()

	var appID uint
	if tokenFound {
		appID = tokenConfig.AppID
	} else if !p.rejectUnknownTokens {
		appID = p.defaultAppID
	} else {
		return nil, fmt.Errorf("token not found")
	}

	// Fetch real app data from service API based on runtime
	var app *pb.App
	var err error

	if ctx.Runtime == plugin_sdk.RuntimeGateway {
		// Use Gateway services
		resp, err := ctx.Services.Gateway().GetApp(ctx, uint32(appID))
		if err != nil {
			return nil, fmt.Errorf("failed to get app from gateway: %w", err)
		}
		// Convert gateway proto to pb.App
		if gwResp, ok := resp.(*pb.GetAppResponse); ok && gwResp.App != nil {
			app = gwResp.App
		} else {
			return nil, fmt.Errorf("failed to get app data from gateway response")
		}
	} else {
		// Use Studio services
		resp, err := ctx.Services.Studio().GetApp(ctx, uint32(appID))
		if err != nil {
			return nil, fmt.Errorf("failed to get app from studio: %w", err)
		}
		// Convert studio proto to pb.App
		if studioResp, ok := resp.(*pb.GetAppResponse); ok && studioResp.App != nil {
			app = studioResp.App
		} else {
			return nil, fmt.Errorf("failed to get app data from studio response")
		}
	}

	return app, err
}

// GetUserByCredential implements plugin_sdk.AuthHandler
func (p *CustomAuthUIPlugin) GetUserByCredential(ctx plugin_sdk.Context, credential string) (*pb.User, error) {
	// User data is not available in microgateway - this method is not supported
	return nil, fmt.Errorf("GetUserByCredential not supported - user data not available in this context")
}

// === RPC Method Implementations ===

type ListTokensResponse struct {
	Tokens []TokenSummary `json:"tokens"`
	Count  int            `json:"count"`
}

type TokenSummary struct {
	ID          string `json:"id"`
	TokenMask   string `json:"token_mask"` // Masked for security
	AppID       uint   `json:"app_id"`
	UserID      string `json:"user_id"`
	Description string `json:"description"`
}

func (p *CustomAuthUIPlugin) rpcListTokens(payload []byte) (interface{}, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tokens := make([]TokenSummary, 0, len(p.tokenByID))
	for _, tokenConfig := range p.tokenByID {
		// Mask token for security - show first 4 and last 4 characters
		tokenMask := maskToken(tokenConfig.Token)
		tokens = append(tokens, TokenSummary{
			ID:          tokenConfig.ID,
			TokenMask:   tokenMask,
			AppID:       tokenConfig.AppID,
			UserID:      tokenConfig.UserID,
			Description: tokenConfig.Description,
		})
	}

	return ListTokensResponse{
		Tokens: tokens,
		Count:  len(tokens),
	}, nil
}

type GetTokenRequest struct {
	ID string `json:"id"`
}

func (p *CustomAuthUIPlugin) rpcGetToken(payload []byte) (interface{}, error) {
	var req GetTokenRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	tokenConfig, ok := p.tokenByID[req.ID]
	if !ok {
		return nil, fmt.Errorf("token not found: %s", req.ID)
	}

	return TokenSummary{
		ID:          tokenConfig.ID,
		TokenMask:   maskToken(tokenConfig.Token),
		AppID:       tokenConfig.AppID,
		UserID:      tokenConfig.UserID,
		Description: tokenConfig.Description,
	}, nil
}

type AddTokenRequest struct {
	Token       string `json:"token"`
	AppID       uint   `json:"app_id"`
	UserID      string `json:"user_id"`
	Description string `json:"description"`
}

type AddTokenResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

func (p *CustomAuthUIPlugin) rpcAddToken(payload []byte) (interface{}, error) {
	var req AddTokenRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	// Validate
	if req.Token == "" {
		return nil, fmt.Errorf("token is required")
	}
	if req.AppID == 0 {
		return nil, fmt.Errorf("app_id must be greater than 0")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for duplicate token
	if _, exists := p.tokenMap[req.Token]; exists {
		return nil, fmt.Errorf("token already exists")
	}

	// Generate new ID
	id := fmt.Sprintf("token-%d", p.nextID)
	p.nextID++

	// Create token config
	tokenConfig := &TokenConfig{
		ID:          id,
		Token:       req.Token,
		AppID:       req.AppID,
		UserID:      req.UserID,
		Description: req.Description,
	}

	// Add to maps
	p.tokenMap[req.Token] = tokenConfig
	p.tokenByID[id] = tokenConfig

	log.Printf("%s: Added token ID '%s' with AppID %d", PluginName, id, req.AppID)

	return AddTokenResponse{
		Success: true,
		ID:      id,
		Message: "Token added successfully",
	}, nil
}

type UpdateTokenRequest struct {
	ID          string `json:"id"`
	AppID       uint   `json:"app_id"`
	UserID      string `json:"user_id"`
	Description string `json:"description"`
}

type UpdateTokenResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p *CustomAuthUIPlugin) rpcUpdateToken(payload []byte) (interface{}, error) {
	var req UpdateTokenRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if req.AppID == 0 {
		return nil, fmt.Errorf("app_id must be greater than 0")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	tokenConfig, ok := p.tokenByID[req.ID]
	if !ok {
		return nil, fmt.Errorf("token not found: %s", req.ID)
	}

	// Update fields (token value is immutable)
	tokenConfig.AppID = req.AppID
	tokenConfig.UserID = req.UserID
	tokenConfig.Description = req.Description

	log.Printf("%s: Updated token ID '%s'", PluginName, req.ID)

	return UpdateTokenResponse{
		Success: true,
		Message: "Token updated successfully",
	}, nil
}

type DeleteTokenRequest struct {
	ID string `json:"id"`
}

type DeleteTokenResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p *CustomAuthUIPlugin) rpcDeleteToken(payload []byte) (interface{}, error) {
	var req DeleteTokenRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	tokenConfig, ok := p.tokenByID[req.ID]
	if !ok {
		return nil, fmt.Errorf("token not found: %s", req.ID)
	}

	// Remove from both maps
	delete(p.tokenMap, tokenConfig.Token)
	delete(p.tokenByID, req.ID)

	log.Printf("%s: Deleted token ID '%s'", PluginName, req.ID)

	return DeleteTokenResponse{
		Success: true,
		Message: "Token deleted successfully",
	}, nil
}

// === Helper Functions ===

func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

// === Main ===

func main() {
	log.Printf("Starting %s Plugin v%s", PluginName, PluginVersion)
	log.Printf("Hybrid plugin with auth + UI capabilities using unified SDK")

	plugin := NewCustomAuthUIPlugin()
	plugin_sdk.Serve(plugin)
}
