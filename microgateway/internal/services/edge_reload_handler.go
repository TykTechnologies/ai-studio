// internal/services/edge_reload_handler.go
package services

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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

	// Reconcile running plugins with updated DB state (async, non-blocking).
	// Capture the callback value while holding the lock to avoid a data race
	// with concurrent SetPluginReconciler calls.
	if reconciler := h.pluginReconciler; reconciler != nil {
		go func() {
			if err := reconciler(); err != nil {
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

// safeUpdateSQLite applies the configuration. SyncConfiguration runs in one
// transaction, so a failed sync leaves the previous configuration in place.
// (A separate copy-and-restore of the tables used to run on failure; its
// restore deleted every App and LLM outside any transaction, which foreign
// keys refuse on Postgres, and left the edge half-restored.)
func (h *EdgeReloadHandler) safeUpdateSQLite(newConfig *pb.ConfigurationSnapshot) error {
	log.Info().
		Str("version", newConfig.Version).
		Int("llm_count", len(newConfig.Llms)).
		Int("app_count", len(newConfig.Apps)).
		Int("filter_count", len(newConfig.Filters)).
		Int("plugin_count", len(newConfig.Plugins)).
		Int("model_price_count", len(newConfig.ModelPrices)).
		Msg("Starting configuration update")

	if err := h.syncService.SyncConfiguration(newConfig); err != nil {
		log.Error().Err(err).Msg("Configuration sync failed; the previous configuration is unchanged")
		return fmt.Errorf("configuration sync failed, rolled back: %w", err)
	}

	log.Info().Str("version", newConfig.Version).Msg("Configuration update completed")
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