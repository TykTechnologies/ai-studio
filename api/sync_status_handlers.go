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
	// redactPending, when set, hides from the caller what its grants do
	// not let it see (API.redactPendingChanges).
	redactPending func(*gin.Context, *services.PendingChanges)
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
// @Router /sync/status [get]
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

// GetPendingChanges handles GET /api/v1/sync/pending-changes?namespace=default
// @Summary Preview what a push would change
// @Description List the gateway-visible objects created, updated or deleted in a namespace since its last configuration push. Use "global" (or omit) for the global namespace. Each change names its owning Workspace (none for plugins, model prices, OAuth clients and access tokens), and by_workspace / unattributed_total count every change. With workspace_id, outside_total and stained report the changes a push limited to that Workspace would carry from outside it: changes owned by another Workspace, and plugin and model price changes (OAuth clients and access tokens never count).
// @Tags sync
// @Param namespace query string false "Namespace (default: global)"
// @Param workspace_id query int false "Workspace to report the stain for"
// @Produce json
// @Success 200 {object} map[string]services.PendingChanges
// @Failure 422 {object} map[string]interface{} "invalid_workspace: unknown Workspace"
// @Router /sync/pending-changes [get]
// @Security BearerAuth
func (h *SyncStatusHandlers) GetPendingChanges(c *gin.Context) {
	namespace := c.Query("namespace")
	if namespace != "" && namespace != "global" {
		if err := validateNamespace(namespace); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	var workspaceID *uint
	if raw := c.Query("workspace_id"); raw != "" {
		// One invalid_workspace body for a malformed and an unknown id, so
		// Workspace ids cannot be probed.
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			invalidWorkspace(c)
			return
		}
		exists, err := h.syncStatusService.WorkspaceExists(uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !exists {
			invalidWorkspace(c)
			return
		}
		wid := uint(id)
		workspaceID = &wid
	}

	changes, err := h.syncStatusService.GetPendingChangesFor(namespace, workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.redactPending != nil {
		h.redactPending(c, changes)
		if c.IsAborted() {
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": changes})
}

// GetNamespaceSyncStatus handles GET /api/v1/sync/status/:namespace
// @Summary Get sync status for namespace
// @Description Get detailed synchronization status for a specific namespace
// @Tags sync
// @Param namespace path string true "Namespace"
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /sync/status/{namespace} [get]
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
// @Router /sync/audit [get]
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
