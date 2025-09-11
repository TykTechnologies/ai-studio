// internal/providers/grpc_provider.go
package providers

import (
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto"
	"gorm.io/gorm"
)

// GRPCProvider implements ConfigurationProvider using gRPC communication with control instance
type GRPCProvider struct {
	namespace        string
	namespaceFilter  NamespaceFilter
	
	// Local cache of configuration from control instance
	configCache      *pb.ConfigurationSnapshot
	cacheMutex       sync.RWMutex
	lastUpdate       time.Time
	
	// Connection status
	connected        bool
	
	// Edge client reference (will be set after creation)
	edgeClient       interface{} // Avoid import cycle, will be cast when needed
}

// NewGRPCProvider creates a new gRPC-backed configuration provider for edge instances
func NewGRPCProvider(namespace string) *GRPCProvider {
	provider := &GRPCProvider{
		namespace:       namespace,
		namespaceFilter: &DefaultNamespaceFilter{},
		connected:       false, // Will be set to true when edge client connects
	}
	
	return provider
}

// SetEdgeClient sets the edge client reference (avoids import cycle)
func (p *GRPCProvider) SetEdgeClient(edgeClient interface{}) {
	p.edgeClient = edgeClient
	p.connected = true
}

// SetConfigurationCache updates the configuration cache from edge client
func (p *GRPCProvider) SetConfigurationCache(config *pb.ConfigurationSnapshot) {
	p.cacheMutex.Lock()
	p.configCache = config
	p.lastUpdate = time.Now()
	p.connected = true
	p.cacheMutex.Unlock()
}

// GetProviderType returns the provider type
func (p *GRPCProvider) GetProviderType() ProviderType {
	return ProviderTypeGRPC
}

// IsHealthy checks if the gRPC connection to control is healthy
func (p *GRPCProvider) IsHealthy() bool {
	return p.connected && p.configCache != nil
}

// GetNamespace returns the provider's namespace
func (p *GRPCProvider) GetNamespace() string {
	return p.namespace
}

// onConfigurationUpdate handles configuration updates from the control instance
func (p *GRPCProvider) onConfigurationUpdate(config *pb.ConfigurationSnapshot) {
	p.cacheMutex.Lock()
	p.configCache = config
	p.lastUpdate = time.Now()
	p.cacheMutex.Unlock()
}

// ensureCacheValid checks if we have valid configuration cache
func (p *GRPCProvider) ensureCacheValid() error {
	p.cacheMutex.RLock()
	hasCache := p.configCache != nil
	p.cacheMutex.RUnlock()
	
	if !hasCache {
		return fmt.Errorf("no configuration cache available")
	}
	
	return nil
}

// GetLLM retrieves an LLM by ID from the gRPC cache
func (p *GRPCProvider) GetLLM(id uint) (*database.LLM, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbLLM := range p.configCache.Llms {
		if uint(pbLLM.Id) == id && p.namespaceFilter.MatchesNamespace(pbLLM.Namespace, p.namespace) {
			return p.convertPBLLMToDatabase(pbLLM), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// GetLLMBySlug retrieves an LLM by slug from the gRPC cache
func (p *GRPCProvider) GetLLMBySlug(slug string) (*database.LLM, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbLLM := range p.configCache.Llms {
		if pbLLM.Slug == slug && p.namespaceFilter.MatchesNamespace(pbLLM.Namespace, p.namespace) {
			return p.convertPBLLMToDatabase(pbLLM), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// ListLLMs retrieves LLMs from the gRPC cache
func (p *GRPCProvider) ListLLMs(namespace string, active bool) ([]database.LLM, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var llms []database.LLM
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	for _, pbLLM := range p.configCache.Llms {
		if p.namespaceFilter.MatchesNamespace(pbLLM.Namespace, targetNamespace) {
			if !active || pbLLM.IsActive {
				llms = append(llms, *p.convertPBLLMToDatabase(pbLLM))
			}
		}
	}
	
	return llms, nil
}

// GetApp retrieves an app by ID from the gRPC cache
func (p *GRPCProvider) GetApp(id uint) (*database.App, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbApp := range p.configCache.Apps {
		if uint(pbApp.Id) == id && p.namespaceFilter.MatchesNamespace(pbApp.Namespace, p.namespace) {
			return p.convertPBAppToDatabase(pbApp), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// ListApps retrieves apps from the gRPC cache
func (p *GRPCProvider) ListApps(namespace string, active bool) ([]database.App, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var apps []database.App
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	for _, pbApp := range p.configCache.Apps {
		if p.namespaceFilter.MatchesNamespace(pbApp.Namespace, targetNamespace) {
			if !active || pbApp.IsActive {
				apps = append(apps, *p.convertPBAppToDatabase(pbApp))
			}
		}
	}
	
	return apps, nil
}

// GetToken retrieves a token from the gRPC cache
func (p *GRPCProvider) GetToken(token string) (*database.APIToken, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbToken := range p.configCache.Tokens {
		if pbToken.Token == token && p.namespaceFilter.MatchesNamespace(pbToken.Namespace, p.namespace) {
			return p.convertPBTokenToDatabase(pbToken), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// ValidateToken validates a token from the gRPC cache
func (p *GRPCProvider) ValidateToken(token string) (*database.APIToken, error) {
	apiToken, err := p.GetToken(token)
	if err != nil {
		return nil, err
	}
	
	// Check if token is active
	if !apiToken.IsActive {
		return nil, fmt.Errorf("token is inactive")
	}
	
	// Check expiration
	if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token is expired")
	}
	
	return apiToken, nil
}

// GetModelPrice retrieves model pricing from the gRPC cache
func (p *GRPCProvider) GetModelPrice(vendor, model string) (*database.ModelPrice, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbPrice := range p.configCache.ModelPrices {
		if pbPrice.Vendor == vendor && pbPrice.ModelName == model && 
		   p.namespaceFilter.MatchesNamespace(pbPrice.Namespace, p.namespace) {
			return p.convertPBModelPriceToDatabase(pbPrice), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// ListModelPrices retrieves model prices from the gRPC cache
func (p *GRPCProvider) ListModelPrices(namespace string) ([]database.ModelPrice, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var prices []database.ModelPrice
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	for _, pbPrice := range p.configCache.ModelPrices {
		if p.namespaceFilter.MatchesNamespace(pbPrice.Namespace, targetNamespace) {
			prices = append(prices, *p.convertPBModelPriceToDatabase(pbPrice))
		}
	}
	
	return prices, nil
}

// GetFilter retrieves a filter by ID from the gRPC cache
func (p *GRPCProvider) GetFilter(id uint) (*database.Filter, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbFilter := range p.configCache.Filters {
		if uint(pbFilter.Id) == id && p.namespaceFilter.MatchesNamespace(pbFilter.Namespace, p.namespace) {
			return p.convertPBFilterToDatabase(pbFilter), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// GetFiltersForLLM retrieves filters associated with an LLM from the gRPC cache
func (p *GRPCProvider) GetFiltersForLLM(llmID uint) ([]database.Filter, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var filters []database.Filter
	
	for _, pbFilter := range p.configCache.Filters {
		if p.namespaceFilter.MatchesNamespace(pbFilter.Namespace, p.namespace) && pbFilter.IsActive {
			// Check if filter is associated with the LLM
			for _, filterLLMID := range pbFilter.LlmIds {
				if uint(filterLLMID) == llmID {
					filters = append(filters, *p.convertPBFilterToDatabase(pbFilter))
					break
				}
			}
		}
	}
	
	return filters, nil
}

// ListFilters retrieves filters from the gRPC cache
func (p *GRPCProvider) ListFilters(namespace string, active bool) ([]database.Filter, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var filters []database.Filter
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	for _, pbFilter := range p.configCache.Filters {
		if p.namespaceFilter.MatchesNamespace(pbFilter.Namespace, targetNamespace) {
			if !active || pbFilter.IsActive {
				filters = append(filters, *p.convertPBFilterToDatabase(pbFilter))
			}
		}
	}
	
	return filters, nil
}

// GetPlugin retrieves a plugin by ID from the gRPC cache
func (p *GRPCProvider) GetPlugin(id uint) (*database.Plugin, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	for _, pbPlugin := range p.configCache.Plugins {
		if uint(pbPlugin.Id) == id && p.namespaceFilter.MatchesNamespace(pbPlugin.Namespace, p.namespace) {
			return p.convertPBPluginToDatabase(pbPlugin), nil
		}
	}
	
	return nil, gorm.ErrRecordNotFound
}

// GetPluginsForLLM retrieves plugins associated with an LLM from the gRPC cache
func (p *GRPCProvider) GetPluginsForLLM(llmID uint) ([]database.Plugin, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var plugins []database.Plugin
	
	for _, pbPlugin := range p.configCache.Plugins {
		if p.namespaceFilter.MatchesNamespace(pbPlugin.Namespace, p.namespace) && pbPlugin.IsActive {
			// Check if plugin is associated with the LLM
			for _, pluginLLMID := range pbPlugin.LlmIds {
				if uint(pluginLLMID) == llmID {
					plugins = append(plugins, *p.convertPBPluginToDatabase(pbPlugin))
					break
				}
			}
		}
	}
	
	return plugins, nil
}

// ListPlugins retrieves plugins from the gRPC cache
func (p *GRPCProvider) ListPlugins(namespace string, hookType string, active bool) ([]database.Plugin, error) {
	if err := p.ensureCacheValid(); err != nil {
		return nil, err
	}
	
	p.cacheMutex.RLock()
	defer p.cacheMutex.RUnlock()
	
	var plugins []database.Plugin
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	for _, pbPlugin := range p.configCache.Plugins {
		if p.namespaceFilter.MatchesNamespace(pbPlugin.Namespace, targetNamespace) {
			if (!active || pbPlugin.IsActive) && (hookType == "" || pbPlugin.HookType == hookType) {
				plugins = append(plugins, *p.convertPBPluginToDatabase(pbPlugin))
			}
		}
	}
	
	return plugins, nil
}

// Conversion methods from protobuf to database models

func (p *GRPCProvider) convertPBLLMToDatabase(pbLLM *pb.LLMConfig) *database.LLM {
	// Convert protobuf timestamps
	var createdAt, updatedAt time.Time
	if pbLLM.CreatedAt != nil {
		createdAt = pbLLM.CreatedAt.AsTime()
	}
	if pbLLM.UpdatedAt != nil {
		updatedAt = pbLLM.UpdatedAt.AsTime()
	}
	
	return &database.LLM{
		Model: gorm.Model{
			ID:        uint(pbLLM.Id),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Name:            pbLLM.Name,
		Slug:            pbLLM.Slug,
		Vendor:          pbLLM.Vendor,
		Endpoint:        pbLLM.Endpoint,
		APIKeyEncrypted: pbLLM.ApiKeyEncrypted,
		DefaultModel:    pbLLM.DefaultModel,
		MaxTokens:       int(pbLLM.MaxTokens),
		TimeoutSeconds:  int(pbLLM.TimeoutSeconds),
		RetryCount:      int(pbLLM.RetryCount),
		IsActive:        pbLLM.IsActive,
		MonthlyBudget:   pbLLM.MonthlyBudget,
		RateLimitRPM:    int(pbLLM.RateLimitRpm),
		Namespace:       pbLLM.Namespace,
		// Note: JSON fields (Metadata, AllowedModels, AuthConfig) would need proper JSON unmarshaling
		// For now, we'll leave them empty or implement conversion if needed
	}
}

func (p *GRPCProvider) convertPBAppToDatabase(pbApp *pb.AppConfig) *database.App {
	var createdAt, updatedAt time.Time
	if pbApp.CreatedAt != nil {
		createdAt = pbApp.CreatedAt.AsTime()
	}
	if pbApp.UpdatedAt != nil {
		updatedAt = pbApp.UpdatedAt.AsTime()
	}
	
	return &database.App{
		Model: gorm.Model{
			ID:        uint(pbApp.Id),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Name:         pbApp.Name,
		Description:  pbApp.Description,
		OwnerEmail:   pbApp.OwnerEmail,
		IsActive:     pbApp.IsActive,
		MonthlyBudget: pbApp.MonthlyBudget,
		BudgetResetDay: int(pbApp.BudgetResetDay),
		RateLimitRPM: int(pbApp.RateLimitRpm),
		Namespace:    pbApp.Namespace,
	}
}

func (p *GRPCProvider) convertPBTokenToDatabase(pbToken *pb.TokenConfig) *database.APIToken {
	var createdAt, updatedAt time.Time
	if pbToken.CreatedAt != nil {
		createdAt = pbToken.CreatedAt.AsTime()
	}
	if pbToken.UpdatedAt != nil {
		updatedAt = pbToken.UpdatedAt.AsTime()
	}
	
	return &database.APIToken{
		ID:        uint(pbToken.Id),
		Token:     pbToken.Token,
		Name:      pbToken.Name,
		AppID:     uint(pbToken.AppId),
		IsActive:  pbToken.IsActive,
		Namespace: pbToken.Namespace,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		// Note: Scopes JSON field would need proper conversion
	}
}

func (p *GRPCProvider) convertPBModelPriceToDatabase(pbPrice *pb.ModelPriceConfig) *database.ModelPrice {
	var createdAt, updatedAt time.Time
	if pbPrice.CreatedAt != nil {
		createdAt = pbPrice.CreatedAt.AsTime()
	}
	if pbPrice.UpdatedAt != nil {
		updatedAt = pbPrice.UpdatedAt.AsTime()
	}
	
	return &database.ModelPrice{
		Model: gorm.Model{
			ID:        uint(pbPrice.Id),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Vendor:       pbPrice.Vendor,
		ModelName:    pbPrice.ModelName,
		CPT:          pbPrice.Cpt,
		CPIT:         pbPrice.Cpit,
		CacheWritePT: pbPrice.CacheWritePt,
		CacheReadPT:  pbPrice.CacheReadPt,
		Currency:     pbPrice.Currency,
		Namespace:    pbPrice.Namespace,
	}
}

func (p *GRPCProvider) convertPBFilterToDatabase(pbFilter *pb.FilterConfig) *database.Filter {
	var createdAt, updatedAt time.Time
	if pbFilter.CreatedAt != nil {
		createdAt = pbFilter.CreatedAt.AsTime()
	}
	if pbFilter.UpdatedAt != nil {
		updatedAt = pbFilter.UpdatedAt.AsTime()
	}
	
	return &database.Filter{
		ID:          uint(pbFilter.Id),
		Name:        pbFilter.Name,
		Description: pbFilter.Description,
		Script:      pbFilter.Script,
		IsActive:    pbFilter.IsActive,
		OrderIndex:  int(pbFilter.OrderIndex),
		Namespace:   pbFilter.Namespace,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func (p *GRPCProvider) convertPBPluginToDatabase(pbPlugin *pb.PluginConfig) *database.Plugin {
	var createdAt, updatedAt time.Time
	if pbPlugin.CreatedAt != nil {
		createdAt = pbPlugin.CreatedAt.AsTime()
	}
	if pbPlugin.UpdatedAt != nil {
		updatedAt = pbPlugin.UpdatedAt.AsTime()
	}
	
	return &database.Plugin{
		ID:          uint(pbPlugin.Id),
		Name:        pbPlugin.Name,
		Slug:        pbPlugin.Slug,
		Description: pbPlugin.Description,
		Command:     pbPlugin.Command,
		Checksum:    pbPlugin.Checksum,
		HookType:    pbPlugin.HookType,
		IsActive:    pbPlugin.IsActive,
		Namespace:   pbPlugin.Namespace,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		// Note: Config JSON field would need proper conversion
	}
}