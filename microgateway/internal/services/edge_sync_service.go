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

// calculateBudgetPeriod determines the budget period for an app based on its budget_start_date.
// If no budget_start_date is set, uses calendar month (1st to last day).
// When a budget is reset on the same day, this preserves the exact reset time to ensure
// usage from before the reset is not counted.
// Note: Timestamps are truncated to second precision to ensure consistency across all components.
func calculateBudgetPeriod(budgetStartDate *time.Time, now time.Time) (time.Time, time.Time) {
	if budgetStartDate == nil {
		// Default to calendar month
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)
		return periodStart, periodEnd
	}

	budgetDay := budgetStartDate.Day()
	currentYear := now.Year()
	currentMonth := now.Month()

	// If we haven't reached the budget day in current month,
	// the period started on the budget day of previous month
	if now.Day() < budgetDay {
		if currentMonth == time.January {
			currentMonth = time.December
			currentYear--
		} else {
			currentMonth--
		}
	}

	// Calculate the normalized period start (midnight of the budget day)
	normalizedPeriodStart := time.Date(currentYear, currentMonth, budgetDay, 0, 0, 0, 0, now.Location())
	periodEnd := normalizedPeriodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Check if the actual budget_start_date falls within this period.
	// If it does (e.g., budget was reset mid-period), use the exact timestamp
	// to ensure usage from before the reset is not counted.
	// Truncate to second precision to ensure consistency across control server and edges.
	if budgetStartDate.After(normalizedPeriodStart) && budgetStartDate.Before(periodEnd) {
		truncated := budgetStartDate.Truncate(time.Second)
		return truncated, periodEnd
	}

	return normalizedPeriodStart, periodEnd
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
	log.Debug().
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

	// 5. Sync Model Routers (Enterprise feature)
	if err := s.syncModelRouters(tx, config.ModelRouters); err != nil {
		return fmt.Errorf("failed to sync ModelRouters: %w", err)
	}

	// 6. Sync Tools with filter and app associations
	if err := s.syncTools(tx, config.Tools); err != nil {
		return fmt.Errorf("failed to sync Tools: %w", err)
	}

	// 7. Sync Datasources with app associations
	if err := s.syncDatasources(tx, config.Datasources); err != nil {
		return fmt.Errorf("failed to sync Datasources: %w", err)
	}

	// 8. Sync OAuth Clients (for MCP authentication on edge)
	if err := s.syncOAuthClients(tx, config.OauthClients); err != nil {
		return fmt.Errorf("failed to sync OAuthClients: %w", err)
	}

	// 9. Sync Access Tokens (for MCP authentication on edge)
	if err := s.syncAccessTokens(tx, config.AccessTokens); err != nil {
		return fmt.Errorf("failed to sync AccessTokens: %w", err)
	}

	// 10. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit sync transaction: %w", err)
	}

	log.Debug().
		Str("version", config.Version).
		Str("namespace", s.namespace).
		Msg("Configuration sync to local SQLite completed successfully")

	return nil
}

// clearExistingData clears existing configuration for this namespace
func (s *EdgeSyncService) clearExistingData(tx *gorm.DB) error {
	log.Debug().Str("namespace", s.namespace).Msg("Clearing existing configuration data")

	// Clear ALL join tables first (before main tables, so subqueries still work)
	if err := tx.Exec("DELETE FROM app_llms WHERE app_id IN (SELECT id FROM apps WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear app_llms: %w", err)
	}

	if err := tx.Exec("DELETE FROM llm_plugins WHERE llm_id IN (SELECT id FROM llms WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear llm_plugins: %w", err)
	}

	if err := tx.Exec("DELETE FROM llm_filters WHERE llm_id IN (SELECT id FROM llms WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear llm_filters: %w", err)
	}

	// Tool/Datasource join tables (must clear before apps and tools are deleted)
	if err := tx.Exec("DELETE FROM app_tools WHERE app_id IN (SELECT id FROM apps WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear app_tools: %w", err)
	}
	if err := tx.Exec("DELETE FROM app_datasources WHERE app_id IN (SELECT id FROM apps WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear app_datasources: %w", err)
	}
	if err := tx.Exec("DELETE FROM tool_filters WHERE tool_id IN (SELECT id FROM tools WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear tool_filters: %w", err)
	}

	// Clear main entity tables (after all join tables are cleared)
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

	if err := tx.Exec("DELETE FROM tools WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear tools: %w", err)
	}
	if err := tx.Exec("DELETE FROM datasources WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear datasources: %w", err)
	}

	// Clear OAuth tables (global, not namespaced)
	if err := tx.Exec("DELETE FROM oauth_clients").Error; err != nil {
		return fmt.Errorf("failed to clear oauth_clients: %w", err)
	}
	if err := tx.Exec("DELETE FROM access_tokens").Error; err != nil {
		return fmt.Errorf("failed to clear access_tokens: %w", err)
	}

	// Clear Model Router tables (cascade from routers to pools to vendors/mappings)
	// Note: model_mappings now reference vendor_id, so delete via pool_vendors
	if err := tx.Exec("DELETE FROM model_mappings WHERE vendor_id IN (SELECT id FROM pool_vendors WHERE pool_id IN (SELECT id FROM model_pools WHERE router_id IN (SELECT id FROM model_routers WHERE namespace = ? OR namespace = '')))", s.namespace).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to clear model_mappings (table may not exist)")
	}
	if err := tx.Exec("DELETE FROM pool_vendors WHERE pool_id IN (SELECT id FROM model_pools WHERE router_id IN (SELECT id FROM model_routers WHERE namespace = ? OR namespace = ''))", s.namespace).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to clear pool_vendors (table may not exist)")
	}
	if err := tx.Exec("DELETE FROM model_pools WHERE router_id IN (SELECT id FROM model_routers WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to clear model_pools (table may not exist)")
	}
	if err := tx.Exec("DELETE FROM model_routers WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to clear model_routers (table may not exist)")
	}

	log.Debug().Msg("Existing configuration data cleared")
	return nil
}

// syncLLMs syncs LLM entities and their join table relationships
func (s *EdgeSyncService) syncLLMs(tx *gorm.DB, llms []*pb.LLMConfig) error {
	log.Debug().Int("count", len(llms)).Msg("Syncing LLMs to local SQLite")

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

		// Create llm_filters join table entries for this LLM
		for i, filterID := range pbLLM.FilterIds {
			llmFilter := map[string]interface{}{
				"llm_id":      pbLLM.Id,
				"filter_id":   filterID,
				"is_active":   true,
				"order_index": i,
				"created_at":  time.Now(),
			}
			if err := tx.Table("llm_filters").Create(llmFilter).Error; err != nil {
				return fmt.Errorf("failed to create llm_filter for LLM %d, Filter %d: %w", pbLLM.Id, filterID, err)
			}
		}

		log.Debug().
			Uint32("llm_id", pbLLM.Id).
			Str("llm_slug", pbLLM.Slug).
			Int("filter_count", len(pbLLM.FilterIds)).
			Msg("LLM synced to SQLite with filters")
	}

	return nil
}

// syncApps syncs App entities and recreates app_llms join table - THE CRITICAL PART
func (s *EdgeSyncService) syncApps(tx *gorm.DB, apps []*pb.AppConfig) error {
	log.Debug().Int("count", len(apps)).Msg("Syncing Apps to local SQLite")

	// Collect join table records across all apps for batch insert
	var allAppLLMs []database.AppLLM
	var allAppTools []database.AppTool
	var allAppDatasources []database.AppDatasource

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

		// Serialize plugin resource associations for gateway access
		if len(pbApp.PluginResources) > 0 {
			if prJSON, err := json.Marshal(pbApp.PluginResources); err == nil {
				app.PluginResourcesJSON = datatypes.JSON(prJSON)
			}
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

		// Collect join table records for batch insert
		now := time.Now()
		for _, llmID := range pbApp.LlmIds {
			allAppLLMs = append(allAppLLMs, database.AppLLM{
				AppID: uint(pbApp.Id), LLMID: uint(llmID), IsActive: true, CreatedAt: now,
			})
		}
		for _, toolID := range pbApp.ToolIds {
			allAppTools = append(allAppTools, database.AppTool{
				AppID: uint(pbApp.Id), ToolID: uint(toolID), CreatedAt: now,
			})
		}
		for _, dsID := range pbApp.DatasourceIds {
			allAppDatasources = append(allAppDatasources, database.AppDatasource{
				AppID: uint(pbApp.Id), DatasourceID: uint(dsID), CreatedAt: now,
			})
		}

		log.Debug().
			Uint32("app_id", pbApp.Id).
			Str("app_name", pbApp.Name).
			Int("llm_access_count", len(pbApp.LlmIds)).
			Int("tool_access_count", len(pbApp.ToolIds)).
			Int("ds_access_count", len(pbApp.DatasourceIds)).
			Msg("App synced to SQLite with LLM/Tool/Datasource access relationships")

		// Initialize budget usage from control server's current_period_usage
		// This ensures edge budget enforcement respects usage tracked by the control plane
		// Note: CurrentPeriodUsage comes in dollars, but we store as dollars * 10000 for consistency
		if pbApp.MonthlyBudget > 0 {
			now := time.Now()
			// Use app's BudgetStartDate to calculate the correct budget period
			// This ensures budget resets are properly reflected on the edge
			periodStart, periodEnd := calculateBudgetPeriod(app.BudgetStartDate, now)

			// Convert from dollars (control server format) to dollars * 10000 (edge storage format)
			storedCost := pbApp.CurrentPeriodUsage * 10000

			budgetUsage := &database.BudgetUsage{
				AppID:       uint(pbApp.Id),
				PeriodStart: periodStart,
				PeriodEnd:   periodEnd,
				TotalCost:   storedCost,
			}

			// Use FirstOrCreate to avoid duplicates, and update TotalCost if record exists
			result := tx.Where("app_id = ? AND period_start = ?", pbApp.Id, periodStart).
				Assign(map[string]interface{}{"total_cost": storedCost}).
				FirstOrCreate(budgetUsage)
			if result.Error != nil {
				log.Warn().Err(result.Error).
					Uint32("app_id", pbApp.Id).
					Float64("current_usage", pbApp.CurrentPeriodUsage).
					Float64("stored_cost", storedCost).
					Msg("Failed to initialize budget usage from control server")
			} else {
				log.Debug().
					Uint32("app_id", pbApp.Id).
					Float64("current_usage_dollars", pbApp.CurrentPeriodUsage).
					Float64("stored_cost", storedCost).
					Float64("monthly_budget", pbApp.MonthlyBudget).
					Time("period_start", periodStart).
					Msg("Initialized budget usage from control server snapshot")
			}
		}
	}

	// Batch insert all join table records
	if len(allAppLLMs) > 0 {
		if err := tx.Create(&allAppLLMs).Error; err != nil {
			return fmt.Errorf("failed to batch insert app_llms: %w", err)
		}
	}
	if len(allAppTools) > 0 {
		if err := tx.Create(&allAppTools).Error; err != nil {
			return fmt.Errorf("failed to batch insert app_tools: %w", err)
		}
	}
	if len(allAppDatasources) > 0 {
		if err := tx.Create(&allAppDatasources).Error; err != nil {
			return fmt.Errorf("failed to batch insert app_datasources: %w", err)
		}
	}

	return nil
}

// syncFilters syncs Filter entities
func (s *EdgeSyncService) syncFilters(tx *gorm.DB, filters []*pb.FilterConfig) error {
	log.Debug().Int("count", len(filters)).Msg("Syncing Filters to local SQLite")

	for _, pbFilter := range filters {
		filter := &database.Filter{
			ID:             uint(pbFilter.Id),
			Name:           pbFilter.Name,
			Description:    pbFilter.Description,
			Script:         pbFilter.Script,
			ResponseFilter: pbFilter.ResponseFilter,
			IsActive:       pbFilter.IsActive,
			OrderIndex:     int(pbFilter.OrderIndex),
			Namespace:      pbFilter.Namespace,
			CreatedAt:      pbFilter.CreatedAt.AsTime(),
			UpdatedAt:      pbFilter.UpdatedAt.AsTime(),
		}

		if err := tx.Create(filter).Error; err != nil {
			return fmt.Errorf("failed to insert Filter %d: %w", pbFilter.Id, err)
		}

		// Recreate llm_filters join table relationships
		// Use FirstOrCreate to avoid duplicate key violations since LLMs may have already created these entries
		for _, llmID := range pbFilter.LlmIds {
			llmFilter := &database.LLMFilter{
				LLMID:      uint(llmID),
				FilterID:   uint(pbFilter.Id),
				IsActive:   true,
				OrderIndex: int(pbFilter.OrderIndex),
			}

			// Use FirstOrCreate to handle cases where the relationship was already created by LLM sync
			if err := tx.Where("llm_id = ? AND filter_id = ?", llmID, pbFilter.Id).
				FirstOrCreate(llmFilter).Error; err != nil {
				return fmt.Errorf("failed to create llm_filter relationship (llm=%d, filter=%d): %w", llmID, pbFilter.Id, err)
			}
		}
	}

	return nil
}

// syncPlugins syncs Plugin entities
func (s *EdgeSyncService) syncPlugins(tx *gorm.DB, plugins []*pb.PluginConfig) error {
	log.Debug().Int("count", len(plugins)).Msg("Syncing Plugins to local SQLite")

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
	log.Debug().Int("count", len(modelPrices)).Msg("Syncing Model Prices to local SQLite")

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

// syncModelRouters syncs ModelRouter entities (Enterprise feature)
func (s *EdgeSyncService) syncModelRouters(tx *gorm.DB, modelRouters []*pb.ModelRouterConfig) error {
	log.Debug().Int("count", len(modelRouters)).Msg("Syncing Model Routers to local SQLite")

	for _, pbRouter := range modelRouters {
		// Insert ModelRouter record
		router := &database.ModelRouter{
			ID:          uint(pbRouter.Id),
			Name:        pbRouter.Name,
			Slug:        pbRouter.Slug,
			Description: pbRouter.Description,
			APICompat:   pbRouter.ApiCompat,
			IsActive:    pbRouter.IsActive,
			Namespace:   pbRouter.Namespace,
			CreatedAt:   pbRouter.CreatedAt.AsTime(),
			UpdatedAt:   pbRouter.UpdatedAt.AsTime(),
		}

		if err := tx.Create(router).Error; err != nil {
			return fmt.Errorf("failed to insert ModelRouter %d: %w", pbRouter.Id, err)
		}

		// Insert pools for this router
		for _, pbPool := range pbRouter.Pools {
			pool := &database.ModelPool{
				ID:                 uint(pbPool.Id),
				RouterID:           uint(pbRouter.Id),
				Name:               pbPool.Name,
				ModelPattern:       pbPool.ModelPattern,
				SelectionAlgorithm: pbPool.SelectionAlgorithm,
				Priority:           int(pbPool.Priority),
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}

			if err := tx.Create(pool).Error; err != nil {
				return fmt.Errorf("failed to insert ModelPool %d for router %d: %w", pbPool.Id, pbRouter.Id, err)
			}

			// Insert vendors for this pool (with their mappings)
			for _, pbVendor := range pbPool.Vendors {
				vendor := &database.PoolVendor{
					ID:        uint(pbVendor.Id),
					PoolID:    uint(pbPool.Id),
					LLMID:     uint(pbVendor.LlmId),
					LLMSlug:   pbVendor.LlmSlug,
					Weight:    int(pbVendor.Weight),
					IsActive:  pbVendor.IsActive,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				if err := tx.Create(vendor).Error; err != nil {
					return fmt.Errorf("failed to insert PoolVendor %d for pool %d: %w", pbVendor.Id, pbPool.Id, err)
				}

				// Insert vendor-specific mappings
				for _, pbMapping := range pbVendor.Mappings {
					mapping := &database.ModelMapping{
						ID:          uint(pbMapping.Id),
						VendorID:    uint(pbVendor.Id),
						SourceModel: pbMapping.SourceModel,
						TargetModel: pbMapping.TargetModel,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}

					if err := tx.Create(mapping).Error; err != nil {
						return fmt.Errorf("failed to insert ModelMapping %d for vendor %d: %w", pbMapping.Id, pbVendor.Id, err)
					}
				}
			}
		}

		log.Debug().
			Uint32("router_id", pbRouter.Id).
			Str("router_slug", pbRouter.Slug).
			Int("pool_count", len(pbRouter.Pools)).
			Msg("Model Router synced to SQLite")
	}

	return nil
}

// syncTools syncs Tool entities with filter associations (batch insert)
func (s *EdgeSyncService) syncTools(tx *gorm.DB, tools []*pb.ToolConfig) error {
	if len(tools) == 0 {
		return nil
	}
	log.Debug().Int("count", len(tools)).Msg("Syncing Tools to local SQLite")

	// Build batch of tool records
	toolRecords := make([]database.Tool, 0, len(tools))
	var toolFilterRecords []database.ToolFilter

	for _, pbTool := range tools {
		toolRecords = append(toolRecords, database.Tool{
			Model: gorm.Model{
				ID:        uint(pbTool.Id),
				CreatedAt: pbTool.CreatedAt.AsTime(),
				UpdatedAt: pbTool.UpdatedAt.AsTime(),
			},
			Name:                pbTool.Name,
			Slug:                pbTool.Slug,
			Description:         pbTool.Description,
			ToolType:            pbTool.ToolType,
			OASSpec:             pbTool.OasSpec,
			AvailableOperations: pbTool.AvailableOperations,
			PrivacyScore:        int(pbTool.PrivacyScore),
			AuthKeyEncrypted:    pbTool.AuthKeyEncrypted,
			AuthSchemaName:      pbTool.AuthSchemaName,
			Active:              pbTool.IsActive,
			Namespace:           pbTool.Namespace,
		})

		// Collect tool_filter join table entries
		now := time.Now()
		for _, filterID := range pbTool.FilterIds {
			toolFilterRecords = append(toolFilterRecords, database.ToolFilter{
				ToolID:    uint(pbTool.Id),
				FilterID:  uint(filterID),
				CreatedAt: now,
			})
		}
	}

	// Batch insert tools
	if err := tx.Create(&toolRecords).Error; err != nil {
		return fmt.Errorf("failed to batch insert tools: %w", err)
	}

	// Batch insert tool_filters
	if len(toolFilterRecords) > 0 {
		if err := tx.Create(&toolFilterRecords).Error; err != nil {
			return fmt.Errorf("failed to batch insert tool_filters: %w", err)
		}
	}

	// Note: app_tools join table entries are created by syncApps (sole authority for app relationships)

	log.Debug().Int("tool_count", len(toolRecords)).Int("filter_count", len(toolFilterRecords)).Msg("Tools batch synced to SQLite")
	return nil
}

// syncDatasources syncs Datasource entities (batch insert)
func (s *EdgeSyncService) syncDatasources(tx *gorm.DB, datasources []*pb.DatasourceConfig) error {
	if len(datasources) == 0 {
		return nil
	}
	log.Debug().Int("count", len(datasources)).Msg("Syncing Datasources to local SQLite")

	records := make([]database.Datasource, 0, len(datasources))
	for _, pbDS := range datasources {
		records = append(records, database.Datasource{
			Model: gorm.Model{
				ID:        uint(pbDS.Id),
				CreatedAt: pbDS.CreatedAt.AsTime(),
				UpdatedAt: pbDS.UpdatedAt.AsTime(),
			},
			Name:                  pbDS.Name,
			ShortDescription:      pbDS.ShortDescription,
			LongDescription:       pbDS.LongDescription,
			Icon:                  pbDS.Icon,
			Url:                   pbDS.Url,
			PrivacyScore:          int(pbDS.PrivacyScore),
			DBSourceType:          pbDS.DbSourceType,
			DBConnStringEncrypted: pbDS.DbConnStringEncrypted,
			DBConnAPIKeyEncrypted: pbDS.DbConnApiKeyEncrypted,
			DBName:                pbDS.DbName,
			EmbedVendor:           pbDS.EmbedVendor,
			EmbedUrl:              pbDS.EmbedUrl,
			EmbedAPIKeyEncrypted:  pbDS.EmbedApiKeyEncrypted,
			EmbedModel:            pbDS.EmbedModel,
			Active:                pbDS.IsActive,
			Namespace:             pbDS.Namespace,
		})
	}

	// Batch insert datasources
	if err := tx.Create(&records).Error; err != nil {
		return fmt.Errorf("failed to batch insert datasources: %w", err)
	}

	// Note: app_datasources join table entries are created by syncApps (sole authority for app relationships)

	log.Debug().Int("count", len(records)).Msg("Datasources batch synced to SQLite")
	return nil
}

// syncOAuthClients syncs OAuth client records for MCP authentication on edge (batch insert)
func (s *EdgeSyncService) syncOAuthClients(tx *gorm.DB, clients []*pb.OAuthClientConfig) error {
	if len(clients) == 0 {
		return nil
	}
	log.Debug().Int("count", len(clients)).Msg("Syncing OAuth Clients to local SQLite")

	records := make([]database.OAuthClientEdge, 0, len(clients))
	for _, pbClient := range clients {
		records = append(records, database.OAuthClientEdge{
			Model: gorm.Model{
				ID:        uint(pbClient.Id),
				CreatedAt: pbClient.CreatedAt.AsTime(),
				UpdatedAt: pbClient.UpdatedAt.AsTime(),
			},
			ClientID:     pbClient.ClientId,
			ClientSecret: pbClient.ClientSecretHash,
			ClientName:   pbClient.ClientName,
			RedirectURIs: pbClient.RedirectUris,
			UserID:       uint(pbClient.UserId),
			Scope:        pbClient.Scope,
		})
	}

	if err := tx.Create(&records).Error; err != nil {
		return fmt.Errorf("failed to batch insert OAuth clients: %w", err)
	}

	log.Debug().Int("count", len(records)).Msg("OAuth Clients batch synced to SQLite")
	return nil
}

// syncAccessTokens syncs OAuth access tokens for MCP authentication on edge (batch insert)
func (s *EdgeSyncService) syncAccessTokens(tx *gorm.DB, tokens []*pb.AccessTokenConfig) error {
	if len(tokens) == 0 {
		return nil
	}
	log.Debug().Int("count", len(tokens)).Msg("Syncing Access Tokens to local SQLite")

	records := make([]database.AccessTokenEdge, 0, len(tokens))
	for _, pbToken := range tokens {
		records = append(records, database.AccessTokenEdge{
			Model: gorm.Model{
				ID:        uint(pbToken.Id),
				CreatedAt: pbToken.CreatedAt.AsTime(),
				UpdatedAt: pbToken.UpdatedAt.AsTime(),
			},
			TokenHash:      pbToken.TokenHash,
			TokenEncrypted: pbToken.TokenEncrypted,
			ClientID:       pbToken.ClientId,
			UserID:         uint(pbToken.UserId),
			Scope:          pbToken.Scope,
			ExpiresAt:      pbToken.ExpiresAt.AsTime(),
		})
	}

	if err := tx.Create(&records).Error; err != nil {
		return fmt.Errorf("failed to batch insert access tokens: %w", err)
	}

	log.Debug().Int("count", len(records)).Msg("Access Tokens batch synced to SQLite")
	return nil
}