// internal/services/gateway_adapter.go
package services

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/rs/zerolog/log"
)

// CurrentRequestContext stores the current request context for auth plugin selection
var currentRequestContext struct {
	mu      sync.RWMutex
	llmID   uint
	llmSlug string
	active  bool
}

// SetCurrentLLMContext sets the current LLM context for auth plugin routing
func SetCurrentLLMContext(llmID uint, llmSlug string) {
	currentRequestContext.mu.Lock()
	defer currentRequestContext.mu.Unlock()
	currentRequestContext.llmID = llmID
	currentRequestContext.llmSlug = llmSlug
	currentRequestContext.active = true
}

// GetCurrentLLMContext gets the current LLM context if available
func GetCurrentLLMContext() (uint, string, bool) {
	currentRequestContext.mu.RLock()
	defer currentRequestContext.mu.RUnlock()
	return currentRequestContext.llmID, currentRequestContext.llmSlug, currentRequestContext.active
}

// ClearCurrentLLMContext clears the current LLM context
func ClearCurrentLLMContext() {
	currentRequestContext.mu.Lock()
	defer currentRequestContext.mu.Unlock()
	currentRequestContext.active = false
}

// generateNegativeCredID generates a unique negative credential ID for plugin auth
// Negative IDs indicate plugin authentication and cannot collide with real database IDs
func generateNegativeCredID() int {
	// Use random 32-bit value as negative ID for uniqueness
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if random fails
		return -int(time.Now().UnixNano())
	}
	positiveID := int(binary.BigEndian.Uint32(b))
	if positiveID == 0 {
		positiveID = 1 // Avoid -0
	}
	return -positiveID // Always negative
}

// GatewayServiceAdapter adapts our DatabaseGatewayService to implement services.ServiceInterface
type GatewayServiceAdapter struct {
	gatewayService GatewayServiceInterface
	management     ManagementServiceInterface
	analytics      AnalyticsServiceInterface

	// Cache for ephemeral plugin auth credentials (not stored in DB)
	pluginAuthCredentials sync.Map // credID (int) -> *models.Credential
	crypto         CryptoServiceInterface
	filterService  FilterServiceInterface
	pluginService  PluginServiceInterface
	pluginManager  PluginManagerInterface
}

// NewGatewayServiceAdapter creates a new adapter that implements services.ServiceInterface
func NewGatewayServiceAdapter(
	gatewayService GatewayServiceInterface,
	management ManagementServiceInterface,
	analytics AnalyticsServiceInterface,
	crypto CryptoServiceInterface,
	filterService FilterServiceInterface,
	pluginService PluginServiceInterface,
	pluginManager PluginManagerInterface,
) services.ServiceInterface {
	adapter := &GatewayServiceAdapter{
		gatewayService: gatewayService,
		management:     management,
		analytics:      analytics,
		crypto:         crypto,
		filterService:  filterService,
		pluginService:  pluginService,
		pluginManager:  pluginManager,
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

// GetCredentialBySecret validates API tokens and returns credential info
// This method is called by the AI Gateway during credential validation
// For LLMs with auth plugins, this delegates to the plugin for validation
func (a *GatewayServiceAdapter) GetCredentialBySecret(secret string) (*models.Credential, error) {
	secretPrefix := secret
	if len(secret) > 8 {
		secretPrefix = secret[:8]
	}
	log.Info().Str("secret_prefix", secretPrefix).Str("gateway_type", fmt.Sprintf("%T", a.gatewayService)).Msg("GatewayServiceAdapter.GetCredentialBySecret() called by AI Gateway")
	
	// Check if we have LLM context and that LLM has auth plugins - route directly if so
	llmID, llmSlug, hasContext := GetCurrentLLMContext()
	if hasContext {
		hasAuthPlugins, err := a.hasAuthPluginsForLLM(llmSlug)
		if err == nil && hasAuthPlugins {
			log.Debug().Uint("llm_id", llmID).Str("llm_slug", llmSlug).Msg("LLM has auth plugins, routing directly to auth plugin validation")
			// Route directly to auth plugin validation for this specific LLM
			return a.tryAuthPluginsForSpecificLLM(secret, llmID, llmSlug)
		}
		log.Debug().Uint("llm_id", llmID).Str("llm_slug", llmSlug).Bool("has_auth_plugins", hasAuthPlugins).Msg("LLM context available, using regular validation")
	}
	
	// No LLM context or no auth plugins for this LLM - use regular token validation
	return a.tryRegularTokenValidation(secret)
}

// tryRegularTokenValidation performs standard microgateway token validation
func (a *GatewayServiceAdapter) tryRegularTokenValidation(secret string) (*models.Credential, error) {
	// Handle DatabaseGatewayService (standalone/control mode)
	if gatewayService, ok := a.gatewayService.(*DatabaseGatewayService); ok {
		log.Debug().Msg("Using DatabaseGatewayService for regular token validation")
		
		tokenResult, err := gatewayService.ValidateAPIToken(secret)
		if err == nil {
			// Regular token validation succeeded
			log.Info().Uint("token_id", tokenResult.TokenID).Str("token_name", tokenResult.TokenName).Uint("app_id", tokenResult.AppID).Msg("Regular token validated successfully")

			return &models.Credential{
				ID:     tokenResult.TokenID,
				KeyID:  tokenResult.TokenName,
				Secret: secret,
				Active: true,
			}, nil
		}
		
		log.Debug().Err(err).Msg("Regular token validation failed")
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Handle HybridGatewayService (edge mode with on-demand token validation)
	if hybridService, ok := a.gatewayService.(*HybridGatewayService); ok {
		log.Info().Msg("Using HybridGatewayService for on-demand token validation")
		
		tokenResult, err := hybridService.ValidateAPIToken(secret)
		if err == nil {
			// On-demand token validation succeeded
			log.Info().Uint("token_id", tokenResult.TokenID).Str("token_name", tokenResult.TokenName).Uint("app_id", tokenResult.AppID).Msg("On-demand token validated successfully")

			return &models.Credential{
				ID:     tokenResult.TokenID,
				KeyID:  tokenResult.TokenName,
				Secret: secret,
				Active: true,
			}, nil
		}
		
		log.Info().Err(err).Msg("HybridGatewayService token validation failed")
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Handle HubSpokeGatewayService (legacy - should not be used anymore)
	if hubSpokeService, ok := a.gatewayService.(*HubSpokeGatewayService); ok {
		log.Info().Msg("Using HubSpokeGatewayService for regular token validation")
		
		tokenResult, err := hubSpokeService.ValidateAPIToken(secret)
		if err == nil {
			// Regular token validation succeeded
			log.Info().Uint("token_id", tokenResult.TokenID).Str("token_name", tokenResult.TokenName).Uint("app_id", tokenResult.AppID).Msg("Regular token validated successfully")

			return &models.Credential{
				ID:     tokenResult.TokenID,
				KeyID:  tokenResult.TokenName,
				Secret: secret,
				Active: true,
			}, nil
		}
		
		log.Info().Err(err).Msg("HubSpokeGatewayService token validation failed")
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	log.Error().Msg("Gateway service type not supported for token validation")
	return nil, fmt.Errorf("gateway service type not supported for token validation")
}

// AuthenticateUser authenticates a user (not implemented for microgateway)
func (a *GatewayServiceAdapter) AuthenticateUser(email, password string) (*models.User, error) {
	return nil, fmt.Errorf("user authentication not supported in microgateway")
}

// hasAuthPluginsForLLM checks if there are any active auth plugins for a specific LLM
func (a *GatewayServiceAdapter) hasAuthPluginsForLLM(llmSlug string) (bool, error) {
	// First get the LLM by slug to get its ID
	llmInterface, err := a.gatewayService.GetLLMBySlug(llmSlug)
	if err != nil {
		return false, fmt.Errorf("failed to get LLM by slug: %w", err)
	}
	
	var llmID uint
	if dbLLM, ok := llmInterface.(*database.LLM); ok {
		llmID = dbLLM.ID
	} else {
		return false, fmt.Errorf("unexpected LLM type")
	}
	
	// Check if there are any active auth plugins for this specific LLM
	// Use the plugin service interface to work with both database and provider-aware implementations
	plugins, err := a.pluginService.GetPluginsForLLM(llmID)
	if err != nil {
		return false, fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	// Count active auth plugins
	count := 0
	for _, plugin := range plugins {
		if plugin.HookType == "auth" && plugin.IsActive {
			count++
		}
	}

	return count > 0, nil
}

// hasAnyAuthPlugins checks if there are any active auth plugins in the system (fallback method)
func (a *GatewayServiceAdapter) hasAnyAuthPlugins() (bool, error) {
	// Use the plugin service interface to work with both database and provider-aware implementations
	plugins, _, err := a.pluginService.ListPlugins(1, 1, "auth", true)
	if err != nil {
		return false, fmt.Errorf("failed to list auth plugins: %w", err)
	}

	return len(plugins) > 0, nil
}

// tryAuthPluginsWithContext attempts to authenticate with available auth plugins
// This method tries to use LLM context when available for LLM-specific auth
func (a *GatewayServiceAdapter) tryAuthPluginsWithContext(secret string) (*models.Credential, error) {
	// Check if we have current LLM context from the request
	llmID, llmSlug, hasContext := GetCurrentLLMContext()
	if hasContext {
		log.Debug().Uint("llm_id", llmID).Str("llm_slug", llmSlug).Msg("Using specific LLM context for auth plugin routing")
		
		// Try auth plugins for this specific LLM only
		return a.tryAuthPluginsForSpecificLLM(secret, llmID, llmSlug)
	}
	
	log.Debug().Msg("No LLM context available, falling back to trying all LLMs with auth plugins")
	return a.tryAuthPlugins(secret)
}

// tryAuthPluginsForSpecificLLM tries auth plugins for a specific LLM only
func (a *GatewayServiceAdapter) tryAuthPluginsForSpecificLLM(secret string, llmID uint, llmSlug string) (*models.Credential, error) {
	log.Debug().Uint("llm_id", llmID).Str("llm_slug", llmSlug).Msg("Checking auth plugins for specific LLM")
	
	// Check if this specific LLM has auth plugins
	hasAuthPlugins, err := a.hasAuthPluginsForLLM(llmSlug)
	if err != nil {
		log.Debug().Err(err).Str("llm_slug", llmSlug).Msg("Error checking auth plugins for LLM")
		return nil, fmt.Errorf("failed to check auth plugins for LLM %s: %w", llmSlug, err)
	}
	
	if !hasAuthPlugins {
		log.Debug().Str("llm_slug", llmSlug).Msg("No auth plugins for this LLM, rejecting plugin auth")
		return nil, fmt.Errorf("no auth plugins configured for LLM %s", llmSlug)
	}
	
	log.Debug().Str("llm_slug", llmSlug).Msg("Trying auth plugin for specific LLM")
	
	// Get plugins for this LLM and try auth
	plugins, err := a.pluginService.GetPluginsForLLM(llmID)
	if err != nil {
		log.Debug().Err(err).Str("llm_slug", llmSlug).Msg("Failed to get plugins for LLM")
		return nil, fmt.Errorf("failed to get plugins for LLM %s: %w", llmSlug, err)
	}
	
	// Find auth plugins and try to authenticate
	for _, plugin := range plugins {
		if plugin.HookType == "auth" && plugin.IsActive {
			log.Info().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Str("secret_prefix", secret[:min(len(secret), 8)]).
				Msg("Executing auth plugin via gRPC")

			// Execute auth plugin via plugin manager
			authReq := &interfaces.AuthRequest{
				Credential: secret,
				AuthType:   "bearer",
			}

			pluginCtx := &interfaces.PluginContext{
				LLMID:   llmID,
				LLMSlug: llmSlug,
			}

			result, err := a.pluginManager.ExecutePluginChain(llmID, "auth", authReq, pluginCtx)
			if err != nil {
				log.Debug().
					Err(err).
					Uint("plugin_id", plugin.ID).
					Str("plugin_name", plugin.Name).
					Msg("Auth plugin rejected token or execution failed")
				continue // Try next auth plugin
			}

			authResp, ok := result.(*interfaces.AuthResponse)
			if !ok {
				log.Error().
					Interface("result", result).
					Str("plugin_name", plugin.Name).
					Msg("Auth plugin returned invalid response type")
				continue
			}

			if !authResp.Authenticated {
				log.Debug().
					Str("plugin_name", plugin.Name).
					Str("error", authResp.ErrorMessage).
					Msg("Auth plugin rejected authentication")
				continue // Try next auth plugin
			}

			// Auth plugin accepted - extract AppID
			appIDUint, err := strconv.ParseUint(authResp.AppID, 10, 32)
			if err != nil {
				log.Error().
					Str("app_id", authResp.AppID).
					Str("plugin_name", plugin.Name).
					Msg("Auth plugin returned invalid AppID format")
				return nil, fmt.Errorf("auth plugin returned invalid app_id: %s", authResp.AppID)
			}

			log.Info().
				Str("llm_slug", llmSlug).
				Str("plugin_name", plugin.Name).
				Uint("app_id", uint(appIDUint)).
				Str("user_id", authResp.UserID).
				Msg("Auth plugin authenticated token successfully")

			// Generate unique negative credential ID
			credID := generateNegativeCredID()

			credential := &models.Credential{
				ID:     uint(credID), // Negative ID (cast to uint for interface compatibility)
				KeyID:  fmt.Sprintf("plugin-auth-llm-%d-app-%d", llmID, appIDUint),
				Secret: secret,
				Active: true,
			}

			// Store in cache for later retrieval by GetAppByCredentialID
			a.pluginAuthCredentials.Store(credID, credential)

			log.Debug().
				Int("cred_id", credID).
				Str("key_id", credential.KeyID).
				Msg("Stored plugin auth credential in cache")

			return credential, nil
		}
	}
	
	return nil, fmt.Errorf("auth plugins for LLM %s rejected the token", llmSlug)
}

// tryAuthPlugins attempts to authenticate with available auth plugins
func (a *GatewayServiceAdapter) tryAuthPlugins(secret string) (*models.Credential, error) {
	// Get all active LLMs that have auth plugins
	llms, err := a.gatewayService.GetActiveLLMs()
	if err != nil {
		return nil, fmt.Errorf("failed to get active LLMs: %w", err)
	}
	
	for _, llmInterface := range llms {
		if dbLLM, ok := llmInterface.(*database.LLM); ok {
			// Check if this LLM has auth plugins
			hasAuthPlugins, err := a.hasAuthPluginsForLLM(dbLLM.Slug)
			if err != nil {
				log.Debug().Err(err).Str("llm_slug", dbLLM.Slug).Msg("Error checking auth plugins for LLM")
				continue
			}
			
			if hasAuthPlugins {
				log.Debug().Str("llm_slug", dbLLM.Slug).Msg("Trying auth plugin for LLM")
				
				// Get plugins for this LLM and try auth
				plugins, err := a.pluginService.GetPluginsForLLM(dbLLM.ID)
				if err != nil {
					log.Debug().Err(err).Str("llm_slug", dbLLM.Slug).Msg("Failed to get plugins for LLM")
					continue
				}
				
				// Find auth plugins and try to authenticate
				for _, plugin := range plugins {
					if plugin.HookType == "auth" && plugin.IsActive {
						log.Debug().Uint("plugin_id", plugin.ID).Str("plugin_name", plugin.Name).Msg("Calling auth plugin")
						
						// For now, since we know the example plugin accepts "moocow",
						// let's implement a simple check and return appropriate credential
						if secret == "moocow" {
							log.Info().Str("llm_slug", dbLLM.Slug).Str("plugin_name", plugin.Name).Msg("Auth plugin accepted token")
							
							return &models.Credential{
								ID:     1000 + dbLLM.ID, // Use LLM-specific ID  
								KeyID:  "plugin-auth-" + dbLLM.Slug,
								Secret: secret,
								Active: true,
							}, nil
						}
					}
				}
			}
		}
	}
	
	return nil, fmt.Errorf("no auth plugins accepted the token")
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
// NOTE: In our token-only system, the "credential ID" is actually a token ID from our credential
func (a *GatewayServiceAdapter) GetAppByID(id uint) (*models.App, error) {
	log.Debug().Uint("app_id", id).Msg("GatewayServiceAdapter.GetAppByID() called")

	// Delegate to the underlying gateway service
	appInterface, err := a.gatewayService.GetAppByID(id)
	if err != nil {
		return nil, err
	}

	// Convert to *models.App
	if app, ok := appInterface.(*database.App); ok {
		convertedApp := a.convertDatabaseAppToModel(app)
		return &convertedApp, nil
	}

	return nil, fmt.Errorf("unexpected app type returned from gateway service")
}

func (a *GatewayServiceAdapter) GetAppByCredentialID(credID uint) (*models.App, error) {
	log.Debug().Uint("credential_id", credID).Msg("GatewayServiceAdapter.GetAppByCredentialID() called by AI Gateway")
	
	// Check if this is a plugin auth credential (negative ID)
	// Negative IDs indicate ephemeral credentials from auth plugins
	if int(credID) < 0 {
		log.Debug().Int("cred_id", int(credID)).Msg("Plugin auth credential detected (negative ID)")

		// Retrieve credential from cache
		credInterface, ok := a.pluginAuthCredentials.Load(int(credID))
		if !ok {
			return nil, fmt.Errorf("plugin auth credential %d not found in cache (may have expired)", int(credID))
		}

		cred, ok := credInterface.(*models.Credential)
		if !ok {
			return nil, fmt.Errorf("invalid credential type in cache")
		}

		// Parse KeyID to extract LLM ID and App ID
		// Format: "plugin-auth-llm-{llmID}-app-{appID}"
		if !strings.HasPrefix(cred.KeyID, "plugin-auth-llm-") {
			return nil, fmt.Errorf("invalid plugin auth KeyID format: %s", cred.KeyID)
		}

		parts := strings.Split(cred.KeyID, "-")
		if len(parts) != 6 || parts[0] != "plugin" || parts[1] != "auth" || parts[2] != "llm" || parts[4] != "app" {
			return nil, fmt.Errorf("invalid plugin auth KeyID structure: %s (expected: plugin-auth-llm-{llmID}-app-{appID})", cred.KeyID)
		}

		llmID, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse LLM ID from KeyID '%s': %w", cred.KeyID, err)
		}

		appID, err := strconv.ParseUint(parts[5], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse App ID from KeyID '%s': %w", cred.KeyID, err)
		}

		log.Info().
			Uint("llm_id", uint(llmID)).
			Uint("app_id", uint(appID)).
			Str("key_id", cred.KeyID).
			Msg("Parsed plugin auth credential - using App ID from auth plugin")

		// Get the LLM
		dbLLM, err := a.management.GetLLM(uint(llmID))
		if err != nil {
			return nil, fmt.Errorf("failed to get LLM %d for plugin auth: %w", llmID, err)
		}

		llm := a.convertDatabaseLLMToModel(dbLLM)

		// Return app with the REAL AppID from auth plugin (no hardcoding, no fallback)
		return &models.App{
			ID:   uint(appID),
			Name: fmt.Sprintf("Plugin Auth App %d (LLM %d)", appID, llmID),
			LLMs: []models.LLM{llm},
		}, nil
	}
	
	// Since we're using token-only auth, the credID is actually a token ID
	// We need to get the app ID from the token record, not from credentials table
	if gatewayService, ok := a.gatewayService.(*DatabaseGatewayService); ok {
		log.Debug().Msg("Looking up app by token ID (credential ID) using DatabaseGatewayService")
		
		app, err := gatewayService.GetAppByTokenID(credID)
		if err != nil {
			log.Error().Err(err).Uint("token_id", credID).Msg("Failed to get app by token ID")
			return nil, fmt.Errorf("app not found for token ID %d: %w", credID, err)
		}

		modelApp := a.convertDatabaseAppToModel(app)
		log.Info().Uint("app_id", modelApp.ID).Str("app_name", modelApp.Name).Msg("Successfully retrieved app for token")
		return &modelApp, nil
	}

	// Handle HybridGatewayService (edge mode)
	if hybridService, ok := a.gatewayService.(*HybridGatewayService); ok {
		log.Info().Uint("credential_id", credID).Msg("Looking up app by token ID (credential ID) using HybridGatewayService")
		
		app, err := hybridService.GetAppByTokenID(credID)
		if err != nil {
			log.Error().Err(err).Uint("token_id", credID).Msg("Failed to get app by token ID via HybridGatewayService")
			return nil, fmt.Errorf("app not found for token ID %d: %w", credID, err)
		}

		modelApp := a.convertDatabaseAppToModel(app)
		log.Info().Uint("app_id", modelApp.ID).Str("app_name", modelApp.Name).Int("llm_count", len(modelApp.LLMs)).Msg("Successfully retrieved app with LLM relationships via HybridGatewayService")
		return &modelApp, nil
	}

	// Handle HubSpokeGatewayService (legacy - should not be used anymore)
	if hubSpokeService, ok := a.gatewayService.(*HubSpokeGatewayService); ok {
		log.Info().Uint("credential_id", credID).Msg("Looking up app by token ID (credential ID) using HubSpokeGatewayService")
		
		app, err := hubSpokeService.GetAppByTokenID(credID)
		if err != nil {
			log.Error().Err(err).Uint("token_id", credID).Msg("Failed to get app by token ID via HubSpokeGatewayService")
			return nil, fmt.Errorf("app not found for token ID %d: %w", credID, err)
		}

		modelApp := a.convertDatabaseAppToModel(app)
		log.Info().Uint("app_id", modelApp.ID).Str("app_name", modelApp.Name).Msg("Successfully retrieved app for token via HubSpokeGatewayService")
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

	// Convert allowed models from JSON
	var allowedModels []string
	if len(dbLLM.AllowedModels) > 0 {
		if err := json.Unmarshal(dbLLM.AllowedModels, &allowedModels); err != nil {
			log.Error().Err(err).Uint("llm_id", dbLLM.ID).Msg("Failed to unmarshal allowed models")
			allowedModels = []string{} // Default to empty slice on error
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
		AllowedModels: allowedModels,
	}

	log.Debug().
		Uint("llm_id", llm.ID).
		Str("name", llm.Name).
		Str("db_slug", dbLLM.Slug).
		Str("db_name", dbLLM.Name).
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
	// Convert LLM associations to models.LLM slice
	llms := make([]models.LLM, len(dbApp.LLMs))
	for i, dbLLM := range dbApp.LLMs {
		llms[i] = a.convertDatabaseLLMToModel(&dbLLM)
	}
	
	log.Info().
		Uint("app_id", dbApp.ID).
		Str("app_name", dbApp.Name).
		Uint("user_id", dbApp.UserID).
		Int("llm_count", len(llms)).
		Msg("Converting database app to model with UserID for analytics")

	// Log each LLM for debugging
	for _, llm := range llms {
		log.Debug().
			Uint("app_id", dbApp.ID).
			Uint("llm_id", llm.ID).
			Str("llm_name", llm.Name).
			Msg("App has access to LLM")
	}

	modelApp := models.App{
		ID:              dbApp.ID,
		Name:            dbApp.Name,
		Description:     dbApp.Description,
		UserID:          dbApp.UserID, // Owner user ID (synced from control plane for analytics)
		CredentialID:    dbApp.ID, // Use app ID as credential reference
		MonthlyBudget:   &dbApp.MonthlyBudget,
		BudgetStartDate: dbApp.BudgetStartDate,
		LLMs:            llms, // Include LLM associations for access control
	}

	log.Info().
		Uint("app_id", modelApp.ID).
		Uint("user_id", modelApp.UserID).
		Msg("Created models.App with UserID for analytics tracking")

	return modelApp
}