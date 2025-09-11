// internal/services/hub_spoke_gateway_service.go
package services

import (
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/providers"
	"github.com/rs/zerolog/log"
)

// HubSpokeGatewayService implements GatewayServiceInterface using ConfigurationProvider abstraction
// This service works in all gateway modes (standalone, control, edge)
type HubSpokeGatewayService struct {
	configProvider providers.ConfigurationProvider
	mu             sync.RWMutex
}

// NewHubSpokeGatewayService creates a new hub-spoke aware gateway service
func NewHubSpokeGatewayService(configProvider providers.ConfigurationProvider) GatewayServiceInterface {
	service := &HubSpokeGatewayService{
		configProvider: configProvider,
	}
	
	log.Info().
		Str("provider_type", string(configProvider.GetProviderType())).
		Str("namespace", configProvider.GetNamespace()).
		Msg("Created hub-spoke gateway service")
	
	return service
}

// GetActiveLLMs returns all active LLMs from the configuration provider
func (s *HubSpokeGatewayService) GetActiveLLMs() ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Debug().
		Str("provider_type", string(s.configProvider.GetProviderType())).
		Str("namespace", s.configProvider.GetNamespace()).
		Msg("Getting active LLMs from provider")

	// Get LLMs from configuration provider (filtered by namespace)
	llms, err := s.configProvider.ListLLMs("", true) // empty namespace = use provider's namespace
	if err != nil {
		return nil, fmt.Errorf("failed to get active LLMs: %w", err)
	}

	// Convert to interface slice (store pointers)
	result := make([]interface{}, len(llms))
	for i := range llms {
		result[i] = &llms[i]
	}

	log.Debug().
		Int("llm_count", len(result)).
		Str("provider_type", string(s.configProvider.GetProviderType())).
		Msg("Retrieved active LLMs from provider")

	return result, nil
}

// GetLLMBySlug retrieves an LLM configuration by its slug from the configuration provider
func (s *HubSpokeGatewayService) GetLLMBySlug(slug string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Debug().
		Str("slug", slug).
		Str("provider_type", string(s.configProvider.GetProviderType())).
		Msg("Getting LLM by slug from provider")

	llm, err := s.configProvider.GetLLMBySlug(slug)
	if err != nil {
		return nil, fmt.Errorf("LLM not found: %s", slug)
	}

	return llm, nil
}

// GetCredentialBySecret validates a credential secret (legacy - not used in microgateway)
func (s *HubSpokeGatewayService) GetCredentialBySecret(secret string) (interface{}, error) {
	return nil, fmt.Errorf("credential authentication not supported - use token authentication")
}

// GetAppByCredentialID returns the app associated with a credential
func (s *HubSpokeGatewayService) GetAppByCredentialID(credID uint) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// For token-based auth, we don't use credentials
	// This method is for backward compatibility only
	return nil, fmt.Errorf("credential-based authentication not supported")
}

// ValidateAppAccess validates if an app can access a specific LLM
func (s *HubSpokeGatewayService) ValidateAppAccess(appID uint, llmSlug string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get LLM by slug
	llm, err := s.configProvider.GetLLMBySlug(llmSlug)
	if err != nil {
		return fmt.Errorf("LLM not found: %s", llmSlug)
	}

	if !llm.IsActive {
		return fmt.Errorf("LLM is inactive: %s", llmSlug)
	}

	// Get app to verify it exists and check namespace compatibility
	app, err := s.configProvider.GetApp(appID)
	if err != nil {
		return fmt.Errorf("app not found: %d", appID)
	}

	if !app.IsActive {
		return fmt.Errorf("app is inactive: %d", appID)
	}

	// Check namespace compatibility
	// App can access LLM if:
	// 1. LLM is global (namespace = "")
	// 2. App and LLM are in the same namespace
	if llm.Namespace != "" && llm.Namespace != app.Namespace {
		return fmt.Errorf("app %d (namespace: %s) cannot access LLM %s (namespace: %s)", 
			appID, app.Namespace, llmSlug, llm.Namespace)
	}

	// Check explicit app-LLM association
	// TODO: This would need to be implemented in the configuration provider
	// For now, we allow access if namespace rules are satisfied
	
	return nil
}

// Reload reloads the gateway configuration
func (s *HubSpokeGatewayService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Info().
		Str("provider_type", string(s.configProvider.GetProviderType())).
		Msg("Reloading gateway configuration")

	// For database providers, no action needed (queries are live)
	// For gRPC providers, could trigger a full sync
	if _, ok := s.configProvider.(*providers.GRPCProvider); ok {
		// TODO: Request full sync from control when edge client is available
		log.Info().Msg("Reload requested for gRPC provider - sync will be implemented with edge client integration")
	}

	return nil
}

// HubSpokeTokenValidationResult represents the result of token validation for hub-spoke gateway
type HubSpokeTokenValidationResult struct {
	TokenID   uint
	TokenName string
	AppID     uint
	App       *database.App
}

// ValidateAPIToken validates an API token and returns token information
func (s *HubSpokeGatewayService) ValidateAPIToken(token string) (*HubSpokeTokenValidationResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	apiToken, err := s.configProvider.ValidateToken(token)
	if err != nil {
		return nil, err
	}

	// Get associated app
	app, err := s.configProvider.GetApp(apiToken.AppID)
	if err != nil {
		return nil, fmt.Errorf("app not found for token: %w", err)
	}

	return &HubSpokeTokenValidationResult{
		TokenID:   apiToken.ID,
		TokenName: apiToken.Name,
		AppID:     apiToken.AppID,
		App:       app,
	}, nil
}

// GetAppByTokenID returns an app by token ID
func (s *HubSpokeGatewayService) GetAppByTokenID(tokenID uint) (*database.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// This is a simplified approach - in a real implementation we'd need
	// to look up the token by ID and then get the associated app
	// For now, return an error since we typically validate by token string
	return nil, fmt.Errorf("get app by token ID not implemented - use ValidateAPIToken instead")
}

// GetLLMStats returns statistics for a specific LLM
func (s *HubSpokeGatewayService) GetLLMStats(llmID uint) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Verify LLM exists
	_, err := s.configProvider.GetLLM(llmID)
	if err != nil {
		return nil, fmt.Errorf("LLM not found: %d", llmID)
	}

	// For gRPC providers (edge instances), we don't have access to analytics
	// Return basic stats
	if s.configProvider.GetProviderType() == providers.ProviderTypeGRPC {
		return map[string]interface{}{
			"request_count": 0,
			"total_tokens":  0,
			"total_cost":    0.0,
			"note":          "Analytics not available on edge instances",
		}, nil
	}

	// For database providers, we could implement full analytics
	// TODO: Implement analytics querying through the provider
	return map[string]interface{}{
		"request_count": 0,
		"total_tokens":  0,
		"total_cost":    0.0,
	}, nil
}