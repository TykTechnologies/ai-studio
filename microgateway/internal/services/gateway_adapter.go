// internal/services/gateway_adapter.go
package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog/log"
)

// GatewayServiceAdapter adapts our DatabaseGatewayService to implement services.ServiceInterface
type GatewayServiceAdapter struct {
	gatewayService GatewayServiceInterface
	management     ManagementServiceInterface
	analytics      AnalyticsServiceInterface
	crypto         CryptoServiceInterface
	filterService  FilterServiceInterface
	pluginService  PluginServiceInterface
}

// NewGatewayServiceAdapter creates a new adapter that implements services.ServiceInterface
func NewGatewayServiceAdapter(
	gatewayService GatewayServiceInterface,
	management ManagementServiceInterface,
	analytics AnalyticsServiceInterface,
	crypto CryptoServiceInterface,
	filterService FilterServiceInterface,
	pluginService PluginServiceInterface,
) services.ServiceInterface {
	adapter := &GatewayServiceAdapter{
		gatewayService: gatewayService,
		management:     management,
		analytics:      analytics,
		crypto:         crypto,
		filterService:  filterService,
		pluginService:  pluginService,
	}
	
	log.Info().Msg("GatewayServiceAdapter created - testing LLM loading...")
	
	// Test LLM loading immediately to debug
	llms, err := adapter.GetActiveLLMs()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load LLMs in adapter creation")
	} else {
		log.Info().Int("llm_count", len(llms)).Msg("LLMs loaded successfully in adapter")
		for i, llm := range llms {
			log.Debug().Int("index", i).Uint("llm_id", llm.ID).Str("name", llm.Name).Str("vendor", string(llm.Vendor)).Msg("LLM loaded")
		}
	}
	
	return adapter
}

// GetActiveLLMs returns all active LLMs
func (a *GatewayServiceAdapter) GetActiveLLMs() ([]models.LLM, error) {
	log.Debug().Msg("GatewayServiceAdapter.GetActiveLLMs() called by AI Gateway")
	
	llmInterfaces, err := a.gatewayService.GetActiveLLMs()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get active LLMs from gateway service")
		return nil, err
	}

	// Convert from interface{} to models.LLM
	llms := make([]models.LLM, len(llmInterfaces))
	for i, llmInterface := range llmInterfaces {
		if dbLLM, ok := llmInterface.(*database.LLM); ok {
			llm := a.convertDatabaseLLMToModel(dbLLM)
			llms[i] = llm
		} else {
			return nil, fmt.Errorf("unexpected LLM type at index %d", i)
		}
	}

	log.Debug().Int("llm_count", len(llms)).Msg("Successfully retrieved active LLMs for AI Gateway")
	return llms, nil
}

// GetLLMByID returns an LLM by its ID
func (a *GatewayServiceAdapter) GetLLMByID(id uint) (*models.LLM, error) {
	dbLLM, err := a.management.GetLLM(id)
	if err != nil {
		return nil, err
	}

	llm := a.convertDatabaseLLMToModel(dbLLM)
	return &llm, nil
}

// GetLLMSettingsByID returns LLM settings (not implemented for now)
func (a *GatewayServiceAdapter) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	return nil, fmt.Errorf("LLM settings not implemented in microgateway")
}

// GetActiveDatasources returns active datasources (empty for microgateway)
func (a *GatewayServiceAdapter) GetActiveDatasources() ([]models.Datasource, error) {
	return []models.Datasource{}, nil
}

// GetDatasourceByID returns a datasource by ID (not implemented)
func (a *GatewayServiceAdapter) GetDatasourceByID(id uint) (*models.Datasource, error) {
	return nil, fmt.Errorf("datasource with ID %d not found", id)
}

// GetCredentialBySecret validates API tokens and returns a mock credential for compatibility
// The AI Gateway expects this method but we use API tokens, so we validate the token and return credential info
func (a *GatewayServiceAdapter) GetCredentialBySecret(secret string) (*models.Credential, error) {
	secretPrefix := secret
	if len(secret) > 8 {
		secretPrefix = secret[:8]
	}
	log.Debug().Str("secret_prefix", secretPrefix).Msg("GatewayServiceAdapter.GetCredentialBySecret() called by AI Gateway")
	
	// First try regular token validation
	if gatewayService, ok := a.gatewayService.(*DatabaseGatewayService); ok {
		log.Debug().Msg("Using DatabaseGatewayService to validate token")
		
		tokenResult, err := gatewayService.ValidateAPIToken(secret)
		if err == nil {
			// Regular token validation succeeded
			log.Info().Uint("token_id", tokenResult.TokenID).Str("token_name", tokenResult.TokenName).Uint("app_id", tokenResult.AppID).Msg("Token validated successfully")

			return &models.Credential{
				ID:     tokenResult.TokenID,
				KeyID:  tokenResult.TokenName,
				Secret: secret,
				Active: true,
			}, nil
		}

		// Regular token validation failed, check if there are any auth plugins
		log.Debug().Err(err).Str("token_prefix", secretPrefix).Msg("Regular token validation failed, checking for auth plugins")
		
		// Check if there are any LLMs with auth plugins
		hasAuthPlugins, pluginErr := a.hasAnyAuthPlugins()
		if pluginErr != nil {
			log.Error().Err(pluginErr).Msg("Failed to check for auth plugins")
			return nil, fmt.Errorf("invalid token: %w", err)
		}

		if hasAuthPlugins {
			// Auth plugins are available, return a plugin credential that will be handled by plugin middleware
			log.Debug().Str("token_prefix", secretPrefix).Msg("Auth plugins available, allowing plugin authentication")
			
			return &models.Credential{
				ID:     999999, // Special ID indicating plugin auth
				KeyID:  "plugin-auth",
				Secret: secret,
				Active: true,
			}, nil
		}

		// No auth plugins available, return original error
		log.Error().Err(err).Str("token_prefix", secretPrefix).Msg("Token validation failed and no auth plugins available")
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	log.Error().Msg("Gateway service type not supported for token validation")
	return nil, fmt.Errorf("gateway service type not supported for token validation")
}

// AuthenticateUser authenticates a user (not implemented for microgateway)
func (a *GatewayServiceAdapter) AuthenticateUser(email, password string) (*models.User, error) {
	return nil, fmt.Errorf("user authentication not supported in microgateway")
}

// hasAnyAuthPlugins checks if there are any active auth plugins in the system
func (a *GatewayServiceAdapter) hasAnyAuthPlugins() (bool, error) {
	// Check if there are any active auth plugins in the system
	var count int64
	err := a.pluginService.(*PluginService).db.Table("plugins").
		Where("hook_type = ? AND is_active = ?", "auth", true).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check for auth plugins: %w", err)
	}

	return count > 0, nil
}

// GetUserByAPIKey returns a user by API key (not implemented)
func (a *GatewayServiceAdapter) GetUserByAPIKey(apiKey string) (*models.User, error) {
	return nil, fmt.Errorf("user API key authentication not supported in microgateway")
}

// GetUserByEmail returns a user by email (not implemented)
func (a *GatewayServiceAdapter) GetUserByEmail(email string) (*models.User, error) {
	return nil, fmt.Errorf("user lookup by email not supported in microgateway")
}

// GetUserByID returns a user by ID (simplified implementation)
func (a *GatewayServiceAdapter) GetUserByID(id uint, preload ...string) (*models.User, error) {
	// Return a basic user for compatibility
	return &models.User{
		ID:    id,
		Email: fmt.Sprintf("user%d@microgateway.local", id),
		Name:  fmt.Sprintf("User %d", id),
	}, nil
}

// AddUserToGroup adds a user to a group (not implemented)
func (a *GatewayServiceAdapter) AddUserToGroup(userID, groupID uint) error {
	return fmt.Errorf("user group management not supported in microgateway")
}

// GetValidAccessTokenByToken returns an access token (not implemented)
func (a *GatewayServiceAdapter) GetValidAccessTokenByToken(token string) (*models.AccessToken, error) {
	return nil, fmt.Errorf("OAuth access tokens not supported in microgateway")
}

// GetOAuthClient returns an OAuth client (not implemented)
func (a *GatewayServiceAdapter) GetOAuthClient(clientID string) (*models.OAuthClient, error) {
	return nil, fmt.Errorf("OAuth clients not supported in microgateway")
}

// GetAppByCredentialID returns an app by credential ID
// NOTE: In our token-only system, the "credential ID" is actually a token ID from our mock credential
func (a *GatewayServiceAdapter) GetAppByCredentialID(credID uint) (*models.App, error) {
	log.Debug().Uint("credential_id", credID).Msg("GatewayServiceAdapter.GetAppByCredentialID() called by AI Gateway")
	
	// Check if this is the special plugin auth credential ID
	if credID == 999999 {
		log.Debug().Msg("Plugin auth credential detected, delegating to auth plugin for app lookup")
		
		// TODO: Delegate to auth plugin for app lookup
		// For now, return a default app until we implement plugin delegation
		return &models.App{
			ID:   1, // Default app ID for plugin auth
			Name: "Plugin Authenticated App",
		}, nil
	}
	
	// Since we're using token-only auth, the credID is actually a token ID
	// We need to get the app ID from the token record, not from credentials table
	if gatewayService, ok := a.gatewayService.(*DatabaseGatewayService); ok {
		log.Debug().Msg("Looking up app by token ID (not credential ID)")
		
		app, err := gatewayService.GetAppByTokenID(credID)
		if err != nil {
			log.Error().Err(err).Uint("token_id", credID).Msg("Failed to get app by token ID")
			return nil, fmt.Errorf("app not found for token ID %d: %w", credID, err)
		}

		modelApp := a.convertDatabaseAppToModel(app)
		log.Info().Uint("app_id", modelApp.ID).Str("app_name", modelApp.Name).Msg("Successfully retrieved app for token")
		return &modelApp, nil
	}

	log.Error().Uint("credential_id", credID).Msg("Gateway service type not supported")
	return nil, fmt.Errorf("gateway service type not supported for app lookup")
}

// GetToolByID returns a tool by ID (not implemented)
func (a *GatewayServiceAdapter) GetToolByID(id uint) (*models.Tool, error) {
	return nil, fmt.Errorf("tool with ID %d not found", id)
}

// GetToolBySlug returns a tool by slug (not implemented)
func (a *GatewayServiceAdapter) GetToolBySlug(slug string) (*models.Tool, error) {
	return nil, fmt.Errorf("tool with slug %s not found", slug)
}

// CallToolOperation executes a tool operation (not implemented)
func (a *GatewayServiceAdapter) CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	return nil, fmt.Errorf("tool operations not supported in microgateway")
}

// GetModelPriceByModelNameAndVendor returns model pricing from database
func (a *GatewayServiceAdapter) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	// Look up pricing in database using management service
	dbPrice, err := a.management.GetModelPrice(modelName, vendor)
	
	if err != nil {
		// Return default pricing if not found
		log.Debug().Str("model", modelName).Str("vendor", vendor).Msg("No pricing found, using default rates")
		return &models.ModelPrice{
			ID:        0,
			ModelName: modelName,
			Vendor:    vendor,
			CPT:       0.0003,  // Default: $0.0003 per 1K prompt tokens
			CPIT:      0.0015,  // Default: $0.0015 per 1K completion tokens
			Currency:  "USD",
		}, nil
	}

	// Convert database model to midsommar model
	return &models.ModelPrice{
		ID:           dbPrice.ID,
		ModelName:    dbPrice.ModelName,
		Vendor:       dbPrice.Vendor,
		CPT:          dbPrice.CPT,
		CPIT:         dbPrice.CPIT,
		CacheWritePT: dbPrice.CacheWritePT,
		CacheReadPT:  dbPrice.CacheReadPT,
		Currency:     dbPrice.Currency,
	}, nil
}

// GetFilterByID returns a filter by ID
func (a *GatewayServiceAdapter) GetFilterByID(id uint) (*models.Filter, error) {
	dbFilter, err := a.filterService.GetFilter(id)
	if err != nil {
		return nil, err
	}

	return &models.Filter{
		ID:          dbFilter.ID,
		Name:        dbFilter.Name,
		Description: dbFilter.Description,
		Script:      []byte(dbFilter.Script),
	}, nil
}

// GetAllFilters returns all filters with pagination
func (a *GatewayServiceAdapter) GetAllFilters(pageSize int, pageNumber int, all bool) ([]models.Filter, int64, int, error) {
	// Calculate limit and offset
	limit := pageSize
	if all {
		limit = 1000 // Large number to get all
	}
	
	dbFilters, total, err := a.filterService.ListFilters(pageNumber, limit, true)
	if err != nil {
		return nil, 0, 0, err
	}

	// Convert database filters to models
	modelFilters := make([]models.Filter, len(dbFilters))
	for i, dbFilter := range dbFilters {
		modelFilters[i] = models.Filter{
			ID:          dbFilter.ID,
			Name:        dbFilter.Name,
			Description: dbFilter.Description,
			Script:      []byte(dbFilter.Script),
		}
	}

	totalPages := 1
	if pageSize > 0 && !all {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return modelFilters, total, totalPages, nil
}

// Conversion helper functions
func (a *GatewayServiceAdapter) convertDatabaseLLMToModel(dbLLM *database.LLM) models.LLM {
	// Decrypt the API key for the AI Gateway to use
	apiKey := ""
	if dbLLM.APIKeyEncrypted != "" {
		decryptedKey, err := a.crypto.Decrypt(dbLLM.APIKeyEncrypted)
		if err != nil {
			log.Error().Err(err).Uint("llm_id", dbLLM.ID).Msg("Failed to decrypt API key")
			apiKey = "" // Don't provide invalid key
		} else {
			apiKey = decryptedKey
		}
	}

	// Convert associated filters
	filters := make([]*models.Filter, len(dbLLM.Filters))
	for i, dbFilter := range dbLLM.Filters {
		filters[i] = &models.Filter{
			ID:          dbFilter.ID,
			Name:        dbFilter.Name,
			Description: dbFilter.Description,
			Script:      []byte(dbFilter.Script),
		}
	}

	llm := models.LLM{
		ID:            dbLLM.ID,
		Name:          dbLLM.Slug, // Use slug as name for AI Gateway routing
		Vendor:        models.Vendor(dbLLM.Vendor),
		APIKey:        apiKey,
		APIEndpoint:   dbLLM.Endpoint,
		DefaultModel:  dbLLM.DefaultModel,
		Active:        dbLLM.IsActive,
		MonthlyBudget: &dbLLM.MonthlyBudget,
		Filters:       filters,
	}

	log.Debug().
		Uint("llm_id", llm.ID).
		Str("name", llm.Name).
		Str("vendor", string(llm.Vendor)).
		Str("endpoint", llm.APIEndpoint).
		Bool("active", llm.Active).
		Msg("Converted database LLM to models.LLM")

	return llm
}

func (a *GatewayServiceAdapter) convertDatabaseCredentialToModel(dbCred *database.Credential) models.Credential {
	return models.Credential{
		ID:     dbCred.ID,
		KeyID:  dbCred.KeyID,
		Secret: dbCred.SecretHash, // Note: this is the hashed version
		Active: dbCred.IsActive,
	}
}

func (a *GatewayServiceAdapter) convertDatabaseAppToModel(dbApp *database.App) models.App {
	return models.App{
		ID:              dbApp.ID,
		Name:            dbApp.Name,
		Description:     dbApp.Description,
		UserID:          1, // Default user ID for microgateway
		CredentialID:    dbApp.ID, // Use app ID as credential reference
		MonthlyBudget:   &dbApp.MonthlyBudget,
		BudgetStartDate: dbApp.BudgetStartDate,
	}
}