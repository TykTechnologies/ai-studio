package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// SyncStatusHandlers handles sync status API endpoints
type SyncStatusHandlers struct {
	syncStatusService *services.SyncStatusService
}

// NewSyncStatusHandlers creates a new SyncStatusHandlers
func NewSyncStatusHandlers(syncStatusService *services.SyncStatusService) *SyncStatusHandlers {
	return &SyncStatusHandlers{
		syncStatusService: syncStatusService,
	}
}

// GetSyncStatus handles GET /api/v1/sync/status
// @Summary Get sync status summary
// @Description Get synchronization status for all namespaces
// @Tags sync
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/sync/status [get]
// @Security BearerAuth
func (h *SyncStatusHandlers) GetSyncStatus(c *gin.Context) {
	summary, err := h.syncStatusService.GetNamespaceSyncSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hasPending, _ := h.syncStatusService.HasPendingSyncs()
	pendingNamespaces, _ := h.syncStatusService.GetPendingNamespaces()

	c.JSON(http.StatusOK, gin.H{
		"data":               summary,
		"has_pending":        hasPending,
		"pending_namespaces": pendingNamespaces,
	})
}

// GetNamespaceSyncStatus handles GET /api/v1/sync/status/:namespace
// @Summary Get sync status for namespace
// @Description Get detailed synchronization status for a specific namespace
// @Tags sync
// @Param namespace path string true "Namespace"
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/sync/status/{namespace} [get]
// @Security BearerAuth
func (h *SyncStatusHandlers) GetNamespaceSyncStatus(c *gin.Context) {
	namespace := c.Param("namespace")

	summary, edges, err := h.syncStatusService.GetNamespaceSyncStatus(namespace)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "namespace not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"namespace": summary,
		"edges":     edges,
	})
}

// GetSyncAuditLog handles GET /api/v1/sync/audit
// @Summary Get sync audit log
// @Description Get sync audit log entries with filtering
// @Tags sync
// @Param namespace query string false "Filter by namespace"
// @Param edge_id query string false "Filter by edge ID"
// @Param event_type query string false "Filter by event type"
// @Param limit query int false "Limit results" default(50)
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/sync/audit [get]
// @Security BearerAuth
func (h *SyncStatusHandlers) GetSyncAuditLog(c *gin.Context) {
	namespace := c.Query("namespace")
	edgeID := c.Query("edge_id")
	eventType := c.Query("event_type")

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	logs, err := h.syncStatusService.GetSyncAuditLog(namespace, edgeID, eventType, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": logs})
}
