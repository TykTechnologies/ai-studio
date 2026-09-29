// internal/services/edge_sync_service.go
package services

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EdgeSyncService handles syncing flattened configuration to local SQLite
type EdgeSyncService struct {
	db        *gorm.DB
	namespace string
}

// appliedSnapshots holds, per database and namespace, a fingerprint of the
// snapshot last applied. A reload hands the same pulled snapshot to both the
// config callback and the reload handler; the second application is skipped.
// The whole snapshot is hashed: the hub's checksum leaves out Apps.
var appliedSnapshots sync.Map // appliedKey -> [sha256.Size]byte

// snapshotFingerprint hashes the whole snapshot; ok is false when it cannot
// be serialized, and then the snapshot is always applied.
func snapshotFingerprint(config *pb.ConfigurationSnapshot) (sum [sha256.Size]byte, ok bool) {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(config)
	if err != nil {
		return sum, false
	}
	return sha256.Sum256(b), true
}

type appliedKey struct {
	config    *gorm.Config // shared by every session of one database
	namespace string
}

// snapshotCutoff is when the snapshot was taken: an App created after it
// cannot be in it. Hubs that do not send snapshot_time set the version to the
// snapshot's Unix time. Zero when neither is known.
func snapshotCutoff(config *pb.ConfigurationSnapshot) time.Time {
	if config.SnapshotTime != nil {
		return config.SnapshotTime.AsTime()
	}
	if secs, err := strconv.ParseInt(config.Version, 10, 64); err == nil && secs > 0 {
		return time.Unix(secs, 0)
	}
	return time.Time{}
}

// namespaceScope selects the rows a snapshot for this edge covers: its
// namespace and the global one.
const namespaceScope = "(namespace = ? OR namespace = '')"

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

	key := appliedKey{config: s.db.Config, namespace: s.namespace}
	fingerprint, fingerprinted := snapshotFingerprint(config)
	if applied, ok := appliedSnapshots.Load(key); ok && fingerprinted && applied.([sha256.Size]byte) == fingerprint {
		log.Debug().Str("version", config.Version).Msg("Configuration snapshot already applied, skipping")
		return nil
	}

	// Start transaction for atomic sync
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	// Apps the edge stored after the snapshot was taken (pull-on-miss) are
	// not in it; they keep their rows and grants.
	kept, err := s.appsNewerThan(tx, config.Apps, snapshotCutoff(config))
	if err != nil {
		return fmt.Errorf("failed to read local apps: %w", err)
	}

	// 1. Clear existing data for this namespace (and global). Apps and LLMs
	// are updated in place instead (steps 2 and 3): analytics and budget rows
	// reference them, and an App must never be missing while the snapshot
	// still has it.
	if err := s.clearExistingData(tx); err != nil {
		return fmt.Errorf("failed to clear existing data: %w", err)
	}

	// 2. Sync LLMs with embedded relationships
	if err := s.syncLLMs(tx, config.Llms); err != nil {
		return fmt.Errorf("failed to sync LLMs: %w", err)
	}

	// 3. Sync Apps with embedded relationships (THE CRITICAL PART)
	raised, err := s.syncApps(tx, config.Apps, kept.appIDs)
	if err != nil {
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

	// 5b. Sync Semantic Routers (Enterprise feature)
	if err := s.syncSemanticRouters(tx, config.SemanticRouters); err != nil {
		return fmt.Errorf("failed to sync SemanticRouters: %w", err)
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

	// 10. Restore the grants of the Apps newer than the snapshot
	if err := kept.restore(tx); err != nil {
		return fmt.Errorf("failed to restore grants of newer Apps: %w", err)
	}

	// 11. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit sync transaction: %w", err)
	}
	// The per-statement callbacks already bumped the config generation, but
	// before this commit. Bump again so no request-path cache keeps an entry
	// that was read from the pre-sync data in between.
	database.BumpConfigGeneration()
	if ledger := edgeBudgetLedger.Load(); ledger != nil {
		for _, r := range raised {
			ledger.Reconcile(r.appID, r.start, r.cost)
		}
	}
	if fingerprinted {
		appliedSnapshots.Store(key, fingerprint)
	} else {
		appliedSnapshots.Delete(key)
	}

	log.Debug().
		Str("version", config.Version).
		Str("namespace", s.namespace).
		Msg("Configuration sync to local SQLite completed successfully")

	return nil
}

// clearExistingData clears existing configuration for this namespace. Apps
// and LLMs are not deleted here: syncApps and syncLLMs update them in place
// and retire (soft-delete) those the snapshot no longer has. The grants of
// Apps newer than the snapshot are cleared too, and restored at the end of
// the sync (keptApps.restore), because the tools and routers they point at
// are re-created in between.
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

	if err := tx.Exec("DELETE FROM app_model_routers WHERE app_id IN (SELECT id FROM apps WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear app_model_routers: %w", err)
	}
	if err := tx.Exec("DELETE FROM app_semantic_routers WHERE app_id IN (SELECT id FROM apps WHERE namespace = ? OR namespace = '')", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear app_semantic_routers: %w", err)
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
	if err := tx.Exec("DELETE FROM semantic_routers WHERE namespace = ? OR namespace = ''", s.namespace).Error; err != nil {
		return fmt.Errorf("failed to clear semantic_routers: %w", err)
	}

	log.Debug().Msg("Existing configuration data cleared")
	return nil
}

// syncLLMs syncs LLM entities and their join table relationships. LLMs are
// updated in place and those missing from the snapshot are retired
// (soft-deleted): analytics and budget rows reference them, and on Postgres
// deleting them fails.
func (s *EdgeSyncService) syncLLMs(tx *gorm.DB, llms []*pb.LLMConfig) error {
	log.Debug().Int("count", len(llms)).Msg("Syncing LLMs to local SQLite")

	// Slugs are unique and may move between LLMs (or to a new one when one
	// is deleted). Give every current LLM a unique placeholder first; the
	// upserts below set the snapshot's slugs, and a retired LLM keeps its
	// placeholder, which frees its slug.
	if err := tx.Model(&database.LLM{}).Where(namespaceScope, s.namespace).
		Update("slug", gorm.Expr("slug || '#' || id")).Error; err != nil {
		return fmt.Errorf("failed to free LLM slugs: %w", err)
	}

	ids := make([]uint, 0, len(llms))
	for _, pbLLM := range llms {
		ids = append(ids, uint(pbLLM.Id))
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
			DontLogBodies:   pbLLM.DontLogBodies,
		}

		// Handle JSON fields with proper conversion
		if pbLLM.GovernedMetadata != "" {
			llm.GovernedMetadata = database.JSON(pbLLM.GovernedMetadata)
		}
		if pbLLM.Metadata != "" {
			llm.Metadata = database.JSON(pbLLM.Metadata)
		}
		if pbLLM.AllowedModels != "" {
			llm.AllowedModels = database.JSON(pbLLM.AllowedModels)
		}
		if pbLLM.Failover != "" {
			llm.Failover = database.JSON(pbLLM.Failover)
		}
		if pbLLM.AuthConfig != "" {
			llm.AuthConfig = database.JSON(pbLLM.AuthConfig)
		}
		if pbLLM.AuthMechanism != "" {
			llm.AuthMechanism = pbLLM.AuthMechanism
		}

		if err := upsertByID(tx, llm, uint(pbLLM.Id), pbLLM.IsActive); err != nil {
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

	if err := retireMissing(tx, &database.LLM{}, s.namespace, ids); err != nil {
		return fmt.Errorf("failed to retire LLMs: %w", err)
	}
	return nil
}

// upsertByID inserts row, or overwrites every column of the row with its ID
// (a retired one included, which it restores). GORM writes a column's
// default in place of a zero value, so a row that must be inactive is set
// so afterwards: is_active defaults to true.
func upsertByID(tx *gorm.DB, row interface{}, id uint, isActive bool) error {
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, UpdateAll: true}).
		Create(row).Error; err != nil {
		return err
	}
	if isActive {
		return nil
	}
	return tx.Model(row).Where("id = ?", id).Update("is_active", false).Error
}

// retireMissing soft-deletes the rows of model in this edge's namespaces
// whose IDs are not in keep.
func retireMissing(tx *gorm.DB, model interface{}, namespace string, keep []uint) error {
	q := tx.Where(namespaceScope, namespace)
	if len(keep) > 0 {
		q = q.Where("id NOT IN ?", keep)
	}
	return q.Delete(model).Error
}

// budgetRaise is a stored period usage a sync raised, for the ledger.
type budgetRaise struct {
	appID uint
	start time.Time
	cost  float64
}

// syncApps syncs App entities and recreates app_llms join table - THE CRITICAL PART.
// Apps are updated in place, so one the snapshot has is never missing; those
// it no longer has are retired (soft-deleted), except kept: Apps the edge
// stored after the snapshot was taken.
func (s *EdgeSyncService) syncApps(tx *gorm.DB, apps []*pb.AppConfig, kept []uint) ([]budgetRaise, error) {
	log.Debug().Int("count", len(apps)).Msg("Syncing Apps to local SQLite")

	var raised []budgetRaise
	ids := append(make([]uint, 0, len(apps)+len(kept)), kept...)

	// Collect join table records across all apps for batch insert
	var allAppLLMs []database.AppLLM
	var allAppTools []database.AppTool
	var allAppDatasources []database.AppDatasource
	var allAppModelRouters []database.AppModelRouter
	var allAppSemanticRouters []database.AppSemanticRouter

	for _, pbApp := range apps {
		ids = append(ids, uint(pbApp.Id))
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
			app.AllowedIPs = database.JSON(pbApp.AllowedIps)
		}
		if pbApp.Metadata != "" {
			app.Metadata = database.JSON(pbApp.Metadata)
		}

		// Serialize plugin resource associations for gateway access
		if len(pbApp.PluginResources) > 0 {
			if prJSON, err := json.Marshal(pbApp.PluginResources); err == nil {
				app.PluginResourcesJSON = database.JSON(prJSON)
			}
		}

		// Handle budget start date if available  
		if pbApp.BudgetStartDate != "" {
			if startDate, err := time.Parse(time.RFC3339, pbApp.BudgetStartDate); err == nil {
				app.BudgetStartDate = &startDate
			}
		}

		if err := upsertByID(tx, app, uint(pbApp.Id), pbApp.IsActive); err != nil {
			return nil, fmt.Errorf("failed to insert App %d: %w", pbApp.Id, err)
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
		for _, routerID := range pbApp.ModelRouterIds {
			allAppModelRouters = append(allAppModelRouters, database.AppModelRouter{
				AppID: uint(pbApp.Id), ModelRouterID: uint(routerID), CreatedAt: now,
			})
		}
		for _, routerID := range pbApp.SemanticRouterIds {
			allAppSemanticRouters = append(allAppSemanticRouters, database.AppSemanticRouter{
				AppID: uint(pbApp.Id), SemanticRouterID: uint(routerID), CreatedAt: now,
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

			// Raise, never lower: the control plane's figure only has the
			// edge spend its analytics pulse has delivered, so it can be
			// behind the edge's own. Taking it reopened spent budgets.
			if err := raiseStoredUsage(tx, uint(pbApp.Id), periodStart, periodEnd, storedCost); err != nil {
				log.Warn().Err(err).
					Uint32("app_id", pbApp.Id).
					Float64("current_usage", pbApp.CurrentPeriodUsage).
					Float64("stored_cost", storedCost).
					Msg("Failed to initialize budget usage from control server")
			} else {
				raised = append(raised, budgetRaise{appID: uint(pbApp.Id), start: periodStart, cost: storedCost})
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

	if err := retireMissing(tx, &database.App{}, s.namespace, ids); err != nil {
		return nil, fmt.Errorf("failed to retire Apps: %w", err)
	}

	// Batch insert all join table records
	if len(allAppLLMs) > 0 {
		if err := tx.Create(&allAppLLMs).Error; err != nil {
			return nil, fmt.Errorf("failed to batch insert app_llms: %w", err)
		}
	}
	if len(allAppTools) > 0 {
		if err := tx.Create(&allAppTools).Error; err != nil {
			return nil, fmt.Errorf("failed to batch insert app_tools: %w", err)
		}
	}
	if len(allAppDatasources) > 0 {
		if err := tx.Create(&allAppDatasources).Error; err != nil {
			return nil, fmt.Errorf("failed to batch insert app_datasources: %w", err)
		}
	}
	if len(allAppModelRouters) > 0 {
		if err := tx.Create(&allAppModelRouters).Error; err != nil {
			return nil, fmt.Errorf("failed to batch insert app_model_routers: %w", err)
		}
	}
	if len(allAppSemanticRouters) > 0 {
		if err := tx.Create(&allAppSemanticRouters).Error; err != nil {
			return nil, fmt.Errorf("failed to batch insert app_semantic_routers: %w", err)
		}
	}

	return raised, nil
}

// keptApps are Apps the edge stored after the snapshot was taken (pull-on-miss
// token validation), so the snapshot cannot have them. They keep their rows;
// their grants are read before the join tables are cleared and restored at
// the end of the sync.
type keptApps struct {
	appIDs          []uint
	llms            []database.AppLLM
	tools           []database.AppTool
	datasources     []database.AppDatasource
	modelRouters    []database.AppModelRouter
	semanticRouters []database.AppSemanticRouter
}

// appsNewerThan returns the Apps in this edge's namespaces that the snapshot
// does not have and that were created at or after cutoff, with their grants.
// A zero cutoff (the snapshot's time is unknown) keeps none.
func (s *EdgeSyncService) appsNewerThan(tx *gorm.DB, snapshot []*pb.AppConfig, cutoff time.Time) (*keptApps, error) {
	k := &keptApps{}
	if cutoff.IsZero() {
		return k, nil
	}
	inSnapshot := make(map[uint]bool, len(snapshot))
	for _, a := range snapshot {
		inSnapshot[uint(a.Id)] = true
	}
	var local []database.App
	if err := tx.Select("id", "created_at").Where(namespaceScope, s.namespace).Find(&local).Error; err != nil {
		return nil, err
	}
	for _, a := range local {
		if !inSnapshot[a.ID] && !a.CreatedAt.Before(cutoff) {
			k.appIDs = append(k.appIDs, a.ID)
		}
	}
	if len(k.appIDs) == 0 {
		return k, nil
	}
	log.Info().Interface("app_ids", k.appIDs).Time("snapshot_time", cutoff).
		Msg("Keeping Apps created after the configuration snapshot was taken")
	for _, dest := range []interface{}{&k.llms, &k.tools, &k.datasources, &k.modelRouters, &k.semanticRouters} {
		if err := tx.Where("app_id IN ?", k.appIDs).Find(dest).Error; err != nil {
			return nil, err
		}
	}
	return k, nil
}

// restore re-inserts the kept Apps' grants whose targets still exist.
func (k *keptApps) restore(tx *gorm.DB) error {
	if len(k.appIDs) == 0 {
		return nil
	}
	if err := restoreGrants(tx, k.llms, func(g database.AppLLM) uint { return g.LLMID }, &database.LLM{}); err != nil {
		return err
	}
	if err := restoreGrants(tx, k.tools, func(g database.AppTool) uint { return g.ToolID }, &database.Tool{}); err != nil {
		return err
	}
	if err := restoreGrants(tx, k.datasources, func(g database.AppDatasource) uint { return g.DatasourceID }, &database.Datasource{}); err != nil {
		return err
	}
	if err := restoreGrants(tx, k.modelRouters, func(g database.AppModelRouter) uint { return g.ModelRouterID }, &database.ModelRouter{}); err != nil {
		return err
	}
	return restoreGrants(tx, k.semanticRouters, func(g database.AppSemanticRouter) uint { return g.SemanticRouterID }, &database.SemanticRouter{})
}

// restoreGrants inserts the grants whose target (a row of model) exists.
func restoreGrants[G any](tx *gorm.DB, grants []G, target func(G) uint, model interface{}) error {
	if len(grants) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(grants))
	for _, g := range grants {
		ids = append(ids, target(g))
	}
	var have []uint
	if err := tx.Model(model).Where("id IN ?", ids).Pluck("id", &have).Error; err != nil {
		return err
	}
	found := make(map[uint]bool, len(have))
	for _, id := range have {
		found[id] = true
	}
	var rows []G
	for _, g := range grants {
		if found[target(g)] {
			rows = append(rows, g)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// raiseStoredUsage raises an App's stored usage for the period starting at
// start to at least cost, creating the row if it is missing. The maximum is
// taken by the database in one statement, so increments the analytics writer
// makes meanwhile are not lost.
func raiseStoredUsage(db *gorm.DB, appID uint, start, end time.Time, cost float64) error {
	now := time.Now()
	maxExpr := "MAX(total_cost, ?)"
	if db.Dialector.Name() == "postgres" {
		maxExpr = "GREATEST(total_cost, ?)"
	}
	raise := func() (int64, error) {
		res := db.Model(&database.BudgetUsage{}).
			Where("app_id = ? AND period_start = ?", appID, start).
			Updates(map[string]interface{}{
				"total_cost": gorm.Expr(maxExpr, cost),
				"updated_at": now,
			})
		return res.RowsAffected, res.Error
	}

	n, err := raise()
	if err != nil || n > 0 {
		return err
	}
	// Create the row; if the writer created it meanwhile, raise that.
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&database.BudgetUsage{
		AppID: appID, PeriodStart: start, PeriodEnd: end, TotalCost: cost, CreatedAt: now, UpdatedAt: now,
	})
	if res.Error == nil && res.RowsAffected == 0 {
		_, err = raise()
		return err
	}
	return res.Error
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
			Kind:           pbFilter.Kind,
			Config:         pbFilter.Config,
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
			plugin.Config = database.JSON(pbPlugin.Config)
		}

		// Handle HookTypes JSON field with proper conversion
		if len(pbPlugin.HookTypes) > 0 {
			hookTypesJSON, err := json.Marshal(pbPlugin.HookTypes)
			if err == nil {
				plugin.HookTypes = database.JSON(hookTypesJSON)
			}
		}

		// Handle ServiceScopes JSON field with proper conversion
		if len(pbPlugin.ServiceScopes) > 0 {
			scopesJSON, err := json.Marshal(pbPlugin.ServiceScopes)
			if err == nil {
				plugin.ServiceScopes = database.JSON(scopesJSON)
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

// syncSemanticRouters stores Semantic Routers as the hub sent them; the
// SemanticRouterService compiles them on reload.
func (s *EdgeSyncService) syncSemanticRouters(tx *gorm.DB, routers []*pb.SemanticRouterConfig) error {
	for _, pbRouter := range routers {
		router := &database.SemanticRouter{
			ID:         uint(pbRouter.Id),
			Name:       pbRouter.Name,
			Slug:       pbRouter.Slug,
			Namespace:  pbRouter.Namespace,
			IsActive:   pbRouter.IsActive,
			ConfigJSON: pbRouter.ConfigJson,
			CreatedAt:  pbRouter.CreatedAt.AsTime(),
			UpdatedAt:  pbRouter.UpdatedAt.AsTime(),
		}
		if err := tx.Create(router).Error; err != nil {
			return fmt.Errorf("failed to insert SemanticRouter %d: %w", pbRouter.Id, err)
		}
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
			RESTAccessDisabled:  pbTool.RestAccessDisabled,
			MCPAccessDisabled:   pbTool.McpAccessDisabled,
			Namespace:           pbTool.Namespace,
			GovernedMetadata:    database.GovernedMetadataJSON(pbTool.GovernedMetadata),
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
			GovernedMetadata:      database.GovernedMetadataJSON(pbDS.GovernedMetadata),
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
			AppID:          uint(pbToken.AppId),
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