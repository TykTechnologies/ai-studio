// internal/services/edge_reload_handler.go
package services

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// EdgeClientInterface defines the interface for edge client operations (avoids import cycle)
type EdgeClientInterface interface {
	RequestFullSync() error
	GetCurrentConfiguration() *pb.ConfigurationSnapshot
}

// EdgeReloadHandler manages configuration reload process on edge instances
type EdgeReloadHandler struct {
	edgeClient      EdgeClientInterface
	syncService     *EdgeSyncService
	db              *gorm.DB
	edgeID          string
	currentVersion  string
	reloadMutex     sync.Mutex
	
	// Callback to send status updates to control
	sendStatusUpdate func(*pb.ConfigurationReloadResponse)

	// Callback to reload gateway after config sync
	gatewayReloader func() error

	// Callback to reconcile plugins after config sync
	pluginReconciler func() error
}

// NewEdgeReloadHandler creates a new edge reload handler
func NewEdgeReloadHandler(
	edgeClient EdgeClientInterface,
	syncService *EdgeSyncService,
	db *gorm.DB,
	edgeID string,
	sendStatusCallback func(*pb.ConfigurationReloadResponse),
	gatewayReloader func() error,
) *EdgeReloadHandler {
	return &EdgeReloadHandler{
		edgeClient:       edgeClient,
		syncService:      syncService,
		db:               db,
		edgeID:           edgeID,
		sendStatusUpdate: sendStatusCallback,
		gatewayReloader:  gatewayReloader,
	}
}

// SetGatewayReloader sets the gateway reloader callback
// This allows setting the reloader after the Server is created
func (h *EdgeReloadHandler) SetGatewayReloader(reloader func() error) {
	h.reloadMutex.Lock()
	defer h.reloadMutex.Unlock()
	h.gatewayReloader = reloader
	log.Debug().Msg("Gateway reloader callback set for edge reload handler")
}

// SetPluginReconciler sets the plugin reconciliation callback
func (h *EdgeReloadHandler) SetPluginReconciler(reconciler func() error) {
	h.reloadMutex.Lock()
	defer h.reloadMutex.Unlock()
	h.pluginReconciler = reconciler
	log.Debug().Msg("Plugin reconciler callback set for edge reload handler")
}

// HandleReloadRequest processes a configuration reload request from control
func (h *EdgeReloadHandler) HandleReloadRequest(req *pb.ConfigurationReloadRequest) {
	h.reloadMutex.Lock()
	defer h.reloadMutex.Unlock()

	log.Info().
		Str("operation_id", req.OperationId).
		Str("target_namespace", req.TargetNamespace).
		Str("initiated_by", req.InitiatedBy).
		Msg("Edge received configuration reload request")

	// Get current configuration version
	currentConfig := h.edgeClient.GetCurrentConfiguration()
	if currentConfig != nil {
		h.currentVersion = currentConfig.Version
	}

	// Phase 1: PONG - Acknowledge reload request
	h.sendStatus(req.OperationId, pb.ReloadPhase_PONG, true, "Reload request acknowledged", h.currentVersion, "")

	// Phase 2: PULL_STARTED - Request fresh configuration
	h.sendStatus(req.OperationId, pb.ReloadPhase_PULL_STARTED, true, "Pulling new configuration from control", h.currentVersion, "")

	if err := h.edgeClient.RequestFullSync(); err != nil {
		h.sendStatus(req.OperationId, pb.ReloadPhase_FAILED, false, fmt.Sprintf("Failed to pull configuration: %v", err), h.currentVersion, "")
		return
	}

	// Wait a moment for the new configuration to arrive
	time.Sleep(2 * time.Second)

	newConfig := h.edgeClient.GetCurrentConfiguration()
	if newConfig == nil {
		h.sendStatus(req.OperationId, pb.ReloadPhase_FAILED, false, "No configuration received after sync request", h.currentVersion, "")
		return
	}

	newVersion := newConfig.Version
	if newVersion == h.currentVersion {
		// No change in configuration - still successful
		h.sendStatus(req.OperationId, pb.ReloadPhase_READY, true, "Configuration up to date, no changes needed", h.currentVersion, newVersion)
		return
	}

	// Phase 3: UPDATING - Apply configuration with safe SQLite update
	h.sendStatus(req.OperationId, pb.ReloadPhase_UPDATING, true, "Updating local SQLite database", h.currentVersion, newVersion)

	if err := h.safeUpdateSQLite(newConfig); err != nil {
		h.sendStatus(req.OperationId, pb.ReloadPhase_FAILED, false, fmt.Sprintf("Failed to update SQLite: %v", err), h.currentVersion, "")
		return
	}

	// Reload gateway to refresh in-memory LLM cache with new API keys
	if h.gatewayReloader != nil {
		if err := h.gatewayReloader(); err != nil {
			log.Error().Err(err).Msg("Failed to reload gateway after config sync")
			// Non-fatal: DB is updated, gateway will use fresh data on next restart
		} else {
			log.Info().Msg("Gateway reloaded with new configuration")
		}
	}

	// Reconcile running plugins with updated DB state (async, non-blocking)
	if h.pluginReconciler != nil {
		go func() {
			if err := h.pluginReconciler(); err != nil {
				log.Error().Err(err).Msg("Failed to reconcile plugins after config sync")
			}
		}()
	}

	// Phase 4: UPDATED - Configuration applied to SQLite
	h.sendStatus(req.OperationId, pb.ReloadPhase_UPDATED, true, "Local SQLite database updated successfully", h.currentVersion, newVersion)

	// Phase 5: READY - Edge operational with new configuration
	h.currentVersion = newVersion
	h.sendStatus(req.OperationId, pb.ReloadPhase_READY, true, "Edge ready with new configuration", h.currentVersion, newVersion)

	log.Info().
		Str("operation_id", req.OperationId).
		Str("version_before", h.currentVersion).
		Str("version_after", newVersion).
		Msg("Edge configuration reload completed successfully")
}

// safeUpdateSQLite performs safe SQLite update with backup/rollback capability
func (h *EdgeReloadHandler) safeUpdateSQLite(newConfig *pb.ConfigurationSnapshot) error {
	log.Info().
		Str("version", newConfig.Version).
		Int("llm_count", len(newConfig.Llms)).
		Int("app_count", len(newConfig.Apps)).
		Int("filter_count", len(newConfig.Filters)).
		Int("plugin_count", len(newConfig.Plugins)).
		Int("model_price_count", len(newConfig.ModelPrices)).
		Msg("Starting safe SQLite configuration update")

	// Create backup tables first
	if err := h.createBackupTables(); err != nil {
		return fmt.Errorf("failed to create backup tables: %w", err)
	}

	// Attempt to sync new configuration
	if err := h.syncService.SyncConfiguration(newConfig); err != nil {
		log.Error().Err(err).Msg("Configuration sync failed, attempting rollback")
		
		// Attempt rollback from backup
		if rollbackErr := h.restoreFromBackup(); rollbackErr != nil {
			log.Error().Err(rollbackErr).Msg("CRITICAL: Rollback failed - edge may be in inconsistent state")
			return fmt.Errorf("sync failed and rollback failed: sync_error=%v, rollback_error=%v", err, rollbackErr)
		}
		
		log.Info().Msg("Successfully rolled back to previous configuration")
		return fmt.Errorf("configuration sync failed, rolled back: %w", err)
	}

	// Cleanup backup tables on success
	if err := h.cleanupBackupTables(); err != nil {
		log.Warn().Err(err).Msg("Failed to cleanup backup tables (non-critical)")
	}

	log.Info().Str("version", newConfig.Version).Msg("Safe SQLite configuration update completed")
	return nil
}

// createBackupTables creates backup copies of current configuration
func (h *EdgeReloadHandler) createBackupTables() error {
	log.Debug().Msg("Creating backup tables for safe configuration update")

	// Create backup tables with _backup suffix
	backupQueries := []string{
		"CREATE TABLE IF NOT EXISTS llms_backup AS SELECT * FROM llms",
		"CREATE TABLE IF NOT EXISTS apps_backup AS SELECT * FROM apps", 
		"CREATE TABLE IF NOT EXISTS filters_backup AS SELECT * FROM filters",
		"CREATE TABLE IF NOT EXISTS plugins_backup AS SELECT * FROM plugins",
		"CREATE TABLE IF NOT EXISTS model_prices_backup AS SELECT * FROM model_prices",
		"CREATE TABLE IF NOT EXISTS app_llms_backup AS SELECT * FROM app_llms",
		"CREATE TABLE IF NOT EXISTS llm_filters_backup AS SELECT * FROM llm_filters",
		"CREATE TABLE IF NOT EXISTS llm_plugins_backup AS SELECT * FROM llm_plugins",
	}

	for _, query := range backupQueries {
		if err := h.db.Exec(query).Error; err != nil {
			return fmt.Errorf("failed to create backup table: %w", err)
		}
	}

	log.Debug().Msg("Backup tables created successfully")
	return nil
}

// restoreFromBackup restores configuration from backup tables
func (h *EdgeReloadHandler) restoreFromBackup() error {
	log.Info().Msg("Restoring configuration from backup tables")

	// Clear current tables and restore from backup
	restoreQueries := []string{
		// Clear join tables first (foreign key constraints)
		"DELETE FROM app_llms",
		"DELETE FROM llm_filters", 
		"DELETE FROM llm_plugins",
		
		// Clear main tables
		"DELETE FROM llms",
		"DELETE FROM apps",
		"DELETE FROM filters", 
		"DELETE FROM plugins",
		"DELETE FROM model_prices",
		
		// Restore from backup
		"INSERT INTO llms SELECT * FROM llms_backup",
		"INSERT INTO apps SELECT * FROM apps_backup",
		"INSERT INTO filters SELECT * FROM filters_backup",
		"INSERT INTO plugins SELECT * FROM plugins_backup", 
		"INSERT INTO model_prices SELECT * FROM model_prices_backup",
		"INSERT INTO app_llms SELECT * FROM app_llms_backup",
		"INSERT INTO llm_filters SELECT * FROM llm_filters_backup",
		"INSERT INTO llm_plugins SELECT * FROM llm_plugins_backup",
	}

	for _, query := range restoreQueries {
		if err := h.db.Exec(query).Error; err != nil {
			return fmt.Errorf("failed to restore from backup: %w", err)
		}
	}

	log.Info().Msg("Configuration restored from backup successfully")
	return nil
}

// cleanupBackupTables removes backup tables after successful update
func (h *EdgeReloadHandler) cleanupBackupTables() error {
	log.Debug().Msg("Cleaning up backup tables")

	dropQueries := []string{
		"DROP TABLE IF EXISTS llms_backup",
		"DROP TABLE IF EXISTS apps_backup",
		"DROP TABLE IF EXISTS filters_backup", 
		"DROP TABLE IF EXISTS plugins_backup",
		"DROP TABLE IF EXISTS model_prices_backup",
		"DROP TABLE IF EXISTS app_llms_backup",
		"DROP TABLE IF EXISTS llm_filters_backup",
		"DROP TABLE IF EXISTS llm_plugins_backup",
	}

	for _, query := range dropQueries {
		if err := h.db.Exec(query).Error; err != nil {
			return fmt.Errorf("failed to drop backup table: %w", err)
		}
	}

	log.Debug().Msg("Backup tables cleaned up successfully")
	return nil
}

// sendStatus sends a reload status update to the control server
func (h *EdgeReloadHandler) sendStatus(operationID string, phase pb.ReloadPhase, success bool, message string, versionBefore string, versionAfter string) {
	response := &pb.ConfigurationReloadResponse{
		OperationId:          operationID,
		EdgeId:               h.edgeID,
		Phase:                phase,
		Success:              success,
		Message:              message,
		ConfigVersionBefore:  versionBefore,
		ConfigVersionAfter:   versionAfter,
		Timestamp:            timestamppb.Now(),
	}

	log.Info().
		Str("operation_id", operationID).
		Str("phase", phase.String()).
		Bool("success", success).
		Str("message", message).
		Msg("Sending reload status update to control")

	if h.sendStatusUpdate != nil {
		h.sendStatusUpdate(response)
	}
}