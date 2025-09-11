// internal/services/hybrid_gateway_service.go
package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// HybridGatewayService wraps DatabaseGatewayService and overrides token validation
// for on-demand validation while keeping all other operations local (database-backed)
type HybridGatewayService struct {
	*DatabaseGatewayService                                  // Embed DatabaseGatewayService for all other methods
	edgeClient interface{}                                   // Edge client for on-demand token validation
	edgeNamespace string                                     // Edge namespace for token validation
}

// NewHybridGatewayService creates a hybrid gateway service for edge instances
func NewHybridGatewayService(db *gorm.DB, repo *database.Repository, edgeNamespace string) *HybridGatewayService {
	dbService := NewDatabaseGatewayService(db, repo)
	
	return &HybridGatewayService{
		DatabaseGatewayService: dbService.(*DatabaseGatewayService),
		edgeNamespace:         edgeNamespace,
	}
}

// SetEdgeClient sets the edge client reference for on-demand token validation
func (h *HybridGatewayService) SetEdgeClient(edgeClient interface{}) {
	h.edgeClient = edgeClient
	log.Info().Msg("Edge client set for hybrid gateway service on-demand token validation")
}

// ValidateAPIToken overrides DatabaseGatewayService to use on-demand validation
func (h *HybridGatewayService) ValidateAPIToken(token string) (*TokenValidationResult, error) {
	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}

	log.Info().Str("token_prefix", tokenPrefix).Msg("HybridGatewayService: using on-demand token validation")

	if h.edgeClient == nil {
		return nil, fmt.Errorf("edge client not available for on-demand token validation")
	}

	// Cast to edge client and make validation call
	if edgeClient, ok := h.edgeClient.(interface{ ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) }); ok {
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

		// Get the app from local SQLite database (now has full relationships!)
		app, err := h.DatabaseGatewayService.GetAppByTokenID(uint(resp.AppId))
		if err != nil {
			// App not found in SQLite, create a minimal app object with the validated app_id
			log.Info().Uint32("app_id", resp.AppId).Msg("App not found in local SQLite, using minimal app object")
			
			// Create minimal app - the app_llm relationships should exist from sync
			var dbApp database.App
			if err := h.db.Where("id = ?", resp.AppId).Preload("LLMs").First(&dbApp).Error; err != nil {
				return nil, fmt.Errorf("app %d not found in synced SQLite: %w", resp.AppId, err)
			}
			
			app = &dbApp
		}

		return &TokenValidationResult{
			TokenID:   uint(resp.AppId), // Use app_id as pseudo token ID  
			TokenName: "on-demand-validated",
			AppID:     uint(resp.AppId),
			App:       app,
		}, nil
	}

	return nil, fmt.Errorf("edge client does not support token validation")
}

// GetAppByTokenID overrides to handle pseudo token IDs from on-demand validation
func (h *HybridGatewayService) GetAppByTokenID(tokenID uint) (*database.App, error) {
	log.Info().Uint("token_id", tokenID).Msg("HybridGatewayService.GetAppByTokenID called")

	// For on-demand validation, token_id equals app_id
	// Get the app directly from local SQLite (now has full relationships!)
	var app database.App
	if err := h.db.Where("id = ?", tokenID).Preload("LLMs").First(&app).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Info().Uint("app_id", tokenID).Msg("App not found in local SQLite")
			return nil, fmt.Errorf("app not found: %d", tokenID)
		}
		return nil, fmt.Errorf("failed to get app from SQLite: %w", err)
	}

	log.Info().
		Uint("token_id", tokenID).
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Int("llm_count", len(app.LLMs)).
		Msg("Successfully found app with LLM relationships from synced SQLite")

	return &app, nil
}