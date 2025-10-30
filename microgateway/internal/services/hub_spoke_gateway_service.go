// internal/services/hub_spoke_gateway_service.go
package services

import (
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/providers"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
)

// HubSpokeGatewayService implements GatewayServiceInterface using ConfigurationProvider abstraction
// This service works in all gateway modes (standalone, control, edge)
type HubSpokeGatewayService struct {
	configProvider providers.ConfigurationProvider
	edgeClient     interface{} // EdgeClient reference (avoid import cycle)
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

// SetEdgeClient sets the edge client reference for on-demand token validation
func (s *HubSpokeGatewayService) SetEdgeClient(edgeClient interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edgeClient = edgeClient
	log.Info().Msg("Edge client reference set for on-demand token validation")
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
func (s *HubSpokeGatewayService) GetAppByID(id uint) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get app from database via configProvider
	return s.configProvider.GetApp(id)
}

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

// ValidateAPIToken validates an API token and returns token information
func (s *HubSpokeGatewayService) ValidateAPIToken(token string) (*TokenValidationResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}
	log.Info().Str("token_prefix", tokenPrefix).Str("provider_type", string(s.configProvider.GetProviderType())).Msg("HubSpokeGatewayService.ValidateAPIToken called")

	// For database provider (control/standalone), use normal validation
	if s.configProvider.GetProviderType() == providers.ProviderTypeDatabase {
		apiToken, err := s.configProvider.ValidateToken(token)
		if err != nil {
			log.Info().Err(err).Str("token_prefix", tokenPrefix).Msg("Database provider token validation failed")
			return nil, err
		}

		// Get associated app
		app, err := s.configProvider.GetApp(apiToken.AppID)
		if err != nil {
			log.Info().Err(err).Uint("app_id", apiToken.AppID).Msg("Failed to get app for validated token")
			return nil, fmt.Errorf("app not found for token: %w", err)
		}

		return &TokenValidationResult{
			TokenID:   apiToken.ID,
			TokenName: apiToken.Name,
			AppID:     apiToken.AppID,
			App:       app,
		}, nil
	}

	// For gRPC provider (edge), use on-demand validation via control instance
	log.Info().Str("token_prefix", tokenPrefix).Msg("Using on-demand token validation via control instance")
	
	if s.edgeClient == nil {
		return nil, fmt.Errorf("edge client not available for on-demand token validation")
	}

	// Cast to edge client and make validation call
	if edgeClient, ok := s.edgeClient.(interface{ ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) }); ok {
		resp, err := edgeClient.ValidateTokenOnDemand(token)
		if err != nil {
			log.Info().Err(err).Str("token_prefix", tokenPrefix).Msg("On-demand token validation failed")
			return nil, fmt.Errorf("token validation failed: %w", err)
		}

		if !resp.Valid {
			log.Info().Str("token_prefix", tokenPrefix).Str("error", resp.ErrorMessage).Msg("Token validation rejected by control")
			return nil, fmt.Errorf("invalid token: %s", resp.ErrorMessage)
		}

		log.Info().
			Str("token_prefix", tokenPrefix).
			Uint32("app_id", resp.AppId).
			Str("app_name", resp.AppName).
			Msg("On-demand token validation successful")

		// Get the app from local config cache
		app, err := s.configProvider.GetApp(uint(resp.AppId))
		if err != nil {
			log.Info().Err(err).Uint32("app_id", resp.AppId).Msg("App not found in local cache after token validation")
			return nil, fmt.Errorf("app not found in local cache: %w", err)
		}

		// Create a pseudo token ID for the response (since we don't have actual token ID)
		// Use app_id directly since we know it's valid - avoid plugin auth range (>= 1000)
		pseudoTokenID := uint(resp.AppId)

		return &TokenValidationResult{
			TokenID:   pseudoTokenID,
			TokenName: "on-demand-validated",
			AppID:     uint(resp.AppId),
			App:       app,
		}, nil
	}

	return nil, fmt.Errorf("edge client does not support token validation")
}

// GetAppByTokenID returns an app by token ID
func (s *HubSpokeGatewayService) GetAppByTokenID(tokenID uint) (*database.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Info().Uint("token_id", tokenID).Str("provider_type", string(s.configProvider.GetProviderType())).Msg("HubSpokeGatewayService.GetAppByTokenID called")

	// For on-demand validation, token_id equals app_id (since we use app_id as pseudo token id)
	// Try to get the app directly using the token_id as app_id
	app, err := s.configProvider.GetApp(tokenID)
	if err == nil {
		log.Info().Uint("token_id", tokenID).Uint("app_id", app.ID).Str("app_name", app.Name).Msg("Found app directly using token_id as app_id (on-demand validation)")
		return app, nil
	}

	// For database provider, could implement token lookup
	if s.configProvider.GetProviderType() == providers.ProviderTypeDatabase {
		// This would require implementing token lookup in DatabaseProvider
		return nil, fmt.Errorf("token ID lookup not implemented for database provider")
	}

	// For gRPC provider with regular token IDs, we can't efficiently look this up
	return nil, fmt.Errorf("get app by token ID not supported for provider type: %s", s.configProvider.GetProviderType())
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