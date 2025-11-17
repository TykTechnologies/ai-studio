// internal/services/edge_sync_service.go
package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// EdgeSyncService handles syncing flattened configuration to local SQLite
type EdgeSyncService struct {
	db        *gorm.DB
	namespace string
}

// NewEdgeSyncService creates a new edge sync service
func NewEdgeSyncService(db *gorm.DB, namespace string) *EdgeSyncService {
	return &EdgeSyncService{
		db:        db,
		namespace: namespace,
	}
}

// SyncConfiguration syncs flattened configuration to local SQLite with join table recreation
func (s *EdgeSyncService) SyncConfiguration(config *pb.ConfigurationSnapshot) error {
	log.Info().
		Str("version", config.Version).
		Str("namespace", s.namespace).
		Int("llm_count", len(config.Llms)).
		Int("app_count", len(config.Apps)).
		Msg("Starting configuration sync to local SQLite")

	// Start transaction for atomic sync
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	// 1. Clear existing data for this namespace (and global)
	if err := s.clearExistingData(tx); err != nil {
		return fmt.Errorf("failed to clear existing data: %w", err)
	}

	// 2. Sync LLMs with embedded relationships
	if err := s.syncLLMs(tx, config.Llms); err != nil {
		return fmt.Errorf("failed to sync LLMs: %w", err)
	}

	// 3. Sync Apps with embedded relationships (THE CRITICAL PART)
	if err := s.syncApps(tx, config.Apps); err != nil {
		return fmt.Errorf("failed to sync Apps: %w", err)
	}

	// 4. Sync other critical entities
	if err := s.syncFilters(tx, config.Filters); err != nil {
		return fmt.Errorf("failed to sync Filters: %w", err)
	}
	
	if err := s.syncPlugins(tx, config.Plugins); err != nil {
		return fmt.Errorf("failed to sync Plugins: %w", err)
	}

	if err := s.syncModelPrices(tx, config.ModelPrices); err != nil {
		return fmt.Errorf("failed to sync ModelPrices: %w", err)
	}

	// 5. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit sync transaction: %w", err)
	}

	log.Info().
		Str("version", config.Version).
		Str("namespace", s.namespace).
		Msg("Configuration sync to local SQLite completed successfully")

	return nil
}

// clearExistingData clears existing configuration for this namespace
func (s *EdgeSyncService) clearExistingData(tx *gorm.DB) error {
	log.Info().Str("namespace", s.namespace).Msg("Clearing existing configuration data")

	// Clear join tables first (foreign key constraints)
	if err := tx.Exec("DELETE FROM app_llms WHERE app_id IN (SELECT id FROM apps WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear app_llms: %w", err)
	}
	
	if err := tx.Exec("DELETE FROM llm_plugins WHERE llm_id IN (SELECT id FROM llms WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear llm_plugins: %w", err)
	}
	
	if err := tx.Exec("DELETE FROM llm_filters WHERE llm_id IN (SELECT id FROM llms WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear llm_filters: %w", err)
	}

	// Clear main tables
	if err := tx.Exec("DELETE FROM apps WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear apps: %w", err)
	}
	
	if err := tx.Exec("DELETE FROM llms WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear llms: %w", err)
	}
	
	if err := tx.Exec("DELETE FROM filters WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear filters: %w", err)
	}
	
	if err := tx.Exec("DELETE FROM plugins WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear plugins: %w", err)
	}

	if err := tx.Exec("DELETE FROM model_prices WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear model_prices: %w", err)
	}

	log.Info().Msg("Existing configuration data cleared")
	return nil
}

// syncLLMs syncs LLM entities and their join table relationships
func (s *EdgeSyncService) syncLLMs(tx *gorm.DB, llms []*pb.LLMConfig) error {
	log.Info().Int("count", len(llms)).Msg("Syncing LLMs to local SQLite")

	for _, pbLLM := range llms {
		// Insert main LLM record
		llm := &database.LLM{
			Model: gorm.Model{
				ID:        uint(pbLLM.Id),
				CreatedAt: pbLLM.CreatedAt.AsTime(),
				UpdatedAt: pbLLM.UpdatedAt.AsTime(),
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
		}

		// Handle JSON fields with proper conversion
		if pbLLM.Metadata != "" {
			llm.Metadata = datatypes.JSON(pbLLM.Metadata)
		}
		if pbLLM.AllowedModels != "" {
			llm.AllowedModels = datatypes.JSON(pbLLM.AllowedModels)
		}
		if pbLLM.AuthConfig != "" {
			llm.AuthConfig = datatypes.JSON(pbLLM.AuthConfig)
		}
		if pbLLM.AuthMechanism != "" {
			llm.AuthMechanism = pbLLM.AuthMechanism
		}

		if err := tx.Create(llm).Error; err != nil {
			return fmt.Errorf("failed to insert LLM %d: %w", pbLLM.Id, err)
		}

		log.Debug().
			Uint32("llm_id", pbLLM.Id).
			Str("llm_slug", pbLLM.Slug).
			Msg("LLM synced to SQLite")
	}

	return nil
}

// syncApps syncs App entities and recreates app_llms join table - THE CRITICAL PART
func (s *EdgeSyncService) syncApps(tx *gorm.DB, apps []*pb.AppConfig) error {
	log.Info().Int("count", len(apps)).Msg("Syncing Apps to local SQLite")

	for _, pbApp := range apps {
		// Insert main App record
		app := &database.App{
			Model: gorm.Model{
				ID:        uint(pbApp.Id),
				CreatedAt: pbApp.CreatedAt.AsTime(),
				UpdatedAt: pbApp.UpdatedAt.AsTime(),
			},
			Name:            pbApp.Name,
			Description:     pbApp.Description,
			OwnerEmail:      pbApp.OwnerEmail,
			UserID:          uint(pbApp.UserId), // Owner user ID (synced from control plane for analytics)
			IsActive:        pbApp.IsActive,
			MonthlyBudget:   pbApp.MonthlyBudget,
			BudgetResetDay:  int(pbApp.BudgetResetDay),
			RateLimitRPM:    int(pbApp.RateLimitRpm),
			Namespace:       pbApp.Namespace,
		}

		// Handle JSON fields with proper conversion
		if pbApp.AllowedIps != "" {
			app.AllowedIPs = datatypes.JSON(pbApp.AllowedIps)
		}
		if pbApp.Metadata != "" {
			app.Metadata = datatypes.JSON(pbApp.Metadata)
		}

		// Handle budget start date if available  
		if pbApp.BudgetStartDate != "" {
			if startDate, err := time.Parse(time.RFC3339, pbApp.BudgetStartDate); err == nil {
				app.BudgetStartDate = &startDate
			}
		}

		if err := tx.Create(app).Error; err != nil {
			return fmt.Errorf("failed to insert App %d: %w", pbApp.Id, err)
		}

		// Recreate app_llms join table relationships - THE CRITICAL MISSING PIECE
		for _, llmID := range pbApp.LlmIds {
			appLLM := &database.AppLLM{
				AppID:    uint(pbApp.Id),
				LLMID:    uint(llmID),
				IsActive: true,
				CreatedAt: time.Now(),
			}

			if err := tx.Create(appLLM).Error; err != nil {
				return fmt.Errorf("failed to create app_llm relationship (app=%d, llm=%d): %w", pbApp.Id, llmID, err)
			}

			log.Debug().
				Uint32("app_id", pbApp.Id).
				Uint32("llm_id", llmID).
				Msg("Created app_llm relationship in SQLite")
		}

		log.Debug().
			Uint32("app_id", pbApp.Id).
			Str("app_name", pbApp.Name).
			Int("llm_access_count", len(pbApp.LlmIds)).
			Msg("App synced to SQLite with LLM access relationships")
	}

	return nil
}

// syncFilters syncs Filter entities
func (s *EdgeSyncService) syncFilters(tx *gorm.DB, filters []*pb.FilterConfig) error {
	log.Info().Int("count", len(filters)).Msg("Syncing Filters to local SQLite")

	for _, pbFilter := range filters {
		filter := &database.Filter{
			ID:          uint(pbFilter.Id),
			Name:        pbFilter.Name,
			Description: pbFilter.Description,
			Script:      pbFilter.Script,
			IsActive:    pbFilter.IsActive,
			OrderIndex:  int(pbFilter.OrderIndex),
			Namespace:   pbFilter.Namespace,
			CreatedAt:   pbFilter.CreatedAt.AsTime(),
			UpdatedAt:   pbFilter.UpdatedAt.AsTime(),
		}

		if err := tx.Create(filter).Error; err != nil {
			return fmt.Errorf("failed to insert Filter %d: %w", pbFilter.Id, err)
		}

		// Recreate llm_filters join table relationships
		for _, llmID := range pbFilter.LlmIds {
			llmFilter := &database.LLMFilter{
				LLMID:      uint(llmID),
				FilterID:   uint(pbFilter.Id),
				IsActive:   true,
				OrderIndex: int(pbFilter.OrderIndex),
			}

			if err := tx.Create(llmFilter).Error; err != nil {
				return fmt.Errorf("failed to create llm_filter relationship (llm=%d, filter=%d): %w", llmID, pbFilter.Id, err)
			}
		}
	}

	return nil
}

// syncPlugins syncs Plugin entities  
func (s *EdgeSyncService) syncPlugins(tx *gorm.DB, plugins []*pb.PluginConfig) error {
	log.Info().Int("count", len(plugins)).Msg("Syncing Plugins to local SQLite")

	for _, pbPlugin := range plugins {
		plugin := &database.Plugin{
			ID:          uint(pbPlugin.Id),
			Name:        pbPlugin.Name,
			Description: pbPlugin.Description,
			Command:     pbPlugin.Command,
			Checksum:    pbPlugin.Checksum,
			HookType:    pbPlugin.HookType,
			IsActive:    pbPlugin.IsActive,
			Namespace:   pbPlugin.Namespace,
			CreatedAt:   pbPlugin.CreatedAt.AsTime(),
			UpdatedAt:   pbPlugin.UpdatedAt.AsTime(),
		}

		// Handle Config JSON field with proper conversion
		if pbPlugin.Config != "" {
			plugin.Config = datatypes.JSON(pbPlugin.Config)
		}

		// Handle HookTypes JSON field with proper conversion
		if len(pbPlugin.HookTypes) > 0 {
			hookTypesJSON, err := json.Marshal(pbPlugin.HookTypes)
			if err == nil {
				plugin.HookTypes = datatypes.JSON(hookTypesJSON)
			}
		}

		// Handle ServiceScopes JSON field with proper conversion
		if len(pbPlugin.ServiceScopes) > 0 {
			scopesJSON, err := json.Marshal(pbPlugin.ServiceScopes)
			if err == nil {
				plugin.ServiceScopes = datatypes.JSON(scopesJSON)
			}
		}

		if err := tx.Create(plugin).Error; err != nil {
			return fmt.Errorf("failed to insert Plugin %d: %w", pbPlugin.Id, err)
		}

		// Recreate llm_plugins join table relationships with order preservation
		for index, llmID := range pbPlugin.LlmIds {
			llmPlugin := &database.LLMPlugin{
				LLMID:      uint(llmID),
				PluginID:   uint(pbPlugin.Id),
				IsActive:   true,
				OrderIndex: index, // Use position in slice as order index
				CreatedAt:  time.Now(),
			}

			if err := tx.Create(llmPlugin).Error; err != nil {
				return fmt.Errorf("failed to create llm_plugin relationship (llm=%d, plugin=%d): %w", llmID, pbPlugin.Id, err)
			}
		}
	}

	return nil
}

// syncModelPrices syncs ModelPrice entities
func (s *EdgeSyncService) syncModelPrices(tx *gorm.DB, modelPrices []*pb.ModelPriceConfig) error {
	log.Info().Int("count", len(modelPrices)).Msg("Syncing Model Prices to local SQLite")

	for _, pbPrice := range modelPrices {
		// Insert ModelPrice record
		price := &database.ModelPrice{
			Model: gorm.Model{
				ID:        uint(pbPrice.Id),
				CreatedAt: pbPrice.CreatedAt.AsTime(),
				UpdatedAt: pbPrice.UpdatedAt.AsTime(),
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

		if err := tx.Create(price).Error; err != nil {
			return fmt.Errorf("failed to insert ModelPrice %d: %w", pbPrice.Id, err)
		}

		log.Debug().
			Uint32("price_id", pbPrice.Id).
			Str("vendor", pbPrice.Vendor).
			Str("model", pbPrice.ModelName).
			Msg("Model price synced to SQLite")
	}

	return nil
}